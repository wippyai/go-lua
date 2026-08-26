package placement

import (
	heapdomain "github.com/wippyai/go-lua/domain/heap"
)

// PlacementSummaryObservation is the detached answer of the Placement summary
// query. Its planes have exactly Placement's allocation-root denominator;
// Boot roots never enter this factor or its public codec.
//
// owner is the exact Placement schema (and therefore exact Heap schema) that
// opened the fold. It is a transient authority and never crosses the result
// codec boundary.
type PlacementSummaryObservation struct {
	Values  []Fact
	Present []bool
	Rows    uint32
	Valid   bool
	// evidence is dense allocation-root producer proof. It is intentionally
	// private: callers may add a producer-owned row only through the
	// owner-fenced helpers below, and evidencePublished records whether the
	// zero-capable row was actually authenticated.
	evidence []AllocationEvidence
	// evidencePublished is separate from evidence because the zero value of an
	// AllocationEvidence is a valid all-absent row. A zero row in a dense fold
	// is not, by itself, an authenticated publication: it is merely the absence
	// of a producer write. Keep that distinction through the result boundary.
	evidencePublished []bool
	owner             Schema
}

// BeginPlacementSummary starts one allocation-root Placement summary fold.
// Heap supplies the dense denominator and its owner-fenced Key identities.
func BeginPlacementSummary(schema Schema) PlacementSummaryObservation {
	if !schema.Valid() {
		return PlacementSummaryObservation{}
	}
	count := schema.DenseKeyCount()
	return PlacementSummaryObservation{
		Values:            make([]Fact, count),
		Present:           make([]bool, count),
		evidence:          make([]AllocationEvidence, count),
		evidencePublished: make([]bool, count),
		Valid:             true,
		owner:             schema,
	}
}

// AccumulatePlacementSummaryRows folds one dense Placement vector.
// Sparse cells are admitted only when the Placement owner supplied its exact
// Stack default. The default is projected as a present Stack row because every
// coordinate already denotes an allocation site; displacement rules only move
// it upward. Present cells are joined coordinatewise in the Placement lattice.
//
// The vector arrives as its width and its row accessor, which is the one shape
// every delivery of a many-valued read already states a row in, so the fold a
// solve runs is the fold a sealed reader runs.
func AccumulatePlacementSummaryRows(schema Schema, result PlacementSummaryObservation, count int, at func(index int) (Fact, bool, bool)) (PlacementSummaryObservation, bool) {
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
		if key.Kind() != heapdomain.RootAllocation {
			return PlacementSummaryObservation{}, false
		}
		value, valueOK := AuthenticateFactCell(value, present, available)
		if !valueOK {
			return PlacementSummaryObservation{}, false
		}
		if !result.Present[index] {
			result.Values[index], result.Present[index] = value, true
		} else {
			joined, joinedOK := JoinFactChecked(result.Values[index], value)
			if !joinedOK {
				return PlacementSummaryObservation{}, false
			}
			result.Values[index] = joined
		}
		// Presence is monotone over the fold, so the public one-row
		// cardinality can be maintained without rescanning the dense vector.
		result.Rows = 1
	}
	return result, true
}

// ClonePlacementSummary detaches the mutable fold planes while retaining the
// exact Heap owner fence used by the running query.
func ClonePlacementSummary(input PlacementSummaryObservation) PlacementSummaryObservation {
	input.Values = append([]Fact(nil), input.Values...)
	input.Present = append([]bool(nil), input.Present...)
	input.evidence = append([]AllocationEvidence(nil), input.evidence...)
	input.evidencePublished = append([]bool(nil), input.evidencePublished...)
	return input
}

// PlacementSummaryEvidence returns one producer-published proof row for an
// existing allocation coordinate. A false result is unavailable, not an
// all-absent row; a proof column carries EvidenceUnknown only after a producer
// authenticated and published that state, and EvidenceAbsent until then.
func PlacementSummaryEvidence(schema Schema, observation PlacementSummaryObservation, key heapdomain.Key) (AllocationEvidence, bool) {
	if !summaryObservationBase(schema, observation) || key.Kind() != heapdomain.RootAllocation || !schema.Heap().OwnsKey(key) {
		return invalidAllocationEvidence(), false
	}
	index, indexOK := schema.Heap().AllocationKeyIndex(key)
	if !indexOK || index < 0 || index >= len(observation.evidence) || !observation.evidencePublished[index] {
		return invalidAllocationEvidence(), false
	}
	return observation.evidence[index], true
}

