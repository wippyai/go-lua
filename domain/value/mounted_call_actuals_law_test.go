package value_test

import (
	"testing"
)

// TestEveryMountedCallActualsParentSpansExactlyItsOwnActuals is the seal law
// for Value's per-call parent row.
//
// The parent exists so a member set can be addressed by (parent, ordinal). It
// is therefore only correct if the members it hands out at ordinals 0..n-1 are
// exactly the actual rows this Schema already publishes for that call, in the
// same order, with nothing borrowed from a neighbouring call. That is what a
// grouping of one's own sealed rows means, and it is the whole claim the
// declaration rests on: the parent mints no identity and re-derives no
// geometry, so if the span ever disagreed with the directory the row would be
// a second, wrong, authority over which actuals a call has.
func TestEveryMountedCallActualsParentSpansExactlyItsOwnActuals(t *testing.T) {
	const source = "local receiver = {}\n" +
		"function receiver:method(a, b)\n" +
		"\treturn a\n" +
		"end\n" +
		"local function two(a, b)\n" +
		"\treturn a\n" +
		"end\n" +
		"local function noop()\n" +
		"end\n" +
		"noop()\n" +
		"two(1, 2)\n" +
		"receiver:method(3, 4)\n"

	fixture := buildMountedCallArgumentFixture(t, source)
	values := fixture.values

	parents := values.MountedCallActualsCount()
	if parents == 0 {
		t.Fatal("no mounted call actual parent rows were sealed, so this law measures nothing")
	}

	covered := 0
	exercisedEmpty := false
	exercisedWide := false
	for index := 0; index < parents; index++ {
		parent, parentOK := values.MountedCallActualsAt(index)
		if !parentOK || !values.OwnsMountedCallActuals(parent) {
			t.Fatalf("parent %d is not an owned row", index)
		}
		ordinal, ordinalOK := values.MountedCallActualsOrdinal(parent)
		if !ordinalOK || ordinal != uint32(index) {
			t.Fatalf("parent %d round-trips to ordinal %d/%t", index, ordinal, ordinalOK)
		}
		module, moduleOK := parent.Module()
		call, callOK := parent.CallID()
		if !moduleOK || !callOK {
			t.Fatalf("parent %d has no mounted call address", index)
		}
		resolved, resolvedOK := values.MountedCallActualsForMountedOccurrence(module, call)
		if !resolvedOK || resolved != parent {
			t.Fatalf("parent %d does not resolve from its own mounted occurrence", index)
		}

		// The selection tag ranks in authored order. This is the clause a
		// consuming rule used to have to mint for itself: because the tags
		// strictly increase with the ordinal and are unique under one parent, a
		// read ranked by tag is ranked in authored order and no rule decodes a
		// correspondence of its own.
		previousTag := uint64(0)
		seenTags := make(map[uint64]int, parent.MemberCount())

		count := parent.MemberCount()
		if count == 0 {
			exercisedEmpty = true
		}
		if count > 1 {
			exercisedWide = true
		}
		for member := 0; member < count; member++ {
			row, rowOK := parent.MemberAt(member)
			if !rowOK {
				t.Fatalf("parent %d has no member at ordinal %d of %d", index, member, count)
			}
			// The member must be the row the directory publishes for exactly
			// this (call, ordinal) - not merely some owned actual row.
			direct, directOK := values.MountedCallArgumentFor(module, call, uint32(member))
			if !directOK || direct != row {
				t.Fatalf("parent %d member %d is not the directory's row for that actual", index, member)
			}
			actual, actualOK := row.ActualIndex()
			tag, tagOK := row.ActualTag()
			if !actualOK || actual != uint32(member) || !tagOK || tag != uint64(member)+1 {
				t.Fatalf("parent %d member %d carries ordinal %d/%t tag %d/%t", index, member, actual, actualOK, tag, tagOK)
			}
			if tag == 0 {
				t.Fatalf("parent %d member %d carries the reserved zero tag", index, member)
			}
			if member > 0 && tag <= previousTag {
				t.Fatalf("parent %d member %d tag %d does not follow %d", index, member, tag, previousTag)
			}
			if earlier, repeated := seenTags[tag]; repeated {
				t.Fatalf("parent %d tag %d names both ordinal %d and %d", index, tag, earlier, member)
			}
			seenTags[tag], previousTag = member, tag
			covered++
		}
		if _, beyond := parent.MemberAt(count); beyond {
			t.Fatalf("parent %d answers a member beyond its census of %d", index, count)
		}
	}

	// Every actual row belongs to exactly one parent's span: the grouping
	// partitions the directory rather than covering part of it.
	if covered != values.MountedCallArgumentCount() {
		t.Fatalf("parents span %d actual rows, the directory publishes %d", covered, values.MountedCallArgumentCount())
	}
	if !exercisedEmpty {
		t.Fatal("no zero-actual call was exercised, so the empty member set is unmeasured")
	}
	if !exercisedWide {
		t.Fatal("no multi-actual call was exercised, so member ordering is unmeasured")
	}
}

