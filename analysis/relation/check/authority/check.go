// Package authority checks the parts of an unchecked logical schema that are
// about identity and publication authority.  It deliberately does not know
// how declarations were compiled or how a relation is mounted.
package authority

import (
	"fmt"
	"sort"
	"strings"

	checkregistry "github.com/wippyai/go-lua/analysis/relation/check/registry"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// Code identifies one deterministic authority failure.  The checker keeps
// these codes independent from compiler diagnostics so a certificate checker
// can be implemented without importing the declaration compiler.
type Code uint16

const (
	CodeInvalidSchema Code = iota + 1
	CodeDuplicateDeclaration
	CodeUnavailableIdentity
	CodeForeignSchema
	CodeForeignOwner
	CodeUnknownRelation
	CodeUnknownColumn
	CodeUnknownKey
	CodeUnknownScope
	CodeUnknownType
	CodeInvalidMembership
	CodeInvalidDenominator
	CodeInvalidDelivery
	CodeInvalidSignature
	CodeDuplicateOutput
	CodeInvalidOutputAuthority
	CodeUnknownOperation
	CodeInvalidExpression
	CodeInvalidScopeOrder
	CodeInvalidPublication
	CodeUndeclaredPublication
	CodeInvalidDependencyProjection
)

func (code Code) String() string {
	names := [...]string{
		"",
		"invalid-schema",
		"duplicate-declaration",
		"unavailable-identity",
		"foreign-schema",
		"foreign-owner",
		"unknown-relation",
		"unknown-column",
		"unknown-key",
		"unknown-scope",
		"unknown-type",
		"invalid-membership",
		"invalid-denominator",
		"invalid-delivery",
		"invalid-signature",
		"duplicate-output",
		"invalid-output-authority",
		"unknown-operation",
		"invalid-expression",
		"invalid-scope-order",
		"invalid-publication",
		"undeclared-publication",
		"invalid-dependency-projection",
	}
	if int(code) < len(names) {
		return names[code]
	}
	return "unknown-authority-error"
}

// Issue is one proof failure. Path is a stable logical path, not a source
// file location; this lets independent checkers compare refusal sets.
type Issue struct {
	Code Code
	Path string
}

func (issue Issue) String() string {
	if issue.Path == "" {
		return issue.Code.String()
	}
	return issue.Code.String() + ": " + issue.Path
}

// Report is the complete deterministic result of checking an unchecked
// ExecutionSchema. No error is hidden behind the first failure.
type Report struct {
	issues []Issue
}

// Check independently validates identity fences, relation membership,
// denominators, scopes, operation signatures, expression ownership and
// publication authority.
func Check(schema plan.ExecutionSchema) Report {
	indexed := checkregistry.Build(schema)
	report := CheckView(indexed)
	appendRegistryIssues(&report, indexed)
	return report
}

// CheckView runs the authority proof against an already indexed schema. The
// certificate layer uses this composition surface to avoid rebuilding the
// declaration registry for each proof pass.
func CheckView(indexed *checkregistry.View) Report {
	if indexed == nil {
		indexed = checkregistry.Build(plan.ExecutionSchema{})
	}
	schema := indexed.Schema()
	checker := newChecker(schema, indexed)
	checker.checkSchema()
	return Report{issues: checker.sortedIssues()}
}

// Valid reports whether the schema contains no authority failures.
func (report Report) Valid() bool { return len(report.issues) == 0 }

// Issues returns a defensive copy in deterministic order.
func (report Report) Issues() []Issue { return append([]Issue(nil), report.issues...) }

// Error implements error for callers that want the compact refusal set.
func (report Report) Error() string {
	if report.Valid() {
		return ""
	}
	parts := make([]string, 0, len(report.issues))
	for _, issue := range report.issues {
		parts = append(parts, issue.String())
	}
	return strings.Join(parts, "; ")
}

type checker struct {
	schema   plan.ExecutionSchema
	registry *checkregistry.View
	issues   []Issue
}

func newChecker(schema plan.ExecutionSchema, indexed *checkregistry.View) *checker {
	return &checker{schema: schema, registry: indexed}
}

func (checker *checker) add(code Code, path string) {
	checker.issues = append(checker.issues, Issue{Code: code, Path: path})
}

func (checker *checker) sortedIssues() []Issue {
	issues := append([]Issue(nil), checker.issues...)
	sort.Slice(issues, func(left, right int) bool {
		if issues[left].Code != issues[right].Code {
			return issues[left].Code < issues[right].Code
		}
		return issues[left].Path < issues[right].Path
	})
	return issues
}

func (checker *checker) checkSchema() {
	if !checker.schema.Available() || !checker.schema.SchemaID().Available() {
		return
	}
	checker.checkRelations()
	checker.checkScopes()
	checker.checkKeys()
	checker.checkSignatures()
	checker.checkExpressions()
}

func appendRegistryIssues(report *Report, indexed *checkregistry.View) {
	seen := make(map[Issue]struct{}, len(report.issues)+len(indexed.Issues()))
	for _, issue := range report.issues {
		seen[issue] = struct{}{}
	}
	for _, issue := range indexed.Issues() {
		code := CodeUnavailableIdentity
		switch issue.Code {
		case checkregistry.CodeSchemaUnavailable, checkregistry.CodeSchemaIdentityUnavailable:
			code = CodeInvalidSchema
		case checkregistry.CodeRelationDuplicate, checkregistry.CodeColumnDuplicate,
			checkregistry.CodeKeyDuplicate, checkregistry.CodeScopeDuplicate,
			checkregistry.CodeExpressionDuplicate, checkregistry.CodeDependencyDuplicate,
			checkregistry.CodeSignatureDuplicate:
			code = CodeDuplicateDeclaration
		case checkregistry.CodeExpressionUnavailable, checkregistry.CodeExpressionNil,
			checkregistry.CodeExpressionDigest:
			code = CodeInvalidExpression
		case checkregistry.CodeDependencyUnavailable, checkregistry.CodeSCCUnavailable:
			code = CodeInvalidDependencyProjection
		case checkregistry.CodeSignatureUnavailable:
			code = CodeInvalidSignature
		}
		mapped := Issue{Code: code, Path: issue.Path}
		if _, exists := seen[mapped]; exists {
			continue
		}
		seen[mapped] = struct{}{}
		report.issues = append(report.issues, mapped)
	}
	sort.Slice(report.issues, func(left, right int) bool {
		if report.issues[left].Code != report.issues[right].Code {
			return report.issues[left].Code < report.issues[right].Code
		}
		return report.issues[left].Path < report.issues[right].Path
	})
}

func (checker *checker) checkRelations() {
	for _, relation := range checker.registry.Relations() {
		relationID := relation.ID()
		path := "relation/" + relationIDString(relationID)
		if !relation.Available() {
			continue
		}
		if _, ok := checker.registry.Scope(relation.Scope()); !ok {
			checker.add(CodeUnknownScope, path+".scope")
		}
		seenColumns := make(map[model.ColumnID]struct{})
		for index, column := range relation.Columns() {
			columnPath := fmt.Sprintf("%s.columns[%d]", path, index)
			if _, duplicate := seenColumns[column]; duplicate {
				checker.add(CodeDuplicateDeclaration, columnPath)
			}
			seenColumns[column] = struct{}{}
			declared, ok := checker.registry.Column(column)
			if !ok {
				checker.add(CodeUnknownColumn, columnPath)
				continue
			}
			if !declared.Available() {
				continue
			}
			if declared.Relation() != relationID {
				checker.add(CodeInvalidMembership, columnPath)
			}
		}
		seenKeys := make(map[model.KeyID]struct{})
		for index, key := range relation.Keys() {
			keyPath := fmt.Sprintf("%s.keys[%d]", path, index)
			if _, duplicate := seenKeys[key]; duplicate {
				checker.add(CodeDuplicateDeclaration, keyPath)
			}
			seenKeys[key] = struct{}{}
			declared, ok := checker.registry.Key(key)
			if !ok {
				checker.add(CodeUnknownKey, keyPath)
				continue
			}
			if !declared.Available() {
				continue
			}
			if declared.Relation() != relationID {
				checker.add(CodeInvalidMembership, keyPath)
			}
		}
	}
	for _, column := range checker.registry.Columns() {
		if !column.Available() {
			continue
		}
		columnID := column.ID()
		path := "column/" + columnIDString(columnID)
		if _, ok := checker.registry.Relation(column.Relation()); !ok {
			checker.add(CodeUnknownRelation, path+".relation")
		}
		if !column.Type().Available() {
			checker.add(CodeUnknownType, path+".type")
		}
	}
}

func (checker *checker) checkKeys() {
	for _, key := range checker.registry.Keys() {
		if !key.Available() {
			continue
		}
		keyID := key.ID()
		path := "key/" + keyIDString(keyID)
		relation, ok := checker.registry.Relation(key.Relation())
		if !ok {
			checker.add(CodeUnknownRelation, path+".relation")
			continue
		}
		if !relation.HasKey(keyID) {
			checker.add(CodeInvalidMembership, path+".relation")
		}
		columns := key.Columns()
		if len(columns) == 0 {
			checker.add(CodeInvalidMembership, path+".columns")
		}
		seen := make(map[model.ColumnID]struct{})
		for index, columnID := range columns {
			columnPath := fmt.Sprintf("%s.columns[%d]", path, index)
			if _, duplicate := seen[columnID]; duplicate {
				checker.add(CodeDuplicateDeclaration, columnPath)
			}
			seen[columnID] = struct{}{}
			column, ok := checker.registry.Column(columnID)
			if !ok {
				checker.add(CodeUnknownColumn, columnPath)
				continue
			}
			if !column.Available() {
				continue
			}
			if columnID.Relation() != key.Relation() {
				checker.add(CodeInvalidMembership, columnPath)
			}
		}
	}
}

func (checker *checker) checkScopes() {
	for _, scope := range checker.registry.Scopes() {
		if !scope.Available() {
			continue
		}
		scopeID := scope.ID()
		path := "scope/" + scopeIDString(scopeID)
		seen := make(map[model.ColumnID]struct{})
		for index, columnID := range scope.Dimensions() {
			columnPath := fmt.Sprintf("%s.dimensions[%d]", path, index)
			if _, duplicate := seen[columnID]; duplicate {
				checker.add(CodeDuplicateDeclaration, columnPath)
			}
			seen[columnID] = struct{}{}
			column, ok := checker.registry.Column(columnID)
			if !ok {
				checker.add(CodeUnknownColumn, columnPath)
				continue
			}
			if !column.Available() {
				continue
			}
			if column.Relation().Owner() != scope.Owner() {
				checker.add(CodeForeignOwner, columnPath)
			}
		}
	}
}

func (checker *checker) checkSignatures() {
	for index, value := range checker.registry.Signatures() {
		checker.checkSignature(value, fmt.Sprintf("signatures[%d]", index))
	}
}

func (checker *checker) checkSignature(value signature.Signature, path string) {
	if !value.Available() {
		return
	}
	identity := value.Identity()
	fence := value.Fence()
	if !identity.Available() || !fence.Available() {
		checker.add(CodeInvalidSignature, path+".identity")
		return
	}
	if fence.Schema != checker.schema.SchemaID() {
		checker.add(CodeForeignSchema, path+".fence.schema")
	}
	if fence.Owner != identity.Operation.Owner() {
		checker.add(CodeForeignOwner, path+".fence.owner")
	}
	if !value.Cardinality().Available() {
		checker.add(CodeInvalidSignature, path+".contract")
	}
	if !hasOutcome(value) {
		checker.add(CodeInvalidSignature, path+".outcomes")
	}
	for index, input := range value.Inputs() {
		checker.checkInput(input, fmt.Sprintf("%s.inputs[%d]", path, index))
	}
	outputs := value.Outputs()
	seenOutputs := make(map[[2]string]struct{})
	for index, output := range outputs {
		outputPath := fmt.Sprintf("%s.outputs[%d]", path, index)
		checker.checkOutput(output, outputPath)
		key := [2]string{relationIDString(output.Relation), columnIDString(output.Column)}
		if _, duplicate := seenOutputs[key]; duplicate {
			checker.add(CodeDuplicateOutput, outputPath)
		}
		seenOutputs[key] = struct{}{}
	}
	authority := value.Authority().Denominator
	if len(outputs) == 0 {
		if authority.Available() {
			checker.add(CodeInvalidOutputAuthority, path+".authority")
		}
	} else {
		if !authority.Available() {
			checker.add(CodeInvalidOutputAuthority, path+".authority")
		} else {
			checker.checkDenominator(authority, path+".authority.denominator")
			for outputIndex, output := range outputs {
				if output.Relation != authority.Relation() {
					checker.add(CodeInvalidOutputAuthority, fmt.Sprintf("%s.outputs[%d].relation", path, outputIndex))
				}
			}
		}
	}
}

func (checker *checker) checkInput(input signature.Input, path string) {
	if !input.Available() {
		checker.add(CodeInvalidSignature, path)
		return
	}
	relation, relationOK := checker.registry.Relation(input.Relation)
	if !relationOK {
		checker.add(CodeUnknownRelation, path+".relation")
	} else if !relation.Available() {
		return
	}
	column, columnOK := checker.registry.Column(input.Column)
	if !columnOK {
		checker.add(CodeUnknownColumn, path+".column")
	} else if !column.Available() {
		return
	}
	if columnOK && (!relationOK || column.Relation() != input.Relation || !relation.HasColumn(input.Column)) {
		checker.add(CodeInvalidMembership, path+".column")
	}
	if !input.Type.Available() {
		checker.add(CodeUnknownType, path+".type")
	} else if columnOK && column.Type() != input.Type {
		checker.add(CodeInvalidMembership, path+".type")
	}
	checker.checkDenominator(input.Denominator, path+".denominator")
	if input.Denominator.Relation() != input.Relation {
		checker.add(CodeInvalidDenominator, path+".denominator.relation")
	}
	delivery := input.Delivery
	if !delivery.Available() {
		checker.add(CodeInvalidDelivery, path+".delivery")
		return
	}
	if delivery.IsSpan() {
		if _, ok := checker.registry.Key(delivery.OrderKey()); !ok || delivery.OrderKey().Relation() != input.Relation {
			checker.add(CodeInvalidDelivery, path+".delivery.order")
		}
		if delivery.OrderKey() != input.Denominator.Key() {
			checker.add(CodeInvalidDenominator, path+".delivery.order")
		}
	}
}

func (checker *checker) checkOutput(output signature.Output, path string) {
	if !output.Available() {
		checker.add(CodeInvalidSignature, path)
		return
	}
	relation, relationOK := checker.registry.Relation(output.Relation)
	if !relationOK {
		checker.add(CodeUnknownRelation, path+".relation")
	} else if !relation.Available() {
		return
	}
	column, columnOK := checker.registry.Column(output.Column)
	if !columnOK {
		checker.add(CodeUnknownColumn, path+".column")
	} else if !column.Available() {
		return
	}
	if columnOK && (!relationOK || column.Relation() != output.Relation || !relation.HasColumn(output.Column)) {
		checker.add(CodeInvalidMembership, path+".column")
	}
	if !output.Type.Available() {
		checker.add(CodeUnknownType, path+".type")
	} else if columnOK && column.Type() != output.Type {
		checker.add(CodeInvalidMembership, path+".type")
	}
}

func (checker *checker) checkDenominator(value model.DenominatorRef, path string) {
	if !value.Available() {
		checker.add(CodeInvalidDenominator, path)
		return
	}
	relation, ok := checker.registry.Relation(value.Relation())
	if !ok {
		checker.add(CodeUnknownRelation, path+".relation")
		return
	}
	key, ok := checker.registry.Key(value.Key())
	if !ok {
		checker.add(CodeUnknownKey, path+".key")
		return
	}
	if !relation.Available() || !key.Available() {
		return
	}
	if key.Relation() != value.Relation() || !relation.HasKey(value.Key()) {
		checker.add(CodeInvalidDenominator, path)
	}
}

type exprInfo struct {
	relations map[model.RelationID]struct{}
	columns   map[model.ColumnID]struct{}
	produced  map[model.RelationID]map[model.ColumnID]struct{}
	outputKey map[model.RelationID]model.KeyID
	scoped    bool
	published bool
}

func newExprInfo() exprInfo {
	return exprInfo{relations: make(map[model.RelationID]struct{}), columns: make(map[model.ColumnID]struct{}), produced: make(map[model.RelationID]map[model.ColumnID]struct{}), outputKey: make(map[model.RelationID]model.KeyID)}
}

func (info *exprInfo) addRelation(relation model.RelationID, checker *checker) {
	info.relations[relation] = struct{}{}
	if value, ok := checker.registry.Relation(relation); ok {
		for _, column := range value.Columns() {
			info.columns[column] = struct{}{}
		}
	}
}

func (info *exprInfo) addProduced(relation model.RelationID, column model.ColumnID) {
	if info.produced[relation] == nil {
		info.produced[relation] = make(map[model.ColumnID]struct{})
	}
	info.produced[relation][column] = struct{}{}
}

func (checker *checker) checkExpressions() {
	for index, entry := range checker.registry.Expressions() {
		if entry.Expression() == nil {
			continue
		}
		checker.checkExpression(entry.Expression(), fmt.Sprintf("expressions[%d]", index), make(map[model.ExpressionID]bool))
	}
}

func (checker *checker) checkExpression(expression algebra.Expression, path string, stack map[model.ExpressionID]bool) exprInfo {
	info := newExprInfo()
	if expression == nil {
		checker.add(CodeInvalidExpression, path)
		return info
	}
	switch value := expression.(type) {
	case algebra.Input:
		if relation, ok := checker.registry.Relation(value.Relation()); ok {
			if !relation.Available() {
				return info
			}
			info.addRelation(relation.ID(), checker)
			info.scoped = checker.relationScopeReady(relation)
		} else {
			checker.add(CodeUnknownRelation, path+".relation")
		}
	case algebra.Select:
		child := checker.checkExpression(value.Child(), path+".child", stack)
		info = child
		if _, ok := checker.registry.Scope(value.Contract().Scope()); !ok {
			checker.add(CodeUnknownScope, path+".scope")
		} else if !child.scoped {
			checker.add(CodeInvalidScopeOrder, path+".scope")
		}
		info.scoped = child.scoped
	case algebra.Project:
		child := checker.checkExpression(value.Child(), path+".child", stack)
		info = child
		// A projection establishes a new logical output. Intermediate rows
		// from its child are not independently published by this expression.
		info.produced = make(map[model.RelationID]map[model.ColumnID]struct{})
		info.outputKey = make(map[model.RelationID]model.KeyID)
		target := value.Contract().Target()
		targetRelation, targetOK := checker.registry.Relation(target)
		if !targetOK {
			checker.add(CodeUnknownRelation, path+".target")
		} else if !targetRelation.Available() {
			return info
		} else {
			info.addRelation(target, checker)
			if key := value.Contract().Key(); !key.Available() || key.Relation() != target || !targetRelation.HasKey(key) {
				checker.add(CodeInvalidMembership, path+".key")
			} else {
				info.outputKey[target] = key
			}
		}
		for index, mapping := range value.Contract().Mappings() {
			mappingPath := fmt.Sprintf("%s.mappings[%d]", path, index)
			if _, ok := checker.registry.Column(mapping.Source()); !ok {
				checker.add(CodeUnknownColumn, mappingPath+".source")
			} else if _, ok := child.columns[mapping.Source()]; !ok {
				checker.add(CodeInvalidMembership, mappingPath+".source")
			}
			targetColumn, ok := checker.registry.Column(mapping.Target())
			if !ok {
				checker.add(CodeUnknownColumn, mappingPath+".target")
			} else if targetColumn.Relation() != target {
				checker.add(CodeInvalidMembership, mappingPath+".target")
			} else {
				info.columns[mapping.Target()] = struct{}{}
				info.addProduced(target, mapping.Target())
			}
		}
	case algebra.Join:
		left := checker.checkExpression(value.Left(), path+".left", stack)
		right := checker.checkExpression(value.Right(), path+".right", stack)
		info = mergeInfo(left, right)
		leftColumns, rightColumns := value.Contract().LeftColumns(), value.Contract().RightColumns()
		if len(leftColumns) == 0 || len(leftColumns) != len(rightColumns) {
			checker.add(CodeInvalidExpression, path+".contract.columns")
		}
		for index := range leftColumns {
			if _, ok := left.columns[leftColumns[index]]; !ok {
				checker.add(CodeInvalidMembership, fmt.Sprintf("%s.contract.left[%d]", path, index))
			}
			if _, ok := right.columns[rightColumns[index]]; !ok {
				checker.add(CodeInvalidMembership, fmt.Sprintf("%s.contract.right[%d]", path, index))
			}
		}
	case algebra.Merge:
		inputs := value.Inputs()
		if len(inputs) == 0 {
			checker.add(CodeInvalidExpression, path+".inputs")
		}
		childInfos := make([]exprInfo, 0, len(inputs))
		for index, childExpression := range inputs {
			child := checker.checkExpression(childExpression, fmt.Sprintf("%s.inputs[%d]", path, index), stack)
			childInfos = append(childInfos, child)
			info = mergeInfo(info, child)
		}
		info.scoped = allScoped(childInfos)
		if key := value.Contract().Key(); !checker.keyUsableByInfo(key, info) {
			checker.add(CodeInvalidMembership, path+".key")
		}
	case algebra.Group:
		child := checker.checkExpression(value.Child(), path+".child", stack)
		info = child
		if !checker.keyUsableByInfo(value.Contract().Key(), child) {
			checker.add(CodeInvalidMembership, path+".key")
		}
		if !value.Contract().Cardinality().Available() {
			checker.add(CodeInvalidExpression, path+".cardinality")
		}
	case algebra.Complete:
		child := checker.checkExpression(value.Child(), path+".child", stack)
		info = child
		checker.checkDenominator(value.Denominator(), path+".denominator")
		if _, ok := child.relations[value.Denominator().Relation()]; !ok {
			checker.add(CodeInvalidDenominator, path+".denominator.relation")
		}
		if value.Denominator().Relation().Available() {
			info.addRelation(value.Denominator().Relation(), checker)
		}
	case algebra.Apply:
		childInfos := make([]exprInfo, len(value.Inputs()))
		for index, childExpression := range value.Inputs() {
			childInfos[index] = checker.checkExpression(childExpression, fmt.Sprintf("%s.inputs[%d]", path, index), stack)
			info = mergeInfo(info, childInfos[index])
		}
		operation := value.Contract().Operation()
		sealed, ok := checker.registry.Signature(operation)
		if !ok {
			checker.add(CodeUnknownOperation, path+".operation")
			return info
		}
		if !sealed.Available() {
			return info
		}
		if len(childInfos) != sealed.InputLen() {
			checker.add(CodeInvalidSignature, path+".inputs")
		}
		for index, input := range sealed.Inputs() {
			if index >= len(childInfos) {
				break
			}
			child := childInfos[index]
			if !child.scoped {
				checker.add(CodeInvalidScopeOrder, fmt.Sprintf("%s.inputs[%d].scope", path, index))
			}
			if _, ok := child.relations[input.Relation]; !ok {
				checker.add(CodeInvalidMembership, fmt.Sprintf("%s.inputs[%d].relation", path, index))
			}
			if _, ok := child.columns[input.Column]; !ok {
				checker.add(CodeInvalidMembership, fmt.Sprintf("%s.inputs[%d].column", path, index))
			}
		}
		info.scoped = allScoped(childInfos)
		// Apply is the semantic publication boundary. Only its declared
		// outputs are candidates for the next logical publication; child
		// outputs are inputs, not an implicit second write.
		info.produced = make(map[model.RelationID]map[model.ColumnID]struct{})
		info.outputKey = make(map[model.RelationID]model.KeyID)
		for _, output := range sealed.Outputs() {
			info.addRelation(output.Relation, checker)
			info.addProduced(output.Relation, output.Column)
		}
		if denominator := sealed.Authority().Denominator; denominator.Available() {
			info.outputKey[denominator.Relation()] = denominator.Key()
		}
	case algebra.Publish:
		child := checker.checkExpression(value.Child(), path+".child", stack)
		info = child
		if !child.scoped {
			checker.add(CodeInvalidScopeOrder, path+".scope")
		}
		destination, key := value.Contract().Destination(), value.Contract().Key()
		relation, relationOK := checker.registry.Relation(destination)
		if !relationOK {
			checker.add(CodeUnknownRelation, path+".destination")
		} else if !relation.Available() {
			return info
		}
		if keyValue, ok := checker.registry.Key(key); !ok {
			checker.add(CodeUnknownKey, path+".key")
		} else if keyValue.Relation() != destination || !relationOK || !relation.HasKey(key) {
			checker.add(CodeInvalidPublication, path+".key")
		}
		if columns, ok := child.produced[destination]; !ok || len(columns) == 0 {
			checker.add(CodeUndeclaredPublication, path+".destination")
		}
		if expected, ok := child.outputKey[destination]; !ok || expected != key {
			checker.add(CodeInvalidPublication, path+".key")
		}
		info.published = true
	default:
		// The algebra package seals the expression interface, but pointers to
		// nodes or a future unregistered node must not silently pass as an
		// empty expression in an independent checker.
		checker.add(CodeInvalidExpression, path+".kind")
	}
	return info
}

func (checker *checker) relationScopeReady(relation model.RelationSchema) bool {
	scope, ok := checker.registry.Scope(relation.Scope())
	return ok && scope.Available()
}

func (checker *checker) keyUsableByInfo(key model.KeyID, info exprInfo) bool {
	if _, ok := checker.registry.Key(key); !ok {
		return false
	}
	_, ok := info.relations[key.Relation()]
	return ok
}

func mergeInfo(left, right exprInfo) exprInfo {
	result := newExprInfo()
	for relation := range left.relations {
		result.relations[relation] = struct{}{}
	}
	for relation := range right.relations {
		result.relations[relation] = struct{}{}
	}
	for column := range left.columns {
		result.columns[column] = struct{}{}
	}
	for column := range right.columns {
		result.columns[column] = struct{}{}
	}
	for relation, columns := range left.produced {
		for column := range columns {
			result.addProduced(relation, column)
		}
	}
	for relation, columns := range right.produced {
		for column := range columns {
			result.addProduced(relation, column)
		}
	}
	for relation, key := range left.outputKey {
		result.outputKey[relation] = key
	}
	for relation, key := range right.outputKey {
		if prior, ok := result.outputKey[relation]; !ok || prior == key {
			result.outputKey[relation] = key
		} else {
			// A relation with two distinct authority keys cannot be
			// published without an explicit projection choosing one.
			delete(result.outputKey, relation)
		}
	}
	result.scoped = left.scoped && right.scoped
	result.published = left.published || right.published
	return result
}

func allScoped(values []exprInfo) bool {
	for _, value := range values {
		if !value.scoped {
			return false
		}
	}
	return true
}

// The ID formatting helpers intentionally avoid exposing identity internals
// or using physical ordinals in diagnostics.
func relationIDString(value model.RelationID) string {
	return value.Owner().Content().String() + "/" + value.Content().String()
}
func columnIDString(value model.ColumnID) string {
	return relationIDString(value.Relation()) + "/" + value.Content().String()
}
func keyIDString(value model.KeyID) string {
	return relationIDString(value.Relation()) + "/" + value.Content().String()
}
func scopeIDString(value model.ScopeID) string {
	return value.Owner().Content().String() + "/" + value.Content().String()
}

func hasOutcome(value signature.Signature) bool {
	for _, code := range []outcome.Code{outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused} {
		if value.Allows(code) {
			return true
		}
	}
	return false
}
