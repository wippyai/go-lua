package pack_test

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	memberrelation "github.com/wippyai/go-lua/analysis/schema/axis/member/relation"
	packdomain "github.com/wippyai/go-lua/domain/pack"
)

func TestGeneratedRelationOwnerProjectsSource(t *testing.T) {
	local := sealedPackSchema(t, "pack-relation-owner-local", "return 1\n")
	foreign := sealedPackSchema(t, "pack-relation-owner-foreign", "return 2\n")
	if local.SourceCount() == 0 || foreign.SourceCount() == 0 {
		t.Fatal("source directory is empty")
	}
	source, sourceOK := local.SourceAt(0)
	foreignSource, foreignSourceOK := foreign.SourceAt(0)
	if !sourceOK || !foreignSourceOK {
		t.Fatal("source at 0")
	}
	mount, occurrence, occurrenceOK := source.Occurrence()
	foreignMount, foreignOccurrence, foreignOccurrenceOK := foreignSource.Occurrence()
	if !occurrenceOK || !foreignOccurrenceOK {
		t.Fatal("source occurrence")
	}

	owner := packdomain.NewRelationOwner(local)
	var relations memberrelation.Owner = owner
	candidate, candidateOK := relations.Candidate(0, mount, occurrence)
	wantCandidate, wantCandidateOK := local.SourceOrdinal(source)
	if !candidateOK || !wantCandidateOK || candidate != wantCandidate {
		t.Fatalf("candidate ordinal=%d/%t, want=%d/%t", candidate, candidateOK, wantCandidate, wantCandidateOK)
	}

	root, _, resultOK := source.Result()
	wantRoot, wantRootOK := local.RootIndex(root)
	gotRoot, gotRootOK := relations.Project(0, 0, candidate)
	if !resultOK || !wantRootOK || !gotRootOK || gotRoot != wantRoot {
		t.Fatalf("projected root=%d/%t want=%d/%t", gotRoot, gotRootOK, wantRoot, wantRootOK)
	}

	columnOwner, columnOK := owner.SourceFactColumn(0)
	wantFact, wantOutcome := packdomain.SourceFact(source)
	fact, outcome, factOK := columnOwner.At(candidate)
	if !columnOK || columnOwner.Count() != local.SourceCount() || !factOK || outcome != wantOutcome || wantOutcome != structure.Concrete || local.Fingerprint(fact) != local.Fingerprint(wantFact) {
		t.Fatal("source fact")
	}
	if _, ok := owner.SourceFactColumn(99); ok {
		t.Fatal("out-of-range source relation exposed a column")
	}
	if _, _, ok := columnOwner.At(uint32(columnOwner.Count())); ok {
		t.Fatal("out-of-range source candidate indexed a column")
	}

	if _, ok := relations.Candidate(0, identity.ContentID{}, occurrence); ok {
		t.Fatal("unavailable mount admitted")
	}
	if _, ok := relations.Candidate(0, mount, identity.ContentID{}); ok {
		t.Fatal("unavailable occurrence admitted")
	}
	if _, ok := relations.Candidate(99, mount, occurrence); ok {
		t.Fatal("out-of-range relation admitted")
	}
	if _, ok := relations.Candidate(0, foreignMount, foreignOccurrence); ok {
		t.Fatal("foreign occurrence crossed the local relation owner")
	}
	if _, ok := relations.Project(0, 99, candidate); ok {
		t.Fatal("out-of-range projection projected")
	}
	if _, ok := relations.Project(0, 0, uint32(local.SourceCount())); ok {
		t.Fatal("out-of-range candidate projected")
	}
}
