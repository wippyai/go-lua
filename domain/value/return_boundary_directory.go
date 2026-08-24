package value

import (
	"github.com/wippyai/go-lua/analysis/identity"
)

// ReturnBoundaryMember is one row of a sealed ReturnBoundary's ordered fixed
// member set: the already-issued Value coordinate at one position of the
// Schema-owned member arena, together with the mounted identity that names
// that position.
//
// It is a row rather than a bare Coordinate because the member set is a
// published relation. A relation's rows carry their own identity and project
// to a coordinate; handing out the coordinate alone would leave the position
// addressable only by the boundary that holds it, and every consumer would
// have to rebuild the address the arena already has.
type ReturnBoundaryMember struct {
	schema *Schema
	// position is the one-based arena index, so the zero row is not a member.
	position uint32
}

func (member ReturnBoundaryMember) valid() bool {
	if member.schema == nil || member.position == 0 ||
		uint64(member.position) > uint64(len(member.schema.returnBoundaryMembers)) {
		return false
	}
	row := member.schema.returnBoundaryMembers[member.position-1]
	return row.coordinate.schema == member.schema && row.coordinate.Valid() && row.content.Available()
}

// OwnsReturnBoundaryMember is the exact Schema owner fence for a detached
// member row. Equal-content Value schemas cannot exchange rows.
func (schema *Schema) OwnsReturnBoundaryMember(member ReturnBoundaryMember) bool {
	return schema != nil && member.schema == schema && member.valid()
}

// ID returns the owner-issued mounted identity of this member. It is the exact
// inverse of ReturnBoundaryMemberForMountedOccurrence.
func (member ReturnBoundaryMember) ID() (identity.ContentID, bool) {
	if !member.valid() {
		return identity.ContentID{}, false
	}
	return member.schema.returnBoundaryMembers[member.position-1].content, true
}

// Coordinate returns the already-issued Value coordinate this member resolves
// to. It is the member relation's key projection.
func (member ReturnBoundaryMember) Coordinate() (Coordinate, bool) {
	if !member.valid() {
		return Coordinate{}, false
	}
	return member.schema.returnBoundaryMembers[member.position-1].coordinate, true
}

// ReturnBoundaryCount is the census of Value's dense return-boundary
// directory.
func (schema *Schema) ReturnBoundaryCount() int {
	if schema == nil {
		return 0
	}
	return len(schema.returnBoundaryOrder)
}

// ReturnBoundaryAt returns one dense return boundary. Order is sealed mount
// order, then Program occurrence order within a mount.
func (schema *Schema) ReturnBoundaryAt(index int) (ReturnBoundary, bool) {
	if schema == nil || index < 0 || index >= len(schema.returnBoundaryOrder) {
		return ReturnBoundary{}, false
	}
	boundary, ok := schema.returnBoundaries[schema.returnBoundaryOrder[index]]
	if !ok || !schema.OwnsReturnBoundary(boundary) || int(boundary.ordinal) != index {
		return ReturnBoundary{}, false
	}
	return boundary, true
}

// ReturnBoundaryOrdinal is the exact inverse of ReturnBoundaryAt over this
// Schema.
func (schema *Schema) ReturnBoundaryOrdinal(boundary ReturnBoundary) (uint32, bool) {
	if schema == nil || !schema.OwnsReturnBoundary(boundary) ||
		uint64(boundary.ordinal) >= uint64(len(schema.returnBoundaryOrder)) {
		return 0, false
	}
	if schema.returnBoundaryOrder[boundary.ordinal] != boundary.key {
		return 0, false
	}
	return boundary.ordinal, true
}

// ReturnBoundaryMemberCount is the census of the sealed member arena: every
// fixed member of every sealed return boundary, in boundary order.
func (schema *Schema) ReturnBoundaryMemberCount() int {
	if schema == nil {
		return 0
	}
	return len(schema.returnBoundaryMembers)
}

// ReturnBoundaryMemberAt returns one dense member row of that arena.
func (schema *Schema) ReturnBoundaryMemberAt(index int) (ReturnBoundaryMember, bool) {
	if schema == nil || index < 0 || index >= len(schema.returnBoundaryMembers) {
		return ReturnBoundaryMember{}, false
	}
	member := ReturnBoundaryMember{schema: schema, position: uint32(index) + 1}
	if !member.valid() {
		return ReturnBoundaryMember{}, false
	}
	return member, true
}

// ReturnBoundaryMemberOrdinal is the exact inverse of
// ReturnBoundaryMemberAt over this Schema.
func (schema *Schema) ReturnBoundaryMemberOrdinal(member ReturnBoundaryMember) (uint32, bool) {
	if !schema.OwnsReturnBoundaryMember(member) {
		return 0, false
	}
	return member.position - 1, true
}

// ReturnBoundaryMemberForMountedOccurrence is the mount-qualified candidate
// resolver: occurrence is the member row's own owner-issued content identity,
// the exact inverse of the identity ReturnBoundaryMember.ID returns.
func (schema *Schema) ReturnBoundaryMemberForMountedOccurrence(module, occurrence identity.ContentID) (ReturnBoundaryMember, bool) {
	if schema == nil || schema.returnBoundaryMemberIndex == nil || !module.Available() || !occurrence.Available() {
		return ReturnBoundaryMember{}, false
	}
	position, ok := schema.returnBoundaryMemberIndex[computationKey{module: module, occurrence: occurrence}]
	if !ok {
		return ReturnBoundaryMember{}, false
	}
	return schema.ReturnBoundaryMemberAt(int(position))
}
