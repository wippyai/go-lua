package coalesce

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestJoinNonDiscriminantFieldPreservesEqualLiteral(t *testing.T) {
	lit := typ.LiteralString("ready")
	got, ok := joinNonDiscriminantField(lit, typ.LiteralString("ready"))
	if !ok {
		t.Fatal("joinNonDiscriminantField returned !ok")
	}
	if !typ.TypeEquals(got, lit) {
		t.Fatalf("joinNonDiscriminantField(equal literals) = %v, want %v", got, lit)
	}
}

func TestJoinNonDiscriminantFieldWidensDifferingSameBaseLiterals(t *testing.T) {
	got, ok := joinNonDiscriminantField(typ.LiteralString("left"), typ.LiteralString("right"))
	if !ok {
		t.Fatal("joinNonDiscriminantField returned !ok")
	}
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("joinNonDiscriminantField(differing strings) = %v, want string", got)
	}
}

func TestJoinNonDiscriminantFieldFollowsLiteralAliases(t *testing.T) {
	lit := typ.LiteralString("ready")
	alias := typ.NewAlias("Status", lit)

	got, ok := joinNonDiscriminantField(alias, typ.LiteralString("ready"))
	if !ok {
		t.Fatal("joinNonDiscriminantField returned !ok")
	}
	if got != alias {
		t.Fatalf("joinNonDiscriminantField(alias literal) = %v, want alias", got)
	}
}

func TestMergeLiteralFamilyBasesIntegerAndNumberToNumber(t *testing.T) {
	got, ok := mergeLiteralFamilyBases(typ.Integer, typ.Number)
	if !ok {
		t.Fatal("mergeLiteralFamilyBases returned !ok")
	}
	if !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("mergeLiteralFamilyBases(integer, number) = %v, want number", got)
	}
}

func TestLiteralFamilyBaseLiteralUnion(t *testing.T) {
	u := typ.NewUnion(typ.LiteralInt(1), typ.LiteralNumber(2.5))
	got, ok := literalFamilyBase(u)
	if !ok {
		t.Fatal("literalFamilyBase returned !ok")
	}
	if !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("literalFamilyBase(integer-number literal union) = %v, want number", got)
	}
}

func TestJoinRecordFieldSlotWidensAccumulatedNonDiscriminantLiteralUnion(t *testing.T) {
	acc := typ.NewUnion(typ.LiteralString("a"), typ.LiteralString("b"))

	got := JoinRecordFieldSlot(acc, typ.LiteralString("c"), RecordPolicy{})
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("JoinRecordFieldSlot(non-discriminant literal union) = %v, want string", got)
	}
}
