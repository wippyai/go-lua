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

// productCallProjection is the canonical adapter from one product call context
// to the caller-visible projections derived from its selected summary outcome.
type productCallProjection struct {
	typer   callTyper
	call    *ast.FuncCallExpr
	ctx     transfer.ProductCallContext
	outcome canonicalcall.CallOutcome
}

func (ct callTyper) productCallProjection(call *ast.FuncCallExpr, ctx transfer.ProductCallContext, opts productCallOutcomeOptions) (productCallProjection, bool) {
	if ct.d == nil || call == nil || ct.d.activeProgram == nil {
		return productCallProjection{}, false
	}
	return productCallProjection{
		typer:   ct,
		call:    call,
		ctx:     ctx,
		outcome: ct.callOutcomeForProductCallWithOptions(call, ctx, opts),
	}, true
}

func (ct callTyper) summaryOnlyProductCallProjection(call *ast.FuncCallExpr, ctx transfer.ProductCallContext) (productCallProjection, bool) {
	return ct.productCallProjection(call, ctx, productCallOutcomeOptions{
		skipSignatureReturns:   true,
		skipSignatureRelations: true,
	})
}

func (ct callTyper) productCallRelationsProjection(call *ast.FuncCallExpr, ctx transfer.ProductCallContext) (productCallProjection, bool) {
	if ct.d == nil || call == nil {
		return productCallProjection{}, false
	}
	proj := productCallProjection{
		typer: ct,
		call:  call,
		ctx:   ctx,
	}
	if ct.d.activeProgram != nil {
		proj.outcome = ct.callOutcomeForProductCall(call, ctx)
	}
	return proj, true
}

func (p productCallProjection) inferredReturnValues() []product.AbstractValue {
	return p.outcome.InferredReturnValues()
}

func (p productCallProjection) callReturnValues() ([]product.AbstractValue, bool) {
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
				p.ctx.Cells,
				p.ctx.FunctionRefs,
				p.ctx.ClosureRefs,
				p.ctx.SelfType,
			)
			if !ok {
				return nil, false
			}
			return proj.types()
		},
	}.Values()
}

func (p productCallProjection) result(projector cellEffectProjector, elementUnions []effect.ContainerElementUnion) transfer.ProductCallResult {
	values, ok := p.callReturnValues()
	return transfer.ProductCallResult{
		ReturnValues:    values,
		HasReturnValues: ok,
		Effects:         p.effects(projector, elementUnions),
	}
}

func (p productCallProjection) returnRefs() transfer.CallReturnRefs {
	return transfer.CallReturnRefs{
		FunctionRefs: p.outcome.ReturnFunctionRefs(),
		ClosureRefs:  p.outcome.ReturnClosureRefs(),
	}
}

func (p productCallProjection) returnRelations() flow.ReturnRelations {
	return p.outcome.ReturnRelations(p.call, p.typer.callTypeResolver(p.ctx.ExprType), p.ctx.ExprValue != nil)
}

func (p productCallProjection) argDemands() ([]callobligation.Obligation, bool) {
	targets := p.demandTargets()
	if len(targets) == 0 {
		return nil, false
	}
	return paramevidence.CallArgDemandProjection{
		Call:    p.call,
		Targets: targets,
	}.Obligations(), true
}

func (p productCallProjection) demandTargets() []paramevidence.CallArgDemandTarget {
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

func (p productCallProjection) effects(projector cellEffectProjector, elementUnions []effect.ContainerElementUnion) transfer.CallEffects {
	return transfer.CallEffects{
		CellEffects:     projector.productCallEffects(p.outcome, p.call, p.ctx),
		ReceiverEffects: p.outcome.ReceiverEffects(),
		BoundaryFacts:   p.outcome.BoundaryFacts(),
		ElementUnions:   elementUnions,
	}
}

func (p productCallProjection) neverReturns(isNoReturn func(summary.FuncRef) bool) bool {
	return p.outcome.NeverReturns(isNoReturn)
}
