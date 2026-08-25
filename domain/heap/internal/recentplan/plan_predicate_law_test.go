package recentplan

import (
	"testing"

	"github.com/wippyai/go-lua/domain/heap"
)

// A route answers its predicate under exactly the fence its coordinates answer
// under. A routed output pairs a selected cell with the member it belongs to by
// this tag, so a row that cannot state where it reads must not state a tag
// either: one fence, two projections, and never a row addressable under one and
// silent under the other.
//
// The states reachable without a sealed Heap schema are the refusals, which is
// the direction that matters here - a tag answered beside absent coordinates is
// what would let the emitted worker pair a cell with nothing. The agreeing
// positive path runs against a real sealed schema in the emitted family laws.
func TestRoutePredicateAnswersUnderTheCoordinateFence(t *testing.T) {
	for name, route := range map[string]Route{
		"no key and no tag": {},
		"tag without a key": {Tag: heap.RawRouteTag(7)},
	} {
		tag, predicateOK := route.Predicate()
		_, _, coordinatesOK := route.Coordinates()
		if predicateOK != coordinatesOK {
			t.Fatalf("%s: predicate ok %v, coordinates ok %v", name, predicateOK, coordinatesOK)
		}
		if predicateOK {
			t.Fatalf("%s answered predicate %d", name, tag)
		}
	}
}

// The predicate is the tag Heap issued, not a discriminator this package
// derives: whatever a route carries is what it projects.
func TestRoutePredicateProjectsTheIssuedTag(t *testing.T) {
	for _, tag := range []heap.RawRouteTag{1, 7, ^heap.RawRouteTag(0)} {
		route := Route{Tag: tag}
		if projected, _ := route.Predicate(); projected != uint64(tag) {
			t.Fatalf("route tag %d projected %d", tag, projected)
		}
	}
}
