package transfer

import "testing"

// TestTransferSparseActualBottomIsTheFactorDefault states the Value Factor
// default law for the Target-transfer planner. The owner's exact sparse
// Bottom carries no allocation evidence, so it contributes no transfer demand
// and leaves a valid no-route plan. Sparse non-Bottom remains malformed.
func TestTransferSparseActualBottomIsTheFactorDefault(t *testing.T) {
	fixture := newTransferHotLawFixture(t, true, "transfer-sparse-bottom")
	present, presentOK := planFor(fixture.packs, fixture.calls, fixture.placement, fixture.values, fixture.contract, fixture.mounted, fixture.callFact, fixture.observations)
	if !presentOK || present.routeCount() != 1 {
		t.Fatalf("present transfer plan = %t/%d, want one exact route", presentOK, present.routeCount())
	}
	payload := -1
	for index, observation := range fixture.observations {
		if !fixture.values.Equal(observation.fact, fixture.values.Bottom()) {
			payload = index
			break
		}
	}
	if payload < 0 {
		t.Fatal("fixture has no rooted actual to sparsify")
	}
	sparse := append([]actualObservation(nil), fixture.observations...)
	sparse[payload] = actualObservation{fact: fixture.values.Bottom(), valid: true}
	plan, planOK := planFor(fixture.packs, fixture.calls, fixture.placement, fixture.values, fixture.contract, fixture.mounted, fixture.callFact, sparse)
	if !planOK || plan.routeCount() != 0 {
		t.Fatalf("owner-issued sparse Bottom transfer plan = %t/%d, want valid no-route", planOK, plan.routeCount())
	}
	forged := append([]actualObservation(nil), fixture.observations...)
	forged[payload].present = false
	if _, forgedOK := planFor(fixture.packs, fixture.calls, fixture.placement, fixture.values, fixture.contract, fixture.mounted, fixture.callFact, forged); forgedOK {
		t.Fatal("sparse non-Bottom Value cell admitted as the Value Factor default")
	}
}
