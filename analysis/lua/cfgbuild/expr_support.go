package cfgbuild

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/callorder"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (b *builder) hasUnsupportedExprs(exprs ...ast.Expr) bool {
	for _, expr := range exprs {
		if !b.exprCovered(expr) {
			return true
		}
	}
	return false
}

func (b *builder) hasUnsupportedValueListExprs(exprs ...ast.Expr) bool {
	_, ok := b.valueListCalls(exprs)
	return !ok
}

func (b *builder) hasUnsupportedExprInCall(expr ast.Expr) bool {
	_, ok := b.exprCalls(expr)
	return !ok
}

func (b *builder) appendValueListCalls(state flowState, stmt ast.Stmt, exprs []ast.Expr) flowState {
	calls, ok := b.valueListCalls(exprs)
	if !ok {
		b.unsupported = true
		return flowState{current: state.current}
	}
	for range calls {
		state = b.appendCall(state, stmt)
	}
	return state
}

func (b *builder) appendExprCalls(state flowState, stmt ast.Stmt, expr ast.Expr) flowState {
	calls, ok := b.exprCalls(expr)
	if !ok {
		b.unsupported = true
		return flowState{current: state.current}
	}
	for range calls {
		state = b.appendCall(state, stmt)
	}
	return state
}

func (b *builder) hasUnsupportedConditionExpr(expr ast.Expr) bool {
	if call, _, ok := branchcond.PredicateCall(expr); ok {
		return b.hasUnsupportedExprInCall(call)
	}
	if b.conditionExprCovered(expr) {
		return false
	}
	return !branchcond.SupportsTypeComparison(expr, b.bindings)
}

func (b *builder) appendConditionCall(state flowState, stmt ast.Stmt, expr ast.Expr) (flowState, cfg.Point, bool) {
	if _, _, ok := branchcond.PredicateCall(expr); !ok {
		return state, 0, false
	}
	calls, ok := b.exprCalls(expr)
	if !ok {
		b.unsupported = true
		return flowState{current: state.current}, 0, false
	}
	first := cfg.Point(0)
	for range calls {
		state = b.appendCall(state, stmt)
		if first == 0 {
			first = state.current
		}
	}
	return state, first, state.live && first != 0
}

func (b *builder) valueListCalls(exprs []ast.Expr) ([]callorder.Occurrence, bool) {
	return callorder.ValueList(exprs, b.callOrderOptions())
}

func (b *builder) exprCalls(expr ast.Expr) ([]callorder.Occurrence, bool) {
	return callorder.Expr(expr, b.callOrderOptions())
}

func (b *builder) callOrderOptions() callorder.Options {
	return callorder.LuaOptions(b.bindings)
}

func (b *builder) exprCovered(expr ast.Expr) bool {
	return b.exprCoveredMode(expr, true)
}

func (b *builder) conditionExprCovered(expr ast.Expr) bool {
	return b.exprCoveredMode(expr, false)
}

func (b *builder) exprCoveredMode(expr ast.Expr, allowProjectedCalls bool) bool {
	switch expr := expr.(type) {
	case nil:
		return true
	case *ast.TrueExpr, *ast.FalseExpr, *ast.NilExpr, *ast.NumberExpr, *ast.StringExpr, *ast.Comma3Expr:
		return true
	case *ast.IdentExpr:
		return true
	case *ast.AttrGetExpr:
		if allowProjectedCalls {
			return b.attrObjectCovered(expr.Object) && b.exprCoveredMode(expr.Key, allowProjectedCalls)
		}
		return b.exprCoveredMode(expr.Object, allowProjectedCalls) && b.exprCoveredMode(expr.Key, allowProjectedCalls)
	case *ast.TableExpr:
		for _, field := range expr.Fields {
			if field == nil {
				continue
			}
			if !b.exprCoveredMode(field.Key, allowProjectedCalls) || !b.exprCoveredMode(field.Value, allowProjectedCalls) {
				return false
			}
		}
		return true
	case *ast.FuncCallExpr:
		return allowProjectedCalls && b.pureTypeCallCovered(expr)
	case *ast.FunctionExpr:
		return true
	case *ast.LogicalOpExpr:
		return b.exprCoveredMode(expr.Lhs, allowProjectedCalls) && b.exprCoveredMode(expr.Rhs, allowProjectedCalls)
	case *ast.RelationalOpExpr:
		return b.exprCoveredMode(expr.Lhs, allowProjectedCalls) && b.exprCoveredMode(expr.Rhs, allowProjectedCalls)
	case *ast.StringConcatOpExpr:
		return b.exprCoveredMode(expr.Lhs, allowProjectedCalls) && b.exprCoveredMode(expr.Rhs, allowProjectedCalls)
	case *ast.ArithmeticOpExpr:
		return b.exprCoveredMode(expr.Lhs, allowProjectedCalls) && b.exprCoveredMode(expr.Rhs, allowProjectedCalls)
	case *ast.UnaryMinusOpExpr:
		return b.exprCoveredMode(expr.Expr, allowProjectedCalls)
	case *ast.UnaryNotOpExpr:
		return b.exprCoveredMode(expr.Expr, allowProjectedCalls)
	case *ast.UnaryLenOpExpr:
		return b.exprCoveredMode(expr.Expr, allowProjectedCalls)
	case *ast.UnaryBNotOpExpr:
		return b.exprCoveredMode(expr.Expr, allowProjectedCalls)
	case *ast.CastExpr:
		return b.exprCoveredMode(expr.Expr, allowProjectedCalls)
	case *ast.NonNilAssertExpr:
		return b.exprCoveredMode(expr.Expr, allowProjectedCalls)
	default:
		return false
	}
}

func (b *builder) attrObjectCovered(expr ast.Expr) bool {
	if call, ok := sourceprovenance.Call(expr); ok {
		return !b.hasUnsupportedExprInCall(call)
	}
	return b.exprCovered(expr)
}

func (b *builder) pureTypeCallCovered(call *ast.FuncCallExpr) bool {
	call, ok := branchcond.TypeCall(call)
	if !ok {
		return false
	}
	fn, ok := call.Func.(*ast.IdentExpr)
	if !ok || !b.bindings.ResolvesToGlobal(fn, "type") {
		return false
	}
	_, ok = pathexpr.Resolve(call.Args[0], b.bindings)
	return ok
}
