package engine

import "github.com/wippyai/go-lua/analysis/engine/internal/composition"

// BeginSelectedExactRuleBinding starts the no-carry heterogeneous selected
// read transaction for one exact-write Rule. It shares the one-shot read and
// commit authority with the routed/carry transaction, but requires the cold
// Rule to prove CarryCount=0. There is no optional-carry mode: the two shapes
// have disjoint constructors and validation laws.
func BeginSelectedExactRuleBinding[K ~uint32 | ~uint64, V, O any](binding *SchemaBinding, slot *RuleSlot[V, O], write SchemaWriteSlot[V], output FactorRef[V], spec HotRuleSpec[V, O]) (*SelectedRouteRuleBindingTransaction[K, V, O], bool) {
	state := bindingState(binding)
	if state == nil {
		return nil, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || slot == nil || slot.cell == nil || slot.cell.schema != state.schema || write.cell == nil || write.cell.schema != state.schema || output.cell == nil || output.cell.schema != state.schema || spec.OperandContent == nil || !spec.Admission.valid() || spec.Transfer == nil {
		state.poisonLocked()
		return nil, false
	}
	ruleOrdinal, ruleOK := slot.Ordinal()
	if !ruleOK || ruleOrdinal >= uint64(len(state.rules)) || state.rules[ruleOrdinal] != nil || state.pendingRules[ruleOrdinal] != nil || write.cell.ordinal>>32 != ruleOrdinal || uint64(uint32(write.cell.ordinal)) != 0 {
		state.poisonLocked()
		return nil, false
	}
	shape, shapeOK := state.schema.ruleShapeAt(ruleOrdinal)
	writeShape, writeOK := state.schema.ruleWriteShapeAt(ruleOrdinal, 0)
	if !shapeOK || !writeOK || shape.OutputKind != composition.FactorOutput || shape.Inputs == 0 || shape.CarryCount != 0 || shape.WriteCount != 1 || writeShape.Factor != shape.Output || writeShape.Kind != composition.WriteExact || writeShape.Route != 0 || writeShape.CandidateCount != 0 || writeShape.DependencyCount != 0 {
		state.poisonLocked()
		return nil, false
	}
	coldAdmission, admissionOK := coldRuleAdmission(shape.Admission)
	if !admissionOK || spec.Admission.kind != coldAdmission.kind || spec.Admission.identity != coldAdmission.identity {
		state.poisonLocked()
		return nil, false
	}
	outputOrdinal, outputOK := factorRefOrdinal(output, state.schema)
	if !outputOK || outputOrdinal >= uint64(len(state.factors)) || state.schema.factorSemanticAt(outputOrdinal) != shape.Output {
		state.poisonLocked()
		return nil, false
	}
	outputCell, outputOK := state.factors[outputOrdinal].(*schemaFactorBindingCell[K, V])
	if !outputOK || outputCell == nil || outputCell.schema != state.schema || outputCell.impl == nil || outputCell.impl.algebra == nil {
		state.poisonLocked()
		return nil, false
	}
	cell := &schemaRuleBindingCellImpl[K, V, O]{state: state, schema: state.schema, ordinal: ruleOrdinal}
	token := &schemaRuleBindingToken{}
	if state.pendingRules == nil {
		state.pendingRules = make(map[uint64]*schemaRuleBindingToken)
	}
	state.pendingRules[ruleOrdinal] = token
	return &SelectedRouteRuleBindingTransaction[K, V, O]{shared: &selectedRouteTxnState[K, V, O]{
		state: state, cell: cell, output: outputCell, ordinal: ruleOrdinal, rule: slot, write: write,
		operand: spec.OperandContent, admission: spec.Admission, transfer: spec.Transfer, token: token,
	}}, true
}
