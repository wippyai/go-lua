package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestConditionEffectVariantOriginDNFJoinsAllowedCases(t *testing.T) {
	sym := cfg.SymbolID(42)
	family := uint64(777)
	path := constraint.Path{Root: "result", Symbol: sym}
	fact := constraint.Or(
		constraint.FromConstraints(constraint.VariantCaseEquals{
			Target:       path,
			OriginFamily: family,
			CaseIndex:    0,
		}),
		constraint.FromConstraints(constraint.VariantCaseEquals{
			Target:       path,
			OriginFamily: family,
			CaseIndex:    1,
		}),
	)
	base := product.FromType(typ.String)
	state := flow.PointState{
		Cond: constraint.TrueCondition(),
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym): product.WithVariantOrigin(base, family, []int{0, 1, 2}),
		},
	}

	tr := &Transfer{}
	if !tr.applyConditionEffect(&state, ConditionEffect{Fact: fact}) {
		t.Fatalf("condition effect reported no change")
	}
	got, ok := tr.symbolValue(&state, sym)
	if !ok {
		t.Fatalf("condition effect removed symbol value")
	}
	want := product.WithVariantOrigin(base, family, []int{0, 1})
	if !product.Equal(got, want) {
		t.Fatalf("DNF origin reduction = %#v, want %#v", got, want)
	}
}
