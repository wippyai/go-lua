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

func TestFactsEqual_FunctionFactSummary(t *testing.T) {
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

func TestFactsEqual_DifferentFunctionFactSummary(t *testing.T) {
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

func TestFactsEqual_UsesCanonicalFunctionFactsOnly(t *testing.T) {
	sym := cfg.SymbolID(77)
	fn := typ.Func().Returns(typ.String).Build()

	a := api.Facts{
		FunctionFacts: api.FunctionFacts{
			sym: {
				Summary: []typ.Type{typ.String},
				Narrow:  []typ.Type{typ.String},
				Type:    fn,
			},
		},
	}
	b := api.Facts{
		FunctionFacts: api.FunctionFacts{
			sym: {
				Summary: []typ.Type{typ.String},
				Narrow:  []typ.Type{typ.String},
				Type:    fn,
			},
		},
	}

	if !FactsEqual(a, b) {
		t.Fatal("expected facts to be equal by canonical function facts")
	}
}

func TestFactsEqual_DifferentCanonicalFunctionFacts(t *testing.T) {
	sym := cfg.SymbolID(91)

	a := api.Facts{
		FunctionFacts: api.FunctionFacts{
			sym: {Summary: []typ.Type{typ.String}, Type: typ.Func().Returns(typ.String).Build()},
		},
	}
	b := api.Facts{
		FunctionFacts: api.FunctionFacts{
			sym: {Summary: []typ.Type{typ.Number}, Type: typ.Func().Returns(typ.Number).Build()},
		},
	}

	if FactsEqual(a, b) {
		t.Fatal("different canonical function facts should not be equal")
	}
}

func TestTypeVectorMapEqual_Empty(t *testing.T) {
	if !symbolTypeVectorMapEqual(nil, nil) {
		t.Error("nil summaries should be equal")
	}
}

func TestTypeVectorMapEqual_DifferentLength(t *testing.T) {
	a := map[cfg.SymbolID][]typ.Type{1: {typ.String}}
	b := map[cfg.SymbolID][]typ.Type{}
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

func TestSymbolTypeMapEqual_Empty(t *testing.T) {
	if !symbolTypeMapEqual(nil, nil) {
		t.Error("nil func types should be equal")
	}
}

func TestSymbolTypeMapEqual_Same(t *testing.T) {
	fn := typ.Func().Returns(typ.String).Build()
	a := map[cfg.SymbolID]typ.Type{1: fn}
	b := map[cfg.SymbolID]typ.Type{1: fn}
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
	a := api.CapturedTypes{cfg.SymbolID(1): typ.String}
	b := api.CapturedTypes{cfg.SymbolID(1): typ.String}
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

func TestCapturedContainerMutationsEqual_DifferentOperatorKind(t *testing.T) {
	a := api.CapturedContainerMutations{
		cfg.SymbolID(1): {
			cfg.SymbolID(2): {
				{Kind: api.ContainerMutationContainerElement, ValueType: typ.Number},
			},
		},
	}
	b := api.CapturedContainerMutations{
		cfg.SymbolID(1): {
			cfg.SymbolID(2): {
				{Kind: api.ContainerMutationTableElement, ValueType: typ.Number},
			},
		},
	}
	if CapturedContainerMutationsEqual(a, b) {
		t.Error("same path with different mutation operators should not be equal")
	}
}

func TestCapturedFieldAssignsEqual_CanonicalizesOptionalFunctionValues(t *testing.T) {
	fn := typ.Func().Param("fn", typ.Unknown).Build()
	left := api.CapturedFieldAssigns{1: {2: {"after_all": typ.NewOptional(fn)}}}
	right := api.CapturedFieldAssigns{1: {2: {"after_all": fn}}}
	if !CapturedFieldAssignsEqual(left, right) {
		t.Fatal("expected optional function captured field to equal canonical function value")
	}
}
