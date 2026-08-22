package composite

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// TestRecursiveConstructorContainmentReachesPlacementSummary proves the
// mounted Value -> Heap -> Placement chain, not merely the detached graph
// solver. Each constructor owns one allocation root; the closed Heap rule must
// retain the exact child edges, and return displacement must then propagate
// through those edges without collapsing the three roots into one unit.
func TestRecursiveConstructorContainmentReachesPlacementSummary(t *testing.T) {
	wantDepths := [3]int{1, 1, 1}
	assertPlacementContainmentGraph(t, "placement-recursive-containment", `
local third = {}
local second = { child = third }
local first = { child = second }
return first
`, 3, &wantDepths)
}

// TestPlacementContainmentConvergesForCyclicObjects exercises actual mounted
// Heap recurrence. Visitor-only laws are insufficient: these programs prove
// that self and mutual edges reach a fixed point in the engine and that return
// displacement reaches every allocation without inventing an acyclic depth.
func TestPlacementContainmentConvergesForCyclicObjects(t *testing.T) {
	for _, fixture := range []struct {
		name            string
		source          string
		wantAllocations int
	}{
		{name: "self", wantAllocations: 1, source: `
local root = {}
root.self = root
return root
`},
		{name: "mutual", wantAllocations: 2, source: `
local left = {}
local right = {}
left.other = right
right.other = left
return left
`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			assertPlacementContainmentGraph(t, "placement-containment-cycle-"+fixture.name, fixture.source, fixture.wantAllocations, nil)
		})
	}
}

// TestPlacementContainmentConvergesAcrossControlFlow pins the mounted-point
// closure to authored branch and WTO loop geometry. Both programs assemble the
// same two-root graph; control structure may add Points but cannot change the
// owner-authenticated Placement result.
func TestPlacementContainmentConvergesAcrossControlFlow(t *testing.T) {
	for _, fixture := range []struct {
		name   string
		source string
	}{
		{name: "branch", source: `
local child = {}
local root = {}
if root then root.child = child end
return root
`},
		{name: "loop", source: `
local child = {}
local root = {}
for index = 1, 2 do root.child = child end
return root
`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			wantDepths := [3]int{1, 1, 0}
			assertPlacementContainmentGraph(t, "placement-containment-control-"+fixture.name, fixture.source, 2, &wantDepths)
		})
	}
}

func assertPlacementContainmentGraph(t testing.TB, name, source string, wantAllocations int, wantDepths *[3]int) {
	t.Helper()
	record := mountedRecord(t, name, source)
	bound := materializerBinding(t, record)
	committed, sites := queryCanonicalProgram(t, record, bound)
	sealed, failure, sealedOK := committed.Seal(nil)
	if !sealedOK || sealed == nil {
		t.Fatalf("seal recursive Placement program: %v", failure)
	}
	state, status, report := sealed.SolveWithReport(context.Background())
	if status != engine.SolveComplete || state == nil {
		t.Fatalf("solve recursive Placement program: status=%v reason=%v failure=%v", status, report.Reason(), report.Failure())
	}
	published, publishedOK := sealed.PublishedSnapshot(state)
	if !publishedOK {
		t.Fatal("recursive Placement solve published no snapshot")
	}
	view := published.Snapshot()
	plan, planOK := snapshot.OpenQuery[identity.ContentID, engine.Answer](&view, published.QueryFamily())
	publications, publicationsOK := bound.QueryPublications(committed, sites)
	if !planOK || !publicationsOK {
		t.Fatal("recursive Placement query publication unavailable")
	}

	bestDepths, bestOwned, bestMutable := [3]int{}, 0, 0
	for _, publication := range publications {
		if publication.Site.Family != QueryFamilyPlacementSummary {
			continue
		}
		answer, read := snapshot.Query(&view, plan, publication.Key)
		if read != snapshot.ReadHit || !answer.Available() {
			continue
		}
		cell, cellOK := publication.CanonicalCell(answer)
		result, resultOK := placementdomain.DecodeSummaryResult(record.PlacementSchema, cell.Present(), cell.RowCount(), cell.Payload())
		if !cellOK || !resultOK || result.AllocationCount() != wantAllocations {
			continue
		}
		depths, owned, mutable := [3]int{}, 0, 0
		rowsOK := true
		iterator := result.Allocations()
		for {
			allocation, available := iterator.Next()
			if !available {
				break
			}
			class, classOK := allocation.Placement()
			frozen, frozenOK := allocation.DeepFrozen()
			if !classOK || !frozenOK {
				rowsOK = false
				break
			}
			if wantDepths != nil {
				depth, depthOK := allocation.Depth()
				if !depthOK || depth > 2 {
					rowsOK = false
					break
				}
				depths[depth]++
			}
			if class == placementdomain.OwnedHeap {
				owned++
			}
			if frozen == placementdomain.EvidenceRefuted {
				mutable++
			}
		}
		bestDepths, bestOwned, bestMutable = depths, owned, mutable
		depthsOK := wantDepths == nil || depths == *wantDepths
		if rowsOK && depthsOK && owned == wantAllocations && mutable == wantAllocations {
			return
		}
	}
	t.Fatalf("no typed Placement point retained the graph: depths=%v owned=%d mutable=%d want-depths=%v want-allocations=%d", bestDepths, bestOwned, bestMutable, wantDepths, wantAllocations)
}
