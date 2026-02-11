package callsite

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/pathkey"
)

// StaticPathWithBaseSymbol resolves an expression to a static segment path rooted at a base symbol.
//
// Supported forms:
//   - ident: x
//   - static attr chain: x.f, x["k"], x[1], x.f["k"]
//
// Dynamic keys (for example x[k] where k is a variable) are not supported.
func StaticPathWithBaseSymbol(bindings *bind.BindingTable, expr ast.Expr) (cfg.SymbolID, []constraint.Segment, bool) {
	if bindings == nil || expr == nil {
		return 0, nil, false
	}
	switch e := expr.(type) {
	case *ast.IdentExpr:
		sym, ok := bindings.SymbolOf(e)
		if !ok || sym == 0 {
			return 0, nil, false
		}
		return sym, nil, true
	case *ast.AttrGetExpr:
		baseSym, segs, ok := StaticPathWithBaseSymbol(bindings, e.Object)
		if !ok || baseSym == 0 {
			return 0, nil, false
		}
		seg, ok := staticSegmentFromExpr(e.Key)
		if !ok {
			return 0, nil, false
		}
		out := append(append([]constraint.Segment{}, segs...), seg)
		return baseSym, out, true
	default:
		return 0, nil, false
	}
}

func staticSegmentFromExpr(expr ast.Expr) (constraint.Segment, bool) {
	switch k := expr.(type) {
	case *ast.StringExpr:
		if k.Value == "" {
			return constraint.Segment{}, false
		}
		if pathkey.IsIdentName(k.Value) {
			return constraint.Segment{Kind: constraint.SegmentField, Name: k.Value}, true
		}
		return constraint.Segment{Kind: constraint.SegmentIndexString, Name: k.Value}, true
	default:
		return constraint.Segment{}, false
	}
}
