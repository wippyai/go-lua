package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/access"
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

func TestAppendOriginDestinationsIgnoresLegacyStoredRoutes(t *testing.T) {
	outPath := constraint.NewPath(cfg.SymbolID(204), "out")
	arrayPath := outPath.Field("id")
	iterArrayPath := constraint.NewPath(cfg.SymbolID(205), "source")
	aliasArrayPath := constraint.NewPath(cfg.SymbolID(206), "alias")
	out := testStableAddressPath(t, outPath)
	state := PointState{
		ValueOrigins: ValueOriginFacts{}.With(ValueOriginFact{
			Value:    out.Key(),
			Source:   iterArrayPath.Key(),
			Kind:     ValueOriginIndexedIterator,
			VarIndex: 1,
		}),
		PathAliases: PathAliasFacts{}.With(PathAliasFact{
			Value:  out.Key(),
			Source: aliasArrayPath.Key(),
		}),
	}
	if len(state.ValueOrigins.OriginsCoveringAddress(testStableAddressPath(t, arrayPath))) != 1 ||
		len(state.PathAliases.AliasesCoveringAddress(testStableAddressPath(t, arrayPath))) != 1 {
		t.Fatalf("test setup did not keep legacy stored routes: origins=%s aliases=%s", state.ValueOrigins.Format(), state.PathAliases.Format())
	}

	got := AppendOriginDestinations(state, testStableAddressPath(t, arrayPath), nil)
	if len(got) != 1 {
		t.Fatalf("destinations got %d, want only direct destination for legacy routes: %v", len(got), got)
	}
	assertAppendDestination(t, got[0], arrayPath, nil)
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

func TestApplyAppendElementFieldOriginUseIgnoresLegacyStoredSource(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(211), "out")
	sourcePath := constraint.NewPath(cfg.SymbolID(212), "source")
	iterPath := constraint.NewPath(cfg.SymbolID(213), "entry")
	source := testStableAddressPath(t, sourcePath)
	state := PointState{
		KeyPresence: KeyPresenceFacts{}.
			WithAppendHistoryBaseAddress(source).
			WithAppendElementFieldOriginFromSource(
				source.Key(),
				AppendElementFieldPathKey([]constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}}),
				iterPath.Key(),
				"",
			),
	}
	raw := state.KeyPresence.AppendElementFieldSources(source.Key(), []constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}})
	if len(raw) != 1 || raw[0].Origin.Source != iterPath.Key() {
		t.Fatalf("test setup did not keep legacy stored source key: %s", state.KeyPresence.Format())
	}
	originUse := ValueOriginUse{
		Origin: ValueOriginFact{
			Source:   source.Key(),
			Kind:     ValueOriginIndexedIterator,
			VarIndex: 1,
		},
		Remainder: []constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}},
	}
	destinations := []AppendOriginDestination{{
		Array:       testStableAddressPath(t, arrayPath),
		FieldPrefix: []constraint.Segment{{Kind: constraint.SegmentField, Name: "payload"}},
	}}

	if ApplyAppendElementFieldOriginUse(&state, destinations, []constraint.Segment{{Kind: constraint.SegmentField, Name: "name"}}, originUse) {
		t.Fatal("legacy stored source key replayed as canonical append field origin")
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

func TestAppendOriginSourcesIgnoresLegacyStoredRoutes(t *testing.T) {
	targetPath := constraint.NewPath(cfg.SymbolID(217), "target")
	fieldPath := targetPath.Field("id")
	iterArrayPath := constraint.NewPath(cfg.SymbolID(218), "items")
	assignmentSourcePath := constraint.NewPath(cfg.SymbolID(219), "assigned")
	aliasSourcePath := constraint.NewPath(cfg.SymbolID(220), "alias")
	target := testStableAddressPath(t, targetPath)
	field := testStableAddressPath(t, fieldPath)
	state := PointState{
		ValueOrigins: ValueOriginFacts{}.
			With(ValueOriginFact{
				Value:    target.Key(),
				Source:   iterArrayPath.Key(),
				Kind:     ValueOriginIndexedIterator,
				VarIndex: 1,
			}).
			With(ValueOriginFact{
				Value:  target.Key(),
				Source: assignmentSourcePath.Key(),
				Kind:   ValueOriginAssignmentAlias,
			}),
		PathAliases: PathAliasFacts{}.With(PathAliasFact{
			Value:  target.Key(),
			Source: aliasSourcePath.Key(),
		}),
	}
	if len(state.ValueOrigins.OriginsCoveringAddress(field)) != 2 ||
		len(state.PathAliases.AliasesCoveringAddress(field)) != 1 {
		t.Fatalf("test setup did not keep legacy stored routes: origins=%s aliases=%s", state.ValueOrigins.Format(), state.PathAliases.Format())
	}

	got := AppendOriginSources(state, field)
	if len(got) != 1 || !got[0].Source.Equal(field) || len(got[0].SourceField) != 0 {
		t.Fatalf("sources = %+v, want only direct source for legacy routes", got)
	}
}

func TestAppendElementFieldOriginUsesIgnoresLegacyStoredAliasSource(t *testing.T) {
	elementPath := constraint.NewPath(cfg.SymbolID(214), "element").Field("name")
	sourcePath := constraint.NewPath(cfg.SymbolID(215), "source")
	arrayPath := constraint.NewPath(cfg.SymbolID(216), "items")
	element := testStableAddressPath(t, elementPath)
	source := testStableAddressPath(t, sourcePath)
	state := PointState{
		PathAliases: PathAliasFacts{}.With(PathAliasFact{
			Value:  element.Key(),
			Source: sourcePath.Key(),
		}),
		ValueOrigins: ValueOriginFacts{}.WithAddresses(
			source,
			testStableAddressPath(t, arrayPath),
			ValueOriginIndexedIterator,
			1,
		),
	}
	if len(state.PathAliases.AliasesCoveringAddress(element)) != 1 {
		t.Fatalf("test setup did not keep legacy stored alias source: %s", state.PathAliases.Format())
	}

	uses := AppendElementFieldOriginUses(state, element)
	if len(uses) != 0 {
		t.Fatalf("legacy alias source produced append element origin uses: %#v", uses)
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

func TestAppendKeyArrayTablesIgnoresLegacyStoredKeyArrayTable(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(234), "keys")
	tablePath := constraint.NewPath(cfg.SymbolID(235), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(236), "node_id")
	array := testStableAddressPath(t, arrayPath)
	key := testStableAddressPath(t, keyPath)
	state := PointState{
		KeyPresence: KeyPresenceFacts{}.
			WithKeyArray(array.Key(), tablePath.Key()),
	}
	if tables := state.KeyPresence.KeyArrayTables(array.Key()); len(tables) != 1 || tables[0] != tablePath.Key() {
		t.Fatalf("test setup did not keep legacy stored table key: %s", state.KeyPresence.Format())
	}

	tables := AppendKeyArrayTables(state, AppendKeyArrayTableQuery{
		Array: array,
		Key:   key,
	})
	if len(tables) != 0 {
		t.Fatalf("legacy key-array table was selected: %v", tables)
	}
}

func TestAppendKeyArrayTablesFreshEmptyIgnoresLegacyStoredDirectTable(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(237), "keys")
	tablePath := constraint.NewPath(cfg.SymbolID(238), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(239), "node_id")
	array := testStableAddressPath(t, arrayPath)
	key := testStableAddressPath(t, keyPath)
	state := PointState{
		KeyPresence: KeyPresenceFacts{}.
			With(tablePath.Key(), key.Key()),
	}
	entries := state.KeyPresence.Entries()
	if len(entries) != 1 || entries[0].Table != tablePath.Key() {
		t.Fatalf("test setup did not keep legacy stored table key: %s", state.KeyPresence.Format())
	}

	tables := AppendKeyArrayTables(state, AppendKeyArrayTableQuery{
		Array:      array,
		Key:        key,
		FreshEmpty: true,
	})
	if len(tables) != 0 {
		t.Fatalf("legacy direct table/key fact was selected: %v", tables)
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

func TestApplyAppendElementMutationPathProofAppliesCollectionTransaction(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(249), "node_order")
	tablePath := constraint.NewPath(cfg.SymbolID(250), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(251), "node_id")
	sourcePath := constraint.NewPath(cfg.SymbolID(252), "node").Field("status")
	nodeType := typ.NewRecord().Field("node_id", typ.String).Field("status", typ.String).Build()
	state := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(arrayPath.Symbol): product.FromType(typ.NewFreshArray()),
			SymbolValueKey(keyPath.Symbol):   product.FromType(typ.String),
		},
		KeyPresence: KeyPresenceFacts{}.
			WithAddresses(testStableAddressPath(t, tablePath), testStableAddressPath(t, keyPath)),
		IndexWrites: IndexWriteAdmissionFacts{}.WithAddress(IndexWriteAdmissionAddressFact{
			Target:     testStableAddressPath(t, tablePath),
			KeyPath:    testStableAddressPath(t, keyPath),
			HasKeyPath: true,
			Key:        product.FromType(typ.String),
			Value:      product.FromType(nodeType),
		}),
	}

	changed := ApplyAppendElementMutationPathProof(&state, AppendElementMutationPathProof{
		Footprint: access.WriteFootprint{
			WritePath:         arrayPath,
			ExactWritePath:    arrayPath,
			HasExactWritePath: true,
		},
		ArrayPath:    arrayPath,
		ElementPath:  keyPath,
		ElementValue: product.FromType(typ.String),
		FieldSources: []AppendElementFieldOriginPathSource{{
			Field:      []constraint.Segment{{Kind: constraint.SegmentField, Name: "status"}},
			SourcePath: sourcePath,
		}},
	})
	if !changed {
		t.Fatal("ApplyAppendElementMutationPathProof reported no change")
	}
	if tables := state.KeyPresence.KeyArrayTables(StablePathKey(arrayPath)); len(tables) != 1 || tables[0] != StablePathKey(tablePath) {
		t.Fatalf("append transaction did not seed key-array table: %s", state.KeyPresence.Format())
	}
	values := state.KeyPresence.KeyArrayValues(StablePathKey(arrayPath), StablePathKey(tablePath))
	if len(values) != 1 || !product.Domain.Equal(values[0], product.FromType(nodeType)) {
		t.Fatalf("append transaction key-array values = %v, want node record", values)
	}
	fieldSources := state.KeyPresence.AppendElementFieldSources(
		StablePathKey(arrayPath),
		[]constraint.Segment{{Kind: constraint.SegmentField, Name: "status"}},
	)
	if len(fieldSources) != 1 {
		t.Fatalf("append transaction field sources = %v, want one source; facts=%s", fieldSources, state.KeyPresence.Format())
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

func TestAppendKeyArrayPreservationIgnoresLegacyStoredKeyArrayTable(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(242), "keys")
	tablePath := constraint.NewPath(cfg.SymbolID(243), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(244), "id")
	array := testStableAddressPath(t, arrayPath)
	key := testStableAddressPath(t, keyPath)
	state := PointState{
		KeyPresence: KeyPresenceFacts{}.
			WithKeyArray(array.Key(), tablePath.Key()),
	}
	if tables := state.KeyPresence.KeyArrayTables(array.Key()); len(tables) != 1 || tables[0] != tablePath.Key() {
		t.Fatalf("test setup did not keep legacy stored table key: %s", state.KeyPresence.Format())
	}

	selection := AppendKeyArrayPreservation(state, AppendKeyArrayPreservationQuery{
		Array: array,
		Key:   key,
	})
	if len(selection.Tables) != 0 || len(selection.Pending) != 0 {
		t.Fatalf("legacy key-array table produced preservation selection: %+v", selection)
	}
}

func TestAppendKeyArrayPreservationFreshEmptyIgnoresLegacyStoredDirectTable(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(245), "keys")
	tablePath := constraint.NewPath(cfg.SymbolID(246), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(247), "id")
	array := testStableAddressPath(t, arrayPath)
	key := testStableAddressPath(t, keyPath)
	state := PointState{
		KeyPresence: KeyPresenceFacts{}.
			With(tablePath.Key(), key.Key()),
	}
	entries := state.KeyPresence.Entries()
	if len(entries) != 1 || entries[0].Table != tablePath.Key() {
		t.Fatalf("test setup did not keep legacy stored table key: %s", state.KeyPresence.Format())
	}

	selection := AppendKeyArrayPreservation(state, AppendKeyArrayPreservationQuery{
		Array:          array,
		Key:            key,
		FreshEmptySeed: true,
	})
	if len(selection.Tables) != 0 {
		t.Fatalf("legacy direct table/key fact produced table selection: %+v", selection)
	}
	if len(selection.Pending) != 1 || selection.Pending[0].HasTable {
		t.Fatalf("fresh-empty wildcard pending was not preserved: %+v", selection)
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
