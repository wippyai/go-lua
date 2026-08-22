package formal

import (
	"testing"

	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/materialization"
)

// TestFormalSparseActualBottomIsTheFactorDefault proves that the formal
// planner treats the Value owner's sparse Bottom as an authenticated empty
// allocation image, never as missing evidence or an all-root request.
func TestFormalSparseActualBottomIsTheFactorDefault(t *testing.T) {
	fixture := newOpaqueDispatchLawFixture(t, "formal-sparse-bottom")
	keys := routePlanAllocationKeys(t, fixture.placement)
	if len(keys) == 0 || len(fixture.observations) == 0 {
		t.Fatal("formal sparse-Bottom fixture has no rooted actual")
	}
	atom, atomOK := fixture.values.Allocation(keys[0], materialization.Recent)
	fact, factOK := fixture.values.Singleton(atom)
	if !atomOK || !factOK {
		t.Fatal("formal sparse-Bottom allocation fact")
	}
	fixture.observations[0] = actualObservation{fact: fact, present: true, valid: true}
	target := findFormalTargetForOpaqueLaw(t, fixture)
	callFact, callFactOK := fixture.calls.DispatchValue(fixture.key, []calldomain.Target{target}, false)
	if !callFactOK {
		t.Fatal("formal sparse-Bottom exact dispatch fact")
	}
	present, presentOK := planFor(fixture.packs, fixture.calls, fixture.placement, fixture.values, fixture.contract, fixture.mounted, callFact, fixture.observations)
	if !presentOK || present.routeCount() == 0 {
		t.Fatalf("present formal plan = %t/%d, want exact route", presentOK, present.routeCount())
	}
	sparse := append([]actualObservation(nil), fixture.observations...)
	sparse[0] = actualObservation{fact: fixture.values.Bottom(), valid: true}
	plan, planOK := planFor(fixture.packs, fixture.calls, fixture.placement, fixture.values, fixture.contract, fixture.mounted, callFact, sparse)
	if !planOK || plan.routeCount() != 0 {
		t.Fatalf("owner-issued sparse Bottom formal plan = %t/%d, want valid no-route", planOK, plan.routeCount())
	}
	forged := append([]actualObservation(nil), fixture.observations...)
	forged[0].present = false
	if _, forgedOK := planFor(fixture.packs, fixture.calls, fixture.placement, fixture.values, fixture.contract, fixture.mounted, callFact, forged); forgedOK {
		t.Fatal("sparse non-Bottom Value cell admitted as the Value Factor default")
	}
}
