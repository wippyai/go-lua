package infer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/abstract/assign"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/calleffect"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/compiler/check/domain/resolve"
	"github.com/wippyai/go-lua/compiler/check/domain/returnsummary"
	"github.com/wippyai/go-lua/compiler/check/overlaymut"
	"github.com/wippyai/go-lua/compiler/check/phase"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/siblings"
	"github.com/wippyai/go-lua/compiler/check/synth"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

func (i *Inferencer) buildParameterOverlay(ctx *returnInferenceContext) map[cfg.SymbolID]typ.Type {
	fnGraph := ctx.info.Graph
	paramSlots := fnGraph.ParamSlotsReadOnly()
	overlay := make(map[cfg.SymbolID]typ.Type, overlaySymbolCapacity(fnGraph, len(paramSlots)))
	for paramIdx, slot := range paramSlots {
		if slot.Symbol == 0 {
			continue
		}

		// Binder/CFG-injected implicit self parameter.
		_, hasSource := slot.SourceParamIndex()
		if !hasSource {
			if selfType := ctx.resolveScope.SelfType(); selfType != nil {
				overlay[slot.Symbol] = selfType
			} else if ctx.info.ParameterEvidence != nil && paramIdx < len(ctx.info.ParameterEvidence) && ctx.info.ParameterEvidence[paramIdx] != nil {
				overlay[slot.Symbol] = ctx.info.ParameterEvidence[paramIdx]
			} else {
				overlay[slot.Symbol] = typ.Unknown
			}
			continue
		}

		paramType := typ.Unknown
		if slot.Name == "self" {
			if selfType := ctx.resolveScope.SelfType(); selfType != nil {
				paramType = selfType
			}
		}
		if typ.IsAbsentOrUnknown(paramType) {
			if ctx.info.ParameterEvidence != nil && paramIdx < len(ctx.info.ParameterEvidence) && ctx.info.ParameterEvidence[paramIdx] != nil {
				paramType = ctx.info.ParameterEvidence[paramIdx]
			}
		}
		if typ.IsAbsentOrUnknown(paramType) && slot.TypeAnnotation == nil {
			paramType = typ.Any
		}
		if slot.TypeAnnotation != nil {
			resolved := ctx.engine.ResolveType(slot.TypeAnnotation, ctx.resolveScope)
			if resolved != nil {
				paramType = resolved
				if ctx.info.ParameterEvidence != nil && paramIdx < len(ctx.info.ParameterEvidence) {
					paramType = paramevidence.RefineAnnotationWithEvidence(resolved, ctx.info.ParameterEvidence[paramIdx])
				}
			}
		}
		overlay[slot.Symbol] = paramType
	}
	return overlay
}

func overlaySymbolCapacity(fnGraph *cfg.Graph, floor int) int {
	if fnGraph == nil {
		return floor
	}
	if count := fnGraph.SymbolCount(); count > floor {
		return count
	}
	return floor
}

// enrichOverlayWithSiblings adds sibling function types from the current
// return-vector state.
func (i *Inferencer) enrichOverlayWithSiblings(
	ctx *returnInferenceContext,
	overlay map[cfg.SymbolID]typ.Type,
) {
	siblingEntries := make([]siblings.OverlayEntry, 0, len(ctx.localFuncs))
	for _, sym := range cfg.SortedSymbolIDs(ctx.localFuncs) {
		sibInfo := ctx.localFuncs[sym]
		if sibInfo != nil && sibInfo.Fn != nil {
			siblingEntries = append(siblingEntries, siblings.OverlayEntry{
				Symbol: sym,
				Func:   sibInfo.Fn,
			})
		}
	}
	siblingOverlay := siblings.BuildOverlay(siblings.OverlayConfig{
		ReturnVectors: ctx.returnVectors,
		Siblings:      siblingEntries,
		CurrentSym:    ctx.info.Sym,
		Services: siblings.OverlayServicesFuncs{
			SeedTypeFn: func(fn *ast.FunctionExpr) typ.Type {
				var localInfo *returns.LocalFuncInfo
				for _, sym := range cfg.SortedSymbolIDs(ctx.localFuncs) {
					candidate := ctx.localFuncs[sym]
					if candidate != nil && candidate.Fn == fn {
						localInfo = candidate
						break
					}
				}
				var bindings interface {
					ParamSymbols(*ast.FunctionExpr) []cfg.SymbolID
					Name(cfg.SymbolID) string
				}
				if localInfo != nil && localInfo.Graph != nil {
					if b := localInfo.Graph.Bindings(); b != nil {
						bindings = b
					}
				}
				if bindings == nil && ctx.info != nil && ctx.info.Graph != nil {
					if b := ctx.info.Graph.Bindings(); b != nil {
						bindings = b
					}
				}
				seed := returns.BuildSeedFunctionTypeWithBindings(fn, ctx.engine, ctx.resolveScope, bindings)
				fnType, _ := seed.(*typ.Function)
				if localInfo != nil && len(localInfo.ParameterEvidence) > 0 && fnType != nil {
					return paramevidence.MergeIntoSignature(fn, localInfo.ParameterEvidence, fnType)
				}
				return seed
			},
		},
	})
	for sym, ty := range siblingOverlay {
		overlay[sym] = ty
	}
}

