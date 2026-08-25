package relation

import (
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// ValueLattice is domain/value's ascent authority. The owner spells its
// lattice as methods on a sealed schema, so the witness carries that schema
// and nothing else.
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

func (witness ValueLattice) Join(left, right valuedomain.Value) (valuedomain.Value, bool) {
	return witness.schema.Join(left, right)
}

func (witness ValueLattice) Widen(previous, next valuedomain.Value) (valuedomain.Value, bool) {
	return witness.schema.Widen(previous, next)
}

func (witness ValueLattice) LessOrEq(left, right valuedomain.Value) bool {
	return witness.schema.LessOrEq(left, right)
}
