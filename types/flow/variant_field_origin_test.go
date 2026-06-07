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

func TestVariantFieldPathRelationConstraintsSelectMatchingCase(t *testing.T) {
	target := constraint.Path{Root: "selected", Symbol: cfg.SymbolID(11), Version: 2}
	source := constraint.Path{Root: "events", Symbol: cfg.SymbolID(12)}
	origin := VariantFieldOrigin{
		Target:       target,
		Field:        "channel",
		Source:       source,
		OriginFamily: 91,
		CaseIndex:    3,
	}

	got := VariantFieldPathRelationConstraints(VariantFieldPathRelation{
		Origins: []VariantFieldOrigin{origin},
		Target:  target,
		Field:   "channel",
		Source:  source,
		Kind:    VariantFieldPathEquals,
	})
	if len(got) != 2 {
		t.Fatalf("equality constraints = %#v, want base relation and selected case", got)
	}
	base, ok := got[0].(constraint.FieldEqualsPath)
	if !ok {
		t.Fatalf("constraint = %#v, want FieldEqualsPath", got[0])
	}
	if !base.Target.Equal(target) || base.Field != "channel" || !base.Value.Equal(source) {
		t.Fatalf("base relation = %#v, want target channel source relation", base)
	}
	eq, ok := got[1].(constraint.VariantCaseEquals)
	if !ok {
		t.Fatalf("constraint = %#v, want VariantCaseEquals", got[1])
	}
	if !eq.Target.Equal(target) || eq.OriginFamily != 91 || eq.CaseIndex != 3 {
		t.Fatalf("constraint = %#v, want selected family/case", eq)
	}
}

func TestVariantFieldPathRelationConstraintsWithoutOriginsKeepsBaseRelation(t *testing.T) {
	target := constraint.Path{Root: "selected", Symbol: cfg.SymbolID(11)}
	source := constraint.Path{Root: "events", Symbol: cfg.SymbolID(12)}

	got := VariantFieldPathRelationConstraints(VariantFieldPathRelation{
		Target: target,
		Field:  "channel",
		Source: source,
		Kind:   VariantFieldPathEquals,
	})
	if len(got) != 1 {
		t.Fatalf("equality constraints = %#v, want base relation only", got)
	}
	base, ok := got[0].(constraint.FieldEqualsPath)
	if !ok {
		t.Fatalf("constraint = %#v, want FieldEqualsPath", got[0])
	}
	if !base.Target.Equal(target) || base.Field != "channel" || !base.Value.Equal(source) {
		t.Fatalf("base relation = %#v, want target channel source relation", base)
	}
}

func TestVariantFieldPathRelationConstraintsExcludeMatchingCase(t *testing.T) {
	target := constraint.Path{Root: "selected", Symbol: cfg.SymbolID(11)}
	source := constraint.Path{Root: "events", Symbol: cfg.SymbolID(12)}
	origin := VariantFieldOrigin{
		Target:       target,
		Field:        "channel",
		Source:       source,
		OriginFamily: 91,
		CaseIndex:    3,
	}

	got := VariantFieldPathRelationConstraints(VariantFieldPathRelation{
		Origins: []VariantFieldOrigin{origin},
		Target:  target,
		Field:   "channel",
		Source:  source,
		Kind:    VariantFieldPathNotEquals,
	})
	if len(got) != 2 {
		t.Fatalf("not-equality constraints = %#v, want base relation and excluded case", got)
	}
	base, ok := got[0].(constraint.FieldNotEqualsPath)
	if !ok {
		t.Fatalf("constraint = %#v, want FieldNotEqualsPath", got[0])
	}
	if !base.Target.Equal(target) || base.Field != "channel" || !base.Value.Equal(source) {
		t.Fatalf("base relation = %#v, want target channel source relation", base)
	}
	neq, ok := got[1].(constraint.VariantCaseNotEquals)
	if !ok {
		t.Fatalf("constraint = %#v, want VariantCaseNotEquals", got[1])
	}
	if !neq.Target.Equal(target) || neq.OriginFamily != 91 || neq.CaseIndex != 3 {
		t.Fatalf("constraint = %#v, want excluded family/case", neq)
	}
}

