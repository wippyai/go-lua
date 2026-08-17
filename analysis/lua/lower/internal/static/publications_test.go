package static

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
)

func TestPublicationRootRequiresQualifiedDotSyntax(t *testing.T) {
	w := Writer{}
	if _, err := w.publicationRoot(&ast.IdentExpr{Value: "pkg"}, []string{"pkg", "Type"}); err == nil {
		t.Fatal("publicationRoot accepted an unqualified expression for a qualified source")
	}
	if root, err := w.publicationRoot(&ast.IdentExpr{Value: "Type"}, []string{"Type"}); err != nil || root != 0 {
		t.Fatalf("single-component publication root = %d, %v; want zero root", root, err)
	}
}
