package compiler

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func departureID(seed byte) identity.ContentID {
	var id identity.ContentID
	id[0] = seed
	return id
}

// TestRoutedTransferNeverBypassesItsRouteStage states the placement's totality:
// a route that carries routed stages departs from them, and from nothing else.
//
// The stages stand between the route and the point it lands on precisely so
// the point assembles what they proved. A departure taken from the source, or
// from the source's own chain, would deliver the unproved state to the
// destination and leave the stages proving something no reader ever sees -
// which is the defect the placement exists to remove, restored silently.
func TestRoutedTransferNeverBypassesItsRouteStage(t *testing.T) {
	route, source := departureID(1), departureID(2)
	sourceTerminal, routeStagePoint := departureID(3), departureID(4)
	linearFor := map[identity.ContentID][]identity.ContentID{source: {sourceTerminal}}
	routeStage := map[identity.ContentID][]routedStagePlacement{
		route: {{order: 9, point: routeStagePoint}},
	}
	departure := routeDeparture(route, source, routeStage, linearFor)
	if departure != routeStagePoint {
		t.Fatalf("routed departure = %v, want the route's own stage %v", departure, routeStagePoint)
	}
	if departure == source || departure == sourceTerminal {
		t.Fatal("a routed transfer bypassed the stage standing on its route")
	}
}

// TestRoutedTransferDepartsFromTheLastStageOnItsRoute states that a route
// carrying more than one stage - one per axis it proves something about -
// delivers all of them, not merely the first.
func TestRoutedTransferDepartsFromTheLastStageOnItsRoute(t *testing.T) {
	route, source := departureID(1), departureID(2)
	first, last := departureID(5), departureID(6)
	routeStage := map[identity.ContentID][]routedStagePlacement{
		route: {{order: 9, point: first}, {order: 9, point: last}},
	}
	if departure := routeDeparture(route, source, routeStage, nil); departure != last {
		t.Fatalf("routed departure = %v, want the last stage on the route %v", departure, last)
	}
}

// TestUnroutedTransferDepartsFromItsSourceChain states the complementary half:
// a route with no routed stage is untouched by this placement and still leaves
// from where its source finished.
func TestUnroutedTransferDepartsFromItsSourceChain(t *testing.T) {
	route, source, terminal := departureID(1), departureID(2), departureID(3)
	linearFor := map[identity.ContentID][]identity.ContentID{source: {departureID(7), terminal}}
	if departure := routeDeparture(route, source, nil, linearFor); departure != terminal {
		t.Fatalf("unrouted departure = %v, want the source's terminal stage %v", departure, terminal)
	}
	if departure := routeDeparture(route, source, nil, nil); departure.Available() {
		t.Fatal("a source with no chain named a departure other than itself")
	}
}
