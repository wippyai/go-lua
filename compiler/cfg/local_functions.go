package cfg

import "github.com/wippyai/go-lua/compiler/ast"

// EachLocalFunction calls visit, in program order, for every local symbol of
// the graph bound to one function literal:
//   - local function f() ... end and local f = function() ... end;
//   - local f (or local f = nil) followed by exactly one plain assignment
//     f = function() ... end, and no other assignment to f in the graph.
//
// Function definitions that carry a symbol (function f() ... end) are
// visited as well. The point is the one of the binding statement.
func (g *Graph) EachLocalFunction(visit func(p Point, sym SymbolID, fn *ast.FunctionExpr)) {
	if g == nil || visit == nil {
		return
	}

	type deferredBinding struct {
		p  Point
		fn *ast.FunctionExpr
	}
	visited := make(map[SymbolID]bool)
	declaredEmpty := make(map[SymbolID]bool)
	disqualified := make(map[SymbolID]bool)
	assigned := make(map[SymbolID]deferredBinding)
	var assignOrder []SymbolID

	g.EachAssign(func(p Point, info *AssignInfo) {
		if info == nil || len(info.Targets) == 0 {
			return
		}
		info.EachTarget(func(i int, target AssignTarget) {
			if target.Kind != TargetIdent || target.Symbol == 0 {
				return
			}
			sym := target.Symbol
			var source ast.Expr
			if i < len(info.Sources) {
				source = info.Sources[i]
			}
			fn, isFn := source.(*ast.FunctionExpr)
			if info.IsLocal {
				switch {
				case isFn:
					if !visited[sym] {
						visited[sym] = true
						visit(p, sym, fn)
					}
				case source == nil:
					declaredEmpty[sym] = true
				default:
					if _, isNil := source.(*ast.NilExpr); isNil {
						declaredEmpty[sym] = true
					}
				}
				return
			}
			if !isFn {
				disqualified[sym] = true
				return
			}
			if _, seen := assigned[sym]; seen {
				disqualified[sym] = true
				return
			}
			assigned[sym] = deferredBinding{p: p, fn: fn}
			assignOrder = append(assignOrder, sym)
		})
	})

	g.EachFuncDef(func(p Point, info *FuncDefInfo) {
		if info == nil || info.Symbol == 0 || info.FuncExpr == nil || visited[info.Symbol] {
			return
		}
		visited[info.Symbol] = true
		visit(p, info.Symbol, info.FuncExpr)
	})

	for _, sym := range assignOrder {
		if visited[sym] || disqualified[sym] || !declaredEmpty[sym] {
			continue
		}
		binding := assigned[sym]
		visited[sym] = true
		visit(binding.p, sym, binding.fn)
	}
}
