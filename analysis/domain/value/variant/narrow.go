package variant

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// NarrowByPathLiteral keeps the variants of t whose static member path admits
// lit. The returned bool reports whether a strict narrowing was possible.
func NarrowByPathLiteral(t typ.Type, suffix []segment.Segment, lit typ.Type) (typ.Type, bool) {
	return narrowByPathLiteralEntry(t, suffix, lit, narrowByPathLiteral)
}

// NarrowByPathLiteralNot keeps the variants of t whose static member path does
// not admit lit. The returned bool reports whether a strict narrowing was
// possible.
func NarrowByPathLiteralNot(t typ.Type, suffix []segment.Segment, lit typ.Type) (typ.Type, bool) {
	return narrowByPathLiteralEntry(t, suffix, lit, narrowByPathLiteralNot)
}

// narrowByPathLiteralEntry guards the public path-literal narrowers: it rejects
// empty inputs, seeds the recursion at depth 0 via narrow, and reports no change
// when the result is acyclically equal to t.
func narrowByPathLiteralEntry(
	t typ.Type,
	suffix []segment.Segment,
	lit typ.Type,
	narrow func(typ.Type, []segment.Segment, typ.Type, int) (typ.Type, bool),
) (typ.Type, bool) {
	if t == nil || len(suffix) == 0 || lit == nil {
		return nil, false
	}
	narrowed, ok := narrow(t, suffix, lit, 0)
	if !ok || narrowed == nil || typ.SameNodeOrAcyclicEqual(narrowed, t) {
		return narrowed, false
	}
	return narrowed, true
}

func narrowByPathLiteral(t typ.Type, suffix []segment.Segment, lit typ.Type, depth int) (typ.Type, bool) {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Alias:
		return narrowByPathLiteral(v.UnaliasedTarget(), suffix, lit, depth+1)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return nil, false
		}
		return narrowByPathLiteral(v.Body, suffix, lit, depth+1)
	case *typ.Optional:
		return narrowByPathLiteral(v.Inner, suffix, lit, depth+1)
	case *typ.Instantiated:
		expanded, ok := subst.ExpandInstantiatedChanged(v)
		if !ok {
			return t, false
		}
		return narrowByPathLiteral(expanded, suffix, lit, depth+1)
	case *typ.Union:
		out := make([]typ.Type, 0, len(v.Members))
		for _, member := range v.Members {
			if pathAdmitsLiteral(member, suffix, lit, depth+1) {
				out = append(out, member)
			}
		}
		if len(out) == 0 || len(out) == len(v.Members) {
			return t, false
		}
		return normalize.UnionForEvidence(out...), true
	default:
		if pathAdmitsLiteral(t, suffix, lit, depth+1) {
			return t, false
		}
		return typ.Never, true
	}
}

func narrowByPathLiteralNot(t typ.Type, suffix []segment.Segment, lit typ.Type, depth int) (typ.Type, bool) {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Alias:
		return narrowByPathLiteralNot(v.UnaliasedTarget(), suffix, lit, depth+1)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return nil, false
		}
		return narrowByPathLiteralNot(v.Body, suffix, lit, depth+1)
	case *typ.Optional:
		return narrowByPathLiteralNot(v.Inner, suffix, lit, depth+1)
	case *typ.Instantiated:
		expanded, ok := subst.ExpandInstantiatedChanged(v)
		if !ok {
			return t, false
		}
		return narrowByPathLiteralNot(expanded, suffix, lit, depth+1)
	case *typ.Union:
		out := make([]typ.Type, 0, len(v.Members))
		for _, member := range v.Members {
			if !pathAdmitsLiteral(member, suffix, lit, depth+1) {
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
		if pathAdmitsLiteral(t, suffix, lit, depth+1) {
			return typ.Never, true
		}
		return t, false
	}
}

func pathAdmitsLiteral(t typ.Type, suffix []segment.Segment, lit typ.Type, depth int) bool {
	field, ok := fieldAtPath(t, suffix, depth+1)
	return ok && subtype.IsSubtype(lit, field)
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
	union, ok := rootUnion(t, 0)
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

func rootUnion(t typ.Type, depth int) (*typ.Union, bool) {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Union:
		return v, true
	case *typ.Alias:
		return rootUnion(v.UnaliasedTarget(), depth+1)
	case *typ.Optional:
		return rootUnion(v.Inner, depth+1)
	default:
		return nil, false
	}
}
