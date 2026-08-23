package execution

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// routeLawReducer is a concrete reducer type, not a closure or an interface
// value: the fold consumes it as a type parameter, so these calls are the same
// static direct calls a generated family's own reducer gets.
type routeLawReducer struct {
	outcome structure.ReductionOutcome
	empty   structure.ReductionOutcome
	failAt  int
	seen    *int
}

func (reducer routeLawReducer) Reduce(cell SelectedCell[uint64]) (uint64, structure.ReductionOutcome) {
	index := 0
	if reducer.seen != nil {
		index = *reducer.seen
		*reducer.seen++
	}
	if reducer.failAt >= 0 && index == reducer.failAt {
		return 0, reducer.outcome
	}
	return cell.Tag + 100, structure.Concrete
}

func (reducer routeLawReducer) Empty() structure.ReductionOutcome { return reducer.empty }

func routeCells(fixture selectedFixture, width int) ([]SelectedCell[uint64], []carrier.Target) {
	cells := make([]SelectedCell[uint64], 0, width)
	targets := make([]carrier.Target, 0, width)
	for index := 0; index < width; index++ {
		cells = append(cells, SelectedCell[uint64]{
			Value:   uint64(index),
			Present: true,
			Tag:     uint64(index) + 1,
			Region:  fixture.whole,
		})
		targets = append(targets, fixture.targets[index])
	}
	return cells, targets
}

// TestRoutedPublicationIsOneOutputAcrossEveryRoute states the WR shape: a
// routed write with many destinations is still one output. Every route stages
// into the same patch and the invocation drains exactly one, which is what lets
// a routed row share the single-output invocation contract with an exact write
// instead of needing an output slot per destination.
func TestRoutedPublicationIsOneOutputAcrossEveryRoute(t *testing.T) {
	fixture := newSelectedFixture(t)
	write, ok := NewRouteWrite(fixture.binding, 0)
	if !ok || !write.Valid() {
		t.Fatal("route write")
	}
	run := NewRun(1, 1)
	ticket := issueSelected(t, run, fixture, fixture.state)
	cells, targets := routeCells(fixture, 3)
	var scratch RouteScratch[uint64, uint64]
	seen := 0
	outcome := FoldSelectedRoute(ticket, write, &scratch, cells, targets, routeLawReducer{empty: structure.NoSelection, failAt: -1, seen: &seen})
	if outcome != structure.Concrete {
		t.Fatalf("routed outcome = %d, want Concrete", outcome)
	}
	if seen != 3 {
		t.Fatalf("reducer saw %d routes, want one call per route", seen)
	}
	if !run.Submit(&ticket, outcome) {
		t.Fatal("submit")
	}
	patches := make([]carrier.Patch, 1)
	disposition, count, drained := run.Drain(patches)
	if !drained || disposition != structure.Concrete || count != 1 {
		t.Fatalf("drain = (%d,%d,%v), want one patch for the whole routed row", disposition, count, drained)
	}
}

// TestRoutedRowWithNoRouteSettlesTheEmptySelection states where NoSelection
// comes from. A derived relation that produced no route is an explicitly empty
// selection over a population that exists, which is a different answer from
// refusing to look and from an absent candidate. The reducer is the one thing
// that knows its own selection is empty, so it is the one thing that concludes
// it - and no patch is opened at a coordinate that was never selected.
func TestRoutedRowWithNoRouteSettlesTheEmptySelection(t *testing.T) {
	fixture := newSelectedFixture(t)
	write, ok := NewRouteWrite(fixture.binding, 0)
	if !ok {
		t.Fatal("route write")
	}
	run := NewRun(1, 1)
	ticket := issueSelected(t, run, fixture, fixture.state)
	var scratch RouteScratch[uint64, uint64]
	outcome := FoldSelectedRoute(ticket, write, &scratch, nil, nil, routeLawReducer{empty: structure.NoSelection, failAt: -1})
	if outcome != structure.NoSelection {
		t.Fatalf("empty routed outcome = %d, want NoSelection", outcome)
	}
	if run.hasOutput() {
		t.Fatal("an empty selection published an output")
	}
	if !run.Submit(&ticket, outcome) {
		t.Fatal("submit")
	}
	disposition, count, drained := run.Drain(nil)
	if !drained || disposition != structure.NoSelection || count != 0 {
		t.Fatalf("drain = (%d,%d,%v), want NoSelection carrying no patch", disposition, count, drained)
	}
}

