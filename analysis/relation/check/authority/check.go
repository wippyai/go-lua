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
	CodeInvalidOutputDenominator
	CodeUnknownOperation
	CodeInvalidExpression
	CodeInvalidScopeOrder
	CodeInvalidPublication
	CodeUndeclaredPublication
	CodeInvalidDependencyProjection
	CodeInvalidObservation
	CodeInvalidCorrelation
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
		"invalid-output-denominator",
		"unknown-operation",
		"invalid-expression",
		"invalid-scope-order",
		"invalid-publication",
		"undeclared-publication",
		"invalid-dependency-projection",
		"invalid-observation",
		"invalid-correlation",
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
	checker.checkObservations()
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
		case checkregistry.CodeObservationUnavailable, checkregistry.CodeObservationDuplicate:
			code = CodeInvalidObservation
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

// checkObservations proves the closed shape consumed by runtime/snapshot.
// The source population is the parent observation extent. Each output carries
// its own sealed destination denominator, so a child relation may be emitted
// without asking runtime to infer a parent-to-child route.
func (checker *checker) checkObservations() {
	for index, contract := range checker.registry.Observations() {
		path := fmt.Sprintf("observations[%d]", index)
		if !contract.Available() {
			checker.add(CodeInvalidObservation, path)
			continue
		}
		operation, operationOK := checker.registry.Signature(contract.Operation())
		if !operationOK || !operation.Available() {
			checker.add(CodeUnknownOperation, path+".operation")
			continue
		}
		dependency, dependencyOK := checker.registry.Dependency(contract.Dependency())
		if !dependencyOK || !dependency.Available() {
			checker.add(CodeInvalidObservation, path+".dependency")
			continue
		}
		// The dependency is an owner-issued execution declaration, not merely
		// a convenient label. Its expression is the authority from which an
		// observation may redeem an Apply result. Dependency and operation
		// ownership remain independent: a composition may schedule a
		// peer-owned operation, so pairing is proved by the sealed expression
		// occurrence below rather than by an owner-equality shortcut.
		entry, expressionOK := checker.registry.Expression(dependency.Expression())
		if !expressionOK || !entry.Available() {
			checker.add(CodeInvalidObservation, path+".dependency.expression")
			continue
		}
		var applies []algebra.Apply
		if !collectObservationApplies(entry.Expression(), contract.Operation(), &applies) || len(applies) != 1 {
			// The dependency may contain Publish, Merge, or ColumnProject
			// structure around its terminal computation. We accept that sealed
			// algebra, but only when exactly one Apply occurrence carries the
			// declared operation. This uniqueness is the occurrence proof: a
			// second same-operation occurrence cannot be selected by copying the
			// operation identity or by guessing a physical child ordinal.
			checker.add(CodeInvalidObservation, path+".dependency.expression.apply")
		}
		applyInputs := 0
		if len(applies) == 1 {
			applyInputs = len(applies[0].Inputs())
		}
		population := contract.Population()
		checker.checkDenominator(population, path+".population")
		source := contract.Source()
		// Source.Child addresses a physical Apply child, while operation
		// inputs address semantic slots. Several slots may legally share one
		// child, so InputAt(source.Child) is not the relation proof and would
		// reject valid grouped scalar compositions. The schema can prove the
		// conservative child bound from input arity and require that the
		// population is one of the operation's consumed row authorities. The
		// mounted invocation then proves the exact child/tuple/source row by
		// membership; no reverse map is invented here.
		if source.Child() >= uint32(operation.InputLen()) || applyInputs == 0 || source.Child() >= uint32(applyInputs) {
			checker.add(CodeInvalidObservation, path+".source.child")
		}
		populationInput := false
		allowsAnyTuple := false
		maxTuple := uint32(0)
		for _, input := range operation.Inputs() {
			if !input.Available() || input.Denominator != population || input.Relation != population.Relation() {
				continue
			}
			populationInput = true
			switch {
			case input.Delivery.IsComplete():
				allowsAnyTuple = true
			case input.Delivery.IsScalar():
				// Scalar permits only tuple zero; retain the default bound.
			default:
				if limit, bounded := input.Delivery.Limit(); bounded && limit > maxTuple {
					maxTuple = limit
				}
			}
		}
		if !populationInput {
			checker.add(CodeInvalidObservation, path+".population.source")
		}
		if !allowsAnyTuple && source.Tuple() != 0 && source.Tuple() >= maxTuple {
			checker.add(CodeInvalidObservation, path+".source.tuple")
		}
		for outputIndex, output := range contract.Outputs() {
			outputPath := fmt.Sprintf("%s.outputs[%d]", path, outputIndex)
			destination := output.Destination()
			checker.checkDenominator(destination, outputPath+".destination")
			// CompleteDenominator is a signature-owned operation contract. An
			// observation output has no operation-output authority of its own;
			// accepting this arm here would leave runtime with no sound way to
			// prove complete coverage for the copied extent.
			if output.Cardinality().Kind() == model.CompleteDenominator {
				checker.add(CodeInvalidObservation, outputPath+".cardinality")
			}
			column, columnOK := checker.registry.Column(output.Column())
			if !columnOK || !column.Available() {
				checker.add(CodeUnknownColumn, outputPath+".column")
				continue
			}
			if column.Relation() != destination.Relation() || column.Type() != output.Type() {
				checker.add(CodeInvalidObservation, outputPath)
			}
			declared, declaredOK := operation.OutputFor(destination.Relation(), output.Column())
			if !declaredOK || declared.Type != output.Type() {
				checker.add(CodeInvalidObservation, outputPath+".operation")
			}
			// The corresponding sealed output owns this destination. This is the
			// cross-relation seam: every destination is explicit on that output.
			declaredDestination, destinationOK := operation.OutputDestination(destination.Relation(), output.Column())
			if !destinationOK || declaredDestination != destination {
				checker.add(CodeInvalidObservation, outputPath+".destination.operation")
			}
		}
	}
}