// collectAllReturnVectors normalizes the current local return-vector map.
func (i *Inferencer) collectAllReturnVectors(ctx *returnInferenceContext) map[cfg.SymbolID][]typ.Type {
	if ctx == nil || len(ctx.returnVectors) == 0 {
		return nil
	}
	allReturnVectors := make(map[cfg.SymbolID][]typ.Type, len(ctx.returnVectors))
	for _, sym := range cfg.SortedSymbolIDs(ctx.returnVectors) {
		if sym == 0 {
			continue
		}
		normalized := returnsummary.NormalizeOwned(ctx.returnVectors[sym])
		if len(normalized) == 0 {
			continue
		}
		allReturnVectors[sym] = normalized
	}
	return allReturnVectors
}

func (i *Inferencer) returnVectorFromFacts(
	graph *cfg.Graph,
	parentScope *scope.State,
	sym cfg.SymbolID,
) []typ.Type {
	if i == nil || i.store == nil || graph == nil || parentScope == nil || sym == 0 {
		return nil
	}
	facts := i.store.GetInterprocFacts(graph, parentScope).FunctionFacts
	if len(facts) == 0 {
		return nil
	}
	normalized := returnsummary.Normalize(functionfact.ReturnSummaryFromMap(facts, sym))
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func (i *Inferencer) resolveLocalFunctionReturns(
	ctx *returnInferenceContext,
	allReturnVectors map[cfg.SymbolID][]typ.Type,
	sym cfg.SymbolID,
) []typ.Type {
	if sym == 0 {
		return nil
	}

	// Keep the current SCC-derived return vector unless it is still unknown-only.
	returnVector := returnsummary.NormalizeOwned(allReturnVectors[sym])
	if !typ.IsUnknownOnlyOrEmpty(returnVector) {
		return returnVector
	}

	if ctx == nil || i == nil || i.store == nil {
		return returnVector
	}

	// Product fact recovery path: current graph under the current resolve scope.
	if ctx.info != nil && ctx.info.Graph != nil && ctx.resolveScope != nil {
		if factVector := i.returnVectorFromFacts(ctx.info.Graph, ctx.resolveScope, sym); len(factVector) > 0 {
			return factVector
		}
	}

	// Product fact recovery path: parent graph for the function symbol, if known.
	ref := i.store.FunctionRefBySym(sym)
	if ref == nil || ref.ParentGraphID == 0 {
		return returnVector
	}
	parentGraph := i.store.Graphs()[ref.ParentGraphID]
	if parentGraph == nil {
		return returnVector
	}
	parentScope := api.ParentScopeForGraph(i.store, parentGraph.ID(), nil)
	if parentScope == nil {
		return returnVector
	}
	if factVector := i.returnVectorFromFacts(parentGraph, parentScope, sym); len(factVector) > 0 {
		return factVector
	}
	return returnVector
}

// enrichOverlayWithLocalFunctions adds local function types from the function body.
func (i *Inferencer) enrichOverlayWithLocalFunctions(
	ctx *returnInferenceContext,
	overlay map[cfg.SymbolID]typ.Type,
	allReturnVectors map[cfg.SymbolID][]typ.Type,
) {
	for _, assign := range ctx.info.Evidence.Assignments {
		p := assign.Point
		assignInfo := assign.Info
		if assignInfo == nil || !assignInfo.IsLocal || len(assignInfo.Targets) == 0 || len(assignInfo.Sources) == 0 {
			continue
		}
		for idx, target := range assignInfo.Targets {
			if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
				continue
			}
			if _, ok := overlay[target.Symbol]; ok {
				continue
			}
			if idx >= len(assignInfo.Sources) {
				continue
			}
			fnExpr, ok := assignInfo.Sources[idx].(*ast.FunctionExpr)
			if !ok || fnExpr == nil {
				continue
			}
			if i.store != nil && i.graphs != nil {
				if fnGraph := i.graphs.GetOrBuildCFG(fnExpr); fnGraph != nil {
					i.store.RegisterFunctionRef(target.Symbol, fnExpr, fnGraph, ctx.info.Graph.ID(), p)
				}
			}
			returnVector := i.resolveLocalFunctionReturns(ctx, allReturnVectors, target.Symbol)
			sig := ctx.engine.ResolveFunctionSignature(fnExpr, ctx.resolveScope)
			if localInfo := ctx.localFuncs[target.Symbol]; localInfo != nil && len(localInfo.ParameterEvidence) > 0 && sig != nil {
				sig = paramevidence.MergeIntoSignature(fnExpr, localInfo.ParameterEvidence, sig)
			}
			if fnType := returnsummary.ApplyToFunctionType(sig, returnVector); fnType != nil {
				overlay[target.Symbol] = fnType
			}
		}
	}
}

