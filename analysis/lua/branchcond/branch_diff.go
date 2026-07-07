package branchcond

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse/numparse"
)

// LinearTerm is coeff*value(path) + offset, or coeff*len(path) + offset when
// IsLength, or a bare constant when !HasPath. It is syntax-derived branch
// metadata; transfer decides which fact lanes the normalized relation implies.
type LinearTerm struct {
	Coeff    int64
	Path     path.Path
	IsLength bool
	Offset   int64
	HasPath  bool
}

// BranchDiffConstraint is a normalized difference-logic branch descriptor:
//
//	coHi*hi + coHi2*hi2 - lo <= c
//
// Edge is the outer branch edge carrying the proof. HasHi2 is false for ordinary
// two-term differences and true for bounded sum constraints.
type BranchDiffConstraint struct {
	CoHi     int64
	HiPath   path.Path
	HiIsLen  bool
	CoHi2    int64
	Hi2Path  path.Path
	Hi2IsLen bool
	HasHi2   bool
	LoPath   path.Path
	LoIsLen  bool
	C        int64
	Edge     bool
}

// BranchDiffConstraintsOnBothEdges extracts normalized difference-logic branch
// descriptors from relational comparisons proven on either branch edge.
func BranchDiffConstraintsOnBothEdges(expr ast.Expr, bindings *bind.Result) []BranchDiffConstraint {
	var out []BranchDiffConstraint
	for _, implied := range ImpliedRelationalOpsOnEdge(expr, true) {
		out = append(out, diffConstraintsFromComparisonOnEdge(implied.Expr, bindings, implied.Edge, implied.Polarity)...)
	}
	for _, implied := range ImpliedRelationalOpsOnEdge(expr, false) {
		out = append(out, diffConstraintsFromComparisonOnEdge(implied.Expr, bindings, implied.Edge, implied.Polarity)...)
	}
	return out
}

func parseLinearTerm(expr ast.Expr, bindings *bind.Result) (LinearTerm, bool) {
	switch e := expr.(type) {
	case *ast.NumberExpr:
		v, ok := numparse.ParseIntegerLiteral(e.Value)
		if !ok {
			return LinearTerm{}, false
		}
		return LinearTerm{Offset: v}, true
	case *ast.UnaryMinusOpExpr:
		inner, ok := parseLinearTerm(e.Expr, bindings)
		if !ok || inner.HasPath {
			return LinearTerm{}, false
		}
		return LinearTerm{Offset: -inner.Offset}, true
	case *ast.UnaryLenOpExpr:
		p, ok := pathexpr.ResolveLengthOperand(e, bindings)
		if !ok || p.IsEmpty() {
			return LinearTerm{}, false
		}
		return LinearTerm{Coeff: 1, Path: p, IsLength: true, HasPath: true}, true
	case *ast.ArithmeticOpExpr:
		switch e.Operator {
		case "+", "-":
			return parseArithmeticTerm(e, bindings)
		case "*":
			return parseProductTerm(e, bindings)
		}
		return LinearTerm{}, false
	}
	if p, ok := pathexpr.Resolve(expr, bindings); ok && !p.IsEmpty() {
		return LinearTerm{Coeff: 1, Path: p, HasPath: true}, true
	}
	return LinearTerm{}, false
}

// parseProductTerm handles a positive integer constant times a pure value path,
// const * path or path * const, yielding coeff*value(path). It rejects products
// of two paths, a coefficient on a length term, and a non-positive constant.
func parseProductTerm(e *ast.ArithmeticOpExpr, bindings *bind.Result) (LinearTerm, bool) {
	if c, ok := integerConstExpr(e.Lhs); ok {
		return scaledPathTerm(c, e.Rhs, bindings)
	}
	if c, ok := integerConstExpr(e.Rhs); ok {
		return scaledPathTerm(c, e.Lhs, bindings)
	}
	return LinearTerm{}, false
}

func integerConstExpr(expr ast.Expr) (int64, bool) {
	num, ok := expr.(*ast.NumberExpr)
	if !ok {
		return 0, false
	}
	return numparse.ParseIntegerLiteral(num.Value)
}

func scaledPathTerm(coeff int64, pathExpr ast.Expr, bindings *bind.Result) (LinearTerm, bool) {
	if coeff <= 0 {
		return LinearTerm{}, false
	}
	p, ok := pathexpr.Resolve(pathExpr, bindings)
	if !ok || p.IsEmpty() {
		return LinearTerm{}, false
	}
	return LinearTerm{Coeff: coeff, Path: p, HasPath: true}, true
}

// parseArithmeticTerm handles base +/- const and const + base (a single path
// term plus a constant offset).
func parseArithmeticTerm(e *ast.ArithmeticOpExpr, bindings *bind.Result) (LinearTerm, bool) {
	left, leftOK := parseLinearTerm(e.Lhs, bindings)
	right, rightOK := parseLinearTerm(e.Rhs, bindings)
	if !leftOK || !rightOK {
		return LinearTerm{}, false
	}
	switch {
	case left.HasPath && !right.HasPath:
		off := right.Offset
		if e.Operator == "-" {
			off = -off
		}
		left.Offset += off
		return left, true
	case !left.HasPath && right.HasPath && e.Operator == "+":
		right.Offset += left.Offset
		return right, true
	default:
		return LinearTerm{}, false
	}
}

