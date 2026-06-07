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

func TestApplyMapWritePathTransactionLowersStructuredPaths(t *testing.T) {
	tablePath := constraint.NewPath(cfg.SymbolID(8), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(9), "node_id")
	valuePath := constraint.NewPath(cfg.SymbolID(10), "node")
	state := PointState{}

	if changed := ApplyMapWritePathTransaction(&state, MapWritePathTransaction{
		TablePath:              tablePath,
		KeyPath:                keyPath,
		KeyValue:               product.FromType(typ.LiteralString("n1")),
		ValuePath:              valuePath,
		Value:                  product.FromType(typ.Number),
		AllowOpaqueKeyReadback: true,
	}); !changed {
		t.Fatal("ApplyMapWritePathTransaction reported unchanged")
	}

	if !state.KeyPresence.Has(StablePathKey(tablePath), StablePathKey(keyPath)) {
		t.Fatalf("path proof did not publish key presence: %s", state.KeyPresence.Format())
	}
	got, ok := state.IndexWrites.AdmissionAtAddress(testIndexWriteAddressQuery(t, tablePath, keyPath, typ.LiteralString("n1"), valuePath))
	if !ok {
		t.Fatal("path proof readback missing")
	}
	if !product.Domain.Equal(got, product.FromType(typ.Number)) {
		t.Fatalf("path proof readback = %v, want number", got.ProjectValue())
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

func TestApplyKeyPresenceAliasProofCopiesMembershipAndValuePath(t *testing.T) {
	tablePath := constraint.NewPath(cfg.SymbolID(25), "nodes")
	sourcePath := constraint.NewPath(cfg.SymbolID(26), "source_id")
	targetPath := constraint.NewPath(cfg.SymbolID(27), "target_id")
	valuePath := constraint.NewPath(cfg.SymbolID(28), "node")
	tableKey := StablePathKey(tablePath)
	targetKey := StablePathKey(targetPath)
	valueKey := StablePathKey(valuePath)

	state := PointStateDomain.Top()
	state.KeyPresence = state.KeyPresence.
		WithValueAddresses(
			testStableAddressPath(t, tablePath),
			testStableAddressPath(t, sourcePath),
			testStableAddressPath(t, valuePath),
		)

	if changed := ApplyKeyPresenceAliasProof(&state, KeyPresenceAliasProof{
		SourceKey: testStableAddressPath(t, sourcePath),
		TargetKey: testStableAddressPath(t, targetPath),
	}); !changed {
		t.Fatal("ApplyKeyPresenceAliasProof reported no change")
	}

	if !state.KeyPresence.Has(tableKey, targetKey) || !state.KeyPresence.HasValue(tableKey, targetKey, valueKey) {
		t.Fatalf("alias proof did not copy membership and value path: %s", state.KeyPresence.Format())
	}
}

func TestApplyKeyPresenceAliasPathProofCopiesMembership(t *testing.T) {
	tablePath := constraint.NewPath(cfg.SymbolID(29), "nodes")
	sourcePath := constraint.NewPath(cfg.SymbolID(30), "source_id")
	targetPath := constraint.NewPath(cfg.SymbolID(31), "target_id")
	tableKey := StablePathKey(tablePath)
	targetKey := StablePathKey(targetPath)

	state := PointStateDomain.Top()
	state.KeyPresence = state.KeyPresence.WithAddresses(
		testStableAddressPath(t, tablePath),
		testStableAddressPath(t, sourcePath),
	)

	if !ApplyKeyPresenceAliasPathProof(&state, KeyPresenceAliasPathProof{
		SourcePath: sourcePath,
		TargetPath: targetPath,
	}) {
		t.Fatal("ApplyKeyPresenceAliasPathProof reported unchanged")
	}
	if !state.KeyPresence.Has(tableKey, targetKey) {
		t.Fatalf("path alias did not copy key presence: %s", state.KeyPresence.Format())
	}
}

func TestApplyKeyProvenancePathProofPublishesKeyPresence(t *testing.T) {
	tablePath := constraint.NewPath(cfg.SymbolID(101), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(102), "node_id")
	state := PointState{}

	_, changed := ApplyKeyProvenancePathProof(&state, KeyProvenancePathProof{
		Kind:      KeyProvenanceKeyedIteration,
		TablePath: tablePath,
		KeyPath:   keyPath,
	})

	if !changed {
		t.Fatal("ApplyKeyProvenancePathProof reported unchanged")
	}
	if !state.KeyPresence.Has(StablePathKey(tablePath), StablePathKey(keyPath)) {
		t.Fatalf("key presence missing: %s", state.KeyPresence.Format())
	}
}

func TestApplyKeyProvenanceTransactionPublishesKeyPresence(t *testing.T) {
	table := testStableAddressPath(t, constraint.NewPath(cfg.SymbolID(103), "nodes"))
	key := testStableAddressPath(t, constraint.NewPath(cfg.SymbolID(104), "node_id"))
	state := PointState{}

	_, changed := ApplyKeyProvenanceTransaction(&state, KeyProvenanceTransaction{
		Kind:  KeyProvenanceKeyedIteration,
		Table: table,
		Key:   key,
	})

	if !changed {
		t.Fatal("ApplyKeyProvenanceTransaction reported unchanged")
	}
	if !state.KeyPresence.HasAddresses(table, key) {
		t.Fatalf("key presence missing: %s", state.KeyPresence.Format())
	}
}

func TestApplyKeyProvenancePathProofReturnsIndexedKeyDomain(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(111), "keys")
	tablePath := constraint.NewPath(cfg.SymbolID(112), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(113), "node_id")
	state := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(keyPath.Symbol): product.FromType(typ.String),
			SymbolValueKey(tablePath.Symbol): product.FromType(
				typ.NewRecord().Field("id", typ.Number).Build(),
			),
		},
	}
	state.KeyPresence = state.KeyPresence.WithKeyArray(StablePathKey(arrayPath), StablePathKey(tablePath))

	result, changed := ApplyKeyProvenancePathProof(&state, KeyProvenancePathProof{
		Kind:      KeyProvenanceIndexedKeyArrayIteration,
		ArrayPath: arrayPath,
		KeyPath:   keyPath,
	})

	if !changed {
		t.Fatal("ApplyKeyProvenancePathProof reported unchanged")
	}
	wantKeyDomain := product.FromType(typ.LiteralString("id"))
	if result.KeyRefinementPath.Key() != keyPath.Key() || !product.Domain.Equal(result.KeyRefinementValue, wantKeyDomain) {
		t.Fatalf("key refinement = %s/%v, want %s/%v", result.KeyRefinementPath.Key(), result.KeyRefinementValue.ProjectValue(), keyPath.Key(), wantKeyDomain.ProjectValue())
	}
}

