// infer.go implements return type inference for local functions.
// This runs as a pre-phase before the main analysis pipeline to ensure
// return summaries are available when the parent function is analyzed.
//
// # RETURN TYPE INFERENCE
//
// Local functions (defined via "local function f()" or "local f = function()")
// require return type inference to support interprocedural analysis. The inference
// uses a two-level fixpoint:
//
//  1. SCC-level: Functions are grouped into strongly connected components (SCCs)
//     based on their call graph. Non-recursive functions have single-element SCCs.
//
//  2. Per-SCC iteration: Functions within an SCC iterate until return types
//     stabilize. Each iteration uses monotone union to combine new results
//     with previous results, ensuring convergence.
//
// # MONOTONE UNION
//
// Return type inference uses monotone union for convergence:
//   - New return types are joined with previous return types
//   - Types can only grow (become more general), never shrink
//   - Bounded iteration with widening to unknown on non-convergence
//
// # PARAM HINTS
//
// Parameter type hints are collected from call sites:
//   - When a() calls b(10), b's first param gets hint "number"
//   - Hints from multiple call sites are joined
//   - Hints propagate through the call graph (if a() calls b(), b() calls c())
//
// # SEED PROPAGATION
//
// Return summaries are seeded from the previous fixpoint iteration:
//   - Seeds provide initial return type estimates
//   - Iteration refines seeds using actual function body analysis
//   - Convergence occurs when seeds stabilize across iterations
package infer

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/assign"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/cond"
	fbcore "github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/mutator"
	"github.com/wippyai/go-lua/compiler/check/modules"
	"github.com/wippyai/go-lua/compiler/check/phase"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/siblings"
	"github.com/wippyai/go-lua/compiler/check/synth"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/join"
)

// Config holds dependencies for return inference.
type Config struct {
	Types         core.TypeOps
	GlobalTypes   map[string]typ.Type
	Manifests     io.ManifestQuerier
	Stdlib        *scope.State
	Store         api.StoreView
	Graphs        api.GraphProvider
	SourceName    string
	MaxIterations int
}

// Inferencer computes pre-flow return summaries for local functions.
type Inferencer struct {
	types         core.TypeOps
	globalTypes   map[string]typ.Type
	manifests     io.ManifestQuerier
	stdlib        *scope.State
	store         api.StoreView
	graphs        api.GraphProvider
	sourceName    string
	maxIterations int
}

// New creates a configured return inferencer.
func New(cfg Config) *Inferencer {
	maxIter := cfg.MaxIterations
	if maxIter <= 0 {
		maxIter = 10
	}
	return &Inferencer{
		types:         cfg.Types,
		globalTypes:   cfg.GlobalTypes,
		manifests:     cfg.Manifests,
		stdlib:        cfg.Stdlib,
		store:         cfg.Store,
		graphs:        cfg.Graphs,
		sourceName:    cfg.SourceName,
		maxIterations: maxIter,
	}
}

// RunContext carries per-run inputs for return inference.
type RunContext struct {
	Ctx          *db.QueryContext
	ParentFacts  flow.TypeFacts
	EffectLookup constraint.EffectLookupBySym
}

// collectLocalFunctions gathers local function definitions from assignments and FuncDef nodes.
func (i *Inferencer) collectLocalFunctions(
	graph *cfg.Graph,
	pointScopes map[cfg.Point]*scope.State,
	parentFn *ast.FunctionExpr,
) map[cfg.SymbolID]*returns.LocalFuncInfo {
	localFuncs := make(map[cfg.SymbolID]*returns.LocalFuncInfo)

	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil || !info.IsLocal || len(info.Targets) == 0 || len(info.Sources) == 0 {
			return
		}
		for idx, target := range info.Targets {
			if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
				continue
			}
			if idx >= len(info.Sources) {
				continue
			}
			fnExpr, ok := info.Sources[idx].(*ast.FunctionExpr)
			if !ok {
				continue
			}

			fnGraph := (*cfg.Graph)(nil)
			if i.graphs != nil {
				fnGraph = i.graphs.GetOrBuildCFG(fnExpr)
			}
			localFuncs[target.Symbol] = &returns.LocalFuncInfo{
				Sym:      target.Symbol,
				Fn:       fnExpr,
				DefScope: pointScopes[p],
				Graph:    fnGraph,
				ParentFn: parentFn,
				DefPoint: p,
			}
		}
	})

	graph.EachFuncDef(func(p cfg.Point, info *cfg.FuncDefInfo) {
		if info == nil || info.Symbol == 0 || info.FuncExpr == nil {
			return
		}
		if _, exists := localFuncs[info.Symbol]; exists {
			return
		}
		fnGraph := (*cfg.Graph)(nil)
		if i.graphs != nil {
			fnGraph = i.graphs.GetOrBuildCFG(info.FuncExpr)
		}
		localFuncs[info.Symbol] = &returns.LocalFuncInfo{
			Sym:      info.Symbol,
			Fn:       info.FuncExpr,
			DefScope: pointScopes[p],
			Graph:    fnGraph,
			ParentFn: parentFn,
			DefPoint: p,
		}
	})

	return localFuncs
}

