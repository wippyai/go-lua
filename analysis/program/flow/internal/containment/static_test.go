package containment

import (
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/static"
)

func TestStaticCallOwnerDeepChainScales(t *testing.T) {
	const smallWidth, largeWidth = uint32(256), uint32(1024)
	smallLocal, smallCounts := buildStaticCallChain(t, smallWidth)
	largeLocal, largeCounts := buildStaticCallChain(t, largeWidth)

	if got, want := runStaticCallOwnerChain(t, smallLocal, smallCounts), int(smallWidth)+1; got != want {
		t.Fatalf("small chain marks = %d, want %d", got, want)
	}
	if got, want := runStaticCallOwnerChain(t, largeLocal, largeCounts), int(largeWidth)+1; got != want {
		t.Fatalf("large chain marks = %d, want %d", got, want)
	}

	measure := func(local static.LocalContainment, counts [keyspace.FamilyCount]uint32) uint64 {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		const runs = 3
		for index := 0; index < runs; index++ {
			marks := newStaticMarkBits(counts)
			if err := markCallOwnedStaticTypes(local, counts, &marks); err != nil {
				t.Fatalf("scaled mark run: %v", err)
			}
		}
		runtime.ReadMemStats(&after)
		return (after.TotalAlloc - before.TotalAlloc) / runs
	}

	smallBytes := measure(smallLocal, smallCounts)
	largeBytes := measure(largeLocal, largeCounts)
	if smallBytes == 0 || largeBytes > smallBytes*8+128*1024 {
		t.Fatalf("Call-owned chain allocation is superlinear: small=%d large=%d", smallBytes, largeBytes)
	}
}

// TestProveStaticMarksCallTypeSubtreeOnly checks the complete Result
// projection rather than the local owner walk in isolation. A Call-owned
// type argument and its child are static marks; an unrelated static peer and
// the runtime Call identity are not.
