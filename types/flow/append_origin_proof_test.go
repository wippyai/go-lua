package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestAppendOriginDestinationsRoutesThroughIteratorAndAliasFacts(t *testing.T) {
	outPath := constraint.NewPath(cfg.SymbolID(1), "out")
	arrayPath := outPath.Field("id")
	iterArrayPath := constraint.NewPath(cfg.SymbolID(2), "source")
	aliasArrayPath := constraint.NewPath(cfg.SymbolID(3), "alias")
	state := PointState{
		ValueOrigins: ValueOriginFacts{}.WithAddresses(
			testStableAddressPath(t, outPath),
			testStableAddressPath(t, iterArrayPath),
			ValueOriginIndexedIterator,
			1,
		),
		PathAliases: PathAliasFacts{}.WithAddresses(
			testStableAddressPath(t, outPath),
			testStableAddressPath(t, aliasArrayPath),
		),
	}

	got := AppendOriginDestinations(state, testStableAddressPath(t, arrayPath), nil)
	if len(got) != 3 {
		t.Fatalf("destinations got %d, want direct + iterator + alias", len(got))
	}
	assertAppendDestination(t, got[0], arrayPath, nil)
	assertAppendDestination(t, got[1], iterArrayPath, []constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}})
	assertAppendDestination(t, got[2], aliasArrayPath.Field("id"), nil)
}

func TestAppendOriginDestinationsPathNormalizesArray(t *testing.T) {
	outPath := constraint.NewPath(cfg.SymbolID(4), "out")
	arrayPath := outPath.Field("id")
	iterArrayPath := constraint.NewPath(cfg.SymbolID(5), "source")
	state := PointState{
		ValueOrigins: ValueOriginFacts{}.WithAddresses(
			testStableAddressPath(t, outPath),
			testStableAddressPath(t, iterArrayPath),
			ValueOriginIndexedIterator,
			1,
		),
	}

	got := AppendOriginDestinationsPath(state, arrayPath, nil)
	if len(got) != 2 {
		t.Fatalf("destinations got %d, want direct + iterator", len(got))
	}
	assertAppendDestination(t, got[0], arrayPath, nil)
	assertAppendDestination(t, got[1], iterArrayPath, []constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}})
}

func TestApplyAppendElementFieldOriginUseReplaysSourcesToDestinations(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(11), "out")
	sourcePath := constraint.NewPath(cfg.SymbolID(12), "source")
	iterPath := constraint.NewPath(cfg.SymbolID(13), "entry")
	state := PointState{
		KeyPresence: KeyPresenceFacts{}.
			WithAppendHistoryBaseAddress(testStableAddressPath(t, sourcePath)).
			WithAppendElementFieldOriginFromAddresses(
				testStableAddressPath(t, sourcePath),
				[]constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}},
				testStableAddressPath(t, iterPath),
				nil,
			),
	}
	originUse := ValueOriginUse{
		Origin: ValueOriginFact{
			Source:   testStableAddressPath(t, sourcePath).Key(),
			Kind:     ValueOriginIndexedIterator,
			VarIndex: 1,
		},
		Remainder: []constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}},
	}
	destinations := []AppendOriginDestination{{
		Array:       testStableAddressPath(t, arrayPath),
		FieldPrefix: []constraint.Segment{{Kind: constraint.SegmentField, Name: "payload"}},
	}}

	if !ApplyAppendElementFieldOriginUse(&state, destinations, []constraint.Segment{{Kind: constraint.SegmentField, Name: "name"}}, originUse) {
		t.Fatalf("expected append element field origin replay to change state")
	}
	if !state.KeyPresence.HasAppendHistoryBase(testStableAddressPath(t, arrayPath).Key()) {
		t.Fatalf("destination append history base was not recorded")
	}
	field := []constraint.Segment{
		{Kind: constraint.SegmentField, Name: "payload"},
		{Kind: constraint.SegmentField, Name: "name"},
	}
	if len(state.KeyPresence.AppendElementFieldSources(testStableAddressPath(t, arrayPath).Key(), field)) != 1 {
		t.Fatalf("destination field origin was not replayed")
	}
}

func TestAppendElementFieldOriginFieldsReturnsStructuredFields(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(114), "out")
	sourcePath := constraint.NewPath(cfg.SymbolID(115), "source")
	field := []constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}}
	state := PointState{
		KeyPresence: KeyPresenceFacts{}.
			WithAppendHistoryBaseAddress(testStableAddressPath(t, arrayPath)).
			WithAppendElementFieldOriginFromAddresses(
				testStableAddressPath(t, arrayPath),
				field,
				testStableAddressPath(t, sourcePath),
				nil,
			),
	}

	got := AppendElementFieldOriginFields(state)
	if len(got) != 1 || len(got[0]) != 1 || got[0][0].Name != "id" {
		t.Fatalf("AppendElementFieldOriginFields = %#v, want [.id]", got)
	}
}

