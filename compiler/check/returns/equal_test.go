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
		ReturnSummaries: api.ReturnSummaries{
			1: []typ.Type{typ.String},
		},
	}
	b := api.Facts{
		ReturnSummaries: api.ReturnSummaries{
			1: []typ.Type{typ.String},
		},
	}
	if !FactsEqual(a, b) {
		t.Error("facts with same return summaries should be equal")
	}
}

func TestFactsEqual_DifferentReturnSummaries(t *testing.T) {
	a := api.Facts{
		ReturnSummaries: api.ReturnSummaries{
			1: []typ.Type{typ.String},
		},
	}
	b := api.Facts{
		ReturnSummaries: api.ReturnSummaries{
			1: []typ.Type{typ.Number},
		},
	}
	if FactsEqual(a, b) {
		t.Error("facts with different return summaries should not be equal")
	}
}

func TestReturnSummariesEqual_Empty(t *testing.T) {
	if !ReturnSummariesEqual(nil, nil) {
		t.Error("nil summaries should be equal")
	}
}

func TestReturnSummariesEqual_DifferentLength(t *testing.T) {
	a := api.ReturnSummaries{1: []typ.Type{typ.String}}
	b := api.ReturnSummaries{}
	if ReturnSummariesEqual(a, b) {
		t.Error("summaries with different lengths should not be equal")
	}
}

func TestParamHintsEqual_Empty(t *testing.T) {
	if !ParamHintsEqual(nil, nil) {
		t.Error("nil param hints should be equal")
	}
}

func TestParamHintsEqual_Same(t *testing.T) {
	a := api.ParamHints{1: []typ.Type{typ.String}}
	b := api.ParamHints{1: []typ.Type{typ.String}}
	if !ParamHintsEqual(a, b) {
		t.Error("same param hints should be equal")
	}
}

func TestFuncTypesEqual_Empty(t *testing.T) {
	if !FuncTypesEqual(nil, nil) {
		t.Error("nil func types should be equal")
	}
}

func TestFuncTypesEqual_Same(t *testing.T) {
	fn := typ.Func().Returns(typ.String).Build()
	a := api.FuncTypes{1: fn}
	b := api.FuncTypes{1: fn}
	if !FuncTypesEqual(a, b) {
		t.Error("same func types should be equal")
	}
}

func TestLiteralSigsEqual_Empty(t *testing.T) {
	if !LiteralSigsEqual(nil, nil) {
		t.Error("nil literal sigs should be equal")
	}
}

func TestCapturedTypesEqual_Empty(t *testing.T) {
	if !CapturedTypesEqual(nil, nil) {
		t.Error("nil captured types should be equal")
	}
}

func TestCapturedTypesEqual_Same(t *testing.T) {
	a := api.CapturedTypes{cfg.SymbolID(1): typ.String}
	b := api.CapturedTypes{cfg.SymbolID(1): typ.String}
	if !CapturedTypesEqual(a, b) {
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
		cfg.SymbolID(1): {cfg.SymbolID(2): {"foo": typ.String}},
	}
	b := api.CapturedFieldAssigns{
		cfg.SymbolID(3): {cfg.SymbolID(2): {"foo": typ.String}},
	}
	if CapturedFieldAssignsEqual(a, b) {
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
