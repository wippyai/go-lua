package composite

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/plane"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	"github.com/wippyai/go-lua/analysis/snapshot"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// TestReturnPlacementPublishesOwnedHeapThroughTheCompositeEngine is the
// engine-level return boundary law. The source is lowered, linked, mounted,
// admitted into the canonical Program, solved, queried, and closed through
// Placement's schema-bound result decoder. No Placement observation is
// supplied by the test: the returned root must be the demand emitted by the
// mounted placement-return-escape rule itself.
func TestReturnPlacementPublishesOwnedHeapThroughTheCompositeEngine(t *testing.T) {
	for _, fixture := range []struct {
		name               string
		source             string
		alternate          bool
		wantReturnBranches int
	}{
		{name: "exact", source: "local returned = {}; return returned", wantReturnBranches: 1},
		// Both branches return the same allocation. The closed condition keeps
		// fixture construction entirely canonical while giving the engine two
		// independently issued return demands to join.
		{name: "alternate-return-paths", source: "local returned = {}; if returned then return returned else return returned end", alternate: true, wantReturnBranches: 2},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			record := mountedRecord(t, "return-placement-engine-"+fixture.name, fixture.source)
			bound := materializerBinding(t, record)
			committed, table := queryCanonicalProgram(t, record, bound)
			sealed, failure, ok := committed.Seal(nil)
			if !ok || sealed == nil {
				t.Fatalf("seal: %v", failure)
			}
			state, status, report := sealed.SolveWithReport(context.Background())
			if status != engine.SolveComplete || state == nil {
				t.Fatalf("solve: status=%v reason=%v failure=%v point=%v group=%v member=%v rule=%v", status, report.Reason(), report.Failure(), report.Point(), report.Group(), report.Member(), report.Rule())
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
			summaryLayout, summaryLayoutOK := queryResultLayout(bound.catalog, valuedomain.SummaryResultFamily)
			if !summaryLayoutOK {
				t.Fatal("the compilation sealed no value-summary layout")
			}
			publications, ok := bound.QueryPublications(committed, table)
			if !ok {
				t.Fatal("query publications")
			}

			allocationIDs := heapAllocationIDsForReturnLaw(t, record)
			returnTargets := returnEscapeTargetsForLaw(t, record)
			if len(allocationIDs) != 1 {
				t.Fatalf("minimal return fixture sealed %d allocation roots, want one returned table root", len(allocationIDs))
			}
			var returnedID identity.ContentID
			for id := range allocationIDs {
				returnedID = id
			}
			if len(returnTargets) != fixture.wantReturnBranches {
				t.Fatalf("return escape branches = %d, want %d", len(returnTargets), fixture.wantReturnBranches)
			}
			if fixture.alternate {
				if returnTargets[0].point == returnTargets[1].point {
					t.Fatal("alternate returns did not retain two distinct points for one allocation")
				}
			}

			byPoint := make(map[identity.ContentID]QueryPublication)
			valueByPoint := make(map[identity.ContentID]QueryPublication)
			for _, publication := range publications {
				if publication.Site.Family == QueryFamilyPlacementSummary {
					byPoint[publication.Site.Point] = publication
				} else if publication.Site.Family == QueryFamilyValueSummary {
					valueByPoint[publication.Site.Point] = publication
				}
			}
			joined := placementdomain.BottomFact()
			for _, target := range returnTargets {
				publication, publicationOK := byPoint[target.point]
				if !publicationOK {
					t.Fatalf("return escape point %s has no typed Placement publication", target.point)
				}
				answer, read := snapshot.Query(&view, plan, publication.Key)
				if read != snapshot.ReadHit || !answer.Available() {
					t.Fatalf("return escape point %s query status=%s available=%t", target.point, read, answer.Available())
				}
				cell, cellOK := publication.CanonicalCell(answer)
				if !cellOK || !cell.Available() || cell.ContractID() != publication.Contract().ContentID() {
					t.Fatalf("return escape point %s did not close its typed result contract", target.point)
				}
				result, resultOK := placementdomain.DecodeSummaryResult(record.PlacementSchema, cell.Present(), cell.RowCount(), cell.Payload())
				if !resultOK || !result.Available() || result.SchemaID() != record.PlacementSchema.ContentID() {
					t.Fatalf("return escape point %s did not decode under the mounted Placement schema", target.point)
				}
				if result.AllocationCount() != len(allocationIDs) {
					t.Fatalf("return escape point %s allocation denominator=%d, want Heap denominator=%d", target.point, result.AllocationCount(), len(allocationIDs))
				}
				rows := placementFactsForReturnLaw(t, result)
				if len(rows) != len(allocationIDs) {
					t.Fatalf("return escape point %s decoded rows=%d, want Heap denominator=%d", target.point, len(rows), len(allocationIDs))
				}
				for allocationID := range allocationIDs {
					if _, present := rows[allocationID]; !present {
						t.Fatalf("return escape point %s omitted Heap allocation %s", target.point, allocationID)
					}
				}
				got := rows[returnedID]
				want := placementdomain.Fact{Class: placementdomain.OwnedHeap, RetainEscape: placementdomain.EvidenceProven}
				if got != want {
					valueState := returnBoundaryMemberStatesForLaw(t, summaryLayout, &view, plan, valueByPoint[target.point], record.ValueSchema, target.members)
					t.Fatalf("return escape point %s allocation %s=%s, want exact %s demand (boundary members %s)", target.point, returnedID, got, want, valueState)
				}
				if !placementdomain.LessOrEqFact(joined, got) {
					t.Fatalf("alternate return join descended from %s to %s at point %s", joined, got, target.point)
				}
				joined = placementdomain.JoinFact(joined, got)
			}
			want := placementdomain.Fact{Class: placementdomain.OwnedHeap, RetainEscape: placementdomain.EvidenceProven}
			if joined != want {
				t.Fatalf("joined return demand=%s, want %s", joined, want)
			}
		})
	}
}

