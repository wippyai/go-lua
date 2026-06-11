package subst

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/control"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestSubstitute(t *testing.T) {
	t.Run("empty subs", func(t *testing.T) {
		if Substitute(typ.String, nil) != typ.String {
			t.Error("empty subs should return original")
		}
	})

	t.Run("type param", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		subs := map[string]typ.Type{"T": typ.String}
		result := Substitute(tp, subs)
		if result != typ.String {
			t.Error("should substitute type param")
		}
	})

	t.Run("no match", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		subs := map[string]typ.Type{"U": typ.String}
		result := Substitute(tp, subs)
		if result != tp {
			t.Error("unmatched param should remain")
		}
	})

	t.Run("in function", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		row := effect.Empty.With(control.IO{})
		fn := typ.Func().Param("x", tp).Returns(tp).Effects(row).Build()
		subs := map[string]typ.Type{"T": typ.Number}
		result := Substitute(fn, subs)
		resultFn, ok := result.(*typ.Function)
		if !ok {
			t.Fatal("result should be function")
		}
		if resultFn.Params[0].Type != typ.Number {
			t.Error("param type should be substituted")
		}
		if resultFn.Returns[0] != typ.Number {
			t.Error("return type should be substituted")
		}
		if !resultFn.Effect.Equals(row) {
			t.Fatalf("effect = %v, want %v", resultFn.Effect, row)
		}
	})
}

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

	t.Run("instantiates function own binder", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		fn := typ.Func().
			TypeParamRef(tp).
			Param("x", tp).
			Returns(tp).
			Build()

		result := Params(fn, []*typ.TypeParam{tp}, []typ.Type{typ.String})
		resultFn, ok := result.(*typ.Function)
		if !ok {
			t.Fatalf("result should be function, got %T", result)
		}
		if len(resultFn.TypeParams) != 0 {
			t.Fatalf("instantiated function should drop owned binder, got %d", len(resultFn.TypeParams))
		}
		if resultFn.Params[0].Type != typ.String {
			t.Fatalf("param type = %v, want string", resultFn.Params[0].Type)
		}
		if resultFn.Returns[0] != typ.String {
			t.Fatalf("return type = %v, want string", resultFn.Returns[0])
		}
	})

	t.Run("does not capture nested same-name function binder", func(t *testing.T) {
		outer := typ.NewTypeParam("T", nil)
		inner := typ.NewTypeParam("T", nil)
		nested := typ.Func().
			TypeParamRef(inner).
			Param("x", inner).
			Returns(inner).
			Build()
		rec := typ.NewRecord().
			Field("value", outer).
			Field("callback", nested).
			Build()

		result := Params(rec, []*typ.TypeParam{outer}, []typ.Type{typ.String})
		resultRec, ok := result.(*typ.Record)
		if !ok {
			t.Fatalf("result should be record, got %T", result)
		}
		if field := resultRec.GetField("value"); field == nil || field.Type != typ.String {
			t.Fatalf("value field = %v, want string", field)
		}
		callback := resultRec.GetField("callback")
		if callback == nil {
			t.Fatal("missing callback field")
		}
		if callback.Type != nested {
			t.Fatalf("nested same-name binder was captured: got %v, want original nested function", callback.Type)
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

	t.Run("replace self unioned with recursive payload", func(t *testing.T) {
		rec := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
			return typ.NewRecord().Field("next", self).Build()
		})
		arg := typ.NewUnion(typ.Self, rec)
		result := Self(arg, typ.String)
		if !typ.TypeEquals(result, typ.NewUnion(typ.String, rec)) {
			t.Fatalf("union self substitution = %v", result)
		}
	})

	t.Run("skip concrete recursive payload without self", func(t *testing.T) {
		rec := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
			return typ.NewRecord().
				Field("next", self).
				Field("handler", typ.Func().Param("node", self).Returns(self).Build()).
				Build()
		})
		wrapper := typ.NewRecord().
			Field("payload", rec).
			Field("fn", typ.Func().Param("node", rec).Returns(rec).Build()).
			Build()
		if got := Self(wrapper, typ.String); got != wrapper {
			t.Fatalf("recursive payload without Self should be returned unchanged, got %v", got)
		}
	})
}

