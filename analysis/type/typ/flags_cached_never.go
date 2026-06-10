package typ

func knownContainsNever(t Type) bool {
	if t == nil {
		return false
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return false
	}
	if IsNever(t) {
		return true
	}
	switch n := t.(type) {
	case *Recursive:
		n.ensureContainsFlags()
		return n.containsNever
	case *Optional:
		return n.containsNever
	case *Union:
		return n.containsNever
	case *Intersection:
		return n.containsNever
	case *Array:
		return n.containsNever
	case *Map:
		return n.containsNever
	case *ReadonlyMap:
		return n.containsNever
	case *Tuple:
		return n.containsNever
	case *Function:
		return n.containsNever
	case *Record:
		return n.containsNever
	case *Alias:
		return n.containsNever
	case *Meta:
		return n.containsNever
	case *Generic:
		return n.containsNever
	case *Instantiated:
		return n.containsNever
	case *TypeParam:
		return n.containsNever
	case *Sum:
		return n.containsNever
	case *Interface:
		return n.containsNever
	default:
		return false
	}
}
