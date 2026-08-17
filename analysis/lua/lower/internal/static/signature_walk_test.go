package static

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
)

func TestStaticSignatureWalkRejectsInvalidCursors(t *testing.T) {
	w := Writer{}
	if err := w.runSignatureGenerics(walkStep{node: &ast.FunctionTypeExpr{}, index: -1}); err == nil {
		t.Fatal("runSignatureGenerics accepted a negative cursor")
	}
	if err := w.runSignatureReturns(walkStep{node: &ast.FunctionTypeExpr{}, index: 1}); err == nil {
		t.Fatal("runSignatureReturns accepted an out-of-range cursor")
	}
}
