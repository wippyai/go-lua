package body

import (
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

const globalTableName = "_G"

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
	seeds = append(seeds, ambientModuleGlobalEntrySeeds(reg, typeValues, bindings, moduleExports, globalTypes)...)
	seeds = append(seeds, configuredGlobalEntrySeeds(reg, typeValues, bindings, globals, globalTypes)...)
	return seeds
}

func appendBoundaryGlobalContractEntrySeeds(seeds []state.ValueSeed, plan *operationplan.Plan) []state.ValueSeed {
	if plan == nil || !plan.BoundaryGlobalsValid() {
		return seeds
	}
	globals := plan.BoundaryGlobals()
	contracts := plan.BoundaryGlobalContracts()
	if len(globals) != len(contracts) {
		return seeds
	}
	for index, global := range globals {
		slot := key.SymbolValue(global)
		if slot == 0 {
			continue
		}
		present := false
		for _, seed := range seeds {
			if seed.Slot == slot {
				present = true
				break
			}
		}
		if !present {
			seeds = append(seeds, state.ValueSeed{Slot: slot, Value: contracts[index]})
		}
	}
	return seeds
}

func applyMethodReceiverEntrySeed(reg *axis.Registry, values *typevalue.Cache, bindings *bind.Result, fn *ast.FunctionExpr, receivers map[symbol.ID]typ.Type, seeds []state.ValueSeed) []state.ValueSeed {
	if reg == nil || bindings == nil || fn == nil || len(receivers) == 0 {
		return seeds
	}
	origin, ok := bindings.FunctionOrigin(fn)
	if !ok || origin.Kind != bind.FunctionOriginMethod {
		return seeds
	}
	receiver, ok := bindings.MethodOriginReceiverSymbol(origin)
	if !ok {
		return seeds
	}
	t := receivers[receiver]
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) {
		return seeds
	}
	if values == nil {
		values = typevalue.NewCache()
	}
	value := values.FromTypeWithWitness(reg, t)
	for _, slot := range bindings.ParamSlots(fn) {
		if !slot.ImplicitSelf || slot.Symbol == 0 {
			continue
		}
		valueSlot := key.SymbolValue(slot.Symbol)
		for index := range seeds {
			if seeds[index].Slot == valueSlot {
				seeds[index].Value = value
				return seeds
			}
		}
		return append(seeds, state.ValueSeed{Slot: valueSlot, Value: value})
	}
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
		value, ok := functionParamContractValue(reg, typeValues, bindings, fn, resolver, slot, expectedSig, hasExpectedSig, true)
		if !ok {
			continue
		}
		seeds = append(seeds, state.ValueSeed{
			Slot:  valueSlot,
			Value: value,
		})
	}
	return seeds
}

// functionParamContractValue is the sole declared/contextual parameter law
// shared by entry seeding and symbolic definition frames. In particular,
// method self occupies a real runtime slot even though it is absent from the
// AST's explicit parameter list.
func functionParamContractValue(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	bindings *bind.Result,
	fn *ast.FunctionExpr,
	resolver *typeresolve.Resolver,
	slot bind.ParamSlot,
	expectedSig *typ.Function,
	hasExpectedSig bool,
	gradualUntyped bool,
) (product.Value, bool) {
	if reg == nil || bindings == nil || fn == nil || resolver == nil {
		return product.Value{}, false
	}
	if slot.Type != nil {
		t, ok := resolver.Type(slot.Type)
		if !ok {
			return product.Value{}, false
		}
		return typeValues.FromTypeWithWitness(reg, t), true
	}
	if slot.Name == "self" {
		if t, ok := methodReceiverType(bindings, resolver, fn); ok {
			return typeValues.FromTypeWithWitness(reg, t), true
		}
	}
	if hasExpectedSig && !slot.ImplicitSelf {
		if t, ok := contextualParamType(expectedSig, slot.SourceIndex); ok {
			return typeValues.FromTypeWithWitness(reg, t), true
		}
	}
	if slot.ImplicitSelf {
		return product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), evidence.Key, evidence.GradualTop()), true
	}
	if !gradualUntyped {
		return product.Top(), true
	}
	return product.Set(reg, product.Top(), evidence.Key, evidence.GradualTop()), true
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
	presentGlobalTop := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), evidence.Key, evidence.GradualTop())
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
		if name == globalTableName {
			value = presentGlobalTop
		}
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

func ambientModuleGlobalEntrySeeds(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	bindings *bind.Result,
	exports importlookup.Source,
	globalTypes map[string]typ.Type,
) []state.ValueSeed {
	if reg == nil || bindings == nil || len(exports.Manifests) == 0 {
		return nil
	}
	seeds := make([]state.ValueSeed, 0, len(exports.Manifests))
	seen := make(map[string]struct{}, len(exports.Manifests))
	for i := len(exports.Manifests) - 1; i >= 0; i-- {
		m := exports.Manifests[i]
		if m == nil || m.Path == "" || m.Export == nil {
			continue
		}
		if _, exists := seen[m.Path]; exists {
			continue
		}
		seen[m.Path] = struct{}{}
		if globalTypes[m.Path] != nil {
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
		exportType, ok := exports.LookupExport(m.Path)
		if !ok || exportType == nil {
			continue
		}
		exportValue := typeValues.FromTypeWithWitness(reg, exportType)
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
