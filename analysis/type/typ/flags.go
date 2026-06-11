package typ

func containsRecursiveType(t Type) bool {
	return knownContainsRecursive(t)
}
