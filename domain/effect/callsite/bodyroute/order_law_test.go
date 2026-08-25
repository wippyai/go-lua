package bodyroute

import "testing"

// TestAMemberSetIsOrderedByTheCoordinateItsCellsAreReadAt states the canonical
// order this relation publishes its members in.
//
// The engine canonicalizes a selection by ascending tag, and every route is
// tagged with the Effect root coordinate its own cell is observed at, so
// ascending tag is that order. Call's target order and Effect's root order are
// separate authorities; a member ordinal is taken from Effect's, which is why
// the set is sorted here rather than published in the order the call's targets
// happened to be declared in.
func TestAMemberSetIsOrderedByTheCoordinateItsCellsAreReadAt(t *testing.T) {
	ordered, ok := order([]Route{
		{tag: 7, set: true},
		{tag: 2, set: true},
		{tag: 5, set: true},
	})
	if !ok || len(ordered) != 3 {
		t.Fatalf("a well formed route set was refused: %v %t", ordered, ok)
	}
	for index := 1; index < len(ordered); index++ {
		if ordered[index].tag <= ordered[index-1].tag {
			t.Fatalf("members are published as %v, want ascending root coordinate", ordered)
		}
	}
}

// TestTwoRoutesOnOneCoordinateAddressOneMemberTwice is the refusal half. A
// selection carries one cell per member, so two routes resolving to one root
// have no second ordinal between them; observing the set would fold one body's
// effect into the site twice.
func TestTwoRoutesOnOneCoordinateAddressOneMemberTwice(t *testing.T) {
	if _, ok := order([]Route{{tag: 3, set: true}, {tag: 3, set: true}}); ok {
		t.Fatal("two routes on one root coordinate were admitted as two members")
	}
}

// TestAnEmptyRouteSetIsOrdered states that a call reaching no body is a member
// set of nothing rather than a refusal: which bodies a call reaches is the
// call's own answer, and none is an answer.
func TestAnEmptyRouteSetIsOrdered(t *testing.T) {
	ordered, ok := order(nil)
	if !ok || len(ordered) != 0 {
		t.Fatalf("an empty route set was refused: %v %t", ordered, ok)
	}
}
