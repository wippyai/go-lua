package typeaccess

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/annotation"
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

func recordWithMetamethod(name string, mt typ.Type) *typ.Record {
	return typetable.NewRecord().
		Metatable(typetable.NewRecord().Field(name, mt).Build()).
		Build()
}
