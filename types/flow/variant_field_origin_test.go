package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

func TestInputsEqualIncludesVariantOriginFamilyAndCase(t *testing.T) {
	base := []VariantFieldOrigin{variantOriginForTest(7, 1)}
	same := []VariantFieldOrigin{variantOriginForTest(7, 1)}
	otherFamily := []VariantFieldOrigin{variantOriginForTest(8, 1)}
	otherCase := []VariantFieldOrigin{variantOriginForTest(7, 2)}

	if !InputsEqual(&Inputs{VariantFieldOrigins: base}, &Inputs{VariantFieldOrigins: same}) {
		t.Fatalf("same variant-origin facts should be equal")
	}
	if InputsEqual(&Inputs{VariantFieldOrigins: base}, &Inputs{VariantFieldOrigins: otherFamily}) {
		t.Fatalf("different origin family must affect InputsEqual")
	}
	if InputsEqual(&Inputs{VariantFieldOrigins: base}, &Inputs{VariantFieldOrigins: otherCase}) {
		t.Fatalf("different case index must affect InputsEqual")
	}
	otherProjection := []VariantFieldOrigin{variantOriginForTest(7, 1)}
	otherProjection[0].ProjectionField = "payload"
	if InputsEqual(&Inputs{VariantFieldOrigins: base}, &Inputs{VariantFieldOrigins: otherProjection}) {
		t.Fatalf("different projection field must affect InputsEqual")
	}
}

func TestNormalizeOrdersVariantOriginsByFamilyAndCase(t *testing.T) {
	in := Inputs{VariantFieldOrigins: []VariantFieldOrigin{
		variantOriginForTest(9, 2),
		variantOriginForTest(7, 2),
		variantOriginForTest(7, 1),
	}}
	in.Normalize()

	got := in.VariantFieldOrigins
	if len(got) != 3 {
		t.Fatalf("normalized origins length = %d, want 3", len(got))
	}
	if got[0].OriginFamily != 7 || got[0].CaseIndex != 1 ||
		got[1].OriginFamily != 7 || got[1].CaseIndex != 2 ||
		got[2].OriginFamily != 9 || got[2].CaseIndex != 2 {
		t.Fatalf("unexpected normalized order: %#v", got)
	}
}

func variantOriginForTest(family uint64, caseIndex int) VariantFieldOrigin {
	return VariantFieldOrigin{
		Target: constraint.Path{
			Root:   "result",
			Symbol: cfg.SymbolID(1),
		},
		Field: "channel",
		Source: constraint.Path{
			Root:   "timeout",
			Symbol: cfg.SymbolID(2),
		},
		OriginFamily: family,
		CaseIndex:    caseIndex,
		ProjectionField: "value",
	}
}
