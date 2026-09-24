package callsite

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
)

// EachCallSiteWithNested calls fn for each call site of graph and for each
// call expression nested in a call site's arguments, in source order. Nested
// calls report the point of their enclosing call site.
func EachCallSiteWithNested(graph *cfg.Graph, bindings *bind.BindingTable, fn func(cfg.Point, *cfg.CallInfo)) {
	if graph == nil || fn == nil {
		return
	}
	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil {
			return
		}
		fn(p, info)

		var nested nestedCalls
		for _, arg := range info.Args {
			collectNestedFuncCalls(arg, &nested)
		}
		for _, call := range nested.calls {
			nestedInfo := graph.CallSiteAt(p, call)
			if nestedInfo == nil {
				nestedInfo = callInfoFromExpr(call, bindings)
			}
			if nestedInfo != nil {
				fn(p, nestedInfo)
			}
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
	switch e := expr.(type) {
	case *ast.FuncCallExpr:
		out.add(e)
		collectNestedFuncCalls(e.Func, out)
		collectNestedFuncCalls(e.Receiver, out)
		for _, arg := range e.Args {
			collectNestedFuncCalls(arg, out)
		}
	case *ast.AttrGetExpr:
		collectNestedFuncCalls(e.Object, out)
		collectNestedFuncCalls(e.Key, out)
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			collectNestedFuncCalls(field.Key, out)
			collectNestedFuncCalls(field.Value, out)
		}
	case *ast.LogicalOpExpr:
		collectNestedFuncCalls(e.Lhs, out)
		collectNestedFuncCalls(e.Rhs, out)
	case *ast.RelationalOpExpr:
		collectNestedFuncCalls(e.Lhs, out)
		collectNestedFuncCalls(e.Rhs, out)
	case *ast.StringConcatOpExpr:
		collectNestedFuncCalls(e.Lhs, out)
		collectNestedFuncCalls(e.Rhs, out)
	case *ast.ArithmeticOpExpr:
		collectNestedFuncCalls(e.Lhs, out)
		collectNestedFuncCalls(e.Rhs, out)
	case *ast.UnaryMinusOpExpr:
		collectNestedFuncCalls(e.Expr, out)
	case *ast.UnaryNotOpExpr:
		collectNestedFuncCalls(e.Expr, out)
	case *ast.UnaryLenOpExpr:
		collectNestedFuncCalls(e.Expr, out)
	case *ast.UnaryBNotOpExpr:
		collectNestedFuncCalls(e.Expr, out)
	}
}