// newReturnInferenceEngine creates a synthesis engine configured for return type
// inference within the pre-flow return summary computation phase.
//
// The engine operates in PhaseScopeCompute mode with:
//   - Declared types from the overlay (params, siblings, captured variables)
//   - Global types for built-in function resolution
//   - Module aliases for require() resolution
//   - Return summaries from previous iteration for recursive call resolution
//
// Unlike the main analysis engine, this engine does not have access to flow
// solution or narrowed types, producing "declared-phase" type estimates.
func (i *Inferencer) newReturnInferenceEngine(
	run RunContext,
	scopes map[cfg.Point]*scope.State,
	ctx api.DeclaredEnv,
) *synth.Engine {
	return synth.New(synth.Config{
		Ctx:            run.Ctx,
		Types:          i.types,
		Scopes:         scopes,
		Manifests:      i.manifests,
		Env:            ctx,
		Phase:          api.PhaseScopeCompute,
		ModuleBindings: i.store.ModuleBindings(),
		ModuleAliases:  i.store.ModuleAliases(),
	})
}

// computeReturnSummariesForGraph computes return summaries for local functions in a graph
// and stores them into the interproc facts for the current iteration.
func (i *Inferencer) ComputeForGraph(
	run RunContext,
	graph *cfg.Graph,
	parent *scope.State,
) (api.ReturnSummaries, []diag.Diagnostic) {
	if i == nil || i.store == nil || graph == nil || parent == nil {
		return nil, nil
	}

	parentScope := parent
	if parentHash := i.store.GraphParentHashOf(graph.ID()); parentHash != 0 {
		if storedParent := i.store.Parents()[parentHash]; storedParent != nil {
			parentScope = storedParent
		}
	}

	engine := phase.CreateTypeResolutionEngine(run.Ctx, graph, i.globalTypes, nil, parentScope, i.types, i.manifests)
	pointScopes := scope.BuildTypeDefScopes(graph, parentScope, engine.ResolveTypeDef)
	localFuncs := i.collectLocalFunctions(graph, pointScopes, graph.Func())
	if len(localFuncs) == 0 {
		return nil, nil
	}

	// Apply param hints from the stable snapshot (deterministic order).
	if hints := i.store.GetParamHintsSnapshot(graph, parentScope); len(hints) > 0 {
		for _, sym := range returns.SortedLocalFuncSymbols(localFuncs) {
			info := localFuncs[sym]
			if info == nil {
				continue
			}
			if hintVec, ok := hints[sym]; ok && len(hintVec) > 0 {
				info.ParamHints = hintVec
			}
		}
	}

	seed := i.store.GetReturnSummariesSnapshot(graph, parentScope)
	summaries, diags := i.computeReturnSummariesForGroup(run, parentScope.GroupHash(), localFuncs, seed)
	return summaries, diags
}

// computeReturnSummariesForGroup computes return type summaries for a scope group
// using strongly connected component (SCC) based fixpoint iteration.
//
// SCC ORDERING: Functions are partitioned into SCCs by their call graph. SCCs are
// processed in topological order so that callees are resolved before callers.
// This minimizes iteration count for non-recursive function groups.
//
// PER-SCC ITERATION: Within each SCC (which may contain mutually recursive functions),
// a fixpoint loop iterates until return types stabilize:
//   - Each iteration computes new return types from function bodies
//   - New types are joined with previous types via monotone union
//   - Iteration stops when no type changes
//
// WIDENING: If SCC iteration exceeds MaxReturnSummaryIterations, types are widened
// to unknown to guarantee termination. A diagnostic is emitted for the non-convergence.
//
// SEEDING: Initial return type estimates come from the seed map (previous fixpoint
// iteration). This accelerates convergence for iteratively-refined modules.
func (i *Inferencer) computeReturnSummariesForGroup(
	run RunContext,
	groupHash uint64,
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
	seed map[cfg.SymbolID][]typ.Type,
) (map[cfg.SymbolID][]typ.Type, []diag.Diagnostic) {
	if len(localFuncs) == 0 {
		return nil, nil
	}

	// Build call graph
	adj := returns.BuildLocalCallGraph(localFuncs, i.store.ModuleBindings())

	// Compute SCCs in topological order
	sccs := returns.ComputeSymbolSCCs(adj)
	if len(sccs) == 0 {
		return nil, nil
	}

	// Initialize summaries from seed (if any).
	summaries := make(map[cfg.SymbolID][]typ.Type, len(localFuncs))
	for _, sym := range returns.SortedLocalFuncSymbols(localFuncs) {
		if seed != nil {
			if t := seed[sym]; len(t) > 0 {
				summaries[sym] = t
				continue
			}
		}
	}

	var diags []diag.Diagnostic

	// Process each SCC in topological order.
	for _, scc := range sccs {
		if len(scc) == 0 {
			continue
		}
		converged := i.iterateSCCFixpoint(run, scc, localFuncs, summaries)
		if !converged {
			if warn := i.widenSCCToUnknown(scc, localFuncs, summaries); warn != nil {
				diags = append(diags, *warn)
			}
		}
	}

	return summaries, diags
}

