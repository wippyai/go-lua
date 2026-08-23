package value

import (
	"encoding/binary"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/plane"
)

func TestAdmitSummaryResultRoundTripAndWalk(t *testing.T) {
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
	present, rows, payload, ok := plane.Publish(summaryResultTestLayout, SummaryPublication().Projection, observation)
	if !ok {
		t.Fatal("the value summary declaration rejected decode fixture")
	}
	view, refusal := plane.Admit(summaryResultTestLayout, present, rows, string(payload))
	if refusal.Available() || !view.Owner().Available() || view.RowCount() != 3 {
		t.Fatalf("admitted summary = refusal:%s owner:%v count:%d", refusal, view.Owner().Available(), view.RowCount())
	}
	coordinates := [3]plane.Row{}
	for index := range coordinates {
		row, found := view.At(index)
		if !found {
			t.Fatalf("coordinate %d missing", index)
		}
		coordinates[index] = row
	}
	if _, found := view.At(len(coordinates)); found {
		t.Fatal("summary view yielded a trailing coordinate")
	}
	// Rows are published in ascending portable identity, which is the layout's
	// one row order; the seeds are chosen so that order is first/second/third.
	if got := coordinates[0].ID(); got != firstID || !coordinates[0].Written() || coordinates[0].Flag(SummaryColumnTop) || coordinates[0].Count() != 2 {
		t.Fatalf("first coordinate metadata = id:%v written:%v top:%v words:%d", got, coordinates[0].Written(), coordinates[0].Flag(SummaryColumnTop), coordinates[0].Count())
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
	if _, found := coordinates[0].WordAt(coordinates[0].Count()); found {
		t.Fatal("first coordinate accepted an out-of-range word index")
	}
	if coordinates[1].ID() != secondID || coordinates[1].Written() || coordinates[1].Flag(SummaryColumnTop) || coordinates[1].Count() != 0 {
		t.Fatal("absent coordinate view was not empty")
	}
	if _, found := coordinates[1].WordAt(0); found {
		t.Fatal("absent coordinate exposed a compact word")
	}
	if coordinates[2].ID() != thirdID || !coordinates[2].Written() || !coordinates[2].Flag(SummaryColumnTop) || coordinates[2].Count() != 0 {
		t.Fatal("top coordinate metadata was not preserved")
	}
	if row, found := view.Lookup(thirdID); !found || row.ID() != thirdID {
		t.Fatal("the coordinate plane did not resolve its own row by identity")
	}
}

func TestAdmitSummaryResultRoundTripsRowZero(t *testing.T) {
	firstID := summaryDecodeTestID(71)
	secondID := summaryDecodeTestID(103)
	thirdID := summaryDecodeTestID(135)
	schema := summaryDecodeSchema(map[identity.ContentID]uint32{firstID: 1, secondID: 2, thirdID: 3})
	observation := ValueSummaryObservation{
		Values: make([]Value, 3), Present: []bool{false, false, false}, Valid: true, owner: schema,
	}
	present, rows, payload, ok := plane.Publish(summaryResultTestLayout, SummaryPublication().Projection, observation)
	if !ok || present || rows != 0 {
		t.Fatal("the value summary declaration rejected row-zero decode fixture")
	}
	view, refusal := plane.Admit(summaryResultTestLayout, present, rows, string(payload))
	if refusal.Available() || view.Owner() != schema.LinkID() || view.RowCount() != 3 {
		t.Fatalf("row-zero summary did not admit: %s", refusal)
	}
	wantIDs := []identity.ContentID{firstID, secondID, thirdID}
	for index := 0; index < view.RowCount(); index++ {
		row, found := view.At(index)
		if !found || row.ID() != wantIDs[index] || row.Written() || row.Flag(SummaryColumnTop) || row.Count() != 0 {
			t.Fatalf("row-zero coordinate %d lost metadata", index)
		}
	}
}

func TestAdmitSummaryResultRejectsMalformedPayloads(t *testing.T) {
	firstID := summaryDecodeTestID(17)
	secondID := summaryDecodeTestID(51)
	schema := summaryDecodeSchema(map[identity.ContentID]uint32{firstID: 1, secondID: 2})
	observation := ValueSummaryObservation{
		Values:  []Value{{schema: schema, image: []uint64{3, 8}}, {schema: schema, top: true}},
		Present: []bool{true, true}, Rows: 1, Valid: true, owner: schema,
	}
	present, rows, payload, ok := plane.Publish(summaryResultTestLayout, SummaryPublication().Projection, observation)
	if !ok {
		t.Fatal("the value summary declaration rejected malformed-payload fixture")
	}

	mutate := func(change func([]byte)) string {
		copyPayload := append([]byte(nil), payload...)
		change(copyPayload)
		return string(copyPayload)
	}
	reject := func(name string, metadataPresent bool, metadataRows uint64, encoded string) {
		t.Run(name, func(t *testing.T) {
			view, refusal := plane.Admit(summaryResultTestLayout, metadataPresent, metadataRows, encoded)
			if !refusal.Available() || view.Available() {
				t.Fatal("plane.Admit accepted a malformed value-summary payload")
			}
		})
	}

	reject("truncated header", present, rows, string(payload[:valueSummaryHeaderSize-1]))
	reject("truncated record", present, rows, string(payload[:len(payload)-1]))
	reject("trailing bytes", present, rows, string(append(append([]byte(nil), payload...), 0)))
	reject("version", present, rows, mutate(func(raw []byte) {
		binary.BigEndian.PutUint64(raw[:8], plane.Format+1)
	}))
	reject("present metadata mismatch", !present, rows, string(payload))
	reject("rows metadata mismatch", present, 0, string(payload))
	reject("rows above one", present, 2, string(payload))
	reject("undeclared row state", present, rows, mutate(func(raw []byte) {
		raw[valueSummaryRowAt(2, 0)] = 2
	}))
	reject("top boolean", present, rows, mutate(func(raw []byte) {
		raw[valueSummaryRowAt(2, 0)+1] = 2
	}))
	reject("image extent beyond the payload", present, rows, mutate(func(raw []byte) {
		offsets := valueSummaryOffsetsAt(2)
		binary.BigEndian.PutUint64(raw[offsets+8:offsets+16], 4*8)
	}))
	reject("unavailable owner", present, rows, mutate(func(raw []byte) {
		for index := valueSummaryOwnerAt; index < valueSummaryOwnerAt+32; index++ {
			raw[index] = 0
		}
	}))
	reject("unavailable coordinate", present, rows, mutate(func(raw []byte) {
		for index := valueSummaryHeaderSize; index < valueSummaryHeaderSize+valueSummaryCoordinateIDSize; index++ {
			raw[index] = 0
		}
	}))
	reject("duplicate coordinate", present, rows, mutate(func(raw []byte) {
		first := valueSummaryHeaderSize
		second := first + valueSummaryCoordinateIDSize
		copy(raw[second:second+valueSummaryCoordinateIDSize], raw[first:first+valueSummaryCoordinateIDSize])
	}))
	rowZeroObservation := ValueSummaryObservation{
		Values: make([]Value, 2), Present: []bool{false, false}, Valid: true, owner: schema,
	}
	_, _, rowZero, rowZeroOK := plane.Publish(summaryResultTestLayout, SummaryPublication().Projection, rowZeroObservation)
	if !rowZeroOK {
		t.Fatal("the value summary declaration rejected the row-zero fixture")
	}
	// A value summary that observed a result row while writing no coordinate is
	// refused by the encoder, which is the authority for that coupling; the
	// generic admission holds the metadata to the bytes and nothing more.
	reject("row zero claimed present", true, 0, string(rowZero))
}

var (
	summaryDecodeResultSink     plane.View
	summaryDecodeCoordinateSink plane.Row
	summaryDecodeIDSink         identity.ContentID
	summaryDecodeWordSink       uint64
	summaryDecodeBoolSink       bool
)

func TestAdmitSummaryResultAllocatesZero(t *testing.T) {
	firstID := summaryDecodeTestID(23)
	secondID := summaryDecodeTestID(57)
	schema := summaryDecodeSchema(map[identity.ContentID]uint32{firstID: 1, secondID: 2})
	observation := ValueSummaryObservation{
		Values:  []Value{{schema: schema, image: []uint64{4, 9}}, {schema: schema, image: []uint64{7, 11}}},
		Present: []bool{true, true}, Rows: 1, Valid: true, owner: schema,
	}
	present, rows, payload, ok := plane.Publish(summaryResultTestLayout, SummaryPublication().Projection, observation)
	if !ok {
		t.Fatal("the value summary declaration rejected allocation-law fixture")
	}
	payloadString := string(payload)
	allocations := testing.AllocsPerRun(100, func() {
		view, refusal := plane.Admit(summaryResultTestLayout, present, rows, payloadString)
		for index := 0; index < view.RowCount(); index++ {
			row, found := view.At(index)
			if !found {
				break
			}
			summaryDecodeCoordinateSink = row
			summaryDecodeIDSink = row.ID()
			for word := 0; word < row.Count(); word++ {
				summaryDecodeWordSink, _ = row.WordAt(word)
			}
			summaryDecodeBoolSink = row.Written() && !row.Flag(SummaryColumnTop)
		}
		summaryDecodeResultSink = view
		summaryDecodeBoolSink = !refusal.Available() && view.RowCount() == 2
	})
	if allocations != 0 {
		t.Fatalf("plane.Admit allocations = %v, want zero", allocations)
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
	schema.installCanonicalCoordinateOrder()
	return schema
}
