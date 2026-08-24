package value

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

// TestValuePublishesEveryReturnBoundaryColumnAReturnJudgmentAddresses is the
// owner obligation a return-escape judgment reads against.
//
// Return escape is a Placement judgment over a Value distinction: which
// coordinate one executable return anchors at, and which coordinates its
// ordered fixed members occupy. Value is the earliest owner that holds those
// rows, so Value publishes them. A child that has to rediscover them is
// reading a column its parent never issued, and the topology it rebuilds is a
// second authority over the one Value already sealed.
//
// The law applies the conditions the rule plan compiler applies to the three
// addresses such a rule names - a candidate directory, an exact root join
// hanging off it, and a selected join over the member set - so a projection
// that drifts off its relation, loses its Key role, or stops naming the
// return-boundary directory as its provider is refused here rather than at
// the seal of every rule that reads it.
func TestValuePublishesEveryReturnBoundaryColumnAReturnJudgmentAddresses(t *testing.T) {
	catalog := AxisMemberCatalog()

	candidates, candidatesOK := catalog.Relation(ReturnBoundaryCandidates)
	if !candidatesOK || candidates.Subject != ReturnBoundaryCarrier ||
		candidates.CandidateProvider.AxisRelation.Member != ReturnBoundaryCandidates {
		t.Fatalf("value publishes no self-provided return-boundary candidate directory: %#v", candidates)
	}

	roots, rootsOK := catalog.Relation(ReturnBoundaryRoots)
	if !rootsOK || roots.CandidateProvider != candidates.CandidateProvider {
		t.Fatalf("the return-boundary root relation does not hang off the candidate directory: %#v", roots)
	}
	rootKey, rootKeyOK := catalog.Projection(ReturnBoundaryRootKey)
	if !rootKeyOK || rootKey.Role != member.Key || rootKey.Relation != roots.Key ||
		rootKey.Result != ValueCoordinateCarrier || rootKey.CandidateProvider != roots.CandidateProvider {
		t.Fatalf("the root key is not a Key projection of the root relation: %#v", rootKey)
	}

	// The member set is self-provided so a member is densified through its own
	// directory. That is what lets one row be addressed by (return, ordinal)
	// and still project its coordinate the way every other row does.
	members, membersOK := catalog.Relation(ReturnBoundaryMembers)
	if !membersOK || members.Subject != ReturnBoundaryMemberCarrier ||
		members.CandidateProvider.AxisRelation.Member != ReturnBoundaryMembers {
		t.Fatalf("value publishes no self-provided return-boundary member relation: %#v", members)
	}
	memberKey, memberKeyOK := catalog.Projection(ReturnBoundaryMemberKey)
	if !memberKeyOK || memberKey.Role != member.Key || memberKey.Relation != members.Key ||
		memberKey.Result != ValueCoordinateCarrier || memberKey.CandidateProvider != members.CandidateProvider {
		t.Fatalf("the member key is not a Key projection of the member relation: %#v", memberKey)
	}
}

