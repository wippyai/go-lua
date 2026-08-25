package execution

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// The carry of a publication is indexed by the thing that publishes. An exact
// row publishes at one coordinate and carries one image through one transition;
// a routed row publishes at N derived destinations and carries N images through
// N transitions, one per published row. These laws state that arity: the fold
// takes a closure per row, proves each one against the Factor's declared
// default at the row that brought it, and publishes nothing at all when any of
// them cannot be proven.

// routeCarryIdentity is the trivial closure: the transition an identity carry
// performs at every row.
func routeCarryIdentity(value uint64) (uint64, bool) { return value, true }

// routeCarryMovesDefault does not fix the Factor's declared default, so it
// invents a fact at every coordinate the Factor never wrote.
func routeCarryMovesDefault(value uint64) (uint64, bool) { return value + 1, true }

func routeCarries(width int, carry RouteCarry[uint64]) []RouteCarry[uint64] {
	carries := make([]RouteCarry[uint64], 0, width)
	for index := 0; index < width; index++ {
		carries = append(carries, carry)
	}
	return carries
}

// TestRoutedCarryPublishesTheSameRowTheUncarriedFoldDoes states that the
// one-row case agrees with the form it generalizes: a single-route carried fold
// with the trivial closure publishes exactly what the uncarried routed fold
// publishes, at the same destination and as one output.
func TestRoutedCarryPublishesTheSameRowTheUncarriedFoldDoes(t *testing.T) {
	fixture := newSelectedFixture(t)
	write, ok := NewRouteWrite(fixture.binding, 0)
	if !ok {
		t.Fatal("route write")
	}
	run := NewRun(1, 1)
	ticket := issueSelected(t, run, fixture, fixture.state)
	cells, members, routes := routeCells(fixture, 1)
	var scratch RouteScratch[uint64, uint64]
	outcome := FoldSelectedRouteCarry(ticket, write, &scratch, cells, members, routes,
		routeCarries(1, routeCarryIdentity), routeLawReducer{empty: structure.NoSelection, failAt: -1})
	if outcome != structure.Concrete {
		t.Fatalf("carried routed outcome = %d, want Concrete", outcome)
	}
	if !run.Submit(&ticket, outcome) {
		t.Fatal("submit")
	}
	patches := make([]carrier.Patch, 1)
	disposition, count, drained := run.Drain(patches)
	if !drained || disposition != structure.Concrete || count != 1 {
		t.Fatalf("drain = (%d,%d,%v), want one patch for the whole carried routed row", disposition, count, drained)
	}
}

// TestRoutedCarryProvesEveryRowsMapAgainstTheDeclaredDefault states the one law
// a carried map owes, at the arity a routed row has it: the proof is owed by
// each published row's own transition, because each row brought its own. A row
// whose map moves the default publishes nothing, and neither does the routed
// row it belongs to - a routed publication is one output, so half of it is not
// an answer.
func TestRoutedCarryProvesEveryRowsMapAgainstTheDeclaredDefault(t *testing.T) {
	for _, test := range []struct {
		name    string
		carries []RouteCarry[uint64]
	}{
		{name: "first-row", carries: []RouteCarry[uint64]{routeCarryMovesDefault, routeCarryIdentity, routeCarryIdentity}},
		{name: "last-row", carries: []RouteCarry[uint64]{routeCarryIdentity, routeCarryIdentity, routeCarryMovesDefault}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newSelectedFixture(t)
			write, ok := NewRouteWrite(fixture.binding, 0)
			if !ok {
				t.Fatal("route write")
			}
			run := NewRun(1, 1)
			ticket := issueSelected(t, run, fixture, fixture.state)
			cells, members, routes := routeCells(fixture, 3)
			var scratch RouteScratch[uint64, uint64]
			outcome := FoldSelectedRouteCarry(ticket, write, &scratch, cells, members, routes, test.carries,
				routeLawReducer{empty: structure.NoSelection, failAt: -1})
			if outcome != structure.Refuse {
				t.Fatalf("a routed row carrying a map that moves the default settled %d, want Refuse", outcome)
			}
			if !run.Submit(&ticket, outcome) {
				t.Fatal("submit")
			}
			patches := make([]carrier.Patch, 1)
			disposition, count, drained := run.Drain(patches)
			if !drained || disposition != structure.Refuse || count != 0 {
				t.Fatalf("drain = (%d,%d,%v), want a refused row to publish nothing", disposition, count, drained)
			}
		})
	}
}

// TestRoutedCarryRequiresOneClosurePerPublishedRow states the arity itself. A
// vector shorter than the selection, or one with a row that brought no
// transition, is a fold that would carry some published rows and not others -
// which is not a weaker answer but an unstated one.
func TestRoutedCarryRequiresOneClosurePerPublishedRow(t *testing.T) {
	for _, test := range []struct {
		name    string
		carries []RouteCarry[uint64]
	}{
		{name: "short-vector", carries: routeCarries(2, routeCarryIdentity)},
		{name: "no-vector", carries: nil},
		{name: "absent-closure", carries: []RouteCarry[uint64]{routeCarryIdentity, nil, routeCarryIdentity}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newSelectedFixture(t)
			write, ok := NewRouteWrite(fixture.binding, 0)
			if !ok {
				t.Fatal("route write")
			}
			run := NewRun(1, 1)
			ticket := issueSelected(t, run, fixture, fixture.state)
			cells, members, routes := routeCells(fixture, 3)
			var scratch RouteScratch[uint64, uint64]
			outcome := FoldSelectedRouteCarry(ticket, write, &scratch, cells, members, routes, test.carries,
				routeLawReducer{empty: structure.NoSelection, failAt: -1})
			if outcome != structure.Refuse {
				t.Fatalf("a carried routed fold with %s settled %d, want Refuse", test.name, outcome)
			}
		})
	}
}

// TestRoutedCarryOverAnEmptySelectionIsTheEmptyAnswer states that the carry
// changes nothing about the row that publishes at no destination: there is no
// image to carry where there is no coordinate to publish at, so the fold still
// settles whatever the reducer concludes about its own empty selection.
func TestRoutedCarryOverAnEmptySelectionIsTheEmptyAnswer(t *testing.T) {
	fixture := newSelectedFixture(t)
	write, ok := NewRouteWrite(fixture.binding, 0)
	if !ok {
		t.Fatal("route write")
	}
	run := NewRun(1, 1)
	ticket := issueSelected(t, run, fixture, fixture.state)
	var scratch RouteScratch[uint64, uint64]
	outcome := FoldSelectedRouteCarry(ticket, write, &scratch, nil, nil, nil, nil,
		routeLawReducer{empty: structure.NoSelection, failAt: -1})
	if outcome != structure.NoSelection {
		t.Fatalf("an empty carried selection settled %d, want the reducer's own empty answer", outcome)
	}
}