// iterateSCCFixpoint runs fixpoint iteration for a single SCC until convergence.
// Returns true if types stabilized within the iteration limit.
func (i *Inferencer) iterateSCCFixpoint(
	run RunContext,
	scc []cfg.SymbolID,
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
	summaries map[cfg.SymbolID][]typ.Type,
) bool {
	for iter := 0; iter < i.maxIterations; iter++ {
		changed := false
		next := make(map[cfg.SymbolID][]typ.Type, len(scc))

		for _, sym := range scc {
			info := localFuncs[sym]
			if info == nil || info.Fn == nil {
				continue
			}
			newReturn := i.inferReturnWithSummary(run, info, summaries, localFuncs)
			oldReturn := summaries[sym]
			merged := returns.JoinReturnVectorsPreferNonSoft(oldReturn, newReturn)
			if returns.ReturnTypesRefine(newReturn, oldReturn) {
				merged = newReturn
			} else if returns.ReturnTypesRefine(oldReturn, newReturn) {
				merged = oldReturn
			} else if returns.ReturnTypesExtendRecord(newReturn, oldReturn) {
				merged = newReturn
			} else if returns.ReturnTypesExtendRecord(oldReturn, newReturn) {
				merged = oldReturn
			}
			next[sym] = merged
			if !returns.ReturnTypesEqual(merged, oldReturn) {
				changed = true
			}
		}

		for _, sym := range scc {
			if v, ok := next[sym]; ok {
				summaries[sym] = v
			}
		}

		if !changed {
			return true
		}
	}
	return false
}

// widenSCCToUnknown widens all SCC members to unknown when fixpoint did not converge.
// Preserves return arity while replacing type slots with unknown.
func (i *Inferencer) widenSCCToUnknown(
	scc []cfg.SymbolID,
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
	summaries map[cfg.SymbolID][]typ.Type,
) *diag.Diagnostic {
	for _, sym := range scc {
		existing := summaries[sym]
		if len(existing) == 0 {
			summaries[sym] = []typ.Type{typ.Unknown}
		} else {
			widened := make([]typ.Type, len(existing))
			for i := range widened {
				widened[i] = typ.Unknown
			}
			summaries[sym] = widened
		}
	}
	if info := localFuncs[scc[0]]; info != nil && info.Fn != nil {
		return &diag.Diagnostic{
			Position: diag.Position{File: i.sourceName, Line: info.Fn.Line(), Column: info.Fn.Column()},
			Span:     ast.SpanOf(info.Fn),
			Severity: diag.SeverityWarning,
			Message:  "return type fixpoint did not converge; using unknown",
		}
	}
	return nil
}

// returnInferenceContext holds shared state for return type inference phases.
type returnInferenceContext struct {
	run           RunContext
	info          *returns.LocalFuncInfo
	summaries     map[cfg.SymbolID][]typ.Type
	localFuncs    map[cfg.SymbolID]*returns.LocalFuncInfo
	engine        *synth.Engine
	resolveScope  *scope.State
	moduleAliases map[cfg.SymbolID]string
	bindings      *bind.BindingTable
	parentFacts   flow.TypeFacts
}

// buildParameterOverlay creates the initial type overlay with parameter types.
// Parameters are typed from annotations, hints, or default to unknown.
func (i *Inferencer) buildParameterOverlay(ctx *returnInferenceContext) map[cfg.SymbolID]typ.Type {
	overlay := make(map[cfg.SymbolID]typ.Type)
	fnGraph := ctx.info.Graph
	for _, slot := range fnGraph.ParamSlots() {
		if slot.Symbol == 0 {
			continue
		}

		// Binder/CFG-injected implicit self parameter.
		if slot.SourceIndex < 0 {
			if selfType := ctx.resolveScope.SelfType(); selfType != nil {
				overlay[slot.Symbol] = selfType
			} else {
				overlay[slot.Symbol] = typ.Unknown
			}
			continue
		}

		i := slot.SourceIndex
		paramType := typ.Unknown
		if slot.Name == "self" {
			if selfType := ctx.resolveScope.SelfType(); selfType != nil {
				paramType = selfType
			}
		}
		if paramType == nil || paramType.Kind() == typ.Unknown.Kind() {
			if ctx.info.ParamHints != nil && i < len(ctx.info.ParamHints) && ctx.info.ParamHints[i] != nil {
				paramType = ctx.info.ParamHints[i]
			}
		}
		if slot.TypeAnnotation != nil {
			resolved := ctx.engine.ResolveType(slot.TypeAnnotation, ctx.resolveScope)
			if resolved != nil {
				if typ.IsRefinableAnnotation(resolved) {
					if paramType == nil || paramType.Kind() == typ.Unknown.Kind() {
						paramType = resolved
					}
				} else {
					paramType = resolved
				}
			}
		}
		overlay[slot.Symbol] = paramType
	}
	return overlay
}

