package typ

// containsAnyType is the canonical exact dynamic-any predicate for the type
// algebra. Immutable non-recursive types answer from construction-time flags;
// recursive graphs are traversed with cycle protection because placeholder
// bodies can be assigned after wrapper nodes have been constructed.
func containsAnyType(t Type) bool {
	if t == nil {
		return false
	}
	if _, ok := UnwrapAnnotated(t).(*Recursive); ok {
		return knownContainsAny(t)
	}
	if !knownContainsOpenRecursive(t) {
		return knownContainsAny(t)
	}
	return containsAnyDynamic(t, make(map[Type]bool), 0)
}

func containsNeverType(t Type) bool {
	if t == nil {
		return false
	}
	if _, ok := UnwrapAnnotated(t).(*Recursive); ok {
		return knownContainsNever(t)
	}
	if !knownContainsOpenRecursive(t) {
		return knownContainsNever(t)
	}
	return containsNeverDynamic(t, make(map[Type]bool))
}

func containsTypeParamType(t Type) bool {
	if t == nil {
		return false
	}
	if _, ok := UnwrapAnnotated(t).(*Recursive); ok {
		return knownContainsTypeParam(t)
	}
	if !knownContainsOpenRecursive(t) {
		return knownContainsTypeParam(t)
	}
	return containsTypeParamDynamic(t, make(map[Type]bool), 0)
}

func containsInstantiatedType(t Type) bool {
	if t == nil {
		return false
	}
	if _, ok := UnwrapAnnotated(t).(*Recursive); ok {
		return knownContainsInstantiated(t)
	}
	if !knownContainsOpenRecursive(t) {
		return knownContainsInstantiated(t)
	}
	return containsInstantiatedDynamic(t, make(map[Type]bool), 0)
}