// collectObservationApplies walks only sealed algebra child edges. It records
// every Apply carrying operation so the caller can require one unique
// occurrence without inventing a second path/occurrence vocabulary.
func collectObservationApplies(expression algebra.Expression, operation signature.Identity, result *[]algebra.Apply) bool {
	if expression == nil {
		return false
	}
	visit := func(children []algebra.Expression) bool {
		for _, child := range children {
			if !collectObservationApplies(child, operation, result) {
				return false
			}
		}
		return true
	}
	switch value := expression.(type) {
	case algebra.Input:
		return true
	case *algebra.Input:
		return value != nil
	case algebra.Select:
		return collectObservationApplies(value.Child(), operation, result)
	case *algebra.Select:
		return value != nil && collectObservationApplies(value.Child(), operation, result)
	case algebra.Project:
		return collectObservationApplies(value.Child(), operation, result)
	case *algebra.Project:
		return value != nil && collectObservationApplies(value.Child(), operation, result)
	case algebra.ColumnProject:
		return collectObservationApplies(value.Child(), operation, result)
	case *algebra.ColumnProject:
		return value != nil && collectObservationApplies(value.Child(), operation, result)
	case algebra.Join:
		return collectObservationApplies(value.Left(), operation, result) && collectObservationApplies(value.Right(), operation, result)
	case *algebra.Join:
		return value != nil && collectObservationApplies(value.Left(), operation, result) && collectObservationApplies(value.Right(), operation, result)
	case algebra.Expand:
		return collectObservationApplies(value.Child(), operation, result)
	case *algebra.Expand:
		return value != nil && collectObservationApplies(value.Child(), operation, result)
	case algebra.Merge:
		return visit(value.Inputs())
	case *algebra.Merge:
		return value != nil && visit(value.Inputs())
	case algebra.Group:
		return collectObservationApplies(value.Child(), operation, result)
	case *algebra.Group:
		return value != nil && collectObservationApplies(value.Child(), operation, result)
	case algebra.Complete:
		return collectObservationApplies(value.Child(), operation, result)
	case *algebra.Complete:
		return value != nil && collectObservationApplies(value.Child(), operation, result)
	case algebra.Apply:
		if value.Contract().Operation() == operation {
			*result = append(*result, value)
		}
		return visit(value.Inputs())
	case *algebra.Apply:
		if value == nil {
			return false
		}
		if value.Contract().Operation() == operation {
			*result = append(*result, *value)
		}
		return visit(value.Inputs())
	case algebra.Publish:
		return collectObservationApplies(value.Child(), operation, result)
	case *algebra.Publish:
		return value != nil && collectObservationApplies(value.Child(), operation, result)
	default:
		return false
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
	if value.Cardinality().Kind() == model.CompleteDenominator {
		checker.checkCompleteDenominator(outputs, path)
	}
}

// checkCompleteDenominator proves the output-side authority for a
// mount-dependent complete result. The mounted witness supplies cardinality;
// the sealed operation supplies the exact denominator and its output columns.
func (checker *checker) checkCompleteDenominator(outputs []signature.Output, path string) {
	if len(outputs) == 0 {
		checker.add(CodeInvalidSignature, path+".outputs")
		return
	}
	var denominator model.DenominatorRef
	for index, output := range outputs {
		if !output.Available() || !output.Denominator.Available() {
			continue
		}
		if !denominator.Available() {
			denominator = output.Denominator
			continue
		}
		if output.Denominator != denominator {
			checker.add(CodeInvalidOutputDenominator, fmt.Sprintf("%s.outputs[%d].denominator", path, index))
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
	checker.checkDenominator(output.Denominator, path+".denominator")
	if output.Denominator.Available() && output.Denominator.Relation() != output.Relation {
		checker.add(CodeInvalidOutputDenominator, path+".denominator.relation")
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
	case algebra.ColumnProject:
		child := checker.checkExpression(value.Child(), path+".child", stack)
		info = child
		slots := value.Contract().Slots()
		if len(slots) == 0 {
			checker.add(CodeInvalidExpression, path+".slots")
		}
		seen := make(map[model.ColumnID]struct{}, len(slots))
		for index, slot := range slots {
			slotPath := fmt.Sprintf("%s.slots[%d]", path, index)
			if _, duplicate := seen[slot.Column()]; duplicate {
				checker.add(CodeInvalidMembership, slotPath)
			}
			seen[slot.Column()] = struct{}{}
			if _, ok := checker.registry.Column(slot.Column()); !ok {
				checker.add(CodeUnknownColumn, slotPath+".column")
			} else if _, ok := child.columns[slot.Column()]; !ok {
				checker.add(CodeInvalidMembership, slotPath+".column")
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
	case algebra.Expand:
		// Expand retains the child's C-left tuple and appends the sealed R
		// tuple. P and correlation are logical dependencies of the contract,
		// not extra output columns, so they are validated here without being
		// smuggled into the downstream expression shape.
		child := checker.checkExpression(value.Child(), path+".child", stack)
		info = child
		contract := value.Contract()
		if !contract.Available() {
			checker.add(CodeInvalidExpression, path+".contract")
			break
		}
		candidate, candidateOK := checker.registry.Relation(contract.Candidate())
		if !candidateOK || !candidate.Available() {
			checker.add(CodeUnknownRelation, path+".candidate")
		} else if _, present := child.relations[contract.Candidate()]; !present {
			checker.add(CodeInvalidMembership, path+".candidate")
		}
		for relationID, relationPath := range map[model.RelationID]string{
			contract.Publisher():   path + ".publisher",
			contract.Correlation(): path + ".correlation",
		} {
			relation, ok := checker.registry.Relation(relationID)
			if !ok || !relation.Available() {
				checker.add(CodeUnknownRelation, relationPath)
			}
		}
		reader, readerOK := checker.registry.Relation(contract.Reader())
		if !readerOK || !reader.Available() {
			checker.add(CodeUnknownRelation, path+".reader")
			break
		}
		key, keyOK := checker.registry.Column(contract.Key())
		if !keyOK || !key.Available() {
			checker.add(CodeUnknownColumn, path+".key")
		} else if key.Relation() != reader.ID() || !reader.HasColumn(contract.Key()) {
			checker.add(CodeInvalidMembership, path+".key")
		} else {
			checker.checkExpandKeySchema(reader, contract.Key(), path)
		}
		if scope := contract.Scope(); !scope.Available() {
			checker.add(CodeUnknownScope, path+".scope")
		} else if _, ok := checker.registry.Scope(scope); !ok {
			checker.add(CodeUnknownScope, path+".scope")
		} else {
			info.scoped = child.scoped
		}
		info.addRelation(reader.ID(), checker)
	case algebra.Merge:
		inputs := value.Inputs()
		childInfos := make([]exprInfo, len(inputs))
		if len(inputs) == 0 {
			checker.add(CodeInvalidExpression, path+".inputs")
		}
		for index, childExpression := range inputs {
			childInfos[index] = checker.checkExpression(childExpression, fmt.Sprintf("%s.inputs[%d]", path, index), stack)
			if index == 0 {
				info = childInfos[index]
			} else {
				info = mergeInfo(info, childInfos[index])
			}
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
		} else if value.Contract().Cardinality().Kind() == model.CompleteDenominator {
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
			if index == 0 {
				info = childInfos[index]
			} else {
				info = mergeInfo(info, childInfos[index])
			}
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
		slotSource := value.Contract().SlotSource()
		if len(slotSource) != sealed.InputLen() {
			checker.add(CodeInvalidSignature, path+".slotSource")
		}
		seenChildren := make([]bool, len(childInfos))
		for index, input := range sealed.Inputs() {
			if index >= len(slotSource) {
				break
			}
			childOrdinal := int(slotSource[index].Child())
			if childOrdinal >= len(childInfos) {
				checker.add(CodeInvalidSignature, fmt.Sprintf("%s.slotSource[%d]", path, index))
				continue
			}
			seenChildren[childOrdinal] = true
			child := childInfos[childOrdinal]
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
		for childIndex, seen := range seenChildren {
			if !seen {
				checker.add(CodeInvalidSignature, fmt.Sprintf("%s.inputs[%d].slotSource", path, childIndex))
			}
		}
		checker.checkApplyCorrelation(value.Contract().Correlation(), value.Inputs(), childInfos, sealed, value.Contract().SlotSource(), path)
		info.scoped = allScoped(childInfos)
		// Apply is the semantic publication boundary. Only its declared
		// outputs are candidates for the next logical publication; child
		// outputs are inputs, not an implicit second write.
		info.produced = make(map[model.RelationID]map[model.ColumnID]struct{})
		info.outputKey = make(map[model.RelationID]model.KeyID)
		for _, output := range sealed.Outputs() {
			info.addRelation(output.Relation, checker)
			info.addProduced(output.Relation, output.Column)
			if output.Denominator.Available() {
				info.outputKey[output.Denominator.Relation()] = output.Denominator.Key()
			}
		}
	case algebra.Publish:
		childExpression := value.Child()
		child := checker.checkExpression(childExpression, path+".child", stack)
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
		columns := value.Contract().Columns()
		if len(columns) == 0 {
			columns = relation.Columns()
		}
		seen := make(map[model.ColumnID]struct{}, len(columns))
		for index, column := range columns {
			columnPath := fmt.Sprintf("%s.columns[%d]", path, index)
			if _, duplicate := seen[column]; duplicate {
				checker.add(CodeInvalidPublication, columnPath)
				continue
			}
			seen[column] = struct{}{}
			declared, declaredOK := checker.registry.Column(column)
			if !declaredOK {
				checker.add(CodeUnknownColumn, columnPath)
				continue
			}
			if declared.Relation() != destination || !relation.HasColumn(column) {
				checker.add(CodeInvalidPublication, columnPath)
			}
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

// checkApplyCorrelation proves the schema half of a heterogeneous Apply.
// Each child contributes one owner-issued ordered projection and one exact
// Complete range authority from the semantic signature. The common
// coordinate is only a typed declaration; no checker invents a relation,
// denominator, cache, or value copy to make unrelated ranges agree.
func (checker *checker) checkApplyCorrelation(correlation algebra.ApplyCorrelation, expressions []algebra.Expression, children []exprInfo, sealed signature.Signature, slots []algebra.SlotSource, path string) {
	if !correlation.Specified() {
		return
	}
	if !correlation.Available() {
		checker.add(CodeInvalidCorrelation, path+".correlation")
		return
	}
	// Population is an independent closed Q authority. Its key must be the
	// singleton coordinate key: that is the existing schema fact that proves
	// coordinate totality and uniqueness. Scope is deliberately not consulted
	// here; it is a cofiber and never a key substitute.
	population := correlation.Population()
	checker.checkDenominator(population, path+".correlation.population")
	populationRelation, populationRelationOK := checker.registry.Relation(population.Relation())
	populationKey, populationKeyOK := checker.registry.Key(population.Key())
	coordinate, coordinateOK := checker.registry.Column(correlation.Coordinate())
	if !coordinateOK || !coordinate.Available() {
		checker.add(CodeUnknownColumn, path+".correlation.coordinate")
	} else if coordinate.Type() != correlation.Type() {
		checker.add(CodeInvalidCorrelation, path+".correlation.type")
	}
	if coordinateOK && coordinate.Available() && populationRelationOK && populationRelation.Available() {
		if coordinate.Relation() != population.Relation() || !populationRelation.HasColumn(correlation.Coordinate()) {
			checker.add(CodeInvalidCorrelation, path+".correlation.population.coordinate")
		}
	}
	if populationKeyOK && populationKey.Available() {
		keyColumns := populationKey.Columns()
		if len(keyColumns) != 1 || keyColumns[0] != correlation.Coordinate() {
			checker.add(CodeInvalidCorrelation, path+".correlation.population.key")
		}
	}
	if correlation.ProjectionCount() != len(children) {
		checker.add(CodeInvalidCorrelation, path+".correlation.projections")
		return
	}
	if len(slots) != sealed.InputLen() {
		return
	}
	// The mixed population ABI has one direct population Input at child zero;
	// unlike the historical all-complete form it owns no Complete denominator
	// or posting directory.  Keep that child on the schema authority path only
	// after checking its scalar source, then validate every remaining child by
	// the same exact Complete range/projection rules below.
	if scalarPopulationChildAuthority(expressions, children, sealed, slots, correlation) {
		checker.checkScalarPopulationCorrelation(correlation, expressions, children, sealed, slots, path)
		return
	}
	type rangeProof struct {
		relation    model.RelationID
		denominator model.DenominatorRef
		delivery    signature.Delivery
		set         bool
	}
	ranges := make([]rangeProof, len(children))
	for index, input := range sealed.Inputs() {
		childIndex := int(slots[index].Child())
		if childIndex < 0 || childIndex >= len(children) || !input.Delivery.IsComplete() {
			continue
		}
		proof := &ranges[childIndex]
		if !proof.set {
			proof.relation, proof.denominator, proof.delivery, proof.set = input.Relation, input.Denominator, input.Delivery, true
			continue
		}
		if proof.relation != input.Relation || proof.denominator != input.Denominator || proof.delivery != input.Delivery {
			checker.add(CodeInvalidCorrelation, fmt.Sprintf("%s.correlation.child[%d].range", path, childIndex))
		}
	}
	for childIndex, child := range children {
		projection, ok := correlation.ProjectionAt(childIndex)
		if !ok || !ranges[childIndex].set {
			checker.add(CodeInvalidCorrelation, fmt.Sprintf("%s.correlation.child[%d]", path, childIndex))
			continue
		}
		if len(projection) == 0 {
			checker.checkSharedCompleteCorrelationChild(correlation, expressions[childIndex], child, sealed, slots, childIndex, path)
			continue
		}
		seen := make(map[model.ColumnID]struct{}, len(projection))
		for columnIndex, columnID := range projection {
			columnPath := fmt.Sprintf("%s.correlation.child[%d].columns[%d]", path, childIndex, columnIndex)
			if _, duplicate := seen[columnID]; duplicate {
				checker.add(CodeInvalidCorrelation, columnPath)
				continue
			}
			seen[columnID] = struct{}{}
			column, columnOK := checker.registry.Column(columnID)
			if !columnOK || !column.Available() {
				checker.add(CodeUnknownColumn, columnPath)
				continue
			}
			if _, retained := child.columns[columnID]; !retained || column.Relation() != ranges[childIndex].relation {
				checker.add(CodeInvalidMembership, columnPath)
			}
			if column.Type() != correlation.Type() {
				checker.add(CodeInvalidCorrelation, columnPath+".type")
			}
		}
	}
}

// scalarPopulationChildAuthority recognizes the same closed mixed ABI as the
// typing pass: child zero is a direct Input and has one scalar delivery.  It
// is intentionally structural, so malformed scalar declarations enter the
// mixed checker and receive an explicit refusal instead of falling through to
// the all-complete range proof.
func scalarPopulationChildAuthority(expressions []algebra.Expression, children []exprInfo, sealed signature.Signature, slots []algebra.SlotSource, correlation algebra.ApplyCorrelation) bool {
	if len(expressions) == 0 || len(children) == 0 || len(slots) != sealed.InputLen() || !correlation.Available() {
		return false
	}
	if _, ok := directInputRelation(expressions[0]); !ok {
		return false
	}
	for index, source := range slots {
		if source.Child() != 0 {
			continue
		}
		input, ok := sealed.InputAt(index)
		if ok && input.Delivery.IsScalar() {
			return true
		}
	}
	return false
}

func (checker *checker) checkScalarPopulationCorrelation(correlation algebra.ApplyCorrelation, expressions []algebra.Expression, children []exprInfo, sealed signature.Signature, slots []algebra.SlotSource, path string) {
	population := correlation.Population()
	coordinate := correlation.Coordinate()
	if len(children) < 2 {
		checker.add(CodeInvalidCorrelation, path+".correlation.children")
	}
	directRelation, directOK := directInputRelation(expressions[0])
	if !directOK || directRelation != population.Relation() {
		checker.add(CodeInvalidCorrelation, path+".correlation.population.child")
	}
	if len(children) != 0 {
		if _, present := children[0].relations[population.Relation()]; !present {
			checker.add(CodeInvalidMembership, path+".correlation.population.child")
		}
	}

	// The positional source is checked against the direct Input's authored
	// relation vector.  This is the authority-side equivalent of the typing
	// cell check and prevents an out-of-range or misaddressed scalar from being
	// accepted merely because the relation contains the coordinate somewhere.
	scalarSlots := make([]int, 0, 1)
	for index, source := range slots {
		if source.Child() != 0 {
			continue
		}
		input, inputOK := sealed.InputAt(index)
		if !inputOK {
			continue
		}
		if !input.Delivery.IsScalar() {
			checker.add(CodeInvalidCorrelation, fmt.Sprintf("%s.correlation.population.slot[%d]", path, index))
			continue
		}
		scalarSlots = append(scalarSlots, index)
	}
	if len(scalarSlots) != 1 {
		checker.add(CodeInvalidCorrelation, path+".correlation.population.source")
	} else {
		index := scalarSlots[0]
		source := slots[index]
		input, _ := sealed.InputAt(index)
		valid := directOK && directRelation == population.Relation() && input.Relation == population.Relation() && input.Column == coordinate && input.Type == correlation.Type() && input.Denominator == population
		if directOK {
			relation, relationOK := checker.registry.Relation(directRelation)
			if !relationOK || !relation.Available() {
				valid = false
			} else {
				columns := relation.Columns()
				cell := int(source.Cell())
				if cell < 0 || cell >= len(columns) || columns[cell] != coordinate {
					valid = false
				}
			}
		}
		if !valid {
			checker.add(CodeInvalidCorrelation, fmt.Sprintf("%s.correlation.population.source", path))
		}
	}
	projection, projectionOK := correlation.ProjectionAt(0)
	if !projectionOK || len(projection) != 1 || projection[0] != coordinate {
		checker.add(CodeInvalidCorrelation, path+".correlation.population.projection")
	}

	// Every remaining child keeps the historical Complete range authority.
	type rangeProof struct {
		relation    model.RelationID
		denominator model.DenominatorRef
		delivery    signature.Delivery
		set         bool
	}
	ranges := make([]rangeProof, len(children))
	for index, input := range sealed.Inputs() {
		childIndex := int(slots[index].Child())
		if childIndex <= 0 || childIndex >= len(children) || !input.Delivery.IsComplete() {
			continue
		}
		proof := &ranges[childIndex]
		if !proof.set {
			proof.relation, proof.denominator, proof.delivery, proof.set = input.Relation, input.Denominator, input.Delivery, true
			continue
		}
		if proof.relation != input.Relation || proof.denominator != input.Denominator || proof.delivery != input.Delivery {
			checker.add(CodeInvalidCorrelation, fmt.Sprintf("%s.correlation.child[%d].range", path, childIndex))
		}
	}
	for childIndex := 1; childIndex < len(children); childIndex++ {
		child := children[childIndex]
		projection, projectionOK := correlation.ProjectionAt(childIndex)
		if !projectionOK {
			checker.add(CodeInvalidCorrelation, fmt.Sprintf("%s.correlation.child[%d]", path, childIndex))
			continue
		}
		if len(projection) == 0 {
			checker.checkSharedCompleteCorrelationChild(correlation, expressions[childIndex], child, sealed, slots, childIndex, path)
			continue
		}
		spanDenominator, spanRelation, exactSpan := exactCompleteSelectInputAuthority(expressions[childIndex])
		if !exactSpan {
			checker.add(CodeInvalidCorrelation, fmt.Sprintf("%s.correlation.child[%d].shape", path, childIndex))
		}
		if !ranges[childIndex].set {
			checker.add(CodeInvalidCorrelation, fmt.Sprintf("%s.correlation.child[%d]", path, childIndex))
			continue
		}
		if !exactSpan || spanRelation != ranges[childIndex].relation || spanDenominator != ranges[childIndex].denominator {
			checker.add(CodeInvalidCorrelation, fmt.Sprintf("%s.correlation.child[%d].range", path, childIndex))
		}
		seen := make(map[model.ColumnID]struct{}, len(projection))
		for columnIndex, columnID := range projection {
			columnPath := fmt.Sprintf("%s.correlation.child[%d].columns[%d]", path, childIndex, columnIndex)
			if _, duplicate := seen[columnID]; duplicate {
				checker.add(CodeInvalidCorrelation, columnPath)
				continue
			}
			seen[columnID] = struct{}{}
			column, columnOK := checker.registry.Column(columnID)
			if !columnOK || !column.Available() {
				checker.add(CodeUnknownColumn, columnPath)
				continue
			}
			if _, retained := child.columns[columnID]; !retained || column.Relation() != ranges[childIndex].relation {
				checker.add(CodeInvalidMembership, columnPath)
			}
			if column.Type() != correlation.Type() {
				checker.add(CodeInvalidCorrelation, columnPath+".type")
			}
		}
		for slotIndex, source := range slots {
			if int(source.Child()) != childIndex {
				continue
			}
			input, inputOK := sealed.InputAt(slotIndex)
			if !inputOK || !input.Delivery.IsComplete() || input.Relation != ranges[childIndex].relation || input.Denominator != ranges[childIndex].denominator {
				checker.add(CodeInvalidCorrelation, fmt.Sprintf("%s.correlation.child[%d].slot[%d]", path, childIndex, slotIndex))
				continue
			}
			if prior, priorOK := sealed.InputAt(firstSlotForAuthorityChild(slots, childIndex, slotIndex)); priorOK && (prior.Relation != input.Relation || prior.Denominator != input.Denominator || prior.Delivery != input.Delivery) {
				checker.add(CodeInvalidCorrelation, fmt.Sprintf("%s.correlation.child[%d].range", path, childIndex))
			}
			if exactSpan {
				relation, relationOK := checker.registry.Relation(spanRelation)
				if !relationOK || !relation.Available() || int(source.Cell()) >= len(relation.Columns()) || relation.Columns()[source.Cell()] != input.Column {
					checker.add(CodeInvalidCorrelation, fmt.Sprintf("%s.correlation.child[%d].slot[%d]", path, childIndex, slotIndex))
				}
			}
		}
	}
}

// checkSharedCompleteCorrelationChild proves the authority half of the
// broadcast form. An empty projection is not a missing posting key: it is an
// exact global Complete(Select(Input)) vector with no Q-row directory. The
// vector cannot retain or deliver the population coordinate, and every slot
// must name its exact global Complete range.
//
// Keep the source relation and Complete denominator equal here. Correlated
// joins require a distinct source/carrier witness contract; accepting them by
// relaxing this local proof would let a global cell masquerade as a carrier
// row before mount has a way to authenticate both.
func (checker *checker) checkSharedCompleteCorrelationChild(correlation algebra.ApplyCorrelation, expression algebra.Expression, child exprInfo, sealed signature.Signature, slots []algebra.SlotSource, childIndex int, path string) {
	childPath := fmt.Sprintf("%s.correlation.child[%d]", path, childIndex)
	denominator, relation, exact := exactCompleteSelectInputAuthority(expression)
	if !exact {
		checker.add(CodeInvalidCorrelation, childPath+".shape")
	}
	if _, retainsCoordinate := child.columns[correlation.Coordinate()]; retainsCoordinate {
		checker.add(CodeInvalidCorrelation, childPath+".columns")
	}

	spanSet := false
	var prior signature.Input
	priorSet := false
	for slotIndex, source := range slots {
		if int(source.Child()) != childIndex {
			continue
		}
		input, inputOK := sealed.InputAt(slotIndex)
		if !inputOK {
			continue
		}
		if !input.Delivery.IsComplete() {
			checker.add(CodeInvalidCorrelation, fmt.Sprintf("%s.slot[%d]", childPath, slotIndex))
			continue
		}
		spanSet = true
		if input.Column == correlation.Coordinate() {
			checker.add(CodeInvalidCorrelation, fmt.Sprintf("%s.slot[%d]", childPath, slotIndex))
		}
		if !exact || input.Relation != relation || input.Denominator != denominator {
			checker.add(CodeInvalidCorrelation, childPath+".range")
		}
		if priorSet && (prior.Relation != input.Relation || prior.Denominator != input.Denominator || prior.Delivery != input.Delivery) {
			checker.add(CodeInvalidCorrelation, childPath+".range")
		}
		if exact {
			relationSchema, relationOK := checker.registry.Relation(relation)
			if !relationOK || !relationSchema.Available() || int(source.Cell()) >= len(relationSchema.Columns()) || relationSchema.Columns()[source.Cell()] != input.Column {
				checker.add(CodeInvalidCorrelation, fmt.Sprintf("%s.slot[%d]", childPath, slotIndex))
			}
		}
		prior, priorSet = input, true
	}
	if !spanSet {
		checker.add(CodeInvalidCorrelation, childPath)
	}
}

// exactCompleteSelectInputAuthority is the authority-side shape proof for a
// mixed Apply span. A range denominator alone is insufficient: Group, Merge,
// or another range-producing tree would not have the mounted posting payload
// required by the replay ABI.
func exactCompleteSelectInputAuthority(expression algebra.Expression) (model.DenominatorRef, model.RelationID, bool) {
	var complete algebra.Complete
	switch value := expression.(type) {
	case algebra.Complete:
		complete = value
	case *algebra.Complete:
		if value == nil {
			return model.DenominatorRef{}, model.RelationID{}, false
		}
		complete = *value
	default:
		return model.DenominatorRef{}, model.RelationID{}, false
	}
	if !complete.Denominator().Available() {
		return model.DenominatorRef{}, model.RelationID{}, false
	}
	var selectExpression algebra.Select
	switch value := complete.Child().(type) {
	case algebra.Select:
		selectExpression = value
	case *algebra.Select:
		if value == nil {
			return model.DenominatorRef{}, model.RelationID{}, false
		}
		selectExpression = *value
	default:
		return model.DenominatorRef{}, model.RelationID{}, false
	}
	relation, ok := directInputRelation(selectExpression.Child())
	if !ok || relation != complete.Denominator().Relation() {
		return model.DenominatorRef{}, model.RelationID{}, false
	}
	return complete.Denominator(), relation, true
}

func firstSlotForAuthorityChild(slots []algebra.SlotSource, child, before int) int {
	for index := 0; index < before; index++ {
		if int(slots[index].Child()) == child {
			return index
		}
	}
	return before
}

func directInputRelation(expression algebra.Expression) (model.RelationID, bool) {
	switch value := expression.(type) {
	case algebra.Input:
		return value.Relation(), value.Relation().Available()
	case *algebra.Input:
		if value == nil {
			return model.RelationID{}, false
		}
		return value.Relation(), value.Relation().Available()
	default:
		return model.RelationID{}, false
	}
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
