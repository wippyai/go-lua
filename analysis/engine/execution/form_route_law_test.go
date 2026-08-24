package execution

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/generated"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
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

func routeCells(fixture selectedFixture, width int) ([]SelectedCell[uint64], []RouteMember) {
	cells := make([]SelectedCell[uint64], 0, width)
	members := make([]RouteMember, 0, width)
	for index := 0; index < width; index++ {
		cells = append(cells, SelectedCell[uint64]{
			Value:   uint64(index),
			Present: true,
			Tag:     uint64(index) + 1,
			Region:  fixture.whole,
		})
		members = append(members, fixture.member(index, uint64(index)+1))
	}
	return cells, members
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
	cells, members := routeCells(fixture, 3)
	var scratch RouteScratch[uint64, uint64]
	seen := 0
	outcome := FoldSelectedRoute(ticket, write, &scratch, cells, members, routeLawReducer{empty: structure.NoSelection, failAt: -1, seen: &seen})
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
		cells, members := routeCells(fixture, 3)
		var scratch RouteScratch[uint64, uint64]
		seen := 0
		outcome := FoldSelectedRoute(ticket, write, &scratch, cells, members, routeLawReducer{outcome: settled, empty: structure.NoSelection, failAt: 2, seen: &seen})
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

// routedDescriptor builds one routed rule whose route selection is fed by
// earlier joins. selectedFeeds is how many selected reads precede the route
// join, which is the ordinary dependent shape: a route set computed from an
// earlier selection rather than from the candidate alone.
func routedDescriptor(t testing.TB, selectedFeeds int) generated.CompiledRule {
	t.Helper()
	reads := make([]generated.ReadPlan, 0, selectedFeeds+2)
	reads = append(reads, generated.ReadPlan{
		Input: 0, Factor: 1, Axis: 0,
		Relation:          ruleplan.RelationAddr{Axis: 0, Member: 0},
		Key:               ruleplan.ProjectionAddr{Axis: 0, Member: 0},
		Addressing:        ruleplan.RelationAddr{Axis: 0, Member: 0},
		AddressingPresent: true,
		Form:              ruleprogram.Exact,
		PointBound:        ruleprogram.PointBound,
		Contract:          ruleplan.ReadContract{Order: ruleprogram.OrderCanonical, Sparse: ruleprogram.SparseExplicit, OnOpaque: ruleprogram.OnOpaqueRefuse, Multiplicity: ruleprogram.MultiplicityOne},
		RowCapacity:       4, CellCapacity: 4,
	})
	for feed := 0; feed < selectedFeeds; feed++ {
		reads = append(reads, generated.ReadPlan{
			Input: uint32(len(reads)), Factor: 1, Axis: 0,
			Relation:         ruleplan.RelationAddr{Axis: 0, Member: 0},
			Key:              ruleplan.ProjectionAddr{Axis: 0, Member: 0},
			Predicate:        ruleplan.ProjectionAddr{Axis: 0, Member: 1},
			PredicatePresent: true,
			Form:             ruleprogram.Selected,
			PointBound:       ruleprogram.PointBound,
			Contract:         ruleplan.ReadContract{Order: ruleprogram.OrderByTag, Sparse: ruleprogram.SparseDefault, OnOpaque: ruleprogram.OnOpaqueRefuse, Multiplicity: ruleprogram.MultiplicityOne},
			Denominator:      ruleplan.DenominatorAddr{Ordinal: 0, Present: true},
			RowCapacity:      4, CellCapacity: 4,
		})
	}
	routeJoin := uint32(len(reads))
	reads = append(reads, generated.ReadPlan{
		Input: routeJoin, Factor: 2, Axis: 2,
		Relation:         ruleplan.RelationAddr{Axis: 2, Member: 0},
		Key:              ruleplan.ProjectionAddr{Axis: 2, Member: 0},
		Predicate:        ruleplan.ProjectionAddr{Axis: 2, Member: 1},
		PredicatePresent: true,
		Form:             ruleprogram.Selected,
		PointBound:       ruleprogram.PointBound,
		Contract:         ruleplan.ReadContract{Order: ruleprogram.OrderCanonical, Sparse: ruleprogram.SparseExplicit, OnOpaque: ruleprogram.OnOpaqueRefuse, Multiplicity: ruleprogram.MultiplicityOne},
		Denominator:      ruleplan.DenominatorAddr{Ordinal: 1, Present: true},
		RowCapacity:      4, CellCapacity: 4,
	})
	rule, ok := generated.NewPlanCompiledRule(generated.CompiledRuleSpec{
		AxisCount: 3, InputCount: len(reads),
		Candidate: ruleplan.RelationAddr{Axis: 0, Member: 0},
		Reducer:   ruleplan.ReducerAddr{Axis: 2, Member: 0},
		Reads:     reads,
		Outputs: []generated.OutputPlan{{
			Factor: 2, Axis: 2, Address: ruleplan.OutputAddr{Axis: 2, Frame: 0},
			Destination: ruleplan.ProjectionAddr{Axis: 2, Member: 2}, Mode: ruleprogram.ModeRoute,
			RouteJoin: routeJoin, RouteJoinPresent: true,
		}},
	})
	if !ok {
		t.Fatal("routed descriptor")
	}
	return rule
}

// TestSelectedRouteClaimsARouteSelectionFedByAnEarlierSelection states the J
// shape the form exists for. A route set computed from an earlier read's result
// is the whole point of the dependent join, and that earlier read is itself
// often a selection - heap/formalfreeze selects the call's mounted actuals and
// then selects the heap routes those actuals justify. Only the join the output
// publishes over has to be the route; the joins that feed it are ordinary
// reads, and counting selections instead of naming the route one refuses the
// specimen this form was built for.
func TestSelectedRouteClaimsARouteSelectionFedByAnEarlierSelection(t *testing.T) {
	for _, feeds := range []int{0, 1, 2} {
		row, claimed := DeclaredForm(routedDescriptor(t, feeds))
		if !claimed {
			t.Fatalf("a routed rule with %d selected feeds was claimed by no form", feeds)
		}
		if row.Form != FormSelectedRoute {
			t.Fatalf("routed rule with %d selected feeds classified as %q", feeds, row.Form.Name())
		}
	}
}