func TestApplyKeyProvenanceTransactionReturnsIndexedKeyDomain(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(114), "keys")
	tablePath := constraint.NewPath(cfg.SymbolID(115), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(116), "node_id")
	array := testStableAddressPath(t, arrayPath)
	table := testStableAddressPath(t, tablePath)
	key := testStableAddressPath(t, keyPath)
	state := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(keyPath.Symbol): product.FromType(typ.String),
			SymbolValueKey(tablePath.Symbol): product.FromType(
				typ.NewRecord().Field("id", typ.Number).Build(),
			),
		},
	}
	state.KeyPresence = state.KeyPresence.WithKeyArrayAddresses(array, table)

	result, changed := ApplyKeyProvenanceTransaction(&state, KeyProvenanceTransaction{
		Kind:  KeyProvenanceIndexedKeyArrayIteration,
		Array: array,
		Key:   key,
	})

	if !changed {
		t.Fatal("ApplyKeyProvenanceTransaction reported unchanged")
	}
	wantKeyDomain := product.FromType(typ.LiteralString("id"))
	if !result.KeyRefinementAddress.Equal(key) || !product.Domain.Equal(result.KeyRefinementValue, wantKeyDomain) {
		t.Fatalf("key refinement = %s/%v, want %s/%v", result.KeyRefinementAddress.Key(), result.KeyRefinementValue.ProjectValue(), key.Key(), wantKeyDomain.ProjectValue())
	}
}

