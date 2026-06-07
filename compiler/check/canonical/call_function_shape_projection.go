package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	canonicalcall "github.com/wippyai/go-lua/compiler/check/canonical/call"
	"github.com/wippyai/go-lua/types/typ"
)

// callFunctionShapeProjection owns the callable shape used by call-boundary
// fallback. Summary-known targets win; otherwise it delegates to caller-visible
// type resolution through the pure call-boundary normalizer.
type callFunctionShapeProjection struct {
	typer    callTyper
	call     *ast.FuncCallExpr
	exprType func(ast.Expr) typ.Type
}

func (ct callTyper) callFunctionShapeProjection(call *ast.FuncCallExpr, exprType func(ast.Expr) typ.Type) (callFunctionShapeProjection, bool) {
	if ct.d == nil || call == nil || ct.d.activeProgram == nil {
		return callFunctionShapeProjection{}, false
	}
	return callFunctionShapeProjection{
		typer:    ct,
		call:     call,
		exprType: exprType,
	}, true
}

func (p callFunctionShapeProjection) functionShape() *typ.Function {
	return (canonicalcall.DemandFunctionProjection{
		Call:            p.call,
		SummaryFunction: p.summaryFunction,
		Resolver:        p.typer.callTypeResolver(p.exprType),
	}).Function()
}

func (p callFunctionShapeProjection) summaryFunction(call *ast.FuncCallExpr) *typ.Function {
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
