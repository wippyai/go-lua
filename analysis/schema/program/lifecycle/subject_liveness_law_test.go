package lifecycle

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func subjectLivenessLawID(seed byte) identity.ContentID {
	var id identity.ContentID
	for index := range id {
		id[index] = seed + byte(index)
	}
	return id
}

func TestSubjectLivenessSpanIdentityIsRecomputedAtAdmission(t *testing.T) {
	subject := subjectLivenessLawID(97)
	id, idOK := SubjectLivenessSpanIdentity(SubjectLivenessCell, subject, 2, 5)
	if !idOK {
		t.Fatal("subject-liveness span identity")
	}
	row, rowOK := NewSubjectLivenessSpan(id, subject, SubjectLivenessCell, 2, 5, SubjectLivenessLive)
	if !rowOK || !row.Available() {
		t.Fatal("canonical subject-liveness span was not admitted")
	}
	if row.ID() != id || row.SubjectID() != subject || row.Lo() != 2 || row.Hi() != 5 {
		t.Fatal("subject-liveness span lost its canonical coordinates")
	}

	malformed := id
	malformed[0]++
	if forged, forgedOK := NewSubjectLivenessSpan(malformed, subject, SubjectLivenessCell, 2, 5, SubjectLivenessLive); forgedOK || forged.Available() {
		t.Fatal("subject-liveness span admitted an ID with a different preimage")
	}
	if changed, changedOK := NewSubjectLivenessSpan(id, subject, SubjectLivenessValue, 2, 5, SubjectLivenessLive); changedOK || changed.Available() {
		t.Fatal("subject-liveness span reused an ID across subject families")
	}
	// The range is part of the coordinate: a span that covers different
	// boundaries is a different fact, not the same fact restated.
	if moved, movedOK := NewSubjectLivenessSpan(id, subject, SubjectLivenessCell, 3, 5, SubjectLivenessLive); movedOK || moved.Available() {
		t.Fatal("subject-liveness span reused an ID across ranges")
	}
	widerID, widerOK := SubjectLivenessSpanIdentity(SubjectLivenessCell, subject, 2, 6)
	if !widerOK || widerID == id {
		t.Fatal("subject-liveness span identity did not commit its range")
	}
}

// An inverted range names no boundaries at all, so it is refused at the
// identity rather than admitted as an empty fact.
func TestSubjectLivenessSpanRejectsInvertedRange(t *testing.T) {
	subject := subjectLivenessLawID(98)
	if _, ok := SubjectLivenessSpanIdentity(SubjectLivenessRoot, subject, 5, 2); ok {
		t.Fatal("subject-liveness span identity admitted an inverted range")
	}
	id, idOK := SubjectLivenessSpanIdentity(SubjectLivenessRoot, subject, 5, 5)
	if !idOK {
		t.Fatal("subject-liveness span identity")
	}
	if row, ok := NewSubjectLivenessSpan(id, subject, SubjectLivenessRoot, 5, 2, SubjectLivenessUnknown); ok || row.Available() {
		t.Fatal("subject-liveness span admitted an inverted range")
	}
	single, singleOK := NewSubjectLivenessSpan(id, subject, SubjectLivenessRoot, 5, 5, SubjectLivenessUnknown)
	if !singleOK || !single.Available() || !single.Covers(5) || single.Covers(4) || single.Covers(6) {
		t.Fatal("a one-boundary span did not answer exactly its own boundary")
	}
}

func TestSubjectLivenessSpanIdentityExcludesState(t *testing.T) {
	subject := subjectLivenessLawID(99)
	id, idOK := SubjectLivenessSpanIdentity(SubjectLivenessValues, subject, 0, 3)
	if !idOK {
		t.Fatal("subject-liveness span identity")
	}
	live, liveOK := NewSubjectLivenessSpan(id, subject, SubjectLivenessValues, 0, 3, SubjectLivenessLive)
	dead, deadOK := NewSubjectLivenessSpan(id, subject, SubjectLivenessValues, 0, 3, SubjectLivenessDiesBefore)
	if !liveOK || !deadOK || live.ID() != dead.ID() || live.State() != SubjectLivenessLive || dead.State() != SubjectLivenessDiesBefore {
		t.Fatal("subject-liveness answer state changed its coordinate identity")
	}
}

func TestSubjectYieldBoundaryIdentityIsRecomputedAtAdmission(t *testing.T) {
	call := subjectLivenessLawID(129)
	route := subjectLivenessLawID(1)
	from := subjectLivenessLawID(33)
	to := subjectLivenessLawID(65)
	id, idOK := SubjectYieldBoundaryIdentity(call, route)
	if !idOK {
		t.Fatal("subject-yield-boundary identity")
	}
	row, rowOK := NewSubjectYieldBoundary(id, call, route, from, to, 7)
	if !rowOK || !row.Available() || row.Ordinal() != 7 || row.CallID() != call || row.YieldRouteID() != route {
		t.Fatal("canonical subject-yield-boundary row was not admitted")
	}

	malformed := id
	malformed[0]++
	if forged, forgedOK := NewSubjectYieldBoundary(malformed, call, route, from, to, 7); forgedOK || forged.Available() {
		t.Fatal("subject-yield-boundary admitted an ID with a different preimage")
	}
	otherCall := subjectLivenessLawID(161)
	otherID, otherOK := SubjectYieldBoundaryIdentity(otherCall, route)
	if !otherOK || otherID == id {
		t.Fatal("subject-yield-boundary identity did not commit its Call occurrence")
	}
	// The ordinal is a coordinate in a numbering, not part of the boundary's
	// identity: a boundary that moves because an unrelated body grew is the
	// same boundary.
	moved, movedOK := NewSubjectYieldBoundary(id, call, route, from, to, 9)
	if !movedOK || !moved.Available() || moved.ID() != row.ID() {
		t.Fatal("subject-yield-boundary identity committed its ordinal")
	}
}

func TestSubjectYieldBoundaryRequiresPairedEndpointPaths(t *testing.T) {
	call := subjectLivenessLawID(130)
	route := subjectLivenessLawID(2)
	id, idOK := SubjectYieldBoundaryIdentity(call, route)
	if !idOK {
		t.Fatal("subject-yield-boundary identity")
	}
	path := subjectLivenessLawID(34)
	if row, rowOK := NewSubjectYieldBoundary(id, call, route, path, identity.ContentID{}, 0); rowOK || row.Available() {
		t.Fatal("subject-yield-boundary admitted one-sided endpoint provenance")
	}
	if row, rowOK := NewSubjectYieldBoundary(id, call, route, path, path, 0); !rowOK || !row.Available() {
		t.Fatal("subject-yield-boundary rejected paired endpoint provenance")
	}
}
