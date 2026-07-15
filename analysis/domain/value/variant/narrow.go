package variant

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/internal/typegraph"
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

// NarrowByLiteralNot keeps the members of a union t that lit does not inhabit:
// the complement of an `x == lit` guard's true edge for a root value. A
// non-union type (an open scalar) cannot have a single literal subtracted, so it
// reports no narrowing. The returned bool reports whether a strict narrowing was
// possible.
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
		return nil, false
	}
}
