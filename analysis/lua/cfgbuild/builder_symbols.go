package cfgbuild

import (
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (b *builder) simpleIdentSymbol(expr ast.Expr) (symbol.ID, bool) {
	ident, ok := expr.(*ast.IdentExpr)
	if !ok {
		return 0, false
	}
	return b.identSymbol(ident)
}

func (b *builder) assignmentRootSymbol(expr ast.Expr) (symbol.ID, bool) {
	if id, ok := b.simpleIdentSymbol(expr); ok {
		return id, true
	}
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok {
		return 0, false
	}
	return b.assignmentRootSymbol(attr.Object)
}

func (b *builder) identSymbol(ident *ast.IdentExpr) (symbol.ID, bool) {
	if b.bindings == nil || ident == nil {
		return 0, false
	}
	id, ok := b.bindings.SymbolOf(ident)
	return id, ok && id != 0
}
