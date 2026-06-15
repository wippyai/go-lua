package semantics

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (r *Result) extractCall(stmt *ast.FuncCallStmt, bindings *bind.Result, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	call, ok := stmt.Expr.(*ast.FuncCallExpr)
	if !ok {
		return nil
	}
	calls, ok := exprCalls(stmt.Expr, bindings)
	if !ok {
		return ErrPointMismatch
	}
	if len(points) != len(calls) {
		return ErrPointMismatch
	}
	resolver := callPointResolver(calls, points)
	for i, occurrence := range calls {
		context, exprs, exprIndex := CallContextExpressionProducer, []ast.Expr(nil), occurrence.index
		callStmt := (*ast.FuncCallStmt)(nil)
		if occurrence.call == call {
			context, exprs, exprIndex = CallContextStatement, []ast.Expr{call}, 0
			callStmt = stmt
		}
		r.calls[points[i]] = buildCallFact(stmt, callStmt, context, exprs, exprIndex, occurrence.call, bindings, nil, resolver)
	}
	r.extractObjectLiteral(stmt.Expr, resolver)
	return nil
}

func (r *Result) extractReturn(stmt *ast.ReturnStmt, bindings *bind.Result, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	calls, ok := valueListCalls(stmt.Exprs, bindings)
	if !ok {
		return ErrPointMismatch
	}
	if len(points) != len(calls)+1 {
		return ErrPointMismatch
	}
	resolver := callPointResolver(calls, points)
	for i, call := range calls {
		context, exprs := CallContextExpressionProducer, []ast.Expr(nil)
		if topLevelValueListCall(stmt.Exprs, call) {
			context, exprs = CallContextReturnSource, stmt.Exprs
		}
		r.calls[points[i]] = buildCallFact(stmt, nil, context, exprs, call.index, call.call, bindings, nil, resolver)
	}
	returnPoint := points[len(calls)]
	r.returns[returnPoint] = ReturnFact{
		Stmt:    stmt,
		Exprs:   copyExprs(stmt.Exprs),
		Sources: returnValueSources(stmt.Exprs, resolver),
	}
	r.extractObjectLiterals(stmt.Exprs, resolver)
	return nil
}
