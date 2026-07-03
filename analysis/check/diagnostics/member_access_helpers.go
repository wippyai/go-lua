package diagnostics

import (
	"github.com/wippyai/go-lua/compiler/ast"
)

func memberPathName(root, member string) string {
	if member == "" {
		return root
	}
	if member[0] == '[' {
		return root + member
	}
	return root + "." + member
}

func staticMemberReadName(expr *ast.AttrGetExpr) (string, bool) {
	if expr == nil {
		return "", false
	}
	switch expr.KeySyntax {
	case ast.AttrKeyDot:
		name := ast.KeyName(expr.Key)
		return name, name != ""
	case ast.AttrKeyIndex:
		key, ok := expr.Key.(*ast.StringExpr)
		if !ok || key.Value == "" {
			return "", false
		}
		return key.Value, true
	default:
		return "", false
	}
}
