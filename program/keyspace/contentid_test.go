package keyspace

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
