package typing

import (
	"bytes"
	"fmt"
	"sort"

	checkregistry "github.com/wippyai/go-lua/analysis/relation/check/registry"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// Check independently validates one unchecked ExecutionSchema. It never
// calls a compiler, declaration validator, generated binding, or runtime
// helper. The returned report is deterministic for a fixed logical artifact.
func Check(schema plan.ExecutionSchema) Report {
	indexed := checkregistry.Build(schema)
	report := CheckView(indexed)
	for _, issue := range indexed.Issues() {
		report.addRegistryIssue(issue)
	}
	report.sort()
	return report
}

// CheckView runs the typing proof against an already indexed schema. A
// certificate composes this entry point with the authority and recurrence
// passes so every pass observes one immutable registry.
func CheckView(indexed *checkregistry.View) Report {
	if indexed == nil {
		indexed = checkregistry.Build(plan.ExecutionSchema{})
	}
	schema := indexed.Schema()
	report := Report{}
	checker := checker{schema: schema, registry: indexed, report: &report, shapes: make(map[model.ExpressionID]shape), visiting: make(map[model.ExpressionID]bool)}
	checker.validateDeclarations()
	checker.checkSignatures()
	checker.checkExpressions()
	report.sort()
	report.algebraRequirements = deriveAlgebraRequirements(indexed)
	return report
}

// deriveAlgebraRequirements is the one typing-side projection consumed by
// mount. Relation columns cover values that can enter committed state, while
// signature inputs and outputs cover semantic frames and operation results.
// The checker has already validated these declarations before this projection
// is built; no Merge-specific collection is used as an authority here.
func deriveAlgebraRequirements(indexed *checkregistry.View) []model.TypeID {
	if indexed == nil {
		return nil
	}
	seen := make(map[model.TypeID]struct{})
	for _, column := range indexed.Columns() {
		if column.Available() {
			seen[column.Type()] = struct{}{}
		}
	}
	for _, value := range indexed.Signatures() {
		if !value.Available() {
			continue
		}
		for _, input := range value.Inputs() {
			if input.Available() {
				seen[input.Type] = struct{}{}
			}
		}
		for _, output := range value.Outputs() {
			if output.Available() {
				seen[output.Type] = struct{}{}
			}
		}
	}
	result := make([]model.TypeID, 0, len(seen))
	for typeID := range seen {
		result = append(result, typeID)
	}
	sort.Slice(result, func(left, right int) bool { return typeIDLess(result[left], result[right]) })
	return result
}

func typeIDLess(left, right model.TypeID) bool {
	leftOwner, rightOwner := left.Owner().Content(), right.Owner().Content()
	if comparison := bytes.Compare(leftOwner[:], rightOwner[:]); comparison != 0 {
		return comparison < 0
	}
	leftContent, rightContent := left.Content(), right.Content()
	return bytes.Compare(leftContent[:], rightContent[:]) < 0
}

// Validate is a convenience adapter for callers that only need an error.
// The independent Report remains available through Check for certificate
// construction and mutation-law diagnostics.
func Validate(schema plan.ExecutionSchema) error { return Check(schema).Error() }

type checker struct {
	schema   plan.ExecutionSchema
	registry *checkregistry.View
	report   *Report
	shapes   map[model.ExpressionID]shape
	visiting map[model.ExpressionID]bool
}

func (checker *checker) checkSignatures() {
	for _, identity := range checker.registry.SignatureIdentities() {
		signatureValue, ok := checker.registry.Signature(identity)
		if !ok {
			continue
		}
		checker.checkSignature(signatureValue)
	}
}

func (checker *checker) checkSignature(value signature.Signature) {
	if !value.Available() {
		return
	}
	identity := value.Identity()
	path := signaturePath(identity)
	fence := value.Fence()
	if !fence.Available() {
		checker.report.add(CodeUnavailable, path, "signature fence is unavailable")
	} else {
		if fence.Schema != checker.schema.SchemaID() {
			checker.report.add(CodeSchemaIdentity, path, "signature does not carry the exact execution schema identity")
		}
		if fence.Owner != identity.Operation.Owner() {
			checker.report.add(CodeForeignReference, path, "signature fence owner differs from operation owner")
		}
	}
	if !value.Cardinality().Available() {
		checker.report.add(CodeOperatorContract, path, "signature cardinality is unavailable")
	}
	if !value.Authority().Available() {
		checker.report.add(CodeDenominatorMismatch, path, "signature output denominator is unavailable")
	} else {
		checker.checkDenominator(value.Authority().Denominator, path+".authority")
		if outputs := value.Outputs(); len(outputs) == 0 {
			checker.report.add(CodeDenominatorMismatch, path, "signature has output authority but no output columns")
		} else if value.Authority().Denominator.Relation() != outputs[0].Relation {
			checker.report.add(CodeDenominatorMismatch, path, "signature output denominator is not owned by output relation")
		}
	}
	if value.InputLen() == 0 && value.OutputLen() == 0 {
		checker.report.add(CodeOperatorContract, path, "signature has neither input nor output")
	}

	seenInputs := make(map[model.ColumnID]struct{})
	for index, input := range value.Inputs() {
		inputPath := fmt.Sprintf("%s.input[%d]", path, index)
		if !input.Available() {
			checker.report.add(CodeUnavailable, inputPath, "input contract is unavailable")
			continue
		}
		checker.checkColumnType(input.Relation, input.Column, input.Type, inputPath)
		checker.checkDenominator(input.Denominator, inputPath+".denominator")
		if input.Denominator.Relation() != input.Relation {
			checker.report.add(CodeDenominatorMismatch, inputPath, "input denominator is not owned by input relation")
		}
		if !input.Presence.Input() {
			checker.report.add(CodeOperatorContract, inputPath, "input presence contract is not an input form")
		}
		checker.checkDelivery(input.Delivery, input.Relation, input.Denominator, inputPath)
		if _, duplicate := seenInputs[input.Column]; duplicate {
			checker.report.add(CodeDuplicateMember, inputPath, "signature repeats an input column")
		}
		seenInputs[input.Column] = struct{}{}
	}

	seenOutputs := make(map[outputIdentity]struct{})
	var outputRelation model.RelationID
	for index, output := range value.Outputs() {
		outputPath := fmt.Sprintf("%s.output[%d]", path, index)
		if !output.Available() {
			checker.report.add(CodeUnavailable, outputPath, "output contract is unavailable")
			continue
		}
		checker.checkColumnType(output.Relation, output.Column, output.Type, outputPath)
		if !output.Presence.Output() {
			checker.report.add(CodeOperatorContract, outputPath, "output presence contract is not an output form")
		}
		if outputRelation.Available() && output.Relation != outputRelation {
			checker.report.add(CodeShapeMismatch, outputPath, "semantic operation outputs more than one relation")
		} else {
			outputRelation = output.Relation
		}
		key := outputIdentity{Relation: output.Relation, Column: output.Column}
		if _, duplicate := seenOutputs[key]; duplicate {
			checker.report.add(CodeDuplicateMember, outputPath, "signature repeats an output column")
		}
		seenOutputs[key] = struct{}{}
	}

	// A signature must expose at least one closed outcome.  There is no
	// arbitrary outcome callback and no hidden refusal path.
	allowed := 0
	for _, code := range []outcome.Code{outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused} {
		if value.Allows(code) {
			allowed++
		}
	}
	if allowed == 0 {
		checker.report.add(CodeOperatorContract, path, "signature has no allowed outcome")
	}
}

func (checker *checker) checkDelivery(delivery signature.Delivery, relation model.RelationID, denominator model.DenominatorRef, path string) {
	if !delivery.Available() {
		checker.report.add(CodeDeliveryMismatch, path, "delivery contract is unavailable")
		return
	}
	if delivery.IsScalar() {
		return
	}
	order := delivery.OrderKey()
	key, ok := checker.registry.Key(order)
	if !ok {
		checker.report.add(CodeMissingReference, path, "delivery order key is not registered")
		return
	}
	if !key.Available() {
		return
	}
	if key.Relation() != relation || order.Relation() != denominator.Relation() {
		checker.report.add(CodeDeliveryMismatch, path, "delivery order key is not owned by the input denominator")
	}
	if delivery.Kind == signature.BoundedSpanDelivery {
		if limit, ok := delivery.Limit(); !ok || limit == 0 {
			checker.report.add(CodeDeliveryMismatch, path, "bounded delivery has no positive bound")
		}
	}
}

func (checker *checker) checkExpressions() {
	for _, id := range checker.registry.ExpressionIDs() {
		checker.expression(id)
	}
}

func (checker *checker) expression(id model.ExpressionID) (shape, bool) {
	if result, ok := checker.shapes[id]; ok {
		return result, result.valid()
	}
	if checker.visiting[id] {
		checker.report.add(CodeExpressionCycle, expressionPath(id), "expression DAG contains a cycle")
		return shape{}, false
	}
	entry, ok := checker.registry.Expression(id)
	if !ok {
		checker.report.add(CodeMissingReference, expressionPath(id), "expression reference is not registered")
		return shape{}, false
	}
	checker.visiting[id] = true
	defer delete(checker.visiting, id)
	if !entry.Available() || entry.Expression() == nil {
		checker.shapes[id] = shape{}
		return shape{}, false
	}
	result := checker.node(entry.Expression(), expressionPath(id))
	checker.shapes[id] = result
	return result, result.valid()
}

func (checker *checker) node(expression algebra.Expression, path string) shape {
	switch value := expression.(type) {
	case algebra.Input:
		return checker.input(value, path)
	case algebra.Select:
		return checker.selectNode(value, path)
	case algebra.Project:
		return checker.project(value, path)
	case algebra.Join:
		return checker.join(value, path)
	case algebra.Merge:
		return checker.merge(value, path)
	case algebra.Group:
		return checker.group(value, path)
	case algebra.Complete:
		return checker.complete(value, path)
	case algebra.Apply:
		return checker.apply(value, path)
	case algebra.Publish:
		return checker.publish(value, path)
	default:
		checker.report.add(CodeOperatorContract, path, "expression kind is outside the closed vocabulary")
		return shape{}
	}
}

func (checker *checker) input(value algebra.Input, path string) shape {
	relation, ok := checker.registry.Relation(value.Relation())
	if !ok {
		checker.report.add(CodeMissingReference, path, "Input relation is not registered")
		return shape{}
	}
	return checker.relationShape(relation, path)
}

func (checker *checker) selectNode(value algebra.Select, path string) shape {
	child := checker.child(value.Child(), path+".child")
	contract := value.Contract()
	if contract.Mode() != algebra.SelectByScope {
		checker.report.add(CodeOperatorContract, path, "Select has an unknown filter mode")
	}
	if !contract.Scope().Available() {
		checker.report.add(CodeScopeMismatch, path, "Select has no scope identity")
	} else if _, ok := checker.registry.Scope(contract.Scope()); !ok {
		checker.report.add(CodeMissingReference, path, "Select scope is not registered")
	}
	return child
}

func (checker *checker) project(value algebra.Project, path string) shape {
	child := checker.child(value.Child(), path+".child")
	target, ok := checker.registry.Relation(value.Contract().Target())
	if !ok {
		checker.report.add(CodeMissingReference, path, "Project target relation is not registered")
		return shape{}
	}
	if !target.Available() {
		return shape{}
	}
	key, keyOK := checker.registry.Key(value.Contract().Key())
	if !keyOK {
		checker.report.add(CodeMissingReference, path, "Project key is not registered")
	} else if key.Relation() != target.ID() {
		checker.report.add(CodeKeyMismatch, path, "Project key is not owned by target relation")
	}
	mappings := value.Contract().Mappings()
	if len(mappings) == 0 {
		checker.report.add(CodeOperatorContract, path, "Project has no column mappings")
	}
	seenSources := make(map[model.ColumnID]struct{}, len(mappings))
	seenTargets := make(map[model.ColumnID]struct{}, len(mappings))
	for index, mapping := range mappings {
		mappingPath := fmt.Sprintf("%s.mapping[%d]", path, index)
		source, sourceOK := child.column(mapping.Source())
		targetColumn, targetOK := checker.registry.Column(mapping.Target())
		if !sourceOK {
			checker.report.add(CodeMissingReference, mappingPath, "Project source column is not in child")
		}
		if !targetOK {
			checker.report.add(CodeMissingReference, mappingPath, "Project target column is not registered")
		} else if targetColumn.Relation() != target.ID() || !target.HasColumn(mapping.Target()) {
			checker.report.add(CodeMembership, mappingPath, "Project target column is not owned by target relation")
		}
		if sourceOK && targetOK && source.Type != targetColumn.Type() {
			checker.report.add(CodeTypeMismatch, mappingPath, "Project source and target column types differ")
		}
		if _, duplicate := seenSources[mapping.Source()]; duplicate {
			checker.report.add(CodeDuplicateMember, mappingPath, "Project repeats a source column")
		}
		if _, duplicate := seenTargets[mapping.Target()]; duplicate {
			checker.report.add(CodeDuplicateMember, mappingPath, "Project repeats a target column")
		}
		seenSources[mapping.Source()] = struct{}{}
		seenTargets[mapping.Target()] = struct{}{}
	}
	// Project is a typed row construction, not an open-ended partial map. All
	// target columns must have exactly one source mapping.
	if len(seenTargets) != len(target.Columns()) {
		checker.report.add(CodeShapeMismatch, path, "Project does not define every target column exactly once")
	}
	return checker.relationShape(target, path)
}

func (checker *checker) join(value algebra.Join, path string) shape {
	left := checker.child(value.Left(), path+".left")
	right := checker.child(value.Right(), path+".right")
	leftColumns := value.Contract().LeftColumns()
	rightColumns := value.Contract().RightColumns()
	if len(leftColumns) == 0 || len(leftColumns) != len(rightColumns) {
		checker.report.add(CodeOperatorContract, path, "Join requires a non-empty equal-arity column vector")
	}
	for index := 0; index < len(leftColumns) && index < len(rightColumns); index++ {
		joinPath := fmt.Sprintf("%s.column[%d]", path, index)
		leftColumn, leftOK := left.column(leftColumns[index])
		rightColumn, rightOK := right.column(rightColumns[index])
		if !leftOK || !rightOK {
			checker.report.add(CodeMissingReference, joinPath, "Join column is not present in its child")
			continue
		}
		if leftColumn.Type != rightColumn.Type {
			checker.report.add(CodeTypeMismatch, joinPath, "Join columns have different TypeIDs")
		}
	}
	result := shape{columns: append(append([]columnType(nil), left.columns...), right.columns...)}
	if len(result.columns) != uniqueColumnCount(result.columns) {
		checker.report.add(CodeShapeMismatch, path, "Join output repeats a nominal column identity")
	}
	return result
}

func (checker *checker) merge(value algebra.Merge, path string) shape {
	children := value.Inputs()
	if len(children) == 0 {
		checker.report.add(CodeOperatorContract, path, "Merge requires at least one input")
		return shape{}
	}
	keyID := value.Contract().Key()
	key, keyOK := checker.registry.Key(keyID)
	if !keyOK {
		checker.report.add(CodeMissingReference, path, "Merge key is not registered")
	}
	var result shape
	for index, childExpression := range children {
		child := checker.child(childExpression, fmt.Sprintf("%s.input[%d]", path, index))
		if !child.valid() {
			continue
		}
		if index == 0 {
			result = child
		} else if !sameShape(result, child) {
			checker.report.add(CodeShapeMismatch, path, "Merge inputs do not have one typed row shape")
		}
		if keyOK && key.Available() && (child.relation != key.Relation() || !child.hasKey(keyID)) {
			checker.report.add(CodeKeyMismatch, path, "Merge key is not in every input relation")
		}
	}
	if result.valid() {
		for _, column := range result.columns {
			checker.report.requirements = append(checker.report.requirements, MergeRequirement{Path: path, Column: column.ID, Type: column.Type})
		}
	}
	return result
}

func (checker *checker) group(value algebra.Group, path string) shape {
	child := checker.child(value.Child(), path+".child")
	key, ok := checker.registry.Key(value.Contract().Key())
	if !ok || !key.Available() {
		checker.report.add(CodeMissingReference, path, "Group key is not registered")
	} else if child.relation != key.Relation() || !child.hasKey(key.ID()) {
		checker.report.add(CodeKeyMismatch, path, "Group key is not in the child relation")
	}
	if !value.Contract().Cardinality().Available() {
		checker.report.add(CodeOperatorContract, path, "Group cardinality is unavailable")
	}
	return child
}

func (checker *checker) complete(value algebra.Complete, path string) shape {
	child := checker.child(value.Child(), path+".child")
	denominator := value.Denominator()
	checker.checkDenominator(denominator, path+".denominator")
	if child.relation.Available() && denominator.Available() && child.relation != denominator.Relation() {
		checker.report.add(CodeDenominatorMismatch, path, "Complete denominator is not the child relation")
	}
	return child
}

func (checker *checker) apply(value algebra.Apply, path string) shape {
	contract := value.Contract()
	signatureValue, ok := checker.registry.Signature(contract.Operation())
	if !ok || !signatureValue.Available() {
		checker.report.add(CodeSignatureMismatch, path, "Apply operation does not resolve to an exact registered signature")
		return shape{}
	}
	inputs := value.Inputs()
	if len(inputs) != signatureValue.InputLen() {
		checker.report.add(CodeShapeMismatch, path, "Apply input count differs from exact signature")
	}
	limit := len(inputs)
	if signatureValue.InputLen() < limit {
		limit = signatureValue.InputLen()
	}
	for index := 0; index < limit; index++ {
		inputShape := checker.child(inputs[index], fmt.Sprintf("%s.input[%d]", path, index))
		input, _ := signatureValue.InputAt(index)
		column, ok := inputShape.column(input.Column)
		if !ok {
			checker.report.add(CodeMissingReference, fmt.Sprintf("%s.input[%d]", path, index), "Apply input column is not in child result")
			continue
		}
		if inputShape.relation != input.Relation {
			checker.report.add(CodeMembership, fmt.Sprintf("%s.input[%d]", path, index), "Apply input relation differs from child result")
		}
		if column.Type != input.Type {
			checker.report.add(CodeTypeMismatch, fmt.Sprintf("%s.input[%d]", path, index), "Apply input TypeID differs from signature")
		}
		checker.checkAppliedDelivery(input, inputShape, fmt.Sprintf("%s.input[%d]", path, index))
	}
	outputs := signatureValue.Outputs()
	if len(outputs) == 0 {
		checker.report.add(CodeShapeMismatch, path, "Apply signature has no output columns")
		return shape{}
	}
	result := shape{relation: outputs[0].Relation}
	for _, output := range outputs {
		if output.Relation != result.relation {
			continue
		}
		result.columns = append(result.columns, columnType{ID: output.Column, Type: output.Type})
	}
	if relation, ok := checker.registry.Relation(result.relation); ok {
		result.keys = relation.Keys()
	}
	return result
}

func (checker *checker) checkAppliedDelivery(input signature.Input, child shape, path string) {
	if !input.Delivery.Available() || !child.relation.Available() {
		return
	}
	if input.Denominator.Relation() != child.relation {
		checker.report.add(CodeDenominatorMismatch, path, "Apply input denominator is not the child result relation")
	}
	if input.Delivery.IsScalar() {
		return
	}
	order := input.Delivery.OrderKey()
	key, ok := checker.registry.Key(order)
	if !ok || !key.Available() {
		return
	}
	if order.Relation() != child.relation || !child.hasKey(order) {
		checker.report.add(CodeDeliveryMismatch, path, "Apply span delivery order key is not in the child result")
	}
}

func (checker *checker) publish(value algebra.Publish, path string) shape {
	child := checker.child(value.Child(), path+".child")
	contract := value.Contract()
	destination, ok := checker.registry.Relation(contract.Destination())
	if !ok {
		checker.report.add(CodeMissingReference, path, "Publish destination relation is not registered")
		return shape{}
	}
	if !destination.Available() {
		return shape{}
	}
	key, ok := checker.registry.Key(contract.Key())
	if !ok {
		checker.report.add(CodeMissingReference, path, "Publish key is not registered")
	} else if key.Relation() != destination.ID() {
		checker.report.add(CodeKeyMismatch, path, "Publish key is not owned by destination relation")
	}
	if child.relation != destination.ID() {
		checker.report.add(CodeMembership, path, "Publish child relation differs from destination")
	}
	for _, targetColumn := range destination.Columns() {
		want, ok := checker.registry.Column(targetColumn)
		got, present := child.column(targetColumn)
		if !ok || !present {
			checker.report.add(CodeShapeMismatch, path, "Publish child does not provide every destination column")
			continue
		}
		if want.Type() != got.Type {
			checker.report.add(CodeTypeMismatch, path, "Publish child column type differs from destination")
		}
	}
	return destinationShape(destination, checker.registry)
}

func (checker *checker) child(expression algebra.Expression, path string) shape {
	if expression == nil {
		checker.report.add(CodeUnavailable, path, "child expression is nil")
		return shape{}
	}
	// Nested algebra nodes are retained directly in the expression registry.
	// Validate them through a synthetic stable path; dependency references are
	// the only IDs in the plan and are checked separately.
	return checker.node(expression, path)
}

func (checker *checker) relationShape(relation model.RelationSchema, path string) shape {
	if !relation.Available() {
		return shape{}
	}
	result := shape{relation: relation.ID(), keys: relation.Keys()}
	for index, id := range relation.Columns() {
		column, ok := checker.registry.Column(id)
		if !ok {
			checker.report.add(CodeMissingReference, fmt.Sprintf("%s.column[%d]", path, index), "relation shape column is not registered")
			continue
		}
		result.columns = append(result.columns, columnType{ID: id, Type: column.Type()})
	}
	return result
}

func destinationShape(relation model.RelationSchema, registry *checkregistry.View) shape {
	result := shape{relation: relation.ID(), keys: relation.Keys()}
	for _, id := range relation.Columns() {
		if column, ok := registry.Column(id); ok {
			result.columns = append(result.columns, columnType{ID: id, Type: column.Type()})
		}
	}
	return result
}

func (checker *checker) checkColumnType(relationID model.RelationID, columnID model.ColumnID, typeID model.TypeID, path string) {
	relation, relationOK := checker.registry.Relation(relationID)
	column, columnOK := checker.registry.Column(columnID)
	if (relationOK && !relation.Available()) || (columnOK && !column.Available()) {
		return
	}
	if !relationOK {
		checker.report.add(CodeMissingReference, path, "column relation is not registered")
	}
	if !columnOK {
		checker.report.add(CodeMissingReference, path, "column is not registered")
	} else {
		if column.Relation() != relationID {
			checker.report.add(CodeMembership, path, "column is not owned by declared relation")
		}
		if column.Type() != typeID {
			checker.report.add(CodeTypeMismatch, path, "declared TypeID differs from column schema")
		}
	}
	if !typeID.Available() {
		checker.report.add(CodeUnavailable, path, "TypeID is unavailable")
	}
	if relationOK && !relation.HasColumn(columnID) {
		checker.report.add(CodeMembership, path, "column is absent from declared relation")
	}
}

func (checker *checker) checkDenominator(value model.DenominatorRef, path string) {
	if !value.Available() {
		checker.report.add(CodeDenominatorMismatch, path, "denominator reference is unavailable")
		return
	}
	relation, relationOK := checker.registry.Relation(value.Relation())
	key, keyOK := checker.registry.Key(value.Key())
	if (relationOK && !relation.Available()) || (keyOK && !key.Available()) {
		return
	}
	if !relationOK {
		checker.report.add(CodeMissingReference, path, "denominator relation is not registered")
	}
	if !keyOK {
		checker.report.add(CodeMissingReference, path, "denominator key is not registered")
	}
	if relationOK && !relation.HasKey(value.Key()) {
		checker.report.add(CodeMembership, path, "denominator key is absent from relation")
	}
	if keyOK && key.Relation() != value.Relation() {
		checker.report.add(CodeMembership, path, "denominator key belongs to another relation")
	}
}

func (report *Report) add(code Code, path, detail string) {
	*report = Report{
		issues:              append(report.issues, Issue{Code: code, Path: path, Detail: detail}),
		requirements:        report.requirements,
		algebraRequirements: report.algebraRequirements,
	}
}

func (report *Report) sort() {
	sort.SliceStable(report.issues, func(left, right int) bool {
		if report.issues[left].Path != report.issues[right].Path {
			return report.issues[left].Path < report.issues[right].Path
		}
		if report.issues[left].Code != report.issues[right].Code {
			return report.issues[left].Code < report.issues[right].Code
		}
		return report.issues[left].Detail < report.issues[right].Detail
	})
	sort.SliceStable(report.requirements, func(left, right int) bool {
		if report.requirements[left].Path != report.requirements[right].Path {
			return report.requirements[left].Path < report.requirements[right].Path
		}
		return report.requirements[left].Column.Content()[0] < report.requirements[right].Column.Content()[0]
	})
}
