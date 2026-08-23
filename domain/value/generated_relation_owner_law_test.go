package value_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	memberrelation "github.com/wippyai/go-lua/analysis/schema/axis/member/relation"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

func TestGeneratedRelationOwnerProjectsStorageTransfer(t *testing.T) {
	local := sealedStorageTransferSchema(t, "relation owner local")
	foreign := sealedStorageTransferSchema(t, "relation owner foreign")
	transfer, transferOK := local.StorageTransferAt(0)
	foreignTransfer, foreignTransferOK := foreign.StorageTransferAt(0)
	if !transferOK || !foreignTransferOK {
		t.Fatal("storage transfer directory is empty")
	}
	mount, occurrence, occurrenceOK := transfer.Occurrence()
	foreignMount, foreignOccurrence, foreignOccurrenceOK := foreignTransfer.Occurrence()
	if !occurrenceOK || !foreignOccurrenceOK {
		t.Fatal("storage transfer occurrence")
	}

	owner := valuedomain.NewRelationOwner(local)
	var relations memberrelation.Owner = owner
	candidate, candidateOK := relations.Candidate(0, mount, occurrence)
	wantCandidate, wantCandidateOK := local.StorageTransferOrdinal(transfer)
	if !candidateOK || !wantCandidateOK || candidate != wantCandidate {
		t.Fatalf("candidate ordinal=%d/%t, want=%d/%t", candidate, candidateOK, wantCandidate, wantCandidateOK)
	}

	from, to, endpointsOK := transfer.Endpoints()
	wantFrom, wantFromOK := local.CoordinateIndex(from)
	wantTo, wantToOK := local.CoordinateIndex(to)
	if !endpointsOK || !wantFromOK || !wantToOK {
		t.Fatal("storage transfer endpoints")
	}
	gotFrom, gotFromOK := relations.Project(1, 0, candidate)
	gotTo, gotToOK := relations.Project(0, 1, candidate)
	if !gotFromOK || gotFrom != wantFrom || !gotToOK || gotTo != wantTo {
		t.Fatalf("projected endpoints from=%d/%t want=%d; to=%d/%t want=%d", gotFrom, gotFromOK, wantFrom, gotTo, gotToOK, wantTo)
	}

	if _, ok := relations.Candidate(0, identity.ContentID{}, occurrence); ok {
		t.Fatal("unavailable mount admitted")
	}
	if _, ok := relations.Candidate(0, mount, identity.ContentID{}); ok {
		t.Fatal("unavailable occurrence admitted")
	}
	if _, ok := relations.Candidate(1, mount, occurrence); ok {
		t.Fatal("derived relation exposed a candidate directory")
	}
	if _, ok := relations.Candidate(99, mount, occurrence); ok {
		t.Fatal("out-of-range relation admitted")
	}
	if _, ok := relations.Candidate(0, foreignMount, foreignOccurrence); ok {
		t.Fatal("foreign occurrence crossed the local relation owner")
	}
	if _, ok := relations.Project(99, 1, candidate); ok {
		t.Fatal("out-of-range relation projected")
	}
	if _, ok := relations.Project(0, 99, candidate); ok {
		t.Fatal("out-of-range projection projected")
	}
	if _, ok := relations.Project(0, 1, uint32(local.StorageTransferCount())); ok {
		t.Fatal("out-of-range candidate projected")
	}
}
