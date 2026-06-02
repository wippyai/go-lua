package product

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestVariantOriginRefinesProductOrderWithoutChangingProjection(t *testing.T) {
	base := FromType(typ.String)
	withOrigin := WithVariantOrigin(base, 11, []int{0, 1})
	case0, changed := NarrowVariantOriginCase(withOrigin, 11, 0, true)
	if !changed {
		t.Fatalf("NarrowVariantOriginCase reported no change")
	}
	if !withOrigin.Covers(case0) {
		t.Fatalf("origin union should cover narrowed case")
	}
	if case0.Covers(withOrigin) {
		t.Fatalf("narrowed case must not cover origin union")
	}
	if !withOrigin.ProjectValue().Equals(base.ProjectValue()) {
		t.Fatalf("variant origin changed projected type: got %s want %s", withOrigin.ProjectValue(), base.ProjectValue())
	}
}

func TestVariantOriginExclusionAndContradiction(t *testing.T) {
	base := WithVariantOrigin(FromType(typ.String), 11, []int{0, 1})
	case1, changed := NarrowVariantOriginCase(base, 11, 0, false)
	if !changed {
		t.Fatalf("case exclusion reported no change")
	}
	want := WithVariantOrigin(FromType(typ.String), 11, []int{1})
	if !Equal(case1, want) {
		t.Fatalf("exclude case = %#v, want %#v", case1, want)
	}

	impossible, changed := NarrowVariantOriginCase(case1, 11, 0, true)
	if !changed {
		t.Fatalf("contradictory case equality reported no change")
	}
	if !Equal(impossible, Bottom()) {
		t.Fatalf("contradictory case equality = %#v, want Bottom", impossible)
	}
}
