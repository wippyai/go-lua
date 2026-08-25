package value

import (
	"github.com/wippyai/go-lua/analysis/identity"
	calldomain "github.com/wippyai/go-lua/domain/call"
)

// mountedCallActualsKey is the parent address of one mounted call's ordered
// actual list. It is the (module, call) prefix of the actual rows' own key, so
// the parent is a grouping of rows this Schema already sealed rather than a
// second directory over call identity: Call still owns which mounted calls
// exist, and Pack still owns the endpoint geometry the actual rows were
// projected from.
type mountedCallActualsKey struct {
	module identity.ContentID
	call   identity.ContentID
}

// MountedCallActuals is Value's per-call parent row over the mounted actuals
// it publishes: the bounded ordered member set one mounted call carries.
//
// It exists because a member set has to be addressed by (parent, ordinal), and
// the parent of a mounted actual is its call. The row holds the dense span its
// members occupy in this Schema's own actual directory, so a member is reached
// by the same directory every other actual row is reached by and no consumer
// re-correlates a call to its actuals.
type MountedCallActuals struct {
	schema  *Schema
	key     mountedCallActualsKey
	content identity.ContentID
	// coordinate is the mounted-call coordinate Call published for this call.
	// Call is the earliest owner of it, so the seal copies it here rather than
	// leaving every consumer to resolve the occurrence against Call again.
	coordinate calldomain.CallCoordinate
	// callee is Value's owner-issued coordinate for the portable Boundary
	// Value identity Call published on coordinate. Value resolves it once while
	// sealing this parent so dispatch consumers never reopen Call geometry or
	// reconstruct a Value coordinate from a foreign dense ordinal.
	callee Coordinate
	// first is the dense ordinal of actual zero in mountedCallArgumentOrder and
	// count is the member census. The rows are contiguous because they are
	// admitted in per-call actual order by one pass of sealMountedCallArguments.
	first uint32
	count uint32
	// freshFirst and freshCount are the same span shape over this Schema's
	// fresh-result directory: the fresh results this call produces, addressed
	// by their result ordinal. They are attached after the fresh directory is
	// sealed, which is later than the actuals, so a call with no fresh result
	// carries the empty span it has.
	freshFirst uint32
	freshCount uint32
}

func (row MountedCallActuals) valid() bool {
	return row.schema != nil && row.schema.Valid() && row.key.module.Available() &&
		row.key.call.Available() && row.content.Available() && row.coordinate.Valid() &&
		row.callee.Valid() && row.callee.schema == row.schema &&
		uint64(row.first)+uint64(row.count) <= uint64(len(row.schema.mountedCallArgumentOrder)) &&
		uint64(row.freshFirst)+uint64(row.freshCount) <= uint64(len(row.schema.freshResultCallKeys))
}

// OwnsMountedCallActuals is the exact Schema owner fence for a detached parent
// row. Equal-content Value schemas cannot exchange rows.
func (schema *Schema) OwnsMountedCallActuals(row MountedCallActuals) bool {
	if schema == nil || row.schema != schema || !row.valid() || schema.mountedCallActuals == nil {
		return false
	}
	canonical, ok := schema.mountedCallActuals[row.key]
	return ok && canonical == row
}

// MountedCallActualsFor resolves one mounted call's actual list by its exact
// mounted call coordinate.
func (schema *Schema) MountedCallActualsFor(module, call identity.ContentID) (MountedCallActuals, bool) {
	if schema == nil || !schema.Valid() || schema.mountedCallActuals == nil || !module.Available() || !call.Available() {
		return MountedCallActuals{}, false
	}
	row, ok := schema.mountedCallActuals[mountedCallActualsKey{module: module, call: call}]
	return row, ok && schema.OwnsMountedCallActuals(row)
}

// MountedCallActualsCount is the dense, mount-major census of parent rows.
func (schema *Schema) MountedCallActualsCount() int {
	if schema == nil {
		return 0
	}
	return len(schema.mountedCallActualsOrder)
}

// MountedCallActualsAt returns one dense parent row in sealed mount order then
// call order.
func (schema *Schema) MountedCallActualsAt(index int) (MountedCallActuals, bool) {
	if schema == nil || index < 0 || index >= len(schema.mountedCallActualsOrder) {
		return MountedCallActuals{}, false
	}
	row, ok := schema.mountedCallActuals[schema.mountedCallActualsOrder[index]]
	return row, ok && schema.OwnsMountedCallActuals(row)
}

// MountedCallActualsOrdinal is the exact inverse of MountedCallActualsAt over
// this Schema.
func (schema *Schema) MountedCallActualsOrdinal(row MountedCallActuals) (uint32, bool) {
	if schema == nil || !schema.OwnsMountedCallActuals(row) {
		return 0, false
	}
	ordinal, ok := schema.mountedCallActualsOrdinals[row.key]
	return ordinal, ok
}

