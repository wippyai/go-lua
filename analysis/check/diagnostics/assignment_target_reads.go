package diagnostics

import "github.com/wippyai/go-lua/compiler/ast"

func emitAssignmentTargetReads(target ast.Expr, emitExpr func(ast.Expr)) {
	switch t := target.(type) {
	case *ast.AttrGetExpr:
		emitExpr(t.Object)
		if t.KeySyntax == ast.AttrKeyIndex {
			emitExpr(t.Key)
		}
	case *ast.CastExpr:
		emitAssignmentTargetReads(t.Expr, emitExpr)
	case *ast.NonNilAssertExpr:
		emitAssignmentTargetReads(t.Expr, emitExpr)
	}
}
