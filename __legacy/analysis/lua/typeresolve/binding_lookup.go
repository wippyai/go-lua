package typeresolve

import (
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
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

type qualifiedBindings interface {
	QualifiedTypeRef(*ast.TypeRefExpr) (bind.QualifiedTypeAlias, bool)
}

// QualifiedBindingInExpr returns the exact qualified alias binding matching
// path inside expr, when the binder recorded one.
func QualifiedBindingInExpr(bindings Bindings, expr ast.TypeExpr, path []string) (bind.QualifiedTypeAlias, bool) {
	qualified, ok := bindings.(qualifiedBindings)
	if !ok || expr == nil || len(path) < 2 {
		return bind.QualifiedTypeAlias{}, false
	}
	var found bind.QualifiedTypeAlias
	WalkTypeNameExpr(expr, func(ref *ast.TypeRefExpr) bool {
		if ref == nil || !sameTypePath(ref.Path, path) {
			return true
		}
		alias, hasAlias := qualified.QualifiedTypeRef(ref)
		if !hasAlias {
			return true
		}
		found, ok = alias, true
		return false
	}, func(*ast.PrimitiveTypeExpr) bool { return true })
	return found, ok
}

func sameTypePath(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