// enrichOverlayWithCaptured adds captured variable types from parent function result.
func (i *Inferencer) enrichOverlayWithCaptured(
	ctx *returnInferenceContext,
	overlay map[cfg.SymbolID]typ.Type,
) {
	if ctx.info.ParentFn == nil || ctx.info.Graph == nil {
		return
	}
	defPoint := ctx.info.DefPoint
	if defPoint == 0 {
		return
	}
	localBindings := ctx.info.Graph.Bindings()
	if localBindings == nil {
		return
	}
	if i.store != nil && ctx.info.DefScope != nil {
		parentScope := api.ParentScopeForGraph(i.store, ctx.info.Graph.ID(), ctx.info.DefScope)
		if capturedTypes := i.store.GetInterprocFacts(ctx.info.Graph, parentScope).CapturedTypes; len(capturedTypes) > 0 {
			for _, sym := range cfg.SortedSymbolIDs(capturedTypes) {
				t := capturedTypes[sym]
				if sym == 0 || t == nil {
					continue
				}
				if existing, ok := overlay[sym]; ok && existing != nil && !typ.IsSoft(existing, typ.SoftAnnotationPolicy) {
					continue
				}
				overlay[sym] = t
			}
		}
	}
	if ctx.parentFacts == nil {
		return
	}
	resolveCapturedAnnotation := func(sym cfg.SymbolID) typ.Type {
		if sym == 0 {
			return nil
		}
		var annType typ.Type
		for _, assign := range ctx.info.ParentEvidence.Assignments {
			info := assign.Info
			if annType != nil || info == nil || !info.IsLocal || len(info.Targets) == 0 {
				continue
			}
			info.EachTarget(func(i int, target cfg.AssignTarget) {
				if annType != nil || target.Kind != cfg.TargetIdent || target.Symbol != sym {
					return
				}
				ann := info.TypeAnnotationAt(i)
				if ann == nil {
					return
				}
				resolveScope := ctx.info.DefScope
				if resolveScope == nil {
					resolveScope = ctx.resolveScope
				}
				annType = ctx.engine.ResolveType(ann, resolveScope)
			})
		}
		return annType
	}
	for _, sym := range localBindings.CapturedSymbols(ctx.info.Fn) {
		if sym == 0 {
			continue
		}
		if fnType := i.capturedFunctionFactType(ctx, sym); fnType != nil {
			overlay[sym] = fnType
			continue
		}
		if existing, ok := overlay[sym]; ok && existing != nil && !typ.IsSoft(existing, typ.SoftAnnotationPolicy) {
			continue
		}
		if annType := resolveCapturedAnnotation(sym); annType != nil {
			overlay[sym] = annType
			continue
		}
		if tv := ctx.parentFacts.EffectiveTypeAt(defPoint, sym); tv.State == flow.StateResolved && tv.Type != nil {
			overlay[sym] = tv.Type
		}
	}
}

func (i *Inferencer) capturedFunctionFactType(ctx *returnInferenceContext, sym cfg.SymbolID) typ.Type {
	if i == nil || i.store == nil || ctx == nil || sym == 0 {
		return nil
	}
	ref := i.store.FunctionRefBySym(sym)
	if ref == nil {
		return nil
	}
	parentGraphID := ref.ParentGraphID
	if parentGraphID == 0 {
		parentGraphID = ref.GraphID
	}
	parentGraph := i.store.Graphs()[parentGraphID]
	if parentGraph == nil {
		return nil
	}
	parentScope := ctx.info.DefScope
	if parentHash := i.store.GraphParentHashOf(parentGraphID); parentHash != 0 {
		if scoped := i.store.Parents()[parentHash]; scoped != nil {
			parentScope = scoped
		}
	}
	return functionfact.TypeForGraph(i.store, parentGraph, sym, parentScope, nil)
}

