package factor

import (
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/plane"
)

var (
	effectResultCodecPayloadSink  []byte
	effectResultCodecDecodeSink   plane.View
	effectResultCodecDecodeOKSink bool
	effectResultCodecAtomSink     identity.ContentID
)

// BenchmarkEncodeResult measures the detached wire image at the small,
// ordinary, and wide atom widths used by Effect observations.  Fixtures are
// built before timing so the benchmark covers EncodeResult itself.
func BenchmarkEncodeResult(b *testing.B) {
	for _, atoms := range []int{1, 16, 128} {
		atoms := atoms
		b.Run("atoms="+strconv.Itoa(atoms), func(b *testing.B) {
			observation := resultCodecBenchmarkObservation(atoms)
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				_, _, payload, ok := EncodeResult(exactResultTestLayout, observation)
				if !ok {
					b.Fatal("effect result codec refused benchmark observation")
				}
				effectResultCodecPayloadSink = payload
			}
		})
	}
}

// BenchmarkAdmitResult measures opening and walking the detached wire image at
// the same atom widths as BenchmarkEncodeResult.  The payload is encoded before
// timing so the benchmark covers admission and the complete atom iteration
// only.
func BenchmarkAdmitResult(b *testing.B) {
	for _, atoms := range []int{1, 16, 128} {
		atoms := atoms
		b.Run("atoms="+strconv.Itoa(atoms), func(b *testing.B) {
			observation := resultCodecBenchmarkObservation(atoms)
			present, rows, payload, ok := EncodeResult(exactResultTestLayout, observation)
			if !ok {
				b.Fatal("effect result codec refused decode benchmark observation")
			}
			encoded := string(payload)
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				view, refusal := plane.Admit(exactResultTestLayout, present, rows, encoded)
				row, rowOK := view.At(0)
				effectResultCodecDecodeSink = view
				effectResultCodecDecodeOKSink = !refusal.Available() && rowOK &&
					row.Written() && row.Count() == atoms
				for atomIndex := 0; atomIndex < atoms; atomIndex++ {
					atom, atomOK := row.AtomAt(atomIndex)
					effectResultCodecAtomSink = atom
					if !atomOK || !atom.Available() {
						effectResultCodecDecodeOKSink = false
					}
				}
				if !effectResultCodecDecodeOKSink {
					b.Fatal("effect result codec failed decode benchmark iteration")
				}
			}
		})
	}
}

func TestEncodeResultAllocatesOnePayload(t *testing.T) {
	observation := resultCodecBenchmarkObservation(16)
	allocations := testing.AllocsPerRun(100, func() {
		_, _, payload, ok := EncodeResult(exactResultTestLayout, observation)
		if !ok {
			t.Fatal("effect result codec refused allocation-law observation")
		}
		effectResultCodecPayloadSink = payload
	})
	if allocations != 1 {
		t.Fatalf("EncodeResult allocations = %v, want one payload allocation", allocations)
	}
}

func resultCodecBenchmarkObservation(atoms int) EffectObservation {
	projection := make([]identity.ContentID, atoms)
	for index := range projection {
		for byteIndex := range projection[index] {
			projection[index][byteIndex] = byte(index + byteIndex + 1)
		}
	}
	return EffectObservation{Atoms: projection, Rows: 1, Present: true, Valid: true, seal: sealAtoms(projection)}
}
