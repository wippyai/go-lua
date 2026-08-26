package relcompile

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// Compile lowers resolved declaration data into one immutable logical
// ExecutionSchema. It deliberately does not perform the independent
// certificate checks; malformed references are rejected here only when they
// cannot be represented as a closed expression, while cross-entry authority
// belongs to relation/check.
func Compile(declaration Declaration) (plan.ExecutionSchema, error) {
	if !declaration.SchemaID.Available() {
		return plan.ExecutionSchema{}, fmt.Errorf("relcompile: unavailable schema identity")
	}

	relations := make(map[model.RelationID]struct{}, len(declaration.Relations))
	relationColumns := make(map[model.RelationID][]model.ColumnID, len(declaration.Relations))
	for _, relation := range declaration.Relations {
		if !relation.Available() {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: unavailable relation schema")
		}
		if _, duplicate := relations[relation.ID()]; duplicate {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: duplicate relation")
		}
		relations[relation.ID()] = struct{}{}
		relationColumns[relation.ID()] = relation.Columns()
	}
	columns := make(map[model.ColumnID]struct{}, len(declaration.Columns))
	for _, column := range declaration.Columns {
		if !column.Available() {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: unavailable column schema")
		}
		if _, duplicate := columns[column.ID()]; duplicate {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: duplicate column")
		}
		columns[column.ID()] = struct{}{}
	}
	capabilities := make(map[model.TypeID]struct{}, len(declaration.TypeCapabilities))
	for _, capability := range declaration.TypeCapabilities {
		if !capability.Available() {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: unavailable type capability")
		}
		if _, duplicate := capabilities[capability.Type()]; duplicate {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: duplicate type capability")
		}
		capabilities[capability.Type()] = struct{}{}
	}
	keys := make(map[model.KeyID]model.KeySchema, len(declaration.Keys))
	for _, key := range declaration.Keys {
		if !key.Available() {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: unavailable key schema")
		}
		if _, duplicate := keys[key.ID()]; duplicate {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: duplicate key")
		}
		keys[key.ID()] = key
	}
	scopes := make(map[model.ScopeID]struct{}, len(declaration.Scopes))
	for _, scope := range declaration.Scopes {
		if !scope.Available() {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: unavailable scope schema")
		}
		if _, duplicate := scopes[scope.ID()]; duplicate {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: duplicate scope")
		}
		scopes[scope.ID()] = struct{}{}
	}
	signatures := make(map[signature.Identity]signature.Signature, len(declaration.Signatures))

	builder := plan.NewBuilder(declaration.SchemaID)
	for _, relation := range declaration.Relations {
		if !builder.AddRelation(relation) {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: add relation")
		}
	}
	for _, column := range declaration.Columns {
		if !builder.AddColumn(column) {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: add column")
		}
	}
	for _, capability := range declaration.TypeCapabilities {
		if !builder.AddTypeCapability(capability) {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: add type capability")
		}
	}
	for _, key := range declaration.Keys {
		if !builder.AddKey(key) {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: add key")
		}
	}
	for _, scope := range declaration.Scopes {
		if !builder.AddScope(scope) {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: add scope")
		}
	}
	for _, semantic := range declaration.Signatures {
		if !semantic.Available() {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: unavailable semantic signature")
		}
		if _, duplicate := signatures[semantic.Identity()]; duplicate {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: duplicate semantic signature")
		}
		signatures[semantic.Identity()] = semantic
		if !builder.AddSignature(semantic) {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: add semantic signature")
		}
	}
	initials := make(map[plan.Initial]struct{}, len(declaration.Initials))
	for _, initial := range declaration.Initials {
		if !initial.Available() {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: unavailable initial declaration")
		}
		if _, ok := signatures[initial.Operation()]; !ok {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: initial signature is not declared")
		}
		if _, duplicate := initials[initial]; duplicate {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: duplicate initial declaration")
		}
		initials[initial] = struct{}{}
		if !builder.AddInitial(initial) {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: add initial declaration")
		}
	}
	expressions := make(map[model.ExpressionID]struct{}, len(declaration.Rules))
	dependencies := make(map[model.DependencyID]struct{}, len(declaration.Rules))
	footprints := make([]footprint, 0, len(declaration.Rules))

	for _, rule := range declaration.Rules {
		expression, reads, writes, err := lowerRule(rule, relations, relationColumns, columns, keys, scopes, signatures)
		if err != nil {
			return plan.ExecutionSchema{}, err
		}
		if _, duplicate := expressions[rule.Expression]; duplicate {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: duplicate expression identity")
		}
		expressions[rule.Expression] = struct{}{}
		if _, duplicate := dependencies[rule.ID]; duplicate {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: duplicate dependency identity")
		}
		dependencies[rule.ID] = struct{}{}
		entry := plan.DefineExpressionRef(rule.Expression, expression)
		if !entry.Available() || !builder.AddExpression(entry) {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: invalid expression entry")
		}
		dependency := plan.DefineDependency(
			rule.ID,
			rule.Expression,
			relationRefs(reads),
			relationRefs(writes),
			"",
		)
		if !dependency.Available() || !builder.AddDependency(dependency) {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: invalid dependency")
		}
		footprints = append(footprints, footprint{id: rule.ID, reads: reads, writes: writes})
	}

	// Recurrence is a property of the footprints the rules already stated, so
	// the compiler derives the components rather than asking an author to
	// restate the graph beside the rules that form it.
	for _, component := range declareComponents(footprints) {
		if !component.Available() || !builder.AddSCC(component) {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: invalid strongly connected component")
		}
	}

	compiled, ok := builder.Build()
	if !ok {
		return plan.ExecutionSchema{}, fmt.Errorf("relcompile: build execution schema")
	}
	return compiled, nil
}

