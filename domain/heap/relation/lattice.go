package relation

import (
	heapdomain "github.com/wippyai/go-lua/domain/heap"
)

// HeapLattice is domain/heap's ascent authority. The owner spells its lattice
// as package functions over values that carry their own owner, so the witness
// holds nothing, and it reaches the same generic surface as an owner that
// spells its lattice as methods on a sealed schema.
type HeapLattice struct{}

// NewHeapLattice adopts the heap ascent surface.
func NewHeapLattice() (HeapLattice, bool) { return HeapLattice{}, true }

func (HeapLattice) Join(left, right heapdomain.Value) (heapdomain.Value, bool) {
	return heapdomain.Join(left, right)
}

func (HeapLattice) Widen(previous, next heapdomain.Value) (heapdomain.Value, bool) {
	return heapdomain.Widen(previous, next)
}

func (HeapLattice) LessOrEq(left, right heapdomain.Value) bool {
	return heapdomain.LessOrEq(left, right)
}
