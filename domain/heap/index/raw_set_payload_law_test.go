package index_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	calldomain "github.com/wippyai/go-lua/domain/call"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	indexdomain "github.com/wippyai/go-lua/domain/heap/index"
	"github.com/wippyai/go-lua/domain/materialization"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// TestRawSetUnconstrainedStoreReadsBackEveryAlternative is the write/read
// round-trip law for an unconstrained right-hand side.
//
// Top denotes every sealed alternative. Storing it must therefore read back at
// least what storing each of those alternatives reads back: the read lens
// selects a disjoint stored-class band per containment kind, so a single
// opaque child edge answers only the untracked band and drops every tracked
// allocation and boot child the same write could have installed. The
// enumerated sibling branch is the independent denominator.
func TestRawSetUnconstrainedStoreReadsBackEveryAlternative(t *testing.T) {
	heap, values, calls, mounts := rawSetPayloadFixture(t)
	topology := indexTopology(t, heap, values, calls, mounts)
	root, slot, payload := rawSetPayloadTarget(t, heap)
	selector, selectorOK := heap.SelectorForSlot(slot)
	none, noneOK := heap.ContainmentNone()
	fact, factOK := mutableAllocationFact(heap, root)
	if !selectorOK || !noneOK || !factOK {
		t.Fatalf("raw set payload fixture selector=%t none=%t fact=%t", selectorOK, noneOK, factOK)
	}

	raw, rawOK := singleRawAccess(heap, root, fact, selector)
	if !rawOK {
		t.Fatal("raw set payload access")
	}

	unconstrained, unconstrainedOK := indexdomain.ApplyRawSetTopPayload(topology, raw, slot, payload, none)
	if !unconstrainedOK {
		t.Fatal("unconstrained store")
	}
	observed, observedOK := indexdomain.ReadStoredPayload(topology, root, unconstrained, materialization.Recent, selector, values.Top())
	if !observedOK {
		t.Fatal("unconstrained read-back")
	}

	for _, alternative := range rawSetPayloadAlternatives(t, heap, values, root) {
		single, singleOK := values.Singleton(alternative.atom)
		if !singleOK {
			t.Fatalf("%s alternative singleton", alternative.name)
		}
		stored, storedOK := indexdomain.ApplyRawSetSourcePayload(topology, raw, slot, payload, none, single)
		if !storedOK {
			t.Fatalf("%s enumerated store", alternative.name)
		}
		want, wantOK := indexdomain.ReadStoredPayload(topology, root, stored, materialization.Recent, selector, values.Top())
		if !wantOK {
			t.Fatalf("%s enumerated read-back", alternative.name)
		}
		if values.Equal(want, values.Bottom()) {
			t.Fatalf("%s enumerated read-back is empty", alternative.name)
		}
		joined, joinedOK := values.Join(observed, want)
		if !joinedOK || !values.Equal(joined, observed) {
			t.Fatalf("unconstrained store dropped the %s alternative on read-back", alternative.name)
		}
	}
}

type rawSetPayloadAlternative struct {
	name string
	atom valuedomain.Atom
}

// rawSetPayloadAlternatives selects the two tracked child families the opaque
// stored class cannot answer for: a tracked allocation and a boot root.
func rawSetPayloadAlternatives(t testing.TB, heap heapdomain.Schema, values *valuedomain.Schema, root heapdomain.Key) []rawSetPayloadAlternative {
	t.Helper()
	allocation, allocationOK := values.Allocation(root, materialization.Recent)
	if !allocationOK {
		t.Fatal("tracked allocation alternative")
	}
	alternatives := []rawSetPayloadAlternative{{name: "tracked allocation", atom: allocation}}
	for index := 0; index < heap.BootCount(); index++ {
		bootID, bootIDOK := heap.BootIDAt(index)
		if !bootIDOK {
			continue
		}
		if atom, atomOK := values.BootID(bootID); atomOK {
			alternatives = append(alternatives, rawSetPayloadAlternative{name: "boot root", atom: atom})
			break
		}
	}
	if len(alternatives) != 2 {
		t.Fatal("raw set payload fixture exposes no boot root alternative")
	}
	return alternatives
}

func mutableAllocationFact(heap heapdomain.Schema, root heapdomain.Key) (heapdomain.Value, bool) {
	predecessor, predecessorOK := heap.EmptyObject(root)
	none, noneOK := heap.ContainmentNone()
	if !predecessorOK || !noneOK {
		return heapdomain.Value{}, false
	}
	initializer, initializerOK := heap.BeginObject(heapdomain.ShapeEligible, heapdomain.FrozenMutable, none)
	if !initializerOK {
		return heapdomain.Value{}, false
	}
	fresh, freshOK := initializer.Finish()
	if !freshOK {
		return heapdomain.Value{}, false
	}
	return heap.Create(predecessor, root, fresh)
}

func singleRawAccess(heap heapdomain.Schema, root heapdomain.Key, fact heapdomain.Value, selector heapdomain.KeySelector) (heapdomain.RawAccess, bool) {
	var selected heapdomain.RawAccess
	count := 0
	if !heap.VisitRawAccess(root, fact, materialization.Recent, selector, func(access heapdomain.RawAccess) bool {
		selected, count = access, count+1
		return true
	}) || count != 1 || !selected.Valid() {
		return heapdomain.RawAccess{}, false
	}
	return selected, true
}

func rawSetPayloadTarget(t testing.TB, heap heapdomain.Schema) (heapdomain.Key, heapdomain.Slot, heapdomain.Payload) {
	t.Helper()
	var root heapdomain.Key
	for index := 0; index < heap.KeyCount(); index++ {
		candidate, ok := heap.KeyAt(index)
		_, _, _, kind, _, source := heap.AllocationOriginForKey(candidate)
		if ok && source && kind == heapdomain.AllocationTable {
			root = candidate
			break
		}
	}
	for index := 0; index < heap.IndexAccessCount(); index++ {
		access, accessOK := heap.IndexAccessAt(index)
		if !accessOK {
			continue
		}
		geometry, geometryOK := heap.IndexAccessGeometry(access)
		if !geometryOK || geometry.Read {
			continue
		}
		slot, slotOK := heap.SlotForIndexAccess(access)
		payload, payloadOK := heap.PayloadForIndexAccess(access)
		if slotOK && payloadOK && root.Valid() {
			return root, slot, payload
		}
	}
	t.Fatal("raw set payload fixture exposes no written index access")
	return heapdomain.Key{}, heapdomain.Slot{}, heapdomain.Payload{}
}

func rawSetPayloadFixture(t testing.TB) (heapdomain.Schema, *valuedomain.Schema, *calldomain.Algebra, indexFixtureMounts) {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "heap_index_raw_set_payload.lua", Text: []byte(`local t = {}; t.field = _G; return t`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := compiler.Seal(&declaration.Spec{
		Semantics:    domaincontract.NewSemantics(),
		InitialRoots: []vocabulary.InitialRootSpec{{Identity: "GlobalEnvRoot", Shape: vocabulary.BootShapeSpec{Aggregate: vocabulary.BootAggregateTable, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"}}}},
		Operations: []vocabulary.OperationSpec{{
			Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"require"}}},
			Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
			Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
			Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}},
		InitialEntries: []vocabulary.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: vocabulary.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__link_absent"}, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueAbsent}, Mutability: vocabulary.InitialMutable},
		},
		InitialBindings: []vocabulary.InitialBindingSpec{{Name: "_G", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	heap, values, calls, mounts := indexSchemas(t, linked)
	return heap, values, calls, mounts
}
