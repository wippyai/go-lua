package semantics

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (r *Result) extractLocalAssign(stmt *ast.LocalAssignStmt, bindings *bind.Result, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	calls, ok := valueListCalls(stmt.Exprs, bindings)
	if !ok {
		return ErrPointMismatch
	}
	if len(points) != len(calls)+len(stmt.Names) {
		return ErrPointMismatch
	}
	targets := localResultTargets(stmt, bindings)
	resolver := callPointResolver(calls, points)
	for i, call := range calls {
		context, exprs, callTargets := CallContextExpressionProducer, []ast.Expr(nil), []CallResultTarget(nil)
		if topLevelValueListCall(stmt.Exprs, call) {
			context, exprs, callTargets = CallContextAssignmentSource, stmt.Exprs, targets
		}
		r.setCall(points[i], buildCallFact(stmt, nil, context, exprs, call.index, call.call, bindings, callTargets, resolver))
	}
	return nil
}

func (r *Result) extractAssign(stmt *ast.AssignStmt, bindings *bind.Result, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	calls, ok := valueListCalls(stmt.Rhs, bindings)
	if !ok {
		return ErrPointMismatch
	}
	if len(points) != len(calls)+len(stmt.Lhs) {
		return ErrPointMismatch
	}
	targets := ordinaryResultTargets(stmt, bindings)
	resolver := callPointResolver(calls, points)
	for i, call := range calls {
		context, exprs, callTargets := CallContextExpressionProducer, []ast.Expr(nil), []CallResultTarget(nil)
		if topLevelValueListCall(stmt.Rhs, call) {
			context, exprs, callTargets = CallContextAssignmentSource, stmt.Rhs, targets
		}
		r.setCall(points[i], buildCallFact(stmt, nil, context, exprs, call.index, call.call, bindings, callTargets, resolver))
	}
	return nil
}
