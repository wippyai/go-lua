package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/guard"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// ParamNarrow is one parameter-narrowing effect a function body proves on every
// live exit. It is transfer-local extraction vocabulary; summary lowering must
// convert it into ReturnPostconditions before it crosses a function boundary.
type ParamNarrow = paramevidence.ParamNarrow

// ParamNarrowEffects extracts the parameter-narrowing effects this function's body
// proves on every normal exit: an assert(param-path[, msg]) whose continuation is
// the only live path, and an `if param-path == nil then error() end` (or `if not
// param-path then ...`) guard whose then-arm terminates. Both reduce to "the
// parameter satisfies a presence/truthy check whenever the function returns". A
// pattern testing a non-parameter value, or a guard whose then-arm does not
// terminate, yields no effect.
func (t *Transfer) ParamNarrowEffects() []ParamNarrow {
	g := t.in.Graph
	if g == nil || t.params.IsEmpty() {
		return nil
	}
	var out []ParamNarrow
	add := func(e ParamNarrow, ok bool) {
		if !ok {
			return
		}
		out = append(out, e)
	}
	g.EachCall(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil || info.CalleeName != "assert" || info.Call == nil || len(info.Call.Args) == 0 {
			return
		}
		// Only an assert that runs on EVERY normal return refines the parameter for
		// every caller: an assert nested in a conditional arm may be skipped.
		if !dominatesExit(g, p) {
			return
		}
		add(t.paramEffectFromCondition(info.Call.Args[0], false))
	})
	// A type-cast `T(param)` that dominates the exit asserts the parameter IS T on
	// every normal return. The cast appears as a call site, so visit EachCallSite.
	g.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil || info.Call == nil || info.CalleeName == "assert" {
			return
		}
		if !dominatesExit(g, p) {
			return
		}
		add(t.paramCastEffect(info.Call))
	})
	g.EachBranch(func(p cfg.Point, info *cfg.BranchInfo) {
		if info == nil || !dominatesExit(g, p) {
			return
		}
		e, ok := t.paramEffectFromGuard(g, p, info)
		add(e, ok)
	})
	return paramevidence.SortParamNarrows(out)
}

// paramEffectFromGuard derives the parameter-narrowing effect an `if cond then
// <terminates> end` guard proves on its live continuation edge.
func (t *Transfer) paramEffectFromGuard(g *cfg.Graph, p cfg.Point, info *cfg.BranchInfo) (ParamNarrow, bool) {
	succs := g.Successors(p)
	if len(succs) != 2 {
		return ParamNarrow{}, false
	}
	var trueSucc, falseSucc cfg.Point
	for _, s := range succs {
		if taken, ok := g.EdgeCond(p, s); ok && taken {
			trueSucc = s
		} else {
			falseSucc = s
		}
	}
	if trueSucc == 0 && falseSucc == 0 {
		return ParamNarrow{}, false
	}
	trueLive := reachesExit(g, trueSucc)
	falseLive := reachesExit(g, falseSucc)
	switch {
	case falseLive && !trueLive:
		return t.paramEffectFromBranchEdge(info, false)
	case trueLive && !falseLive:
		return t.paramEffectFromBranchEdge(info, true)
	default:
		return ParamNarrow{}, false
	}
}

func (t *Transfer) paramEffectFromBranchEdge(info *cfg.BranchInfo, taken bool) (ParamNarrow, bool) {
	return t.paramConditionEffect(info.Condition, taken)
}

// paramConditionEffect maps a condition expression whose truth value is proven to
// a caller-portable parameter effect. Direct assertions, terminating guards, and
// delegated condition-argument calls all lower through this transfer-local shape.
func (t *Transfer) paramConditionEffect(cond ast.Expr, taken bool) (ParamNarrow, bool) {
	if e, ok := t.paramEqEffect(cond, relEqualLive(cond, taken)); ok {
		return e, true
	}
	if e, ok := t.paramTypeProbeEffect(cond, taken); ok {
		return e, true
	}
	if e, ok := t.condArgEffect(cond, taken); ok {
		return e, true
	}
	sym, segs, baseCheck, ok := t.assertCondition(cond)
	if !ok {
		return ParamNarrow{}, false
	}
	check := effectiveCheck(baseCheck, taken)
	return t.toParamEffect(sym, segs, check, narrow.TypeKey{})
}

