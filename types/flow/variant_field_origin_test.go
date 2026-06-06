package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
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

func TestVariantCaseFieldProjectionValuesJoinSelectedPayloads(t *testing.T) {
	resultSym := cfg.SymbolID(11)
	eventsSym := cfg.SymbolID(12)
	timersSym := cfg.SymbolID(13)
	family := uint64(91)
	result := constraint.Path{Root: "selected", Symbol: resultSym}
	events := constraint.Path{Root: "events", Symbol: eventsSym}
	timers := constraint.Path{Root: "timers", Symbol: timersSym}
	channel := typ.NewGeneric("Channel", []*typ.TypeParam{typ.NewTypeParam("T", nil)}, typ.NewInterface("Channel", nil))
	state := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(eventsSym): product.FromType(typ.Instantiate(channel, typ.String)),
			SymbolValueKey(timersSym): product.FromType(typ.Instantiate(channel, typ.Number)),
		},
	}
	projections := []VariantCaseFieldProjection{
		{
			Target:       result,
			Field:        "value",
			Source:       events,
			SourceSteps:  []effect.TypeProjectionStep{effect.ProjectGenericArg(0)},
			OriginFamily: family,
			CaseIndex:    0,
		},
		{
			Target:       result,
			Field:        "value",
			Source:       timers,
			SourceSteps:  []effect.TypeProjectionStep{effect.ProjectGenericArg(0)},
			OriginFamily: family,
			CaseIndex:    1,
		},
	}
	fact := constraint.Or(
		constraint.FromConstraints(constraint.VariantCaseEquals{Target: result, OriginFamily: family, CaseIndex: 0}),
		constraint.FromConstraints(constraint.VariantCaseEquals{Target: result, OriginFamily: family, CaseIndex: 1}),
	)

	values := VariantCaseFieldProjectionValues(state, fact, projections)
	if len(values) != 1 {
		t.Fatalf("projection values = %#v, want one joined selected.value proof", values)
	}
	if !values[0].Path.Equal(result.Field("value")) {
		t.Fatalf("projected path = %s, want selected.value", values[0].Path.String())
	}
	want := typ.NewUnion(typ.String, typ.Number)
	if !typ.TypeEquals(values[0].Value.ProjectValue(), want) {
		t.Fatalf("projected payload = %v, want %v", values[0].Value.ProjectValue(), want)
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
