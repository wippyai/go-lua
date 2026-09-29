package callsite

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
)

// EachCallSiteWithNested visits call sites and calls nested in their arguments,
// assignment sources, and return expressions. Nested calls report the point
// of their enclosing expression.
func EachCallSiteWithNested(graph *cfg.Graph, bindings *bind.BindingTable, fn func(cfg.Point, *cfg.CallInfo)) {
	if graph == nil || fn == nil {
		return
	}
	seen := make(map[cfg.Point]map[*ast.FuncCallExpr]bool)
	emit := func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil || info.Call == nil {
			return
		}
		if seen[p] == nil {
			seen[p] = make(map[*ast.FuncCallExpr]bool)
		}
		if seen[p][info.Call] {
			return
		}
		seen[p][info.Call] = true
		fn(p, info)
	}
	visitExpr := func(p cfg.Point, expr ast.Expr) {
		var nested nestedCalls
		collectNestedFuncCalls(expr, &nested)
		for _, call := range nested.calls {
			info := graph.CallSiteAt(p, call)
			if info == nil {
				info = callInfoFromExpr(call, bindings)
			}
			emit(p, info)
		}
	}
	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil {
			return
		}
		emit(p, info)

		for _, arg := range info.Args {
			visitExpr(p, arg)
		}
	})
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info != nil {
			for _, expr := range info.Sources {
				visitExpr(p, expr)
			}
		}
	})
	graph.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if info != nil {
			for _, expr := range info.Exprs {
				visitExpr(p, expr)
			}
		}
	})
	graph.EachBranch(func(p cfg.Point, info *cfg.BranchInfo) {
		if info != nil {
			visitExpr(p, info.Condition)
		}
	})
}

type nestedCalls struct {
	calls []*ast.FuncCallExpr
	seen  map[*ast.FuncCallExpr]struct{}
}

func (n *nestedCalls) add(call *ast.FuncCallExpr) {
	if n.seen == nil {
		n.seen = make(map[*ast.FuncCallExpr]struct{})
	}
	if _, ok := n.seen[call]; ok {
		return
	}
	n.seen[call] = struct{}{}
	n.calls = append(n.calls, call)
}

func callInfoFromExpr(ex *ast.FuncCallExpr, bindings *bind.BindingTable) *cfg.CallInfo {
	if ex == nil {
		return nil
	}
	info := &cfg.CallInfo{
		Call:     ex,
		Callee:   ex.Func,
		Args:     ex.Args,
		Method:   ex.Method,
		Receiver: ex.Receiver,
		IsStmt:   false,
	}
	if id, ok := ex.Func.(*ast.IdentExpr); ok {
		info.CalleeName = id.Value
	}
	if bindings != nil {
		info.CalleeSymbol = SymbolFromExpr(ex.Func, bindings)
		if ex.Receiver != nil {
			info.ReceiverSymbol = SymbolFromExpr(ex.Receiver, bindings)
			if id, ok := ex.Receiver.(*ast.IdentExpr); ok {
				info.ReceiverName = id.Value
			}
		}
		info.ArgSymbols = make([]cfg.SymbolID, len(ex.Args))
		for i, arg := range ex.Args {
			info.ArgSymbols[i] = SymbolFromExpr(arg, bindings)
		}
	}
	return info
}

func collectNestedFuncCalls(expr ast.Expr, out *nestedCalls) {
	if expr == nil || out == nil {
		return
	}
	if call, ok := expr.(*ast.FuncCallExpr); ok {
		out.add(call)
	}
	if _, ok := expr.(*ast.CastExpr); ok {
		return
	}
	if _, ok := expr.(*ast.NonNilAssertExpr); ok {
		return
	}
	ast.WalkExprChildren(expr, func(child ast.Expr, _ int) {
		collectNestedFuncCalls(child, out)
	})
}