func (t *Transfer) paramTypeProbeEffect(cond ast.Expr, taken bool) (ParamNarrow, bool) {
	cmp, ok := guard.ExtractTypeProbeComparison(cond)
	if !ok || cmp.Probe.Key.IsZero() {
		return ParamNarrow{}, false
	}
	sym, segs, ok := t.pathSymbol(cmp.Probe.Expr)
	if !ok {
		return ParamNarrow{}, false
	}
	check := cfg.CheckTypeNot
	if cmp.Equal {
		check = cfg.CheckTypeEqual
	}
	return t.toParamEffect(sym, segs, effectiveCheck(check, taken), cmp.Probe.Key)
}

// condArgEffect recognizes a guard that tests a parameter directly as a CONDITION.
// The effect carries proven truthiness so the caller narrows the value tested by
// the argument expression.
func (t *Transfer) condArgEffect(cond ast.Expr, taken bool) (ParamNarrow, bool) {
	var ident *ast.IdentExpr
	check := cfg.CheckTruthy
	switch e := cond.(type) {
	case *ast.IdentExpr:
		ident = e
	case *ast.UnaryNotOpExpr:
		inner, ok := e.Expr.(*ast.IdentExpr)
		if !ok {
			return ParamNarrow{}, false
		}
		ident = inner
		check = cfg.CheckFalsy
	default:
		return ParamNarrow{}, false
	}
	param, isParam := t.params.Lookup(t.symbolOf(ident))
	if !isParam {
		return ParamNarrow{}, false
	}
	proven := effectiveCheck(check, taken)
	switch proven {
	case cfg.CheckTruthy, cfg.CheckFalsy:
		return ParamNarrow{Param: param.Index, Check: proven, EqParam: -1, CondArg: true}, true
	default:
		return ParamNarrow{}, false
	}
}

func (t *Transfer) paramEffectFromCondition(cond ast.Expr, _ bool) (ParamNarrow, bool) {
	return t.paramConditionEffect(cond, true)
}

func (t *Transfer) toParamEffect(sym cfg.SymbolID, segs []constraint.Segment, check cfg.CondCheckKind, key narrow.TypeKey) (ParamNarrow, bool) {
	param, isParam := t.params.Lookup(sym)
	if !isParam {
		return ParamNarrow{}, false
	}
	idx := param.Index
	switch check {
	case cfg.CheckTruthy, cfg.CheckNotNil, cfg.CheckNil, cfg.CheckFalsy:
		return ParamNarrow{Param: idx, Segments: segs, Check: check, EqParam: -1}, true
	case cfg.CheckTypeEqual, cfg.CheckTypeNot:
		if key.IsZero() {
			return ParamNarrow{}, false
		}
		if t.declaredParamAlreadyProvesTypeEffect(idx, check, key) {
			return ParamNarrow{}, false
		}
		return ParamNarrow{Param: idx, Segments: segs, Check: check, TypeKey: key, EqParam: -1}, true
	default:
		return ParamNarrow{}, false
	}
}

func (t *Transfer) declaredParamAlreadyProvesTypeEffect(idx int, check cfg.CondCheckKind, key narrow.TypeKey) bool {
	declared := t.declaredParamBySlot[idx]
	if declared == nil {
		return false
	}
	k, ok := key.BuiltinKind()
	if !ok {
		return false
	}
	target := narrow.TypeForKind(k)
	if target == nil {
		return false
	}
	switch check {
	case cfg.CheckTypeEqual:
		return subtype.IsSubtype(declared, target)
	case cfg.CheckTypeNot:
		return !narrow.TypesOverlap(declared, target)
	default:
		return false
	}
}

func (t *Transfer) paramCastEffect(call *ast.FuncCallExpr) (ParamNarrow, bool) {
	if t.callTyper == nil || call == nil || len(call.Args) != 1 {
		return ParamNarrow{}, false
	}
	target, ok := t.callTyper.TypeCastTarget(call, func(ast.Expr) typ.Type { return typ.Unknown })
	if !ok || target == nil || typ.IsAbsentOrUnknown(target) {
		return ParamNarrow{}, false
	}
	ident, ok := call.Args[0].(*ast.IdentExpr)
	if !ok {
		return ParamNarrow{}, false
	}
	sym := t.symbolOf(ident)
	if sym == 0 {
		return ParamNarrow{}, false
	}
	param, isParam := t.params.Lookup(sym)
	if !isParam {
		return ParamNarrow{}, false
	}
	return ParamNarrow{Param: param.Index, EqParam: -1, CastType: target}, true
}

