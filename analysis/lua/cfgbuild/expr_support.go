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

func (b *builder) appendValueListCalls(state flowState, stmt ast.Stmt, exprs []ast.Expr) flowState {
	calls, _ := b.valueListCalls(exprs)
	if len(calls) == 0 {
		return state
	}
	for _, expr := range exprs {
		state = b.appendValueExprCalls(state, stmt, expr)
	}
	return state
}

func (b *builder) appendExprCalls(state flowState, stmt ast.Stmt, expr ast.Expr) flowState {
	calls, _ := b.exprCalls(expr)
	if len(calls) == 0 {
		return state
	}
	return b.appendValueExprCalls(state, stmt, expr)
}

func (b *builder) appendConditionCall(state flowState, stmt ast.Stmt, expr ast.Expr) (flowState, cfg.Point, bool) {
	calls, _ := b.conditionExprCalls(expr)
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
	return callorder.Expr(expr, b.callOrderOptions())
}

func (b *builder) callOrderOptions() callorder.Options {
	options := callorder.LuaOptions(b.bindings)
	options.AllowShortCircuitCalls = true
	return options
}

func (b *builder) appendValueExprCalls(state flowState, stmt ast.Stmt, expr ast.Expr) flowState {
	if !state.live {
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
		// Every runtime value-expression form is handled above; type-level
		// nodes carry no runtime calls, so an unhandled node sequences nothing.
		return state
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
	}
	return b.appendCall(state, stmt)
}

func (b *builder) appendShortCircuitValueCalls(state flowState, stmt ast.Stmt, expr *ast.LogicalOpExpr) flowState {
	if expr == nil {
		return state
	}
	state = b.appendValueExprCalls(state, stmt, expr.Lhs)
	if !state.live {
		return state
	}
	rhsCalls, _ := callorder.Expr(expr.Rhs, b.callOrderOptions())
	if len(rhsCalls) == 0 {
		return state
	}
	rhsCond := shortCircuitRHSCond(expr.Operator)
	branch := b.graph.AddBranch()
	b.connect(state, branch)
	b.meta.SetShortCircuitGuard(branch, cfgfacts.ShortCircuitGuardFact{Stmt: stmt, Condition: expr.Lhs})
	join := b.graph.AddNode(cfg.NodeJoin)
	b.graph.AddEdge(branch, join, !rhsCond)
	rhsState := b.appendValueExprCalls(branchPath(branch, rhsCond), stmt, expr.Rhs)
	b.connect(rhsState, join)
	return flowState{current: join, live: len(b.graph.Predecessors(join)) > 0}
}

// shortCircuitRHSCond reports the branch condition under which a logical
// operator evaluates its right-hand side: "and" evaluates the RHS when the LHS
// is truthy, "or" when the LHS is falsy. Lua logical operators are always
// "and" or "or".
func shortCircuitRHSCond(operator string) bool {
	return operator == "and"
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
	if _, ok := sourceprovenance.Call(expr); ok {
		// Calls under an attribute object are always orderable, so the object
		// is covered for call-sequencing purposes.
		return true
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

