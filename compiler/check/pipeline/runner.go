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
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/infer/captured"
	"github.com/wippyai/go-lua/compiler/check/modules"
	"github.com/wippyai/go-lua/compiler/check/phase"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
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
}

// Runner executes the phase pipeline for a single function.
// It is used as the compute function for FuncResult queries.
type Runner struct {
	types         core.TypeOps
	globalTypes   map[string]typ.Type
	stdlib        *scope.State
	manifests     io.ManifestQuerier
	resolver      narrow.Resolver
	maxScopeDepth int
	computePasses []api.ComputePass
}

// NewRunner returns a configured pipeline runner.
func NewRunner(cfg RunnerConfig) *Runner {
	return &Runner{
		types:         cfg.Types,
		globalTypes:   cfg.GlobalTypes,
		stdlib:        cfg.Stdlib,
		manifests:     cfg.Manifests,
		resolver:      cfg.Resolver,
		maxScopeDepth: cfg.MaxScopeDepth,
		computePasses: cfg.ComputePasses,
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
		synthSig = mergeSynthesizedSignatureContext(synthSig, analysisCtx.ExpectedFunction)
	}

	graphFacts := store.GetInterprocFacts(graph, parent)
	functionFacts := graphFacts.FunctionFacts

	localAliases := modules.AliasesFromAssignments(graphEvidence.Assignments, graph)
	mergedAliases := modules.MergeAliases(store.ModuleAliases(), localAliases)
	env := phase.PhaseEnv{
		Ctx:            ctx,
		Graph:          graph,
		Fn:             fn,
		Types:          r.types,
		Manifests:      r.manifests,
		GlobalTypes:    globalTypes,
		ModuleAliases:  mergedAliases,
		ModuleBindings: store.ModuleBindings(),
		Refinements:    refinementFactsFrom(store),
		Evidence:       graphEvidence,
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

	if capturedTypes := graphFacts.CapturedTypes; len(capturedTypes) > 0 {
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
		for sym, fact := range functionFacts {
			if fact.Type == nil {
				continue
			}
			literalOut.LiteralTypes[sym] = fact.Type
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
	r.appendCapturedMutatorAssignments(store, graph, parent, env, scopeOut, literalOut, functionFacts, &extractOut)

	// Phase C: Solve flow system.
	solveOut := phase.RunSolve(phase.FlowSolveInput{
		PhaseEnv: env,
		Extract:  extractOut,
		Resolver: r.resolver,
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

	return &api.FuncResult{
		Graph:              graph,
		ModuleBindings:     env.ModuleBindings,
		BaseScope:          scopeOut.BaseScope,
		Scopes:             scopeOut.Scopes,
		Facts:              narrowOut.Facts,
		FlowInputs:         extractOut.Inputs,
		FlowSolution:       solveOut.Solution,
		Evidence:           extractOut.Evidence,
		FnRefinement:       narrowOut.Refinement,
		NarrowSynth:        narrowOut.Synth,
		LiteralSignatures:  literalOut.Signatures,
		Extras:             extras,
		DepthLimitExceeded: scopeOut.DepthLimitExceeded,
	}
}

func mergeGlobalOverlay(base map[string]typ.Type, overlay map[string]typ.Type) map[string]typ.Type {
	if len(overlay) == 0 {
		return base
	}
	out := make(map[string]typ.Type, len(base)+len(overlay))
	for name, t := range base {
		out[name] = t
	}
	for name, t := range overlay {
		if name != "" && t != nil {
			out[name] = t
		}
	}
	return out
}
