package typ

func containsAnyDynamic(t Type, seen map[Type]bool, depth int) bool {
	return containsDynamicFlag(t, seen, depth, DefaultRecursionDepth, knownContainsAny)
}

func containsNeverDynamic(t Type, seen map[Type]bool) bool {
	return containsDynamicFlag(t, seen, 0, -1, knownContainsNever)
}

func containsTypeParamDynamic(t Type, seen map[Type]bool, depth int) bool {
	return containsDynamicFlag(t, seen, depth, DefaultRecursionDepth, knownContainsTypeParam)
}

func containsInstantiatedDynamic(t Type, seen map[Type]bool, depth int) bool {
	return containsDynamicFlag(t, seen, depth, DefaultRecursionDepth, knownContainsInstantiated)
}

func containsGenericDynamic(t Type, seen map[Type]bool, depth int) bool {
	return containsDynamicFlag(t, seen, depth, DefaultRecursionDepth, knownContainsGeneric)
}

func containsDynamicFlag(
	t Type,
	seen map[Type]bool,
	depth int,
	maxDepth int,
	known func(Type) bool,
) bool {
	if t == nil || known == nil || (maxDepth >= 0 && depth > maxDepth) {
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
		return containsDynamicFlag(recursive.Body, seen, depth+1, maxDepth, known)
	}
	// Construction-time product flags may include an earlier revision of a
	// recursive child. For a recursive-containing product, walk its children so
	// freshness is governed by the recursive generation fence rather than by a
	// stale conservative positive. Non-recursive products keep the O(1) cache.
	if !knownContainsRecursive(t) {
		return known(t)
	}
	if seen == nil {
		seen = make(map[Type]bool)
	}
	if seen[t] {
		return false
	}
	seen[t] = true

	next := func(child Type) bool {
		return containsDynamicFlag(child, seen, depth+1, maxDepth, known)
	}

	return WalkChildren(t, next)
}
