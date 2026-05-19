package interproc

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	checkcallsite "github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/erreffect"
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
	api.StoreReader

	MergeInterprocFactsNext(key api.GraphKey, delta api.Facts)
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
	writer := newInterprocFactWriter(store)
	writer.writeLiteralSignatures(result.Graph, parent, result.LiteralSignatures)

	if result.NarrowSynth == nil {
		return
	}
	fnSym := cfg.SymbolID(0)
	if fn != nil {
		if resolvedSym, ok := store.SymbolForFunc(fn); ok && resolvedSym != 0 {
			fnSym = resolvedSym
		}
	}
	// Collect parameter hints regardless of whether the function has a symbol.
	CollectParamHintsFromResult(store, result, parent)

	if fnSym == 0 {
		return
	}
	storeCapturedFactsFromResult(store, writer, fn, fnSym, result)

	fnType := narrowFunctionTypeFromResult(result, fn)
	if fnType == nil {
		return
	}
	narrowSummary := returns.NormalizeReturnVector(fnType.Returns)
	if snapNarrow := narrowSummarySnapshotForSymbol(store, result, parent, fnSym); len(snapNarrow) > 0 {
		narrowSummary = returns.MergeReturnSummary(narrowSummary, snapNarrow)
		if aligned, changed := returns.AlignFunctionTypeWithSummary(fnType, narrowSummary); changed {
			fnType = aligned
		}
	}
	summaryFromSnapshot := returnSummarySnapshotForSymbol(store, result, parent, fnSym)

	candidateFunc := fnType
	if hints := store.GetParamHintsSnapshot(result.Graph, parent); len(hints) > 0 {
		if hinted := paramhints.MergeIntoSignature(fn, hints[fnSym], unwrap.Function(candidateFunc)); hinted != nil {
			candidateFunc = hinted
		}
	}
	delta := api.Facts{FunctionFacts: api.FunctionFacts{
		fnSym: returns.JoinFunctionFact(api.FunctionFact{}, api.FunctionFact{
			Summary: summaryFromSnapshot,
			Narrow:  narrowSummary,
			Type:    candidateFunc,
		}),
	}}
	writer.mergeParentFactsForSymbol(fnSym, delta)
}

func storeCapturedFactsFromResult(
	store Store,
	writer interprocFactWriter,
	fn *ast.FunctionExpr,
	fnSym cfg.SymbolID,
	result *api.FuncResult,
) {
	if store == nil || fn == nil || fnSym == 0 || result == nil || result.Graph == nil || result.NarrowSynth == nil {
		return
	}
	bindings := bindingsForGraphOrModule(result.Graph, store)
	if bindings == nil {
		return
	}
	capturedSet := capturedSymbolSet(bindings, fn)
	if len(capturedSet) == 0 {
		return
	}

	fields := nested.CollectCapturedFieldAssignments(result.Graph, capturedSet, result.NarrowSynth.TypeOf)
	if len(fields) > 0 {
		writer.mergeParentFactsForSymbol(fnSym, api.Facts{
			CapturedFields: api.CapturedFieldAssigns{
				fnSym: fields,
			},
		})
	}

	mutations := nested.CollectCapturedContainerMutations(result.Graph, capturedSet, result.NarrowSynth.TypeOf)
	if len(mutations) > 0 {
		writer.mergeParentFactsForSymbol(fnSym, api.Facts{
			CapturedContainers: api.CapturedContainerMutations{
				fnSym: mutations,
			},
		})
	}
}

func bindingsForGraphOrModule(graph *cfg.Graph, store Store) *bind.BindingTable {
	if graph == nil {
		return nil
	}
	bindings := graph.Bindings()
	if bindings != nil {
		return bindings
	}
	if store != nil {
		return store.ModuleBindings()
	}
	return nil
}

