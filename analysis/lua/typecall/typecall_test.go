package typecall

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/annotation"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestCallableDirectFunction(t *testing.T) {
	fn := typ.Func().
		Param("input", typ.String).
		Returns(typ.Number).
		Build()

	got, ok := Callable(fn)
	if !ok {
		t.Fatal("Callable(function) failed")
	}
	assertType(t, got, fn)

	got, ok = Callable(typ.NewOptional(fn))
	if !ok {
		t.Fatal("Callable(optional function) failed")
	}
	assertType(t, got, fn)
}

func TestCallableRecordMetatableCall(t *testing.T) {
	fn := typ.Func().Returns(typ.String).Build()

	rec := recordWithMetamethod("__call", fn)
	got, ok := Callable(rec)
	if !ok {
		t.Fatal("Callable(record with __call) failed")
	}
	assertType(t, got, fn)

	noCall := recordWithMetamethod("__index", typ.String)
	if _, ok := Callable(noCall); ok {
		t.Fatal("Callable(record without __call) succeeded")
	}

	unconstrained := typetable.NewRecord().
		Metatable(typetable.MetatableUnconstrained).
		Build()
	if _, ok := Callable(unconstrained); ok {
		t.Fatal("Callable(record with unconstrained metatable) succeeded")
	}
	if HasMetamethod(unconstrained, "__call") {
		t.Fatal("HasMetamethod(record with unconstrained metatable, __call) = true")
	}
}

func TestGetMetamethodDirectCallAndIndex(t *testing.T) {
	call := typ.Func().Returns(typ.Boolean).Build()
	index := typetable.NewRecord().
		Field("method", typ.Func().Returns(typ.String).Build()).
		Build()
	mt := typetable.NewRecord().
		Field("__call", call).
		Field("__index", index).
		Build()
	rec := typetable.NewRecord().Metatable(mt).Build()

	got, ok := GetMetamethod(rec, "__call")
	if !ok {
		t.Fatal("GetMetamethod(record, __call) failed")
	}
	assertType(t, got, call)

	got, ok = GetMetamethod(rec, "__index")
	if !ok {
		t.Fatal("GetMetamethod(record, __index) failed")
	}
	assertType(t, got, index)
}

func TestGetMetamethodWrappers(t *testing.T) {
	fn := typ.Func().Returns(typ.String).Build()
	rec := recordWithMetamethod("__call", fn)

	t.Run("alias", func(t *testing.T) {
		got, ok := GetMetamethod(typ.NewAlias("CallableRecord", rec), "__call")
		if !ok {
			t.Fatal("GetMetamethod(alias record, __call) failed")
		}
		assertType(t, got, fn)
	})

	t.Run("optional", func(t *testing.T) {
		got, ok := GetMetamethod(typ.NewOptional(rec), "__call")
		if !ok {
			t.Fatal("GetMetamethod(optional record, __call) failed")
		}
		assertType(t, got, fn)
	})

	t.Run("annotated", func(t *testing.T) {
		wrapped := typ.NewAnnotated(rec, []annotation.Annotation{{Name: "tag"}})
		got, ok := GetMetamethod(wrapped, "__call")
		if !ok {
			t.Fatal("GetMetamethod(annotated record, __call) failed")
		}
		assertType(t, got, fn)
	})

	t.Run("instantiated", func(t *testing.T) {
		param := typ.NewTypeParam("T", nil)
		body := typetable.NewRecord().
			Metatable(typetable.NewRecord().
				Field("__call", typ.Func().Returns(param).Build()).
				Build()).
			Build()
		box := typ.NewGeneric("CallableBox", []*typ.TypeParam{param}, body)

		got, ok := GetMetamethod(typ.Instantiate(box, typ.Number), "__call")
		if !ok {
			t.Fatal("GetMetamethod(CallableBox<number>, __call) failed")
		}
		assertType(t, got, typ.Func().Returns(typ.Number).Build())
	})
}

