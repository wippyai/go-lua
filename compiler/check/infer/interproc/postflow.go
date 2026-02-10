package interproc

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/infer/paramhints"
	"github.com/wippyai/go-lua/compiler/check/nested"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

// Store is the minimal store interface required to record post-flow interproc facts.
type Store interface {
	api.StoreView

	UpdateInterprocFactsNext(key api.GraphKey, update func(*api.Facts))
	StoreLiteralSigs(graphID uint64, sigs map[*ast.FunctionExpr]*typ.Function)
	ParentGraphKeyForSymbol(sym cfg.SymbolID) (api.GraphKey, bool)
}

// StoreFactsFromResult records post-flow interproc facts for the current iteration.
// Facts are written into InterprocFactsNext and become visible after FixpointSwap.
func StoreFactsFromResult(
	store Store,
	fn *ast.FunctionExpr,
	result *api.FuncResult,
	parent *scope.State,
) {
	if store == nil || result == nil || result.Graph == nil {
		return
	}
	// Record literal signatures for this graph.
	if len(result.LiteralSignatures) > 0 {
		store.StoreLiteralSigs(result.Graph.ID(), result.LiteralSignatures)
		if key, ok := store.GraphKeyFor(result.Graph, parent); ok {
			store.UpdateInterprocFactsNext(key, func(facts *api.Facts) {
				if facts.LiteralSigs == nil {
					facts.LiteralSigs = make(api.LiteralSigs, len(result.LiteralSignatures))
				}
				for fnExpr, sig := range result.LiteralSignatures {
					if fnExpr != nil && sig != nil {
						facts.LiteralSigs[fnExpr] = sig
					}
				}
			})
		}
	}

	if result.NarrowSynth == nil {
		return
	}
	// Collect parameter hints regardless of whether the function has a symbol.
	CollectParamHintsFromResult(store, result)

	// Collect captured field assignments for nested functions.
	if fn != nil {
		bindings := result.Graph.Bindings()
		if bindings == nil {
			bindings = store.ModuleBindings()
		}
		if bindings != nil {
			capturedList := bindings.CapturedSymbols(fn)
			if len(capturedList) > 0 {
				capturedSet := make(map[cfg.SymbolID]bool, len(capturedList))
				for _, sym := range capturedList {
					if sym != 0 {
						capturedSet[sym] = true
					}
				}
				if len(capturedSet) > 0 {
					fields := nested.CollectCapturedFieldAssignments(result.Graph, capturedSet, result.NarrowSynth.TypeOf)
					if len(fields) > 0 {
						if sym, ok := store.SymbolForFunc(fn); ok && sym != 0 {
							if parentKey, ok := store.ParentGraphKeyForSymbol(sym); ok {
								store.UpdateInterprocFactsNext(parentKey, func(facts *api.Facts) {
									if facts.CapturedFields == nil {
										facts.CapturedFields = make(api.CapturedFieldAssigns)
									}
									existing := facts.CapturedFields[sym]
									facts.CapturedFields[sym] = mergeCapturedFieldAssigns(existing, fields)
								})
							}
						}
					}

					mutations := nested.CollectCapturedContainerMutations(result.Graph, capturedSet, result.NarrowSynth.TypeOf)
					if len(mutations) > 0 {
						if sym, ok := store.SymbolForFunc(fn); ok && sym != 0 {
							if parentKey, ok := store.ParentGraphKeyForSymbol(sym); ok {
								store.UpdateInterprocFactsNext(parentKey, func(facts *api.Facts) {
									if facts.CapturedContainers == nil {
										facts.CapturedContainers = make(api.CapturedContainerMutations)
									}
									existing := facts.CapturedContainers[sym]
									facts.CapturedContainers[sym] = mergeCapturedContainerMutations(existing, mutations)
								})
							}
						}
					}
				}
			}
		}
	}

	if fn == nil {
		return
	}
	sym, ok := store.SymbolForFunc(fn)
	if !ok || sym == 0 {
		return
	}

	fnType := result.NarrowSynth.FunctionType(fn, result.BaseScope)
	if fnType == nil {
		return
	}
	narrowReturns := returns.NormalizeReturnVector(fnType.Returns)

	parentKey, ok := store.ParentGraphKeyForSymbol(sym)
	if !ok {
		return
	}
	store.UpdateInterprocFactsNext(parentKey, func(facts *api.Facts) {
		if facts.FuncTypes == nil {
			facts.FuncTypes = make(api.FuncTypes, 1)
		}
		if facts.NarrowReturns == nil {
			facts.NarrowReturns = make(api.NarrowReturnSummaries, 1)
		}
		facts.FuncTypes[sym] = fnType
		facts.NarrowReturns[sym] = narrowReturns
	})
}