// inferLocalVariableTypes runs phase 1 synthesis to infer local variable types.
func (i *Inferencer) inferLocalVariableTypes(
	ctx *returnInferenceContext,
	overlay map[cfg.SymbolID]typ.Type,
	localValueSeeds map[cfg.SymbolID]bool,
) (map[cfg.SymbolID]typ.Type, *synth.Engine, func(ast.Expr, cfg.Point) typ.Type) {
	fnGraph := ctx.info.Graph
	annotated := make(map[cfg.SymbolID]bool, len(overlay))
	paramSet := paramSymbolSet(fnGraph)
	explicitParamAnnotations := make(map[cfg.SymbolID]bool)
	for _, slot := range fnGraph.ParamSlotsReadOnly() {
		if slot.Symbol == 0 || slot.TypeAnnotation == nil || ctx.engine == nil {
			continue
		}
		resolved := ctx.engine.ResolveType(slot.TypeAnnotation, ctx.resolveScope)
		if resolved != nil && !typ.IsRefinableAnnotation(resolved) {
			explicitParamAnnotations[slot.Symbol] = true
		}
	}
	for sym, tp := range overlay {
		if paramSet[sym] {
			if explicitParamAnnotations[sym] {
				annotated[sym] = true
			} else if tp != nil && !typ.IsAny(tp) && !typ.IsAbsentOrUnknown(tp) && !typ.IsSoft(tp, typ.SoftAnnotationPolicy) {
				annotated[sym] = true
			}
			continue
		}
		if localValueSeeds[sym] {
			continue
		}
		if tp != nil && !typ.IsSoft(tp, typ.SoftAnnotationPolicy) {
			annotated[sym] = true
		}
	}

	for _, assign := range ctx.info.Evidence.Assignments {
		assignInfo := assign.Info
		if assignInfo == nil || len(assignInfo.TypeAnnotations) == 0 {
			continue
		}
		for idx, target := range assignInfo.Targets {
			if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
				continue
			}
			if idx < len(assignInfo.TypeAnnotations) && assignInfo.TypeAnnotations[idx] != nil {
				if tp, ok := overlay[target.Symbol]; ok && tp != nil {
					if !typ.IsSoft(tp, typ.SoftAnnotationPolicy) {
						annotated[target.Symbol] = true
					}
				} else if resolved := ctx.engine.ResolveType(assignInfo.TypeAnnotations[idx], ctx.resolveScope); resolved != nil {
					if !typ.IsSoft(resolved, typ.SoftAnnotationPolicy) {
						annotated[target.Symbol] = true
					}
				}
			}
		}
	}

	inferenceOverlay := overlay
	if len(localValueSeeds) > 0 {
		inferenceOverlay = cloneOverlay(overlay, 0)
		for sym := range localValueSeeds {
			delete(inferenceOverlay, sym)
		}
	}

	fnScopes := uniformFunctionScopes(fnGraph, ctx.resolveScope)

	phaseFunctionFacts := functionfact.FromSummaries(ctx.returnVectors)
	prelimCtx := api.NewReturnInferenceEnv(api.ReturnInferenceEnvConfig{
		Graph:         fnGraph,
		Bindings:      ctx.bindings,
		BaseScope:     ctx.resolveScope,
		DeclaredTypes: inferenceOverlay,
		GlobalTypes:   i.globalTypes,
		ModuleAliases: ctx.moduleAliases,
		FunctionType:  functionfact.TypeLookup(phaseFunctionFacts),
	})

	prelimEngine := i.newReturnInferenceEngine(ctx.run, fnScopes, prelimCtx, ctx.info.Evidence, phaseFunctionFacts)

	synthAdapter := func(expr ast.Expr, p cfg.Point) typ.Type {
		return prelimEngine.TypeOf(expr, p)
	}
	symResolver := func(p cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		if prelimCtx == nil || prelimCtx.Types() == nil {
			return nil, false
		}
		tv := prelimCtx.Types().EffectiveTypeAt(p, sym)
		if tv.State == flow.StateResolved && tv.Type != nil {
			return tv.Type, true
		}
		if t, ok := prelimCtx.GlobalType(sym); ok && t != nil {
			return t, true
		}
		return nil, false
	}

	inferred := assign.InferLocalTypes(assign.LocalInferenceConfig{
		Graph:          fnGraph,
		Evidence:       ctx.info.Evidence,
		Scopes:         fnScopes,
		Synth:          synthAdapter,
		SynthAPI:       prelimEngine,
		SymResolver:    symResolver,
		SeedTypes:      inferenceOverlay,
		Annotated:      annotated,
		ModuleBindings: ctx.bindings,
		CallCtx:        ctx.run.Ctx,
		TypeOps:        i.types,
	})

	return inferred, prelimEngine, synthAdapter
}

