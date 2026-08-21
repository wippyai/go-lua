package placement

import (
	"github.com/wippyai/go-lua/analysis/engine"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
)

// PlacementSummaryObservation is the detached answer of the Placement summary
// query. Its two interior planes retain the Heap dense coordinate shape while
// the public codec projects only allocation roots. Boot roots remain engine
// coordinates, but they are not Placement result rows.
//
// owner is the exact Placement schema (and therefore exact Heap schema) that
// opened the fold. It is a transient authority and never crosses the result
// codec boundary.
type PlacementSummaryObservation struct {
	Values  []Placement
	Present []bool
	Rows    uint32
	Valid   bool
	// evidence is dense Heap-aligned optional proof. It is intentionally
	// private: callers may add a producer-owned row only through the
	// owner-fenced helpers below, and the public codec projects one row per
	// allocation root.
	evidence []AllocationEvidence
	owner    Schema
}

// BeginPlacementSummary starts one Heap-aligned Placement summary fold.
// Placement has no independent coordinate universe: Heap supplies the dense
// denominator and its owner-fenced Key identities.
func BeginPlacementSummary(schema Schema) PlacementSummaryObservation {
	if !schema.Valid() {
		return PlacementSummaryObservation{}
	}
	count := schema.DenseKeyCount()
	return PlacementSummaryObservation{
		Values:   make([]Placement, count),
		Present:  make([]bool, count),
		evidence: make([]AllocationEvidence, count),
		Valid:    true,
		owner:    schema,
	}
}

// AccumulatePlacementSummary folds one engine-owned dense Placement vector.
// Absent cells leave their coordinate untouched; present cells are joined
// coordinatewise in the Placement lattice. The fold validates scalar values,
// including values at Boot positions, but only allocation positions determine
// the public row cardinality.
func AccumulatePlacementSummary(schema Schema, result PlacementSummaryObservation, cells engine.OrderedCells[Placement]) (PlacementSummaryObservation, bool) {
	return AccumulatePlacementSummaryRows(schema, result, cells.Count(), cells.At)
}

// AccumulatePlacementSummaryRows states the same fold over an explicit dense
// vector. Keeping this form beside the OrderedCells form makes package laws and
// sealed readers exercise the exact fold the engine invokes.
func AccumulatePlacementSummaryRows(schema Schema, result PlacementSummaryObservation, count int, at func(index int) (Placement, bool, bool)) (PlacementSummaryObservation, bool) {
	if !summaryObservationShape(schema, result) || at == nil {
		return PlacementSummaryObservation{}, false
	}
	// A valid empty Heap produces a genuinely empty factor observation. The
	// engine admits zero-width factors, so Placement never invents a coordinate
	// that Heap cannot authenticate.
	if schema.DenseKeyCount() == 0 {
		if count != 0 || len(result.Values) != 0 || len(result.Present) != 0 {
			return PlacementSummaryObservation{}, false
		}
		return result, true
	}
	if count == 0 || count != schema.DenseKeyCount() || len(result.Values) != count || len(result.Present) != count {
		return PlacementSummaryObservation{}, false
	}
	for index := 0; index < count; index++ {
		key, keyOK := schema.KeyAt(index)
		if !keyOK {
			return PlacementSummaryObservation{}, false
		}
		value, present, available := at(index)
		if !available {
			return PlacementSummaryObservation{}, false
		}
		if !present {
			continue
		}
		if !validAnalysisPlacement(value) {
			return PlacementSummaryObservation{}, false
		}
		kind := key.Kind()
		if kind == heapdomain.RootBoot && value != Bottom {
			return PlacementSummaryObservation{}, false
		}
		if kind != heapdomain.RootAllocation && kind != heapdomain.RootBoot {
			return PlacementSummaryObservation{}, false
		}
		if !result.Present[index] {
			result.Values[index], result.Present[index] = value, true
		} else {
			result.Values[index] = Join(result.Values[index], value)
			if !validAnalysisPlacement(result.Values[index]) {
				return PlacementSummaryObservation{}, false
			}
		}
		if kind == heapdomain.RootAllocation {
			// Presence is monotone over the fold, so the public one-row
			// cardinality can be maintained without rescanning the dense
			// vector after every accumulation.
			result.Rows = 1
		}
	}
	return result, true
}

