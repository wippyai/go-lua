package exprdisplay

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestNameOKPreservesReadablePaths(t *testing.T) {
	expr := &ast.FuncCallExpr{
		Receiver: &ast.AttrGetExpr{
			Object:    &ast.IdentExpr{Value: "client"},
			Key:       &ast.IdentExpr{Value: "session"},
			KeySyntax: ast.AttrKeyDot,
		},
		Method: "refresh",
	}
	if got := NameOK(expr); got != "client.session:refresh(...)" {
		t.Fatalf("NameOK = %q, want readable receiver call path", got)
	}
}

func TestNameFallsBackAtAdversarialDepth(t *testing.T) {
	var expr ast.Expr = &ast.IdentExpr{Value: "value"}
	for i := 0; i < typ.DefaultRecursionDepth+8; i++ {
		expr = &ast.NonNilAssertExpr{Expr: expr}
	}
	if got := NameOK(expr); got != "" {
		t.Fatalf("NameOK = %q, want bounded empty result", got)
	}
	if got := Name(expr, "assigned value"); got != "assigned value" {
		t.Fatalf("Name = %q, want fallback", got)
	}
}
