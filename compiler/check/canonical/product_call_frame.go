package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	canonicalcall "github.com/wippyai/go-lua/compiler/check/canonical/call"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/compiler/check/domain/callobligation"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// productCallFrame is the canonical semantic frame for one product-domain call
// boundary. It is built once from the call expression, solved point context, and
// selected summary outcome; callers ask the frame for outward projections instead
// of reconstructing call facts through driver helpers.
type productCallFrame struct {
	typer   callTyper
	ctx     transfer.ProductCallContext
	site    callSiteFrame
	outcome canonicalcall.CallOutcome
}

func (ct callTyper) productCallFrame(call *ast.FuncCallExpr, ctx transfer.ProductCallContext, opts productCallOutcomeOptions) (productCallFrame, bool) {
	if ct.d == nil || call == nil || ct.d.activeProgram == nil {
		return productCallFrame{}, false
	}
	site, ok := ct.productCallSiteFrame(call, ctx)
	if !ok {
		return productCallFrame{}, false
	}
	return productCallFrame{
		typer:   ct,
		ctx:     ctx,
		site:    site,
		outcome: ct.productCallOutcomeProjection(site, ctx, opts, nil).outcome(),
	}, true
}

func (p productCallFrame) inferredReturnValues() []product.AbstractValue {
	return p.outcome.InferredReturnValues()
}

func (p productCallFrame) callReturnValues() ([]product.AbstractValue, bool) {
	summaryReturns := p.inferredReturnValues()
	return canonicalcall.ReturnValueInput{
		Call:                 p.site.call,
		Env:                  p.typer.callInterceptEnv(p.site.exprType),
		TypePolicyAvailable:  p.typer.d.cfg.Types != nil,
		PendingInput:         p.ctx.PendingInput,
		BlockDynamicFallback: p.outcome.HasTargets() && !p.outcome.HasInformativeReturnValues(),
		SummaryReturnValues: func(call *ast.FuncCallExpr) []product.AbstractValue {
			return summaryReturns
		},
		ExprValue: p.ctx.ExprValue,
		TypeFallback: func() ([]typ.Type, bool) {
			return p.site.returnTypes(func(call *ast.FuncCallExpr, exprType func(ast.Expr) typ.Type) []typ.Type {
				return p.typer.callOutcomeForTypedCall(call, exprType, p.site.references).InferredReturnTypes()
			})
		},
	}.Values()
}

func (p productCallFrame) result(evidence canonicalcall.BoundaryEvidence, effects transfer.CallEffects) transfer.ProductCallResult {
	values, ok := p.callReturnValues()
	return transfer.ProductCallResult{
		ReturnValues:    values,
		HasReturnValues: ok,
		ReturnRefs:      evidence.ReturnRefs,
		ReturnRelations: evidence.ReturnRelations,
		Effects:         effects,
		ArgDemands:      evidence.ArgDemands,
		NeverReturns:    evidence.NeverReturns,
		Postconditions:  evidence.Postconditions,
		ParamNarrows:    evidence.ParamNarrows,
	}
}

func (p productCallFrame) callArgDemands() []callobligation.Obligation {
	return (canonicalcall.CallArgDemandProjection{
		Call: p.site.call,
		SummaryDemands: func(*ast.FuncCallExpr) ([]callobligation.Obligation, bool) {
			return p.argDemands()
		},
		FunctionShape: func(*ast.FuncCallExpr) *typ.Function {
			return p.callFunctionShape()
		},
		SelfType: func(*ast.FuncCallExpr) typ.Type {
			return p.ctx.SelfType
		},
	}).Demands()
}

func (p productCallFrame) callFunctionShape() *typ.Function {
	return p.site.functionShape()
}

func (p productCallFrame) paramNarrows() []transfer.ParamNarrow {
	d := p.typer.d
	if d == nil || d.activeProgram == nil || p.site.call == nil {
		return nil
	}
	prog := d.activeProgram
	return (canonicalcall.ParamNarrowProjection{
		Call: p.site.call,
		SummaryNarrows: func(call *ast.FuncCallExpr) ([]paramevidence.ParamNarrow, bool) {
			ref, ok := p.typer.resolveCalleeRef(call, prog)
			if !ok {
				return nil, false
			}
			return d.summaryReader().ParamNarrows(ref), true
		},
		Resolver: p.typer.callTypeResolver(nil),
	}).Narrows()
}

