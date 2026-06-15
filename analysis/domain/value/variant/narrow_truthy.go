package variant

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// isTruthySentinel reports whether lit is the literal-true value that the
// truthy-guard narrowing paths pass to signal a presence/truthiness query
// rather than a structural literal-tag comparison.
func isTruthySentinel(lit typ.Type) bool {
	l, ok := unwrap.Annotated(lit).(*typ.Literal)
	return ok && l.Base == kind.Boolean && l.Value == true
}

// armAdmitsTruthiness reports whether a single union arm can hold the requested
// truthiness at suffix. A field that is absent reads as nil: it can be falsy but
// never truthy.
func armAdmitsTruthiness(arm typ.Type, suffix []segment.Segment, wantTruthy bool, depth int) bool {
	field, ok := fieldAtPath(arm, suffix, depth+1)
	if !ok {
		return !wantTruthy
	}
	if wantTruthy {
		return typeCanBeTruthy(field, 0)
	}
	return typeCanBeFalsy(field, 0)
}

// typeCanBeTruthy reports whether t has a non-nil, non-false inhabitant.
func typeCanBeTruthy(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	if typ.IsAny(t) || typ.IsUnknown(t) {
		return true
	}
	switch v := unwrap.Annotated(unwrap.NormalizeNil(t)).(type) {
	case *typ.Alias:
		return typeCanBeTruthy(v.UnaliasedTarget(), depth+1)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return false
		}
		return typeCanBeTruthy(v.Body, depth+1)
	case *typ.Optional:
		return typeCanBeTruthy(v.Inner, depth+1)
	case *typ.Instantiated:
		expanded, ok := subst.ExpandInstantiatedChanged(v)
		if !ok {
			return true
		}
		return typeCanBeTruthy(expanded, depth+1)
	case *typ.Union:
		for _, member := range v.Members {
			if typeCanBeTruthy(member, depth+1) {
				return true
			}
		}
		return false
	case *typ.Literal:
		return !(v.Base == kind.Boolean && v.Value == false)
	default:
		if v == nil {
			return false
		}
		return v.Kind() != kind.Nil
	}
}

// typeCanBeFalsy reports whether t admits nil or false.
func typeCanBeFalsy(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	if typ.IsAny(t) || typ.IsUnknown(t) {
		return true
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Alias:
		return typeCanBeFalsy(v.UnaliasedTarget(), depth+1)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return false
		}
		return typeCanBeFalsy(v.Body, depth+1)
	case *typ.Optional:
		return true
	case *typ.Instantiated:
		expanded, ok := subst.ExpandInstantiatedChanged(v)
		if !ok {
			return true
		}
		return typeCanBeFalsy(expanded, depth+1)
	case *typ.Union:
		for _, member := range v.Members {
			if typeCanBeFalsy(member, depth+1) {
				return true
			}
		}
		return false
	case *typ.Literal:
		return v.Base == kind.Boolean && v.Value == false
	default:
		normalized := unwrap.NormalizeNil(unwrap.Annotated(t))
		if normalized == nil {
			return true
		}
		k := normalized.Kind()
		return k == kind.Nil || k == kind.Boolean
	}
}