func lowerRule(rule Rule, relations map[model.RelationID]struct{}, relationColumns map[model.RelationID][]model.ColumnID, columns map[model.ColumnID]struct{}, keys map[model.KeyID]model.KeySchema, scopes map[model.ScopeID]struct{}, signatures map[signature.Identity]signature.Signature) (algebra.Expression, []model.RelationID, []model.RelationID, error) {
	if !rule.ID.Available() || !rule.Expression.Available() || !rule.Candidate.Available() {
		return nil, nil, nil, fmt.Errorf("relcompile: incomplete rule identity")
	}
	if !containsRelation(relations, rule.Candidate) {
		return nil, nil, nil, fmt.Errorf("relcompile: candidate relation is not declared")
	}
	if rule.Scope.Available() {
		if _, ok := scopes[rule.Scope]; !ok {
			return nil, nil, nil, fmt.Errorf("relcompile: scope is not declared")
		}
	}
	if rule.Complete != nil && !rule.Complete.Available() {
		return nil, nil, nil, fmt.Errorf("relcompile: malformed completion denominator")
	}
	if rule.Complete != nil {
		if !containsRelation(relations, rule.Complete.Relation()) {
			return nil, nil, nil, fmt.Errorf("relcompile: completion relation is not declared")
		}
		if _, ok := keys[rule.Complete.Key()]; !ok || rule.Complete.Key().Relation() != rule.Complete.Relation() {
			return nil, nil, nil, fmt.Errorf("relcompile: completion key is not declared by denominator relation")
		}
	}
	var (
		writableColumns         []model.ColumnID
		explicitWritableColumns bool
	)
	if rule.Publish != nil {
		if !containsRelation(relations, rule.Publish.Relation) {
			return nil, nil, nil, fmt.Errorf("relcompile: publication relation is not declared")
		}
		if _, ok := keys[rule.Publish.Key]; !ok || rule.Publish.Key.Relation() != rule.Publish.Relation {
			return nil, nil, nil, fmt.Errorf("relcompile: publication key is not declared by destination")
		}
		writableColumns = append([]model.ColumnID(nil), rule.Publish.Columns...)
		explicitWritableColumns = len(writableColumns) != 0
		if !explicitWritableColumns {
			writableColumns = append([]model.ColumnID(nil), relationColumns[rule.Publish.Relation]...)
		}
		if !validPublicationColumns(writableColumns, rule.Publish.Relation, columns) {
			return nil, nil, nil, fmt.Errorf("relcompile: publication writable columns are not one exact destination layout")
		}
	}

	var (
		expression algebra.Expression
		layout     tupleLayout
		reads      []model.RelationID
	)
	var carryCandidateColumns []model.ColumnID
	if rule.Carry != nil && rule.Carry.Relation == rule.Candidate {
		if rule.Carry.Transform != nil {
			carrySemantic, carryOK := signatures[*rule.Carry.Transform]
			if carryOK {
				for index := 0; index < carrySemantic.InputLen(); index++ {
					input, inputOK := carrySemantic.InputAt(index)
					if inputOK && input.Relation == rule.Carry.Relation {
						carryCandidateColumns = append(carryCandidateColumns, input.Column)
					}
				}
			}
		} else {
			carryCandidateColumns = append(carryCandidateColumns, rule.Carry.Columns...)
		}
	}
	hasApply := rule.ApplyShape != nil || rule.Apply.Operation.Available() || rule.Apply.Version != 0
	var semantic signature.Signature
	if hasApply {
		if !rule.Apply.Available() {
			return nil, nil, nil, fmt.Errorf("relcompile: malformed semantic operation identity")
		}
		var ok bool
		semantic, ok = signatures[rule.Apply]
		if !ok {
			return nil, nil, nil, fmt.Errorf("relcompile: semantic operation is not declared")
		}
	}
	var fullLayout tupleLayout
	if rule.ApplyShape == nil {
		projections, full, projectionErr := ordinaryInputProjections(
			rule.Candidate, rule.Joins, rule.Complete, rule.ApplySlots, semantic, hasApply, rule.Output,
			relationColumns, carryCandidateColumns,
		)
		if projectionErr != nil {
			return nil, nil, nil, projectionErr
		}
		fullLayout = full
		base, err := lowerExpression(expressionSpec{
			Candidate: rule.Candidate,
			Joins:     rule.Joins,
			Scope:     rule.Scope,
			Complete:  rule.Complete,
		}, relations, relationColumns, columns, keys, scopes, projections)
		if err != nil {
			return nil, nil, nil, err
		}
		expression = base.expression
		layout = base.layout
		reads = base.reads
	} else {
		// A multi-child shape owns all terminal child geometry. Rule.Candidate
		// remains the first child identity so the dependency still has one
		// canonical candidate without compiling an unused base expression.
		if len(rule.Joins) != 0 || rule.Scope.Available() || rule.Complete != nil {
			return nil, nil, nil, fmt.Errorf("relcompile: multi-child Apply owns child geometry; rule-level joins/scope/completion are not allowed")
		}
		if len(rule.ApplyShape.Children) == 0 || rule.ApplyShape.Children[0].Candidate != rule.Candidate {
			return nil, nil, nil, fmt.Errorf("relcompile: multi-child Apply first child must be the rule candidate")
		}
	}
	if hasApply {
		var slotSource []algebra.SlotSource
		var sourceErr error
		if rule.ApplyShape != nil {
			if !rule.ApplyShape.Correlation.Available() {
				return nil, nil, nil, fmt.Errorf("relcompile: multi-child Apply has no sealed correlation declaration")
			}
			projections, fullLayouts, projectionErr := applyShapeInputProjections(*rule.ApplyShape, semantic, relationColumns)
			if projectionErr != nil {
				return nil, nil, nil, projectionErr
			}
			children, layouts, childReads, lowerErr := lowerApplyChildren(*rule.ApplyShape, relations, relationColumns, columns, keys, scopes, projections)
			if lowerErr != nil {
				return nil, nil, nil, lowerErr
			}
			slotSource, sourceErr = remapSlotSources(rule.ApplyShape.Slots, fullLayouts, layouts)
			if sourceErr != nil {
				return nil, nil, nil, sourceErr
			}
			slotSource, sourceErr = explicitApplySlotSources(slotSource, semantic, layouts)
			if sourceErr != nil {
				return nil, nil, nil, sourceErr
			}
			if !rule.ApplyShape.Output.Available() {
				return nil, nil, nil, fmt.Errorf("relcompile: multi-child Apply has no sealed output geometry")
			}
			output, outputOK := remapOutputAddressAcrossChildren(rule.ApplyShape.Output, fullLayouts, layouts)
			if !outputOK {
				return nil, nil, nil, fmt.Errorf("relcompile: multi-child Apply output geometry does not address a retained child cell")
			}
			contract, contractOK := algebra.NewCorrelatedApplyContract(rule.Apply, slotSource, rule.ApplyShape.Correlation, output)
			if !contractOK {
				return nil, nil, nil, fmt.Errorf("relcompile: multi-child Apply output geometry is unavailable after projection")
			}
			expression = algebra.NewApply(children, contract)
			for _, childRead := range childReads {
				reads = appendUniqueRelation(reads, childRead)
			}
		} else {
			slotSource, sourceErr = applySlotSources(rule.ApplySlots, semantic, layout, len(rule.Joins))
			if sourceErr != nil {
				return nil, nil, nil, sourceErr
			}
			if !rule.Output.Available() {
				return nil, nil, nil, fmt.Errorf("relcompile: Apply has no sealed output geometry")
			}
			output, outputOK := remapOutputAddress(rule.Output, fullLayout, layout)
			if !outputOK {
				return nil, nil, nil, fmt.Errorf("relcompile: Apply output geometry does not address a retained child cell")
			}
			expression = algebra.NewApply([]algebra.Expression{expression}, algebra.NewApplyContract(rule.Apply, slotSource, output))
		}
		var outputOK bool
		layout, outputOK = applyOutputLayout(semantic)
		if !outputOK {
			return nil, nil, nil, fmt.Errorf("relcompile: semantic operation does not provide one closed output layout")
		}
	}
	if rule.Publish != nil && explicitWritableColumns {
		var projectErr error
		expression, layout, projectErr = projectColumns(expression, layout, writableColumns)
		if projectErr != nil {
			return nil, nil, nil, projectErr
		}
	}
	if rule.Carry != nil {
		if rule.Publish == nil {
			return nil, nil, nil, fmt.Errorf("relcompile: a carried derivation has no publication to merge under")
		}
		if !containsRelation(relations, rule.Carry.Relation) {
			return nil, nil, nil, fmt.Errorf("relcompile: carried relation is not declared")
		}
		carryFullLayout, carryFullLayoutOK := inputLayout(rule.Carry.Relation, relationColumns)
		if !carryFullLayoutOK {
			return nil, nil, nil, fmt.Errorf("relcompile: carry tuple layout is not declared")
		}
		carryRequirements := newInputRequirements(rule.Carry.Relation, nil)
		if rule.Carry.Transform != nil {
			semantic := signatures[*rule.Carry.Transform]
			for index := 0; index < semantic.InputLen(); index++ {
				input, inputOK := semantic.InputAt(index)
				if !inputOK {
					return nil, nil, nil, fmt.Errorf("relcompile: carry transform input %d is unavailable", index)
				}
				if err := carryRequirements.add(CandidateOccurrence(), rule.Carry.Relation, nil, input.Column); err != nil {
					return nil, nil, nil, fmt.Errorf("relcompile: carry transform input %d: %w", index, err)
				}
			}
		} else {
			carryColumns := rule.Carry.Columns
			if len(carryColumns) == 0 {
				// An identity carry with no writable subset publishes the
				// complete carried row. Its exact source therefore follows the
				// authored relation layout as the publication contract requires;
				// this is an explicit whole-row carry declaration, not a
				// consumer-requirement fallback for ordinary reads.
				carryColumns = relationColumns[rule.Carry.Relation]
			}
			for _, column := range carryColumns {
				if err := carryRequirements.add(CandidateOccurrence(), rule.Carry.Relation, nil, column); err != nil {
					return nil, nil, nil, fmt.Errorf("relcompile: carried column: %w", err)
				}
			}
		}
		// A carried destination row is keyed by the publication authority even
		// when the semantic payload is only a writable subset.  Request those
		// owner-issued key columns from the source before lowering the exact
		// Input.  The later ColumnProject removes them from the semantic payload
		// while the typed checker retains the key proof on the projected shape;
		// no key cell is fabricated and no Merge rule is weakened.
		for _, column := range keys[rule.Publish.Key].Columns() {
			if err := carryRequirements.add(CandidateOccurrence(), rule.Carry.Relation, nil, column); err != nil {
				return nil, nil, nil, fmt.Errorf("relcompile: carried publication key: %w", err)
			}
		}
		if source, sourceOK := rule.Carry.Output.Source(); sourceOK {
			if err := addSlotRequirement(carryRequirements, source, rule.Carry.Relation, nil, carryFullLayout); err != nil {
				return nil, nil, nil, fmt.Errorf("relcompile: carry output geometry: %w", err)
			}
		}
		carryProjections, projectionErr := carryRequirements.ordered(rule.Carry.Relation, nil, relationColumns)
		if projectionErr != nil {
			return nil, nil, nil, projectionErr
		}
		carryInputColumns, carryInputOK := carryProjections[CandidateOccurrence()]
		if !carryInputOK {
			return nil, nil, nil, fmt.Errorf("relcompile: carry input projection is unavailable")
		}
		carryInput, carryInputOK := exactInput(rule.Carry.Relation, carryInputColumns)
		if !carryInputOK {
			return nil, nil, nil, fmt.Errorf("relcompile: carry input projection is not exact")
		}
		carried := algebra.Expression(carryInput)
		carryLayout, carryLayoutOK := projectedInputLayout(rule.Carry.Relation, carryInputColumns, relationColumns)
		if !carryLayoutOK {
			return nil, nil, nil, fmt.Errorf("relcompile: carry tuple layout is not declared")
		}
		if rule.Carry.Scope.Available() {
			if _, ok := scopes[rule.Carry.Scope]; !ok {
				return nil, nil, nil, fmt.Errorf("relcompile: carried scope is not declared")
			}
			carried = algebra.NewSelect(carried, algebra.NewSelectContract(algebra.SelectByScope, rule.Carry.Scope))
		}
		if rule.Carry.Transform != nil {
			if _, ok := signatures[*rule.Carry.Transform]; !ok {
				return nil, nil, nil, fmt.Errorf("relcompile: carry transform operation is not declared")
			}
			semantic := signatures[*rule.Carry.Transform]
			if !rule.Carry.Output.Available() {
				return nil, nil, nil, fmt.Errorf("relcompile: carry transform has no sealed output geometry")
			}
			output, outputOK := remapOutputAddress(rule.Carry.Output, carryFullLayout, carryLayout)
			if !outputOK {
				return nil, nil, nil, fmt.Errorf("relcompile: carry transform output geometry does not address a retained child cell")
			}
			slotSource, sourceErr := applySlotSources(repeatCandidateOccurrences(semantic.InputLen()), semantic, carryLayout, 0)
			if sourceErr != nil {
				return nil, nil, nil, sourceErr
			}
			if !uniqueCarryOutputSource(output, slotSource) {
				return nil, nil, nil, fmt.Errorf("relcompile: carry transform output geometry is absent or ambiguous in its authored input slots")
			}
			carried = algebra.NewApply([]algebra.Expression{carried}, algebra.NewApplyContract(*rule.Carry.Transform, slotSource, output))
			carryLayout, outputOK = applyOutputLayout(semantic)
			if !outputOK {
				return nil, nil, nil, fmt.Errorf("relcompile: carry transform does not provide one closed output layout")
			}
		}
		carryColumns := append([]model.ColumnID(nil), rule.Carry.Columns...)
		if len(carryColumns) != 0 {
			if !validPublicationColumns(carryColumns, rule.Publish.Relation, columns) {
				return nil, nil, nil, fmt.Errorf("relcompile: carried writable columns are not one exact destination layout")
			}
			if !sameColumnLayout(carryColumns, writableColumns) {
				return nil, nil, nil, fmt.Errorf("relcompile: carried writable columns differ from publication layout")
			}
			var projectErr error
			carried, carryLayout, projectErr = projectColumns(carried, carryLayout, carryColumns)
			if projectErr != nil {
				return nil, nil, nil, projectErr
			}
		}
		if explicitWritableColumns && !sameTupleLayout(layout, carryLayout) {
			return nil, nil, nil, fmt.Errorf("relcompile: carried derivation does not have publication typed row shape")
		}
		expression = algebra.NewMerge([]algebra.Expression{expression, carried}, algebra.NewMergeContract(rule.Publish.Key))
		reads = appendUniqueRelation(reads, rule.Carry.Relation)
	}
	if rule.Publish != nil {
		if explicitWritableColumns {
			expression = algebra.NewPublish(expression, algebra.NewPublishContract(rule.Publish.Relation, rule.Publish.Key, writableColumns...))
		} else {
			expression = algebra.NewPublish(expression, algebra.NewPublishContract(rule.Publish.Relation, rule.Publish.Key))
		}
		writes := []model.RelationID{rule.Publish.Relation}
		return expression, reads, writes, nil
	}
	return expression, reads, nil, nil
}

