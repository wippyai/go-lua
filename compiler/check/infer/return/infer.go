// infer.go implements return type inference for local functions.
// This runs as a pre-phase before the main analysis pipeline to ensure
// return vectors are available when the parent function is analyzed.
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
//   - Recursive SCCs use convergence widening, so iteration is governed by
//     domain stabilization rather than an artificial budget
//
// # PARAMETER EVIDENCE
//
// Parameter evidence is collected from call sites:
//   - When a() calls b(10), b's first param records number evidence.
//   - Multiple call sites are joined.
//   - Evidence propagates through the call graph (if a() calls b(), b() calls c()).
//
// # SEED PROPAGATION
//
// Return vectors are seeded from the previous fixpoint iteration:
//   - Seeds provide initial return type estimates
//   - Iteration refines seeds using actual function body analysis
//   - Convergence occurs when seeds stabilize across iterations
package infer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	flowpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/domain/returnsummary"
	"github.com/wippyai/go-lua/compiler/check/modules"
	"github.com/wippyai/go-lua/compiler/check/phase"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// Config holds dependencies for return inference.
type Config struct {
	Types       core.TypeOps
	GlobalTypes map[string]typ.Type
	Manifests   io.ManifestQuerier
	Stdlib      *scope.State
	Store       api.StoreReader
	Graphs      api.GraphProvider
	SourceName  string
}

// Inferencer computes pre-flow return vectors for local functions.
type Inferencer struct {
	types       core.TypeOps
	globalTypes map[string]typ.Type
	manifests   io.ManifestQuerier
	stdlib      *scope.State
	store       api.StoreReader
	graphs      api.GraphProvider
	sourceName  string
}

// New creates a configured return inferencer.
func New(cfg Config) *Inferencer {
	return &Inferencer{
		types:       cfg.Types,
		globalTypes: cfg.GlobalTypes,
		manifests:   cfg.Manifests,
		stdlib:      cfg.Stdlib,
		store:       cfg.Store,
		graphs:      cfg.Graphs,
		sourceName:  cfg.SourceName,
	}
}

// RunContext carries per-run inputs for return inference.
type RunContext struct {
	Ctx          *db.QueryContext
	ParentFacts  flow.TypeFacts
	EffectLookup constraint.RefinementLookupBySym
}

// collectLocalFunctions gathers function definitions from transfer evidence.
func (i *Inferencer) collectLocalFunctions(
	graph *cfg.Graph,
	pointScopes map[cfg.Point]*scope.State,
	parentFn *ast.FunctionExpr,
) map[cfg.SymbolID]*returns.LocalFuncInfo {
	localFuncs := make(map[cfg.SymbolID]*returns.LocalFuncInfo)
	parentEvidence := i.transferEvidenceForGraph(graph)

	for _, def := range parentEvidence.FunctionDefinitions {
		if def.Symbol == 0 || def.Nested.Func == nil {
			continue
		}
		if _, exists := localFuncs[def.Symbol]; exists {
			continue
		}
		fnGraph := (*cfg.Graph)(nil)
		if i.graphs != nil {
			fnGraph = i.graphs.GetOrBuildCFG(def.Nested.Func)
		}
		localFuncs[def.Symbol] = &returns.LocalFuncInfo{
			Sym:            def.Symbol,
			Fn:             def.Nested.Func,
			DefScope:       pointScopes[def.Nested.Point],
			Graph:          fnGraph,
			ParentGraph:    graph,
			ParentFn:       parentFn,
			DefPoint:       def.Nested.Point,
			Evidence:       i.transferEvidenceForGraph(fnGraph),
			ParentEvidence: parentEvidence,
		}
	}

	return localFuncs
}

func (i *Inferencer) transferEvidenceForGraph(graph *cfg.Graph) api.FlowEvidence {
	if i != nil && i.store != nil {
		return i.store.EvidenceForGraph(graph)
	}
	return api.FlowEvidence{}
}

