package variant

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// NarrowByPathLiteral keeps the variants of t whose static member path admits
// lit. The returned bool reports whether a strict narrowing was possible.
func NarrowByPathLiteral(t typ.Type, suffix []segment.Segment, lit typ.Type) (typ.Type, bool) {
	if t == nil || len(suffix) == 0 || lit == nil {
		return nil, false
	}
	narrowed, ok := narrowByPathLiteral(t, suffix, lit, 0)
	if !ok || narrowed == nil || typ.SameNodeOrAcyclicEqual(narrowed, t) {
		return narrowed, false
	}
	return narrowed, true
}

// NarrowByPathLiteralNot keeps the variants of t whose static member path does
// not admit lit. The returned bool reports whether a strict narrowing was
// possible.
func NarrowByPathLiteralNot(t typ.Type, suffix []segment.Segment, lit typ.Type) (typ.Type, bool) {
	if t == nil || len(suffix) == 0 || lit == nil {
		return nil, false
	}
	narrowed, ok := narrowByPathLiteralNot(t, suffix, lit, 0)
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
	case *typ.Optional:
		return narrowByPathLiteral(v.Inner, suffix, lit, depth+1)
	case *typ.Instantiated:
		expanded, ok := expandInstantiated(v)
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
	case *typ.Optional:
		return narrowByPathLiteralNot(v.Inner, suffix, lit, depth+1)
	case *typ.Instantiated:
		expanded, ok := expandInstantiated(v)
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
