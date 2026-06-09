package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
)

// comparisonTruthyOnOperand reports whether the guard condition is a value comparison
// (`a == b`, `a ~= b`, `a < b`, ...) whose resolved check is the fall-through truthy /
// falsy, in which case the operand's truthiness is NOT constrained by the comparison's
// result. A comparison the CFG could interpret as a presence/type guard carries
// CheckNil / CheckNotNil / CheckTypeEqual / CheckTypeNot instead (and is narrowed
// correctly); only the uninterpreted comparison reduces to the bare truthy/falsy that
// would otherwise project the comparison's truthiness onto its root operand symbol. A
// `not (a == b)` wraps the comparison in a CheckFalsy, so the inner comparison is
// unwrapped. A non-comparison condition (a bare value / field path) returns false so the
// ordinary truthy/falsy narrowing runs.
func comparisonTruthyOnOperand(cond ast.Expr, check cfg.CondCheckKind) bool {
	switch check {
	case cfg.CheckTruthy, cfg.CheckFalsy:
	default:
		return false
	}
	expr := cond
	if not, ok := expr.(*ast.UnaryNotOpExpr); ok {
		expr = not.Expr
	}
	rel, ok := expr.(*ast.RelationalOpExpr)
	if !ok {
		return false
	}
	switch rel.Operator {
	case "==", "~=", "<", "<=", ">", ">=":
		return true
	default:
		return false
	}
}

// narrowIndexPresenceLength records `#arr >= i` on the guarded edge when the guard
// is a positive presence check (truthy / not-nil) on a literal index path arr[i]
// rooted at sym. A present element at index i implies the container has at least i
// elements, the sound length floor an in-range index read consults to drop its
// optional element. A negative check, a non-index or non-literal-index path, or a
// missing numeric component leaves the state unchanged. The numeric component is
// cloned before the floor is applied so the shared predecessor state is never
// mutated; the merge-LUB rebuilds the unbounded length past the guard.
func (t *Transfer) narrowIndexPresenceLength(res flow.PointState, sym cfg.SymbolID, segments []constraint.Segment, check cfg.CondCheckKind) flow.PointState {
	switch check {
	case cfg.CheckTruthy, cfg.CheckNotNil:
	default:
		return res
	}
	if len(segments) != 1 {
		return res
	}
	seg := segments[0]
	if seg.Kind != constraint.SegmentIndexInt || seg.Index < 1 {
		return res
	}
	if op, ok := flow.NumericLenGeConstSymbolOp(sym, int64(seg.Index)); ok {
		flow.ApplyNumericEffect(&res, flow.NumericEffect{
			Ops:             []flow.NumericOp{op},
			RequireExisting: true,
		})
	}
	return res
}

func (t *Transfer) numericForBodyEdgeState(g *cfg.Graph, branch cfg.Point, out flow.PointState) flow.PointState {
	node := g.Node(branch)
	if node == nil || !node.LoopPreheaderSet {
		return out
	}
	info := g.Assign(node.LoopPreheader)
	if info == nil || info.NumericFor == nil || info.NumericFor.VarName == "" {
		return out
	}
	target, ok := info.FirstTarget()
	if !ok || target.Kind != cfg.TargetIdent || target.Symbol == 0 {
		return out
	}
	res := flow.ClonePointState(out)
	t.seedNumericForBodyBounds(&res, target.Symbol, info.NumericFor)
	return res
}

