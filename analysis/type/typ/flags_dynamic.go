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
	if known(t) {
		return true
	}
	if _, ok := t.(*Recursive); ok {
		return false
	}
	if !knownContainsOpenRecursive(t) {
		return false
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

	if r, ok := t.(*Recursive); ok {
		return next(r.Body)
	}
	return walkChildren(t, next)
}
