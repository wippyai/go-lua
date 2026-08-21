package placement_test

import (
	"strconv"
	"testing"

	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// TestPlacementSummaryEvidenceBatchKeepsPublishedSourceDetached exercises the
// ownership boundary used by the suspension fold: clone the current summary
// once, refine that producer-owned copy in place, and leave the published
// source untouched.
func TestPlacementSummaryEvidenceBatchKeepsPublishedSourceDetached(t *testing.T) {
	fixture := newDeepFrozenValueFixture(t)
	source := placementSummaryEvidenceFoldSeed(t, fixture)
	snapshot := placementdomain.ClonePlacementSummary(source)
	working := placementdomain.ClonePlacementSummary(source)
	evidence := placementdomain.AllocationEvidence{DiesBeforeSuspension: placementdomain.EvidenceProven}
	for index := 0; index < 128; index++ {
		key := fixture.allocations[index%len(fixture.allocations)]
		if !placementdomain.SetPlacementSummaryEvidence(fixture.placement, &working, key, evidence) {
			t.Fatalf("set evidence row %d", index)
		}
	}
	if !placementdomain.EqualPlacementSummary(fixture.placement, source, snapshot) {
		t.Fatal("published Placement summary changed through detached evidence fold")
	}
	key := fixture.allocations[0]
	if got, ok := placementdomain.PlacementSummaryEvidence(fixture.placement, source, key); !ok || got.DiesBeforeSuspension != placementdomain.EvidenceUnknown {
		t.Fatalf("published source evidence = %v/%t, want unknown/true", got.DiesBeforeSuspension, ok)
	}
	if got, ok := placementdomain.PlacementSummaryEvidence(fixture.placement, working, key); !ok || got.DiesBeforeSuspension != placementdomain.EvidenceProven {
		t.Fatalf("detached working evidence = %v/%t, want proven/true", got.DiesBeforeSuspension, ok)
	}
}

// BenchmarkPlacementSummaryEvidenceBatchSet measures the in-place primitive
// over N evidence rows. The working observation is detached before timing, so
// each row exercises only owner/schema validation and one evidence-cell merge;
// there must be no O(N) plane clone per row.
func BenchmarkPlacementSummaryEvidenceBatchSet(b *testing.B) {
	fixture := newDeepFrozenValueFixture(b)
	source := placementSummaryEvidenceFoldSeed(b, fixture)
	evidence := placementdomain.AllocationEvidence{DiesBeforeSuspension: placementdomain.EvidenceProven}
	for _, rows := range []int{64, 1024, 16384} {
		b.Run(strconv.Itoa(rows), func(b *testing.B) {
			working := placementdomain.ClonePlacementSummary(source)
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				for row := 0; row < rows; row++ {
					key := fixture.allocations[row%len(fixture.allocations)]
					if !placementdomain.SetPlacementSummaryEvidence(fixture.placement, &working, key, evidence) {
						b.Fatal("set evidence row")
					}
				}
			}
			b.ReportMetric(float64(rows), "evidence-rows/op")
		})
	}
}

func placementSummaryEvidenceFoldSeed(t testing.TB, fixture deepFrozenValueFixture) placementdomain.PlacementSummaryObservation {
	t.Helper()
	observation := placementdomain.BeginPlacementSummary(fixture.placement)
	for _, key := range fixture.allocations {
		index, ok := fixture.placement.Heap().KeyIndex(key)
		if !ok {
			t.Fatal("allocation summary coordinate")
		}
		observation.Values[index] = placementdomain.OwnedHeap
		observation.Present[index] = true
	}
	observation.Rows = 1
	return observation
}
