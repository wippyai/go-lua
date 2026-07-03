package inspect

import (
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func containsMatching(t typ.Type, pred func(typ.Type) bool) bool {
	if pred == nil {
		return false
	}
	scan := NewScanner(ScanOptions{Seen: newEqualitySeen()})
	return contains(t, pred, scan)
}

// ContainsAny reports whether t contains the gradual any type.
func ContainsAny(t typ.Type) bool {
	return typ.ContainsAny(t)
}

// ContainsUnknown reports whether t contains the unresolved unknown type.
func ContainsUnknown(t typ.Type) bool {
	return containsMatching(t, typ.IsUnknown)
}

// ContainsTypeParam reports whether t contains a type parameter.
func ContainsTypeParam(t typ.Type) bool {
	return typ.ContainsTypeParam(t)
}

// ContainsInstantiated reports whether t contains a generic instantiation.
func ContainsInstantiated(t typ.Type) bool {
	return typ.ContainsInstantiated(t)
}

// ContainsRecursive reports whether t contains a recursive product.
func ContainsRecursive(t typ.Type) bool {
	return typ.ContainsRecursive(t)
}

// IsMultiArmUnion reports whether t resolves through transparent wrappers to a
// union with at least two members.
func IsMultiArmUnion(t typ.Type) bool {
	return isMultiArmUnionDepth(t, 0)
}

func isMultiArmUnionDepth(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Union:
		return len(v.Members) >= 2
	case *typ.Alias:
		return isMultiArmUnionDepth(v.UnaliasedTarget(), depth+1)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return false
		}
		return isMultiArmUnionDepth(v.Body, depth+1)
	default:
		return false
	}
}

func contains(t typ.Type, pred func(typ.Type) bool, scan *Scanner) bool {
	if t == nil {
		return false
	}
	t = unwrapTransparent(t)
	if t == nil {
		return false
	}
	if !scan.Enter(t) {
		return false
	}
	if pred(t) {
		return true
	}

	return typ.WalkChildren(t, func(child typ.Type) bool {
		return contains(child, pred, scan)
	})
}