// ClonePlacementSummary detaches the mutable fold planes while retaining the
// exact Heap owner fence used by the running query.
func ClonePlacementSummary(input PlacementSummaryObservation) PlacementSummaryObservation {
	input.Values = append([]Placement(nil), input.Values...)
	input.Present = append([]bool(nil), input.Present...)
	input.evidence = append([]AllocationEvidence(nil), input.evidence...)
	return input
}

// PlacementSummaryEvidence returns the optional proof row for one existing
// allocation coordinate. The returned row is a value detached from the fold;
// an unavailable row is the explicit all-unknown state.
func PlacementSummaryEvidence(schema Schema, observation PlacementSummaryObservation, key heapdomain.Key) (AllocationEvidence, bool) {
	if !summaryObservationBase(schema, observation) || key.Kind() != heapdomain.RootAllocation || !schema.Heap().OwnsKey(key) {
		return AllocationEvidence{}, false
	}
	index, indexOK := schema.Heap().KeyIndex(key)
	if !indexOK || index < 0 || index >= len(observation.evidence) {
		return AllocationEvidence{}, false
	}
	return observation.evidence[index], true
}

// WithPlacementSummaryEvidence joins one producer-owned proof row into a
// detached observation. The Heap-derived owner identity and allocation kind
// remain authoritative; a producer may only refine independently produced
// columns or repeat the same canonical identity/kind.
func WithPlacementSummaryEvidence(schema Schema, observation PlacementSummaryObservation, key heapdomain.Key, evidence AllocationEvidence) (PlacementSummaryObservation, bool) {
	if !summaryObservationBase(schema, observation) || !evidence.Valid() {
		return PlacementSummaryObservation{}, false
	}
	result := ClonePlacementSummary(observation)
	if !setPlacementSummaryEvidenceInPlace(schema, &result, key, evidence) {
		return PlacementSummaryObservation{}, false
	}
	return result, true
}

// SetPlacementSummaryEvidence is the in-place spelling for a producer that
// already owns a mutable fold copy. It retains the same exact-schema fence as
// WithPlacementSummaryEvidence.
func SetPlacementSummaryEvidence(schema Schema, observation *PlacementSummaryObservation, key heapdomain.Key, evidence AllocationEvidence) bool {
	if observation == nil {
		return false
	}
	return setPlacementSummaryEvidenceInPlace(schema, observation, key, evidence)
}

// setPlacementSummaryEvidenceInPlace is the owner-fenced primitive shared by
// the detached and mutable spellings. Callers that retain an observation as a
// published result must use WithPlacementSummaryEvidence; a producer that
// owns a mutable fold copy can apply many rows through this primitive without
// cloning the O(N) summary planes for every coordinate.
func setPlacementSummaryEvidenceInPlace(schema Schema, observation *PlacementSummaryObservation, key heapdomain.Key, evidence AllocationEvidence) bool {
	if observation == nil || !summaryObservationBase(schema, *observation) || !evidence.Valid() || key.Kind() != heapdomain.RootAllocation || !schema.Heap().OwnsKey(key) {
		return false
	}
	index, indexOK := schema.Heap().KeyIndex(key)
	if !indexOK || index < 0 || index >= len(observation.evidence) {
		return false
	}
	canonical, canonicalOK := allocationEvidenceForKey(schema, key, observation.Values[index], observation.Present[index])
	if !canonicalOK {
		return false
	}
	if evidence.HasClass && (!observation.Present[index] || evidence.Class != observation.Values[index]) {
		return false
	}
	if evidence.FrameLocal == EvidenceProven && observation.Present[index] && observation.Values[index] != Stack {
		return false
	}
	if evidence.FrameLocal == EvidenceRefuted && observation.Present[index] && observation.Values[index] == Stack {
		return false
	}
	if evidence.HasOwnerIdentity && (!canonical.HasOwnerIdentity || evidence.OwnerIdentity != canonical.OwnerIdentity) {
		return false
	}
	if evidence.HasKind && canonical.HasKind && evidence.Kind != canonical.Kind {
		return false
	}
	merged := observation.evidence[index].Merge(evidence)
	if !merged.Valid() {
		return false
	}
	observation.evidence[index] = merged
	return true
}

