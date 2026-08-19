package typ

import "context"

// EqualityHash returns the canonical hash used by structural equality and
// deduplication. It matches Hash for immutable products, but recomputes
// wrappers around mutable recursive/generic nodes so SetBody cannot leave
// stale construction-time hashes in the type algebra.
func EqualityHash(t Type) uint64 {
	var ok bool
	t, ok = unwrapAliasForEquals(t)
	if !ok || t == nil {
		return 0
	}
	if equalityHashNeedsRefresh(t) {
		if h, ok := cachedEqualityHash(t); ok {
			return h
		}
		scratch := getRecursiveHashScratch()
		h := hashBodyWithVisitedMemo(t, scratch)
		putRecursiveHashScratch(scratch)
		cacheEqualityHash(t, h)
		return h
	}
	return t.Hash()
}

// EqualityHashContext is EqualityHash with bounded cancellation checks during
// recursive traversal. It deliberately does not populate the equality cache:
// recording dependency revisions requires a separate full graph walk that is
// not part of this cancelable operation.
func EqualityHashContext(ctx context.Context, t Type) (uint64, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
	}
	var ok bool
	t, ok = unwrapAliasForEquals(t)
	if !ok || t == nil {
		return 0, nil
	}
	if !equalityHashNeedsRefresh(t) {
		return t.Hash(), nil
	}
	if h, ok := cachedEqualityHash(t); ok {
		return h, nil
	}
	scratch := getRecursiveHashScratch()
	scratch.ctx = ctx
	h := hashBodyWithVisitedMemo(t, scratch)
	err := scratch.err
	putRecursiveHashScratch(scratch)
	if err != nil {
		return 0, err
	}
	return h, nil
}

func equalityHashNeedsRefresh(t Type) bool {
	if knownContainsRecursive(t) || mayContainOpenRecursive(t) || knownContainsInstantiated(t) {
		return true
	}
	return knownContainsGeneric(t)
}

func (r *Recursive) Hash() uint64 {
	if r == nil {
		return 0
	}
	if memo := r.hashMemo.Load(); memo != nil {
		return memo.hash
	}
	// Compute hash on demand with cycle detection. Recursive types are mutable
	// only until SetBody completes, then share the same cached-hash contract as
	// other type nodes. Only a hash computed over a fully closed graph is
	// cached: Body is write-once, so once every reachable placeholder has a
	// body the hash is permanent.
	scratch := getRecursiveHashScratch()
	h := hashWithVisitedMemo(r, scratch)
	putRecursiveHashScratch(scratch)
	if recursiveGraphClosureForRecursive(r) {
		r.hashMemo.Store(&recursiveHashMemo{hash: h})
	}
	return h
}

type recursiveHashMemo struct {
	hash uint64
}
