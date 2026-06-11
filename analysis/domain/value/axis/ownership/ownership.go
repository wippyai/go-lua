package ownership

import internal "github.com/wippyai/go-lua/analysis/internal/hash"

// Value is the Ownership/Linearity axis abstraction of how a value is owned.
//
// It is a point in the three-element chain bottom < unique < shared. Unique is a
// uniquely-owned value on which a strong update is sound; shared is a value
// reachable through more than one reference (aliased, captured, escaped) on which
// only a weak update is sound. The order is precision: the higher a value sits,
// the less the analysis may assume about exclusive ownership, so Join conservatively
// weakens toward shared. Top is shared (the conservative assumption) and Bottom is
// unreachable.
type Value uint8

const (
	// bottom is the unreachable ownership state (no concrete observation).
	bottom Value = iota
	// unique is a uniquely-owned value: a strong update is sound.
	unique
	// shared is a value reachable through more than one reference: only a weak
	// update is sound. It is Top.
	shared
)

// Bottom is the unreachable ownership state: no concrete observation reaches it.
func Bottom() Value { return bottom }

// Top is the fully-shared ownership state: the conservative assumption.
func Top() Value { return shared }

// Unique is the uniquely-owned state on which a strong update is sound.
func Unique() Value { return unique }

// Shared is the multiply-referenced state on which only a weak update is sound
// (Top).
func Shared() Value { return shared }

// IsBottom reports whether the value is unreachable.
func (v Value) IsBottom() bool { return v == bottom }

// IsTop reports whether the value is the fully-shared state.
func (v Value) IsTop() bool { return v == shared }

// Join is the least upper bound of the three-point chain.
//
// Bottom is the identity; otherwise Join takes the higher (less-precise) point,
// so joining unique with shared weakens to shared.
func Join(a, b Value) Value {
	if a > b {
		return a
	}
	return b
}

// Meet is the greatest lower bound of the three-point chain.
func Meet(a, b Value) Value {
	if a < b {
		return a
	}
	return b
}

// Widen equals Join: the chain has finite height, so no acceleration is needed.
func Widen(prev, next Value) Value {
	return Join(prev, next)
}

// Equal is lattice equivalence (identity on this finite chain).
func Equal(a, b Value) bool {
	return a == b
}

// Hash is a stable hash consistent with Equal.
func (v Value) Hash() uint64 {
	return internal.MixHash(internal.FnvString("ownership"), uint64(v))
}

// Covers reports whether the receiver is at least as high as other in the chain.
func (v Value) Covers(other Value) bool {
	return Join(v, other) == v
}

// String renders the ownership state for diagnostics.
func (v Value) String() string {
	switch v {
	case bottom:
		return "bottom"
	case unique:
		return "unique"
	case shared:
		return "shared"
	default:
		return "ownership(invalid)"
	}
}
