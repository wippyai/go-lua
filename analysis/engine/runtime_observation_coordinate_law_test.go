package engine

import "testing"

// TestExactObservationAtAcceptsOnlyTheIssuingFactorRef proves the explicit
// observation-coordinate seam: a sealed Factor Ref can authorize an exact
// read when a routed member has no static exact-write coordinate, while a Ref
// from another sealed binding cannot cross the query's authority fence.
func TestExactObservationAtAcceptsOnlyTheIssuingFactorRef(t *testing.T) {
	fixture := newObservedReceiptQueryMatrixFixture(t, 1, nil, nil)
	observation := fixture.observations[0]
	implementation := fixture.queryImplementations[0]
	factor, factorOK := FactorImplementationAt[uint64, uint64](fixture.binding, fixture.factor)
	if !factorOK || factor == nil {
		t.Fatal("sealed observation Factor implementation")
	}
	ref, refOK := factor.Ref(0)
	if !refOK {
		t.Fatal("owner-issued exact observation Ref")
	}
	explicit, explicitOK := NewExactObservationAdmissionAt(
		implementation, ref, observation.ID, observation.Role, observation.Mount,
		observation.Point, observation.Occurrence, observation.Context,
	)
	if !explicitOK || !explicit.exactSurfaceOK {
		t.Fatal("owner-issued exact observation admission")
	}
	if _, failure, sealed := fixture.graph.Seal([]ProgramObservationAdmission{explicit}); !sealed || failure.Available() {
		t.Fatalf("owner-issued exact observation refused: sealed=%t failure=%v", sealed, failure)
	}

	foreignFixture := newObservedReceiptQueryMatrixFixture(t, 1, nil, nil)
	foreignFactor, foreignFactorOK := FactorImplementationAt[uint64, uint64](foreignFixture.binding, foreignFixture.factor)
	if !foreignFactorOK || foreignFactor == nil {
		t.Fatal("foreign Factor implementation")
	}
	foreignRef, foreignRefOK := foreignFactor.Ref(0)
	if !foreignRefOK {
		t.Fatal("foreign exact observation Ref")
	}
	foreign, foreignOK := NewExactObservationAdmissionAt(
		implementation, foreignRef, observation.ID, observation.Role, observation.Mount,
		observation.Point, observation.Occurrence, observation.Context,
	)
	if !foreignOK {
		t.Fatal("foreign Ref should remain an available candidate until binding")
	}
	if _, failure, sealed := fixture.graph.Seal([]ProgramObservationAdmission{foreign}); sealed || !failure.Available() {
		t.Fatal("foreign Factor Ref crossed the query authority fence")
	}
}
