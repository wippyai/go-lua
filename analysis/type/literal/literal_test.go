package literal

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestExtractAliasOnlyFollowsAliases(t *testing.T) {
	lit := typ.LiteralString("tag")
	alias := typ.NewAlias("Tag", typ.NewAlias("InnerTag", lit))

	got, ok := ExtractAliasOnly(alias)
	if !ok {
		t.Fatal("ExtractAliasOnly(alias literal) returned !ok")
	}
	if got != lit {
		t.Fatalf("ExtractAliasOnly(alias literal) = %v, want original literal", got)
	}
}

func TestExtractAliasOnlyDoesNotUnwrapAnnotations(t *testing.T) {
	lit := typ.LiteralString("tag")
	annotated := &typ.Annotated{Inner: lit}

	if got, ok := ExtractAliasOnly(annotated); ok {
		t.Fatalf("ExtractAliasOnly(annotated literal) = %v, want !ok", got)
	}
}

func TestPrimitiveBase(t *testing.T) {
	tests := []struct {
		name string
		lit  *typ.Literal
		want typ.Type
	}{
		{name: "boolean", lit: typ.LiteralBool(true), want: typ.Boolean},
		{name: "integer", lit: typ.LiteralInt(1), want: typ.Integer},
		{name: "number", lit: typ.LiteralNumber(1.5), want: typ.Number},
		{name: "string", lit: typ.LiteralString("x"), want: typ.String},
		{name: "nil", lit: nil, want: nil},
		{name: "unknown base", lit: &typ.Literal{Base: kind.Nil, Value: nil}, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PrimitiveBase(tt.lit); got != tt.want {
				t.Fatalf("PrimitiveBase(%v) = %v, want %v", tt.lit, got, tt.want)
			}
		})
	}
}

func TestFamilyBaseLiteralUnion(t *testing.T) {
	u := typeexpr.Union(typ.LiteralInt(1), typ.LiteralNumber(2.5))

	got, ok := FamilyBase(u)
	if !ok {
		t.Fatal("FamilyBase(integer-number literal union) returned !ok")
	}
	if !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("FamilyBase(integer-number literal union) = %v, want number", got)
	}
}

func TestFamilyBaseUnwrapsAnnotatedAlias(t *testing.T) {
	lit := typ.LiteralString("tag")
	annotated := &typ.Annotated{Inner: typ.NewAlias("Tag", lit)}

	got, ok := FamilyBase(annotated)
	if !ok {
		t.Fatal("FamilyBase(annotated alias literal) returned !ok")
	}
	if got != typ.String {
		t.Fatalf("FamilyBase(annotated alias literal) = %v, want string", got)
	}
}

func TestMergeFamilyBasesIntegerAndNumberToNumber(t *testing.T) {
	got, ok := MergeFamilyBases(typ.Integer, typ.Number)
	if !ok {
		t.Fatal("MergeFamilyBases(integer, number) returned !ok")
	}
	if got != typ.Number {
		t.Fatalf("MergeFamilyBases(integer, number) = %v, want number", got)
	}
}

func TestMergeFamilyBasesRejectsUnrelatedBases(t *testing.T) {
	if got, ok := MergeFamilyBases(typ.String, typ.Boolean); ok {
		t.Fatalf("MergeFamilyBases(string, boolean) = %v, want !ok", got)
	}
}
