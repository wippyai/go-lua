package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/compiler/check/domain/functionsymbols"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

func TestConditionEffectVariantOriginDNFJoinsAllowedCases(t *testing.T) {
	sym := cfg.SymbolID(42)
	family := uint64(777)
	path := constraint.Path{Root: "result", Symbol: sym}
	fact := constraint.Or(
		constraint.FromConstraints(constraint.VariantCaseEquals{
			Target:       path,
			OriginFamily: family,
			CaseIndex:    0,
		}),
		constraint.FromConstraints(constraint.VariantCaseEquals{
			Target:       path,
			OriginFamily: family,
			CaseIndex:    1,
		}),
	)
	base := product.FromType(typ.String)
	state := flow.PointState{
		Cond: constraint.TrueCondition(),
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym): product.WithVariantOrigin(base, family, []int{0, 1, 2}),
		},
	}

	tr := &Transfer{}
	if !tr.applyConditionEffect(&state, ConditionEffect{Fact: fact}) {
		t.Fatalf("condition effect reported no change")
	}
	got, ok := tr.symbolValue(&state, sym)
	if !ok {
		t.Fatalf("condition effect removed symbol value")
	}
	want := product.WithVariantOrigin(base, family, []int{0, 1})
	if !product.Equal(got, want) {
		t.Fatalf("DNF origin reduction = %#v, want %#v", got, want)
	}
}

func TestConditionEffectVariantOriginProjectsCaseType(t *testing.T) {
	sym := cfg.SymbolID(43)
	sourceSym := cfg.SymbolID(430)
	family := uint64(778)
	path := constraint.Path{Root: "result", Symbol: sym}
	source := constraint.Path{Root: "numbers", Symbol: sourceSym}
	first := typ.NewRecord().Field("channel", typ.String).Field("value", typ.String).Build()
	second := typ.NewRecord().Field("channel", typ.Number).Field("value", typ.Number).Build()
	channel := typ.NewGeneric("Channel", []*typ.TypeParam{typ.NewTypeParam("T", nil)}, typ.NewInterface("Channel", nil))
	in := input.Inputs{
		VariantFieldOrigins: []flow.VariantFieldOrigin{{
			Target:       path,
			OriginFamily: family,
			CaseIndex:    1,
		}},
		VariantCaseFieldProjections: []flow.VariantCaseFieldProjection{{
			Target:       path,
			Field:        "value",
			Source:       source,
			SourceSteps:  []effect.TypeProjectionStep{effect.ProjectGenericArg(0)},
			OriginFamily: family,
			CaseIndex:    1,
		}},
	}
	state := flow.PointState{
		Cond: constraint.TrueCondition(),
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym):       product.WithVariantOrigin(product.FromType(typ.NewUnion(first, second)), family, []int{0, 1}),
			flow.SymbolValueKey(sourceSym): product.FromType(typ.Instantiate(channel, typ.Number)),
		},
	}
	tr := &Transfer{in: in}

	if !tr.applyConditionEffect(&state, ConditionEffect{Fact: constraint.FromConstraints(
		constraint.VariantCaseEquals{Target: path, OriginFamily: family, CaseIndex: 1},
	)}) {
		t.Fatal("condition effect reported no change")
	}
	got, ok := tr.symbolValue(&state, sym)
	wantRoot := product.WithVariantOrigin(product.FromType(typ.NewUnion(first, second)), family, []int{1})
	if !ok || !product.Equal(got, wantRoot) {
		t.Fatalf("origin-narrowed root = %#v/%v, want %#v", got, ok, wantRoot)
	}
	valueFact, ok := flow.PointFactsOf(state).PathValue(path.Field("value"))
	if !ok || !typ.TypeEquals(valueFact.ProjectValue(), typ.Number) {
		t.Fatalf("projected value field = %v/%v, want number", valueFact.ProjectValue(), ok)
	}
}

