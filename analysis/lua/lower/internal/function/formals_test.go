package function

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestFunctionFormalCursorRejectsOutOfRangeIndex(t *testing.T) {
	w := Writer{}
	current := step{fn: &ast.FunctionExpr{}, index: 1}
	if err := w.runFormal(current, &continuation.Stack{}); err == nil {
		t.Fatal("runFormal accepted an out-of-range cursor")
	}
}

func TestFunctionReturnHeaderRejectsAbsentAuthoredType(t *testing.T) {
	w := Writer{}
	current := step{fn: &ast.FunctionExpr{ReturnTypes: []ast.TypeExpr{nil}}}
	if err := w.runHeaderReturns(current, &continuation.Stack{}); err == nil {
		t.Fatal("runHeaderReturns accepted an absent return type")
	}
}
