package semantics

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (r *Result) extractBranch(stmt ast.Stmt, kind BranchKind, condition ast.Expr, bindings *bind.Result, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	calls, ok := branchConditionCalls(condition, bindings)
	if !ok {
		return ErrPointMismatch
	}
	if len(points) != len(calls)+1 {
		return ErrPointMismatch
	}
	resolver := callPointResolver(calls, points)
	conditionCall, conditionNegated, hasConditionCall := branchcond.PredicateCall(condition)
	for i, call := range calls {
		context, exprs, exprIndex := CallContextExpressionProducer, []ast.Expr(nil), call.index
		callConditionNegated := false
		if hasConditionCall && call.call == conditionCall {
			context, exprs, exprIndex = CallContextCondition, []ast.Expr{condition}, 0
			callConditionNegated = conditionNegated
		}
		fact := buildCallFact(stmt, nil, context, exprs, exprIndex, call.call, bindings, nil, resolver)
		fact.ConditionNegated = callConditionNegated
		r.setCall(points[i], fact)
	}
	branchPoint := points[len(calls)]
	fact := BranchConditionFact{
		Kind:      kind,
		Stmt:      stmt,
		Condition: condition,
		Source:    conditionValueSource(condition, resolver),
		Check:     branchcond.Normalize(condition, bindings),
	}
	switch stmt := stmt.(type) {
	case *ast.IfStmt:
		fact.If = stmt
	case *ast.WhileStmt:
		fact.While = stmt
	case *ast.RepeatStmt:
		fact.Repeat = stmt
	}
	r.branches[branchPoint] = fact
	return nil
}

func branchConditionCalls(condition ast.Expr, bindings *bind.Result) ([]indexedCall, bool) {
	return exprCalls(condition, bindings)
}
