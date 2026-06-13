package nodeid

import (
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestPointer(t *testing.T) {
	rec := typetable.NewRecord().Field("name", typ.String).Build()
	opt := typ.NewOptional(typ.String)
	if got := Pointer(nil); got != 0 {
		t.Fatalf("Pointer(nil) = %d, want 0", got)
	}
	if got := Pointer(rec); got == 0 {
		t.Fatalf("Pointer(record) = 0, want stable pointer")
	}
	if got := Pointer(opt); got == 0 {
		t.Fatalf("Pointer(optional) = 0, want stable pointer")
	}
	if got := Pointer(typ.String); got != 0 {
		t.Fatalf("Pointer(scalar singleton) = %d, want 0", got)
	}
}

func TestPointerTypedNil(t *testing.T) {
	var rec *typ.Record
	if got := Pointer(rec); got != 0 {
		t.Fatalf("Pointer(typed nil) = %d, want 0", got)
	}
}

func TestStructuralPointer(t *testing.T) {
	rec := typetable.NewRecord().Field("name", typ.String).Build()
	if got := StructuralPointer(nil); got != 0 {
		t.Fatalf("StructuralPointer(nil) = %d, want 0", got)
	}
	if got := StructuralPointer(rec); got == 0 {
		t.Fatalf("StructuralPointer(record) = 0, want stable pointer")
	}
	if got := StructuralPointer(typ.NewRecursivePlaceholder("Node")); got == 0 {
		t.Fatalf("StructuralPointer(recursive) = 0, want stable pointer")
	}
}

func TestStructuralPointerTypedNil(t *testing.T) {
	var rec *typ.Record
	if got := StructuralPointer(rec); got != 0 {
		t.Fatalf("StructuralPointer(typed nil) = %d, want 0", got)
	}
}

func TestStructuralPointerExcludesWrappersAndScalars(t *testing.T) {
	tests := []struct {
		name string
		t    typ.Type
	}{
		{name: "optional", t: typ.NewOptional(typ.String)},
		{name: "array", t: typ.NewArray(typ.String)},
		{name: "map", t: typ.NewMap(typ.String, typ.Number)},
		{name: "readonly map", t: typ.NewReadonlyMap(typ.String, typ.Number)},
		{name: "tuple", t: typ.NewTuple(typ.String, typ.Number)},
		{name: "alias", t: typ.NewAlias("Name", typ.String)},
		{name: "ref", t: typ.NewRef("", "Name")},
		{name: "scalar", t: typ.String},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StructuralPointer(tt.t); got != 0 {
				t.Fatalf("StructuralPointer(%s) = %d, want 0", tt.name, got)
			}
		})
	}
}