func TestSelfValueStopsAtNestedCallableBinders(t *testing.T) {
	selfType := typ.NewRecord().Field("id", typ.String).Build()
	nestedMethod := typ.Func().Param("self", typ.Self).Returns(typ.Self).Build()
	value := typ.NewRecord().
		Field("owner", typ.Self).
		Field("method", nestedMethod).
		Build()

	result := SelfValue(value, selfType)
	rec, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("expected record, got %T", result)
	}
	owner := rec.GetField("owner")
	if owner == nil || owner.Type != selfType {
		t.Fatalf("expected free Self field to become receiver type, got %v", owner)
	}
	method := rec.GetField("method")
	if method == nil || method.Type != nestedMethod {
		t.Fatalf("expected nested function Self binder to be preserved, got %v", method)
	}
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

	t.Run("non-instantiated structural product", func(t *testing.T) {
		concrete := typ.Func().
			Param("ctx", typ.NewRecord().
				Field("id", typ.String).
				Field("items", typ.NewArray(typ.NewRecord().Field("name", typ.String).Build())).
				Build()).
			Returns(typ.NewRecord().Field("ok", typ.Boolean).Build()).
			Build()
		if ExpandInstantiated(concrete) != concrete {
			t.Error("concrete structural product should return original")
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

	t.Run("normalizes instantiated table keys", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		optionalParam := typ.NewTypeParam("U", nil)
		optionalGeneric := typ.NewGeneric("OptionalKey", []*typ.TypeParam{optionalParam}, typ.NewOptional(optionalParam))
		nilableKey := typ.Instantiate(optionalGeneric, tp)

		mapGeneric := typ.NewGeneric("Map", []*typ.TypeParam{tp}, typ.NewMap(nilableKey, typ.Number))
		mapExpanded := ExpandInstantiated(typ.Instantiate(mapGeneric, typ.String))
		mapResult, ok := mapExpanded.(*typ.Map)
		if !ok {
			t.Fatalf("expected map, got %T", mapExpanded)
		}
		if mapResult.Key != typ.String {
			t.Fatalf("instantiated map key = %v, want string", mapResult.Key)
		}

		readonlyGeneric := typ.NewGeneric("ReadonlyMap", []*typ.TypeParam{tp}, typ.NewReadonlyMap(nilableKey, typ.Number))
		readonlyExpanded := ExpandInstantiated(typ.Instantiate(readonlyGeneric, typ.String))
		readonlyResult, ok := readonlyExpanded.(*typ.ReadonlyMap)
		if !ok {
			t.Fatalf("expected readonly map, got %T", readonlyExpanded)
		}
		if readonlyResult.Key != typ.String {
			t.Fatalf("instantiated readonly map key = %v, want string", readonlyResult.Key)
		}

		recordGeneric := typ.NewGeneric("RecordMap", []*typ.TypeParam{tp}, typ.NewRecord().
			MapComponent(nilableKey, typ.Number).
			Build())
		recordExpanded := ExpandInstantiated(typ.Instantiate(recordGeneric, typ.String))
		recordResult, ok := recordExpanded.(*typ.Record)
		if !ok {
			t.Fatalf("expected record, got %T", recordExpanded)
		}
		if recordResult.MapKey != typ.String {
			t.Fatalf("instantiated record map key = %v, want string", recordResult.MapKey)
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

	t.Run("preserves nested function type params", func(t *testing.T) {
		boxParam := typ.NewTypeParam("T", nil)
		box := typ.NewGeneric("Box", []*typ.TypeParam{boxParam}, boxParam)
		inner := typ.NewTypeParam("S", nil)
		fn := typ.Func().
			TypeParamRef(inner).
			Param("x", typ.Instantiate(box, typ.String)).
			Returns(inner).
			Build()

		result := ExpandInstantiated(fn)
		resultFn, ok := result.(*typ.Function)
		if !ok {
			t.Fatalf("result should be function, got %T", result)
		}
		if len(resultFn.TypeParams) != 1 || resultFn.TypeParams[0] != inner {
			t.Fatalf("nested function binder not preserved: %v", resultFn.TypeParams)
		}
		if resultFn.Params[0].Type != typ.String {
			t.Fatalf("param type = %v, want string", resultFn.Params[0].Type)
		}
		if resultFn.Returns[0] != inner {
			t.Fatalf("return type = %v, want nested binder", resultFn.Returns[0])
		}
	})

	t.Run("keeps recursive instantiated function parameter lazy", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		node := typ.NewGeneric("Node", []*typ.TypeParam{tp}, nil)
		node.SetBody(typ.NewRecord().
			Field("value", tp).
			Field("next", typ.NewOptional(typ.Instantiate(node, tp))).
			Build())
		fn := typ.Func().
			Param("node", typ.Instantiate(node, typ.String)).
			Build()

		result := ExpandInstantiated(fn)
		resultFn, ok := result.(*typ.Function)
		if !ok {
			t.Fatalf("result should be function, got %T", result)
		}
		inst, ok := resultFn.Params[0].Type.(*typ.Instantiated)
		if !ok {
			t.Fatalf("param type = %T %v, want lazy recursive instantiation", resultFn.Params[0].Type, resultFn.Params[0].Type)
		}
		if inst.Generic != node || len(inst.TypeArgs) != 1 || inst.TypeArgs[0] != typ.String {
			t.Fatalf("param instantiation = %v, want Node<string>", inst)
		}
	})
}
