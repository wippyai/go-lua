package cfgbuild

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/callorder"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (b *builder) appendValueListCalls(state flowState, stmt ast.Stmt, exprs []ast.Expr) flowState {
	for _, expr := range exprs {
		state = b.appendValueExprCalls(state, stmt, expr)
	}
	return state
}

func (b *builder) appendExprCalls(state flowState, stmt ast.Stmt, expr ast.Expr) flowState {
	return b.appendValueExprCalls(state, stmt, expr)
}

func (b *builder) appendConditionCall(state flowState, stmt ast.Stmt, expr ast.Expr) (flowState, cfg.Point, bool) {
	before := b.graph.RPO()
	state = b.appendValueExprCalls(state, stmt, expr)
	first := firstNewCallPoint(b.graph, before)
	return state, first, state.live && first != 0
}

func firstNewCallPoint(graph cfg.Graph, before []cfg.Point) cfg.Point {
	if graph == nil {
		return 0
	}
	seen := make(map[cfg.Point]struct{}, len(before))
	for _, point := range before {
		seen[point] = struct{}{}
	}
	for _, point := range graph.RPO() {
		if _, ok := seen[point]; ok {
			continue
		}
		node := graph.Node(point)
		if node != nil && node.Kind == cfg.NodeCall {
			return point
		}
	}
	return 0
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
	rhsCalls, ok := callorder.Expr(expr.Rhs, b.callOrderOptions())
	if !ok {
		return state
	}
	rhsCond := shortCircuitRHSCond(expr.Operator)
	branch := b.graph.AddBranch()
	b.connect(state, branch)
	b.shortCircuits.SetGuard(branch, ShortCircuitGuard{Condition: expr.Lhs})
	join := b.graph.AddNode(cfg.NodeJoin)
	region := ShortCircuitRegion{Guard: branch, Join: join, RHSOnTrue: rhsCond}
	if len(rhsCalls) == 0 {
		eval := b.graph.AddNode(cfg.NodeNoop)
		b.graph.AddEdge(branch, eval, rhsCond)
		b.shortCircuits.SetEvaluation(eval, ExpressionEvaluation{Expr: expr.Rhs})
		b.graph.AddEdge(eval, join, false)
		b.graph.AddEdge(branch, join, !rhsCond)
		region.OwnedRHSPoints = []cfg.Point{eval}
		if rhsCond {
			region.TrueTarget, region.FalseTarget = eval, join
		} else {
			region.TrueTarget, region.FalseTarget = join, eval
		}
		region.complete = true
		b.shortCircuits.setRegion(expr, region)
		return flowState{current: join, live: len(b.graph.Predecessors(join)) > 0}
	}
	b.graph.AddEdge(branch, join, !rhsCond)
	rhsStart := b.graph.Size()
	rhsState := b.appendValueExprCalls(branchPath(branch, rhsCond), stmt, expr.Rhs)
	rhsEnd := b.graph.Size()
	b.connect(rhsState, join)
	for raw := rhsStart; raw < rhsEnd; raw++ {
		region.OwnedRHSPoints = append(region.OwnedRHSPoints, cfg.Point(raw))
	}
	for _, target := range b.graph.Successors(branch) {
		cond, ok := b.graph.EdgeCond(branch, target)
		if !ok {
			continue
		}
		if cond {
			region.TrueTarget = target
		} else {
			region.FalseTarget = target
		}
	}
	rhsTarget := region.FalseTarget
	if rhsCond {
		rhsTarget = region.TrueTarget
	}
	region.complete = len(region.OwnedRHSPoints) != 0 && rhsTarget != join &&
		region.TrueTarget != region.FalseTarget
	b.shortCircuits.setRegion(expr, region)
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
