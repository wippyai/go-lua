package index_test

import (
	"testing"

	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	indexdomain "github.com/wippyai/go-lua/analysis/domain/heap/index"
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
)

type bootInitialIndex struct {
	route   heapdomain.RawRouteTag
	payload heapdomain.RawPayloadTag
}

// bootInitialExpectation derives the complete Target boot-slot frontier from
// Heap's and Value's own owner projections. It is the independent denominator
// the sealed index catalog must reproduce.
func bootInitialExpectation(t testing.TB, heap heapdomain.Schema, values *valuedomain.Schema) (map[bootInitialIndex]valuedomain.Value, heapdomain.RawPayloadTag) {
	t.Helper()
	tags := make(map[heapdomain.Payload]heapdomain.RawPayloadTag)
	highest := heapdomain.RawPayloadTag(0)
	if !heap.VisitRawPayloadTags(func(tag heapdomain.RawPayloadTag, payload heapdomain.Payload) bool {
		tags[payload] = tag
		if tag > highest {
			highest = tag
		}
		return true
	}) {
		t.Fatal("boot initial payload universe")
	}
	expected := make(map[bootInitialIndex]valuedomain.Value)
	for index := 0; index < heap.BootEntryCount(); index++ {
		entry, entryOK := heap.BootEntryAt(index)
		if !entryOK {
			t.Fatal("boot entry")
		}
		presence, payload, projectionOK := entry.Projection()
		if !projectionOK {
			t.Fatal("boot entry projection")
		}
		if presence != heapdomain.RawPresent {
			continue
		}
		key, keyOK := entry.Key()
		root, rootOK := key.BootID()
		initial, initialOK := payload.InitialValue()
		tag, tagOK := tags[payload]
		if !keyOK || !rootOK || !initialOK || !tagOK {
			t.Fatal("boot entry identity")
		}
		value, valueOK := values.TargetInitialID(root, initial)
		if !valueOK {
			t.Fatal("owner boot initial value")
		}
		for _, role := range materialization.Roles() {
			route, routeOK := heap.RouteTag(key, role)
			if !routeOK {
				continue
			}
			expected[bootInitialIndex{route: route, payload: tag}] = value
		}
	}
	return expected, highest
}

// TestRawGetBootInitialValuesAreSealedOwnerReceipts is the single-authority law
// for RawGet's immutable Target boot slots. The Value owner issues every boot
// initial fact once, at seal, into this Topology's own cold table indexed by
// the route and payload tags the hot read already holds. The lookup must
// therefore answer with the rule's live Value schema detached: a rule that
// reopened Value mid-solve to manufacture the fact declares a footprint that
// invalidation and scheduling cannot see.
func TestRawGetBootInitialValuesAreSealedOwnerReceipts(t *testing.T) {
	heap, values, calls, _, _, mounts := freshTopologyFixture(t)
	topology := indexTopology(t, heap, values, calls, mounts)
	expected, highest := bootInitialExpectation(t, heap, values)
	if len(expected) == 0 {
		t.Fatal("boot initial law fixture exposes no Target boot slot")
	}

	sealed := make(map[bootInitialIndex]valuedomain.Value, len(expected))
	if !indexdomain.VisitSealedBootInitials(topology, func(route heapdomain.RawRouteTag, payload heapdomain.RawPayloadTag, value valuedomain.Value) bool {
		sealed[bootInitialIndex{route: route, payload: payload}] = value
		return true
	}) {
		t.Fatal("sealed boot initial walk")
	}
	if len(sealed) != len(expected) {
		t.Fatalf("sealed boot initials=%d owner boot initials=%d", len(sealed), len(expected))
	}
	for index, want := range expected {
		got, found := sealed[index]
		if !found || !values.Equal(got, want) {
			t.Fatalf("sealed boot initial route=%d payload=%d found=%t equal=%t", index.route, index.payload, found, found && values.Equal(got, want))
		}
		detached, detachedOK := indexdomain.SealedBootInitialWithoutValueSchema(topology, index.route, index.payload)
		if !detachedOK || !values.Equal(detached, want) {
			t.Fatalf("detached boot initial route=%d payload=%d resolved=%t equal=%t", index.route, index.payload, detachedOK, detachedOK && values.Equal(detached, want))
		}
	}
	for index := range expected {
		if _, ok := indexdomain.SealedBootInitialWithoutValueSchema(topology, index.route, highest+1); ok {
			t.Fatal("unsealed payload tag resolved a boot initial")
		}
		if _, ok := indexdomain.SealedBootInitialWithoutValueSchema(topology, 0, index.payload); ok {
			t.Fatal("unsealed route tag resolved a boot initial")
		}
	}
}

