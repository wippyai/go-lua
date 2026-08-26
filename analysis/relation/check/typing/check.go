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
	checker.checkInitials()
	checker.checkExpressions()
	checker.checkTypeCapabilities()
	report.sort()
	report.algebraRequirements = checker.deriveAlgebraRequirements()
	return report
}

// deriveAlgebraRequirements is the one typing-side projection consumed by
// mount. A checked Merge requires a sealed ascending authority for every
// non-key output cell, and a committed Present output is an ascent site.
// Merely declaring a Present output in a signature is not an execution use.
// Keep every valid obligation in this projection; capability validation must
// not filter a DecodeOnly/Equatable mismatch into an apparently complete
// certificate.
func (checker *checker) deriveAlgebraRequirements() []model.TypeID {
	seen := make(map[model.TypeID]struct{})
	add := func(typeID model.TypeID) {
		if typeID.Available() {
			seen[typeID] = struct{}{}
		}
	}
	for _, requirement := range checker.report.requirements {
		add(requirement.Type)
	}
	// PresentRequirements is populated by checker.publish. It is intentionally
	// not reconstructed from the signature catalogue here: doing so would
	// reintroduce the Presence=>ascent inference this pass is meant to remove.
	// The caller folds this list into the report before invoking this helper.
	for _, requirement := range checker.report.presentRequirements {
		add(requirement.Type)
	}
	result := make([]model.TypeID, 0, len(seen))
	for typeID := range seen {
		result = append(result, typeID)
	}
	sort.Slice(result, func(left, right int) bool { return typeIDLess(result[left], result[right]) })
	return result
}

// checkTypeCapabilities enforces the explicit schema policy at the point
// where a type is consumed. A Merge needs Ascending for every checked
// non-key output cell because the physical reducer invokes Join; no
// DecodeOnly/Equatable fallback is sound. A committed Present output asks to
// ascend. AuthenticatedOpaque and unused outputs deliberately need no lattice
// authority.
func (checker *checker) checkTypeCapabilities() {
	for _, requirement := range checker.report.equalityRequirements {
		checker.requireEquatable(requirement.Type, requirement.Path, "semantic key operation")
	}
	for _, requirement := range checker.report.requirements {
		checker.requireAscending(requirement.Type, requirement.Path, "Merge output")
	}
	for _, requirement := range checker.report.presentRequirements {
		checker.requireAscending(requirement.Type, requirement.Path, "committed Present output")
	}
}

func (checker *checker) requireEquatable(typeID model.TypeID, path, use string) {
	if !typeID.Available() {
		return
	}
	capability, ok := checker.registry.TypeCapability(typeID)
	if !ok {
		checker.report.add(CodeTypeCapabilityMismatch, path, fmt.Sprintf("%s TypeID has no sealed capability", use))
		return
	}
	if !capability.Equatable() {
		checker.report.add(CodeTypeCapabilityMismatch, path, fmt.Sprintf("%s requires Equatable capability, got %s", use, capability.Kind()))
	}
}

