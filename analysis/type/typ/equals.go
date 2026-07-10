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
	seen := typePairSet{}
	return typeEqualsGuard(a, b, guard, &seen)
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

// SameNodeOrAcyclicEqual reports the identity-or-acyclic-equality fast path.
// It is intentionally narrower than TypeEquals: recursive product-family
// equivalence is a domain relation, and callers that need that must use the
// full recursive equality machinery.
func SameNodeOrAcyclicEqual(a, b Type) bool {
	return sameNodeOrAcyclicEqual(a, b)
}

// SameNodeOrRecursiveIdentityEqual reports equality suitable for preserving an
// existing recursive-containing wrapper during no-op rewrites. Distinct
// recursive identity graphs are never collapsed even when their unfolded
// structure is equal.
func SameNodeOrRecursiveIdentityEqual(a, b Type) bool {
	if sameNodeOrAcyclicEqual(a, b) {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if knownContainsRecursive(a) || knownContainsRecursive(b) ||
		mayContainOpenRecursive(a) || mayContainOpenRecursive(b) {
		if !sameRecursiveIdentityGraph(a, b) {
			return false
		}
		return typeEquals(a, b)
	}
	return false
}

// sameNodeOrAcyclicEqual reports the identity-or-acyclic-equality fast path.
// It is intentionally narrower than TypeEquals: recursive product-family
// equivalence is a domain relation, and callers that need that must use the
// full recursive equality machinery.
func sameNodeOrAcyclicEqual(a, b Type) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if knownContainsRecursive(a) || knownContainsRecursive(b) ||
		mayContainOpenRecursive(a) || mayContainOpenRecursive(b) {
		return false
	}
	return typeEquals(a, b)
}
