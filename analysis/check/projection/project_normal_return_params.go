package projection

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func projectNormalReturnParams(reg *axis.Registry, result ResultReader, exit state.State) []product.Value {
	entryReader, ok := result.(entryStateReader)
	if !ok {
		return nil
	}
	slotReader, ok := result.(parameterValueSlotReader)
	if !ok {
		return nil
	}
	entry, ok := entryReader.EntryState()
	if !ok {
		return nil
	}
	slots := slotReader.ParameterValueSlots()
	if len(slots) == 0 {
		return nil
	}
	multiPathNormalReturn := normalReturnHasMultiPathChoice(reg, result)
	var reassigned map[key.Value]struct{}
	if reassignedReader, ok := result.(reassignedParameterValueSlotReader); ok {
		reassigned = reassignedReader.ReassignedParameterValueSlots()
	}
	out := make([]product.Value, len(slots))
	for i := range out {
		out[i] = product.Top()
	}
	for i, slot := range slots {
		if slot == 0 {
			continue
		}
		if _, ok := reassigned[slot]; ok {
			continue
		}
		if multiPathNormalReturn {
			continue
		}
		value, ok := normalReturnParamConstraint(reg, entry.ReadValue(reg, slot), exit.ReadValue(reg, slot))
		if !ok {
			continue
		}
		out[i] = portableBoundaryValue(reg, value)
	}
	return out
}

func normalReturnParamConstraint(reg *axis.Registry, entry, exit product.Value) (product.Value, bool) {
	if product.Equal(reg, exit, product.Bottom(reg)) || product.Equal(reg, exit, product.Top()) {
		return product.Value{}, false
	}
	if product.Equal(reg, exit, entry) {
		return product.Value{}, false
	}
	if !normalReturnParamRefinesEntry(reg, entry, exit) {
		return product.Value{}, false
	}
	return exit, true
}

func normalReturnParamRefinesEntry(reg *axis.Registry, entry, exit product.Value) bool {
	if product.LessOrEq(reg, exit, entry) {
		return true
	}
	if !product.Get(reg, exit, assertion.Key).Has(assertion.RuntimeClaim) {
		return false
	}
	exitKind := product.Get(reg, exit, runtimekind.Key)
	if exitKind.IsTop() || exitKind.IsBottom() {
		return false
	}
	entryKind := product.Get(reg, entry, runtimekind.Key)
	return runtimekind.LessOrEq(exitKind, entryKind)
}

func normalReturnHasMultiPathChoice(reg *axis.Registry, result ResultReader) bool {
	graph := result.Graph()
	if reg == nil || graph == nil {
		return false
	}
	reachability, ok := newNormalReturnReachability(reg, result, graph)
	if !ok {
		return false
	}
	for _, point := range cfg.RPOReadOnly(graph) {
		if !graph.IsBranch(point) {
			continue
		}
		var normalSuccessors int
		for _, succ := range cfg.SuccessorsReadOnly(graph, point) {
			if reachability.canCompleteNormally(succ) {
				normalSuccessors++
			}
		}
		if normalSuccessors > 1 {
			return true
		}
	}
	return false
}

func parameterValuePaths(result ResultReader) []path.Path {
	slotReader, ok := result.(parameterValueSlotReader)
	if !ok {
		return nil
	}
	slots := slotReader.ParameterValueSlots()
	if len(slots) == 0 {
		return nil
	}
	out := make([]path.Path, len(slots))
	for i, slot := range slots {
		sym, ok := key.ParseSymbolValue(slot)
		if !ok {
			continue
		}
		out[i] = path.NewPath(sym, "")
	}
	return out
}