// PlacementSummaryAllocation returns the complete owner-authenticated row for
// one allocation directly from an in-memory summary answer. It is the same
// composition EncodeSummaryResult writes: the Placement factor supplies class
// and retain provenance, Heap supplies identity/kind/frame locality, and the
// private producer plane supplies refinements such as depth and deep freeze.
// Consumers therefore do not encode and decode a summary merely to read it.
func PlacementSummaryAllocation(schema Schema, observation PlacementSummaryObservation, key heapdomain.Key) (Fact, AllocationEvidence, bool) {
	if !summaryObservationBase(schema, observation) || key.Kind() != heapdomain.RootAllocation || !schema.Heap().OwnsKey(key) {
		return Fact{}, invalidAllocationEvidence(), false
	}
	index, indexOK := schema.Heap().AllocationKeyIndex(key)
	if !indexOK || index < 0 || index >= len(observation.Values) || !observation.Present[index] || !observation.evidencePublished[index] {
		return Fact{}, invalidAllocationEvidence(), false
	}
	fact := observation.Values[index]
	evidence, evidenceOK := allocationEvidenceForKey(schema, key, fact, true)
	if !evidenceOK {
		return Fact{}, invalidAllocationEvidence(), false
	}
	producer := observation.evidence[index]
	composed, composedOK := ComposeAllocationEvidence(evidence, producer)
	if !composedOK {
		return Fact{}, invalidAllocationEvidence(), false
	}
	return fact, composed, true
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
	index, indexOK := schema.Heap().AllocationKeyIndex(key)
	if !indexOK || index < 0 || index >= len(observation.evidence) {
		return false
	}
	// Evidence is a refinement of a published allocation row. A producer may
	// not make an absent Placement coordinate look complete by attaching a
	// proof record to it.
	if !observation.Present[index] {
		return false
	}
	canonical, canonicalOK := allocationEvidenceForKey(schema, key, observation.Values[index], observation.Present[index])
	if !canonicalOK {
		return false
	}
	if evidence.HasClass && (!observation.Present[index] || evidence.Class != observation.Values[index].Class) {
		return false
	}
	if evidence.FrameLocal == EvidenceProven && observation.Present[index] && observation.Values[index].Class != Stack {
		return false
	}
	if evidence.FrameLocal == EvidenceRefuted && observation.Present[index] && observation.Values[index].Class == Stack {
		return false
	}
	if evidence.HasOwnerIdentity && (!canonical.HasOwnerIdentity || evidence.OwnerIdentity != canonical.OwnerIdentity) {
		return false
	}
	if evidence.HasKind && canonical.HasKind && evidence.Kind != canonical.Kind {
		return false
	}
	merged, mergedOK := ComposeAllocationEvidence(observation.evidence[index], evidence)
	if !mergedOK {
		return false
	}
	observation.evidence[index] = merged
	observation.evidencePublished[index] = true
	return true
}

// EqualPlacementSummary is the frozen-result equality contract. Classes at
// absent coordinates are deliberately ignored because absence is the public
// fact at that coordinate.
func EqualPlacementSummary(schema Schema, left, right PlacementSummaryObservation) bool {
	if !summaryObservationShape(schema, left) || !summaryObservationShape(schema, right) || left.Rows != right.Rows || left.Valid != right.Valid || len(left.Values) != len(right.Values) || len(left.Present) != len(right.Present) || len(left.evidence) != len(right.evidence) || len(left.evidencePublished) != len(right.evidencePublished) {
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
		if left.evidencePublished[index] != right.evidencePublished[index] {
			return false
		}
	}
	return true
}

// FingerprintPlacementSummary is the frozen-result fingerprint contract. It
// commits to row cardinality, validity, every presence bit, every evidence
// publication bit, and every present Placement class in dense allocation order.
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
			result ^= value.Values[index].Hash() * 0xc2b2ae3d27d4eb4f
		}
		evidence := value.evidence[index]
		if value.evidencePublished[index] {
			result ^= uint64(index+1) * 0x6eed0e9da4d94a4f
		}
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
		if observation.evidencePublished[index] {
			key, keyOK := schema.KeyAt(index)
			if !keyOK || key.Kind() != heapdomain.RootAllocation || !present {
				return false
			}
		}
		if present && (!observation.Values[index].Valid() || observation.Values[index].Class == Bottom || observation.Values[index].RetainEscape == EvidenceAbsent) {
			return false
		}
		if !present {
			continue
		}
		key, keyOK := schema.KeyAt(index)
		if !keyOK {
			return false
		}
		if key.Kind() != heapdomain.RootAllocation {
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
	return len(observation.Values) == count && len(observation.Present) == count && len(observation.evidence) == count && len(observation.evidencePublished) == count
}
