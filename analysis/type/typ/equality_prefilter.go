package typ

func typeEqualsCanUseHashPrefilter(a, b Type) bool {
	return !knownContainsRecursive(a) &&
		!knownContainsRecursive(b) &&
		!knownContainsOpenRecursive(a) &&
		!knownContainsOpenRecursive(b)
}
