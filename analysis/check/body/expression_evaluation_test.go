package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestExpressionEvaluationUsesWIRShortCircuitAnchor(t *testing.T) {
	result, err := CheckChunk(parseChunk(t, `
local flag = true
local obj = { field = "ok" }
local selected = flag and obj.field
`), Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	for _, point := range result.Graph().RPO() {
		fact, ok := result.ExpressionEvaluation(point)
		if !ok {
			continue
		}
		attr, ok := fact.Expr.(*ast.AttrGetExpr)
		if !ok {
			t.Fatalf("expression evaluation fact expr = %T, want attr get", fact.Expr)
		}
		if got := ast.KeyName(attr.Key); got != "field" {
			t.Fatalf("expression evaluation key = %q, want field", got)
		}
		if !sourceSpanValid(fact.Span) {
			t.Fatalf("expression evaluation span is invalid: %#v", fact)
		}
		return
	}
	t.Fatal("missing WIR-backed expression evaluation for short-circuit RHS")
}
