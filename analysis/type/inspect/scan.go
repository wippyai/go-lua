package inspect

import (
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// Contains reports whether t or any nested type satisfies pred.
func Contains(t typ.Type, pred func(typ.Type) bool) bool {
	if pred == nil {
		return false
	}
	scan := NewScanner(ScanOptions{Seen: NewEqualitySeen()})
	return contains(t, pred, scan)
}

// ContainsInstantiated reports whether t contains a generic instantiation.
func ContainsInstantiated(t typ.Type) bool {
	return typ.ContainsInstantiated(t)
}

// ContainsRecursive reports whether t contains a recursive product.
func ContainsRecursive(t typ.Type) bool {
	return typ.ContainsRecursive(t)
}

func contains(t typ.Type, pred func(typ.Type) bool, scan *Scanner) bool {
	if t == nil {
		return false
	}
	t = unwrap.Annotated(t)
	if t == nil {
		return false
	}
	if !scan.Enter(t) {
		return false
	}
	if pred(t) {
		return true
	}

	return scan.WalkChildren(t, func(child typ.Type) bool {
		return contains(child, pred, scan)
	})
}
