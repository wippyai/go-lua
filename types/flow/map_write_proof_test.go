package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestApplyMapWriteProofDecouplesKeyArrayFromReadbackAdmission(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(1), "node_order")
	tablePath := constraint.NewPath(cfg.SymbolID(2), "edges")
	keyPath := constraint.NewPath(cfg.SymbolID(3), "node_id")
	arrayKey := StablePathKey(arrayPath)
	tableKey := StablePathKey(tablePath)
	keyKey := StablePathKey(keyPath)

	state := PointStateDomain.Top()
	state.KeyPresence = state.KeyPresence.WithPendingKeyArray(arrayKey, tableKey, keyKey)

	proof := testMapWriteProof(
		t,
		tablePath,
		keyPath,
		product.FromType(typ.Any),
		constraint.Path{},
		product.FromType(typ.String),
		false,
	)
	if changed := ApplyMapWriteProof(&state, proof); !changed {
		t.Fatal("ApplyMapWriteProof reported no change")
	}
	if !state.KeyPresence.Has(tableKey, keyKey) {
		t.Fatalf("key presence missing in %s", state.KeyPresence.Format())
	}
	if tables := state.KeyPresence.KeyArrayTables(arrayKey); len(tables) != 1 || tables[0] != tableKey {
		t.Fatalf("key-array tables = %v, want %s", tables, tableKey)
	}
	values := state.KeyPresence.KeyArrayValues(arrayKey, tableKey)
	if len(values) != 1 || !product.Domain.Equal(values[0], product.FromType(typ.String)) {
		t.Fatalf("key-array values = %v, want string", values)
	}
	if len(state.IndexWrites.Entries()) != 0 {
		t.Fatalf("readback fact was published despite inadmissible key: %s", state.IndexWrites.Format())
	}
}

func TestApplyMapWriteProofPublishesReadbackWhenAdmissible(t *testing.T) {
	tablePath := constraint.NewPath(cfg.SymbolID(2), "edges")
	keyPath := constraint.NewPath(cfg.SymbolID(3), "node_id")

	state := PointStateDomain.Top()
	proof := testMapWriteProof(
		t,
		tablePath,
		keyPath,
		product.FromType(typ.String),
		constraint.Path{},
		product.FromType(typ.Number),
		false,
	)
	ApplyMapWriteProof(&state, proof)

	value, ok := state.IndexWrites.AdmissionAtAddress(testIndexWriteAddressQuery(t, tablePath, keyPath, typ.String, constraint.Path{}))
	if !ok || !product.Domain.Equal(value, product.FromType(typ.Number)) {
		t.Fatalf("readback = %v/%v, want number", value, ok)
	}
}

func TestApplyMapWriteProofWidensExistingKeyArrayValueForPresentWrite(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(11), "node_order")
	tablePath := constraint.NewPath(cfg.SymbolID(12), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(13), "node_id")
	arrayKey := StablePathKey(arrayPath)
	tableKey := StablePathKey(tablePath)
	oldValue := product.FromType(typ.String)
	writtenValue := product.FromType(typ.Number)

	state := PointStateDomain.Top()
	state.KeyPresence = state.KeyPresence.WithKeyArrayValue(arrayKey, tableKey, oldValue)

	proof := testMapWriteProof(
		t,
		tablePath,
		keyPath,
		product.FromType(typ.String),
		constraint.Path{},
		writtenValue,
		false,
	)
	if changed := ApplyMapWriteProof(&state, proof); !changed {
		t.Fatal("ApplyMapWriteProof reported no change")
	}

	values := state.KeyPresence.KeyArrayValues(arrayKey, tableKey)
	want := product.Domain.Join(oldValue, writtenValue)
	if len(values) != 1 || !product.Domain.Equal(values[0], want) {
		t.Fatalf("key-array values = %v, want %v", values, want.ProjectValue())
	}
}

func TestApplyMapWriteProofCoversTrackedAppendEvent(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(21), "node_order")
	tablePath := constraint.NewPath(cfg.SymbolID(22), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(23), "node_id")
	arrayKey := StablePathKey(arrayPath)
	tableKey := StablePathKey(tablePath)
	keyKey := StablePathKey(keyPath)
	nodeValue := product.FromType(typ.String)

	state := PointStateDomain.Top()
	state.KeyPresence = state.KeyPresence.
		WithAppendHistoryBase(arrayKey).
		WithAppendHistoryEvent(arrayKey, keyKey)

	proof := testMapWriteProof(
		t,
		tablePath,
		keyPath,
		product.FromType(typ.String),
		constraint.Path{},
		nodeValue,
		false,
	)
	if changed := ApplyMapWriteProof(&state, proof); !changed {
		t.Fatal("ApplyMapWriteProof reported no change")
	}

	values := state.KeyPresence.KeyArrayValues(arrayKey, tableKey)
	if len(values) != 1 || !product.Domain.Equal(values[0], nodeValue) {
		t.Fatalf("tracked append coverage values = %v, want string; facts=%s", values, state.KeyPresence.Format())
	}
}

