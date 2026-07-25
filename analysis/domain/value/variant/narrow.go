package variant

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/internal/typegraph"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// NarrowByPathLiteral keeps the variants of t whose static member path admits
// lit. The returned bool reports whether a strict narrowing was possible.
func NarrowByPathLiteral(t typ.Type, suffix []segment.Segment, lit typ.Type) (typ.Type, bool) {
	return narrowByPathLiteralEntry(t, suffix, lit, narrowByPathLiteralSeen)
}

// NarrowByPathLiteralNot keeps the variants of t whose static member path does
// not admit lit. The returned bool reports whether a strict narrowing was
// possible.
func NarrowByPathLiteralNot(t typ.Type, suffix []segment.Segment, lit typ.Type) (typ.Type, bool) {
	return narrowByPathLiteralEntry(t, suffix, lit, narrowByPathLiteralNotSeen)
}

// NarrowByPathType keeps the variants of t whose static member path can still
// equal a value of peer. It is the type-level companion of NarrowByPathLiteral:
// an equality whose other side is a typed path or local selects the arms whose
// member overlaps peer and refutes the arms proven disjoint from it. The
// returned bool reports whether a strict narrowing was possible.
func NarrowByPathType(t typ.Type, suffix []segment.Segment, peer typ.Type) (typ.Type, bool) {
	if t == nil || len(suffix) == 0 || peer == nil {
		return nil, false
	}
	narrowed, ok := narrowByPathTypeSeen(t, suffix, peer, &typegraph.Path{})
	if !ok || narrowed == nil || typ.SameNodeOrAcyclicEqual(narrowed, t) {
		return narrowed, false
	}
	return narrowed, true
}

func narrowByPathTypeSeen(t typ.Type, suffix []segment.Segment, peer typ.Type, active *typegraph.Path) (typ.Type, bool) {
	if t == nil {
		return nil, false
	}
	t = unwrap.Annotated(t)
	if !active.Enter(t, len(suffix)) {
		return t, false
	}
	defer active.Leave(t, len(suffix))
	switch v := t.(type) {
	case *typ.Alias:
		return narrowByPathTypeSeen(v.UnaliasedTarget(), suffix, peer, active)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return nil, false
		}
		return narrowByPathTypeSeen(v.Body, suffix, peer, active)
	case *typ.Optional:
		return narrowByPathTypeSeen(v.Inner, suffix, peer, active)
	case *typ.Instantiated:
		expanded, ok := subst.ExpandInstantiatedChanged(v)
		if !ok {
			return t, false
		}
		return narrowByPathTypeSeen(expanded, suffix, peer, active)
	case *typ.Union:
		out := make([]typ.Type, 0, len(v.Members))
		for _, member := range v.Members {
			if pathOverlapsType(member, suffix, peer) {
				out = append(out, member)
			}
		}
		if len(out) == 0 || len(out) == len(v.Members) {
			return t, false
		}
		return normalize.UnionForEvidence(out...), true
	default:
		if pathOverlapsType(t, suffix, peer) {
			return t, false
		}
		return typ.Never, true
	}
}

// pathOverlapsType reports whether the member at suffix of t can equal a value
// of peer. An unresolved member cannot be proven disjoint, so it keeps its arm:
// narrowing only drops an arm whose member is decisively incompatible.
func pathOverlapsType(t typ.Type, suffix []segment.Segment, peer typ.Type) bool {
	field, ok := fieldAtPath(t, suffix)
	if !ok {
		return true
	}
	return typesOverlap(field, peer)
}

// narrowByPathLiteralEntry guards the public path-literal narrowers: it rejects
// empty inputs, seeds an exact query-local cycle proof, and reports no change
// when the result is acyclically equal to t.
func narrowByPathLiteralEntry(
	t typ.Type,
	suffix []segment.Segment,
	lit typ.Type,
	narrow func(typ.Type, []segment.Segment, typ.Type, *typegraph.Path) (typ.Type, bool),
) (typ.Type, bool) {
	if t == nil || len(suffix) == 0 || lit == nil {
		return nil, false
	}
	narrowed, ok := narrow(t, suffix, lit, &typegraph.Path{})
	if !ok || narrowed == nil || typ.SameNodeOrAcyclicEqual(narrowed, t) {
		return narrowed, false
	}
	return narrowed, true
}