// returnBoundaryCatalogFixture seals one Schema carrying two return
// boundaries over one shared member arena: the first closed with one member,
// the second open-tailed with two. It is built directly because the laws
// below are about the directory Value publishes over its sealed rows, not
// about which Program shapes produce them.
func returnBoundaryCatalogFixture(t *testing.T) (*Schema, [2]computationKey) {
	t.Helper()
	module := returnBoundaryLawID(1)
	body := returnBoundaryLawID(2)
	keys := [2]computationKey{
		{module: module, occurrence: returnBoundaryLawID(3)},
		{module: module, occurrence: returnBoundaryLawID(4)},
	}
	schema := &Schema{
		coordinateCount:           5,
		returnBoundaries:          make(map[computationKey]ReturnBoundary),
		returnBoundariesByBody:    make(map[computationKey][]computationKey),
		returnBoundaryMemberIndex: make(map[computationKey]uint32),
		returnBoundaryMembers: []returnBoundaryMember{
			{coordinate: Coordinate{index: 2}, content: returnBoundaryLawID(10)},
			{coordinate: Coordinate{index: 3}, content: returnBoundaryLawID(11)},
			{coordinate: Coordinate{index: 4}, content: returnBoundaryLawID(12)},
		},
	}
	for index := range schema.returnBoundaryMembers {
		schema.returnBoundaryMembers[index].coordinate.schema = schema
		schema.returnBoundaryMemberIndex[computationKey{module: module, occurrence: schema.returnBoundaryMembers[index].content}] = uint32(index)
	}
	rows := [2]ReturnBoundary{
		{schema: schema, key: keys[0], body: body, ordinal: 0, content: returnBoundaryLawID(5), root: Coordinate{schema: schema, index: 1}, memberOffset: 0, memberCount: 1},
		{schema: schema, key: keys[1], body: body, ordinal: 1, content: returnBoundaryLawID(6), root: Coordinate{schema: schema, index: 1}, memberOffset: 1, memberCount: 2},
	}
	for index, row := range rows {
		schema.returnBoundaries[keys[index]] = row
		schema.returnBoundaryOrder = append(schema.returnBoundaryOrder, keys[index])
	}
	bodyKey := computationKey{module: module, occurrence: body}
	schema.returnBoundariesByBody[bodyKey] = []computationKey{keys[0], keys[1]}
	return schema, keys
}

// TestReturnBoundaryDirectoriesAreDenseAndInvertible states what a published
// directory owes: a census, a row at every ordinal below it, and an ordinal
// that takes each row back to exactly the position it was read from. The
// member arena owes the same, plus the inverse from a member's owner-issued
// mounted identity, which is the address the candidate resolver answers.
func TestReturnBoundaryDirectoriesAreDenseAndInvertible(t *testing.T) {
	schema, _ := returnBoundaryCatalogFixture(t)
	module := returnBoundaryLawID(1)

	count := schema.ReturnBoundaryCount()
	if count != 2 {
		t.Fatalf("return-boundary census = %d, want 2", count)
	}
	for index := 0; index < count; index++ {
		boundary, boundaryOK := schema.ReturnBoundaryAt(index)
		if !boundaryOK {
			t.Fatalf("ReturnBoundaryAt(%d)", index)
		}
		ordinal, ordinalOK := schema.ReturnBoundaryOrdinal(boundary)
		if !ordinalOK || int(ordinal) != index {
			t.Fatalf("ReturnBoundaryOrdinal at %d = %d/%t", index, ordinal, ordinalOK)
		}
		if _, rootOK := boundary.Root(); !rootOK {
			t.Fatalf("return boundary %d publishes no root coordinate", index)
		}
	}
	if _, ok := schema.ReturnBoundaryAt(count); ok {
		t.Fatal("the return-boundary directory answered past its own census")
	}

	members := schema.ReturnBoundaryMemberCount()
	if members != 3 {
		t.Fatalf("member census = %d, want 3", members)
	}
	for index := 0; index < members; index++ {
		row, rowOK := schema.ReturnBoundaryMemberAt(index)
		if !rowOK {
			t.Fatalf("ReturnBoundaryMemberAt(%d)", index)
		}
		ordinal, ordinalOK := schema.ReturnBoundaryMemberOrdinal(row)
		if !ordinalOK || int(ordinal) != index {
			t.Fatalf("ReturnBoundaryMemberOrdinal at %d = %d/%t", index, ordinal, ordinalOK)
		}
		id, idOK := row.ID()
		if !idOK || !id.Available() {
			t.Fatalf("member %d publishes no mounted identity", index)
		}
		resolved, resolvedOK := schema.ReturnBoundaryMemberForMountedOccurrence(module, id)
		if !resolvedOK || resolved != row {
			t.Fatalf("ReturnBoundaryMemberForMountedOccurrence at %d did not answer the row it identifies", index)
		}
		if _, coordinateOK := row.Coordinate(); !coordinateOK {
			t.Fatalf("member %d publishes no coordinate", index)
		}
	}
	if _, ok := schema.ReturnBoundaryMemberAt(members); ok {
		t.Fatal("the member arena answered past its own census")
	}

	foreign := *schema
	first, firstOK := schema.ReturnBoundaryMemberAt(0)
	if !firstOK || foreign.OwnsReturnBoundaryMember(first) {
		t.Fatal("foreign equal-content Schema accepted a return-boundary member")
	}
}

