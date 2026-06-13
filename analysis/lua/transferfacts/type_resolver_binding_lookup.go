package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (r *typeResolver) currentBinding(name string) (bind.TypeDecl, bool) {
	if r == nil || r.bindings == nil || name == "" || len(r.current) == 0 {
		return bind.TypeDecl{}, false
	}
	return typeBindingInExpr(r.bindings, r.current[len(r.current)-1], name)
}

func typeBindingInExpr(bindings *bind.Result, expr ast.TypeExpr, name string) (bind.TypeDecl, bool) {
	if bindings == nil || expr == nil || name == "" {
		return bind.TypeDecl{}, false
	}
	var found bind.TypeDecl
	var ok bool
	walkTypeNameExpr(expr, func(ref *ast.TypeRefExpr) bool {
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
		if prim == nil || prim.Name != name || isBuiltinPrimitiveTypeName(prim.Name) {
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
