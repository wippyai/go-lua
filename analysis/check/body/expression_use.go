package body

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/compiler/ast"
)

// ExpressionUseRole identifies why a reachable expression is attached to a CFG
// point. It is syntax metadata only; solved state still owns value facts.
type ExpressionUseRole uint8

const (
	ExpressionUseLocalAssignmentSource ExpressionUseRole = iota
	ExpressionUseOrdinaryAssignmentSource
	ExpressionUseOrdinaryAssignmentTarget
	ExpressionUseCall
	ExpressionUseReturn
	ExpressionUseBranchCondition
	ExpressionUseExpressionEvaluation
)

// ExpressionUse is a syntax expression attached to a reachable CFG point.
// It centralizes the remaining AST/source scan policy while WIR/factflow own
// value and path facts.
type ExpressionUse struct {
	Point cfg.Point
	Role  ExpressionUseRole
	Expr  ast.Expr
}

// ForEachReachableExpressionUse visits reachable expression positions in
// deterministic RPO order.
func (r *Result) ForEachReachableExpressionUse(visit func(ExpressionUse) bool) bool {
	if r == nil || visit == nil || r.Graph() == nil {
		return false
	}
	visited := false
	emit := func(point cfg.Point, role ExpressionUseRole, expr ast.Expr) bool {
		if expr == nil {
			return true
		}
		visited = true
		return visit(ExpressionUse{Point: point, Role: role, Expr: expr})
	}
	for _, point := range r.Graph().RPO() {
		if !r.PointNormallyReachable(point) {
			continue
		}
		for _, use := range r.expressionUsesAt(point) {
			if !emit(use.Point, use.Role, use.Expr) {
				return true
			}
		}
	}
	return visited
}

func (r *Result) expressionUsesAt(point cfg.Point) []ExpressionUse {
	if r == nil {
		return nil
	}
	var out []ExpressionUse
	add := func(role ExpressionUseRole, expr ast.Expr) {
		if expr != nil {
			out = append(out, ExpressionUse{Point: point, Role: role, Expr: expr})
		}
	}
	if fact, ok := r.LocalAssignment(point); ok {
		add(ExpressionUseLocalAssignmentSource, fact.Expr)
	}
	if fact, ok := r.OrdinaryAssignment(point); ok {
		add(ExpressionUseOrdinaryAssignmentSource, fact.Value)
		add(ExpressionUseOrdinaryAssignmentTarget, fact.Target)
	}
	if fact, ok := r.Call(point); ok {
		add(ExpressionUseCall, fact.Call)
	}
	if fact, ok := r.ReturnFact(point); ok {
		for _, expr := range fact.Exprs {
			add(ExpressionUseReturn, expr)
		}
	}
	if fact, ok := r.BranchCondition(point); ok {
		add(ExpressionUseBranchCondition, fact.Condition)
	}
	return out
}
