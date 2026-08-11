// Package typevalue owns the finite runtime LType relation. Its State values
// contain only authority-local handles; Program, Link, and typeauthority
// remain the sole structural authorities from which those handles are sealed.
package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/program/keyspace"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
)

type rootKind uint8

const (
	rootRuntime rootKind = iota + 1
	rootFresh
)

type rootRow struct {
	kind  rootKind
	value linkboundary.Value
	fresh heap.Key
}

// Root is one existing Boundary Value or one canonical fresh Heap Key.
// It is the TypeValue Factor coordinate; zero is a valid first coordinate.
type Root struct {
	owner *Authority
	index uint32
}

func (a *Authority) sealRoots() bool {
	if a == nil || a.source == nil || uint64(a.values.Count()) > uint64(^uint32(0)) {
		return false
	}
	seedKeys := make(map[linkboundary.Value]keyspace.ContentID)
	if !a.forEachStaticSeed(func(value linkboundary.Value, _ string, root keyspace.ContentID, _ typeauthority.RuntimeInner, _ bool) bool {
		if !root.Available() {
			return false
		}
		seedKeys[value] = root
		return true
	}) {
		return false
	}
	a.runtimeRoots = make(map[linkboundary.Value]uint32, a.values.Count())
	a.allocationRoots = make(map[heap.Key]uint32, a.heap.KeyCount())
	representatives := make(map[keyspace.ContentID]uint32, len(seedKeys))
	for index := 0; index < a.values.Count(); index++ {
		value, ok := a.values.At(index)
		if !ok {
			return false
		}
		if _, duplicate := a.runtimeRoots[value]; duplicate {
			return false
		}
		if key, seeded := seedKeys[value]; seeded {
			if representative, found := representatives[key]; found {
				a.runtimeRoots[value] = representative
				continue
			}
			representatives[key] = uint32(len(a.roots))
		}
		a.runtimeRoots[value] = uint32(len(a.roots))
		a.roots = append(a.roots, rootRow{kind: rootRuntime, value: value})
	}
	for index := 0; index < a.heap.KeyCount(); index++ {
		allocation, ok := a.heap.KeyAt(index)
		if !ok {
			return false
		}
		if allocation.Kind() != heap.RootAllocation {
			continue
		}
		if !a.heap.OwnsKey(allocation) {
			return false
		}
		if _, duplicate := a.allocationRoots[allocation]; duplicate {
			return false
		}
		if shard, term, _, programRoot := allocation.ProgramAllocation(); programRoot {
			value, valueOK := a.values.Of(shard, term)
			rootIndex, admitted := a.runtimeRoots[value]
			if !valueOK || !admitted {
				return false
			}
			a.allocationRoots[allocation] = rootIndex
			continue
		}
		// Heap's sealed allocation family has exactly two cases: Program
		// aggregate or target-fresh aggregate.  The latter remains opaque here;
		// its creation relation stays Heap/Call-owned rather than being decoded
		// into a second TypeValue identity.
		if uint64(len(a.roots)) > uint64(^uint32(0)) {
			return false
		}
		rootIndex := uint32(len(a.roots))
		a.roots = append(a.roots, rootRow{kind: rootFresh, fresh: allocation})
		a.allocationRoots[allocation] = rootIndex
	}
	return true
}

func (a *Authority) RootCount() int {
	if a == nil {
		return 0
	}
	return len(a.roots)
}

func (a *Authority) RootAt(index int) (Root, bool) {
	if a == nil || index < 0 || index >= len(a.roots) {
		return Root{}, false
	}
	return Root{owner: a, index: uint32(index)}, true
}

func (a *Authority) RootIndex(root Root) (uint32, bool) {
	if !a.ownsRoot(root) {
		return 0, false
	}
	return root.index, true
}

func (a *Authority) RootForValue(value linkboundary.Value) (Root, bool) {
	if a == nil || a.source == nil {
		return Root{}, false
	}
	if _, _, ok := a.values.Origin(value); !ok {
		return Root{}, false
	}
	index, ok := a.runtimeRoots[value]
	return Root{owner: a, index: index}, ok
}

func (a *Authority) RootForHeapKey(key heap.Key) (Root, bool) {
	if a == nil || !a.heap.OwnsKey(key) || key.Kind() != heap.RootAllocation {
		return Root{}, false
	}
	index, ok := a.allocationRoots[key]
	return Root{owner: a, index: index}, ok
}

func (a *Authority) RootValue(root Root) (linkboundary.Value, bool) {
	row, ok := a.root(root)
	if !ok || row.kind != rootRuntime {
		return linkboundary.Value{}, false
	}
	return row.value, true
}

func (a *Authority) FreshRoot(root Root) (heap.Key, bool) {
	row, ok := a.root(root)
	if !ok || row.kind != rootFresh {
		return heap.Key{}, false
	}
	return row.fresh, true
}

func (a *Authority) root(root Root) (rootRow, bool) {
	if !a.ownsRoot(root) {
		return rootRow{}, false
	}
	return a.roots[root.index], true
}

func (a *Authority) ownsRoot(root Root) bool {
	return a != nil && root.owner == a && uint64(root.index) < uint64(len(a.roots))
}
