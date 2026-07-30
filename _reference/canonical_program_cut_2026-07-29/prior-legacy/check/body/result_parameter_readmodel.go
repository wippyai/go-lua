package body

import (
	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
)

func (r *Result) ParameterValueSlots() []statekey.Value {
	if r == nil || r.bindings == nil {
		return nil
	}
	if r.paramSlotsOK {
		return r.paramValueSlots
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
	r.paramValueSlots = out
	r.paramSlotsOK = true
	return r.paramValueSlots
}

func (r *Result) ReassignedParameterValueSlots() map[statekey.Value]struct{} {
	if r == nil || r.bindings == nil {
		return nil
	}
	if r.reassignedOK {
		return r.reassignedParams
	}
	params := make(map[statekey.Value]struct{})
	for _, slot := range r.ParameterValueSlots() {
		params[slot] = struct{}{}
	}
	if len(params) == 0 {
		r.reassignedOK = true
		return nil
	}
	out := make(map[statekey.Value]struct{})
	graph := r.Graph()
	if graph == nil {
		r.reassignedOK = true
		return nil
	}
	for _, point := range graph.RPO() {
		assignment, ok := r.OrdinaryAssignment(point)
		if !ok || !assignment.HasSymbol {
			continue
		}
		slot := statekey.SymbolValue(assignment.Symbol)
		if _, ok := params[slot]; ok {
			out[slot] = struct{}{}
		}
	}
	if len(out) == 0 {
		r.reassignedOK = true
		return nil
	}
	r.reassignedParams = out
	r.reassignedOK = true
	return r.reassignedParams
}
