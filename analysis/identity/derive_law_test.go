package identity

import (
	"crypto/sha256"
	"encoding/binary"
	"runtime"
	"testing"
)

// runtimeAllocSample is one reading of the process allocation counters, used
// to state a byte bound that AllocsPerRun's object count cannot express.
type runtimeAllocSample struct {
	bytes uint64
}

func sampleAllocations() runtimeAllocSample {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return runtimeAllocSample{bytes: stats.TotalAlloc}
}

// streamedContentID is the framed digest construction stated as an explicit
// stream: one length-prefixed frame per field, written in order into a
// standalone hash. It is the reference DeriveContentID must equal byte for
// byte, so the derivation can be built without a per-call frame vector.
func streamedContentID(tag string, parts ...[]byte) (ContentID, bool) {
	if tag == "" {
		return ContentID{}, false
	}
	hash := sha256.New()
	write := func(value []byte) {
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(value)))
		hash.Write(size[:])
		hash.Write(value)
	}
	write([]byte(tag))
	for _, part := range parts {
		write(part)
	}
	var id ContentID
	copy(id[:], hash.Sum(nil))
	return id, id.Available()
}

// deriveLawCases are the payload shapes the analysis actually derives over:
// no payload, one identity, several identities, an empty frame between two
// populated ones, and a payload far wider than any inline frame buffer.
func deriveLawCases() [][][]byte {
	wide := make([][]byte, 0, 512)
	for index := 0; index < 512; index++ {
		part := make([]byte, 32)
		part[0], part[31] = byte(index), byte(index>>8)
		wide = append(wide, part)
	}
	long := make([]byte, 9_000)
	for index := range long {
		long[index] = byte(index * 7)
	}
	return [][][]byte{
		nil,
		{{}},
		{[]byte("alpha")},
		{[]byte("alpha"), []byte("beta"), []byte("gamma")},
		{[]byte("alpha"), {}, []byte("gamma")},
		{long},
		wide,
	}
}

// TestDeriveContentIDEqualsTheStreamedFraming pins that the derivation is the
// same framed digest whatever buffer it is assembled in. Every identity in the
// analysis is addressed by this value, so the construction is fixed content
// and not an implementation detail.
func TestDeriveContentIDEqualsTheStreamedFraming(t *testing.T) {
	for index, parts := range deriveLawCases() {
		for _, tag := range []string{"a", "analysis/identity/derive-law/v1", string(make([]byte, 4_096))} {
			expected, expectedOK := streamedContentID(tag, parts...)
			actual, actualOK := DeriveContentID(tag, parts...)
			if expectedOK != actualOK || expected != actual {
				t.Fatalf("case %d tag %d: derived %x/%v streamed %x/%v", index, len(tag), actual, actualOK, expected, expectedOK)
			}
		}
	}
	if _, ok := DeriveContentID(""); ok {
		t.Fatal("an empty domain tag derived an identity")
	}
	if _, ok := DeriveContentID("", []byte("alpha")); ok {
		t.Fatal("an empty domain tag with a payload derived an identity")
	}
}

// TestDeriveContentIDAllocatesNothing pins the cost of the shared framed
// digest. It is the hottest identity operation in a solve; a frame buffer,
// a digest object, or a tag copy allocated per call is paid once per derived
// identity across the whole analysis.
func TestDeriveContentIDAllocatesNothing(t *testing.T) {
	first, second := ContentID{1}, ContentID{2}
	for _, size := range []int{0, 1, 2, 4} {
		parts := make([][]byte, 0, size)
		for index := 0; index < size; index++ {
			if index%2 == 0 {
				parts = append(parts, first[:])
				continue
			}
			parts = append(parts, second[:])
		}
		if allocations := testing.AllocsPerRun(500, func() {
			if _, ok := DeriveContentID("analysis/identity/derive-law/v1", parts...); !ok {
				t.Fatal("derivation")
			}
		}); allocations != 0 {
			t.Fatalf("derivation over %d parts allocated %v objects per call", size, allocations)
		}
	}
	// The shape every owner actually writes: a literal argument list, whose
	// variadic vector must stay on the caller's stack.
	if allocations := testing.AllocsPerRun(500, func() {
		if _, ok := DeriveContentID("analysis/identity/derive-law/v1", first[:], second[:]); !ok {
			t.Fatal("derivation")
		}
	}); allocations != 0 {
		t.Fatalf("derivation over a literal argument list allocated %v objects per call", allocations)
	}
}

// TestDeriveContentIDIsBoundedByTheInlineFrame pins that a derivation never
// costs a buffer the width of its payload. Plane digests run over whole owner
// vectors; a preimage assembled proportionally would make one such derivation
// more expensive than every narrow derivation around it put together.
func TestDeriveContentIDIsBoundedByTheInlineFrame(t *testing.T) {
	narrow := make([][]byte, 0, 8)
	wide := make([][]byte, 0, 4_096)
	for index := 0; index < cap(wide); index++ {
		part := make([]byte, 32)
		part[0], part[31] = byte(index), byte(index>>8)
		if index < cap(narrow) {
			narrow = append(narrow, part)
		}
		wide = append(wide, part)
	}
	const runs = 64
	measure := func(parts [][]byte) uint64 {
		var before, after runtimeAllocSample
		before = sampleAllocations()
		for run := 0; run < runs; run++ {
			if _, ok := DeriveContentID("analysis/identity/derive-law/v1", parts...); !ok {
				t.Fatal("derivation")
			}
		}
		after = sampleAllocations()
		return (after.bytes - before.bytes) / runs
	}
	// Warm the code paths so the first-call cost is not attributed to either.
	measure(narrow)
	measure(wide)

	wideBytes := measure(wide)
	if wideBytes > 4*deriveInlineFrame {
		t.Fatalf("a %d part derivation allocated %d bytes per call, above the inline frame bound", len(wide), wideBytes)
	}
}
