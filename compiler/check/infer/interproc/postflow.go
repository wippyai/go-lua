package interproc

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	checkcallsite "github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/infer/paramhints"
	"github.com/wippyai/go-lua/compiler/check/nested"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/typ"
	typjoin "github.com/wippyai/go-lua/types/typ/join"
	"github.com/wippyai/go-lua/types/typ/unwrap"
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
		fnTypeForFacts := fnType
		if summary := returns.NormalizeReturnVector(facts.ReturnSummaries[sym]); len(summary) > 0 {
			normalizedSummary := make([]typ.Type, len(summary))
			hasInformativeSummary := false
			for i, ret := range summary {
				normalizedSummary[i] = typ.PruneSoftUnionMembers(ret)
				if paramhints.IsInformativeHintType(normalizedSummary[i]) {
					hasInformativeSummary = true
				}
			}
			shouldUseSummary := hasInformativeSummary ||
				len(fnTypeForFacts.Returns) == 0 ||
				(returns.ReturnTypesAllNil(fnTypeForFacts.Returns) && !returns.ReturnTypesAllNil(normalizedSummary)) ||
				returns.ReturnTypesRefine(normalizedSummary, fnTypeForFacts.Returns) ||
				returns.ReturnTypesElideOptional(normalizedSummary, fnTypeForFacts.Returns) ||
				returns.ReturnTypesExtendRecord(normalizedSummary, fnTypeForFacts.Returns)
			if shouldUseSummary {
				fnTypeForFacts = typjoin.WithReturns(fnTypeForFacts, normalizedSummary)
			}
		}
		facts.FuncTypes[sym] = fnTypeForFacts
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

	moduleBindings := store.ModuleBindings()
	bindings := graph.Bindings()
	if bindings == nil {
		bindings = moduleBindings
	}

	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil || len(info.Args) == 0 {
			return
		}
		argTypes := make([]typ.Type, len(info.Args))
		for i, arg := range info.Args {
			if arg != nil {
				argTypes[i] = result.NarrowSynth.TypeOf(arg, p)
			}
		}
		def := ops.CallDef{
			Args:  argTypes,
			Query: result.NarrowSynth.CallQuery(),
		}
		if checkcallsite.IsMethodCallInfo(info) {
			def.IsMethod = true
			def.MethodName = info.Method
			def.Receiver = result.NarrowSynth.TypeOf(info.Receiver, p)
		} else if info.Callee != nil {
			def.Callee = result.NarrowSynth.TypeOf(info.Callee, p)
		}
		infer := ops.InferCall(result.NarrowSynth.Context(), def)
		if len(info.Args) > 0 {
			updated := make([]typ.Type, len(argTypes))
			copy(updated, argTypes)
			changed := false
			for i, arg := range info.Args {
				if arg == nil {
					continue
				}
				expected := infer.ExpectedArgType(i)
				if expected == nil {
					continue
				}
				reSynthed := result.NarrowSynth.TypeOfWithExpected(arg, p, expected)
				if reSynthed == nil {
					continue
				}
				if !typ.TypeEquals(updated[i], reSynthed) {
					updated[i] = reSynthed
					changed = true
				}
			}
			if changed {
				def.Args = updated
				infer = ops.ReInfer(result.NarrowSynth.Context(), def, infer)
				argTypes = updated
			}
		}

		hasFunctionRef := func(sym cfg.SymbolID) bool {
			return sym != 0 && store.FunctionRefBySym(sym) != nil
		}
		calleeSym := checkcallsite.CanonicalSymbolFromExpr(
			info.Callee,
			info.CalleeSymbol,
			bindings,
			moduleBindings,
			hasFunctionRef,
		)
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
			hints := paramhints.EnsureHintCapacity(facts.ParamHints[calleeSym], len(info.Args))
			for i, arg := range info.Args {
				if arg == nil {
					continue
				}
				if expectedFn := unwrap.Function(infer.ExpectedArgType(i)); expectedFn != nil {
					argSym := checkcallsite.CanonicalSymbolFromExpr(
						arg,
						0,
						bindings,
						moduleBindings,
						hasFunctionRef,
					)
					if argSym != 0 && hasFunctionRef(argSym) {
						hintsForFn := facts.ParamHints[argSym]
						for j, param := range expectedFn.Params {
							hintsForFn, _ = paramhints.MergeHintAt(hintsForFn, j, param.Type, returns.JoinInterprocTypes)
						}
						if len(hintsForFn) > 0 {
							facts.ParamHints[argSym] = hintsForFn
						}
					}
				}

				argType := argTypes[i]
				if argType == nil {
					argType = result.NarrowSynth.TypeOf(arg, p)
				}
				hints, _ = paramhints.MergeHintAt(hints, i, argType, returns.JoinInterprocTypes)
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
	return returns.MergeCapturedContainerMutationMaps(existing, next, func(prev *api.ContainerMutation, next api.ContainerMutation) api.ContainerMutation {
		if prev != nil {
			next.ValueType = returns.JoinInterprocTypes(prev.ValueType, next.ValueType)
		}
		return next
	})
}
