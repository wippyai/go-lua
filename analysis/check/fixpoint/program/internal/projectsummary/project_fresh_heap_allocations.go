package projectsummary

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

// projectFreshHeapAllocations records only returned heap identities introduced
// by this callee. Entry-owned identities are parameters, captures, globals, or
// caller heap and retain their identity across the boundary.
func projectFreshHeapAllocations(
	reg *axis.Registry,
	result ResultReader,
	objects map[identity.ID]heapidentity.TableObject,
	returns []product.Value,
) []identity.ID {
	if reg == nil || result == nil || len(objects) == 0 || len(returns) == 0 {
		return nil
	}
	entryReader, ok := result.(entryStateReader)
	if !ok {
		return nil
	}
	entry, ok := entryReader.EntryState()
	if !ok {
		return nil
	}
	entryHeap := entry.HeapTableObjectsSnapshot()
	if entryHeap.Top {
		// Unknown provenance must never be guessed fresh.
		return nil
	}
	seen := make(map[identity.ID]struct{})
	var fresh []identity.ID
	var visitValue func(product.Value)
	var visitID func(identity.ID)
	visitValue = func(value product.Value) {
		id, ok := product.Get(reg, value, identity.Key).ID()
		if ok {
			visitID(id)
		}
	}
	visitID = func(id identity.ID) {
		if id == (identity.ID{}) {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		object, ok := objects[id]
		if !ok {
			return
		}
		if _, existedAtEntry := entryHeap.Objects[id]; !existedAtEntry {
			fresh = append(fresh, id)
		}
		visitValue(object.Root())
		for _, value := range object.StaticMembers() {
			visitValue(value)
		}
		for _, fact := range object.DynamicIndexFacts() {
			visitValue(fact.KeyValue)
			visitValue(fact.Value)
		}
	}
	for _, value := range returns {
		visitValue(value)
	}
	return fresh
}
