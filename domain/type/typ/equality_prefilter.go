package typ

func typeEqualsCanUseHashPrefilter(a, b Type) bool {
	return !knownContainsRecursive(a) &&
		!knownContainsRecursive(b) &&
		!mayContainOpenRecursive(a) &&
		!mayContainOpenRecursive(b) &&
		// EqualityHash must refresh every graph containing an Instantiated
		// or Generic declaration because SetBody can change its structural
		// meaning. Applying that refresh at every nested pair turns a deep
		// generic product into repeated suffix traversals. Structural equality
		// below remains the collision-resolving proof, so these mutable graphs
		// simply skip the optional hash prefilter.
		!knownContainsInstantiated(a) &&
		!knownContainsInstantiated(b) &&
		!knownContainsGeneric(a) &&
		!knownContainsGeneric(b)
}

// mayContainOpenRecursive is deliberately cache-only. Equality's hash
// prefilter is an optional optimization, so an old positive cache entry may
// only make equality take the structural path. It must not start a live graph
// traversal while equality is already walking a recursive product.
func mayContainOpenRecursive(t Type) bool {
	t = unwrapAnnotatedOrNil(t)
	if t == nil {
		return false
	}
	switch t.(type) {
	case *Recursive:
		return true
	}
	properties := nodeProperties(t)
	return properties != nil && properties.containsOpenRecursive
}
