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

func TestFunctionFactMapsEqual(t *testing.T) {
	if !FunctionFactMapsEqual(nil, nil) {
		t.Fatal("two nil fact maps should be equal")
	}
	if !FunctionFactMapsEqual(map[api.GraphKey]api.FunctionFacts{}, map[api.GraphKey]api.FunctionFacts{}) {
		t.Fatal("two empty fact maps should be equal")
	}
	a := map[api.GraphKey]api.FunctionFacts{{GraphID: 1}: {}}
	b := map[api.GraphKey]api.FunctionFacts{}
	if FunctionFactMapsEqual(a, b) {
		t.Fatal("maps of different length should differ")
	}
}

func TestWidenFunctionFactMaps_Empty(t *testing.T) {
	result := WidenFunctionFactMaps(nil, nil)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result) != 0 {
		t.Fatal("expected empty map")
	}
}

func TestWidenFunctionFactMaps_OnlyPrev(t *testing.T) {
	prev := map[api.GraphKey]api.FunctionFacts{
		{GraphID: 1}: {
			1: {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.String})}},
		},
	}
	result := WidenFunctionFactMaps(prev, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
}

func TestWidenFunctionFactMaps_OnlyNext(t *testing.T) {
	next := map[api.GraphKey]api.FunctionFacts{
		{GraphID: 1}: {
			1: {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.Number})}},
		},
	}
	result := WidenFunctionFactMaps(nil, next)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
}

func TestWidenCapturedFieldAssignMaps_NormalizesNewFacts(t *testing.T) {
	fn := typ.Func().Param("value", typ.Unknown).Build()
	key := api.GraphKey{GraphID: 1, ParentHash: 2}
	next := map[api.GraphKey]api.CapturedFieldAssigns{
		key: {
			cfg.SymbolID(10): {
				cfg.SymbolID(20): {
					fieldKey("after_all"): product.FromType(typ.NewOptional(fn)),
				},
			},
		},
	}

	result := WidenCapturedFieldAssignMaps(nil, next)
	got := result[key][cfg.SymbolID(10)][cfg.SymbolID(20)][fieldKey("after_all")].ProjectValue()
	if !typ.TypeEquals(got, fn) {
		t.Fatalf("expected new facts to be normalized through WidenCapturedFieldAssignMaps, got %v", got)
	}
}

func TestWidenFunctionFactMaps_Merge(t *testing.T) {
	prev := map[api.GraphKey]api.FunctionFacts{
		{GraphID: 1}: {
			1: {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.String})}},
		},
	}
	next := map[api.GraphKey]api.FunctionFacts{
		{GraphID: 2}: {
			1: {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.Number})}},
		},
	}
	result := WidenFunctionFactMaps(prev, next)
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
}
