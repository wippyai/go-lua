package static

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
)

func TestStaticTypeWalkRejectsAbsentAndMalformedChildren(t *testing.T) {
	var w Writer
	if err := w.runType(walkStep{}); err == nil {
		t.Fatal("runType accepted an absent type expression")
	}
	if err := w.runTypeList(walkStep{types: []ast.TypeExpr{nil}, index: 0}); err == nil {
		t.Fatal("runTypeList accepted an absent child")
	}
	if err := w.runConditionalChildren(walkStep{node: &ast.ConditionalTypeExpr{}, index: 0, staticMark: 0}); err == nil {
		t.Fatal("runConditionalChildren accepted an absent conditional child")
	}
}
