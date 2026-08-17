package index_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	calldomain "github.com/wippyai/go-lua/domain/call"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	indexdomain "github.com/wippyai/go-lua/domain/heap/index"
	"github.com/wippyai/go-lua/domain/materialization"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

func portableAnyTypes(count int) []schematype.Type {
	values := make([]schematype.Type, count)
	for index := range values {
		value, ok := schematype.NewPrimitive(schematype.PrimitiveAny)
		if !ok {
			panic("portable any type")
		}
		values[index] = value
	}
	return values
}

func TestTopologyStaticRoutesPreserveExactTopAndHeapExtremes(t *testing.T) {
	heap, values, calls, _, rootKey, candidate, mounts := staticTopologyFixture(t)
	topology, sealed := indexdomain.Seal(heap, values, calls, mounts.packs)
	access, accessed := topology.Access(candidate)
	rooted := rootKey.Valid()
	atom, atomOK := values.Allocation(rootKey, materialization.Recent)
	receiver, receiverOK := values.Singleton(atom)
	if !sealed || !accessed || !rooted || !atomOK || !receiverOK {
		t.Fatalf("topology fixture sealed=%t accessed=%t rooted=%t atom=%t receiver=%t", sealed, accessed, rooted, atomOK, receiverOK)
	}
	coordinate, coordinateOK := access.Receiver()
	result, resultOK := access.Result()
	_, slotOK := access.Slot()
	wantID, idOK := heap.IndexAccessID(candidate)
	id, gotID := access.ID()
	if !coordinateOK || !resultOK || !slotOK || !idOK || !gotID || id != wantID || !coordinate.Valid() || !result.Valid() || !access.Read() {
		t.Fatal("access did not retain exact existing Link route")
	}
	if _, dynamic := access.DynamicKey(); dynamic {
		t.Fatal("exact index key fabricated a dynamic Value coordinate")
	}

	var exact []indexdomain.Route
	if !topology.VisitReceiver(receiver, nil, func(route indexdomain.Route) bool { exact = append(exact, route); return true }) || len(exact) != 1 {
		t.Fatal("exact rooted receiver did not produce exactly one route")
	}
	gotRoot, gotRole, exactOK := exact[0].Root()
	if !exactOK || gotRoot != rootKey || gotRole != materialization.Recent {
		t.Fatal("exact rooted receiver lost root-role correlation")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if !topology.VisitReceiver(receiver, nil, func(indexdomain.Route) bool { return true }) {
			panic("exact route")
		}
	}); allocations != 0 {
		t.Fatalf("exact receiver topology allocated %v times", allocations)
	}

	roles := map[materialization.Role]bool{}
	unknown, other := false, false
	if !topology.VisitReceiver(values.Top(), nil, func(route indexdomain.Route) bool {
		switch route.Kind() {
		case indexdomain.RouteRoot:
			key, role, ok := route.Root()
			if ok && key == rootKey {
				roles[role] = true
			}
		case indexdomain.RouteUnknown:
			unknown = true
		case indexdomain.RouteOther:
			other = true
		}
		return true
	}) || !roles[materialization.Recent] || !roles[materialization.Summary] || !unknown || !other {
		t.Fatal("Value.Top did not retain all table root roles plus unknown/other")
	}
	if topology.HeapState(rootKey, heap.Bottom()) != indexdomain.HeapStateNone || topology.HeapState(rootKey, heap.Top()) != indexdomain.HeapStateTop {
		t.Fatal("Heap.Top was conflated with no Heap fact")
	}
	foreignHeap, _, _, _, foreignRoot, _, _ := staticTopologyFixture(t)
	foreignOK := foreignRoot.Valid()
	if !foreignOK || topology.HeapState(rootKey, foreignHeap.Top()) != indexdomain.HeapStateInvalid {
		t.Fatal("foreign Heap.Top crossed topology owner fence")
	}
	foreignValues := topologyForeignValues(t)
	if topology.VisitReceiver(foreignValues.Top(), nil, func(indexdomain.Route) bool { return true }) {
		t.Fatal("foreign Value.Top crossed topology owner fence")
	}
	_, _, _, _, _, foreignCandidate, _ := staticTopologyFixture(t)
	if access, ok := topology.Access(foreignCandidate); ok || access != (indexdomain.Access{}) {
		t.Fatal("foreign index access crossed topology owner fence")
	}
}

