package typ

// EqualityHash returns the canonical hash used by structural equality and
// deduplication. It matches Hash for immutable closed products, but recomputes
// wrappers around open recursive placeholders so SetBody cannot leave stale
// construction-time hashes in the type algebra.
func EqualityHash(t Type) uint64 {
	t = unwrapAliasForEquals(t, NewGuard())
	if t == nil {
		return 0
	}
	if knownContainsOpenRecursive(t) {
		var scratch recursiveHashScratch
		return hashBodyWithVisitedMemo(t, &scratch)
	}
	return t.Hash()
}

func typeEqualityHash(t Type) uint64 {
	return EqualityHash(t)
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