func TestVariantOriginPathMatchesVersionAgnosticCounterpart(t *testing.T) {
	origin := constraint.Path{Root: "selected", Symbol: cfg.SymbolID(11)}
	actual := constraint.Path{Root: "selected", Symbol: cfg.SymbolID(11), Version: 7}
	if !VariantOriginPathMatches(origin, actual) {
		t.Fatalf("version-agnostic origin should match versioned actual path")
	}

	other := actual
	other.Version = 8
	if VariantOriginPathMatches(actual, other) {
		t.Fatalf("different concrete versions must not match")
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

func TestApplyVariantCaseFieldProjectionsWritesStaticMemberFact(t *testing.T) {
	resultSym := cfg.SymbolID(14)
	sourceSym := cfg.SymbolID(15)
	family := uint64(92)
	result := constraint.Path{Root: "selected", Symbol: resultSym}
	source := constraint.Path{Root: "events", Symbol: sourceSym}
	payload := typ.NewRecord().Field("id", typ.String).Build()
	channel := typ.NewGeneric("Channel", []*typ.TypeParam{typ.NewTypeParam("T", nil)}, typ.NewInterface("Channel", nil))
	state := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(sourceSym): product.FromType(typ.Instantiate(channel, payload)),
		},
	}
	SetStaticMemberPath(&state, result.Field("value").Field("id"), product.FromType(typ.String))

	changed := ApplyVariantCaseFieldProjections(&state,
		constraint.FromConstraints(constraint.VariantCaseEquals{Target: result, OriginFamily: family, CaseIndex: 0}),
		[]VariantCaseFieldProjection{{
			Target:       result,
			Field:        "value",
			Source:       source,
			SourceSteps:  []effect.TypeProjectionStep{effect.ProjectGenericArg(0)},
			OriginFamily: family,
			CaseIndex:    0,
		}},
	)
	if !changed {
		t.Fatal("ApplyVariantCaseFieldProjections reported no change")
	}
	valueFact, ok := PointFactsOf(state).PathValue(result.Field("value"))
	if !ok || !typ.TypeEquals(valueFact.ProjectValue(), payload) {
		t.Fatalf("projected value = %v/%v, want payload record", valueFact.ProjectValue(), ok)
	}
	childFact, ok := PointFactsOf(state).PathValue(result.Field("value").Field("id"))
	if !ok || !typ.TypeEquals(childFact.ProjectValue(), typ.String) {
		t.Fatalf("existing projected child = %v/%v, want string", childFact.ProjectValue(), ok)
	}
}

func TestVariantOriginConditionReducerJoinsDNFCaseReductions(t *testing.T) {
	sym := cfg.SymbolID(21)
	family := uint64(101)
	path := constraint.Path{Root: "selected", Symbol: sym}
	base := product.WithVariantOrigin(product.FromType(typ.String), family, []int{0, 1, 2})
	fact := constraint.Or(
		constraint.FromConstraints(constraint.VariantCaseEquals{Target: path, OriginFamily: family, CaseIndex: 0}),
		constraint.FromConstraints(constraint.VariantCaseEquals{Target: path, OriginFamily: family, CaseIndex: 1}),
	)

	reductions := VariantOriginConditionReducer{
		SymbolValue: func(got cfg.SymbolID) (product.AbstractValue, bool) {
			if got != sym {
				return product.AbstractValue{}, false
			}
			return base, true
		},
	}.Reductions(fact)
	if len(reductions) != 1 {
		t.Fatalf("reductions = %#v, want one symbol reduction", reductions)
	}
	want := product.WithVariantOrigin(product.FromType(typ.String), family, []int{0, 1})
	if reductions[0].Symbol != sym || !product.Domain.Equal(reductions[0].Value, want) {
		t.Fatalf("reduction = %#v, want %d -> %#v", reductions[0], sym, want)
	}
}

func TestVariantOriginConditionReducerIgnoresNestedTargets(t *testing.T) {
	sym := cfg.SymbolID(22)
	family := uint64(102)
	path := constraint.Path{Root: "selected", Symbol: sym}.Field("value")
	base := product.WithVariantOrigin(product.FromType(typ.String), family, []int{0, 1})
	fact := constraint.FromConstraints(constraint.VariantCaseEquals{Target: path, OriginFamily: family, CaseIndex: 0})

	reductions := VariantOriginConditionReducer{
		SymbolValue: func(got cfg.SymbolID) (product.AbstractValue, bool) {
			if got != sym {
				return product.AbstractValue{}, false
			}
			return base, true
		},
	}.Reductions(fact)
	if len(reductions) != 0 {
		t.Fatalf("nested target reductions = %#v, want none", reductions)
	}
}

