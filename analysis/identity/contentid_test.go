package identity

import (
	"crypto/sha256"
	"strings"
	"testing"
)

func TestContentIDOwnsExactFixedWidthIdentity(t *testing.T) {
	var zero ContentID
	if zero.Available() || zero.String() != strings.Repeat("0", 64) {
		t.Fatalf("zero ContentID = %q/%v", zero.String(), zero.Available())
	}
	var id ContentID
	id[0], id[len(id)-1] = 0x12, 0xab
	if !id.Available() {
		t.Fatal("nonzero ContentID is unavailable")
	}
	if got, want := id.String(), "12"+strings.Repeat("0", 60)+"ab"; got != want {
		t.Fatalf("ContentID.String = %q, want %q", got, want)
	}
}

func TestContentIDAvailableChecksEveryByte(t *testing.T) {
	var zero ContentID
	if zero.Available() {
		t.Fatal("zero ContentID is available")
	}

	for offset := range zero {
		for _, value := range []byte{0x01, 0x80, 0xff} {
			id := zero
			id[offset] = value
			if !id.Available() {
				t.Fatalf("ContentID with byte %d=%#x is unavailable", offset, value)
			}
		}
	}

	digest := sha256.Sum256([]byte("arbitrary ContentID digest"))
	if !ContentID(digest).Available() {
		t.Fatal("arbitrary nonzero digest is unavailable")
	}
}

func TestDeriveContentIDSeparatesDomainAndPartBoundaries(t *testing.T) {
	first, firstOK := DeriveContentID("owner/value/v1", []byte("ab"), []byte("c"))
	replayed, replayedOK := DeriveContentID("owner/value/v1", []byte("ab"), []byte("c"))
	reframed, reframedOK := DeriveContentID("owner/value/v1", []byte("a"), []byte("bc"))
	otherDomain, otherDomainOK := DeriveContentID("owner/effect/v1", []byte("ab"), []byte("c"))
	if !firstOK || !replayedOK || !reframedOK || !otherDomainOK {
		t.Fatal("valid content identity derivation failed")
	}
	if first != replayed {
		t.Fatal("equivalent content identity derivation changed")
	}
	if first == reframed {
		t.Fatal("payload boundaries were not framed")
	}
	if first == otherDomain {
		t.Fatal("identity domains were not separated")
	}
	if empty, ok := DeriveContentID("", []byte("payload")); ok || empty.Available() {
		t.Fatal("empty identity domain did not fail closed")
	}
}

func TestArtifactScalarRoleIdentityUsesVersionedCanonicalFraming(t *testing.T) {
	artifact := ContentID{0: 0x19, 31: 0xe1}
	key := []byte("rule/slot")
	current, currentOK := DeriveContentID("analysis/artifact-scalar-role/v2", artifact[:], key)
	previous, previousOK := DeriveContentID("analysis/artifact-scalar-role/v1", artifact[:], key)
	if !currentOK || !previousOK {
		t.Fatal("versioned artifact scalar role identity did not derive")
	}
	if current == previous {
		t.Fatal("artifact scalar role identity version did not separate formulas")
	}
	for _, domain := range []string{"analysis/artifact-scalar-role/v1", "analysis/artifact-scalar-role/v2"} {
		raw := append([]byte(domain), artifact[:]...)
		raw = append(raw, key...)
		legacy := ContentID(sha256.Sum256(raw))
		if current == legacy {
			t.Fatalf("artifact scalar role identity still accepts raw concatenation for %q", domain)
		}
	}
	reframed, reframedOK := DeriveContentID("analysis/artifact-scalar-role/v2", artifact[:], append([]byte("rule/"), []byte("slot")...))
	if !reframedOK || current != reframed {
		t.Fatal("equivalent artifact scalar role payload changed identity")
	}
}

var contentIDAvailableSink bool

func BenchmarkContentIDAvailable(b *testing.B) {
	cases := []struct {
		name string
		id   ContentID
	}{
		{name: "zero", id: ContentID{}},
		{name: "first-byte", id: ContentID{0: 1}},
		{name: "last-byte", id: ContentID{31: 1}},
	}
	for _, test := range cases {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			var available bool
			for index := 0; index < b.N; index++ {
				available = test.id.Available()
			}
			contentIDAvailableSink = available
		})
	}
}
