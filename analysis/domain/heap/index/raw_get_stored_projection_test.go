package index

import (
	"testing"

	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestRawGetStoredProjectionFollowsTypedHeapContainment(t *testing.T) {
	linked, heap, schema := rawGetStoredProjectionFixture(t)
	composition := engine.NewComposition()
	values, valuesOK := valueowner.Declare(composition, rawGetKey(80), rawGetKey(81), schema)
	if !valuesOK {
		t.Fatal("Value owner")
	}
	rule := &RawGetRule{values: values}

	var key heapdomain.Key
	for index := 0; index < heap.KeyCount(); index++ {
		candidate, candidateOK := heap.KeyAt(index)
		if candidateOK && candidate.Kind() == heapdomain.RootAllocation {
			key = candidate
			break
		}
	}
	keyOK := key.Valid()
	heapReference, referenceOK := heap.Reference(key, materialization.Recent)
	exactContainment, exactOK := heap.ContainmentExact(heapReference)
	noneContainment, noneOK := heap.ContainmentNone()
	unknownContainment, unknownOK := heap.ContainmentUnknown()
	exactAtom, exactAtomOK := schema.Allocation(key, materialization.Recent)
	numberAtom, numberOK := schema.OpaqueKind(runtimekind.Number)
	endpoint, endpointOK := linked.Boundary().Endpoints().At(0)
	endpointAtom, endpointAtomOK := schema.Endpoint(endpoint)
	opaqueReference, opaqueOK := schema.OpaqueReference(valuedomain.ReferenceTable)
	callableAtom, callableOK := rawGetStoredCallable(schema, linked)
	if !keyOK || !referenceOK || !exactOK || !noneOK || !unknownOK ||
		!exactAtomOK || !numberOK || !endpointOK || !endpointAtomOK || !opaqueOK || !callableOK {
		t.Fatal("stored projection fixture denominator")
	}

	unknownInput, inputOK := schema.Alternatives(callableAtom, endpointAtom, opaqueReference)
	unknownResult, unknownPresent, unknownValid := rawGetStoredReduction(rule, unknownContainment, unknownInput)
	if !inputOK || !unknownValid || !unknownPresent || schema.Equal(unknownResult, schema.Bottom()) || !schema.Equal(unknownResult, unknownInput) {
		t.Fatal("unknown containment dropped callable, endpoint, or opaque identity")
	}

	noneInput, inputOK := schema.Alternatives(numberAtom, exactAtom, callableAtom, endpointAtom, opaqueReference)
	wantNone, wantNoneOK := schema.Singleton(numberAtom)
	noneResult, nonePresent, noneValid := rawGetStoredReduction(rule, noneContainment, noneInput)
	if !inputOK || !wantNoneOK || !noneValid || !nonePresent || !schema.Equal(noneResult, wantNone) {
		t.Fatal("none containment retained a reference or dropped its scalar")
	}

	exactResult, exactPresent, exactValid := rawGetStoredReduction(rule, exactContainment, noneInput)
	wantExact, wantExactOK := schema.Singleton(exactAtom)
	if !wantExactOK || !exactValid || !exactPresent || !schema.Equal(exactResult, wantExact) {
		t.Fatal("exact containment selected anything except its tracked child")
	}
}

func TestRawGetBootCallableUsesUnknownContainmentProjection(t *testing.T) {
	linked, heap, schema := rawGetStoredProjectionFixture(t)
	composition := engine.NewComposition()
	values, valuesOK := valueowner.Declare(composition, rawGetKey(82), rawGetKey(83), schema)
	if !valuesOK {
		t.Fatal("Value owner")
	}
	rule := &RawGetRule{values: values}

	for index := 0; index < heap.BootEntryCount(); index++ {
		entry, entryOK := heap.BootEntryAt(index)
		key, keyOK := entry.Key()
		root, rootOK := key.BootRoot()
		raw, payload, projectionOK := entry.Projection()
		initial, initialOK := payload.InitialValue()
		_, _, callable := linked.Boundary().Seeds().BootstrapCallable(initial)
		if !entryOK || !keyOK || !rootOK || !projectionOK || !initialOK || raw != heapdomain.RawPresent || !callable {
			continue
		}
		containment, containmentOK := entry.ValueContainment()
		input, inputOK := schema.TargetInitial(root, initial)
		result, present, valid := rawGetStoredReduction(rule, containment, input)
		if !containmentOK || containment.Kind() != heapdomain.ContainmentUnknown || !inputOK || !valid || !present ||
			schema.Equal(result, schema.Bottom()) || !schema.Equal(result, input) {
			t.Fatal("boot callable did not survive its Heap-issued unknown containment")
		}
		return
	}
	t.Fatal("stored projection fixture has no callable boot entry")
}

func rawGetStoredReduction(rule *RawGetRule, containment heapdomain.Containment, input valuedomain.Value) (valuedomain.Value, bool, bool) {
	result := rule.values.Schema().Bottom()
	present := false
	valid := rule.reduceAndJoin(containment, input, &result, &present)
	return result, present, valid
}

func rawGetStoredCallable(schema *valuedomain.Schema, linked *link.Link) (valuedomain.Atom, bool) {
	contract, ok := linked.Boundary().Target()
	if !ok || contract == nil {
		return valuedomain.Atom{}, false
	}
	for index := 0; index < contract.InitialEntryCount(); index++ {
		_, _, initial, _, entryOK := contract.InitialEntryAt(index)
		seed, _, callable := linked.Boundary().Seeds().BootstrapCallable(initial)
		if entryOK && callable {
			return schema.Callable(seed)
		}
	}
	return valuedomain.Atom{}, false
}

func rawGetStoredProjectionFixture(t testing.TB) (*link.Link, heapdomain.Schema, *valuedomain.Schema) {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: "raw_get_stored_projection.lua", Text: []byte("local object = {}; return object, 1")})
	if err != nil {
		t.Fatal(err)
	}
	callable := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"admitted"}}
	endpoint := target.BindingSpec{Namespace: target.BindingProvider, Owner: []string{"actor"}, Member: []string{"send"}}
	operation := func(binding target.BindingSpec) target.OperationSpec {
		return target.OperationSpec{
			Bindings: []target.BindingSpec{binding},
			Input:    target.ValuesSpec{Tail: target.ValuesClosed},
			Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
			Effects:  target.RowSpec{Tail: target.RowClosed},
		}
	}
	contract, err := target.Seal(&target.Spec{
		InitialRoots: []target.InitialRootSpec{{
			Identity: "GlobalEnvRoot",
			Shape: target.BootShapeSpec{
				Aggregate: target.BootAggregateTable,
				Value:     target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"},
			},
		}},
		Operations: []target.OperationSpec{operation(callable), operation(endpoint)},
		InitialEntries: []target.InitialEntrySpec{{
			Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "admitted"},
			Value: target.InitialValueSpec{Kind: target.InitialValueOperation, Operation: callable}, Mutability: target.InitialMutable,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{
		Target: contract, Modules: []linkproject.Module{{Name: "main", Program: p}},
		EndpointRequests: []linkboundary.EndpointRequest{{Identity: "actor.send", Binding: endpoint}},
	})
	if err != nil {
		t.Fatal(err)
	}
	heap, heapOK := heapdomain.Seal(linked)
	values, valuesOK := valuedomain.Seal(linked, heap)
	if !heapOK || !valuesOK {
		t.Fatal("stored projection schemas")
	}
	return linked, heap, values
}
