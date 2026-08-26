package engine

import "testing"

// TestExactObservationAtAcceptsOnlyTheIssuingFactorRef proves the explicit
// observation-coordinate seam: a sealed Factor Ref can authorize an exact
// read when a routed member has no static exact-write coordinate, while a Ref
// from another sealed binding cannot cross the query's authority fence.
//
// It also states the closed set that seam draws from. A Factor's exact read
// catalog is the coordinates its owner can issue a read of - every cell a
// strong unrouted write names, and every cell of the sealed universe where a
// member publishes through a route - so a Ref for a cell this Factor neither
// writes nor routes into carries no read unit and binds nothing. That set was
// previously whatever the declarations happened to spell, which is how a Ref
// for an unwritten cell used to bind whenever some query had named it.
func TestExactObservationAtAcceptsOnlyTheIssuingFactorRef(t *testing.T) {
	fixture := newObservedReceiptQueryMatrixFixture(t, 1, nil, nil)
	observation := fixture.observations[0]
	implementation := fixture.queryImplementations[0]
	factor, factorOK := FactorImplementationAt[uint64, uint64](fixture.binding, fixture.factor)
	if !factorOK || factor == nil {
		t.Fatal("sealed observation Factor implementation")
	}
	// The Ref names a coordinate this Factor's owner can issue a read of: the
	// closed set is every cell a strong unrouted write names, which for this
	// matrix is the one its rule writes. A Ref for a cell no member of the
	// Factor writes and no route resolves is not in that set, and the catalog
	// carries no read unit for it.
	ref, refOK := factor.Ref(1)
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
	foreignRef, foreignRefOK := foreignFactor.Ref(1)
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

	// This Factor's rule writes one cell and routes nothing, so the cell
	// before it is not one its owner can issue a read of.
	unissued, unissuedRefOK := factor.Ref(0)
	if !unissuedRefOK {
		t.Fatal("owner Ref for an unwritten coordinate")
	}
	unissuedAdmission, unissuedAdmissionOK := NewExactObservationAdmissionAt(
		implementation, unissued, observation.ID, observation.Role, observation.Mount,
		observation.Point, observation.Occurrence, observation.Context,
	)
	if !unissuedAdmissionOK {
		t.Fatal("unissued coordinate should remain an available candidate until binding")
	}
	if _, failure, sealed := fixture.graph.Seal([]ProgramObservationAdmission{unissuedAdmission}); sealed || !failure.Available() {
		t.Fatal("a coordinate outside the owner-issuable set was bound")
	}
}
