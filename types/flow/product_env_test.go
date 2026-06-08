package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

func TestSymbolProductEnvReadsBaseAndPointFacts(t *testing.T) {
	const sym = cfg.SymbolID(41)
	base := product.FromType(typ.NewRecord().
		Field("id", typ.String).
		Build())
	extraPath := constraint.NewPath(sym, "node").Field("extra")
	state := PointState{
		StaticMembers: StaticMemberFactsDomain.Top().WithAddress(
			testStableAddressPath(t, extraPath),
			product.FromType(typ.Number),
		),
	}
	env, rootKey := SymbolProductEnv(sym, base, PointFactsOf(state), &core.FuncResolver{FieldFunc: core.Field})

	if got := env.LookupPathType(rootKey); !typ.TypeEquals(got, base.ProjectValue()) {
		t.Fatalf("root type = %s, want %s", got, base.ProjectValue())
	}
	if got := env.LookupPathType(StablePathKey(constraint.NewPath(sym, "node").Field("id"))); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("base member type = %s, want string", got)
	}
	if got := env.LookupPathType(StablePathKey(extraPath)); !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("point fact type = %s, want number", got)
	}
}

func TestProductWithMemberPathUsesStructuralSegmentsAtBoundary(t *testing.T) {
	base := product.FromType(typ.NewRecord().Build())
	path := []constraint.Segment{
		{Kind: constraint.SegmentField, Name: "headers"},
		{Kind: constraint.SegmentIndexString, Name: "Accept-Language"},
	}

	got := ProductWithMemberPath(base, path, product.FromType(typ.String))
	rec, ok := got.ProjectValue().(*typ.Record)
	if !ok {
		t.Fatalf("ProductWithMemberPath result = %T, want record (%v)", got.ProjectValue(), got.ProjectValue())
	}
	headers := rec.GetField("headers")
	if headers == nil {
		t.Fatalf("ProductWithMemberPath lost outer field: %v", rec)
	}
	inner, ok := headers.Type.(*typ.Record)
	if !ok {
		t.Fatalf("headers field = %T, want record with exact static member (%v)", headers.Type, headers.Type)
	}
	member := inner.GetStaticStringIndex("Accept-Language")
	if member == nil || !typ.TypeEquals(member.Type, typ.String) {
		t.Fatalf("inner static member = %#v, want [\"Accept-Language\"]: string", member)
	}
	if !inner.HasMapComponent() ||
		!typ.TypeEquals(inner.MapKey, typ.LiteralString("Accept-Language")) ||
		!typ.TypeEquals(inner.MapValue, typ.String) {
		t.Fatalf("inner map component = [%v]: %v, want [\"Accept-Language\"]: string", inner.MapKey, inner.MapValue)
	}
}

func TestProductWithMemberPathUsesNumericIndexAsIndexedWrite(t *testing.T) {
	baseType := typ.NewRecord().Field("stable", typ.Number).Build()
	base := product.FromType(baseType)

	got := ProductWithMemberPath(base, []constraint.Segment{{Kind: constraint.SegmentIndexInt, Index: 1}}, product.FromType(typ.String))
	rec, ok := got.ProjectValue().(*typ.Record)
	if !ok {
		t.Fatalf("numeric-index write = %T, want record (%v)", got.ProjectValue(), got.ProjectValue())
	}
	stable := rec.GetField("stable")
	if stable == nil || !typ.TypeEquals(stable.Type, typ.Number) {
		t.Fatalf("stable field changed: %v", rec)
	}
	if !rec.HasMapComponent() || !typ.TypeEquals(rec.MapKey, typ.LiteralInt(1)) || !typ.TypeEquals(rec.MapValue, typ.String) {
		t.Fatalf("numeric-index write map component = [%v]: %v, want [1]: string", rec.MapKey, rec.MapValue)
	}
}

func TestProductWithOnlyMemberPathBuildsMinimalNestedProduct(t *testing.T) {
	got := ProductWithOnlyMemberPath([]constraint.Segment{
		{Kind: constraint.SegmentField, Name: "config"},
		{Kind: constraint.SegmentField, Name: "used"},
	}, product.FromType(typ.Number))

	config, ok := ProductMemberPathValue(got, []constraint.Segment{{Kind: constraint.SegmentField, Name: "config"}})
	if !ok || config.IsZero() {
		t.Fatalf("config missing from minimal product: %v", got.ProjectValue())
	}
	used, ok := ProductMemberPathValue(got, []constraint.Segment{
		{Kind: constraint.SegmentField, Name: "config"},
		{Kind: constraint.SegmentField, Name: "used"},
	})
	if !ok || !typ.TypeEquals(used.ProjectValue(), typ.Number) {
		t.Fatalf("config.used = %v/%v, want number", used.ProjectValue(), ok)
	}
	if _, ok := ProductMemberPathValue(got, []constraint.Segment{{Kind: constraint.SegmentField, Name: "stable"}}); ok {
		t.Fatalf("minimal product retained sibling: %v", got.ProjectValue())
	}
}

func TestProductDomainHasNarrowingForSymbolUsesStableAddress(t *testing.T) {
	const sym = cfg.SymbolID(51)
	path := constraint.NewPath(sym, "node")
	env, _ := SymbolProductEnv(sym, product.FromType(typ.NewOptional(typ.String)), PointFactsOf(PointState{}), &core.FuncResolver{FieldFunc: core.Field})
	dom := NewProductDomain(env)
	if !dom.ApplyCondition(constraint.FromConstraints(constraint.Truthy{Path: path})) {
		t.Fatal("ApplyCondition returned false")
	}

	if !ProductDomainHasNarrowingForSymbol(dom, sym) {
		t.Fatalf("expected narrowing for symbol %d", sym)
	}
	if ProductDomainHasNarrowingForSymbol(dom, cfg.SymbolID(52)) {
		t.Fatalf("unexpected narrowing for unrelated symbol")
	}
}

