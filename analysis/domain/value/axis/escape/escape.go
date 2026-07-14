package escape

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
)

var Key = axis.NewKey[Value]("escape")

func Spec() axis.Spec[Value] {
	return axis.Spec[Value]{
		Key:       Key,
		Bottom:    Bottom,
		Top:       Top,
		Equal:     Equal,
		LessOrEq:  func(a, b Value) bool { return b.Covers(a) },
		Join:      Join,
		Meet:      Meet,
		Widen:     Widen,
		Hash:      Value.Hash,
		Retention: axis.ImmutableRetention[Value](),
		Boundary:  axis.PortableIdentity,
	}
}

// Value is the Escape/Allocation axis abstraction of whether a value escapes its
// allocating frame.
//
// It is a point in the three-element chain bottom < fresh < escaped. Fresh is a
// value still confined to its allocating frame (not yet exposed through an alias,
// call, return, field store, or function definition), so it is stack-allocatable
// and retains its strong-update identity; escaped is a value that has been
// published and may be observed elsewhere, so it is heap-required. The order is
// conservatism: Join takes the higher point, so a value that escapes on any
// incoming path escapes at the merge point. Top is escaped (the conservative assumption)
// and Bottom is unreachable.
type Value uint8

const (
	// bottom is the unreachable escape state (no concrete observation).
	bottom Value = iota
	// fresh is a value still confined to its allocating frame.
	fresh
	// escaped is a value that has been published and may be observed elsewhere. It
	// is Top.
	escaped
)

// Bottom is the unreachable escape state: no concrete observation reaches it.
func Bottom() Value { return bottom }

// Top is the conservative "escapes" state.
func Top() Value { return escaped }

// Fresh is the state of a value still confined to its allocating frame.
func Fresh() Value { return fresh }

// Escaped is the state of a value that has been published (Top).
func Escaped() Value { return escaped }

// IsBottom reports whether the value is unreachable.
func (v Value) IsBottom() bool { return v == bottom }

// IsTop reports whether the value is the conservative escaped state.
func (v Value) IsTop() bool { return v == escaped }

// Join is the least upper bound of the three-point chain.
//
// Bottom is the identity; otherwise Join takes the higher point, so a fresh value
// joined with an escaped one escapes.
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
	return internal.MixHash(internal.FnvString("escape"), uint64(v))
}

// Covers reports whether the receiver is at least as high as other in the chain.
func (v Value) Covers(other Value) bool {
	return Join(v, other) == v
}

// String renders the escape state for diagnostics.
func (v Value) String() string {
	switch v {
	case bottom:
		return "bottom"
	case fresh:
		return "fresh"
	case escaped:
		return "escaped"
	default:
		return "escape(invalid)"
	}
}
