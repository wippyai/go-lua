package subst

import (
	"strconv"
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func requireNoInstantiated(t *testing.T, tt typ.Type) {
	t.Helper()
	if typ.ContainsInstantiated(tt) {
		t.Fatalf("type still contains instantiated node: %v", tt)
	}
}

func requireUnionShape(t *testing.T, got typ.Type, wants ...typ.Type) *typ.Union {
	t.Helper()
	union, ok := got.(*typ.Union)
	if !ok {
		t.Fatalf("expanded type = %T %[1]v, want union", got)
	}
	if len(union.Members) != len(wants) {
		t.Fatalf("union members = %v, want %d members", union.Members, len(wants))
	}
	for _, want := range wants {
		found := false
		for _, member := range union.Members {
			if typ.TypeEquals(member, want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("union members = %v, missing %v", union.Members, want)
		}
	}
	return union
}

func requireIntersectionShape(t *testing.T, got typ.Type, wants ...typ.Type) *typ.Intersection {
	t.Helper()
	intersection, ok := got.(*typ.Intersection)
	if !ok {
		t.Fatalf("expanded type = %T %[1]v, want intersection", got)
	}
	if len(intersection.Members) != len(wants) {
		t.Fatalf("intersection members = %v, want %d members", intersection.Members, len(wants))
	}
	for _, want := range wants {
		found := false
		for _, member := range intersection.Members {
			if typ.TypeEquals(member, want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("intersection members = %v, missing %v", intersection.Members, want)
		}
	}
	return intersection
}

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
		fn := typ.Func().Param("x", tp).Returns(tp).Build()
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
	})

	t.Run("does not corrupt nested generic binder through instantiated result", func(t *testing.T) {
		outer := typ.NewTypeParam("T", nil)
		resultParam := typ.NewTypeParam("U", nil)
		resultGeneric := typ.NewGeneric("Result", []*typ.TypeParam{resultParam}, typetable.NewRecord().
			Field("ok", typ.Boolean).
			Field("value", resultParam).
			Build())
		callbackParam := typ.NewTypeParam("U", nil)
		callback := typ.Func().
			TypeParamRef(callbackParam).
			Param("input", outer).
			Returns(typ.Instantiate(resultGeneric, callbackParam)).
			Build()
		rec := typetable.NewRecord().
			Field("payload", outer).
			Field("callback", callback).
			Build()

		result := Substitute(rec, map[string]typ.Type{"T": typ.String})
		resultRec, ok := result.(*typ.Record)
		if !ok {
			t.Fatalf("result = %T %[1]v, want record", result)
		}
		if field := resultRec.GetField("payload"); field == nil || field.Type != typ.String {
			t.Fatalf("payload field = %v, want string", field)
		}
		callbackField := resultRec.GetField("callback")
		if callbackField == nil {
			t.Fatal("missing callback field")
		}
		resultCallback, ok := callbackField.Type.(*typ.Function)
		if !ok {
			t.Fatalf("callback field = %T %[1]v, want function", callbackField.Type)
		}
		if len(resultCallback.TypeParams) != 1 || resultCallback.TypeParams[0] != callbackParam {
			t.Fatalf("callback binder changed/captured: %#v, want original U binder", resultCallback.TypeParams)
		}
		if resultCallback.Params[0].Type != typ.String {
			t.Fatalf("callback input = %v, want substituted outer string", resultCallback.Params[0].Type)
		}
		callbackReturn, ok := resultCallback.Returns[0].(*typ.Instantiated)
		if !ok {
			t.Fatalf("callback return = %T %[1]v, want Result<U>", resultCallback.Returns[0])
		}
		if callbackReturn.Generic != resultGeneric {
			t.Fatalf("callback return generic = %v, want Result", callbackReturn.Generic)
		}
		if len(callbackReturn.TypeArgs) != 1 || callbackReturn.TypeArgs[0] != callbackParam {
			t.Fatalf("callback return args = %#v, want owned U binder", callbackReturn.TypeArgs)
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
		rec := typetable.NewRecord().
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

	t.Run("rewrites free outer param inside nested generic without capturing owned binder", func(t *testing.T) {
		outer := typ.NewTypeParam("T", nil)
		inner := typ.NewTypeParam("U", outer)
		resultGeneric := typ.NewGeneric("Result", []*typ.TypeParam{inner}, typetable.NewRecord().
			Field("value", inner).
			Field("outer", outer).
			Build())
		container := typetable.NewRecord().
			Field("payload", outer).
			Field("result", resultGeneric).
			Build()

		result := Params(container, []*typ.TypeParam{outer}, []typ.Type{typ.String})
		resultRec, ok := result.(*typ.Record)
		if !ok {
			t.Fatalf("result should be record, got %T", result)
		}
		if field := resultRec.GetField("payload"); field == nil || field.Type != typ.String {
			t.Fatalf("payload field = %v, want string", field)
		}
		resultField := resultRec.GetField("result")
		if resultField == nil {
			t.Fatal("missing result field")
		}
		gotGeneric, ok := resultField.Type.(*typ.Generic)
		if !ok {
			t.Fatalf("result field = %T %[1]v, want Result generic", resultField.Type)
		}
		if len(gotGeneric.TypeParams) != 1 {
			t.Fatalf("Result type params = %#v, want one owned binder", gotGeneric.TypeParams)
		}
		gotInner := gotGeneric.TypeParams[0]
		if gotInner == outer {
			t.Fatalf("nested generic binder was captured by outer binder")
		}
		if gotInner.Name != "U" {
			t.Fatalf("nested generic binder name = %q, want U", gotInner.Name)
		}
		if gotInner.Constraint != typ.String {
			t.Fatalf("nested generic binder constraint = %v, want rewritten outer string", gotInner.Constraint)
		}
		body, ok := gotGeneric.Body.(*typ.Record)
		if !ok {
			t.Fatalf("Result body = %T %[1]v, want record", gotGeneric.Body)
		}
		valueField := body.GetField("value")
		if valueField == nil || valueField.Type != gotInner {
			t.Fatalf("Result.value = %v, want owned binder %v", valueField, gotInner)
		}
		outerField := body.GetField("outer")
		if outerField == nil || outerField.Type != typ.String {
			t.Fatalf("Result.outer = %v, want rewritten outer string", outerField)
		}
	})

	t.Run("expands wrapper without corrupting instantiated result binder", func(t *testing.T) {
		outer := typ.NewTypeParam("T", nil)
		resultParam := typ.NewTypeParam("U", nil)
		resultGeneric := typ.NewGeneric("Result", []*typ.TypeParam{resultParam}, typetable.NewRecord().
			Field("ok", typ.Boolean).
			Field("value", resultParam).
			Build())
		callbackParam := typ.NewTypeParam("U", nil)
		callback := typ.Func().
			TypeParamRef(callbackParam).
			Param("input", outer).
			Returns(typ.Instantiate(resultGeneric, callbackParam)).
			Build()
		wrapper := typ.NewGeneric("Wrapper", []*typ.TypeParam{outer}, typetable.NewRecord().
			Field("payload", outer).
			Field("callback", callback).
			Field("result", typ.Instantiate(resultGeneric, outer)).
			Build())

		expanded := ExpandInstantiated(typ.Instantiate(wrapper, typ.String))
		expandedRecord, ok := expanded.(*typ.Record)
		if !ok {
			t.Fatalf("expanded = %T %[1]v, want record", expanded)
		}
		if field := expandedRecord.GetField("payload"); field == nil || field.Type != typ.String {
			t.Fatalf("payload field = %v, want string", field)
		}
		callbackField := expandedRecord.GetField("callback")
		if callbackField == nil {
			t.Fatal("missing callback field")
		}
		expandedCallback, ok := callbackField.Type.(*typ.Function)
		if !ok {
			t.Fatalf("callback field = %T %[1]v, want function", callbackField.Type)
		}
		if len(expandedCallback.TypeParams) != 1 || expandedCallback.TypeParams[0] != callbackParam {
			t.Fatalf("callback binder changed/captured: %#v, want original U binder", expandedCallback.TypeParams)
		}
		if expandedCallback.Params[0].Type != typ.String {
			t.Fatalf("callback input = %v, want substituted outer string", expandedCallback.Params[0].Type)
		}
		callbackReturn, ok := expandedCallback.Returns[0].(*typ.Record)
		if !ok {
			t.Fatalf("callback return = %T %[1]v, want expanded Result<U> record", expandedCallback.Returns[0])
		}
		callbackValue := callbackReturn.GetField("value")
		if callbackValue == nil || callbackValue.Type != callbackParam {
			t.Fatalf("callback return value = %v, want callback U binder", callbackValue)
		}
		resultField := expandedRecord.GetField("result")
		if resultField == nil {
			t.Fatal("missing result field")
		}
		resultRecord, ok := resultField.Type.(*typ.Record)
		if !ok {
			t.Fatalf("result field = %T %[1]v, want expanded Result<string> record", resultField.Type)
		}
		resultValue := resultRecord.GetField("value")
		if resultValue == nil || resultValue.Type != typ.String {
			t.Fatalf("result field value = %v, want string", resultValue)
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

	t.Run("rewrites function-valued record fields", func(t *testing.T) {
		method := typ.Func().Param("self", typ.Self).Returns(typ.Self).Build()
		rec := typetable.NewRecord().Field("method", method).Build()
		result := Self(rec, typ.String)
		resultRec, ok := result.(*typ.Record)
		if !ok {
			t.Fatalf("expected record, got %T", result)
		}
		field := resultRec.GetField("method")
		if field == nil {
			t.Fatal("missing method field")
		}
		gotFn, ok := field.Type.(*typ.Function)
		if !ok {
			t.Fatalf("method field = %T, want function", field.Type)
		}
		if gotFn.Params[0].Type != typ.String || gotFn.Returns[0] != typ.String {
			t.Fatalf("method type = %v, want self rewritten to string", gotFn)
		}
	})

	t.Run("replace self unioned with recursive payload", func(t *testing.T) {
		rec := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
			return typetable.NewRecord().Field("next", self).Build()
		})
		arg := typeexpr.Union(typ.Self, rec)
		result := Self(arg, typ.String)
		if !typ.TypeEquals(result, typeexpr.Union(typ.String, rec)) {
			t.Fatalf("union self substitution = %v", result)
		}
	})

	t.Run("skip concrete recursive payload without self", func(t *testing.T) {
		rec := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
			return typetable.NewRecord().
				Field("next", self).
				Field("handler", typ.Func().Param("node", self).Returns(self).Build()).
				Build()
		})
		wrapper := typetable.NewRecord().
			Field("payload", rec).
			Field("fn", typ.Func().Param("node", rec).Returns(rec).Build()).
			Build()
		if got := Self(wrapper, typ.String); got != wrapper {
			t.Fatalf("recursive payload without Self should be returned unchanged, got %v", got)
		}
	})

	t.Run("skip self inside recursive payload body", func(t *testing.T) {
		rec := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
			return typetable.NewRecord().
				Field("next", self).
				Field("owner", typ.Self).
				Build()
		})
		wrapper := typetable.NewRecord().Field("payload", rec).Build()
		if got := Self(wrapper, typ.String); got != wrapper {
			t.Fatalf("recursive payload body Self should not be surfaced, got %v", got)
		}
	})

	t.Run("retains surface self across large recursive types", func(t *testing.T) {
		const recordCount = 2050
		elements := make([]typ.Type, 0, recordCount+1)
		for i := 0; i < recordCount; i++ {
			elements = append(elements, typetable.NewRecord().Field("field_"+strconv.Itoa(i), typ.String).Build())
		}
		elements = append(elements, typ.Self)
		tuple := typ.NewTuple(elements...)
		rec := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
			return typetable.NewRecord().Field("next", self).Build()
		})

		result := Self(typeexpr.Union(rec, tuple), typ.String)
		union, ok := result.(*typ.Union)
		if !ok {
			t.Fatalf("result = %T, want union", result)
		}
		var rewritten *typ.Tuple
		for _, member := range union.Members {
			if candidate, ok := member.(*typ.Tuple); ok {
				rewritten = candidate
				break
			}
		}
		if rewritten == nil || len(rewritten.Elements) != recordCount+1 {
			t.Fatalf("rewritten tuple = %#v, want %d elements", rewritten, recordCount+1)
		}
		if rewritten.Elements[recordCount] != typ.String {
			t.Fatalf("surface Self after %d records = %v, want string", recordCount, rewritten.Elements[recordCount])
		}
	})
}

