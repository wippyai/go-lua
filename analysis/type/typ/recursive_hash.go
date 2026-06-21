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
		scratch := getRecursiveHashScratch()
		h := hashBodyWithVisitedMemo(t, scratch)
		putRecursiveHashScratch(scratch)
		return h
	}
	return t.Hash()
}

func equalityHashNeedsRefresh(t Type) bool {
	if knownContainsRecursive(t) || knownContainsOpenRecursive(t) || knownContainsInstantiated(t) {
		return true
	}
	return knownContainsGeneric(t)
}

func (r *Recursive) Hash() uint64 {
	if r.hash != 0 && recursiveHashDepsValid(r.hashDeps) {
		return r.hash
	}
	// Compute hash on demand with cycle detection. Recursive types are mutable
	// only until SetBody completes, then share the same cached-hash contract as
	// other type nodes.
	scratch := getRecursiveHashScratch()
	h := hashWithVisitedMemo(r, scratch)
	putRecursiveHashScratch(scratch)
	if deps, ok := recursiveHashDeps(r); ok {
		r.hash = h
		r.hashDeps = deps
	}
	return h
}
