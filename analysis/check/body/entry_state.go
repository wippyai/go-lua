package body

import (
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/compiler/ast"
)

type paramEntrySeed struct {
	slot  key.Value
	value product.Value
}

func parameterEntryState(
	reg *axis.Registry,
	graph cfg.Graph,
	bindings *bind.Result,
	fn *ast.FunctionExpr,
	moduleExports importlookup.Source,
	entry state.State,
	initial transfer.InitialState,
) (state.State, transfer.InitialState) {
	seeds := functionParamEntrySeeds(reg, bindings, fn)
	seeds = append(seeds, ambientModuleGlobalEntrySeeds(reg, bindings, moduleExports)...)
	if len(seeds) == 0 {
		return entry, initial
	}
	entry = seedEntryStateValues(reg, entry, seeds)
	if graph == nil || initial == nil {
		return entry, initial
	}
	entryPoint := graph.Entry()
	return entry, func(point cfg.Point) (state.State, bool) {
		st, ok := initial(point)
		if !ok {
			return state.State{}, false
		}
		if point == entryPoint {
			st = seedEntryStateValues(reg, st, seeds)
		}
		return st, true
	}
}

func functionParamEntrySeeds(reg *axis.Registry, bindings *bind.Result, fn *ast.FunctionExpr) []paramEntrySeed {
	if reg == nil || bindings == nil || fn == nil {
		return nil
	}
	resolver := typeresolve.New(bindings)
	slots := bindings.ParamSlots(fn)
	seeds := make([]paramEntrySeed, 0, len(slots))
	for _, slot := range slots {
		if slot.Symbol == 0 {
			continue
		}
		valueSlot := key.SymbolValue(slot.Symbol)
		if valueSlot == "" {
			continue
		}
		if slot.Type == nil {
			seeds = append(seeds, paramEntrySeed{
				slot:  valueSlot,
				value: product.Set(reg, product.Top(), evidence.Key, evidence.GradualTop()),
			})
			continue
		}
		t, ok := resolver.Type(slot.Type)
		if !ok {
			continue
		}
		seeds = append(seeds, paramEntrySeed{
			slot:  valueSlot,
			value: typevalue.FromType(reg, t),
		})
	}
	return seeds
}

func ambientModuleGlobalEntrySeeds(reg *axis.Registry, bindings *bind.Result, exports importlookup.Source) []paramEntrySeed {
	if reg == nil || bindings == nil || len(exports.Manifests) == 0 {
		return nil
	}
	seeds := make([]paramEntrySeed, 0, len(exports.Manifests))
	for _, m := range exports.Manifests {
		if m == nil || m.Path == "" || m.Export == nil {
			continue
		}
		id, ok := bindings.GlobalSymbol(m.Path)
		if !ok || id == 0 || bindings.IsImplicitGlobalSymbol(id) {
			continue
		}
		valueSlot := key.SymbolValue(id)
		if valueSlot == "" {
			continue
		}
		exportValue := typevalue.WithWitness(reg, typevalue.FromType(reg, m.Export), m.Export)
		seeds = append(seeds, paramEntrySeed{slot: valueSlot, value: exportValue})
	}
	return seeds
}

func seedEntryStateValues(reg *axis.Registry, entry state.State, seeds []paramEntrySeed) state.State {
	if reg == nil || len(seeds) == 0 {
		return entry
	}
	bottom := product.Bottom(reg)
	out := entry
	for _, seed := range seeds {
		if seed.slot == "" {
			continue
		}
		if !product.Equal(reg, out.ReadValue(reg, seed.slot), bottom) {
			continue
		}
		out = out.WriteValue(reg, seed.slot, seed.value)
	}
	return out
}
