package value

import (
	"encoding/binary"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestDecodeSummaryResultRoundTripAndWalk(t *testing.T) {
	firstID := summaryDecodeTestID(31)
	secondID := summaryDecodeTestID(63)
	thirdID := summaryDecodeTestID(95)
	schema := summaryDecodeSchema(map[identity.ContentID]uint32{
		firstID:  1,
		secondID: 2,
		thirdID:  3,
	})
	observation := ValueSummaryObservation{
		Values:  []Value{{schema: schema, image: []uint64{2, 5}}, {}, {schema: schema, top: true}},
		Present: []bool{true, false, true}, Rows: 1, Valid: true, owner: schema,
	}
	present, rows, payload, ok := EncodeSummaryResult(observation)
	if !ok {
		t.Fatal("EncodeSummaryResult rejected decode fixture")
	}
	result, ok := DecodeSummaryResult(present, rows, string(payload))
	if !ok || !result.Available() || !result.LinkID().Available() || result.CoordinateCount() != 3 {
		t.Fatalf("decoded summary = available:%v owner:%v count:%d", result.Available(), result.LinkID().Available(), result.CoordinateCount())
	}
	iterator := result.Coordinates()
	coordinates := [3]SummaryResultCoordinate{}
	for index := range coordinates {
		coordinate, found := iterator.Next()
		if !found {
			t.Fatalf("coordinate %d missing", index)
		}
		coordinates[index] = coordinate
	}
	if _, found := iterator.Next(); found {
		t.Fatal("summary iterator yielded a trailing coordinate")
	}
	if got := coordinates[0].ID(); got != firstID || !coordinates[0].Present() || coordinates[0].Top() || coordinates[0].WordCount() != 2 {
		t.Fatalf("first coordinate metadata = id:%v present:%v top:%v words:%d", got, coordinates[0].Present(), coordinates[0].Top(), coordinates[0].WordCount())
	}
	for index, want := range []uint64{2, 5} {
		got, found := coordinates[0].WordAt(index)
		if !found || got != want {
			t.Fatalf("first coordinate word %d = %d/%v, want %d/true", index, got, found, want)
		}
	}
	if _, found := coordinates[0].WordAt(-1); found {
		t.Fatal("first coordinate accepted a negative word index")
	}
	if _, found := coordinates[0].WordAt(coordinates[0].WordCount()); found {
		t.Fatal("first coordinate accepted an out-of-range word index")
	}
	if coordinates[1].ID() != secondID || coordinates[1].Present() || coordinates[1].Top() || coordinates[1].WordCount() != 0 {
		t.Fatal("absent coordinate view was not empty")
	}
	if _, found := coordinates[1].WordAt(0); found {
		t.Fatal("absent coordinate exposed a compact word")
	}
	if coordinates[2].ID() != thirdID || !coordinates[2].Present() || !coordinates[2].Top() || coordinates[2].WordCount() != 0 {
		t.Fatal("top coordinate metadata was not preserved")
	}
}

func TestDecodeSummaryResultRoundTripsRowZero(t *testing.T) {
	firstID := summaryDecodeTestID(71)
	secondID := summaryDecodeTestID(103)
	thirdID := summaryDecodeTestID(135)
	schema := summaryDecodeSchema(map[identity.ContentID]uint32{firstID: 1, secondID: 2, thirdID: 3})
	observation := ValueSummaryObservation{
		Values: make([]Value, 3), Present: []bool{false, false, false}, Valid: true, owner: schema,
	}
	present, rows, payload, ok := EncodeSummaryResult(observation)
	if !ok || present || rows != 0 {
		t.Fatal("EncodeSummaryResult rejected row-zero decode fixture")
	}
	result, ok := DecodeSummaryResult(present, rows, string(payload))
	if !ok || !result.Available() || result.LinkID() != schema.LinkID() || result.CoordinateCount() != 3 {
		t.Fatal("row-zero summary did not decode")
	}
	wantIDs := []identity.ContentID{firstID, secondID, thirdID}
	iterator := result.Coordinates()
	for index := 0; index < result.CoordinateCount(); index++ {
		coordinate, found := iterator.Next()
		if !found || coordinate.ID() != wantIDs[index] || coordinate.Present() || coordinate.Top() || coordinate.WordCount() != 0 {
			t.Fatalf("row-zero coordinate %d lost metadata", index)
		}
	}
}

func TestDecodeSummaryResultRejectsMalformedPayloads(t *testing.T) {
	firstID := summaryDecodeTestID(17)
	secondID := summaryDecodeTestID(51)
	schema := summaryDecodeSchema(map[identity.ContentID]uint32{firstID: 1, secondID: 2})
	observation := ValueSummaryObservation{
		Values:  []Value{{schema: schema, image: []uint64{3, 8}}, {schema: schema, top: true}},
		Present: []bool{true, true}, Rows: 1, Valid: true, owner: schema,
	}
	present, rows, payload, ok := EncodeSummaryResult(observation)
	if !ok {
		t.Fatal("EncodeSummaryResult rejected malformed-payload fixture")
	}

	mutate := func(change func([]byte)) string {
		copyPayload := append([]byte(nil), payload...)
		change(copyPayload)
		return string(copyPayload)
	}
	reject := func(name string, metadataPresent bool, metadataRows uint64, encoded string) {
		t.Run(name, func(t *testing.T) {
			if result, decodedOK := DecodeSummaryResult(metadataPresent, metadataRows, encoded); decodedOK || result.Available() {
				t.Fatal("DecodeSummaryResult accepted malformed payload")
			}
		})
	}

	reject("truncated header", present, rows, string(payload[:valueSummaryResultHeaderSize-1]))
	reject("truncated record", present, rows, string(payload[:len(payload)-1]))
	reject("trailing bytes", present, rows, string(append(append([]byte(nil), payload...), 0)))
	reject("version", present, rows, mutate(func(raw []byte) {
		binary.BigEndian.PutUint64(raw[:8], valueSummaryResultFormat+1)
	}))
	reject("present metadata mismatch", !present, rows, string(payload))
	reject("rows metadata mismatch", present, 0, string(payload))
	reject("rows above one", present, 2, string(payload))
	reject("coordinate presence boolean", present, rows, mutate(func(raw []byte) {
		raw[valueSummaryResultHeaderSize+valueSummaryCoordinateIDSize*2] = 2
	}))
	reject("top boolean", present, rows, mutate(func(raw []byte) {
		raw[valueSummaryResultHeaderSize+2+valueSummaryCoordinateIDSize*2] = 2
	}))
	reject("word count out of bounds", present, rows, mutate(func(raw []byte) {
		firstRecord := valueSummaryResultHeaderSize + valueSummaryCoordinateIDSize*2 + 2
		binary.BigEndian.PutUint64(raw[firstRecord+1:firstRecord+9], 4)
	}))
	reject("unavailable owner", present, rows, mutate(func(raw []byte) {
		for index := 8; index < 40; index++ {
			raw[index] = 0
		}
	}))
	reject("unavailable coordinate", present, rows, mutate(func(raw []byte) {
		for index := valueSummaryResultHeaderSize; index < valueSummaryResultHeaderSize+valueSummaryCoordinateIDSize; index++ {
			raw[index] = 0
		}
	}))
	reject("duplicate coordinate", present, rows, mutate(func(raw []byte) {
		first := valueSummaryResultHeaderSize
		second := first + valueSummaryCoordinateIDSize
		copy(raw[second:second+valueSummaryCoordinateIDSize], raw[first:first+valueSummaryCoordinateIDSize])
	}))
	rowZero := mutate(func(raw []byte) {
		for index := 8; index < 40; index++ {
			raw[index] = 0
		}
		for index := valueSummaryResultHeaderSize; index < valueSummaryResultHeaderSize+valueSummaryCoordinateIDSize*2; index++ {
			raw[index] = 0
		}
		presence := valueSummaryResultHeaderSize + valueSummaryCoordinateIDSize*2
		raw[presence], raw[presence+1] = 0, 0
	})
	reject("row zero with rows one", false, 1, rowZero)
}

var (
	summaryDecodeResultSink     SummaryResult
	summaryDecodeCoordinateSink SummaryResultCoordinate
	summaryDecodeIDSink         identity.ContentID
	summaryDecodeWordSink       uint64
	summaryDecodeBoolSink       bool
)

func TestDecodeSummaryResultAllocatesZero(t *testing.T) {
	firstID := summaryDecodeTestID(23)
	secondID := summaryDecodeTestID(57)
	schema := summaryDecodeSchema(map[identity.ContentID]uint32{firstID: 1, secondID: 2})
	observation := ValueSummaryObservation{
		Values:  []Value{{schema: schema, image: []uint64{4, 9}}, {schema: schema, image: []uint64{7, 11}}},
		Present: []bool{true, true}, Rows: 1, Valid: true, owner: schema,
	}
	present, rows, payload, ok := EncodeSummaryResult(observation)
	if !ok {
		t.Fatal("EncodeSummaryResult rejected allocation-law fixture")
	}
	payloadString := string(payload)
	allocations := testing.AllocsPerRun(100, func() {
		result, decodedOK := DecodeSummaryResult(present, rows, payloadString)
		iterator := result.Coordinates()
		for {
			coordinate, found := iterator.Next()
			if !found {
				break
			}
			summaryDecodeCoordinateSink = coordinate
			summaryDecodeIDSink = coordinate.ID()
			for index := 0; index < coordinate.WordCount(); index++ {
				summaryDecodeWordSink, _ = coordinate.WordAt(index)
			}
			summaryDecodeBoolSink = coordinate.Present() && !coordinate.Top()
		}
		summaryDecodeResultSink = result
		summaryDecodeBoolSink = decodedOK && result.Available() && summaryDecodeResultSink.CoordinateCount() == 2
	})
	if allocations != 0 {
		t.Fatalf("DecodeSummaryResult allocations = %v, want zero", allocations)
	}
}

func summaryDecodeTestID(seed byte) identity.ContentID {
	var id identity.ContentID
	for index := range id {
		id[index] = seed + byte(index)
	}
	return id
}

func summaryDecodeSchema(coordinates map[identity.ContentID]uint32) *Schema {
	schema := &Schema{
		linkID:          summaryDecodeTestID(1),
		potential:       4,
		capWords:        1,
		coordinateCount: uint32(len(coordinates)),
		coordinates:     make(map[identity.ContentID]coordinateRow, len(coordinates)),
	}
	for id, ordinal := range coordinates {
		schema.coordinates[id] = coordinateRow{coordinate: ordinal}
	}
	return schema
}
