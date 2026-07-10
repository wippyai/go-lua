package branchcond

import (
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/compiler/ast"
)

// StaticLuaTruthiness reports statically-known Lua truthiness for expression
// shapes whose result is independent of runtime values.
func StaticLuaTruthiness(expr ast.Expr) (bool, bool) {
	switch expr := sourceprovenance.AssertionInner(expr).(type) {
	case *ast.NilExpr, *ast.FalseExpr:
		return false, true
	case *ast.TrueExpr, *ast.NumberExpr, *ast.StringExpr, *ast.TableExpr, *ast.FunctionExpr:
		return true, true
	case *ast.UnaryNotOpExpr:
		value, ok := StaticLuaTruthiness(expr.Expr)
		if !ok {
			return false, false
		}
		return !value, true
	case *ast.LogicalOpExpr:
		return staticLogicalTruthiness(expr)
	default:
		return false, false
	}
}

func staticLogicalTruthiness(expr *ast.LogicalOpExpr) (bool, bool) {
	left, ok := StaticLuaTruthiness(expr.Lhs)
	if !ok {
		return false, false
	}
	switch expr.Operator {
	case "and":
		if !left {
			return false, true
		}
		return StaticLuaTruthiness(expr.Rhs)
	case "or":
		if left {
			return true, true
		}
		return StaticLuaTruthiness(expr.Rhs)
	default:
		return false, false
	}
}
