package specimen

import (
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// ValueLattice is domain/value's ascent authority stated once for the generic
// surface. The owner spells its lattice as methods on a sealed *Schema, so the
// witness carries that schema and nothing else.
type ValueLattice struct {
	schema *valuedomain.Schema
}

// NewValueLattice adopts one sealed correlated-value schema.
func NewValueLattice(schema *valuedomain.Schema) (ValueLattice, bool) {
	if schema == nil || !schema.Valid() {
		return ValueLattice{}, false
	}
	return ValueLattice{schema: schema}, true
}

func (lattice ValueLattice) Join(left, right valuedomain.Value) (valuedomain.Value, bool) {
	return lattice.schema.Join(left, right)
}

func (lattice ValueLattice) Widen(previous, next valuedomain.Value) (valuedomain.Value, bool) {
	return lattice.schema.Widen(previous, next)
}

func (lattice ValueLattice) LessOrEq(left, right valuedomain.Value) bool {
	return lattice.schema.LessOrEq(left, right)
}

// HeapLattice is domain/heap's ascent authority. The owner spells its lattice
// as package functions over values that carry their own owner, so the witness
// is empty. Two unrelated owner APIs reach one generic surface without either
// owner changing and without the payload ever being boxed.
type HeapLattice struct{}

func (HeapLattice) Join(left, right heapdomain.Value) (heapdomain.Value, bool) {
	return heapdomain.Join(left, right)
}

func (HeapLattice) Widen(previous, next heapdomain.Value) (heapdomain.Value, bool) {
	return heapdomain.Widen(previous, next)
}

func (HeapLattice) LessOrEq(left, right heapdomain.Value) bool {
	return heapdomain.LessOrEq(left, right)
}
