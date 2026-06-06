package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestKeyDomainFromTypeRecordAndMap(t *testing.T) {
	record := typ.NewRecord().
		Field("id", typ.String).
		Field("name", typ.String).
		Build()
	got := KeyDomainFromType(record)
	if !typ.TypeEquals(got, typ.NewUnion(typ.LiteralString("id"), typ.LiteralString("name"))) {
		t.Fatalf("record key domain = %s, want id|name", got)
	}

	got = KeyDomainFromType(typ.NewMap(typ.Number, typ.String))
	if !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("map key domain = %s, want number", got)
	}
}

func TestKeyDomainFromTypeUnionDropsNonMapMembers(t *testing.T) {
	got := KeyDomainFromType(typ.NewUnion(
		typ.NewMap(typ.String, typ.Number),
		typ.Number,
		typ.NewRecord().Field("id", typ.String).Build(),
	))
	want := typ.NewUnion(typ.String, typ.LiteralString("id"))
	if !typ.TypeEquals(got, want) {
		t.Fatalf("union key domain = %s, want %s", got, want)
	}
}

func TestPointFactsKeyDomainAtAddressUsesAddressValue(t *testing.T) {
	const sym = cfg.SymbolID(31)
	path := constraint.NewPath(sym, "tables").Field("nodes")
	addr := testStableAddressPath(t, path)
	state := PointState{
		StaticMembers: StaticMemberFactsDomain.Top().WithAddress(
			addr,
			product.FromType(typ.NewRecord().Field("id", typ.String).Build()),
		),
	}

	got, ok := PointFactsOf(state).KeyDomainAtAddress(addr)
	if !ok || !typ.TypeEquals(got, typ.LiteralString("id")) {
		t.Fatalf("KeyDomainAtAddress = %s/%v, want literal id", got, ok)
	}
}