type returnEscapeTarget struct {
	point identity.ContentID
	// members are the Value coordinates the return-escape rule actually reads:
	// the boundary's fixed member set. The boundary Root is the aggregate the
	// members belong to and carries no Value fact of its own, so a diagnostic
	// that inspected it would report on a coordinate nothing writes.
	members []valuedomain.Coordinate
}

func returnEscapeTargetsForLaw(t testing.TB, record LinkInputs) []returnEscapeTarget {
	t.Helper()
	var targets []returnEscapeTarget
	for _, mount := range record.Artifacts {
		count, ok := mount.Program.RuleOccurrenceCount()
		if !ok {
			t.Fatal("rule occurrences")
		}
		for index := 0; index < count; index++ {
			row, ok := mount.Program.RuleOccurrenceAt(index)
			if !ok || string(row.Key()) != "placement-return-escape" || row.Stage() != programissuance.StageSuccessor {
				continue
			}
			point := row.PointID()
			if !point.Available() {
				t.Fatal("return escape point")
			}
			occurrenceOrdinal, occurrenceOK := row.Occurrence()
			occurrence, occurrenceRowOK := mount.Program.OccurrenceAt(int(occurrenceOrdinal))
			if !occurrenceOK || !occurrenceRowOK || occurrence.Kind() != programschema.OccurrenceReturnBoundary {
				t.Fatal("return escape row is not backed by a return boundary")
			}
			boundary, boundaryOK := record.ValueSchema.ReturnBoundary(mount.ModuleKey, occurrence.ID())
			if !boundaryOK || boundary.MemberCount() == 0 {
				t.Fatal("return boundary has no canonical Value member set")
			}
			members := make([]valuedomain.Coordinate, 0, boundary.MemberCount())
			for memberIndex := 0; memberIndex < boundary.MemberCount(); memberIndex++ {
				member, memberOK := boundary.MemberAt(memberIndex)
				coordinate, coordinateOK := member.Coordinate()
				if !memberOK || !coordinateOK {
					t.Fatalf("return boundary member %d has no Value coordinate", memberIndex)
				}
				members = append(members, coordinate)
			}
			targets = append(targets, returnEscapeTarget{point: point, members: members})
		}
	}
	if len(targets) == 0 {
		t.Fatal("no return escape points")
	}
	return targets
}

