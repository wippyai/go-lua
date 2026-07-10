package typ

func knownContainsOpenRecursive(t Type) bool {
	t = unwrapAnnotatedOrNil(t)
	if t == nil {
		return false
	}
	if n, ok := t.(*Recursive); ok {
		n.ensureContainsClosedFlag()
		return !n.containsFlagsClosed
	}
	if !knownContainsRecursive(t) {
		return false
	}
	p := openRecursiveProperties(t)
	if p == nil {
		return false
	}
	if p.containsOpenRecursiveComputed && recursiveHashDepsValid(p.containsOpenRecursiveDeps) {
		return p.containsOpenRecursive
	}
	closed, deps := recursiveGraphClosureForType(t)
	p.containsOpenRecursive = !closed
	p.containsOpenRecursiveDeps = deps
	p.containsOpenRecursiveComputed = true
	return p.containsOpenRecursive
}

// openRecursiveProperties returns the mutable memo stored by each product node.
// The node structure itself is immutable once built; only this derived cache is
// refreshed when a recursive placeholder revision changes.
func openRecursiveProperties(t Type) *typeProperties {
	switch n := t.(type) {
	case *Optional:
		return &n.typeProperties
	case *Union:
		return &n.typeProperties
	case *Intersection:
		return &n.typeProperties
	case *Array:
		return &n.typeProperties
	case *Map:
		return &n.typeProperties
	case *ReadonlyMap:
		return &n.typeProperties
	case *Tuple:
		return &n.typeProperties
	case *Function:
		return &n.typeProperties
	case *Record:
		return &n.typeProperties
	case *Alias:
		return &n.typeProperties
	case *Meta:
		return &n.typeProperties
	case *Generic:
		return &n.typeProperties
	case *Instantiated:
		return &n.typeProperties
	case *TypeParam:
		return &n.typeProperties
	case *Interface:
		return &n.typeProperties
	default:
		return nil
	}
}