// EqualPlacementSummary is the frozen-result equality contract. Classes at
// absent coordinates are deliberately ignored because absence is the public
// fact at that coordinate.
func EqualPlacementSummary(schema Schema, left, right PlacementSummaryObservation) bool {
	if !summaryObservationShape(schema, left) || !summaryObservationShape(schema, right) || left.Rows != right.Rows || left.Valid != right.Valid || len(left.Values) != len(right.Values) || len(left.Present) != len(right.Present) || len(left.evidence) != len(right.evidence) {
		return false
	}
	for index := range left.Values {
		if left.Present[index] != right.Present[index] {
			return false
		}
		if left.Present[index] && left.Values[index] != right.Values[index] {
			return false
		}
		if left.evidence[index] != right.evidence[index] {
			return false
		}
	}
	return true
}

// FingerprintPlacementSummary is the frozen-result fingerprint contract. It
// commits to row cardinality, validity, every presence bit, and every present
// Placement class in dense Heap order.
func FingerprintPlacementSummary(schema Schema, value PlacementSummaryObservation) uint64 {
	if !summaryObservationShape(schema, value) {
		return 0
	}
	result := uint64(value.Rows) << 32
	if value.Valid {
		result ^= 1 << 63
	}
	for index := range value.Values {
		result ^= uint64(index+1) * 0x9e3779b97f4a7c15
		if value.Present[index] {
			result ^= (uint64(value.Values[index]) + 1) * 0xc2b2ae3d27d4eb4f
		}
		evidence := value.evidence[index]
		result ^= uint64(evidence.Kind+1) * 0x165667b19e3779f9
		if evidence.HasKind {
			result ^= 1 << uint((index+1)%63)
		}
		if evidence.HasOwnerIdentity {
			for byteIndex, byteValue := range evidence.OwnerIdentity {
				result ^= uint64(byteValue+1) * uint64(byteIndex+1) * 0x27d4eb2f165667c5
			}
		}
		if evidence.HasDepth {
			result ^= uint64(evidence.Depth+1) * 0x94d049bb133111eb
		}
		result ^= uint64(evidence.FrameLocal+1) * 0xbb67ae8584caa73b
		result ^= uint64(evidence.DiesBeforeSuspension+1) * 0xa54ff53a5f1d36f1
		result ^= uint64(evidence.DeepFrozen+1) * 0x510e527fade682d1
	}
	return result
}

func summaryObservationShape(schema Schema, observation PlacementSummaryObservation) bool {
	if !summaryObservationBase(schema, observation) {
		return false
	}
	for _, evidence := range observation.evidence {
		if !evidence.Valid() {
			return false
		}
	}
	anyAllocation := false
	for index, present := range observation.Present {
		if present && !validAnalysisPlacement(observation.Values[index]) {
			return false
		}
		if !present {
			continue
		}
		key, keyOK := schema.KeyAt(index)
		if !keyOK {
			return false
		}
		kind := key.Kind()
		if kind == heapdomain.RootBoot {
			if observation.Values[index] != Bottom {
				return false
			}
			continue
		}
		if kind != heapdomain.RootAllocation {
			return false
		}
		anyAllocation = true
	}
	if anyAllocation {
		return observation.Rows == 1
	}
	return observation.Rows == 0
}

func summaryObservationBase(schema Schema, observation PlacementSummaryObservation) bool {
	if !schema.Valid() || observation.owner != schema || !observation.Valid || observation.Rows > 1 {
		return false
	}
	count := schema.DenseKeyCount()
	return len(observation.Values) == count && len(observation.Present) == count && len(observation.evidence) == count
}
