package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	canonicalcall "github.com/wippyai/go-lua/compiler/check/canonical/call"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/types/domain/value/product"
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
	return canonicalcall.InferReturnValues(canonicalcall.ReturnValueInput{
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
			return canonicalcall.InferReturnTypes(p.typer.callReturnInput(
				p.call,
				argTypes,
				exprType,
				p.ctx.Cells,
				p.ctx.FunctionRefs,
				p.ctx.ClosureRefs,
				p.ctx.SelfType,
			))
		},
	})
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

func (p productCallProjection) cellEffects(projector cellEffectProjector) flow.CaptureEffects {
	return projector.productCallEffects(p.outcome, p.call, p.ctx)
}

func (p productCallProjection) postEffects() transfer.CallPostEffects {
	return transfer.CallPostEffects{
		ReceiverEffects: p.outcome.ReceiverEffects(),
		BoundaryFacts:   p.outcome.BoundaryFacts(),
	}
}

func (p productCallProjection) neverReturns(isNoReturn func(summary.FuncRef) bool) bool {
	return p.outcome.NeverReturns(isNoReturn)
}
