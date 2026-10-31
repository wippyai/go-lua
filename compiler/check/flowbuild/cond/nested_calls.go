package cond

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// evaluatedNestedCalls visits only calls that must run before expr finishes.
// Function bodies and the right side of a short-circuit operator are excluded.
func evaluatedNestedCalls(expr ast.Expr, graph *cfg.Graph) []*cfg.CallInfo {
	if graph == nil {
		return nil
	}
	var calls []*cfg.CallInfo
	var visit func(ast.Expr)
	visit = func(expr ast.Expr) {
		switch e := expr.(type) {
		case *ast.FuncCallExpr:
			visit(e.Func)
			visit(e.Receiver)
			for _, arg := range e.Args {
				visit(arg)
			}
			calls = append(calls, cfg.BuildCallInfoWithBindings(e, graph.Bindings()))
		case *ast.AttrGetExpr:
			visit(e.Object)
			visit(e.Key)
		case *ast.TableExpr:
			for _, field := range e.Fields {
				visit(field.Key)
				visit(field.Value)
			}
		case *ast.LogicalOpExpr:
			visit(e.Lhs)
		case *ast.RelationalOpExpr:
			visit(e.Lhs)
			visit(e.Rhs)
		case *ast.ArithmeticOpExpr:
			visit(e.Lhs)
			visit(e.Rhs)
		case *ast.StringConcatOpExpr:
			visit(e.Lhs)
			visit(e.Rhs)
		case *ast.UnaryMinusOpExpr:
			visit(e.Expr)
		case *ast.UnaryNotOpExpr:
			visit(e.Expr)
		case *ast.UnaryLenOpExpr:
			visit(e.Expr)
		case *ast.UnaryBNotOpExpr:
			visit(e.Expr)
		case *ast.CastExpr:
			visit(e.Expr)
		case *ast.NonNilAssertExpr:
			visit(e.Expr)
		}
	}
	visit(expr)
	return calls
}

// stableNestedFacts keeps only facts about direct local values. An enclosing
// call can mutate an object field or invoke a closure that rebinds a capture,
// so those facts need more precise effect tracking before they can be retained.
func stableNestedFacts(cond constraint.Condition, graph *cfg.Graph, capturedReassignments, assigned map[cfg.SymbolID]bool) constraint.Condition {
	if !cond.HasConstraints() || graph == nil || graph.Bindings() == nil {
		return constraint.Condition{}
	}
	var kept []constraint.Constraint
	for _, fact := range cond.MustConstraints() {
		stable := true
		constraint.VisitPaths(fact, func(p constraint.Path) bool {
			if p.Symbol == 0 || len(p.Segments) != 0 || capturedReassignments[p.Symbol] || assigned[p.Symbol] {
				stable = false
				return true
			}
			kind, ok := graph.Bindings().Kind(p.Symbol)
			if !ok || (kind != cfg.SymbolLocal && kind != cfg.SymbolParam) {
				stable = false
				return true
			}
			return false
		})
		if stable {
			kept = append(kept, fact)
		}
	}
	return constraint.FromConstraints(kept...)
}
