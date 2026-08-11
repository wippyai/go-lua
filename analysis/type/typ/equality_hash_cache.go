package typ

import "sync"

// equalityHashCache stores a traversal hash for an immutable product node.
// A product's children can contain recursive or deferred generic declarations,
// so a cached value remains usable only while those declarations have the same
// revisions that were observed during traversal.
type equalityHashCache struct {
	mu    sync.RWMutex
	value uint64
	deps  []equalityHashDependency
	valid bool
}

type equalityHashDependency struct {
	rec    *Recursive
	recRev uint64
	gen    *Generic
	genRev uint64
}

func (c *equalityHashCache) load() (uint64, bool) {
	if c == nil {
		return 0, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.valid || !equalityHashDependenciesValid(c.deps) {
		return 0, false
	}
	return c.value, true
}

func (c *equalityHashCache) store(value uint64, deps []equalityHashDependency) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.value = value
	c.deps = deps
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
	deps, ok := equalityHashDependencies(t)
	if !ok {
		return
	}
	cache.store(value, deps)
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

func equalityHashDependenciesValid(deps []equalityHashDependency) bool {
	for _, dep := range deps {
		if dep.rec != nil && dep.rec.rev != dep.recRev {
			return false
		}
		if dep.gen != nil && dep.gen.rev != dep.genRev {
			return false
		}
	}
	return true
}

// equalityHashDependencies records every mutable declaration that can change
// a traversal hash. An unresolved placeholder is intentionally not cached:
// SetBody may still supply either a recursive or generic body.
func equalityHashDependencies(t Type) ([]equalityHashDependency, bool) {
	seen := make(map[Type]bool)
	deps := make([]equalityHashDependency, 0, 2)
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
				return nil, false
			}
			deps = append(deps, equalityHashDependency{rec: node, recRev: node.rev})
		case *Generic:
			if node.Body == nil {
				return nil, false
			}
			deps = append(deps, equalityHashDependency{gen: node, genRev: node.rev})
		}
		WalkChildren(current, func(child Type) bool {
			work = append(work, child)
			return false
		})
	}
	return deps, true
}
