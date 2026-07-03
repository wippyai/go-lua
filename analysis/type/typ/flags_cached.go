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

// ContainsAny reports whether t is, or transitively contains, the gradual any
// type. A cached positive is definitive. A cached negative is definitive only
// for non-recursive products; recursive placeholders can receive a later body,
// so recursive graphs fall back to a live scan.
func ContainsAny(t Type) bool {
	if knownContainsAny(t) {
		return true
	}
	if !knownContainsRecursive(t) {
		return false
	}
	return containsAnyDynamic(t, nil, 0)
}

// ContainsNever reports whether t is, or transitively contains, never.
func ContainsNever(t Type) bool {
	if knownContainsNever(t) {
		return true
	}
	if !knownContainsRecursive(t) {
		return false
	}
	return containsNeverDynamic(t, nil)
}

// ContainsTypeParam reports whether t is, or transitively contains, a type
// parameter node.
func ContainsTypeParam(t Type) bool {
	if knownContainsTypeParam(t) {
		return true
	}
	if !knownContainsRecursive(t) {
		return false
	}
	return containsTypeParamDynamic(t, nil, 0)
}

// ContainsInstantiated reports whether t is, or transitively contains, a
// generic instantiation. A cached positive is definitive. A cached negative is
// definitive only for non-recursive products; recursive placeholders can receive
// a later body, so recursive graphs fall back to a live scan.
func ContainsInstantiated(t Type) bool {
	if knownContainsInstantiated(t) {
		return true
	}
	if !knownContainsRecursive(t) {
		return false
	}
	return containsInstantiatedDynamic(t, nil, 0)
}

// ContainsGeneric reports whether t is, or transitively contains, a generic
// declaration or instantiation node.
func ContainsGeneric(t Type) bool {
	return knownContainsGeneric(t)
}

// knownContainsGeneric reports whether t transitively contains a *Generic node,
// mirroring the transitive predicate computed by containsGenericNode. A *Generic
// node trivially satisfies it, and an *Instantiated node always wraps a *Generic
// and so satisfies it too. The result is read from a construction-time cached
// flag rather than walking the type on every call.
func knownContainsGeneric(t Type) bool {
	t = unwrapAnnotatedOrNil(t)
	if t == nil {
		return false
	}
	return cachedContainsGeneric(t)
}

func cachedContainsGeneric(t Type) bool {
	switch n := t.(type) {
	case *Recursive:
		n.ensureContainsFlags()
		return n.containsGeneric
	case *Generic:
		return true
	case *Instantiated:
		return true
	case *TypeParam:
		return n.containsGeneric
	case *Optional:
		return n.containsGeneric
	case *Union:
		return n.containsGeneric
	case *Intersection:
		return n.containsGeneric
	case *Array:
		return n.containsGeneric
	case *Map:
		return n.containsGeneric
	case *ReadonlyMap:
		return n.containsGeneric
	case *Tuple:
		return n.containsGeneric
	case *Function:
		return n.containsGeneric
	case *Record:
		return n.containsGeneric
	case *Alias:
		return n.containsGeneric
	case *Meta:
		return n.containsGeneric
	case *Interface:
		return n.containsGeneric
	default:
		return false
	}
}
