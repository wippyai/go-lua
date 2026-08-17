package contract

import (
	"encoding/hex"
	"testing"
)

// The denied-entry payload is a member its owner declares and will not hand out.
// A library refuses a member it models; an environment additionally boots without
// one at all, and the two are different facts about the same address: a refused
// member exists and is withheld, an absent one is not there. A payload that could
// not tell them apart would make every consumer guess which it was reading.

func deniedEntryCorpus() []DeniedEntry {
	return []DeniedEntry{
		{Denial: DenialRefused, Entry: Export("dump")},
		{Denial: DenialAbsent, Entry: Export("dump")},
		{Denial: DenialRefused, Entry: NewPath(Step{Kind: StepExport, Key: "io"}, Step{Kind: StepExport, Key: "open"})},
		{Denial: DenialAbsent, Entry: NewPath(Step{Kind: StepExport, Key: "io"}, Step{Kind: StepExport, Key: "stdin"})},
		{Denial: DenialRefused, Entry: Metatable("__index")},
	}
}

// TestDeniedEntryRoundTripsThroughItsOwnBytes states the format from both ends:
// what it writes it reads back as the same refusal, for every address it carries.
func TestDeniedEntryRoundTripsThroughItsOwnBytes(t *testing.T) {
	for _, denied := range deniedEntryCorpus() {
		data, err := EncodeDeniedEntry(denied)
		if err != nil {
			t.Fatalf("%+v: the denied entry did not encode: %v", denied, err)
		}
		got, err := DecodeDeniedEntry(data)
		if err != nil {
			t.Fatalf("%+v: the denied entry did not decode: %v", denied, err)
		}
		if got.Denial != denied.Denial || !got.Entry.Equal(denied.Entry) {
			t.Fatalf("the payload reads back as %+v, want %+v", got, denied)
		}
	}
}

// TestDeniedEntryDistinguishesRefusalFromAbsence is the law the format exists
// for. One address, two dispositions, two payloads: a member the host refuses to
// publish is not a member the host never booted.
func TestDeniedEntryDistinguishesRefusalFromAbsence(t *testing.T) {
	written := make(map[string]DeniedEntry)
	for _, denied := range deniedEntryCorpus() {
		data, err := EncodeDeniedEntry(denied)
		if err != nil {
			t.Fatalf("%+v: the denied entry did not encode: %v", denied, err)
		}
		key := string(data)
		if prior, collided := written[key]; collided {
			t.Fatalf("%+v and %+v write one payload", prior, denied)
		}
		written[key] = denied
	}
}

// TestDeniedEntryRejectsWhatItCannotCarry is the boundary law. A refusal with no
// disposition says nothing about the member it names, a refusal of an unwalkable
// address names no member, and another payload's stream is not this one.
func TestDeniedEntryRejectsWhatItCannotCarry(t *testing.T) {
	rejected := []DeniedEntry{
		{},
		{Entry: Export("dump")},
		{Denial: DenialRefused},
		{Denial: DenialRefused, Entry: Export("")},
		{Denial: denialLimit, Entry: Export("dump")},
	}
	for _, denied := range rejected {
		if denied.Available() {
			t.Fatalf("%+v is admitted as a denied entry", denied)
		}
		if _, err := EncodeDeniedEntry(denied); err == nil {
			t.Fatalf("%+v encoded as a payload", denied)
		}
	}
	if _, err := DecodeDeniedEntry(nil); err == nil {
		t.Fatal("an empty payload decoded as a denied entry")
	}
	path, err := EncodePath(Export("dump"))
	if err != nil {
		t.Fatalf("the export path did not encode: %v", err)
	}
	if _, err := DecodeDeniedEntry(path); err == nil {
		t.Fatal("an export path payload decoded as a denied entry")
	}
	denied, err := EncodeDeniedEntry(DeniedEntry{Denial: DenialRefused, Entry: Export("dump")})
	if err != nil {
		t.Fatalf("the denied entry did not encode: %v", err)
	}
	if _, err := DecodePath(denied); err == nil {
		t.Fatal("a denied entry payload decoded as an export path")
	}
	if _, err := DecodeDeniedEntry(denied[:len(denied)-1]); err == nil {
		t.Fatal("a truncated denied entry decoded as a published refusal")
	}
	if _, err := DecodeDeniedEntry(append(append([]byte(nil), denied...), 0)); err == nil {
		t.Fatal("a denied entry with trailing bytes decoded as a published refusal")
	}
}

// TestDeniedEntryPayloadWireIsPinned holds the payload bytes still. A refusal
// published in a shipped contract is read by another build, so its encoding is a
// compatibility surface and not an implementation detail.
func TestDeniedEntryPayloadWireIsPinned(t *testing.T) {
	const pinnedRefused = "0126616e616c797369732f6c6962726172792f636f6e74726163742f64656e6965642d656e74727902010105010107390125616e616c797369732f6c6962726172792f636f6e74726163742f6578706f72742d70617468020101040101030103050101080464756d70"
	const pinnedAbsent = "0126616e616c797369732f6c6962726172792f636f6e74726163742f64656e6965642d656e74727902010105010207390125616e616c797369732f6c6962726172792f636f6e74726163742f6578706f72742d70617468020101040101030103050101080464756d70"
	refused, err := EncodeDeniedEntry(DeniedEntry{Denial: DenialRefused, Entry: Export("dump")})
	if err != nil {
		t.Fatalf("the refused entry did not encode: %v", err)
	}
	if got := hex.EncodeToString(refused); got != pinnedRefused {
		t.Errorf("refused entry payload wire is %s, pinned %s", got, pinnedRefused)
	}
	absent, err := EncodeDeniedEntry(DeniedEntry{Denial: DenialAbsent, Entry: Export("dump")})
	if err != nil {
		t.Fatalf("the absent entry did not encode: %v", err)
	}
	if got := hex.EncodeToString(absent); got != pinnedAbsent {
		t.Errorf("absent entry payload wire is %s, pinned %s", got, pinnedAbsent)
	}
}