// CollectParamHintsFromResult records parameter hints based on call sites
// within the current function's graph using narrowed expression types.
func CollectParamHintsFromResult(store Store, result *api.FuncResult) {
	if store == nil || result == nil || result.Graph == nil || result.NarrowSynth == nil {
		return
	}
	graph := result.Graph

	bindings := graph.Bindings()
	if bindings == nil {
		bindings = store.ModuleBindings()
	}

	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil || len(info.Args) == 0 {
			return
		}
		calleeSym := info.CalleeSymbol
		if calleeSym == 0 {
			if ident, ok := info.Callee.(*ast.IdentExpr); ok && bindings != nil {
				if sym, found := bindings.SymbolOf(ident); found {
					calleeSym = sym
				}
			}
		}
		if calleeSym == 0 {
			return
		}
		ref := store.FunctionRefBySym(calleeSym)
		if ref == nil {
			return
		}
		parentGraphID := ref.ParentGraphID
		if parentGraphID == 0 {
			parentGraphID = ref.GraphID
		}
		if parentGraphID == 0 {
			return
		}
		parentGraph := store.Graphs()[parentGraphID]
		if parentGraph == nil {
			return
		}
		parentHash := store.GraphParentHashOf(parentGraphID)
		if parentHash == 0 {
			return
		}
		key := api.KeyForGraph(parentGraph, parentHash)

		store.UpdateInterprocFactsNext(key, func(facts *api.Facts) {
			if facts.ParamHints == nil {
				facts.ParamHints = make(api.ParamHints)
			}
			hints := facts.ParamHints[calleeSym]
			if len(hints) < len(info.Args) {
				expanded := make([]typ.Type, len(info.Args))
				copy(expanded, hints)
				hints = expanded
			}
			for i, arg := range info.Args {
				if arg == nil {
					continue
				}
				argType := result.NarrowSynth.TypeOf(arg, p)
				argType = paramhints.WidenParamHintType(argType)
				argType = typ.PruneSoftUnionMembers(argType)
				if !paramhints.IsInformativeHintType(argType) {
					continue
				}
				prev := hints[i]
				joined := returns.JoinInterprocTypes(prev, argType)
				if !typ.TypeEquals(prev, joined) {
					hints[i] = joined
				}
			}
			if len(hints) > 0 {
				facts.ParamHints[calleeSym] = hints
			}
		})
	})
}

func mergeCapturedFieldAssigns(
	existing map[cfg.SymbolID]map[string]typ.Type,
	next map[cfg.SymbolID]map[string]typ.Type,
) map[cfg.SymbolID]map[string]typ.Type {
	if existing == nil {
		return next
	}
	if next == nil {
		return existing
	}
	merged := make(map[cfg.SymbolID]map[string]typ.Type, len(existing)+len(next))
	for _, sym := range cfg.SortedSymbolIDs(existing) {
		merged[sym] = existing[sym]
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		fields := next[sym]
		existingFields := merged[sym]
		if existingFields == nil {
			merged[sym] = fields
			continue
		}
		out := make(map[string]typ.Type, len(existingFields)+len(fields))
		for _, name := range cfg.SortedFieldNames(existingFields) {
			out[name] = existingFields[name]
		}
		for _, name := range cfg.SortedFieldNames(fields) {
			t := fields[name]
			if prev := out[name]; prev != nil {
				out[name] = returns.JoinInterprocTypes(prev, t)
			} else {
				out[name] = t
			}
		}
		merged[sym] = out
	}
	return merged
}

func mergeCapturedContainerMutations(
	existing map[cfg.SymbolID][]api.ContainerMutation,
	next map[cfg.SymbolID][]api.ContainerMutation,
) map[cfg.SymbolID][]api.ContainerMutation {
	if existing == nil {
		return next
	}
	if next == nil {
		return existing
	}
	merged := make(map[cfg.SymbolID][]api.ContainerMutation, len(existing)+len(next))
	for _, sym := range cfg.SortedSymbolIDs(existing) {
		merged[sym] = existing[sym]
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		merged[sym] = mergeContainerMutationSlice(merged[sym], next[sym])
	}
	return merged
}

func mergeContainerMutationSlice(
	existing []api.ContainerMutation,
	next []api.ContainerMutation,
) []api.ContainerMutation {
	if len(existing) == 0 {
		return next
	}
	if len(next) == 0 {
		return existing
	}
	byKey := make(map[string]api.ContainerMutation, len(existing)+len(next))
	for _, m := range existing {
		byKey[containerMutationKey(m)] = m
	}
	for _, m := range next {
		key := containerMutationKey(m)
		if prev, ok := byKey[key]; ok {
			m.ValueType = returns.JoinInterprocTypes(prev.ValueType, m.ValueType)
		}
		byKey[key] = m
	}
	keys := make([]string, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]api.ContainerMutation, 0, len(byKey))
	for _, key := range keys {
		out = append(out, byKey[key])
	}
	return out
}

func containerMutationKey(m api.ContainerMutation) string {
	return constraint.FormatSegments(m.Segments)
}