func BenchmarkSelfAcyclicNoSelf(b *testing.B) {
	fn := typ.Func().
		Param("payload", typetable.NewRecord().
			Field("id", typ.String).
			Field("count", typ.Number).
			Build()).
		Returns(typ.Boolean).
		Build()

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if Self(fn, typ.String) != fn {
			b.Fatal("acyclic type without Self should be returned unchanged")
		}
	}
}

func BenchmarkSelfAcyclicWithSelf(b *testing.B) {
	fn := typ.Func().
		Param("self", typ.Self).
		Param("payload", typetable.NewRecord().
			Field("id", typ.String).
			Field("count", typ.Number).
			Build()).
		Returns(typ.Self).
		Build()

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if Self(fn, typ.String) == fn {
			b.Fatal("acyclic type with Self should be rewritten")
		}
	}
}

func BenchmarkSelfRecursivePayloadNoSurfaceSelf(b *testing.B) {
	rec := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typetable.NewRecord().
			Field("next", self).
			Field("owner", typ.Self).
			Build()
	})
	wrapper := typetable.NewRecord().
		Field("payload", rec).
		Field("fn", typ.Func().Param("node", rec).Returns(rec).Build()).
		Build()

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if Self(wrapper, typ.String) != wrapper {
			b.Fatal("recursive payload without surface Self should be unchanged")
		}
	}
}

