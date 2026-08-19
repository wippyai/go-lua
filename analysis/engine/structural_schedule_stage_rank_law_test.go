package engine

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
)

// TestConstructedScheduleStageRankIsTotalOverOffendingPlacements keeps the
// schedule law on the sealed, domain-neutral scheduler. Semantic ranks are a
// complete permutation, every point receives one transfer event, and cyclic
// points receive one balanced region bracket.
func TestConstructedScheduleStageRankIsTotalOverOffendingPlacements(t *testing.T) {
	acyclic, err := schedule.PrepareOrdered(4, nil, []int{3, 0, 2, 1})
	if err != nil {
		t.Fatalf("ranked acyclic schedule: %v", err)
	}
	if acyclic.NodeCount() != 4 || acyclic.RegionCount() != 0 || acyclic.EventCount() != 4 {
		t.Fatalf("ranked acyclic shape nodes/events/regions=%d/%d/%d", acyclic.NodeCount(), acyclic.EventCount(), acyclic.RegionCount())
	}
	wantOrder := []schedule.Node{1, 3, 2, 0}
	for index, want := range wantOrder {
		event, ok := acyclic.EventAt(index)
		if !ok || event.Kind != schedule.EventNode || event.Node != want || event.Region != schedule.NoRegion {
			t.Fatalf("ranked event[%d]=%#v, want node %d", index, event, want)
		}
	}

	cyclic, err := schedule.PrepareOrdered(4, []schedule.Edge{{From: 0, To: 1}, {From: 1, To: 2}, {From: 2, To: 1}, {From: 2, To: 3}}, []int{3, 0, 2, 1})
	if err != nil {
		t.Fatalf("ranked cyclic schedule: %v", err)
	}
	seen := make([]int, cyclic.NodeCount())
	for index := range seen {
		seen[index] = -1
	}
	for index := 0; index < cyclic.EventCount(); index++ {
		event, ok := cyclic.EventAt(index)
		if !ok {
			t.Fatalf("missing event %d", index)
		}
		switch event.Kind {
		case schedule.EventNode:
			if event.Node < 0 || int(event.Node) >= len(seen) || seen[event.Node] != -1 {
				t.Fatalf("duplicate or invalid transfer event %#v", event)
			}
			seen[event.Node] = index
		case schedule.EventEnter, schedule.EventExit:
			region, regionOK := cyclic.RegionAt(event.Region)
			if !regionOK || region.Head != event.Node {
				t.Fatalf("event[%d]=%#v has no matching region", index, event)
			}
		default:
			t.Fatalf("unknown schedule event kind %d", event.Kind)
		}
	}
	for node, position := range seen {
		if position < 0 {
			t.Fatalf("node %d has no transfer event", node)
		}
	}
	for index := 0; index < cyclic.RegionCount(); index++ {
		region, ok := cyclic.RegionAt(index)
		if !ok || region.Enter >= region.Exit {
			t.Fatalf("region[%d]=%#v is not bracketed", index, region)
		}
		enter, enterOK := cyclic.EventAt(region.Enter)
		exit, exitOK := cyclic.EventAt(region.Exit)
		if !enterOK || !exitOK || enter.Kind != schedule.EventEnter || exit.Kind != schedule.EventExit || enter.Region != index || exit.Region != index {
			t.Fatalf("region[%d] bracket events=%#v/%#v", index, enter, exit)
		}
	}

	for name, ranks := range map[string][]int{
		"short":   {0, 1},
		"repeat":  {0, 0, 2},
		"missing": {0, 2},
	} {
		if got, err := schedule.PrepareOrdered(3, nil, ranks); got != nil || !errors.Is(err, schedule.ErrInvalidOrder) {
			t.Fatalf("invalid rank %s produced schedule=%v err=%v", name, got, err)
		}
	}
}
