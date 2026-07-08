package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (l *lowerer) resolveType(expr ast.TypeExpr) (typ.Type, bool) {
	if l == nil {
		return nil, false
	}
	resolver := l.typeResolver
	if resolver == nil {
		resolver = typeresolve.New(l.bindings)
	}
	return resolver.Type(expr)
}

func (l *lowerer) resolveDecl(decl bind.TypeDecl) (typ.Type, bool) {
	if l == nil {
		return nil, false
	}
	resolver := l.typeResolver
	if resolver == nil {
		resolver = typeresolve.New(l.bindings)
	}
	return resolver.Decl(decl)
}
