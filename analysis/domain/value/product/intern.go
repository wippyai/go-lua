package product

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
)

var globalInterner = &interner{nodes: make(map[*axis.Registry]map[uint64][]*node)}

type interner struct {
	mu    sync.Mutex
	nodes map[*axis.Registry]map[uint64][]*node
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

	h := rt.stableHash(shape, p, slots)
	globalInterner.mu.Lock()
	defer globalInterner.mu.Unlock()

	bucket := globalInterner.nodes[rt.reg]
	if bucket == nil {
		bucket = make(map[uint64][]*node)
		globalInterner.nodes[rt.reg] = bucket
	}

	for _, existing := range bucket[h] {
		if rt.sameNode(existing, shape, p, slots) {
			return Value{n: existing}
		}
	}

	stored := make([]slot, len(slots))
	copy(stored, slots)
	n := &node{reg: rt.reg, shape: shape, presence: p, slots: stored, hash: h}
	bucket[h] = append(bucket[h], n)
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
