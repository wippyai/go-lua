package paramevidence

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

func TestWidenMap_StopsSelfEmbeddingRecordGrowth(t *testing.T) {
	prevHint := typ.NewUnion(
		typ.Number,
		typ.NewRecord().
			Field("limit", typ.Any).
			SetOpen(true).
			Build(),
	)
	nextHint := typ.NewRecord().
		Field("limit", prevHint).
		SetOpen(true).
		Build()

	merged := WidenMap(
		map[cfg.SymbolID][]typ.Type{1: {prevHint}},
		map[cfg.SymbolID][]typ.Type{1: {nextHint}},
	)

	got := merged[1][0]
	if !typ.TypeEquals(got, prevHint) {
		t.Fatalf("expected stable previous evidence, got %v", got)
	}
}

func TestWidenMap_StopsSelfEmbeddingContainerGrowth(t *testing.T) {
	prevHint := typ.NewUnion(
		typ.Number,
		typ.NewRecord().
			Field("limit", typ.Any).
			SetOpen(true).
			Build(),
	)

	tests := []struct {
		name string
		next typ.Type
	}{
		{
			name: "record",
			next: typ.NewRecord().
				Field("value", prevHint).
				SetOpen(true).
				Build(),
		},
		{
			name: "array",
			next: typ.NewArray(prevHint),
		},
		{
			name: "map",
			next: typ.NewMap(typ.String, prevHint),
		},
		{
			name: "tuple",
			next: typ.NewTuple(prevHint),
		},
		{
			name: "function",
			next: typ.Func().
				Param("value", prevHint).
				Returns(prevHint).
				Build(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merged := WidenMap(
				map[cfg.SymbolID][]typ.Type{1: {prevHint}},
				map[cfg.SymbolID][]typ.Type{1: {tt.next}},
			)

			got := merged[1][0]
			if !typ.TypeEquals(got, prevHint) {
				t.Fatalf("expected stable previous evidence, got %v", got)
			}
		})
	}
}

func TestWidenMap_KeepsFirstRecordWrapperObservation(t *testing.T) {
	nextHint := typ.NewRecord().
		Field("limit", typ.Number).
		SetOpen(true).
		Build()

	merged := WidenMap(
		map[cfg.SymbolID][]typ.Type{1: {typ.Number}},
		map[cfg.SymbolID][]typ.Type{1: {nextHint}},
	)

	got := merged[1][0]
	if typ.TypeEquals(got, typ.Number) {
		t.Fatalf("expected wrapper observation to be preserved, got %v", got)
	}
	if !typ.TypeEquals(got, typ.NewUnion(typ.Number, nextHint)) {
		t.Fatalf("expected number | wrapper evidence, got %v", got)
	}
}

func TestWidenMap_JoinsNestedRecordObservations(t *testing.T) {
	nested := typ.NewRecord().
		Field("routes", typ.NewRecord().Field("users", typ.Boolean).SetOpen(true).Build()).
		SetOpen(true).
		Build()
	outer := typ.NewRecord().
		Field("api", nested).
		SetOpen(true).
		Build()

	merged := WidenMap(
		map[cfg.SymbolID][]typ.Type{1: {outer}},
		map[cfg.SymbolID][]typ.Type{1: {nested}},
	)

	got := merged[1][0]
	want := typ.NewUnion(outer, nested)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("expected nested record observations to be joined as %v, got %v", want, got)
	}
}

func TestWidenMap_ReplacesStaleBroadHintWithCurrentRefinement(t *testing.T) {
	stale := typ.NewUnion(typ.String, typ.False)
	current := typ.String

	merged := WidenMap(
		map[cfg.SymbolID][]typ.Type{1: {stale}},
		map[cfg.SymbolID][]typ.Type{1: {current}},
	)

	got := merged[1][0]
	if !typ.TypeEquals(got, current) {
		t.Fatalf("expected current refined evidence %v to replace stale broad evidence, got %v", current, got)
	}
}

func TestWidenMap_ReplacesSoftContainerPlaceholderWithConcreteElementShape(t *testing.T) {
	entry := typ.NewRecord().Field("id", typ.String).Build()
	stale := typ.NewUnion(
		typ.NewArray(typ.Any),
		typ.NewRecord().SetOpen(true).Build(),
	)
	current := typ.NewArray(entry)

	merged := WidenMap(
		map[cfg.SymbolID][]typ.Type{1: {stale}},
		map[cfg.SymbolID][]typ.Type{1: {current}},
	)

	got := merged[1][0]
	if !typ.TypeEquals(got, current) {
		t.Fatalf("expected concrete array evidence %v to replace soft stale evidence, got %v", current, got)
	}
}