// enrichOverlayWithSiblings adds sibling function types to the overlay using summaries.
func (i *Inferencer) enrichOverlayWithSiblings(
	ctx *returnInferenceContext,
	overlay map[cfg.SymbolID]typ.Type,
) {
	siblingEntries := make([]siblings.OverlayEntry, 0, len(ctx.localFuncs))
	for _, sym := range returns.SortedLocalFuncSymbols(ctx.localFuncs) {
		sibInfo := ctx.localFuncs[sym]
		if sibInfo != nil && sibInfo.Fn != nil {
			siblingEntries = append(siblingEntries, siblings.OverlayEntry{
				Symbol: sym,
				Func:   sibInfo.Fn,
			})
		}
	}
	siblingOverlay := siblings.BuildOverlay(siblings.OverlayConfig{
		Summaries:  ctx.summaries,
		Siblings:   siblingEntries,
		CurrentSym: ctx.info.Sym,
		Services: siblings.OverlayServicesFuncs{
			SeedTypeFn: func(fn *ast.FunctionExpr) typ.Type {
				return returns.BuildSeedFunctionType(fn, ctx.engine, ctx.resolveScope)
			},
		},
	})
	for sym, ty := range siblingOverlay {
		overlay[sym] = ty
	}
}

// collectAllReturnSummaries gathers return summaries from all scope groups.
func (i *Inferencer) collectAllReturnSummaries(ctx *returnInferenceContext) map[cfg.SymbolID][]typ.Type {
	allSummaries := make(map[cfg.SymbolID][]typ.Type)
	for _, sym := range sortedSummarySymbols(ctx.summaries) {
		t := ctx.summaries[sym]
		if sym == 0 || len(t) == 0 {
			continue
		}
		if existing, ok := allSummaries[sym]; ok {
			allSummaries[sym] = returns.JoinReturnVectorsPreferNonSoft(existing, t)
		} else {
			allSummaries[sym] = t
		}
	}
	return allSummaries
}

func sortedSummarySymbols(summaries map[cfg.SymbolID][]typ.Type) []cfg.SymbolID {
	if len(summaries) == 0 {
		return nil
	}
	syms := make([]cfg.SymbolID, 0, len(summaries))
	for sym := range summaries {
		syms = append(syms, sym)
	}
	sort.Slice(syms, func(i, j int) bool {
		return syms[i] < syms[j]
	})
	return syms
}

// enrichOverlayWithLocalFunctions adds local function types from the function body.
func (i *Inferencer) enrichOverlayWithLocalFunctions(
	ctx *returnInferenceContext,
	overlay map[cfg.SymbolID]typ.Type,
	allSummaries map[cfg.SymbolID][]typ.Type,
) {
	isUnknownSummary := func(s []typ.Type) bool {
		if len(s) == 0 {
			return true
		}
		for _, t := range s {
			if t != nil && !typ.TypeEquals(t, typ.Unknown) {
				return false
			}
		}
		return true
	}
	ctx.info.Graph.EachAssign(func(p cfg.Point, assignInfo *cfg.AssignInfo) {
		if assignInfo == nil || !assignInfo.IsLocal || len(assignInfo.Targets) == 0 || len(assignInfo.Sources) == 0 {
			return
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
			summary := allSummaries[target.Symbol]
			if isUnknownSummary(summary) && i.store != nil {
				if ctx.info != nil && ctx.info.Graph != nil && ctx.resolveScope != nil {
					if snap := i.store.GetReturnSummariesSnapshot(ctx.info.Graph, ctx.resolveScope); len(snap) > 0 {
						if snapSummary := snap[target.Symbol]; len(snapSummary) > 0 {
							summary = snapSummary
						}
					}
				}
				if len(summary) == 0 {
					if ref := i.store.FunctionRefBySym(target.Symbol); ref != nil && ref.ParentGraphID != 0 {
						parentGraph := i.store.Graphs()[ref.ParentGraphID]
						if parentGraph != nil {
							if parentHash := i.store.GraphParentHashOf(parentGraph.ID()); parentHash != 0 {
								if parent := i.store.Parents()[parentHash]; parent != nil {
									if snap := i.store.GetReturnSummariesSnapshot(parentGraph, parent); len(snap) > 0 {
										summary = snap[target.Symbol]
									}
								}
							}
						}
					}
				}
			}
			sig := ctx.engine.ResolveFunctionSignature(fnExpr, ctx.resolveScope)
			if fnType := returns.BuildFunctionSignatureWithSummary(sig, summary); fnType != nil {
				overlay[target.Symbol] = fnType
			}
		}
	})
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
		parentScope := ctx.info.DefScope
		if parentHash := i.store.GraphParentHashOf(ctx.info.Graph.ID()); parentHash != 0 {
			if storedParent := i.store.Parents()[parentHash]; storedParent != nil {
				parentScope = storedParent
			}
		}
		if capturedTypes := i.store.GetCapturedTypesSnapshot(ctx.info.Graph, parentScope); len(capturedTypes) > 0 {
			for _, sym := range cfg.SortedSymbolIDs(capturedTypes) {
				t := capturedTypes[sym]
				if sym == 0 || t == nil {
					continue
				}
				if _, ok := overlay[sym]; ok {
					continue
				}
				overlay[sym] = t
			}
		}
	}
	if ctx.parentFacts == nil {
		return
	}
	for _, sym := range localBindings.CapturedSymbols(ctx.info.Fn) {
		if sym == 0 {
			continue
		}
		if _, ok := overlay[sym]; ok {
			continue
		}
		if tv := ctx.parentFacts.EffectiveTypeAt(defPoint, sym); tv.State == flow.StateResolved && tv.Type != nil {
			overlay[sym] = tv.Type
		}
	}
}

