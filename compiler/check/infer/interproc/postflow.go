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
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

type functionTypeWithExpected interface {
	SynthFunctionTypeWithExpected(fn *ast.FunctionExpr, sc *scope.State, expected *typ.Function) *typ.Function
}

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
	CollectParamHintsFromResult(store, result, parent)

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
	if expected := expectedFunctionFromResult(result); expected != nil {
		if withExpected, ok := result.NarrowSynth.(functionTypeWithExpected); ok {
			if inferred := withExpected.SynthFunctionTypeWithExpected(fn, result.BaseScope, expected); inferred != nil {
				fnType = inferred
			}
		}
	}
	if fnType == nil {
		return
	}
	narrowReturns := returns.NormalizeReturnVector(fnType.Returns)

	parentKey, ok := store.ParentGraphKeyForSymbol(sym)
	if !ok {
		return
	}
	var summaryFromSnapshot []typ.Type
	summaryScope := parent
	if summaryScope == nil && result.Graph != nil {
		if parentHash := store.GraphParentHashOf(result.Graph.ID()); parentHash != 0 {
			summaryScope = store.Parents()[parentHash]
		}
	}
	if result.Graph != nil && summaryScope != nil {
		if snap := store.GetReturnSummariesSnapshot(result.Graph, summaryScope); len(snap) > 0 {
			summaryFromSnapshot = snap[sym]
		}
	}
	store.UpdateInterprocFactsNext(parentKey, func(facts *api.Facts) {
		candidateFunc := fnType
		if hinted := paramhints.MergeIntoSignature(fn, facts.ParamHints[sym], unwrap.Function(candidateFunc)); hinted != nil {
			candidateFunc = hinted
		}
		reconciled := returns.ReconcileFunctionFact(returns.ReconcileFunctionFactInput{
			ExistingSummary:  facts.ReturnSummaries[sym],
			ExistingNarrow:   facts.NarrowReturns[sym],
			ExistingFunc:     facts.FuncTypes[sym],
			CandidateSummary: summaryFromSnapshot,
			CandidateNarrow:  narrowReturns,
			CandidateFunc:    candidateFunc,
		})
		if len(reconciled.Summary) > 0 {
			if facts.ReturnSummaries == nil {
				facts.ReturnSummaries = make(api.ReturnSummaries, 1)
			}
			facts.ReturnSummaries[sym] = reconciled.Summary
		}
		if len(reconciled.Narrow) > 0 {
			if facts.NarrowReturns == nil {
				facts.NarrowReturns = make(api.NarrowReturnSummaries, 1)
			}
			facts.NarrowReturns[sym] = reconciled.Narrow
		}
		if reconciled.Func != nil {
			if facts.FuncTypes == nil {
				facts.FuncTypes = make(api.FuncTypes, 1)
			}
			facts.FuncTypes[sym] = reconciled.Func
		}
	})
}

func expectedFunctionFromResult(result *api.FuncResult) *typ.Function {
	if result == nil || result.Graph == nil || result.FlowInputs == nil {
		return nil
	}
	slots := result.Graph.ParamSlots()
	if len(slots) == 0 {
		return nil
	}
	declared := result.FlowInputs.DeclaredTypes
	builder := typ.Func()
	hasUntypedSourceParam := false
	sourceFn := result.Graph.Func()
	for _, slot := range slots {
		name := slot.Name
		if name == "" {
			name = result.Graph.NameOf(slot.Symbol)
		}
		paramType := typ.Unknown
		if slot.Symbol != 0 && declared != nil {
			if t := declared[slot.Symbol]; t != nil {
				paramType = t
			}
		}
		if !slot.HasSourceParam() {
			builder = builder.Param(name, paramType)
			continue
		}

		optional := false
		if slot.TypeAnnotation == nil {
			optional = true
			hasUntypedSourceParam = true
		}
		if _, ok := slot.TypeAnnotation.(*ast.OptionalTypeExpr); ok {
			optional = true
		}
		if optional {
			builder = builder.OptParam(name, paramType)
		} else {
			builder = builder.Param(name, paramType)
		}
	}

	if sourceFn != nil && sourceFn.ParList != nil && sourceFn.ParList.HasVargs {
		builder = builder.Variadic(typ.Any)
	} else if hasUntypedSourceParam {
		// Unannotated Lua functions accept extra positional arguments.
		builder = builder.Variadic(typ.Any)
	}
	return builder.Build()
}