// MountedCallActualsForMountedOccurrence is the mount-qualified candidate
// resolver. The occurrence is the authored Call identity, the same occurrence
// the mounted call candidate directory is addressed by, so a rule whose
// candidate is a mounted call reaches this parent under the row it already has.
func (schema *Schema) MountedCallActualsForMountedOccurrence(module, occurrence identity.ContentID) (MountedCallActuals, bool) {
	return schema.MountedCallActualsFor(module, occurrence)
}

// mountedCallParentsCorrespond proves a bijection between Call's mounted-call
// candidates and Value's parent rows by their shared semantic address. Dense
// ordinals are deliberately ignored: each owner is free to order its own
// directory independently.
func mountedCallParentsCorrespond(schema *Schema, calls *calldomain.Algebra) bool {
	if schema == nil || calls == nil || len(schema.mountedCallActualsOrder) != calls.CallCoordinateCount() {
		return false
	}
	for ordinal := 0; ordinal < calls.CallCoordinateCount(); ordinal++ {
		coordinate, coordinateOK := calls.CallCoordinateAt(ordinal)
		if !coordinateOK || !calls.OwnsCallCoordinate(coordinate) {
			return false
		}
		module, moduleOK := coordinate.ModuleID()
		call, callOK := coordinate.CallID()
		if !moduleOK || !callOK {
			return false
		}
		parent, parentOK := schema.mountedCallActuals[mountedCallActualsKey{module: module, call: call}]
		if !parentOK || !schema.OwnsMountedCallActuals(parent) || parent.coordinate != coordinate {
			return false
		}
	}
	return true
}

// ID returns the owner-issued identity of this parent row.
func (row MountedCallActuals) ID() (identity.ContentID, bool) {
	if !row.valid() {
		return identity.ContentID{}, false
	}
	return row.content, true
}

// Module returns the exact mounted module identity for this row.
func (row MountedCallActuals) Module() (identity.ContentID, bool) {
	if !row.valid() {
		return identity.ContentID{}, false
	}
	return row.key.module, true
}

// CallID returns the authored Call identity this actual list belongs to.
func (row MountedCallActuals) CallID() (identity.ContentID, bool) {
	if !row.valid() {
		return identity.ContentID{}, false
	}
	return row.key.call, true
}

// CallCoordinate returns the mounted-call coordinate Call published for this
// call. It is the key this parent row's order and Call's own candidate order
// agree on, which is what lets a rule keyed by a mounted call address these
// actuals without re-correlating the two directories.
func (row MountedCallActuals) CallCoordinate() (calldomain.CallCoordinate, bool) {
	if !row.valid() || !row.coordinate.Valid() {
		return calldomain.CallCoordinate{}, false
	}
	return row.coordinate, true
}

// CalleeCoordinate returns Value's owner-issued coordinate for the portable
// callee Value of this call. The coordinate is sealed on the parent
// because call dispatch and every other consumer must use the same Value
// directory rather than derive an address from Call's local ordinal.
func (row MountedCallActuals) CalleeCoordinate() (Coordinate, bool) {
	if !row.valid() || !row.callee.Valid() || row.callee.schema != row.schema {
		return Coordinate{}, false
	}
	return row.callee, true
}

// MemberCount is the census of this call's ordered actual list.
func (row MountedCallActuals) MemberCount() int {
	if !row.valid() {
		return 0
	}
	return int(row.count)
}

// MemberAt addresses one actual of this call by its ordinal: the receiver
// first for a method-form call, then each declared argument in order.
func (row MountedCallActuals) MemberAt(ordinal int) (MountedCallArgument, bool) {
	if !row.valid() || ordinal < 0 || uint64(ordinal) >= uint64(row.count) {
		return MountedCallArgument{}, false
	}
	member, memberOK := row.schema.MountedCallArgumentAt(int(row.first) + ordinal)
	if !memberOK {
		return MountedCallArgument{}, false
	}
	// A member of THIS parent is a row of this call. The span is contiguous by
	// construction; proving it here keeps a malformed directory from handing a
	// consumer another call's actual under this call's ordinal.
	if member.key.module != row.key.module || member.key.call != row.key.call || member.key.actual != uint32(ordinal) {
		return MountedCallArgument{}, false
	}
	return member, true
}

// FreshResultCount is the census of the fresh results this call produces.
func (row MountedCallActuals) FreshResultCount() int {
	if !row.valid() {
		return 0
	}
	return int(row.freshCount)
}