func (i *Inferencer) enrichOverlayWithLocalDeclarations(
	ctx *returnInferenceContext,
	overlay map[cfg.SymbolID]typ.Type,
) map[cfg.SymbolID]bool {
	fnGraph := ctx.info.Graph
	if fnGraph == nil {
		return nil
	}

	localValueSeeds := make(map[cfg.SymbolID]bool)
	for _, assign := range ctx.info.Evidence.Assignments {
		p := assign.Point
		info := assign.Info
		if info == nil || !info.IsLocal {
			continue
		}

		if info.NumericFor != nil {
			target, ok := info.FirstTarget()
			if !ok {
				continue
			}
			if target.Kind == cfg.TargetIdent && target.Symbol != 0 {
				if _, exists := overlay[target.Symbol]; !exists {
					overlay[target.Symbol] = typ.Integer
				}
			}
		}

		if len(info.IterExprs) > 0 && len(info.Targets) > 0 {
			varTypes := ctx.engine.InferIterVarsWithSpecTypes(info.IterExprs, len(info.Targets), p, overlay)
			info.EachTarget(func(idx int, target cfg.AssignTarget) {
				if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
					return
				}
				if _, exists := overlay[target.Symbol]; exists {
					return
				}
				varType := typ.Unknown
				if idx < len(varTypes) && varTypes[idx] != nil {
					varType = varTypes[idx]
				}
				overlay[target.Symbol] = varType
			})
		}

		info.EachTarget(func(idx int, target cfg.AssignTarget) {
			if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
				return
			}
			if _, exists := overlay[target.Symbol]; exists {
				return
			}
			if ann := info.TypeAnnotationAt(idx); ann != nil {
				if resolved := ctx.engine.ResolveType(ann, ctx.resolveScope); resolved != nil {
					overlay[target.Symbol] = resolved
				}
				return
			}
			if idx < len(info.Sources) {
				if _, ok := info.Sources[idx].(*ast.TableExpr); ok {
					if seeded := ctx.engine.TypeOf(info.Sources[idx], p); seeded != nil {
						overlay[target.Symbol] = seeded
						localValueSeeds[target.Symbol] = true
					}
				}
			}
		})
	}

	if len(localValueSeeds) == 0 {
		return nil
	}
	return localValueSeeds
}

type localSymbolLookup interface {
	SymbolOf(expr *ast.IdentExpr) (cfg.SymbolID, bool)
}

type overlayMutationStage struct {
	fnGraph              *cfg.Graph
	paramSyms            map[cfg.SymbolID]bool
	localValueSeeds      map[cfg.SymbolID]bool
	finalOverlay         map[cfg.SymbolID]typ.Type
	inferred             map[cfg.SymbolID]typ.Type
	synthAdapter         func(ast.Expr, cfg.Point) typ.Type
	enrichedSynthAdapter func(ast.Expr, cfg.Point) typ.Type
}

// collectAndApplyMutations collects field/indexer assignments and applies mutations to overlay.
func (i *Inferencer) collectAndApplyMutations(
	ctx *returnInferenceContext,
	overlay map[cfg.SymbolID]typ.Type,
	inferred map[cfg.SymbolID]typ.Type,
	synthAdapter func(ast.Expr, cfg.Point) typ.Type,
	localValueSeeds map[cfg.SymbolID]bool,
) map[cfg.SymbolID]typ.Type {
	stage := newOverlayMutationStage(ctx, overlay, inferred, synthAdapter, localValueSeeds)
	mergeInferredIntoOverlay(stage.finalOverlay, stage.inferred, stage.paramSyms, stage.localValueSeeds)
	stage.enrichedSynthAdapter = buildEnrichedSynthAdapter(stage.fnGraph.Bindings(), stage.inferred, stage.finalOverlay, stage.localValueSeeds, i.types, ctx.run.Ctx, stage.synthAdapter)

	i.applyFieldMutations(ctx, &stage)
	i.applyIndexerMutations(ctx, &stage)
	i.applyDirectMutations(ctx, &stage)

	return stage.finalOverlay
}

func newOverlayMutationStage(
	ctx *returnInferenceContext,
	overlay map[cfg.SymbolID]typ.Type,
	inferred map[cfg.SymbolID]typ.Type,
	synthAdapter func(ast.Expr, cfg.Point) typ.Type,
	localValueSeeds map[cfg.SymbolID]bool,
) overlayMutationStage {
	fnGraph := (*cfg.Graph)(nil)
	if ctx != nil && ctx.info != nil {
		fnGraph = ctx.info.Graph
	}
	return overlayMutationStage{
		fnGraph:         fnGraph,
		paramSyms:       paramSymbolSet(fnGraph),
		localValueSeeds: localValueSeeds,
		finalOverlay:    cloneOverlay(overlay, len(inferred)),
		inferred:        inferred,
		synthAdapter:    synthAdapter,
	}
}

func paramSymbolSet(graph *cfg.Graph) map[cfg.SymbolID]bool {
	if graph == nil {
		return nil
	}
	paramSlots := graph.ParamSlotsReadOnly()
	if len(paramSlots) == 0 {
		return nil
	}
	out := make(map[cfg.SymbolID]bool, len(paramSlots))
	for _, slot := range paramSlots {
		if slot.Symbol != 0 {
			out[slot.Symbol] = true
		}
	}
	return out
}

