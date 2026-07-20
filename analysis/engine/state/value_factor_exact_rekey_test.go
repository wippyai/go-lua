package state

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestExactValueFactorRekeyIsInjectiveTotalAndAllocationFreeAtBounds(t *testing.T) {
	domain := RegisteredProductDomain(standard.Registry())
	plan, err := SealExactValueFactorRekey(domain, []ExactValueSlotBinding[string, int]{
		{Source: "left", Target: 1}, {Source: "right", Target: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		got, applyErr := plan.Apply(ValueFactor[string]{})
		if applyErr != nil || got.Top || len(got.Values) != 0 {
			panic("Bottom exact Values rekey changed")
		}
	}); allocs != 0 {
		t.Fatalf("Bottom exact Values rekey allocations = %g", allocs)
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		got, applyErr := plan.Apply(ValueFactor[string]{Top: true})
		if applyErr != nil || !got.Top || len(got.Values) != 0 {
			panic("Top exact Values rekey changed")
		}
	}); allocs != 0 {
		t.Fatalf("Top exact Values rekey allocations = %g", allocs)
	}

	value := product.Top()
	got, err := plan.Apply(ValueFactor[string]{Values: map[string]product.Value{"left": value, "right": value}})
	if err != nil || len(got.Values) != 2 || !product.Equal(domain.Registry(), got.Values[1], value) || !product.Equal(domain.Registry(), got.Values[2], value) {
		t.Fatalf("exact Values rekey = %#v, %v", got, err)
	}
	if _, err := plan.Apply(ValueFactor[string]{Values: map[string]product.Value{"unbound": value}}); err == nil {
		t.Fatal("exact Values rekey dropped an unbound live coordinate")
	}
	foreign, err := standard.RegistryWithAxes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Apply(ValueFactor[string]{Values: map[string]product.Value{"left": presentValue(foreign)}}); err == nil {
		t.Fatal("exact Values rekey accepted a foreign product")
	}
	if _, err := SealExactValueFactorRekey(domain, []ExactValueSlotBinding[string, int]{
		{Source: "left", Target: 1}, {Source: "right", Target: 1},
	}); err == nil {
		t.Fatal("exact Values rekey accepted a non-injective target")
	}
}
