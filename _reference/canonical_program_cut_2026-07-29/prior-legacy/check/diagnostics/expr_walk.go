package diagnostics

import "github.com/wippyai/go-lua/compiler/ast"

func walkExprChildren(expr ast.Expr, visit func(ast.Expr)) {
	if expr == nil || visit == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		visit(e.Object)
		if e.KeySyntax == ast.AttrKeyIndex {
			visit(e.Key)
		}
	case *ast.FuncCallExpr:
		visit(e.Func)
		visit(e.Receiver)
		for _, arg := range e.Args {
			visit(arg)
		}
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			if field.KeySyntax == ast.AttrKeyIndex {
				visit(field.Key)
			}
			visit(field.Value)
		}
	case *ast.LogicalOpExpr:
		visit(e.Lhs)
		visit(e.Rhs)
	case *ast.RelationalOpExpr:
		visit(e.Lhs)
		visit(e.Rhs)
	case *ast.StringConcatOpExpr:
		visit(e.Lhs)
		visit(e.Rhs)
	case *ast.ArithmeticOpExpr:
		visit(e.Lhs)
		visit(e.Rhs)
	case *ast.UnaryMinusOpExpr:
		visit(e.Expr)
	case *ast.UnaryNotOpExpr:
		visit(e.Expr)
	case *ast.UnaryLenOpExpr:
		visit(e.Expr)
	case *ast.UnaryBNotOpExpr:
		visit(e.Expr)
	case *ast.CastExpr:
		visit(e.Expr)
	case *ast.NonNilAssertExpr:
		visit(e.Expr)
	}
}