type expressionSpec struct {
	Candidate model.RelationID
	Joins     []JoinSpec
	Scope     model.ScopeID
	Complete  *model.DenominatorRef
}

type loweredExpression struct {
	expression algebra.Expression
	layout     tupleLayout
	reads      []model.RelationID
}

// inputRequirements is the cold, occurrence-local source projection under
// construction.  A map keyed only by RelationID would collapse two reads of
// one relation and make their Apply slots indistinguishable; ReadOccurrence is
// deliberately the key here.
type inputRequirements map[ReadOccurrence]map[model.ColumnID]struct{}

func newInputRequirements(candidate model.RelationID, joins []JoinSpec) inputRequirements {
	result := make(inputRequirements, len(joins)+1)
	result[CandidateOccurrence()] = make(map[model.ColumnID]struct{})
	for index := range joins {
		result[JoinOccurrence(uint32(index))] = make(map[model.ColumnID]struct{})
	}
	return result
}

func fullExpressionLayout(candidate model.RelationID, joins []JoinSpec, relationColumns map[model.RelationID][]model.ColumnID) (tupleLayout, bool) {
	result, ok := inputLayout(candidate, relationColumns)
	if !ok {
		return tupleLayout{}, false
	}
	for _, join := range joins {
		right, rightOK := inputLayout(join.Relation, relationColumns)
		if !rightOK {
			return tupleLayout{}, false
		}
		result = joinLayout(result, right)
	}
	return result, true
}

func occurrenceRelation(occurrence ReadOccurrence, candidate model.RelationID, joins []JoinSpec) (model.RelationID, bool) {
	if occurrence.Candidate() {
		return candidate, true
	}
	join, ok := occurrence.Join()
	if !ok || int(join) >= len(joins) {
		return model.RelationID{}, false
	}
	return joins[join].Relation, true
}

func (requirements inputRequirements) add(occurrence ReadOccurrence, candidate model.RelationID, joins []JoinSpec, column model.ColumnID) error {
	relation, ok := occurrenceRelation(occurrence, candidate, joins)
	if !ok || !column.Available() || column.Relation() != relation {
		return fmt.Errorf("relcompile: required input column does not belong to its read occurrence")
	}
	set, ok := requirements[occurrence]
	if !ok {
		return fmt.Errorf("relcompile: required input occurrence is unavailable")
	}
	set[column] = struct{}{}
	return nil
}

