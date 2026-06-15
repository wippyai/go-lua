package placement

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
)

var Key = axis.NewKey[Value]("placement")

func Spec() axis.Spec[Value] {
	return axis.Spec[Value]{
		Key:      Key,
		Bottom:   func() Value { return Bottom },
		Top:      func() Value { return Unknown },
		Equal:    Equal,
		LessOrEq: LessOrEq,
		Join:     Join,
		Meet:     Meet,
		Widen:    Widen,
		Hash:     Value.Hash,
	}
}

// Value is the allocation-placement axis abstraction.
//
// The chain is:
//
//	bottom < stack < owned-heap < shared-heap < unknown
//
// Higher points are less precise and more conservative. Joining any path that
// requires a more shared placement moves the result upward; Unknown is Top.
type Value uint8

const (
	// Bottom is the unreachable placement state.
	Bottom Value = iota
	// Stack is stack/local placement confined to the current activation.
	Stack
	// OwnedHeap is heap placement owned by one actor or analysis owner.
	OwnedHeap
	// SharedHeap is heap placement that may be shared, escaped, or observed
	// outside the owning actor.
	SharedHeap
	// Unknown is the conservative top placement.
	Unknown
)

const (
	// EscapedHeap is a domain-name alias for SharedHeap at escape boundaries.
	EscapedHeap = SharedHeap
)

// IsBottom reports whether the value is unreachable.
func (v Value) IsBottom() bool { return v == Bottom }

// IsTop reports whether the value is the conservative unknown placement.
func (v Value) IsTop() bool { return v == Unknown }

// LessOrEq reports whether b conservatively covers a.
func LessOrEq(a, b Value) bool {
	return a <= b
}

// Join is the least upper bound of the placement chain.
func Join(a, b Value) Value {
	if a > b {
		return a
	}
	return b
}

// Meet is the greatest lower bound of the placement chain.
func Meet(a, b Value) Value {
	if a < b {
		return a
	}
	return b
}

// Widen equals Join because the placement lattice has finite height.
func Widen(prev, next Value) Value {
	return Join(prev, next)
}

// Equal is lattice equivalence.
func Equal(a, b Value) bool {
	return a == b
}

// Hash is stable and consistent with Equal.
func (v Value) Hash() uint64 {
	return internal.MixHash(internal.FnvString("placement"), uint64(v))
}

// Covers reports whether the receiver is at least as high as other in the chain.
func (v Value) Covers(other Value) bool {
	return Join(v, other) == v
}

func (v Value) String() string {
	switch v {
	case Bottom:
		return "bottom"
	case Stack:
		return "stack"
	case OwnedHeap:
		return "owned-heap"
	case SharedHeap:
		return "shared-heap"
	case Unknown:
		return "unknown"
	default:
		return "placement(invalid)"
	}
}
