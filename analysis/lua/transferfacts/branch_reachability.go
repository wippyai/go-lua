package transferfacts

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/compiler/ast"
)

func branchEdgeReachability(expr ast.Expr) (factflow.BranchEdgeReachability, bool) {
	truthy, ok := staticLuaTruthiness(expr)
	if !ok {
		return factflow.BranchEdgeReachability{}, false
	}
	return factflow.NewBranchEdgeReachability(!truthy, truthy), true
}

func staticLuaTruthiness(expr ast.Expr) (bool, bool) {
	switch expr := sourceprovenance.AssertionInner(expr).(type) {
	case *ast.NilExpr, *ast.FalseExpr:
		return false, true
	case *ast.TrueExpr, *ast.NumberExpr, *ast.StringExpr, *ast.TableExpr, *ast.FunctionExpr:
		return true, true
	case *ast.UnaryNotOpExpr:
		value, ok := staticLuaTruthiness(expr.Expr)
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
	left, ok := staticLuaTruthiness(expr.Lhs)
	if !ok {
		return false, false
	}
	switch expr.Operator {
	case "and":
		if !left {
			return false, true
		}
		return staticLuaTruthiness(expr.Rhs)
	case "or":
		if left {
			return true, true
		}
		return staticLuaTruthiness(expr.Rhs)
	default:
		return false, false
	}
}
