package typ

import (
	"math/rand"
	"testing"
)

func TestTypePairSetMatchesMapOracleAcrossGrowth(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	var set typePairSet
	want := make(map[typePair]struct{})

	for i := 0; i < 5000; i++ {
		pair := typePair{
			a: uintptr(rng.Intn(713) + 1),
			b: uintptr(rng.Intn(719) + 1),
		}
		_, already := want[pair]
		if got := set.seenOrAdd(pair); got != already {
			t.Fatalf("step %d pair %+v: seenOrAdd=%v, map=%v", i, pair, got, already)
		}
		want[pair] = struct{}{}
	}
	set.release()
}

func TestTypePairSetExactUnderHashCollisions(t *testing.T) {
	const (
		tableSize = 64
		cluster   = 48
	)

	var colliding []typePair
	for a := uintptr(1); len(colliding) < cluster; a++ {
		pair := typePair{a: a, b: a*17 + 3}
		if hashTypePair(pair)&(tableSize-1) == 0 {
			colliding = append(colliding, pair)
		}
	}

	var set typePairSet
	for i, pair := range colliding {
		if set.seenOrAdd(pair) {
			t.Fatalf("distinct colliding pair %d was reported seen", i)
		}
	}
	for i := len(colliding) - 1; i >= 0; i-- {
		if !set.seenOrAdd(colliding[i]) {
			t.Fatalf("colliding pair %d was lost", i)
		}
	}
	if got := len(set.table.slots); got != tableSize {
		t.Fatalf("duplicate lookups grew table to %d slots, want %d", got, tableSize)
	}
	set.release()
}

func TestTypePairSetKeepsPairOrientation(t *testing.T) {
	var set typePairSet
	for i := 0; i < typePairInlineCapacity+1; i++ {
		set.seenOrAdd(typePair{a: uintptr(i + 1), b: uintptr(i + 1001)})
	}
	forward := typePair{a: 0x100, b: 0x200}
	reverse := typePair{a: forward.b, b: forward.a}

	if set.seenOrAdd(forward) {
		t.Fatal("first forward pair was reported seen")
	}
	if set.seenOrAdd(reverse) {
		t.Fatal("reverse pair must remain distinct")
	}
	if !set.seenOrAdd(forward) || !set.seenOrAdd(reverse) {
		t.Fatal("both pair orientations must remain present")
	}
	set.release()
}

func TestTypePairSetReleaseClearsPooledStateAndAllowsReuse(t *testing.T) {
	var set typePairSet
	old := make([]typePair, 96)
	for i := range old {
		old[i] = typePair{a: uintptr(i + 1), b: uintptr(i + 1001)}
		if set.seenOrAdd(old[i]) {
			t.Fatalf("old pair %d was reported seen", i)
		}
	}
	if set.table == nil {
		t.Fatal("test must exercise the pooled table path")
	}
	set.release()
	if set.table != nil || set.count != 0 || set.inlineN != 0 {
		t.Fatalf("release did not reset set: table=%v count=%d inline=%d", set.table, set.count, set.inlineN)
	}
	for i, pair := range old {
		if set.seenOrAdd(pair) {
			t.Fatalf("released pair %d leaked into reused set", i)
		}
	}
	set.release()
}

func TestTypePairSetDoesNotPoolOversizedTables(t *testing.T) {
	if got := typePairPoolIndex(256); got < 0 {
		t.Fatal("256-slot table should be pooled")
	}
	if got := typePairPoolIndex(512); got >= 0 {
		t.Fatalf("512-slot table has pool index %d, want discarded", got)
	}

	var set typePairSet
	for i := 0; i < 400; i++ {
		pair := typePair{a: uintptr(i + 1), b: uintptr(i + 10001)}
		if set.seenOrAdd(pair) {
			t.Fatalf("pair %d was reported seen", i)
		}
	}
	if set.table == nil || len(set.table.slots) <= 256 {
		t.Fatalf("test must reach an oversized table, got %v", set.table)
	}
	set.release()
}

func BenchmarkTypePairSetOverflow(b *testing.B) {
	pairs := make([]typePair, 192)
	for i := range pairs {
		pairs[i] = typePair{a: uintptr(i + 1), b: uintptr(i + 1001)}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var set typePairSet
		for _, pair := range pairs {
			set.seenOrAdd(pair)
		}
		set.release()
	}
}