// sourceOccurrenceForColumn resolves an already-authored tuple cell to the
// occurrence that owns it.  Candidate-owned columns are intentionally kept on
// the candidate even when a later self-join has the same RelationID; all other
// repeated relations use the most recently introduced matching source, which
// is the left-deep occurrence available to the next join.
func sourceOccurrenceForColumn(full tupleLayout, candidate model.RelationID, joins []JoinSpec, column model.ColumnID, beforeSource int) (ReadOccurrence, bool) {
	if column.Relation() == candidate && len(full.sources) != 0 && full.sources[0] == candidate {
		for _, cell := range full.cells {
			if cell.source == 0 && cell.column == column {
				return CandidateOccurrence(), true
			}
		}
	}
	if beforeSource >= len(full.sources) {
		beforeSource = len(full.sources) - 1
	}
	for source := beforeSource; source >= 0; source-- {
		if full.sources[source] != column.Relation() {
			continue
		}
		for _, cell := range full.cells {
			if int(cell.source) == source && cell.column == column {
				if source == 0 {
					return CandidateOccurrence(), true
				}
				return JoinOccurrence(uint32(source - 1)), true
			}
		}
	}
	return ReadOccurrence{}, false
}

func addSlotRequirement(requirements inputRequirements, slot algebra.SlotSource, candidate model.RelationID, joins []JoinSpec, full tupleLayout) error {
	if slot.Child() != 0 || int(slot.Cell()) >= len(full.cells) {
		return fmt.Errorf("relcompile: input/output slot is outside its sealed tuple layout")
	}
	cell := full.cells[slot.Cell()]
	if int(cell.source) >= len(full.sources) {
		return fmt.Errorf("relcompile: input/output slot source is outside its sealed tuple layout")
	}
	occurrence := CandidateOccurrence()
	if cell.source != 0 {
		occurrence = JoinOccurrence(cell.source - 1)
	}
	return requirements.add(occurrence, candidate, joins, cell.column)
}

func addSemanticRequirements(requirements inputRequirements, candidate model.RelationID, joins []JoinSpec, occurrences []ReadOccurrence, semantic signature.Signature, full tupleLayout) error {
	if len(occurrences) != semantic.InputLen() {
		return fmt.Errorf("relcompile: semantic input count %d does not match declared apply occurrences %d", semantic.InputLen(), len(occurrences))
	}
	for index, occurrence := range occurrences {
		if !occurrence.available(len(joins)) {
			return fmt.Errorf("relcompile: apply slot %d names an unavailable read occurrence", index)
		}
		source, sourceOK := resolveOccurrenceSource(occurrence, candidate, full)
		input, inputOK := semantic.InputAt(index)
		if !sourceOK || !inputOK || int(source) >= len(full.sources) || full.sources[source] != input.Relation {
			return fmt.Errorf("relcompile: apply slot %d declared occurrence does not own the semantic input relation", index)
		}
		if err := requirements.add(occurrence, candidate, joins, input.Column); err != nil {
			return fmt.Errorf("relcompile: apply slot %d: %w", index, err)
		}
	}
	return nil
}

// addCompleteRequirements closes every compiler-emitted Complete child over
// the exact relation row its mounted CompleteBinding can materialize.  This
// is deliberately a projection requirement, not a post-lowering ordinal
// shift: the Input tuple and Complete output must share one canonical cell
// layout before Apply SlotSource is minted.
func addCompleteRequirements(requirements inputRequirements, candidate model.RelationID, joins []JoinSpec, complete *model.DenominatorRef, relationColumns map[model.RelationID][]model.ColumnID) error {
	add := func(occurrence ReadOccurrence, relation model.RelationID, denominator *model.DenominatorRef) error {
		if denominator == nil {
			return nil
		}
		// Compile deliberately preserves a structurally representable but
		// semantically invalid denominator mismatch for the independent
		// certificate checker to report.  There is no physical Complete output
		// law to pre-close in that case, so leave its authored projection alone.
		if !denominator.Available() || denominator.Relation() != relation {
			return nil
		}
		columns, columnsOK := relationColumns[relation]
		if !columnsOK || len(columns) == 0 {
			return fmt.Errorf("relcompile: Complete relation has no declared row layout")
		}
		for _, column := range columns {
			if err := requirements.add(occurrence, candidate, joins, column); err != nil {
				return fmt.Errorf("relcompile: Complete row column: %w", err)
			}
		}
		return nil
	}
	if err := add(CandidateOccurrence(), candidate, complete); err != nil {
		return err
	}
	for index := range joins {
		if err := add(JoinOccurrence(uint32(index)), joins[index].Relation, joins[index].Complete); err != nil {
			return fmt.Errorf("relcompile: join %d: %w", index, err)
		}
	}
	return nil
}

func addJoinRequirements(requirements inputRequirements, candidate model.RelationID, joins []JoinSpec, full tupleLayout) error {
	for index, join := range joins {
		occurrence := JoinOccurrence(uint32(index))
		for _, column := range join.RightColumns {
			if err := requirements.add(occurrence, candidate, joins, column); err != nil {
				return fmt.Errorf("relcompile: join %d: %w", index, err)
			}
		}
		// Expand's reader key is the exact right-side cell consumed by the
		// dependent lookup.  It is not a scope or completion column and must
		// therefore be retained without widening the reader row.
		if join.Expand != nil {
			if err := requirements.add(occurrence, candidate, joins, join.Expand.Key()); err != nil {
				return fmt.Errorf("relcompile: expand %d: %w", index, err)
			}
			// Expand emits the complete candidate row alongside the complete
			// reader row.  Retain every authored candidate cell before the
			// occurrence-local Input is sealed; keeping only a structural witness
			// would make the emitted tuple lose candidate payload columns while
			// leaving the reader projection apparently complete.  This is still
			// an exact owner projection: only source-0 cells from the already
			// authored expression layout are admitted, and no runtime widening is
			// needed to redeem the Expand result.
			for _, cell := range full.cells {
				if cell.source != 0 {
					continue
				}
				if err := requirements.add(CandidateOccurrence(), candidate, joins, cell.column); err != nil {
					return fmt.Errorf("relcompile: expand %d candidate: %w", index, err)
				}
			}
		}
		for _, column := range join.LeftColumns {
			before := index
			occurrence, occurrenceOK := sourceOccurrenceForColumn(full, candidate, joins, column, before)
			if !occurrenceOK {
				return fmt.Errorf("relcompile: join %d left column is not present in an earlier read occurrence", index)
			}
			if err := requirements.add(occurrence, candidate, joins, column); err != nil {
				return fmt.Errorf("relcompile: join %d: %w", index, err)
			}
		}
	}
	return nil
}

func (requirements inputRequirements) ordered(candidate model.RelationID, joins []JoinSpec, relationColumns map[model.RelationID][]model.ColumnID) (map[ReadOccurrence][]model.ColumnID, error) {
	result := make(map[ReadOccurrence][]model.ColumnID, len(joins)+1)
	occurrences := make([]ReadOccurrence, 0, len(joins)+1)
	occurrences = append(occurrences, CandidateOccurrence())
	for index := range joins {
		occurrences = append(occurrences, JoinOccurrence(uint32(index)))
	}
	for _, occurrence := range occurrences {
		relation, relationOK := occurrenceRelation(occurrence, candidate, joins)
		if !relationOK {
			return nil, fmt.Errorf("relcompile: required input occurrence has no relation")
		}
		declared, declaredOK := relationColumns[relation]
		if !declaredOK || len(declared) == 0 {
			return nil, fmt.Errorf("relcompile: required input relation has no authored columns")
		}
		set := requirements[occurrence]
		ordered := make([]model.ColumnID, 0, len(set))
		for _, column := range declared {
			if _, needed := set[column]; needed {
				ordered = append(ordered, column)
			}
		}
		if len(ordered) == 0 || len(ordered) != len(set) {
			return nil, fmt.Errorf("relcompile: required input columns are not one authored relation projection")
		}
		result[occurrence] = ordered
	}
	return result, nil
}

