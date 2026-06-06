package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/query/core"
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

	if got := env.PathTypeAt(rootKey); !typ.TypeEquals(got, base.ProjectValue()) {
		t.Fatalf("root type = %s, want %s", got, base.ProjectValue())
	}
	if got := env.PathTypeAt(StablePathKey(constraint.NewPath(sym, "node").Field("id"))); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("base member type = %s, want string", got)
	}
	if got := env.PathTypeAt(StablePathKey(extraPath)); !typ.TypeEquals(got, typ.Number) {
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
