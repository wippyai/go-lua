package callsite

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/flow/pathkey"
)

// FieldPathWithBaseSymbol resolves an expression to (base symbol, dotted field path)
// for static field access chains like obj.f or obj.f.g.
func FieldPathWithBaseSymbol(bindings *bind.BindingTable, expr ast.Expr) (cfg.SymbolID, string, bool) {
	if bindings == nil || expr == nil {
		return 0, "", false
	}
	switch e := expr.(type) {
	case *ast.IdentExpr:
		sym, ok := bindings.SymbolOf(e)
		if !ok || sym == 0 {
			return 0, "", false
		}
		return sym, "", true
	case *ast.AttrGetExpr:
		baseSym, basePath, ok := FieldPathWithBaseSymbol(bindings, e.Object)
		if !ok || baseSym == 0 {
			return 0, "", false
		}
		seg := fieldSegmentName(e.Key)
		if seg == "" {
			return 0, "", false
		}
		if basePath == "" {
			return baseSym, seg, true
		}
		return baseSym, basePath + "." + seg, true
	default:
		return 0, "", false
	}
}

func fieldSegmentName(expr ast.Expr) string {
	switch k := expr.(type) {
	case *ast.IdentExpr:
		if pathkey.IsIdentName(k.Value) {
			return k.Value
		}
		return ""
	case *ast.StringExpr:
		if pathkey.IsIdentName(k.Value) {
			return k.Value
		}
		return ""
	default:
		return ""
	}
}
