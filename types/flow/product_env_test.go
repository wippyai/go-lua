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