func ordinaryInputProjections(candidate model.RelationID, joins []JoinSpec, complete *model.DenominatorRef, occurrences []ReadOccurrence, semantic signature.Signature, hasApply bool, output algebra.OutputAddress, relationColumns map[model.RelationID][]model.ColumnID, seedCandidateColumns []model.ColumnID) (map[ReadOccurrence][]model.ColumnID, tupleLayout, error) {
	full, fullOK := fullExpressionLayout(candidate, joins, relationColumns)
	if !fullOK {
		return nil, tupleLayout{}, fmt.Errorf("relcompile: rule input tuple layout is not declared")
	}
	requirements := newInputRequirements(candidate, joins)
	for _, column := range seedCandidateColumns {
		if err := requirements.add(CandidateOccurrence(), candidate, joins, column); err != nil {
			return nil, tupleLayout{}, fmt.Errorf("relcompile: candidate seed: %w", err)
		}
	}
	// Complete owns a closed denominator row, not a sparse projection plus a
	// runtime ordinal adjustment.  Retain its whole relation vector before
	// lowering so the emitted Input and CompleteBinding already agree on the
	// one physical output cell layout.  CompleteCellLayout then verifies that
	// it has no hidden second extension.
	if err := addCompleteRequirements(requirements, candidate, joins, complete, relationColumns); err != nil {
		return nil, tupleLayout{}, err
	}
	if err := addJoinRequirements(requirements, candidate, joins, full); err != nil {
		return nil, tupleLayout{}, err
	}
	if hasApply {
		if err := addSemanticRequirements(requirements, candidate, joins, occurrences, semantic, full); err != nil {
			return nil, tupleLayout{}, err
		}
	}
	if source, sourceOK := output.Source(); sourceOK {
		if err := addSlotRequirement(requirements, source, candidate, joins, full); err != nil {
			return nil, tupleLayout{}, fmt.Errorf("relcompile: output geometry: %w", err)
		}
	}
	// An ordinary root with no semantic/read consumer still needs one concrete
	// Input form. In particular, Compile intentionally preserves a
	// structurally representable but denominator-mismatched Complete for the
	// independent checker to diagnose; it must not turn that checker finding
	// into an earlier "zero exact projection" compiler failure. Retain the
	// candidate's declared row in this otherwise unconsumed case, matching the
	// historic explicit-all-columns lowering without inventing a second layout.
	if len(requirements[CandidateOccurrence()]) == 0 {
		declared, declaredOK := relationColumns[candidate]
		if !declaredOK || len(declared) == 0 {
			return nil, tupleLayout{}, fmt.Errorf("relcompile: unconsumed candidate has no authored row layout")
		}
		for _, column := range declared {
			if err := requirements.add(CandidateOccurrence(), candidate, joins, column); err != nil {
				return nil, tupleLayout{}, fmt.Errorf("relcompile: unconsumed candidate: %w", err)
			}
		}
	}
	projections, projectionErr := requirements.ordered(candidate, joins, relationColumns)
	if projectionErr != nil {
		return nil, tupleLayout{}, projectionErr
	}
	return projections, full, nil
}

func applyShapeInputProjections(shape ApplyShape, semantic signature.Signature, relationColumns map[model.RelationID][]model.ColumnID) ([]map[ReadOccurrence][]model.ColumnID, []tupleLayout, error) {
	if len(shape.Children) == 0 {
		return nil, nil, fmt.Errorf("relcompile: multi-child Apply has no children")
	}
	projections := make([]map[ReadOccurrence][]model.ColumnID, len(shape.Children))
	fullLayouts := make([]tupleLayout, len(shape.Children))
	requirements := make([]inputRequirements, len(shape.Children))
	for index, child := range shape.Children {
		full, fullOK := fullExpressionLayout(child.Candidate, child.Joins, relationColumns)
		if !fullOK {
			return nil, nil, fmt.Errorf("relcompile: Apply child %d tuple layout is not declared", index)
		}
		fullLayouts[index] = full
		requirements[index] = newInputRequirements(child.Candidate, child.Joins)
		if err := addCompleteRequirements(requirements[index], child.Candidate, child.Joins, child.Complete, relationColumns); err != nil {
			return nil, nil, fmt.Errorf("relcompile: Apply child %d: %w", index, err)
		}
		if err := addJoinRequirements(requirements[index], child.Candidate, child.Joins, full); err != nil {
			return nil, nil, fmt.Errorf("relcompile: Apply child %d: %w", index, err)
		}
	}
	for index, source := range shape.Slots {
		child := int(source.Child())
		if child < 0 || child >= len(shape.Children) {
			return nil, nil, fmt.Errorf("relcompile: Apply slot %d child %d is outside the sealed child set", index, source.Child())
		}
		value := shape.Children[child]
		local := algebra.NewSlotSource(0, source.Cell())
		if err := addSlotRequirement(requirements[child], local, value.Candidate, value.Joins, fullLayouts[child]); err != nil {
			return nil, nil, fmt.Errorf("relcompile: Apply slot %d: %w", index, err)
		}
	}
	for index, child := range shape.Children {
		projection, projectionOK := shape.Correlation.ProjectionAt(index)
		if !projectionOK {
			return nil, nil, fmt.Errorf("relcompile: Apply child %d correlation projection is unavailable", index)
		}
		for _, column := range projection {
			occurrence, occurrenceOK := sourceOccurrenceForColumn(fullLayouts[index], child.Candidate, child.Joins, column, len(child.Joins))
			if !occurrenceOK {
				return nil, nil, fmt.Errorf("relcompile: Apply child %d correlation column is not retained by a child occurrence", index)
			}
			if err := requirements[index].add(occurrence, child.Candidate, child.Joins, column); err != nil {
				return nil, nil, fmt.Errorf("relcompile: Apply child %d correlation: %w", index, err)
			}
		}
	}
	if source, sourceOK := shape.Output.Source(); sourceOK {
		child := int(source.Child())
		if child < 0 || child >= len(shape.Children) {
			return nil, nil, fmt.Errorf("relcompile: multi-child Apply output child is outside the sealed child set")
		}
		value := shape.Children[child]
		local := algebra.NewSlotSource(0, source.Cell())
		if err := addSlotRequirement(requirements[child], local, value.Candidate, value.Joins, fullLayouts[child]); err != nil {
			return nil, nil, fmt.Errorf("relcompile: multi-child Apply output: %w", err)
		}
	}
	for index, child := range shape.Children {
		projection, projectionErr := requirements[index].ordered(child.Candidate, child.Joins, relationColumns)
		if projectionErr != nil {
			return nil, nil, fmt.Errorf("relcompile: Apply child %d: %w", index, projectionErr)
		}
		projections[index] = projection
	}
	_ = semantic
	return projections, fullLayouts, nil
}

