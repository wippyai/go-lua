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
	if len(points) != 1 {
		return ErrPointMismatch
	}
	r.calls[points[0]] = buildCallFact(stmt, stmt, CallContextStatement, []ast.Expr{call}, 0, call, bindings, nil)
	return nil
}

func (r *Result) extractReturn(stmt *ast.ReturnStmt, bindings *bind.Result, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	calls := topLevelValueListCalls(stmt.Exprs)
	if len(points) != len(calls)+1 {
		return ErrPointMismatch
	}
	for i, call := range calls {
		r.calls[points[i]] = buildCallFact(stmt, nil, CallContextReturnSource, stmt.Exprs, call.index, call.call, bindings, nil)
	}
	returnPoint := points[len(calls)]
	r.returns[returnPoint] = ReturnFact{
		Stmt:    stmt,
		Exprs:   copyExprs(stmt.Exprs),
		Sources: returnValueSources(stmt.Exprs, callPointsByExprIndex(calls, points)),
	}
	return nil
}
