package interproc

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestRefinementEqual(t *testing.T) {
	if !RefinementEqual(nil, nil) {
		t.Fatal("two nil refinements should be equal")
	}
	eff := &constraint.FunctionRefinement{Terminates: true}
	if RefinementEqual(eff, nil) || RefinementEqual(nil, eff) {
		t.Fatal("nil and non-nil refinements should differ")
	}
	if !RefinementEqual(eff, eff) {
		t.Fatal("same refinement pointer should be equal")
	}
}

func TestFactMapEqual(t *testing.T) {
	if !FactMapEqual(nil, nil) {
		t.Fatal("two nil fact maps should be equal")
	}
	if !FactMapEqual(map[api.GraphKey]api.Facts{}, map[api.GraphKey]api.Facts{}) {
		t.Fatal("two empty fact maps should be equal")
	}
	a := map[api.GraphKey]api.Facts{{GraphID: 1}: {}}
	b := map[api.GraphKey]api.Facts{}
	if FactMapEqual(a, b) {
		t.Fatal("maps of different length should differ")
	}
}

func TestWidenFactMap_Empty(t *testing.T) {
	result := WidenFactMap(nil, nil)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result) != 0 {
		t.Fatal("expected empty map")
	}
}

func TestWidenFactMap_OnlyPrev(t *testing.T) {
	prev := map[api.GraphKey]api.Facts{
		{GraphID: 1}: {
			FunctionFacts: api.FunctionFacts{
				1: {Summary: product.LiftVector([]typ.Type{typ.String})},
			},
		},
	}
	result := WidenFactMap(prev, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
}

func TestWidenFactMap_OnlyNext(t *testing.T) {
	next := map[api.GraphKey]api.Facts{
		{GraphID: 1}: {
			FunctionFacts: api.FunctionFacts{
				1: {Summary: product.LiftVector([]typ.Type{typ.Number})},
			},
		},
	}
	result := WidenFactMap(nil, next)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
}

func TestWidenFactMap_NormalizesNewFacts(t *testing.T) {
	fn := typ.Func().Param("value", typ.Unknown).Build()
	key := api.GraphKey{GraphID: 1, ParentHash: 2}
	next := map[api.GraphKey]api.Facts{
		key: {
			CapturedFields: api.CapturedFieldAssigns{
				cfg.SymbolID(10): {
					cfg.SymbolID(20): {
						fieldKey("after_all"): product.FromType(typ.NewOptional(fn)),
					},
				},
			},
		},
	}

	result := WidenFactMap(nil, next)
	got := result[key].CapturedFields[cfg.SymbolID(10)][cfg.SymbolID(20)][fieldKey("after_all")].ProjectValue()
	if !typ.TypeEquals(got, fn) {
		t.Fatalf("expected new facts to be normalized through WidenFacts, got %v", got)
	}
}

func TestWidenFactMap_Merge(t *testing.T) {
	prev := map[api.GraphKey]api.Facts{
		{GraphID: 1}: {
			FunctionFacts: api.FunctionFacts{
				1: {Summary: product.LiftVector([]typ.Type{typ.String})},
			},
		},
	}
	next := map[api.GraphKey]api.Facts{
		{GraphID: 2}: {
			FunctionFacts: api.FunctionFacts{
				1: {Summary: product.LiftVector([]typ.Type{typ.Number})},
			},
		},
	}
	result := WidenFactMap(prev, next)
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
}

func TestOverlayFacts_UsesConvergenceLawForVisibleProduct(t *testing.T) {
	lit := &ast.FunctionExpr{}
	prevReturn := typ.NewRecord().
		Field("next", typ.Func().Returns(typ.Unknown).Build()).
		Build()
	nextReturn := typ.NewRecord().
		Field("next", typ.Func().Returns(typ.String).Build()).
		Build()
	prev := api.Facts{
		LiteralSigs: api.LiteralSigs{
			lit: typ.Func().Returns(prevReturn).Build(),
		},
	}
	next := api.Facts{
		LiteralSigs: api.LiteralSigs{
			lit: typ.Func().Returns(nextReturn).Build(),
		},
	}

	got := OverlayFacts(prev, next)
	want := WidenFacts(prev, next)
	if !FactsEqual(got, want) {
		t.Fatalf("visible overlay must use convergence product law:\ngot=%#v\nwant=%#v", got, want)
	}
}
