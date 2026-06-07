package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCallablePathValueRefinesRootAsPresentCallable(t *testing.T) {
	sym := cfg.SymbolID(1)
	path := constraint.NewPath(sym, "")
	sig := typ.Func().Returns(typ.String).Build()
	state := PointState{
		FunctionRefs: WithFunctionRefPath(nil, path, FunctionRefSetOf(FunctionRef{GraphID: 7})),
	}

	got, ok := PointFactsOf(state).CallablePathValue(path, func(query CallableSignatureQuery) (typ.Type, bool) {
		if query.Ref.GraphID != 7 {
			t.Fatalf("resolver query ref = %#v, want graph 7", query.Ref)
		}
		return sig, true
	})

	if !ok || !typ.TypeEquals(got.ProjectValue(), sig) {
		t.Fatalf("callable root = %v/%v, want %v/true", got.ProjectValue(), ok, sig)
	}
}

func TestCallablePathValueRefinesMissingMemberAsOptionalCallable(t *testing.T) {
	path := constraint.NewPath(cfg.SymbolID(2), "obj").Field("run")
	sig := typ.Func().Returns(typ.Number).Build()
	state := PointState{
		FunctionRefs: WithFunctionRefPath(nil, path, FunctionRefSetOf(FunctionRef{GraphID: 11})),
	}

	got, ok := PointFactsOf(state).CallablePathValue(path, func(query CallableSignatureQuery) (typ.Type, bool) {
		if query.Ref.GraphID != 11 {
			t.Fatalf("resolver query ref = %#v, want graph 11", query.Ref)
		}
		return sig, true
	})

	want := typ.NewOptional(sig)
	if !ok || !typ.TypeEquals(got.ProjectValue(), want) {
		t.Fatalf("callable member = %v/%v, want %v/true", got.ProjectValue(), ok, want)
	}
}

func TestCallablePathReadWithoutSignatureKeepsRuntimeRead(t *testing.T) {
	path := constraint.NewPath(cfg.SymbolID(3), "rows").IndexInt(1)
	row := typ.NewRecord().Field("text", typ.String).Build()
	read := product.FromType(row)

	got, ok := PointFactsOf(PointState{}).CallablePathRead(path, read, nil)

	if !ok || !typ.TypeEquals(got.ProjectValue(), row) {
		t.Fatalf("non-callable read = %v/%v, want %v/true", got.ProjectValue(), ok, row)
	}
}

func TestReadStaticMemberValueOverlaysCallableSignature(t *testing.T) {
	path := constraint.NewPath(cfg.SymbolID(4), "service").Field("run")
	sig := typ.Func().Returns(typ.Boolean).Build()
	state := PointState{
		StaticMembers: StaticMemberFactsDomain.Top().
			WithAddress(testStableAddressKey(t, SymbolPathKey(path.Symbol, path.Segments)), product.FromType(typ.Any)),
		FunctionRefs: WithFunctionRefPath(nil, path, FunctionRefSetOf(FunctionRef{GraphID: 13})),
	}

	got := PointFactsOf(state).ReadStaticMemberValue(path, PointReadPolicy{
		CallableSignature: func(query CallableSignatureQuery) (typ.Type, bool) {
			if query.Ref.GraphID != 13 {
				t.Fatalf("resolver query ref = %#v, want graph 13", query.Ref)
			}
			return sig, true
		},
	})

	if got.State != StateResolved || !typ.TypeEquals(got.Value.ProjectValue(), sig) {
		t.Fatalf("static callable read = %v/%v, want %v/resolved", got.Value.ProjectValue(), got.State, sig)
	}
}

func TestReadKnownCallablePathRequiresCallableEvidence(t *testing.T) {
	path := constraint.NewPath(cfg.SymbolID(5), "service").Field("run")
	facts := PointFactsOf(PointState{})

	missing := facts.ReadKnownCallablePath(path, product.AbstractValue{}, PointReadPolicy{})
	if missing.State != StateUnknown {
		t.Fatalf("known callable without evidence = %#v, want unknown", missing)
	}

	sig := typ.Func().Returns(typ.Number).Build()
	state := PointState{
		FunctionRefs: WithFunctionRefPath(nil, path, FunctionRefSetOf(FunctionRef{GraphID: 21})),
	}
	got := PointFactsOf(state).ReadKnownCallablePath(path, product.AbstractValue{}, PointReadPolicy{
		CallableSignature: func(query CallableSignatureQuery) (typ.Type, bool) {
			if query.Ref.GraphID != 21 {
				t.Fatalf("resolver query ref = %#v, want graph 21", query.Ref)
			}
			return sig, true
		},
	})

	want := typ.NewOptional(sig)
	if got.State != StateResolved || !typ.TypeEquals(got.Value.ProjectValue(), want) {
		t.Fatalf("known callable read = %v/%v, want %v/resolved", got.Value.ProjectValue(), got.State, want)
	}
}
