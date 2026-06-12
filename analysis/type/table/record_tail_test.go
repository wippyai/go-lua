package table

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestRecordTailFieldTypeUsesStringMapComponent(t *testing.T) {
	rec := NewRecord().
		MapComponent(typ.String, typ.Number).
		Build()

	tail, ok := RecordTailFieldType(rec, "status")
	if !ok {
		t.Fatal("RecordTailFieldType returned ok=false")
	}
	want := typ.NewOptional(typ.Number)
	if !typ.TypeEquals(tail, want) {
		t.Fatalf("tail = %v, want %v", tail, want)
	}
	if !RecordMapTailMayContainFieldName(rec, "status") {
		t.Fatal("expected string map tail to contain status field")
	}
}

func TestRecordTailMayContainUsesExactMemberOverlapPolicy(t *testing.T) {
	rec := NewRecord().
		MapComponent(typ.LiteralString("raw"), typ.Number).
		Build()
	rawMember := typ.StaticMember{Kind: typ.StaticMemberStringIndex, Name: "raw"}
	otherMember := typ.StaticMember{Kind: typ.StaticMemberStringIndex, Name: "other"}

	if !RecordMapTailMayContainFieldName(rec, "raw") {
		t.Fatal("expected map tail to contain exact raw field")
	}
	if RecordMapTailMayContainFieldName(rec, "other") {
		t.Fatal("did not expect map tail to contain unrelated field")
	}
	if !RecordMapTailMayContainStaticMember(rec, rawMember) {
		t.Fatal("expected map tail to contain exact raw static member")
	}
	if RecordMapTailMayContainStaticMember(rec, otherMember) {
		t.Fatal("did not expect map tail to contain unrelated static member")
	}

	tail, ok := RecordTailStaticMemberType(rec, rawMember)
	if !ok {
		t.Fatal("RecordTailStaticMemberType returned ok=false")
	}
	want := typ.NewOptional(typ.Number)
	if !typ.TypeEquals(tail, want) {
		t.Fatalf("static member tail = %v, want %v", tail, want)
	}
}

func TestRecordMapTailStaticMemberContainment(t *testing.T) {
	rec := NewRecord().
		MapComponent(typ.NewUnion(typ.LiteralString("raw"), typ.Integer), typ.Boolean).
		Build()
	stringMember := typ.StaticMember{Kind: typ.StaticMemberStringIndex, Name: "raw"}
	intMember := typ.StaticMember{Kind: typ.StaticMemberIntIndex, Index: 7}
	missingString := typ.StaticMember{Kind: typ.StaticMemberStringIndex, Name: "other"}

	if !RecordMapTailMayContainStaticMember(rec, stringMember) {
		t.Fatal("expected map tail to contain static string member")
	}
	if !RecordMapTailMayContainStaticMember(rec, intMember) {
		t.Fatal("expected map tail to contain static int member")
	}
	if RecordMapTailMayContainStaticMember(rec, missingString) {
		t.Fatal("did not expect map tail to contain unrelated static string member")
	}

	tail, ok := RecordTailStaticMemberType(rec, intMember)
	if !ok {
		t.Fatal("RecordTailStaticMemberType returned ok=false")
	}
	want := typ.NewOptional(typ.Boolean)
	if !typ.TypeEquals(tail, want) {
		t.Fatalf("static member tail = %v, want %v", tail, want)
	}
}

func TestRecordTailTypeReturnsUnknownForOpenRecord(t *testing.T) {
	rec := NewRecord().
		SetOpen(true).
		Build()

	fieldTail, ok := RecordTailFieldType(rec, "missing")
	if !ok {
		t.Fatal("RecordTailFieldType(open record) returned ok=false")
	}
	if !typ.TypeEquals(fieldTail, typ.Unknown) {
		t.Fatalf("field tail = %v, want unknown", fieldTail)
	}

	member := typ.StaticMember{Kind: typ.StaticMemberStringIndex, Name: "missing"}
	memberTail, ok := RecordTailStaticMemberType(rec, member)
	if !ok {
		t.Fatal("RecordTailStaticMemberType(open record) returned ok=false")
	}
	if !typ.TypeEquals(memberTail, typ.Unknown) {
		t.Fatalf("static member tail = %v, want unknown", memberTail)
	}
	if RecordMapTailMayContainFieldName(rec, "missing") {
		t.Fatal("open record without map component should not report map-tail containment")
	}
}

func TestRecordTailFieldTypeKeepsOptionalMapValueShape(t *testing.T) {
	rec := NewRecord().
		MapComponent(typ.String, typ.NewOptional(typ.Number)).
		Build()

	tail, ok := RecordTailFieldType(rec, "maybe")
	if !ok {
		t.Fatal("RecordTailFieldType returned ok=false")
	}
	want := typ.NewOptional(typ.Number)
	if !typ.TypeEquals(tail, want) {
		t.Fatalf("tail = %v, want %v", tail, want)
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

	if !MapComponentKeyAdmitsType(typ.String, typ.NewUnion(typ.LiteralString("raw"), typ.LiteralString("name"))) {
		t.Fatal("expected string key domain to admit union of string literals")
	}
	if MapComponentKeyAdmitsType(typ.LiteralString("raw"), typ.NewUnion(typ.LiteralString("raw"), typ.LiteralString("name"))) {
		t.Fatal("did not expect exact literal domain to admit partially matching union key type")
	}
}
