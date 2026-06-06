package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	canonicalcall "github.com/wippyai/go-lua/compiler/check/canonical/call"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/compiler/check/domain/callobligation"
	"github.com/wippyai/go-lua/types/typ"
)

// callDemandProjection owns the callable shape used by argument-demand fallback.
// Summary-known targets win; otherwise it delegates to caller-visible type
// resolution through the pure call-boundary demand normalizer.
type callDemandProjection struct {
	typer    callTyper
	call     *ast.FuncCallExpr
	exprType func(ast.Expr) typ.Type
}

func (ct callTyper) callDemandProjection(call *ast.FuncCallExpr, exprType func(ast.Expr) typ.Type) (callDemandProjection, bool) {
	if ct.d == nil || call == nil || ct.d.activeProgram == nil {
		return callDemandProjection{}, false
	}
	return callDemandProjection{
		typer:    ct,
		call:     call,
		exprType: exprType,
	}, true
}

func (p callDemandProjection) functionShape() *typ.Function {
	return (canonicalcall.DemandFunctionProjection{
		Call:            p.call,
		SummaryFunction: p.summaryFunction,
		Resolver:        p.typer.callTypeResolver(p.exprType),
	}).Function()
}

func (p callDemandProjection) demands(ctx transfer.ProductCallContext) []callobligation.Obligation {
	return (canonicalcall.CallArgDemandProjection{
		Call: p.call,
		SummaryDemands: func(call *ast.FuncCallExpr) ([]callobligation.Obligation, bool) {
			proj, ok := p.typer.summaryOnlyProductCallProjection(call, ctx)
			if !ok {
				return nil, false
			}
			return proj.argDemands()
		},
		FunctionShape: func(*ast.FuncCallExpr) *typ.Function {
			return p.functionShape()
		},
		SelfType: func(*ast.FuncCallExpr) typ.Type {
			return ctx.SelfType
		},
	}).Demands()
}

func (p callDemandProjection) summaryFunction(call *ast.FuncCallExpr) *typ.Function {
	d := p.typer.d
	if d == nil || d.activeProgram == nil || d.activeQueries == nil || d.activeCtx == nil {
		return nil
	}
	ref, ok := p.typer.resolveCalleeRef(call, d.activeProgram)
	if !ok {
		return nil
	}
	return d.signatureForRef(d.activeProgram, ref)
}