// lowerExpression is the shared cold lowering path for a rule child. Keeping
// this geometry in one function is important: a multi-child Apply is allowed
// to add independent sealed children, not a second relation compiler with a
// subtly different join/complete implementation.
func lowerExpression(spec expressionSpec, relations map[model.RelationID]struct{}, relationColumns map[model.RelationID][]model.ColumnID, columns map[model.ColumnID]struct{}, keys map[model.KeyID]model.KeySchema, scopes map[model.ScopeID]struct{}, projections map[ReadOccurrence][]model.ColumnID) (loweredExpression, error) {
	if !containsRelation(relations, spec.Candidate) {
		return loweredExpression{}, fmt.Errorf("relcompile: child candidate relation is not declared")
	}
	candidateColumns, candidateColumnsOK := projections[CandidateOccurrence()]
	if !candidateColumnsOK {
		return loweredExpression{}, fmt.Errorf("relcompile: candidate input projection is unavailable")
	}
	candidateInput, candidateInputOK := exactInput(spec.Candidate, candidateColumns)
	if !candidateInputOK {
		return loweredExpression{}, fmt.Errorf("relcompile: candidate input projection is not exact")
	}
	expression := algebra.Expression(candidateInput)
	layout, layoutOK := projectedInputLayout(spec.Candidate, candidateColumns, relationColumns)
	if !layoutOK {
		return loweredExpression{}, fmt.Errorf("relcompile: child candidate tuple layout is not declared")
	}
	reads := []model.RelationID{spec.Candidate}
	for index, join := range spec.Joins {
		if !containsRelation(relations, join.Relation) {
			return loweredExpression{}, fmt.Errorf("relcompile: join %d relation is not declared", index)
		}
		if join.Expand != nil {
			contract := *join.Expand
			if !contract.Available() || contract.Reader() != join.Relation || contract.Scope() != join.Scope {
				return loweredExpression{}, fmt.Errorf("relcompile: expand %d has an invalid reader/scope contract", index)
			}
			// Candidate and reader are the runtime sources of an Expand.  The
			// publisher is sealed owner evidence carried by the contract; it is
			// checked by the schema/authority layers and must not become a
			// compiled runtime relation dependency.
			if !containsRelation(relations, contract.Candidate()) {
				return loweredExpression{}, fmt.Errorf("relcompile: expand %d references an undeclared candidate relation", index)
			}
			if contract.Key().Relation() != join.Relation || !containsColumn(columns, contract.Key()) {
				return loweredExpression{}, fmt.Errorf("relcompile: expand %d key is not owned by reader relation", index)
			}
			readerLayout, readerOK := inputLayout(join.Relation, relationColumns)
			if !readerOK {
				return loweredExpression{}, fmt.Errorf("relcompile: expand %d reader tuple layout is not declared", index)
			}
			expression = algebra.NewExpand(expression, contract)
			layout = expandLayout(layout, readerLayout)
			reads = appendUniqueRelation(reads, contract.Candidate())
			reads = appendUniqueRelation(reads, contract.Reader())
			continue
		}
		if !validColumns(join.LeftColumns, columns) || !validColumns(join.RightColumns, columns) || len(join.LeftColumns) == 0 || len(join.LeftColumns) != len(join.RightColumns) {
			return loweredExpression{}, fmt.Errorf("relcompile: join %d has incompatible typed columns", index)
		}
		for _, column := range join.RightColumns {
			if column.Relation() != join.Relation {
				return loweredExpression{}, fmt.Errorf("relcompile: join %d right column is not owned by joined relation", index)
			}
		}
		joinedColumns, joinedColumnsOK := projections[JoinOccurrence(uint32(index))]
		if !joinedColumnsOK {
			return loweredExpression{}, fmt.Errorf("relcompile: join %d input projection is unavailable", index)
		}
		joinedInput, joinedInputOK := exactInput(join.Relation, joinedColumns)
		if !joinedInputOK {
			return loweredExpression{}, fmt.Errorf("relcompile: join %d input projection is not exact", index)
		}
		joined := algebra.Expression(joinedInput)
		joinedLayout, joinedLayoutOK := projectedInputLayout(join.Relation, joinedColumns, relationColumns)
		if !joinedLayoutOK {
			return loweredExpression{}, fmt.Errorf("relcompile: join %d tuple layout is not declared", index)
		}
		if join.Scope.Available() {
			if _, ok := scopes[join.Scope]; !ok {
				return loweredExpression{}, fmt.Errorf("relcompile: join %d scope is not declared", index)
			}
			joined = algebra.NewSelect(joined, algebra.NewSelectContract(algebra.SelectByScope, join.Scope))
		}
		if join.Complete != nil {
			if !join.Complete.Available() {
				return loweredExpression{}, fmt.Errorf("relcompile: join %d has a malformed completion denominator", index)
			}
			if !containsRelation(relations, join.Complete.Relation()) {
				return loweredExpression{}, fmt.Errorf("relcompile: join %d completion relation is not declared", index)
			}
			if _, ok := keys[join.Complete.Key()]; !ok {
				return loweredExpression{}, fmt.Errorf("relcompile: join %d completion key is not declared by denominator relation", index)
			}
			joined = algebra.NewComplete(joined, *join.Complete)
			if join.Complete.Relation() == join.Relation {
				joinedLayout, joinedLayoutOK = completeTupleLayout(joinedLayout, *join.Complete, relationColumns[join.Relation])
				if !joinedLayoutOK {
					return loweredExpression{}, fmt.Errorf("relcompile: join %d Complete output layout is not canonical", index)
				}
			}
			reads = appendUniqueRelation(reads, join.Complete.Relation())
		}
		contract := algebra.NewJoinContract(join.LeftColumns, join.RightColumns)
		expression = algebra.NewJoin(expression, joined, contract)
		layout = joinLayout(layout, joinedLayout)
		reads = appendUniqueRelation(reads, join.Relation)
	}
	if spec.Scope.Available() {
		if _, ok := scopes[spec.Scope]; !ok {
			return loweredExpression{}, fmt.Errorf("relcompile: child scope is not declared")
		}
		expression = algebra.NewSelect(expression, algebra.NewSelectContract(algebra.SelectByScope, spec.Scope))
	}
	if spec.Complete != nil {
		if !spec.Complete.Available() {
			return loweredExpression{}, fmt.Errorf("relcompile: child completion denominator is malformed")
		}
		if !containsRelation(relations, spec.Complete.Relation()) {
			return loweredExpression{}, fmt.Errorf("relcompile: child completion relation is not declared")
		}
		if _, ok := keys[spec.Complete.Key()]; !ok || spec.Complete.Key().Relation() != spec.Complete.Relation() {
			return loweredExpression{}, fmt.Errorf("relcompile: child completion key is not declared by denominator relation")
		}
		expression = algebra.NewComplete(expression, *spec.Complete)
		if spec.Complete.Relation() == spec.Candidate {
			layout, layoutOK = completeTupleLayout(layout, *spec.Complete, relationColumns[spec.Candidate])
			if !layoutOK {
				return loweredExpression{}, fmt.Errorf("relcompile: child Complete output layout is not canonical")
			}
		}
		reads = appendUniqueRelation(reads, spec.Complete.Relation())
	}
	return loweredExpression{expression: expression, layout: layout, reads: reads}, nil
}

func lowerApplyChildren(shape ApplyShape, relations map[model.RelationID]struct{}, relationColumns map[model.RelationID][]model.ColumnID, columns map[model.ColumnID]struct{}, keys map[model.KeyID]model.KeySchema, scopes map[model.ScopeID]struct{}, projections []map[ReadOccurrence][]model.ColumnID) ([]algebra.Expression, []tupleLayout, []model.RelationID, error) {
	if len(shape.Children) == 0 {
		return nil, nil, nil, fmt.Errorf("relcompile: multi-child Apply has no children")
	}
	if len(projections) != len(shape.Children) {
		return nil, nil, nil, fmt.Errorf("relcompile: multi-child Apply child projection count does not match child count")
	}
	children := make([]algebra.Expression, len(shape.Children))
	layouts := make([]tupleLayout, len(shape.Children))
	var reads []model.RelationID
	for index, child := range shape.Children {
		lowered, err := lowerExpression(expressionSpec{
			Candidate: child.Candidate,
			Joins:     child.Joins,
			Scope:     child.Scope,
			Complete:  child.Complete,
		}, relations, relationColumns, columns, keys, scopes, projections[index])
		if err != nil {
			return nil, nil, nil, fmt.Errorf("relcompile: Apply child %d: %w", index, err)
		}
		children[index] = lowered.expression
		layouts[index] = lowered.layout
		for _, read := range lowered.reads {
			reads = appendUniqueRelation(reads, read)
		}
	}
	return children, layouts, reads, nil
}

// explicitApplySlotSources validates an authored multi-child map without
// recovering any address from a nominal relation/column.  A slot is valid only
// when its exact child/cell points at the signature's declared column and
// owner relation.
func explicitApplySlotSources(sources []algebra.SlotSource, semantic signature.Signature, layouts []tupleLayout) ([]algebra.SlotSource, error) {
	if len(sources) != semantic.InputLen() {
		return nil, fmt.Errorf("relcompile: semantic input count %d does not match explicit Apply slot sources %d", semantic.InputLen(), len(sources))
	}
	result := append([]algebra.SlotSource(nil), sources...)
	for index, source := range result {
		child := int(source.Child())
		if child < 0 || child >= len(layouts) {
			return nil, fmt.Errorf("relcompile: Apply slot %d child %d is outside the sealed child set", index, source.Child())
		}
		cell := int(source.Cell())
		if cell < 0 || cell >= len(layouts[child].cells) {
			return nil, fmt.Errorf("relcompile: Apply slot %d cell %d is outside child %d", index, source.Cell(), source.Child())
		}
		input, ok := semantic.InputAt(index)
		if !ok {
			return nil, fmt.Errorf("relcompile: Apply slot %d has no sealed semantic input", index)
		}
		selected := layouts[child].cells[cell]
		if selected.column != input.Column || selected.column.Relation() != input.Relation {
			return nil, fmt.Errorf("relcompile: Apply slot %d does not address its declared semantic input", index)
		}
	}
	return result, nil
}