func cloneOverlay(base map[cfg.SymbolID]typ.Type, extra int) map[cfg.SymbolID]typ.Type {
	out := make(map[cfg.SymbolID]typ.Type, len(base)+extra)
	for sym, t := range base {
		out[sym] = t
	}
	return out
}

func mergeInferredIntoOverlay(
	finalOverlay map[cfg.SymbolID]typ.Type,
	inferred map[cfg.SymbolID]typ.Type,
	paramSyms map[cfg.SymbolID]bool,
	localValueSeeds map[cfg.SymbolID]bool,
) {
	for sym, inferredType := range inferred {
		baseType := finalOverlay[sym]
		// Parameter domains are seeded from annotations/evidence and must not be
		// rewritten by local variable inference artifacts.
		if paramSyms[sym] {
			if typ.IsAbsentOrUnknown(baseType) {
				finalOverlay[sym] = inferredType
			}
			continue
		}
		if localValueSeeds[sym] {
			finalOverlay[sym] = inferredType
			continue
		}
		if typ.IsAbsentOrUnknown(baseType) {
			finalOverlay[sym] = inferredType
			continue
		}
		if typ.IsSoft(baseType, typ.SoftAnnotationPolicy) && inferredType != nil && !typ.IsSoft(inferredType, typ.SoftAnnotationPolicy) {
			finalOverlay[sym] = reconcileSoftAnnotatedInference(baseType, inferredType)
		}
	}
}

func extractMapComponentType(t typ.Type) (typ.Type, typ.Type, bool) {
	if t == nil {
		return nil, nil, false
	}
	switch v := t.(type) {
	case *typ.Alias:
		return extractMapComponentType(v.Target)
	case *typ.Optional:
		return extractMapComponentType(v.Inner)
	case *typ.Map:
		return v.Key, v.Value, true
	case *typ.Record:
		if v.HasMapComponent() {
			return v.MapKey, v.MapValue, true
		}
	case *typ.Union:
		var key, val typ.Type
		ok := false
		for _, member := range v.Members {
			k, vv, memberOK := extractMapComponentType(member)
			if !memberOK {
				continue
			}
			if !ok {
				key, val = k, vv
				ok = true
				continue
			}
			key = typ.JoinPreferNonSoft(key, k)
			val = overlaymut.JoinValueTypes(val, vv)
		}
		if ok {
			return key, val, true
		}
	}
	return nil, nil, false
}

func buildRecordWithMap(template *typ.Record, mapKey, mapValue typ.Type) *typ.Record {
	if template == nil {
		return nil
	}
	builder := typ.NewRecord()
	if template.Open {
		builder.SetOpen(true)
	}
	for _, f := range template.Fields {
		builder.Field(f.Name, f.Type)
	}
	if template.Metatable != nil {
		builder.Metatable(template.Metatable)
	}
	builder.MapComponent(mapKey, mapValue)
	return builder.Build()
}

func reconcileSoftAnnotatedInference(baseType, inferredType typ.Type) typ.Type {
	if baseMap, ok := baseType.(*typ.Map); ok {
		switch inferred := inferredType.(type) {
		case *typ.Map:
			mergedKey := typ.JoinPreferNonSoft(baseMap.Key, inferred.Key)
			mergedVal := typ.JoinPreferNonSoft(baseMap.Value, inferred.Value)
			return typ.NewMap(mergedKey, mergedVal)
		case *typ.Record:
			if inferred.HasMapComponent() {
				mergedKey := typ.JoinPreferNonSoft(baseMap.Key, inferred.MapKey)
				mergedVal := typ.JoinPreferNonSoft(baseMap.Value, inferred.MapValue)
				if merged := buildRecordWithMap(inferred, mergedKey, mergedVal); merged != nil {
					return merged
				}
				return typ.NewMap(mergedKey, mergedVal)
			}
			// Prefer annotated map over inferred empty record.
			return baseType
		case *typ.Union:
			if key, val, ok := extractMapComponentType(inferred); ok {
				mergedKey := typ.JoinPreferNonSoft(baseMap.Key, key)
				mergedVal := typ.JoinPreferNonSoft(baseMap.Value, val)
				return typ.NewMap(mergedKey, mergedVal)
			}
		}
	}
	if baseRec, ok := baseType.(*typ.Record); ok && baseRec.HasMapComponent() {
		switch inferred := inferredType.(type) {
		case *typ.Map:
			mergedKey := typ.JoinPreferNonSoft(baseRec.MapKey, inferred.Key)
			mergedVal := typ.JoinPreferNonSoft(baseRec.MapValue, inferred.Value)
			if merged := buildRecordWithMap(baseRec, mergedKey, mergedVal); merged != nil {
				return merged
			}
		case *typ.Record:
			mergedKey := baseRec.MapKey
			mergedVal := baseRec.MapValue
			if inferred.HasMapComponent() {
				mergedKey = typ.JoinPreferNonSoft(baseRec.MapKey, inferred.MapKey)
				mergedVal = typ.JoinPreferNonSoft(baseRec.MapValue, inferred.MapValue)
			}
			if merged := buildRecordWithMap(inferred, mergedKey, mergedVal); merged != nil {
				return merged
			}
		case *typ.Union:
			if key, val, ok := extractMapComponentType(inferred); ok {
				mergedKey := typ.JoinPreferNonSoft(baseRec.MapKey, key)
				mergedVal := typ.JoinPreferNonSoft(baseRec.MapValue, val)
				if merged := buildRecordWithMap(baseRec, mergedKey, mergedVal); merged != nil {
					return merged
				}
			}
		}
	}
	return inferredType
}

