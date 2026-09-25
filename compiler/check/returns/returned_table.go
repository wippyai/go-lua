package returns

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
)

// ReturnedMethodTable identifies a local table returned directly by every
// value-returning path and populated with function fields in this graph.
func ReturnedMethodTable(graph *cfg.Graph) (cfg.SymbolID, cfg.Point) {
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
		if len(info.Symbols) == 0 || info.Symbols[0] == 0 {
			valid = false
			return
		}
		if symbol != 0 && symbol != info.Symbols[0] {
			valid = false
			return
		}
		symbol, point = info.Symbols[0], at
	})
	if !valid || symbol == 0 {
		return 0, 0
	}
	hasMethod := false
	graph.EachFuncDef(func(_ cfg.Point, info *cfg.FuncDefInfo) {
		if info.TargetPath.Symbol == symbol && len(info.TargetPath.Segments) > 0 {
			hasMethod = true
		}
	})
	graph.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		info.EachTargetSource(func(_ int, target cfg.AssignTarget, src ast.Expr) {
			if target.Kind == cfg.TargetField && target.BaseSymbol == symbol && len(target.FieldPath) > 0 {
				if _, ok := src.(*ast.FunctionExpr); ok {
					hasMethod = true
				}
			}
		})
	})
	if !hasMethod {
		return 0, 0
	}
	return symbol, point
}