// narrowNumericComparison seeds the numeric component with the integer bound a
// relational comparison guard proves on the edge it holds. It recognizes a guard
// `var OP bound` where var is a tracked symbol and bound is an integer constant or
// `#container` over a tracked sequence, on either side of the comparison. On the
// taken edge the comparison's effective operator holds; on the not-taken edge its
// negation. The proven `var <= c` / `var >= c` is applied as a constant bound, and
// `var <= #container (+/- k)` as a symbolic length reference, the same bound forms
// a numeric-for loop seeds, so a body read `container[var]` consults them through
// the in-range index narrowing. A guard the helper cannot classify as a numeric
// comparison on a tracked symbol leaves the state unchanged (precision loss, never
// unsoundness). The numeric component is cloned before the bound is applied so the
// shared predecessor state is never mutated; the merge-LUB rebuilds the unbounded
// range past the guard.
func (t *Transfer) narrowNumericComparison(out flow.PointState, info *cfg.BranchInfo, taken bool) flow.PointState {
	if info == nil {
		return out
	}
	rel, ok := info.Condition.(*ast.RelationalOpExpr)
	if !ok {
		return out
	}
	// A `#container OP const` length guard (`#x > 0`, `#x >= 1`, `#x ~= 0`, `#x == 0`)
	// bounds the container's length on the edge it holds, raising the proven length
	// floor (or ceiling) the same numeric component an in-range index read consults.
	// It is recognized before the index-variable orientation because the guarded value
	// is `#container`, not a tracked integer variable.
	if narrowed, applied := t.narrowLengthGuard(out, rel, info, taken); applied {
		return narrowed
	}
	op := rel.Operator
	switch op {
	case "<", "<=", ">", ">=":
	default:
		return out
	}
	// Resolve which side is the tracked integer variable and which is the bound,
	// orienting the operator so it always reads `var OP bound`.
	varExpr, boundExpr, op, ok := t.orientComparison(rel.Lhs, rel.Rhs, op)
	if !ok {
		return out
	}
	idxIdent, ok := varExpr.(*ast.IdentExpr)
	if !ok {
		return out
	}
	idxSym := t.symbolOf(idxIdent)
	if idxSym == 0 {
		return out
	}
	// The CFG records the whole comparison branch as a truthy check; the taken edge
	// holds the comparison as written, the not-taken edge its logical negation.
	if !effectiveTruthy(info.CondCheck.Kind, taken) {
		op = negateComparisonOp(op)
	}
	idxKey, ok := flow.NumericVarKeyOfSymbol(idxSym)
	if !ok {
		return out
	}
	if c, ok := t.constInt(boundExpr); ok {
		ops := flow.NumericConstComparisonOps(idxKey, op, c)
		if len(ops) == 0 {
			return out
		}
		res := flow.ClonePointState(out)
		flow.ApplyNumericEffect(&res, flow.NumericEffect{Ops: ops})
		return res
	}
	// `var <= #container` / `var < #container`: a symbolic length reference. Only the
	// upper-bound senses bound the index by the container length; a lower-bound sense
	// does not establish in-range presence and is left unseeded.
	if container, off, ok := t.lengthBoundComparison(boundExpr, op); ok {
		numericOp, ok := flow.NumericVarLeLenOffsetContainerOp(idxSym, container, off)
		if !ok {
			return out
		}
		res := flow.ClonePointState(out)
		flow.ApplyNumericEffect(&res, flow.NumericEffect{
			Ops: []flow.NumericOp{numericOp},
		})
		return res
	}
	return out
}

// narrowLengthGuard seeds the container length bound a `#container OP const` guard
// proves on the edge it holds. It orients the comparison so the `#container` side is
// the value and the integer constant the bound, applies the comparison's
// proven-edge operator, and translates it into a length floor / ceiling on the
// container's canonical flow ref:
//
//   - `#x >  c`  proves  #x >= c+1  (floor c+1)
//   - `#x >= c`  proves  #x >= c    (floor c)
//   - `#x <  c`  proves  #x <= c-1  (ceiling c-1)
//   - `#x <= c`  proves  #x <= c    (ceiling c)
//   - `#x == c`  proves  #x >= c AND #x <= c
//   - `#x ~= 0`  proves  #x >= 1    (a sequence length is non-negative, so != 0 is >= 1)
//
// A `~= c` for c other than 0 proves nothing soundly (the length may sit above or
// below c), so it is declined. A floor of c >= 1 on the true edge lets a later
// in-range index read `x[1]` / `x[#x]` in the guarded block drop its soundly-optional
// element through the same length proof a bounded loop seeds. The numeric component is
// cloned before the bound is applied so the shared predecessor state is never mutated;
// the merge-LUB rebuilds the unbounded length past the guard. A comparison neither side
// of which is `#container` over a tracked sequence, or whose other side is not an integer
// constant, reports applied=false so the index-variable orientation runs.
func (t *Transfer) narrowLengthGuard(out flow.PointState, rel *ast.RelationalOpExpr, info *cfg.BranchInfo, taken bool) (flow.PointState, bool) {
	switch rel.Operator {
	case "<", "<=", ">", ">=", "==", "~=":
	default:
		return out, false
	}
	place, container, c, op, ok := t.orientLengthComparison(rel.Lhs, rel.Rhs, rel.Operator)
	if !ok {
		return out, false
	}
	// The CFG records the comparison branch as a truthy/falsy check; the taken edge
	// holds the comparison as written, the not-taken edge its logical negation.
	if !effectiveTruthy(info.CondCheck.Kind, taken) {
		op = negateLengthOp(op)
	}
	floor, _, hasFloor, hasCeil := flow.LengthBoundFromOp(op, c)
	if !hasFloor && !hasCeil {
		return out, false
	}
	ops := flow.NumericLengthBoundContainerOps(container, op, c)
	res := flow.ClonePointState(out)
	flow.ApplyNumericEffect(&res, flow.NumericEffect{Ops: ops})
	if hasFloor && floor > 0 {
		t.applyRefinementEffect(&res, RefinementEffect{
			Place:     place,
			Kind:      RefinementLengthLowerBound,
			LengthMin: floor,
			PreferEnv: true,
		})
	}
	return res, true
}

