package suspension

import (
	"github.com/wippyai/go-lua/analysis/engine"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// AccumulatePlacementSummarySuspension folds this vertical's independent
// evidence Factor into the public Placement summary. The evidence vector is
// Heap-aligned and independently sealed; this fold never reads Placement's
// class, so a class cannot manufacture a suspension proof.
func AccumulatePlacementSummarySuspension(schema placementdomain.Schema, observation placementdomain.PlacementSummaryObservation, cells engine.OrderedCells[Evidence]) (placementdomain.PlacementSummaryObservation, bool) {
	if !placementdomain.EqualPlacementSummary(schema, observation, observation) {
		return placementdomain.PlacementSummaryObservation{}, false
	}
	denseCount := schema.DenseKeyCount()
	if denseCount == 0 {
		if cells.Count() != 0 {
			return placementdomain.PlacementSummaryObservation{}, false
		}
		return observation, true
	}
	if cells.Count() != denseCount {
		return placementdomain.PlacementSummaryObservation{}, false
	}
	// A query projection receives the current fold value by value, but the
	// summary planes behind that value are slices. Detach them once before the
	// evidence-only refinement, then mutate only this producer-owned copy. The
	// old per-coordinate WithPlacementSummaryEvidence path detached all three
	// O(N) planes on every row, making a dense fold quadratic.
	result := placementdomain.ClonePlacementSummary(observation)
	for index := 0; index < denseCount; index++ {
		key, keyOK := schema.KeyAt(index)
		if !keyOK {
			return placementdomain.PlacementSummaryObservation{}, false
		}
		state, present, available := cells.At(index)
		if !available {
			return placementdomain.PlacementSummaryObservation{}, false
		}
		if present && !state.Valid() {
			return placementdomain.PlacementSummaryObservation{}, false
		}
		if key.Kind() != heapdomain.RootAllocation {
			continue
		}
		if !present || state == EvidenceMissing {
			continue
		}
		public := state.Public()
		if public == placementdomain.EvidenceUnknown {
			continue
		}
		if !placementdomain.SetPlacementSummaryEvidence(schema, &result, key, placementdomain.AllocationEvidence{DiesBeforeSuspension: public}) {
			return placementdomain.PlacementSummaryObservation{}, false
		}
	}
	return result, placementdomain.EqualPlacementSummary(schema, result, result)
}