// CollectParamHintsFromResult records parameter hints based on call sites
// within the current function's graph using narrowed expression types.
func CollectParamHintsFromResult(store Store, result *api.FuncResult, parent *scope.State) {
	if store == nil || result == nil || result.Graph == nil || result.NarrowSynth == nil {
		return
	}
	graph := result.Graph

	moduleBindings := store.ModuleBindings()
	bindings := graph.Bindings()
	if bindings == nil {
		bindings = moduleBindings
	}
	preAssignTargets := checkcallsite.PreAssignmentTargetsByCall(graph)

	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil || len(info.Args) == 0 {
			return
		}
		callTargets := preAssignTargets[info]
		argTypes := make([]typ.Type, len(info.Args))
		for i, arg := range info.Args {
			if arg == nil {
				continue
			}
			argType := result.NarrowSynth.TypeOf(arg, p)
			argSym := cfg.SymbolID(0)
			if i < len(info.ArgSymbols) {
				argSym = info.ArgSymbols[i]
			}
			if argSym == 0 && bindings != nil {
				argSym = checkcallsite.SymbolFromExpr(arg, bindings)
			}
			if preType := typeAtPredJoin(result, graph, p, argSym); preType != nil {
				if callTargets[argSym] {
					argType = preType
				} else {
					argType = typ.JoinPreferNonSoft(argType, preType)
				}
			}
			argTypes[i] = argType
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
				merged := typ.JoinPreferNonSoft(updated[i], reSynthed)
				if !typ.TypeEquals(updated[i], merged) {
					updated[i] = merged
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
		calleeSym := checkcallsite.PreferredCalleeSymbolWithAliases(info, result.Graph, bindings, moduleBindings, hasFunctionRef)
		if calleeSym == 0 {
			return
		}
		ref := store.FunctionRefBySym(calleeSym)
		if ref == nil {
			return
		}
		parentKey, ok := parentGraphKeyForCallee(store, result, parent, calleeSym)
		if !ok {
			return
		}

		store.UpdateInterprocFactsNext(parentKey, func(facts *api.Facts) {
			if facts.ParamHints == nil {
				facts.ParamHints = make(api.ParamHints)
			}
			hints := paramhints.EnsureHintCapacity(facts.ParamHints[calleeSym], len(info.Args))
			for i, arg := range info.Args {
				if arg == nil {
					continue
				}
				if expectedFn := unwrap.Function(infer.ExpectedArgType(i)); expectedFn != nil {
					argSym := checkcallsite.CanonicalSymbolFromExprWithAliases(
						arg,
						0,
						result.Graph,
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
				hints, _ = paramhints.MergeCallArgHintAt(hints, i, argType, returns.JoinInterprocTypes, true)
			}
			if len(hints) > 0 {
				facts.ParamHints[calleeSym] = hints
			}
		})
	})
}

func parentGraphKeyForCallee(store Store, result *api.FuncResult, parent *scope.State, calleeSym cfg.SymbolID) (api.GraphKey, bool) {
	if store == nil || result == nil || result.Graph == nil || calleeSym == 0 {
		return api.GraphKey{}, false
	}
	if key, ok := store.ParentGraphKeyForSymbol(calleeSym); ok {
		return key, true
	}

	ref := store.FunctionRefBySym(calleeSym)
	if ref == nil {
		return api.GraphKey{}, false
	}
	parentGraphID := ref.ParentGraphID
	if parentGraphID == 0 {
		parentGraphID = ref.GraphID
	}
	if parentGraphID != result.Graph.ID() {
		return api.GraphKey{}, false
	}
	return store.GraphKeyFor(result.Graph, parent)
}

func typeAtPredJoin(result *api.FuncResult, graph *cfg.Graph, p cfg.Point, sym cfg.SymbolID) typ.Type {
	if result == nil || result.NarrowSynth == nil || graph == nil || sym == 0 {
		return nil
	}
	var joined typ.Type
	for _, pred := range graph.Predecessors(p) {
		tv := result.EffectiveTypeAt(pred, sym)
		if tv.State != flow.StateResolved || tv.Type == nil {
			continue
		}
		if joined == nil {
			joined = tv.Type
		} else {
			joined = typ.JoinPreferNonSoft(joined, tv.Type)
		}
	}
	return joined
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
