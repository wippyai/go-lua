package typeresolve

import (
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// BindingInExpr returns the lexical type declaration matching name inside expr.
func BindingInExpr(bindings Bindings, expr ast.TypeExpr, name string) (bind.TypeDecl, bool) {
	if bindings == nil || expr == nil || name == "" {
		return bind.TypeDecl{}, false
	}
	var found bind.TypeDecl
	var ok bool
	WalkTypeNameExpr(expr, func(ref *ast.TypeRefExpr) bool {
		if ref == nil || len(ref.Path) != 1 || ref.Path[0] != name {
			return true
		}
		decl, hasDecl := bindings.TypeRef(ref)
		if !hasDecl {
			return true
		}
		found, ok = decl, true
		return false
	}, func(prim *ast.PrimitiveTypeExpr) bool {
		if prim == nil || prim.Name != name || typ.BuiltinPrimitiveName(prim.Name) {
			return true
		}
		decl, hasDecl := bindings.PrimitiveTypeRef(prim)
		if !hasDecl {
			return true
		}
		found, ok = decl, true
		return false
	})
	return found, ok
}