func (t *Transfer) paramEqEffect(cond ast.Expr, equalLive bool) (ParamNarrow, bool) {
	rel, ok := cond.(*ast.RelationalOpExpr)
	if !ok {
		return ParamNarrow{}, false
	}
	a, aOK := t.paramOperand(rel.Lhs)
	b, bOK := t.paramOperand(rel.Rhs)
	if !aOK || !bOK || a == b {
		return ParamNarrow{}, false
	}
	if !equalLive {
		return ParamNarrow{Param: a, EqParam: b, NotEqual: true}, true
	}
	return ParamNarrow{Param: a, EqParam: b}, true
}

func (t *Transfer) paramOperand(expr ast.Expr) (int, bool) {
	ident, ok := expr.(*ast.IdentExpr)
	if !ok {
		return 0, false
	}
	sym := t.symbolOf(ident)
	if sym == 0 {
		return 0, false
	}
	param, isParam := t.params.Lookup(sym)
	return param.Index, isParam
}

func relEqualLive(cond ast.Expr, taken bool) bool {
	rel, ok := cond.(*ast.RelationalOpExpr)
	if !ok {
		return false
	}
	switch rel.Operator {
	case "==":
		return taken
	case "~=":
		return !taken
	default:
		return false
	}
}

func reachesExit(g *cfg.Graph, p cfg.Point) bool {
	if g == nil || p == 0 {
		return false
	}
	exit := g.Exit()
	seen := make(map[cfg.Point]bool)
	stack := []cfg.Point{p}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if cur == exit {
			return true
		}
		if seen[cur] {
			continue
		}
		seen[cur] = true
		stack = append(stack, g.Successors(cur)...)
	}
	return false
}

// DelegatedCall is an exit-dominating call inside a function body that may forward
// a parameter narrowing from its callee.
type DelegatedCall = paramevidence.DelegatedCall

// ExitDominatingCalls returns calls in this body that run on every normal return
// paired with the caller-parameter each argument forwards. The driver composes the
// callee's lowered ReturnPostconditions through this mapping.
func (t *Transfer) ExitDominatingCalls() []DelegatedCall {
	g := t.in.Graph
	if g == nil || t.params.IsEmpty() {
		return nil
	}
	var out []DelegatedCall
	g.EachCall(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil || info.Call == nil || info.CalleeName == "assert" {
			return
		}
		if !dominatesExit(g, p) {
			return
		}
		argParams := make([]int, len(info.Call.Args))
		argTruthy := make([][]ParamNarrow, len(info.Call.Args))
		argFalsy := make([][]ParamNarrow, len(info.Call.Args))
		any := false
		for i, arg := range info.Call.Args {
			argParams[i] = -1
			if e, ok := t.paramConditionEffect(arg, true); ok {
				argTruthy[i] = []ParamNarrow{e}
				any = true
			}
			if e, ok := t.paramConditionEffect(arg, false); ok {
				argFalsy[i] = []ParamNarrow{e}
				any = true
			}
			ident, ok := arg.(*ast.IdentExpr)
			if !ok {
				continue
			}
			sym := t.symbolOf(ident)
			if sym == 0 {
				continue
			}
			if param, isParam := t.params.Lookup(sym); isParam {
				argParams[i] = param.Index
				any = true
			}
		}
		if any {
			out = append(out, DelegatedCall{
				Call:             info.Call,
				ArgParams:        argParams,
				ArgTruthyEffects: argTruthy,
				ArgFalsyEffects:  argFalsy,
			})
		}
	})
	return out
}

func dominatesExit(g *cfg.Graph, q cfg.Point) bool {
	if g == nil {
		return false
	}
	entry := g.Entry()
	exit := g.Exit()
	if q == entry {
		return true
	}
	seen := make(map[cfg.Point]bool)
	stack := []cfg.Point{entry}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if cur == q {
			continue
		}
		if cur == exit {
			return false
		}
		if seen[cur] {
			continue
		}
		seen[cur] = true
		stack = append(stack, g.Successors(cur)...)
	}
	return true
}