func validColumns(values []model.ColumnID, columns map[model.ColumnID]struct{}) bool {
	for _, column := range values {
		if _, ok := columns[column]; !ok {
			return false
		}
	}
	return true
}

// validPublicationColumns proves the one closed semantic output vector a
// Publish contract may commit.  The vector is positional at runtime, but the
// compiler still checks its nominal ownership while resolving declarations so
// it never emits a projection that searches a row for an alien column.
func validPublicationColumns(values []model.ColumnID, relation model.RelationID, columns map[model.ColumnID]struct{}) bool {
	if !relation.Available() || len(values) == 0 {
		return false
	}
	seen := make(map[model.ColumnID]struct{}, len(values))
	for _, column := range values {
		if !column.Available() || column.Relation() != relation {
			return false
		}
		if _, ok := columns[column]; !ok {
			return false
		}
		if _, duplicate := seen[column]; duplicate {
			return false
		}
		seen[column] = struct{}{}
	}
	return true
}

func containsRelation(relations map[model.RelationID]struct{}, relation model.RelationID) bool {
	_, ok := relations[relation]
	return ok
}

func containsColumn(columns map[model.ColumnID]struct{}, column model.ColumnID) bool {
	_, ok := columns[column]
	return ok
}

func appendUniqueRelation(values []model.RelationID, relation model.RelationID) []model.RelationID {
	for _, existing := range values {
		if existing == relation {
			return values
		}
	}
	return append(values, relation)
}

func relationRefs(values []model.RelationID) []plan.RelationRef {
	refs := make([]plan.RelationRef, 0, len(values))
	for _, relation := range values {
		ref, ok := plan.NewRelationRef(relation)
		if ok {
			refs = append(refs, ref)
		}
	}
	return refs
}

// tupleLayout is the compiler-side mirror of the sealed tuple structure a
// relational expression produces. It is not a second runtime representation:
// it exists only while Compile lowers declared read occurrences into immutable
// Apply slot addresses. The mounted evaluator later validates/redeems the
// same ordered child and cell coordinates.
type tupleLayout struct {
	sources []model.RelationID
	cells   []tupleCell
}

type tupleCell struct {
	column model.ColumnID
	source uint32
}

// completeTupleLayout redeems the schema-level Complete cell law while the
// compiler still owns declaration identities.  The compiler emits complete
// Inputs with their full denominator vectors (see addCompleteRequirements),
// so this is normally an idempotence check; keeping the shared law here makes
// a future lowering unable to mint SlotSources against a hidden extension.
func completeTupleLayout(layout tupleLayout, denominator model.DenominatorRef, columns []model.ColumnID) (tupleLayout, bool) {
	cells := make([]algebra.CellLayoutCell, len(layout.cells))
	for index, cell := range layout.cells {
		cells[index] = algebra.NewCellLayoutCell(cell.column, cell.source)
	}
	canonical, canonicalOK := algebra.NewCellLayout(layout.sources, cells)
	if !canonicalOK {
		return tupleLayout{}, false
	}
	completed, completedOK := algebra.CompleteCellLayout(canonical, denominator, columns)
	if !completedOK {
		return tupleLayout{}, false
	}
	result := tupleLayout{sources: completed.Sources(), cells: make([]tupleCell, completed.Len())}
	for index := 0; index < completed.Len(); index++ {
		cell, cellOK := completed.CellAt(index)
		if !cellOK {
			return tupleLayout{}, false
		}
		result.cells[index] = tupleCell{column: cell.Column(), source: cell.Source()}
	}
	return result, true
}

func inputLayout(relation model.RelationID, relationColumns map[model.RelationID][]model.ColumnID) (tupleLayout, bool) {
	columns, ok := relationColumns[relation]
	if !ok {
		return tupleLayout{}, false
	}
	return inputLayoutColumns(relation, columns)
}

func projectedInputLayout(relation model.RelationID, columns []model.ColumnID, relationColumns map[model.RelationID][]model.ColumnID) (tupleLayout, bool) {
	declared, ok := relationColumns[relation]
	if !ok || len(columns) == 0 {
		return tupleLayout{}, false
	}
	declaredSet := make(map[model.ColumnID]struct{}, len(declared))
	for _, column := range declared {
		declaredSet[column] = struct{}{}
	}
	for _, column := range columns {
		if _, ok := declaredSet[column]; !ok {
			return tupleLayout{}, false
		}
	}
	return inputLayoutColumns(relation, columns)
}

func inputLayoutColumns(relation model.RelationID, columns []model.ColumnID) (tupleLayout, bool) {
	if !relation.Available() || len(columns) == 0 {
		return tupleLayout{}, false
	}
	seen := make(map[model.ColumnID]struct{}, len(columns))
	result := tupleLayout{sources: []model.RelationID{relation}, cells: make([]tupleCell, len(columns))}
	for index, column := range columns {
		if !column.Available() || column.Relation() != relation {
			return tupleLayout{}, false
		}
		if _, duplicate := seen[column]; duplicate {
			return tupleLayout{}, false
		}
		seen[column] = struct{}{}
		result.cells[index] = tupleCell{column: column, source: 0}
	}
	return result, true
}

func exactInput(relation model.RelationID, columns []model.ColumnID) (algebra.Input, bool) {
	input, ok := algebra.NewInputColumns(relation, columns)
	return input, ok && input.IsExactColumns()
}

func joinLayout(left, right tupleLayout) tupleLayout {
	result := tupleLayout{
		sources: append([]model.RelationID(nil), left.sources...),
		cells:   append([]tupleCell(nil), left.cells...),
	}
	offset := uint32(len(result.sources))
	result.sources = append(result.sources, right.sources...)
	for _, cell := range right.cells {
		cell.source += offset
		result.cells = append(result.cells, cell)
	}
	return result
}

func expandLayout(left, right tupleLayout) tupleLayout {
	return joinLayout(left, right)
}

// applyOutputLayout mirrors the exact semantic row emitted by Apply.  A
// signature has already named the output columns in order; compilation keeps
// that order instead of reconstructing a destination relation's full row.
func applyOutputLayout(semantic signature.Signature) (tupleLayout, bool) {
	outputs := semantic.Outputs()
	if !semantic.Available() || len(outputs) == 0 {
		return tupleLayout{}, false
	}
	relation := outputs[0].Relation
	if !relation.Available() {
		return tupleLayout{}, false
	}
	result := tupleLayout{sources: []model.RelationID{relation}, cells: make([]tupleCell, 0, len(outputs))}
	seen := make(map[model.ColumnID]struct{}, len(outputs))
	for _, output := range outputs {
		if !output.Available() || output.Relation != relation || !output.Column.Available() {
			return tupleLayout{}, false
		}
		if _, duplicate := seen[output.Column]; duplicate {
			return tupleLayout{}, false
		}
		seen[output.Column] = struct{}{}
		result.cells = append(result.cells, tupleCell{column: output.Column, source: 0})
	}
	return result, true
}

