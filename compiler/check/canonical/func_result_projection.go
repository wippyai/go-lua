package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	canonref "github.com/wippyai/go-lua/compiler/check/canonical/ref"
	canonicalsig "github.com/wippyai/go-lua/compiler/check/canonical/signature"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/domain/observation"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/resolve"
	"github.com/wippyai/go-lua/types/typ"
)

// funcResultProjection bridges converged canonical state to the public
// api.FuncResult consumed by diagnostics. It projects solved carriers into API
// fields with matching semantics and keeps immutable extraction evidence intact.
type funcResultProjection struct {
	driver      *Driver
	session     api.AnalysisSession
	program     *program
	artifact    canonicalSolveArtifact
	diagnostics diagnosticObservationArtifact
	ref         summary.FuncRef
	graph       *cfg.Graph
}

func (d *Driver) funcResultProjection(sess api.AnalysisSession, prog *program, artifact canonicalSolveArtifact, diagnostics diagnosticObservationArtifact, ref summary.FuncRef) funcResultProjection {
	return funcResultProjection{
		driver:      d,
		session:     sess,
		program:     prog,
		artifact:    artifact,
		diagnostics: diagnostics,
		ref:         ref,
		graph:       prog.Graph(ref),
	}
}

func (p funcResultProjection) build() *api.FuncResult {
	evidence := p.evidence()
	callEdges := p.callEdgeEvidence(evidence)
	facts := p.observationContext()
	sum := summary.SummaryDomain.Bottom()
	if p.artifact.Summaries != nil {
		sum = p.artifact.Summaries[p.ref]
	}
	reader := summary.NewSnapshotReader(p.artifact.Snapshot)
	flowProjection := p.driver.newCanonicalFacts(
		p.graph,
		p.observationState(),
		facts,
		p.program,
		reader,
		evidence,
	)
	literalSignatures := p.literalSignatures()
	pointScopes := p.driver.buildPointScopes(p.graph)
	sourceSignature := p.driver.declaredSignatureForRef(p.program, p.ref)
	result := &api.FuncResult{
		Graph:                    p.graph,
		Evidence:                 evidence,
		GlobalTypes:              p.driver.globalTypes.ToMap(),
		GlobalTypeBindings:       p.driver.globalTypes,
		BaseScope:                p.driver.returnScope(p.graph),
		Scopes:                   pointScopes,
		FlowInputs:               buildObservationInputs(p.graph, facts),
		Facts:                    flowProjection,
		FlowProjection:           flowProjection,
		ReturnRelations:          sum.Relations,
		LiteralSignatures:        literalSignatures,
		LiteralSignatureProvider: api.LiteralSignatureLookupFromMap(literalSignatures),
		SourceSignature:          sourceSignature,
		PublicSeedSignature:      sourceSignature,
		TypeOps:                  p.driver.cfg.Types,
		QueryContext:             p.session.Context(),
		Extras:                   p.driver.runComputePasses(p.graph, pointScopes),
		DepthLimitExceeded:       p.driver.scopeDepthExceededFor(p.graph),
	}
	obs := observation.FromFuncResult(result, nil).WithProofValues()
	result.ResolvedTypeDefs = p.resolvedTypeDefs(pointScopes, obs.TypeOf)
	result.CallExpectedArgs = callEdges.ExpectedArgs
	result.CallContracts = callEdges.Contracts
	result.NarrowSynth = &returnSynth{
		driver: p.driver,
		obs:    obs.TypeOf,
		ctx:    result.QueryContext,
	}
	result.FnRefinement = sum.Postconditions.FunctionRefinement(p.program.facts.HasNoReturn(p.ref))
	return result
}

func (p funcResultProjection) observationState() state.FunctionState {
	if fs, ok := p.diagnostics.FunctionStates[p.ref]; ok {
		return state.CloneFunctionState(fs)
	}
	if contexts := p.diagnostics.Contexts[p.ref]; len(contexts) != 0 {
		out := state.FunctionStateDomain.Bottom()
		found := false
		for _, key := range contexts {
			fs, ok := p.diagnostics.States[key]
			if !ok {
				continue
			}
			if !found {
				out = state.CloneFunctionState(fs)
				found = true
				continue
			}
			out = state.FunctionStateDomain.Join(out, fs)
		}
		if found {
			return state.CloneFunctionState(out)
		}
	}
	return p.artifact.States[p.ref]
}

func (p funcResultProjection) resolvedTypeDefs(pointScopes map[cfg.Point]*scope.State, typeOf api.ExprSynth) map[string]typ.Type {
	if p.driver == nil || p.graph == nil || typeOf == nil {
		return nil
	}
	var out map[string]typ.Type
	p.graph.EachTypeDef(func(point cfg.Point, info *cfg.TypeDefInfo) {
		if info == nil || info.Name == "" || info.TypeExpr == nil {
			return
		}
		sc := pointScopes[point]
		if sc == nil {
			sc = p.driver.returnScope(p.graph)
		}
		typePoint := point
		resolver := resolve.New(resolve.Config{
			Manifests:      p.driver.cfg.Manifests,
			ModuleBindings: p.driver.moduleBindings,
			ModuleAliases:  p.driver.moduleAliases,
			ExprSynth: func(expr ast.Expr, _ cfg.Point) typ.Type {
				return typeOf(expr, typePoint)
			},
		})
		resolved := resolver.ResolveTypeDef(info.Name, info.TypeExpr, scope.ToTypeParamExprs(info.TypeParams), sc)
		if resolved == nil {
			return
		}
		if _, isGeneric := resolved.(*typ.Generic); !isGeneric {
			resolved = typ.NewAlias(info.Name, resolved)
		}
		if out == nil {
			out = make(map[string]typ.Type)
		}
		out[info.Name] = resolved
	})
	return out
}

func (p funcResultProjection) callEdgeEvidence(evidence api.FlowEvidence) solvedCallEdgeEvidence {
	projection, ok := p.driver.solvedCallEvidenceProjection(p.program, p.artifact, p.ref, evidence)
	if !ok {
		return solvedCallEdgeEvidence{}
	}
	return projection.project()
}

func (p funcResultProjection) evidence() api.FlowEvidence {
	if p.session != nil && p.graph != nil {
		return p.session.EvidenceForGraph(p.graph)
	}
	return api.FlowEvidence{}
}

func (p funcResultProjection) observationContext() functionObservationContext {
	obsCtx := cloneFunctionObservationContext(p.program.observationContexts[p.ref])
	funcSigs := p.program.facts.FunctionBindingTypes(func(ref canonref.FuncRef) typ.Type {
		return p.driver.signatureForRef(p.program, ref)
	})
	recordFunctionBindingTypes(&obsCtx, funcSigs, p.graph)
	recordCallbackEnvBindingTypes(&obsCtx, p.program.facts.CallbackEnv(p.ref))
	return obsCtx
}

func (p funcResultProjection) literalSignatures() map[*ast.FunctionExpr]*typ.Function {
	return canonicalsig.LiteralInput{
		Graph:           p.graph,
		Base:            p.driver.baseScope(),
		ResolveType:     p.driver.resolveType,
		InferredReturns: p.driver.inferredReturnsForFunction,
		MethodFor: func(fn *ast.FunctionExpr) *cfg.FuncDefInfo {
			if ref, ok := p.program.refByFunc(fn); ok {
				return p.program.methodDef(ref)
			}
			return nil
		},
	}.Signatures()
}
