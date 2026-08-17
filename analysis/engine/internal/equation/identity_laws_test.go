package equation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/internal/canonical"
)

var smallIdentityPayload = []byte("small")
var smallIdentityKey = composition.Key{ID: composition.ID{1}, Version: identityVersion}

func TestIdentityWriterMatchesCanonicalWire(t *testing.T) {
	const domain = "analysis/engine/equation/identity-wire-law"
	payload := append([]byte{0, 1, 2, 0xff}, bytes.Repeat([]byte{0xa5}, 64<<10)...)
	gotKey, ok := identityKey(domain, func(writer *canonical.DigestWriter) bool {
		return writer.Bytes(payload) == nil && writer.Count(300) == nil && writer.Uint(99) == nil
	})
	if !ok {
		t.Fatal("digest writer rejected valid fixture")
	}

	hash := sha256.New()
	var writer canonical.Writer
	if err := writer.Reset(context.Background(), hash, domain, identityVersion); err != nil {
		t.Fatal(err)
	}
	if writer.Bytes(payload) != nil || writer.Count(300) != nil || writer.Uint(99) != nil {
		t.Fatal("canonical writer rejected valid fixture")
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	var wantID [sha256.Size]byte
	hash.Sum(wantID[:0])
	if !bytes.Equal(gotKey.ID[:], wantID[:]) {
		t.Fatalf("identity digest differs from canonical wire: got %x want %x", gotKey.ID, wantID)
	}
}

func TestIdentityKeyReplayAndOrderLaw(t *testing.T) {
	encode := func(writer *canonical.DigestWriter) bool {
		return writer.Count(2) == nil && writer.Uint(7) == nil && writer.Bytes([]byte("payload")) == nil
	}
	first, firstOK := identityKey("analysis/engine/equation/replay", encode)
	second, secondOK := identityKey("analysis/engine/equation/replay", encode)
	if !firstOK || !secondOK || first != second {
		t.Fatalf("replaying one canonical preimage changed identity: %#v/%#v", first, second)
	}
	changed, changedOK := identityKey("analysis/engine/equation/replay", func(writer *canonical.DigestWriter) bool {
		return writer.Uint(7) == nil && writer.Count(2) == nil && writer.Bytes([]byte("payload")) == nil
	})
	if !changedOK || changed == first {
		t.Fatal("canonical event order did not participate in identity")
	}
}

func TestIdentityKeySmallPreimageAllocationLaw(t *testing.T) {
	allocs := testing.AllocsPerRun(1000, func() {
		key, ok := identityKey("analysis/engine/equation/allocation", func(writer *canonical.DigestWriter) bool {
			return writeKey(writer, smallIdentityKey) && writer.Bytes(smallIdentityPayload) == nil && writer.Count(3) == nil && writer.Uint(11) == nil
		})
		if !ok || !key.Available() {
			t.Fatal("identity construction failed")
		}
	})
	if allocs > 2 {
		t.Fatalf("small identity construction allocates too much: %.2f allocs/op", allocs)
	}
	t.Logf("small identity allocations: %.2f allocs/op", allocs)
}

func BenchmarkIdentityKeyAllocObjects(b *testing.B) {
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		key, ok := identityKey("analysis/engine/equation/allocation-benchmark", func(writer *canonical.DigestWriter) bool {
			return writeKey(writer, smallIdentityKey) && writer.Bytes(smallIdentityPayload) == nil && writer.Count(3) == nil && writer.Uint(11) == nil
		})
		if !ok || !key.Available() {
			b.Fatal("identity construction failed")
		}
	}
}
