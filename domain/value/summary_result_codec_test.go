package value

import (
	"encoding/binary"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func summaryCodecID(seed byte) identity.ContentID {
	var id identity.ContentID
	for index := range id {
		id[index] = seed + byte(index)
	}
	return id
}

func TestEncodeSummaryResultOwnsCompactCorrelatedImage(t *testing.T) {
	firstID := summaryCodecID(33)
	secondID := summaryCodecID(67)
	schema := summaryCodecSchema(firstID, secondID)
	observation := ValueSummaryObservation{
		Values: []Value{
			{schema: schema, image: []uint64{2, 5}},
			{schema: schema, top: true},
		},
		Present: []bool{true, true}, Rows: 1, Valid: true, owner: schema,
	}
	present, rows, payload, ok := EncodeSummaryResult(observation)
	if !ok || !present || rows != 1 || binary.BigEndian.Uint64(payload[:8]) != valueSummaryResultFormat {
		t.Fatal("value summary codec refused canonical observation")
	}
	if got := identity.ContentID(payload[8:40]); got != schema.LinkID() {
		t.Fatal("value summary codec lost Link owner identity")
	}
	for index, want := range []identity.ContentID{firstID, secondID} {
		start := valueSummaryResultHeaderSize + index*valueSummaryCoordinateIDSize
		if got := identity.ContentID(payload[start : start+valueSummaryCoordinateIDSize]); got != want {
			t.Fatalf("coordinate %d encoded at the wrong dense ordinal", index)
		}
	}
	before := append([]byte(nil), payload...)
	observation.Values[0].image[0] = 99
	if string(payload) != string(before) {
		t.Fatal("encoded payload aliases the mutable source image")
	}
}

func TestEncodeSummaryResultCoordinateIDsIgnoreMapOrder(t *testing.T) {
	firstID := summaryCodecID(101)
	secondID := summaryCodecID(135)
	left := summaryCodecSchemaWithOrdinals(map[identity.ContentID]uint32{
		firstID:  2,
		secondID: 1,
	})
	right := summaryCodecSchemaWithOrdinals(map[identity.ContentID]uint32{
		secondID: 1,
		firstID:  2,
	})
	leftObservation := ValueSummaryObservation{
		Values:  []Value{{schema: left, image: []uint64{1, 0}}, {schema: left, image: []uint64{1, 0}}},
		Present: []bool{true, true}, Rows: 1, Valid: true, owner: left,
	}
	rightObservation := ValueSummaryObservation{
		Values:  []Value{{schema: right, image: []uint64{1, 0}}, {schema: right, image: []uint64{1, 0}}},
		Present: []bool{true, true}, Rows: 1, Valid: true, owner: right,
	}
	_, _, leftPayload, leftOK := EncodeSummaryResult(leftObservation)
	_, _, rightPayload, rightOK := EncodeSummaryResult(rightObservation)
	if !leftOK || !rightOK || string(leftPayload) != string(rightPayload) {
		t.Fatal("coordinate IDs made the summary payload depend on map iteration order")
	}
}

func TestEncodeSummaryResultRowZeroRetainsOwnerAndCoordinateSlots(t *testing.T) {
	firstID := summaryCodecID(11)
	secondID := summaryCodecID(45)
	schema := summaryCodecSchema(firstID, secondID)
	observation := ValueSummaryObservation{
		Values:  make([]Value, 2),
		Present: []bool{false, false},
		Valid:   true,
		owner:   schema,
	}
	present, rows, payload, ok := EncodeSummaryResult(observation)
	if !ok || present || rows != 0 || identity.ContentID(payload[8:40]) != schema.LinkID() {
		t.Fatal("row-zero summary observation was rejected")
	}
	for index, want := range []identity.ContentID{firstID, secondID} {
		start := valueSummaryResultHeaderSize + index*valueSummaryCoordinateIDSize
		if got := identity.ContentID(payload[start : start+valueSummaryCoordinateIDSize]); got != want {
			t.Fatalf("row-zero coordinate %d lost its identity", index)
		}
	}
	for index := 0; index < len(observation.Values); index++ {
		if payload[valueSummaryResultHeaderSize+len(observation.Values)*valueSummaryCoordinateIDSize+index] != 0 {
			t.Fatal("row-zero summary encoded coordinate presence")
		}
	}
}

func TestEncodeSummaryResultRejectsMixedOwnersAndRowMismatch(t *testing.T) {
	left := &Schema{linkID: summaryCodecID(1), potential: 1}
	right := &Schema{linkID: summaryCodecID(2), potential: 1}
	mixed := ValueSummaryObservation{
		Values:  []Value{{schema: left, top: true}, {schema: right, top: true}},
		Present: []bool{true, true}, Rows: 1, Valid: true, owner: left,
	}
	if _, _, _, ok := EncodeSummaryResult(mixed); ok {
		t.Fatal("mixed Value owners encoded")
	}
	mixed.Values = []Value{{}, {}}
	mixed.Present = []bool{false, false}
	mixed.Rows = 1
	if _, _, _, ok := EncodeSummaryResult(mixed); ok {
		t.Fatal("row cardinality disagreed with Value presence")
	}
}

func summaryCodecSchema(coordinates ...identity.ContentID) *Schema {
	ordinals := make(map[identity.ContentID]uint32, len(coordinates))
	for index, id := range coordinates {
		ordinals[id] = uint32(index + 1)
	}
	return summaryCodecSchemaWithOrdinals(ordinals)
}

func summaryCodecSchemaWithOrdinals(coordinates map[identity.ContentID]uint32) *Schema {
	schema := &Schema{
		linkID:          summaryCodecID(1),
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