func TestVariantOriginConditionReducerReducesMultipleSymbolsWithExclusions(t *testing.T) {
	leftSym := cfg.SymbolID(23)
	rightSym := cfg.SymbolID(24)
	leftFamily := uint64(103)
	rightFamily := uint64(104)
	leftPath := constraint.Path{Root: "left", Symbol: leftSym}
	rightPath := constraint.Path{Root: "right", Symbol: rightSym}
	values := map[cfg.SymbolID]product.AbstractValue{
		leftSym:  product.WithVariantOrigin(product.FromType(typ.String), leftFamily, []int{0, 1, 2}),
		rightSym: product.WithVariantOrigin(product.FromType(typ.Number), rightFamily, []int{0, 1, 2}),
	}
	fact := constraint.FromConstraints(
		constraint.VariantCaseEquals{Target: leftPath, OriginFamily: leftFamily, CaseIndex: 2},
		constraint.VariantCaseNotEquals{Target: rightPath, OriginFamily: rightFamily, CaseIndex: 1},
	)

	reductions := VariantOriginConditionReducer{
		SymbolValue: func(sym cfg.SymbolID) (product.AbstractValue, bool) {
			av, ok := values[sym]
			return av, ok
		},
	}.Reductions(fact)
	if len(reductions) != 2 {
		t.Fatalf("reductions = %#v, want two symbol reductions", reductions)
	}
	wantLeft := product.WithVariantOrigin(product.FromType(typ.String), leftFamily, []int{2})
	wantRight := product.WithVariantOrigin(product.FromType(typ.Number), rightFamily, []int{0, 2})
	if reductions[0].Symbol != leftSym || !product.Domain.Equal(reductions[0].Value, wantLeft) {
		t.Fatalf("left reduction = %#v, want %d -> %#v", reductions[0], leftSym, wantLeft)
	}
	if reductions[1].Symbol != rightSym || !product.Domain.Equal(reductions[1].Value, wantRight) {
		t.Fatalf("right reduction = %#v, want %d -> %#v", reductions[1], rightSym, wantRight)
	}
}

func TestVariantOriginConditionReducerKeepsUnconstrainedDNFBranch(t *testing.T) {
	sym := cfg.SymbolID(25)
	otherSym := cfg.SymbolID(26)
	family := uint64(105)
	path := constraint.Path{Root: "selected", Symbol: sym}
	other := constraint.Path{Root: "other", Symbol: otherSym}
	base := product.WithVariantOrigin(product.FromType(typ.String), family, []int{0, 1})
	fact := constraint.Or(
		constraint.FromConstraints(constraint.VariantCaseEquals{Target: path, OriginFamily: family, CaseIndex: 0}),
		constraint.FromConstraints(constraint.Truthy{Path: other}),
	)

	reductions := VariantOriginConditionReducer{
		SymbolValue: func(got cfg.SymbolID) (product.AbstractValue, bool) {
			if got != sym {
				return product.AbstractValue{}, false
			}
			return base, true
		},
	}.Reductions(fact)
	if len(reductions) != 0 {
		t.Fatalf("unconstrained DNF branch reductions = %#v, want none", reductions)
	}
}

func TestConditionDerivedFactsBatchesSymbolAndStaticMemberReductions(t *testing.T) {
	resultSym := cfg.SymbolID(27)
	sourceSym := cfg.SymbolID(28)
	family := uint64(106)
	result := constraint.Path{Root: "selected", Symbol: resultSym}
	source := constraint.Path{Root: "records", Symbol: sourceSym}
	payload := typ.NewRecord().Field("id", typ.String).Build()
	channel := typ.NewGeneric("Channel", []*typ.TypeParam{typ.NewTypeParam("T", nil)}, typ.NewInterface("Channel", nil))
	state := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(sourceSym): product.FromType(typ.Instantiate(channel, payload)),
		},
	}
	rootValue := product.WithVariantOrigin(product.FromType(typ.NewRecord().Build()), family, []int{0, 1})
	fact := constraint.FromConstraints(constraint.VariantCaseEquals{
		Target:       result,
		OriginFamily: family,
		CaseIndex:    1,
	})

	reductions := ConditionDerivedFacts{
		State: state,
		Fact:  fact,
		VariantCaseFieldProjections: []VariantCaseFieldProjection{{
			Target:       result,
			Field:        "value",
			Source:       source,
			SourceSteps:  []effect.TypeProjectionStep{effect.ProjectGenericArg(0)},
			OriginFamily: family,
			CaseIndex:    1,
		}},
		SymbolValue: func(sym cfg.SymbolID) (product.AbstractValue, bool) {
			if sym != resultSym {
				return product.AbstractValue{}, false
			}
			return rootValue, true
		},
	}.Reductions()

	if len(reductions.SymbolValues) != 1 {
		t.Fatalf("symbol reductions = %#v, want one root reduction", reductions.SymbolValues)
	}
	wantRoot := product.WithVariantOrigin(product.FromType(typ.NewRecord().Build()), family, []int{1})
	if reductions.SymbolValues[0].Symbol != resultSym || !product.Domain.Equal(reductions.SymbolValues[0].Value, wantRoot) {
		t.Fatalf("symbol reduction = %#v, want %d -> %#v", reductions.SymbolValues[0], resultSym, wantRoot)
	}
	if len(reductions.StaticMembers) != 1 {
		t.Fatalf("static member reductions = %#v, want one payload projection", reductions.StaticMembers)
	}
	if !reductions.StaticMembers[0].Path.Equal(result.Field("value")) ||
		!typ.TypeEquals(reductions.StaticMembers[0].Value.ProjectValue(), payload) {
		t.Fatalf("static member reduction = %#v, want selected.value payload", reductions.StaticMembers[0])
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