func TestProductConditionReductionValueAddsPositiveFieldPresence(t *testing.T) {
	const sym = cfg.SymbolID(61)
	path := constraint.NewPath(sym, "tool_specs")
	spec := typ.NewRecord().
		Field("id", typ.String).
		OptField("alias", typ.String).
		Build()
	toolSpec := typ.NewAlias("ToolSpec", typ.NewUnion(typ.String, spec))
	toolSpecArray := typ.NewArray(toolSpec)
	base := product.FromType(typ.NewUnion(toolSpec, toolSpecArray))
	fact := constraint.FromConstraints(
		constraint.HasType{Path: path, Type: narrow.BuiltinTypeKey("table")},
		constraint.NotHasType{Path: path, Type: narrow.BuiltinTypeKey("string")},
		constraint.Truthy{Path: path.Field("id")},
	)

	got, ok := ProductConditionReductionValue(ProductConditionReduction{
		Symbol:   sym,
		Base:     base,
		HasBase:  true,
		Fact:     fact,
		Facts:    PointFactsOf(PointState{}),
		Resolver: &core.FuncResolver{FieldFunc: core.Field},
	})
	if !ok || got.IsZero() {
		t.Fatalf("ProductConditionReductionValue = %v/%v, want narrowed record", got.ProjectValue(), ok)
	}
	projected := got.ProjectValue()
	if !subtype.IsSubtype(spec, projected) {
		t.Fatalf("reduced type = %v, want ToolSpec record branch retained", projected)
	}
	if subtype.IsSubtype(toolSpecArray, projected) {
		t.Fatalf("reduced type = %v, array branch should be excluded by inferred field presence", projected)
	}
}

func TestConditionValueSymbolsReturnsStableRootSet(t *testing.T) {
	first := constraint.NewPath(cfg.SymbolID(71), "first")
	second := constraint.NewPath(cfg.SymbolID(72), "second")
	got := ConditionValueSymbols(constraint.FromConstraints(
		constraint.Truthy{Path: second.Field("id")},
		constraint.FieldEqualsPath{Target: first, Field: "owner", Value: second},
	))
	want := []cfg.SymbolID{71, 72}
	if len(got) != len(want) {
		t.Fatalf("ConditionValueSymbols = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ConditionValueSymbols = %#v, want %#v", got, want)
		}
	}
}

func TestProductConditionReducerSkipsVariantOriginSymbols(t *testing.T) {
	const sym = cfg.SymbolID(81)
	family := uint64(108)
	path := constraint.NewPath(sym, "selected")
	base := product.WithVariantOrigin(product.FromType(typ.NewOptional(typ.String)), family, []int{0, 1})
	fact := constraint.FromConstraints(
		constraint.VariantCaseEquals{Target: path, OriginFamily: family, CaseIndex: 0},
		constraint.NotNil{Path: path},
	)

	got := ProductConditionReducer{
		Fact:     fact,
		Facts:    PointFactsOf(PointState{}),
		Resolver: &core.FuncResolver{FieldFunc: core.Field},
		Base: func(got cfg.SymbolID) ProductConditionBase {
			if got != sym {
				return ProductConditionBase{Skip: true}
			}
			return ProductConditionBase{Current: base, HasCurrent: true, Base: base, HasBase: true}
		},
	}.Reductions()
	if len(got) != 0 {
		t.Fatalf("ProductConditionReducer reductions = %#v, want variant-origin symbol skipped", got)
	}
}

func TestProductConditionReducerUsesCallerBasePolicy(t *testing.T) {
	const sym = cfg.SymbolID(82)
	path := constraint.NewPath(sym, "maybe")
	current := product.FromType(typ.String)
	base := product.FromType(typ.NewOptional(typ.String))
	fact := constraint.FromConstraints(constraint.NotNil{Path: path})

	got := ProductConditionReducer{
		Fact:     fact,
		Facts:    PointFactsOf(PointState{}),
		Resolver: &core.FuncResolver{FieldFunc: core.Field},
		Base: func(got cfg.SymbolID) ProductConditionBase {
			if got != sym {
				return ProductConditionBase{Skip: true}
			}
			return ProductConditionBase{Current: current, HasCurrent: true, Base: base, HasBase: true}
		},
	}.Reductions()
	if len(got) != 0 {
		t.Fatalf("ProductConditionReducer reductions = %#v, want no non-semantic current narrowing", got)
	}

	got = ProductConditionReducer{
		Fact:     fact,
		Facts:    PointFactsOf(PointState{}),
		Resolver: &core.FuncResolver{FieldFunc: core.Field},
		Base: func(got cfg.SymbolID) ProductConditionBase {
			if got != sym {
				return ProductConditionBase{Skip: true}
			}
			return ProductConditionBase{Base: base, HasBase: true}
		},
	}.Reductions()
	if len(got) != 1 {
		t.Fatalf("ProductConditionReducer reductions = %#v, want one base narrowing", got)
	}
	if got[0].Symbol != sym || !typ.TypeEquals(got[0].Value.ProjectValue(), typ.String) {
		t.Fatalf("ProductConditionReducer reduction = %#v, want string", got[0])
	}
}
