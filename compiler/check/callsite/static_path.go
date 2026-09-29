package callsite

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/pathseg"
	"github.com/wippyai/go-lua/types/constraint"
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
	depth := 0
	root := expr
	for {
		attr, ok := root.(*ast.AttrGetExpr)
		if !ok {
			break
		}
		depth++
		root = attr.Object
	}
	ident, ok := root.(*ast.IdentExpr)
	if !ok {
		return 0, nil, false
	}
	sym, ok := bindings.SymbolOf(ident)
	if !ok || sym == 0 {
		return 0, nil, false
	}
	if depth == 0 {
		return sym, nil, true
	}
	segments := make([]constraint.Segment, depth)
	for i := depth - 1; i >= 0; i-- {
		attr := expr.(*ast.AttrGetExpr)
		seg, ok := pathseg.StaticAttrKeySegment(attr.Key)
		if !ok {
			return 0, nil, false
		}
		segments[i] = seg
		expr = attr.Object
	}
	return sym, segments, true
}
