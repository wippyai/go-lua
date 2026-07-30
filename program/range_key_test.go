package program

import (
	"math"
	"strconv"
	"testing"
)

func TestBoundedRangePreservesLargestRepresentableExclusiveEnd(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("requires an int large enough to model persisted uint32 boundaries")
	}
	max := int(uint64(math.MaxUint32))

	start, end, ok := boundedRange(max-1, 1)
	if !ok || start != math.MaxUint32-1 || end != math.MaxUint32 {
		t.Fatalf("boundedRange(max-1, 1) = (%d, %d, %v), want (%d, %d, true)", start, end, ok, uint32(math.MaxUint32-1), uint32(math.MaxUint32))
	}
	if _, _, ok := boundedRange(max, 1); ok {
		t.Fatal("boundedRange accepted a range whose persisted end would overflow")
	}
	if start, end, ok := boundedRange(max, 0); !ok || start != math.MaxUint32 || end != math.MaxUint32 {
		t.Fatalf("boundedRange(max, 0) = (%d, %d, %v), want (%d, %d, true)", start, end, ok, uint32(math.MaxUint32), uint32(math.MaxUint32))
	}
}

func TestExactKeyIndexReservesOnlyZero(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("requires an int large enough to model uint32 key capacity")
	}
	max := int(uint64(math.MaxUint32))

	if key, ok := exactKeyIndex(0); !ok || key != 1 {
		t.Fatalf("exactKeyIndex(0) = (%d, %v), want (1, true)", key, ok)
	}
	if key, ok := exactKeyIndex(max - 1); !ok || key != Key(math.MaxUint32) {
		t.Fatalf("exactKeyIndex(max-1) = (%d, %v), want (%d, true)", key, ok, uint32(math.MaxUint32))
	}
	if key, ok := exactKeyIndex(max); ok || key != 0 {
		t.Fatalf("exactKeyIndex(max) = (%d, %v), want (0, false)", key, ok)
	}
}