func TestApplyKeyArrayProofPublishesArrayTable(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(31), "node_order")
	tablePath := constraint.NewPath(cfg.SymbolID(32), "nodes")
	arrayKey := StablePathKey(arrayPath)
	tableKey := StablePathKey(tablePath)

	state := PointStateDomain.Top()
	if changed := ApplyKeyArrayProof(&state, KeyArrayProof{
		Array: testStableAddressPath(t, arrayPath),
		Table: testStableAddressPath(t, tablePath),
	}); !changed {
		t.Fatal("ApplyKeyArrayProof reported no change")
	}

	if tables := state.KeyPresence.KeyArrayTables(arrayKey); len(tables) != 1 || tables[0] != tableKey {
		t.Fatalf("key-array tables = %v, want %s", tables, tableKey)
	}
}

func TestApplyEmptyKeyArrayProofPublishesEmptyArray(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(35), "node_order")
	arrayKey := StablePathKey(arrayPath)

	state := PointStateDomain.Top()
	if changed := ApplyEmptyKeyArrayProof(&state, EmptyKeyArrayProof{
		Array: testStableAddressPath(t, arrayPath),
	}); !changed {
		t.Fatal("ApplyEmptyKeyArrayProof reported no change")
	}

	if !state.KeyPresence.HasEmptyKeyArray(arrayKey) || !state.KeyPresence.HasAppendHistoryBase(arrayKey) {
		t.Fatalf("empty key-array proof missing empty/base facts: %s", state.KeyPresence.Format())
	}
}

func TestApplyIndexedKeyArrayIterationProofPublishesTableKey(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(41), "node_order")
	tablePath := constraint.NewPath(cfg.SymbolID(42), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(43), "node_id")
	tableKey := StablePathKey(tablePath)
	keyKey := StablePathKey(keyPath)

	state := PointStateDomain.Top()
	ApplyKeyArrayProof(&state, KeyArrayProof{
		Array: testStableAddressPath(t, arrayPath),
		Table: testStableAddressPath(t, tablePath),
	})

	tables, changed := ApplyIndexedKeyArrayIterationProof(&state, testStableAddressPath(t, arrayPath), testStableAddressPath(t, keyPath))
	if !changed {
		t.Fatal("ApplyIndexedKeyArrayIterationProof reported no change")
	}
	if len(tables) != 1 || tables[0] != tableKey {
		t.Fatalf("iteration tables = %v, want %s", tables, tableKey)
	}
	if !state.KeyPresence.Has(tableKey, keyKey) {
		t.Fatalf("iteration did not publish table/key presence: %s", state.KeyPresence.Format())
	}
}

func TestApplyKeyArrayValueProofPublishesCoverage(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(51), "node_order")
	tablePath := constraint.NewPath(cfg.SymbolID(52), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(53), "node_id")
	arrayKey := StablePathKey(arrayPath)
	tableKey := StablePathKey(tablePath)
	value := product.FromType(typ.String)

	state := PointStateDomain.Top()
	state.KeyPresence = state.KeyPresence.WithAppendHistoryBase(arrayKey)

	if changed := ApplyKeyArrayValueProof(&state, KeyArrayValueProof{
		Array:        testStableAddressPath(t, arrayPath),
		Table:        testStableAddressPath(t, tablePath),
		Value:        value,
		AppendKey:    testStableAddressPath(t, keyPath),
		HasAppendKey: true,
	}); !changed {
		t.Fatal("ApplyKeyArrayValueProof reported no change")
	}

	values := state.KeyPresence.KeyArrayValues(arrayKey, tableKey)
	if len(values) != 1 || !product.Domain.Equal(values[0], value) {
		t.Fatalf("key-array values = %v, want string", values)
	}
	if got, ok := state.KeyPresence.AppendHistoryCoverageValue(arrayKey, tableKey); !ok || !product.Domain.Equal(got, value) {
		t.Fatalf("append coverage = %v/%v, want string", got, ok)
	}
}

func TestApplyPendingKeyArrayProofAllowsWildcardTable(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(61), "node_order")
	keyPath := constraint.NewPath(cfg.SymbolID(62), "node_id")
	arrayKey := StablePathKey(arrayPath)
	keyKey := StablePathKey(keyPath)

	state := PointStateDomain.Top()
	if changed := ApplyPendingKeyArrayProof(&state, PendingKeyArrayProof{
		Array: testStableAddressPath(t, arrayPath),
		Key:   testStableAddressPath(t, keyPath),
	}); !changed {
		t.Fatal("ApplyPendingKeyArrayProof reported no change")
	}

	entries := state.KeyPresence.PendingKeyArrayEntries()
	if len(entries) != 1 || entries[0].Array != arrayKey || entries[0].Key != keyKey || entries[0].Table != "" {
		t.Fatalf("pending key-array entries = %v, want wildcard table for array/key", entries)
	}
}

