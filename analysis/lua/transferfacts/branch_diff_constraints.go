package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/bind"
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

// linearTerm is coeff*value(path) + offset, or coeff*len(path) + offset when
// isLength, or a bare constant when !hasPath. coeff is 1 unless a constant
// multiplies a value path; a coefficient on a length term is out of scope.
type linearTerm struct {
	coeff    int64
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
		return linearTerm{coeff: 1, path: p, isLength: true, hasPath: true}, true
	case *ast.ArithmeticOpExpr:
		switch e.Operator {
		case "+", "-":
			return l.parseArithmeticTerm(e)
		case "*":
			return l.parseProductTerm(e)
		}
		return linearTerm{}, false
	}
	if p, ok := pathexpr.Resolve(expr, l.bindings); ok && !p.IsEmpty() {
		return linearTerm{coeff: 1, path: p, hasPath: true}, true
	}
	return linearTerm{}, false
}

// parseProductTerm handles a positive integer constant times a pure value path,
// const * path or path * const, yielding coeff*value(path). It rejects products
// of two paths, a coefficient on a length term, and a non-positive constant.
func (l *lowerer) parseProductTerm(e *ast.ArithmeticOpExpr) (linearTerm, bool) {
	if c, ok := integerConstExpr(e.Lhs); ok {
		return scaledPathTerm(c, e.Rhs, l.bindings)
	}
	if c, ok := integerConstExpr(e.Rhs); ok {
		return scaledPathTerm(c, e.Lhs, l.bindings)
	}
	return linearTerm{}, false
}

func integerConstExpr(expr ast.Expr) (int64, bool) {
	num, ok := expr.(*ast.NumberExpr)
	if !ok {
		return 0, false
	}
	return numparse.ParseIntegerLiteral(num.Value)
}

