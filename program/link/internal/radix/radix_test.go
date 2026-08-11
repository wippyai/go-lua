package radix

import (
	"math"
	"testing"
)

func TestExhaustiveSmallRelations(t *testing.T) {
	const width = 8
	for mask := 0; mask < 1<<width; mask++ {
		pairs := make([]Pair, 0, width)
		for key := 0; key < width; key++ {
			if mask&(1<<key) != 0 {
				pairs = append(pairs, Pair{Key: uint32(key), Value: uint32(key*17 + 3)})
			}
		}
		store, index := seal(t, pairs)
		for key := 0; key < width+2; key++ {
			got, found := store.Lookup(index, uint32(key))
			wantFound := mask&(1<<key) != 0
			if found != wantFound {
				t.Fatalf("mask=%08b key=%d found=%t want %t", mask, key, found, wantFound)
			}
			if wantFound && got != uint32(key*17+3) {
				t.Fatalf("mask=%08b key=%d value=%d", mask, key, got)
			}
		}
	}
}

func TestUint32BoundaryAndEveryNibbleSplit(t *testing.T) {
	for shift := uint8(0); shift < maxKeyBits; shift += radixBits {
		pairs := []Pair{{Key: 0, Value: 0}, {Key: uint32(1) << shift, Value: uint32(shift) + 1}}
		store, index := seal(t, pairs)
		if got, found := store.Lookup(index, 0); !found || got != 0 {
			t.Fatalf("shift=%d zero=%d/%t", shift, got, found)
		}
		key := uint32(1) << shift
		if got, found := store.Lookup(index, key); !found || got != uint32(shift)+1 {
			t.Fatalf("shift=%d key=%08x value=%d/%t", shift, key, got, found)
		}
	}
	store, index := seal(t, []Pair{
		{Key: 0, Value: 7},
		{Key: 0x01000000, Value: 11},
		{Key: 0x01fffff0, Value: 13},
		{Key: 0x01ffffff, Value: 17},
		{Key: math.MaxUint32, Value: 19},
	})
	for _, pair := range []Pair{
		{Key: 0, Value: 7},
		{Key: 0x01000000, Value: 11},
		{Key: 0x01fffff0, Value: 13},
		{Key: 0x01ffffff, Value: 17},
		{Key: math.MaxUint32, Value: 19},
	} {
		if got, found := store.Lookup(index, pair.Key); !found || got != pair.Value {
			t.Fatalf("boundary key=%08x value=%d/%t", pair.Key, got, found)
		}
	}
	if _, found := store.Lookup(index, 0x02000000); found {
		t.Fatal("absent common-prefix key found")
	}
}

func TestDeepRadixPathIsExact(t *testing.T) {
	pairs := []Pair{{Key: 0, Value: 1}}
	for shift := uint8(0); shift < maxKeyBits; shift += radixBits {
		pairs = append(pairs, Pair{Key: uint32(1) << shift, Value: uint32(shift) + 2})
	}
	store, index := seal(t, pairs)
	if got, found := store.Lookup(index, 0); !found || got != 1 {
		t.Fatalf("deep path root=%d/%t", got, found)
	}
	for _, pair := range pairs[1:] {
		if got, found := store.Lookup(index, pair.Key); !found || got != pair.Value {
			t.Fatalf("deep path key=%08x value=%d/%t", pair.Key, got, found)
		}
	}
	if _, found := store.Lookup(index, 3); found {
		t.Fatal("deep-path near miss found")
	}
}

func TestRejectsUnorderedAndDuplicatePairsWithoutPublication(t *testing.T) {
	var builder Builder
	if _, err := builder.AddSorted([]Pair{{Key: 2}, {Key: 1}}); err != errPairsOrder {
		t.Fatalf("unordered error=%v", err)
	}
	if _, err := builder.AddSorted([]Pair{{Key: 1}, {Key: 1}}); err != errPairsOrder {
		t.Fatalf("duplicate error=%v", err)
	}
	index, err := builder.AddSorted([]Pair{{Key: 1, Value: 4}, {Key: 9, Value: 8}})
	if err != nil {
		t.Fatal(err)
	}
	store, err := builder.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if got, found := store.Lookup(index, 9); !found || got != 8 {
		t.Fatalf("valid table after rejected input=%d/%t", got, found)
	}
	if _, found := store.Lookup(0, 1); found {
		t.Fatal("zero Index selected a relation")
	}
	if _, found := store.Lookup(index+1, 1); found {
		t.Fatal("out-of-range Index selected a relation")
	}
	if _, err := builder.AddSorted(nil); err != errBuilderSealed {
		t.Fatalf("post-seal mutation error=%v", err)
	}
	if _, err := builder.Seal(); err != errBuilderSealed {
		t.Fatalf("second seal error=%v", err)
	}
}

func TestMultipleTablesDoNotAlias(t *testing.T) {
	var builder Builder
	left, err := builder.AddSorted([]Pair{{Key: 1, Value: 10}, {Key: 4, Value: 40}})
	if err != nil {
		t.Fatal(err)
	}
	right, err := builder.AddSorted([]Pair{{Key: 1, Value: 100}, {Key: math.MaxUint32, Value: 200}})
	if err != nil {
		t.Fatal(err)
	}
	store, err := builder.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if got, found := store.Lookup(left, 1); !found || got != 10 {
		t.Fatalf("left value=%d/%t", got, found)
	}
	if got, found := store.Lookup(right, 1); !found || got != 100 {
		t.Fatalf("right value=%d/%t", got, found)
	}
	if _, found := store.Lookup(left, math.MaxUint32); found {
		t.Fatal("right key leaked into left table")
	}
}

