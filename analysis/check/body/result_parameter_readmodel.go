package body

import (
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
)

func (r *Result) ParameterValueSlots() []statekey.Value {
	if r == nil || r.bindings == nil {
		return nil
	}
	slots := r.bindings.ParamSlots(r.Function())
	out := make([]statekey.Value, 0, len(slots))
	for _, slot := range slots {
		valueSlot := statekey.SymbolValue(slot.Symbol)
		if valueSlot == 0 {
			continue
		}
		out = append(out, valueSlot)
	}
	return out
}

func (r *Result) ReassignedParameterValueSlots() map[statekey.Value]struct{} {
	if r == nil || r.bindings == nil {
		return nil
	}
	params := make(map[statekey.Value]struct{})
	for _, slot := range r.ParameterValueSlots() {
		params[slot] = struct{}{}
	}
	if len(params) == 0 {
		return nil
	}
	out := make(map[statekey.Value]struct{})
	graph := r.Graph()
	if graph == nil {
		return nil
	}
	for _, point := range graph.RPO() {
		assignment, ok := r.facts.RootAssignment(point)
		if !ok || assignment.Kind() != factflow.RootAssignmentOrdinaryRootWrite {
			continue
		}
		slot := statekey.SymbolValue(assignment.TargetSymbol())
		if _, ok := params[slot]; ok {
			out[slot] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
