package typ

// EqualityHash returns the canonical hash used by structural equality and
// deduplication. It matches Hash for immutable products, but recomputes
// wrappers around mutable recursive/generic nodes so SetBody cannot leave
// stale construction-time hashes in the type algebra.
func EqualityHash(t Type) uint64 {
	t = unwrapAliasForEquals(t, NewGuard())
	if t == nil {
		return 0
	}
	if equalityHashNeedsRefresh(t) {
		var scratch recursiveHashScratch
		return hashBodyWithVisitedMemo(t, &scratch)
	}
	return t.Hash()
}

func equalityHashNeedsRefresh(t Type) bool {
	if knownContainsRecursive(t) || knownContainsOpenRecursive(t) || knownContainsInstantiated(t) {
		return true
	}
	return containsGenericNode(t, nil)
}

func containsGenericNode(t Type, seen map[uintptr]bool) bool {
	t = unwrapAnnotatedOrNil(t)
	if t == nil {
		return false
	}
	if _, ok := t.(*Generic); ok {
		return true
	}
	ptr := typePointer(t)
	if ptr != 0 {
		if seen == nil {
			seen = make(map[uintptr]bool)
		}
		if seen[ptr] {
			return false
		}
		seen[ptr] = true
	}
	return walkChildren(t, func(child Type) bool {
		return containsGenericNode(child, seen)
	})
}

func (r *Recursive) Hash() uint64 {
	if r.hash != 0 && recursiveHashDepsValid(r.hashDeps) {
		return r.hash
	}
	// Compute hash on demand with cycle detection. Recursive types are mutable
	// only until SetBody completes, then share the same cached-hash contract as
	// other type nodes.
	var scratch recursiveHashScratch
	h := hashWithVisitedMemo(r, &scratch)
	if deps, ok := recursiveHashDeps(r); ok {
		r.hash = h
		r.hashDeps = deps
	}
	return h
}
