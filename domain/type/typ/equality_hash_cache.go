package typ

import "sync"

// equalityHashCache stores a traversal hash for an immutable product node.
// A product's children can contain recursive or deferred generic declarations,
// so a cached value is only published once every such declaration reachable
// from the node already has a body. Body is write-once, so a published value
// is then permanent.
//
// interior additionally records whether the value is safe to substitute for
// this node when it is reached as an INTERIOR node of a different traversal
// (recursive_hash_traversal.go's enterBody mid-walk shortcut), as opposed to
// only when it is asked about directly as EqualityHash/Hash's own query root.
// Function/Record/Generic/Instantiated use the same activeContains-style
// cycle detection as every ordinary composite node: the value a node
// contributes depends on whether it, or an ancestor it cycles back to, was
// "already active" at the moment it was entered - which is a property of
// where the walk started, not of the node itself, whenever the node's own
// computation ever crossed a productive cycle (hashMachine.sawCycle). A node
// whose computation never saw one has no such dependency and its value is the
// same from every position, so it is safe to reuse; one that did is only
// trustworthy as its own query root.
type equalityHashCache struct {
	mu       sync.RWMutex
	value    uint64
	valid    bool
	interior bool
}

func (c *equalityHashCache) load() (uint64, bool) {
	if c == nil {
		return 0, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.valid {
		return 0, false
	}
	return c.value, true
}

// loadInterior is load, gated additionally on interior-reuse safety. See the
// equalityHashCache.interior field comment.
func (c *equalityHashCache) loadInterior() (uint64, bool) {
	if c == nil {
		return 0, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.valid || !c.interior {
		return 0, false
	}
	return c.value, true
}

func (c *equalityHashCache) store(value uint64, interior bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.value = value
	c.valid = true
	c.interior = interior
	c.mu.Unlock()
}

func cachedEqualityHash(t Type) (uint64, bool) {
	return equalityHashCacheFor(t).load()
}

// cachedEqualityHashInterior is cachedEqualityHash gated on interior-reuse
// safety; only recursive_hash_traversal.go's mid-walk shortcut should call it.
func cachedEqualityHashInterior(t Type) (uint64, bool) {
	return equalityHashCacheFor(t).loadInterior()
}

func cacheEqualityHash(t Type, value uint64, interior bool) {
	cache := equalityHashCacheFor(t)
	if cache == nil {
		return
	}
	columns := columnsOf(t)
	if !columns.closed || !columns.declarationsClosed {
		return
	}
	cache.store(value, interior)
}

func equalityHashCacheFor(t Type) *equalityHashCache {
	switch n := t.(type) {
	case *Function:
		return n.equalityHashCache
	case *Record:
		return n.equalityHashCache
	case *Instantiated:
		return n.equalityHashCache
	case *Generic:
		return n.equalityHashCache
	default:
		return nil
	}
}