// TestEveryMountedCallActualsParentPublishesCallsOwnCoordinate is the
// correspondence law of Value's per-call parent row.
//
// The parent row and Call's mounted-call directory enumerate the same
// subjects, and the row states which of Call's rows it is by carrying the
// coordinate Call published for that occurrence. Call is the earliest owner of
// that coordinate, so the seal copies it once instead of leaving every rule
// keyed by a mounted call to resolve the occurrence against Call again - which
// is the re-correlation the parent row exists to remove.
//
// The claim has to hold for every sealed row, not most of them: a parent that
// published no coordinate, or one Call does not own, would make the
// correspondence a coincidence of two independent enumerations.
func TestEveryMountedCallActualsParentPublishesCallsOwnCoordinate(t *testing.T) {
	const source = "local function two(a, b)\n" +
		"\treturn a\n" +
		"end\n" +
		"two(1, 2)\n" +
		"two(3, 4)\n"

	fixture := buildMountedCallArgumentFixture(t, source)
	values, calls := fixture.values, fixture.calls

	parents := values.MountedCallActualsCount()
	if parents == 0 {
		t.Fatal("no mounted call actual parent rows were sealed, so this law measures nothing")
	}
	if parents != calls.CallCoordinateCount() {
		t.Fatalf("Value parents = %d, Call candidates = %d", parents, calls.CallCoordinateCount())
	}
	// Enumerate Call's owner-local order and resolve Value by the shared
	// semantic address. This proves the correspondence without assuming the
	// owners assign equal dense ordinals.
	for ordinal := 0; ordinal < calls.CallCoordinateCount(); ordinal++ {
		coordinate, coordinateOK := calls.CallCoordinateAt(ordinal)
		module, moduleOK := coordinate.ModuleID()
		callID, callIDOK := coordinate.CallID()
		parent, parentOK := values.MountedCallActualsForMountedOccurrence(module, callID)
		published, publishedOK := parent.CallCoordinate()
		if !coordinateOK || !moduleOK || !callIDOK || !parentOK || !publishedOK || published != coordinate {
			t.Fatalf("Call candidate %d does not resolve to Value's matching parent", ordinal)
		}
	}
	for index := 0; index < parents; index++ {
		parent, parentOK := values.MountedCallActualsAt(index)
		if !parentOK {
			t.Fatalf("parent %d is not addressable in its own directory", index)
		}
		module, moduleOK := parent.Module()
		callID, callIDOK := parent.CallID()
		coordinate, coordinateOK := parent.CallCoordinate()
		if !moduleOK || !callIDOK || !coordinateOK {
			t.Fatalf("parent %d publishes module=%t call=%t coordinate=%t", index, moduleOK, callIDOK, coordinateOK)
		}
		if !calls.OwnsCallCoordinate(coordinate) {
			t.Fatalf("parent %d publishes a coordinate Call does not own", index)
		}
		published, publishedOK := calls.CallCoordinateForOccurrence(module, callID)
		if !publishedOK || published != coordinate {
			t.Fatalf("parent %d coordinate = %#v, want Call's own %#v/%t", index, coordinate, published, publishedOK)
		}
		calleeID, calleeIDOK := coordinate.CalleeValueID()
		callee, calleeOK := parent.CalleeCoordinate()
		expected, expectedOK := values.CoordinateForID(calleeID)
		if !calleeIDOK || !calleeOK || !expectedOK || callee != expected {
			t.Fatalf("parent %d callee coordinate = %#v/%t, want portable Value coordinate %#v/%t", index, callee, calleeOK, expected, expectedOK)
		}
		// CalleeValueID is a Link-owned Boundary Value identity. Treating it as
		// an artifact-semantic identity would cross identity domains and used to
		// reject every valid parent during this cut.
		if _, wrongDomain := values.CoordinateForMountedSemantic(module, calleeID); wrongDomain {
			t.Fatalf("parent %d Boundary callee identity was admitted as artifact semantic identity", index)
		}
	}
}
