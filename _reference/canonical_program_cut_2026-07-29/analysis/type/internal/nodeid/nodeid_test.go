package nodeid

import (
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestPointer(t *testing.T) {
	rec := typetable.NewRecord().Field("name", typ.String).Build()
	opt := typ.MaterializeOptional(typ.String)
	if got := Pointer(nil); got != 0 {
		t.Fatalf("Pointer(nil) = %d, want 0", got)
	}
	if got := Pointer(rec); got == 0 {
		t.Fatalf("Pointer(record) = 0, want stable pointer")
	}
	if got := Pointer(opt); got == 0 {
		t.Fatalf("Pointer(optional) = 0, want stable pointer")
	}
	for name, singleton := range map[string]typ.Type{
		"nil":     typ.Nil,
		"boolean": typ.Boolean,
		"number":  typ.Number,
		"integer": typ.Integer,
		"string":  typ.String,
		"any":     typ.Any,
		"unknown": typ.Unknown,
		"never":   typ.Never,
		"self":    typ.Self,
	} {
		if got := Pointer(singleton); got != 0 {
			t.Fatalf("Pointer(%s singleton) = %d, want 0", name, got)
		}
	}
}

func TestPointerTypedNil(t *testing.T) {
	var rec *typ.Record
	if got := Pointer(rec); got != 0 {
		t.Fatalf("Pointer(typed nil) = %d, want 0", got)
	}
}
