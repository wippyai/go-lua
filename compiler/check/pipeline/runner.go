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
//   - Parameter hint inference from call sites
package pipeline

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/compiler/check/infer/captured"
	"github.com/wippyai/go-lua/compiler/check/infer/paramhints"
	"github.com/wippyai/go-lua/compiler/check/modules"
	"github.com/wippyai/go-lua/compiler/check/phase"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth"
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

	// Prefer literal signature from parent graph for nested functions.
	synthSig := r.literalSignatureForFunction(store, graph, fn)

	// Apply param hints to synthesized signature when available.
	paramHintSigs := paramhints.BuildParamHintSigView(store, graph, parent, r.stdlib)
	if paramHintSigs != nil {
		if hints := paramHintSigs[fn]; len(hints) > 0 {
			if synthSig == nil {
				engine := synth.New(synth.Config{
					Ctx:       ctx,
					Types:     r.types,
					Manifests: r.manifests,
					Phase:     api.PhaseTypeResolution,
				})
				if sig := engine.ResolveFunctionSignature(fn, parent); sig != nil {
					synthSig = sig
				} else if seedFn, ok := returns.BuildSeedFunctionTypeWithBindings(fn, engine, parent, graph.Bindings()).(*typ.Function); ok {
					synthSig = seedFn
				}
			}
			if synthSig != nil {
				synthSig = phase.MergeParamHintsIntoSig(fn, hints, synthSig)
			}
		}
	}

	// Canonical local function types for this graph (stable snapshot).
	siblingTypes := store.GetLocalFuncTypesSnapshot(graph, parent)
	// Return summaries include captured field assignments (stable snapshot).
	returnSummaries := store.GetReturnSummariesSnapshot(graph, parent)
	var narrowReturnSummaries map[cfg.SymbolID][]typ.Type
	withPhase(api.PhaseNarrowing, func() {
		narrowReturnSummaries = store.GetNarrowReturnSummariesSnapshot(graph, parent)
	})

	// Build shared phase environment once.
	localAliases := modules.CollectAliases(graph)
	mergedAliases := modules.MergeAliases(store.ModuleAliases(), localAliases)
	env := phase.PhaseEnv{
		Ctx:            ctx,
		Graph:          graph,
		Fn:             fn,
		Types:          r.types,
		Manifests:      r.manifests,
		GlobalTypes:    r.globalTypes,
		ModuleAliases:  mergedAliases,
		ModuleBindings: store.ModuleBindings(),
		EffectStore:    effectStoreFrom(store),
	}

	// Phase A: Resolve type annotations.
	resolveOut := phase.RunResolve(phase.ResolveInput{
		PhaseEnv:  env,
		Bindings:  graph.Bindings(),
		BaseScope: parent,
	})

	// Literal signatures are provided by the stable snapshot.
	literalSigs := r.literalSigProvider(store, graph, parent)

	// Phase B: Build scopes and extract declared types.
	scopeOut := phase.RunScope(phase.ScopeInput{
		PhaseEnv:                  env,
		Parent:                    parent,
		MaxScopeDepth:             r.maxScopeDepth,
		Resolve:                   resolveOut,
		SynthesizedFunctionSig:    synthSig,
		FunctionLiteralSignatures: literalSigs,
		ParamHintSignatures:       paramHintSigs,
		SiblingTypes:              siblingTypes,
		ReturnSummaries:           returnSummaries,
	})
	// Declared is the default phase for scope/extract and interproc reads.

	if capturedTypes := store.GetCapturedTypesSnapshot(graph, parent); len(capturedTypes) > 0 {
		scopeOut.DeclaredTypes = captured.MergeCapturedTypes(scopeOut.DeclaredTypes, capturedTypes)
	}

	// Populate scopes in env for later phases.
	env.Scopes = scopeOut.Scopes

	// Phase B (continued): Synthesize function literal types.
	literalOut := phase.RunLiteral(phase.LiteralInput{
		PhaseEnv:        env,
		Scope:           scopeOut,
		SiblingTypes:    scopeOut.SiblingTypes,
		ReturnSummaries: returnSummaries,
	})
	// Ensure literal function types use canonical local function types.
	if len(siblingTypes) > 0 {
		if literalOut.LiteralTypes == nil {
			literalOut.LiteralTypes = make(flow.DeclaredTypes, len(siblingTypes))
		}
		for sym, fnType := range siblingTypes {
			if fnType == nil {
				continue
			}
			literalOut.LiteralTypes[sym] = fnType
		}
	}

	// Phase B (continued): Extract flow constraints.
	extractOut := phase.RunExtract(phase.FlowExtractInput{
		PhaseEnv:        env,
		Resolve:         resolveOut,
		Scope:           scopeOut,
		SiblingTypes:    scopeOut.SiblingTypes,
		LiteralTypes:    literalOut.LiteralTypes,
		ReturnSummaries: returnSummaries,
	})

	if extractOut.Inputs != nil {
		capturedContainers := store.GetCapturedContainerMutationsSnapshot(graph, parent)
		if len(capturedContainers) > 0 {
			bindings := graph.Bindings()
			if bindings == nil {
				bindings = store.ModuleBindings()
			}

			declaredEnv := phase.NewContextBuilder(env).
				WithScope(scopeOut).
				WithSiblingTypes(scopeOut.SiblingTypes).
				WithLiteralTypes(literalOut.LiteralTypes).
				WithReturnSummaries(returnSummaries).
				BuildDeclared()

			synthEngine := synth.New(synth.Config{
				Ctx:            env.Ctx,
				Types:          env.Types,
				Scopes:         scopeOut.Scopes,
				Manifests:      env.Manifests,
				Env:            declaredEnv,
				Phase:          api.PhaseScopeCompute,
				ModuleBindings: env.ModuleBindings,
				ModuleAliases:  env.ModuleAliases,
			})

			symResolver := resolve.BuildInputSymbolResolver(declaredEnv, extractOut.Inputs)
			assignmentTypes := resolve.BuildAssignmentTypeResolver(extractOut.Inputs)
			calleeTypeResolver := func(info *cfg.CallInfo, p cfg.Point) typ.Type {
				return resolve.CalleeType(info, p, synthEngine.TypeOf, symResolver, assignmentTypes, bindings, env.ModuleBindings)
			}

			extra := returns.CollectCalledNestedContainerMutatorAssignments(graph, bindings, capturedContainers, calleeTypeResolver)
			if len(extra) > 0 {
				extractOut.Inputs.ContainerMutatorAssignments = append(extractOut.Inputs.ContainerMutatorAssignments, extra...)
			}
		}
	}

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
			PhaseEnv:              env,
			Scope:                 scopeOut,
			Extract:               extractOut,
			Solve:                 solveOut,
			SiblingTypes:          scopeOut.SiblingTypes,
			LiteralTypes:          literalOut.LiteralTypes,
			NarrowReturnSummaries: narrowReturnSummaries,
		})
	})

	// Run compute passes.
	var extras map[string]any
	if len(r.computePasses) > 0 {
		extras = make(map[string]any, len(r.computePasses))
		for _, pass := range r.computePasses {
			extras[pass.Name()] = pass.Run(graph, scopeOut.Scopes)
		}
	}

	return &api.FuncResult{
		Graph:              graph,
		BaseScope:          scopeOut.BaseScope,
		Scopes:             scopeOut.Scopes,
		Facts:              narrowOut.Facts,
		FlowInputs:         extractOut.Inputs,
		FlowSolution:       solveOut.Solution,
		FnEffect:           narrowOut.Effect,
		NarrowSynth:        narrowOut.Synth,
		LiteralSignatures:  literalOut.Signatures,
		Extras:             extras,
		DepthLimitExceeded: scopeOut.DepthLimitExceeded,
	}
}