// newReturnInferenceEngine creates a synthesis engine configured for return type
// inference within the pre-flow return-vector computation phase.
//
// The engine operates in PhaseScopeCompute mode with:
//   - Declared types from the overlay (params, siblings, captured variables)
//   - Global types for built-in function resolution
//   - Module aliases for require() resolution
//   - Return vectors from previous iteration for recursive call resolution
//
// Unlike the main analysis engine, this engine does not have access to flow
// solution or narrowed types, producing "declared-phase" type estimates.
func (i *Inferencer) newReturnInferenceEngine(
	run RunContext,
	scopes map[cfg.Point]*scope.State,
	ctx api.DeclaredEnv,
	evidence api.FlowEvidence,
	functionFacts api.FunctionFacts,
) *synth.Engine {
	return synth.New(synth.Config{
		Ctx:            run.Ctx,
		Types:          i.types,
		Scopes:         scopes,
		Manifests:      i.manifests,
		Env:            ctx,
		FunctionFacts:  functionFacts,
		Phase:          api.PhaseScopeCompute,
		Evidence:       evidence,
		ModuleBindings: i.store.ModuleBindings(),
		ModuleAliases:  i.store.ModuleAliases(),
	})
}

// ComputeForGraph computes canonical function facts for local functions in a graph.
func (i *Inferencer) ComputeForGraph(
	run RunContext,
	graph *cfg.Graph,
	parent *scope.State,
) (api.FunctionFacts, []diag.Diagnostic) {
	if i == nil || i.store == nil || graph == nil || parent == nil {
		return nil, nil
	}

	parentScope := api.ParentScopeForGraph(i.store, graph.ID(), parent)

	engine := phase.CreateTypeResolutionEngine(run.Ctx, graph, i.globalTypes, nil, parentScope, i.types, i.manifests, i.store.ModuleAliases())
	pointScopes := scope.BuildTypeDefScopes(graph, parentScope, engine.ResolveTypeDef)
	localFuncs := i.collectLocalFunctions(graph, pointScopes, graph.Func())
	if len(localFuncs) == 0 {
		return nil, nil
	}

	// Apply parameter evidence from the stable canonical function facts.
	if facts := i.store.GetInterprocFacts(graph, parentScope).FunctionFacts; len(facts) > 0 {
		for _, sym := range cfg.SortedSymbolIDs(localFuncs) {
			info := localFuncs[sym]
			if info == nil {
				continue
			}
			if hintVec := functionfact.ParameterEvidenceFromMap(facts, sym); len(hintVec) > 0 {
				info.ParameterEvidence = paramevidence.ProjectToParameterUse(info.Graph.ParamSlotsReadOnly(), info.Evidence.ParameterUses, hintVec)
			}
		}
	}

	seedFacts := i.store.GetInterprocFacts(graph, parentScope).FunctionFacts
	seed := make(map[cfg.SymbolID][]typ.Type, len(seedFacts))
	for sym, fact := range seedFacts {
		if len(fact.Summary) > 0 {
			seed[sym] = fact.Summary
		}
	}
	returnVectors, diags := i.computeReturnVectorsForGroup(run, parentScope.GroupHash(), localFuncs, seed)
	functionTypes := i.buildLocalFunctionTypes(localFuncs, returnVectors, engine, parentScope)
	return assembleFunctionFacts(localFuncs, returnVectors, functionTypes), diags
}

