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

func TestOfAcceptsClosedGenericInstantiation(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{param},
		typetable.NewRecord().Field("value", param).Build())

	got := Of(typ.Instantiate(box, typ.String))
	if got.IsTop() || got.IsBottom() {
		t.Fatalf("closed generic witness = %v, want concrete", got)
	}
	if gotType, ok := got.Type(); !ok || !typ.TypeEquals(gotType, typ.Instantiate(box, typ.String)) {
		t.Fatalf("witness type = %v/%v, want Box<string>", gotType, ok)
	}
	if got := Of(typ.Instantiate(box, param)); !got.IsTop() {
		t.Fatalf("open generic witness = %v, want top", got)
	}
}