// TestRoutedEmptySelectionMayNotSettleConcrete states that an empty selection
// cannot be a proved fact. Concrete means a fact was published, and a row with
// no route has no coordinate to publish it at, so a reducer claiming Concrete
// over an empty selection is refused rather than believed.
func TestRoutedEmptySelectionMayNotSettleConcrete(t *testing.T) {
	fixture := newSelectedFixture(t)
	write, ok := NewRouteWrite(fixture.binding, 0)
	if !ok {
		t.Fatal("route write")
	}
	run := NewRun(1, 1)
	ticket := issueSelected(t, run, fixture, fixture.state)
	var scratch RouteScratch[uint64, uint64]
	if outcome := FoldSelectedRoute(ticket, write, &scratch, nil, nil, routeLawReducer{empty: structure.Concrete, failAt: -1}); outcome != structure.Refuse {
		t.Fatalf("empty Concrete outcome = %d, want Refuse", outcome)
	}
}

// TestRoutedRowRefusesToPublishHalfAStrongWrite states the atomicity of a
// routed publication. The destinations of one row are one judgment: if any
// route does not settle Concrete, the row settles that disposition and every
// already-staged route is discarded. Publishing the prefix would leave the axis
// holding a strong write the rule never concluded.
func TestRoutedRowRefusesToPublishHalfAStrongWrite(t *testing.T) {
	for _, settled := range []structure.ReductionOutcome{structure.NoCandidate, structure.AuthenticatedOpaque, structure.Refuse} {
		fixture := newSelectedFixture(t)
		write, ok := NewRouteWrite(fixture.binding, 0)
		if !ok {
			t.Fatal("route write")
		}
		run := NewRun(1, 1)
		ticket := issueSelected(t, run, fixture, fixture.state)
		cells, targets := routeCells(fixture, 3)
		var scratch RouteScratch[uint64, uint64]
		seen := 0
		outcome := FoldSelectedRoute(ticket, write, &scratch, cells, targets, routeLawReducer{outcome: settled, empty: structure.NoSelection, failAt: 2, seen: &seen})
		if outcome != settled {
			t.Fatalf("routed outcome = %d, want the route's own %d", outcome, settled)
		}
		if run.hasOutput() {
			t.Fatalf("a row settling %d published a partial strong write", settled)
		}
	}
}

// TestRoutedWriteRefusesADestinationItsFactorDoesNotOwn states the owner fence
// on the write side. A route's destination is drawn from the presealed strong
// target universe of the Factor being written; a target from anywhere else is
// refused at the boundary, so no fold has to re-prove destination ownership.
func TestRoutedWriteRefusesADestinationItsFactorDoesNotOwn(t *testing.T) {
	fixture := newSelectedFixture(t)
	foreign := newSelectedFixture(t)
	write, ok := NewRouteWrite(fixture.binding, 0)
	if !ok {
		t.Fatal("route write")
	}
	run := NewRun(1, 1)
	ticket := issueSelected(t, run, fixture, fixture.state)
	var scratch RouteScratch[uint64, uint64]
	if write.Stage(ticket, &scratch, foreign.targets[0], fixture.whole, 5) {
		t.Fatal("a foreign Factor's target was staged as a route destination")
	}
	if run.hasOutput() {
		t.Fatal("a refused stage published an output")
	}
}

// TestRoutedWriteRefusesASupportRowItDidNotObserve states that a route
// publishes under the support row of the member it was read at. An empty or
// unentailed region is refused, so a routed write cannot widen its own
// applicability past the evidence that selected it.
func TestRoutedWriteRefusesASupportRowItDidNotObserve(t *testing.T) {
	fixture := newSelectedFixture(t)
	write, ok := NewRouteWrite(fixture.binding, 0)
	if !ok {
		t.Fatal("route write")
	}
	run := NewRun(1, 1)
	ticket := issueSelected(t, run, fixture, fixture.state)
	var scratch RouteScratch[uint64, uint64]
	if write.Stage(ticket, &scratch, fixture.targets[0], support.Mask{}, 5) {
		t.Fatal("a route staged under an unauthenticated support row")
	}
}
