package product

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
)

// The interner is a bounded owner-local hint, rather than an owner of every
// product value ever constructed. Values and summaries own their immutable
// nodes directly, and product equality is structural, so evicting a canonical
// node can only lose a pointer-identity fast path. Each frozen product runtime
// owns its interner; no process-global table retains registries or values.
const (
	internerShards       = 64
	internerShardMaxNode = 256
)

type interner struct {
	shards [internerShards]internerShard
}

type internerShard struct {
	mu    sync.Mutex
	nodes map[uint64][]*node
	fifo  []internerEntry
}

type internerEntry struct {
	hash uint64
	node *node
}

func newInterner() *interner {
	return &interner{}
}

func (i *interner) shardFor(hash uint64) *internerShard {
	return &i.shards[hash&(internerShards-1)]
}

func (s *internerShard) evictOldest() {
	if len(s.fifo) == 0 {
		return
	}
	evicted := s.fifo[0]
	copy(s.fifo, s.fifo[1:])
	s.fifo[len(s.fifo)-1] = internerEntry{}
	s.fifo = s.fifo[:len(s.fifo)-1]
	entries := s.nodes[evicted.hash]
	for index, existing := range entries {
		if existing != evicted.node {
			continue
		}
		copy(entries[index:], entries[index+1:])
		entries[len(entries)-1] = nil
		entries = entries[:len(entries)-1]
		if len(entries) == 0 {
			delete(s.nodes, evicted.hash)
		} else {
			s.nodes[evicted.hash] = entries
		}
		break
	}
}

func intern(reg *axis.Registry, shape Shape, p presence.Value, slots []slot) Value {
	rt := mustRuntime(reg)
	return internRuntime(rt, shape, p, slots)
}

func internRuntime(rt *registryRuntime, shape Shape, p presence.Value, slots []slot) Value {
	shape = normalizeShape(shape)
	p = normalizePresence(p)
	slots = rt.canonicalSlots(slots)
	shape, p, slots = reduce(rt, shape, p, slots)
	slots = rt.canonicalSlots(slots)
	if shape == ShapeTop && presence.Equal(p, presence.Top()) && len(slots) == 0 {
		return Top()
	}

	return internCanonicalNoReducer(rt, shape, p, slots)
}

// internCanonicalNoReducer probes a known-canonical, reducer-free candidate.
// Keeping this path separate is important: reduceEditor's erased Writer makes
// its slice parameter escape even when no reducer can run. The schema-owned
// constructor uses this function for its stack candidate.
func internCanonicalNoReducer(rt *registryRuntime, shape Shape, p presence.Value, slots []slot) Value {
	shape = normalizeShape(shape)
	p = normalizePresence(p)
	shape, p, _ = reducePresenceShape(shape, p)
	if rt.isProductBottom(p, slots) {
		shape, p, slots = ShapeBottom, presence.Bottom(), rt.bottomSlots
	}
	return internCanonicalNoBottom(rt, shape, p, slots)
}

// internCanonicalNoBottom is the allocation-sensitive half of interning. Its
// caller has already established that the candidate cannot be product bottom.
// Keeping that proof outside the probe lets a stack candidate remain on stack.
func internCanonicalNoBottom(rt *registryRuntime, shape Shape, p presence.Value, slots []slot) Value {
	shape = normalizeShape(shape)
	p = normalizePresence(p)
	shape, p, _ = reducePresenceShape(shape, p)
	if shape == ShapeTop && presence.Equal(p, presence.Top()) && len(slots) == 0 {
		return Top()
	}

	h := rt.stableHash(shape, p, slots)
	shard := rt.interner.shardFor(h)
	shard.mu.Lock()

	bucket := shard.nodes[h]
	if shard.nodes == nil {
		shard.nodes = make(map[uint64][]*node)
	}

	for _, existing := range bucket {
		if rt.sameNode(existing, shape, p, slots) {
			shard.mu.Unlock()
			return Value{n: existing}
		}
	}

	stored := make([]slot, len(slots))
	copy(stored, slots)
	n := &node{reg: rt.reg, shape: shape, presence: p, slots: stored, hash: h}
	bucket = append(bucket, n)
	shard.nodes[h] = bucket
	shard.fifo = append(shard.fifo, internerEntry{hash: h, node: n})
	if len(shard.fifo) > internerShardMaxNode {
		shard.evictOldest()
	}
	shard.mu.Unlock()
	return Value{n: n}
}

func normalizeShape(shape Shape) Shape {
	switch shape {
	case ShapeBottom, ShapeTop:
		return shape
	default:
		panic("product: invalid shape")
	}
}

func normalizePresence(p presence.Value) presence.Value {
	switch {
	case presence.Equal(p, presence.Bottom()):
		return presence.Bottom()
	case presence.Equal(p, presence.Present()):
		return presence.Present()
	case presence.Equal(p, presence.Absent()):
		return presence.Absent()
	case presence.Equal(p, presence.Top()):
		return presence.Top()
	default:
		panic("product: invalid presence")
	}
}

func Hash(reg *axis.Registry, v Value) uint64 {
	rt := mustRuntime(reg)
	rt.validateValue(v)
	return CanonicalHash(v)
}

// CanonicalHash returns the stable semantic hash carried by v. It is safe for
// consumers which need to order an already-validated value but do not own the
// registry required by Hash. Product operations should continue to use Hash so
// they validate that values belong to their registry.
func CanonicalHash(v Value) uint64 {
	if v.n != nil {
		return v.n.hash
	}
	return topHash()
}

func topHash() uint64 {
	h := internal.FnvString("value.product")
	h = internal.MixHash(h, uint64(ShapeTop)+1)
	return internal.MixHash(h, presence.Value.Hash(presence.Top()))
}
