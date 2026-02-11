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
	bindings := bind.NewBindingTable()
	base := &ast.IdentExpr{Value: "obj"}
	baseSym := cfg.SymbolID(301)
	bindings.Bind(base, baseSym)
	bindings.SetName(baseSym, "obj")
	fieldSym := bindings.GetOrCreateFieldSymbol(baseSym, "field")

	var refs []cfg.SymbolID
	expr := &ast.AttrGetExpr{
		Object: base,
		Key:    &ast.StringExpr{Value: "field"},
	}
	collectExprSymbols(expr, bindings, &refs)
	if !hasSymbol(refs, fieldSym) {
		t.Fatalf("expected refs to include field symbol %d, got %v", fieldSym, refs)
	}
	if !hasSymbol(refs, baseSym) {
		t.Fatalf("expected refs to include base symbol %d, got %v", baseSym, refs)
	}
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

func TestJoinInferredType_StabilizesSelfEmbeddingFromUnknown(t *testing.T) {
	old := typ.Unknown
	next := typ.NewArray(typ.Unknown)

	got := joinInferredType(old, next)
	if !typ.TypeEquals(got, next) {
		t.Fatalf("joinInferredType(unknown, any[]) = %v, want %v", got, next)
	}
}

func TestJoinInferredType_StopsRecursiveNestingGrowth(t *testing.T) {
	old := typ.NewArray(typ.Unknown)
	next := typ.NewArray(old)

	got := joinInferredType(old, next)
	if !typ.TypeEquals(got, old) {
		t.Fatalf("joinInferredType(any[], any[][]) = %v, want %v", got, old)
	}
}

func TestTypeContains(t *testing.T) {
	base := typ.NewArray(typ.Unknown)
	outer := typ.NewArray(base)
	if !typeContains(outer, base) {
		t.Fatal("expected typeContains(any[][], any[]) to be true")
	}
	if typeContains(typ.Number, base) {
		t.Fatal("expected typeContains(number, any[]) to be false")
	}
}

func TestMergeSpecTypesSoft_IgnoresUnknownAndNilOverrides(t *testing.T) {
	sym := cfg.SymbolID(1)
	base := api.SpecTypes{
		sym: typ.NewOptional(typ.LuaError),
	}
	override := api.SpecTypes{
		sym: typ.Nil,
	}
	merged := mergeSpecTypesSoft(base, override)
	got, ok := merged[sym]
	if !ok || got == nil {
		t.Fatalf("expected merged type for symbol %d", sym)
	}
	if !typ.TypeEquals(got, base[sym]) {
		t.Fatalf("merged type = %v, want %v", got, base[sym])
	}
}

func hasSymbol(refs []cfg.SymbolID, sym cfg.SymbolID) bool {
	for _, r := range refs {
		if r == sym {
			return true
		}
	}
	return false
}
