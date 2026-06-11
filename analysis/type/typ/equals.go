package typ

import "reflect"

// TypeEquals compares two types for structural equality with cycle detection.
func TypeEquals(a, b Type) bool {
	return typeEquals(a, b)
}

// typeEquals compares two types for structural equality with cycle detection.
//
// Uses coinductive equality for recursive types: if the same type pair is
// encountered again during traversal, they are assumed equal. This handles
// infinite recursive structures correctly.
//
// Aliases are transparent: compares through to their targets.
func typeEquals(a, b Type) bool {
	if a == b {
		return true
	}
	guard := NewGuard()
	return typeEqualsGuard(a, b, guard, nil)
}

// SameNode reports whether two Type interface values point at the same
// immutable type node. It is intentionally not structural equality; callers
// use it to detect no-op rewrites without walking recursive products.
func SameNode(a, b Type) bool {
	if a == nil || b == nil {
		return a == b
	}
	va := reflect.ValueOf(a)
	vb := reflect.ValueOf(b)
	if va.Type() != vb.Type() || !va.Type().Comparable() {
		return false
	}
	return a == b
}

// SameNodeOrAcyclicEqual reports identity or structural equality for products
// that cannot contain recursive cycles. Recursive product-family equivalence is
// a domain relation; generic constructors and hot convergence paths must not
// prove it by unfolding structural equality.
func SameNodeOrAcyclicEqual(a, b Type) bool {
	return sameNodeOrAcyclicEqual(a, b)
}

// sameNodeOrAcyclicEqual reports identity or structural equality for products
// that cannot contain recursive cycles. Recursive product-family equivalence is
// a domain relation; generic constructors and hot convergence paths must not
// prove it by unfolding structural equality.
func sameNodeOrAcyclicEqual(a, b Type) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if knownContainsRecursive(a) || knownContainsRecursive(b) ||
		knownContainsOpenRecursive(a) || knownContainsOpenRecursive(b) {
		return false
	}
	return typeEquals(a, b)
}
