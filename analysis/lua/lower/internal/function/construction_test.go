package function

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestFunctionBeginRequiresConstructionAuthorities(t *testing.T) {
	w := Writer{}
	fn := &ast.FunctionExpr{}
	if err := w.begin(fn, 1, source.Span{File: "function.lua"}, completion{}); err == nil {
		t.Fatal("begin accepted an incomplete function authority")
	}
}

func TestFunctionRunBeginRejectsInvalidGenericCursor(t *testing.T) {
	w := Writer{}
	current := step{fn: &ast.FunctionExpr{}, index: -1}
	if err := w.runBegin(current, &continuation.Stack{}); err == nil {
		t.Fatal("runBegin accepted a negative generic cursor")
	}
}