// inferLocalVariableTypes runs phase 1 synthesis to infer local variable types.
func (i *Inferencer) inferLocalVariableTypes(
	ctx *returnInferenceContext,
	overlay map[cfg.SymbolID]typ.Type,
) (map[cfg.SymbolID]typ.Type, *synth.Engine, func(ast.Expr, cfg.Point) typ.Type) {
	fnGraph := ctx.info.Graph
	annotated := make(map[cfg.SymbolID]bool, len(overlay))
	paramSet := make(map[cfg.SymbolID]bool)
	for _, sym := range fnGraph.ParamSymbols() {
		if sym != 0 {
			paramSet[sym] = true
		}
	}
	for sym, tp := range overlay {
		if paramSet[sym] {
			annotated[sym] = true
			continue
		}
		if tp != nil && !typ.IsSoft(tp, typ.SoftAnnotationPolicy) {
			annotated[sym] = true
		}
	}

	fnGraph.EachAssign(func(_ cfg.Point, assignInfo *cfg.AssignInfo) {
		if assignInfo == nil || len(assignInfo.TypeAnnotations) == 0 {
			return
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
	})

	fnScopes := make(map[cfg.Point]*scope.State)
	fnGraph.EachNode(func(p cfg.Point, _ cfg.NodeInfo) {
		fnScopes[p] = ctx.resolveScope
	})
	fnScopes[fnGraph.Entry()] = ctx.resolveScope

	prelimCtx := api.NewReturnInferenceEnv(api.ReturnInferenceEnvConfig{
		Graph:           fnGraph,
		Bindings:        ctx.bindings,
		BaseScope:       ctx.resolveScope,
		DeclaredTypes:   overlay,
		GlobalTypes:     i.globalTypes,
		ModuleAliases:   ctx.moduleAliases,
		ReturnSummaries: ctx.summaries,
	})

	prelimEngine := i.newReturnInferenceEngine(ctx.run, fnScopes, prelimCtx)

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

	inferred := assign.CollectInferredTypes(&fbcore.FlowContext{
		Graph:   fnGraph,
		Scopes:  fnScopes,
		API:     prelimEngine,
		CallCtx: ctx.run.Ctx,
		TypeOps: i.types,
		Derived: &fbcore.Derived{
			SymResolver: symResolver,
		},
	}, overlay, annotated, nil)

	return inferred, prelimEngine, synthAdapter
}

func (i *Inferencer) enrichOverlayWithLocalDeclarations(
	ctx *returnInferenceContext,
	overlay map[cfg.SymbolID]typ.Type,
) {
	fnGraph := ctx.info.Graph
	if fnGraph == nil {
		return
	}

	fnGraph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil || !info.IsLocal {
			return
		}

		if info.NumericFor != nil && len(info.Targets) > 0 {
			target := info.Targets[0]
			if target.Kind == cfg.TargetIdent && target.Symbol != 0 {
				if _, exists := overlay[target.Symbol]; !exists {
					overlay[target.Symbol] = typ.Integer
				}
			}
		}

		if len(info.IterExprs) > 0 && len(info.Targets) > 0 {
			varTypes := ctx.engine.InferIterVarsWithSpecTypes(info.IterExprs, len(info.Targets), p, overlay)
			for idx, target := range info.Targets {
				if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
					continue
				}
				if _, exists := overlay[target.Symbol]; exists {
					continue
				}
				varType := typ.Unknown
				if idx < len(varTypes) && varTypes[idx] != nil {
					varType = varTypes[idx]
				}
				overlay[target.Symbol] = varType
			}
		}

		for idx, target := range info.Targets {
			if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
				continue
			}
			if _, exists := overlay[target.Symbol]; exists {
				continue
			}
			if info.TypeAnnotations != nil && idx < len(info.TypeAnnotations) && info.TypeAnnotations[idx] != nil {
				if resolved := ctx.engine.ResolveType(info.TypeAnnotations[idx], ctx.resolveScope); resolved != nil {
					overlay[target.Symbol] = resolved
				}
			}
		}
	})

}

