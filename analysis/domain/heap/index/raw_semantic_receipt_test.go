package index_test

import (
	"testing"

	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/materialization"
)

// RawGet/RawSet share Heap's receipt-native raw projection. This law walks a
// real route, checks the complete absent/present payload boundary, then sends
// both immutable write branches through Heap's RawStore/RawDelete owner.
func TestRawReceiptProjectionAndMutationUseRealHeapRoutes(t *testing.T) {
	heap, _, _, _, root, _, _ := staticTopologyFixture(t)
	var candidate heapdomain.IndexAccess
	for index := 0; index < heap.IndexAccessCount(); index++ {
		access, accessOK := heap.IndexAccessAt(index)
		geometry, geometryOK := heap.IndexAccessGeometry(access)
		if accessOK && geometryOK && !geometry.Read {
			candidate = access
			break
		}
	}
	if candidate == (heapdomain.IndexAccess{}) {
		t.Fatal("raw receipt fixture has no write candidate")
	}
	slot, slotOK := heap.SlotForIndexAccess(candidate)
	payload, payloadOK := heap.PayloadForIndexAccess(candidate)
	none, noneOK := heap.ContainmentNone()
	selector, selectorOK := heap.SelectorForSlot(slot)
	kindSelector, kindSelectorOK := heap.KindSelector()
	initializer, initOK := heap.BeginObject(heapdomain.ShapeEligible, heapdomain.FrozenMutable, none)
	cell, cellOK := heap.CellPresent(slot, payload, none, none)
	if !slotOK || !payloadOK || !noneOK || !selectorOK || !kindSelectorOK || !initOK || !cellOK || !initializer.Apply(selector, cell) {
		t.Fatal("raw receipt fixture")
	}
	fresh, freshOK := initializer.Finish()
	seed, seedOK := heap.EmptyObject(root)
	fact, factOK := heap.Create(seed, root, fresh)
	route, routeOK := heap.RouteTag(root, materialization.Recent)
	if !freshOK || !seedOK || !factOK || !routeOK {
		t.Fatal("raw route fact")
	}

	seenPresent, seenAbsent := false, false
	if !heap.VisitRawAccessRoute(route, fact, kindSelector, func(access heapdomain.RawAccess) bool {
		if access.IsTop() {
			return false
		}
		state, stateOK := access.Cell()
		raw, rawOK := state.Raw()
		if !stateOK || !rawOK {
			return false
		}
		if raw == heapdomain.RawAbsent {
			seenAbsent = true
			_, deleteOK := heap.RawDelete(access, heapdomain.MutationLicence{})
			return deleteOK
		}
		if raw != heapdomain.RawPresent || state.PresentCount() != 1 {
			return false
		}
		seenPresent = true
		present, presentOK := state.PresentAt(0)
		if !presentOK {
			return false
		}
		payloadTag, tagOK := access.PayloadTag(present)
		_, payloadOK := heap.PayloadForRawTag(payloadTag)
		presentSlot, slotOK := present.Slot()
		presentPayload, payloadOK := present.Payload()
		valueContainment, keyContainment, containmentOK := present.Containment()
		replacement, replacementOK := heap.CellPresent(presentSlot, presentPayload, valueContainment, keyContainment)
		branches, storeOK := heap.RawStore(access, replacement, heapdomain.MutationLicence{})
		_, normalOK := branches.Normal()
		return tagOK && slotOK && payloadOK && containmentOK && replacementOK && storeOK && normalOK
	}) {
		t.Fatal("raw receipt route projection")
	}
	if !seenPresent {
		t.Fatal("raw receipt route omitted stored payload")
	}
	if !seenAbsent {
		t.Fatal("raw receipt route omitted the RawAbsent branch")
	}
}
