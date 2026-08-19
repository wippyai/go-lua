package index_test

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	calldomain "github.com/wippyai/go-lua/domain/call"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	indexdomain "github.com/wippyai/go-lua/domain/heap/index"
	"github.com/wippyai/go-lua/domain/materialization"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

func TestTopologyReceiverCallDemandFencesExactStaticBottomAndForeign(t *testing.T) {
	heap, values, calls, _, fresh, mounts := freshTopologyFixture(t)
	topology, sealed := indexdomain.Seal(heap, values, calls, mounts.packs)
	application, _, _, _, source := fresh.FreshResultID()
	wantKey, keyed := calls.KeyForApplicationID(application)
	atom, atomOK := values.Allocation(fresh, materialization.Recent)
	receiver, receiverOK := values.Singleton(atom)
	if !sealed || !source || !keyed || !atomOK || !receiverOK {
		t.Fatalf("fresh demand fixture sealed=%t source=%t key=%t atom=%t receiver=%t", sealed, source, keyed, atomOK, receiverOK)
	}

	var exact []struct {
		key calldomain.Key
		tag uint64
	}
	if !topology.VisitReceiverCallDemand(receiver, func(key calldomain.Key, tag uint64) bool {
		exact = append(exact, struct {
			key calldomain.Key
			tag uint64
		}{key: key, tag: tag})
		return true
	}) || len(exact) != 1 || exact[0].key != wantKey || exact[0].tag == 0 {
		t.Fatalf("exact fresh receiver demand=%#v, want one nonzero tag for its Call key", exact)
	}
	unsupported, unsupportedAtom := values.Allocation(fresh, materialization.Exact)
	unsupportedReceiver, unsupportedValue := values.Singleton(unsupported)
	unsupportedCalls := 0
	if !unsupportedAtom || !unsupportedValue || !topology.VisitReceiverCallDemand(unsupportedReceiver, func(calldomain.Key, uint64) bool {
		unsupportedCalls++
		return true
	}) || unsupportedCalls != 0 {
		t.Fatal("unsupported fresh materialization demanded Call state")
	}

	bottomCalls := 0
	if !topology.VisitReceiverCallDemand(values.Bottom(), func(calldomain.Key, uint64) bool {
		bottomCalls++
		return true
	}) || bottomCalls != 0 {
		t.Fatal("Value.Bottom was not a valid empty demand")
	}

	staticHeap, staticValues, staticCalls, _, staticRoot, _, staticMounts := staticTopologyFixture(t)
	staticTopology, staticSealed := indexdomain.Seal(staticHeap, staticValues, staticCalls, staticMounts.packs)
	staticAtom, staticAtomOK := staticValues.Allocation(staticRoot, materialization.Recent)
	staticReceiver, staticReceiverOK := staticValues.Singleton(staticAtom)
	staticCallsObserved := 0
	if !staticSealed || !staticAtomOK || !staticReceiverOK || !staticTopology.VisitReceiverCallDemand(staticReceiver, func(calldomain.Key, uint64) bool {
		staticCallsObserved++
		return true
	}) || staticCallsObserved != 0 {
		t.Fatal("static table receiver demanded guarded Call state")
	}

	foreignCalls := 0
	foreignValues := topologyForeignValues(t)
	if topology.VisitReceiverCallDemand(foreignValues.Top(), func(calldomain.Key, uint64) bool {
		foreignCalls++
		return true
	}) || foreignCalls != 0 {
		t.Fatal("foreign Value crossed the demand owner fence")
	}
}

