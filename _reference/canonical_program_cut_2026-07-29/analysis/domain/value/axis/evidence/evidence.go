package evidence

import (
	"strconv"
	"strings"

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

// Value is the SemanticEvidence axis abstraction of the path-sensitive proofs
// attached to a value.
//
// The first non-trivial proofs carried here distinguish gradual top values
// introduced by unannotated Lua from explicit top values introduced by `any` or
// `unknown` annotations. Keeping those proofs in the product carrier makes them
// part of Equal and Hash, so query/change detection observes the semantic
// distinction instead of recovering it from driver-side maps.
type Value struct {
	kind    kind
	origins originSet
}

type kind uint8

const (
	bottom kind = iota
	gradualTop
	explicitTop
	top
)

const maxOrigins = 4

// OriginKind classifies where a proof entered the abstract value domain. It is
// deliberately syntax-free: lowering/query layers own the mapping from these
// stable ids back to source spans.
type OriginKind uint8

const (
	OriginUnknown OriginKind = iota
	OriginSource
	OriginBranch
	OriginCall
	OriginAnnotation
)

// Origin identifies one proof source without importing syntax, diagnostics, or
// CFG internals into the value domain.
type Origin struct {
	Kind OriginKind
	ID   uint64
}

type originSet struct {
	items     [maxOrigins]Origin
	count     uint8
	truncated bool
}

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

// WithOrigin returns v annotated with an explanatory origin. Origins do not
// change the proof kind; they only make later judgments explain where the proof
// entered the solved state.
func (v Value) WithOrigin(origin Origin) Value {
	if v.kind == bottom || v.kind == top || origin.Kind == OriginUnknown {
		return v
	}
	v.origins = v.origins.add(origin)
	return v
}

// Origins returns a deterministic defensive copy of the bounded origin set.
func (v Value) Origins() []Origin {
	return v.origins.slice()
}

// OriginsTruncated reports whether additional origins were dropped to keep this
// axis finite under joins and widening.
func (v Value) OriginsTruncated() bool {
	return v.origins.truncated
}

// Join keeps only evidence proven on all incoming paths.
func Join(a, b Value) Value {
	if a.kind == b.kind {
		if a.kind == top || a.kind == bottom {
			return Value{kind: a.kind}
		}
		return Value{kind: a.kind, origins: a.origins.union(b.origins)}
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
		if a.kind == top || a.kind == bottom {
			return Value{kind: a.kind}
		}
		return Value{kind: a.kind, origins: a.origins.intersection(b.origins)}
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
	h := internal.MixHash(internal.FnvString("evidence"), uint64(v.kind))
	h = internal.MixHash(h, uint64(v.origins.count))
	if v.origins.truncated {
		h = internal.MixHash(h, 1)
	} else {
		h = internal.MixHash(h, 0)
	}
	for _, origin := range v.origins.slice() {
		h = internal.MixHash(h, uint64(origin.Kind))
		h = internal.MixHash(h, origin.ID)
	}
	return h
}

// Covers reports whether the receiver is at least as high as other in the lattice.
func (v Value) Covers(other Value) bool {
	return Join(v, other) == v
}

// String renders the evidence state for diagnostics and law-test failures.
func (v Value) String() string {
	var base string
	switch v.kind {
	case bottom:
		base = "bottom"
	case gradualTop:
		base = "gradual-top"
	case explicitTop:
		base = "explicit-top"
	case top:
		base = "top"
	default:
		return "evidence(invalid)"
	}
	origins := v.origins.slice()
	if len(origins) == 0 && !v.origins.truncated {
		return base
	}
	var b strings.Builder
	b.WriteString(base)
	b.WriteByte('[')
	for i, origin := range origins {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(origin.String())
	}
	if v.origins.truncated {
		if len(origins) > 0 {
			b.WriteByte(',')
		}
		b.WriteString("+")
	}
	b.WriteByte(']')
	return b.String()
}

func (o Origin) String() string {
	var kindName string
	switch o.Kind {
	case OriginSource:
		kindName = "source"
	case OriginBranch:
		kindName = "branch"
	case OriginCall:
		kindName = "call"
	case OriginAnnotation:
		kindName = "annotation"
	default:
		kindName = "unknown"
	}
	return kindName + ":" + strconv.FormatUint(o.ID, 10)
}

func (s originSet) slice() []Origin {
	if s.count == 0 {
		return nil
	}
	out := make([]Origin, int(s.count))
	copy(out, s.items[:s.count])
	return out
}

func (s originSet) add(origin Origin) originSet {
	if origin.Kind == OriginUnknown {
		return s
	}
	for i := 0; i < int(s.count); i++ {
		if s.items[i] == origin {
			return s
		}
	}
	if int(s.count) < maxOrigins {
		s.items[s.count] = origin
		s.count++
		sortOrigins(s.items[:s.count])
		return s
	}
	s.truncated = true
	if originLess(origin, s.items[maxOrigins-1]) {
		s.items[maxOrigins-1] = origin
		sortOrigins(s.items[:])
	}
	return s
}

func (s originSet) union(other originSet) originSet {
	out := originSet{truncated: s.truncated || other.truncated}
	for _, origin := range s.slice() {
		out = out.add(origin)
	}
	for _, origin := range other.slice() {
		out = out.add(origin)
	}
	return out
}

func (s originSet) intersection(other originSet) originSet {
	var out originSet
	for _, origin := range s.slice() {
		if other.contains(origin) {
			out = out.add(origin)
		}
	}
	return out
}

func (s originSet) contains(origin Origin) bool {
	for i := 0; i < int(s.count); i++ {
		if s.items[i] == origin {
			return true
		}
	}
	return false
}

func sortOrigins(origins []Origin) {
	for i := 1; i < len(origins); i++ {
		for j := i; j > 0 && originLess(origins[j], origins[j-1]); j-- {
			origins[j], origins[j-1] = origins[j-1], origins[j]
		}
	}
}

func originLess(a, b Origin) bool {
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	return a.ID < b.ID
}