func TestApplyAppendKeyProofPublishesHistoryEvent(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(71), "node_order")
	keyPath := constraint.NewPath(cfg.SymbolID(72), "node_id")
	arrayKey := StablePathKey(arrayPath)
	keyKey := StablePathKey(keyPath)

	state := PointStateDomain.Top()
	state.KeyPresence = state.KeyPresence.WithAppendHistoryBase(arrayKey)
	if changed := ApplyAppendKeyProof(&state, AppendKeyProof{
		Array: testStableAddressPath(t, arrayPath),
		Key:   testStableAddressPath(t, keyPath),
	}); !changed {
		t.Fatal("ApplyAppendKeyProof reported no change")
	}

	if entries := state.KeyPresence.AppendedKeyEntries(); len(entries) != 1 || entries[0].Array != arrayKey || entries[0].Key != keyKey {
		t.Fatalf("appended key entries = %v, want array/key", entries)
	}
	if events := state.KeyPresence.AppendHistoryEventEntries(); len(events) != 1 || events[0].Array != arrayKey || events[0].Key != keyKey {
		t.Fatalf("append history events = %v, want array/key", events)
	}
}

func TestApplyAppendElementFieldOriginProofPublishesBaseAndOrigin(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(81), "items")
	sourcePath := constraint.NewPath(cfg.SymbolID(82), "source")
	arrayKey := StablePathKey(arrayPath)
	field := []constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}}

	state := PointStateDomain.Top()
	if changed := ApplyAppendElementFieldOriginProof(&state, AppendElementFieldOriginProof{
		Array:  testStableAddressPath(t, arrayPath),
		Field:  field,
		Source: testStableAddressPath(t, sourcePath),
	}); !changed {
		t.Fatal("ApplyAppendElementFieldOriginProof reported no change")
	}

	if !state.KeyPresence.HasAppendHistoryBase(arrayKey) {
		t.Fatalf("append-history base missing: %s", state.KeyPresence.Format())
	}
	origins := state.KeyPresence.AppendElementFieldOriginEntries()
	if len(origins) != 1 || origins[0].Array != arrayKey || origins[0].Field == "" || origins[0].Source != StablePathKey(sourcePath) {
		t.Fatalf("append element origins = %v, want array field source", origins)
	}
}

func testMapWriteProof(
	t *testing.T,
	tablePath constraint.Path,
	keyPath constraint.Path,
	keyValue product.AbstractValue,
	valuePath constraint.Path,
	value product.AbstractValue,
	allowOpaqueKeyReadback bool,
) MapWriteProof {
	t.Helper()
	table := testStableAddressPath(t, tablePath)
	proof := MapWriteProof{
		Table:                  table,
		KeyValue:               keyValue,
		Value:                  value,
		AllowOpaqueKeyReadback: allowOpaqueKeyReadback,
	}
	if !keyPath.IsEmpty() {
		proof.Key = testStableAddressPath(t, keyPath)
		proof.HasKey = true
	}
	if !valuePath.IsEmpty() {
		proof.ValuePath = testStableAddressPath(t, valuePath)
		proof.HasValuePath = true
	}
	return proof
}

func TestIndexedIteratorKeyArrayReadbackDerivesValueFromStableFacts(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(1), "node_order")
	tablePath := constraint.NewPath(cfg.SymbolID(2), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(3), "current_node_id")

	keyPresence := KeyPresenceFacts{}.
		WithKeyArrayValueAddresses(testStableAddressPath(t, arrayPath), testStableAddressPath(t, tablePath), product.FromType(typ.String))
	origins := ValueOriginFacts{}.
		WithAddresses(testStableAddressPath(t, keyPath), testStableAddressPath(t, arrayPath), ValueOriginIndexedIterator, 1)

	got, ok := IndexedIteratorKeyArrayReadback(keyPresence, origins, testStableAddressPath(t, tablePath), testStableAddressPath(t, keyPath))
	if !ok || !product.Domain.Equal(got, product.FromType(typ.String)) {
		t.Fatalf("readback = %v/%v, want string", got, ok)
	}
}

func TestIndexedIteratorKeyArrayReadbackFollowsAssignmentAlias(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(1), "node_order")
	tablePath := constraint.NewPath(cfg.SymbolID(2), "edges")
	iterKeyPath := constraint.NewPath(cfg.SymbolID(3), "node_id")
	aliasPath := constraint.NewPath(cfg.SymbolID(4), "current_node_id")

	keyPresence := KeyPresenceFacts{}.
		WithKeyArrayValueAddresses(testStableAddressPath(t, arrayPath), testStableAddressPath(t, tablePath), product.FromType(typ.Number))
	origins := ValueOriginFacts{}.
		WithAddresses(testStableAddressPath(t, iterKeyPath), testStableAddressPath(t, arrayPath), ValueOriginIndexedIterator, 1).
		WithAddresses(testStableAddressPath(t, aliasPath), testStableAddressPath(t, iterKeyPath), ValueOriginAssignmentAlias, 0)

	got, ok := IndexedIteratorKeyArrayReadback(keyPresence, origins, testStableAddressPath(t, tablePath), testStableAddressPath(t, aliasPath))
	if !ok || !product.Domain.Equal(got, product.FromType(typ.Number)) {
		t.Fatalf("alias readback = %v/%v, want number", got, ok)
	}
}