func capturedSymbolSet(bindings *bind.BindingTable, fn *ast.FunctionExpr) map[cfg.SymbolID]bool {
	if bindings == nil || fn == nil {
		return nil
	}
	captured := bindings.CapturedSymbols(fn)
	if len(captured) == 0 {
		return nil
	}
	set := make(map[cfg.SymbolID]bool, len(captured))
	for _, sym := range captured {
		if sym != 0 {
			set[sym] = true
		}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

func narrowFunctionTypeFromResult(result *api.FuncResult, fn *ast.FunctionExpr) *typ.Function {
	if result == nil || result.NarrowSynth == nil || fn == nil {
		return nil
	}
	fnType := result.NarrowSynth.FunctionType(fn, result.BaseScope)
	if expected := expectedFunctionFromResult(result); expected != nil {
		if withExpected, ok := result.NarrowSynth.(functionTypeWithExpected); ok {
			if inferred := withExpected.SynthFunctionTypeWithExpected(fn, result.BaseScope, expected); inferred != nil {
				fnType = inferred
			}
		}
	}
	return erreffect.AttachInferredErrorReturnSpec(fnType, result.Graph, result.FlowSolution, result.NarrowSynth)
}

func returnSummarySnapshotForSymbol(store Store, result *api.FuncResult, parent *scope.State, sym cfg.SymbolID) []typ.Type {
	if store == nil || result == nil || result.Graph == nil || sym == 0 {
		return nil
	}
	summaryGraph := result.Graph
	summaryScope := api.ParentScopeForGraph(store, result.Graph.ID(), parent)
	if parentKey, ok := store.ParentGraphKeyForSymbol(sym); ok {
		if g := store.Graphs()[parentKey.GraphID]; g != nil {
			summaryGraph = g
			if scopedParent, ok := store.Parents()[parentKey.ParentHash]; ok {
				summaryScope = scopedParent
			}
		}
	}
	facts := store.GetFunctionFactsSnapshot(summaryGraph, summaryScope)
	if len(facts) == 0 {
		return nil
	}
	return facts.Summary(sym)
}

func narrowSummarySnapshotForSymbol(store Store, result *api.FuncResult, parent *scope.State, sym cfg.SymbolID) []typ.Type {
	if store == nil || result == nil || result.Graph == nil || sym == 0 {
		return nil
	}
	summaryGraph := result.Graph
	summaryScope := api.ParentScopeForGraph(store, result.Graph.ID(), parent)
	if parentKey, ok := store.ParentGraphKeyForSymbol(sym); ok {
		if g := store.Graphs()[parentKey.GraphID]; g != nil {
			summaryGraph = g
			if scopedParent, ok := store.Parents()[parentKey.ParentHash]; ok {
				summaryScope = scopedParent
			}
		}
	}

	var facts api.FunctionFacts
	if phaser, ok := any(store).(interface {
		WithPhase(api.Phase, func())
	}); ok {
		phaser.WithPhase(api.PhaseNarrowing, func() {
			facts = store.GetFunctionFactsSnapshot(summaryGraph, summaryScope)
		})
	} else {
		facts = store.GetFunctionFactsSnapshot(summaryGraph, summaryScope)
	}
	if len(facts) == 0 {
		return nil
	}
	return facts.NarrowSummary(sym)
}

func expectedFunctionFromResult(result *api.FuncResult) *typ.Function {
	if result == nil || result.Graph == nil || result.FlowInputs == nil {
		return nil
	}
	slots := result.Graph.ParamSlotsReadOnly()
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
	hasFunctionRef := func(sym cfg.SymbolID) bool {
		return sym != 0 && store.FunctionRefBySym(sym) != nil
	}
	collectCallHints := func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil || checkcallsite.RuntimeArgCount(info) == 0 {
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
			preType := checkcallsite.PreAssignmentTypeAtJoin(graph, p, argSym, func(point cfg.Point, id cfg.SymbolID) (typ.Type, bool) {
				tv := result.EffectiveTypeAt(point, id)
				if tv.State != flow.StateResolved || tv.Type == nil {
					return nil, false
				}
				return tv.Type, true
			})
			if preType != nil {
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
		calleeSym := checkcallsite.SelectPreferredSymbol(
			checkcallsite.CallableCalleeSymbolCandidates(info, result.Graph, bindings, moduleBindings),
			hasFunctionRef,
		)
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

		deltaHints := make(api.ParamHints)
		runtimeArgCount := checkcallsite.RuntimeArgCount(info)
		hints := paramhints.EnsureHintCapacity(nil, runtimeArgCount)
		for runtimeIdx := 0; runtimeIdx < runtimeArgCount; runtimeIdx++ {
			arg := checkcallsite.RuntimeArgAt(info, runtimeIdx)
			if arg == nil {
				continue
			}
			var argType typ.Type
			if checkcallsite.IsMethodCallInfo(info) && runtimeIdx == 0 {
				argType = def.Receiver
			} else {
				argIdx := runtimeIdx
				if checkcallsite.IsMethodCallInfo(info) {
					argIdx--
				}
				if argIdx >= 0 && argIdx < len(argTypes) {
					argType = argTypes[argIdx]
				}
			}
			if argType == nil {
				argType = result.NarrowSynth.TypeOf(arg, p)
			}
			hints, _ = paramhints.MergeCallArgHintAt(hints, runtimeIdx, argType, typ.JoinPreferNonSoft, true)
		}
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
					hintsForFn := deltaHints[argSym]
					for j, param := range expectedFn.Params {
						hintsForFn, _ = paramhints.MergeHintAt(hintsForFn, j, param.Type, typ.JoinPreferNonSoft)
					}
					if len(hintsForFn) > 0 {
						deltaHints[argSym] = hintsForFn
					}
				}
			}
		}
		if len(hints) > 0 {
			deltaHints[calleeSym] = hints
		}
		if len(deltaHints) > 0 {
			store.MergeInterprocFactsNext(parentKey, api.Facts{ParamHints: deltaHints})
		}
	}

	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		collectCallHints(p, info)

		seenNested := make(map[*ast.FuncCallExpr]struct{})
		for _, arg := range info.Args {
			collectNestedFuncCalls(arg, seenNested)
		}
		for nested := range seenNested {
			nestedInfo := graph.CallSiteAt(p, nested)
			if nestedInfo == nil {
				nestedInfo = synthCallInfoFromExpr(nested, bindings)
			}
			collectCallHints(p, nestedInfo)
		}
	})
}