func buildEnrichedSynthAdapter(
	bindings *bind.BindingTable,
	inferred map[cfg.SymbolID]typ.Type,
	finalOverlay map[cfg.SymbolID]typ.Type,
	localValueSeeds map[cfg.SymbolID]bool,
	typeOps core.TypeOps,
	queryCtx *db.QueryContext,
	baseAdapter func(ast.Expr, cfg.Point) typ.Type,
) func(ast.Expr, cfg.Point) typ.Type {
	return func(expr ast.Expr, p cfg.Point) typ.Type {
		if ident, ok := expr.(*ast.IdentExpr); ok && bindings != nil {
			if sym, found := bindings.SymbolOf(ident); found && sym != 0 {
				if t, exists := inferred[sym]; exists && !typ.IsAbsentOrUnknown(t) {
					if localValueSeeds[sym] {
						return t
					}
					if baseType := finalOverlay[sym]; baseType != nil && !typ.IsSoft(baseType, typ.SoftAnnotationPolicy) {
						return baseType
					}
					return t
				}
			}
		}
		if t, ok := overlayPathType(expr, finalOverlay, bindings, typeOps, queryCtx); ok && !typ.IsAbsentOrUnknown(t) {
			return t
		}
		return baseAdapter(expr, p)
	}
}

func (i *Inferencer) applyFieldMutations(ctx *returnInferenceContext, stage *overlayMutationStage) {
	if i == nil || ctx == nil || stage == nil || stage.fnGraph == nil || stage.enrichedSynthAdapter == nil {
		return
	}
	fieldAssignments := overlaymut.CollectFieldAssignments(ctx.info.Evidence.Assignments, stage.enrichedSynthAdapter, nil)

	nestedBindings := stage.fnGraph.Bindings()
	if nestedBindings == nil {
		nestedBindings = i.store.ModuleBindings()
	}
	var capturedByCallee map[cfg.SymbolID]map[cfg.SymbolID]map[string]typ.Type
	if i.store != nil {
		capturedParent := api.ParentScopeForGraph(i.store, stage.fnGraph.ID(), ctx.info.DefScope)
		capturedByCallee = i.store.GetInterprocFacts(stage.fnGraph, capturedParent).CapturedFields
	}
	calleeTypeResolver := func(info *cfg.CallInfo, p cfg.Point) typ.Type {
		return resolve.CalleeType(info, p, stage.enrichedSynthAdapter, nil, nil, stage.fnGraph, nestedBindings, i.store.ModuleBindings())
	}
	nestedFieldAssignments := returns.CollectCalledNestedFieldAssignments(stage.fnGraph, nestedBindings, ctx.info.Evidence.Calls, capturedByCallee, calleeTypeResolver)
	overlaymut.MergeFieldAssignments(fieldAssignments, nestedFieldAssignments)

	overlaymut.ApplyFieldMergeToOverlay(stage.finalOverlay, fieldAssignments)
}

func (i *Inferencer) applyIndexerMutations(ctx *returnInferenceContext, stage *overlayMutationStage) {
	if i == nil || ctx == nil || ctx.info == nil || stage == nil || stage.fnGraph == nil || stage.enrichedSynthAdapter == nil {
		return
	}
	indexerBindings := stage.fnGraph.Bindings()
	if indexerBindings == nil {
		indexerBindings = i.store.ModuleBindings()
	}
	indexerAssignments := overlaymut.CollectIndexerAssignments(ctx.info.Evidence.Assignments, stage.enrichedSynthAdapter, indexerBindings, nil)
	tableMutations := calleffect.CollectTableInsertMutations(ctx.info.Evidence.Calls, stage.fnGraph, stage.enrichedSynthAdapter, indexerBindings)
	overlaymut.MergeIndexerMutations(indexerAssignments, tableMutations)
	overlaymut.ApplyIndexerMergeToOverlay(stage.finalOverlay, indexerAssignments)
}

