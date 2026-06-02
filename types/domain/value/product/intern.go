package product

import "sync"

// interner is the canonical store of AbstractValue nodes.
//
// It maps a node's product hash to the bucket of canonical nodes sharing that
// hash, and probes the bucket with nodeEqual to find an existing canonical node.
// Equal nodes therefore collapse to one pointer (the interning identity that the
// Equal fast path and Salsa value identity rely on). Hash collisions are handled
// by the per-bucket Equal probe, so distinct-but-colliding values stay distinct.
//
// The store is process-global and append-only: canonical nodes are never mutated
// or evicted, matching the deeply-immutable value contract. A read-mostly mutex
// guards the map.
type interner struct {
	mu      sync.RWMutex
	buckets map[uint64][]*node
}

var canonical = &interner{buckets: make(map[uint64][]*node)}

// ResetCanonicalInterner clears the package-level product-value interner.
//
// Product nodes are immutable, but the canonical store owns analysis-local
// identity. Keeping nodes from an unrelated checker run lets same-shaped function
// values with different query context collapse, which is not a valid Salsa key.
func ResetCanonicalInterner() {
	canonical.mu.Lock()
	defer canonical.mu.Unlock()
	canonical.buckets = make(map[uint64][]*node)
}

// intern returns the canonical node equal to n, inserting n when none exists yet.
//
// The fast path takes the read lock and probes the bucket. On a miss it takes the
// write lock, re-probes (another goroutine may have inserted an equal node), and
// only then installs n. The returned pointer is stable for the process lifetime.
func intern(n *node) *node {
	h := nodeHash(n)

	canonical.mu.RLock()
	if existing, ok := lookup(canonical.buckets[h], n); ok {
		canonical.mu.RUnlock()
		return existing
	}
	canonical.mu.RUnlock()

	canonical.mu.Lock()
	defer canonical.mu.Unlock()
	if existing, ok := lookup(canonical.buckets[h], n); ok {
		return existing
	}
	canonical.buckets[h] = append(canonical.buckets[h], n)
	return n
}

// lookup probes a bucket for a node equal to n.
func lookup(bucket []*node, n *node) (*node, bool) {
	for _, c := range bucket {
		if nodeEqual(c, n) {
			return c, true
		}
	}
	return nil, false
}