func TestTopologyReceiverCallDemandDeduplicatesFreshApplicationAndWarms(t *testing.T) {
	heap, values, calls, _, mounts := callDemandTopologyFixture(t, "heap_index_two_fresh_results.lua", `
local first, second = fresh()
return first, second
`, []vocabulary.FreshResultSpec{
		{Result: 0, Kind: schematype.FreshClassTable},
		{Result: 1, Kind: schematype.FreshClassTable},
	})
	topology, sealed := indexdomain.Seal(heap, values, calls, mounts.packs)
	if !sealed {
		t.Fatal("two-result fresh topology did not seal")
	}
	var application identity.ContentID
	for index := 0; index < heap.KeyCount(); index++ {
		root, _ := heap.KeyAt(index)
		candidate, _, _, _, fresh := root.FreshResultID()
		if fresh {
			application = candidate
			break
		}
	}
	if !application.Available() {
		t.Fatal("two-result fixture has no fresh application")
	}
	roots := freshRootsForApplication(t, heap, application)
	if len(roots) != 2 {
		t.Fatalf("fresh roots for one application=%d, want two", len(roots))
	}
	wantKey, keyed := calls.KeyForApplicationID(application)
	firstRecent, firstOK := values.Allocation(roots[0], materialization.Recent)
	firstSummary, summaryOK := values.Allocation(roots[0], materialization.Summary)
	secondExact, secondOK := values.Allocation(roots[1], materialization.Exact)
	receiver, receiverOK := values.Alternatives(firstRecent, firstSummary, secondExact)
	if !keyed || !firstOK || !summaryOK || !secondOK || !receiverOK {
		t.Fatalf("same-application atoms key=%t first=%t summary=%t second=%t receiver=%t", keyed, firstOK, summaryOK, secondOK, receiverOK)
	}

	callsObserved, tag := 0, uint64(0)
	if !topology.VisitReceiverCallDemand(receiver, func(key calldomain.Key, gotTag uint64) bool {
		if key != wantKey || gotTag == 0 {
			t.Fatalf("demand key/tag=%v/%d, want exact application/nonzero", key, gotTag)
		}
		callsObserved++
		tag = gotTag
		return true
	}) || callsObserved != 1 || tag == 0 {
		t.Fatalf("same fresh application demanded %d times, tag=%d", callsObserved, tag)
	}

	if !topology.VisitReceiverCallDemand(receiver, func(calldomain.Key, uint64) bool { return true }) {
		t.Fatal("warm demand traversal")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if !topology.VisitReceiverCallDemand(receiver, func(calldomain.Key, uint64) bool { return true }) {
			panic("warm demand traversal")
		}
	}); allocations != 0 {
		t.Fatalf("warm exact demand allocated %v times", allocations)
	}
}

func TestTopologyReceiverCallDemandTopScopesOnlyFreshApplicationsAndTagsAreStable(t *testing.T) {
	heap, values, calls, _, mounts := callDemandTopologyFixture(t, "heap_index_fresh_groups.lua", `
local function localOnly()
  return 1
end
local first = fresh()
local second = fresh()
local ignored = localOnly()
return first, second, ignored
`, []vocabulary.FreshResultSpec{{Result: 0, Kind: schematype.FreshClassTable}})
	topology, sealed := indexdomain.Seal(heap, values, calls, mounts.packs)
	if !sealed {
		t.Fatal("fresh-group topology did not seal")
	}
	want := freshCallIDs(t, heap, calls)
	all := allCallIDs(t, calls)
	// Link conservatively admits every executable Call application to a
	// non-require Target operation. Consequently this fixture's localOnly call
	// receives the same nominal FreshResult relation as the two fresh() calls;
	// the semantic exclusion law is therefore exact equality with Call's own
	// universe, not an invented source-level callee filter.
	if len(want) != len(all) {
		t.Fatalf("fresh Call universe=%d, all Call keys=%d", len(want), len(all))
	}
	for id := range want {
		if _, ok := all[id]; !ok {
			t.Fatalf("fresh Call universe contains non-Call key %v", id)
		}
	}
	for id := range all {
		if _, ok := want[id]; !ok {
			t.Fatalf("Call key %v lacks the conservative FreshResult relation", id)
		}
	}
	got := callDemandTags(t, topology, values.Top())
	if !sameDemandUniverse(got, want) {
		t.Fatalf("Value.Top Call demand=%#v, want only fresh Call groups %#v", got, want)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if !topology.VisitReceiverCallDemand(values.Top(), func(calldomain.Key, uint64) bool { return true }) {
			panic("top Call demand traversal")
		}
	}); allocations != 0 {
		t.Fatalf("receipt-hot Top Call demand allocated %v times", allocations)
	}
	stateTags := make(map[identity.ContentID]uint64)
	if !topology.VisitReceiver(values.Top(), func(key calldomain.Key, tag uint64) (calldomain.Value, bool) {
		id, ok := key.ContentID()
		if !ok || got[id] != tag {
			t.Fatalf("Value.Top CallState key/tag=%v/%d did not match demand %#v", key, tag, got)
		}
		stateTags[id] = tag
		return calls.Bottom(), true
	}, func(indexdomain.Route) bool { return true }) || !reflect.DeepEqual(stateTags, got) {
		t.Fatalf("Value.Top route observation changed Call demand selection: state=%#v demand=%#v", stateTags, got)
	}

	heap2, values2, calls2, _, mounts2 := callDemandTopologyFixture(t, "heap_index_fresh_groups.lua", `
local function localOnly()
  return 1
end
local first = fresh()
local second = fresh()
local ignored = localOnly()
return first, second, ignored
`, []vocabulary.FreshResultSpec{{Result: 0, Kind: schematype.FreshClassTable}})
	topology2, sealed2 := indexdomain.Seal(heap2, values2, calls2, mounts2.packs)
	if !sealed2 {
		t.Fatal("equivalent fresh-group topology did not seal")
	}
	if want2 := freshCallIDs(t, heap2, calls2); !reflect.DeepEqual(want2, want) {
		t.Fatalf("equivalent Link changed fresh Call identity: %#v != %#v", want2, want)
	}
	if got2 := callDemandTags(t, topology2, values2.Top()); !reflect.DeepEqual(got2, got) {
		t.Fatalf("equivalent topology changed transient Call demand tags: %#v != %#v", got2, got)
	}
}

