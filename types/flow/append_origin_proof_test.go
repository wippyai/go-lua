package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
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