func narrowByPathLiteralSeen(t typ.Type, suffix []segment.Segment, lit typ.Type, active *typegraph.Path) (typ.Type, bool) {
	if t == nil {
		return nil, false
	}
	t = unwrap.Annotated(t)
	if !active.Enter(t, len(suffix)) {
		return t, false
	}
	defer active.Leave(t, len(suffix))
	switch v := t.(type) {
	case *typ.Alias:
		return narrowByPathLiteralSeen(v.UnaliasedTarget(), suffix, lit, active)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return nil, false
		}
		return narrowByPathLiteralSeen(v.Body, suffix, lit, active)
	case *typ.Optional:
		return narrowByPathLiteralSeen(v.Inner, suffix, lit, active)
	case *typ.Instantiated:
		expanded, ok := subst.ExpandInstantiatedChanged(v)
		if !ok {
			return t, false
		}
		return narrowByPathLiteralSeen(expanded, suffix, lit, active)
	case *typ.Union:
		out := make([]typ.Type, 0, len(v.Members))
		for _, member := range v.Members {
			if pathAdmitsLiteral(member, suffix, lit) {
				out = append(out, member)
			}
		}
		if len(out) == 0 || len(out) == len(v.Members) {
			return t, false
		}
		return normalize.UnionForEvidence(out...), true
	default:
		if pathAdmitsLiteral(t, suffix, lit) {
			return t, false
		}
		return typ.Never, true
	}
}

func narrowByPathLiteralNotSeen(t typ.Type, suffix []segment.Segment, lit typ.Type, active *typegraph.Path) (typ.Type, bool) {
	if t == nil {
		return nil, false
	}
	t = unwrap.Annotated(t)
	if !active.Enter(t, len(suffix)) {
		return t, false
	}
	defer active.Leave(t, len(suffix))
	switch v := t.(type) {
	case *typ.Alias:
		return narrowByPathLiteralNotSeen(v.UnaliasedTarget(), suffix, lit, active)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return nil, false
		}
		return narrowByPathLiteralNotSeen(v.Body, suffix, lit, active)
	case *typ.Optional:
		return narrowByPathLiteralNotSeen(v.Inner, suffix, lit, active)
	case *typ.Instantiated:
		expanded, ok := subst.ExpandInstantiatedChanged(v)
		if !ok {
			return t, false
		}
		return narrowByPathLiteralNotSeen(expanded, suffix, lit, active)
	case *typ.Union:
		out := make([]typ.Type, 0, len(v.Members))
		for _, member := range v.Members {
			if !pathForcesLiteral(member, suffix, lit) {
				out = append(out, member)
			}
		}
		if len(out) == len(v.Members) {
			return t, false
		}
		if len(out) == 0 {
			return typ.Never, true
		}
		return normalize.UnionForEvidence(out...), true
	default:
		if pathForcesLiteral(t, suffix, lit) {
			return typ.Never, true
		}
		return t, false
	}
}

func pathAdmitsLiteral(t typ.Type, suffix []segment.Segment, lit typ.Type) bool {
	field, ok := fieldAtPath(t, suffix)
	return ok && subtype.IsSubtype(lit, field)
}

func pathForcesLiteral(t typ.Type, suffix []segment.Segment, lit typ.Type) bool {
	field, ok := fieldAtPath(t, suffix)
	return ok && subtype.IsSubtype(field, lit)
}

// booleanRootUnion is the implicit two-member decomposition of typ.Boolean.
// Boolean is a closed, two-valued primitive rather than a *typ.Union node, but
// excluding one literal from it is exactly as representable as excluding a
// member from an explicit true|false union, so rootUnion treats it as one.
var booleanRootUnion = &typ.Union{Members: []typ.Type{typ.True, typ.False}}

// NarrowByLiteral keeps the members of a union t that lit inhabits: the
// `x == lit` guard's true edge for a root value, and the exact complement of
// NarrowByLiteralNot. A non-union type reports no narrowing, so an open scalar
// is never collapsed onto one literal; typ.Boolean is the one exception, since
// it is a closed two-valued type and decomposes into true|false. The returned
// bool reports whether a strict narrowing was possible.
func NarrowByLiteral(t typ.Type, lit typ.Type) (typ.Type, bool) {
	if t == nil || lit == nil {
		return nil, false
	}
	union, ok := rootUnion(t)
	if !ok {
		return t, false
	}
	out := make([]typ.Type, 0, len(union.Members))
	for _, member := range union.Members {
		if subtype.IsSubtype(lit, member) {
			out = append(out, member)
		}
	}
	if len(out) == 0 || len(out) == len(union.Members) {
		return t, false
	}
	return normalize.UnionForEvidence(out...), true
}

