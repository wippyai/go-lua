package evidence

import "github.com/wippyai/go-lua/internal"

// Value is the SemanticEvidence axis abstraction of the path-sensitive proofs
// attached to a value.
//
// The axis currently carries a single element: the no-evidence (Top) state. Its
// lattice is therefore the trivial one-point lattice, which is sound (every
// operation is total and the laws hold degenerately). The evidence carriers
// (discriminant, correlation, predicate) and their reducers land in Phase 5; they
// extend this carrier without changing the axis surface.
type Value struct {
	_ struct{}
}

// Bottom is the unreachable evidence state. On the one-point lattice it coincides
// with Top.
func Bottom() Value {
	return Value{}
}

// Top carries no evidence.
func Top() Value {
	return Value{}
}

// Join keeps only evidence proven on all incoming paths. On the one-point lattice
// it returns the sole element.
func Join(a, b Value) Value {
	return Value{}
}

// Widen accelerates an ascending chain. The one-point lattice has height zero, so
// Widen equals Join.
func Widen(prev, next Value) Value {
	return Join(prev, next)
}

// Equal is lattice equivalence.
func Equal(a, b Value) bool {
	return a == b
}

// Hash is a stable hash consistent with Equal.
func (v Value) Hash() uint64 {
	return internal.FnvString("evidence")
}

// Covers reports whether the receiver carries at least as much evidence as other.
func (v Value) Covers(other Value) bool {
	return true
}