func TestApplyKeyArrayElementKeyProofPublishesTargetKey(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(29), "node_order")
	tablePath := constraint.NewPath(cfg.SymbolID(30), "nodes")
	targetPath := constraint.NewPath(cfg.SymbolID(31), "node_id")
	tableKey := StablePathKey(tablePath)
	targetKey := StablePathKey(targetPath)

	state := PointStateDomain.Top()
	state.KeyPresence = state.KeyPresence.WithKeyArrayAddresses(
		testStableAddressPath(t, arrayPath),
		testStableAddressPath(t, tablePath),
	)

	result, changed := ApplyKeyArrayElementKeyProof(&state, KeyArrayElementKeyProof{
		Array:     testStableAddressPath(t, arrayPath),
		TargetKey: testStableAddressPath(t, targetPath),
	})
	if !changed {
		t.Fatal("ApplyKeyArrayElementKeyProof reported no change")
	}
	if len(result.Tables) != 1 || result.Tables[0].Key() != tableKey {
		t.Fatalf("proof tables = %v, want %s", result.Tables, tableKey)
	}
	if !state.KeyPresence.Has(tableKey, targetKey) {
		t.Fatalf("proof did not publish table/target key presence: %s", state.KeyPresence.Format())
	}
}

func TestApplyKeyArrayElementKeyPathProofLowersStructuredPaths(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(129), "node_order")
	tablePath := constraint.NewPath(cfg.SymbolID(130), "nodes")
	targetPath := constraint.NewPath(cfg.SymbolID(131), "node_id")
	tableKey := StablePathKey(tablePath)
	targetKey := StablePathKey(targetPath)

	state := PointStateDomain.Top()
	state.KeyPresence = state.KeyPresence.WithKeyArrayAddresses(
		testStableAddressPath(t, arrayPath),
		testStableAddressPath(t, tablePath),
	)

	result, changed := ApplyKeyArrayElementKeyPathProof(&state, KeyArrayElementKeyPathProof{
		ArrayPath:     arrayPath,
		TargetKeyPath: targetPath,
	})
	if !changed {
		t.Fatal("ApplyKeyArrayElementKeyPathProof reported no change")
	}
	if len(result.Tables) != 1 || result.Tables[0].Key() != tableKey {
		t.Fatalf("path proof tables = %v, want %s", result.Tables, tableKey)
	}
	if !state.KeyPresence.Has(tableKey, targetKey) {
		t.Fatalf("path proof did not publish table/target key presence: %s", state.KeyPresence.Format())
	}
}

func TestApplyKeyArrayElementKeyProofPublishesReadbackAdmission(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(32), "node_order")
	tablePath := constraint.NewPath(cfg.SymbolID(33), "nodes")
	targetPath := constraint.NewPath(cfg.SymbolID(34), "node_id")
	array := testStableAddressPath(t, arrayPath)
	table := testStableAddressPath(t, tablePath)
	target := testStableAddressPath(t, targetPath)
	value := product.FromType(typ.String)
	state := PointState{
		KeyPresence: KeyPresenceFacts{}.
			WithKeyArrayValueAddresses(array, table, value),
	}

	if _, changed := ApplyKeyArrayElementKeyProof(&state, KeyArrayElementKeyProof{
		Array:     array,
		TargetKey: target,
		KeyValue:  product.FromType(typ.LiteralString("node-1")),
	}); !changed {
		t.Fatal("ApplyKeyArrayElementKeyProof reported no change")
	}
	got, ok := state.IndexWrites.AdmissionAtAddress(IndexWriteAddressQuery{
		Target:     table,
		KeyPath:    target,
		HasKeyPath: true,
		KeyValue:   product.FromType(typ.LiteralString("node-1")),
	})
	if !ok || !product.Domain.Equal(got, value) {
		t.Fatalf("admission = %v/%v, want string", got, ok)
	}
}

