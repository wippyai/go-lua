package typ

func knownContainsOpenRecursive(t Type) bool {
	t = unwrapAnnotatedOrNil(t)
	if t == nil {
		return false
	}
	switch n := t.(type) {
	case *Recursive:
		n.ensureContainsClosedFlag()
		return !n.containsFlagsClosed
	case *Optional:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	case *Union:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	case *Intersection:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	case *Array:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	case *Map:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	case *ReadonlyMap:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	case *Tuple:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	case *Function:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	case *Record:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	case *Alias:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	case *Meta:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	case *Generic:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	case *Instantiated:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	case *TypeParam:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	case *Interface:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	default:
		return false
	}
}

func currentContainsOpenRecursive(t Type, cached bool) bool {
	if !cached && !knownContainsRecursive(t) {
		return false
	}
	return !recursiveContainsGraphClosed(t, nil)
}
