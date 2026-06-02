package api

import (
	"testing"

	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestAnalysisContextParentHash_NormalizesRecursiveExpectedFunction(t *testing.T) {
	seed := typ.NewRecord().
		Field("new", typ.Func().Returns(typ.Unknown).Build()).
		Field("method", typ.Func().Returns(typ.Unknown).Build()).
		Build()
	owner := typ.NewRecord().
		Field("new", typ.Func().Returns(seed).Build()).
		Field("method", typ.Func().Param("self", seed).Returns(seed).Build()).
		Build()
	raw := typ.Func().Param("self", owner).Returns(owner).Build()
	normalized := value.WidenFunctionForConvergence(raw)

	rawHash := AnalysisContext{ExpectedFunction: raw}.ParentHash(123)
	normalizedHash := AnalysisContext{ExpectedFunction: normalized}.ParentHash(123)
	if rawHash != normalizedHash {
		t.Fatalf("ParentHash(raw) = %x, ParentHash(normalized) = %x", rawHash, normalizedHash)
	}

	merged := MergeAnalysisContext(AnalysisContext{}, AnalysisContext{ExpectedFunction: raw})
	if !typ.ContainsRecursive(merged.ExpectedFunction) {
		t.Fatalf("merged expected function was not convergence-normalized: %v", merged.ExpectedFunction)
	}
}

func TestMergeAnalysisContext_ExpectedFunctionUsesValueConvergence(t *testing.T) {
	base := typ.NewRecord().Field("x", typ.Number).Build()
	grown := typ.NewRecord().
		Field("x", typ.Number).
		Field("next", typ.NewRecord().Field("value", base).Build()).
		Build()
	left := AnalysisContext{
		ExpectedFunction: typ.Func().Param("self", base).Returns(base).Build(),
	}
	right := AnalysisContext{
		ExpectedFunction: typ.Func().Param("self", grown).Returns(grown).Build(),
	}

	merged := MergeAnalysisContext(left, right)
	if merged.ExpectedFunction == nil {
		t.Fatal("merged expected function is nil")
	}
	if len(merged.ExpectedFunction.Params) != 1 || !typ.ContainsRecursive(merged.ExpectedFunction.Params[0].Type) {
		t.Fatalf("merged param did not use value convergence: %v", merged.ExpectedFunction.Params)
	}
	if len(merged.ExpectedFunction.Returns) != 1 || !typ.ContainsRecursive(merged.ExpectedFunction.Returns[0]) {
		t.Fatalf("merged return did not use value convergence: %v", merged.ExpectedFunction.Returns)
	}
}

func TestAnalysisContextGlobalOverlayUsesCarrierSemantics(t *testing.T) {
	left := AnalysisContext{
		GlobalOverlay: GlobalOverlayFromValues(map[GlobalName]product.AbstractValue{
			GlobalName("z"): product.FromType(typ.Number),
			GlobalName("a"): product.FromType(typ.String),
		}),
	}
	right := AnalysisContext{
		GlobalOverlay: GlobalOverlayFromValues(map[GlobalName]product.AbstractValue{
			GlobalName("a"): product.FromType(typ.Boolean),
			GlobalName("m"): product.FromType(typ.Integer),
		}),
	}

	merged := MergeAnalysisContext(left, right)
	if len(merged.GlobalOverlay) != 3 {
		t.Fatalf("merged overlay = %+v, want 3 bindings", merged.GlobalOverlay)
	}
	if merged.GlobalOverlay[0].Name != GlobalName("a") || merged.GlobalOverlay[1].Name != GlobalName("m") || merged.GlobalOverlay[2].Name != GlobalName("z") {
		t.Fatalf("merged overlay order = %+v, want a,m,z", merged.GlobalOverlay)
	}
	wantA := product.CarryForward(product.FromType(typ.String), product.FromType(typ.Boolean))
	if got, ok := merged.GlobalOverlay.Value("a"); !ok || !product.Equal(got, wantA) {
		t.Fatalf("merged a = %v, %v; want CarryForward(string, boolean)", got, ok)
	}

	projected := ProjectGlobalOverlay(merged.GlobalOverlay)
	if len(projected) != 3 || !typ.TypeEquals(projected["m"], typ.Integer) || !typ.TypeEquals(projected["z"], typ.Number) {
		t.Fatalf("ProjectGlobalOverlay = %+v, want m=integer,z=number plus merged a", projected)
	}
}

func TestAnalysisContextParentHashNormalizesGlobalOverlay(t *testing.T) {
	a := AnalysisContext{
		GlobalOverlay: GlobalOverlayFromValues(map[GlobalName]product.AbstractValue{
			GlobalName("z"): product.FromType(typ.Number),
			GlobalName("a"): product.FromType(typ.String),
		}),
	}
	b := AnalysisContext{
		GlobalOverlay: GlobalOverlayFromValues(map[GlobalName]product.AbstractValue{
			GlobalName("a"): product.FromType(typ.String),
			GlobalName("z"): product.FromType(typ.Number),
		}),
	}
	if gotA, gotB := a.ParentHash(99), b.ParentHash(99); gotA != gotB {
		t.Fatalf("ParentHash order-dependent: %x != %x", gotA, gotB)
	}
}
