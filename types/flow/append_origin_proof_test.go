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
