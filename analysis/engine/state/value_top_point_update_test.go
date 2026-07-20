package state

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
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

func TestProductPatchBuilderPreservesNonValuesWriteWhenFiniteWriteIsDominatedByValuesTop(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	lane, ok := domain.ProductLane(LanePlacement)
	if !ok {
		t.Fatal("missing placement lane")
	}
	slot := key.SymbolValue(9802)
	plan, err := domain.SealProductPatch(
		[]ProductLane{lane}, []ProductLane{lane},
		[]key.Value{slot}, false, []key.Value{slot}, false, true,
	)
	if err != nil {
		t.Fatal(err)
	}
	id := identity.ID{Kind: "table", Site: "values-top-product-patch", Index: 1}
	base := Reachable(domain.Lattice().Bottom()).WritePlacement(id, placement.Stack)
	top := RecomposeValueLane(reg, domain.Lattice(), base, ValueLaneFactor{Top: true})
	carry, err := domain.DecomposeLanes(top, []ProductLane{lane})
	if err != nil {
		t.Fatal(err)
	}
	_, carryValues := DecomposeValueLane(domain.Lattice(), top)
	builder, err := plan.NewBuilder(carry, carryValues, true)
	if err != nil {
		t.Fatal(err)
	}
	fragment := top.WritePlacement(id, placement.OwnedHeap).WriteValue(reg, slot, product.Bottom(reg))
	if err := builder.WriteDeclaredFragment(fragment, true); err != nil {
		t.Fatal(err)
	}
	factors, values, reachable, err := builder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	residual, err := domain.ComposeSparse(factors)
	if err != nil {
		t.Fatal(err)
	}
	if !reachable || residual.ReadPlacement(id) != placement.OwnedHeap {
		t.Fatalf("non-Values effect was lost: reachable=%t placement=%v", reachable, residual.ReadPlacement(id))
	}
	if values.Top || len(values.Values) != 0 {
		t.Fatalf("dominated finite Values patch = %#v, want canonical empty patch", values)
	}
}
