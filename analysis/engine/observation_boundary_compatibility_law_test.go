package engine

import "testing"

// TestObservationAdmissionRequiresOneCanonicalReadCoordinate states the
// positive boundary (the ordinary member point is used when no read lane is
// present) and its nearest negative: once a read coordinate is declared, it
// must be exactly the canonical Point carried by the row.
func TestObservationAdmissionRequiresOneCanonicalReadCoordinate(t *testing.T) {
	fixture := newObservedReceiptQueryMatrixFixture(t, 1, nil, nil)
	if len(fixture.observations) != 1 {
		t.Fatal("observation fixture did not issue one row")
	}
	if _, failure, sealed := fixture.graph.Seal([]ProgramObservationAdmission{fixture.observations[0]}); !sealed || failure.Available() {
		t.Fatalf("canonical observation rejected: sealed=%t failure=%v", sealed, failure)
	}

	foreign := fixture.observations[0]
	foreign.readPoint = programMatrixID(999)
	if _, failure, sealed := fixture.graph.Seal([]ProgramObservationAdmission{foreign}); sealed || !failure.Available() {
		t.Fatalf("observation accepted a non-equal read coordinate: sealed=%t failure=%v", sealed, failure)
	}
}