// orientLengthComparison resolves a `#container OP const` comparison into the
// container's canonical flow ref, the integer constant, and the operator oriented so it
// reads `#container OP const`. The `#container` side may be either operand; when it
// is on the right the operator is flipped. A comparison neither side of which is
// `#container` over a tracked sequence, or whose other side is not an integer
// constant, reports ok=false.
func (t *Transfer) orientLengthComparison(lhs, rhs ast.Expr, op string) (Place, flow.ContainerRef, int64, string, bool) {
	if place, container, ok := t.lenExprContainerPlace(lhs); ok {
		if c, ok := t.constInt(rhs); ok {
			return place, container, c, op, true
		}
	}
	if place, container, ok := t.lenExprContainerPlace(rhs); ok {
		if c, ok := t.constInt(lhs); ok {
			return place, container, c, flipComparisonOp(op), true
		}
	}
	return Place{}, flow.ContainerRef{}, 0, op, false
}

// negateLengthOp returns the logical negation of a relational operator over the
// integer length, including the equality operators the index-variable negation does
// not handle (not (a == b) is a ~= b).
func negateLengthOp(op string) string {
	switch op {
	case "==":
		return "~="
	case "~=":
		return "=="
	default:
		return negateComparisonOp(op)
	}
}

// orientComparison resolves which operand of a relational comparison is the
// candidate index variable and which is the bound, returning them in `var OP bound`
// orientation with the operator flipped when the variable is on the right. It
// prefers an identifier operand as the variable; when both or neither are
// identifiers it picks the left as the variable. A comparison with no usable
// operand reports ok=false.
func (t *Transfer) orientComparison(lhs, rhs ast.Expr, op string) (ast.Expr, ast.Expr, string, bool) {
	_, lIdent := lhs.(*ast.IdentExpr)
	_, rIdent := rhs.(*ast.IdentExpr)
	switch {
	case lIdent:
		return lhs, rhs, op, true
	case rIdent:
		return rhs, lhs, flipComparisonOp(op), true
	default:
		return nil, nil, op, false
	}
}

// flipComparisonOp swaps the sides of a relational operator (a < b  <=>  b > a).
func flipComparisonOp(op string) string {
	switch op {
	case "<":
		return ">"
	case "<=":
		return ">="
	case ">":
		return "<"
	case ">=":
		return "<="
	}
	return op
}

// negateComparisonOp returns the logical negation of a relational operator over
// integers: not (a < b) is a >= b, etc.
func negateComparisonOp(op string) string {
	switch op {
	case "<":
		return ">="
	case "<=":
		return ">"
	case ">":
		return "<="
	case ">=":
		return "<"
	}
	return op
}

// effectiveTruthy reports whether the branch condition holds on the chosen edge: a
// CheckTruthy branch holds on the taken edge, a CheckFalsy on the not-taken. A
// comparison the CFG could not classify as a presence/type guard falls through to
// one of these bare truthiness checks.
func effectiveTruthy(check cfg.CondCheckKind, taken bool) bool {
	switch check {
	case cfg.CheckFalsy:
		return !taken
	default:
		return taken
	}
}

// lengthBoundComparison recognizes a `var <= #container` / `var < #container`
// upper bound, returning the container's canonical flow ref and the inclusive integer
// offset (a strict `<` is `<= #container - 1`). Only the upper-bound senses (`<`,
// `<=`) bound the index by the container length; a lower-bound sense proves no
// in-range presence and reports ok=false.
func (t *Transfer) lengthBoundComparison(boundExpr ast.Expr, op string) (flow.ContainerRef, int64, bool) {
	container, ok := t.lenExprContainerRef(boundExpr)
	if !ok {
		return flow.ContainerRef{}, 0, false
	}
	switch op {
	case "<=":
		return container, 0, true
	case "<":
		return container, -1, true
	default:
		return flow.ContainerRef{}, 0, false
	}
}
