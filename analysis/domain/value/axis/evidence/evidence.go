package evidence

import internal "github.com/wippyai/go-lua/analysis/internal/hash"

// Value is the SemanticEvidence axis abstraction of the path-sensitive proofs
// attached to a value.
//
// The first non-trivial proof carried here is GradualTop: the value is the
// dynamic top introduced by an unannotated source, not a strict declared `any`.
// Keeping that proof in the product carrier makes it part of Equal and Hash, so
// query/change detection observes the semantic distinction instead of recovering
// it from driver-side maps.
type Value uint8

const (
	bottom Value = iota
	gradualTop
	top
)

// Bottom is the unreachable evidence state.
func Bottom() Value {
	return bottom
}

// Top carries no evidence.
func Top() Value {
	return top
}

// GradualTop proves that a dynamic `any` came from an unannotated source and is
// therefore admissible at gradual-consistency boundaries.
func GradualTop() Value {
	return gradualTop
}

// IsGradualTop reports whether this evidence proves the gradual top.
func (v Value) IsGradualTop() bool {
	return v == gradualTop
}

// Join keeps only evidence proven on all incoming paths.
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
	return top
}

// Meet is the greatest lower bound of the evidence chain.
func Meet(a, b Value) Value {
	if a < b {
		return a
	}
	return b
}

// Widen accelerates an ascending chain. The evidence lattice is finite, so Widen
// equals Join.
func Widen(prev, next Value) Value {
	return Join(prev, next)
}

// Equal is lattice equivalence.
func Equal(a, b Value) bool {
	return a == b
}

// Hash is a stable hash consistent with Equal.
func (v Value) Hash() uint64 {
	return internal.MixHash(internal.FnvString("evidence"), uint64(v))
}

// Covers reports whether the receiver is at least as high as other in the lattice.
func (v Value) Covers(other Value) bool {
	return Join(v, other) == v
}

// String renders the evidence state for diagnostics and law-test failures.
func (v Value) String() string {
	switch v {
	case bottom:
		return "bottom"
	case gradualTop:
		return "gradual-top"
	case top:
		return "top"
	default:
		return "evidence(invalid)"
	}
}
