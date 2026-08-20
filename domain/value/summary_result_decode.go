package value

import "github.com/wippyai/go-lua/analysis/identity"

// SummaryResult is an immutable, allocation-free view of an encoded Value
// summary. It retains the wire image and the offsets needed to walk it; it
// does not materialize coordinate IDs, presence bits, or compact words.
type SummaryResult struct {
	payload         string
	coordinateCount int
	idOffset        int
	presenceOffset  int
	recordOffset    int
}

// SummaryResultIterator walks the coordinate records in declaration order.
// The iterator is deliberately a small mutable cursor over its immutable
// SummaryResult payload.
type SummaryResultIterator struct {
	payload         string
	coordinateCount int
	idOffset        int
	presenceOffset  int
	cursor          int
	index           int
}

// SummaryResultCoordinate is one coordinate view returned by a
// SummaryResultIterator. Its fields are offsets into the immutable payload,
// so retaining a coordinate does not copy any part of the wire image.
type SummaryResultCoordinate struct {
	payload        string
	idOffset       int
	presenceOffset int
	recordOffset   int
	wordsOffset    int
	wordsEnd       int
}

// DecodeSummaryResult validates and opens one encoded Value summary. The
// present and rows arguments are publication metadata carried beside the
// payload; both are checked against the canonical wire image.
func DecodeSummaryResult(present bool, rows uint64, payload string) (SummaryResult, bool) {
	if len(payload) < valueSummaryResultHeaderSize || rows > 1 {
		return SummaryResult{}, false
	}
	if summaryResultUint64(payload, 0) != valueSummaryResultFormat {
		return SummaryResult{}, false
	}

	coordinateCount64 := summaryResultUint64(payload, 8+32)
	idOffset, presenceOffset, recordOffset, coordinateCount, ok := summaryResultOffsets(len(payload), coordinateCount64)
	if !ok {
		return SummaryResult{}, false
	}

	ownerAvailable := summaryResultIDAvailable(payload, 8)
	if !ownerAvailable {
		return SummaryResult{}, false
	}
	anyPresent := false
	for index := 0; index < coordinateCount; index++ {
		idAt := idOffset + index*valueSummaryCoordinateIDSize
		presenceAt := presenceOffset + index
		presence := payload[presenceAt]
		if !summaryResultIDAvailable(payload, idAt) || presence > 1 {
			return SummaryResult{}, false
		}
		if presence == 1 {
			anyPresent = true
		}
		if summaryResultDuplicateID(payload, idOffset, index) {
			return SummaryResult{}, false
		}
	}

	if rows != uint64(summaryRowsForPresence(anyPresent)) || present != anyPresent {
		return SummaryResult{}, false
	}

	cursor := recordOffset
	for index := 0; index < coordinateCount; index++ {
		if payload[presenceOffset+index] == 0 {
			continue
		}
		if len(payload)-cursor < 9 {
			return SummaryResult{}, false
		}
		top := payload[cursor]
		if top > 1 {
			return SummaryResult{}, false
		}
		wordCount := summaryResultUint64(payload, cursor+1)
		remainingWords := uint64((len(payload) - cursor - 9) / 8)
		if wordCount > remainingWords {
			return SummaryResult{}, false
		}
		if top == 1 && wordCount != 0 {
			return SummaryResult{}, false
		}
		wordBytes := int(wordCount) * 8
		cursor += 9 + wordBytes
	}
	if cursor != len(payload) {
		return SummaryResult{}, false
	}
	return SummaryResult{
		payload:         payload,
		coordinateCount: coordinateCount,
		idOffset:        idOffset,
		presenceOffset:  presenceOffset,
		recordOffset:    recordOffset,
	}, true
}

// Available reports whether this is a successfully decoded summary.
func (result SummaryResult) Available() bool {
	return result.payload != "" && result.coordinateCount > 0 &&
		result.idOffset >= valueSummaryResultHeaderSize &&
		result.presenceOffset >= result.idOffset &&
		result.recordOffset >= result.presenceOffset &&
		result.recordOffset <= len(result.payload)
}

// LinkID returns the detached owner identity carried by the summary. An
// unavailable result returns the zero identity.
func (result SummaryResult) LinkID() identity.ContentID {
	var id identity.ContentID
	if !result.Available() {
		return id
	}
	for index := range id {
		id[index] = result.payload[8+index]
	}
	return id
}

// CoordinateCount reports the number of coordinate slots in the summary.
func (result SummaryResult) CoordinateCount() int {
	if !result.Available() {
		return 0
	}
	return result.coordinateCount
}

// Coordinates opens a declaration-order iterator over the coordinate slots.
func (result SummaryResult) Coordinates() SummaryResultIterator {
	if !result.Available() {
		return SummaryResultIterator{}
	}
	return SummaryResultIterator{
		payload:         result.payload,
		coordinateCount: result.coordinateCount,
		idOffset:        result.idOffset,
		presenceOffset:  result.presenceOffset,
		cursor:          result.recordOffset,
	}
}