func TestTopologyRejectsSameLinkResealedHeap(t *testing.T) {
	heap, values, calls, linked, _, _, mounts := staticTopologyFixture(t)
	if !values.OwnsHeapSchema(heap) {
		t.Fatal("Value did not retain the exact Heap schema handle")
	}
	resealed, resealedFailure := heapdomain.SealWithArtifacts(linked, mounts.heap)
	if resealedFailure != heapdomain.SealFailureNone || values.OwnsHeapSchema(resealed) {
		t.Fatal("independently resealed same-Link Heap was not distinguished")
	}
	if topology, ok := indexdomain.Seal(resealed, values, calls, mounts.packs); ok || topology != nil {
		t.Fatal("Topology accepted independently resealed Heap")
	}
	if topology, ok := indexdomain.Seal(heap, values, calls, mounts.packs); !ok || topology == nil {
		t.Fatal("Topology rejected exact Value/Heap schema pair")
	}
	foreignLink := sameContentLink(t, linked)
	foreignCalls, foreignCallsOK := calldomain.NewWithMountedArtifacts(foreignLink, indexMounts(t, foreignLink).call)
	if !foreignCallsOK || foreignLink.ContentID() != linked.ContentID() || foreignLink == linked {
		t.Fatal("same-content independent Call fixture")
	}
	if topology, ok := indexdomain.Seal(heap, values, foreignCalls, mounts.packs); ok || topology != nil {
		t.Fatal("Topology accepted same-content independent Call algebra")
	}
}

func TestTopologyOwnsAccessRejectsDuplicateSameContentTopology(t *testing.T) {
	heap, values, calls, _, _, candidate, mounts := staticTopologyFixture(t)
	primary, primaryOK := indexdomain.Seal(heap, values, calls, mounts.packs)
	duplicate, duplicateOK := indexdomain.Seal(heap, values, calls, mounts.packs)
	if !primaryOK || primary == nil || !duplicateOK || duplicate == nil || primary == duplicate {
		t.Fatal("same-content topology fixtures did not seal independently")
	}
	primaryAccess, primaryAccessOK := primary.Access(candidate)
	duplicateAccess, duplicateAccessOK := duplicate.Access(candidate)
	if !primaryAccessOK || !duplicateAccessOK {
		t.Fatal("topology access fixture unavailable")
	}
	if !primary.OwnsAccess(primaryAccess) || !duplicate.OwnsAccess(duplicateAccess) {
		t.Fatal("issuing topology rejected its own canonical access")
	}
	if primary.OwnsAccess(duplicateAccess) || duplicate.OwnsAccess(primaryAccess) {
		t.Fatal("duplicate same-content topology access crossed owner fence")
	}
}

func sameContentLink(t testing.TB, original *link.Link) *link.Link {
	t.Helper()
	contract, ok := original.Boundary().Target()
	if !ok || contract == nil {
		t.Fatal("original Link target")
	}
	mounts := original.Project().Mounts()
	shard, ok := mounts.At(0)
	if !ok {
		t.Fatal("original Link shard")
	}
	name, nameOK := mounts.Name(shard)
	program, programOK := mounts.Program(shard)
	if !nameOK || !programOK || program == nil {
		t.Fatal("original Link module")
	}
	module := linkproject.Module{Name: name, Program: program}
	clone, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{module}})
	if err != nil {
		t.Fatal(err)
	}
	return clone
}

