package table

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestMapComponentKeyMayContainString(t *testing.T) {
	if !MapComponentKeyMayContainString(typ.String, "status") {
		t.Fatal("expected string key domain to contain status")
	}
	if !MapComponentKeyMayContainString(typ.LiteralString("raw"), "raw") {
		t.Fatal("expected literal string key domain to contain exact raw key")
	}
	if MapComponentKeyMayContainString(typ.LiteralString("raw"), "other") {
		t.Fatal("did not expect literal string key domain to contain unrelated key")
	}
	if !MapComponentKeyMayContainString(typeexpr.Union(typ.Integer, typ.LiteralString("raw")), "raw") {
		t.Fatal("expected union key domain to contain raw string key")
	}
	if MapComponentKeyMayContainString(typeexpr.Union(typ.Integer, typ.LiteralString("raw")), "other") {
		t.Fatal("did not expect union key domain to contain unrelated string key")
	}
}

func TestMapComponentKeyMayContainInt(t *testing.T) {
	if !MapComponentKeyMayContainInt(typ.Integer, 7) {
		t.Fatal("expected integer key domain to contain int key")
	}
	if !MapComponentKeyMayContainInt(typ.Number, 7) {
		t.Fatal("expected number key domain to contain integer-valued key")
	}
	if !MapComponentKeyMayContainInt(typ.LiteralInt(7), 7) {
		t.Fatal("expected literal int key domain to contain exact int key")
	}
	if !MapComponentKeyMayContainInt(typ.LiteralNumber(7), 7) {
		t.Fatal("expected integer-valued number literal key domain to contain int key")
	}
	if MapComponentKeyMayContainInt(typ.LiteralInt(7), 8) {
		t.Fatal("did not expect literal int key domain to contain unrelated int key")
	}
}

func TestMapComponentKeyMayContainStaticMember(t *testing.T) {
	key := typeexpr.Union(typ.LiteralString("raw"), typ.Integer)
	stringMember := typ.StaticMember{Kind: typ.StaticMemberStringIndex, Name: "raw"}
	intMember := typ.StaticMember{Kind: typ.StaticMemberIntIndex, Index: 7}
	missingString := typ.StaticMember{Kind: typ.StaticMemberStringIndex, Name: "other"}
	unknownMember := typ.StaticMember{}

	if !MapComponentKeyMayContainStaticMember(key, stringMember) {
		t.Fatal("expected key domain to contain exact static string member")
	}
	if !MapComponentKeyMayContainStaticMember(key, intMember) {
		t.Fatal("expected key domain to contain static int member")
	}
	if MapComponentKeyMayContainStaticMember(key, missingString) {
		t.Fatal("did not expect key domain to contain unrelated static string member")
	}
	if MapComponentKeyMayContainStaticMember(key, unknownMember) {
		t.Fatal("did not expect key domain to contain unsupported static member kind")
	}
}

func TestMapComponentKeyAdmitsTypeUsesCanonicalPredicate(t *testing.T) {
	if !MapComponentKeyAdmitsType(typ.String, typ.String) {
		t.Fatal("expected string key domain to admit string key type")
	}
	if MapComponentKeyAdmitsType(typ.LiteralString("raw"), typ.String) {
		t.Fatal("did not expect literal string domain to admit broad string key type")
	}

	if !MapComponentKeyAdmitsType(typ.Integer, typ.Integer) {
		t.Fatal("expected integer key domain to admit integer key type")
	}
	if MapComponentKeyAdmitsType(typ.LiteralInt(7), typ.Integer) {
		t.Fatal("did not expect literal int domain to admit broad integer key type")
	}

	if !MapComponentKeyAdmitsType(typ.String, typeexpr.Union(typ.LiteralString("raw"), typ.LiteralString("name"))) {
		t.Fatal("expected string key domain to admit union of string literals")
	}
	if MapComponentKeyAdmitsType(typ.LiteralString("raw"), typeexpr.Union(typ.LiteralString("raw"), typ.LiteralString("name"))) {
		t.Fatal("did not expect exact literal domain to admit partially matching union key type")
	}
}
