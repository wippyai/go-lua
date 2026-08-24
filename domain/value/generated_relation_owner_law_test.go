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

func TestGeneratedRelationOwnerProjectsMountedCallGeometry(t *testing.T) {
	fixture := buildMountedCallArgumentFixture(t, "local function f(a, b) return a end\nf(1, 2)\n")
	values := fixture.values
	parent, parentOK := values.MountedCallActualsAt(0)
	module, moduleOK := parent.Module()
	call, callOK := parent.CallID()
	if !parentOK || !moduleOK || !callOK {
		t.Fatal("mounted-call parent fixture")
	}

	owner := valuedomain.NewRelationOwner(values)
	var relations memberrelation.Owner = owner
	catalog := valuedomain.AxisMemberCatalog()
	parentRelation, parentRelationOK := catalog.RelationOrdinal(valuedomain.MountedCallParents)
	memberRelation, memberRelationOK := catalog.RelationOrdinal(valuedomain.MountedCallActualMembers)
	calleeProjection, calleeProjectionOK := catalog.ProjectionOrdinal(valuedomain.MountedCallCalleeKey)
	actualProjection, actualProjectionOK := catalog.ProjectionOrdinal(valuedomain.MountedCallActualKey)
	tagProjection, tagProjectionOK := catalog.ProjectionOrdinal(valuedomain.MountedCallActualTag)
	if !parentRelationOK || !memberRelationOK || !calleeProjectionOK || !actualProjectionOK || !tagProjectionOK {
		t.Fatal("mounted-call member ordinals")
	}

	parentCandidate, candidateOK := relations.CandidateAt(parentRelation, module, call, 0)
	wantParent, wantParentOK := values.MountedCallActualsOrdinal(parent)
	if !candidateOK || !wantParentOK || parentCandidate != wantParent {
		t.Fatalf("parent candidate=%d/%t, want=%d/%t", parentCandidate, candidateOK, wantParent, wantParentOK)
	}
	callee, calleeOK := parent.CalleeCoordinate()
	wantCallee, wantCalleeOK := values.CoordinateIndex(callee)
	gotCallee, gotCalleeOK := relations.Project(parentRelation, calleeProjection, parentCandidate)
	if !calleeOK || !wantCalleeOK || !gotCalleeOK || gotCallee != wantCallee {
		t.Fatalf("callee projection=%d/%t, want=%d/%t", gotCallee, gotCalleeOK, wantCallee, wantCalleeOK)
	}

	count, countOK := relations.MemberCount(memberRelation, parentCandidate)
	if !countOK || count != parent.MemberCount() || count == 0 {
		t.Fatalf("actual member count=%d/%t, want nonzero %d", count, countOK, parent.MemberCount())
	}
	for ordinal := 0; ordinal < count; ordinal++ {
		memberCandidate, memberOK := relations.MemberAt(memberRelation, parentCandidate, ordinal)
		member, directOK := parent.MemberAt(ordinal)
		coordinate, coordinateOK := member.Coordinate()
		wantCoordinate, wantCoordinateOK := values.CoordinateIndex(coordinate)
		tag, tagOK := member.ActualTag()
		gotCoordinate, gotCoordinateOK := relations.Project(memberRelation, actualProjection, memberCandidate)
		gotTag, gotTagOK := relations.Project(memberRelation, tagProjection, memberCandidate)
		if !memberOK || !directOK || !coordinateOK || !wantCoordinateOK || !tagOK ||
			!gotCoordinateOK || gotCoordinate != wantCoordinate || !gotTagOK || uint64(gotTag) != tag {
			t.Fatalf("actual %d projection coordinate=%d/%t want=%d/%t tag=%d/%t want=%d/%t", ordinal, gotCoordinate, gotCoordinateOK, wantCoordinate, wantCoordinateOK, gotTag, gotTagOK, tag, tagOK)
		}
	}
	if _, ok := relations.CandidateAt(parentRelation, module, identity.ContentID{}, 0); ok {
		t.Fatal("missing call occurrence admitted")
	}
	if _, ok := relations.MemberAt(memberRelation, parentCandidate, count); ok {
		t.Fatal("member beyond the parent census admitted")
	}
	if _, ok := relations.Project(parentRelation, actualProjection, parentCandidate); ok {
		t.Fatal("actual projection admitted on the parent relation")
	}
}
