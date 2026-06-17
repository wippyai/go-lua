package typ

// cachedContainsFlags returns the cached containment flags for a composite type
// node: whether it transitively contains any, never, a type parameter, or an
// instantiated generic. Non-composite types return all false. An *Instantiated
// node trivially contains an instantiated type, and a *TypeParam node trivially
// contains a type parameter.
func cachedContainsFlags(t Type) (containsAny, containsNever, containsTypeParam, containsInstantiated bool) {
	switch n := t.(type) {
	case *Recursive:
		n.ensureContainsFlags()
		return n.containsAny, n.containsNever, n.containsTypeParam, n.containsInstantiated
	case *Instantiated:
		return n.containsAny, n.containsNever, n.containsTypeParam, true
	case *TypeParam:
		return n.containsAny, n.containsNever, true, n.containsInstantiated
	case *Optional:
		return n.containsAny, n.containsNever, n.containsTypeParam, n.containsInstantiated
	case *Union:
		return n.containsAny, n.containsNever, n.containsTypeParam, n.containsInstantiated
	case *Intersection:
		return n.containsAny, n.containsNever, n.containsTypeParam, n.containsInstantiated
	case *Array:
		return n.containsAny, n.containsNever, n.containsTypeParam, n.containsInstantiated
	case *Map:
		return n.containsAny, n.containsNever, n.containsTypeParam, n.containsInstantiated
	case *ReadonlyMap:
		return n.containsAny, n.containsNever, n.containsTypeParam, n.containsInstantiated
	case *Tuple:
		return n.containsAny, n.containsNever, n.containsTypeParam, n.containsInstantiated
	case *Function:
		return n.containsAny, n.containsNever, n.containsTypeParam, n.containsInstantiated
	case *Record:
		return n.containsAny, n.containsNever, n.containsTypeParam, n.containsInstantiated
	case *Alias:
		return n.containsAny, n.containsNever, n.containsTypeParam, n.containsInstantiated
	case *Meta:
		return n.containsAny, n.containsNever, n.containsTypeParam, n.containsInstantiated
	case *Generic:
		return n.containsAny, n.containsNever, n.containsTypeParam, n.containsInstantiated
	case *Interface:
		return n.containsAny, n.containsNever, n.containsTypeParam, n.containsInstantiated
	default:
		return false, false, false, false
	}
}

func knownContainsAny(t Type) bool {
	t = unwrapAnnotatedOrNil(t)
	if t == nil {
		return false
	}
	if IsAny(t) {
		return true
	}
	containsAny, _, _, _ := cachedContainsFlags(t)
	return containsAny
}

func knownContainsNever(t Type) bool {
	t = unwrapAnnotatedOrNil(t)
	if t == nil {
		return false
	}
	if IsNever(t) {
		return true
	}
	_, containsNever, _, _ := cachedContainsFlags(t)
	return containsNever
}

func knownContainsTypeParam(t Type) bool {
	t = unwrapAnnotatedOrNil(t)
	if t == nil {
		return false
	}
	_, _, containsTypeParam, _ := cachedContainsFlags(t)
	return containsTypeParam
}

func knownContainsInstantiated(t Type) bool {
	t = unwrapAnnotatedOrNil(t)
	if t == nil {
		return false
	}
	_, _, _, containsInstantiated := cachedContainsFlags(t)
	return containsInstantiated
}
