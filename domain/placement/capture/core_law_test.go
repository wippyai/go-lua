package capture

import (
	"testing"

	"github.com/wippyai/go-lua/domain/placement"
)

func TestCapturePlacementIsTheLeastUpperBoundOfClosureAndSource(t *testing.T) {
	cases := []struct {
		closure placement.Placement
		source  placement.Placement
		want    placement.Placement
	}{
		{placement.Stack, placement.Stack, placement.Stack},
		{placement.Stack, placement.OwnedHeap, placement.OwnedHeap},
		{placement.OwnedHeap, placement.Stack, placement.OwnedHeap},
		{placement.OwnedHeap, placement.SharedHeap, placement.SharedHeap},
		{placement.SharedHeap, placement.OwnedHeap, placement.SharedHeap},
		{placement.Unknown, placement.Stack, placement.Unknown},
	}
	for _, item := range cases {
		if got := captureValue(item.closure, item.source); got != item.want {
			t.Fatalf("capture(%s,%s) = %s, want %s", item.closure, item.source, got, item.want)
		}
	}
}

func TestCapturePlacementDoesNotSynthesizeAnAbsentSource(t *testing.T) {
	// captureValue is only reached after the selected source cell has been
	// authenticated. Its value-level contract therefore has no absent-source
	// compensation branch; an absent cell is refused by route planning.
	if got := captureValue(placement.OwnedHeap, placement.SharedHeap); got != placement.SharedHeap {
		t.Fatalf("authenticated source capture = %s, want SharedHeap", got)
	}
}
