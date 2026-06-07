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
	call    *ast.FuncCallExpr
	ctx     transfer.ProductCallContext
	outcome canonicalcall.CallOutcome
}

func (ct callTyper) productCallFrame(call *ast.FuncCallExpr, ctx transfer.ProductCallContext, opts productCallOutcomeOptions) (productCallFrame, bool) {
	if ct.d == nil || call == nil || ct.d.activeProgram == nil {
		return productCallFrame{}, false
	}
	return productCallFrame{
		typer:   ct,
		call:    call,
		ctx:     ctx,
		outcome: ct.callOutcomeForProductCallWithOptions(call, ctx, opts),
	}, true
}

func (p productCallFrame) inferredReturnValues() []product.AbstractValue {
	return p.outcome.InferredReturnValues()
}

func (p productCallFrame) callReturnValues() ([]product.AbstractValue, bool) {
	argTypes := p.ctx.ArgTypes()
	exprType := p.ctx.ExprType
	summaryReturns := p.inferredReturnValues()
	return canonicalcall.ReturnValueInput{
		Call:                 p.call,
		Env:                  p.typer.callInterceptEnv(exprType),
		TypePolicyAvailable:  p.typer.d.cfg.Types != nil,
		PendingInput:         p.ctx.PendingInput,
		BlockDynamicFallback: p.outcome.HasTargets() && !p.outcome.HasInformativeReturnValues(),
		SummaryReturnValues: func(call *ast.FuncCallExpr) []product.AbstractValue {
			return summaryReturns
		},
		ExprValue: p.ctx.ExprValue,
		TypeFallback: func() ([]typ.Type, bool) {
			proj, ok := p.typer.callReturnProjection(
				p.call,
				argTypes,
				exprType,
				p.ctx.References,
				p.ctx.SelfType,
			)
			if !ok {
				return nil, false
			}
			return proj.types()
		},
	}.Values()
}

func (p productCallFrame) result(effects transfer.CallEffects) transfer.ProductCallResult {
	values, ok := p.callReturnValues()
	return transfer.ProductCallResult{
		ReturnValues:    values,
		HasReturnValues: ok,
		ReturnRefs:      p.outcome.ReturnRefs(),
		ReturnRelations: p.returnRelations(),
		Effects:         effects,
		ArgDemands:      p.callArgDemands(),
		NeverReturns: p.neverReturns(func(ref summary.FuncRef) bool {
			return p.typer.d.activeProgram.facts.HasNoReturn(ref)
		}),
		ParamNarrows: p.paramNarrows(),
	}
}

func (p productCallFrame) returnRelations() flow.ReturnRelations {
	return p.outcome.ReturnRelations(p.call, p.typer.callTypeResolver(p.ctx.ExprType), p.ctx.ExprValue != nil)
}

func (p productCallFrame) callArgDemands() []callobligation.Obligation {
	return (canonicalcall.CallArgDemandProjection{
		Call: p.call,
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
	proj, ok := p.typer.callFunctionShapeProjection(p.call, p.ctx.ExprType)
	if !ok {
		return nil
	}
	return proj.functionShape()
}

func (p productCallFrame) paramNarrows() []transfer.ParamNarrow {
	d := p.typer.d
	if d == nil || d.activeProgram == nil || p.call == nil {
		return nil
	}
	prog := d.activeProgram
	return (canonicalcall.ParamNarrowProjection{
		Call: p.call,
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

func (p productCallFrame) argDemands() ([]callobligation.Obligation, bool) {
	targets := p.demandTargets()
	if len(targets) == 0 {
		return nil, false
	}
	return paramevidence.CallArgDemandProjection{
		Call:    p.call,
		Targets: targets,
	}.Obligations(), true
}

func (p productCallFrame) demandTargets() []paramevidence.CallArgDemandTarget {
	d := p.typer.d
	if d == nil || d.activeProgram == nil || p.call == nil || len(p.call.Args) == 0 || !p.outcome.HasTargets() {
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
				return projector.slotType(ref, p.call, p.ctx.RuntimeArgValues, target.EntryValues, slot)
			},
			SourceParamAnnotated: func(sourceParam int) bool {
				return paramevidence.SourceParamAnnotated(fn, sourceParam)
			},
		})
	}
	return out
}

func (p productCallFrame) effects(projector cellEffectProjector, elementUnions []effect.ContainerElementUnion) transfer.CallEffects {
	callbackRefs := projector.callEntry.productArgProjection(p.ctx).callbackRefsForCall(p.call)
	return transfer.CallEffects{
		CellEffects: p.outcome.CellEffects(summary.CellEffectAggregation{
			CallbackSpec: projector.callbackSpecForCall(p.call, p.ctx.ExprType),
			CallbackArgs: p.call.Args,
			MethodCall:   p.call.Method != "",
			ResolveCallback: func(arg ast.Expr) ([]summary.FuncRef, bool) {
				refs, ok := callbackRefs[arg]
				return refs, ok
			},
			EffectOf: func(ref summary.FuncRef, entryValues summary.EntryValues) flow.CaptureEffects {
				entryFacts := projector.callEntry.access().productFacts(ref, p.call, p.ctx)
				return projector.effectsForRef(ref, p.ctx.References, entryValues, entryFacts)
			},
		}),
		ReceiverEffects: p.outcome.ReceiverEffects(),
		BoundaryFacts:   p.outcome.BoundaryFacts(),
		ElementUnions:   elementUnions,
	}
}

func (p productCallFrame) neverReturns(isNoReturn func(summary.FuncRef) bool) bool {
	return p.outcome.NeverReturns(isNoReturn)
}