func synthCallInfoFromExpr(ex *ast.FuncCallExpr, bindings *bind.BindingTable) *cfg.CallInfo {
	if ex == nil {
		return nil
	}
	info := &cfg.CallInfo{
		Call:     ex,
		Callee:   ex.Func,
		Args:     ex.Args,
		Method:   ex.Method,
		Receiver: ex.Receiver,
		IsStmt:   false,
	}
	if id, ok := ex.Func.(*ast.IdentExpr); ok {
		info.CalleeName = id.Value
	}
	if bindings != nil {
		info.CalleeSymbol = checkcallsite.SymbolFromExpr(ex.Func, bindings)
		if ex.Receiver != nil {
			info.ReceiverSymbol = checkcallsite.SymbolFromExpr(ex.Receiver, bindings)
			if id, ok := ex.Receiver.(*ast.IdentExpr); ok {
				info.ReceiverName = id.Value
			}
		}
		info.ArgSymbols = make([]cfg.SymbolID, len(ex.Args))
		for i, arg := range ex.Args {
			info.ArgSymbols[i] = checkcallsite.SymbolFromExpr(arg, bindings)
		}
	}
	return info
}

func collectNestedFuncCalls(expr ast.Expr, out map[*ast.FuncCallExpr]struct{}) {
	if expr == nil || out == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.FuncCallExpr:
		out[e] = struct{}{}
		collectNestedFuncCalls(e.Func, out)
		collectNestedFuncCalls(e.Receiver, out)
		for _, arg := range e.Args {
			collectNestedFuncCalls(arg, out)
		}
	case *ast.AttrGetExpr:
		collectNestedFuncCalls(e.Object, out)
		collectNestedFuncCalls(e.Key, out)
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			collectNestedFuncCalls(field.Key, out)
			collectNestedFuncCalls(field.Value, out)
		}
	case *ast.LogicalOpExpr:
		collectNestedFuncCalls(e.Lhs, out)
		collectNestedFuncCalls(e.Rhs, out)
	case *ast.RelationalOpExpr:
		collectNestedFuncCalls(e.Lhs, out)
		collectNestedFuncCalls(e.Rhs, out)
	case *ast.StringConcatOpExpr:
		collectNestedFuncCalls(e.Lhs, out)
		collectNestedFuncCalls(e.Rhs, out)
	case *ast.ArithmeticOpExpr:
		collectNestedFuncCalls(e.Lhs, out)
		collectNestedFuncCalls(e.Rhs, out)
	case *ast.UnaryMinusOpExpr:
		collectNestedFuncCalls(e.Expr, out)
	case *ast.UnaryNotOpExpr:
		collectNestedFuncCalls(e.Expr, out)
	case *ast.UnaryLenOpExpr:
		collectNestedFuncCalls(e.Expr, out)
	case *ast.UnaryBNotOpExpr:
		collectNestedFuncCalls(e.Expr, out)
	}
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
