package presence

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
)

var Key = axis.NewKey[Value]("presence")

func Spec() axis.Spec[Value] {
	return axis.Spec[Value]{
		Key:      Key,
		Bottom:   Bottom,
		Top:      Top,
		Equal:    Equal,
		LessOrEq: func(a, b Value) bool { return b.Covers(a) },
		Join:     Join,
		Meet:     Meet,
		Widen:    Widen,
		Hash:     Value.Hash,
	}
}

// Reader exposes core presence to reducers through a presence-owned contract.
type Reader interface {
	Presence() Value
}

// Writer exposes mutable core presence to reducers through a presence-owned contract.
type Writer interface {
	Reader
	SetPresence(Value)
}

// Get reads core presence from a reducer view.
func Get(r axis.Reader) Value {
	pr, ok := r.(Reader)
	if !ok {
		panic("presence: reducer view does not expose core presence; use the product reducer view")
	}
	return pr.Presence()
}

// Set writes core presence to a reducer view.
func Set(w axis.Writer, p Value) {
	pw, ok := w.(Writer)
	if !ok {
		panic("presence: reducer view does not expose core presence; use the product reducer view")
	}
	pw.SetPresence(p)
}

// Value is the Presence/Nilability axis abstraction of whether a slot holds a
// value. It is a point in the four-element lattice bottom < {present, absent} < maybe.
type Value uint8

const (
	// bottom is the unreachable state (no concrete observation).
	bottom Value = iota
	// present means the slot definitely holds a non-nil value.
	present
	// absent means the slot is definitely nil or missing.
	absent
	// maybe means the slot may or may not hold a value (Top).
	maybe
)

// Bottom is the unreachable state: no concrete observation reaches the slot.
func Bottom() Value { return bottom }

// Top is the most general presence state: the slot may or may not hold a value.
func Top() Value { return maybe }

// Present is the state in which the slot definitely holds a non-nil value.
func Present() Value { return present }

// Absent is the state in which the slot is definitely nil or missing.
func Absent() Value { return absent }

// Maybe is the state in which the slot may or may not hold a value (Top).
func Maybe() Value { return maybe }

// IsBottom reports whether the value is unreachable.
func (v Value) IsBottom() bool { return v == bottom }

// IsTop reports whether the value is the most general state.
func (v Value) IsTop() bool { return v == maybe }

// Join is the least upper bound of the four-point lattice.
//
// Joining a state with itself returns it; Bottom is the identity; Present and
// Absent are incomparable so their join is Maybe; Maybe absorbs everything.
func Join(a, b Value) Value {
	if a == b {
		return a
	}
	if a == bottom {
		return b
	}
	if b == bottom {
		return a
	}
	// a != b and neither is bottom: either one is maybe, or they are the
	// incomparable present/absent siblings. Both cases join to maybe.
	return maybe
}

// Meet is the greatest lower bound of the four-point lattice.
func Meet(a, b Value) Value {
	if a == b {
		return a
	}
	if a == maybe {
		return b
	}
	if b == maybe {
		return a
	}
	return bottom
}

// Widen equals Join: the lattice has finite height, so no acceleration is needed.
func Widen(prev, next Value) Value {
	return Join(prev, next)
}

// Equal is lattice equivalence (identity on this finite lattice).
func Equal(a, b Value) bool {
	return a == b
}

// Hash is a stable hash consistent with Equal.
func (v Value) Hash() uint64 {
	return internal.MixHash(internal.FnvString("presence"), uint64(v))
}

// Covers reports whether the receiver is at least as high as other in the lattice.
//
// Defined through Join so it is consistent with the order: v covers other iff
// joining them does not raise v.
func (v Value) Covers(other Value) bool {
	return Join(v, other) == v
}

// String renders the presence state for diagnostics.
func (v Value) String() string {
	switch v {
	case bottom:
		return "bottom"
	case present:
		return "present"
	case absent:
		return "absent"
	case maybe:
		return "maybe"
	default:
		return "presence(invalid)"
	}
}
