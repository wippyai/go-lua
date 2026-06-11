package assertion

import (
	"testing"

	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
)

func TestAssertionLatticeLaws(t *testing.T) {
	suite := latticelaws.LawSuite[Value]{
		Name:   "assertion",
		Domain: Spec().Lattice(),
		Sample: []Value{
			Bottom(),
			Top(),
			Type(),
			Any(),
			NonNil(),
			Of(TypeAssertion, NonNilAssertion),
			Of(TypeAssertion, AnyAssertion, NonNilAssertion),
		},
		Format: Value.String,
	}
	suite.Run(t)
}

func TestJoinKeepsOnlyCommonAssertions(t *testing.T) {
	typeAndNonNil := Of(TypeAssertion, NonNilAssertion)
	typeAndAny := Of(TypeAssertion, AnyAssertion)

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

func TestCombineAddsSamePathAssertions(t *testing.T) {
	got := Combine(Type(), NonNil())
	if !got.Has(TypeAssertion) || !got.Has(NonNilAssertion) {
		t.Fatalf("Combine(type, non-nil) = %s, want both flags", got)
	}
	if !Equal(Combine(Top(), Any()), Any()) {
		t.Fatalf("Combine(top, any) should add any assertion")
	}
}

func TestHashStringAndFlagsDistinguishStates(t *testing.T) {
	typeValue := Type()
	anyValue := Any()
	nonNilValue := NonNil()
	if typeValue.Hash() == anyValue.Hash() || anyValue.Hash() == nonNilValue.Hash() || typeValue.Hash() == nonNilValue.Hash() {
		t.Fatal("distinct assertion states should hash distinctly in this sample")
	}
	if typeValue.String() != "assertion(type)" {
		t.Fatalf("type assertion string = %q", typeValue.String())
	}
	combo := Of(TypeAssertion, NonNilAssertion)
	flags := combo.Flags()
	if len(flags) != 2 || flags[0] != TypeAssertion || flags[1] != NonNilAssertion {
		t.Fatalf("combo flags = %v, want [type non-nil]", flags)
	}
	flags[0] = AnyAssertion
	if !combo.Has(TypeAssertion) {
		t.Fatalf("mutating Flags result changed combo: %s", combo)
	}
}