// FreshResultAt addresses one fresh result of this call by its member ordinal,
// which is the call result ordinal the fresh root fills.
func (row MountedCallActuals) FreshResultAt(ordinal int) (FreshResultCall, bool) {
	if !row.valid() || ordinal < 0 || uint64(ordinal) >= uint64(row.freshCount) {
		return FreshResultCall{}, false
	}
	member, memberOK := row.schema.FreshResultCallAt(int(row.freshFirst) + ordinal)
	if !memberOK {
		return FreshResultCall{}, false
	}
	// A member of THIS parent is a fresh result of this call. The span is
	// contiguous by construction; proving it here keeps a malformed directory
	// from handing a consumer another call's result under this call's ordinal.
	if member.module != row.key.module || member.call != row.key.call {
		return FreshResultCall{}, false
	}
	return member, true
}

// attachFreshResultMembers records one parent's fresh-result span. It runs
// after the fresh directory is sealed because the span is over that directory:
// the parent groups rows it did not create, exactly as it does for actuals.
func (builder *valueBuilder) attachFreshResultMembers(key mountedCallActualsKey, first int, count uint32) bool {
	if builder == nil || builder.Schema == nil || builder.Schema.mountedCallActuals == nil || first < 0 ||
		uint64(first)+uint64(count) > uint64(len(builder.Schema.freshResultCallKeys)) {
		return false
	}
	row, found := builder.Schema.mountedCallActuals[key]
	if !found || row.freshCount != 0 || row.freshFirst != 0 {
		return false
	}
	row.freshFirst, row.freshCount = uint32(first), count
	builder.Schema.mountedCallActuals[key] = row
	return builder.Schema.OwnsMountedCallActuals(row)
}

// addMountedCallActuals admits one mounted call's parent row over the actual
// rows just sealed for it. The span is the prefix of this Schema's own actual
// directory those rows occupy, so the parent groups rows it did not create and
// states nothing about the call beyond which of its own rows belong to it. A
// call with no actuals publishes the empty list it has.
func (builder *valueBuilder) addMountedCallActuals(module, call identity.ContentID, first int, count uint32) bool {
	if builder == nil || builder.Schema == nil || builder.Schema.mountedCallActuals == nil ||
		builder.Schema.mountedCallActualsOrdinals == nil || first < 0 ||
		uint64(first)+uint64(count) != uint64(len(builder.Schema.mountedCallArgumentOrder)) {
		return false
	}
	key := mountedCallActualsKey{module: module, call: call}
	if _, duplicate := builder.Schema.mountedCallActuals[key]; duplicate {
		return false
	}
	content := computationContent(builder.linkID, "val-callactuals!", module, call)
	if !content.Available() {
		return false
	}
	// The coordinate is Call's, and its absence is a refusal: a call whose
	// occurrence Call does not name has no coordinate to publish, and a zero
	// one would be a default standing in for a fact nobody derived.
	coordinate, coordinateOK := builder.callCoordinateForOccurrence(module, call)
	if !coordinateOK {
		return false
	}
	coordinateModule, coordinateModuleOK := coordinate.ModuleID()
	coordinateCall, coordinateCallOK := coordinate.CallID()
	calleeID, calleeIDOK := coordinate.CalleeValueID()
	if !coordinateModuleOK || !coordinateCallOK || !calleeIDOK || coordinateModule != module || coordinateCall != call {
		return false
	}
	// Call's callee identity is the Link-owned Boundary Value identity, not an
	// artifact semantic identity. Value therefore resolves it through its one
	// portable coordinate directory; qualifying it as mounted semantic would
	// address a different identity domain and reject valid calls.
	callee, calleeOK := builder.Schema.CoordinateForID(calleeID)
	if !calleeOK {
		return false
	}
	row := MountedCallActuals{schema: builder.Schema, key: key, content: content, coordinate: coordinate, callee: callee, first: uint32(first), count: count}
	// Every ordinal the parent will answer must already be the directory's row
	// for this call at that ordinal. Proving the span here is what lets a
	// consumer address a member by (parent, ordinal) without re-correlating.
	for ordinal := uint32(0); ordinal < count; ordinal++ {
		member, memberOK := builder.Schema.MountedCallArgumentAt(first + int(ordinal))
		if !memberOK || member.key.module != module || member.key.call != call || member.key.actual != ordinal {
			return false
		}
	}
	builder.Schema.mountedCallActualsOrdinals[key] = uint32(len(builder.Schema.mountedCallActualsOrder))
	builder.Schema.mountedCallActualsOrder = append(builder.Schema.mountedCallActualsOrder, key)
	builder.Schema.mountedCallActuals[key] = row
	return builder.Schema.OwnsMountedCallActuals(row)
}