func (i *Inferencer) buildLocalFunctionTypes(
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
	returnVectors map[cfg.SymbolID][]typ.Type,
	engine *synth.Engine,
	parentScope *scope.State,
) map[cfg.SymbolID]typ.Type {
	if len(localFuncs) == 0 {
		return nil
	}
	out := make(map[cfg.SymbolID]typ.Type, len(localFuncs))
	for _, sym := range cfg.SortedSymbolIDs(localFuncs) {
		info := localFuncs[sym]
		if info == nil || info.Fn == nil {
			continue
		}
		resolveScope := info.DefScope
		if resolveScope == nil {
			resolveScope = parentScope
		}
		if resolveScope == nil {
			resolveScope = scope.New()
		}
		var bindings interface {
			ParamSymbols(*ast.FunctionExpr) []cfg.SymbolID
			Name(cfg.SymbolID) string
		}
		if info.Graph != nil {
			bindings = info.Graph.Bindings()
		}
		seed := returns.BuildSeedFunctionTypeWithBindings(info.Fn, engine, resolveScope, bindings)
		fnType := unwrap.Function(seed)
		if fnType == nil {
			continue
		}
		if len(info.ParameterEvidence) > 0 {
			if merged := paramevidence.MergeIntoSignature(info.Fn, info.ParameterEvidence, fnType); merged != nil {
				fnType = merged
			}
		}
		if returnVector := returnVectors[sym]; len(returnVector) > 0 {
			if withSummary := returnsummary.ApplyToFunctionType(fnType, returnVector); withSummary != nil {
				fnType = withSummary
			}
		}
		out[sym] = fnType
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func assembleFunctionFacts(
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
	returnVectors map[cfg.SymbolID][]typ.Type,
	funcs map[cfg.SymbolID]typ.Type,
) api.FunctionFacts {
	params := make(map[cfg.SymbolID][]typ.Type, len(localFuncs))
	for sym, info := range localFuncs {
		if sym != 0 && info != nil && len(info.ParameterEvidence) > 0 {
			params[sym] = info.ParameterEvidence
		}
	}
	return functionfact.FromMaps(params, returnVectors, funcs)
}

// computeReturnVectorsForGroup computes return type vectors for a scope group
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
// WIDENING: Recursive SCCs merge through the convergence widening operator each
// round. The domain owns termination; callers do not cap iteration count.
//
// SEEDING: Initial return type estimates come from the seed map (previous fixpoint
// iteration). This accelerates convergence for iteratively-refined modules.
func (i *Inferencer) computeReturnVectorsForGroup(
	run RunContext,
	groupHash uint64,
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
	seed map[cfg.SymbolID][]typ.Type,
) (map[cfg.SymbolID][]typ.Type, []diag.Diagnostic) {
	_ = groupHash
	if len(localFuncs) == 0 {
		return nil, nil
	}

	sccs := i.planLocalFunctionSCCs(localFuncs)
	if len(sccs) == 0 {
		return nil, nil
	}

	returnVectors := seedReturnVectorsFromSeed(localFuncs, seed)
	return returnVectors, i.processSCCReturnVectors(run, sccs, localFuncs, returnVectors)
}

// returnInferenceContext holds shared state for return type inference phases.
type returnInferenceContext struct {
	run           RunContext
	info          *returns.LocalFuncInfo
	returnVectors map[cfg.SymbolID][]typ.Type
	localFuncs    map[cfg.SymbolID]*returns.LocalFuncInfo
	engine        *synth.Engine
	resolveScope  *scope.State
	moduleAliases map[cfg.SymbolID]string
	bindings      *bind.BindingTable
	parentFacts   flow.TypeFacts
}

// buildParameterOverlay creates the initial type overlay with parameter types.
// Parameters are typed from annotations, parameter evidence, or default to unknown.
func collectReturnTypes(
	returns []api.ReturnEvidence,
	synthEngine api.Synth,
	deadPoints map[cfg.Point]bool,
	skipReturnExpr func(ast.Expr) bool,
) []typ.Type {
	if len(returns) == 0 || synthEngine == nil {
		return nil
	}
	var returnTypes []typ.Type
	seenReturn := false

	for _, ret := range returns {
		p := ret.Point
		retInfo := ret.Info
		if retInfo == nil {
			continue
		}
		_ = deadPoints

		if len(retInfo.Exprs) == 1 && skipReturnExpr != nil && skipReturnExpr(retInfo.Exprs[0]) {
			continue
		}
		types := synthesizeReturnExprs(synthEngine, retInfo, p, skipReturnExpr)
		if !seenReturn {
			seenReturn = true
			returnTypes = types
			continue
		}

		returnTypes = joinReturnTypes(returnTypes, types)
	}

	return returnsummary.NormalizeOwned(returnTypes)
}

// synthesizeReturnExprs computes types for a single return statement's expressions.
func synthesizeReturnExprs(
	synthEngine api.Synth,
	retInfo *cfg.ReturnInfo,
	p cfg.Point,
	skipReturnExpr func(ast.Expr) bool,
) []typ.Type {
	if len(retInfo.Exprs) == 0 {
		return nil
	}
	types := make([]typ.Type, 0, len(retInfo.Exprs))
	for i, expr := range retInfo.Exprs {
		if i == len(retInfo.Exprs)-1 && ast.CanProduceMultipleValues(expr) {
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
		existing[i] = typ.JoinReturnSlot(existing[i], t)
	}
	return existing
}

// inferReturnTypesFromBody runs phase 2 synthesis to compute return types.
func (i *Inferencer) inferReturnTypesFromBody(
	ctx *returnInferenceContext,
	finalOverlay map[cfg.SymbolID]typ.Type,
) []typ.Type {
	state := i.runPhase2FlowNarrowing(ctx, finalOverlay)
	skipUnresolvedLocalCall := i.skipUnresolvedLocalReturnCall(ctx)
	narrowed := collectReturnTypes(ctx.info.Evidence.Returns, state.synth, state.deadPoints, skipUnresolvedLocalCall)

	fnGraph := ctx.info.Graph
	if fnGraph == nil {
		return narrowed
	}
	phaseFunctionFacts := functionfact.FromSummariesExcept(ctx.returnVectors, ctx.info.Sym)
	declCheckCtx := api.NewReturnInferenceEnv(api.ReturnInferenceEnvConfig{
		Graph:         fnGraph,
		Bindings:      ctx.bindings,
		BaseScope:     ctx.resolveScope,
		DeclaredTypes: finalOverlay,
		GlobalTypes:   i.globalTypes,
		ModuleAliases: ctx.moduleAliases,
		FunctionFacts: phaseFunctionFacts,
	})
	declSynth := i.newReturnInferenceEngine(
		ctx.run,
		uniformFunctionScopes(fnGraph, ctx.resolveScope),
		declCheckCtx,
		ctx.info.Evidence,
		phaseFunctionFacts,
	)
	declared := collectReturnTypes(ctx.info.Evidence.Returns, declSynth, nil, skipUnresolvedLocalCall)

	return returnsummary.Merge(declared, narrowed)
}

func (i *Inferencer) skipUnresolvedLocalReturnCall(ctx *returnInferenceContext) func(ast.Expr) bool {
	if ctx == nil || ctx.info == nil || ctx.info.Graph == nil || len(ctx.localFuncs) == 0 {
		return nil
	}
	bindings := ctx.bindings
	if bindings == nil {
		bindings = ctx.info.Graph.Bindings()
	}
	if bindings == nil {
		return nil
	}
	return func(expr ast.Expr) bool {
		call, ok := expr.(*ast.FuncCallExpr)
		if !ok || call == nil || call.Method != "" {
			return false
		}
		sym := callsite.SymbolFromExpr(call.Func, bindings)
		if sym == 0 || ctx.localFuncs[sym] == nil {
			return false
		}
		return typ.IsUnknownOnlyOrEmpty(returnsummary.Normalize(ctx.returnVectors[sym]))
	}
}

// inferReturnForFunction infers return types for one local function from the
// current SCC return-vector state.
// This is the core inference logic called by computeReturnVectorsForGroup for each function.
//
// TWO-PHASE INFERENCE:
//
// Phase 1 (Preliminary): Collect inferred types for local variables within the function.
// This uses a preliminary synthesis engine with:
//   - Parameter types (from annotations or parameter evidence)
//   - Sibling function types (from return vectors)
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
func (i *Inferencer) inferReturnForFunction(
	run RunContext,
	info *returns.LocalFuncInfo,
	returnVectors map[cfg.SymbolID][]typ.Type,
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
) []typ.Type {
	if info == nil || info.Fn == nil || info.Graph == nil {
		return nil
	}

	fn := info.Fn
	fnGraph := info.Graph
	parentScope := info.DefScope
	moduleAliases := modules.MergeAliases(i.store.ModuleAliases(), modules.AliasesFromAssignments(info.Evidence.Assignments, fnGraph))

	engine := phase.CreateTypeResolutionEngine(run.Ctx, fnGraph, i.globalTypes, nil, parentScope, i.types, i.manifests, moduleAliases)

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
		returnVectors: returnVectors,
		localFuncs:    localFuncs,
		engine:        engine,
		resolveScope:  resolveScope,
		moduleAliases: moduleAliases,
		bindings:      bindings,
		parentFacts:   run.ParentFacts,
	}

	// Build type overlay with parameter types.
	overlay := i.buildParameterOverlay(ctx)

	// Add sibling function types from return vectors.
	i.enrichOverlayWithSiblings(ctx, overlay)

	// Collect normalized return vectors and add local function types.
	allReturnVectors := i.collectAllReturnVectors(ctx)
	i.enrichOverlayWithLocalFunctions(ctx, overlay, allReturnVectors)

	// Add captured variable types from parent.
	i.enrichOverlayWithCaptured(ctx, overlay)

	// Add local declared types (annotations, loop variables) as overlay hints.
	localValueSeeds := i.enrichOverlayWithLocalDeclarations(ctx, overlay)

	// Body-derived parameter contracts are needed by local assignment inference
	// in the same function. For example, a helper call may prove that a parameter
	// field is string?, which then makes `param.field or "default"` synthesize as
	// string without relying on a value-level fallback shortcut.
	i.mergeParameterEvidenceFromBodyUses(ctx, overlay)
	i.applyParameterEvidenceToOverlay(ctx, overlay)

	// Phase 1: Infer local variable types.
	inferred, _, synthAdapter := i.inferLocalVariableTypes(ctx, overlay, localValueSeeds)

	// Collect field/indexer assignments and apply mutations.
	finalOverlay := i.collectAndApplyMutations(ctx, overlay, inferred, synthAdapter, localValueSeeds)

	// Phase 2: Infer return types from body.
	return i.inferReturnTypesFromBody(ctx, finalOverlay)
}

func (i *Inferencer) applyParameterEvidenceToOverlay(ctx *returnInferenceContext, overlay map[cfg.SymbolID]typ.Type) {
	if ctx == nil || ctx.info == nil || ctx.info.Graph == nil || len(ctx.info.ParameterEvidence) == 0 || overlay == nil {
		return
	}
	for idx, slot := range ctx.info.Graph.ParamSlotsReadOnly() {
		if slot.Symbol == 0 || idx >= len(ctx.info.ParameterEvidence) {
			continue
		}
		evidence := ctx.info.ParameterEvidence[idx]
		if !paramevidence.IsInformative(evidence) {
			continue
		}
		if slot.TypeAnnotation != nil {
			resolved := ctx.engine.ResolveType(slot.TypeAnnotation, ctx.resolveScope)
			if resolved == nil || !typ.IsRefinableAnnotation(resolved) {
				continue
			}
			overlay[slot.Symbol] = paramevidence.RefineAnnotationWithEvidence(resolved, evidence)
			continue
		}
		overlay[slot.Symbol] = evidence
	}
}

func (i *Inferencer) mergeParameterEvidenceFromBodyUses(ctx *returnInferenceContext, overlay map[cfg.SymbolID]typ.Type) {
	if i == nil || ctx == nil || ctx.info == nil || ctx.info.Graph == nil || ctx.info.Fn == nil {
		return
	}
	bindings := ctx.info.Graph.Bindings()
	if bindings == nil || i.types == nil {
		return
	}
	paramIndexBySym := make(map[cfg.SymbolID]int)
	for idx, slot := range ctx.info.Graph.ParamSlotsReadOnly() {
		if slot.Symbol == 0 {
			continue
		}
		_, hasSource := slot.SourceParamIndex()
		if hasSource && hardParameterAnnotation(ctx, slot) {
			continue
		}
		paramIndexBySym[slot.Symbol] = idx
	}
	if len(paramIndexBySym) == 0 {
		return
	}

	mergeReceiver := func(receiver ast.Expr, method string) {
		if receiver == nil || method == "" {
			return
		}
		ident, ok := receiver.(*ast.IdentExpr)
		if !ok || ident == nil {
			return
		}
		sym, ok := bindings.SymbolOf(ident)
		if !ok || sym == 0 {
			return
		}
		idx, ok := paramIndexBySym[sym]
		if !ok {
			return
		}
		evidence := i.receiverEvidenceForMethod(ctx, method)
		if !paramevidence.IsInformative(evidence) {
			return
		}
		next, merged := paramevidence.MergeAt(ctx.info.ParameterEvidence, idx, evidence, typ.JoinPreferNonSoft)
		if merged {
			ctx.info.ParameterEvidence = next
		}
	}
	mergeParamFieldEvidence := func(sym cfg.SymbolID, field string, evidence typ.Type, required bool) {
		if sym == 0 || field == "" || !paramevidence.IsInformative(evidence) {
			return
		}
		idx, ok := paramIndexBySym[sym]
		if !ok {
			return
		}
		builder := typ.NewRecord()
		if required {
			builder.Field(field, evidence)
		} else {
			builder.OptField(field, evidence)
		}
		rec := builder.Build()
		next, merged := paramevidence.MergeAt(ctx.info.ParameterEvidence, idx, rec, typ.JoinPreferNonSoft)
		if merged {
			ctx.info.ParameterEvidence = next
		}
	}
	bodyContractJoin := func(prev, next typ.Type) typ.Type {
		if next != nil {
			return next
		}
		return prev
	}
	mergeParameterEvidence := func(sym cfg.SymbolID, evidence typ.Type) {
		if sym == 0 || !paramevidence.IsInformative(evidence) {
			return
		}
		idx, ok := paramIndexBySym[sym]
		if !ok {
			return
		}
		next, merged := paramevidence.MergeAt(ctx.info.ParameterEvidence, idx, evidence, bodyContractJoin)
		if merged {
			ctx.info.ParameterEvidence = next
		}
	}
	paramSymbol := func(expr ast.Expr) (cfg.SymbolID, bool) {
		ident, ok := expr.(*ast.IdentExpr)
		if !ok || ident == nil {
			return 0, false
		}
		sym, ok := bindings.SymbolOf(ident)
		if !ok || sym == 0 {
			return 0, false
		}
		if _, ok := paramIndexBySym[sym]; !ok {
			return 0, false
		}
		return sym, true
	}
	paramFieldPath := func(expr ast.Expr) (cfg.SymbolID, string, bool) {
		attr, ok := expr.(*ast.AttrGetExpr)
		if !ok || attr == nil {
			return 0, "", false
		}
		obj, ok := attr.Object.(*ast.IdentExpr)
		if !ok || obj == nil {
			return 0, "", false
		}
		key, ok := attr.Key.(*ast.StringExpr)
		if !ok || key == nil || key.Value == "" {
			return 0, "", false
		}
		sym, ok := bindings.SymbolOf(obj)
		if !ok {
			return 0, "", false
		}
		if _, ok := paramIndexBySym[sym]; !ok {
			return 0, "", false
		}
		return sym, key.Value, true
	}
	typeAt := func(expr ast.Expr, p cfg.Point) typ.Type {
		if expr == nil {
			return typ.Unknown
		}
		if t, ok := overlayPathType(expr, overlay, bindings, i.types, ctx.run.Ctx); ok {
			return t
		}
		if ctx.engine != nil {
			if t := ctx.engine.TypeOf(expr, p); t != nil {
				return t
			}
		}
		return typ.Unknown
	}
	isDirectSelfRecursiveCall := func(info *cfg.CallInfo) bool {
		if info == nil || ctx.info.Sym == 0 {
			return false
		}
		for _, sym := range callsite.CallableCalleeSymbolCandidates(info, ctx.info.Graph, bindings, bindings) {
			if sym == ctx.info.Sym {
				return true
			}
		}
		return false
	}
	var bodyParamContracts map[cfg.SymbolID]typ.Type
	mergeParamContract := func(sym cfg.SymbolID, evidence typ.Type) {
		if sym == 0 || !paramevidence.IsInformative(evidence) {
			return
		}
		if bodyParamContracts == nil {
			bodyParamContracts = make(map[cfg.SymbolID]typ.Type)
		}
		if prev := bodyParamContracts[sym]; prev != nil {
			bodyParamContracts[sym] = subtype.NormalizeIntersection(prev, evidence)
			return
		}
		bodyParamContracts[sym] = evidence
	}
	mergeExpectedFieldEvidence := func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil || i.types == nil {
			return
		}
		if isDirectSelfRecursiveCall(info) {
			return
		}
		args := make([]typ.Type, len(info.Args))
		for idx, arg := range info.Args {
			args[idx] = typeAt(arg, p)
		}
		def := ops.CallDef{
			Args:  args,
			Query: i.types,
		}
		if info.Method != "" {
			def.IsMethod = true
			def.MethodName = info.Method
			def.Receiver = typeAt(info.Receiver, p)
			def.ForceMethodReceiver = callsite.ForceMethodReceiver(bindings, ctx.info.Graph, ctx.info.Evidence, info)
		} else {
			def.Callee = typeAt(info.Callee, p)
		}
		inferredCall := ops.InferCall(ctx.run.Ctx, def)
		for idx, arg := range info.Args {
			expected := inferredCall.ExpectedArgType(idx)
			if !paramevidence.IsInformative(expected) {
				continue
			}
			if sym, ok := paramSymbol(arg); ok {
				mergeParamContract(sym, expected)
				continue
			}
			if sym, field, ok := paramFieldPath(arg); ok {
				mergeParamFieldEvidence(sym, field, expected, true)
				continue
			}
		}
	}
	for _, ev := range ctx.info.Evidence.Calls {
		mergeExpectedFieldEvidence(ev.Point, ev.Info)
	}
	for _, sym := range cfg.SortedSymbolIDs(bodyParamContracts) {
		mergeParameterEvidence(sym, bodyParamContracts[sym])
	}
	defaultLiteralType := func(expr ast.Expr) typ.Type {
		switch expr.(type) {
		case *ast.StringExpr:
			return typ.String
		case *ast.NumberExpr:
			return typ.Number
		case *ast.TrueExpr, *ast.FalseExpr:
			return typ.Boolean
		default:
			return nil
		}
	}
	for _, ev := range ctx.info.Evidence.Calls {
		if ev.Info != nil {
			mergeReceiver(ev.Info.Receiver, ev.Info.Method)
		}
	}
	for _, ev := range ctx.info.Evidence.FieldDefaults {
		mergeParamFieldEvidence(ev.Target, ev.Field, defaultLiteralType(ev.Value), false)
	}
}

func hardParameterAnnotation(ctx *returnInferenceContext, slot cfg.ParamSlot) bool {
	if ctx == nil || ctx.engine == nil || slot.TypeAnnotation == nil {
		return false
	}
	resolved := ctx.engine.ResolveType(slot.TypeAnnotation, ctx.resolveScope)
	return resolved != nil && !typ.IsRefinableAnnotation(resolved)
}

func (i *Inferencer) receiverEvidenceForMethod(ctx *returnInferenceContext, method string) typ.Type {
	if i == nil || i.types == nil || method == "" {
		return nil
	}
	methodType, ok := i.types.Method(ctx.run.Ctx, typ.String, method)
	if !ok || methodType == nil {
		return nil
	}
	fn, ok := methodType.(*typ.Function)
	if !ok || len(fn.Params) == 0 || !typ.TypeEquals(fn.Params[0].Type, typ.String) {
		return nil
	}
	return typ.String
}

func overlayPathType(
	expr ast.Expr,
	overlay map[cfg.SymbolID]typ.Type,
	bindings *bind.BindingTable,
	typeOps core.TypeOps,
	ctx *db.QueryContext,
) (typ.Type, bool) {
	if expr == nil || len(overlay) == 0 || bindings == nil {
		return nil, false
	}
	p := flowpath.FromExprWithBindings(expr, nil, bindings)
	if p.IsEmpty() || p.Symbol == 0 {
		return nil, false
	}
	t, ok := overlay[p.Symbol]
	if !ok || t == nil {
		return nil, false
	}
	for _, seg := range p.Segments {
		if typeOps == nil {
			return nil, false
		}
		switch seg.Kind {
		case constraint.SegmentField:
			ft, ok := typeOps.Field(ctx, t, seg.Name)
			if !ok {
				return nil, false
			}
			t = ft
		case constraint.SegmentIndexString:
			ft, ok := typeOps.Index(ctx, t, typ.LiteralString(seg.Name))
			if !ok {
				return nil, false
			}
			t = ft
		case constraint.SegmentIndexInt:
			ft, ok := typeOps.Index(ctx, t, typ.LiteralInt(int64(seg.Index)))
			if !ok {
				return nil, false
			}
			t = ft
		default:
			return nil, false
		}
	}
	return t, true
}
