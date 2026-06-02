// runner.go implements the multi-phase analysis pipeline for functions.
//
// The pipeline executes four phases for each function:
//
//	Phase A (Resolve): Resolves type annotations into concrete types
//	Phase B (Scope): Builds lexical scope states and extracts flow constraints
//	Phase C (Solve): Solves the flow constraint system
//	Phase D (Narrow): Applies narrowing to compute effective types
//
// The [Runner] is used as the compute function for memoized FuncResult queries.
// It receives a FuncKey (graph ID + parent hash + revision) and produces a
// complete [api.FuncResult] containing all phase outputs.
//
// The pipeline integrates several subsystems:
//   - Synthesis engine for expression type computation
//   - Flow solver for control flow analysis
//   - Effect propagation for side effect tracking
//   - Parameter evidence inference from call sites
package pipeline

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/domain/globalenv"
	"github.com/wippyai/go-lua/compiler/check/infer/captured"
	"github.com/wippyai/go-lua/compiler/check/modules"
	"github.com/wippyai/go-lua/compiler/check/phase"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	phasecore "github.com/wippyai/go-lua/compiler/check/synth/phase/core"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// RunnerConfig configures a pipeline runner.
type RunnerConfig struct {
	Types         core.TypeOps
	GlobalTypes   map[string]typ.Type
	Stdlib        *scope.State
	Manifests     io.ManifestQuerier
	Resolver      narrow.Resolver
	MaxScopeDepth int

	ComputePasses []api.ComputePass

	// RecursiveFamilies is the compilation-scoped recursive-family interner.
	// Class-metatable sealing widens family bodies only through it, so one
	// compilation's convergence seed cannot mutate type state shared with another.
	RecursiveFamilies *typ.RecursiveFamilyInterner
}

// Runner executes the phase pipeline for a single function.
// It is used as the compute function for FuncResult queries.
type Runner struct {
	types             core.TypeOps
	globalTypes       globalenv.TypeOverlay
	stdlib            *scope.State
	manifests         io.ManifestQuerier
	resolver          narrow.Resolver
	maxScopeDepth     int
	computePasses     []api.ComputePass
	recursiveFamilies *typ.RecursiveFamilyInterner
}

// NewRunner returns a configured pipeline runner.
func NewRunner(cfg RunnerConfig) *Runner {
	return &Runner{
		types:             cfg.Types,
		globalTypes:       globalenv.TypeOverlayFromMap(cfg.GlobalTypes),
		stdlib:            cfg.Stdlib,
		manifests:         cfg.Manifests,
		resolver:          cfg.Resolver,
		maxScopeDepth:     cfg.MaxScopeDepth,
		computePasses:     cfg.ComputePasses,
		recursiveFamilies: cfg.RecursiveFamilies,
	}
}

