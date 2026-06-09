package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
)

// applyAssertNarrow refines out by the truthy refinement an assert(cond, ...) call
// proves about its first argument in the CONTINUATION after the call. assert raises
// when cond is falsy, so a body reaching the next point has cond truthy; the
// continuation therefore narrows the asserted value exactly as the TRUE edge of an
// `if cond` branch would. It recognizes the same condition shapes the branch
// narrowing does — a bare value (assert(x) / assert(x.f), truthy), a not-nil / nil
// comparison (assert(x ~= nil), assert(x == nil)), and a discriminant equality
// (assert(x.tag == "a")) — reusing the value-domain narrowers. A refinement that
// collapses the asserted value to the lattice Bottom proves the continuation
// unreachable: dead reports true so the caller terminates the flow (assert(false),
// or assert of an always-false comparison), the same way error() does.
//
// An argument shape the narrowing does not interpret leaves out unchanged and dead
// false (a precision loss, never unsoundness): the assert still type-checks its
// argument through the ordinary call-arg demand.
func (t *Transfer) applyAssertNarrow(out *flow.PointState, call *ast.FuncCallExpr) (dead bool) {
	if call == nil || len(call.Args) == 0 {
		return false
	}
	arg := call.Args[0]
	// assert(<falsy literal>) always raises: the continuation is unreachable.
	if isAlwaysFalsyLiteral(arg) {
		return true
	}
	sym, segments, check, ok := t.assertCondition(arg)
	if ok {
		return t.narrowAssertPath(out, sym, segments, check)
	}
	if t.narrowAssertDiscriminant(out, arg) {
		return false
	}
	return false
}

// assertCondition resolves the asserted argument to the (symbol, field path, check)
// the continuation narrows. A bare identifier or field path is a truthy check; a
// `path ~= nil` / `path == nil` comparison is the corresponding presence check. The
// path's root must bind to a tracked symbol. Returns ok=false for any other shape.
func (t *Transfer) assertCondition(arg ast.Expr) (cfg.SymbolID, []constraint.Segment, cfg.CondCheckKind, bool) {
	switch e := arg.(type) {
	case *ast.IdentExpr:
		if sym := t.symbolOf(e); sym != 0 {
			return sym, nil, cfg.CheckTruthy, true
		}
	case *ast.AttrGetExpr:
		if sym, segs, ok := t.pathSymbol(e); ok {
			return sym, segs, cfg.CheckTruthy, true
		}
	case *ast.RelationalOpExpr:
		return t.assertNilComparison(e)
	}
	return 0, nil, cfg.CheckNone, false
}

// assertNilComparison resolves a `path ~= nil` / `path == nil` assert argument to its
// presence check on the path's symbol: `~= nil` proves the continuation not-nil,
// `== nil` proves it nil. The nil literal may be on either side. A comparison that is
// not against nil, or whose other side is not a tracked path, returns ok=false.
func (t *Transfer) assertNilComparison(rel *ast.RelationalOpExpr) (cfg.SymbolID, []constraint.Segment, cfg.CondCheckKind, bool) {
	var notNil bool
	switch rel.Operator {
	case "~=":
		notNil = true
	case "==":
		notNil = false
	default:
		return 0, nil, cfg.CheckNone, false
	}
	path, ok := assertNilPathSide(rel.Lhs, rel.Rhs)
	if !ok {
		return 0, nil, cfg.CheckNone, false
	}
	sym, segs, ok := t.pathSymbol(path)
	if !ok {
		return 0, nil, cfg.CheckNone, false
	}
	if notNil {
		return sym, segs, cfg.CheckNotNil, true
	}
	return sym, segs, cfg.CheckNil, true
}

// assertNilPathSide returns the non-nil operand of a comparison whose other operand
// is the nil literal. It reports false when neither operand is nil.
func assertNilPathSide(lhs, rhs ast.Expr) (ast.Expr, bool) {
	if _, ok := rhs.(*ast.NilExpr); ok {
		return lhs, true
	}
	if _, ok := lhs.(*ast.NilExpr); ok {
		return rhs, true
	}
	return nil, false
}

// narrowAssertPath narrows the asserted symbol (or field path under it) in out by
// the proven check and reports whether the refinement proves the continuation dead.
// It refines over the declared-type base (narrowBase) so a `local s: string?`
// parameter narrows its declared union, then writes the result back. A refinement
// that collapses the value to Bottom (an asserted comparison that cannot hold for
// the value's type, e.g. assert(x == nil) over a non-optional x) terminates the
// flow. An unrefined value leaves out unchanged.
func (t *Transfer) narrowAssertPath(out *flow.PointState, sym cfg.SymbolID, segments []constraint.Segment, check cfg.CondCheckKind) (dead bool) {
	return t.narrowAssertPathWithBase(out, sym, segments, check, false)
}

func (t *Transfer) narrowAssertPathWithBase(out *flow.PointState, sym cfg.SymbolID, segments []constraint.Segment, check cfg.CondCheckKind, preferEnv bool) (dead bool) {
	return t.narrowAssertPathWithTypeName(out, sym, segments, check, "", preferEnv)
}

func (t *Transfer) narrowAssertPathWithTypeName(out *flow.PointState, sym cfg.SymbolID, segments []constraint.Segment, check cfg.CondCheckKind, typeName string, preferEnv bool) (dead bool) {
	baseAV, has := t.narrowBaseFor(*out, sym, preferEnv)
	if !has {
		return false
	}
	narrowed, ok := narrowAtPath(baseAV, segments, check, typeName)
	if !ok {
		return false
	}
	if product.Domain.Equal(narrowed, product.Bottom()) {
		// The asserted condition cannot hold for this value: the continuation is
		// unreachable, so the caller terminates the flow like error().
		return true
	}
	t.setNarrowedSymbol(out, sym, narrowed)
	return false
}

// narrowAssertDiscriminant applies a discriminant-equality assert (assert(x.tag ==
// "a")) by narrowing x's union to the matching variant in out, reusing the branch
// discriminant narrowing on the TRUE edge. It reports whether a discriminant guard
// was recognized; a non-discriminant argument leaves out unchanged.
func (t *Transfer) narrowAssertDiscriminant(out *flow.PointState, arg ast.Expr) bool {
	info := &cfg.BranchInfo{Condition: arg}
	narrowed, applied := t.narrowByDiscriminant(*out, info, true)
	if !applied {
		return false
	}
	applyNarrowedEdgeState(out, narrowed)
	return true
}

// isAlwaysFalsyLiteral reports whether expr is a literal that is always falsy in Lua
// (nil or false), so an assert of it always raises and the continuation is dead.
func isAlwaysFalsyLiteral(expr ast.Expr) bool {
	switch expr.(type) {
	case *ast.NilExpr, *ast.FalseExpr:
		return true
	default:
		return false
	}
}
