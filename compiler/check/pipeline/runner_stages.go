package pipeline

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/calleffect"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/compiler/check/domain/resolve"
	"github.com/wippyai/go-lua/compiler/check/phase"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func (r *Runner) resolveSynthesizedSignature(
	ctx *db.QueryContext,
	store api.StoreReader,
	graph *cfg.Graph,
	fn *ast.FunctionExpr,
	parent *scope.State,
	flowEvidence api.FlowEvidence,
	parameterEvidenceSigs map[*ast.FunctionExpr][]typ.Type,
) *typ.Function {
	if graph == nil || fn == nil {
		return nil
	}

	factSig := paramevidence.ProjectSignatureToParamUse(graph.ParamSlotsReadOnly(), flowEvidence.ParameterUses, r.functionFactSignatureForFunction(store, graph, fn))
	synthSig := r.literalSignatureForFunction(store, graph, fn)
	baseSig := functionfact.MergeSignature(synthSig, factSig)
	if parameterEvidenceSigs == nil {
		return baseSig
	}
	paramEvidence := parameterEvidenceSigs[fn]
	if len(paramEvidence) == 0 {
		return baseSig
	}
	if baseSig == nil {
		engine := synth.New(synth.Config{
			Ctx:       ctx,
			Types:     r.types,
			Manifests: r.manifests,
			Phase:     api.PhaseTypeResolution,
		})
		if sig := engine.ResolveFunctionSignature(fn, parent); sig != nil {
			baseSig = sig
		} else if seedFn, ok := returns.BuildSeedFunctionTypeWithBindings(fn, engine, parent, graph.Bindings()).(*typ.Function); ok {
			baseSig = seedFn
		}
	}
	if baseSig == nil {
		return nil
	}
	return paramevidence.MergeIntoSignature(fn, paramEvidence, baseSig)
}

func (r *Runner) functionFactSignatureForFunction(
	store api.StoreReader,
	graph *cfg.Graph,
	fn *ast.FunctionExpr,
) *typ.Function {
	if store == nil || graph == nil || fn == nil {
		return nil
	}
	sym, ok := store.SymbolForFunc(fn)
	if !ok || sym == 0 {
		return nil
	}
	meta, ok := store.NestedMetaFor(graph.ID())
	if !ok || meta.ParentGraphID == 0 {
		return nil
	}
	parentGraph := store.Graphs()[meta.ParentGraphID]
	if parentGraph == nil {
		return nil
	}
	parentScope := r.parentScopeForGraph(store, parentGraph)
	if parentScope == nil {
		return nil
	}
	ff, ok := store.InterprocFacts(parentGraph, parentScope).FunctionFact(sym)
	if !ok {
		return nil
	}
	return unwrap.Function(functionfact.ProjectType(ff, functionfact.ProjectionBody, api.PhaseScopeCompute))
}

func (r *Runner) appendCapturedCallEffectAssignments(
	store api.StoreReader,
	graph *cfg.Graph,
	parent *scope.State,
	env phase.PhaseEnv,
	scopeOut phase.ScopeOutput,
	literalOut phase.LiteralOutput,
	functionFacts api.FunctionFacts,
	extractOut *phase.FlowExtractOutput,
) {
	if store == nil || graph == nil || extractOut == nil || extractOut.Inputs == nil {
		return
	}

	product := store.InterprocFacts(graph, parent)
	capturedFields := product.CapturedFieldAssigns()
	capturedContainers := product.CapturedContainerMutations()
	if len(capturedFields) == 0 && len(capturedContainers) == 0 {
		return
	}

	bindings := graph.Bindings()
	if bindings == nil {
		bindings = store.ModuleBindings()
	}

	declaredEnv := phase.NewContextBuilder(env).
		WithScope(scopeOut).
		WithLiteralTypes(literalOut.LiteralTypes).
		WithFunctionFacts(functionFacts).
		BuildFlowInput()

	synthEngine := synth.New(synth.Config{
		Ctx:               env.Ctx,
		Types:             env.Types,
		Scopes:            scopeOut.Scopes,
		Manifests:         env.Manifests,
		Env:               declaredEnv,
		FunctionFacts:     functionFacts,
		Phase:             api.PhaseScopeCompute,
		Evidence:          env.Evidence,
		ModuleBindings:    env.ModuleBindings,
		ModuleAliases:     env.ModuleAliases,
		RecursiveFamilies: env.RecursiveFamilies,
	})

	symResolver := resolve.BuildInputSymbolResolver(declaredEnv, extractOut.Inputs)
	assignmentTypes := resolve.BuildAssignmentTypeResolver(extractOut.Inputs)
	calleeTypeResolver := func(info *cfg.CallInfo, p cfg.Point) typ.Type {
		return resolve.CalleeType(info, p, synthEngine.TypeOf, symResolver, assignmentTypes, graph, bindings, env.ModuleBindings)
	}

	extra := calleffect.CollectNestedAssignments(
		graph,
		bindings,
		extractOut.Evidence.Calls,
		extractOut.Evidence.EscapedFunctions,
		capturedFields,
		capturedContainers,
		calleeTypeResolver,
	)
	if len(extra.Fields) > 0 {
		extractOut.Inputs.Assignments = append(extractOut.Inputs.Assignments, extra.Fields...)
	}
	if len(extra.Map) > 0 {
		extractOut.Inputs.MapMutatorAssignments = append(extractOut.Inputs.MapMutatorAssignments, extra.Map...)
	}
	if len(extra.Table) > 0 {
		extractOut.Inputs.TableMutatorAssignments = append(extractOut.Inputs.TableMutatorAssignments, extra.Table...)
	}
	if len(extra.Container) > 0 {
		extractOut.Inputs.ContainerMutatorAssignments = append(extractOut.Inputs.ContainerMutatorAssignments, extra.Container...)
	}
}

