package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/effect"
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
}

func TestInputsEqualIncludesVariantCaseFieldProjections(t *testing.T) {
	base := []VariantCaseFieldProjection{variantCaseProjectionForTest(7, 1)}
	same := []VariantCaseFieldProjection{variantCaseProjectionForTest(7, 1)}
	other := []VariantCaseFieldProjection{variantCaseProjectionForTest(7, 1)}
	other[0].SourceSteps = []effect.TypeProjectionStep{effect.ProjectGenericArg(1)}

	if !InputsEqual(&Inputs{VariantCaseFieldProjections: base}, &Inputs{VariantCaseFieldProjections: same}) {
		t.Fatalf("same variant case field projections should be equal")
	}
	if InputsEqual(&Inputs{VariantCaseFieldProjections: base}, &Inputs{VariantCaseFieldProjections: other}) {
		t.Fatalf("different source projection steps must affect InputsEqual")
	}
}

func TestNormalizeOrdersVariantOriginsAndCaseFieldProjections(t *testing.T) {
	in := Inputs{
		VariantFieldOrigins: []VariantFieldOrigin{
			variantOriginForTest(9, 2),
			variantOriginForTest(7, 2),
			variantOriginForTest(7, 1),
		},
		VariantCaseFieldProjections: []VariantCaseFieldProjection{
			variantCaseProjectionForTest(9, 2),
			variantCaseProjectionForTest(7, 2),
			variantCaseProjectionForTest(7, 1),
		},
	}
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

	projections := in.VariantCaseFieldProjections
	if len(projections) != 3 {
		t.Fatalf("normalized projections length = %d, want 3", len(projections))
	}
	if projections[0].OriginFamily != 7 || projections[0].CaseIndex != 1 ||
		projections[1].OriginFamily != 7 || projections[1].CaseIndex != 2 ||
		projections[2].OriginFamily != 9 || projections[2].CaseIndex != 2 {
		t.Fatalf("unexpected normalized projection order: %#v", projections)
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
	}
}

func variantCaseProjectionForTest(family uint64, caseIndex int) VariantCaseFieldProjection {
	return VariantCaseFieldProjection{
		Target: constraint.Path{
			Root:   "result",
			Symbol: cfg.SymbolID(1),
		},
		Field: "value",
		Source: constraint.Path{
			Root:   "timeout",
			Symbol: cfg.SymbolID(2),
		},
		SourceSteps:  []effect.TypeProjectionStep{effect.ProjectGenericArg(0)},
		OriginFamily: family,
		CaseIndex:    caseIndex,
	}
}