func TestWidenMap_PreservesStructuredHintOverNilOnlyObservation(t *testing.T) {
	context := typ.NewMap(typ.String, typ.Any)

	merged := WidenMap(
		map[cfg.SymbolID][]typ.Type{1: {typ.String, typ.Any, context}},
		map[cfg.SymbolID][]typ.Type{1: {typ.String, typ.Any, typ.Nil}},
	)

	got := merged[1][2]
	if !typ.TypeEquals(got, context) {
		t.Fatalf("expected nil-only observation to preserve structured evidence %v, got %v", context, got)
	}

	again := WidenMap(merged, map[cfg.SymbolID][]typ.Type{1: {typ.String, typ.Any, typ.Nil}})
	if !evidenceMapsEqual(merged, again) {
		t.Fatalf("expected idempotent nil-only observation widening, got %v then %v", merged, again)
	}
}

func TestWidenMap_PreservesMapHintOverOptionalOpenRecordObservation(t *testing.T) {
	context := typ.NewMap(typ.String, typ.Any)
	optionalContextRecord := typ.NewOptional(typ.NewRecord().
		MapComponent(typ.String, typ.Any).
		SetOpen(true).
		Build())

	merged := WidenMap(
		map[cfg.SymbolID][]typ.Type{1: {typ.String, typ.Any, context}},
		map[cfg.SymbolID][]typ.Type{1: {typ.String, typ.Any, optionalContextRecord}},
	)

	got := merged[1][2]
	if got == nil || typ.TypeEquals(got, typ.Nil) {
		t.Fatalf("expected optional structured observation to preserve context evidence, got %v", got)
	}
	if !typ.TypeEquals(got, typ.NewOptional(context)) {
		t.Fatalf("expected pure map observation to stay canonical, got %v", got)
	}

	again := WidenMap(merged, map[cfg.SymbolID][]typ.Type{1: {typ.String, typ.Any, optionalContextRecord}})
	if !evidenceMapsEqual(merged, again) {
		t.Fatalf("expected idempotent optional structured observation widening, got %v then %v", merged, again)
	}
}

func TestWidenMap_CollapsesPureOpenRecordMapToCanonicalMap(t *testing.T) {
	entry := typ.NewRecord().Field("id", typ.String).Build()
	canonical := typ.NewMap(typ.String, typ.NewArray(entry))
	staleRecordView := typ.NewRecord().
		MapComponent(typ.NewUnion(typ.String, typ.False), typ.NewArray(entry)).
		SetOpen(true).
		Build()

	merged := WidenMap(
		map[cfg.SymbolID][]typ.Type{1: {staleRecordView}},
		map[cfg.SymbolID][]typ.Type{1: {canonical}},
	)

	got := merged[1][0]
	if !typ.TypeEquals(got, canonical) {
		t.Fatalf("expected pure keyed table evidence to canonicalize to %v, got %v", canonical, got)
	}
}

func TestWidenMap_TableTopUpperBoundAbsorbsRecordUnion(t *testing.T) {
	tableTop := typ.NewOptional(typ.NewInterface("table", nil))
	strategySpec := typ.NewRecord().
		Field("kind", typ.LiteralString("strategy")).
		Field("tools", typ.NewTuple(typ.String, typ.String, typ.String)).
		Build()
	contextSpec := typ.NewRecord().
		Field("kind", typ.LiteralString("context")).
		Field("scope", typ.String).
		Build()
	nextHint := typ.NewUnion(strategySpec, contextSpec)

	merged := WidenMap(
		map[cfg.SymbolID][]typ.Type{1: {tableTop}},
		map[cfg.SymbolID][]typ.Type{1: {nextHint}},
	)

	got := merged[1][0]
	if !typ.TypeEquals(got, tableTop) {
		t.Fatalf("expected table top upper bound %v, got %v", tableTop, got)
	}

	again := WidenMap(merged, map[cfg.SymbolID][]typ.Type{1: {nextHint}})
	if !evidenceMapsEqual(merged, again) {
		t.Fatalf("expected idempotent table-top widening, got %v then %v", merged, again)
	}
}

func TestWidenMap_TableTopUpperBoundAbsorbsAnyObservation(t *testing.T) {
	tableTop := typ.NewOptional(typ.NewInterface("table", nil))

	merged := WidenMap(
		map[cfg.SymbolID][]typ.Type{1: {tableTop}},
		map[cfg.SymbolID][]typ.Type{1: {typ.Any}},
	)

	got := merged[1][0]
	if !typ.TypeEquals(got, tableTop) {
		t.Fatalf("expected dynamic observation to preserve table top upper bound %v, got %v", tableTop, got)
	}

	again := WidenMap(merged, map[cfg.SymbolID][]typ.Type{1: {typ.Any}})
	if !evidenceMapsEqual(merged, again) {
		t.Fatalf("expected idempotent table-top/any widening, got %v then %v", merged, again)
	}
}

func evidenceMapsEqual(a, b map[cfg.SymbolID][]typ.Type) bool {
	if len(a) != len(b) {
		return false
	}
	for _, sym := range cfg.SortedSymbolIDs(a) {
		right, ok := b[sym]
		if !ok || !evidenceVectorsEqual(a[sym], right) {
			return false
		}
	}
	return true
}

func evidenceVectorsEqual(a, b []typ.Type) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !typ.TypeEquals(a[i], b[i]) {
			return false
		}
	}
	return true
}