func (r *Runner) runComputePasses(graph *cfg.Graph, scopes map[cfg.Point]*scope.State) map[string]any {
	if graph == nil || len(r.computePasses) == 0 {
		return nil
	}
	extras := make(map[string]any, len(r.computePasses))
	for _, pass := range r.computePasses {
		extras[pass.Name()] = pass.Run(graph, scopes)
	}
	return extras
}

func (r *Runner) literalSignatureForFunction(store api.StoreReader, graph *cfg.Graph, fn *ast.FunctionExpr) *typ.Function {
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

	parentScope := r.parentScopeForGraph(store, parentGraph)
	if parentScope == nil {
		return nil
	}
	if sig, ok := store.InterprocFacts(parentGraph, parentScope).LiteralSig(fn); ok {
		return sig
	}
	return nil
}

func (r *Runner) literalSigProvider(store api.StoreReader, graph *cfg.Graph, parent *scope.State) phase.LiteralSigsProvider {
	if store == nil || graph == nil || parent == nil {
		return nil
	}
	provider := interprocLiteralSigProvider{
		current: store.InterprocFacts(graph, parent),
	}
	if meta, ok := store.NestedMetaFor(graph.ID()); ok {
		parentGraph := store.Graphs()[meta.ParentGraphID]
		if parentGraph != nil {
			parentScope := r.parentScopeForGraph(store, parentGraph)
			if parentScope != nil {
				provider.parent = store.InterprocFacts(parentGraph, parentScope)
			}
		}
	}
	return provider
}

type interprocLiteralSigProvider struct {
	current api.InterprocFactProduct
	parent  api.InterprocFactProduct
}

func (p interprocLiteralSigProvider) Lookup(fn *ast.FunctionExpr) *typ.Function {
	if fn == nil {
		return nil
	}
	if p.current != nil {
		if sig, ok := p.current.LiteralSig(fn); ok {
			return sig
		}
	}
	if p.parent != nil {
		if sig, ok := p.parent.LiteralSig(fn); ok {
			return sig
		}
	}
	return nil
}

func refinementFactsFrom(store api.StoreReader) api.RefinementFacts {
	return functionfact.RefinementsFromStore(store, nil)
}

func (r *Runner) parentScopeForGraph(store api.StoreReader, graph *cfg.Graph) *scope.State {
	if store == nil || graph == nil {
		return nil
	}
	if parentScope := api.ParentScopeForGraph(store, graph.ID(), nil); parentScope != nil {
		return parentScope
	}
	if _, ok := store.NestedMetaFor(graph.ID()); !ok {
		return r.stdlib
	}
	return nil
}

func (r *Runner) mergeCapturedParentFunctionTypes(
	store api.StoreReader,
	graph *cfg.Graph,
	fn *ast.FunctionExpr,
	scopeOut *phase.ScopeOutput,
) {
	if store == nil || graph == nil || fn == nil || scopeOut == nil {
		return
	}
	meta, ok := store.NestedMetaFor(graph.ID())
	if !ok || meta.ParentGraphID == 0 || graph.Bindings() == nil {
		return
	}
	parentGraph := store.Graphs()[meta.ParentGraphID]
	if parentGraph == nil {
		return
	}
	parentScope := r.parentScopeForGraph(store, parentGraph)
	if parentScope == nil {
		return
	}
	parentProduct := store.InterprocFacts(parentGraph, parentScope)
	for _, sym := range graph.Bindings().CapturedSymbols(fn) {
		ff, ok := parentProduct.FunctionFact(sym)
		if !ok {
			continue
		}
		ft := functionfact.ProjectType(ff, functionfact.ProjectionPublic, api.PhaseScopeCompute)
		if sym == 0 || ft == nil {
			continue
		}
		if scopeOut.DeclaredTypes == nil {
			scopeOut.DeclaredTypes = make(flow.DeclaredTypes)
		}
		scopeOut.DeclaredTypes[sym] = ft
	}
}
