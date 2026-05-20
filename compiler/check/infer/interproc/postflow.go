package interproc

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	checkcallsite "github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	interprocdomain "github.com/wippyai/go-lua/compiler/check/domain/interproc"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/compiler/check/domain/returnsummary"
	"github.com/wippyai/go-lua/compiler/check/erreffect"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	typjoin "github.com/wippyai/go-lua/types/typ/join"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

type functionTypeWithExpected interface {
	SynthFunctionTypeWithExpected(fn *ast.FunctionExpr, sc *scope.State, expected *typ.Function) *typ.Function
}

// Store is the minimal store interface required to record post-flow interproc facts.
type Store interface {
	api.StoreReader

	MergeInterprocFactsNext(key api.GraphKey, delta api.Facts)
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
	// Collect parameter evidence regardless of whether the function has a symbol.
	CollectParameterEvidenceFromResult(store, result, parent)

	if fnSym == 0 {
		return
	}
	storeCapturedFactsFromResult(store, writer, fn, fnSym, result)

	fnType := narrowFunctionTypeFromResult(result, fn)
	if fnType == nil {
		return
	}
	narrowSummary := returnsummary.Normalize(fnType.Returns)
	if factNarrow := narrowSummaryFactForSymbol(store, result, parent, fnSym); len(factNarrow) > 0 {
		narrowSummary = returnsummary.Merge(narrowSummary, factNarrow)
		if aligned, changed := returnsummary.AlignFunction(fnType, narrowSummary); changed {
			fnType = aligned
		}
	}
	summaryFromFacts := returnSummaryFactForSymbol(store, result, parent, fnSym)

	candidateFunc := fnType
	if len(narrowSummary) > 0 && !returnsummary.AllNil(narrowSummary) {
		if aligned := typjoin.WithReturns(candidateFunc, narrowSummary); aligned != nil {
			candidateFunc = aligned
		}
	}
	if facts := store.GetInterprocFacts(result.Graph, parent).FunctionFacts; len(facts) > 0 {
		if hinted := paramevidence.MergeIntoSignature(fn, facts.Params(fnSym), unwrap.Function(candidateFunc)); hinted != nil {
			candidateFunc = hinted
		}
	}
	candidateFunc = stripSyntheticVariadic(fn, unwrap.Function(candidateFunc))
	delta := interprocdomain.FunctionFactDelta(
		fnSym,
		functionfact.Join(api.FunctionFact{}, api.FunctionFact{
			Summary: summaryFromFacts,
			Narrow:  narrowSummary,
			Type:    candidateFunc,
		}),
	)
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

	fields := capturedFieldFactsFromEvidence(result.Evidence.CapturedFields, result.NarrowSynth.TypeOf)
	if len(fields) > 0 {
		writer.mergeParentFactsForSymbol(fnSym, interprocdomain.CapturedFieldAssignsDelta(fnSym, fields))
	}

	mutations := capturedContainerFactsFromEvidence(result.Evidence.CapturedContainers, result.NarrowSynth.TypeOf)
	if len(mutations) > 0 {
		writer.mergeParentFactsForSymbol(fnSym, interprocdomain.CapturedContainerMutationsDelta(fnSym, mutations))
	}
}

