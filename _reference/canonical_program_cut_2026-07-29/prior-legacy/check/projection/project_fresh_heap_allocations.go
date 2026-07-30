package projection

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// projectFreshHeapAllocations records complete, caller-instantiable returned
// heap graphs. Eligibility is atomic per return root: a shared, unstable, or
// provenance-free local node rejects the entire graph so substitution can never
// sever escape propagation between a template parent and template descendant.
func projectFreshHeapAllocations(
	reg *axis.Registry,
	result ResultReader,
	exit state.State,
	objects map[identity.ID]heapidentity.TableObject,
	returns []product.Value,
	declared []product.Value,
) []summary.FreshHeapAllocation {
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
		return nil
	}

	var out []summary.FreshHeapAllocation
	for i, value := range returns {
		if returnContractHidesAllocationIdentity(reg, declared, i) {
			continue
		}
		root, ok := product.Get(reg, value, identity.Key).ID()
		if !ok || root == (identity.ID{}) {
			continue
		}
		candidate, ok := collectFreshHeapGraph(reg, exit, entryHeap.Objects, objects, root)
		if ok {
			out = append(out, candidate...)
		}
	}
	return out
}

func collectFreshHeapGraph(
	reg *axis.Registry,
	exit state.State,
	entryObjects map[identity.ID]heapidentity.TableObject,
	objects map[identity.ID]heapidentity.TableObject,
	root identity.ID,
) ([]summary.FreshHeapAllocation, bool) {
	seen := make(map[identity.ID]struct{})
	var out []summary.FreshHeapAllocation
	var visitValue func(product.Value) bool
	var visitID func(identity.ID) bool
	visitValue = func(value product.Value) bool {
		id, ok := product.Get(reg, value, identity.Key).ID()
		return !ok || visitID(id)
	}
	visitID = func(id identity.ID) bool {
		if id == (identity.ID{}) {
			return true
		}
		if _, ok := seen[id]; ok {
			return true
		}
		seen[id] = struct{}{}
		if _, existedAtEntry := entryObjects[id]; existedAtEntry {
			return true
		}
		object, ok := objects[id]
		if !ok {
			return true
		}
		p := exit.ReadPlacement(id)
		if !object.StableShape() || (p != placement.Stack && p != placement.OwnedHeap) {
			return false
		}
		out = append(out, summary.FreshHeapAllocation{ID: id, Placement: p})
		if !visitValue(object.Root()) {
			return false
		}
		for _, member := range object.StaticMembers() {
			if !visitValue(member) {
				return false
			}
		}
		for _, fact := range object.DynamicIndexFacts() {
			if !visitValue(fact.KeyValue) || !visitValue(fact.Value) {
				return false
			}
		}
		return true
	}
	if !visitID(root) || len(out) == 0 {
		return nil, false
	}
	return out, true
}

func returnContractHidesAllocationIdentity(reg *axis.Registry, declared []product.Value, index int) bool {
	if index < 0 || index >= len(declared) {
		return false
	}
	t, ok := typevalue.TypeOf(reg, declared[index])
	if ok && t != nil && (typ.IsAny(t) || typ.IsUnknown(t)) {
		return true
	}
	ev := product.Get(reg, declared[index], evidence.Key)
	return ev.IsExplicitTop() || ev.IsGradualTop()
}
