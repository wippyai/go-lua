package state

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

func TestValuesTopAbsorbsFinitePointWriteAndUpdate(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	id := identity.ID{Kind: "table", Site: "values-top-point-update", Index: 1}
	base := Reachable(domain.Lattice().Bottom()).WritePlacement(id, placement.Stack)
	top := RecomposeValueLane(reg, domain.Lattice(), base, ValueLaneFactor{Top: true})
	slot := key.SymbolValue(9801)

	if got := top.WriteValue(reg, slot, product.Bottom(reg)); !domain.Lattice().Equal(got, top) {
		t.Fatal("finite WriteValue changed lifted-map Top")
	}
	called := false
	got := top.UpdateValue(reg, slot, func(product.Value) product.Value {
		called = true
		return product.Bottom(reg)
	})
	if called || !domain.Lattice().Equal(got, top) {
		t.Fatalf("finite UpdateValue over Top called transformer=%t or changed state", called)
	}
	if got.ReadPlacement(id) != placement.Stack {
		t.Fatal("absorbed Values update changed another product lane")
	}
}