func TestAppendElementFieldOriginUsesRoutesThroughPathAlias(t *testing.T) {
	elementPath := constraint.NewPath(cfg.SymbolID(14), "element").Field("name")
	sourcePath := constraint.NewPath(cfg.SymbolID(15), "source")
	arrayPath := constraint.NewPath(cfg.SymbolID(16), "items")
	state := PointState{
		PathAliases: PathAliasFacts{}.WithAddresses(
			testStableAddressPath(t, elementPath),
			testStableAddressPath(t, sourcePath),
		),
		ValueOrigins: ValueOriginFacts{}.WithAddresses(
			testStableAddressPath(t, sourcePath),
			testStableAddressPath(t, arrayPath),
			ValueOriginIndexedIterator,
			1,
		),
	}

	uses := AppendElementFieldOriginUses(state, testStableAddressPath(t, elementPath))
	if len(uses) != 1 {
		t.Fatalf("uses got %d, want alias-routed origin", len(uses))
	}
	if uses[0].Origin.Source != testStableAddressPath(t, arrayPath).Key() || uses[0].Origin.Kind != ValueOriginIndexedIterator {
		t.Fatalf("use = %#v, want indexed iterator source", uses[0])
	}
}

func TestAppendElementFieldOriginUsesPathNormalizesField(t *testing.T) {
	elementPath := constraint.NewPath(cfg.SymbolID(17), "element").Field("name")
	sourcePath := constraint.NewPath(cfg.SymbolID(18), "source")
	arrayPath := constraint.NewPath(cfg.SymbolID(19), "items")
	state := PointState{
		PathAliases: PathAliasFacts{}.WithAddresses(
			testStableAddressPath(t, elementPath),
			testStableAddressPath(t, sourcePath),
		),
		ValueOrigins: ValueOriginFacts{}.WithAddresses(
			testStableAddressPath(t, sourcePath),
			testStableAddressPath(t, arrayPath),
			ValueOriginIndexedIterator,
			1,
		),
	}

	uses := AppendElementFieldOriginUsesPath(state, elementPath)
	if len(uses) != 1 {
		t.Fatalf("uses got %d, want alias-routed origin", len(uses))
	}
	if uses[0].Origin.Source != testStableAddressPath(t, arrayPath).Key() || uses[0].Origin.Kind != ValueOriginIndexedIterator {
		t.Fatalf("use = %#v, want indexed iterator source", uses[0])
	}
}

func TestApplyAppendKeyArrayConsequencesPublishesReadbackValue(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(21), "keys")
	tablePath := constraint.NewPath(cfg.SymbolID(22), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(23), "node_id")
	value := product.FromType(typ.String)
	state := PointState{
		IndexWrites: IndexWriteAdmissionFacts{}.WithAddress(IndexWriteAdmissionAddressFact{
			Target:     testStableAddressPath(t, tablePath),
			KeyPath:    testStableAddressPath(t, keyPath),
			HasKeyPath: true,
			Key:        product.FromType(typ.String),
			Value:      value,
		}),
	}

	if !ApplyAppendKeyArrayConsequences(&state, AppendKeyArrayConsequences{
		Array:    testStableAddressPath(t, arrayPath),
		Key:      testStableAddressPath(t, keyPath),
		HasKey:   true,
		KeyValue: product.FromType(typ.String),
		Tables:   []StableAddress{testStableAddressPath(t, tablePath)},
		Pending: []PendingKeyArrayDestination{{
			Table:    testStableAddressPath(t, tablePath),
			HasTable: true,
		}},
	}) {
		t.Fatalf("expected append key-array consequences to change state")
	}

	arrayKey := StablePathKey(arrayPath)
	tableKey := StablePathKey(tablePath)
	if len(state.KeyPresence.KeyArrayTables(arrayKey)) != 1 || state.KeyPresence.KeyArrayTables(arrayKey)[0] != tableKey {
		t.Fatalf("key-array proof was not published: %s", state.KeyPresence.Format())
	}
	values := state.KeyPresence.KeyArrayValues(arrayKey, tableKey)
	if len(values) != 1 || !product.Domain.Equal(values[0], value) {
		t.Fatalf("key-array values = %v, want string", values)
	}
	if len(state.KeyPresence.PendingKeyArrayEntries()) != 1 {
		t.Fatalf("pending key-array proof was not published: %s", state.KeyPresence.Format())
	}
}