func (i *Inferencer) applyDirectMutations(ctx *returnInferenceContext, stage *overlayMutationStage) {
	if i == nil || ctx == nil || ctx.info == nil || stage == nil || stage.fnGraph == nil || stage.enrichedSynthAdapter == nil {
		return
	}
	indexerBindings := stage.fnGraph.Bindings()
	if indexerBindings == nil {
		indexerBindings = i.store.ModuleBindings()
	}
	directMutations := calleffect.CollectTableInsertOnDirect(ctx.info.Evidence.Calls, stage.fnGraph, stage.enrichedSynthAdapter, indexerBindings)
	overlaymut.ApplyDirectMutationsToOverlay(stage.finalOverlay, directMutations)
}

// phase2InferenceState holds narrowed synthesis and dead return points.
type phase2InferenceState struct {
	synth      api.Synth
	deadPoints map[cfg.Point]bool
}

// runPhase2FlowNarrowing executes extract->solve->narrow over the final overlay.
// This makes return collection path-sensitive instead of declared-only.
func (i *Inferencer) runPhase2FlowNarrowing(
	ctx *returnInferenceContext,
	finalOverlay map[cfg.SymbolID]typ.Type,
) phase2InferenceState {
	fnGraph := ctx.info.Graph
	if fnGraph == nil {
		return phase2InferenceState{}
	}

	fnScopes := uniformFunctionScopes(fnGraph, ctx.resolveScope)

	phaseEnv := phase.PhaseEnv{
		Ctx:            ctx.run.Ctx,
		Graph:          fnGraph,
		Fn:             ctx.info.Fn,
		Types:          i.types,
		Manifests:      i.manifests,
		GlobalTypes:    i.globalTypes,
		ModuleAliases:  ctx.moduleAliases,
		ModuleBindings: i.store.ModuleBindings(),
		Evidence:       ctx.info.Evidence,
		Scopes:         fnScopes,
	}

	scopeOut := phase.ScopeOutput{
		BaseScope:     ctx.resolveScope,
		Scopes:        fnScopes,
		DeclaredTypes: finalOverlay,
		FunctionSignatureResolver: phase.FunctionSignatureResolverFunc(func(fn *ast.FunctionExpr, sc *scope.State) *typ.Function {
			return ctx.engine.ResolveFunctionSignature(fn, sc)
		}),
	}
	phaseFunctionFacts := functionfact.FromSummariesExcept(ctx.returnVectors, ctx.info.Sym)

	extractOut := phase.RunExtract(phase.FlowExtractInput{
		PhaseEnv:      phaseEnv,
		Resolve:       phase.ResolveOutput{TypeResolver: ctx.engine},
		Scope:         scopeOut,
		FunctionFacts: phaseFunctionFacts,
	})
	if extractOut.Inputs == nil {
		return phase2InferenceState{}
	}

	solveOut := phase.RunSolve(phase.FlowSolveInput{
		PhaseEnv: phaseEnv,
		Extract:  extractOut,
		Resolver: core.Resolver(),
	})

	narrowOut := phase.RunNarrow(phase.NarrowInput{
		PhaseEnv:      phaseEnv,
		Scope:         scopeOut,
		Extract:       extractOut,
		Solve:         solveOut,
		FunctionFacts: phaseFunctionFacts,
	})

	deadPoints := map[cfg.Point]bool{}
	if solveOut.Solution != nil {
		for _, ret := range extractOut.Evidence.Returns {
			if solveOut.Solution.IsPointDead(ret.Point) {
				deadPoints[ret.Point] = true
			}
		}
	}

	if narrowOut.Synth != nil {
		return phase2InferenceState{
			synth:      narrowOut.Synth,
			deadPoints: deadPoints,
		}
	}

	// Declared-phase recomputation path for uncommon nil-solution states.
	fnCheckCtx := api.NewReturnInferenceEnv(api.ReturnInferenceEnvConfig{
		Graph:         fnGraph,
		Bindings:      ctx.bindings,
		BaseScope:     ctx.resolveScope,
		DeclaredTypes: finalOverlay,
		GlobalTypes:   i.globalTypes,
		ModuleAliases: ctx.moduleAliases,
		FunctionType:  functionfact.TypeLookup(phaseFunctionFacts),
	})
	return phase2InferenceState{
		synth:      i.newReturnInferenceEngine(ctx.run, fnScopes, fnCheckCtx, ctx.info.Evidence, phaseFunctionFacts),
		deadPoints: deadPoints,
	}
}

func uniformFunctionScopes(graph *cfg.Graph, base *scope.State) map[cfg.Point]*scope.State {
	if graph == nil {
		return nil
	}
	scopes := make(map[cfg.Point]*scope.State)
	graph.EachNode(func(p cfg.Point, _ cfg.NodeInfo) {
		scopes[p] = base
	})
	scopes[graph.Entry()] = base
	return scopes
}
