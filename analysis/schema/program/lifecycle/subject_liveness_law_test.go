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

func TestSubjectLivenessIdentityIsRecomputedAtAdmission(t *testing.T) {
	call := subjectLivenessLawID(129)
	route := subjectLivenessLawID(1)
	from := subjectLivenessLawID(33)
	to := subjectLivenessLawID(65)
	subject := subjectLivenessLawID(97)
	id, idOK := SubjectLivenessIdentity(call, route, SubjectLivenessCell, subject)
	if !idOK {
		t.Fatal("subject-liveness identity")
	}
	row, rowOK := NewSubjectLiveness(id, call, route, from, to, subject, SubjectLivenessCell, SubjectLivenessLive)
	if !rowOK || !row.Available() {
		t.Fatal("canonical subject-liveness row was not admitted")
	}
	if row.ID() != id || row.CallID() != call || row.YieldRouteID() != route || row.SubjectID() != subject {
		t.Fatal("subject-liveness row lost its canonical coordinates")
	}

	malformed := id
	malformed[0]++
	if forged, forgedOK := NewSubjectLiveness(malformed, call, route, from, to, subject, SubjectLivenessCell, SubjectLivenessLive); forgedOK || forged.Available() {
		t.Fatal("subject-liveness row admitted an ID with a different preimage")
	}
	if changed, changedOK := NewSubjectLiveness(id, call, route, from, to, subject, SubjectLivenessValue, SubjectLivenessLive); changedOK || changed.Available() {
		t.Fatal("subject-liveness row reused an ID across subject families")
	}
	otherCall := subjectLivenessLawID(161)
	otherID, otherOK := SubjectLivenessIdentity(otherCall, route, SubjectLivenessCell, subject)
	if !otherOK || otherID == id {
		t.Fatal("subject-liveness identity did not commit its Call occurrence")
	}
	if changed, changedOK := NewSubjectLiveness(id, otherCall, route, from, to, subject, SubjectLivenessCell, SubjectLivenessLive); changedOK || changed.Available() {
		t.Fatal("subject-liveness row reused an ID across Call occurrences")
	}
}

func TestSubjectLivenessRequiresPairedBoundaryPaths(t *testing.T) {
	call := subjectLivenessLawID(130)
	route := subjectLivenessLawID(2)
	subject := subjectLivenessLawID(98)
	id, idOK := SubjectLivenessIdentity(call, route, SubjectLivenessRoot, subject)
	if !idOK {
		t.Fatal("subject-liveness identity")
	}
	path := subjectLivenessLawID(34)
	if row, rowOK := NewSubjectLiveness(id, call, route, path, identity.ContentID{}, subject, SubjectLivenessRoot, SubjectLivenessUnknown); rowOK || row.Available() {
		t.Fatal("subject-liveness row admitted one-sided endpoint provenance")
	}
	if row, rowOK := NewSubjectLiveness(id, call, route, path, path, subject, SubjectLivenessRoot, SubjectLivenessUnknown); !rowOK || !row.Available() {
		t.Fatal("subject-liveness row rejected paired endpoint provenance")
	}
}

func TestSubjectLivenessIdentityExcludesState(t *testing.T) {
	call := subjectLivenessLawID(131)
	route := subjectLivenessLawID(3)
	subject := subjectLivenessLawID(99)
	id, idOK := SubjectLivenessIdentity(call, route, SubjectLivenessValues, subject)
	if !idOK {
		t.Fatal("subject-liveness identity")
	}
	live, liveOK := NewSubjectLiveness(id, call, route, identity.ContentID{}, identity.ContentID{}, subject, SubjectLivenessValues, SubjectLivenessLive)
	dead, deadOK := NewSubjectLiveness(id, call, route, identity.ContentID{}, identity.ContentID{}, subject, SubjectLivenessValues, SubjectLivenessDiesBefore)
	if !liveOK || !deadOK || live.ID() != dead.ID() || live.State() != SubjectLivenessLive || dead.State() != SubjectLivenessDiesBefore {
		t.Fatal("subject-liveness answer state changed its coordinate identity")
	}
}
