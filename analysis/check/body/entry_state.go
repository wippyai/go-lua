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
	typeValues *typevalue.Cache,
	graph cfg.Graph,
	bindings *bind.Result,
	fn *ast.FunctionExpr,
	moduleExports importlookup.Source,
	typeResolver *typeresolve.Resolver,
	entry state.State,
	initial transfer.InitialState,
) (state.State, transfer.InitialState) {
	seeds := functionParamEntrySeeds(reg, typeValues, bindings, fn, typeResolver)
	seeds = append(seeds, ambientModuleGlobalEntrySeeds(reg, typeValues, bindings, moduleExports)...)
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

func functionParamEntrySeeds(reg *axis.Registry, typeValues *typevalue.Cache, bindings *bind.Result, fn *ast.FunctionExpr, resolver *typeresolve.Resolver) []paramEntrySeed {
	if reg == nil || bindings == nil || fn == nil {
		return nil
	}
	if resolver == nil {
		resolver = typeresolve.New(bindings)
	}
	slots := bindings.ParamSlots(fn)
	expectedSig, hasExpectedSig := expectedFunctionSignature(bindings, resolver, fn)
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
			if slot.Name == "self" {
				if t, ok := methodReceiverType(bindings, resolver, fn); ok {
					seeds = append(seeds, paramEntrySeed{
						slot:  valueSlot,
						value: typevalue.WithWitness(reg, typevalue.FromTypeCached(typeValues, reg, t), t),
					})
					continue
				}
			}
			if hasExpectedSig && !slot.ImplicitSelf {
				if t, ok := contextualParamType(expectedSig, slot.SourceIndex); ok {
					seeds = append(seeds, paramEntrySeed{
						slot:  valueSlot,
						value: typevalue.WithWitness(reg, typevalue.FromTypeCached(typeValues, reg, t), t),
					})
					continue
				}
			}
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
			value: typevalue.WithWitness(reg, typevalue.FromTypeCached(typeValues, reg, t), t),
		})
	}
	return seeds
}

func ambientModuleGlobalEntrySeeds(reg *axis.Registry, typeValues *typevalue.Cache, bindings *bind.Result, exports importlookup.Source) []paramEntrySeed {
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
		exportValue := typevalue.WithWitness(reg, typevalue.FromTypeCached(typeValues, reg, m.Export), m.Export)
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
