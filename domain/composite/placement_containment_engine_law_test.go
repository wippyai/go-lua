package composite

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// TestPlacementRecursiveContainmentPublishesThroughTheEngine drives one
// recursive Heap graph through the real Link, binding, solve, and publication
// path. The Placement query is deliberately read as its heterogeneous query
// answer: its Placement projection supplies classes while its complete Heap
// projection supplies the authenticated containment-depth evidence. The final
// assertions consume only the typed canonical cell emitted by Placement's
// publication contract; no raw Heap relation or legacy result is injected.
func TestPlacementRecursiveContainmentPublishesThroughTheEngine(t *testing.T) {
	record := mountedRecord(t, "placement-recursive-engine", `
local first = { value = 1 }
local second = { child = first }
local third = { child = second }
return third
`)
	if !record.HeapSchema.Valid() || !record.PlacementSchema.Valid() || record.PlacementSchema.Heap() != record.HeapSchema {
		t.Fatal("mounted record did not retain the canonical Heap/Placement authority pair")
	}

	allocationIDs := make(map[identity.ContentID]struct{})
	for index := 0; index < record.HeapSchema.KeyCount(); index++ {
		key, keyOK := record.HeapSchema.KeyAt(index)
		if !keyOK {
			t.Fatalf("Heap key %d is unavailable", index)
		}
		if key.Kind() != heapdomain.RootAllocation {
			continue
		}
		id, idOK := key.ContentID()
		if !idOK || !id.Available() {
			t.Fatalf("allocation key %d has no canonical identity", index)
		}
		allocationIDs[id] = struct{}{}
	}
	if len(allocationIDs) < 3 {
		t.Fatalf("recursive fixture sealed %d allocation roots, want at least three", len(allocationIDs))
	}

	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("program schema compilation is unavailable")
	}
	bound, bindFailure := BindProgram(compilation, record)
	if bindFailure.Available() || bound == nil || !bound.Available() || bound.PlacementQuery() == nil {
		t.Fatalf("bind the canonical Placement/Heap query: %v", bindFailure)
	}
	committed, table := queryCanonicalProgram(t, record, bound)

	sealed, sealFailure, sealedOK := committed.Seal(nil)
	if !sealedOK || sealed == nil {
		t.Fatalf("seal the committed recursive program: %v", sealFailure)
	}
	state, solveStatus := sealed.Solve(context.Background())
	if solveStatus != engine.SolveComplete || state == nil {
		t.Fatalf("solve the recursive Placement query: status=%v state=%v", solveStatus, state)
	}
	published, publishedOK := sealed.PublishedSnapshot(state)
	if !publishedOK {
		t.Fatal("engine published no solve snapshot")
	}
	view := published.Snapshot()
	queryPlan, queryPlanOK := snapshot.OpenQuery[identity.ContentID, engine.Answer](&view, published.QueryFamily())
	if !queryPlanOK {
		t.Fatal("open the engine's published query family")
	}
	publications, publicationsOK := bound.QueryPublications(committed, table)
	if !publicationsOK {
		t.Fatal("resolve typed query publications from the committed program")
	}

	placementHits := 0
	maxDepth := uint32(0)
	depthRows := 0
	for _, publication := range publications {
		if publication.Site.Family != QueryFamilyPlacementSummary {
			continue
		}
		answer, answerStatus := snapshot.Query(&view, queryPlan, publication.Key)
		if answerStatus == snapshot.ReadProvenAbsent {
			continue
		}
		if answerStatus != snapshot.ReadHit || !answer.Available() {
			t.Fatalf("Placement query publication status = %s, want a typed hit", answerStatus)
		}
		cell, cellOK := publication.CanonicalCell(answer)
		if !cellOK || !cell.Available() || cell.ContractID() != publication.Contract().ContentID() {
			t.Fatal("Placement query answer did not close under its typed publication contract")
		}
		result, resultOK := placementdomain.DecodeSummaryResult(record.PlacementSchema, cell.Present(), cell.RowCount(), cell.Payload())
		if !resultOK || !result.Available() || result.SchemaID() != record.PlacementSchema.ContentID() {
			t.Fatal("typed Placement publication did not decode under the mounted Placement schema")
		}
		if result.AllocationCount() != len(allocationIDs) {
			t.Fatalf("published allocation denominator = %d, want canonical Heap denominator %d", result.AllocationCount(), len(allocationIDs))
		}

		seen := make(map[identity.ContentID]struct{}, result.AllocationCount())
		rows := result.Allocations()
		for index := 0; ; index++ {
			allocation, allocationOK := rows.Next()
			if !allocationOK {
				break
			}
			if !allocation.Available() {
				t.Fatalf("published allocation row %d is unavailable", index)
			}
			id := allocation.AllocationID()
			if _, known := allocationIDs[id]; !known {
				t.Fatalf("published allocation row %d carries a foreign Heap identity", index)
			}
			if _, duplicate := seen[id]; duplicate {
				t.Fatalf("published allocation row %s is duplicated", id)
			}
			seen[id] = struct{}{}
			ownerID, ownerOK := allocation.OwnerIdentity()
			if !ownerOK || ownerID != id {
				t.Fatalf("published allocation %s lost its owner-authenticated Heap identity", id)
			}
			depth, depthOK := allocation.Depth()
			if !depthOK {
				continue
			}
			depthRows++
			if depth > maxDepth {
				maxDepth = depth
			}
		}
		if len(seen) != len(allocationIDs) {
			t.Fatalf("published allocation identities = %d, want %d", len(seen), len(allocationIDs))
		}
		placementHits++
	}

	if placementHits == 0 {
		t.Fatal("recursive fixture published no Placement summary hit")
	}
	if depthRows == 0 || maxDepth < 2 {
		t.Fatalf("typed Placement publication carried no recursive containment depth: rows=%d max=%d", depthRows, maxDepth)
	}
}
