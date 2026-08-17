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
	switch n := t.(type) {
	case *Recursive:
		return true
	case *Optional:
		return n.containsOpenRecursive
	case *Union:
		return n.containsOpenRecursive
	case *Intersection:
		return n.containsOpenRecursive
	case *Array:
		return n.containsOpenRecursive
	case *Map:
		return n.containsOpenRecursive
	case *ReadonlyMap:
		return n.containsOpenRecursive
	case *Tuple:
		return n.containsOpenRecursive
	case *Function:
		return n.containsOpenRecursive
	case *Record:
		return n.containsOpenRecursive
	case *Alias:
		return n.containsOpenRecursive
	case *Meta:
		return n.containsOpenRecursive
	case *Generic:
		return n.containsOpenRecursive
	case *Instantiated:
		return n.containsOpenRecursive
	case *TypeParam:
		return n.containsOpenRecursive
	case *Interface:
		return n.containsOpenRecursive
	default:
		return false
	}
}