func TestApplyKeyArrayElementKeyProofIgnoresLegacyStoredTableKey(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(132), "node_order")
	tablePath := constraint.NewPath(cfg.SymbolID(133), "nodes")
	targetPath := constraint.NewPath(cfg.SymbolID(134), "node_id")
	array := testStableAddressPath(t, arrayPath)
	table := testStableAddressPath(t, tablePath)
	target := testStableAddressPath(t, targetPath)
	state := PointState{
		KeyPresence: KeyPresenceFacts{}.
			WithKeyArrayValue(array.Key(), tablePath.Key(), product.FromType(typ.String)),
	}
	if tables := state.KeyPresence.KeyArrayTables(array.Key()); len(tables) != 1 || tables[0] != tablePath.Key() {
		t.Fatalf("test setup did not keep legacy stored table key: %s", state.KeyPresence.Format())
	}

	result, changed := ApplyKeyArrayElementKeyProof(&state, KeyArrayElementKeyProof{
		Array:     array,
		TargetKey: target,
		KeyValue:  product.FromType(typ.LiteralString("node-1")),
	})
	if changed || len(result.Tables) != 0 {
		t.Fatalf("legacy table key produced proof result: changed=%v tables=%v", changed, result.Tables)
	}
	if state.KeyPresence.Has(table.Key(), target.Key()) {
		t.Fatalf("legacy table key produced key-presence fact: %s", state.KeyPresence.Format())
	}
	if _, ok := state.IndexWrites.AdmissionAtAddress(IndexWriteAddressQuery{
		Target:     table,
		KeyPath:    target,
		HasKeyPath: true,
		KeyValue:   product.FromType(typ.LiteralString("node-1")),
	}); ok {
		t.Fatal("legacy table key produced readback admission")
	}
}

func TestApplyKeyArrayProofPublishesArrayTable(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(35), "node_order")
	tablePath := constraint.NewPath(cfg.SymbolID(36), "nodes")
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

func TestApplyKeyArrayPathProofNormalizesArrayTable(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(48), "node_order")
	tablePath := constraint.NewPath(cfg.SymbolID(49), "nodes")
	arrayKey := StablePathKey(arrayPath)
	tableKey := StablePathKey(tablePath)

	state := PointStateDomain.Top()
	if changed := ApplyKeyArrayPathProof(&state, arrayPath, tablePath); !changed {
		t.Fatal("ApplyKeyArrayPathProof reported no change")
	}

	tables := state.KeyPresence.KeyArrayTables(arrayKey)
	if len(tables) != 1 || tables[0] != tableKey {
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

func TestApplyEmptyKeyArrayPathProofNormalizesArray(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(50), "node_order")
	arrayKey := StablePathKey(arrayPath)

	state := PointStateDomain.Top()
	if changed := ApplyEmptyKeyArrayPathProof(&state, arrayPath); !changed {
		t.Fatal("ApplyEmptyKeyArrayPathProof reported no change")
	}
	if !state.KeyPresence.HasEmptyKeyArray(arrayKey) {
		t.Fatalf("empty key-array proof missing: %s", state.KeyPresence.Format())
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

	result, changed := ApplyIndexedKeyArrayIterationProof(&state, IndexedKeyArrayIterationProof{
		Array: testStableAddressPath(t, arrayPath),
		Key:   testStableAddressPath(t, keyPath),
	})
	if !changed {
		t.Fatal("ApplyIndexedKeyArrayIterationProof reported no change")
	}
	if len(result.Tables) != 1 || result.Tables[0].Key() != tableKey {
		t.Fatalf("iteration tables = %v, want %s", result.Tables, tableKey)
	}
	if !state.KeyPresence.Has(tableKey, keyKey) {
		t.Fatalf("iteration did not publish table/key presence: %s", state.KeyPresence.Format())
	}
}

func TestApplyIndexedKeyArrayIterationProofPublishesReadbackAdmission(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(44), "ids")
	tablePath := constraint.NewPath(cfg.SymbolID(45), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(46), "id")
	array := testStableAddressPath(t, arrayPath)
	table := testStableAddressPath(t, tablePath)
	key := testStableAddressPath(t, keyPath)
	value := product.FromType(typ.String)
	state := PointState{
		KeyPresence: KeyPresenceFacts{}.
			WithKeyArrayValueAddresses(array, table, value),
	}

	if _, changed := ApplyIndexedKeyArrayIterationProof(&state, IndexedKeyArrayIterationProof{
		Array:    array,
		Key:      key,
		KeyValue: product.FromType(typ.LiteralString("n1")),
	}); !changed {
		t.Fatal("ApplyIndexedKeyArrayIterationProof reported no change")
	}
	got, ok := state.IndexWrites.AdmissionAtAddress(IndexWriteAddressQuery{
		Target:     table,
		KeyPath:    key,
		HasKeyPath: true,
		KeyValue:   product.FromType(typ.LiteralString("n1")),
	})
	if !ok || !product.Domain.Equal(got, value) {
		t.Fatalf("admission = %v/%v, want string", got, ok)
	}
}

