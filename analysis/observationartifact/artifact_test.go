package observationartifact

import (
	"bytes"
	"crypto/sha256"
	"testing"

	engineobservation "github.com/wippyai/go-lua/analysis/engine/observation"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

type testCodec struct{ id string }

func (c testCodec) ID() string                      { return c.id }
func (testCodec) ValidateCanonical(raw []byte) bool { return len(raw) == 0 || raw[0] != 0xff }

func testUniverse(t *testing.T, axis byte) Universe {
	t.Helper()
	var semantic, axes [sha256.Size]byte
	semantic[0], axes[0] = 1, axis
	u, ok := SealUniverse(semantic, axes, testCodec{id: "axis-inventory.v1"})
	if !ok {
		t.Fatal("universe did not seal")
	}
	return u
}

func testRecord(t *testing.T, u Universe, payload byte) Record {
	t.Helper()
	var owner lexicalidentity.StableLexicalBodyID
	owner[0] = 1
	var artifact [sha256.Size]byte
	artifact[0] = 2
	anchor, ok := NewSourceAnchor(artifact, engineobservation.Occurrence{Point: wir.DebugPointID{Ordinal: 3, Phase: wir.DebugPhaseAfter}, Kind: engineobservation.Assignment})
	if !ok {
		t.Fatal("anchor")
	}
	actual, ok := u.NewEncodedValue([]byte{payload})
	if !ok {
		t.Fatal("value")
	}
	return Record{Owner: owner, Anchor: anchor, Actual: actual}
}

func TestArtifactCanonicalRoundTripAndCorrelatedAlternatives(t *testing.T) {
	u := testUniverse(t, 1)
	a, b := testRecord(t, u, 1), testRecord(t, u, 2)
	first, err := Encode(u, []Record{b, a, a})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Encode(u, []Record{a, b})
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("canonical encode mismatch: %v", err)
	}
	decoded, err := Decode(u, first)
	if err != nil || len(decoded.Records) != 2 {
		t.Fatalf("decode = %#v, %v", decoded, err)
	}
	occA, _ := a.OccurrenceIdentity()
	occB, _ := b.OccurrenceIdentity()
	recA, _ := a.Identity()
	recB, _ := b.Identity()
	if occA != occB || recA == recB {
		t.Fatal("occurrence and correlated record identities are not separated")
	}
}

func TestArtifactUniverseIncludesArbitraryAxisInventory(t *testing.T) {
	one, many := testUniverse(t, 1), testUniverse(t, 19)
	d1, _ := one.Digest()
	d2, _ := many.Digest()
	if d1 == d2 {
		t.Fatal("axis descriptor inventory missing from universe")
	}
	raw, _ := Encode(one, []Record{testRecord(t, one, 1)})
	if _, err := Decode(many, raw); err == nil {
		t.Fatal("cross-axis universe decoded")
	}
}

func TestArtifactRejectsInvalidCodecAndMalformedWire(t *testing.T) {
	u := testUniverse(t, 1)
	if _, ok := u.NewEncodedValue([]byte{0xff}); ok {
		t.Fatal("invalid codec payload admitted")
	}
	raw, err := Encode(u, []Record{testRecord(t, u, 1)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(u, append(append([]byte(nil), raw...), 0)); err == nil {
		t.Fatal("trailing bytes admitted")
	}
	if _, err := Decode(u, raw[:len(raw)-1]); err == nil {
		t.Fatal("truncated bytes admitted")
	}
	// Header(42) + record length(4) + owner/invocation(64) + artifact
	// digest(32) + debug ordinal(4) = phase; kind follows; the expected flag
	// follows the complete 42-byte anchor.
	for name, mutation := range map[string]struct {
		offset int
		value  byte
	}{
		"phase": {146, 0xff}, "kind": {147, 0xff}, "bool": {152, 2},
	} {
		mutated := append([]byte(nil), raw...)
		mutated[mutation.offset] = mutation.value
		if _, err := Decode(u, mutated); err == nil {
			t.Fatalf("malformed %s admitted", name)
		}
	}
	a, b := testRecord(t, u, 1), testRecord(t, u, 2)
	ordered, _ := Encode(u, []Record{a, b})
	r := wireReader{raw: ordered}
	header := append([]byte(nil), r.take(42)...)
	first, second := r.bytes(), r.bytes()
	var reversed bytes.Buffer
	_, _ = reversed.Write(header)
	writeBytes(&reversed, second)
	writeBytes(&reversed, first)
	if _, err := Decode(u, reversed.Bytes()); err == nil {
		t.Fatal("noncanonical record order admitted")
	}
}

func TestColdBuildObservationIdentityIsDeterministic(t *testing.T) {
	u := testUniverse(t, 1)
	first := testRecord(t, u, 7)
	second := testRecord(t, u, 7)
	a, _ := first.Identity()
	b, _ := second.Identity()
	if a != b {
		t.Fatalf("cold identities differ: %x != %x", a, b)
	}
}

func TestEncodedValuesAndDecodedRecordsOwnPayloadBytes(t *testing.T) {
	u := testUniverse(t, 1)
	payload := []byte{7}
	value, ok := u.NewEncodedValue(payload)
	if !ok {
		t.Fatal("value")
	}
	payload[0] = 9
	if value.Bytes()[0] != 7 {
		t.Fatal("encoded value aliases caller bytes")
	}
	record := testRecord(t, u, 7)
	raw, _ := Encode(u, []Record{record})
	decoded, err := Decode(u, raw)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 1
	if decoded.Records[0].Actual.Bytes()[0] != 7 {
		t.Fatal("decoded record aliases wire bytes")
	}
}
