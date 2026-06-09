package interproc

import (
	"testing"

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

func TestProjectionProductMapEqual(t *testing.T) {
	if !ProjectionProductMapEqual(nil, nil) {
		t.Fatal("two nil fact maps should be equal")
	}
	if !ProjectionProductMapEqual(map[api.GraphKey]ProjectionProduct{}, map[api.GraphKey]ProjectionProduct{}) {
		t.Fatal("two empty fact maps should be equal")
	}
	a := map[api.GraphKey]ProjectionProduct{{GraphID: 1}: {}}
	b := map[api.GraphKey]ProjectionProduct{}
	if ProjectionProductMapEqual(a, b) {
		t.Fatal("maps of different length should differ")
	}
}

func TestWidenProjectionProductMap_Empty(t *testing.T) {
	result := WidenProjectionProductMap(nil, nil)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result) != 0 {
		t.Fatal("expected empty map")
	}
}

func TestWidenProjectionProductMap_OnlyPrev(t *testing.T) {
	prev := map[api.GraphKey]ProjectionProduct{
		{GraphID: 1}: {
			FunctionFacts: api.FunctionFacts{
				1: {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.String})}},
			},
		},
	}
	result := WidenProjectionProductMap(prev, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
}

func TestWidenProjectionProductMap_OnlyNext(t *testing.T) {
	next := map[api.GraphKey]ProjectionProduct{
		{GraphID: 1}: {
			FunctionFacts: api.FunctionFacts{
				1: {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.Number})}},
			},
		},
	}
	result := WidenProjectionProductMap(nil, next)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
}

func TestWidenProjectionProductMap_NormalizesNewFacts(t *testing.T) {
	fn := typ.Func().Param("value", typ.Unknown).Build()
	key := api.GraphKey{GraphID: 1, ParentHash: 2}
	next := map[api.GraphKey]ProjectionProduct{
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

	result := WidenProjectionProductMap(nil, next)
	got := result[key].CapturedFields[cfg.SymbolID(10)][cfg.SymbolID(20)][fieldKey("after_all")].ProjectValue()
	if !typ.TypeEquals(got, fn) {
		t.Fatalf("expected new facts to be normalized through WidenProjectionProduct, got %v", got)
	}
}

func TestWidenProjectionProductMap_Merge(t *testing.T) {
	prev := map[api.GraphKey]ProjectionProduct{
		{GraphID: 1}: {
			FunctionFacts: api.FunctionFacts{
				1: {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.String})}},
			},
		},
	}
	next := map[api.GraphKey]ProjectionProduct{
		{GraphID: 2}: {
			FunctionFacts: api.FunctionFacts{
				1: {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.Number})}},
			},
		},
	}
	result := WidenProjectionProductMap(prev, next)
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
}
