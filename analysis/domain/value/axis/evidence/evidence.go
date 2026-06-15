package evidence

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
)

var Key = axis.NewKey[Value]("evidence")

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

// Value is the SemanticEvidence axis abstraction of the path-sensitive proofs
// attached to a value.
//
// The first non-trivial proofs carried here distinguish gradual top values
// introduced by unannotated Lua from explicit top values introduced by `any` or
// `unknown` annotations. Keeping those proofs in the product carrier makes them
// part of Equal and Hash, so query/change detection observes the semantic
// distinction instead of recovering it from driver-side maps.
type Value uint8

const (
	bottom Value = iota
	gradualTop
	explicitTop
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

// ExplicitTop proves that a dynamic top came from an explicit `any` or
// `unknown` annotation, so it is not admissible as structural proof.
func ExplicitTop() Value {
	return explicitTop
}

// IsGradualTop reports whether this evidence proves the gradual top.
func (v Value) IsGradualTop() bool {
	return v == gradualTop
}

// IsExplicitTop reports whether this evidence proves explicit top.
func (v Value) IsExplicitTop() bool {
	return v == explicitTop
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

// Meet is the greatest lower bound. Gradual and explicit top evidence are
// sibling proofs, so meeting them yields bottom rather than either proof.
func Meet(a, b Value) Value {
	if a == b {
		return a
	}
	if a == top {
		return b
	}
	if b == top {
		return a
	}
	if a == bottom || b == bottom {
		return bottom
	}
	return bottom
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
	case explicitTop:
		return "explicit-top"
	case top:
		return "top"
	default:
		return "evidence(invalid)"
	}
}