func TestTopologyCallStateUsesDemandTagAndSeparatesBottomFromUnavailable(t *testing.T) {
	heap, values, calls, _, fresh, mounts := freshTopologyFixture(t)
	topology, sealed := indexdomain.Seal(heap, values, calls, mounts.packs)
	atom, atomOK := values.Allocation(fresh, materialization.Recent)
	receiver, receiverOK := values.Singleton(atom)
	if !sealed || !atomOK || !receiverOK {
		t.Fatal("Call-state fixture")
	}
	var demandedKey calldomain.Key
	var demandedTag uint64
	if !topology.VisitReceiverCallDemand(receiver, func(key calldomain.Key, tag uint64) bool {
		demandedKey, demandedTag = key, tag
		return true
	}) || !demandedKey.Valid() || demandedTag == 0 {
		t.Fatal("receiver did not expose exact Call demand before route observation")
	}

	var bottomRoutes []indexdomain.Route
	if !topology.VisitReceiver(receiver, func(key calldomain.Key, tag uint64) (calldomain.Value, bool) {
		if key != demandedKey || tag != demandedTag {
			t.Fatalf("CallState observed %v/%d, want demand %v/%d", key, tag, demandedKey, demandedTag)
		}
		return calls.Bottom(), true
	}, func(route indexdomain.Route) bool {
		bottomRoutes = append(bottomRoutes, route)
		return true
	}) || len(bottomRoutes) != 0 {
		t.Fatalf("available Call.Bottom produced routes: %#v", bottomRoutes)
	}

	assertUnknown := func(name string, state indexdomain.CallState) {
		t.Helper()
		var routes []indexdomain.Route
		if !topology.VisitReceiver(receiver, state, func(route indexdomain.Route) bool {
			routes = append(routes, route)
			return true
		}) || len(routes) != 1 || routes[0].Kind() != indexdomain.RouteUnknown {
			t.Fatalf("%s routes=%#v, want one unknown", name, routes)
		}
	}
	assertUnknown("unavailable", func(key calldomain.Key, tag uint64) (calldomain.Value, bool) {
		if key != demandedKey || tag != demandedTag {
			t.Fatal("unavailable CallState did not retain demand selection")
		}
		return calls.Bottom(), false
	})
	assertUnknown("malformed", func(calldomain.Key, uint64) (calldomain.Value, bool) {
		return calldomain.Value{}, true
	})
	_, _, foreignCalls, _, _, _ := freshTopologyFixture(t)
	assertUnknown("foreign", func(calldomain.Key, uint64) (calldomain.Value, bool) {
		return foreignCalls.Top(), true
	})
}