func TestGetMetamethodDoesNotFollowIndexChain(t *testing.T) {
	call := typ.Func().Returns(typ.Number).Build()
	methods := typetable.NewRecord().Field("__call", call).Build()
	rec := recordWithMetamethod("__index", methods)

	got, ok := GetMetamethod(rec, "__index")
	if !ok {
		t.Fatal("GetMetamethod(record, __index) failed")
	}
	assertType(t, got, methods)

	if _, ok := GetMetamethod(rec, "__call"); ok {
		t.Fatal("GetMetamethod followed __index chain for __call")
	}
	if _, ok := Callable(rec); ok {
		t.Fatal("Callable followed __index chain for __call")
	}
}

func TestCallableUnionAndIntersection(t *testing.T) {
	fn := typ.Func().Returns(typ.String).Build()
	rec := recordWithMetamethod("__call", fn)

	got, ok := Callable(typ.NewUnion(fn, rec))
	if !ok {
		t.Fatal("Callable(union of callable members) failed")
	}
	assertType(t, got, fn)

	if _, ok := Callable(typ.NewUnion(fn, typ.String)); ok {
		t.Fatal("Callable(union with non-callable member) succeeded")
	}

	got, ok = Callable(typ.NewIntersection(typ.String, fn))
	if !ok {
		t.Fatal("Callable(intersection with callable member) failed")
	}
	assertType(t, got, fn)
}

func TestCallableUnionRequiresStableRepresentative(t *testing.T) {
	stringFn := typ.Func().Returns(typ.String).Build()
	numberFn := typ.Func().Returns(typ.Number).Build()

	if _, ok := Callable(typ.NewUnion(stringFn, numberFn)); ok {
		t.Fatal("Callable(union with different function witnesses) succeeded")
	}
}

func TestMetamethodAnyUnknownNeverPolicy(t *testing.T) {
	tests := []struct {
		name string
		in   typ.Type
		want typ.Type
	}{
		{name: "any", in: typ.Any, want: typ.Any},
		{name: "unknown", in: typ.Unknown, want: typ.Unknown},
		{name: "never", in: typ.Never, want: typ.Never},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := GetMetamethod(tt.in, "__call")
			if !ok {
				t.Fatalf("GetMetamethod(%s, __call) failed", tt.name)
			}
			assertType(t, got, tt.want)
		})
	}
}

func TestCallableAnyUnknownNeverStrict(t *testing.T) {
	for _, tt := range []typ.Type{typ.Any, typ.Unknown, typ.Never} {
		if _, ok := Callable(tt); ok {
			t.Fatalf("Callable(%v) succeeded", tt)
		}
	}
}

func TestGetMetamethodUnionFailsWhenOneBranchLacksMetamethod(t *testing.T) {
	withCall := recordWithMetamethod("__call", typ.Func().Returns(typ.String).Build())
	withoutCall := typetable.NewRecord().Field("name", typ.String).Build()

	if _, ok := GetMetamethod(typ.NewUnion(withCall, withoutCall), "__call"); ok {
		t.Fatal("GetMetamethod(union, __call) succeeded when one branch lacked the metamethod")
	}
}

func TestCallableUnionFailsWhenOneRecordBranchLacksCall(t *testing.T) {
	withCall := recordWithMetamethod("__call", typ.Func().Returns(typ.String).Build())
	withoutCall := typetable.NewRecord().Field("name", typ.String).Build()

	if _, ok := Callable(typ.NewUnion(withCall, withoutCall)); ok {
		t.Fatal("Callable(union with one non-callable record branch) succeeded")
	}
}

