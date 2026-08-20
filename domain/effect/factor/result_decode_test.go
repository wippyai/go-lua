package factor

import (
	"encoding/binary"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestDecodeResultRoundTrip(t *testing.T) {
	atoms := []identity.ContentID{decodeResultTestID(1), decodeResultTestID(67)}
	observation := EffectObservation{
		Atoms: atoms, Rows: 1, Present: true, Valid: true, seal: sealAtoms(atoms),
	}
	present, rows, payload, ok := EncodeResult(observation)
	if !ok {
		t.Fatal("EncodeResult rejected round-trip observation")
	}
	result, ok := DecodeResult(present, rows, string(payload))
	if !ok || !result.Available() || !result.Present() || result.Top() || result.AtomCount() != len(atoms) {
		t.Fatalf("decoded result metadata = available:%v present:%v top:%v atoms:%d", result.Available(), result.Present(), result.Top(), result.AtomCount())
	}
	for index, want := range atoms {
		got, found := result.AtomAt(index)
		if !found || got != want {
			t.Fatalf("decoded atom %d = %v/%v, want %v/true", index, got, found, want)
		}
	}
	if _, found := result.AtomAt(-1); found {
		t.Fatal("decoded result accepted a negative atom index")
	}
	if _, found := result.AtomAt(result.AtomCount()); found {
		t.Fatal("decoded result accepted an out-of-range atom index")
	}
}

func TestDecodeResultRoundTripsTopAndAbsent(t *testing.T) {
	tests := []struct {
		name    string
		rows    uint32
		present bool
		top     bool
		seal    uint64
	}{
		{name: "top", rows: 1, present: true, top: true, seal: sealAtoms(nil)},
		{name: "absent one row without seal", rows: 1},
		{name: "absent zero rows without seal", rows: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observation := EffectObservation{Rows: test.rows, Present: test.present, Top: test.top, Valid: true, seal: test.seal}
			present, rows, payload, ok := EncodeResult(observation)
			if !ok {
				t.Fatal("EncodeResult rejected round-trip observation")
			}
			result, ok := DecodeResult(present, rows, string(payload))
			if !ok || !result.Available() || result.Present() != test.present || result.Top() != test.top || result.AtomCount() != 0 {
				t.Fatalf("decoded result = available:%v present:%v top:%v atoms:%d", result.Available(), result.Present(), result.Top(), result.AtomCount())
			}
		})
	}
}

func TestDecodeResultRejectsMalformedPayloads(t *testing.T) {
	atoms := []identity.ContentID{decodeResultTestID(1)}
	observation := EffectObservation{
		Atoms: atoms, Rows: 1, Present: true, Valid: true, seal: sealAtoms(atoms),
	}
	present, rows, payload, ok := EncodeResult(observation)
	if !ok {
		t.Fatal("EncodeResult rejected malformed-payload fixture")
	}

	mutate := func(change func([]byte)) string {
		copyPayload := append([]byte(nil), payload...)
		change(copyPayload)
		return string(copyPayload)
	}
	reject := func(name string, metadataPresent bool, metadataRows uint64, encoded string) {
		t.Run(name, func(t *testing.T) {
			if result, ok := DecodeResult(metadataPresent, metadataRows, encoded); ok || result.Available() {
				t.Fatal("DecodeResult accepted malformed payload")
			}
		})
	}

	reject("truncated header", present, rows, string(payload[:effectResultHeaderSize-1]))
	reject("truncated atom", present, rows, string(payload[:len(payload)-1]))
	trailing := append(append([]byte(nil), payload...), 0)
	reject("trailing bytes", present, rows, string(trailing))
	reject("version", present, rows, mutate(func(raw []byte) { binary.BigEndian.PutUint64(raw[:8], effectResultFormat+1) }))
	reject("present metadata mismatch", !present, rows, string(payload))
	reject("rows above one", present, 2, string(payload))
	reject("present boolean byte", present, rows, mutate(func(raw []byte) { raw[8] = 2 }))
	reject("top boolean byte", present, rows, mutate(func(raw []byte) { raw[9] = 2 }))
	reject("absent top", false, rows, mutate(func(raw []byte) {
		raw[8], raw[9], raw[10], raw[11], raw[12], raw[13], raw[14], raw[15], raw[16], raw[17] = 0, 1, 0, 0, 0, 0, 0, 0, 0, 0
	}))
	reject("absent atoms", false, rows, mutate(func(raw []byte) { raw[8] = 0 }))
	reject("top atoms", true, rows, mutate(func(raw []byte) { raw[9] = 1 }))
	reject("unavailable atom", present, rows, mutate(func(raw []byte) {
		for index := effectResultHeaderSize; index < len(raw); index++ {
			raw[index] = 0
		}
	}))
}

var (
	decodeResultSink     Result
	decodeResultIDSink   identity.ContentID
	decodeResultBoolSink bool
)

func TestDecodeResultAllocatesZero(t *testing.T) {
	atoms := []identity.ContentID{decodeResultTestID(33)}
	observation := EffectObservation{
		Atoms: atoms, Rows: 1, Present: true, Valid: true, seal: sealAtoms(atoms),
	}
	present, rows, payload, ok := EncodeResult(observation)
	if !ok {
		t.Fatal("EncodeResult rejected allocation-law fixture")
	}
	payloadString := string(payload)
	allocations := testing.AllocsPerRun(100, func() {
		result, decodedOK := DecodeResult(present, rows, payloadString)
		atom, atomOK := result.AtomAt(0)
		decodeResultSink = result
		decodeResultIDSink = atom
		decodeResultBoolSink = decodedOK && result.Available() && result.Present() && atomOK
	})
	if allocations != 0 {
		t.Fatalf("DecodeResult allocations = %v, want zero", allocations)
	}
}

func decodeResultTestID(seed byte) identity.ContentID {
	var id identity.ContentID
	for index := range id {
		id[index] = seed + byte(index)
	}
	return id
}
