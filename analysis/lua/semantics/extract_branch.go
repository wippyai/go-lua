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
	calls := branchConditionCalls(condition)
	if len(points) != len(calls)+1 {
		return ErrPointMismatch
	}
	for i, call := range calls {
		r.calls[points[i]] = buildCallFact(stmt, nil, CallContextCondition, []ast.Expr{condition}, call.index, call.call, bindings, nil)
	}
	branchPoint := points[len(calls)]
	fact := BranchConditionFact{
		Kind:      kind,
		Stmt:      stmt,
		Condition: condition,
		Source:    conditionValueSource(condition, callPointsByExprIndex(calls, points)),
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

func branchConditionCalls(condition ast.Expr) []indexedCall {
	call, _, ok := branchcond.PredicateCall(condition)
	if !ok {
		return nil
	}
	return []indexedCall{{index: 0, call: call}}
}
