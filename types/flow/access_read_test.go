package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestReadAccessStaticMemberPrefersPointFact(t *testing.T) {
	path := constraint.NewPath(cfg.SymbolID(31), "node").Field("id")
	base := product.FromType(typ.NewRecord().
		Field("id", typ.String).
		Build())
	state := PointState{
		StaticMembers: StaticMemberFactsDomain.Top().
			WithAddress(testStableAddressKey(t, SymbolPathKey(path.Symbol, path.Segments)), product.FromType(typ.Number)),
	}

	got := PointFactsOf(state).ReadAccess(AccessReadQuery{
		Kind:    AccessReadStaticMember,
		Path:    path,
		HasPath: true,
		Base:    base,
		Member:  value.MemberField("id"),
	})

	if got.State != StateResolved || !typ.TypeEquals(got.Value.ProjectValue(), typ.Number) {
		t.Fatalf("ReadAccess static member = %v/%v, want number/resolved", got.Value.ProjectValue(), got.State)
	}
}

func TestReadAccessStaticMemberUsesCallableOverlayForMissingRuntimeSlot(t *testing.T) {
	path := constraint.NewPath(cfg.SymbolID(32), "service").Field("run")
	sig := typ.Func().Returns(typ.String).Build()
	state := PointState{
		FunctionRefs: WithFunctionRefPath(nil, path, FunctionRefSetOf(FunctionRef{GraphID: 33})),
	}
	base := product.FromType(typ.NewRecord().Build())

	got := PointFactsOf(state).ReadAccess(AccessReadQuery{
		Kind:    AccessReadStaticMember,
		Path:    path,
		HasPath: true,
		Base:    base,
		Member:  value.MemberField("run"),
		Policy: PointReadPolicy{
			CallableSignature: func(query CallableSignatureQuery) (typ.Type, bool) {
				if query.Ref.GraphID != 33 {
					t.Fatalf("resolver query ref = %#v, want graph 33", query.Ref)
				}
				return sig, true
			},
		},
	})

	want := typ.NewOptional(sig)
	if got.State != StateResolved || !typ.TypeEquals(got.Value.ProjectValue(), want) {
		t.Fatalf("ReadAccess callable member = %v/%v, want %v/resolved", got.Value.ProjectValue(), got.State, want)
	}
}

func TestReadAccessDynamicIndexUsesRuntimeProductRead(t *testing.T) {
	base := product.FromType(typ.NewMap(typ.String, typ.Number))
	key := product.FromType(typ.LiteralString("count"))

	got := PointFactsOf(PointState{}).ReadAccess(AccessReadQuery{
		Kind: AccessReadDynamicIndex,
		Base: base,
		Key:  key,
	})

	want := typ.NewOptional(typ.Number)
	if got.State != StateResolved || !typ.TypeEquals(got.Value.ProjectValue(), want) {
		t.Fatalf("ReadAccess dynamic index = %v/%v, want %v/resolved", got.Value.ProjectValue(), got.State, want)
	}
}
