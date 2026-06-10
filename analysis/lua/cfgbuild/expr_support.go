package cfgbuild

import "github.com/wippyai/go-lua/compiler/ast"

func (b *builder) hasUnsupportedExprs(exprs ...ast.Expr) bool {
	for _, expr := range exprs {
		if !b.exprCovered(expr) {
			return true
		}
	}
	return false
}

func (b *builder) hasUnsupportedExprInCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.FuncCallExpr)
	if !ok {
		return !b.exprCovered(expr)
	}
	if !b.exprCovered(call.Func) || !b.exprCovered(call.Receiver) {
		return true
	}
	return b.hasUnsupportedExprs(call.Args...)
}

func (b *builder) hasUnsupportedConditionExpr(expr ast.Expr) bool {
	if b.exprCovered(expr) {
		return false
	}
	return !b.typeCompareConditionSupported(expr)
}

func (b *builder) exprCovered(expr ast.Expr) bool {
	switch expr := expr.(type) {
	case nil:
		return true
	case *ast.TrueExpr, *ast.FalseExpr, *ast.NilExpr, *ast.NumberExpr, *ast.StringExpr, *ast.Comma3Expr:
		return true
	case *ast.IdentExpr:
		return true
	case *ast.AttrGetExpr:
		return b.exprCovered(expr.Object) && b.exprCovered(expr.Key)
	case *ast.TableExpr:
		for _, field := range expr.Fields {
			if field == nil {
				continue
			}
			if !b.exprCovered(field.Key) || !b.exprCovered(field.Value) {
				return false
			}
		}
		return true
	case *ast.FuncCallExpr, *ast.FunctionExpr:
		return false
	case *ast.LogicalOpExpr:
		return b.exprCovered(expr.Lhs) && b.exprCovered(expr.Rhs)
	case *ast.RelationalOpExpr:
		return b.exprCovered(expr.Lhs) && b.exprCovered(expr.Rhs)
	case *ast.StringConcatOpExpr:
		return b.exprCovered(expr.Lhs) && b.exprCovered(expr.Rhs)
	case *ast.ArithmeticOpExpr:
		return b.exprCovered(expr.Lhs) && b.exprCovered(expr.Rhs)
	case *ast.UnaryMinusOpExpr:
		return b.exprCovered(expr.Expr)
	case *ast.UnaryNotOpExpr:
		return b.exprCovered(expr.Expr)
	case *ast.UnaryLenOpExpr:
		return b.exprCovered(expr.Expr)
	case *ast.UnaryBNotOpExpr:
		return b.exprCovered(expr.Expr)
	case *ast.CastExpr:
		return b.exprCovered(expr.Expr)
	case *ast.NonNilAssertExpr:
		return b.exprCovered(expr.Expr)
	default:
		return false
	}
}
