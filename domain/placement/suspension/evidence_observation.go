package suspension

import (
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// AccumulatePlacementSummarySuspensionRows folds this vertical's independent
// evidence Factor into the public Placement summary. The evidence vector is
// allocation-aligned and independently sealed; this fold never reads Placement's
// class, so a class cannot manufacture a suspension proof.
//
// The vector arrives as its width and its row accessor, exactly as Placement's
// own summary fold receives one.
func AccumulatePlacementSummarySuspensionRows(schema placementdomain.Schema, observation placementdomain.PlacementSummaryObservation, count int, at func(index int) (Evidence, bool, bool)) (placementdomain.PlacementSummaryObservation, bool) {
	if at == nil || !placementdomain.EqualPlacementSummary(schema, observation, observation) {
		return placementdomain.PlacementSummaryObservation{}, false
	}
	denseCount := schema.DenseKeyCount()
	if denseCount == 0 {
		if count != 0 {
			return placementdomain.PlacementSummaryObservation{}, false
		}
		return observation, true
	}
	if count != denseCount {
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
		if key.Kind() != heapdomain.RootAllocation {
			return placementdomain.PlacementSummaryObservation{}, false
		}
		state, present, available := at(index)
		state, stateOK := authenticateEvidenceCell(state, present, available)
		if !stateOK {
			return placementdomain.PlacementSummaryObservation{}, false
		}
		if !present {
			// The engine retains the Factor owner's value on a sparse cell.
			// EvidenceMissing is this factor's exact Bottom and therefore an
			// authenticated absence of a suspension verdict, not permission to
			// publish Unknown or a proof. Any other sparse payload is malformed.
			continue
		}
		// Every authenticated route verdict is published, including Unknown.
		// Skipping Unknown would leave the column at its absence default and
		// so tell the consumer that no route reported, when in fact every
		// route reported and none of them decided.
		public := state.Public()
		if !public.Valid() || public == placementdomain.EvidenceAbsent {
			return placementdomain.PlacementSummaryObservation{}, false
		}
		if !placementdomain.SetPlacementSummaryEvidence(schema, &result, key, placementdomain.AllocationEvidence{DiesBeforeSuspension: public}) {
			return placementdomain.PlacementSummaryObservation{}, false
		}
	}
	return result, placementdomain.EqualPlacementSummary(schema, result, result)
}
