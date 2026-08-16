package artifact_test

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/artifact"
)

// TestArtifactExactKeyEnumerationReplaysCanonicalAtoms proves that sealed
// exact-key atom identity and typed payloads survive the only persistence
// boundary. Numeric Lua-equal spellings remain one atom after replay.
func TestArtifactExactKeyEnumerationReplaysCanonicalAtoms(t *testing.T) {
	original := mustLower(t, "exact-keys.lua", `
return {
	[true] = 1,
	[false] = 2,
	[7] = 3,
	[1.5] = 4,
	field = 5,
	[-0.0] = 6,
	[0] = 7,
	[7.0] = 8,
}
`)
	contract := mustProfile(t)
	encoded, err := artifact.Encode(original, contract, artifact.Metadata{Provenance: "exact-key-enumeration"})
	if err != nil {
		t.Fatal(err)
	}
	replayed, _, err := artifact.Decode(encoded, contract)
	if err != nil {
		t.Fatal(err)
	}
	want := []program.LiteralValue{
		{Kind: program.LiteralBool},
		{Kind: program.LiteralBool, Bool: true},
		{Kind: program.LiteralInteger},
		{Kind: program.LiteralInteger, Integer: 7},
		{Kind: program.LiteralFloat, FloatBits: math.Float64bits(1.5)},
		{Kind: program.LiteralString, String: "field"},
	}
	if original.ExactKeyCount() != len(want) || replayed.ExactKeyCount() != len(want) {
		t.Fatalf("ExactKeyCount original/replayed = %d/%d, want %d", original.ExactKeyCount(), replayed.ExactKeyCount(), len(want))
	}
	for index, value := range want {
		before, beforeOK := original.ExactKeyAt(index)
		after, afterOK := replayed.ExactKeyAt(index)
		if !beforeOK || !afterOK || before == 0 || after == 0 {
			t.Fatalf("ExactKeyAt(%d) original/replayed = %v/%v and %v/%v, want keys", index, before, beforeOK, after, afterOK)
		}
		beforePayload, beforePayloadOK := original.ExactKey(before)
		if !beforePayloadOK || beforePayload != value {
			t.Fatalf("original ExactKey(ExactKeyAt(%d)) = %#v/%v, want %#v/true", index, beforePayload, beforePayloadOK, value)
		}
		payload, payloadOK := replayed.ExactKey(after)
		if !payloadOK || payload != value {
			t.Fatalf("replayed ExactKey(ExactKeyAt(%d)) = %#v/%v, want %#v/true", index, payload, payloadOK, value)
		}
	}
}