func (r *Runner) literalSignatureForFunction(store api.StoreView, graph *cfg.Graph, fn *ast.FunctionExpr) *typ.Function {
	if store == nil || graph == nil || fn == nil {
		return nil
	}
	meta, ok := store.NestedMetaFor(graph.ID())
	if !ok {
		return nil
	}
	parentGraph := store.Graphs()[meta.ParentGraphID]
	if parentGraph == nil {
		return nil
	}

	if sigs := scratchLiteralSigs(store, parentGraph.ID()); len(sigs) > 0 {
		if sig := sigs[fn]; sig != nil {
			return sig
		}
	}

	parentScope := r.parentScopeForGraph(store, parentGraph)
	if parentScope == nil {
		return nil
	}
	if sigs := store.GetLiteralSigsSnapshot(parentGraph, parentScope); len(sigs) > 0 {
		if sig := sigs[fn]; sig != nil {
			return sig
		}
	}
	return nil
}

func (r *Runner) literalSigProvider(store api.StoreView, graph *cfg.Graph, parent *scope.State) phase.LiteralSigsProvider {
	if store == nil || graph == nil || parent == nil {
		return nil
	}
	var literalSigMap map[*ast.FunctionExpr]*typ.Function
	if sigs := store.GetLiteralSigsSnapshot(graph, parent); len(sigs) > 0 {
		literalSigMap = make(map[*ast.FunctionExpr]*typ.Function, len(sigs))
		for fnExpr, sig := range sigs {
			if fnExpr != nil && sig != nil {
				literalSigMap[fnExpr] = sig
			}
		}
	}
	if meta, ok := store.NestedMetaFor(graph.ID()); ok {
		parentGraph := store.Graphs()[meta.ParentGraphID]
		if parentGraph != nil {
			parentScope := r.parentScopeForGraph(store, parentGraph)
			if parentScope != nil {
				if sigs := store.GetLiteralSigsSnapshot(parentGraph, parentScope); len(sigs) > 0 {
					if literalSigMap == nil {
						literalSigMap = make(map[*ast.FunctionExpr]*typ.Function, len(sigs))
					}
					for fnExpr, sig := range sigs {
						if fnExpr != nil && sig != nil {
							if _, exists := literalSigMap[fnExpr]; !exists {
								literalSigMap[fnExpr] = sig
							}
						}
					}
				}
			}
			if sigs := scratchLiteralSigs(store, parentGraph.ID()); len(sigs) > 0 {
				if literalSigMap == nil {
					literalSigMap = make(map[*ast.FunctionExpr]*typ.Function, len(sigs))
				}
				for fnExpr, sig := range sigs {
					if fnExpr != nil && sig != nil {
						if _, exists := literalSigMap[fnExpr]; !exists {
							literalSigMap[fnExpr] = sig
						}
					}
				}
			}
		}
	}
	if len(literalSigMap) > 0 {
		return phase.LiteralSigsMap(literalSigMap)
	}
	return nil
}

type effectStoreProvider interface {
	EffectStore() api.EffectStore
}

func effectStoreFrom(store api.StoreView) api.EffectStore {
	if store == nil {
		return nil
	}
	if provider, ok := store.(effectStoreProvider); ok {
		return provider.EffectStore()
	}
	return nil
}

type scratchLiteralStore interface {
	ScratchLiteralSigs(graphID uint64) map[*ast.FunctionExpr]*typ.Function
}

func scratchLiteralSigs(store api.StoreView, graphID uint64) map[*ast.FunctionExpr]*typ.Function {
	if store == nil {
		return nil
	}
	if provider, ok := store.(scratchLiteralStore); ok {
		return provider.ScratchLiteralSigs(graphID)
	}
	return nil
}

func (r *Runner) parentScopeForGraph(store api.StoreView, graph *cfg.Graph) *scope.State {
	if store == nil || graph == nil {
		return nil
	}
	parentHash := store.GraphParentHashOf(graph.ID())
	if parentHash != 0 {
		if parentScope := store.Parents()[parentHash]; parentScope != nil {
			return parentScope
		}
	}
	if _, ok := store.NestedMetaFor(graph.ID()); !ok {
		return r.stdlib
	}
	return nil
}