func TestApplyAppendKeyArrayPathConsequencesPublishesReadbackValue(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(24), "keys")
	tablePath := constraint.NewPath(cfg.SymbolID(25), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(26), "node_id")
	value := product.FromType(typ.String)
	state := PointState{
		IndexWrites: IndexWriteAdmissionFacts{}.WithAddress(IndexWriteAdmissionAddressFact{
			Target:     testStableAddressPath(t, tablePath),
			KeyPath:    testStableAddressPath(t, keyPath),
			HasKeyPath: true,
			Key:        product.FromType(typ.String),
			Value:      value,
		}),
	}

	if !ApplyAppendKeyArrayPathConsequences(&state, AppendKeyArrayPathConsequences{
		ArrayPath: arrayPath,
		KeyPath:   keyPath,
		HasKey:    true,
		KeyValue:  product.FromType(typ.String),
		Tables:    []StableAddress{testStableAddressPath(t, tablePath)},
	}) {
		t.Fatalf("expected append key-array path consequences to change state")
	}

	values := state.KeyPresence.KeyArrayValues(StablePathKey(arrayPath), StablePathKey(tablePath))
	if len(values) != 1 || !product.Domain.Equal(values[0], value) {
		t.Fatalf("key-array values = %v, want string", values)
	}
}

func TestAppendKeyArrayTablesSelectsExplicitExistingTable(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(31), "keys")
	tablePath := constraint.NewPath(cfg.SymbolID(32), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(33), "node_id")
	array := testStableAddressPath(t, arrayPath)
	table := testStableAddressPath(t, tablePath)
	key := testStableAddressPath(t, keyPath)
	state := PointState{
		KeyPresence: KeyPresenceFacts{}.
			WithKeyArrayAddresses(array, table),
	}

	tables := AppendKeyArrayTables(state, AppendKeyArrayTableQuery{
		Array:            array,
		Key:              key,
		ExplicitTable:    table,
		HasExplicitTable: true,
	})
	if len(tables) != 1 || !tables[0].Equal(table) {
		t.Fatalf("tables = %v, want explicit table", tables)
	}
}

func TestAppendKeyArrayTablesUsesFreshEmptyEvidence(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(34), "keys")
	tablePath := constraint.NewPath(cfg.SymbolID(35), "nodes")
	otherPath := constraint.NewPath(cfg.SymbolID(36), "edges")
	keyPath := constraint.NewPath(cfg.SymbolID(37), "node_id")
	array := testStableAddressPath(t, arrayPath)
	table := testStableAddressPath(t, tablePath)
	other := testStableAddressPath(t, otherPath)
	key := testStableAddressPath(t, keyPath)
	state := PointState{
		KeyPresence: KeyPresenceFacts{}.
			WithAddresses(table, key),
	}

	tables := AppendKeyArrayTables(state, AppendKeyArrayTableQuery{
		Array:         array,
		Key:           key,
		WrittenTables: []StableAddress{other},
		FreshEmpty:    true,
	})
	if len(tables) != 2 {
		t.Fatalf("tables = %v, want written and key-presence tables", tables)
	}
	if !tables[0].Equal(other) && !tables[1].Equal(other) {
		t.Fatalf("tables missing written table: %v", tables)
	}
	if !tables[0].Equal(table) && !tables[1].Equal(table) {
		t.Fatalf("tables missing key-presence table: %v", tables)
	}
}

func TestAppendKeyArrayTablesPathLowersStructuredInputs(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(134), "keys")
	tablePath := constraint.NewPath(cfg.SymbolID(135), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(136), "node_id")
	table := testStableAddressPath(t, tablePath)
	state := PointState{
		KeyPresence: KeyPresenceFacts{}.
			WithKeyArrayAddresses(testStableAddressPath(t, arrayPath), table),
	}

	tables := AppendKeyArrayTablesPath(state, AppendKeyArrayPathTableQuery{
		ArrayPath:         arrayPath,
		KeyPath:           keyPath,
		ExplicitTablePath: tablePath,
		HasExplicitTable:  true,
	})
	if len(tables) != 1 || !tables[0].Equal(table) {
		t.Fatalf("path tables = %v, want explicit table", tables)
	}
}

func TestApplyAppendKeyReplayPathProofAppliesAppendTransaction(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(137), "keys")
	tablePath := constraint.NewPath(cfg.SymbolID(138), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(139), "node_id")
	value := product.FromType(typ.String)
	state := PointState{
		KeyPresence: KeyPresenceFacts{}.
			WithKeyArrayAddresses(testStableAddressPath(t, arrayPath), testStableAddressPath(t, tablePath)),
		IndexWrites: IndexWriteAdmissionFacts{}.WithAddress(IndexWriteAdmissionAddressFact{
			Target:     testStableAddressPath(t, tablePath),
			KeyPath:    testStableAddressPath(t, keyPath),
			HasKeyPath: true,
			Key:        product.FromType(typ.Unknown),
			Value:      value,
		}),
	}

	if !ApplyAppendKeyReplayPathProof(&state, AppendKeyReplayPathProof{
		ArrayPath:         arrayPath,
		KeyPath:           keyPath,
		WrittenTablePaths: []constraint.Path{tablePath},
		FreshEmpty:        true,
	}) {
		t.Fatal("ApplyAppendKeyReplayPathProof reported no change")
	}
	appends := state.KeyPresence.AppendedKeyEntries()
	if len(appends) != 1 || appends[0].Array != StablePathKey(arrayPath) || appends[0].Key != StablePathKey(keyPath) {
		t.Fatalf("append key was not recorded: %s", state.KeyPresence.Format())
	}
	values := state.KeyPresence.KeyArrayValues(StablePathKey(arrayPath), StablePathKey(tablePath))
	if len(values) != 1 || !product.Domain.Equal(values[0], value) {
		t.Fatalf("key-array values = %v, want string", values)
	}
}

