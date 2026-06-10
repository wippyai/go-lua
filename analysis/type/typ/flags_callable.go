package typ

// HasCallableSurface reports whether t is, or is transparently wrapped around,
// a function at the callable surface. It deliberately does not descend through
// data containers such as records, arrays, maps, or tuples.
func HasCallableSurface(t Type) bool {
	if result, ok := hasCallableSurfaceFast(t); ok {
		return result
	}
	return hasCallableSurface(t, make(map[Type]bool))
}

// RecordHasCallableSurface reports whether a record exposes a callable value
// directly through a field, metatable, or map value. It does not descend
// through data containers.
func RecordHasCallableSurface(r *Record) bool {
	return r != nil && r.containsCallableSurf
}

func hasCallableSurfaceFast(t Type) (bool, bool) {
	t = UnwrapAnnotated(t)
	if t == nil {
		return false, true
	}
	switch n := t.(type) {
	case *Function:
		return true, true
	case *Optional:
		return hasCallableSurfaceFast(n.Inner)
	case *Union:
		if n.memberHashes != nil || n.hash != 0 {
			return n.containsCallableSurf, true
		}
		return false, false
	case *Alias, *Intersection:
		return false, false
	default:
		return false, true
	}
}

func hasCallableSurface(t Type, seen map[Type]bool) bool {
	if t == nil {
		return false
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return false
	}
	if seen[t] {
		return false
	}
	seen[t] = true
	switch n := t.(type) {
	case *Function:
		return true
	case *Optional:
		return hasCallableSurface(n.Inner, seen)
	case *Union:
		if n.memberHashes != nil || n.hash != 0 {
			return n.containsCallableSurf
		}
		for _, member := range n.Members {
			if hasCallableSurface(member, seen) {
				return true
			}
		}
		return false
	case *Intersection:
		for _, member := range n.Members {
			if hasCallableSurface(member, seen) {
				return true
			}
		}
		return false
	case *Alias:
		return hasCallableSurface(n.Target, seen)
	default:
		return false
	}
}
