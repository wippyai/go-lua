package state

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestApplyHeapBoundaryReusesSemanticallyUnchangedDestination(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	heapLane, ok := domain.ProductLane(LaneHeapTableIdentity)
	if !ok {
		t.Fatal("heap lane missing")
	}
	keys := keyspace.New()
	first := identity.ID{Kind: "table", Site: "boundary-reuse", Index: 1}
	second := identity.ID{Kind: "table", Site: "boundary-reuse", Index: 2}
	firstObject := heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: product.Top()})
	secondObject := heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: product.Bottom(reg)})

	stateWith := func(entries map[identity.ID]heapidentity.TableObject) State {
		out := domain.Lattice().Bottom()
		for id, object := range entries {
			out = out.WriteHeapTableObject(reg, id, object)
		}
		return domain.Normalize(out)
	}
	laneFactor := func(value State) LaneFactor {
		factors, err := domain.Decompose(value)
		if err != nil {
			t.Fatal(err)
		}
		return factors[int(heapLane.Ordinal())]
	}
	apply := func(destination, fragment State, closure BoundaryClosure) State {
		out := destination
		if !applyHeapBoundary(&boundaryApplyContext{reg: reg, keys: keys, closure: closure}, destination, fragment, &out) {
			t.Fatal("heap boundary apply failed")
		}
		out.canonical = true
		return out
	}
	assertSame := func(name string, destination, got State, want bool) {
		t.Helper()
		same, err := domain.LaneSame(laneFactor(destination), laneFactor(got))
		if err != nil {
			t.Fatal(err)
		}
		if same != want {
			t.Fatalf("%s LaneSame = %v, want %v", name, same, want)
		}
	}

	destination := stateWith(map[identity.ID]heapidentity.TableObject{first: firstObject, second: secondObject})
	selected := emptyBoundaryClosure()
	selected.identities[identity.ConcreteTerm(first)] = struct{}{}
	unchanged := apply(destination, stateWith(map[identity.ID]heapidentity.TableObject{first: firstObject}), selected)
	assertSame("unchanged finite closure", destination, unchanged, true)

	changedObject := heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: product.Top(), StableShape: true})
	changed := apply(destination, stateWith(map[identity.ID]heapidentity.TableObject{first: changedObject}), selected)
	assertSame("changed replacement", destination, changed, false)

	deleted := apply(destination, stateWith(nil), selected)
	assertSame("deletion", destination, deleted, false)

	third := identity.ID{Kind: "table", Site: "boundary-reuse", Index: 3}
	additionClosure := emptyBoundaryClosure()
	additionClosure.identities[identity.ConcreteTerm(third)] = struct{}{}
	added := apply(destination, stateWith(map[identity.ID]heapidentity.TableObject{third: firstObject}), additionClosure)
	assertSame("addition", destination, added, false)

	all := emptyBoundaryClosure()
	all.allIdentities = true
	fullFragment := stateWith(map[identity.ID]heapidentity.TableObject{first: firstObject, second: secondObject})
	allUnchanged := apply(destination, fullFragment, all)
	assertSame("all-identities unchanged", destination, allUnchanged, true)

	top := destination
	top.heapTableIdentity = heapTableIdentityLane{top: true}
	topApplied := apply(top, stateWith(nil), selected)
	assertSame("top destination", top, topApplied, true)
}