func TestAppendKeyArrayPreservationSplitsDirectAndPendingTables(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(38), "keys")
	nodesPath := constraint.NewPath(cfg.SymbolID(39), "nodes")
	edgesPath := constraint.NewPath(cfg.SymbolID(40), "edges")
	keyPath := constraint.NewPath(cfg.SymbolID(41), "id")
	array := testStableAddressPath(t, arrayPath)
	nodes := testStableAddressPath(t, nodesPath)
	edges := testStableAddressPath(t, edgesPath)
	key := testStableAddressPath(t, keyPath)
	state := PointState{
		KeyPresence: KeyPresenceFacts{}.
			WithKeyArrayAddresses(array, nodes).
			WithKeyArrayAddresses(array, edges).
			WithAddresses(nodes, key),
	}

	selection := AppendKeyArrayPreservation(state, AppendKeyArrayPreservationQuery{
		Array: array,
		Key:   key,
	})
	if len(selection.Tables) != 1 || !selection.Tables[0].Equal(nodes) {
		t.Fatalf("direct tables = %v, want nodes", selection.Tables)
	}
	if len(selection.Pending) != 1 || !selection.Pending[0].HasTable || !selection.Pending[0].Table.Equal(edges) {
		t.Fatalf("pending = %v, want edges", selection.Pending)
	}
}

func TestAppendKeyArrayPreservationUsesFreshEmptySeed(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(42), "keys")
	tablePath := constraint.NewPath(cfg.SymbolID(43), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(44), "id")
	array := testStableAddressPath(t, arrayPath)
	table := testStableAddressPath(t, tablePath)
	key := testStableAddressPath(t, keyPath)
	state := PointState{
		KeyPresence: KeyPresenceFacts{}.
			WithAddresses(table, key),
	}

	selection := AppendKeyArrayPreservation(state, AppendKeyArrayPreservationQuery{
		Array:          array,
		Key:            key,
		FreshEmptySeed: true,
	})
	if len(selection.Pending) != 1 || selection.Pending[0].HasTable {
		t.Fatalf("pending = %v, want wildcard pending", selection.Pending)
	}
	if len(selection.Tables) != 1 || !selection.Tables[0].Equal(table) {
		t.Fatalf("direct tables = %v, want table with key", selection.Tables)
	}
}

func TestAppendKeyArrayPathPreservationNormalizesPaths(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(45), "keys")
	tablePath := constraint.NewPath(cfg.SymbolID(46), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(47), "id")
	table := testStableAddressPath(t, tablePath)
	state := PointState{
		KeyPresence: KeyPresenceFacts{}.
			WithAddresses(table, testStableAddressPath(t, keyPath)),
	}

	selection := AppendKeyArrayPathPreservation(state, AppendKeyArrayPathPreservationQuery{
		ArrayPath:      arrayPath,
		KeyPath:        keyPath,
		FreshEmptySeed: true,
	})
	if len(selection.Pending) != 1 || selection.Pending[0].HasTable {
		t.Fatalf("pending = %v, want wildcard pending", selection.Pending)
	}
	if len(selection.Tables) != 1 || !selection.Tables[0].Equal(table) {
		t.Fatalf("direct tables = %v, want table with key", selection.Tables)
	}
}

func assertAppendDestination(t *testing.T, got AppendOriginDestination, wantPath constraint.Path, wantPrefix []constraint.Segment) {
	t.Helper()
	want := testStableAddressPath(t, wantPath)
	if !got.Array.Equal(want) {
		t.Fatalf("destination array = %s, want %s", got.Array.Key(), want.Key())
	}
	if len(got.FieldPrefix) != len(wantPrefix) {
		t.Fatalf("field prefix = %#v, want %#v", got.FieldPrefix, wantPrefix)
	}
	for i := range wantPrefix {
		if got.FieldPrefix[i] != wantPrefix[i] {
			t.Fatalf("field prefix = %#v, want %#v", got.FieldPrefix, wantPrefix)
		}
	}
}