// TestRawGetBootInitialTransferJoinsTheSealedReceipt walks one real sealed boot
// RawAccess through the production transfer branch. Every present boot slot
// must reduce to a nonempty result bounded by the owner-issued receipt.
func TestRawGetBootInitialTransferJoinsTheSealedReceipt(t *testing.T) {
	heap, values, calls, _, fresh, mounts := freshTopologyFixture(t)
	topology := indexTopology(t, heap, values, calls, mounts)
	expected, _ := bootInitialExpectation(t, heap, values)
	if len(expected) == 0 {
		t.Fatal("boot initial transfer fixture exposes no Target boot slot")
	}
	foreign, foreignOK := heap.RouteTag(fresh, materialization.Recent)
	if !foreignOK {
		t.Fatal("foreign allocation route")
	}

	bootID, bootIDOK := heap.BootIDAt(0)
	key, keyOK := heap.KeyForBootID(bootID)
	if !bootIDOK || !keyOK {
		t.Fatal("boot root key")
	}
	route, routeOK := heap.RouteTag(key, materialization.Exact)
	fact, factOK := bootRelation(t, heap, key)
	if !routeOK || !factOK {
		t.Fatalf("boot route=%t fact=%t", routeOK, factOK)
	}

	visited := 0
	for index := 0; index < heap.BootEntryCount(); index++ {
		entry, entryOK := heap.BootEntryAt(index)
		if !entryOK {
			t.Fatal("boot entry")
		}
		entryKey, entryKeyOK := entry.Key()
		presence, _, projectionOK := entry.Projection()
		if !entryOK || !entryKeyOK || !projectionOK || entryKey != key || presence != heapdomain.RawPresent {
			continue
		}
		slot, slotOK := entry.Slot()
		selector, selectorOK := heap.SelectorForSlot(slot)
		if !slotOK || !selectorOK {
			t.Fatal("boot entry selector")
		}
		if !heap.VisitRawAccess(key, fact, materialization.Exact, selector, func(access heapdomain.RawAccess) bool {
			cell, cellOK := access.Cell()
			if !cellOK || cell.PresentCount() != 1 {
				return false
			}
			present, presentOK := cell.PresentAt(0)
			if !presentOK {
				return false
			}
			payload, payloadOK := access.PayloadTag(present)
			if !payloadOK {
				return false
			}
			want, wanted := expected[bootInitialIndex{route: route, payload: payload}]
			result, any, ok := indexdomain.ApplyBootInitialPresent(topology, route, access, present)
			if !wanted || !ok || !any {
				t.Fatalf("boot transfer slot=%d wanted=%t ok=%t any=%t", index, wanted, ok, any)
			}
			if values.Equal(result, values.Bottom()) {
				t.Fatalf("boot transfer slot=%d reduced a present initial to bottom", index)
			}
			joined, joinOK := values.Join(result, want)
			if !joinOK || !values.Equal(joined, want) {
				t.Fatalf("boot transfer slot=%d escaped its owner receipt join=%t", index, joinOK)
			}
			// The receipt is selected by the route the read arrived on. A
			// branch that recomputed the fact from the Present tuple alone
			// would answer here for a route that seals no such boot slot.
			if _, _, ok := indexdomain.ApplyBootInitialPresent(topology, foreign, access, present); ok {
				t.Fatalf("boot transfer slot=%d answered a foreign allocation route", index)
			}
			if _, _, ok := indexdomain.ApplyBootInitialPresent(topology, 0, access, present); ok {
				t.Fatalf("boot transfer slot=%d answered an unsealed route", index)
			}
			visited++
			return true
		}) {
			t.Fatalf("boot raw access slot=%d", index)
		}
	}
	if visited == 0 {
		t.Fatal("boot transfer visited no present initial slot")
	}
}

// bootRelation rebuilds one exact bootstrap Heap fact from Heap's own boot
// entry projection.
func bootRelation(t testing.TB, heap heapdomain.Schema, key heapdomain.Key) (heapdomain.Value, bool) {
	t.Helper()
	frozen, frozenOK := heap.BootFrozen(key)
	none, noneOK := heap.ContainmentNone()
	if !frozenOK || !noneOK {
		return heapdomain.Value{}, false
	}
	initializer, initializerOK := heap.BeginObject(heapdomain.ShapeEligible, frozen, none)
	if !initializerOK {
		return heapdomain.Value{}, false
	}
	for index := 0; index < heap.BootEntryCount(); index++ {
		entry, entryOK := heap.BootEntryAt(index)
		entryKey, entryKeyOK := entry.Key()
		if !entryOK || !entryKeyOK || entryKey != key {
			continue
		}
		slot, slotOK := entry.Slot()
		selector, selectorOK := heap.SelectorForSlot(slot)
		presence, payload, projectionOK := entry.Projection()
		if !slotOK || !selectorOK || !projectionOK {
			return heapdomain.Value{}, false
		}
		var state heapdomain.CellState
		var stateOK bool
		if presence == heapdomain.RawAbsent {
			state, stateOK = heap.CellAbsent()
		} else {
			containment, containmentOK := entry.ValueContainment()
			if !containmentOK {
				return heapdomain.Value{}, false
			}
			state, stateOK = heap.CellPresent(slot, payload, containment, none)
		}
		if !stateOK || !initializer.Apply(selector, state) {
			return heapdomain.Value{}, false
		}
	}
	object, objectOK := initializer.Finish()
	if !objectOK {
		return heapdomain.Value{}, false
	}
	world, worldOK := heap.Exact(key, object)
	if !worldOK {
		return heapdomain.Value{}, false
	}
	return heap.Relation(key, world)
}
