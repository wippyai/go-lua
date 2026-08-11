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

// Key is an opaque Heap-scoped Factor key. Heap owns aggregate identity, so
// Footprint cannot pair a root with an unrelated schema or reconstruct one
// from Link topology.
type Key struct {
	heap heap.Schema
	kind KeyKind
	root heap.Key
	id   keyspace.ContentID
}

func AllocationKey(source heap.Schema, root heap.Key) (Key, bool) {
	if !source.Valid() || !source.OwnsKey(root) || root.Kind() != heap.RootAllocation {
		return Key{}, false
	}
	id, ok := root.ContentID()
	if !ok || !id.Available() {
		return Key{}, false
	}
	return Key{heap: source, kind: KeyAllocation, root: root, id: id}, true
}

func (k Key) Kind() KeyKind { return k.kind }

func (k Key) ContentID() keyspace.ContentID {
	if !k.valid() {
		return keyspace.ContentID{}
	}
	return k.id
}

func (k Key) HeapKey() (heap.Key, bool) {
	return k.root, k.valid() && k.kind == KeyAllocation
}

func (k Key) valid() bool {
	return k.heap.Valid() && k.kind == KeyAllocation && k.id.Available() && k.heap.OwnsKey(k.root) && k.root.Kind() == heap.RootAllocation
}

func (k Key) validFor(source heap.Schema) bool {
	if !k.valid() || !source.Valid() || k.heap != source {
		return false
	}
	id, ok := k.root.ContentID()
	return ok && id == k.id
}
