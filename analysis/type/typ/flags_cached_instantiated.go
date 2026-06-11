package typ

func knownContainsInstantiated(t Type) bool {
	if t == nil {
		return false
	}
	t = unwrapAnnotated(t)
	if t == nil {
		return false
	}
	switch n := t.(type) {
	case *Instantiated:
		return true
	case *Recursive:
		n.ensureContainsFlags()
		return n.containsInstantiated
	case *Optional:
		return n.containsInstantiated
	case *Union:
		return n.containsInstantiated
	case *Intersection:
		return n.containsInstantiated
	case *Array:
		return n.containsInstantiated
	case *Map:
		return n.containsInstantiated
	case *ReadonlyMap:
		return n.containsInstantiated
	case *Tuple:
		return n.containsInstantiated
	case *Function:
		return n.containsInstantiated
	case *Record:
		return n.containsInstantiated
	case *Alias:
		return n.containsInstantiated
	case *Meta:
		return n.containsInstantiated
	case *Generic:
		return n.containsInstantiated
	case *TypeParam:
		return n.containsInstantiated
	case *Interface:
		return n.containsInstantiated
	default:
		return false
	}
}
