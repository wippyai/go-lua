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
	catalog := valuedomain.AxisMemberCatalog()
	candidateRelation, candidateRelationOK := catalog.RelationOrdinal(valuedomain.StorageTransferCandidates)
	sourceRelation, sourceRelationOK := catalog.RelationOrdinal(valuedomain.StorageTransferSources)
	targetProjection, targetProjectionOK := catalog.ProjectionOrdinal(valuedomain.StorageTransferTarget)
	sourceProjection, sourceProjectionOK := catalog.ProjectionOrdinal(valuedomain.StorageTransferSourceKey)
	if !candidateRelationOK || !sourceRelationOK || !targetProjectionOK || !sourceProjectionOK {
		t.Fatal("storage transfer member ordinals")
	}
	candidate, candidateOK := relations.CandidateAt(candidateRelation, mount, occurrence, 0)
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
	gotFrom, gotFromOK := relations.Project(sourceRelation, sourceProjection, candidate)
	gotTo, gotToOK := relations.Project(candidateRelation, targetProjection, candidate)
	if !gotFromOK || gotFrom != wantFrom || !gotToOK || gotTo != wantTo {
		t.Fatalf("projected endpoints from=%d/%t want=%d; to=%d/%t want=%d", gotFrom, gotFromOK, wantFrom, gotTo, gotToOK, wantTo)
	}

	if _, ok := relations.CandidateAt(candidateRelation, identity.ContentID{}, occurrence, 0); ok {
		t.Fatal("unavailable mount admitted")
	}
	if _, ok := relations.CandidateAt(candidateRelation, mount, identity.ContentID{}, 0); ok {
		t.Fatal("unavailable occurrence admitted")
	}
	if _, ok := relations.CandidateAt(sourceRelation, mount, occurrence, 0); ok {
		t.Fatal("derived relation exposed a candidate directory")
	}
	if _, ok := relations.CandidateAt(99, mount, occurrence, 0); ok {
		t.Fatal("out-of-range relation admitted")
	}
	if _, ok := relations.CandidateAt(candidateRelation, foreignMount, foreignOccurrence, 0); ok {
		t.Fatal("foreign occurrence crossed the local relation owner")
	}
	if _, ok := relations.Project(99, 1, candidate); ok {
		t.Fatal("out-of-range relation projected")
	}
	if _, ok := relations.Project(candidateRelation, 99, candidate); ok {
		t.Fatal("out-of-range projection projected")
	}
	if _, ok := relations.Project(candidateRelation, targetProjection, uint32(local.StorageTransferCount())); ok {
		t.Fatal("out-of-range candidate projected")
	}
}
