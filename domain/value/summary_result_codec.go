package value

import (
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/schema"
)

// SummaryResultFamily is the canonical query family key for value summaries.
const SummaryResultFamily schema.Key = "value-summary"

const valueSummaryResultFormat uint64 = 1

const (
	valueSummaryResultHeaderSize = 8 + 32 + 8
	valueSummaryCoordinateIDSize = 32
)

// EncodeSummaryResult canonically detaches the complete correlated Value
// summary. The payload keeps Value's compact atom/capability word image rather
// than expanding it into structural objects. Present values must belong to one
// exact Link schema; that Link identity fences the private word ordinals.
func EncodeSummaryResult(observation ValueSummaryObservation) (present bool, rows uint64, payload []byte, ok bool) {
	count := len(observation.Values)
	owner := observation.owner
	if owner == nil || !summaryObservationOwned(owner, observation) || count == 0 || len(observation.Present) != count || observation.Rows > 1 || !owner.LinkID().Available() {
		return false, 0, nil, false
	}
	words := 0
	any := false
	for index, held := range observation.Present {
		if !held {
			continue
		}
		value := observation.Values[index]
		if !value.valid() || value.schema != owner {
			return false, 0, nil, false
		}
		words += len(value.image)
		any = true
	}
	if any != (observation.Rows == 1) || owner.CoordinateCount() != count {
		return false, 0, nil, false
	}
	// format + owner + coordinate count, then one raw portable coordinate ID
	// slot per coordinate followed by one presence byte per coordinate; a
	// present coordinate adds top + word count + its compact word image.
	size := valueSummaryResultHeaderSize + count*valueSummaryCoordinateIDSize + count + words*8
	for _, held := range observation.Present {
		if held {
			size += 1 + 8
		}
	}
	payload = make([]byte, size)
	cursor := 0
	putUint := func(value uint64) {
		binary.BigEndian.PutUint64(payload[cursor:cursor+8], value)
		cursor += 8
	}
	putUint(valueSummaryResultFormat)
	link := owner.LinkID()
	copy(payload[cursor:cursor+32], link[:])
	cursor += 32
	putUint(uint64(count))
	if !fillSummaryCoordinateIDs(payload, owner, count) {
		return false, 0, nil, false
	}
	cursor += count * valueSummaryCoordinateIDSize
	for _, held := range observation.Present {
		if held {
			payload[cursor] = 1
		}
		cursor++
	}
	for index, held := range observation.Present {
		if !held {
			continue
		}
		value := observation.Values[index]
		if value.top {
			payload[cursor] = 1
		}
		cursor++
		putUint(uint64(len(value.image)))
		for _, word := range value.image {
			putUint(word)
		}
	}
	return any, uint64(observation.Rows), payload, cursor == len(payload)
}

// fillSummaryCoordinateIDs writes each sealed portable Value identity into
// the slot named by its one-based dense coordinate ordinal. The payload is
// zeroed when allocated, so each slot also provides the no-allocation marker
// needed to reject duplicate ordinals while the map is traversed.
func fillSummaryCoordinateIDs(payload []byte, owner *Schema, count int) bool {
	if owner == nil || len(owner.coordinates) != count {
		return false
	}
	filled := 0
	for id, row := range owner.coordinates {
		if !id.Available() || row.coordinate == 0 || uint64(row.coordinate) > uint64(count) {
			return false
		}
		ordinal := int(row.coordinate - 1)
		start := valueSummaryResultHeaderSize + ordinal*valueSummaryCoordinateIDSize
		slot := payload[start : start+valueSummaryCoordinateIDSize]
		if !summaryCoordinateIDSlotEmpty(slot) {
			return false
		}
		copy(slot, id[:])
		filled++
	}
	if filled != count {
		return false
	}
	for ordinal := 0; ordinal < count; ordinal++ {
		start := valueSummaryResultHeaderSize + ordinal*valueSummaryCoordinateIDSize
		if summaryCoordinateIDSlotEmpty(payload[start : start+valueSummaryCoordinateIDSize]) {
			return false
		}
	}
	return true
}

func summaryCoordinateIDSlotEmpty(slot []byte) bool {
	return binary.BigEndian.Uint64(slot[:8])|
		binary.BigEndian.Uint64(slot[8:16])|
		binary.BigEndian.Uint64(slot[16:24])|
		binary.BigEndian.Uint64(slot[24:32]) == 0
}