// projectColumns seals the exact child cell ordinals carrying a writable
// output vector.  Nominal lookup is confined to this cold declaration
// lowering pass and must resolve exactly one existing cell; evaluation only
// redeems the emitted ColumnSlot positions.
func projectColumns(expression algebra.Expression, layout tupleLayout, columns []model.ColumnID) (algebra.Expression, tupleLayout, error) {
	if expression == nil || len(columns) == 0 {
		return nil, tupleLayout{}, fmt.Errorf("relcompile: projection has no closed child/output layout")
	}
	if sameColumnLayout(columns, layoutColumns(layout)) {
		return expression, layout, nil
	}
	slots := make([]algebra.ColumnSlot, len(columns))
	projected := tupleLayout{sources: append([]model.RelationID(nil), layout.sources...), cells: make([]tupleCell, 0, len(columns))}
	seen := make(map[model.ColumnID]struct{}, len(columns))
	for index, column := range columns {
		if _, duplicate := seen[column]; duplicate {
			return nil, tupleLayout{}, fmt.Errorf("relcompile: projection repeats writable column")
		}
		seen[column] = struct{}{}
		cellIndex := -1
		for candidate, cell := range layout.cells {
			if cell.column != column {
				continue
			}
			if cellIndex >= 0 {
				return nil, tupleLayout{}, fmt.Errorf("relcompile: projection column has more than one sealed child cell")
			}
			cellIndex = candidate
		}
		if cellIndex < 0 {
			return nil, tupleLayout{}, fmt.Errorf("relcompile: projection column is not present in child layout")
		}
		slots[index] = algebra.NewColumnSlot(column, uint32(cellIndex))
		projected.cells = append(projected.cells, layout.cells[cellIndex])
	}
	return algebra.NewColumnProject(expression, algebra.NewColumnProjectContract(slots)), projected, nil
}

func layoutColumns(layout tupleLayout) []model.ColumnID {
	columns := make([]model.ColumnID, len(layout.cells))
	for index, cell := range layout.cells {
		columns[index] = cell.column
	}
	return columns
}

func sameColumnLayout(left, right []model.ColumnID) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sameTupleLayout(left, right tupleLayout) bool {
	if len(left.sources) != len(right.sources) || len(left.cells) != len(right.cells) {
		return false
	}
	for index := range left.sources {
		if left.sources[index] != right.sources[index] {
			return false
		}
	}
	for index := range left.cells {
		if left.cells[index] != right.cells[index] {
			return false
		}
	}
	return true
}

func remapSlotSource(source algebra.SlotSource, full, projected tupleLayout) (algebra.SlotSource, bool) {
	if source.Child() != 0 || int(source.Cell()) >= len(full.cells) {
		return algebra.SlotSource{}, false
	}
	cell := full.cells[source.Cell()]
	var remapped algebra.SlotSource
	found := false
	for index, candidate := range projected.cells {
		if candidate.column != cell.column || candidate.source != cell.source {
			continue
		}
		if found {
			return algebra.SlotSource{}, false
		}
		remapped = algebra.NewSlotSource(0, uint32(index))
		found = true
	}
	return remapped, found
}

func remapSlotSources(sources []algebra.SlotSource, full, projected []tupleLayout) ([]algebra.SlotSource, error) {
	result := make([]algebra.SlotSource, len(sources))
	for index, source := range sources {
		child := int(source.Child())
		if child < 0 || child >= len(full) || child >= len(projected) {
			return nil, fmt.Errorf("relcompile: Apply slot %d child is outside the sealed child set", index)
		}
		// remapSlotSource addresses one child-local tuple. Keep the authored
		// child coordinate on the result, but strip it while looking up the
		// cell so a slot from child N is not rejected as a child-0 source.
		local := algebra.NewSlotSource(0, source.Cell())
		remapped, ok := remapSlotSource(local, full[child], projected[child])
		if !ok {
			return nil, fmt.Errorf("relcompile: Apply slot %d does not address a retained child cell", index)
		}
		result[index] = algebra.NewSlotSource(source.Child(), remapped.Cell())
	}
	return result, nil
}

func remapOutputAddress(address algebra.OutputAddress, full, projected tupleLayout) (algebra.OutputAddress, bool) {
	if !address.Available() {
		return algebra.OutputAddress{}, false
	}
	if address.IsOwnerNamed() {
		return address, true
	}
	source, ok := address.Source()
	if !ok {
		return algebra.OutputAddress{}, false
	}
	remapped, ok := remapSlotSource(source, full, projected)
	if !ok {
		return algebra.OutputAddress{}, false
	}
	if address.IsScalarSource() {
		return algebra.ScalarSource(remapped), true
	}
	if address.IsSpanSource() {
		return algebra.SpanSource(remapped), true
	}
	return algebra.OutputAddress{}, false
}

func remapOutputAddressAcrossChildren(address algebra.OutputAddress, full, projected []tupleLayout) (algebra.OutputAddress, bool) {
	if !address.Available() {
		return algebra.OutputAddress{}, false
	}
	if address.IsOwnerNamed() {
		return address, true
	}
	source, ok := address.Source()
	if !ok || int(source.Child()) >= len(full) || int(source.Child()) >= len(projected) {
		return algebra.OutputAddress{}, false
	}
	child := source.Child()
	local := algebra.NewSlotSource(0, source.Cell())
	remapped, ok := remapSlotSource(local, full[child], projected[child])
	if !ok {
		return algebra.OutputAddress{}, false
	}
	if address.IsScalarSource() {
		return algebra.ScalarSource(algebra.NewSlotSource(child, remapped.Cell())), true
	}
	if address.IsSpanSource() {
		return algebra.SpanSource(algebra.NewSlotSource(child, remapped.Cell())), true
	}
	return algebra.OutputAddress{}, false
}

// applySlotSources is the only declaration-to-physical Apply address
// lowering. Every source is an authored Rule occurrence; every cell is found
// in the ordered tuple layout of that occurrence. It never resolves by the
// first nominal relation/column match, so duplicate relation reads remain
// distinct and multi-column slots from one row share one source occurrence.
func applySlotSources(occurrences []ReadOccurrence, semantic signature.Signature, layout tupleLayout, joinCount int) ([]algebra.SlotSource, error) {
	if len(occurrences) != semantic.InputLen() {
		return nil, fmt.Errorf("relcompile: semantic input count %d does not match declared apply occurrences %d", semantic.InputLen(), len(occurrences))
	}
	result := make([]algebra.SlotSource, len(occurrences))
	for index, occurrence := range occurrences {
		if !occurrence.available(joinCount) {
			return nil, fmt.Errorf("relcompile: apply slot %d names an unavailable read occurrence", index)
		}
		source := uint32(0)
		if join, joined := occurrence.Join(); joined {
			source = join + 1 // candidate is source row zero.
		}
		if int(source) >= len(layout.sources) {
			return nil, fmt.Errorf("relcompile: apply slot %d source is outside the sealed tuple layout", index)
		}
		input, inputOK := semantic.InputAt(index)
		if !inputOK || layout.sources[source] != input.Relation {
			return nil, fmt.Errorf("relcompile: apply slot %d declared occurrence does not own the semantic input relation", index)
		}
		cellIndex := -1
		for candidate, cell := range layout.cells {
			if cell.source != source || cell.column != input.Column {
				continue
			}
			if cellIndex >= 0 {
				return nil, fmt.Errorf("relcompile: apply slot %d has duplicate cells in one declared read occurrence", index)
			}
			cellIndex = candidate
		}
		if cellIndex < 0 {
			return nil, fmt.Errorf("relcompile: apply slot %d declared occurrence does not publish the semantic input column", index)
		}
		result[index] = algebra.NewSlotSource(0, uint32(cellIndex))
	}
	return result, nil
}

// repeatCandidateOccurrences is the one-source carry form: Carry declares one
// carried relation input, so every transform slot is explicitly supplied by
// that relation's only tuple source. This is a declared same-row grouping, not
// a relation-name fallback.
func repeatCandidateOccurrences(count int) []ReadOccurrence {
	result := make([]ReadOccurrence, count)
	for index := range result {
		result[index] = CandidateOccurrence()
	}
	return result
}

// uniqueCarryOutputSource proves the authored destination geometry names one
// and only one semantic slot of the carried Apply.  A relation/column scan is
// intentionally not used here: repeated nominal relations are valid and only
// the sealed SlotSource occurrence distinguishes their cells.
func uniqueCarryOutputSource(output algebra.OutputAddress, slots []algebra.SlotSource) bool {
	if !output.Available() {
		return false
	}
	if output.IsOwnerNamed() {
		return true
	}
	source, ok := output.Source()
	if !ok {
		return false
	}
	matches := 0
	for _, slot := range slots {
		if slot == source {
			matches++
		}
	}
	return matches == 1
}
