package postcondition

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/domain/effect"
)

func TestNormalReturnRefinementEqualsHashAndString(t *testing.T) {
	label := NormalReturnRefinement{
		Target:     effect.ParamRef{Index: 0},
		Refinement: Present{},
	}
	same := NormalReturnRefinement{
		Target:     effect.ParamRef{Index: 0},
		Refinement: &Present{},
	}
	otherTarget := NormalReturnRefinement{
		Target:     effect.ParamRef{Index: 1},
		Refinement: Present{},
	}

	if label.Kind() != NormalReturnRefinementKind {
		t.Fatalf("Kind = %q, want %q", label.Kind(), NormalReturnRefinementKind)
	}
	if !label.Equals(same) || !same.Equals(label) {
		t.Fatalf("labels should be equal: %v / %v", label, same)
	}
	if label.Hash() != same.Hash() {
		t.Fatalf("hash = %d, want %d", label.Hash(), same.Hash())
	}
	if label.Equals(otherTarget) {
		t.Fatalf("labels with different targets should not be equal")
	}
	text := label.String()
	if !strings.Contains(text, "normal_return_refine") || !strings.Contains(text, "param[0]") || !strings.Contains(text, PresentKind) {
		t.Fatalf("String = %q, want target and refinement", text)
	}

	absent := NormalReturnRefinement{
		Target:     effect.ParamRef{Index: 0},
		Refinement: Absent{},
	}
	if label.Equals(absent) {
		t.Fatalf("present refinement should not equal absent refinement")
	}
	if !strings.Contains(absent.String(), AbsentKind) {
		t.Fatalf("absent String = %q, want refinement kind", absent.String())
	}
}

func TestNormalReturnRefinementNilRefinementBehaviorIsExplicit(t *testing.T) {
	label := NormalReturnRefinement{Target: effect.ParamRef{Index: 0}}
	same := NormalReturnRefinement{Target: effect.ParamRef{Index: 0}}
	withPresent := NormalReturnRefinement{Target: effect.ParamRef{Index: 0}, Refinement: Present{}}

	if !label.Equals(same) {
		t.Fatalf("nil-refinement labels with same target should compare equal")
	}
	if label.Equals(withPresent) {
		t.Fatalf("nil refinement should not equal present refinement")
	}
	if label.Hash() != same.Hash() {
		t.Fatalf("nil-refinement hash = %d, want %d", label.Hash(), same.Hash())
	}
	text := label.String()
	if !strings.Contains(text, "param[0]") || !strings.Contains(text, "<nil>") {
		t.Fatalf("String = %q, want explicit nil refinement", text)
	}
}

func TestPresentEqualsHashAndString(t *testing.T) {
	present := Present{}
	if present.Kind() != PresentKind {
		t.Fatalf("Kind = %q, want %q", present.Kind(), PresentKind)
	}
	if present.String() != PresentKind {
		t.Fatalf("String = %q, want %q", present.String(), PresentKind)
	}
	if !present.Equals(Present{}) || !present.Equals(&Present{}) {
		t.Fatalf("Present should equal value and pointer forms")
	}
	if present.Equals(nil) {
		t.Fatalf("Present should not equal nil")
	}
	if present.Hash() == 0 {
		t.Fatalf("Hash should be non-zero")
	}
}

func TestNormalizeRefinementCanonicalizesSupportedPointerAndValueForms(t *testing.T) {
	cases := []struct {
		name string
		in   Refinement
		want Refinement
		ok   bool
	}{
		{name: "present value", in: Present{}, want: Present{}, ok: true},
		{name: "present pointer", in: &Present{}, want: Present{}, ok: true},
		{name: "absent value", in: Absent{}, want: Absent{}, ok: true},
		{name: "absent pointer", in: &Absent{}, want: Absent{}, ok: true},
		{name: "nil", in: nil, ok: false},
	}
	for _, tc := range cases {
		got, ok := NormalizeRefinement(tc.in)
		if ok != tc.ok {
			t.Fatalf("%s: ok = %v, want %v", tc.name, ok, tc.ok)
		}
		if !tc.ok {
			continue
		}
		if !got.Equals(tc.want) {
			t.Fatalf("%s: got %T/%v, want %T/%v", tc.name, got, got, tc.want, tc.want)
		}
	}

	var nilPresent *Present
	if got, ok := NormalizeRefinement(nilPresent); ok || got != nil {
		t.Fatalf("nil *Present normalized to %T/%v, want none", got, got)
	}
	var nilAbsent *Absent
	if got, ok := NormalizeRefinement(nilAbsent); ok || got != nil {
		t.Fatalf("nil *Absent normalized to %T/%v, want none", got, got)
	}
}

func TestRefinementIsNilIncludesSupportedTypedNilPointers(t *testing.T) {
	if !RefinementIsNil(nil) {
		t.Fatal("nil refinement was not nil")
	}
	var nilPresent *Present
	if !RefinementIsNil(nilPresent) {
		t.Fatal("nil *Present was not nil")
	}
	var nilAbsent *Absent
	if !RefinementIsNil(nilAbsent) {
		t.Fatal("nil *Absent was not nil")
	}
	if RefinementIsNil(Present{}) || RefinementIsNil(&Present{}) || RefinementIsNil(Absent{}) || RefinementIsNil(&Absent{}) {
		t.Fatal("non-nil supported refinements reported nil")
	}
}

func TestAbsentEqualsHashAndString(t *testing.T) {
	absent := Absent{}
	if absent.Kind() != AbsentKind {
		t.Fatalf("Kind = %q, want %q", absent.Kind(), AbsentKind)
	}
	if absent.String() != AbsentKind {
		t.Fatalf("String = %q, want %q", absent.String(), AbsentKind)
	}
	if !absent.Equals(Absent{}) || !absent.Equals(&Absent{}) {
		t.Fatalf("Absent should equal value and pointer forms")
	}
	if absent.Equals(Present{}) || absent.Equals(nil) {
		t.Fatalf("Absent should only equal absent refinements")
	}
	if absent.Hash() == 0 {
		t.Fatalf("Hash should be non-zero")
	}
}