// Run executes the phase pipeline and returns the full function analysis result.
func (r *Runner) Run(ctx *db.QueryContext, key api.FuncKey) *api.FuncResult {
	store := api.StoreFrom(ctx)
	if store == nil {
		return nil
	}
	if tracker, ok := store.(interface {
		PushFactReadContext(*db.QueryContext) func()
	}); ok {
		pop := tracker.PushFactReadContext(ctx)
		defer pop()
	}
	withPhase := func(_ api.Phase, fn func()) { fn() }
	if phaser, ok := store.(interface{ WithPhase(api.Phase, func()) }); ok {
		withPhase = phaser.WithPhase
	}
	graphs := store.Graphs()
	parents := store.Parents()
	if graphs == nil || parents == nil {
		return nil
	}
	graph := graphs[key.GraphID]
	if graph == nil {
		return nil
	}
	fn := graph.Func()
	if fn == nil {
		return nil
	}
	parent := parents[key.ParentHash]
	if setter, ok := store.(interface {
		SetGraphParentHash(graphID, parentHash uint64)
	}); ok {
		setter.SetGraphParentHash(graph.ID(), key.ParentHash)
	}

	// Build shared transfer evidence and phase environment once.
	graphEvidence := store.EvidenceForGraph(graph)
	parameterEvidenceSigs := functionfact.ParameterEvidenceSignatures(store, graph, parent, r.stdlib)
	analysisCtx := api.AnalysisContext{}
	globalTypes := r.globalTypes
	if contextual, ok := store.(interface {
		GraphAnalysisContext(api.GraphKey) api.AnalysisContext
	}); ok {
		analysisCtx = contextual.GraphAnalysisContext(api.GraphKey(key))
		globalTypes = mergeGlobalOverlay(globalTypes, analysisCtx.GlobalOverlay)
	}
	synthSig := r.resolveSynthesizedSignature(ctx, store, graph, fn, parent, graphEvidence, parameterEvidenceSigs)
	if analysisCtx.ExpectedFunction != nil {
		synthSig = functionfact.MergeExpectedSignature(synthSig, analysisCtx.ExpectedFunction)
	}

	functionFacts := functionfact.VisibleFactsForGraph(store, graph, parent, r.stdlib)

	localAliases := modules.AliasesFromAssignments(graphEvidence.Assignments, graph)
	mergedAliases := modules.MergeAliases(store.ModuleAliases(), localAliases)
	env := phase.PhaseEnv{
		Ctx:               ctx,
		Graph:             graph,
		Fn:                fn,
		Types:             r.types,
		Manifests:         r.manifests,
		GlobalTypes:       globalTypes,
		ModuleAliases:     mergedAliases,
		ModuleBindings:    store.ModuleBindings(),
		Refinements:       refinementFactsFrom(store),
		Evidence:          graphEvidence,
		RecursiveFamilies: r.recursiveFamilies,
	}

	// Phase A: Resolve type annotations.
	resolveOut := phase.RunResolve(phase.ResolveInput{
		PhaseEnv:  env,
		Bindings:  graph.Bindings(),
		BaseScope: parent,
	})

	// Literal signatures are provided by the visible interproc product.
	literalSigs := r.literalSigProvider(store, graph, parent)

	// Phase B: Build scopes and extract declared types.
	scopeOut := phase.RunScope(phase.ScopeInput{
		PhaseEnv:                    env,
		Parent:                      parent,
		MaxScopeDepth:               r.maxScopeDepth,
		Resolve:                     resolveOut,
		SynthesizedFunctionSig:      synthSig,
		FunctionLiteralSignatures:   literalSigs,
		ParameterEvidenceSignatures: parameterEvidenceSigs,
		FunctionFacts:               functionFacts,
	})
	// Declared is the default phase for scope/extract and interproc reads.

	if capturedTypes := capturedTypesForFunction(store.InterprocFacts(graph, parent), graph, fn); len(capturedTypes) > 0 {
		scopeOut.DeclaredTypes = captured.MergeCapturedTypes(scopeOut.DeclaredTypes, capturedTypes)
	}
	r.mergeCapturedParentFunctionTypes(store, graph, fn, &scopeOut)

	// Populate scopes in env for later phases.
	env.Scopes = scopeOut.Scopes

	// Phase B (continued): Synthesize function literal types.
	literalOut := phase.RunLiteral(phase.LiteralInput{
		PhaseEnv:      env,
		Scope:         scopeOut,
		FunctionFacts: functionFacts,
	})
	// Ensure literal function types use canonical local function types.
	if len(functionFacts) > 0 {
		if literalOut.LiteralTypes == nil {
			literalOut.LiteralTypes = make(flow.DeclaredTypes, len(functionFacts))
		}
		for sym := range functionFacts {
			projected := functionfact.SiblingTypeProjection(functionFacts, sym, api.PhaseScopeCompute)
			if projected == nil {
				continue
			}
			literalOut.LiteralTypes[sym] = projected
		}
	}

	// Phase B (continued): Extract flow constraints.
	extractOut := phase.RunExtract(phase.FlowExtractInput{
		PhaseEnv:      env,
		Resolve:       resolveOut,
		Scope:         scopeOut,
		FunctionFacts: functionFacts,
		LiteralTypes:  literalOut.LiteralTypes,
	})
	r.appendCapturedCallEffectAssignments(store, graph, parent, env, scopeOut, literalOut, functionFacts, &extractOut)

	// Phase C: Solve flow system.
	solveResolver := r.resolver
	if queryResolver := core.NewQueryResolver(ctx, r.types); queryResolver != nil {
		solveResolver = queryResolver
	}
	solveOut := phase.RunSolve(phase.FlowSolveInput{
		PhaseEnv: env,
		Extract:  extractOut,
		Resolver: solveResolver,
	})
	// Phase D: Narrowing and effect inference.
	var narrowOut phase.NarrowOutput
	withPhase(api.PhaseNarrowing, func() {
		narrowOut = phase.RunNarrow(phase.NarrowInput{
			PhaseEnv:      env,
			Scope:         scopeOut,
			Extract:       extractOut,
			Solve:         solveOut,
			FunctionFacts: functionFacts,
			LiteralTypes:  literalOut.LiteralTypes,
		})
	})

	extras := r.runComputePasses(graph, scopeOut.Scopes)
	sourceSignature, publicSeedSignature := postflowSignatures(fn, graph, scopeOut.BaseScope, narrowOut.Synth)
	sourceSignature = functionfact.MergeSignatureContracts(sourceSignature, synthSig)
	callExpectedArgs := r.projectCallExpectedArgs(ctx, graph, extractOut.Evidence, narrowOut.Synth)

	return &api.FuncResult{
		Graph:                    graph,
		ModuleBindings:           env.ModuleBindings,
		AnalysisContext:          analysisCtx,
		BaseScope:                scopeOut.BaseScope,
		Scopes:                   scopeOut.Scopes,
		Facts:                    narrowOut.Facts,
		FlowInputs:               extractOut.Inputs,
		FlowSolution:             solveOut.Solution,
		Evidence:                 extractOut.Evidence,
		CallExpectedArgs:         callExpectedArgs,
		FnRefinement:             narrowOut.Refinement,
		SourceSignature:          sourceSignature,
		PublicSeedSignature:      publicSeedSignature,
		NarrowSynth:              narrowOut.Synth,
		QueryContext:             narrowOut.QueryContext,
		TypeOps:                  narrowOut.TypeOps,
		GlobalTypes:              globalTypes.ToMap(),
		GlobalTypeBindings:       globalTypes,
		LiteralSignatures:        literalOut.Signatures,
		LiteralSignatureProvider: api.LiteralSignatureLookupFromMap(literalOut.Signatures),
		Extras:                   extras,
		DepthLimitExceeded:       scopeOut.DepthLimitExceeded,
		RecursiveFamilies:        r.recursiveFamilies,
		ClassFamilyJoin:          functionfact.ClassFamilyJoin,
	}
}