// collectAndApplyMutations collects field/indexer assignments and applies mutations to overlay.
func (i *Inferencer) collectAndApplyMutations(
	ctx *returnInferenceContext,
	overlay map[cfg.SymbolID]typ.Type,
	inferred map[cfg.SymbolID]typ.Type,
	synthAdapter func(ast.Expr, cfg.Point) typ.Type,
) map[cfg.SymbolID]typ.Type {
	fnGraph := ctx.info.Graph
	localBindings := fnGraph.Bindings()
	var extractMapComponent func(t typ.Type) (typ.Type, typ.Type, bool)
	extractMapComponent = func(t typ.Type) (typ.Type, typ.Type, bool) {
		if t == nil {
			return nil, nil, false
		}
		switch v := t.(type) {
		case *typ.Alias:
			return extractMapComponent(v.Target)
		case *typ.Optional:
			return extractMapComponent(v.Inner)
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
				k, vv, memberOK := extractMapComponent(member)
				if !memberOK {
					continue
				}
				if !ok {
					key, val = k, vv
					ok = true
					continue
				}
				key = typ.JoinPreferNonSoft(key, k)
				val = returns.JoinValueTypes(val, vv)
			}
			if ok {
				return key, val, true
			}
		}
		return nil, nil, false
	}
	finalOverlay := make(map[cfg.SymbolID]typ.Type, len(overlay)+len(inferred))
	for sym, t := range overlay {
		finalOverlay[sym] = t
	}
	for sym, t := range inferred {
		baseType := finalOverlay[sym]
		if baseType == nil || baseType.Kind() == kind.Unknown {
			finalOverlay[sym] = t
			continue
		}
		if typ.IsSoft(baseType, typ.SoftAnnotationPolicy) && t != nil && !typ.IsSoft(t, typ.SoftAnnotationPolicy) {
			if baseMap, ok := baseType.(*typ.Map); ok {
				if inferredMap, ok := t.(*typ.Map); ok {
					mergedKey := typ.JoinPreferNonSoft(baseMap.Key, inferredMap.Key)
					mergedVal := typ.JoinPreferNonSoft(baseMap.Value, inferredMap.Value)
					finalOverlay[sym] = typ.NewMap(mergedKey, mergedVal)
					continue
				}
				if inferredRec, ok := t.(*typ.Record); ok {
					if inferredRec.HasMapComponent() {
						mergedKey := typ.JoinPreferNonSoft(baseMap.Key, inferredRec.MapKey)
						mergedVal := typ.JoinPreferNonSoft(baseMap.Value, inferredRec.MapValue)
						builder := typ.NewRecord()
						if inferredRec.Open {
							builder.SetOpen(true)
						}
						for _, f := range inferredRec.Fields {
							builder.Field(f.Name, f.Type)
						}
						if inferredRec.Metatable != nil {
							builder.Metatable(inferredRec.Metatable)
						}
						builder.MapComponent(mergedKey, mergedVal)
						finalOverlay[sym] = builder.Build()
						continue
					}
					// Prefer annotated map over inferred empty record.
					finalOverlay[sym] = baseType
					continue
				}
				if inferredUnion, ok := t.(*typ.Union); ok {
					if key, val, ok := extractMapComponent(inferredUnion); ok {
						mergedKey := typ.JoinPreferNonSoft(baseMap.Key, key)
						mergedVal := typ.JoinPreferNonSoft(baseMap.Value, val)
						finalOverlay[sym] = typ.NewMap(mergedKey, mergedVal)
						continue
					}
				}
			}
			if baseRec, ok := baseType.(*typ.Record); ok && baseRec.HasMapComponent() {
				switch inferred := t.(type) {
				case *typ.Map:
					mergedKey := typ.JoinPreferNonSoft(baseRec.MapKey, inferred.Key)
					mergedVal := typ.JoinPreferNonSoft(baseRec.MapValue, inferred.Value)
					builder := typ.NewRecord()
					if baseRec.Open {
						builder.SetOpen(true)
					}
					for _, f := range baseRec.Fields {
						builder.Field(f.Name, f.Type)
					}
					if baseRec.Metatable != nil {
						builder.Metatable(baseRec.Metatable)
					}
					builder.MapComponent(mergedKey, mergedVal)
					finalOverlay[sym] = builder.Build()
					continue
				case *typ.Record:
					mergedKey := baseRec.MapKey
					mergedVal := baseRec.MapValue
					if inferred.HasMapComponent() {
						mergedKey = typ.JoinPreferNonSoft(baseRec.MapKey, inferred.MapKey)
						mergedVal = typ.JoinPreferNonSoft(baseRec.MapValue, inferred.MapValue)
					}
					builder := typ.NewRecord()
					if inferred.Open {
						builder.SetOpen(true)
					}
					for _, f := range inferred.Fields {
						builder.Field(f.Name, f.Type)
					}
					if inferred.Metatable != nil {
						builder.Metatable(inferred.Metatable)
					}
					builder.MapComponent(mergedKey, mergedVal)
					finalOverlay[sym] = builder.Build()
					continue
				case *typ.Union:
					if key, val, ok := extractMapComponent(inferred); ok {
						mergedKey := typ.JoinPreferNonSoft(baseRec.MapKey, key)
						mergedVal := typ.JoinPreferNonSoft(baseRec.MapValue, val)
						builder := typ.NewRecord()
						if baseRec.Open {
							builder.SetOpen(true)
						}
						for _, f := range baseRec.Fields {
							builder.Field(f.Name, f.Type)
						}
						if baseRec.Metatable != nil {
							builder.Metatable(baseRec.Metatable)
						}
						builder.MapComponent(mergedKey, mergedVal)
						finalOverlay[sym] = builder.Build()
						continue
					}
				}
			}
			finalOverlay[sym] = t
			continue
		}
	}

	enrichedSynthAdapter := func(expr ast.Expr, p cfg.Point) typ.Type {
		if ident, ok := expr.(*ast.IdentExpr); ok && localBindings != nil {
			if sym, found := localBindings.SymbolOf(ident); found && sym != 0 {
				if t, exists := inferred[sym]; exists && t != nil && t.Kind() != kind.Unknown {
					if baseType := finalOverlay[sym]; baseType != nil && !typ.IsSoft(baseType, typ.SoftAnnotationPolicy) {
						return baseType
					}
					return t
				}
			}
		}
		return synthAdapter(expr, p)
	}

	fieldAssignments := assign.CollectFieldAssignments(fnGraph, enrichedSynthAdapter, nil)
	nestedBindings := fnGraph.Bindings()
	if nestedBindings == nil {
		nestedBindings = i.store.ModuleBindings()
	}
	var capturedByCallee map[cfg.SymbolID]map[cfg.SymbolID]map[string]typ.Type
	if i.store != nil {
		capturedParent := ctx.info.DefScope
		if parentHash := i.store.GraphParentHashOf(fnGraph.ID()); parentHash != 0 {
			if parentScope := i.store.Parents()[parentHash]; parentScope != nil {
				capturedParent = parentScope
			}
		}
		capturedByCallee = i.store.GetCapturedFieldAssignsSnapshot(fnGraph, capturedParent)
	}
	nestedFieldAssignments := returns.CollectCalledNestedFieldAssignments(fnGraph, nestedBindings, capturedByCallee)
	returns.MergeFieldAssignments(fieldAssignments, nestedFieldAssignments)

	returns.ApplyFieldMergeToOverlay(finalOverlay, fieldAssignments)

	indexerBindings := fnGraph.Bindings()
	if indexerBindings == nil {
		indexerBindings = i.store.ModuleBindings()
	}
	indexerAssignments := assign.CollectIndexerAssignments(fnGraph, enrichedSynthAdapter, indexerBindings, nil)

	tableMutations := mutator.CollectTableInsertMutations(fnGraph, enrichedSynthAdapter, indexerBindings)
	mutator.MergeIndexerMutations(indexerAssignments, tableMutations)

	returns.ApplyIndexerMergeToOverlay(finalOverlay, indexerAssignments)

	directMutations := mutator.CollectTableInsertOnDirect(fnGraph, enrichedSynthAdapter, indexerBindings)
	returns.ApplyDirectMutationsToOverlay(finalOverlay, directMutations)

	return finalOverlay
}