type sumTerm struct {
	t1     LinearTerm
	t2     LinearTerm
	offset int64
}

// parseSumTerm parses expr as a two-path sum t1 + t2 (each a pure value or
// length path term, optionally scaled by a positive constant, with at most a
// constant offset). It rejects sums of three or more paths and any side that is
// not a pure path term, so 2*i + 3*j is accepted while a bare product is not.
func parseSumTerm(expr ast.Expr, bindings *bind.Result) (sumTerm, bool) {
	e, ok := expr.(*ast.ArithmeticOpExpr)
	if !ok || e.Operator != "+" {
		return sumTerm{}, false
	}
	left, leftOK := parseLinearTerm(e.Lhs, bindings)
	right, rightOK := parseLinearTerm(e.Rhs, bindings)
	if !leftOK || !rightOK || !left.HasPath || !right.HasPath {
		return sumTerm{}, false
	}
	offset := left.Offset + right.Offset
	left.Offset = 0
	right.Offset = 0
	return sumTerm{t1: left, t2: right, offset: offset}, true
}

func diffConstraintsFromComparisonOnEdge(cmp *ast.RelationalOpExpr, bindings *bind.Result, edge bool, polarity bool) []BranchDiffConstraint {
	op := cmp.Operator
	if !polarity {
		var ok bool
		op, ok = negatedDiffRelop(op)
		if !ok {
			return nil
		}
	}
	return diffConstraintsFromRelop(cmp.Lhs, op, cmp.Rhs, bindings, edge)
}

func negatedDiffRelop(op string) (string, bool) {
	switch op {
	case "<":
		return ">=", true
	case "<=":
		return ">", true
	case ">":
		return "<=", true
	case ">=":
		return "<", true
	case "~=":
		return "==", true
	default:
		return "", false
	}
}

func diffConstraintsFromRelop(lhs ast.Expr, op string, rhs ast.Expr, bindings *bind.Result, edge bool) []BranchDiffConstraint {
	if out, ok := sumConstraintsFromComparison(lhs, op, rhs, bindings, edge); ok {
		return out
	}
	a, aOK := parseLinearTerm(lhs, bindings)
	b, bOK := parseLinearTerm(rhs, bindings)
	if !aOK || !bOK || !a.HasPath || !b.HasPath {
		return nil
	}
	le := func(hi, lo LinearTerm, strict bool) (BranchDiffConstraint, bool) {
		if lo.Coeff != 1 || (hi.IsLength && hi.Coeff != 1) {
			return BranchDiffConstraint{}, false
		}
		c := lo.Offset - hi.Offset
		if strict {
			c--
		}
		return BranchDiffConstraint{CoHi: hi.Coeff, HiPath: hi.Path, HiIsLen: hi.IsLength, LoPath: lo.Path, LoIsLen: lo.IsLength, C: c, Edge: edge}, true
	}
	emit := func(hi, lo LinearTerm, strict bool) []BranchDiffConstraint {
		fact, ok := le(hi, lo, strict)
		if !ok {
			return nil
		}
		return []BranchDiffConstraint{fact}
	}
	switch op {
	case "<=":
		return emit(a, b, false)
	case "<":
		return emit(a, b, true)
	case ">=":
		return emit(b, a, false)
	case ">":
		return emit(b, a, true)
	case "==":
		var out []BranchDiffConstraint
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
func sumConstraintsFromComparison(lhs ast.Expr, op string, rhs ast.Expr, bindings *bind.Result, edge bool) ([]BranchDiffConstraint, bool) {
	leftSum, leftIsSum := parseSumTerm(lhs, bindings)
	rightSum, rightIsSum := parseSumTerm(rhs, bindings)
	if leftIsSum == rightIsSum {
		return nil, false
	}
	otherExpr, sumOnLeft := rhs, true
	sum := leftSum
	if rightIsSum {
		otherExpr, sumOnLeft = lhs, false
		sum = rightSum
	}
	other, otherOK := parseLinearTerm(otherExpr, bindings)
	if !otherOK || !other.HasPath {
		return nil, false
	}
	if other.Coeff != 1 || (sum.t1.IsLength && sum.t1.Coeff != 1) || (sum.t2.IsLength && sum.t2.Coeff != 1) {
		return nil, false
	}
	sumLe := func(strict bool) BranchDiffConstraint {
		k := other.Offset - sum.offset
		if strict {
			k--
		}
		return BranchDiffConstraint{
			CoHi:     sum.t1.Coeff,
			HiPath:   sum.t1.Path,
			HiIsLen:  sum.t1.IsLength,
			CoHi2:    sum.t2.Coeff,
			Hi2Path:  sum.t2.Path,
			Hi2IsLen: sum.t2.IsLength,
			HasHi2:   true,
			LoPath:   other.Path,
			LoIsLen:  other.IsLength,
			C:        k,
			Edge:     edge,
		}
	}
	upper := func(op string) bool {
		switch op {
		case "<=", "<":
			return sumOnLeft
		case ">=", ">":
			return !sumOnLeft
		}
		return false
	}
	switch op {
	case "<=", ">=":
		if upper(op) {
			return []BranchDiffConstraint{sumLe(false)}, true
		}
	case "<", ">":
		if upper(op) {
			return []BranchDiffConstraint{sumLe(true)}, true
		}
	case "==":
		return []BranchDiffConstraint{sumLe(false)}, true
	}
	return nil, false
}
