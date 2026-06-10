package typ

func knownContainsAny(t Type) bool {
	if t == nil {
		return false
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return false
	}
	if IsAny(t) {
		return true
	}
	switch n := t.(type) {
	case *Recursive:
		n.ensureContainsFlags()
		return n.containsAny
	case *Optional:
		return n.containsAny
	case *Union:
		return n.containsAny
	case *Intersection:
		return n.containsAny
	case *Array:
		return n.containsAny
	case *Map:
		return n.containsAny
	case *ReadonlyMap:
		return n.containsAny
	case *Tuple:
		return n.containsAny
	case *Function:
		return n.containsAny
	case *Record:
		return n.containsAny
	case *Alias:
		return n.containsAny
	case *Meta:
		return n.containsAny
	case *Generic:
		return n.containsAny
	case *Instantiated:
		return n.containsAny
	case *TypeParam:
		return n.containsAny
	case *Interface:
		return n.containsAny
	default:
		return false
	}
}