func TestConditionEffectVariantOriginFieldProjectionPreservesChildren(t *testing.T) {
	sym := cfg.SymbolID(45)
	sourceSym := cfg.SymbolID(450)
	family := uint64(780)
	path := constraint.Path{Root: "result", Symbol: sym}
	source := constraint.Path{Root: "entries", Symbol: sourceSym}
	entryType := typ.NewRecord().Field("id", typ.String).Build()
	channel := typ.NewGeneric("Channel", []*typ.TypeParam{typ.NewTypeParam("T", nil)}, typ.NewInterface("Channel", nil))
	state := flow.PointState{
		Cond: constraint.TrueCondition(),
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym):       product.WithVariantOrigin(product.FromType(typ.NewRecord().Build()), family, []int{0}),
			flow.SymbolValueKey(sourceSym): product.FromType(typ.Instantiate(channel, entryType)),
		},
	}
	flow.SetStaticMemberPath(&state, path.Field("value").Field("id"), product.FromType(typ.String))
	tr := &Transfer{in: input.Inputs{VariantCaseFieldProjections: []flow.VariantCaseFieldProjection{{
		Target:       path,
		Field:        "value",
		Source:       source,
		SourceSteps:  []effect.TypeProjectionStep{effect.ProjectGenericArg(0)},
		OriginFamily: family,
		CaseIndex:    0,
	}}}}

	if !tr.applyConditionEffect(&state, ConditionEffect{Fact: constraint.FromConstraints(
		constraint.VariantCaseEquals{Target: path, OriginFamily: family, CaseIndex: 0},
	)}) {
		t.Fatal("condition effect reported no change")
	}
	idFact, ok := flow.PointFactsOf(state).PathValue(path.Field("value").Field("id"))
	if !ok || !typ.TypeEquals(idFact.ProjectValue(), typ.String) {
		t.Fatalf("projected child fact = %v/%v, want string", idFact.ProjectValue(), ok)
	}
}

func TestConditionEffectVariantOriginFieldProjectionDNFJoinsPayloads(t *testing.T) {
	sym := cfg.SymbolID(46)
	strSourceSym := cfg.SymbolID(461)
	numSourceSym := cfg.SymbolID(462)
	family := uint64(781)
	path := constraint.Path{Root: "result", Symbol: sym}
	strSource := constraint.Path{Root: "strings", Symbol: strSourceSym}
	numSource := constraint.Path{Root: "numbers", Symbol: numSourceSym}
	channel := typ.NewGeneric("Channel", []*typ.TypeParam{typ.NewTypeParam("T", nil)}, typ.NewInterface("Channel", nil))
	state := flow.PointState{
		Cond: constraint.TrueCondition(),
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym):          product.WithVariantOrigin(product.FromType(typ.NewRecord().Build()), family, []int{0, 1}),
			flow.SymbolValueKey(strSourceSym): product.FromType(typ.Instantiate(channel, typ.String)),
			flow.SymbolValueKey(numSourceSym): product.FromType(typ.Instantiate(channel, typ.Number)),
		},
	}
	tr := &Transfer{in: input.Inputs{VariantCaseFieldProjections: []flow.VariantCaseFieldProjection{
		{
			Target:       path,
			Field:        "value",
			Source:       strSource,
			SourceSteps:  []effect.TypeProjectionStep{effect.ProjectGenericArg(0)},
			OriginFamily: family,
			CaseIndex:    0,
		},
		{
			Target:       path,
			Field:        "value",
			Source:       numSource,
			SourceSteps:  []effect.TypeProjectionStep{effect.ProjectGenericArg(0)},
			OriginFamily: family,
			CaseIndex:    1,
		},
	}}}

	fact := constraint.Or(
		constraint.FromConstraints(constraint.VariantCaseEquals{Target: path, OriginFamily: family, CaseIndex: 0}),
		constraint.FromConstraints(constraint.VariantCaseEquals{Target: path, OriginFamily: family, CaseIndex: 1}),
	)
	if !tr.applyConditionEffect(&state, ConditionEffect{Fact: fact}) {
		t.Fatal("condition effect reported no change")
	}
	valueFact, ok := flow.PointFactsOf(state).PathValue(path.Field("value"))
	want := typ.NewUnion(typ.String, typ.Number)
	if !ok || !typ.TypeEquals(valueFact.ProjectValue(), want) {
		t.Fatalf("DNF projected value field = %v/%v, want %v", valueFact.ProjectValue(), ok, want)
	}
}