func TestApplyIndexedKeyArrayIterationProofIgnoresLegacyStoredTableKey(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(144), "ids")
	tablePath := constraint.NewPath(cfg.SymbolID(145), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(146), "id")
	array := testStableAddressPath(t, arrayPath)
	table := testStableAddressPath(t, tablePath)
	key := testStableAddressPath(t, keyPath)
	state := PointState{
		KeyPresence: KeyPresenceFacts{}.
			WithKeyArrayValue(array.Key(), tablePath.Key(), product.FromType(typ.String)),
	}
	if tables := state.KeyPresence.KeyArrayTables(array.Key()); len(tables) != 1 || tables[0] != tablePath.Key() {
		t.Fatalf("test setup did not keep legacy stored table key: %s", state.KeyPresence.Format())
	}

	result, changed := ApplyIndexedKeyArrayIterationProof(&state, IndexedKeyArrayIterationProof{
		Array:    array,
		Key:      key,
		KeyValue: product.FromType(typ.LiteralString("n1")),
	})
	if changed || len(result.Tables) != 0 {
		t.Fatalf("legacy table key produced iteration result: changed=%v tables=%v", changed, result.Tables)
	}
	if state.KeyPresence.Has(table.Key(), key.Key()) {
		t.Fatalf("legacy table key produced key-presence fact: %s", state.KeyPresence.Format())
	}
	if _, ok := state.IndexWrites.AdmissionAtAddress(IndexWriteAddressQuery{
		Target:     table,
		KeyPath:    key,
		HasKeyPath: true,
		KeyValue:   product.FromType(typ.LiteralString("n1")),
	}); ok {
		t.Fatal("legacy table key produced readback admission")
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

func TestApplyKeyArrayValueProofRecordsAppendCoverage(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(54), "node_order")
	tablePath := constraint.NewPath(cfg.SymbolID(55), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(56), "node_id")
	arrayKey := StablePathKey(arrayPath)
	tableKey := StablePathKey(tablePath)
	array := testStableAddressPath(t, arrayPath)
	table := testStableAddressPath(t, tablePath)
	key := testStableAddressPath(t, keyPath)
	value := product.FromType(typ.String)

	state := PointStateDomain.Top()
	state.KeyPresence = state.KeyPresence.WithAppendHistoryBase(arrayKey)

	if changed := ApplyKeyArrayValueProof(&state, KeyArrayValueProof{
		Array:        array,
		Table:        table,
		Value:        value,
		AppendKey:    key,
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

func TestApplyAppendHistoryBasePathProofNormalizesPath(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(75), "node_order")
	arrayKey := StablePathKey(arrayPath)

	state := PointStateDomain.Top()
	if changed := ApplyAppendHistoryBasePathProof(&state, arrayPath); !changed {
		t.Fatal("ApplyAppendHistoryBasePathProof reported no change")
	}
	if !state.KeyPresence.HasAppendHistoryBase(arrayKey) {
		t.Fatalf("append-history base missing: %s", state.KeyPresence.Format())
	}
}

func TestApplyAppendKeyPathProofNormalizesHistoryEvent(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(73), "node_order")
	keyPath := constraint.NewPath(cfg.SymbolID(74), "node_id")
	arrayKey := StablePathKey(arrayPath)
	keyKey := StablePathKey(keyPath)

	state := PointStateDomain.Top()
	state.KeyPresence = state.KeyPresence.WithAppendHistoryBase(arrayKey)
	if changed := ApplyAppendKeyPathProof(&state, AppendKeyPathProof{
		ArrayPath: arrayPath,
		KeyPath:   keyPath,
	}); !changed {
		t.Fatal("ApplyAppendKeyPathProof reported no change")
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

func TestApplyAppendElementFieldOriginProofRecordsSourceField(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(83), "items")
	sourcePath := constraint.NewPath(cfg.SymbolID(84), "source")
	arrayKey := StablePathKey(arrayPath)
	field := []constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}}
	sourceField := []constraint.Segment{{Kind: constraint.SegmentField, Name: "payload"}}

	state := PointStateDomain.Top()
	if changed := ApplyAppendElementFieldOriginProof(&state, AppendElementFieldOriginProof{
		Array:       testStableAddressPath(t, arrayPath),
		Field:       field,
		Source:      testStableAddressPath(t, sourcePath),
		SourceField: sourceField,
	}); !changed {
		t.Fatal("ApplyAppendElementFieldOriginProof reported no change")
	}

	if !state.KeyPresence.HasAppendHistoryBase(arrayKey) {
		t.Fatalf("append-history base missing: %s", state.KeyPresence.Format())
	}
	origins := state.KeyPresence.AppendElementFieldOriginEntries()
	if len(origins) != 1 || origins[0].Array != arrayKey || origins[0].Field == "" ||
		origins[0].Source != StablePathKey(sourcePath) || origins[0].SourceField == "" {
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

func TestIndexedIteratorKeyArrayReadbackIgnoresLegacyAliasSource(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(11), "node_order")
	tablePath := constraint.NewPath(cfg.SymbolID(12), "edges")
	iterKeyPath := constraint.NewPath(cfg.SymbolID(13), "node_id")
	aliasPath := constraint.NewPath(cfg.SymbolID(14), "current_node_id")
	iterKey := testStableAddressPath(t, iterKeyPath)
	alias := testStableAddressPath(t, aliasPath)

	keyPresence := KeyPresenceFacts{}.
		WithKeyArrayValueAddresses(testStableAddressPath(t, arrayPath), testStableAddressPath(t, tablePath), product.FromType(typ.Number))
	origins := ValueOriginFacts{}.
		WithAddresses(iterKey, testStableAddressPath(t, arrayPath), ValueOriginIndexedIterator, 1).
		With(ValueOriginFact{
			Value:  alias.Key(),
			Source: iterKeyPath.Key(),
			Kind:   ValueOriginAssignmentAlias,
		})
	raw := origins.OriginsCoveringAddress(alias)
	if len(raw) != 1 || raw[0].Origin.Source != iterKeyPath.Key() {
		t.Fatalf("test setup did not keep legacy stored source key: %s", origins.Format())
	}

	if got, ok := IndexedIteratorKeyArrayReadback(keyPresence, origins, testStableAddressPath(t, tablePath), alias); ok || !got.IsZero() {
		t.Fatalf("legacy alias source produced readback = %v/%v", got, ok)
	}
}
