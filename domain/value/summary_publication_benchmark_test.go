package value

import (
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/plane"
)

var (
	summaryResultCodecPayloadSink    []byte
	summaryResultCodecDecodeSink     plane.View
	summaryResultCodecDecodeOKSink   bool
	summaryResultCodecCoordinateSink identity.ContentID
	summaryResultCodecWordSink       uint64
)

// BenchmarkPublishSummaryResult measures the detached wire image at the small,
// ordinary, and wide coordinate widths used by summary queries.  Fixtures are
// built before timing so the benchmark covers the declared publication walk
// itself.
func BenchmarkPublishSummaryResult(b *testing.B) {
	for _, coordinates := range []int{1, 16, 128} {
		coordinates := coordinates
		b.Run("coordinates="+strconv.Itoa(coordinates), func(b *testing.B) {
			observation := summaryResultCodecBenchmarkObservation(coordinates)
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				_, _, payload, ok := plane.Publish(summaryResultTestLayout, summaryPublicationProjection, observation)
				if !ok {
					b.Fatal("the value summary declaration refused a benchmark observation")
				}
				summaryResultCodecPayloadSink = payload
			}
		})
	}
}

// BenchmarkAdmitSummaryResult measures opening and walking the detached wire
// image at the same coordinate widths as BenchmarkPublishSummaryResult.  The
// payload is encoded before timing so the benchmark covers admission and the
// complete coordinate/word iteration only.
func BenchmarkAdmitSummaryResult(b *testing.B) {
	for _, coordinates := range []int{1, 16, 128} {
		coordinates := coordinates
		b.Run("coordinates="+strconv.Itoa(coordinates), func(b *testing.B) {
			observation := summaryResultCodecBenchmarkObservation(coordinates)
			present, rows, payload, ok := plane.Publish(summaryResultTestLayout, summaryPublicationProjection, observation)
			if !ok {
				b.Fatal("the value summary declaration refused a decode benchmark observation")
			}
			encoded := string(payload)
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				view, refusal := plane.Admit(summaryResultTestLayout, present, rows, encoded)
				summaryResultCodecDecodeSink = view
				summaryResultCodecDecodeOKSink = !refusal.Available() &&
					view.RowCount() == coordinates
				var coordinateCount int
				for index := 0; index < view.RowCount(); index++ {
					coordinate, found := view.At(index)
					if !found {
						break
					}
					coordinateCount++
					summaryResultCodecCoordinateSink = coordinate.ID()
					present := coordinate.Written()
					top := coordinate.Flag(SummaryColumnTop)
					wordCount := coordinate.Count()
					if !present || top || wordCount == 0 {
						summaryResultCodecDecodeOKSink = false
					}
					for wordIndex := 0; wordIndex < wordCount; wordIndex++ {
						word, wordOK := coordinate.WordAt(wordIndex)
						summaryResultCodecWordSink ^= word
						if !wordOK {
							summaryResultCodecDecodeOKSink = false
						}
					}
				}
				if coordinateCount != coordinates {
					summaryResultCodecDecodeOKSink = false
				}
				if !summaryResultCodecDecodeOKSink {
					b.Fatal("summary result codec failed decode benchmark iteration")
				}
			}
		})
	}
}

func TestPublishSummaryResultAllocatesOnePayload(t *testing.T) {
	observation := summaryResultCodecBenchmarkObservation(16)
	allocations := testing.AllocsPerRun(100, func() {
		_, _, payload, ok := plane.Publish(summaryResultTestLayout, summaryPublicationProjection, observation)
		if !ok {
			t.Fatal("the value summary declaration refused an allocation-law observation")
		}
		summaryResultCodecPayloadSink = payload
	})
	if allocations != 1 {
		t.Fatalf("publishing a value summary allocates = %v, want one payload allocation", allocations)
	}
}

func summaryResultCodecBenchmarkObservation(coordinates int) ValueSummaryObservation {
	schema := &Schema{
		linkID:          summaryResultCodecBenchmarkID(1),
		potential:       uint64(coordinates),
		capWords:        0,
		coordinateCount: uint32(coordinates),
		coordinates:     make(map[identity.ContentID]coordinateRow, coordinates),
	}
	values := make([]Value, coordinates)
	image := make([]uint64, coordinates)
	present := make([]bool, coordinates)
	for index := range values {
		coordinateID := summaryResultCodecBenchmarkID(byte(index + 2))
		schema.coordinates[coordinateID] = coordinateRow{coordinate: uint32(index + 1)}
		image[index] = uint64(index + 1)
		values[index] = Value{schema: schema, image: image[index : index+1]}
		present[index] = true
	}
	schema.installCanonicalCoordinateOrder()
	return ValueSummaryObservation{Values: values, Present: present, Rows: 1, Valid: true, owner: schema}
}

func summaryResultCodecBenchmarkID(seed byte) identity.ContentID {
	var id identity.ContentID
	for index := range id {
		id[index] = seed + byte(index)
	}
	return id
}