func scaledPathTerm(coeff int64, pathExpr ast.Expr, bindings *bind.Result) (linearTerm, bool) {
	if coeff <= 0 {
		return linearTerm{}, false
	}
	p, ok := pathexpr.Resolve(pathExpr, bindings)
	if !ok || p.IsEmpty() {
		return linearTerm{}, false
	}
	return linearTerm{coeff: coeff, path: p, hasPath: true}, true
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

// sumTerm is value(t1.path) + value(t2.path) plus a combined constant offset,
// where each component term keeps its isLength flag. It captures a guard side
// that adds two pure path terms, such as i + j.
type sumTerm struct {
	t1     linearTerm
	t2     linearTerm
	offset int64
}

// parseSumTerm parses expr as a two-path sum t1 + t2 (each a pure value or
// length path term, optionally scaled by a positive constant, with at most a
// constant offset). It rejects sums of three or more paths and any side that is
// not a pure path term, so 2*i + 3*j is accepted while a bare product is not.
func (l *lowerer) parseSumTerm(expr ast.Expr) (sumTerm, bool) {
	e, ok := expr.(*ast.ArithmeticOpExpr)
	if !ok || e.Operator != "+" {
		return sumTerm{}, false
	}
	left, leftOK := l.parseLinearTerm(e.Lhs)
	right, rightOK := l.parseLinearTerm(e.Rhs)
	if !leftOK || !rightOK || !left.hasPath || !right.hasPath {
		return sumTerm{}, false
	}
	offset := left.offset + right.offset
	left.offset = 0
	right.offset = 0
	return sumTerm{t1: left, t2: right, offset: offset}, true
}

// diffConstraintsFromComparison turns `A op B` into relational constraints. When
// both sides are single linear path terms it produces a two-term difference. When
// exactly one side is a two-path sum (i + j) and the other is a single path term,
// it produces a bounded three-term sum constraint t1 + t2 - lo <= k.
func (l *lowerer) diffConstraintsFromComparison(cmp *ast.RelationalOpExpr) []factflow.BranchDiffConstraint {
	if out, ok := l.sumConstraintsFromComparison(cmp); ok {
		return out
	}
	a, aOK := l.parseLinearTerm(cmp.Lhs)
	b, bOK := l.parseLinearTerm(cmp.Rhs)
	if !aOK || !bOK || !a.hasPath || !b.hasPath {
		return nil
	}
	// The lower/bound side stays unit-coefficient; a scaled bound term is out of
	// scope. The upper side may carry a positive coefficient on a value path.
	le := func(hi, lo linearTerm, strict bool) (factflow.BranchDiffConstraint, bool) {
		if lo.coeff != 1 || (hi.isLength && hi.coeff != 1) {
			return factflow.BranchDiffConstraint{}, false
		}
		c := lo.offset - hi.offset
		if strict {
			c--
		}
		return factflow.NewBranchScaledConstraint(hi.coeff, hi.path, hi.isLength, 0, path.Path{}, false, lo.path, lo.isLength, c), true
	}
	emit := func(hi, lo linearTerm, strict bool) []factflow.BranchDiffConstraint {
		fact, ok := le(hi, lo, strict)
		if !ok {
			return nil
		}
		return []factflow.BranchDiffConstraint{fact}
	}
	switch cmp.Operator {
	case "<=":
		return emit(a, b, false)
	case "<":
		return emit(a, b, true)
	case ">=":
		return emit(b, a, false)
	case ">":
		return emit(b, a, true)
	case "==":
		var out []factflow.BranchDiffConstraint
		if fact, ok := le(a, b, false); ok {
			out = append(out, fact)
		}
		if fact, ok := le(b, a, false); ok {
			out = append(out, fact)
		}
		return out
	}
	return nil
}

// sumConstraintsFromComparison handles a guard where exactly one side is a
// two-path sum, producing sum branch facts t1 + t2 - lo <= k. The sum must be on
// the upper side of the inequality (sum <= lo), since only that orientation gives
// an upper bound on the two positive operands.
func (l *lowerer) sumConstraintsFromComparison(cmp *ast.RelationalOpExpr) ([]factflow.BranchDiffConstraint, bool) {
	leftSum, leftIsSum := l.parseSumTerm(cmp.Lhs)
	rightSum, rightIsSum := l.parseSumTerm(cmp.Rhs)
	if leftIsSum == rightIsSum {
		return nil, false
	}
	otherExpr, sumOnLeft := cmp.Rhs, true
	sum := leftSum
	if rightIsSum {
		otherExpr, sumOnLeft = cmp.Lhs, false
		sum = rightSum
	}
	other, otherOK := l.parseLinearTerm(otherExpr)
	if !otherOK || !other.hasPath {
		return nil, false
	}
	// The bound side stays unit-coefficient; a scaled length term on either
	// positive operand is out of scope.
	if other.coeff != 1 || (sum.t1.isLength && sum.t1.coeff != 1) || (sum.t2.isLength && sum.t2.coeff != 1) {
		return nil, false
	}
	// sumLe builds coHi*sum.t1 + coHi2*sum.t2 - other <= k for the two positive
	// operands against value(other)+other.offset.
	sumLe := func(strict bool) factflow.BranchDiffConstraint {
		k := other.offset - sum.offset
		if strict {
			k--
		}
		return factflow.NewBranchScaledConstraint(sum.t1.coeff, sum.t1.path, sum.t1.isLength, sum.t2.coeff, sum.t2.path, sum.t2.isLength, other.path, other.isLength, k)
	}
	upper := func(op string) bool {
		switch op {
		case "<=":
			return sumOnLeft // sum <= other
		case "<":
			return sumOnLeft
		case ">=":
			return !sumOnLeft // other >= sum  =>  sum <= other
		case ">":
			return !sumOnLeft
		}
		return false
	}
	switch cmp.Operator {
	case "<=", ">=":
		if upper(cmp.Operator) {
			return []factflow.BranchDiffConstraint{sumLe(false)}, true
		}
	case "<", ">":
		if upper(cmp.Operator) {
			return []factflow.BranchDiffConstraint{sumLe(true)}, true
		}
	case "==":
		return []factflow.BranchDiffConstraint{sumLe(false)}, true
	}
	return nil, false
}
