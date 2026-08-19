package typ

import "sync"

// equalityHashCache stores a traversal hash for an immutable product node.
// A product's children can contain recursive or deferred generic declarations,
// so a cached value is only published once every such declaration reachable
// from the node already has a body. Body is write-once, so a published value
// is then permanent.
type equalityHashCache struct {
	mu    sync.RWMutex
	value uint64
	valid bool
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

func (c *equalityHashCache) store(value uint64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.value = value
	c.valid = true
	c.mu.Unlock()
}

func cachedEqualityHash(t Type) (uint64, bool) {
	return equalityHashCacheFor(t).load()
}

func cacheEqualityHash(t Type, value uint64) {
	cache := equalityHashCacheFor(t)
	if cache == nil {
		return
	}
	if !equalityHashGraphClosed(t) {
		return
	}
	cache.store(value)
}

func equalityHashCacheFor(t Type) *equalityHashCache {
	switch n := t.(type) {
	case *Function:
		return n.equalityHashCache
	case *Record:
		return n.equalityHashCache
	case *Instantiated:
		return n.equalityHashCache
	default:
		return nil
	}
}

// equalityHashGraphClosed reports whether every recursive or generic
// declaration reachable from t already has a body. An unresolved placeholder
// is intentionally not cached: SetBody may still supply either body.
func equalityHashGraphClosed(t Type) bool {
	seen := make(map[Type]bool)
	work := []Type{t}
	for len(work) != 0 {
		last := len(work) - 1
		current := unwrapAnnotated(work[last])
		work = work[:last]
		if current == nil || seen[current] {
			continue
		}
		seen[current] = true

		switch node := current.(type) {
		case *Recursive:
			if node.Body == nil {
				return false
			}
		case *Generic:
			if node.Body == nil {
				return false
			}
		}
		WalkChildren(current, func(child Type) bool {
			work = append(work, child)
			return false
		})
	}
	return true
}
