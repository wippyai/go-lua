package boot

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/target/exactkey"
	"github.com/wippyai/go-lua/analysis/program/target/operation"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

func stringKey(value string) keyspace.LiteralValue {
	return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}
}

func TestCompileProducesImmutableBootOwner(t *testing.T) {
	admitted := vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"assert"}}
	denied := vocabulary.BindingSpec{Namespace: vocabulary.BindingModule, Owner: []string{"base"}, Member: []string{"load"}}
	keyValues := []keyspace.LiteralValue{
		stringKey("Global"), stringKey("Meta"), stringKey("assert"), stringKey("load"), stringKey("base"), stringKey("number"),
		stringKey("assert"), stringKey("load"),
	}
	keys, err := exactkey.Compile(keyValues)
	if err != nil {
		t.Fatalf("exactkey.Compile: %v", err)
	}
	geometry, err := operation.CompileGeometry(operation.Input{Operations: []operation.OperationInput{{
		Source: 0, Bindings: []vocabulary.BindingSpec{admitted},
		OutcomeValueSlots: []operation.OutcomeInput{{ValueSlots: 1}},
	}}})
	if err != nil {
		t.Fatalf("operation.CompileGeometry: %v", err)
	}
	operations, err := operation.CompileAnchors(geometry, keys)
	if err != nil {
		t.Fatalf("operation.CompileAnchors: %v", err)
	}
	input := Input{
		InitialRoots: []vocabulary.InitialRootSpec{
			{Identity: "Meta", Shape: vocabulary.BootShapeSpec{Aggregate: vocabulary.BootAggregateMetatable, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "Meta"}}},
			{Identity: "Global", Shape: vocabulary.BootShapeSpec{Aggregate: vocabulary.BootAggregateTable, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "Global"}}},
		},
		InitialEntries: []vocabulary.InitialEntrySpec{
			{Root: "Global", Key: stringKey("assert"), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueOperation, Operation: admitted}, Mutability: vocabulary.InitialMutable},
			{Root: "Global", Key: stringKey("load"), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueDeniedOperation, Operation: denied}, Mutability: vocabulary.InitialMutable},
			{Root: "Global", Key: stringKey("number"), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueInteger, Integer: 42}, Mutability: vocabulary.InitialFrozen},
			{Root: "Meta", Key: stringKey("number"), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "Meta"}, Mutability: vocabulary.InitialMutable},
		},
		InitialMetatables: []vocabulary.InitialMetatableAttachmentSpec{{Base: vocabulary.InitialValueString, Metatable: "Meta"}},
		Operations:        operations,
		Keys:              keys,
	}
	table, err := Compile(input)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	rows := table.CountRows()
	ids := denominator.GeneratedTargetIDs()
	check := func(id schema.EntryID, want uint64) {
		t.Helper()
		if got, ok := rows.Value(id); !ok || got != want {
			t.Fatalf("boot owner row %v = %d/%v, want %d/true", id, got, ok, want)
		}
	}
	check(ids.TargetBoot, 2)
	check(ids.TargetBootEntry, 4)
	check(ids.TargetBootMetatableAttachment, 1)
	check(ids.TargetBootBinding, 0)
	global, ok := table.InitialRootByIdentity("Global")
	if !ok {
		t.Fatal("Global root missing")
	}
	if got, ok := table.InitialRootIdentity(global); !ok || got != "Global" {
		t.Fatalf("root identity = %q/%v", got, ok)
	}
	shape, ok := table.InitialRootBootShape(global)
	if !ok {
		t.Fatal("Global shape missing")
	}
	if aggregate, ok := table.BootShapeAggregate(shape); !ok || aggregate != vocabulary.BootAggregateTable {
		t.Fatalf("Global aggregate = %d/%v", aggregate, ok)
	}
	assertKey, _ := keys.Handle(stringKey("assert"))
	assertValue, _, ok := table.InitialEntry(global, assertKey)
	if !ok {
		t.Fatal("assert entry missing")
	}
	if operation, ok := table.InitialValueOperation(assertValue); !ok || operation != 1 {
		t.Fatalf("assert operation = %d/%v", operation, ok)
	}
	loadKey, _ := keys.Handle(stringKey("load"))
	loadValue, _, ok := table.InitialEntry(global, loadKey)
	if !ok {
		t.Fatal("load entry missing")
	}
	if namespace, ok := table.InitialValueDeniedNamespace(loadValue); !ok || namespace != vocabulary.BindingModule {
		t.Fatalf("load namespace = %d/%v", namespace, ok)
	}
	owner, ownerOK := table.InitialValueDeniedOwnerAt(loadValue, 0)
	if table.InitialValueDeniedOwnerCount(loadValue) != 1 || !ownerOK || owner != "base" {
		t.Fatal("load owner path mismatch")
	}
	if table.InitialValueDeniedMemberCount(loadValue) != 1 {
		t.Fatal("load member count mismatch")
	}
	if member, ok := table.InitialValueDeniedMemberAt(loadValue, 0); !ok || member != "load" {
		t.Fatalf("load member = %q/%v", member, ok)
	}
	meta, _ := table.InitialRootByIdentity("Meta")
	if base, attached, ok := table.InitialMetatableAttachmentAt(0); !ok || base != vocabulary.InitialValueString || attached != meta {
		t.Fatalf("metatable attachment = %d/%d/%v", base, attached, ok)
	}
	if id, ok := table.InitialValueContentID(assertValue); !ok || !id.Available() {
		t.Fatal("admitted value identity unavailable")
	}
	if id, ok := table.BootRelationID(); !ok || !id.Available() {
		t.Fatal("boot identity unavailable")
	}

	// Compile copies authoring geometry. Mutating the caller's nested binding
	// path after the handoff cannot change a sealed denied-value query.
	input.InitialEntries[1].Value.Operation.Owner[0] = "mutated"
	if owner, ok := table.InitialValueDeniedOwnerAt(loadValue, 0); !ok || owner != "base" {
		t.Fatalf("sealed owner path = %q/%v, want base", owner, ok)
	}

	var zero identity.ContentID
	if id, ok := table.InitialValueContentID(0); ok || id != zero {
		t.Fatal("zero value identity succeeded")
	}
}
