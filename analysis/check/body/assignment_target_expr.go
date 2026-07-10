package body

import "github.com/wippyai/go-lua/compiler/ast"

func assignmentTargetAttrExpr(expr ast.Expr) (*ast.AttrGetExpr, bool) {
	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		return e, true
	case *ast.CastExpr:
		return assignmentTargetAttrExpr(e.Expr)
	default:
		return nil, false
	}
}
