package placement

import (
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
)

// SummaryResultFamily is the canonical query family key for Placement
// summaries. Placement is a dense allocation-root summary, not an exact scalar
// result family.
const SummaryResultFamily schema.Key = "placement-summary"

// placementSummaryResultFormat is the current allocation-factor wire revision.
// Revision 9 removes Bottom from public allocation rows: Stack is the factor's
// owner-issued default and every encoded allocation is present at Stack or a
// monotone displacement above it.
// Revision 8 gives every proof column a fourth state: the raw EvidenceState
// ordinals in the evidence record now spell absence, unknown, refuted, proven,
// so an unwritten column no longer occupies the ordinal that means "a producer
// authenticated an undecidable verdict". Every ordinal in the proof bytes
// moved by one, which is why this is a revision rather than an addition.
// Revision 7 replaced the revision-6 presence plus compact class planes with
// one fixed state byte per allocation row, so random row lookup never scans a
// prefix of the presence plane. Revision 6 added the owner-authenticated
// manifest-allocation kind for Target fresh roots. Revision 5 kept the
// revision-4 evidence plane, added the transitive DeepFrozen proof column, and
// made the public allocation denominator canonical: complete allocation rows
// are ordered by their owner-issued ContentID. Draft class-only images and
// revision-3 declaration-order images are not part of this contract.
const SummaryResultFormat uint64 = 9

const placementSummaryResultFormat uint64 = SummaryResultFormat

const (
	placementSummaryResultHeaderSize = 8 + 32 + 8
	placementSummaryAllocationIDSize = 32
	// One fixed record per allocation keeps evidence lookups allocation-free:
	// kind, owner presence+ID, depth presence+value, and the three Placement
	// proof states.
	placementSummaryEvidenceRecordSize = 1 + 1 + 32 + 1 + 4 + 1 + 1 + 1
	placementSummaryDeepFrozenOffset   = placementSummaryEvidenceRecordSize - 1
)

