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
	synthextract "github.com/wippyai/go-lua/compiler/check/synth/phase/extract"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
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
	if factNarrow := functionfact.NarrowSummaryForSymbol(store, fnSym, parent, nil); len(factNarrow) > 0 {
		narrowSummary = returnsummary.Merge(narrowSummary, factNarrow)
		if aligned, changed := returnsummary.AlignFunction(fnType, narrowSummary); changed {
			fnType = aligned
		}
	}
	summaryFromFacts := functionfact.ReturnSummaryForSymbol(store, fnSym, parent, nil)

	candidateFunc := fnType
	if len(narrowSummary) > 0 && !returnsummary.AllNil(narrowSummary) {
		if aligned := typjoin.WithReturns(candidateFunc, narrowSummary); aligned != nil {
			candidateFunc = aligned
		}
	}
	if facts := store.GetInterprocFacts(result.Graph, parent).FunctionFacts; len(facts) > 0 {
		if hinted := paramevidence.MergeIntoSignature(fn, functionfact.ParameterEvidenceFromMap(facts, fnSym), unwrap.Function(candidateFunc)); hinted != nil {
			candidateFunc = hinted
		}
	}
	candidateFunc = stripSyntheticVariadic(fn, unwrap.Function(candidateFunc))
	delta := interprocdomain.FunctionFactsDelta(functionfact.FromPart(fnSym, functionfact.Parts{
		Summary: summaryFromFacts,
		Narrow:  narrowSummary,
		Type:    candidateFunc,
	}))
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
		var keyType typ.Type
		if ev.Key != nil {
			keyType = typ.Unknown
			if synth != nil {
				if t := synth(ev.Key, ev.Point); t != nil {
					keyType = subtype.WidenForInference(t)
				}
			}
		}
		mutations[ev.Target] = append(mutations[ev.Target], api.ContainerMutation{
			Kind:      ev.Kind,
			Segments:  cloneSegments(ev.Segments),
			KeyType:   keyType,
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
	fnType = attachSolvedCallbackOverlaySpec(fnType, result)
	return erreffect.AttachInferredErrorReturnSpec(fnType, result.Evidence, result.FlowSolution, result.NarrowSynth)
}

func attachSolvedCallbackOverlaySpec(fnType *typ.Function, result *api.FuncResult) *typ.Function {
	if fnType == nil || result == nil || result.Graph == nil || result.NarrowSynth == nil {
		return fnType
	}
	overlays := synthextract.InferCallbackEnvOverlays(
		result.Graph,
		result.Evidence,
		result.Graph.ParamSlotsReadOnly(),
		result.NarrowSynth.TypeOf,
		result.ModuleBindings,
	)
	if len(overlays) == 0 {
		return fnType
	}

	spec := cloneContractSpecForCallbacks(fnType)
	for paramIdx, overlay := range overlays {
		if len(overlay) == 0 {
			continue
		}
		cb := spec.GetCallback(paramIdx).Clone()
		if cb == nil {
			cb = &contract.CallbackSpec{Cardinality: contract.CardExactlyOnce}
		}
		cb.EnvOverlay = mergeCallbackEnvOverlay(cb.EnvOverlay, overlay)
		spec.WithCallback(paramIdx, cb)
	}
	return cloneFunctionWithSpec(fnType, spec)
}

func cloneContractSpecForCallbacks(fnType *typ.Function) *contract.Spec {
	if fnType == nil || fnType.Spec == nil {
		return contract.NewSpec()
	}
	spec, ok := fnType.Spec.(*contract.Spec)
	if !ok || spec == nil {
		return contract.NewSpec()
	}
	clone := contract.NewSpec()
	clone.Requires = spec.Requires
	clone.Ensures = spec.Ensures
	if len(spec.ExprRequires) > 0 {
		clone.ExprRequires = append([]constraint.ExprCompare(nil), spec.ExprRequires...)
	}
	if len(spec.ExprEnsures) > 0 {
		clone.ExprEnsures = append([]constraint.ExprCompare(nil), spec.ExprEnsures...)
	}
	clone.Effects = spec.Effects
	if len(spec.Callbacks) > 0 {
		clone.Callbacks = make(map[int]*contract.CallbackSpec, len(spec.Callbacks))
		for idx, cb := range spec.Callbacks {
			clone.Callbacks[idx] = cb.Clone()
		}
	}
	clone.Return = spec.Return
	return clone
}

func mergeCallbackEnvOverlay(base, overlay map[string]typ.Type) map[string]typ.Type {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	out := make(map[string]typ.Type, len(base)+len(overlay))
	for name, t := range base {
		if name != "" && t != nil {
			out[name] = t
		}
	}
	for name, candidate := range overlay {
		if name == "" || candidate == nil {
			continue
		}
		if existing := out[name]; existing != nil {
			out[name] = typ.JoinPreferNonSoft(existing, candidate)
		} else {
			out[name] = candidate
		}
	}
	return out
}

func cloneFunctionWithSpec(fn *typ.Function, spec *contract.Spec) *typ.Function {
	if fn == nil {
		return nil
	}
	builder := typ.Func().ReserveParams(len(fn.Params))
	for _, tp := range fn.TypeParams {
		builder = builder.TypeParam(tp.Name, tp.Constraint)
	}
	for _, p := range fn.Params {
		if p.Optional {
			builder = builder.OptParam(p.Name, p.Type)
		} else {
			builder = builder.Param(p.Name, p.Type)
		}
	}
	if fn.Variadic != nil {
		builder = builder.Variadic(fn.Variadic)
	}
	if len(fn.Returns) > 0 {
		builder = builder.Returns(fn.Returns...)
	}
	if fn.Effects != nil {
		builder = builder.Effects(fn.Effects)
	}
	if spec != nil {
		builder = builder.Spec(spec)
	}
	if fn.Refinement != nil {
		builder = builder.WithRefinement(fn.Refinement)
	}
	return builder.Build()
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
		parentKey, ok := functionfact.GraphKeyForSymbol(store, calleeSym, parent)
		if !ok {
			return
		}

		paramFacts := make(map[cfg.SymbolID][]typ.Type)
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
					fnEvidence := paramFacts[argSym]
					for j, param := range expectedFn.Params {
						fnEvidence, _ = paramevidence.MergeAt(fnEvidence, j, param.Type, typ.JoinPreferNonSoft)
					}
					if len(fnEvidence) > 0 {
						paramFacts[argSym] = fnEvidence
					}
				}
			}
		}
		if len(evidence) > 0 {
			paramFacts[calleeSym] = paramevidence.JoinVectors(paramFacts[calleeSym], evidence)
		}
		if facts := functionfact.FromMaps(paramFacts, nil, nil); len(facts) > 0 {
			store.MergeInterprocFactsNext(parentKey, interprocdomain.FunctionFactsDelta(facts))
		}
	}

	for _, evidence := range result.Evidence.Calls {
		collectCallEvidence(evidence.Point, evidence.Info)
	}
}
