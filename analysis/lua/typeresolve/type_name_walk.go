package typeresolve

import "github.com/wippyai/go-lua/compiler/ast"

// WalkTypeExpr walks type references in expr.
func WalkTypeExpr(expr ast.TypeExpr, visit func(*ast.TypeRefExpr) bool) bool {
	return WalkTypeNameExpr(expr, visit, func(*ast.PrimitiveTypeExpr) bool { return true })
}

// WalkTypeExprs walks type references in exprs.
func WalkTypeExprs(exprs []ast.TypeExpr, visit func(*ast.TypeRefExpr) bool) bool {
	return WalkTypeNameExprs(exprs, visit, func(*ast.PrimitiveTypeExpr) bool { return true })
}

// WalkTypeNameExpr walks named type references, including primitive aliases.
func WalkTypeNameExpr(
	expr ast.TypeExpr,
	visitRef func(*ast.TypeRefExpr) bool,
	visitPrimitive func(*ast.PrimitiveTypeExpr) bool,
) bool {
	switch expr := expr.(type) {
	case nil:
	case *ast.PrimitiveTypeExpr:
		return visitPrimitive(expr)
	case *ast.SelfTypeExpr, *ast.LiteralTypeExpr, *ast.TypeOfExpr:
	case *ast.OptionalTypeExpr:
		return WalkTypeNameExpr(expr.Inner, visitRef, visitPrimitive)
	case *ast.UnionTypeExpr:
		return WalkTypeNameExprs(expr.Types, visitRef, visitPrimitive)
	case *ast.IntersectionTypeExpr:
		return WalkTypeNameExprs(expr.Types, visitRef, visitPrimitive)
	case *ast.ArrayTypeExpr:
		return WalkTypeNameExpr(expr.Element, visitRef, visitPrimitive)
	case *ast.MapTypeExpr:
		return WalkTypeNameExpr(expr.Key, visitRef, visitPrimitive) &&
			WalkTypeNameExpr(expr.Value, visitRef, visitPrimitive)
	case *ast.RecordTypeExpr:
		for _, field := range expr.Fields {
			if !WalkTypeNameExpr(field.Type, visitRef, visitPrimitive) {
				return false
			}
		}
	case *ast.FunctionTypeExpr:
		for _, param := range expr.TypeParams {
			if !WalkTypeNameExpr(param.Constraint, visitRef, visitPrimitive) {
				return false
			}
		}
		for _, param := range expr.Params {
			if !WalkTypeNameExpr(param.Type, visitRef, visitPrimitive) {
				return false
			}
		}
		return WalkTypeNameExpr(expr.Variadic, visitRef, visitPrimitive) &&
			WalkTypeNameExprs(expr.Returns, visitRef, visitPrimitive)
	case *ast.AssertsTypeExpr:
		return WalkTypeNameExpr(expr.NarrowTo, visitRef, visitPrimitive)
	case *ast.TypeRefExpr:
		return visitRef(expr)
	case *ast.GenericTypeExpr:
		if !WalkTypeNameExpr(expr.Base, visitRef, visitPrimitive) {
			return false
		}
		return WalkTypeNameExprs(expr.Args, visitRef, visitPrimitive)
	case *ast.MetaTypeExpr:
		return WalkTypeNameExpr(expr.Inner, visitRef, visitPrimitive)
	case *ast.TupleTypeExpr:
		return WalkTypeNameExprs(expr.Elements, visitRef, visitPrimitive)
	case *ast.KeyOfExpr:
		return WalkTypeNameExpr(expr.Inner, visitRef, visitPrimitive)
	case *ast.IndexAccessExpr:
		return WalkTypeNameExpr(expr.Object, visitRef, visitPrimitive) &&
			WalkTypeNameExpr(expr.Index, visitRef, visitPrimitive)
	case *ast.ConditionalTypeExpr:
		return WalkTypeNameExpr(expr.Check, visitRef, visitPrimitive) &&
			WalkTypeNameExpr(expr.Extends, visitRef, visitPrimitive) &&
			WalkTypeNameExpr(expr.Then, visitRef, visitPrimitive) &&
			WalkTypeNameExpr(expr.Else, visitRef, visitPrimitive)
	}
	return true
}

// WalkTypeNameExprs walks named type references in exprs.
func WalkTypeNameExprs(
	exprs []ast.TypeExpr,
	visitRef func(*ast.TypeRefExpr) bool,
	visitPrimitive func(*ast.PrimitiveTypeExpr) bool,
) bool {
	for _, expr := range exprs {
		if !WalkTypeNameExpr(expr, visitRef, visitPrimitive) {
			return false
		}
	}
	return true
}