func TestSelfRef(t *testing.T) {
	t.Run("rewrites receiver ref but preserves unrelated same-name ref", func(t *testing.T) {
		receiverRef := typ.NewRef("", "Receiver")
		unrelatedRef := typ.NewRef("", "Receiver")
		fn := typ.Func().
			Param("self", receiverRef).
			Param("payload", typetable.NewRecord().
				Field("target", receiverRef).
				Field("shadow", unrelatedRef).
				Build()).
			Returns(receiverRef, unrelatedRef).
			Build()

		result, ok := SelfRef(fn, typ.String).(*typ.Function)
		if !ok {
			t.Fatalf("result should be function, got %T", result)
		}
		if result.Params[0].Type != typ.String {
			t.Fatalf("self param type = %v, want string", result.Params[0].Type)
		}
		payload, ok := result.Params[1].Type.(*typ.Record)
		if !ok {
			t.Fatalf("payload type = %T, want record", result.Params[1].Type)
		}
		target := payload.GetField("target")
		if target == nil || target.Type != typ.String {
			t.Fatalf("receiver ref field = %v, want string", target)
		}
		shadow := payload.GetField("shadow")
		if shadow == nil || shadow.Type != unrelatedRef {
			t.Fatalf("unrelated same-name ref was rewritten: got %v, want %v", shadow, unrelatedRef)
		}
		if len(result.Returns) != 2 {
			t.Fatalf("returns length = %d, want 2", len(result.Returns))
		}
		if result.Returns[0] != typ.String {
			t.Fatalf("first return = %v, want string", result.Returns[0])
		}
		if result.Returns[1] != unrelatedRef {
			t.Fatalf("unrelated return ref was rewritten: got %v, want %v", result.Returns[1], unrelatedRef)
		}
	})
}

