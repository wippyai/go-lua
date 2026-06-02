package interproc

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func fieldKey(name string) FieldKey {
	key, ok := FieldKeyFromName(name)
	if !ok {
		panic("empty test field key")
	}
	return key
}

func TestLiftTypeFieldMapUsesTypedFieldKeys(t *testing.T) {
	fields := LiftTypeFieldMap(map[string]typ.Type{"name": typ.String})
	key := fieldKey("name")
	if len(fields) != 1 || !product.Equal(fields[key], product.FromType(typ.String)) {
		t.Fatalf("LiftTypeFieldMap = %#v, want typed name field", fields)
	}
}

func TestSortedFieldKeysIsDeterministic(t *testing.T) {
	fields := FieldValues{
		{Kind: constraint.SegmentIndexInt, Index: 2}:        product.FromType(typ.Number),
		{Kind: constraint.SegmentField, Name: "z"}:          product.FromType(typ.String),
		{Kind: constraint.SegmentField, Name: "a"}:          product.FromType(typ.Boolean),
		{Kind: constraint.SegmentIndexString, Name: "item"}: product.FromType(typ.Unknown),
	}
	keys := SortedFieldKeys(fields)
	want := []FieldKey{
		{Kind: constraint.SegmentField, Name: "a"},
		{Kind: constraint.SegmentField, Name: "z"},
		{Kind: constraint.SegmentIndexString, Name: "item"},
		{Kind: constraint.SegmentIndexInt, Index: 2},
	}
	if len(keys) != len(want) {
		t.Fatalf("SortedFieldKeys len = %d, want %d", len(keys), len(want))
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("SortedFieldKeys[%d] = %#v, want %#v", i, keys[i], want[i])
		}
	}
}

func TestProjectValueFieldMapKeepsBoundaryShape(t *testing.T) {
	fields := FieldValues{
		fieldKey("name"): product.FromType(typ.String),
	}
	projected := ProjectValueFieldMap(fields)
	if len(projected) != 1 || !typ.TypeEquals(projected["name"], typ.String) {
		t.Fatalf("ProjectValueFieldMap = %#v, want name:string", projected)
	}
}