func callDemandTags(t testing.TB, topology *indexdomain.Topology, receiver valuedomain.Value) map[identity.ContentID]uint64 {
	t.Helper()
	result := make(map[identity.ContentID]uint64)
	seenTags := make(map[uint64]identity.ContentID)
	if !topology.VisitReceiverCallDemand(receiver, func(key calldomain.Key, tag uint64) bool {
		id, ok := key.ContentID()
		if !ok || tag == 0 {
			t.Fatalf("invalid Call demand key/tag %v/%d", key, tag)
		}
		if previous, duplicate := result[id]; duplicate {
			t.Fatalf("duplicate demand id/tag %v/%d (previous tag=%d)", id, tag, previous)
		}
		if previous, duplicate := seenTags[tag]; duplicate {
			t.Fatalf("duplicate transient demand tag %d for %v and %v", tag, previous, id)
		}
		result[id] = tag
		seenTags[tag] = id
		return true
	}) {
		t.Fatal("Call demand traversal failed")
	}
	return result
}

func sameDemandUniverse(got, want map[identity.ContentID]uint64) bool {
	if len(got) != len(want) {
		return false
	}
	for id := range want {
		if _, ok := got[id]; !ok {
			return false
		}
	}
	return true
}

func freshCallIDs(t testing.TB, heap heapdomain.Schema, calls *calldomain.Algebra) map[identity.ContentID]uint64 {
	t.Helper()
	result := make(map[identity.ContentID]uint64)
	for index := 0; index < heap.KeyCount(); index++ {
		root, ok := heap.KeyAt(index)
		if !ok {
			continue
		}
		application, _, _, _, fresh := root.FreshResultID()
		if !fresh {
			continue
		}
		key, ok := calls.KeyForApplicationID(application)
		id, idOK := key.ContentID()
		if !ok || !idOK {
			t.Fatal("fresh root lacks Call key")
		}
		result[id] = 0
	}
	return result
}

func allCallIDs(t testing.TB, calls *calldomain.Algebra) map[identity.ContentID]struct{} {
	t.Helper()
	result := make(map[identity.ContentID]struct{})
	for index := 0; index < calls.KeyCount(); index++ {
		key, ok := calls.KeyAt(index)
		if ok && !key.IsApplication() {
			continue
		}
		id, idOK := key.ContentID()
		if !ok || !idOK {
			t.Fatal("Call key")
		}
		result[id] = struct{}{}
	}
	return result
}

func freshRootsForApplication(t testing.TB, heap heapdomain.Schema, application identity.ContentID) []heapdomain.Key {
	t.Helper()
	var roots []heapdomain.Key
	for index := 0; index < heap.KeyCount(); index++ {
		root, ok := heap.KeyAt(index)
		if !ok {
			continue
		}
		candidate, _, _, _, fresh := root.FreshResultID()
		if fresh && candidate == application {
			roots = append(roots, root)
		}
	}
	return roots
}

func callDemandTopologyFixture(t testing.TB, name, text string, freshResults []vocabulary.FreshResultSpec) (heapdomain.Schema, *valuedomain.Schema, *calldomain.Algebra, *link.Link, indexFixtureMounts) {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: name, Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	resultCount := 1
	for _, fresh := range freshResults {
		if int(fresh.Result)+1 > resultCount {
			resultCount = int(fresh.Result) + 1
		}
	}
	outputs := portableAnyTypes(resultCount)
	binding := vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"fresh"}}
	contract, err := compiler.Seal(&declaration.Spec{
		Semantics:    domaincontract.NewSemantics(),
		InitialRoots: []vocabulary.InitialRootSpec{{Identity: "GlobalEnvRoot", Shape: vocabulary.BootShapeSpec{Aggregate: vocabulary.BootAggregateTable, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"}}}},
		Operations: []vocabulary.OperationSpec{{
			Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"require"}}},
			Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
			Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
			Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}, {
			Bindings: []vocabulary.BindingSpec{binding},
			Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
			Outcomes: []vocabulary.OutcomeSpec{{
				Kind:         flowkind.OutcomeNormal,
				Values:       vocabulary.ValuesSpec{Fixed: outputs, Tail: vocabulary.ValuesClosed},
				FreshResults: freshResults,
			}},
			Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}},
		InitialEntries: []vocabulary.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: vocabulary.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "fresh"}, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueOperation, Operation: binding}, Mutability: vocabulary.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__link_absent"}, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueAbsent}, Mutability: vocabulary.InitialMutable},
		},
		InitialBindings: []vocabulary.InitialBindingSpec{
			{Name: "_G", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}},
			{Name: "fresh", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "fresh"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	heap, values, calls, mounts := indexSchemas(t, linked)
	return heap, values, calls, linked, mounts
}
