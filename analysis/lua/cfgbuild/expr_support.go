package cfgbuild

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/callorder"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
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
	if len(calls) == 0 {
		return state
	}
	for _, expr := range exprs {
		state = b.appendValueExprCalls(state, stmt, expr)
		if b.unsupported {
			return flowState{current: state.current}
		}
	}
	return state
}

func (b *builder) appendExprCalls(state flowState, stmt ast.Stmt, expr ast.Expr) flowState {
	calls, ok := b.exprCalls(expr)
	if !ok {
		b.unsupported = true
		return flowState{current: state.current}
	}
	if len(calls) == 0 {
		return state
	}
	return b.appendValueExprCalls(state, stmt, expr)
}

func (b *builder) hasUnsupportedConditionExpr(expr ast.Expr) bool {
	if b.conditionExprCovered(expr) {
		return false
	}
	if branchcond.SupportsTypeComparison(expr, b.bindings) {
		return false
	}
	if unsupportedTypePredicateComparison(expr) {
		return true
	}
	_, ok := b.conditionExprCalls(expr)
	return !ok
}

func (b *builder) appendConditionCall(state flowState, stmt ast.Stmt, expr ast.Expr) (flowState, cfg.Point, bool) {
	calls, ok := b.conditionExprCalls(expr)
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

func (b *builder) conditionExprCalls(expr ast.Expr) ([]callorder.Occurrence, bool) {
	return callorder.Expr(expr, b.conditionCallOrderOptions())
}

func (b *builder) callOrderOptions() callorder.Options {
	options := callorder.LuaOptions(b.bindings)
	options.AllowShortCircuitCalls = true
	return options
}

func (b *builder) conditionCallOrderOptions() callorder.Options {
	options := callorder.LuaOptions(b.bindings)
	options.AllowShortCircuitCalls = true
	return options
}

func (b *builder) appendValueExprCalls(state flowState, stmt ast.Stmt, expr ast.Expr) flowState {
	if b.unsupported || !state.live {
		return state
	}
	inner := sourceprovenance.AssertionInner(expr)
	if inner != expr {
		return b.appendValueExprCalls(state, stmt, inner)
	}
	switch expr := inner.(type) {
	case nil:
		return state
	case *ast.TrueExpr, *ast.FalseExpr, *ast.NilExpr, *ast.NumberExpr, *ast.StringExpr, *ast.Comma3Expr:
		return state
	case *ast.IdentExpr:
		return state
	case *ast.AttrGetExpr:
		state = b.appendValueExprCalls(state, stmt, expr.Object)
		return b.appendValueExprCalls(state, stmt, expr.Key)
	case *ast.TableExpr:
		for _, field := range expr.Fields {
			if field == nil {
				continue
			}
			state = b.appendValueExprCalls(state, stmt, field.Key)
			state = b.appendValueExprCalls(state, stmt, field.Value)
			if b.unsupported {
				return flowState{current: state.current}
			}
		}
		return state
	case *ast.FuncCallExpr:
		return b.appendValueCall(state, stmt, expr)
	case *ast.FunctionExpr:
		return state
	case *ast.LogicalOpExpr:
		return b.appendShortCircuitValueCalls(state, stmt, expr)
	case *ast.RelationalOpExpr:
		if b.expressionCoveredRelationalCall(expr) {
			return state
		}
		state = b.appendValueExprCalls(state, stmt, expr.Lhs)
		return b.appendValueExprCalls(state, stmt, expr.Rhs)
	case *ast.StringConcatOpExpr:
		state = b.appendValueExprCalls(state, stmt, expr.Lhs)
		return b.appendValueExprCalls(state, stmt, expr.Rhs)
	case *ast.ArithmeticOpExpr:
		state = b.appendValueExprCalls(state, stmt, expr.Lhs)
		return b.appendValueExprCalls(state, stmt, expr.Rhs)
	case *ast.UnaryMinusOpExpr:
		return b.appendValueExprCalls(state, stmt, expr.Expr)
	case *ast.UnaryNotOpExpr:
		return b.appendValueExprCalls(state, stmt, expr.Expr)
	case *ast.UnaryLenOpExpr:
		return b.appendValueExprCalls(state, stmt, expr.Expr)
	case *ast.UnaryBNotOpExpr:
		return b.appendValueExprCalls(state, stmt, expr.Expr)
	default:
		b.unsupported = true
		return flowState{current: state.current}
	}
}

func (b *builder) appendValueCall(state flowState, stmt ast.Stmt, call *ast.FuncCallExpr) flowState {
	if call == nil {
		return state
	}
	options := b.callOrderOptions()
	if options.ExpressionCoveredCall != nil && options.ExpressionCoveredCall(call, call) {
		return state
	}
	if options.OpaqueCall != nil && options.OpaqueCall(call) {
		return b.appendCall(state, stmt)
	}
	if call.Receiver != nil {
		state = b.appendValueExprCalls(state, stmt, call.Receiver)
	} else {
		state = b.appendValueExprCalls(state, stmt, call.Func)
	}
	for _, arg := range call.Args {
		state = b.appendValueExprCalls(state, stmt, arg)
		if b.unsupported {
			return flowState{current: state.current}
		}
	}
	return b.appendCall(state, stmt)
}

func (b *builder) appendShortCircuitValueCalls(state flowState, stmt ast.Stmt, expr *ast.LogicalOpExpr) flowState {
	if expr == nil {
		return state
	}
	state = b.appendValueExprCalls(state, stmt, expr.Lhs)
	if b.unsupported || !state.live {
		return state
	}
	rhsCalls, ok := callorder.Expr(expr.Rhs, b.callOrderOptions())
	if !ok {
		b.unsupported = true
		return flowState{current: state.current}
	}
	if len(rhsCalls) == 0 {
		return state
	}
	rhsCond, ok := shortCircuitRHSCond(expr.Operator)
	if !ok {
		b.unsupported = true
		return flowState{current: state.current}
	}
	branch := b.graph.AddBranch()
	b.connect(state, branch)
	b.meta.SetShortCircuitGuard(branch, cfgfacts.ShortCircuitGuardFact{Stmt: stmt, Condition: expr.Lhs})
	join := b.graph.AddNode(cfg.NodeJoin)
	b.graph.AddEdge(branch, join, !rhsCond)
	rhsState := b.appendValueExprCalls(branchPath(branch, rhsCond), stmt, expr.Rhs)
	b.connect(rhsState, join)
	return flowState{current: join, live: len(b.graph.Predecessors(join)) > 0}
}

func shortCircuitRHSCond(operator string) (bool, bool) {
	switch operator {
	case "and":
		return true, true
	case "or":
		return false, true
	default:
		return false, false
	}
}

func (b *builder) expressionCoveredRelationalCall(expr *ast.RelationalOpExpr) bool {
	if expr == nil {
		return false
	}
	options := b.callOrderOptions()
	if call, ok := sourceprovenance.Call(expr.Lhs); ok && options.ExpressionCoveredCall != nil && options.ExpressionCoveredCall(call, expr) {
		return true
	}
	if call, ok := sourceprovenance.Call(expr.Rhs); ok && options.ExpressionCoveredCall != nil && options.ExpressionCoveredCall(call, expr) {
		return true
	}
	return false
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

func unsupportedTypePredicateComparison(expr ast.Expr) bool {
	rel, ok := expr.(*ast.RelationalOpExpr)
	if !ok {
		return false
	}
	if rel.Operator != "==" && rel.Operator != "~=" {
		return false
	}
	return typePredicateLikeCall(rel.Lhs) || typePredicateLikeCall(rel.Rhs)
}

func typePredicateLikeCall(expr ast.Expr) bool {
	call, ok := sourceprovenance.Call(expr)
	if !ok || call == nil {
		return false
	}
	if call.Method == "type" {
		return true
	}
	fn, ok := call.Func.(*ast.IdentExpr)
	return ok && fn.Value == "type"
}