func TestConditionEffectVariantOriginKeepsProjectionOutWithoutOriginAuthority(t *testing.T) {
	sym := cfg.SymbolID(44)
	family := uint64(779)
	path := constraint.Path{Root: "result", Symbol: sym}
	first := typ.NewRecord().Field("channel", typ.String).Field("value", typ.String).Build()
	second := typ.NewRecord().Field("channel", typ.Number).Field("value", typ.Number).Build()
	base := product.WithVariantOrigin(product.FromType(typ.NewUnion(first, second)), family, []int{0, 1})
	state := flow.PointState{
		Cond: constraint.TrueCondition(),
		Env:  map[flow.ValueKey]product.AbstractValue{flow.SymbolValueKey(sym): base},
	}
	tr := &Transfer{}

	if !tr.applyConditionEffect(&state, ConditionEffect{Fact: constraint.FromConstraints(
		constraint.VariantCaseEquals{Target: path, OriginFamily: family, CaseIndex: 1},
	)}) {
		t.Fatal("condition effect reported no change")
	}
	got, ok := tr.symbolValue(&state, sym)
	want := product.WithVariantOrigin(product.FromType(typ.NewUnion(first, second)), family, []int{1})
	if !ok || !product.Equal(got, want) {
		t.Fatalf("origin-only value = %#v/%v, want %#v", got, ok, want)
	}
}

func TestConditionEffectReducesRootHasTypeIntoProductValue(t *testing.T) {
	sym := cfg.SymbolID(51)
	path := constraint.Path{Root: "value", Symbol: sym}
	state := flow.PointState{
		Cond: constraint.TrueCondition(),
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym): product.FromType(typ.Any),
		},
	}
	tr := &Transfer{}

	if !tr.applyConditionEffect(&state, ConditionEffect{Fact: constraint.FromConstraints(
		constraint.HasType{Path: path, Type: narrow.BuiltinTypeKey("string")},
	)}) {
		t.Fatal("condition effect reported no change")
	}
	got, ok := tr.symbolValue(&state, sym)
	if !ok || !typ.TypeEquals(got.ProjectValue(), typ.String) {
		t.Fatalf("condition-reduced value = %v/%v, want string", got.ProjectValue(), ok)
	}
}

func TestConditionEffectKeepsPendingUnannotatedParamProofInConditionOnly(t *testing.T) {
	sym := cfg.SymbolID(55)
	path := constraint.Path{Root: "name", Symbol: sym}
	fact := constraint.FromConstraints(constraint.Truthy{Path: path})
	state := flow.PointState{Cond: constraint.TrueCondition()}
	tr := &Transfer{unannotatedParam: transferTestSymbolSet(sym)}

	if !tr.applyConditionEffect(&state, ConditionEffect{Fact: fact}) {
		t.Fatal("condition effect reported no change")
	}
	if !constraint.Domain.Equal(state.Cond, fact) {
		t.Fatalf("condition = %v, want %v", state.Cond, fact)
	}
	if got, ok := tr.symbolValue(&state, sym); ok {
		t.Fatalf("pending unannotated parameter materialized value %v; proof should stay in Cond", got.ProjectValue())
	}
}

func transferTestSymbolSet(syms ...cfg.SymbolID) functionsymbols.Set {
	var set functionsymbols.Set
	for _, sym := range syms {
		set.Add(sym)
	}
	return set
}

