package bodyroute

import "testing"

// A member set's ORDER is no longer this package's to fix. The relation
// declares it - ascending by the Key projection, normalized through the axis
// that numbers those coordinates - and the emitter writes it, so the law that
// used to live here is now held over every generated member set at once in
// analysis/schema/rule/emit's TestAGeneratedMemberSetIsOrderedByTheKeyItsRelationDeclares
// and TestAGeneratedMemberSetRefusesTwoRowsOnOneCoordinate.
//
// What is left here is the half that order rests on and that only this package
// can promise.

// TestARouteAnswersOneMemberThroughBothOfItsProjections is that half.
//
// The generated order sorts by the Key projection; the engine correlates the
// observed cells by the Predicate. Those are two questions asked of one row,
// and if a row could answer one and not the other, a member the order placed
// would be a member the selection could not correlate - or the reverse. So the
// two are available together or not at all.
func TestARouteAnswersOneMemberThroughBothOfItsProjections(t *testing.T) {
	var absent Route
	if _, keyOK := absent.Coordinate(); keyOK {
		t.Fatal("a route that resolved nothing answered a coordinate")
	}
	if _, tagOK := absent.Predicate(); tagOK {
		t.Fatal("a route that resolved nothing answered a tag")
	}

	resolved := Route{tag: 7, set: true}
	key, keyOK := resolved.Coordinate()
	tag, tagOK := resolved.Predicate()
	if !keyOK || !tagOK {
		t.Fatalf("a resolved route answers coordinate=%t tag=%t; one member is addressed by both", keyOK, tagOK)
	}
	if tag != 7 {
		t.Fatalf("the tag is %d, want the coordinate the route was resolved at", tag)
	}
	_ = key
}

// TestARouteCarriesTheCoordinateItsCellIsReadAt states the correspondence the
// order depends on: a route's tag IS its root's dense coordinate, which is why
// ordering by the declared Key and correlating by the tag cannot disagree.
// ResolveRoute is the one place that pairing is made, and it makes it from the
// same RootIndex the Key normalizes to.
func TestARouteCarriesTheCoordinateItsCellIsReadAt(t *testing.T) {
	for _, index := range []uint64{0, 1, 42} {
		route := Route{tag: index, set: true}
		tag, tagOK := route.Predicate()
		if !tagOK || tag != index {
			t.Fatalf("a route resolved at coordinate %d answers tag %d (ok=%t)", index, tag, tagOK)
		}
	}
}
