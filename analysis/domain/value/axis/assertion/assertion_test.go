package assertion

import (
	"testing"

	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
)

func TestClaimLatticeLaws(t *testing.T) {
	suite := latticelaws.LawSuite[Value]{
		Name:   "assertion",
		Domain: Spec().Lattice(),
		Sample: []Value{
			Bottom(),
			Top(),
			Type(),
			Any(),
			NonNil(),
			Of(TypeClaim, NonNilClaim),
			Of(TypeClaim, AnyClaim, NonNilClaim),
		},
		Format: Value.String,
	}
	suite.Run(t)
}

func TestJoinKeepsOnlyCommonClaims(t *testing.T) {
	typeAndNonNil := Of(TypeClaim, NonNilClaim)
	typeAndAny := Of(TypeClaim, AnyClaim)

	if got := Join(typeAndNonNil, typeAndAny); !Equal(got, Type()) {
		t.Fatalf("Join(%s,%s) = %s, want %s", typeAndNonNil, typeAndAny, got, Type())
	}
	if got := Join(Type(), Any()); !Equal(got, Top()) {
		t.Fatalf("Join(type, any) = %s, want top/no indicator", got)
	}
	if got := Join(Bottom(), NonNil()); !Equal(got, NonNil()) {
		t.Fatalf("Join(bottom, non-nil) = %s, want non-nil", got)
	}
}

func TestCombineAddsSamePathClaims(t *testing.T) {
	got := Combine(Type(), NonNil())
	if !got.Has(TypeClaim) || !got.Has(NonNilClaim) {
		t.Fatalf("Combine(type, non-nil) = %s, want both flags", got)
	}
	if !Equal(Combine(Top(), Any()), Any()) {
		t.Fatalf("Combine(top, any) should add any claim")
	}
}

func TestHashStringAndFlagsDistinguishStates(t *testing.T) {
	typeValue := Type()
	anyValue := Any()
	nonNilValue := NonNil()
	if typeValue.Hash() == anyValue.Hash() || anyValue.Hash() == nonNilValue.Hash() || typeValue.Hash() == nonNilValue.Hash() {
		t.Fatal("distinct claim states should hash distinctly in this sample")
	}
	if typeValue.String() != "assertion(type)" {
		t.Fatalf("type claim string = %q", typeValue.String())
	}
	combo := Of(TypeClaim, NonNilClaim)
	flags := combo.Flags()
	if len(flags) != 2 || flags[0] != TypeClaim || flags[1] != NonNilClaim {
		t.Fatalf("combo flags = %v, want [type non-nil]", flags)
	}
	flags[0] = AnyClaim
	if !combo.Has(TypeClaim) {
		t.Fatalf("mutating Flags result changed combo: %s", combo)
	}
}