func TestConditionEffectReducesCompoundFieldProofOverDeclaredUnion(t *testing.T) {
	sym := cfg.SymbolID(52)
	entryRecord := typ.NewRecord().Field("id", typ.String).Build()
	entry := typ.NewAlias("Entry", typ.NewUnion(typ.String, entryRecord))
	entryArray := typ.NewArray(entry)
	declared := typ.NewUnion(entry, entryArray)
	path := constraint.Path{Root: "entry", Symbol: sym}
	fact := constraint.Or(
		constraint.FromConstraints(constraint.HasType{
			Path: path,
			Type: narrow.BuiltinTypeKey("string"),
		}),
		constraint.FromConstraints(
			constraint.HasType{Path: path, Type: narrow.BuiltinTypeKey("table")},
			constraint.Truthy{Path: path.Field("id")},
		),
	)
	state := flow.PointState{
		Cond: constraint.TrueCondition(),
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym): product.FromType(declared),
		},
	}
	tr := &Transfer{declaredTypes: map[cfg.SymbolID]typ.Type{sym: declared}}

	if !tr.applyConditionEffect(&state, ConditionEffect{Fact: fact}) {
		t.Fatal("condition effect reported no change")
	}
	got, ok := tr.symbolValue(&state, sym)
	if !ok {
		t.Fatal("condition-reduced value missing")
	}
	gotType := got.ProjectValue()
	if !subtype.IsSubtype(typ.String, gotType) {
		t.Fatalf("condition-reduced type = %v, want string branch", gotType)
	}
	if !subtype.IsSubtype(entryRecord, gotType) {
		t.Fatalf("condition-reduced type = %v, want record branch", gotType)
	}
	if subtype.IsSubtype(entryArray, gotType) {
		t.Fatalf("condition-reduced type = %v, array branch should be excluded", gotType)
	}
}

func TestConditionEffectDoesNotWidenCurrentValueFromDeclaredBase(t *testing.T) {
	sym := cfg.SymbolID(53)
	entryRecord := typ.NewRecord().Field("id", typ.String).Build()
	entry := typ.NewAlias("Entry", typ.NewUnion(typ.String, entryRecord))
	entryArray := typ.NewArray(entry)
	declared := typ.NewUnion(entry, entryArray)
	current := typ.NewUnion(entryArray, entryRecord)
	path := constraint.Path{Root: "entry", Symbol: sym}
	state := flow.PointState{
		Cond: constraint.TrueCondition(),
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym): product.FromType(current),
		},
	}
	tr := &Transfer{declaredTypes: map[cfg.SymbolID]typ.Type{sym: declared}}

	if !tr.applyConditionEffect(&state, ConditionEffect{Fact: constraint.FromConstraints(
		constraint.Truthy{Path: path.Field("id")},
	)}) {
		t.Fatal("condition effect reported no change")
	}
	got, ok := tr.symbolValue(&state, sym)
	if !ok {
		t.Fatal("condition-reduced value missing")
	}
	gotType := got.ProjectValue()
	if subtype.IsSubtype(typ.String, gotType) {
		t.Fatalf("condition effect widened current edge value to %v; string branch was already excluded", gotType)
	}
}

func TestConditionEffectAdmitsSemanticReductionFromDynamicCurrent(t *testing.T) {
	sym := cfg.SymbolID(54)
	path := constraint.Path{Root: "x", Symbol: sym}
	tr := &Transfer{}

	for name, base := range map[string]product.AbstractValue{
		"strict-any":  product.FromType(typ.Any),
		"gradual-any": product.GradualAny(),
	} {
		t.Run(name, func(t *testing.T) {
			state := flow.PointState{
				Cond: constraint.TrueCondition(),
				Env: map[flow.ValueKey]product.AbstractValue{
					flow.SymbolValueKey(sym): base,
				},
			}
			if !tr.applyConditionEffect(&state, ConditionEffect{Fact: constraint.FromConstraints(
				constraint.HasType{Path: path, Type: narrow.BuiltinTypeKey("number")},
			)}) {
				t.Fatal("condition effect reported no change")
			}
			got, ok := tr.symbolValue(&state, sym)
			if !ok || !typ.TypeEquals(got.ProjectValue(), typ.Number) {
				t.Fatalf("condition-reduced value = %v/%v, want number,true", got.ProjectValue(), ok)
			}
			if base.IsGradualTop() && !got.IsGradualTop() {
				t.Fatal("condition reduction over gradual source lost gradual evidence")
			}
		})
	}
}
