package product

import (
	"sort"
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
	requireRegistry(reg)
	shape = normalizeShape(shape)
	p = normalizePresence(p)
	slots = canonicalSlots(reg, slots)
	shape, p, slots = reduce(reg, shape, p, slots)
	slots = canonicalSlots(reg, slots)
	if shape == ShapeTop && presence.Equal(p, presence.Top()) && len(slots) == 0 {
		return Top()
	}

	h := stableHash(reg, shape, p, slots)
	globalInterner.mu.Lock()
	defer globalInterner.mu.Unlock()

	bucket := globalInterner.nodes[reg]
	if bucket == nil {
		bucket = make(map[uint64][]*node)
		globalInterner.nodes[reg] = bucket
	}

	for _, existing := range bucket[h] {
		if sameNode(reg, existing, shape, p, slots) {
			return Value{n: existing}
		}
	}

	stored := make([]slot, len(slots))
	copy(stored, slots)
	n := &node{reg: reg, shape: shape, presence: p, slots: stored, hash: h}
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

func canonicalSlots(reg *axis.Registry, slots []slot) []slot {
	if len(slots) == 0 {
		return nil
	}
	byKey := make(map[string]any, len(slots))
	for _, slot := range slots {
		if slot.key == presence.Key.ID() {
			panic("product: presence is a core lane, not a sparse axis")
		}
		spec, ok := reg.LookupErased(slot.key)
		if !ok {
			panic("product: unregistered axis slot " + slot.key)
		}
		if spec.IsTopAny(slot.value) {
			delete(byKey, slot.key)
			continue
		}
		byKey[slot.key] = slot.value
	}
	if len(byKey) == 0 {
		return nil
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]slot, 0, len(keys))
	for _, key := range keys {
		out = append(out, slot{key: key, value: byKey[key]})
	}
	return out
}

func sameNode(reg *axis.Registry, n *node, shape Shape, p presence.Value, slots []slot) bool {
	if n.reg != reg || n.shape != shape || !presence.Equal(n.presence, p) || len(n.slots) != len(slots) {
		return false
	}
	for i, left := range n.slots {
		right := slots[i]
		if left.key != right.key {
			return false
		}
		spec, ok := reg.LookupErased(left.key)
		if !ok || !spec.EqualAny(left.value, right.value) {
			return false
		}
	}
	return true
}

func Hash(reg *axis.Registry, v Value) uint64 {
	requireRegistry(reg)
	if v.n == nil {
		return stableHash(reg, ShapeTop, presence.Top(), nil)
	}
	validateValue(reg, v)
	return stableHash(reg, v.n.shape, v.n.presence, v.n.slots)
}

func stableHash(reg *axis.Registry, shape Shape, p presence.Value, slots []slot) uint64 {
	h := internal.FnvString("value.product")
	h = internal.MixHash(h, uint64(shape)+1)
	h = internal.MixHash(h, presence.Value.Hash(p))
	for _, slot := range slots {
		spec, ok := reg.LookupErased(slot.key)
		if !ok {
			panic("product: unregistered axis slot " + slot.key)
		}
		h = internal.MixHash(h, internal.FnvString(slot.key))
		h = internal.MixHash(h, spec.HashAny(slot.value))
	}
	return h
}
