package typ

// ContainsAny reports whether t is, or may contain through an open recursive
// body, the explicit dynamic any marker.
func ContainsAny(t Type) bool {
	return containsAnyDynamic(t, nil, 0)
}

// ContainsNever reports whether t is, or may contain through an open recursive
// body, the bottom type marker.
func ContainsNever(t Type) bool {
	return containsNeverDynamic(t, nil)
}

// ContainsTypeParam reports whether t is, or may contain through an open
// recursive body, an unbound type parameter.
func ContainsTypeParam(t Type) bool {
	return containsTypeParamDynamic(t, nil, 0)
}

// ContainsInstantiated reports whether t is, or may contain through an open
// recursive body, a generic instantiation.
func ContainsInstantiated(t Type) bool {
	return containsInstantiatedDynamic(t, nil, 0)
}

// ContainsRecursive reports whether t contains a recursive product marker.
func ContainsRecursive(t Type) bool {
	return knownContainsRecursive(t)
}
