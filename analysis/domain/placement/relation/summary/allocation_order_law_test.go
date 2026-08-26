package summary_test

import (
	"testing"

	heapsummary "github.com/wippyai/go-lua/analysis/domain/heap/relation/summary"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	"github.com/wippyai/go-lua/domain/placement/suspension"
	"github.com/wippyai/go-lua/domain/relationfixture"
)

// TestPlacementSummaryAuthoritiesShareCanonicalAllocationOrder proves the
// only positional contract used by the allocation summary. Heap owns one
// dense allocation prefix; Placement and its suspension-evidence producer
// consume that same prefix through Placement.Schema.KeyAt. The full Heap
// carrier may append Boot roots, but those roots never become allocation
// coordinates or summary rows.
func TestPlacementSummaryAuthoritiesShareCanonicalAllocationOrder(t *testing.T) {
	fixture := relationfixture.New(t)
	placement, ok := placementdomain.NewSchema(fixture.Heap)
	if !ok {
		t.Fatal("Placement schema")
	}
	metadata, ok := heapsummary.NewPublisher(fixture.Heap)
	if !ok {
		t.Fatal("Heap summary publisher")
	}

	allocationCount := fixture.Heap.AllocationKeyCount()
	if allocationCount == 0 {
		t.Fatal("fixture has no allocation coordinates")
	}
	if placement.KeyCount() != allocationCount || metadata.Len() != allocationCount {
		t.Fatalf("allocation widths placement=%d metadata=%d heap=%d, want %d", placement.KeyCount(), metadata.Len(), allocationCount, allocationCount)
	}
	if fixture.Heap.KeyCount() != allocationCount+fixture.Heap.BootCount() {
		t.Fatalf("Heap full-root width=%d, allocation prefix=%d and Boot tail=%d", fixture.Heap.KeyCount(), allocationCount, fixture.Heap.BootCount())
	}

	// Build both factor views through their production owner folds. Distinct
	// evidence values make a positional shift observable without adding an
	// identity-bearing side relation to either factor.
	observation := placementdomain.BeginPlacementSummary(placement)
	observation, ok = placementdomain.AccumulatePlacementSummaryRows(placement, observation, allocationCount,
		func(int) (placementdomain.Fact, bool, bool) {
			return placementdomain.DefaultFact(), true, true
		})
	if !ok {
		t.Fatal("Placement allocation facts")
	}
	evidenceAt := func(index int) (suspension.Evidence, bool, bool) {
		if index%2 == 0 {
			return suspension.EvidenceProven, true, true
		}
		return suspension.EvidenceRefuted, true, true
	}
	withEvidence, ok := suspension.AccumulatePlacementSummarySuspensionRows(placement, observation, allocationCount, evidenceAt)
	if !ok {
		t.Fatal("Suspension evidence")
	}

	for index := 0; index < allocationCount; index++ {
		allocationKey, allocationOK := fixture.Heap.AllocationKeyAt(index)
		placementKey, placementOK := placement.KeyAt(index)
		fullKey, fullOK := fixture.Heap.KeyAt(index)
		metadataRow, metadataOK := metadata.At(index)
		if !allocationOK || !placementOK || !fullOK || !metadataOK {
			t.Fatalf("allocation coordinate %d: heap=%t placement=%t full=%t metadata=%t", index, allocationOK, placementOK, fullOK, metadataOK)
		}
		if allocationKey != placementKey || allocationKey != fullKey || allocationKey.Kind() != heapdomain.RootAllocation {
			t.Fatalf("allocation coordinate %d diverged: heap=%v placement=%v full=%v", index, allocationKey, placementKey, fullKey)
		}
		if dense, denseOK := fixture.Heap.AllocationKeyIndex(allocationKey); !denseOK || dense != index {
			t.Fatalf("Heap allocation inverse at %d = %d/%t", index, dense, denseOK)
		}
		id, idOK := allocationKey.ContentID()
		if !idOK || metadataRow.ID() != id {
			t.Fatalf("Heap metadata identity at %d = %v/%t, want %v", index, metadataRow.ID(), idOK, id)
		}
		if !metadataRow.Valid() || !observation.Present[index] {
			t.Fatalf("allocation metadata/fact at %d is incomplete: metadata=%#v fact-present=%t", index, metadataRow, observation.Present[index])
		}

		evidence, evidenceOK := placementdomain.PlacementSummaryEvidence(placement, withEvidence, placementKey)
		if !evidenceOK {
			t.Fatalf("suspension evidence at allocation %d was not published", index)
		}
		wantEvidence := suspension.EvidenceProven.Public()
		if index%2 != 0 {
			wantEvidence = suspension.EvidenceRefuted.Public()
		}
		if evidence.DiesBeforeSuspension != wantEvidence {
			t.Fatalf("suspension evidence at allocation %d = %v, want %v", index, evidence.DiesBeforeSuspension, wantEvidence)
		}
	}

	// The complete Heap carrier retains its physical prefix and any detached
	// Boot suffix. Boot coordinates are not admitted by the allocation inverse.
	for index := allocationCount; index < fixture.Heap.KeyCount(); index++ {
		key, keyOK := fixture.Heap.KeyAt(index)
		boot, bootOK := fixture.Heap.BootRootAt(index - allocationCount)
		if !keyOK || !bootOK || key != boot || key.Kind() != heapdomain.RootBoot {
			t.Fatalf("Heap Boot tail at %d = key=%v/%t boot=%v/%t", index, key, keyOK, boot, bootOK)
		}
		if _, allocationOK := fixture.Heap.AllocationKeyIndex(key); allocationOK {
			t.Fatalf("Heap allocation inverse admitted Boot coordinate %d", index)
		}
	}
}
