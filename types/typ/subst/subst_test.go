package subst

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestParams(t *testing.T) {
	t.Run("mismatched lengths", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		params := []*typ.TypeParam{tp}
		var args []typ.Type
		if Params(typ.String, params, args) != typ.String {
			t.Error("mismatched lengths should return original")
		}
	})

	t.Run("substitute params", func(t *testing.T) {
		tp1 := typ.NewTypeParam("T", nil)
		tp2 := typ.NewTypeParam("U", nil)
		tuple := typ.NewTuple(tp1, tp2)
		params := []*typ.TypeParam{tp1, tp2}
		args := []typ.Type{typ.String, typ.Number}
		result := Params(tuple, params, args)
		resultTuple, ok := result.(*typ.Tuple)
		if !ok {
			t.Fatal("result should be tuple")
		}
		if resultTuple.Elements[0] != typ.String {
			t.Error("first element should be String")
		}
		if resultTuple.Elements[1] != typ.Number {
			t.Error("second element should be Number")
		}
	})
}

func TestSelf(t *testing.T) {
	t.Run("nil type", func(t *testing.T) {
		if Self(nil, typ.String) != nil {
			t.Error("nil type should return nil")
		}
	})

	t.Run("nil self", func(t *testing.T) {
		if Self(typ.String, nil) != typ.String {
			t.Error("nil self should return original")
		}
	})

	t.Run("replace self", func(t *testing.T) {
		fn := typ.Func().Param("self", typ.Self).Returns(typ.Self).Build()
		result := Self(fn, typ.String)
		resultFn, ok := result.(*typ.Function)
		if !ok {
			t.Fatal("result should be function")
		}
		if resultFn.Params[0].Type != typ.String {
			t.Error("self param should be substituted")
		}
		if resultFn.Returns[0] != typ.String {
			t.Error("self return should be substituted")
		}
	})
}

func TestExpandInstantiated(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if ExpandInstantiated(nil) != nil {
			t.Error("nil should return nil")
		}
	})

	t.Run("non-instantiated", func(t *testing.T) {
		if ExpandInstantiated(typ.String) != typ.String {
			t.Error("non-instantiated should return original")
		}
	})

	t.Run("array of type param", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		generic := typ.NewGeneric("Array", []*typ.TypeParam{tp}, typ.NewArray(tp))
		inst := typ.Instantiate(generic, typ.Number)
		result := ExpandInstantiated(inst)
		arr, ok := result.(*typ.Array)
		if !ok {
			t.Fatalf("expected array, got %T", result)
		}
		if arr.Element != typ.Number {
			t.Error("element should be Number")
		}
	})

	t.Run("optional", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		generic := typ.NewGeneric("Opt", []*typ.TypeParam{tp}, typ.NewOptional(tp))
		inst := typ.Instantiate(generic, typ.String)
		opt := typ.NewOptional(inst)
		result := ExpandInstantiated(opt)
		if result == opt {
			t.Error("should expand nested instantiated")
		}
	})
}

// A type argument may contain a type parameter of another binder with the
// same name as the callee's; substitution keeps the two distinct.
func TestParamsKeepsForeignSameNamedParam(t *testing.T) {
	fn := typ.Func().TypeParam("T", nil).
		Param("list", typ.NewArray(typ.NewTypeParam("T", nil))).
		Returns(typ.NewTypeParam("T", nil)).
		Build()
	foreign := typ.NewTypeParam("T", typ.Number)
	arg := typ.NewRecord().Field("v", foreign).Build()

	result, ok := Params(fn, fn.TypeParams, []typ.Type{arg}).(*typ.Function)
	if !ok {
		t.Fatal("result should be a function")
	}
	elem := result.Params[0].Type.(*typ.Array).Element.(*typ.Record)
	if got := elem.GetField("v").Type; got != foreign {
		t.Fatalf("parameter element field: want the foreign %s, got %s", foreign, got)
	}
	if got := result.Returns[0].(*typ.Record).GetField("v").Type; got != foreign {
		t.Fatalf("return field: want the foreign %s, got %s", foreign, got)
	}
}