func TestSelfValueStopsAtNestedCallableBinders(t *testing.T) {
	selfType := typetable.NewRecord().Field("id", typ.String).Build()
	nestedMethod := typ.Func().Param("self", typ.Self).Returns(typ.Self).Build()
	value := typetable.NewRecord().
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
			Param("ctx", typetable.NewRecord().
				Field("id", typ.String).
				Field("items", typ.NewArray(typetable.NewRecord().Field("name", typ.String).Build())).
				Build()).
			Returns(typetable.NewRecord().Field("ok", typ.Boolean).Build()).
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

	t.Run("looks through nested annotations", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		generic := typ.NewGeneric("Box", []*typ.TypeParam{tp},
			typetable.NewRecord().Field("value", tp).Build())
		inst := typ.Instantiate(generic, typ.String)
		annotated := typ.NewAnnotated(
			typ.NewAnnotated(inst, nil),
			nil,
		)

		result := ExpandInstantiated(annotated)
		rec, ok := result.(*typ.Record)
		if !ok {
			t.Fatalf("expected expanded record, got %T", result)
		}
		field := rec.GetField("value")
		if field == nil || field.Type != typ.String {
			t.Fatalf("expanded field = %#v, want string", field)
		}
	})

	t.Run("flattens union member expanded from type parameter", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		boxParam := typ.NewTypeParam("U", nil)
		box := typ.NewGeneric("Box", []*typ.TypeParam{boxParam}, boxParam)
		generic := typ.NewGeneric("UnionBox", []*typ.TypeParam{tp}, typeexpr.Union(
			typ.Instantiate(box, tp),
			typ.Boolean,
		))

		expanded := ExpandInstantiated(typ.Instantiate(generic, typeexpr.Union(typ.String, typ.Number)))
		union := requireUnionShape(t, expanded, typ.String, typ.Number, typ.Boolean)
		for _, member := range union.Members {
			if _, ok := member.(*typ.Union); ok {
				t.Fatalf("expanded union preserved nested union member: %v", union.Members)
			}
		}
		requireNoInstantiated(t, expanded)
	})

	t.Run("collapses optional union member expanded from type parameter", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		boxParam := typ.NewTypeParam("U", nil)
		box := typ.NewGeneric("Box", []*typ.TypeParam{boxParam}, boxParam)
		generic := typ.NewGeneric("NilableUnionBox", []*typ.TypeParam{tp}, typeexpr.Union(
			typ.Instantiate(box, tp),
			typ.Boolean,
		))

		expanded := ExpandInstantiated(typ.Instantiate(generic, typeexpr.Optional(typ.String)))
		union := requireUnionShape(t, expanded, typ.Nil, typ.String, typ.Boolean)
		for _, member := range union.Members {
			if _, ok := member.(*typ.Optional); ok {
				t.Fatalf("expanded union preserved optional member instead of nil + payload: %v", union.Members)
			}
		}
		requireNoInstantiated(t, expanded)
	})

	t.Run("flattens intersection member expanded from type parameter", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		boxParam := typ.NewTypeParam("U", nil)
		box := typ.NewGeneric("Box", []*typ.TypeParam{boxParam}, boxParam)
		generic := typ.NewGeneric("IntersectionBox", []*typ.TypeParam{tp}, typeexpr.Intersection(
			typ.Instantiate(box, tp),
			typ.Boolean,
		))

		expanded := ExpandInstantiated(typ.Instantiate(generic, typeexpr.Intersection(typ.String, typ.Number)))
		intersection := requireIntersectionShape(t, expanded, typ.String, typ.Number, typ.Boolean)
		for _, member := range intersection.Members {
			if _, ok := member.(*typ.Intersection); ok {
				t.Fatalf("expanded intersection preserved nested intersection member: %v", intersection.Members)
			}
		}
		requireNoInstantiated(t, expanded)
	})

	t.Run("normalizes instantiated table keys", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		optionalParam := typ.NewTypeParam("U", nil)
		optionalGeneric := typ.NewGeneric("OptionalKey", []*typ.TypeParam{optionalParam}, typeexpr.Optional(optionalParam))
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

		recordGeneric := typ.NewGeneric("RecordMap", []*typ.TypeParam{tp}, typetable.NewRecord().
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

	t.Run("normalizes direct optional map key parameter", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		generic := typ.NewGeneric("Map", []*typ.TypeParam{tp}, typ.NewMap(tp, typ.Number))

		expanded := ExpandInstantiated(typ.Instantiate(generic, typeexpr.Optional(typ.String)))
		result, ok := expanded.(*typ.Map)
		if !ok {
			t.Fatalf("expected map, got %T", expanded)
		}
		if !typ.TypeEquals(result.Key, typ.String) {
			t.Fatalf("instantiated map key = %v, want string", result.Key)
		}
	})

	t.Run("normalizes direct optional readonly map key parameter", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		generic := typ.NewGeneric("ReadonlyMap", []*typ.TypeParam{tp}, typ.NewReadonlyMap(tp, typ.Number))

		expanded := ExpandInstantiated(typ.Instantiate(generic, typeexpr.Optional(typ.String)))
		result, ok := expanded.(*typ.ReadonlyMap)
		if !ok {
			t.Fatalf("expected readonly map, got %T", expanded)
		}
		if !typ.TypeEquals(result.Key, typ.String) {
			t.Fatalf("instantiated readonly map key = %v, want string", result.Key)
		}
	})

	t.Run("normalizes direct optional record map key parameter", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		generic := typ.NewGeneric("RecordMap", []*typ.TypeParam{tp}, typetable.NewRecord().
			MapComponent(tp, typ.Number).
			Build())

		expanded := ExpandInstantiated(typ.Instantiate(generic, typeexpr.Optional(typ.String)))
		result, ok := expanded.(*typ.Record)
		if !ok {
			t.Fatalf("expected record, got %T", expanded)
		}
		if !typ.TypeEquals(result.MapKey, typ.String) {
			t.Fatalf("instantiated record map key = %v, want string", result.MapKey)
		}
	})

	t.Run("splits direct optional field payload parameter", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		generic := typ.NewGeneric("OptionalField", []*typ.TypeParam{tp}, typetable.NewRecord().
			OptField("value", tp).
			Build())

		expanded := ExpandInstantiated(typ.Instantiate(generic, typeexpr.Optional(typ.String)))
		result, ok := expanded.(*typ.Record)
		if !ok {
			t.Fatalf("expected record, got %T", expanded)
		}
		field := result.GetField("value")
		if field == nil || !field.Optional {
			t.Fatalf("value field = %#v, want optional field", field)
		}
		if !typ.TypeEquals(field.Type, typ.String) {
			t.Fatalf("value field type = %v, want string", field.Type)
		}
	})

	t.Run("expands and splits static member payload parameters", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		boxParam := typ.NewTypeParam("U", nil)
		optionalBox := typ.NewGeneric("OptionalBox", []*typ.TypeParam{boxParam}, typeexpr.Optional(boxParam))
		generic := typ.NewGeneric("StaticMembers", []*typ.TypeParam{tp}, typetable.RebuildRecord(typ.RecordParts{
			StaticMembers: []typ.StaticMember{
				{
					Kind:     typ.StaticMemberStringIndex,
					Name:     "direct",
					Type:     tp,
					Optional: true,
				},
				{
					Kind:     typ.StaticMemberStringIndex,
					Name:     "boxed",
					Type:     typ.Instantiate(optionalBox, tp),
					Optional: true,
				},
			},
		}))

		expanded := ExpandInstantiated(typ.Instantiate(generic, typeexpr.Optional(typ.String)))
		result, ok := expanded.(*typ.Record)
		if !ok {
			t.Fatalf("expected record, got %T", expanded)
		}
		for _, name := range []string{"direct", "boxed"} {
			member := result.GetStaticStringIndex(name)
			if member == nil || !member.Optional {
				t.Fatalf("static member %q = %#v, want optional member", name, member)
			}
			if !typ.TypeEquals(member.Type, typ.String) {
				t.Fatalf("static member %q type = %v, want string", name, member.Type)
			}
		}
		requireNoInstantiated(t, result)
	})

	t.Run("optional", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		generic := typ.NewGeneric("Opt", []*typ.TypeParam{tp}, typeexpr.Optional(tp))
		inst := typ.Instantiate(generic, typ.String)
		opt := typeexpr.Optional(inst)
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
		node.SetBody(typetable.NewRecord().
			Field("value", tp).
			Field("next", typeexpr.Optional(typ.Instantiate(node, tp))).
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

	t.Run("expands recursive instantiated record field", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		node := typ.NewGeneric("Node", []*typ.TypeParam{tp}, nil)
		node.SetBody(typetable.NewRecord().
			Field("value", tp).
			Field("next", typeexpr.Optional(typ.Instantiate(node, tp))).
			Build())
		wrapper := typetable.NewRecord().
			Field("node", typ.Instantiate(node, typ.String)).
			Build()

		result := ExpandInstantiated(wrapper)
		resultRec, ok := result.(*typ.Record)
		if !ok {
			t.Fatalf("result should be record, got %T", result)
		}
		field := resultRec.GetField("node")
		if field == nil {
			t.Fatal("missing node field")
		}
		if _, ok := field.Type.(*typ.Instantiated); ok {
			t.Fatalf("node field remained a lazy instantiation: %v", field.Type)
		}
		nodeRec, ok := field.Type.(*typ.Record)
		if !ok {
			t.Fatalf("node field type = %T %v, want expanded record", field.Type, field.Type)
		}
		value := nodeRec.GetField("value")
		if value == nil || value.Type != typ.String {
			t.Fatalf("value field = %v, want string field", value)
		}
	})
}

func TestExpandInstantiatedChanged(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		expanded, ok := ExpandInstantiatedChanged(nil)
		if expanded != nil || ok {
			t.Fatalf("nil expansion = (%v, %v), want (nil, false)", expanded, ok)
		}
	})

	t.Run("non-instantiated", func(t *testing.T) {
		expanded, ok := ExpandInstantiatedChanged(typ.String)
		if expanded != nil || ok {
			t.Fatalf("non-instantiated expansion = (%v, %v), want (nil, false)", expanded, ok)
		}
	})

	t.Run("instantiated", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		generic := typ.NewGeneric("Array", []*typ.TypeParam{tp}, typ.NewArray(tp))
		inst := typ.Instantiate(generic, typ.Number)
		expanded, ok := ExpandInstantiatedChanged(inst)
		if !ok {
			t.Fatal("instantiated type should report changed")
		}
		arr, ok := expanded.(*typ.Array)
		if !ok {
			t.Fatalf("expanded type = %T, want array", expanded)
		}
		if arr.Element != typ.Number {
			t.Fatalf("expanded element = %v, want number", arr.Element)
		}
	})
}
