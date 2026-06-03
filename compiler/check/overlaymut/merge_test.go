package overlaymut

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

func TestApplyMapWriteMergeToOverlay_TreatsNilWriteAsDeletion(t *testing.T) {
	overlay := make(map[cfg.SymbolID]typ.Type)
	ApplyMapWriteMergeToOverlay(overlay, map[cfg.SymbolID][]MapWriteInfo{
		1: {
			{KeyType: typ.String, ValueType: typ.Nil},
		},
	})
	if got := overlay[1]; got != nil {
		t.Fatalf("nil index write should not create map value evidence, got %v", got)
	}
}

func TestApplyMapWriteMergeToOverlay_DeletionDoesNotPoisonWriteValue(t *testing.T) {
	entry := typ.NewRecord().OptField("proc", typ.Any).Build()
	overlay := make(map[cfg.SymbolID]typ.Type)
	ApplyMapWriteMergeToOverlay(overlay, map[cfg.SymbolID][]MapWriteInfo{
		1: {
			{KeyType: typ.String, ValueType: typ.Nil},
			{KeyType: typ.String, ValueType: entry},
		},
	})

	got, ok := overlay[1].(*typ.Map)
	if !ok {
		t.Fatalf("mixed deletion/write should create map evidence, got %T", overlay[1])
	}
	if !typ.TypeEquals(got.Value, entry) {
		t.Fatalf("map value evidence = %v, want %v", got.Value, entry)
	}
}

func TestJoinValueTypes_TableInsertRefinesSoftArrayFallback(t *testing.T) {
	entry := typ.NewRecord().
		Field("id", typ.String).
		Field("name", typ.String).
		Build()
	fallback := typ.NewUnion(typ.NewArray(typ.Any), typ.NewRecord().Build())
	got := JoinValueTypes(fallback, typ.NewArray(entry))
	want := typ.NewArray(entry)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("JoinValueTypes() = %v, want %v", got, want)
	}
}

func TestMergeFieldAssignments_ReplacesUnsolvedFunctionSeed(t *testing.T) {
	seed := typ.Func().Build()
	solved := typ.Func().Param("self", typ.Any).Returns(typ.Number).Build()
	dst := liftFieldAssignments(map[cfg.SymbolID]map[string]typ.Type{
		1: {"get_x": seed},
	})
	src := liftFieldAssignments(map[cfg.SymbolID]map[string]typ.Type{
		1: {"get_x": solved},
	})

	MergeFieldAssignments(dst, src)

	got := projectedField(dst[1], "get_x")
	if !typ.TypeEquals(got, solved) {
		t.Fatalf("merged field assignment = %v, want %v", got, solved)
	}
}

func TestMergeFieldsIntoType_MergesExistingFieldAndPreservesShape(t *testing.T) {
	seed := typ.Func().Build()
	solved := typ.Func().Param("self", typ.Any).Returns(typ.Number).Build()
	base := typ.NewRecord().
		OptReadonlyField("get_x", seed).
		Field("name", typ.String).
		Metatable(typ.NewRecord().Field("__index", typ.Any).Build()).
		SetOpen(true).
		Build()

	got := MergeFieldsIntoType(base, fieldValues(map[string]typ.Type{"get_x": solved, "reset": typ.Func().Build()}))
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("merged type = %T %v, want record", got, got)
	}
	field := rec.GetField("get_x")
	if field == nil {
		t.Fatalf("merged record missing get_x: %s", typ.FormatShort(rec))
	}
	if !field.Optional || !field.Readonly {
		t.Fatalf("get_x shape not preserved: %#v", field)
	}
	if !typ.TypeEquals(field.Type, solved) {
		t.Fatalf("get_x type = %v, want %v", field.Type, solved)
	}
	if rec.GetField("reset") == nil {
		t.Fatalf("merged record missing new field reset: %s", typ.FormatShort(rec))
	}
	if rec.Metatable == nil || !rec.Open {
		t.Fatalf("merged record lost metatable/open shape: %s", typ.FormatShort(rec))
	}
}

func TestMergeRequiredFieldsIntoType_MarksSurfaceFieldPresent(t *testing.T) {
	seed := typ.Func().Build()
	solved := typ.Func().Param("self", typ.Any).Returns(typ.Number).Build()
	base := typ.NewRecord().OptField("get_x", seed).Build()

	got := MergeRequiredFieldsIntoType(base, fieldValues(map[string]typ.Type{"get_x": solved}))
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("merged type = %T %v, want record", got, got)
	}
	field := rec.GetField("get_x")
	if field == nil {
		t.Fatalf("merged record missing get_x: %s", typ.FormatShort(rec))
	}
	if field.Optional {
		t.Fatalf("required surface field stayed optional: %#v", field)
	}
	if !typ.TypeEquals(field.Type, solved) {
		t.Fatalf("get_x type = %v, want %v", field.Type, solved)
	}
}

func TestJoinValueTypes_ReplacesUnsolvedFunctionSeed(t *testing.T) {
	seed := typ.Func().Build()
	solved := typ.Func().Param("self", typ.Any).Returns(typ.Number).Build()

	got := JoinValueTypes(seed, solved)
	if !typ.TypeEquals(got, solved) {
		t.Fatalf("JoinValueTypes(seed, solved) = %v, want %v", got, solved)
	}
}

func TestApplyMapWriteMergeToOverlay_TableInsertRefinesAnnotatedMapValue(t *testing.T) {
	entry := typ.NewRecord().
		Field("id", typ.String).
		Field("name", typ.String).
		Build()
	overlay := map[cfg.SymbolID]typ.Type{
		1: typ.NewMap(typ.String, typ.NewArray(typ.Any)),
	}
	ApplyMapWriteMergeToOverlay(overlay, map[cfg.SymbolID][]MapWriteInfo{
		1: {
			{KeyType: typ.String, ValueType: typ.NewUnion(typ.NewArray(typ.Any), typ.NewRecord().Build())},
			{KeyType: typ.String, ValueType: typ.NewArray(entry)},
		},
	})

	got, ok := overlay[1].(*typ.Map)
	if !ok {
		t.Fatalf("expected map after map write merge, got %T", overlay[1])
	}
	want := typ.NewArray(entry)
	if !typ.TypeEquals(got.Value, want) {
		t.Fatalf("map value evidence = %v, want %v", got.Value, want)
	}
}
