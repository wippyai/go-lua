// Package typevalue owns the finite runtime LType relation. Its State values
// contain only authority-local handles; Program, Link, and typeauthority
// remain the sole structural authorities from which those handles are sealed.
package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/program/keyspace"
)

type rootKind uint8

const (
	rootRuntime rootKind = iota + 1
	rootFresh
)

type rootRow struct {
	kind    rootKind
	valueID keyspace.ContentID
	freshID keyspace.ContentID
}

// Root is one existing Boundary Value or one canonical fresh Heap Key.
// It is the TypeValue Factor coordinate; zero is a valid first coordinate.
type Root struct {
	owner *Authority
	index uint32
}

func (a *Authority) sealRoots() bool {
	if a == nil || a.static == nil {
		return false
	}
	seedKeys := make(map[keyspace.ContentID]keyspace.ContentID)
	if !a.forEachStaticSeed(func(valueID keyspace.ContentID, _ string, root keyspace.ContentID, _ typeauthority.RuntimeInner, _ bool) bool {
		if !root.Available() {
			return false
		}
		seedKeys[valueID] = root
		return true
	}) {
		return false
	}
	a.runtimeRoots = make(map[keyspace.ContentID]uint32, len(seedKeys))
	a.allocationRoots = make(map[heap.Key]uint32, a.heap.KeyCount())
	representatives := make(map[keyspace.ContentID]uint32, len(seedKeys))
	for valueID, key := range seedKeys {
		if representative, found := representatives[key]; found {
			a.runtimeRoots[valueID] = representative
			continue
		}
		representatives[key] = uint32(len(a.roots))
		a.runtimeRoots[valueID] = uint32(len(a.roots))
		a.roots = append(a.roots, rootRow{kind: rootRuntime, valueID: valueID})
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
		if _, programRoot := allocation.AllocationReceipt(); programRoot {
			// Program-backed roots are retained as detached allocation identities;
			// resolving their Boundary Value is deliberately outside this domain.
			allocationID, allocationOK := a.heap.KeyID(allocation)
			if !allocationOK || !allocationID.Available() {
				return false
			}
			rootIndex := uint32(len(a.roots))
			a.roots = append(a.roots, rootRow{kind: rootFresh, freshID: allocationID})
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
		freshID, freshOK := a.heap.KeyID(allocation)
		if !freshOK || !freshID.Available() {
			return false
		}
		a.roots = append(a.roots, rootRow{kind: rootFresh, freshID: freshID})
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

func (a *Authority) RootForValueIdentity(valueID keyspace.ContentID) (Root, bool) {
	if a == nil || !valueID.Available() {
		return Root{}, false
	}
	index, ok := a.runtimeRoots[valueID]
	return Root{owner: a, index: index}, ok
}

func (a *Authority) RootValueIdentity(root Root) (keyspace.ContentID, bool) {
	row, ok := a.root(root)
	if !ok || row.kind != rootRuntime {
		return keyspace.ContentID{}, false
	}
	return row.valueID, true
}

func (a *Authority) FreshRootID(root Root) (keyspace.ContentID, bool) {
	row, ok := a.root(root)
	if !ok || row.kind != rootFresh {
		return keyspace.ContentID{}, false
	}
	return row.freshID, true
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
