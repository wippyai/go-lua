package core

import (
	"testing"

	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

func TestIndexWrite_RecordLiteralKeyUsesWritableFieldType(t *testing.T) {
	rec := typ.NewRecord().
		Field("name", typ.LiteralString("draft")).
		Field("count", typ.LiteralInt(4)).
		Build()

	nameSlot, ok := IndexWrite(rec, typ.LiteralString("name"))
	if !ok {
		t.Fatal("expected writable name slot")
	}
	if !typ.TypeEquals(nameSlot, typ.String) {
		t.Fatalf("name writable slot = %v, want string", nameSlot)
	}

	countSlot, ok := IndexWrite(rec, typ.LiteralString("count"))
	if !ok {
		t.Fatal("expected writable count slot")
	}
	if !typ.TypeEquals(countSlot, typ.Integer) {
		t.Fatalf("count writable slot = %v, want integer", countSlot)
	}
}

func TestIndexWrite_DynamicStringKeyOverHeterogeneousFieldsHasNoSingleProjection(t *testing.T) {
	rec := typ.NewRecord().
		Field("name", typ.String).
		Field("count", typ.Integer).
		Build()

	if slot, ok := IndexWrite(rec, typ.String); ok {
		t.Fatalf("heterogeneous dynamic key should require memory/key relation, got %v", slot)
	}
}

func TestIndexWrite_DynamicStringKeyWithUniformFields(t *testing.T) {
	rec := typ.NewRecord().
		Field("first", typ.LiteralString("a")).
		Field("second", typ.String).
		Build()

	slot, ok := IndexWrite(rec, typ.String)
	if !ok {
		t.Fatal("expected writable slot for dynamic string key")
	}
	if !subtype.IsSubtype(typ.String, slot) {
		t.Fatalf("string should be accepted for uniform string fields: %v", slot)
	}
}

func TestIndexWrite_RecordLiteralUnionFieldStaysClosed(t *testing.T) {
	status := typ.NewUnion(typ.LiteralString("queued"), typ.LiteralString("started"))
	rec := typ.NewRecord().
		Field("status", status).
		Build()

	slot, ok := IndexWrite(rec, typ.LiteralString("status"))
	if !ok {
		t.Fatal("expected writable status slot")
	}
	if !typ.TypeEquals(slot, status) {
		t.Fatalf("status writable slot = %v, want %v", slot, status)
	}
	if subtype.IsSubtype(typ.String, slot) {
		t.Fatalf("closed literal-union slot should not widen to string: %v", slot)
	}
}

func TestIndexWrite_MapAndUnionUseWriteSideMeet(t *testing.T) {
	left := typ.NewMap(typ.String, typ.String)
	right := typ.NewMap(typ.String, typ.Integer)
	union := typ.NewUnion(left, right)

	slot, ok := IndexWrite(union, typ.String)
	if !ok {
		t.Fatal("expected writable slot for union of maps")
	}
	if subtype.IsSubtype(typ.String, slot) || subtype.IsSubtype(typ.Integer, slot) {
		t.Fatalf("union write must satisfy both map value slots, got %v", slot)
	}
}

func TestIndexWrite_ExactKeyUnionUsesWriteSideMeet(t *testing.T) {
	rec := typ.NewRecord().
		Field("name", typ.String).
		Field("count", typ.Integer).
		Build()
	key := typ.NewUnion(typ.LiteralString("name"), typ.LiteralString("count"))

	slot, ok := IndexWrite(rec, key)
	if !ok {
		t.Fatal("expected writable slot for exact key union")
	}
	if subtype.IsSubtype(typ.String, slot) || subtype.IsSubtype(typ.Integer, slot) {
		t.Fatalf("exact key union write must satisfy both fields, got %v", slot)
	}
}

func TestIndexWrite_MixedDirectFieldAndRowTailRequiresUniformSlot(t *testing.T) {
	rec := typ.NewRecord().
		Field("known", typ.Number).
		MapComponent(typ.String, typ.String).
		Build()
	key := typ.NewUnion(typ.LiteralString("known"), typ.LiteralString("other"))

	if slot, ok := IndexWrite(rec, key); ok {
		t.Fatalf("heterogeneous direct field plus row-tail write should require relation, got %v", slot)
	}

	uniform := typ.NewRecord().
		Field("known", typ.String).
		MapComponent(typ.String, typ.String).
		Build()
	slot, ok := IndexWrite(uniform, key)
	if !ok {
		t.Fatal("expected uniform direct field plus row-tail write projection")
	}
	if !typ.TypeEquals(slot, typ.String) {
		t.Fatalf("uniform mixed slot = %v, want string", slot)
	}
}

func TestIndexDelete_MapAllowsAbsentEntries(t *testing.T) {
	m := typ.NewMap(typ.String, typ.Integer)
	if !IndexDelete(m, typ.String) {
		t.Fatal("expected nil write to delete a typed map entry")
	}
	if !IndexDelete(m, typ.LiteralString("id")) {
		t.Fatal("expected literal string key to delete a typed map entry")
	}
}

func TestIndexDelete_RecordRequiredFieldRejectsDeletion(t *testing.T) {
	rec := typ.NewRecord().
		Field("required", typ.String).
		OptField("optional", typ.String).
		Build()

	if IndexDelete(rec, typ.LiteralString("required")) {
		t.Fatal("required record field deletion must be rejected")
	}
	if !IndexDelete(rec, typ.LiteralString("optional")) {
		t.Fatal("optional record field deletion should be allowed")
	}
	if IndexDelete(rec, typ.String) {
		t.Fatal("dynamic key deletion may hit the required field and must be rejected")
	}
}

func TestIndexDelete_RecordMapTailAllowsAbsentEntries(t *testing.T) {
	rec := typ.NewRecord().
		Field("required", typ.String).
		MapComponent(typ.String, typ.Integer).
		Build()

	if !IndexDelete(rec, typ.LiteralString("dynamic")) {
		t.Fatal("row-tail key deletion should be allowed")
	}
	if IndexDelete(rec, typ.LiteralString("required")) {
		t.Fatal("literal required field deletion should still be rejected")
	}
	if IndexDelete(rec, typ.String) {
		t.Fatal("broad string deletion could hit required fields and must be rejected")
	}
}