func (checker *checker) requireAscending(typeID model.TypeID, path, use string) {
	if !typeID.Available() {
		return
	}
	capability, ok := checker.registry.TypeCapability(typeID)
	if !ok {
		checker.report.add(CodeTypeCapabilityMismatch, path, fmt.Sprintf("%s TypeID has no sealed capability", use))
		return
	}
	if !capability.Ascending() {
		checker.report.add(CodeTypeCapabilityMismatch, path, fmt.Sprintf("%s requires Ascending capability, got %s", use, capability.Kind()))
	}
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
	// directSeedApply is scoped while checking the immediate child of a
	// Publish. It is the sole exception to ordinary relational Apply: a
	// zero-input owner seed can certify its exact write at that boundary, but
	// cannot become a standalone relational producer.
	directSeedApply bool
	// readRoot/readOrdinal provide checker-local occurrence identity while a
	// sealed expression tree is walked. They are not schema or runtime state.
	readRoot    model.ExpressionID
	readOrdinal uint32
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

// checkInitials validates the schema-owned zero-input invocation rows. An
// initial is not a runtime request and it cannot be recovered from an
// unrelated Publish expression: its exact signature and scope are part of the
// sealed logical artifact. Present-capable outputs are real committed writes,
// so they contribute the same Ascending obligation as an expression-backed
// Publish boundary.
func (checker *checker) checkInitials() {
	present, presentOK := model.NewPresence(model.Present)
	if !presentOK {
		return
	}
	for _, initial := range checker.registry.Initials() {
		path := checkregistry.InitialPath(initial)
		operation := initial.Operation()
		value, signatureOK := checker.registry.Signature(operation)
		if !signatureOK || !value.Available() {
			checker.report.add(CodeMissingReference, path, "initial signature is not registered")
			continue
		}
		if value.InputLen() != 0 {
			checker.report.add(CodeShapeMismatch, path, "initial signature must have zero inputs")
		}
		scope, scopeOK := checker.registry.Scope(initial.Scope())
		if !scopeOK || !scope.Available() {
			checker.report.add(CodeMissingReference, path, "initial scope is not registered")
		} else if initial.Scope().Owner() != operation.Operation.Owner() {
			checker.report.add(CodeForeignReference, path, "initial scope owner differs from operation owner")
		}
		for index, output := range value.Outputs() {
			if !output.Available() || !output.Presence.Allows(present) {
				continue
			}
			checker.report.presentRequirements = append(checker.report.presentRequirements, PresentRequirement{
				Path: fmt.Sprintf("%s.output[%d]", path, index), Column: output.Column, Type: output.Type,
			})
		}
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
	if value.InputLen() == 0 && value.OutputLen() == 0 {
		checker.report.add(CodeOperatorContract, path, "signature has neither input nor output")
	}

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
		// Inputs are positional semantic slots. The same declared column may be
		// read through two authored row occurrences (for example, a self-join).
		// ApplySlot's sealed child/cell address—not nominal ColumnID
		// uniqueness—keeps those reads distinct.
	}

	seenOutputs := make(map[outputIdentity]struct{})
	for index, output := range value.Outputs() {
		outputPath := fmt.Sprintf("%s.output[%d]", path, index)
		if !output.Available() {
			checker.report.add(CodeUnavailable, outputPath, "output contract is unavailable")
			continue
		}
		checker.checkColumnType(output.Relation, output.Column, output.Type, outputPath)
		checker.checkDenominator(output.Denominator, outputPath+".denominator")
		if output.Denominator.Available() && output.Denominator.Relation() != output.Relation {
			checker.report.add(CodeDenominatorMismatch, outputPath, "output denominator is not owned by output relation")
		}
		if !output.Presence.Output() {
			checker.report.add(CodeOperatorContract, outputPath, "output presence contract is not an output form")
		}
		key := outputIdentity{Relation: output.Relation, Column: output.Column}
		if _, duplicate := seenOutputs[key]; duplicate {
			checker.report.add(CodeDuplicateMember, outputPath, "signature repeats an output column")
		}
		seenOutputs[key] = struct{}{}
	}
	if value.Cardinality().Kind() == model.CompleteDenominator {
		checker.checkCompleteDenominator(value.Outputs(), path)
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

// checkCompleteDenominator validates the only schema authority for a
// mount-dependent complete result: its declared output columns. A numeric
// bound cannot describe the mounted row count, so at least one output must
// name a denominator and every output must name that exact same reference.
func (checker *checker) checkCompleteDenominator(outputs []signature.Output, path string) {
	if len(outputs) == 0 {
		checker.report.add(CodeOperatorContract, path, "CompleteDenominator requires at least one output")
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
			checker.report.add(CodeDenominatorMismatch, fmt.Sprintf("%s.output[%d].denominator", path, index), "CompleteDenominator outputs must share one exact denominator")
		}
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
	previousRoot, previousOrdinal := checker.readRoot, checker.readOrdinal
	checker.readRoot, checker.readOrdinal = id, 0
	result := checker.node(entry.Expression(), expressionPath(id))
	checker.readRoot, checker.readOrdinal = previousRoot, previousOrdinal
	checker.shapes[id] = result
	return result, result.valid()
}

func (checker *checker) nextReadOccurrence() readOccurrence {
	occurrence := readOccurrence{root: checker.readRoot, ordinal: checker.readOrdinal}
	checker.readOrdinal++
	return occurrence
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
	case algebra.Expand:
		return checker.expand(value, path)
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
	case algebra.ColumnProject:
		return checker.columnProject(value, path)
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

	// NewInput is deliberately an explicit whole-row form.  It is the only
	// Input whose shape is resolved from the relation declaration; an exact
	// Input must retain the occurrence-local vector it carries so every
	// downstream SlotSource cell remains positional.
	if value.AllColumns() {
		return checker.relationShape(relation, path)
	}

	if value.Projection() != algebra.InputProjectionExactColumns {
		checker.report.add(CodeOperatorContract, path, "Input projection mode is unavailable")
		return shape{}
	}
	return checker.exactInputShape(value, relation, path)
}

// exactInputShape resolves one Input's ordered source vector without
// widening it to the relation's authored row.  The relation catalogue owns
// the semantic type of each nominal column; the checker proves that every
// vector member is registered, relation-owned, and actually present in the
// authored relation before carrying that type into the positional shape.
func (checker *checker) exactInputShape(value algebra.Input, relation model.RelationSchema, path string) shape {
	columns := value.Columns()
	if len(columns) == 0 {
		checker.report.add(CodeOperatorContract, path, "ExactColumns requires at least one column")
		return shape{}
	}

	result := shape{relation: relation.ID(), sources: []model.RelationID{relation.ID()}}
	seen := make(map[model.ColumnID]struct{}, len(columns))
	for index, id := range columns {
		columnPath := fmt.Sprintf("%s.column[%d]", path, index)
		if _, duplicate := seen[id]; duplicate {
			checker.report.add(CodeDuplicateMember, columnPath, "Input exact projection repeats a column")
			continue
		}
		seen[id] = struct{}{}

		column, columnOK := checker.registry.Column(id)
		if !columnOK {
			checker.report.add(CodeMissingReference, columnPath, "Input exact projection column is not registered")
			continue
		}
		if !column.Available() {
			checker.report.add(CodeUnavailable, columnPath, "Input exact projection column is unavailable")
			continue
		}
		if column.Relation() != relation.ID() {
			checker.report.add(CodeMembership, columnPath, "Input exact projection column is owned by another relation")
			continue
		}
		if !relation.HasColumn(id) {
			checker.report.add(CodeMembership, columnPath, "Input exact projection column is absent from the authored relation")
			continue
		}
		if !column.Type().Available() {
			checker.report.add(CodeUnavailable, columnPath, "Input exact projection column type is unavailable")
			continue
		}

		// Do not sort or canonicalize this append: the authored exact vector is
		// the positional ABI consumed by Apply SlotSource and ColumnProject.
		result.columns = append(result.columns, columnType{ID: id, Type: column.Type(), Source: 0})
	}
	result.keys = checker.keysForColumns(relation, result.columns)
	result.rowKeys = append([]model.KeyID(nil), result.keys...)
	return result
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
	// Project redeems a destination row by its declared key. The physical
	// reader may therefore compare independently issued owner values at every
	// key position; require the owner equality witness explicitly rather than
	// letting the index's opaque token identity become a hidden fallback.
	if keyOK && key.Relation() == target.ID() {
		for index, columnID := range key.Columns() {
			column, columnOK := checker.registry.Column(columnID)
			if !columnOK || column.Relation() != target.ID() {
				continue
			}
			checker.report.equalityRequirements = append(checker.report.equalityRequirements,
				EqualityRequirement{
					Path:   fmt.Sprintf("%s.key[%d]", path, index),
					Column: columnID,
					Type:   column.Type(),
				},
			)
		}
	}
	// Project is a typed row construction, not an open-ended partial map. All
	// target columns must have exactly one source mapping.
	if len(seenTargets) != len(target.Columns()) {
		checker.report.add(CodeShapeMismatch, path, "Project does not define every target column exactly once")
	}
	result := checker.relationShape(target, path)
	// ProjectInto retains the child source vector and appends the target row
	// whose cells it constructs. The logical expression has one child, but the
	// sealed Project binding supplies that target occurrence at mount; make the
	// tuple address explicit here so a later Apply never guesses it by column.
	result.sources = append(append([]model.RelationID(nil), child.sources...), target.ID())
	for index := range result.columns {
		result.columns[index].Source = uint32(len(child.sources))
	}
	// Project is a one-row-to-one-row construction. It does not create a new
	// range, but it preserves an already proven boundary; the order is still
	// checked against the target key by Apply's delivery contract.
	result.rangeBound = child.rangeBound
	return result
}

func (checker *checker) join(value algebra.Join, path string) shape {
	occurrence := checker.nextReadOccurrence()
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
		checker.report.equalityRequirements = append(checker.report.equalityRequirements,
			EqualityRequirement{Path: joinPath + ".left", Column: leftColumn.ID, Type: leftColumn.Type},
			EqualityRequirement{Path: joinPath + ".right", Column: rightColumn.ID, Type: rightColumn.Type},
		)
	}
	// Joins are oriented relational extensions: the left row identity and
	// publication key remain the identity of the result while the right side
	// contributes columns.  Dropping that identity here made a valid join
	// appear relation-less to a following Apply/Publish node, so the checker
	// rejected the very plans the compiler emits.  The oracle and the physical
	// kernels use the same left-oriented contract.
	// A repeated read relation is a normal relational extension: the second
	// join contributes a new checker family for each nominal column it reads.
	// The family qualifier is deliberately absent from schema/runtime identity;
	// it only keeps the two reads distinguishable while the Apply input shape is
	// checked. Rejecting the same family twice still catches a malformed read.
	result := shape{relation: left.relation, keys: append([]model.KeyID(nil), left.keys...), sources: append([]model.RelationID(nil), left.sources...)}
	result.columns = append([]columnType(nil), left.columns...)
	rightOffset := uint32(len(result.sources))
	result.sources = append(result.sources, right.sources...)
	for _, rightColumn := range right.columns {
		family := rightColumn
		family.Source += rightOffset
		if !family.Occurrence.available() {
			family.Occurrence = occurrence
		}
		if hasColumnFamily(result, family) {
			// A repeated nominal column from the same read is still a
			// malformed row. Only a different read occurrence is allowed to
			// contribute the same schema ColumnID.
			checker.report.add(CodeShapeMismatch, path, "Join read repeats a column family")
			continue
		}
		result.columns = append(result.columns, family)
	}
	// A join can multiply a row and therefore cannot promise the input's
	// ordered range partition. Group or Complete must re-establish it before a
	// span delivery is admitted.
	result.rangeBound = false
	return result
}

// expand validates the dependent keyed join as one sealed contract. The
// vector contents are mount evidence and therefore never appear in this
// schema pass. The typed result keeps the C-left cells and appends one R row
// occurrence, exactly like the logical operator's output contract states.
func (checker *checker) expand(value algebra.Expand, path string) shape {
	occurrence := checker.nextReadOccurrence()
	left := checker.child(value.Child(), path+".child")
	contract := value.Contract()
	if !contract.Available() {
		checker.report.add(CodeOperatorContract, path, "Expand contract is unavailable")
		return left
	}
	candidate, candidateOK := checker.registry.Relation(contract.Candidate())
	if !candidateOK || !candidate.Available() {
		checker.report.add(CodeMissingReference, path+".candidate", "Expand candidate relation is not registered")
	} else if !containsRelationSource(left.sources, contract.Candidate()) {
		checker.report.add(CodeMembership, path+".candidate", "Expand child does not retain candidate relation")
	}
	publisher, publisherOK := checker.registry.Relation(contract.Publisher())
	if !publisherOK || !publisher.Available() {
		checker.report.add(CodeMissingReference, path+".publisher", "Expand publisher relation is not registered")
	}
	reader, readerOK := checker.registry.Relation(contract.Reader())
	if !readerOK || !reader.Available() {
		checker.report.add(CodeMissingReference, path+".reader", "Expand reader relation is not registered")
		return left
	}
	key, keyOK := checker.registry.Column(contract.Key())
	if !keyOK || !key.Available() {
		checker.report.add(CodeMissingReference, path+".key", "Expand key column is not registered")
	} else {
		if key.Relation() != reader.ID() || !reader.HasColumn(contract.Key()) {
			checker.report.add(CodeMembership, path+".key", "Expand key column is not owned by reader")
		} else {
			checker.checkExpandKeySchema(reader, contract.Key(), path+".key")
			checker.report.equalityRequirements = append(checker.report.equalityRequirements, EqualityRequirement{Path: path + ".key", Column: contract.Key(), Type: key.Type()})
		}
	}
	if correlation, ok := checker.registry.Relation(contract.Correlation()); !ok || !correlation.Available() {
		checker.report.add(CodeMissingReference, path+".correlation", "Expand correlation relation is not registered")
	}
	if scope := contract.Scope(); scope.Available() {
		if _, ok := checker.registry.Scope(scope); !ok {
			checker.report.add(CodeMissingReference, path+".scope", "Expand scope is not registered")
		}
	}
	readerShape := checker.relationShape(reader, path+".reader")
	if !readerShape.valid() {
		return left
	}
	result := left
	result.sources = append(append([]model.RelationID(nil), left.sources...), reader.ID())
	offset := uint32(len(left.sources))
	for _, column := range readerShape.columns {
		cell := column
		cell.Source += offset
		cell.Occurrence = occurrence
		result.columns = append(result.columns, cell)
	}
	result.rangeBound = true
	return result
}

func containsRelationSource(sources []model.RelationID, relation model.RelationID) bool {
	for _, source := range sources {
		if source == relation {
			return true
		}
	}
	return false
}

func hasColumnFamily(value shape, want columnType) bool {
	for _, have := range value.columns {
		if have.ID == want.ID && have.Occurrence.equal(want.Occurrence) {
			return true
		}
	}
	return false
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
	proposalMerge := false
	childShapes := make([]shape, len(children))
	for index, childExpression := range children {
		child := checker.child(childExpression, fmt.Sprintf("%s.input[%d]", path, index))
		childShapes[index] = child
		proposalMerge = proposalMerge || child.proposal
		if !child.valid() {
			continue
		}
		if index == 0 {
			result = child
		} else if !sameShape(result, child) {
			checker.report.add(CodeShapeMismatch, path, "Merge inputs do not have one typed row shape")
		}
	}
	for _, child := range childShapes {
		if !child.valid() || !keyOK || !key.Available() {
			continue
		}
		keyPresent := child.hasKey(keyID)
		if proposalMerge {
			keyPresent = child.hasRowKey(keyID)
		}
		if child.relation != key.Relation() || !keyPresent {
			checker.report.add(CodeKeyMismatch, path, "Merge key is not authenticated by every input row")
		}
	}
	if result.valid() {
		keyColumns := map[model.ColumnID]struct{}{}
		if keyOK && key.Available() {
			for _, column := range key.Columns() {
				keyColumns[column] = struct{}{}
			}
		}
		for _, column := range result.columns {
			if _, isKey := keyColumns[column.ID]; isKey {
				checker.report.equalityRequirements = append(checker.report.equalityRequirements, EqualityRequirement{Path: path + ".key", Column: column.ID, Type: column.Type})
				continue
			}
			checker.report.requirements = append(checker.report.requirements, MergeRequirement{Path: path, Column: column.ID, Type: column.Type})
		}
	}
	// Merge concatenates alternatives; even when their row shapes match it has
	// no proof that their range partitions are one exact ordered denominator.
	result.rangeBound = false
	result.proposal = proposalMerge
	return result
}

func (checker *checker) group(value algebra.Group, path string) shape {
	child := checker.child(value.Child(), path+".child")
	key, ok := checker.registry.Key(value.Contract().Key())
	if !ok || !key.Available() {
		checker.report.add(CodeMissingReference, path, "Group key is not registered")
	} else if child.relation != key.Relation() || !child.hasKey(key.ID()) {
		checker.report.add(CodeKeyMismatch, path, "Group key is not in the child relation")
	} else {
		for _, column := range key.Columns() {
			columnSchema, columnOK := checker.registry.Column(column)
			if columnOK && columnSchema.Available() {
				checker.report.equalityRequirements = append(checker.report.equalityRequirements, EqualityRequirement{Path: path + ".key", Column: column, Type: columnSchema.Type()})
			}
		}
	}
	if !value.Contract().Cardinality().Available() {
		checker.report.add(CodeOperatorContract, path, "Group cardinality is unavailable")
	} else if value.Contract().Cardinality().Kind() == model.CompleteDenominator {
		checker.report.add(CodeOperatorContract, path, "Group cannot use CompleteDenominator cardinality")
	}
	// Group is the first operator that proves one Batch per ordered key group.
	child.rangeBound = true
	child.completeDenominator = model.DenominatorRef{}
	return child
}

func (checker *checker) complete(value algebra.Complete, path string) shape {
	child := checker.child(value.Child(), path+".child")
	denominator := value.Denominator()
	checker.checkDenominator(denominator, path+".denominator")
	if child.relation.Available() && denominator.Available() && child.relation != denominator.Relation() {
		checker.report.add(CodeDenominatorMismatch, path, "Complete denominator is not the child relation")
	}
	// Complete is not layout-transparent. Its sealed binding can materialize
	// every denominator-relation cell that the child omitted, in relation
	// contract order. Redeem that one algebra-level law here so Apply slots are
	// checked against the same physical output positions mount will seal.
	if completed, completedOK := checker.completeShape(child, denominator); completedOK {
		child = completed
	} else if denominator.Available() && child.valid() {
		checker.report.add(CodeShapeMismatch, path, "Complete child/output cell layout is not canonical")
	}
	// Complete materializes the exact denominator range, including a valid
	// empty range. This is a range proof, not merely a relation/key lookup.
	child.rangeBound = true
	child.completeDenominator = denominator
	return child
}

// completeShape is the typing-side redemption of algebra.CompleteCellLayout.
// It deliberately reconstructs types only from the checked declaration
// registry; the shared layout law owns position/source order while typing owns
// nominal TypeID membership.
func (checker *checker) completeShape(child shape, denominator model.DenominatorRef) (shape, bool) {
	if !child.valid() || !denominator.Available() {
		return shape{}, false
	}
	relation, relationOK := checker.registry.Relation(denominator.Relation())
	if !relationOK || !relation.Available() {
		return shape{}, false
	}
	cells := make([]algebra.CellLayoutCell, len(child.columns))
	for index, column := range child.columns {
		cells[index] = algebra.NewCellLayoutCell(column.ID, column.Source)
	}
	layout, layoutOK := algebra.NewCellLayout(child.sources, cells)
	if !layoutOK {
		return shape{}, false
	}
	completed, completedOK := algebra.CompleteCellLayout(layout, denominator, relation.Columns())
	if !completedOK {
		return shape{}, false
	}
	prior := make(map[struct {
		column model.ColumnID
		source uint32
	}]columnType, len(child.columns))
	for _, column := range child.columns {
		key := struct {
			column model.ColumnID
			source uint32
		}{column: column.ID, source: column.Source}
		prior[key] = column
	}
	result := child
	result.columns = make([]columnType, completed.Len())
	for index := 0; index < completed.Len(); index++ {
		cell, cellOK := completed.CellAt(index)
		if !cellOK {
			return shape{}, false
		}
		key := struct {
			column model.ColumnID
			source uint32
		}{column: cell.Column(), source: cell.Source()}
		if existing, exists := prior[key]; exists {
			result.columns[index] = existing
			continue
		}
		column, columnOK := checker.registry.Column(cell.Column())
		if !columnOK || !column.Available() || column.Relation() != denominator.Relation() {
			return shape{}, false
		}
		result.columns[index] = columnType{ID: cell.Column(), Type: column.Type(), Source: cell.Source()}
	}
	result.keys = checker.keysForColumns(relation, result.columns)
	return result, true
}

// columnProject keeps a closed ordered subset of one child's already-typed
// cells.  Its positional contract prevents the next Merge or Publish from
// rediscovering a cell by nominal column name at runtime.
func (checker *checker) columnProject(value algebra.ColumnProject, path string) shape {
	child := checker.child(value.Child(), path+".child")
	slots := value.Contract().Slots()
	if len(slots) == 0 {
		checker.report.add(CodeOperatorContract, path, "ColumnProject requires at least one selected cell")
		return shape{}
	}
	result := child
	result.columns = make([]columnType, 0, len(slots))
	seen := make(map[model.ColumnID]struct{}, len(slots))
	for index, slot := range slots {
		slotPath := fmt.Sprintf("%s.slot[%d]", path, index)
		if !slot.Column().Available() {
			checker.report.add(CodeMissingReference, slotPath, "ColumnProject output column is unavailable")
			continue
		}
		if _, duplicate := seen[slot.Column()]; duplicate {
			checker.report.add(CodeDuplicateMember, slotPath, "ColumnProject repeats an output column")
			continue
		}
		seen[slot.Column()] = struct{}{}
		cell, ok := child.cell(slot.Cell())
		if !ok {
			checker.report.add(CodeMissingReference, slotPath, "ColumnProject cell ordinal is outside child shape")
			continue
		}
		if cell.ID != slot.Column() {
			checker.report.add(CodeMembership, slotPath, "ColumnProject cell ordinal does not name declared output column")
			continue
		}
		result.columns = append(result.columns, cell)
	}
	if relation, ok := checker.registry.Relation(result.relation); ok {
		// ColumnProject preserves the authenticated source row, but only keys
		// whose cells remain delivered are available to tuple operators.
		result.keys = checker.keysForColumns(relation, result.columns)
	}
	return result
}

func (checker *checker) apply(value algebra.Apply, path string) shape {
	contract := value.Contract()
	signatureValue, ok := checker.registry.Signature(contract.Operation())
	if !ok || !signatureValue.Available() {
		checker.report.add(CodeSignatureMismatch, path, "Apply operation does not resolve to an exact registered signature")
		return shape{}
	}
	inputs := value.Inputs()
	slotSource := contract.SlotSource()
	// Apply is normally a judgment over one or more delivered relation facts.
	// The immediate child of Publish is the narrow seed exception: an owner
	// zero-input signature may certify its exact destination write there. It
	// is not a free-standing relational producer and does not admit a
	// zero-child evaluator path.
	if len(inputs) == 0 {
		if !checker.directSeedApply {
			checker.report.add(CodeShapeMismatch, path, "Apply requires at least one delivered child")
		} else if signatureValue.InputLen() != 0 {
			checker.report.add(CodeShapeMismatch, path, "zero-input seed Apply requires a zero-input signature")
		}
	}
	if len(slotSource) != signatureValue.InputLen() {
		checker.report.add(CodeShapeMismatch, path, "Apply slot-source count differs from exact signature")
	}
	if len(slotSource) != 0 && len(inputs) != 0 {
		seenChildren := make([]bool, len(inputs))
		groupInputs := make([]signature.Input, len(inputs))
		groupSet := make([]bool, len(inputs))
		for slot, source := range slotSource {
			child := int(source.Child())
			if child >= len(inputs) {
				checker.report.add(CodeShapeMismatch, fmt.Sprintf("%s.slot[%d]", path, slot), "Apply slot names an unavailable child ordinal")
				continue
			}
			seenChildren[child] = true
			input, inputOK := signatureValue.InputAt(slot)
			if !inputOK {
				continue
			}
			if groupSet[child] {
				prior := groupInputs[child]
				if !sameApplyGroupInput(prior, input) {
					checker.report.add(CodeShapeMismatch, fmt.Sprintf("%s.slot[%d]", path, slot), "Apply slots sharing a child must share one row delivery contract")
				}
			} else {
				groupInputs[child] = input
				groupSet[child] = true
			}
		}
		for child, seen := range seenChildren {
			if !seen {
				checker.report.add(CodeShapeMismatch, fmt.Sprintf("%s.child[%d]", path, child), "Apply child has no delivered slot")
			}
		}
	}
	children := make([]shape, len(inputs))
	for index, childExpression := range inputs {
		children[index] = checker.child(childExpression, fmt.Sprintf("%s.child[%d]", path, index))
	}
	checker.checkApplyCorrelation(contract.Correlation(), inputs, children, signatureValue, slotSource, path)
	limit := len(slotSource)
	if signatureValue.InputLen() < limit {
		limit = signatureValue.InputLen()
	}
	for index := 0; index < limit; index++ {
		source := slotSource[index]
		childIndex := int(source.Child())
		if childIndex >= len(children) {
			continue
		}
		inputShape := children[childIndex]
		input, _ := signatureValue.InputAt(index)
		column, ok := inputShape.cell(source.Cell())
		if !ok {
			checker.report.add(CodeMissingReference, fmt.Sprintf("%s.slot[%d]", path, index), "Apply input cell ordinal is not in mapped child result")
			continue
		}
		rowRelation, rowOK := inputShape.source(column.Source)
		if !rowOK || rowRelation != input.Relation {
			checker.report.add(CodeMembership, fmt.Sprintf("%s.slot[%d]", path, index), "Apply input cell source does not own the signature relation")
		}
		if column.ID != input.Column {
			checker.report.add(CodeMembership, fmt.Sprintf("%s.slot[%d]", path, index), "Apply input cell ordinal does not name the signature column")
		}
		// A scalar slot may be supplied by a sealed composite child (normally a
		// Join) whose left spine has a different nominal relation. The requested
		// column is the authority in that case; the signature checker has already
		// proved that the column belongs to input.Relation. Span delivery is
		// different: its child must own the exact range denominator and remains
		// constrained to the child's relation below.
		if input.Delivery.IsSpan() && inputShape.relation != input.Relation {
			checker.report.add(CodeMembership, fmt.Sprintf("%s.slot[%d]", path, index), "Apply input relation differs from mapped child result")
		}
		if column.Type != input.Type {
			checker.report.add(CodeTypeMismatch, fmt.Sprintf("%s.slot[%d]", path, index), "Apply input TypeID differs from signature")
		}
		checker.checkAppliedDelivery(input, inputShape, fmt.Sprintf("%s.slot[%d]", path, index))
	}
	outputs := signatureValue.Outputs()
	if len(outputs) == 0 {
		checker.report.add(CodeShapeMismatch, path, "Apply signature has no output columns")
		return shape{}
	}
	result := shape{relation: outputs[0].Relation, sources: []model.RelationID{outputs[0].Relation}}
	for _, output := range outputs {
		if output.Relation != result.relation {
			continue
		}
		result.columns = append(result.columns, columnType{ID: output.Column, Type: output.Type, Source: 0})
	}
	// Apply publishes a proposal for the exact owner-authenticated output
	// denominator declared by its signature.  It must not inherit every key
	// on the destination relation: doing so would let a later Merge choose an
	// unrelated alternate key and postpone the mismatch until Publish.
	for _, output := range outputs {
		if output.Relation != result.relation || !output.Denominator.Available() {
			continue
		}
		key := output.Denominator.Key()
		if !result.hasRowKey(key) {
			result.rowKeys = append(result.rowKeys, key)
		}
	}
	result.proposal = true
	// Apply consumes a range (if declared) and publishes semantic outcomes;
	// its output is a fresh relation and has no implicit range partition.
	result.rangeBound = false
	return result
}

// checkApplyCorrelation proves that a declared heterogeneous Apply has one
// exact query-site coordinate and one owner-issued lookup column per child.
// Complete inputs retain their own relation, denominator, and order key; this
// declaration only relates those already-authenticated ranges and never
// merges their authorities into a fabricated relation.
func (checker *checker) checkApplyCorrelation(correlation algebra.ApplyCorrelation, expressions []algebra.Expression, children []shape, signatureValue signature.Signature, slots []algebra.SlotSource, path string) {
	if !correlation.Specified() {
		return
	}
	if !correlation.Available() {
		checker.report.add(CodeCorrelationMismatch, path+".correlation", "Apply correlation declaration is unavailable")
		return
	}
	// Population is the independent closed Q authority. Existing key facts
	// prove totality and uniqueness only when the population key is exactly
	// the coordinate column; scope remains a cofiber and is never consulted as
	// a substitute key.
	population := correlation.Population()
	checker.checkDenominator(population, path+".correlation.population")
	populationRelation, populationRelationOK := checker.registry.Relation(population.Relation())
	populationKey, populationKeyOK := checker.registry.Key(population.Key())
	coordinate, coordinateOK := checker.registry.Column(correlation.Coordinate())
	if !coordinateOK || !coordinate.Available() {
		checker.report.add(CodeMissingReference, path+".correlation.coordinate", "Apply correlation coordinate is not registered")
	} else if coordinate.Type() != correlation.Type() {
		checker.report.add(CodeCorrelationMismatch, path+".correlation.type", "Apply correlation type does not match its coordinate column")
	}
	if coordinateOK && coordinate.Available() && populationRelationOK && populationRelation.Available() {
		if coordinate.Relation() != population.Relation() || !populationRelation.HasColumn(correlation.Coordinate()) {
			checker.report.add(CodeCorrelationMismatch, path+".correlation.population.coordinate", "population coordinate is not a member of its denominator relation")
		}
	}
	if populationKeyOK && populationKey.Available() {
		keyColumns := populationKey.Columns()
		if len(keyColumns) != 1 || keyColumns[0] != correlation.Coordinate() {
			checker.report.add(CodeCorrelationMismatch, path+".correlation.population.key", "population key does not uniquely and totally identify the coordinate")
		}
	}
	if correlation.ProjectionCount() != len(children) {
		checker.report.add(CodeCorrelationMismatch, path+".correlation.projections", "Apply correlation projection count differs from child count")
		return
	}
	if len(slots) != signatureValue.InputLen() {
		return
	}

	// The scalar-population form is a deliberately narrow extension of the
	// existing all-complete form.  Child zero is the exact population Input;
	// its one scalar slot is the owner-issued correlation coordinate.  Every
	// remaining child is still independently closed by Complete(Select(Input))
	// and receives its own projection/posting.  The child ordinal is not
	// inferred from relation names: it is fixed by this closed ABI and the
	// scalar source is redeemed through the authored SlotSource below.
	if scalarPopulationChild(expressions, children, signatureValue, slots, correlation) {
		checker.checkScalarPopulationCorrelation(correlation, expressions, children, signatureValue, slots, path)
		return
	}

	type rangeProof struct {
		relation    model.RelationID
		denominator model.DenominatorRef
		delivery    signature.Delivery
		set         bool
	}
	ranges := make([]rangeProof, len(children))
	for index, input := range signatureValue.Inputs() {
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
			checker.report.add(CodeCorrelationMismatch, fmt.Sprintf("%s.correlation.child[%d].range", path, childIndex), "correlated slots do not share one Complete range authority")
		}
	}
	for childIndex, child := range children {
		projection, projectionOK := correlation.ProjectionAt(childIndex)
		if !projectionOK || !ranges[childIndex].set {
			checker.report.add(CodeCorrelationMismatch, fmt.Sprintf("%s.correlation.child[%d]", path, childIndex), "correlated child lacks an exact Complete range")
			continue
		}
		if len(projection) == 0 {
			checker.checkSharedCompleteCorrelationChild(correlation, expressions[childIndex], child, signatureValue, slots, childIndex, path)
			continue
		}
		if len(projection) != 1 {
			checker.report.add(CodeCorrelationMismatch, fmt.Sprintf("%s.correlation.child[%d].columns", path, childIndex), "correlation projection must contain one coordinate column")
			continue
		}
		columnID := projection[0]
		column, columnOK := checker.registry.Column(columnID)
		if !columnOK || !column.Available() {
			checker.report.add(CodeMissingReference, fmt.Sprintf("%s.correlation.child[%d].columns[0]", path, childIndex), "correlation projection column is not registered")
			continue
		}
		cell, retained := child.column(columnID)
		if !retained || cell.Type != correlation.Type() || column.Relation() != ranges[childIndex].relation {
			checker.report.add(CodeCorrelationMismatch, fmt.Sprintf("%s.correlation.child[%d].columns[0]", path, childIndex), "correlation projection is not the typed child range coordinate")
		}
	}
}

// scalarPopulationChild reports whether the exact scalar-population ABI is
// selected.  It intentionally recognizes only a direct Input at child zero
// and an authored scalar slot for that child.  A malformed scalar attempt is
// still checked by checkScalarPopulationCorrelation; richer or reordered
// trees fall through to the historical all-complete proof and are refused by
// its exact range checks rather than receiving an inferred interpretation.
func scalarPopulationChild(expressions []algebra.Expression, children []shape, signatureValue signature.Signature, slots []algebra.SlotSource, correlation algebra.ApplyCorrelation) bool {
	if len(expressions) == 0 || len(children) == 0 || len(slots) != signatureValue.InputLen() || !correlation.Available() {
		return false
	}
	if !isDirectInput(expressions[0]) {
		return false
	}
	for index, source := range slots {
		if source.Child() == 0 {
			input, ok := signatureValue.InputAt(index)
			if ok && input.Delivery.IsScalar() {
				return true
			}
		}
	}
	return false
}

func isDirectInput(expression algebra.Expression) bool {
	switch value := expression.(type) {
	case algebra.Input:
		return value.Relation().Available()
	case *algebra.Input:
		return value != nil && value.Relation().Available()
	default:
		return false
	}
}

func (checker *checker) checkScalarPopulationCorrelation(correlation algebra.ApplyCorrelation, expressions []algebra.Expression, children []shape, signatureValue signature.Signature, slots []algebra.SlotSource, path string) {
	population := correlation.Population()
	coordinate := correlation.Coordinate()
	if len(children) < 2 {
		checker.report.add(CodeCorrelationMismatch, path+".correlation.children", "scalar population correlation requires at least one Complete span child")
	}
	if !isDirectInput(expressions[0]) || children[0].relation != population.Relation() {
		checker.report.add(CodeCorrelationMismatch, path+".correlation.population.child", "scalar population child must be a direct Input of the population relation")
	}

	scalarSlots := make([]int, 0, 1)
	for index, source := range slots {
		if source.Child() != 0 {
			continue
		}
		input, inputOK := signatureValue.InputAt(index)
		if !inputOK {
			continue
		}
		if input.Delivery.IsScalar() {
			scalarSlots = append(scalarSlots, index)
		} else {
			checker.report.add(CodeCorrelationMismatch, fmt.Sprintf("%s.correlation.population.slot[%d]", path, index), "population child slots must be scalar")
		}
	}
	if len(scalarSlots) != 1 {
		checker.report.add(CodeCorrelationMismatch, path+".correlation.population.source", "scalar population child requires exactly one scalar source")
	} else {
		index := scalarSlots[0]
		source := slots[index]
		input, _ := signatureValue.InputAt(index)
		cell, cellOK := children[0].cell(source.Cell())
		if !cellOK || cell.ID != coordinate || cell.Type != correlation.Type() || input.Relation != population.Relation() || input.Column != coordinate || input.Type != correlation.Type() || input.Denominator != population {
			checker.report.add(CodeCorrelationMismatch, fmt.Sprintf("%s.correlation.population.source", path), "scalar source is not the typed population coordinate")
		}
	}
	projection, projectionOK := correlation.ProjectionAt(0)
	if !projectionOK || len(projection) != 1 || projection[0] != coordinate {
		checker.report.add(CodeCorrelationMismatch, path+".correlation.population.projection", "population projection must be the declared coordinate")
	}

	// The span side of this mode is intentionally exact.  Do not admit Group,
	// Merge, Project, or a pre-joined child here: each span's posting witness
	// is keyed by the Complete denominator and must be independently
	// replayable as Complete(Select(Input)).
	for childIndex := 1; childIndex < len(children); childIndex++ {
		childPath := fmt.Sprintf("%s.correlation.child[%d]", path, childIndex)
		projection, projectionOK := correlation.ProjectionAt(childIndex)
		if !projectionOK {
			checker.report.add(CodeCorrelationMismatch, childPath+".columns", "span child projection is unavailable")
			continue
		}
		if len(projection) == 0 {
			checker.checkSharedCompleteCorrelationChild(correlation, expressions[childIndex], children[childIndex], signatureValue, slots, childIndex, path)
			continue
		}
		if !exactCompleteSelectInput(expressions[childIndex]) {
			checker.report.add(CodeCorrelationMismatch, childPath, "span child must be Complete(Select(Input))")
		}
		if len(projection) != 1 {
			checker.report.add(CodeCorrelationMismatch, childPath+".columns", "span child projection must contain one coordinate column")
			continue
		}
		columnID := projection[0]
		column, columnOK := checker.registry.Column(columnID)
		if !columnOK || !column.Available() {
			checker.report.add(CodeMissingReference, childPath+".columns[0]", "correlation projection column is not registered")
			continue
		}
		if column.Type() != correlation.Type() {
			checker.report.add(CodeCorrelationMismatch, childPath+".columns[0]", "correlation projection type differs from population coordinate")
		}
		retained, retainedOK := children[childIndex].column(columnID)
		if !retainedOK || retained.Type != correlation.Type() {
			checker.report.add(CodeCorrelationMismatch, childPath+".columns[0]", "correlation projection is not a typed span child cell")
		}
		spanSet := false
		for slotIndex, source := range slots {
			if int(source.Child()) != childIndex {
				continue
			}
			input, inputOK := signatureValue.InputAt(slotIndex)
			if !inputOK {
				continue
			}
			if !input.Delivery.IsComplete() {
				checker.report.add(CodeCorrelationMismatch, fmt.Sprintf("%s.slot[%d]", childPath, slotIndex), "span child inputs must use CompleteSpan delivery")
				continue
			}
			if !spanSet {
				spanSet = true
				if input.Relation != children[childIndex].relation || input.Denominator != children[childIndex].completeDenominator {
					checker.report.add(CodeCorrelationMismatch, childPath+".range", "span input does not name the child's exact Complete denominator")
				}
			} else {
				priorIndex := firstSlotForChild(slots, uint32(childIndex), slotIndex)
				prior, priorOK := signatureValue.InputAt(priorIndex)
				if !priorOK || !sameApplyGroupInput(prior, input) {
					checker.report.add(CodeCorrelationMismatch, childPath+".range", "span child slots do not share one Complete range authority")
				}
			}
		}
		if !spanSet {
			checker.report.add(CodeCorrelationMismatch, childPath, "span child has no Complete delivery slot")
		}
	}
}

// checkSharedCompleteCorrelationChild proves the narrow broadcast form.  An
// empty projection does not mean an omitted lookup key: it means this child
// is one exact global Complete(Select(Input)) vector, reused for every
// population row.  It therefore cannot carry the population coordinate or a
// scalar/bounded delivery that would need a population-row redemption.
//
// This deliberately retains the historical source==range relation rule.
// Joined source cells need a separate carrier/source witness ABI and must not
// enter the shared form by weakening this proof.
func (checker *checker) checkSharedCompleteCorrelationChild(correlation algebra.ApplyCorrelation, expression algebra.Expression, child shape, signatureValue signature.Signature, slots []algebra.SlotSource, childIndex int, path string) {
	childPath := fmt.Sprintf("%s.correlation.child[%d]", path, childIndex)
	if !exactCompleteSelectInput(expression) {
		checker.report.add(CodeCorrelationMismatch, childPath, "shared child must be Complete(Select(Input))")
	}
	if _, retainsCoordinate := child.column(correlation.Coordinate()); retainsCoordinate {
		checker.report.add(CodeCorrelationMismatch, childPath+".columns", "shared child retains the population coordinate")
	}
	if !child.completeDenominator.Available() {
		checker.report.add(CodeCorrelationMismatch, childPath+".range", "shared child lacks an exact Complete denominator")
	}

	spanSet := false
	var prior signature.Input
	priorSet := false
	for slotIndex, source := range slots {
		if int(source.Child()) != childIndex {
			continue
		}
		input, inputOK := signatureValue.InputAt(slotIndex)
		if !inputOK {
			continue
		}
		if !input.Delivery.IsComplete() {
			checker.report.add(CodeCorrelationMismatch, fmt.Sprintf("%s.slot[%d]", childPath, slotIndex), "shared child inputs must use CompleteSpan delivery")
			continue
		}
		spanSet = true
		if input.Column == correlation.Coordinate() {
			checker.report.add(CodeCorrelationMismatch, fmt.Sprintf("%s.slot[%d]", childPath, slotIndex), "shared child slot depends on the population coordinate")
		}
		if input.Relation != child.relation || input.Denominator != child.completeDenominator {
			checker.report.add(CodeCorrelationMismatch, childPath+".range", "shared input does not name the child's exact global Complete denominator")
		}
		if priorSet && !sameApplyGroupInput(prior, input) {
			checker.report.add(CodeCorrelationMismatch, childPath+".range", "shared child slots do not share one Complete range authority")
		}
		prior, priorSet = input, true
	}
	if !spanSet {
		checker.report.add(CodeCorrelationMismatch, childPath, "shared child has no Complete delivery slot")
	}
}

func exactCompleteSelectInput(expression algebra.Expression) bool {
	var complete algebra.Complete
	switch value := expression.(type) {
	case algebra.Complete:
		complete = value
	case *algebra.Complete:
		if value == nil {
			return false
		}
		complete = *value
	default:
		return false
	}
	var selectExpression algebra.Select
	switch value := complete.Child().(type) {
	case algebra.Select:
		selectExpression = value
	case *algebra.Select:
		if value == nil {
			return false
		}
		selectExpression = *value
	default:
		return false
	}
	return isDirectInput(selectExpression.Child())
}

func firstSlotForChild(slots []algebra.SlotSource, child uint32, before int) int {
	for index := 0; index < before; index++ {
		if slots[index].Child() == child {
			return index
		}
	}
	return before
}

// sameApplyGroupInput describes when several semantic slots can be redeemed
// from one selected child. Scalar slots read independent cells from the same
// tuple, so a sealed composite row may contribute columns owned by distinct
// relations and denominators. Span slots consume one physical range and must
// retain one exact range contract; allowing them to mix would make one batch
// stand for multiple authorities.
func sameApplyGroupInput(left, right signature.Input) bool {
	if left.Delivery.IsScalar() && right.Delivery.IsScalar() {
		return true
	}
	return left.Relation == right.Relation && left.Denominator == right.Denominator && left.Delivery == right.Delivery
}

func (checker *checker) checkAppliedDelivery(input signature.Input, child shape, path string) {
	if !input.Delivery.Available() {
		return
	}
	if input.Delivery.IsScalar() {
		return
	}
	if !child.relation.Available() {
		return
	}
	if input.Denominator.Relation() != child.relation {
		checker.report.add(CodeDenominatorMismatch, path, "Apply input denominator is not the child result relation")
	}
	if input.Delivery.IsScalar() {
		return
	}
	if !child.rangeBound {
		checker.report.add(CodeDeliveryMismatch, path, "Apply span delivery requires a Group or Complete range boundary")
	}
	if input.Delivery.IsComplete() {
		if !child.completeDenominator.Available() {
			checker.report.add(CodeDeliveryMismatch, path, "CompleteSpan Apply delivery requires a Complete child range")
		} else if input.Delivery.OrderKey() != child.completeDenominator.Key() {
			checker.report.add(CodeDeliveryMismatch, path, "CompleteSpan Apply delivery order differs from the Complete denominator key")
		}
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
	childExpression := value.Child()
	// A zero-input Apply is legal only as this direct child. Nested Select,
	// Merge, or standalone Apply expressions retain ordinary delivered-input
	// requirements, so a seed cannot masquerade as a relational source.
	previousSeedApply := checker.directSeedApply
	if apply, ok := childExpression.(algebra.Apply); ok && len(apply.Inputs()) == 0 {
		checker.directSeedApply = true
	} else {
		checker.directSeedApply = false
	}
	child := checker.child(childExpression, path+".child")
	checker.directSeedApply = previousSeedApply
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
	} else {
		// Publish redeems the destination row by its declared key. Every key
		// component therefore needs the owner semantic equality witness at
		// mount, just like Project/Group/Merge key operations.
		for index, columnID := range key.Columns() {
			column, columnOK := checker.registry.Column(columnID)
			if !columnOK || !column.Available() || column.Relation() != destination.ID() {
				continue
			}
			checker.report.equalityRequirements = append(checker.report.equalityRequirements,
				EqualityRequirement{
					Path:   fmt.Sprintf("%s.key[%d]", path, index),
					Column: columnID,
					Type:   column.Type(),
				},
			)
		}
	}
	if child.relation != destination.ID() {
		checker.report.add(CodeMembership, path, "Publish child relation differs from destination")
	}
	columns := publishColumns(contract, destination)
	if len(columns) == 0 {
		checker.report.add(CodeOperatorContract, path, "Publish has no writable destination columns")
	}
	if len(child.columns) != len(columns) {
		checker.report.add(CodeShapeMismatch, path, "Publish child does not provide the exact writable destination layout")
	}
	seen := make(map[model.ColumnID]struct{}, len(columns))
	for index, targetColumn := range columns {
		columnPath := fmt.Sprintf("%s.column[%d]", path, index)
		want, registered := checker.registry.Column(targetColumn)
		if !registered {
			checker.report.add(CodeMissingReference, columnPath, "Publish writable column is not registered")
			continue
		}
		if want.Relation() != destination.ID() || !destination.HasColumn(targetColumn) {
			checker.report.add(CodeMembership, columnPath, "Publish writable column is not owned by destination")
			continue
		}
		if _, duplicate := seen[targetColumn]; duplicate {
			checker.report.add(CodeDuplicateMember, columnPath, "Publish repeats a writable destination column")
			continue
		}
		seen[targetColumn] = struct{}{}
		got, present := child.cell(uint32(index))
		if !present {
			continue
		}
		if got.ID != targetColumn {
			checker.report.add(CodeShapeMismatch, columnPath, "Publish child cell order does not match writable destination layout")
			continue
		}
		rowRelation, rowOK := child.source(got.Source)
		if !rowOK || rowRelation != destination.ID() {
			checker.report.add(CodeMembership, columnPath, "Publish child cell does not retain destination row authority")
		}
		if want.Type() != got.Type {
			checker.report.add(CodeTypeMismatch, columnPath, "Publish child column type differs from destination")
		}
	}
	checker.recordPresentRequirements(childExpression, destination.ID(), columns, path)
	return destinationShape(destination, checker.registry)
}

// publishColumns resolves the historic full-row form once at typing.
// Resolved programs always provide their exact semantic writable subset;
// runtime only redeems this sealed vector and never searches a relation by
// nominal column name.
func publishColumns(contract algebra.PublishContract, destination model.RelationSchema) []model.ColumnID {
	columns := contract.Columns()
	if len(columns) != 0 {
		return columns
	}
	return destination.Columns()
}

// recordPresentRequirements follows only operators that preserve exact output
// cell positions. It records a requirement only when a checked Apply output
// is both Present-capable and one of this Publish node's sealed writable
// columns. A signature declaration on its own, an unselected output, or a
// raw carried relation never creates a lattice obligation.
func (checker *checker) recordPresentRequirements(expression algebra.Expression, destination model.RelationID, columns []model.ColumnID, path string) {
	if expression == nil || len(columns) == 0 {
		return
	}
	wanted := make(map[model.ColumnID]int, len(columns))
	for index, column := range columns {
		wanted[column] = index
	}
	seen := make(map[model.ColumnID]struct{}, len(columns))
	var walk func(algebra.Expression, map[model.ColumnID]int)
	walk = func(node algebra.Expression, active map[model.ColumnID]int) {
		if node == nil || len(active) == 0 {
			return
		}
		switch value := node.(type) {
		case algebra.Apply:
			signatureValue, ok := checker.registry.Signature(value.Contract().Operation())
			if !ok || !signatureValue.Available() {
				return
			}
			present, presentOK := model.NewPresence(model.Present)
			if !presentOK {
				return
			}
			for _, output := range signatureValue.Outputs() {
				index, wantedOutput := active[output.Column]
				if !wantedOutput || output.Relation != destination || !output.Available() || !output.Presence.Allows(present) {
					continue
				}
				if _, duplicate := seen[output.Column]; duplicate {
					continue
				}
				seen[output.Column] = struct{}{}
				checker.report.presentRequirements = append(checker.report.presentRequirements, PresentRequirement{
					Path:   fmt.Sprintf("%s.column[%d]", path, index),
					Column: output.Column,
					Type:   output.Type,
				})
			}
		case algebra.Select:
			walk(value.Child(), active)
		case algebra.Group:
			walk(value.Child(), active)
		case algebra.Complete:
			walk(value.Child(), active)
		case algebra.Expand:
			walk(value.Child(), active)
		case algebra.ColumnProject:
			selected := make(map[model.ColumnID]int)
			for _, slot := range value.Contract().Slots() {
				if index, ok := active[slot.Column()]; ok {
					selected[slot.Column()] = index
				}
			}
			walk(value.Child(), selected)
		case algebra.Merge:
			for _, child := range value.Inputs() {
				walk(child, active)
			}
		}
	}
	walk(expression, wanted)
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
	result := shape{relation: relation.ID(), sources: []model.RelationID{relation.ID()}}
	for index, id := range relation.Columns() {
		column, ok := checker.registry.Column(id)
		if !ok {
			checker.report.add(CodeMissingReference, fmt.Sprintf("%s.column[%d]", path, index), "relation shape column is not registered")
			continue
		}
		result.columns = append(result.columns, columnType{ID: id, Type: column.Type(), Source: 0})
	}
	result.keys = checker.keysForColumns(relation, result.columns)
	result.rowKeys = append([]model.KeyID(nil), result.keys...)
	return result
}

// keysForColumns keeps key metadata honest for projected rows.  A relation
// key is available downstream only when every one of its ordered columns is
// present in the current positional shape; retaining all nominal relation
// keys on a narrow Input would let Group/Complete/Apply claim a denominator
// whose cells were never delivered.
func (checker *checker) keysForColumns(relation model.RelationSchema, columns []columnType) []model.KeyID {
	if !relation.Available() {
		return nil
	}
	selected := make(map[model.ColumnID]struct{}, len(columns))
	for _, column := range columns {
		selected[column.ID] = struct{}{}
	}
	keys := make([]model.KeyID, 0, len(relation.Keys()))
	for _, keyID := range relation.Keys() {
		key, ok := checker.registry.Key(keyID)
		if !ok || !key.Available() || key.Relation() != relation.ID() {
			continue
		}
		keyColumns := key.Columns()
		if len(keyColumns) == 0 {
			continue
		}
		complete := true
		for _, columnID := range keyColumns {
			if _, present := selected[columnID]; !present {
				complete = false
				break
			}
		}
		if complete {
			keys = append(keys, keyID)
		}
	}
	return keys
}

func destinationShape(relation model.RelationSchema, registry *checkregistry.View) shape {
	result := shape{relation: relation.ID(), keys: relation.Keys(), sources: []model.RelationID{relation.ID()}}
	for _, id := range relation.Columns() {
		if column, ok := registry.Column(id); ok {
			result.columns = append(result.columns, columnType{ID: id, Type: column.Type(), Source: 0})
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
		issues:               append(report.issues, Issue{Code: code, Path: path, Detail: detail}),
		requirements:         report.requirements,
		equalityRequirements: report.equalityRequirements,
		presentRequirements:  report.presentRequirements,
		algebraRequirements:  report.algebraRequirements,
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
	sort.SliceStable(report.equalityRequirements, func(left, right int) bool {
		if report.equalityRequirements[left].Path != report.equalityRequirements[right].Path {
			return report.equalityRequirements[left].Path < report.equalityRequirements[right].Path
		}
		return typeIDLess(report.equalityRequirements[left].Type, report.equalityRequirements[right].Type)
	})
	sort.SliceStable(report.presentRequirements, func(left, right int) bool {
		if report.presentRequirements[left].Path != report.presentRequirements[right].Path {
			return report.presentRequirements[left].Path < report.presentRequirements[right].Path
		}
		if report.presentRequirements[left].Column != report.presentRequirements[right].Column {
			return report.presentRequirements[left].Column.Content()[0] < report.presentRequirements[right].Column.Content()[0]
		}
		return typeIDLess(report.presentRequirements[left].Type, report.presentRequirements[right].Type)
	})
}
