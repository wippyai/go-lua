package body

import (
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func parameterEntryState(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	graph cfg.Graph,
	bindings *bind.Result,
	fn *ast.FunctionExpr,
	globals []string,
	globalTypes map[string]typ.Type,
	moduleExports importlookup.Source,
	typeResolver *typeresolve.Resolver,
	entry state.State,
	initial transfer.InitialState,
) (state.State, transfer.InitialState) {
	seeds := entrySeedPlan(reg, typeValues, bindings, fn, globals, globalTypes, moduleExports, typeResolver)
	return applyEntrySeedPlan(reg, graph, seeds, entry, initial)
}

func entrySeedPlan(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	bindings *bind.Result,
	fn *ast.FunctionExpr,
	globals []string,
	globalTypes map[string]typ.Type,
	moduleExports importlookup.Source,
	typeResolver *typeresolve.Resolver,
) []state.ValueSeed {
	seeds := functionParamEntrySeeds(reg, typeValues, bindings, fn, typeResolver)
	seeds = append(seeds, ambientModuleGlobalEntrySeeds(reg, typeValues, bindings, moduleExports)...)
	seeds = append(seeds, configuredGlobalEntrySeeds(reg, typeValues, bindings, globals, globalTypes)...)
	return seeds
}

func applyEntrySeedPlan(
	reg *axis.Registry,
	graph cfg.Graph,
	seeds []state.ValueSeed,
	entry state.State,
	initial transfer.InitialState,
) (state.State, transfer.InitialState) {
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

func functionParamEntrySeeds(reg *axis.Registry, typeValues *typevalue.Cache, bindings *bind.Result, fn *ast.FunctionExpr, resolver *typeresolve.Resolver) []state.ValueSeed {
	if reg == nil || bindings == nil || fn == nil {
		return nil
	}
	if resolver == nil {
		resolver = typeresolve.New(bindings)
	}
	slots := bindings.ParamSlots(fn)
	expectedSig, hasExpectedSig := expectedFunctionSignature(bindings, resolver, fn)
	seeds := make([]state.ValueSeed, 0, len(slots))
	for _, slot := range slots {
		if slot.Symbol == 0 {
			continue
		}
		valueSlot := key.SymbolValue(slot.Symbol)
		if valueSlot == 0 {
			continue
		}
		if slot.Type == nil {
			if slot.Name == "self" {
				if t, ok := methodReceiverType(bindings, resolver, fn); ok {
					seeds = append(seeds, state.ValueSeed{
						Slot:  valueSlot,
						Value: typeValues.FromTypeWithWitness(reg, t),
					})
					continue
				}
			}
			if hasExpectedSig && !slot.ImplicitSelf {
				if t, ok := contextualParamType(expectedSig, slot.SourceIndex); ok {
					seeds = append(seeds, state.ValueSeed{
						Slot:  valueSlot,
						Value: typeValues.FromTypeWithWitness(reg, t),
					})
					continue
				}
			}
			if slot.ImplicitSelf {
				seeds = append(seeds, state.ValueSeed{
					Slot:  valueSlot,
					Value: product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), evidence.Key, evidence.GradualTop()),
				})
				continue
			}
			seeds = append(seeds, state.ValueSeed{
				Slot:  valueSlot,
				Value: product.Set(reg, product.Top(), evidence.Key, evidence.GradualTop()),
			})
			continue
		}
		t, ok := resolver.Type(slot.Type)
		if !ok {
			continue
		}
		seeds = append(seeds, state.ValueSeed{
			Slot:  valueSlot,
			Value: typeValues.FromTypeWithWitness(reg, t),
		})
	}
	return seeds
}

func configuredGlobalEntrySeeds(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	bindings *bind.Result,
	globals []string,
	globalTypes map[string]typ.Type,
) []state.ValueSeed {
	if reg == nil || bindings == nil || (len(globals) == 0 && len(globalTypes) == 0) {
		return nil
	}
	globalTop := product.Set(reg, product.Top(), evidence.Key, evidence.GradualTop())
	var seeds []state.ValueSeed
	appendSeed := func(name string) {
		id, ok := bindings.GlobalSymbol(name)
		if !ok || id == 0 || bindings.IsImplicitGlobalSymbol(id) {
			return
		}
		valueSlot := key.SymbolValue(id)
		if valueSlot == 0 {
			return
		}
		for _, seed := range seeds {
			if seed.Slot == valueSlot {
				return
			}
		}
		value := globalTop
		if t := globalTypes[name]; t != nil && typeValues != nil {
			value = typeValues.FromTypeWithWitness(reg, t)
		}
		seeds = append(seeds, state.ValueSeed{Slot: valueSlot, Value: value})
	}
	for _, name := range globals {
		appendSeed(name)
	}
	for name := range globalTypes {
		appendSeed(name)
	}
	return seeds
}

func ambientModuleGlobalEntrySeeds(reg *axis.Registry, typeValues *typevalue.Cache, bindings *bind.Result, exports importlookup.Source) []state.ValueSeed {
	if reg == nil || bindings == nil || len(exports.Manifests) == 0 {
		return nil
	}
	seeds := make([]state.ValueSeed, 0, len(exports.Manifests))
	for _, m := range exports.Manifests {
		if m == nil || m.Path == "" || m.Export == nil {
			continue
		}
		id, ok := bindings.GlobalSymbol(m.Path)
		if !ok || id == 0 || bindings.IsImplicitGlobalSymbol(id) {
			continue
		}
		valueSlot := key.SymbolValue(id)
		if valueSlot == 0 {
			continue
		}
		exportValue := typeValues.FromTypeWithWitness(reg, m.Export)
		seeds = append(seeds, state.ValueSeed{Slot: valueSlot, Value: exportValue})
	}
	return seeds
}

func seedEntryStateValues(reg *axis.Registry, entry state.State, seeds []state.ValueSeed) state.State {
	if reg == nil || len(seeds) == 0 {
		return entry
	}
	return entry.SeedValues(reg, seeds)
}
