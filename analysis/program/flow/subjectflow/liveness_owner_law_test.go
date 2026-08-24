package subjectflow

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestSubjectsForOwnerRefusesMissingOwnerDirectory(t *testing.T) {
	owner := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	index := &livenessIndex{subjectsByOwner: make(map[keyspace.Term]map[subjectKey]Subject)}

	if subjects, ok := subjectsForOwner(index, owner); ok || subjects != nil {
		t.Fatalf("missing owner directory accepted: subjects=%v ok=%t", subjects, ok)
	}
}

// A body with no local subject beyond its own root bookkeeping publishes an
// owner directory that is present but empty: buildLivenessIndex still
// creates the map entry for that owner, but no event ever added a member to
// it. That is proof the owner was reached, not proof of malformed evidence,
// so it must be admitted with a zero-length, non-nil member set rather than
// refused as if the directory had never been published.
func TestSubjectsForOwnerAdmitsExplicitlyEmptyOwnerDirectory(t *testing.T) {
	owner := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	index := &livenessIndex{subjectsByOwner: map[keyspace.Term]map[subjectKey]Subject{
		owner: make(map[subjectKey]Subject),
	}}

	subjects, ok := subjectsForOwner(index, owner)
	if !ok || len(subjects) != 0 {
		t.Fatalf("explicit-empty owner directory refused: subjects=%v ok=%t", subjects, ok)
	}
}

func TestSubjectsForOwnerReturnsOnlyPublishedSubjectsInCanonicalOrder(t *testing.T) {
	owner := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	left := Subject{Kind: SubjectRoot, ID: sha256.Sum256([]byte("left")), Term: owner}
	right := Subject{Kind: SubjectValue, ID: sha256.Sum256([]byte("right")), Term: keyspace.MakeTerm(keyspace.FamilyTable, 1)}
	index := &livenessIndex{subjectsByOwner: map[keyspace.Term]map[subjectKey]Subject{
		owner: {
			makeSubjectKey(right): right,
			makeSubjectKey(left):  left,
		},
	}}

	subjects, ok := subjectsForOwner(index, owner)
	if !ok || len(subjects) != 2 {
		t.Fatalf("published subjects = %v, %t", subjects, ok)
	}
	if subjects[0] != left || subjects[1] != right {
		t.Fatalf("published order = %#v, want root then value", subjects)
	}
}
