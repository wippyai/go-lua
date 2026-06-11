package typ

func knownContainsTypeParam(t Type) bool {
	if t == nil {
		return false
	}
	t = unwrapAnnotated(t)
	if t == nil {
		return false
	}
	switch n := t.(type) {
	case *TypeParam:
		return true
	case *Recursive:
		n.ensureContainsFlags()
		return n.containsTypeParam
	case *Optional:
		return n.containsTypeParam
	case *Union:
		return n.containsTypeParam
	case *Intersection:
		return n.containsTypeParam
	case *Array:
		return n.containsTypeParam
	case *Map:
		return n.containsTypeParam
	case *ReadonlyMap:
		return n.containsTypeParam
	case *Tuple:
		return n.containsTypeParam
	case *Function:
		return n.containsTypeParam
	case *Record:
		return n.containsTypeParam
	case *Alias:
		return n.containsTypeParam
	case *Meta:
		return n.containsTypeParam
	case *Generic:
		return n.containsTypeParam
	case *Instantiated:
		return n.containsTypeParam
	case *Interface:
		return n.containsTypeParam
	default:
		return false
	}
}
