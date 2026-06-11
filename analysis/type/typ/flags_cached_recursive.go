package typ

func knownContainsRecursive(t Type) bool {
	if t == nil {
		return false
	}
	t = unwrapAnnotated(t)
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
		return n.containsRecursive
	case *TypeParam:
		return n.containsRecursive
	case *Interface:
		return n.containsRecursive
	default:
		return false
	}
}