func staticTopologyFixture(t testing.TB) (heapdomain.Schema, *valuedomain.Schema, *calldomain.Algebra, *link.Link, heapdomain.Key, heapdomain.IndexAccess, indexFixtureMounts) {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: "heap_index_topology.lua", Text: []byte(`local table = {}; return table.field`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{Semantics: domaincontract.NewSemantics(), Operations: []target.OperationSpec{{
		Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"require"}}},
		Input:    target.ValuesSpec{Tail: target.ValuesClosed},
		Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
		Effects:  target.RowSpec{Tail: target.RowClosed},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	heap, values, calls, mounts := indexSchemas(t, linked)
	var root heapdomain.Key
	for index := 0; index < heap.KeyCount(); index++ {
		candidate, ok := heap.KeyAt(index)
		receipt, source := candidate.AllocationReceipt()
		if ok && source && receipt.Kind() == heapdomain.AllocationTable {
			root = candidate
			break
		}
	}
	var access heapdomain.IndexAccess
	for index := 0; index < heap.IndexAccessCount(); index++ {
		candidate, ok := heap.IndexAccessAt(index)
		if !ok {
			t.Fatal("index access")
		}
		geometry, ok := heap.IndexAccessGeometry(candidate)
		if ok && geometry.Read {
			access = candidate
			break
		}
	}
	if !root.Valid() || access == (heapdomain.IndexAccess{}) {
		t.Fatal("static roots")
	}
	return heap, values, calls, linked, root, access, mounts
}

func topologyForeignValues(t testing.TB) *valuedomain.Schema {
	t.Helper()
	_, values, _, _, _, _, _ := staticTopologyFixture(t)
	return values
}

func freshTopologyFixture(t testing.TB) (heapdomain.Schema, *valuedomain.Schema, *calldomain.Algebra, *link.Link, heapdomain.Key, indexFixtureMounts) {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: "heap_index_fresh_topology.lua", Text: []byte(`return fresh()`)})
	if err != nil {
		t.Fatal(err)
	}
	binding := func(name string) target.BindingSpec {
		return target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{name}}
	}
	contract, err := target.Seal(&target.Spec{Semantics: domaincontract.NewSemantics(), InitialRoots: []target.InitialRootSpec{{Identity: "GlobalEnvRoot", Shape: target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}}}}, Operations: []target.OperationSpec{
		{Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"require"}}}, Input: target.ValuesSpec{Tail: target.ValuesClosed}, Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}}, Effects: target.RowSpec{Tail: target.RowClosed}},
		{Bindings: []target.BindingSpec{binding("fresh")}, Input: target.ValuesSpec{Tail: target.ValuesClosed}, Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Fixed: portableAnyTypes(1), Tail: target.ValuesClosed}, FreshResults: []target.FreshResultSpec{{Result: 0, Kind: schematype.FreshClassTable}}}}, Effects: target.RowSpec{Tail: target.RowClosed}},
		{Bindings: []target.BindingSpec{binding("other")}, Input: target.ValuesSpec{Tail: target.ValuesClosed}, Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Fixed: portableAnyTypes(1), Tail: target.ValuesClosed}, FreshResults: []target.FreshResultSpec{{Result: 0, Kind: schematype.FreshClassFunction}}}}, Effects: target.RowSpec{Tail: target.RowClosed}},
	}, InitialEntries: []target.InitialEntrySpec{{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable}, {Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "fresh"}, Value: target.InitialValueSpec{Kind: target.InitialValueOperation, Operation: binding("fresh")}, Mutability: target.InitialMutable}, {Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__link_absent"}, Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable}}, InitialBindings: []target.InitialBindingSpec{{Name: "_G", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}}, {Name: "fresh", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "fresh"}}}})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	heap, values, calls, mounts := indexSchemas(t, linked)
	for index := 0; index < heap.KeyCount(); index++ {
		root, ok := heap.KeyAt(index)
		if !ok {
			continue
		}
		_, _, _, _, fresh := root.FreshResultID()
		if fresh {
			return heap, values, calls, linked, root, mounts
		}
	}
	t.Fatal("fresh allocation root")
	return heapdomain.Schema{}, nil, nil, nil, heapdomain.Key{}, indexFixtureMounts{}
}
