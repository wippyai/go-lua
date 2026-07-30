package typ

type containmentFlag uint8

const (
	containmentAny containmentFlag = iota + 1
	containmentNever
	containmentTypeParam
	containmentInstantiated
	containmentGeneric
)

func containsAnyDynamic(t Type, seen map[Type]bool, _ int) bool {
	return containsDynamicFlag(t, seen, containmentAny)
}

func containsNeverDynamic(t Type, seen map[Type]bool) bool {
	return containsDynamicFlag(t, seen, containmentNever)
}

func containsTypeParamDynamic(t Type, seen map[Type]bool, _ int) bool {
	return containsDynamicFlag(t, seen, containmentTypeParam)
}

func containsInstantiatedDynamic(t Type, seen map[Type]bool, _ int) bool {
	return containsDynamicFlag(t, seen, containmentInstantiated)
}

func containsDynamicFlag(
	t Type,
	seen map[Type]bool,
	flag containmentFlag,
) bool {
	if t == nil || flag == 0 {
		return false
	}
	t = unwrapAnnotated(t)
	if t == nil {
		return false
	}
	// Traverse nested recursive declarations in the current graph walk instead
	// of starting another cached derivation with a fresh cycle guard. This is
	// both complete for mutually recursive graphs and safe while the root memo
	// is absent.
	if recursive, ok := t.(*Recursive); ok {
		if seen == nil {
			seen = make(map[Type]bool)
		}
		if seen[t] {
			return false
		}
		seen[t] = true
		return containsDynamicFlag(recursive.Body, seen, flag)
	}
	// A node can intrinsically satisfy the query while also containing a
	// recursive back-edge. Preserve that local truth before bypassing stale
	// transitive construction flags. For example, an Instantiated<Generic>
	// remains both instantiated and generic even when Generic.Body reaches the
	// enclosing Recursive.
	if flag.direct(t) {
		return true
	}
	// Construction-time product flags may include an earlier revision of a
	// recursive child. For a recursive-containing product, walk its children so
	// freshness is governed by the recursive generation fence rather than by a
	// stale conservative positive. Non-recursive products keep the O(1) cache.
	if !knownContainsRecursive(t) {
		return flag.known(t)
	}
	if seen == nil {
		seen = make(map[Type]bool)
	}
	if seen[t] {
		return false
	}
	seen[t] = true

	next := func(child Type) bool {
		return containsDynamicFlag(child, seen, flag)
	}

	return WalkChildren(t, next)
}

func (f containmentFlag) direct(t Type) bool {
	switch f {
	case containmentAny:
		return IsAny(t)
	case containmentNever:
		return IsNever(t)
	case containmentTypeParam:
		_, ok := t.(*TypeParam)
		return ok
	case containmentInstantiated:
		_, ok := t.(*Instantiated)
		return ok
	case containmentGeneric:
		switch t.(type) {
		case *Generic, *Instantiated:
			return true
		}
	}
	return false
}

func (f containmentFlag) known(t Type) bool {
	switch f {
	case containmentAny:
		return knownContainsAny(t)
	case containmentNever:
		return knownContainsNever(t)
	case containmentTypeParam:
		return knownContainsTypeParam(t)
	case containmentInstantiated:
		return knownContainsInstantiated(t)
	case containmentGeneric:
		return knownContainsGeneric(t)
	default:
		return false
	}
}
