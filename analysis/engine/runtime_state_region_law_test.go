package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/engine/internal/linkexecutionplan"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

func stateRegionLawID(t *testing.T, label string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("analysis/engine/state-region-law/"+label, nil)
	if !ok {
		t.Fatalf("derive %s", label)
	}
	return id
}

func newStateRegionLawDirectory(t *testing.T, module identity.ContentID) executioncontext.Directory {
	t.Helper()
	link := stateRegionLawID(t, "link")
	left, leftOK := executioncontext.NewContext(link, module, stateRegionLawID(t, "left-actor"), stateRegionLawID(t, "left-representative"))
	right, rightOK := executioncontext.NewContext(link, module, stateRegionLawID(t, "right-actor"), stateRegionLawID(t, "right-representative"))
	leftRoot, leftRootOK := executioncontext.NewRootContext(link, stateRegionLawID(t, "left-root"), left.ID())
	rightRoot, rightRootOK := executioncontext.NewRootContext(link, stateRegionLawID(t, "right-root"), right.ID())
	if !leftOK || !rightOK || !leftRootOK || !rightRootOK {
		t.Fatal("state region contexts")
	}
	directory, ok := executioncontext.Seal(link, []executioncontext.Context{left, right}, []executioncontext.RootContext{leftRoot, rightRoot}, nil)
	if !ok {
		t.Fatal("state region directory")
	}
	return directory
}

func TestLiftStateRegionsUsesContextualWTOForEachStateSCC(t *testing.T) {
	graph, _, _ := newRegionDischargeGraph(t)
	if graph == nil || graph.RegionCount() != 1 || graph.PointCount() != 2 {
		t.Fatalf("state region graph regions=%d points=%d", graph.RegionCount(), graph.PointCount())
	}
	module := stateRegionLawID(t, "module")
	directory := newStateRegionLawDirectory(t, module)
	owner, ownerOK := contextfiber.Mounted(module)
	if !ownerOK {
		t.Fatal("state region owner")
	}
	owners := []contextfiber.PointOwner{owner, owner}
	index, indexOK := contextfiber.New(directory, len(owners), identity.Generation(1))
	if !indexOK {
		t.Fatal("state region index")
	}
	layout, layoutOK := contextfiber.NewLayoutForGraph(index, directory, owners, identity.Generation(1), graph)
	if !layoutOK {
		t.Fatal("state region layout")
	}
	plan, planOK := linkexecutionplan.New(graph, layout, directory, nil)
	if !planOK || plan == nil || plan.StateCount() != 4 || plan.Schedule().RegionCount() != 2 {
		t.Fatalf("state region plan states=%d regions=%d ok=%v", plan.StateCount(), func() int {
			if plan == nil || plan.Schedule() == nil {
				return 0
			}
			return plan.Schedule().RegionCount()
		}(), planOK)
	}
	activePoints := []bool{true, true}
	producerRows, activeStates, rowsOK := buildStateGroupIndex(graph, plan, true, activePoints)
	if !rowsOK || len(activeStates) != int(plan.StateCount()) {
		t.Fatalf("state region producer rows ok=%v states=%d", rowsOK, len(activeStates))
	}
	statePointRows, pointRowsOK := buildStatePointRows(graph, plan, true)
	if !pointRowsOK {
		t.Fatal("state region point rows")
	}
	carrierFixture := newNewtonLawFixture(t, 1)
	runtime := &solverRuntime{
		carrier:        carrierFixture.composition,
		graph:          graph,
		contextLayout:  layout,
		artifactBacked: true,
		executionPlan:  plan,
		producerRows:   producerRows,
		producers:      make([]runtimeProducer, graph.GroupCount()),
		activeStates:   activeStates,
		statePointRows: statePointRows,
	}
	regions, children, pointRegion, activeRegions, events, lifted := liftStateRegions(graph, plan.Schedule(), activeStates, runtime, nil, false)
	if !lifted {
		t.Fatal("state region lift")
	}
	if len(regions) != plan.Schedule().RegionCount() || len(regions) != 2 || len(children) != len(regions) || len(activeRegions) != len(regions) || len(pointRegion) != int(plan.StateCount()) {
		t.Fatalf("state region projection lengths regions=%d children=%d active=%d pointRegion=%d events=%d", len(regions), len(children), len(activeRegions), len(pointRegion), len(events))
	}
	if len(regions) == graph.RegionCount() {
		t.Fatal("state projection copied singular graph region count")
	}
	for regionIndex, region := range regions {
		if !region.active || region.head < 0 || region.head >= int(plan.StateCount()) || len(region.points) == 0 || !activeRegions[regionIndex] {
			t.Fatalf("state region %d malformed active=%v head=%d points=%v", regionIndex, region.active, region.head, region.points)
		}
		static, staticOK := plan.Schedule().RegionAt(regionIndex)
		if !staticOK || static.Head != schedule.Node(region.head) {
			t.Fatalf("state region %d head=%d static=%d/%v", regionIndex, region.head, static.Head, staticOK)
		}
		for _, state := range region.points {
			if state < 0 || state >= int(plan.StateCount()) || pointRegion[state] != regionIndex {
				t.Fatalf("state %d region=%d pointRegion=%d", state, regionIndex, pointRegion[state])
			}
			cell, cellOK := plan.StateAt(contextfiber.StateOrdinal(state))
			if !cellOK || !cell.Available() {
				t.Fatalf("state %d inverse cell", state)
			}
			if _, contextOK := cell.ContextOrdinal(); !contextOK {
				t.Fatalf("state %d lost mounted context", state)
			}
		}
	}
	if len(events) == 0 {
		t.Fatal("state region projection dropped all events")
	}
}