func (p productCallFrame) returnPostconditions() paramevidence.ReturnPostconditions {
	d := p.typer.d
	if d == nil || d.activeProgram == nil || p.site.call == nil {
		return paramevidence.ReturnPostconditionsDomain.Bottom()
	}
	if p.outcome.HasTargets() {
		return p.outcome.Postconditions()
	}
	return (canonicalcall.PostconditionProjection{
		Call:     p.site.call,
		Resolver: p.typer.callTypeResolver(nil),
	}).Postconditions()
}

func (p productCallFrame) argDemands() ([]callobligation.Obligation, bool) {
	targets := p.demandTargets()
	if len(targets) == 0 {
		return nil, false
	}
	return paramevidence.CallArgDemandProjection{
		Call:    p.site.call,
		Targets: targets,
	}.Obligations(), true
}

func (p productCallFrame) demandTargets() []paramevidence.CallArgDemandTarget {
	d := p.typer.d
	if d == nil || d.activeProgram == nil || p.site.call == nil || len(p.site.call.Args) == 0 || !p.outcome.HasTargets() {
		return nil
	}
	prog := d.activeProgram
	projector, ok := p.typer.callEntryProjector()
	if !ok {
		return nil
	}
	currentRef, hasCurrentRef := p.typer.currentRef()
	targets := p.outcome.Targets()
	out := make([]paramevidence.CallArgDemandTarget, 0, len(targets))
	for _, target := range targets {
		ref := target.Ref
		if hasCurrentRef && ref == currentRef {
			continue
		}
		fn := prog.funcExpr(ref)
		out = append(out, paramevidence.CallArgDemandTarget{
			Graph:     prog.Graph(ref),
			Function:  fn,
			Contracts: prog.publicPredicateContracts(ref, target.Summary.Params),
			DeclaredSlotType: func(slot int) typ.Type {
				return prog.paramSlotDeclaredType(ref, slot)
			},
			EntrySlotType: func(slot int) typ.Type {
				return projector.slotType(ref, p.site.call, p.ctx.RuntimeArgValues, target.EntryValues, slot)
			},
			SourceParamAnnotated: func(sourceParam int) bool {
				return paramevidence.SourceParamAnnotated(fn, sourceParam)
			},
		})
	}
	return out
}

func (p productCallFrame) boundaryEvidence(cellEffects summary.CellEffectAggregation) canonicalcall.BoundaryEvidence {
	return p.outcome.BoundaryEvidence(canonicalcall.BoundaryEvidenceInput{
		Call:                 p.site.call,
		Resolver:             p.typer.callTypeResolver(p.site.exprType),
		UseResolvedSignature: p.ctx.ExprValue != nil,
		CellEffects:          cellEffects,
		ArgDemands:           p.callArgDemands(),
		Postconditions:       p.returnPostconditions(),
		ParamNarrows:         p.paramNarrows(),
		HasNoReturn: func(ref summary.FuncRef) bool {
			return p.typer.d.activeProgram.facts.HasNoReturn(ref)
		},
	})
}

func (p productCallFrame) cellEffectAggregation(projector cellEffectProjector) summary.CellEffectAggregation {
	callbackRefs := projector.callEntry.productArgProjection(p.ctx).callbackRefsForCall(p.site.call)
	return summary.CellEffectAggregation{
		CallbackSpec: projector.callbackSpecForCall(p.site.call, p.site.exprType),
		CallbackArgs: p.site.call.Args,
		MethodCall:   p.site.call.Method != "",
		ResolveCallback: func(arg ast.Expr) ([]summary.FuncRef, bool) {
			refs, ok := callbackRefs[arg]
			return refs, ok
		},
		EffectOf: func(ref summary.FuncRef, entryValues summary.EntryValues) flow.CaptureEffects {
			entryFacts := projector.callEntry.access().productFacts(ref, p.site.call, p.ctx)
			return projector.effectsForRef(ref, p.site.references, entryValues, entryFacts)
		},
	}
}

func callEffectsFromBoundaryEvidence(evidence canonicalcall.BoundaryEvidence, elementUnions []effect.ContainerElementUnion) transfer.CallEffects {
	return transfer.CallEffects{
		CellEffects:     evidence.CellEffects,
		ReceiverEffects: evidence.ReceiverEffects,
		BoundaryFacts:   evidence.BoundaryFacts,
		ElementUnions:   elementUnions,
	}
}
