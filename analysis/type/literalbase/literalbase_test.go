package literalbase_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/literalbase"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestJoinNonDiscriminantFieldPreservesEqualLiteral(t *testing.T) {
	lit := typ.LiteralString("ready")
	got, ok := literalbase.JoinNonDiscriminantField(lit, typ.LiteralString("ready"))
	if !ok {
		t.Fatal("JoinNonDiscriminantField returned !ok")
	}
	if !typ.TypeEquals(got, lit) {
		t.Fatalf("JoinNonDiscriminantField(equal literals) = %v, want %v", got, lit)
	}
}

func TestJoinNonDiscriminantFieldWidensDifferingSameBaseLiterals(t *testing.T) {
	got, ok := literalbase.JoinNonDiscriminantField(typ.LiteralString("left"), typ.LiteralString("right"))
	if !ok {
		t.Fatal("JoinNonDiscriminantField returned !ok")
	}
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("JoinNonDiscriminantField(differing strings) = %v, want string", got)
	}
}

func TestMergeBasesIntegerAndNumberToNumber(t *testing.T) {
	got, ok := literalbase.MergeBases(typ.Integer, typ.Number)
	if !ok {
		t.Fatal("MergeBases returned !ok")
	}
	if !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("MergeBases(integer, number) = %v, want number", got)
	}
}

func TestFamilyBaseLiteralUnion(t *testing.T) {
	u := typ.NewUnion(typ.LiteralInt(1), typ.LiteralNumber(2.5))
	got, ok := literalbase.FamilyBase(u)
	if !ok {
		t.Fatal("FamilyBase returned !ok")
	}
	if !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("FamilyBase(integer-number literal union) = %v, want number", got)
	}
}

func TestExtractFollowsAliases(t *testing.T) {
	lit := typ.LiteralString("tag")
	alias := typ.NewAlias("Tag", typ.NewAlias("InnerTag", lit))

	got, ok := literalbase.Extract(alias)
	if !ok {
		t.Fatal("Extract(alias literal) returned !ok")
	}
	if got != lit {
		t.Fatalf("Extract(alias literal) = %v, want original literal", got)
	}
}
