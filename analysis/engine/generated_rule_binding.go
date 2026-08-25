package engine

// generated_rule_binding.go is the separate sealed arm for Plan-generated
// Rules. It intentionally does not implement sealedRuleCell: generated rows
// have no typed operand, callback, selector, or legacy bindMember path.

import (
	"github.com/wippyai/go-lua/analysis/engine/generated"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	memberrelation "github.com/wippyai/go-lua/analysis/schema/axis/member/relation"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// generatedRuleBindingCell is the bind-time schema identity of one generated
// Rule. Its only retained Rule data is the immutable Plan projection on cell;
// all per-occurrence coordinates are reduced during program construction.
type generatedRuleBindingCell struct {
	state     *schemaBindingState
	schema    *Schema
	ordinal   uint64
	generated *generatedRuleCell
	// reads is this rule's sealed read geometry, one row per declared join, in
	// plan order. It is derived once from the cold shape the Plan already
	// sealed, so a generated read and an authored one are the same row and the
	// anchored surface minters have exactly one implementation to serve.
	reads []*schemaRuleReadRow
	// writeMode and routeRead are the publication disposition the Plan
	// declared. A routed write names the selected join it publishes over and
	// resolves no destination here: the destinations are the members of that
	// join's derived relation, which exist only per invocation.
	writeMode directRuleWriteMode
	routeRead uint64
}

var _ schemaRuleBindingCell = (*generatedRuleBindingCell)(nil)
var _ sealedRuleGeometry = (*generatedRuleBindingCell)(nil)

func (cell *generatedRuleBindingCell) schemaBindingSchema() *Schema {
	if cell == nil {
		return nil
	}
	return cell.schema
}

func (cell *generatedRuleBindingCell) schemaRuleOrdinal() uint64 {
	if cell == nil {
		return 0
	}
	return cell.ordinal
}

func (cell *generatedRuleBindingCell) schemaRuleBindingState() *schemaBindingState {
	if cell == nil {
		return nil
	}
	return cell.state
}

// schemaRuleReadAt answers this rule's own sealed read geometry. A generated
// rule has read rows for the same reason an authored one does: a selected read
// and a routed write are anchored against the row they are declared on, and
// that anchoring is one implementation for both arms.
func (cell *generatedRuleBindingCell) schemaRuleReadAt(index uint64) *schemaRuleReadRow {
	if cell == nil || index >= uint64(len(cell.reads)) {
		return nil
	}
	return cell.reads[index]
}

// declareReadGeometry derives this rule's read rows and publication
// disposition from the cold shape its Plan sealed. The cold shape is the one
// authority: nothing here re-reads the descriptor for a fact the shape already
// states, so a generated row cannot disagree with the composition it was
// admitted under.
func (cell *generatedRuleBindingCell) declareReadGeometry() bool {
	if cell == nil || cell.schema == nil || cell.generated == nil || !cell.generated.available() {
		return false
	}
	shape, shapeOK := cell.schema.ruleShapeAt(cell.ordinal)
	if !shapeOK || shape.ReadCount != uint64(cell.generated.program.ReadCount()) {
		return false
	}
	reads := make([]*schemaRuleReadRow, 0, shape.ReadCount)
	for index := uint64(0); index < shape.ReadCount; index++ {
		read, readOK := cell.schema.ruleReadShapeAt(cell.ordinal, index)
		if !readOK || !read.Factor.Available() || !schemaRuleInputFitsInt(index) {
			return false
		}
		row := &schemaRuleReadRow{
			owner: cell, ownerOrdinal: cell.ordinal, readOrdinal: index,
			input: read.Input, kind: read.Kind, factor: read.Factor,
			semantic: read.Semantic, normalizer: read.Normalizer,
		}
		factorOrdinal, factorOrdinalOK := cell.schema.factorOrdinalOf(read.Factor)
		if !factorOrdinalOK {
			return false
		}
		row.factorOrdinal = factorOrdinal
		for dependency := uint64(0); dependency < read.DependencyCount; dependency++ {
			position, positionOK := cell.schema.ruleReadDependencyAt(cell.ordinal, index, dependency)
			if !positionOK || position >= index {
				return false
			}
			row.dependencies = append(row.dependencies, position)
		}
		reads = append(reads, row)
	}
	mode, modeOK := cell.generated.program.OutputMode()
	if !modeOK {
		return false
	}
	if mode == ruleprogram.ModeStructural {
		// A structural rule declares no write shape at all. Its publication is
		// the activation row set the cold row's family admits, and there is no
		// Factor cell for a write to address.
		if shape.WriteCount != 0 {
			return false
		}
		cell.writeMode, cell.routeRead = directRuleWriteStructural, 0
		cell.reads = reads
		return true
	}
	write, writeOK := cell.schema.ruleWriteShapeAt(cell.ordinal, 0)
	if !writeOK || shape.WriteCount != 1 {
		return false
	}
	switch write.Kind {
	case composition.WriteExact:
		if write.Route != 0 {
			return false
		}
		cell.writeMode, cell.routeRead = directRuleWriteExact, 0
	case composition.WriteRoute:
		if write.Route == 0 || write.Route > shape.ReadCount {
			return false
		}
		route := reads[write.Route-1]
		if route.kind != composition.ReadSelect || route.factor != write.Factor {
			return false
		}
		cell.writeMode, cell.routeRead = directRuleWriteRoute, write.Route
	default:
		return false
	}
	cell.reads = reads
	return true
}

func (cell *generatedRuleBindingCell) schemaRuleComplete() bool {
	if cell == nil || cell.state == nil || cell.schema == nil || cell.generated == nil ||
		cell.state.schema != cell.schema || cell.state.authority == nil ||
		cell.ordinal >= uint64(len(cell.state.rules)) || cell.state.rules[cell.ordinal] != cell ||
		!cell.generated.available() {
		return false
	}
	if cell.ordinal > uint64(^uint32(0)) || cell.generated.rule != uint32(cell.ordinal) {
		return false
	}
	factorCount := schemaFactorCount(cell.schema)
	program := cell.generated.program
	candidate := program.CandidateRelation()
	reducer := program.Reducer()
	output := program.OutputAddress()
	destination := program.DestinationProjection()
	if factorCount <= 0 ||
		uint64(candidate.Axis) >= uint64(factorCount) || uint64(reducer.Axis) >= uint64(factorCount) || uint64(output.Axis) >= uint64(factorCount) || uint64(destination.Axis) >= uint64(factorCount) ||
		program.OutputAxis() >= uint32(factorCount) || program.OutputFactor() >= uint32(factorCount) {
		return false
	}
	shape, shapeOK := cell.schema.ruleShapeAt(cell.ordinal)
	if !shapeOK || shape.Inputs != uint64(program.InputCount()) {
		return false
	}
	outputFactor := cell.schema.factorSemanticAt(uint64(program.OutputFactor()))
	if !outputFactor.Available() {
		return false
	}
	structural := shape.OutputKind == composition.StructuralOutput
	if structural {
		// The cold structural row names no output Factor and no write. What it
		// names instead is the activation family its candidate branches are
		// admitted under, and the axis the descriptor still carries is the one
		// its rows are indexed by - which is what routes them to a typed plane
		// at execution, not a cell they publish into.
		if shape.Output.Available() || shape.WriteCount != 0 || shape.CarryCount != 0 ||
			shape.ActivationCount != 1 || !shape.ActivationFamily.Available() ||
			program.TransportCount() == 0 || program.CarryInput() >= 0 ||
			cell.writeMode != directRuleWriteStructural || cell.routeRead != 0 {
			return false
		}
	} else if shape.OutputKind != composition.FactorOutput || shape.WriteCount != 1 || shape.Output != outputFactor {
		return false
	}
	// The publication disposition is the Plan's, and the cold row records it.
	// An exact write resolves its one destination while the row is declared; a
	// routed write names the selected join whose derived members ARE the
	// destinations, and resolves none of them here.
	mode, modeOK := program.OutputMode()
	if !modeOK || (mode == ruleprogram.ModeStructural) != structural {
		return false
	}
	outputPlan, outputPlanOK := program.OutputAt(0)
	if !outputPlanOK {
		return false
	}
	if structural {
		// A structural publication stages no fact, so it carries neither the
		// exact writer's evidence nor a route. Its joins are the whole of what
		// it observes, and a rule that observes nothing selects no branch.
		if outputPlan.RouteJoinPresent || outputPlan.Exact || outputPlan.Strong || program.ReadCount() == 0 {
			return false
		}
	} else {
		write, writeOK := cell.schema.ruleWriteShapeAt(cell.ordinal, 0)
		if !writeOK || write.Factor != outputFactor {
			return false
		}
		switch mode {
		case ruleprogram.ModeExact:
			if write.Kind != composition.WriteExact || write.Route != 0 || cell.writeMode != directRuleWriteExact || cell.routeRead != 0 ||
				outputPlan.RouteJoinPresent {
				return false
			}
		case ruleprogram.ModeRoute:
			if write.Kind != composition.WriteRoute || !outputPlan.RouteJoinPresent ||
				write.Route != uint64(outputPlan.RouteJoin)+1 || cell.writeMode != directRuleWriteRoute || cell.routeRead != write.Route {
				return false
			}
		default:
			return false
		}
	}
	if program.ReadCount() == 0 {
		if shape.ReadCount != 0 || shape.CarryCount != 0 || program.InputCount() != 0 || len(cell.reads) != 0 ||
			mode != ruleprogram.ModeExact {
			return false
		}
		return cell.state.phase == schemaBindingOpen || cell.state.phase == schemaBindingSealed
	}
	if shape.ReadCount != uint64(program.ReadCount()) || len(cell.reads) != program.ReadCount() {
		return false
	}
	// A join names the Factor it reads, which need not be the Factor this rule
	// writes. A rule whose join is foreign is exactly the rule the engine
	// cannot type generically, and it reaches execution through the family its
	// own package installs.
	for index := 0; index < program.ReadCount(); index++ {
		plan, planOK := program.ReadAt(index)
		row := cell.reads[index]
		if !planOK || row == nil || uint64(plan.Factor) >= uint64(factorCount) || uint64(plan.Axis) >= uint64(factorCount) {
			return false
		}
		if row.input != uint64(plan.Input) || row.factorOrdinal != uint64(plan.Factor) ||
			row.factor != cell.schema.factorSemanticAt(uint64(plan.Factor)) {
			return false
		}
		if row.kind != generatedColdReadKind(plan.Form, plan.ParentPresent) {
			return false
		}
	}
	if shape.CarryCount > 1 {
		return false
	}
	if shape.CarryCount == 1 {
		carry, carryOK := cell.schema.ruleCarryShapeAt(cell.ordinal, 0)
		if !carryOK || carry.Input != uint64(program.CarryInput()) || carry.Factor != outputFactor {
			return false
		}
		// The cold carry row of a generated rule names no transform: which
		// transform a carried fact passes through is the descriptor's
		// statement, resolved by the family that installs the fold, so a cold
		// transform key here would be a second authority over it.
		if carry.Transform.Available() {
			return false
		}
	} else if program.CarryInput() >= 0 {
		return false
	}
	return cell.state.phase == schemaBindingOpen || cell.state.phase == schemaBindingSealed
}

// generatedColdReadKind is the one projection of a declared read onto the cold
// read kind. The slot that builds the cold row calls it too, so the row and the
// completeness check above cannot disagree about what a declared form is.
//
// A vector over a self-provided nested member set is the one form whose kind is
// not its form alone. That set's own MemberCount and MemberAt are its
// denominator, so no Factor summary form answers it and its cells are not the
// row's own exact coordinate either: the installing family resolves which
// coordinates the vector spans, which is precisely what a selection cold row
// states. The parent restatement is the declaration that says so.
// generatedColdReadKind maps one declared read form onto the cold row kind it
// is lowered as. A whole-vector read whose cells are addressed one at a time -
// a nested member set, or a span its candidate published - is delivered
// through the selection surface, because that is the surface a per-cell
// delivery has; a vector read over a Factor's own summary form is not.
func generatedColdReadKind(form ruleprogram.ReadForm, memberAddressed bool) composition.ReadKind {
	switch form {
	case ruleprogram.Exact:
		return composition.ReadExact
	case ruleprogram.Selected:
		return composition.ReadSelect
	case ruleprogram.Summary, ruleprogram.Complete:
		if memberAddressed {
			return composition.ReadSelect
		}
		return composition.ReadSummary
	default:
		return 0
	}
}

func (cell *generatedRuleBindingCell) generatedCell() *generatedRuleCell {
	if cell == nil {
		return nil
	}
	return cell.generated
}

// The graph compiler consumes one sealed structural geometry interface for
// ordinary and generated Rules. Generated cells implement that geometry
// directly from their compiled descriptor; they do not recreate a HotRule,
// typed operand, or schemaRuleReadRow.
func (cell *generatedRuleBindingCell) sealedRuleComplete() bool {
	return cell.schemaRuleComplete()
}

func (cell *generatedRuleBindingCell) directRuleSemantic() composition.Key {
	if cell == nil || cell.schema == nil {
		return composition.Key{}
	}
	return cell.schema.ruleSemanticAt(cell.ordinal)
}

func (cell *generatedRuleBindingCell) directRuleOperandFamily() composition.Key {
	if cell == nil || cell.schema == nil {
		return composition.Key{}
	}
	shape, ok := cell.schema.ruleShapeAt(cell.ordinal)
	if !ok {
		return composition.Key{}
	}
	return shape.OperandFamily
}

func (cell *generatedRuleBindingCell) directRuleOutputFactor() composition.Key {
	if cell == nil || cell.schema == nil || cell.generated == nil {
		return composition.Key{}
	}
	return cell.schema.factorSemanticAt(uint64(cell.generated.program.OutputFactor()))
}

func (cell *generatedRuleBindingCell) directRuleReadCount() uint64 {
	if cell == nil || cell.generated == nil {
		return 0
	}
	return uint64(cell.generated.program.ReadCount())
}

func (cell *generatedRuleBindingCell) directRuleWriteMode() directRuleWriteMode {
	if cell == nil {
		return 0
	}
	return cell.writeMode
}

func (cell *generatedRuleBindingCell) directRuleRouteRead() uint64 {
	if cell == nil {
		return 0
	}
	return cell.routeRead
}

func (cell *generatedRuleBindingCell) directRuleCarryPresent() bool {
	if cell == nil || cell.generated == nil {
		return false
	}
	return cell.generated.program.CarryIdentity()
}

func (cell *generatedRuleBindingCell) directRuleCarryInput() uint64 {
	if cell == nil || cell.generated == nil || cell.generated.program.CarryInput() < 0 {
		return 0
	}
	return uint64(cell.generated.program.CarryInput())
}

// generatedRuleSchema returns the canonical cold semantic pair for a generated
// cell. It never consults a caller-provided role or operand provider.
func generatedRuleSchema(cell *generatedRuleBindingCell) (composition.Key, composition.Key, bool) {
	if cell == nil || !cell.schemaRuleComplete() || cell.schema == nil {
		return composition.Key{}, composition.Key{}, false
	}
	shape, ok := cell.schema.ruleShapeAt(cell.ordinal)
	if !ok || !shape.OperandFamily.Available() {
		return composition.Key{}, composition.Key{}, false
	}
	return cell.schema.ruleSemanticAt(cell.ordinal), shape.OperandFamily, true
}

// BindGeneratedRule installs the generated sealed arm at the slot's canonical
// ordinal. It is intentionally callback-free: all executable type ownership is
// supplied later by the matching Factor cells.
func BindGeneratedRule(binding *SchemaBinding, slot *GeneratedRuleSlot) bool {
	state := bindingState(binding)
	if state == nil {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || state.schema == nil || slot == nil || slot.cell == nil || slot.cell.schema != state.schema {
		if state.phase == schemaBindingOpen {
			state.poisonLocked()
		}
		return false
	}
	ordinal, ordinalOK := slot.Ordinal()
	generated := slot.cell.generated
	if !ordinalOK || generated == nil || ordinal >= uint64(len(state.rules)) || state.rules[ordinal] != nil {
		state.poisonLocked()
		return false
	}
	// A structural rule's branches are admitted under an activation family, and
	// that family's cell is bound where the rule that declares it is bound -
	// the same place the hand-declared activation lane binds it. Without this
	// the cold schema counts a family the Binding never seats, and Seal refuses
	// the whole composition rather than the one rule that owes it.
	shape, shapeOK := state.schema.ruleShapeAt(ordinal)
	if !shapeOK {
		state.poisonLocked()
		return false
	}
	if shape.OutputKind == composition.StructuralOutput && !bindActivationFamilyLocked(state, shape.ActivationFamily) {
		state.poisonLocked()
		return false
	}
	cell := &generatedRuleBindingCell{state: state, schema: state.schema, ordinal: ordinal, generated: generated}
	state.rules[ordinal] = cell
	if !cell.declareReadGeometry() {
		state.poisonLocked()
		return false
	}
	if !cell.schemaRuleComplete() {
		state.poisonLocked()
		return false
	}
	return true
}

// completeGeneratedRelationOwnersLocked is called by SchemaBinding.Seal after
// every generated cell has passed its cold shape check. A generated Plan is
// axis-addressed, so every bound axis must publish exactly one owner before
// the binding can seal.
func completeGeneratedRelationOwnersLocked(state *schemaBindingState) bool {
	if state == nil || state.schema == nil || len(state.rules) == 0 {
		return true
	}
	present := false
	for _, raw := range state.rules {
		if _, generated := raw.(*generatedRuleBindingCell); generated {
			present = true
			break
		}
	}
	if !present {
		return true
	}
	if len(state.factors) != schemaFactorCount(state.schema) {
		return false
	}
	for _, raw := range state.rules {
		cell, generated := raw.(*generatedRuleBindingCell)
		if !generated || cell == nil || cell.generated == nil {
			continue
		}
		descriptor := cell.generated.program
		axes, axesOK := generatedRelationOwnerAxes(descriptor)
		if !axesOK {
			return false
		}
		seen := make(map[uint32]struct{}, len(axes))
		for _, axis := range axes {
			if _, duplicate := seen[axis]; duplicate {
				continue
			}
			seen[axis] = struct{}{}
			if uint64(axis) >= uint64(len(state.factors)) || state.factors[axis] == nil || state.factors[axis].schemaFactorRelationOwner() == nil {
				return false
			}
		}
	}
	return true
}

// generatedRelationOwnerAxes returns every axis whose sealed member rows one
// descriptor addresses. It deliberately walks the complete ordered read
// table: the first-read convenience accessors are not a completeness proof
// for an exact product or any dependent join program.
func generatedRelationOwnerAxes(descriptor generated.CompiledRule) ([]uint32, bool) {
	if !descriptor.Available() {
		return nil, false
	}
	axes := []uint32{
		descriptor.CandidateRelation().Axis,
		descriptor.Reducer().Axis,
		descriptor.OutputAddress().Axis,
		descriptor.DestinationProjection().Axis,
		descriptor.OutputAxis(),
	}
	for index := 0; index < descriptor.ReadCount(); index++ {
		read, ok := descriptor.ReadAt(index)
		if !ok {
			return nil, false
		}
		axes = append(axes, read.Relation.Axis, read.Key.Axis, read.Factor)
		if read.PredicatePresent {
			axes = append(axes, read.Predicate.Axis)
		}
	}
	return axes, true
}

// relationOwnerForGeneratedAxis is the construction-only axis lookup. The
// factor ordinal and owner axis are the same dense catalog coordinate; no
// semantic-key or runtime fallback is permitted.
func relationOwnerForGeneratedAxis(state *schemaBindingState, axis uint32) (memberrelation.Owner, bool) {
	if state == nil {
		return nil, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingSealed || state.authority == nil || uint64(axis) >= uint64(len(state.factors)) || state.factors[axis] == nil {
		return nil, false
	}
	owner := state.factors[axis].schemaFactorRelationOwner()
	return owner, owner != nil
}
