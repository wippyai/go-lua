package transferfacts

import "github.com/wippyai/go-lua/compiler/ast"

func walkTypeNameExpr(
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
		return walkTypeNameExpr(expr.Inner, visitRef, visitPrimitive)
	case *ast.UnionTypeExpr:
		return walkTypeNameExprs(expr.Types, visitRef, visitPrimitive)
	case *ast.IntersectionTypeExpr:
		return walkTypeNameExprs(expr.Types, visitRef, visitPrimitive)
	case *ast.ArrayTypeExpr:
		return walkTypeNameExpr(expr.Element, visitRef, visitPrimitive)
	case *ast.MapTypeExpr:
		return walkTypeNameExpr(expr.Key, visitRef, visitPrimitive) &&
			walkTypeNameExpr(expr.Value, visitRef, visitPrimitive)
	case *ast.RecordTypeExpr:
		for _, field := range expr.Fields {
			if !walkTypeNameExpr(field.Type, visitRef, visitPrimitive) {
				return false
			}
		}
	case *ast.FunctionTypeExpr:
		for _, param := range expr.TypeParams {
			if !walkTypeNameExpr(param.Constraint, visitRef, visitPrimitive) {
				return false
			}
		}
		for _, param := range expr.Params {
			if !walkTypeNameExpr(param.Type, visitRef, visitPrimitive) {
				return false
			}
		}
		return walkTypeNameExpr(expr.Variadic, visitRef, visitPrimitive) &&
			walkTypeNameExprs(expr.Returns, visitRef, visitPrimitive)
	case *ast.AssertsTypeExpr:
		return walkTypeNameExpr(expr.NarrowTo, visitRef, visitPrimitive)
	case *ast.TypeRefExpr:
		return visitRef(expr)
	case *ast.GenericTypeExpr:
		if !walkTypeNameExpr(expr.Base, visitRef, visitPrimitive) {
			return false
		}
		return walkTypeNameExprs(expr.Args, visitRef, visitPrimitive)
	case *ast.MetaTypeExpr:
		return walkTypeNameExpr(expr.Inner, visitRef, visitPrimitive)
	case *ast.TupleTypeExpr:
		return walkTypeNameExprs(expr.Elements, visitRef, visitPrimitive)
	case *ast.KeyOfExpr:
		return walkTypeNameExpr(expr.Inner, visitRef, visitPrimitive)
	case *ast.IndexAccessExpr:
		return walkTypeNameExpr(expr.Object, visitRef, visitPrimitive) &&
			walkTypeNameExpr(expr.Index, visitRef, visitPrimitive)
	case *ast.ConditionalTypeExpr:
		return walkTypeNameExpr(expr.Check, visitRef, visitPrimitive) &&
			walkTypeNameExpr(expr.Extends, visitRef, visitPrimitive) &&
			walkTypeNameExpr(expr.Then, visitRef, visitPrimitive) &&
			walkTypeNameExpr(expr.Else, visitRef, visitPrimitive)
	}
	return true
}

func walkTypeNameExprs(
	exprs []ast.TypeExpr,
	visitRef func(*ast.TypeRefExpr) bool,
	visitPrimitive func(*ast.PrimitiveTypeExpr) bool,
) bool {
	for _, expr := range exprs {
		if !walkTypeNameExpr(expr, visitRef, visitPrimitive) {
			return false
		}
	}
	return true
}