// NarrowByLiteralNot keeps the members of a union t that lit does not inhabit:
// the complement of an `x == lit` guard's true edge for a root value. A
// non-union type (an open scalar) cannot have a single literal subtracted, so it
// reports no narrowing; typ.Boolean is the one exception, since it is a closed
// two-valued type and decomposes into true|false. The returned bool reports
// whether a strict narrowing was possible.
func NarrowByLiteralNot(t typ.Type, lit typ.Type) (typ.Type, bool) {
	if t == nil || lit == nil {
		return nil, false
	}
	union, ok := rootUnion(t)
	if !ok {
		return t, false
	}
	out := make([]typ.Type, 0, len(union.Members))
	for _, member := range union.Members {
		if !subtype.IsSubtype(lit, member) {
			out = append(out, member)
		}
	}
	if len(out) == 0 || len(out) == len(union.Members) {
		return t, false
	}
	return normalize.UnionForEvidence(out...), true
}

func rootUnion(t typ.Type) (*typ.Union, bool) {
	return rootUnionSeen(t, &typegraph.Path{})
}

func rootUnionSeen(t typ.Type, active *typegraph.Path) (*typ.Union, bool) {
	if t == nil {
		return nil, false
	}
	t = unwrap.Annotated(t)
	if !active.Enter(t, 0) {
		return nil, false
	}
	defer active.Leave(t, 0)
	switch v := t.(type) {
	case *typ.Union:
		return v, true
	case *typ.Alias:
		return rootUnionSeen(v.UnaliasedTarget(), active)
	case *typ.Optional:
		return rootUnionSeen(v.Inner, active)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return nil, false
		}
		return rootUnionSeen(v.Body, active)
	case *typ.Instantiated:
		expanded, ok := subst.ExpandInstantiatedChanged(v)
		if !ok {
			return nil, false
		}
		return rootUnionSeen(expanded, active)
	default:
		if typ.TypeEquals(t, typ.Boolean) {
			return booleanRootUnion, true
		}
		return nil, false
	}
}

// NarrowByRuntimeType keeps the members of a union t whose Lua `type()` result
// is name when holds is true, and drops exactly those members when holds is
// false. It is the value-set companion of a `type(x) == "string"` guard for a
// root value: the guard states a runtime tag, and a union arm whose tag is
// decidable either matches it or is refuted by it. An arm whose tag cannot be
// decided - a gradual top, an unresolved parameter - is kept on both edges, so
// narrowing never removes an inhabitant the guard has not ruled out. A
// non-union type reports no narrowing. The returned bool reports whether a
// strict narrowing was possible.
func NarrowByRuntimeType(t typ.Type, name string, holds bool) (typ.Type, bool) {
	if t == nil || name == "" {
		return nil, false
	}
	union, ok := rootUnion(t)
	if !ok {
		return t, false
	}
	out := make([]typ.Type, 0, len(union.Members))
	for _, member := range union.Members {
		tag, decided := runtimeTypeNameOf(member)
		if !decided || (tag == name) == holds {
			out = append(out, member)
		}
	}
	if len(out) == 0 || len(out) == len(union.Members) {
		return t, false
	}
	return normalize.UnionForEvidence(out...), true
}

// runtimeTypeNameOf reports the single Lua `type()` result every inhabitant of
// t produces. A type whose inhabitants span more than one runtime tag - a
// union, an optional, a gradual top - has no single result and is undecided.
func runtimeTypeNameOf(t typ.Type) (string, bool) {
	return runtimeTypeNameOfSeen(t, &typegraph.Path{})
}

func runtimeTypeNameOfSeen(t typ.Type, active *typegraph.Path) (string, bool) {
	if t == nil {
		return "", false
	}
	t = unwrap.Annotated(t)
	if !active.Enter(t, 0) {
		return "", false
	}
	defer active.Leave(t, 0)
	switch v := t.(type) {
	case *typ.Alias:
		return runtimeTypeNameOfSeen(v.UnaliasedTarget(), active)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return "", false
		}
		return runtimeTypeNameOfSeen(v.Body, active)
	case *typ.Instantiated:
		expanded, ok := subst.ExpandInstantiatedChanged(v)
		if !ok {
			return "", false
		}
		return runtimeTypeNameOfSeen(expanded, active)
	case *typ.Literal:
		return runtimeTypeNameOfKind(v.Base)
	default:
		return runtimeTypeNameOfKind(t.Kind())
	}
}

func runtimeTypeNameOfKind(k kind.Kind) (string, bool) {
	switch k {
	case kind.Nil:
		return "nil", true
	case kind.Boolean:
		return "boolean", true
	case kind.Number, kind.Integer:
		return "number", true
	case kind.String:
		return "string", true
	case kind.Function:
		return "function", true
	case kind.Array, kind.Map, kind.ReadonlyMap, kind.Record:
		return "table", true
	default:
		return "", false
	}
}
