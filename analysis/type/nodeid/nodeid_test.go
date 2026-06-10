package nodeid

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestPointer(t *testing.T) {
	rec := typ.NewRecord().Field("name", typ.String).Build()
	if got := Pointer(nil); got != 0 {
		t.Fatalf("Pointer(nil) = %d, want 0", got)
	}
	if got := Pointer(rec); got == 0 {
		t.Fatalf("Pointer(record) = 0, want stable pointer")
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
