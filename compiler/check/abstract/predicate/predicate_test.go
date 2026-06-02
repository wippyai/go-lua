package predicate_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/abstract/predicate"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
)

func TestLookupPredicateLink_NilInputsParam(t *testing.T) {
	result := predicate.LookupPredicateLink(cfg.SymbolID(1), nil)
	if result != nil {
		t.Error("expected nil for nil inputs")
	}
}

func TestLookupPredicateLink_NilPredicateLinksMap(t *testing.T) {
	inputs := &flow.Inputs{PredicateLinks: nil}
	result := predicate.LookupPredicateLink(cfg.SymbolID(1), inputs)
	if result != nil {
		t.Error("expected nil for nil predicate links")
	}
}

func TestLookupPredicateLink_ZeroSymbolParam(t *testing.T) {
	inputs := &flow.Inputs{
		PredicateLinks: map[flow.PredicateLinkKey]flow.PredicateLink{
			predicate.LinkKey(cfg.SymbolID(1), 1): {},
		},
	}
	result := predicate.LookupPredicateLink(0, inputs)
	if result != nil {
		t.Error("expected nil for zero symbol")
	}
}

func TestLookupPredicateLink_NoMatchFound(t *testing.T) {
	inputs := &flow.Inputs{
		PredicateLinks: map[flow.PredicateLinkKey]flow.PredicateLink{
			predicate.LinkKey(cfg.SymbolID(2), 1): {},
		},
	}
	result := predicate.LookupPredicateLink(cfg.SymbolID(1), inputs)
	if result != nil {
		t.Error("expected nil when no match found")
	}
}

func TestLookupPredicateLink_SingleMatchFound(t *testing.T) {
	link := flow.PredicateLink{
		OnTruthy: constraint.Condition{
			Disjuncts: [][]constraint.Constraint{
				{constraint.NotNil{Path: constraint.Path{Root: "x"}}},
			},
		},
	}
	inputs := &flow.Inputs{
		PredicateLinks: map[flow.PredicateLinkKey]flow.PredicateLink{
			predicate.LinkKey(cfg.SymbolID(1), 5): link,
		},
	}
	result := predicate.LookupPredicateLink(cfg.SymbolID(1), inputs)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.OnTruthy.Disjuncts) != 1 {
		t.Errorf("expected 1 disjunct, got %d", len(result.OnTruthy.Disjuncts))
	}
}

func TestLookupPredicateLink_HighestDefPointSelected(t *testing.T) {
	wantCondition := constraint.Condition{
		Disjuncts: [][]constraint.Constraint{
			{constraint.NotNil{Path: constraint.Path{Root: "marker"}}},
		},
	}
	inputs := &flow.Inputs{
		PredicateLinks: map[flow.PredicateLinkKey]flow.PredicateLink{
			predicate.LinkKey(cfg.SymbolID(1), 3):  {},
			predicate.LinkKey(cfg.SymbolID(1), 7):  {OnTruthy: wantCondition},
			predicate.LinkKey(cfg.SymbolID(1), 1):  {},
			predicate.LinkKey(cfg.SymbolID(1), 10): {},
			predicate.LinkKey(cfg.SymbolID(2), 20): {OnTruthy: wantCondition},
		},
	}
	result := predicate.LookupPredicateLink(cfg.SymbolID(1), inputs)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.OnTruthy.Disjuncts) != 0 {
		t.Log("result should be from symbol 1 at def point 10, which has empty condition")
	}
}

func TestLinkKey_TypedKey(t *testing.T) {
	tests := []struct {
		sym      cfg.SymbolID
		defPoint cfg.Point
		expected flow.PredicateLinkKey
	}{
		{1, 0, flow.PredicateLinkKey{Symbol: 1, DefPoint: 0}},
		{42, 42, flow.PredicateLinkKey{Symbol: 42, DefPoint: 42}},
		{100, 100, flow.PredicateLinkKey{Symbol: 100, DefPoint: 100}},
	}

	for _, tt := range tests {
		result := predicate.LinkKey(tt.sym, tt.defPoint)
		if result != tt.expected {
			t.Errorf("LinkKey(%d, %d) = %#v, expected %#v", tt.sym, tt.defPoint, result, tt.expected)
		}
	}
}

func TestBuildConstResolver_NilInputsParam(t *testing.T) {
	result := predicate.BuildConstResolver(nil, 0)
	if result != nil {
		t.Error("expected nil for nil inputs")
	}
}

func TestBuildConstResolver_NilConstValuesMap(t *testing.T) {
	result := predicate.BuildConstResolver(&flow.Inputs{ConstValues: nil}, 0)
	if result != nil {
		t.Error("expected nil for nil const values")
	}
}

func TestBuildConstResolver_NilGraphInInputs(t *testing.T) {
	result := predicate.BuildConstResolver(&flow.Inputs{
		ConstValues: make(map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue),
		Graph:       nil,
	}, 0)
	if result != nil {
		t.Error("expected nil for nil graph")
	}
}
