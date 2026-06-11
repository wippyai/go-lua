package coalesce

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/identity"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestJoinNonDiscriminantFieldPreservesEqualLiteral(t *testing.T) {
	lit := typ.LiteralString("ready")
	got, ok := joinNonDiscriminantField(lit, typ.LiteralString("ready"))
	if !ok {
		t.Fatal("joinNonDiscriminantField returned !ok")
	}
	if !identity.TypeEquals(got, lit) {
		t.Fatalf("joinNonDiscriminantField(equal literals) = %v, want %v", got, lit)
	}
}

func TestJoinNonDiscriminantFieldWidensDifferingSameBaseLiterals(t *testing.T) {
	got, ok := joinNonDiscriminantField(typ.LiteralString("left"), typ.LiteralString("right"))
	if !ok {
		t.Fatal("joinNonDiscriminantField returned !ok")
	}
	if !identity.TypeEquals(got, typ.String) {
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

func TestJoinRecordFieldSlotWidensAccumulatedNonDiscriminantLiteralUnion(t *testing.T) {
	acc := typ.NewUnion(typ.LiteralString("a"), typ.LiteralString("b"))

	got := joinRecordFieldSlot(acc, typ.LiteralString("c"), RecordPolicy{})
	if !identity.TypeEquals(got, typ.String) {
		t.Fatalf("joinRecordFieldSlot(non-discriminant literal union) = %v, want string", got)
	}
}
