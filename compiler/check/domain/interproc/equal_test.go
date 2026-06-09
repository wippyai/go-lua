package interproc

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestFactsEqual_Empty(t *testing.T) {
	a := api.Facts{}
	b := api.Facts{}
	if !FactsEqual(a, b) {
		t.Error("empty facts should be equal")
	}
}

func TestFactsEqual_FunctionFactSummary(t *testing.T) {
	a := api.Facts{
		FunctionFacts: api.FunctionFacts{
			1: {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.String})}},
		},
	}
	b := api.Facts{
		FunctionFacts: api.FunctionFacts{
			1: {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.String})}},
		},
	}
	if !FactsEqual(a, b) {
		t.Error("facts with same return summaries should be equal")
	}
}

func TestFactsEqual_DifferentFunctionFactSummary(t *testing.T) {
	a := api.Facts{
		FunctionFacts: api.FunctionFacts{
			1: {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.String})}},
		},
	}
	b := api.Facts{
		FunctionFacts: api.FunctionFacts{
			1: {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.Number})}},
		},
	}
	if FactsEqual(a, b) {
		t.Error("facts with different return summaries should not be equal")
	}
}

func TestFactsEqual_UsesCanonicalFunctionFactsOnly(t *testing.T) {
	sym := cfg.SymbolID(77)
	fn := typ.Func().Returns(typ.String).Build()

	a := api.Facts{
		FunctionFacts: api.FunctionFacts{
			sym: {
				Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.String}), Postflow: product.LiftVector([]typ.Type{typ.String})},
				Public:  api.FunctionPublicProjection{Signature: fn},
			},
		},
	}
	b := api.Facts{
		FunctionFacts: api.FunctionFacts{
			sym: {
				Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.String}), Postflow: product.LiftVector([]typ.Type{typ.String})},
				Public:  api.FunctionPublicProjection{Signature: fn},
			},
		},
	}

	if !FactsEqual(a, b) {
		t.Fatal("expected facts to be equal by FunctionFacts projection")
	}
}

func TestFactsEqual_DifferentCanonicalFunctionFacts(t *testing.T) {
	sym := cfg.SymbolID(91)

	a := api.Facts{
		FunctionFacts: api.FunctionFacts{
			sym: {
				Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.String})},
				Public:  api.FunctionPublicProjection{Signature: typ.Func().Returns(typ.String).Build()},
			},
		},
	}
	b := api.Facts{
		FunctionFacts: api.FunctionFacts{
			sym: {
				Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.Number})},
				Public:  api.FunctionPublicProjection{Signature: typ.Func().Returns(typ.Number).Build()},
			},
		},
	}

	if FactsEqual(a, b) {
		t.Fatal("different FunctionFacts projection should not be equal")
	}
}

func TestSymbolTypeMapEqual_Empty(t *testing.T) {
	if !symbolTypeMapEqual(nil, nil) {
		t.Error("nil func types should be equal")
	}
}

func TestFunctionFactsEqual_Params(t *testing.T) {
	a := api.FunctionFacts{1: {Call: api.FunctionCallProjection{Params: product.LiftVector([]typ.Type{typ.String})}}}
	b := api.FunctionFacts{1: {Call: api.FunctionCallProjection{Params: product.LiftVector([]typ.Type{typ.String})}}}
	if !FunctionFactsEqual(a, b) {
		t.Error("same canonical parameter evidence should be equal")
	}
}

func TestFunctionFactsEqual_DifferentParams(t *testing.T) {
	a := api.FunctionFacts{1: {Call: api.FunctionCallProjection{Params: product.LiftVector([]typ.Type{typ.String})}}}
	b := api.FunctionFacts{1: {Call: api.FunctionCallProjection{Params: product.LiftVector([]typ.Type{typ.Number})}}}
	if FunctionFactsEqual(a, b) {
		t.Error("different canonical parameter evidence should not be equal")
	}
}

func TestFunctionFactsEqual_FunctionSpecIsCanonicalFactState(t *testing.T) {
	callback := typ.Func().Param("value", typ.String).Build()
	spec := contract.NewSpec().WithCallback(0, (&contract.CallbackSpec{}).WithEnvOverlay(map[string]typ.Type{
		"up": callback,
	}))
	withoutSpec := typ.Func().Param("fn", callback).Build()
	withSpec := typ.Func().Param("fn", callback).Spec(spec).Build()

	if !typ.TypeEquals(withoutSpec, withSpec) {
		t.Fatal("ordinary type equality should ignore function specs")
	}

	a := api.FunctionFacts{1: {Public: api.FunctionPublicProjection{Signature: withoutSpec}}}
	b := api.FunctionFacts{1: {Public: api.FunctionPublicProjection{Signature: withSpec}}}
	if FunctionFactsEqual(a, b) {
		t.Fatal("function fact equality must include function specs")
	}
}