func (r *Runner) projectCallExpectedArgs(
	ctx *db.QueryContext,
	graph *cfg.Graph,
	evidence api.FlowEvidence,
	synth api.BaseSynth,
) []api.CallExpectedArgEvidence {
	if graph == nil || synth == nil || len(evidence.Calls) == 0 {
		return nil
	}
	var out []api.CallExpectedArgEvidence
	for i, ev := range evidence.Calls {
		info := ev.Info
		if info == nil || info.Call == nil || len(info.Call.Args) == 0 {
			continue
		}
		def := ops.CallDef{
			Args:                shallowCallArgTypes(synth, info.Args, ev.Point),
			Query:               r.types,
			ForceMethodReceiver: callsite.ForceMethodReceiverAtPoint(graph.Bindings(), graph, evidence, ev.Point, info.Call),
		}
		if callsite.IsMethodCallInfo(info) {
			def.IsMethod = true
			def.Receiver = synth.TypeOf(info.Receiver, ev.Point)
			def.MethodName = info.Method
		} else {
			def.Callee = synth.TypeOf(info.Callee, ev.Point)
		}
		inferred := ops.InferCall(ctx, def)
		args := make([]typ.Type, len(info.Call.Args))
		any := false
		for argIdx := range info.Call.Args {
			expected := inferred.ExpectedArgType(argIdx)
			if expected == nil || typ.IsAbsentOrUnknown(expected) || typ.IsAny(expected) {
				continue
			}
			args[argIdx] = expected
			any = true
		}
		if !any {
			continue
		}
		if out == nil {
			out = make([]api.CallExpectedArgEvidence, len(evidence.Calls))
		}
		out[i] = api.NewCallExpectedArgEvidence(args)
	}
	return out
}

func shallowCallArgTypes(synth api.BaseSynth, args []ast.Expr, point cfg.Point) []typ.Type {
	if len(args) == 0 {
		return nil
	}
	out := make([]typ.Type, len(args))
	for i, arg := range args {
		if fn, ok := arg.(*ast.FunctionExpr); ok {
			out[i] = phasecore.ShallowFunctionLiteralSignature(fn)
			continue
		}
		out[i] = synth.TypeOf(arg, point)
	}
	return out
}

func postflowSignatures(fn *ast.FunctionExpr, graph *cfg.Graph, base *scope.State, synth api.Synth) (*typ.Function, *typ.Function) {
	if fn == nil || synth == nil {
		return nil, nil
	}
	var bindings interface {
		ParamSymbols(*ast.FunctionExpr) []cfg.SymbolID
		Name(cfg.SymbolID) string
	}
	if graph != nil {
		bindings = graph.Bindings()
	}
	return synth.ResolveFunctionSignature(fn, base),
		unwrap.Function(returns.BuildSeedFunctionTypeWithBindings(fn, synth, base, bindings))
}

func mergeGlobalOverlay(base globalenv.TypeOverlay, overlay api.GlobalOverlay) globalenv.TypeOverlay {
	projected := overlay.ToTypeOverlay()
	if len(projected) == 0 {
		return base
	}
	return globalenv.OverrideTypeOverlay(
		base,
		projected,
	)
}

func capturedTypesForFunction(product api.InterprocFactProduct, graph *cfg.Graph, fn *ast.FunctionExpr) map[cfg.SymbolID]typ.Type {
	if product == nil || graph == nil || fn == nil || graph.Bindings() == nil {
		return nil
	}
	var out map[cfg.SymbolID]typ.Type
	for _, sym := range graph.Bindings().CapturedSymbols(fn) {
		if sym == 0 {
			continue
		}
		t, ok := product.CapturedType(sym)
		if !ok || t == nil {
			continue
		}
		if out == nil {
			out = make(map[cfg.SymbolID]typ.Type)
		}
		out[sym] = t
	}
	return out
}
