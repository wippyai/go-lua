package variant

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/internal/typegraph"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// isTruthySentinel reports whether lit is the literal-true value that the
// truthy-guard narrowing paths pass to signal a presence/truthiness query
// rather than a structural literal-tag comparison.
func isTruthySentinel(lit typ.Type) bool {
	l, ok := unwrap.Annotated(lit).(*typ.Literal)
	return ok && l.Base() == kind.Boolean && l.Value() == true
}

// NarrowByPathTruthy keeps variants whose member path can be truthy under Lua
// truthiness. This is intentionally separate from NarrowByPathLiteral(...,
// true): `x == true` is a literal comparison, while `if x` admits any non-nil,
// non-false value.
func NarrowByPathTruthy(t typ.Type, suffix []segment.Segment) (typ.Type, bool) {
	return narrowByPathTruthinessEntry(t, suffix, true)
}

// NarrowByPathFalsy keeps variants whose member path can be falsy under Lua
// truthiness. A missing table member reads as nil, so absence is a falsy arm.
func NarrowByPathFalsy(t typ.Type, suffix []segment.Segment) (typ.Type, bool) {
	return narrowByPathTruthinessEntry(t, suffix, false)
}

func narrowByPathTruthinessEntry(t typ.Type, suffix []segment.Segment, wantTruthy bool) (typ.Type, bool) {
	if t == nil || len(suffix) == 0 {
		return nil, false
	}
	narrowed, ok := narrowByPathTruthinessSeen(t, suffix, wantTruthy, &typegraph.Path{})
	if !ok || narrowed == nil || typ.SameNodeOrAcyclicEqual(narrowed, t) {
		return narrowed, false
	}
	return narrowed, true
}

func narrowByPathTruthinessSeen(t typ.Type, suffix []segment.Segment, wantTruthy bool, active *typegraph.Path) (typ.Type, bool) {
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
		return narrowByPathTruthinessSeen(v.UnaliasedTarget(), suffix, wantTruthy, active)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return nil, false
		}
		return narrowByPathTruthinessSeen(v.Body, suffix, wantTruthy, active)
	case *typ.Optional:
		if wantTruthy {
			narrowed, ok := narrowByPathTruthinessSeen(v.Inner, suffix, true, active)
			if !ok {
				return v.Inner, true
			}
			return narrowed, true
		}
		if armAdmitsTruthiness(v.Inner, suffix, false) {
			return t, false
		}
		return typ.Nil, true
	case *typ.Instantiated:
		expanded, ok := subst.ExpandInstantiatedChanged(v)
		if !ok {
			return t, false
		}
		return narrowByPathTruthinessSeen(expanded, suffix, wantTruthy, active)
	case *typ.Union:
		out := make([]typ.Type, 0, len(v.Members))
		for _, member := range v.Members {
			if armAdmitsTruthiness(member, suffix, wantTruthy) {
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
		if armAdmitsTruthiness(t, suffix, wantTruthy) {
			return t, false
		}
		return typ.Never, true
	}
}

// armAdmitsTruthiness reports whether a single union arm can hold the requested
// truthiness at suffix. A field that is absent reads as nil: it can be falsy but
// never truthy.
func armAdmitsTruthiness(arm typ.Type, suffix []segment.Segment, wantTruthy bool) bool {
	field, ok := fieldAtPath(arm, suffix)
	if !ok {
		return !wantTruthy
	}
	if wantTruthy {
		return typeCanBeTruthy(field)
	}
	return typeCanBeFalsy(field)
}

// typeCanBeTruthy reports whether t has a non-nil, non-false inhabitant.
func typeCanBeTruthy(t typ.Type) bool {
	return typeCanBeTruthySeen(t, &typegraph.Path{})
}

func typeCanBeTruthySeen(t typ.Type, active *typegraph.Path) bool {
	if t == nil {
		return false
	}
	if typ.IsAny(t) || typ.IsUnknown(t) {
		return true
	}
	t = unwrap.Annotated(unwrap.NormalizeNil(t))
	if !active.Enter(t, 0) {
		return false
	}
	defer active.Leave(t, 0)
	switch v := t.(type) {
	case *typ.Alias:
		return typeCanBeTruthySeen(v.UnaliasedTarget(), active)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return false
		}
		return typeCanBeTruthySeen(v.Body, active)
	case *typ.Optional:
		return typeCanBeTruthySeen(v.Inner, active)
	case *typ.Instantiated:
		expanded, ok := subst.ExpandInstantiatedChanged(v)
		if !ok {
			return true
		}
		return typeCanBeTruthySeen(expanded, active)
	case *typ.Union:
		for _, member := range v.Members {
			if typeCanBeTruthySeen(member, active) {
				return true
			}
		}
		return false
	case *typ.Literal:
		return !(v.Base() == kind.Boolean && v.Value() == false)
	default:
		if v == nil {
			return false
		}
		return v.Kind() != kind.Nil
	}
}

// typeCanBeFalsy reports whether t admits nil or false.
func typeCanBeFalsy(t typ.Type) bool {
	return typeCanBeFalsySeen(t, &typegraph.Path{})
}

func typeCanBeFalsySeen(t typ.Type, active *typegraph.Path) bool {
	if t == nil {
		return false
	}
	if typ.IsAny(t) || typ.IsUnknown(t) {
		return true
	}
	t = unwrap.Annotated(t)
	if !active.Enter(t, 0) {
		return false
	}
	defer active.Leave(t, 0)
	switch v := t.(type) {
	case *typ.Alias:
		return typeCanBeFalsySeen(v.UnaliasedTarget(), active)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return false
		}
		return typeCanBeFalsySeen(v.Body, active)
	case *typ.Optional:
		return true
	case *typ.Instantiated:
		expanded, ok := subst.ExpandInstantiatedChanged(v)
		if !ok {
			return true
		}
		return typeCanBeFalsySeen(expanded, active)
	case *typ.Union:
		for _, member := range v.Members {
			if typeCanBeFalsySeen(member, active) {
				return true
			}
		}
		return false
	case *typ.Literal:
		return v.Base() == kind.Boolean && v.Value() == false
	default:
		normalized := unwrap.NormalizeNil(unwrap.Annotated(t))
		if normalized == nil {
			return true
		}
		k := normalized.Kind()
		return k == kind.Nil || k == kind.Boolean
	}
}
