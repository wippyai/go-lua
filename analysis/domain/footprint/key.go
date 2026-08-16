// Package footprint owns symbolic object-graph and size relations. It contains
// no heap-region, COW, GC, or placement-policy decision.
package footprint

import (
	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/program/keyspace"
)

// KeyKind is the closed family of structural observations indexed by the
// Footprint factor.
type KeyKind uint8

const (
	KeyInvalid KeyKind = iota
	KeyAllocation
)

// Key is an opaque Footprint-scoped Factor key. It is a dense capability into
// the immutable universe; the selected Heap root remains owned by that
// universe rather than being copied into every key.
type Key struct {
	universe *universe
	slot     uint32
}

func (k Key) Kind() KeyKind {
	if !k.valid() {
		return KeyInvalid
	}
	return KeyAllocation
}

func (k Key) ContentID() keyspace.ContentID {
	root, ok := k.HeapKey()
	if !ok {
		return keyspace.ContentID{}
	}
	id, ok := root.ContentID()
	if !ok {
		return keyspace.ContentID{}
	}
	return id
}

func (k Key) HeapKey() (heap.Key, bool) {
	if !k.valid() {
		return heap.Key{}, false
	}
	return k.universe.rootAt(k.slot)
}

func (k Key) valid() bool {
	return k.universe != nil && k.slot != 0 && uint64(k.slot) <= uint64(len(k.universe.roots))
}
