package returns

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/typ"
)

func TestFactsEqual_Empty(t *testing.T) {
	a := api.Facts{}
	b := api.Facts{}
	if !FactsEqual(a, b) {
		t.Error("empty facts should be equal")
	}
}

func TestFactsEqual_ReturnSummaries(t *testing.T) {
	a := api.Facts{
		FunctionFacts: api.FunctionFacts{
			1: {Summary: []typ.Type{typ.String}},
		},
	}
	b := api.Facts{
		FunctionFacts: api.FunctionFacts{
			1: {Summary: []typ.Type{typ.String}},
		},
	}
	if !FactsEqual(a, b) {
		t.Error("facts with same return summaries should be equal")
	}
}

func TestFactsEqual_DifferentReturnSummaries(t *testing.T) {
	a := api.Facts{
		FunctionFacts: api.FunctionFacts{
			1: {Summary: []typ.Type{typ.String}},
		},
	}
	b := api.Facts{
		FunctionFacts: api.FunctionFacts{
			1: {Summary: []typ.Type{typ.Number}},
		},
	}
	if FactsEqual(a, b) {
		t.Error("facts with different return summaries should not be equal")
	}
}

func TestReturnSummariesEqual_Empty(t *testing.T) {
	if !symbolTypeVectorMapEqual(nil, nil) {
		t.Error("nil summaries should be equal")
	}
}

func TestReturnSummariesEqual_DifferentLength(t *testing.T) {
	a := api.ReturnSummaries{1: []typ.Type{typ.String}}
	b := api.ReturnSummaries{}
	if symbolTypeVectorMapEqual(a, b) {
		t.Error("summaries with different lengths should not be equal")
	}
}

func TestParamHintsEqual_Empty(t *testing.T) {
	if !symbolTypeVectorMapEqual(nil, nil) {
		t.Error("nil param hints should be equal")
	}
}

func TestParamHintsEqual_Same(t *testing.T) {
	a := api.ParamHints{1: []typ.Type{typ.String}}
	b := api.ParamHints{1: []typ.Type{typ.String}}
	if !symbolTypeVectorMapEqual(a, b) {
		t.Error("same param hints should be equal")
	}
}

func TestFuncTypesEqual_Empty(t *testing.T) {
	if !symbolTypeMapEqual(nil, nil) {
		t.Error("nil func types should be equal")
	}
}

func TestFuncTypesEqual_Same(t *testing.T) {
	fn := typ.Func().Returns(typ.String).Build()
	a := api.FuncTypes{1: fn}
	b := api.FuncTypes{1: fn}
	if !symbolTypeMapEqual(a, b) {
		t.Error("same func types should be equal")
	}
}

func TestCallablesEqual_Empty(t *testing.T) {
	if !CallablesEqual(nil, nil) {
		t.Error("nil callables should be equal")
	}
}

func TestCapturedTypesEqual_Empty(t *testing.T) {
	if !symbolTypeMapEqual(nil, nil) {
		t.Error("nil captured types should be equal")
	}
}

func TestCapturedTypesEqual_Same(t *testing.T) {
	a := api.CapturedTypes{cfg.SymbolID(1): typ.String}
	b := api.CapturedTypes{cfg.SymbolID(1): typ.String}
	if !symbolTypeMapEqual(a, b) {
		t.Error("same captured types should be equal")
	}
}

func TestCapturedFieldAssignsEqual_Empty(t *testing.T) {
	if !FieldWritesEqual(nil, nil) {
		t.Error("nil captured field assigns should be equal")
	}
}

func TestCapturedFieldAssignsEqual_DifferentCallee(t *testing.T) {
	a := api.FieldWrites{
		cfg.SymbolID(1): {cfg.SymbolID(2): {{Field: "foo"}: typ.String}},
	}
	b := api.FieldWrites{
		cfg.SymbolID(3): {cfg.SymbolID(2): {{Field: "foo"}: typ.String}},
	}
	if FieldWritesEqual(a, b) {
		t.Error("different callee symbols should not be equal")
	}
}

func TestCapturedContainerMutationsEqual_Basic(t *testing.T) {
	a := api.CapturedContainerMutations{
		cfg.SymbolID(1): {
			cfg.SymbolID(2): {
				{Segments: nil, ValueType: typ.Number},
			},
		},
	}
	b := api.CapturedContainerMutations{
		cfg.SymbolID(1): {
			cfg.SymbolID(2): {
				{Segments: nil, ValueType: typ.Number},
			},
		},
	}
	if !CapturedContainerMutationsEqual(a, b) {
		t.Error("same container mutations should be equal")
	}
}
