package typ

// ContainsRecursive reports whether t is, or transitively contains, a
// recursive product type.
func ContainsRecursive(t Type) bool {
	return knownContainsRecursive(t)
}

func knownContainsRecursive(t Type) bool {
	t = unwrapAnnotatedOrNil(t)
	if t == nil {
		return false
	}
	switch n := t.(type) {
	case *Recursive:
		return true
	case *Optional:
		return n.containsRecursive
	case *Union:
		return n.containsRecursive
	case *Intersection:
		return n.containsRecursive
	case *Array:
		return n.containsRecursive
	case *Map:
		return n.containsRecursive
	case *ReadonlyMap:
		return n.containsRecursive
	case *Tuple:
		return n.containsRecursive
	case *Function:
		return n.containsRecursive
	case *Record:
		return n.containsRecursive
	case *Alias:
		return n.containsRecursive
	case *Meta:
		return n.containsRecursive
	case *Generic:
		return n.containsRecursive
	case *Instantiated:
		// Read live rather than from the construction-time snapshot for the
		// same reason as instantiatedContainsAny/Never: the Generic's own flag
		// self-heals in place on SetBody, a copy taken before that does not.
		if knownContainsRecursive(n.Generic) {
			return true
		}
		for _, argument := range n.TypeArgs {
			if knownContainsRecursive(argument) {
				return true
			}
		}
		return false
	case *TypeParam:
		return n.containsRecursive
	case *Interface:
		return n.containsRecursive
	default:
		return false
	}
}