// TestReturnBoundaryMemberSetIsAddressedByOrdinalThroughTheGeneratedOwner is
// the addressing half of the same obligation, stated where the child actually
// reads it: through the generated relation owner.
//
// A return's members are a nested ordered member set - a bounded port list -
// so a reader that holds the candidate row addresses member k by its ordinal
// and projects that row's key like any other. The census, the ordinal
// address, and the key projection must agree with the boundary the arena was
// sealed for; if they do not, a reader observing "member 1" is observing some
// other return's coordinate.
func TestReturnBoundaryMemberSetIsAddressedByOrdinalThroughTheGeneratedOwner(t *testing.T) {
	schema, keys := returnBoundaryCatalogFixture(t)
	owner := NewRelationOwner(schema)
	if owner == nil {
		t.Fatal("value publishes no generated relation owner")
	}
	catalog := AxisMemberCatalog()
	candidates, candidatesOK := catalog.RelationOrdinal(ReturnBoundaryCandidates)
	roots, rootsOK := catalog.RelationOrdinal(ReturnBoundaryRoots)
	memberSet, memberSetOK := catalog.RelationOrdinal(ReturnBoundaryMembers)
	rootKey, rootKeyOK := catalog.ProjectionOrdinal(ReturnBoundaryRootKey)
	memberKey, memberKeyOK := catalog.ProjectionOrdinal(ReturnBoundaryMemberKey)
	if !candidatesOK || !rootsOK || !memberSetOK || !rootKeyOK || !memberKeyOK {
		t.Fatal("value publishes no return-boundary relation/projection ordinals")
	}

	for index, key := range keys {
		dense, denseOK := owner.CandidateAt(candidates, key.module, key.occurrence, 0)
		if !denseOK || int(dense) != index {
			t.Fatalf("CandidateAt(return boundary %d) = %d/%t", index, dense, denseOK)
		}
		boundary, boundaryOK := schema.ReturnBoundaryAt(index)
		if !boundaryOK {
			t.Fatalf("ReturnBoundaryAt(%d)", index)
		}
		root, rootOK := boundary.Root()
		rootIndex, rootIndexOK := schema.CoordinateIndex(root)
		projectedRoot, projectedRootOK := owner.Project(roots, rootKey, dense)
		if !rootOK || !rootIndexOK || !projectedRootOK || projectedRoot != rootIndex {
			t.Fatalf("root projection of return boundary %d = %d/%t, want %d", index, projectedRoot, projectedRootOK, rootIndex)
		}

		census, censusOK := owner.MemberCount(memberSet, dense)
		if !censusOK || census != boundary.MemberCount() {
			t.Fatalf("member census of return boundary %d = %d/%t, want %d", index, census, censusOK, boundary.MemberCount())
		}
		for ordinal := 0; ordinal < census; ordinal++ {
			memberDense, memberDenseOK := owner.MemberAt(memberSet, dense, ordinal)
			row, rowOK := boundary.MemberAt(ordinal)
			expected, expectedOK := schema.ReturnBoundaryMemberOrdinal(row)
			if !memberDenseOK || !rowOK || !expectedOK || memberDense != expected {
				t.Fatalf("MemberAt(%d,%d) = %d/%t, want %d", index, ordinal, memberDense, memberDenseOK, expected)
			}
			coordinate, coordinateOK := row.Coordinate()
			coordinateIndex, coordinateIndexOK := schema.CoordinateIndex(coordinate)
			projected, projectedOK := owner.Project(memberSet, memberKey, memberDense)
			if !coordinateOK || !coordinateIndexOK || !projectedOK || projected != coordinateIndex {
				t.Fatalf("member key projection of (%d,%d) = %d/%t, want %d", index, ordinal, projected, projectedOK, coordinateIndex)
			}
		}
		if _, ok := owner.MemberAt(memberSet, dense, census); ok {
			t.Fatalf("return boundary %d answered a member past its own census", index)
		}
	}
}
