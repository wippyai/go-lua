package typ

func knownContainsOpenRecursive(t Type) bool {
	t = unwrapAnnotatedOrNil(t)
	if t == nil {
		return false
	}
	switch n := t.(type) {
	case *Recursive:
		n.ensureContainsClosedFlag()
		return !n.containsFlagsClosed
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
