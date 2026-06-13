package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
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
	var reassigned map[key.Value]struct{}
	if reassignedReader, ok := result.(reassignedParameterValueSlotReader); ok {
		reassigned = reassignedReader.ReassignedParameterValueSlots()
	}
	out := make([]product.Value, len(slots))
	for i := range out {
		out[i] = product.Top()
	}
	for i, slot := range slots {
		if slot == "" {
			continue
		}
		if _, ok := reassigned[slot]; ok {
			continue
		}
		value, ok := normalReturnParamConstraint(reg, entry.ReadValue(reg, slot), exit.ReadValue(reg, slot))
		if !ok {
			continue
		}
		out[i] = value
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
	if !product.LessOrEq(reg, exit, entry) {
		return product.Value{}, false
	}
	return exit, true
}

func normalReturnParamPaths(result ResultReader) []path.Path {
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
