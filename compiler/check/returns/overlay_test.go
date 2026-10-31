package returns

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/mutator"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func TestMergeFieldsIntoType(t *testing.T) {
	t.Run("nil base type creates open record", func(t *testing.T) {
		result := MergeFieldsIntoType(nil, map[string]typ.Type{"x": typ.Number})
		rec, ok := result.(*typ.Record)
		if !ok {
			t.Fatalf("expected record, got %T", result)
		}
		if !rec.Open {
			t.Fatal("expected open record")
		}
	})

	t.Run("empty fields returns base type", func(t *testing.T) {
		base := typ.String
		result := MergeFieldsIntoType(base, nil)
		if result != base {
			t.Fatalf("expected base type, got %v", result)
		}
	})

	t.Run("map base creates record with map component", func(t *testing.T) {
		base := typ.NewMap(typ.String, typ.Number)
		result := MergeFieldsIntoType(base, map[string]typ.Type{"x": typ.Boolean})
		rec, ok := result.(*typ.Record)
		if !ok {
			t.Fatalf("expected record, got %T", result)
		}
		if !rec.HasMapComponent() {
			t.Fatal("expected map component")
		}
	})

	t.Run("record base preserves existing fields", func(t *testing.T) {
		base := typ.NewRecord().Field("existing", typ.String).Build()
		result := MergeFieldsIntoType(base, map[string]typ.Type{"new": typ.Number})
		rec, ok := result.(*typ.Record)
		if !ok {
			t.Fatalf("expected record, got %T", result)
		}
		if len(rec.Fields) != 2 {
			t.Fatalf("expected 2 fields, got %d", len(rec.Fields))
		}
	})
}

func TestIsEmptyRecord(t *testing.T) {
	t.Run("nil returns false", func(t *testing.T) {
		if unwrap.IsEmptyRecord(nil) {
			t.Fatal("expected false for nil")
		}
	})

	t.Run("non-record returns false", func(t *testing.T) {
		if unwrap.IsEmptyRecord(typ.String) {
			t.Fatal("expected false for string")
		}
	})

	t.Run("empty record returns true", func(t *testing.T) {
		rec := typ.NewRecord().Build()
		if !unwrap.IsEmptyRecord(rec) {
			t.Fatal("expected true for empty record")
		}
	})

	t.Run("record with fields returns false", func(t *testing.T) {
		rec := typ.NewRecord().Field("x", typ.Number).Build()
		if unwrap.IsEmptyRecord(rec) {
			t.Fatal("expected false for record with fields")
		}
	})

	t.Run("record with map component returns false", func(t *testing.T) {
		rec := typ.NewRecord().MapComponent(typ.String, typ.Number).Build()
		if unwrap.IsEmptyRecord(rec) {
			t.Fatal("expected false for record with map component")
		}
	})
}

func TestJoinValueTypes(t *testing.T) {
	t.Run("nil a returns b", func(t *testing.T) {
		result := JoinValueTypes(nil, typ.String)
		if result != typ.String {
			t.Fatalf("expected string, got %v", result)
		}
	})

	t.Run("nil b returns a", func(t *testing.T) {
		result := JoinValueTypes(typ.String, nil)
		if result != typ.String {
			t.Fatalf("expected string, got %v", result)
		}
	})

	t.Run("empty record and array prefers array", func(t *testing.T) {
		emptyRec := typ.NewRecord().Build()
		arr := typ.NewArray(typ.Number)
		result := JoinValueTypes(emptyRec, arr)
		if _, ok := result.(*typ.Array); !ok {
			t.Fatalf("expected array, got %T", result)
		}
	})

	t.Run("array and empty record prefers array", func(t *testing.T) {
		emptyRec := typ.NewRecord().Build()
		arr := typ.NewArray(typ.Number)
		result := JoinValueTypes(arr, emptyRec)
		if _, ok := result.(*typ.Array); !ok {
			t.Fatalf("expected array, got %T", result)
		}
	})
}

func TestApplyIndexerMergeToOverlay(t *testing.T) {
	t.Run("empty infos are skipped", func(t *testing.T) {
		overlay := make(map[cfg.SymbolID]typ.Type)
		indexerAssignments := map[cfg.SymbolID][]mutator.IndexerInfo{
			1: {},
		}
		ApplyIndexerMergeToOverlay(overlay, indexerAssignments)
		if _, ok := overlay[1]; ok {
			t.Fatal("expected symbol 1 to not be in overlay")
		}
	})

	t.Run("indexer info is merged", func(t *testing.T) {
		overlay := make(map[cfg.SymbolID]typ.Type)
		indexerAssignments := map[cfg.SymbolID][]mutator.IndexerInfo{
			1: {{KeyType: typ.String, ValType: typ.Number}},
		}
		ApplyIndexerMergeToOverlay(overlay, indexerAssignments)
		if overlay[1] == nil {
			t.Fatal("expected overlay[1] to be set")
		}
	})
}

func TestApplyDirectMutationsToOverlay(t *testing.T) {
	t.Run("nil elem type is skipped", func(t *testing.T) {
		overlay := make(map[cfg.SymbolID]typ.Type)
		mutations := map[cfg.SymbolID]typ.Type{
			1: nil,
		}
		ApplyDirectMutationsToOverlay(overlay, mutations)
		if _, ok := overlay[1]; ok {
			t.Fatal("expected symbol 1 to not be in overlay")
		}
	})

	t.Run("mutation widens array", func(t *testing.T) {
		overlay := map[cfg.SymbolID]typ.Type{
			1: typ.NewArray(typ.Number),
		}
		mutations := map[cfg.SymbolID]typ.Type{
			1: typ.String,
		}
		ApplyDirectMutationsToOverlay(overlay, mutations)
		arr, ok := overlay[1].(*typ.Array)
		if !ok {
			t.Fatalf("expected array, got %T", overlay[1])
		}
		if arr.Element == nil {
			t.Fatal("expected widened element type")
		}
	})
}

func TestWidenArrayElement(t *testing.T) {
	t.Run("nil base creates array", func(t *testing.T) {
		result := flow.WidenArrayElementType(nil, typ.Number, typ.JoinPreferNonSoft)
		arr, ok := result.(*typ.Array)
		if !ok {
			t.Fatalf("expected array, got %T", result)
		}
		if arr.Element != typ.Number {
			t.Fatalf("expected number element, got %v", arr.Element)
		}
	})

	t.Run("array base widens element", func(t *testing.T) {
		base := typ.NewArray(typ.Number)
		result := flow.WidenArrayElementType(base, typ.String, typ.JoinPreferNonSoft)
		arr, ok := result.(*typ.Array)
		if !ok {
			t.Fatalf("expected array, got %T", result)
		}
		if arr.Element == nil {
			t.Fatal("expected joined element type")
		}
	})

	t.Run("empty record becomes array", func(t *testing.T) {
		base := typ.NewRecord().Build()
		result := flow.WidenArrayElementType(base, typ.Number, typ.JoinPreferNonSoft)
		if _, ok := result.(*typ.Array); !ok {
			t.Fatalf("expected array, got %T", result)
		}
	})

	t.Run("non-empty record unchanged", func(t *testing.T) {
		base := typ.NewRecord().Field("x", typ.Number).Build()
		result := flow.WidenArrayElementType(base, typ.String, typ.JoinPreferNonSoft)
		if result != base {
			t.Fatal("expected base to be unchanged")
		}
	})
}
