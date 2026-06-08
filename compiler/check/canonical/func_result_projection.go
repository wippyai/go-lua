package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	canonref "github.com/wippyai/go-lua/compiler/check/canonical/ref"
	canonicalsig "github.com/wippyai/go-lua/compiler/check/canonical/signature"
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
	driver  *Driver
	session api.AnalysisSession
	program *program
	queries *summary.Queries
	ref     summary.FuncRef
	graph   *cfg.Graph
}

func (d *Driver) funcResultProjection(sess api.AnalysisSession, prog *program, queries *summary.Queries, ref summary.FuncRef) funcResultProjection {
	return funcResultProjection{
		driver:  d,
		session: sess,
		program: prog,
		queries: queries,
		ref:     ref,
		graph:   prog.Graph(ref),
	}
}

func (p funcResultProjection) build() *api.FuncResult {
	evidence := p.evidence()
	callEdges := p.callEdgeEvidence(evidence)
	facts := p.functionFacts()
	flowProjection := p.driver.newCanonicalFacts(
		p.graph,
		p.driver.states[p.ref],
		facts,
		p.program,
		p.queries,
		p.session.Context(),
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
		ReturnRelations:          p.driver.summaryReader().ReturnRelations(p.ref),
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
	result.FnRefinement = p.driver.summaryReader().ReturnPostconditions(p.ref).FunctionRefinement(
		p.program.facts.HasNoReturn(p.ref),
	)
	return result
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
	projection, ok := p.driver.solvedCallEvidenceProjection(p.program, p.ref, evidence)
	if !ok {
		return solvedCallEdgeEvidence{}
	}
	return projection.project()
}

func (p funcResultProjection) evidence() api.FlowEvidence {
	if store := p.session.StoreHandle(); store != nil && p.graph != nil {
		return store.EvidenceForGraph(p.graph)
	}
	return api.FlowEvidence{}
}

func (p funcResultProjection) functionFacts() functionFacts {
	facts := cloneFunctionFacts(p.program.functionFacts[p.ref])
	funcSigs := p.program.facts.FunctionBindingTypes(func(ref canonref.FuncRef) typ.Type {
		return p.driver.signatureForRef(p.program, ref)
	})
	recordFunctionBindingTypes(&facts, funcSigs, p.graph)
	recordCallbackEnvBindingTypes(&facts, p.program.facts.CallbackEnv(p.ref))
	return facts
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
