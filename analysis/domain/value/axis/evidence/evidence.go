package evidence

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
)

var Key = axis.NewKey[Value]("evidence")

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
		Canonical: canonicalDescriptor(),
		Boundary:  axis.Projected,
		BoundaryProject: func(Value) Value {
			return Top()
		},
	}
}

// Value is the semantic evidence attached to a value.
//
// It is deliberately just the four-state semantic lattice. Constructive proof
// provenance is not a component of a semantic value and belongs in a separate
// proof sidecar.
type Value struct {
	kind kind
}

type kind uint8

const (
	bottom kind = iota
	gradualTop
	explicitTop
	top
)

// Bottom is the unreachable evidence state.
func Bottom() Value {
	return Value{kind: bottom}
}

// Top carries no evidence.
func Top() Value {
	return Value{kind: top}
}

// GradualTop proves that a dynamic `any` came from an unannotated source and is
// therefore admissible at gradual-consistency boundaries.
func GradualTop() Value {
	return Value{kind: gradualTop}
}

// ExplicitTop proves that a dynamic top came from an explicit `any` or
// `unknown` annotation, so it is not admissible as structural validation.
func ExplicitTop() Value {
	return Value{kind: explicitTop}
}

// IsGradualTop reports whether this evidence proves the gradual top.
func (v Value) IsGradualTop() bool {
	return v.kind == gradualTop
}

// IsExplicitTop reports whether this evidence proves explicit top.
func (v Value) IsExplicitTop() bool {
	return v.kind == explicitTop
}

// Join keeps only evidence proven on all incoming paths.
func Join(a, b Value) Value {
	if a.kind == b.kind {
		return a
	}
	if a.kind == bottom {
		return b
	}
	if b.kind == bottom {
		return a
	}
	return Top()
}

// Meet is the greatest lower bound. Gradual and explicit top evidence are
// sibling proofs, so meeting them yields bottom rather than either proof.
func Meet(a, b Value) Value {
	if a.kind == b.kind {
		return a
	}
	if a.kind == top {
		return b
	}
	if b.kind == top {
		return a
	}
	if a.kind == bottom || b.kind == bottom {
		return Bottom()
	}
	return Bottom()
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
	return internal.MixHash(internal.FnvString("evidence"), uint64(v.kind))
}

// Covers reports whether the receiver is at least as high as other in the lattice.
func (v Value) Covers(other Value) bool {
	return Join(v, other) == v
}

// String renders the evidence state for diagnostics and law-test failures.
func (v Value) String() string {
	switch v.kind {
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
