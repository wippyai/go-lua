package composite

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// TestReturnPlacementHoldsInsideTheArmThatIsNotTakenLast is the branch half of
// the return boundary law, stated on the one shape the composite law's joined
// fixture cannot isolate: a value allocated BEFORE a branch and returned from
// an arm that is not the last one.
//
// Inside an arm the Value of a coordinate written before the branch is held on
// that arm's own guard region and is absent on the complementary one, so the
// return-escape rule's member read spans a partitioned region. The demand it
// publishes is what the coordinate HOLDS, never which half of that partition
// the guard's canonical order enumerates first - so this arm's return owns its
// heap exactly as the sole return of a straight-line body does.
func TestReturnPlacementHoldsInsideTheArmThatIsNotTakenLast(t *testing.T) {
	const source = "local returned = {}; if returned then return returned else return 1 end"
	record := mountedRecord(t, "return-placement-branch-arm", source)
	bound := materializerBinding(t, record)
	committed, table := queryCanonicalProgram(t, record, bound)
	sealed, failure, ok := committed.Seal(nil)
	if !ok || sealed == nil {
		t.Fatalf("seal: %v", failure)
	}
	state, status, report := sealed.SolveWithReport(context.Background())
	if status != engine.SolveComplete || state == nil {
		t.Fatalf("solve: status=%v reason=%v failure=%v", status, report.Reason(), report.Failure())
	}
	published, ok := sealed.PublishedSnapshot(state)
	if !ok {
		t.Fatal("published snapshot")
	}
	view := published.Snapshot()
	plan, ok := snapshot.OpenQuery[identity.ContentID, engine.Answer](&view, published.QueryFamily())
	if !ok {
		t.Fatal("query plan")
	}
	publications, ok := bound.QueryPublications(committed, table)
	if !ok {
		t.Fatal("query publications")
	}
	byPoint := make(map[identity.ContentID]QueryPublication)
	for _, publication := range publications {
		if publication.Site.Family == QueryFamilyPlacementSummary {
			byPoint[publication.Site.Point] = publication
		}
	}

	allocationIDs := heapAllocationIDsForReturnLaw(t, record)
	if len(allocationIDs) != 1 {
		t.Fatalf("fixture sealed %d allocation roots, want one returned table root", len(allocationIDs))
	}
	var returnedID identity.ContentID
	for id := range allocationIDs {
		returnedID = id
	}
	targets := returnEscapeTargetsForLaw(t, record)
	if len(targets) != 2 {
		t.Fatalf("return escape branches = %d, want the two arms", len(targets))
	}

	// The arm that returns the allocation is the one whose return boundary the
	// Program issued first; the second arm returns a constant and names no
	// allocation at all.
	target := targets[0]
	publication, publicationOK := byPoint[target.point]
	if !publicationOK {
		t.Fatalf("return escape point %s has no typed Placement publication", target.point)
	}
	answer, read := snapshot.Query(&view, plan, publication.Key)
	if read != snapshot.ReadHit || !answer.Available() {
		t.Fatalf("return escape point %s query status=%s available=%t", target.point, read, answer.Available())
	}
	cell, cellOK := publication.CanonicalCell(answer)
	if !cellOK || !cell.Available() {
		t.Fatalf("return escape point %s did not close its typed result contract", target.point)
	}
	result, resultOK := placementdomain.DecodeSummaryResult(record.PlacementSchema, cell.Present(), cell.RowCount(), cell.Payload())
	if !resultOK || !result.Available() {
		t.Fatalf("return escape point %s did not decode under the mounted Placement schema", target.point)
	}
	rows := placementFactsForReturnLaw(t, result)
	got, present := rows[returnedID]
	if !present {
		t.Fatalf("return escape point %s omitted Heap allocation %s", target.point, returnedID)
	}
	want := placementdomain.Fact{Class: placementdomain.OwnedHeap, RetainEscape: placementdomain.EvidenceProven}
	if got != want {
		t.Fatalf("arm return allocation %s=%s, want exact %s demand", returnedID, got, want)
	}
}
