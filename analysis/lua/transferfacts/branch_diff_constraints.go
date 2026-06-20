package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse/numparse"
)

// branchDiffConstraints extracts difference-logic facts from the true-edge
// conjuncts of a branch condition: relational comparisons between linear path
// terms, e.g. i < j, i + 1 <= #xs, and #a == #b. Pure path-vs-constant bounds
// (i >= 1, #xs > 0) are left to the numeric- and length-floor lanes.
func (l *lowerer) branchDiffConstraints(fact semantics.BranchConditionFact) []factflow.BranchDiffConstraint {
	var out []factflow.BranchDiffConstraint
	for _, cmp := range conjunctComparisons(fact.Condition) {
		out = append(out, l.diffConstraintsFromComparison(cmp)...)
	}
	return out
}

// conjunctComparisons collects the comparisons that all hold when expr is true:
// it descends `and` nodes (true proves both sides) and stops at `or`.
func conjunctComparisons(expr ast.Expr) []*ast.RelationalOpExpr {
	switch e := expr.(type) {
	case *ast.RelationalOpExpr:
		return []*ast.RelationalOpExpr{e}
	case *ast.LogicalOpExpr:
		if e.Operator != "and" {
			return nil
		}
		return append(conjunctComparisons(e.Lhs), conjunctComparisons(e.Rhs)...)
	}
	return nil
}

// linearTerm is value(path) + offset, or len(path) + offset when isLength, or a
// bare constant when !hasPath.
type linearTerm struct {
	path     path.Path
	isLength bool
	offset   int64
	hasPath  bool
}

func (l *lowerer) parseLinearTerm(expr ast.Expr) (linearTerm, bool) {
	switch e := expr.(type) {
	case *ast.NumberExpr:
		v, ok := numparse.ParseIntegerLiteral(e.Value)
		if !ok {
			return linearTerm{}, false
		}
		return linearTerm{offset: v}, true
	case *ast.UnaryMinusOpExpr:
		inner, ok := l.parseLinearTerm(e.Expr)
		if !ok || inner.hasPath {
			return linearTerm{}, false
		}
		return linearTerm{offset: -inner.offset}, true
	case *ast.UnaryLenOpExpr:
		p, ok := pathexpr.Resolve(e.Expr, l.bindings)
		if !ok || p.IsEmpty() {
			return linearTerm{}, false
		}
		return linearTerm{path: p, isLength: true, hasPath: true}, true
	case *ast.ArithmeticOpExpr:
		if e.Operator != "+" && e.Operator != "-" {
			break
		}
		return l.parseArithmeticTerm(e)
	}
	if p, ok := pathexpr.Resolve(expr, l.bindings); ok && !p.IsEmpty() {
		return linearTerm{path: p, hasPath: true}, true
	}
	return linearTerm{}, false
}

// parseArithmeticTerm handles base +/- const and const + base (a single path
// term plus a constant offset).
func (l *lowerer) parseArithmeticTerm(e *ast.ArithmeticOpExpr) (linearTerm, bool) {
	left, leftOK := l.parseLinearTerm(e.Lhs)
	right, rightOK := l.parseLinearTerm(e.Rhs)
	if !leftOK || !rightOK {
		return linearTerm{}, false
	}
	switch {
	case left.hasPath && !right.hasPath:
		off := right.offset
		if e.Operator == "-" {
			off = -off
		}
		left.offset += off
		return left, true
	case !left.hasPath && right.hasPath && e.Operator == "+":
		right.offset += left.offset
		return right, true
	default:
		return linearTerm{}, false
	}
}

// diffConstraintsFromComparison turns `A op B` over two linear path terms into
// difference constraints. value(A.path) + A.offset (op) value(B.path) + B.offset
// becomes value(A.path) - value(B.path) <= B.offset - A.offset (adjusted by op).
func (l *lowerer) diffConstraintsFromComparison(cmp *ast.RelationalOpExpr) []factflow.BranchDiffConstraint {
	a, aOK := l.parseLinearTerm(cmp.Lhs)
	b, bOK := l.parseLinearTerm(cmp.Rhs)
	if !aOK || !bOK || !a.hasPath || !b.hasPath {
		return nil
	}
	le := func(hi, lo linearTerm, strict bool) factflow.BranchDiffConstraint {
		c := lo.offset - hi.offset
		if strict {
			c--
		}
		return factflow.NewBranchDiffConstraint(hi.path, hi.isLength, lo.path, lo.isLength, c)
	}
	switch cmp.Operator {
	case "<=":
		return []factflow.BranchDiffConstraint{le(a, b, false)}
	case "<":
		return []factflow.BranchDiffConstraint{le(a, b, true)}
	case ">=":
		return []factflow.BranchDiffConstraint{le(b, a, false)}
	case ">":
		return []factflow.BranchDiffConstraint{le(b, a, true)}
	case "==":
		return []factflow.BranchDiffConstraint{le(a, b, false), le(b, a, false)}
	}
	return nil
}
