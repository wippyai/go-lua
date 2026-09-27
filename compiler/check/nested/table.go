package nested

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
)

// MethodOwner returns the table symbol that owns a function definition.
// Named definitions carry their owner directly; assigned functions use the
// target at their definition point, or the containing table literal.
func MethodOwner(graph *cfg.Graph, fn *ast.FunctionExpr, def *cfg.FuncDefInfo, point cfg.Point) cfg.SymbolID {
	if def != nil && (def.TargetKind == cfg.FuncDefField || def.TargetKind == cfg.FuncDefMethod) && len(def.TargetPath.Segments) > 0 {
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

// ReturnedClassTable identifies a table returned by every non-nil return
// path when this graph defines a method owned by that table.
func ReturnedClassTable(graph *cfg.Graph) (cfg.SymbolID, cfg.Point) {
	if graph == nil {
		return 0, 0
	}
	var symbol cfg.SymbolID
	var point cfg.Point
	valid := true
	graph.EachReturn(func(at cfg.Point, info *cfg.ReturnInfo) {
		if !valid || len(info.Exprs) == 0 {
			return
		}
		if _, nilReturn := info.Exprs[0].(*ast.NilExpr); nilReturn {
			return
		}
		if len(info.Symbols) == 0 || info.Symbols[0] == 0 || symbol != 0 && symbol != info.Symbols[0] {
			valid = false
			return
		}
		symbol, point = info.Symbols[0], at
	})
	if !valid || symbol == 0 {
		return 0, 0
	}
	hasMethod := false
	graph.EachFuncDef(func(at cfg.Point, info *cfg.FuncDefInfo) {
		if MethodOwner(graph, info.FuncExpr, info, at) == symbol {
			hasMethod = true
		}
	})
	graph.EachAssign(func(at cfg.Point, info *cfg.AssignInfo) {
		if hasMethod {
			return
		}
		info.EachTargetSource(func(_ int, target cfg.AssignTarget, src ast.Expr) {
			fn, ok := src.(*ast.FunctionExpr)
			if ok && target.Kind == cfg.TargetField && len(target.FieldPath) > 0 && MethodOwner(graph, fn, nil, at) == symbol {
				hasMethod = true
			}
		})
	})
	if !hasMethod {
		return 0, 0
	}
	return symbol, point
}