// phase2InferenceState holds the synthesis engine and dead points for phase 2.
type phase2InferenceState struct {
	engine     *synth.Engine
	deadPoints map[cfg.Point]bool
}

// buildPhase2Context creates the synthesis context for phase 2 return type inference.
func (i *Inferencer) buildPhase2Context(
	ctx *returnInferenceContext,
	finalOverlay map[cfg.SymbolID]typ.Type,
	synthAdapter func(ast.Expr, cfg.Point) typ.Type,
) phase2InferenceState {
	fnGraph := ctx.info.Graph

	fnScopes := make(map[cfg.Point]*scope.State)
	fnGraph.EachNode(func(p cfg.Point, _ cfg.NodeInfo) {
		fnScopes[p] = ctx.resolveScope
	})
	fnScopes[fnGraph.Entry()] = ctx.resolveScope

	fnCheckCtx := api.NewReturnInferenceEnv(api.ReturnInferenceEnvConfig{
		Graph:           fnGraph,
		Bindings:        ctx.bindings,
		BaseScope:       ctx.resolveScope,
		DeclaredTypes:   finalOverlay,
		GlobalTypes:     i.globalTypes,
		ModuleAliases:   ctx.moduleAliases,
		ReturnSummaries: ctx.summaries,
	})

	synthEngine := i.newReturnInferenceEngine(ctx.run, fnScopes, fnCheckCtx)

	effectLookupSym := ctx.run.EffectLookup
	symResolver := func(p cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		if fnCheckCtx == nil || fnCheckCtx.Types() == nil {
			return nil, false
		}
		tv := fnCheckCtx.Types().EffectiveTypeAt(p, sym)
		if tv.State == flow.StateResolved && tv.Type != nil {
			return tv.Type, true
		}
		if t, ok := fnCheckCtx.GlobalType(sym); ok && t != nil {
			return t, true
		}
		return nil, false
	}
	deadPoints := cond.ComputeDeadPoints(fnGraph, synthAdapter, symResolver, effectLookupSym)

	return phase2InferenceState{
		engine:     synthEngine,
		deadPoints: deadPoints,
	}
}

// collectReturnTypes gathers and joins return types from all live return points.
func collectReturnTypes(
	fnGraph *cfg.Graph,
	synthEngine *synth.Engine,
	deadPoints map[cfg.Point]bool,
) []typ.Type {
	var returnTypes []typ.Type
	seenReturn := false

	fnGraph.EachReturn(func(p cfg.Point, retInfo *cfg.ReturnInfo) {
		if retInfo == nil || deadPoints[p] {
			return
		}

		types := synthesizeReturnExprs(synthEngine, retInfo, p)

		if !seenReturn {
			seenReturn = true
			returnTypes = types
			return
		}

		returnTypes = joinReturnTypes(returnTypes, types)
	})

	return returns.NormalizeReturnVector(returnTypes)
}

// synthesizeReturnExprs computes types for a single return statement's expressions.
func synthesizeReturnExprs(
	synthEngine *synth.Engine,
	retInfo *cfg.ReturnInfo,
	p cfg.Point,
) []typ.Type {
	if len(retInfo.Exprs) == 0 {
		return nil
	}

	var types []typ.Type
	for i, expr := range retInfo.Exprs {
		if i == len(retInfo.Exprs)-1 {
			multi := synthEngine.MultiTypeOf(expr, p)
			if len(multi) == 0 {
				multi = []typ.Type{typ.Unknown}
			} else {
				for j, mt := range multi {
					if mt == nil {
						multi[j] = typ.Unknown
					}
				}
			}
			types = append(types, multi...)
		} else {
			t := synthEngine.TypeOf(expr, p)
			if t == nil {
				t = typ.Unknown
			}
			types = append(types, t)
		}
	}
	return types
}

