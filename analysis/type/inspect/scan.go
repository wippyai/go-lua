package inspect

import (
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Contains reports whether t or any nested type satisfies pred.
func Contains(t typ.Type, pred func(typ.Type) bool) bool {
	if pred == nil {
		return false
	}
	scan := NewScanner(ScanOptions{Seen: NewEqualitySeen()})
	return contains(t, pred, scan)
}

// ContainsAny reports whether t contains the dynamic any type.
func ContainsAny(t typ.Type) bool {
	return Contains(t, typ.IsAny)
}

// ContainsNever reports whether t contains the unreachable never type.
func ContainsNever(t typ.Type) bool {
	return Contains(t, typ.IsNever)
}

// ContainsTypeParam reports whether t contains a type parameter.
func ContainsTypeParam(t typ.Type) bool {
	return Contains(t, func(t typ.Type) bool {
		_, ok := t.(*typ.TypeParam)
		return ok
	})
}

// ContainsInstantiated reports whether t contains a generic instantiation.
func ContainsInstantiated(t typ.Type) bool {
	return Contains(t, func(t typ.Type) bool {
		_, ok := t.(*typ.Instantiated)
		return ok
	})
}

// ContainsRecursive reports whether t contains a recursive product.
func ContainsRecursive(t typ.Type) bool {
	scan := NewScanner(ScanOptions{Seen: NewIdentitySeen(trackConcreteNodeIdentity)})
	return contains(t, func(t typ.Type) bool {
		_, ok := t.(*typ.Recursive)
		return ok
	}, scan)
}

func trackConcreteNodeIdentity(t typ.Type) bool {
	switch t.(type) {
	case *typ.Optional,
		*typ.Union,
		*typ.Intersection,
		*typ.Array,
		*typ.Map,
		*typ.ReadonlyMap,
		*typ.Tuple,
		*typ.Function,
		*typ.Record,
		*typ.Alias,
		*typ.Ref,
		*typ.Meta,
		*typ.Generic,
		*typ.Instantiated,
		*typ.TypeParam,
		*typ.Interface,
		*typ.Recursive,
		*typ.Literal:
		return true
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

	return scan.WalkChildren(t, func(child typ.Type) bool {
		return contains(child, pred, scan)
	})
}
