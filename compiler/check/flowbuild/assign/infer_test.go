package assign

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCollectInferredTypes_NilGraph(t *testing.T) {
	fc := &core.FlowContext{}
	result := CollectInferredTypes(fc, nil, nil, nil)
	if result == nil {
		t.Error("expected non-nil result for nil graph")
	}
	if len(result) != 0 {
		t.Errorf("expected empty result for nil graph, got %d entries", len(result))
	}
}

func TestCollectInferredTypes_EmptySpecTypes(t *testing.T) {
	fc := &core.FlowContext{}
	specTypes := make(api.SpecTypes)
	result := CollectInferredTypes(fc, specTypes, nil, nil)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCollectInferredTypes_WithAnnotated(t *testing.T) {
	fc := &core.FlowContext{}
	specTypes := make(api.SpecTypes)
	annotated := make(map[cfg.SymbolID]bool)
	annotated[1] = true
	result := CollectInferredTypes(fc, specTypes, annotated, nil)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCollectInferredTypes_WithInputs(t *testing.T) {
	fc := &core.FlowContext{}
	specTypes := make(api.SpecTypes)
	inputs := &flow.Inputs{
		DeclaredTypes: make(map[cfg.SymbolID]typ.Type),
		AnnotatedVars: make(map[cfg.SymbolID]bool),
	}
	result := CollectInferredTypes(fc, specTypes, nil, inputs)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCollectExprSymbols_NilExpr(t *testing.T) {
	var refs []cfg.SymbolID
	collectExprSymbols(nil, nil, &refs)
	if len(refs) != 0 {
		t.Errorf("expected no refs for nil expr, got %d", len(refs))
	}
}

func TestCollectExprSymbols_NilBindings(t *testing.T) {
	var refs []cfg.SymbolID
	expr := &ast.IdentExpr{Value: "x"}
	collectExprSymbols(expr, nil, &refs)
	if len(refs) != 0 {
		t.Errorf("expected no refs for nil bindings, got %d", len(refs))
	}
}

func TestCollectExprSymbols_IdentExpr(t *testing.T) {
	bindings := &bind.BindingTable{}
	var refs []cfg.SymbolID
	expr := &ast.IdentExpr{Value: "x"}
	collectExprSymbols(expr, bindings, &refs)
	// BindingTable is empty, so no symbol should be found
}

func TestCollectExprSymbols_AttrGetExpr(t *testing.T) {
	bindings := &bind.BindingTable{}
	var refs []cfg.SymbolID
	expr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "obj"},
		Key:    &ast.StringExpr{Value: "field"},
	}
	collectExprSymbols(expr, bindings, &refs)
}

func TestCollectExprSymbols_FuncCallExpr(t *testing.T) {
	bindings := &bind.BindingTable{}
	var refs []cfg.SymbolID
	expr := &ast.FuncCallExpr{
		Func:     &ast.IdentExpr{Value: "fn"},
		Receiver: &ast.IdentExpr{Value: "recv"},
		Args:     []ast.Expr{&ast.IdentExpr{Value: "arg"}},
	}
	collectExprSymbols(expr, bindings, &refs)
}

func TestCollectExprSymbols_TableExpr(t *testing.T) {
	bindings := &bind.BindingTable{}
	var refs []cfg.SymbolID
	expr := &ast.TableExpr{
		Fields: []*ast.Field{
			{
				Key:   &ast.StringExpr{Value: "key"},
				Value: &ast.IdentExpr{Value: "val"},
			},
		},
	}
	collectExprSymbols(expr, bindings, &refs)
}

func TestCollectExprSymbols_UnaryExpressions(t *testing.T) {
	bindings := &bind.BindingTable{}

	tests := []struct {
		name string
		expr ast.Expr
	}{
		{"UnaryMinusOpExpr", &ast.UnaryMinusOpExpr{Expr: &ast.IdentExpr{Value: "x"}}},
		{"UnaryNotOpExpr", &ast.UnaryNotOpExpr{Expr: &ast.IdentExpr{Value: "x"}}},
		{"UnaryLenOpExpr", &ast.UnaryLenOpExpr{Expr: &ast.IdentExpr{Value: "x"}}},
		{"UnaryBNotOpExpr", &ast.UnaryBNotOpExpr{Expr: &ast.IdentExpr{Value: "x"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var refs []cfg.SymbolID
			collectExprSymbols(tt.expr, bindings, &refs)
		})
	}
}

func TestCollectExprSymbols_BinaryExpressions(t *testing.T) {
	bindings := &bind.BindingTable{}

	tests := []struct {
		name string
		expr ast.Expr
	}{
		{"ArithmeticOpExpr", &ast.ArithmeticOpExpr{Lhs: &ast.IdentExpr{Value: "a"}, Rhs: &ast.IdentExpr{Value: "b"}}},
		{"RelationalOpExpr", &ast.RelationalOpExpr{Lhs: &ast.IdentExpr{Value: "a"}, Rhs: &ast.IdentExpr{Value: "b"}}},
		{"LogicalOpExpr", &ast.LogicalOpExpr{Lhs: &ast.IdentExpr{Value: "a"}, Rhs: &ast.IdentExpr{Value: "b"}}},
		{"StringConcatOpExpr", &ast.StringConcatOpExpr{Lhs: &ast.IdentExpr{Value: "a"}, Rhs: &ast.IdentExpr{Value: "b"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var refs []cfg.SymbolID
			collectExprSymbols(tt.expr, bindings, &refs)
		})
	}
}

func TestCollectExprSymbols_CastExpr(t *testing.T) {
	bindings := &bind.BindingTable{}
	var refs []cfg.SymbolID
	expr := &ast.CastExpr{Expr: &ast.IdentExpr{Value: "x"}}
	collectExprSymbols(expr, bindings, &refs)
}

func TestCollectExprSymbols_NonNilAssertExpr(t *testing.T) {
	bindings := &bind.BindingTable{}
	var refs []cfg.SymbolID
	expr := &ast.NonNilAssertExpr{Expr: &ast.IdentExpr{Value: "x"}}
	collectExprSymbols(expr, bindings, &refs)
}

func TestCollectExprSymbols_Comma3Expr(t *testing.T) {
	bindings := &bind.BindingTable{}
	var refs []cfg.SymbolID
	expr := &ast.Comma3Expr{}
	collectExprSymbols(expr, bindings, &refs)
	if len(refs) != 0 {
		t.Errorf("expected no refs for Comma3Expr, got %d", len(refs))
	}
}
