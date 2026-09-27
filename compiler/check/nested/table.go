package nested

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
)

// MethodOwner returns the table symbol that owns a function definition.
// Named definitions carry their owner directly; assigned functions use the
// target at their definition point, or the containing table literal.
func MethodOwner(graph *cfg.Graph, fn *ast.FunctionExpr, def *cfg.FuncDefInfo, point cfg.Point) cfg.SymbolID {
	if def != nil && (def.TargetKind == cfg.FuncDefField || def.TargetKind == cfg.FuncDefMethod) && len(def.TargetPath.Segments) == 1 {
		return def.TargetPath.Symbol
	}
	if graph == nil || fn == nil {
		return 0
	}
	var owner cfg.SymbolID
	visit := func(info *cfg.AssignInfo) {
		if info == nil || owner != 0 {
			return
		}
		info.EachTargetSource(func(_ int, target cfg.AssignTarget, src ast.Expr) {
			if owner != 0 {
				return
			}
			if src == fn && (target.Kind == cfg.TargetField || target.Kind == cfg.TargetIndex) {
				owner = target.BaseSymbol
				return
			}
			tbl, ok := src.(*ast.TableExpr)
			if !ok || target.Symbol == 0 {
				return
			}
			for _, field := range tbl.Fields {
				if field != nil && field.Value == fn {
					owner = target.Symbol
					return
				}
			}
		})
	}
	if point != 0 {
		visit(graph.Assign(point))
	}
	if owner == 0 {
		graph.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) { visit(info) })
	}
	return owner
}