// EncodeSummaryResult canonically detaches one Placement summary. The wire
// denominator contains allocation roots only: Heap Boot roots never enter the
// factor or become public Placement rows. Every allocation root
// carries its stable Heap KeyID and one fixed state byte: every allocation row
// must be present and nonzero, while the row's class encodes Stack through
// Unknown explicitly. Revision 8 carries one fixed-width AllocationEvidence
// record for every row; an evidence record is emitted only after a producer
// has authenticated/published that row. A proof column inside a published
// record carries absence until some producer decides it, and explicit Unknown
// remains a semantic producer state, never a replacement for an unavailable
// row or an undecided column. The complete coordinate record is sorted by its
// ContentID before any of the parallel wire planes are written.
func EncodeSummaryResult(observation PlacementSummaryObservation) (present bool, rows uint64, payload []byte, ok bool) {
	schemaOwner := observation.owner
	coordinates, any, valid := placementSummaryCoordinates(schemaOwner, observation)
	if !valid {
		return false, 0, nil, false
	}
	if !canonicalizePlacementSummaryCoordinates(coordinates) {
		return false, 0, nil, false
	}
	if any != (observation.Rows == 1) {
		return false, 0, nil, false
	}
	count := len(coordinates)
	payloadSize, sizeOK := placementSummaryPayloadSize(count)
	if !sizeOK {
		return false, 0, nil, false
	}
	payload = make([]byte, payloadSize)
	cursor := 0
	binary.BigEndian.PutUint64(payload[cursor:cursor+8], placementSummaryResultFormat)
	cursor += 8
	schemaID := schemaOwner.ContentID()
	if !schemaID.Available() {
		return false, 0, nil, false
	}
	copy(payload[cursor:cursor+32], schemaID[:])
	cursor += 32
	binary.BigEndian.PutUint64(payload[cursor:cursor+8], uint64(count))
	cursor += 8
	for _, coordinate := range coordinates {
		copy(payload[cursor:cursor+placementSummaryAllocationIDSize], coordinate.id[:])
		cursor += placementSummaryAllocationIDSize
	}
	for _, coordinate := range coordinates {
		state, stateOK := placementWireState(observation.Values[coordinate.denseIndex], observation.Present[coordinate.denseIndex])
		if !stateOK {
			return false, 0, nil, false
		}
		payload[cursor] = state
		cursor++
	}
	// Evidence occupies a fixed-width row for every complete allocation
	// denominator coordinate. A row is emitted only after the evidence producer
	// (or an owner-fenced explicit evidence helper) published it; this prevents
	// a zero private row from becoming an unauthenticated Unknown on the wire.
	for _, coordinate := range coordinates {
		if !observation.evidencePublished[coordinate.denseIndex] {
			// The zero value of AllocationEvidence is wire-valid, so checking
			// only Valid would publish a row no producer ever wrote.
			// Publication must be authenticated by the fold or by the
			// owner-fenced evidence helper first.
			return false, 0, nil, false
		}
		evidence, evidenceOK := allocationEvidenceForKey(schemaOwner, coordinate.key, observation.Values[coordinate.denseIndex], observation.Present[coordinate.denseIndex])
		if !evidenceOK {
			return false, 0, nil, false
		}
		// The Heap-derived row is the evidence authority for this coordinate.
		// Validate it before composing any optional producer row so a malformed
		// private evidence plane cannot be turned into a valid empty record and
		// accidentally published after a failed composition.
		if !placementSummaryEvidenceFenced(coordinate.id, evidence) {
			return false, 0, nil, false
		}
		if len(observation.evidence) == len(observation.Values) {
			producerEvidence := observation.evidence[coordinate.denseIndex]
			if !producerEvidence.Valid() {
				return false, 0, nil, false
			}
			var composedOK bool
			evidence, composedOK = ComposeAllocationEvidence(evidence, producerEvidence)
			if !composedOK {
				return false, 0, nil, false
			}
		}
		// The composed row must retain the owner-issued ID because the decoder
		// uses it to pair the evidence plane with the canonical allocation
		// denominator. Fail closed rather than emitting a payload the decoder
		// will reject.
		if !placementSummaryEvidenceFenced(coordinate.id, evidence) || !encodeAllocationEvidence(payload[cursor:cursor+placementSummaryEvidenceRecordSize], evidence) {
			return false, 0, nil, false
		}
		cursor += placementSummaryEvidenceRecordSize
	}
	return any, uint64(observation.Rows), payload, cursor == len(payload)
}

// placementSummaryEvidenceFenced validates the evidence that can cross the
// Placement result boundary. The final row must remain valid and retain the
// exact owner-issued allocation identity used by the ID plane. Keeping this
// check as one helper makes the before/after composition fences impossible to
// accidentally diverge.
func placementSummaryEvidenceFenced(coordinateID identity.ContentID, evidence AllocationEvidence) bool {
	return coordinateID.Available() && evidence.Valid() && evidence.HasOwnerIdentity && evidence.OwnerIdentity == coordinateID
}

type placementSummaryCoordinate struct {
	denseIndex int
	id         identity.ContentID
	key        heapdomain.Key
}

// canonicalizePlacementSummaryCoordinates establishes the wire's one
// deterministic row order while retaining the dense index and owner-fenced
// Heap key beside each ID. Sorting the complete record is essential: the
// fixed state and evidence planes are all emitted from this same slice and
// must never be detached from their allocation identity.
func canonicalizePlacementSummaryCoordinates(coordinates []placementSummaryCoordinate) bool {
	if len(coordinates) < 2 {
		return true
	}
	identity.SortByContentID(coordinates, func(coordinate placementSummaryCoordinate) identity.ContentID {
		return coordinate.id
	})
	for index := 1; index < len(coordinates); index++ {
		if coordinates[index-1].id == coordinates[index].id {
			return false
		}
	}
	return true
}