func TestCopiedBuilderCannotMutatePublishedStore(t *testing.T) {
	var original Builder
	index, err := original.AddSorted([]Pair{{Key: 1, Value: 10}, {Key: 2, Value: 20}})
	if err != nil {
		t.Fatal(err)
	}
	copyAfterAcquire := original
	store, err := original.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := copyAfterAcquire.AddSorted([]Pair{{Key: 3, Value: 30}}); err != errBuilderSealed {
		t.Fatalf("copied builder mutation error=%v", err)
	}
	if _, err := copyAfterAcquire.Seal(); err != errBuilderSealed {
		t.Fatalf("copied builder seal error=%v", err)
	}
	for _, pair := range []Pair{{Key: 1, Value: 10}, {Key: 2, Value: 20}} {
		if got, found := store.Lookup(index, pair.Key); !found || got != pair.Value {
			t.Fatalf("published Store changed at key=%d: %d/%t", pair.Key, got, found)
		}
	}
	if _, found := store.Lookup(index, 3); found {
		t.Fatal("rejected copied-builder write changed published Store")
	}
}

func TestUnacquiredBuilderCopiesAreIndependent(t *testing.T) {
	var original Builder
	copyBeforeAcquire := original
	index, err := original.AddSorted([]Pair{{Key: 7, Value: 70}})
	if err != nil {
		t.Fatal(err)
	}
	store, err := original.Seal()
	if err != nil {
		t.Fatal(err)
	}
	copyIndex, err := copyBeforeAcquire.AddSorted([]Pair{{Key: 8, Value: 80}})
	if err != nil {
		t.Fatal(err)
	}
	copyStore, err := copyBeforeAcquire.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if got, found := store.Lookup(index, 7); !found || got != 70 {
		t.Fatalf("original Store changed: %d/%t", got, found)
	}
	if _, found := store.Lookup(index, 8); found {
		t.Fatal("unacquired Builder copy changed original Store")
	}
	if got, found := copyStore.Lookup(copyIndex, 8); !found || got != 80 {
		t.Fatalf("independent Store lost its relation: %d/%t", got, found)
	}
}

func TestChildWordCountIsExactAtNativeIntegerBoundary(t *testing.T) {
	for _, children := range []uint64{0, 1, wordBits - 1, wordBits, wordBits + 1, uint64(maxInt)} {
		got, ok := childWordCount(children)
		if !ok {
			t.Fatalf("children=%d rejected", children)
		}
		want := children / wordBits
		if children%wordBits != 0 {
			want++
		}
		if uint64(got) != want {
			t.Fatalf("children=%d words=%d want %d", children, got, want)
		}
	}
	// On 32-bit hosts this also proves that an unrepresentable word count is
	// rejected rather than narrowed. On 64-bit hosts every uint64 input has a
	// word count below maxInt, which is itself the exact proof obligation.
	if uint64(maxInt) <= uint64(^uint32(0)) {
		if _, ok := childWordCount(^uint64(0)); ok {
			t.Fatal("unrepresentable word count accepted")
		}
	}
}

func TestLookupAllocatesNothing(t *testing.T) {
	pairs := make([]Pair, 0, 257)
	for i := 0; i < 257; i++ {
		pairs = append(pairs, Pair{Key: uint32(i * 4099), Value: uint32(i + 1)})
	}
	store, index := seal(t, pairs)
	if allocations := testing.AllocsPerRun(1000, func() {
		_, _ = store.Lookup(index, pairs[len(pairs)-1].Key)
		_, _ = store.Lookup(index, math.MaxUint32)
	}); allocations != 0 {
		t.Fatalf("allocations=%v", allocations)
	}
}

func BenchmarkLookupWideHit(b *testing.B) {
	pairs := benchmarkPairs(4096)
	store, index := sealBenchmark(b, pairs)
	key := pairs[len(pairs)-1].Key
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.Lookup(index, key)
	}
}

func BenchmarkLookupWideMiss(b *testing.B) {
	pairs := benchmarkPairs(4096)
	store, index := sealBenchmark(b, pairs)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.Lookup(index, math.MaxUint32)
	}
}

func seal(t testing.TB, pairs []Pair) (Store, Index) {
	t.Helper()
	var builder Builder
	index, err := builder.AddSorted(pairs)
	if err != nil {
		t.Fatal(err)
	}
	store, err := builder.Seal()
	if err != nil {
		t.Fatal(err)
	}
	return store, index
}

func benchmarkPairs(count int) []Pair {
	pairs := make([]Pair, count)
	for i := range pairs {
		pairs[i] = Pair{Key: uint32(i*4099 + 1), Value: uint32(i + 1)}
	}
	return pairs
}

func sealBenchmark(b *testing.B, pairs []Pair) (Store, Index) {
	b.Helper()
	var builder Builder
	index, err := builder.AddSorted(pairs)
	if err != nil {
		b.Fatal(err)
	}
	store, err := builder.Seal()
	if err != nil {
		b.Fatal(err)
	}
	return store, index
}
