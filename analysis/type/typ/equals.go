package typ

// TypeEquals compares two types for structural equality with cycle detection.
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

// SameNodeOrAcyclicEqual reports identity or structural equality for products
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