// placementSummaryPayloadSize computes the exact byte width before Encode
// calls make. The explicit checked arithmetic keeps the codec portable on
// 32-bit hosts and turns an oversized owner projection into a clean refusal
// instead of an integer wrap followed by a slice panic.
func placementSummaryPayloadSize(count int) (int, bool) {
	if count < 0 {
		return 0, false
	}
	maxInt := int(^uint(0) >> 1)
	rowBytes := placementSummaryAllocationIDSize + 1 + placementSummaryEvidenceRecordSize
	size := placementSummaryResultHeaderSize
	if count > (maxInt-size)/rowBytes {
		return 0, false
	}
	return size + count*rowBytes, true
}

// placementSummaryCoordinates validates the exact dense allocation shape in
// one KeyAt pass. In particular, it
// never rescans mounted Program allocations through OccurrenceMount.AllocationAt.
func placementSummaryCoordinates(schemaOwner Schema, observation PlacementSummaryObservation) ([]placementSummaryCoordinate, bool, bool) {
	if !summaryObservationBase(schemaOwner, observation) || !schemaOwner.ContentID().Available() {
		return nil, false, false
	}
	denseCount := schemaOwner.DenseKeyCount()
	coordinates := make([]placementSummaryCoordinate, 0, denseCount)
	any := false
	for index := 0; index < denseCount; index++ {
		key, keyOK := schemaOwner.KeyAt(index)
		if !keyOK {
			return nil, false, false
		}
		present := observation.Present[index]
		if present && (!validAnalysisPlacement(observation.Values[index]) || observation.Values[index] == Bottom) {
			return nil, false, false
		}
		if key.Kind() != heapdomain.RootAllocation {
			return nil, false, false
		}
		// A public denominator row is complete only when its Placement
		// factor published the coordinate. Do not encode state zero for an
		// absent allocation and let a decoder infer a weaker result.
		if !present {
			return nil, false, false
		}
		id, idOK := key.ContentID()
		if !idOK || !id.Available() {
			return nil, false, false
		}
		coordinates = append(coordinates, placementSummaryCoordinate{denseIndex: index, id: id, key: key})
		if present {
			any = true
		}
	}
	return coordinates, any, any == (observation.Rows == 1)
}

func encodeAllocationEvidence(payload []byte, evidence AllocationEvidence) bool {
	if len(payload) != placementSummaryEvidenceRecordSize || !evidence.Valid() {
		return false
	}
	payload[0] = byte(evidence.Kind)
	if evidence.HasOwnerIdentity {
		payload[1] = 1
		copy(payload[2:34], evidence.OwnerIdentity[:])
	}
	if evidence.HasDepth {
		payload[34] = 1
		binary.BigEndian.PutUint32(payload[35:39], evidence.Depth)
	}
	payload[39] = byte(evidence.FrameLocal)
	payload[40] = byte(evidence.DiesBeforeSuspension)
	payload[placementSummaryDeepFrozenOffset] = byte(evidence.DeepFrozen)
	return true
}

// placementWireOrdinal is the frozen Placement class mapping used inside the
// v9 state byte. It is deliberately explicit: raw Placement enum values are
// not wire values.
func placementWireOrdinal(value Placement) (byte, bool) {
	switch value {
	case Stack:
		return 0, true
	case OwnedHeap:
		return 1, true
	case SharedHeap:
		return 2, true
	case Unknown:
		return 3, true
	default:
		return 0, false
	}
}

// placementWireState maps the public presence/class pair to the v9 fixed row
// state. State zero is reserved for absence, so present Stack is state one.
func placementWireState(value Placement, present bool) (byte, bool) {
	if !present {
		return 0, true
	}
	ordinal, ok := placementWireOrdinal(value)
	if !ok {
		return 0, false
	}
	return ordinal + 1, true
}

func placementFromWireState(state byte) (Placement, bool, bool) {
	if state == 0 {
		return Bottom, false, true
	}
	value, ok := placementFromWireOrdinal(state - 1)
	return value, ok, ok
}

func placementFromWireOrdinal(ordinal byte) (Placement, bool) {
	switch ordinal {
	case 0:
		return Stack, true
	case 1:
		return OwnedHeap, true
	case 2:
		return SharedHeap, true
	case 3:
		return Unknown, true
	default:
		return Bottom, false
	}
}
