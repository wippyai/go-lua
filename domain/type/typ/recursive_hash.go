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
		interior := !scratch.sawCycle
		putRecursiveHashScratch(scratch)
		cacheEqualityHash(t, h, interior)
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

// closeGatedHash computes t's structural hash through the equality-hash
// cache unconditionally, without EqualityHash's equalityHashNeedsRefresh
// branch. It is the shared Hash() body for every node kind whose own
// equalityHashCache can predate a reachable Generic/Recursive completing:
// Generic and Instantiated always need a refresh (equalityHashNeedsRefresh is
// unconditionally true for them), but Record and Function do not when they
// contain no Generic/Recursive at all, and EqualityHash's fallback for that
// case is t.Hash() itself - the exact call this function is the body of.
// Delegating Hash() straight to EqualityHash would recurse forever on that
// path, so those four Hash() methods call this instead.
func closeGatedHash(t Type) uint64 {
	if h, ok := cachedEqualityHash(t); ok {
		return h
	}
	scratch := getRecursiveHashScratch()
	h := hashBodyWithVisitedMemo(t, scratch)
	interior := !scratch.sawCycle
	putRecursiveHashScratch(scratch)
	cacheEqualityHash(t, h, interior)
	return h
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
	if columnsOf(r).closed {
		r.hashMemo.Store(&recursiveHashMemo{hash: h})
	}
	return h
}

type recursiveHashMemo struct {
	hash uint64
}
