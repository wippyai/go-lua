package postcondition

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
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