func capturedFieldFactsFromEvidence(
	evidence []api.CapturedFieldEvidence,
	synth func(ast.Expr, cfg.Point) typ.Type,
) map[cfg.SymbolID]map[string]typ.Type {
	if len(evidence) == 0 {
		return nil
	}
	fields := make(map[cfg.SymbolID]map[string]typ.Type)
	for _, ev := range evidence {
		if ev.Target == 0 || ev.Field == "" {
			continue
		}
		fieldType := typ.Unknown
		if synth != nil && ev.Value != nil {
			if t := synth(ev.Value, ev.Point); t != nil {
				fieldType = t
			}
		}
		if fields[ev.Target] == nil {
			fields[ev.Target] = make(map[string]typ.Type)
		}
		if existing := fields[ev.Target][ev.Field]; existing != nil {
			fields[ev.Target][ev.Field] = typ.JoinPreferNonSoft(existing, fieldType)
		} else {
			fields[ev.Target][ev.Field] = fieldType
		}
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

func capturedContainerFactsFromEvidence(
	evidence []api.CapturedContainerEvidence,
	synth func(ast.Expr, cfg.Point) typ.Type,
) map[cfg.SymbolID][]api.ContainerMutation {
	if len(evidence) == 0 {
		return nil
	}
	mutations := make(map[cfg.SymbolID][]api.ContainerMutation)
	for _, ev := range evidence {
		if ev.Target == 0 || ev.Value == nil {
			continue
		}
		valueType := typ.Unknown
		if synth != nil {
			if t := synth(ev.Value, ev.Point); t != nil {
				valueType = t
			}
		}
		mutations[ev.Target] = append(mutations[ev.Target], api.ContainerMutation{
			Kind:      ev.Kind,
			Segments:  cloneSegments(ev.Segments),
			ValueType: subtype.WidenForInference(valueType),
		})
	}
	if len(mutations) == 0 {
		return nil
	}
	return mutations
}

func cloneSegments(segments []constraint.Segment) []constraint.Segment {
	if len(segments) == 0 {
		return nil
	}
	out := make([]constraint.Segment, len(segments))
	copy(out, segments)
	return out
}

func stripSyntheticVariadic(fn *ast.FunctionExpr, sig *typ.Function) *typ.Function {
	if fn == nil || fn.ParList == nil || fn.ParList.HasVargs || sig == nil || sig.Variadic == nil {
		return sig
	}
	builder := typ.Func().ReserveParams(len(sig.Params))
	for _, tp := range sig.TypeParams {
		builder = builder.TypeParam(tp.Name, tp.Constraint)
	}
	for _, p := range sig.Params {
		if p.Optional {
			builder = builder.OptParam(p.Name, p.Type)
		} else {
			builder = builder.Param(p.Name, p.Type)
		}
	}
	if len(sig.Returns) > 0 {
		builder = builder.Returns(sig.Returns...)
	}
	if sig.Effects != nil {
		builder = builder.Effects(sig.Effects)
	}
	if sig.Spec != nil {
		builder = builder.Spec(sig.Spec)
	}
	if sig.Refinement != nil {
		builder = builder.WithRefinement(sig.Refinement)
	}
	return builder.Build()
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
	return erreffect.AttachInferredErrorReturnSpec(fnType, result.Evidence, result.FlowSolution, result.NarrowSynth)
}

func returnSummaryFactForSymbol(store Store, result *api.FuncResult, parent *scope.State, sym cfg.SymbolID) []typ.Type {
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
	facts := store.GetInterprocFacts(summaryGraph, summaryScope).FunctionFacts
	if len(facts) == 0 {
		return nil
	}
	return facts.Summary(sym)
}

func narrowSummaryFactForSymbol(store Store, result *api.FuncResult, parent *scope.State, sym cfg.SymbolID) []typ.Type {
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
			facts = store.GetInterprocFacts(summaryGraph, summaryScope).FunctionFacts
		})
	} else {
		facts = store.GetInterprocFacts(summaryGraph, summaryScope).FunctionFacts
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
	}
	return builder.Build()
}

// CollectParameterEvidenceFromResult reduces transfer-discovered call evidence
// into canonical parameter facts using narrowed expression types.
func CollectParameterEvidenceFromResult(store Store, result *api.FuncResult, parent *scope.State) {
	if store == nil || result == nil || result.Graph == nil || result.NarrowSynth == nil {
		return
	}
	graph := result.Graph

	moduleBindings := store.ModuleBindings()
	bindings := graph.Bindings()
	if bindings == nil {
		bindings = moduleBindings
	}
	preAssignTargets := checkcallsite.PreAssignmentTargetsByCall(result.Evidence.Assignments)
	hasFunctionRef := func(sym cfg.SymbolID) bool {
		return sym != 0 && store.FunctionRefBySym(sym) != nil
	}
	collectCallEvidence := func(p cfg.Point, info *cfg.CallInfo) {
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

		deltaFacts := make(api.FunctionFacts)
		runtimeArgCount := checkcallsite.RuntimeArgCount(info)
		evidence := paramevidence.EnsureCapacity(nil, runtimeArgCount)
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
			evidence, _ = paramevidence.MergeCallArgAt(evidence, runtimeIdx, argType, typ.JoinPreferNonSoft, true)
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
					fnEvidence := deltaFacts.Params(argSym)
					for j, param := range expectedFn.Params {
						fnEvidence, _ = paramevidence.MergeAt(fnEvidence, j, param.Type, typ.JoinPreferNonSoft)
					}
					if len(fnEvidence) > 0 {
						deltaFacts[argSym] = functionfact.Join(deltaFacts[argSym], api.FunctionFact{Params: fnEvidence})
					}
				}
			}
		}
		if len(evidence) > 0 {
			deltaFacts[calleeSym] = functionfact.Join(deltaFacts[calleeSym], api.FunctionFact{Params: evidence})
		}
		if len(deltaFacts) > 0 {
			store.MergeInterprocFactsNext(parentKey, interprocdomain.FunctionFactsDelta(deltaFacts))
		}
	}

	for _, evidence := range result.Evidence.Calls {
		collectCallEvidence(evidence.Point, evidence.Info)
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
