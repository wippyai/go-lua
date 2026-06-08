package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

func TestConditionSymbolEvidenceSeparatesValueAndVariantOriginSymbols(t *testing.T) {
	first := constraint.NewPath(cfg.SymbolID(31), "first")
	second := constraint.NewPath(cfg.SymbolID(32), "second")
	family := uint64(17)

	evidence := newConditionSymbolEvidence(constraint.FromConstraints(
		constraint.Truthy{Path: second.Field("id")},
		constraint.FieldEqualsPath{Target: first, Field: "owner", Value: second},
		constraint.VariantCaseEquals{Target: first, OriginFamily: family, CaseIndex: 0},
	))

	valueWant := []cfg.SymbolID{31, 32}
	if got := evidence.ValueSymbols(); !sameSymbols(got, valueWant) {
		t.Fatalf("ValueSymbols() = %#v, want %#v", got, valueWant)
	}
	variantWant := []cfg.SymbolID{31}
	if got := evidence.VariantOriginSymbols(); !sameSymbols(got, variantWant) {
		t.Fatalf("VariantOriginSymbols() = %#v, want %#v", got, variantWant)
	}
	if !evidence.HasVariantOriginSymbol(31) || evidence.HasVariantOriginSymbol(32) {
		t.Fatalf("variant origin membership mismatch: %#v", evidence.VariantOriginSymbols())
	}
}

func TestConditionSymbolEvidenceProjectsValueConditionForSymbol(t *testing.T) {
	first := constraint.NewPath(cfg.SymbolID(31), "first")
	second := constraint.NewPath(cfg.SymbolID(32), "second")
	evidence := newConditionSymbolEvidence(constraint.FromConstraints(
		constraint.Truthy{Path: first.Field("id")},
		constraint.FieldEqualsPath{Target: first, Field: "owner", Value: second},
		constraint.NotNil{Path: second.Field("name")},
	))

	got := evidence.ValueConditionForSymbol(31)
	if got.IsTrue() || got.IsFalse() || got.NumDisjuncts() != 1 {
		t.Fatalf("projected first condition = %v, want one constrained disjunct", got)
	}
	constraints := got.DisjunctConstraints(0)
	if len(constraints) != 1 {
		t.Fatalf("projected first constraints = %#v, want only local truthy constraint", constraints)
	}
	if _, ok := constraints[0].(constraint.Truthy); !ok {
		t.Fatalf("projected first constraint = %T, want Truthy", constraints[0])
	}
}

func TestConditionSymbolEvidenceProjectionKeepsDNFUnconstrainedBranchSound(t *testing.T) {
	first := constraint.NewPath(cfg.SymbolID(31), "first")
	second := constraint.NewPath(cfg.SymbolID(32), "second")
	evidence := newConditionSymbolEvidence(constraint.FromDisjuncts([][]constraint.Constraint{
		{constraint.Truthy{Path: first.Field("id")}},
		{constraint.Truthy{Path: second.Field("name")}},
	}))

	if got := evidence.ValueConditionForSymbol(31); !got.IsTrue() {
		t.Fatalf("projected first condition = %v, want true because another DNF arm does not constrain first", got)
	}
}

func TestConditionSymbolMaskMatchesSemanticAffectedPaths(t *testing.T) {
	root := constraint.NewPath(cfg.SymbolID(41), "root")
	other := constraint.NewPath(cfg.SymbolID(42), "other")
	mask := NewConditionSymbolMask([]cfg.SymbolID{41})

	if !mask.MatchesConstraint(constraint.FieldEqualsPath{Target: root, Field: "owner", Value: other}) {
		t.Fatal("mask should match a constraint reading the masked target symbol")
	}
	if mask.MatchesConstraint(constraint.Truthy{Path: other.Field("id")}) {
		t.Fatal("mask should not match unrelated symbols")
	}
}

func sameSymbols(got, want []cfg.SymbolID) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