func TestFunctionFactsEqual_MetatableFactStateIsCanonical(t *testing.T) {
	spec := contract.NewSpec().WithCallback(0, &contract.CallbackSpec{Cardinality: contract.CardExactlyOnce})
	method := typ.Func().Returns(typ.String).Build()
	methodWithSpec := typ.Func().Returns(typ.String).Spec(spec).Build()
	withoutMetaSpec := typ.NewRecord().
		Metatable(typ.NewRecord().
			Field("__index", typ.NewRecord().Field("run", method).Build()).
			Build()).
		Build()
	withMetaSpec := typ.NewRecord().
		Metatable(typ.NewRecord().
			Field("__index", typ.NewRecord().Field("run", methodWithSpec).Build()).
			Build()).
		Build()

	if !typ.TypeEquals(withoutMetaSpec, withMetaSpec) {
		t.Fatal("ordinary type equality should ignore nested function specs in metatables")
	}

	a := api.FunctionFacts{1: {Public: api.FunctionPublicProjection{Signature: typ.Func().Returns(withoutMetaSpec).Build()}}}
	b := api.FunctionFacts{1: {Public: api.FunctionPublicProjection{Signature: typ.Func().Returns(withMetaSpec).Build()}}}
	if FunctionFactsEqual(a, b) {
		t.Fatal("function fact equality must include metatable fact state")
	}
}

func TestWidenFacts_PreservesFunctionSpecChange(t *testing.T) {
	callback := typ.Func().Param("value", typ.String).Build()
	spec := contract.NewSpec().WithCallback(0, (&contract.CallbackSpec{}).WithEnvOverlay(map[string]typ.Type{
		"up": callback,
	}))
	withoutSpec := typ.Func().Param("fn", callback).Build()
	withSpec := typ.Func().Param("fn", callback).Spec(spec).Build()
	sym := cfg.SymbolID(7)
	prev := api.Facts{FunctionFacts: api.FunctionFacts{sym: {Public: api.FunctionPublicProjection{Signature: withoutSpec}}}}
	next := api.Facts{FunctionFacts: api.FunctionFacts{sym: {Public: api.FunctionPublicProjection{Signature: withSpec}}}}

	widened := WidenFacts(prev, next)
	got := widened.FunctionFacts[sym].Public.Signature
	gotSpec := contract.ExtractSpec(got)
	if gotSpec == nil || !gotSpec.Equals(spec) {
		t.Fatalf("expected widened fact type to preserve callback spec, got %v", got)
	}
	if FactsEqual(prev, widened) {
		t.Fatal("fact equality must observe a newly inferred function spec")
	}
}

func TestSymbolTypeMapEqual_Same(t *testing.T) {
	fn := typ.Func().Returns(typ.String).Build()
	a := map[cfg.SymbolID]product.AbstractValue{1: product.FromType(fn)}
	b := map[cfg.SymbolID]product.AbstractValue{1: product.FromType(fn)}
	if !symbolTypeMapEqual(a, b) {
		t.Error("same func types should be equal")
	}
}

func TestLiteralSigsEqual_Empty(t *testing.T) {
	if !LiteralSigsEqual(nil, nil) {
		t.Error("nil literal sigs should be equal")
	}
}

func TestCapturedTypesEqual_Empty(t *testing.T) {
	if !symbolTypeMapEqual(nil, nil) {
		t.Error("nil captured types should be equal")
	}
}

func TestCapturedTypesEqual_Same(t *testing.T) {
	a := api.CapturedTypes{cfg.SymbolID(1): product.FromType(typ.String)}
	b := api.CapturedTypes{cfg.SymbolID(1): product.FromType(typ.String)}
	if !symbolTypeMapEqual(a, b) {
		t.Error("same captured types should be equal")
	}
}

func TestCapturedFieldAssignsEqual_Empty(t *testing.T) {
	if !CapturedFieldAssignsEqual(nil, nil) {
		t.Error("nil captured field assigns should be equal")
	}
}

func TestCapturedFieldAssignsEqual_DifferentCallee(t *testing.T) {
	a := api.CapturedFieldAssigns{
		cfg.SymbolID(1): {cfg.SymbolID(2): {fieldKey("foo"): product.FromType(typ.String)}},
	}
	b := api.CapturedFieldAssigns{
		cfg.SymbolID(3): {cfg.SymbolID(2): {fieldKey("foo"): product.FromType(typ.String)}},
	}
	if CapturedFieldAssignsEqual(a, b) {
		t.Error("different callee symbols should not be equal")
	}
}

func TestCapturedFieldAssignsEqual_DoesNotRepairOptionalFunctionValues(t *testing.T) {
	fn := typ.Func().Param("fn", typ.Unknown).Build()
	left := api.CapturedFieldAssigns{1: {2: {fieldKey("after_all"): product.FromType(typ.NewOptional(fn))}}}
	right := api.CapturedFieldAssigns{1: {2: {fieldKey("after_all"): product.FromType(fn)}}}
	if CapturedFieldAssignsEqual(left, right) {
		t.Fatal("equality must compare stored canonical products, not repair non-canonical optional function values")
	}
}
