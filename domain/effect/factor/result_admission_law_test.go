package factor

import (
	"encoding/binary"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/plane"
)

// A published Effect answer is read through the generic plane codec against
// this family's sealed layout. There is no Effect-side decoder: a domain
// wrapper over the layout would be a second reading of bytes the seal already
// describes.

func TestAdmitResultRoundTrip(t *testing.T) {
	atoms := []identity.ContentID{decodeResultTestID(1), decodeResultTestID(67)}
	observation := EffectObservation{
		Atoms: atoms, Rows: 1, Present: true, Valid: true, seal: sealAtoms(atoms),
	}
	present, rows, payload, ok := plane.Publish(exactResultTestLayout, exactPublicationProjection, observation)
	if !ok {
		t.Fatal("the effect-exact declaration rejected round-trip observation")
	}
	view, refusal := plane.Admit(exactResultTestLayout, present, rows, string(payload))
	row, rowOK := view.At(0)
	if refusal.Available() || !rowOK || !row.Written() || row.Flag(ExactColumnTop) || row.Count() != len(atoms) {
		t.Fatalf("admitted result = refusal:%s written:%v top:%v atoms:%d", refusal, row.Written(), row.Flag(ExactColumnTop), row.Count())
	}
	for index, want := range atoms {
		got, found := row.AtomAt(index)
		if !found || got != want {
			t.Fatalf("admitted atom %d = %v/%v, want %v/true", index, got, found, want)
		}
	}
	if _, found := row.AtomAt(-1); found {
		t.Fatal("admitted result accepted a negative atom index")
	}
	if _, found := row.AtomAt(row.Count()); found {
		t.Fatal("admitted result accepted an out-of-range atom index")
	}
}

func TestAdmitResultRoundTripsTopAndAbsent(t *testing.T) {
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
			present, rows, payload, ok := plane.Publish(exactResultTestLayout, exactPublicationProjection, observation)
			if !ok {
				t.Fatal("the effect-exact declaration rejected round-trip observation")
			}
			view, refusal := plane.Admit(exactResultTestLayout, present, rows, string(payload))
			row, rowOK := view.At(0)
			if refusal.Available() || !rowOK || row.Written() != test.present || row.Flag(ExactColumnTop) != test.top || row.Count() != 0 {
				t.Fatalf("admitted result = refusal:%s written:%v top:%v atoms:%d", refusal, row.Written(), row.Flag(ExactColumnTop), row.Count())
			}
		})
	}
}

// TestExactPublicationRefusesAnIncoherentObservation states Effect's own two
// cross-column laws over the layout: an answer no producer wrote decides
// nothing, and the algebra's top value subsumes every atom rather than listing
// them. Neither is a shape the sealed layout can express, so both belong to the
// one writer of that layout rather than being restated by every reader.
func TestExactPublicationRefusesAnIncoherentObservation(t *testing.T) {
	atoms := []identity.ContentID{decodeResultTestID(5)}
	cases := map[string]EffectObservation{
		"absent carrying top":   {Rows: 1, Top: true, Valid: true},
		"absent carrying atoms": {Rows: 1, Atoms: atoms, Valid: true, seal: sealAtoms(atoms)},
		"top carrying atoms":    {Rows: 1, Present: true, Top: true, Atoms: atoms, Valid: true, seal: sealAtoms(atoms)},
	}
	for name, observation := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, _, ok := plane.Publish(exactResultTestLayout, exactPublicationProjection, observation); ok {
				t.Fatal("an incoherent Effect observation reached the wire")
			}
		})
	}
}

func TestAdmitResultRejectsMalformedPayloads(t *testing.T) {
	atoms := []identity.ContentID{decodeResultTestID(1)}
	observation := EffectObservation{
		Atoms: atoms, Rows: 1, Present: true, Valid: true, seal: sealAtoms(atoms),
	}
	present, rows, payload, ok := plane.Publish(exactResultTestLayout, exactPublicationProjection, observation)
	if !ok {
		t.Fatal("the effect-exact declaration rejected malformed-payload fixture")
	}
	mutate := func(change func([]byte)) string {
		copyPayload := append([]byte(nil), payload...)
		change(copyPayload)
		return string(copyPayload)
	}
	reject := func(name string, metadataPresent bool, metadataRows uint64, encoded string, want plane.Refusal) {
		t.Run(name, func(t *testing.T) {
			view, refusal := plane.Admit(exactResultTestLayout, metadataPresent, metadataRows, encoded)
			if refusal != want || view.Available() {
				t.Fatalf("refusal = %v (%s), want %v (%s)", refusal, refusal, want, want)
			}
		})
	}

	reject("truncated header", present, rows, string(payload[:effectHeaderSize-1]), plane.RefusalTruncated)
	reject("truncated atom", present, rows, string(payload[:len(payload)-1]), plane.RefusalTail)
	reject("trailing bytes", present, rows, string(append(append([]byte(nil), payload...), 0)), plane.RefusalTail)
	reject("foreign revision", present, rows, mutate(func(raw []byte) {
		binary.BigEndian.PutUint64(raw[:8], plane.Format+1)
	}), plane.RefusalLayout)
	reject("present metadata mismatch", !present, rows, string(payload), plane.RefusalMetadata)
	reject("rows above one", present, 2, string(payload), plane.RefusalMetadata)
	reject("present without a result row", present, 0, string(payload), plane.RefusalMetadata)
	reject("undeclared row state", present, rows, mutate(func(raw []byte) {
		raw[effectStateAt] = 2
	}), plane.RefusalState)
	reject("top boolean byte", present, rows, mutate(func(raw []byte) {
		raw[effectTopAt] = 2
	}), plane.RefusalColumn)
	reject("absent row carrying a column", present, rows, mutate(func(raw []byte) {
		raw[effectStateAt], raw[effectTopAt] = 0, 1
	}), plane.RefusalAbsentRow)
	reject("unavailable atom", present, rows, mutate(func(raw []byte) {
		for index := effectTailAt; index < len(raw); index++ {
			raw[index] = 0
		}
	}), plane.RefusalColumn)
}

var (
	admitResultViewSink plane.View
	admitResultIDSink   identity.ContentID
	admitResultBoolSink bool
)

func TestAdmitResultAllocatesZero(t *testing.T) {
	atoms := []identity.ContentID{decodeResultTestID(33)}
	observation := EffectObservation{
		Atoms: atoms, Rows: 1, Present: true, Valid: true, seal: sealAtoms(atoms),
	}
	present, rows, payload, ok := plane.Publish(exactResultTestLayout, exactPublicationProjection, observation)
	if !ok {
		t.Fatal("the effect-exact declaration rejected allocation-law fixture")
	}
	payloadString := string(payload)
	allocations := testing.AllocsPerRun(100, func() {
		view, refusal := plane.Admit(exactResultTestLayout, present, rows, payloadString)
		row, rowOK := view.At(0)
		atom, atomOK := row.AtomAt(0)
		admitResultViewSink = view
		admitResultIDSink = atom
		admitResultBoolSink = !refusal.Available() && rowOK && row.Written() && atomOK
	})
	if allocations != 0 {
		t.Fatalf("plane.Admit allocations = %v, want zero", allocations)
	}
}

func decodeResultTestID(seed byte) identity.ContentID {
	var id identity.ContentID
	for index := range id {
		id[index] = seed + byte(index)
	}
	return id
}
