package inspect

import (
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func containsMatching(t typ.Type, pred func(typ.Type) bool) bool {
	if pred == nil {
		return false
	}
	scan := NewScanner(ScanOptions{Seen: newEqualitySeen()})
	return contains(t, pred, scan)
}

// ContainsTypeParam reports whether t contains a type parameter.
func ContainsTypeParam(t typ.Type) bool {
	return containsMatching(t, func(t typ.Type) bool {
		_, ok := t.(*typ.TypeParam)
		return ok
	})
}

// ContainsInstantiated reports whether t contains a generic instantiation.
func ContainsInstantiated(t typ.Type) bool {
	return containsMatching(t, func(t typ.Type) bool {
		_, ok := t.(*typ.Instantiated)
		return ok
	})
}

// ContainsRecursive reports whether t contains a recursive product.
func ContainsRecursive(t typ.Type) bool {
	return typ.ContainsRecursive(t)
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