// joinReturnTypes merges two return type vectors using union semantics.
func joinReturnTypes(existing, incoming []typ.Type) []typ.Type {
	for len(existing) < len(incoming) {
		existing = append(existing, typ.Nil)
	}
	for i := range existing {
		var t typ.Type
		if i < len(incoming) {
			t = incoming[i]
		} else {
			t = typ.Nil
		}
		existing[i] = join.Two(existing[i], t)
	}
	return existing
}

// inferReturnTypesFromBody runs phase 2 synthesis to compute return types.
func (i *Inferencer) inferReturnTypesFromBody(
	ctx *returnInferenceContext,
	finalOverlay map[cfg.SymbolID]typ.Type,
	synthAdapter func(ast.Expr, cfg.Point) typ.Type,
) []typ.Type {
	state := i.buildPhase2Context(ctx, finalOverlay, synthAdapter)
	return collectReturnTypes(ctx.info.Graph, state.engine, state.deadPoints)
}

// inferReturnWithSummary infers return types for a single function using available summaries.
// This is the core inference logic called by computeReturnSummariesForGroup for each function.
//
// TWO-PHASE INFERENCE:
//
// Phase 1 (Preliminary): Collect inferred types for local variables within the function.
// This uses a preliminary synthesis engine with:
//   - Parameter types (from annotations or param hints)
//   - Sibling function types (from summaries)
//   - Captured variable types (from parent function result)
//
// Phase 2 (Final): Compute return types using enriched overlay containing:
//   - All Phase 1 inferred types
//   - Field assignments merged into table types
//   - Indexer assignments merged into map components
//   - Table mutation effects (table.insert, etc.)
//
// EXPLICIT ANNOTATIONS: If the function has explicit @return annotations, those are
// used directly without body inference.
//
// MULTI-RETURN: Functions may return multiple values. The inference handles multi-return
// by expanding the last expression (which may be a call or vararg) and joining position-wise.
func (i *Inferencer) inferReturnWithSummary(
	run RunContext,
	info *returns.LocalFuncInfo,
	summaries map[cfg.SymbolID][]typ.Type,
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
) []typ.Type {
	if info == nil || info.Fn == nil || info.Graph == nil {
		return nil
	}

	fn := info.Fn
	fnGraph := info.Graph
	parentScope := info.DefScope
	moduleAliases := modules.MergeAliases(i.store.ModuleAliases(), modules.CollectAliases(fnGraph))

	engine := phase.CreateTypeResolutionEngine(run.Ctx, fnGraph, i.globalTypes, nil, parentScope, i.types, i.manifests)

	resolveScope := parentScope
	if len(fn.TypeParams) > 0 {
		typeParams := make(map[string]typ.Type, len(fn.TypeParams))
		for _, tp := range fn.TypeParams {
			var constr typ.Type
			if tp.Constraint != nil {
				constr = engine.ResolveType(tp.Constraint, resolveScope)
			}
			typeParams[tp.Name] = typ.NewTypeParam(tp.Name, constr)
		}
		resolveScope = resolveScope.WithTypeParams(typeParams)
	}

	// Check for explicit return type annotations.
	if len(fn.ReturnTypes) > 0 {
		rets := engine.ResolveReturnTypes(fn.ReturnTypes, resolveScope)
		if len(rets) > 0 {
			return rets
		}
	}

	// Resolve bindings for this function.
	bindings := fnGraph.Bindings()
	if bindings == nil && i.store != nil {
		bindings = i.store.ModuleBindings()
	}

	// Build inference context shared across all phases.
	ctx := &returnInferenceContext{
		run:           run,
		info:          info,
		summaries:     summaries,
		localFuncs:    localFuncs,
		engine:        engine,
		resolveScope:  resolveScope,
		moduleAliases: moduleAliases,
		bindings:      bindings,
		parentFacts:   run.ParentFacts,
	}

	// Build type overlay with parameter types.
	overlay := i.buildParameterOverlay(ctx)

	// Add sibling function types from summaries.
	i.enrichOverlayWithSiblings(ctx, overlay)

	// Collect all return summaries and add local function types.
	allSummaries := i.collectAllReturnSummaries(ctx)
	i.enrichOverlayWithLocalFunctions(ctx, overlay, allSummaries)

	// Add captured variable types from parent.
	i.enrichOverlayWithCaptured(ctx, overlay)

	// Add local declared types (annotations, loop variables) as overlay hints.
	i.enrichOverlayWithLocalDeclarations(ctx, overlay)

	// Phase 1: Infer local variable types.
	inferred, _, synthAdapter := i.inferLocalVariableTypes(ctx, overlay)

	// Collect field/indexer assignments and apply mutations.
	finalOverlay := i.collectAndApplyMutations(ctx, overlay, inferred, synthAdapter)

	// Phase 2: Infer return types from body.
	return i.inferReturnTypesFromBody(ctx, finalOverlay, synthAdapter)
}