func heapAllocationIDsForReturnLaw(t testing.TB, record LinkInputs) map[identity.ContentID]struct{} {
	t.Helper()
	ids := make(map[identity.ContentID]struct{})
	for dense := 0; dense < record.HeapSchema.KeyCount(); dense++ {
		key, keyOK := record.HeapSchema.KeyAt(dense)
		if !keyOK || key.Kind() != heapdomain.RootAllocation {
			continue
		}
		id, idOK := key.ContentID()
		receipt, receiptOK := record.ValueSchema.AllocationResultFor(key)
		if !idOK || !id.Available() || !receiptOK || receipt == nil {
			t.Fatal("Heap allocation has no canonical identity")
		}
		ids[id] = struct{}{}
	}
	if len(ids) == 0 {
		t.Fatal("fixture sealed no Heap allocation roots")
	}
	return ids
}

func placementFactsForReturnLaw(t testing.TB, result placementdomain.SummaryResult) map[identity.ContentID]placementdomain.Fact {
	t.Helper()
	rows := make(map[identity.ContentID]placementdomain.Fact, result.AllocationCount())
	iterator := result.Allocations()
	for {
		allocation, next := iterator.Next()
		if !next {
			break
		}
		if !allocation.Available() {
			t.Fatal("typed Placement allocation row is unavailable")
		}
		present, presentOK := allocation.Present()
		if !presentOK || !present {
			t.Fatal("typed Placement allocation row has no published class")
		}
		fact, decodedOK := allocation.Fact()
		if !decodedOK {
			t.Fatal("typed Placement allocation row has no canonical fact")
		}
		id := allocation.AllocationID()
		if _, duplicate := rows[id]; duplicate {
			t.Fatalf("typed Placement allocation row %s is duplicated", id)
		}
		rows[id] = fact
	}
	return rows
}

// returnBoundaryMemberStatesForLaw states what the return-escape rule actually
// reads at this point: the published Value fact of every fixed member of the
// boundary, in member order. A diagnostic reports the coordinates the judgment
// consumed; the boundary Root is an aggregate no rule writes, so reading it
// would report "absent" at every point and carry no signal.
func returnBoundaryMemberStatesForLaw(t testing.TB, layout *plane.Sealed, view *snapshot.Snapshot, plan snapshot.QueryPlan[identity.ContentID, engine.Answer], publication QueryPublication, schema *valuedomain.Schema, members []valuedomain.Coordinate) string {
	t.Helper()
	states := make([]string, 0, len(members))
	for index, coordinate := range members {
		states = append(states, fmt.Sprintf("[%d]=%s", index, returnBoundaryValueStateForLaw(t, layout, view, plan, publication, schema, coordinate)))
	}
	if len(states) == 0 {
		return "no members"
	}
	return strings.Join(states, " ")
}

func returnBoundaryValueStateForLaw(t testing.TB, layout *plane.Sealed, view *snapshot.Snapshot, plan snapshot.QueryPlan[identity.ContentID, engine.Answer], publication QueryPublication, schema *valuedomain.Schema, coordinate valuedomain.Coordinate) string {
	t.Helper()
	index, indexOK := schema.CoordinateIndex(coordinate)
	if !indexOK || !publication.Key.Available() {
		return "unaddressable"
	}
	answer, read := snapshot.Query(view, plan, publication.Key)
	if read != snapshot.ReadHit || !answer.Available() {
		return "query-absent"
	}
	cell, cellOK := publication.CanonicalCell(answer)
	if !cellOK {
		return "cell-invalid"
	}
	summaryView, refusal := plane.Admit(layout, cell.Present(), cell.RowCount(), cell.Payload())
	if refusal.Available() {
		return "decode-invalid"
	}
	// A published row is addressed by the portable identity it carries, never
	// by the schema's private dense position: the wire's row order is a
	// function of the coordinates it holds, so the two orders are unrelated.
	for position := 0; position < summaryView.RowCount(); position++ {
		row, rowOK := summaryView.At(position)
		if !rowOK {
			return "coordinate-missing"
		}
		local, resolved := schema.CoordinateForID(row.ID())
		if !resolved {
			return "coordinate-unresolved"
		}
		dense, denseOK := schema.CoordinateIndex(local)
		if !denseOK || dense != index {
			continue
		}
		if !row.Written() {
			return "absent"
		}
		if row.Flag(valuedomain.SummaryColumnTop) {
			return "top"
		}
		// A written row with no atoms is the Value owner's Bottom: the
		// coordinate was published and the judgment named no alternative at
		// all. That is a different answer from an unwritten coordinate, and
		// the two must not both report as absence.
		if row.Count() == 0 {
			return "bottom"
		}
		return "present"
	}
	return "coordinate-missing"
}
