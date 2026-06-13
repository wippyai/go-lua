package typewitness

import (
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestOfRejectsOpenTypeParameter(t *testing.T) {
	if got := Of(typ.NewTypeParam("T", nil)); !got.IsTop() {
		t.Fatalf("type parameter witness = %v, want top", got)
	}
}

func TestOfAcceptsClosedOptionalRecord(t *testing.T) {
	record := typetable.NewRecord().Field("value", typ.String).Build()
	got := Of(typ.NewOptional(record))
	if got.IsTop() || got.IsBottom() {
		t.Fatalf("closed optional record witness = %v, want concrete", got)
	}
	if gotType, ok := got.Type(); !ok || !typ.TypeEquals(gotType, typ.NewOptional(record)) {
		t.Fatalf("witness type = %v/%v, want optional record", gotType, ok)
	}
}
