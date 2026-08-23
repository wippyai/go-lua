package engine

// generated_rule_binding.go is the separate sealed arm for Plan-generated
// Rules. It intentionally does not implement sealedRuleCell: generated rows
// have no typed operand, callback, selector, or legacy bindMember path.

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	memberrelation "github.com/wippyai/go-lua/analysis/schema/axis/member/relation"
)

// generatedRuleBindingCell is the bind-time schema identity of one generated
// Rule. Its only retained Rule data is the immutable Plan projection on cell;
// all per-occurrence coordinates are reduced during program construction.
type generatedRuleBindingCell struct {
	state     *schemaBindingState
	schema    *Schema
	ordinal   uint64
	generated *generatedRuleCell
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

// Generated Rules own their read surface directly from Factor cells, so they
// have no legacy typed read rows. Returning nil keeps every legacy selector
// path fail-closed if it is accidentally presented with this arm.
func (*generatedRuleBindingCell) schemaRuleReadAt(uint64) *schemaRuleReadRow { return nil }

func (cell *generatedRuleBindingCell) schemaRuleComplete() bool {
	if cell == nil || cell.state == nil || cell.schema == nil || cell.generated == nil ||
		cell.state.schema != cell.schema || cell.state.authority == nil ||
		cell.ordinal >= uint64(len(cell.state.rules)) || cell.state.rules[cell.ordinal] != cell ||
		!cell.generated.available() {
		return false
	}
	ordinal, ordinalOK := cell.generated.program.Ordinal()
	if !ordinalOK || ordinal != uint32(cell.ordinal) || cell.ordinal > uint64(^uint32(0)) {
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
	if !shapeOK || shape.OutputKind != composition.FactorOutput || shape.Inputs != uint64(program.InputCount()) || shape.WriteCount != 1 {
		return false
	}
	outputFactor := cell.schema.factorSemanticAt(uint64(program.OutputFactor()))
	if !outputFactor.Available() || shape.Output != outputFactor {
		return false
	}
	write, writeOK := cell.schema.ruleWriteShapeAt(cell.ordinal, 0)
	if !writeOK || write.Kind != composition.WriteExact || write.Route != 0 || write.Factor != outputFactor {
		return false
	}
	if program.ReadCount() == 0 {
		if shape.ReadCount != 0 || shape.CarryCount != 0 || program.InputCount() != 0 {
			return false
		}
		return cell.state.phase == schemaBindingOpen || cell.state.phase == schemaBindingSealed
	}
	join := program.JoinRelation()
	key := program.KeyProjection()
	if uint64(join.Axis) >= uint64(factorCount) || uint64(key.Axis) >= uint64(factorCount) ||
		program.ReadAxis() >= uint32(factorCount) || program.ReadFactor() >= uint32(factorCount) ||
		program.ReadFactor() != program.OutputFactor() {
		return false
	}
	if shape.ReadCount != 1 || shape.CarryCount != 1 {
		return false
	}
	readFactor := cell.schema.factorSemanticAt(uint64(program.ReadFactor()))
	if !readFactor.Available() {
		return false
	}
	read, readOK := cell.schema.ruleReadShapeAt(cell.ordinal, 0)
	carry, carryOK := cell.schema.ruleCarryShapeAt(cell.ordinal, 0)
	if !readOK || !carryOK || read.Kind != composition.ReadExact || read.Input != uint64(program.ReadInput()) || read.Factor != readFactor ||
		carry.Input != uint64(program.CarryInput()) || carry.Factor != outputFactor || carry.Transform.Available() {
		return false
	}
	return cell.state.phase == schemaBindingOpen || cell.state.phase == schemaBindingSealed
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
	return directRuleWriteExact
}

func (cell *generatedRuleBindingCell) directRuleRouteRead() uint64 { return 0 }

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
	cell := &generatedRuleBindingCell{state: state, schema: state.schema, ordinal: ordinal, generated: generated}
	state.rules[ordinal] = cell
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
		axes := []uint32{
			descriptor.CandidateRelation().Axis,
			descriptor.Reducer().Axis,
			descriptor.OutputAddress().Axis,
			descriptor.DestinationProjection().Axis,
			descriptor.OutputAxis(),
		}
		if descriptor.ReadCount() != 0 {
			axes = append(axes, descriptor.JoinRelation().Axis, descriptor.KeyProjection().Axis, descriptor.ReadAxis())
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
