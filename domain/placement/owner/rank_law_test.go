package owner

import (
	"testing"

	"github.com/wippyai/go-lua/domain/placement"
)

func TestPlacementWidenRankDescendsOnEveryStrictAscent(t *testing.T) {
	values := []placement.Placement{
		placement.Bottom,
		placement.Stack,
		placement.OwnedHeap,
		placement.SharedHeap,
		placement.Unknown,
	}
	want := []uint64{4, 3, 2, 1, 0}
	lattice := placement.Lattice()

	for index, value := range values {
		rank, ok := placementRank(value)
		if !ok || uint64(rank) != want[index] {
			t.Fatalf("placement rank(%v) = %d/%v, want %d/true", value, rank, ok, want[index])
		}
	}
	for _, before := range values {
		for _, after := range values {
			if !lattice.LessOrEq(before, after) || lattice.Equal(before, after) {
				continue
			}
			beforeRank, beforeOK := placementRank(before)
			afterRank, afterOK := placementRank(after)
			if !beforeOK || !afterOK || afterRank >= beforeRank {
				t.Fatalf("strict Placement ascent %v -> %v did not descend: %d -> %d", before, after, beforeRank, afterRank)
			}
		}
	}
}