// Next returns the next coordinate view. It consumes only the variable-size
// record for a present coordinate; absent coordinates have no record.
func (iterator *SummaryResultIterator) Next() (SummaryResultCoordinate, bool) {
	if iterator == nil || iterator.index >= iterator.coordinateCount || iterator.payload == "" {
		return SummaryResultCoordinate{}, false
	}
	index := iterator.index
	iterator.index++
	coordinate := SummaryResultCoordinate{
		payload:        iterator.payload,
		idOffset:       iterator.idOffset + index*valueSummaryCoordinateIDSize,
		presenceOffset: iterator.presenceOffset + index,
	}
	if iterator.payload[coordinate.presenceOffset] == 0 {
		return coordinate, true
	}
	coordinate.recordOffset = iterator.cursor
	wordCount := summaryResultUint64(iterator.payload, iterator.cursor+1)
	coordinate.wordsOffset = iterator.cursor + 9
	coordinate.wordsEnd = coordinate.wordsOffset + int(wordCount)*8
	iterator.cursor = coordinate.wordsEnd
	return coordinate, true
}

// ID returns the portable coordinate identity. An unavailable coordinate
// returns the zero identity.
func (coordinate SummaryResultCoordinate) ID() identity.ContentID {
	var id identity.ContentID
	if !coordinate.Available() {
		return id
	}
	for index := range id {
		id[index] = coordinate.payload[coordinate.idOffset+index]
	}
	return id
}

// Present reports whether this coordinate has a compact Value record.
func (coordinate SummaryResultCoordinate) Present() bool {
	return coordinate.Available() && coordinate.payload[coordinate.presenceOffset] == 1
}

// Top reports whether a present coordinate carries Value Top.
func (coordinate SummaryResultCoordinate) Top() bool {
	return coordinate.Present() && coordinate.payload[coordinate.recordOffset] == 1
}

// WordCount reports the number of compact words in this coordinate. Top and
// absent coordinates have zero words.
func (coordinate SummaryResultCoordinate) WordCount() int {
	if !coordinate.Present() {
		return 0
	}
	return (coordinate.wordsEnd - coordinate.wordsOffset) / 8
}

// WordAt reads one compact word. It returns false for absent coordinates or
// indexes outside the coordinate's word range.
func (coordinate SummaryResultCoordinate) WordAt(index int) (uint64, bool) {
	if !coordinate.Present() || index < 0 || index >= coordinate.WordCount() {
		return 0, false
	}
	return summaryResultUint64(coordinate.payload, coordinate.wordsOffset+index*8), true
}

func (coordinate SummaryResultCoordinate) Available() bool {
	return coordinate.payload != "" && coordinate.idOffset >= valueSummaryResultHeaderSize &&
		coordinate.idOffset+valueSummaryCoordinateIDSize <= len(coordinate.payload) &&
		coordinate.presenceOffset >= coordinate.idOffset+valueSummaryCoordinateIDSize &&
		coordinate.presenceOffset < len(coordinate.payload)
}

func summaryResultOffsets(payloadLength int, coordinateCount uint64) (idOffset, presenceOffset, recordOffset, count int, ok bool) {
	if payloadLength < valueSummaryResultHeaderSize || coordinateCount == 0 {
		return 0, 0, 0, 0, false
	}
	remaining := payloadLength - valueSummaryResultHeaderSize
	if coordinateCount > uint64(remaining/(valueSummaryCoordinateIDSize+1)) {
		return 0, 0, 0, 0, false
	}
	count = int(coordinateCount)
	idOffset = valueSummaryResultHeaderSize
	presenceOffset = idOffset + count*valueSummaryCoordinateIDSize
	recordOffset = presenceOffset + count
	return idOffset, presenceOffset, recordOffset, count, true
}

func summaryResultUint64(payload string, offset int) uint64 {
	return uint64(payload[offset])<<56 |
		uint64(payload[offset+1])<<48 |
		uint64(payload[offset+2])<<40 |
		uint64(payload[offset+3])<<32 |
		uint64(payload[offset+4])<<24 |
		uint64(payload[offset+5])<<16 |
		uint64(payload[offset+6])<<8 |
		uint64(payload[offset+7])
}

func summaryResultIDAvailable(payload string, offset int) bool {
	return payload[offset] != 0 || payload[offset+1] != 0 || payload[offset+2] != 0 || payload[offset+3] != 0 ||
		payload[offset+4] != 0 || payload[offset+5] != 0 || payload[offset+6] != 0 || payload[offset+7] != 0 ||
		payload[offset+8] != 0 || payload[offset+9] != 0 || payload[offset+10] != 0 || payload[offset+11] != 0 ||
		payload[offset+12] != 0 || payload[offset+13] != 0 || payload[offset+14] != 0 || payload[offset+15] != 0 ||
		payload[offset+16] != 0 || payload[offset+17] != 0 || payload[offset+18] != 0 || payload[offset+19] != 0 ||
		payload[offset+20] != 0 || payload[offset+21] != 0 || payload[offset+22] != 0 || payload[offset+23] != 0 ||
		payload[offset+24] != 0 || payload[offset+25] != 0 || payload[offset+26] != 0 || payload[offset+27] != 0 ||
		payload[offset+28] != 0 || payload[offset+29] != 0 || payload[offset+30] != 0 || payload[offset+31] != 0
}

func summaryResultIDZero(payload string, offset int) bool {
	return !summaryResultIDAvailable(payload, offset)
}

func summaryResultDuplicateID(payload string, idOffset, index int) bool {
	if index == 0 {
		return false
	}
	current := idOffset + index*valueSummaryCoordinateIDSize
	for previous := 0; previous < index; previous++ {
		candidate := idOffset + previous*valueSummaryCoordinateIDSize
		equal := true
		for byteIndex := 0; byteIndex < valueSummaryCoordinateIDSize; byteIndex++ {
			if payload[current+byteIndex] != payload[candidate+byteIndex] {
				equal = false
				break
			}
		}
		if equal {
			return true
		}
	}
	return false
}