func TestMemberCallUnionRequiresCallableMemberOnEveryAlternative(t *testing.T) {
	stringMethod, status := MemberCall(typ.String, "upper")
	if status != MemberCallOK {
		t.Fatalf("MemberCall(string, upper) status = %v, want ok", status)
	}
	if !callableValue(stringMethod, 0) {
		t.Fatalf("MemberCall(string, upper) type = %v, want callable", stringMethod)
	}

	if _, status := MemberCall(typ.NewUnion(typ.String, typ.Number), "upper"); status != MemberCallMissing {
		t.Fatalf("MemberCall(string|number, upper) status = %v, want missing", status)
	}

	left := typetable.NewRecord().
		Field("run", typ.Func().Returns(typ.String).Build()).
		Build()
	right := typetable.NewRecord().
		Field("run", typ.Func().Returns(typ.Number).Build()).
		Build()
	member, status := MemberCall(typ.NewUnion(left, right), "run")
	if status != MemberCallOK {
		t.Fatalf("MemberCall(callable record union, run) status = %v, want ok", status)
	}
	assertType(t, member, typ.NewUnion(
		typ.Func().Returns(typ.String).Build(),
		typ.Func().Returns(typ.Number).Build(),
	))
}

func TestMemberCallRejectsOptionalUnionMember(t *testing.T) {
	callable := typ.Func().Build()
	left := typetable.NewRecord().
		Field("run", callable).
		Build()
	right := typetable.NewRecord().
		OptField("run", callable).
		Build()

	member, status := MemberCall(typ.NewUnion(left, right), "run")
	if status != MemberCallNotCallable {
		t.Fatalf("MemberCall(optional member union, run) status = %v, want not-callable", status)
	}
	assertType(t, member, typ.NewOptional(callable))
}

func TestCallableReturnFirstReturn(t *testing.T) {
	fn := typ.Func().
		Param("input", typ.String).
		Returns(typ.Number, typ.Boolean).
		Build()

	got, ok := CallableReturn(fn)
	if !ok {
		t.Fatal("CallableReturn(function) failed")
	}
	assertType(t, got, typ.Number)
}

func TestCallableReturnUnionProjectionUsesNormalizePackage(t *testing.T) {
	callableReturning := func(t typ.Type) typ.Type {
		return typ.Func().Returns(t).Build()
	}

	returns := []typ.Type{typ.Unknown, typ.String, typ.Never}
	got, ok := CallableReturn(typ.NewUnion(
		callableReturning(returns[0]),
		callableReturning(returns[1]),
		callableReturning(returns[2]),
	))
	if !ok {
		t.Fatal("CallableReturn(union) failed")
	}
	assertType(t, got, normalize.UnionForEvidence(returns...))
}

func TestCallableReturnUnionProjectionPolicy(t *testing.T) {
	callableReturning := func(t typ.Type) typ.Type {
		return typ.Func().Returns(t).Build()
	}

	tests := []struct {
		name     string
		receiver typ.Type
		want     typ.Type
	}{
		{
			name: "any absorbs concrete projection",
			receiver: typ.NewUnion(
				callableReturning(typ.Any),
				callableReturning(typ.String),
			),
			want: typ.Any,
		},
		{
			name: "optional return preserves nilability",
			receiver: typ.NewUnion(
				callableReturning(typ.NewOptional(typ.Number)),
				callableReturning(typ.String),
			),
			want: typ.NewUnion(typ.Nil, typ.String, typ.Number),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := CallableReturn(tt.receiver)
			if !ok {
				t.Fatal("CallableReturn(union) failed")
			}
			assertType(t, got, tt.want)
		})
	}
}

func TestCallableReturnAnyUnknownPolicy(t *testing.T) {
	got, ok := CallableReturn(typ.Any)
	if !ok {
		t.Fatal("CallableReturn(any) failed")
	}
	assertType(t, got, typ.Any)

	got, ok = CallableReturn(typ.Unknown)
	if !ok {
		t.Fatal("CallableReturn(unknown) failed")
	}
	assertType(t, got, typ.Unknown)
}

func assertType(t *testing.T, got typ.Type, want typ.Type) {
	t.Helper()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("type = %v, want %v", got, want)
	}
}

func recordWithMetamethod(name string, mt typ.Type) *typ.Record {
	return typetable.NewRecord().
		Metatable(typetable.NewRecord().Field(name, mt).Build()).
		Build()
}
