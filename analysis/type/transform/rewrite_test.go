package transform

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

type testRecordBuilder struct {
	parts typ.RecordParts
}

func newRecord() *testRecordBuilder {
	return &testRecordBuilder{}
}

func (b *testRecordBuilder) Build() *typ.Record {
	return typ.RebuildRecord(b.parts)
}

func (b *testRecordBuilder) Field(name string, t typ.Type) *testRecordBuilder {
	b.parts.Fields = append(b.parts.Fields, typ.Field{Name: name, Type: t})
	return b
}

func (b *testRecordBuilder) OptField(name string, t typ.Type) *testRecordBuilder {
	b.parts.Fields = append(b.parts.Fields, typ.Field{Name: name, Type: t, Optional: true})
	return b
}

func (b *testRecordBuilder) OptStaticStringIndex(name string, t typ.Type) *testRecordBuilder {
	b.parts.StaticMembers = append(b.parts.StaticMembers, typ.StaticMember{Kind: typ.StaticMemberStringIndex, Name: name, Type: t, Optional: true})
	return b
}

func (b *testRecordBuilder) ReadonlyField(name string, t typ.Type) *testRecordBuilder {
	b.parts.Fields = append(b.parts.Fields, typ.Field{Name: name, Type: t, Readonly: true})
	return b
}

func (b *testRecordBuilder) Metatable(t typ.Type) *testRecordBuilder {
	b.parts.Metatable = t
	return b
}

func (b *testRecordBuilder) MapComponent(key, value typ.Type) *testRecordBuilder {
	b.parts.MapKey = key
	b.parts.MapValue = value
	return b
}

// replaceSelf is a test helper that replaces Self with a given type.
func replaceSelf(selfType typ.Type) func(typ.Type) (typ.Type, bool) {
	return func(t typ.Type) (typ.Type, bool) {
		if t.Kind() == kind.Self {
			return selfType, true
		}
		return nil, false
	}
}

// replaceNumber replaces Number with a given type.
func replaceNumber(replacement typ.Type) func(typ.Type) (typ.Type, bool) {
	return func(t typ.Type) (typ.Type, bool) {
		if t == typ.Number {
			return replacement, true
		}
		return nil, false
	}
}

func requireRewriteUnionMembers(t *testing.T, got typ.Type, wants ...typ.Type) *typ.Union {
	t.Helper()
	union, ok := got.(*typ.Union)
	if !ok {
		t.Fatalf("expected Union, got %T %[1]v", got)
	}
	if len(union.Members) != len(wants) {
		t.Fatalf("union members = %v, want %v", union.Members, wants)
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

func requireRewriteIntersectionMembers(t *testing.T, got typ.Type, wants ...typ.Type) *typ.Intersection {
	t.Helper()
	intersection, ok := got.(*typ.Intersection)
	if !ok {
		t.Fatalf("expected Intersection, got %T %[1]v", got)
	}
	if len(intersection.Members) != len(wants) {
		t.Fatalf("intersection members = %v, want %v", intersection.Members, wants)
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

func TestRewrite_Nil(t *testing.T) {
	result := Rewrite(nil, replaceNumber(typ.String))
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
}

func TestRewrite_NoMatch(t *testing.T) {
	result := Rewrite(typ.Boolean, replaceNumber(typ.String))
	if result != typ.Boolean {
		t.Fatalf("expected Boolean unchanged, got %v", result)
	}
}

func TestRewrite_DirectReplacement(t *testing.T) {
	result := Rewrite(typ.Number, replaceNumber(typ.String))
	if result != typ.String {
		t.Fatalf("expected String, got %v", result)
	}
}

func TestRewrite_Optional(t *testing.T) {
	opt := typeexpr.Optional(typ.Number)
	result := Rewrite(opt, replaceNumber(typ.String))
	o, ok := result.(*typ.Optional)
	if !ok {
		t.Fatalf("expected Optional, got %T", result)
	}
	if o.Inner != typ.String {
		t.Fatalf("expected inner String, got %v", o.Inner)
	}
}

func TestRewrite_Union(t *testing.T) {
	u := typeexpr.Union(typ.Number, typ.Boolean)
	result := Rewrite(u, replaceNumber(typ.String))
	union, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("expected Union, got %T", result)
	}
	found := false
	for _, m := range union.Members {
		if m == typ.String {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected String in union, got %v", result)
	}
}

func TestRewrite_UnionReplacementFlattensNestedUnionMember(t *testing.T) {
	nested := typeexpr.Union(typ.String, typ.Integer)
	result := Rewrite(typeexpr.Union(typ.Number, typ.Boolean), replaceNumber(nested))

	union := requireRewriteUnionMembers(t, result, typ.String, typ.Integer, typ.Boolean)
	for _, member := range union.Members {
		if _, ok := member.(*typ.Union); ok {
			t.Fatalf("expected rewritten nested union to flatten, got member %v in %v", member, union.Members)
		}
	}
}

func TestRewrite_UnionReplacementExpandsOptionalMember(t *testing.T) {
	optionalString := typeexpr.Optional(typ.String)
	result := Rewrite(typeexpr.Union(typ.Number, typ.Boolean), replaceNumber(optionalString))

	union := requireRewriteUnionMembers(t, result, typ.Nil, typ.String, typ.Boolean)
	for _, member := range union.Members {
		if _, ok := member.(*typ.Optional); ok {
			t.Fatalf("expected rewritten optional union member to expand, got member %v in %v", member, union.Members)
		}
	}
}

func TestRewrite_Intersection(t *testing.T) {
	rec := newRecord().Field("x", typ.Boolean).Build()
	inter := typeexpr.Intersection(typ.Number, rec)
	result := Rewrite(inter, replaceNumber(typ.String))
	intersection, ok := result.(*typ.Intersection)
	if !ok {
		t.Fatalf("expected Intersection, got %T", result)
	}
	found := false
	for _, m := range intersection.Members {
		if m == typ.String {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected String in intersection, got %v", result)
	}
}

func TestRewrite_IntersectionReplacementFlattensNestedIntersectionMember(t *testing.T) {
	nested := typeexpr.Intersection(typ.String, typ.Integer)
	rec := newRecord().Field("x", typ.Boolean).Build()
	result := Rewrite(typeexpr.Intersection(typ.Number, rec), replaceNumber(nested))

	intersection := requireRewriteIntersectionMembers(t, result, typ.String, typ.Integer, rec)
	for _, member := range intersection.Members {
		if _, ok := member.(*typ.Intersection); ok {
			t.Fatalf("expected rewritten nested intersection to flatten, got member %v in %v", member, intersection.Members)
		}
	}
}

func TestRewrite_Array(t *testing.T) {
	arr := typ.NewArray(typ.Number)
	result := Rewrite(arr, replaceNumber(typ.String))
	a, ok := result.(*typ.Array)
	if !ok {
		t.Fatalf("expected Array, got %T", result)
	}
	if a.Element != typ.String {
		t.Fatalf("expected element String, got %v", a.Element)
	}
}

func TestRewrite_Map(t *testing.T) {
	m := typ.NewMap(typ.String, typ.Number)
	result := Rewrite(m, replaceNumber(typ.Integer))
	mp, ok := result.(*typ.Map)
	if !ok {
		t.Fatalf("expected Map, got %T", result)
	}
	if mp.Key != typ.String {
		t.Fatalf("expected key String, got %v", mp.Key)
	}
	if mp.Value != typ.Integer {
		t.Fatalf("expected value Integer, got %v", mp.Value)
	}
}

func TestRewrite_MapNormalizesRewrittenNilableKey(t *testing.T) {
	m := typ.NewMap(typ.Number, typ.Number)
	result := Rewrite(m, replaceNumber(typeexpr.Optional(typ.String)))
	mp, ok := result.(*typ.Map)
	if !ok {
		t.Fatalf("expected Map, got %T", result)
	}
	if !typ.TypeEquals(mp.Key, typ.String) {
		t.Fatalf("expected normalized key String, got %v", mp.Key)
	}
	if !typ.TypeEquals(mp.Value, typeexpr.Optional(typ.String)) {
		t.Fatalf("expected value optional String, got %v", mp.Value)
	}
}

func TestRewrite_ReadonlyMapNormalizesRewrittenNilableKey(t *testing.T) {
	m := typ.NewReadonlyMap(typ.Number, typ.Number)
	result := Rewrite(m, replaceNumber(typeexpr.Union(typ.String, typ.Nil)))
	mp, ok := result.(*typ.ReadonlyMap)
	if !ok {
		t.Fatalf("expected ReadonlyMap, got %T", result)
	}
	if !typ.TypeEquals(mp.Key, typ.String) {
		t.Fatalf("expected normalized key String, got %v", mp.Key)
	}
	if !typ.TypeEquals(mp.Value, typeexpr.Union(typ.String, typ.Nil)) {
		t.Fatalf("expected value string|nil, got %v", mp.Value)
	}
}

func TestRewrite_Tuple(t *testing.T) {
	tup := typ.NewTuple(typ.Number, typ.Boolean)
	result := Rewrite(tup, replaceNumber(typ.String))
	tuple, ok := result.(*typ.Tuple)
	if !ok {
		t.Fatalf("expected Tuple, got %T", result)
	}
	if tuple.Elements[0] != typ.String {
		t.Fatalf("expected first element String, got %v", tuple.Elements[0])
	}
	if tuple.Elements[1] != typ.Boolean {
		t.Fatalf("expected second element Boolean, got %v", tuple.Elements[1])
	}
}

func TestRewrite_Function(t *testing.T) {
	fn := typ.Func().Param("a", typ.Number).Returns(typ.Number).Build()
	result := Rewrite(fn, replaceNumber(typ.String))
	f, ok := result.(*typ.Function)
	if !ok {
		t.Fatalf("expected Function, got %T", result)
	}
	if f.Params[0].Type != typ.String {
		t.Fatalf("expected param String, got %v", f.Params[0].Type)
	}
	if f.Returns[0] != typ.String {
		t.Fatalf("expected return String, got %v", f.Returns[0])
	}
}

func TestRewrite_FunctionVariadic(t *testing.T) {
	fn := typ.Func().Variadic(typ.Number).Returns(typ.Boolean).Build()
	result := Rewrite(fn, replaceNumber(typ.String))
	f, ok := result.(*typ.Function)
	if !ok {
		t.Fatalf("expected Function, got %T", result)
	}
	if f.Variadic != typ.String {
		t.Fatalf("expected variadic String, got %v", f.Variadic)
	}
}

func TestRewrite_FunctionPreservesOptionalParam(t *testing.T) {
	fn := typ.Func().OptParam("a", typ.Number).Returns(typ.Boolean).Build()
	result := Rewrite(fn, replaceNumber(typ.String))
	f, ok := result.(*typ.Function)
	if !ok {
		t.Fatalf("expected Function, got %T", result)
	}
	if !f.Params[0].Optional {
		t.Fatal("expected param to remain optional")
	}
}

func TestRewrite_Record(t *testing.T) {
	rec := newRecord().Field("x", typ.Number).OptField("y", typ.Number).Build()
	result := Rewrite(rec, replaceNumber(typ.String))
	r, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("expected Record, got %T", result)
	}
	x := r.GetField("x")
	if x == nil || x.Type != typ.String {
		t.Fatalf("expected x String, got %v", x)
	}
	y := r.GetField("y")
	if y == nil || y.Type != typ.String || !y.Optional {
		t.Fatalf("expected y optional String, got %v", y)
	}
}

func TestRewrite_RecordNoOpReturnsSamePointer(t *testing.T) {
	rec := newRecord().
		Field("x", typ.String).
		OptStaticStringIndex("raw", typ.Boolean).
		Build()

	result := Rewrite(rec, replaceNumber(typ.Integer))
	if result != rec {
		t.Fatal("expected record node to preserve pointer when fields and static members are unchanged")
	}
}

func TestRewrite_RecordNormalizesNilableOptionalFieldPayload(t *testing.T) {
	rec := newRecord().OptField("maybe", typ.Number).Build()
	result := Rewrite(rec, replaceNumber(typeexpr.Union(typ.String, typ.Nil)))
	r, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("expected Record, got %T", result)
	}
	field := r.GetField("maybe")
	if field == nil || !field.Optional {
		t.Fatalf("expected maybe optional field, got %v", field)
	}
	if !typ.TypeEquals(field.Type, typ.String) {
		t.Fatalf("expected normalized field payload String, got %v", field.Type)
	}
}

func TestRewrite_RecordNormalizesNilableOptionalStaticMemberPayload(t *testing.T) {
	rec := newRecord().OptStaticStringIndex("maybe", typ.Number).Build()
	result := Rewrite(rec, replaceNumber(typeexpr.Optional(typ.String)))
	r, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("expected Record, got %T", result)
	}
	member := r.GetStaticStringIndex("maybe")
	if member == nil || !member.Optional {
		t.Fatalf("expected maybe optional static member, got %v", member)
	}
	if !typ.TypeEquals(member.Type, typ.String) {
		t.Fatalf("expected normalized static member payload String, got %v", member.Type)
	}
}

func TestRewrite_RecordNormalizesRewrittenMapKey(t *testing.T) {
	rec := newRecord().MapComponent(typ.Number, typ.Boolean).Build()
	result := Rewrite(rec, replaceNumber(typeexpr.Optional(typ.String)))
	r, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("expected Record, got %T", result)
	}
	if !typ.TypeEquals(r.MapKey, typ.String) {
		t.Fatalf("expected normalized map key String, got %v", r.MapKey)
	}
	if r.MapValue != typ.Boolean {
		t.Fatalf("expected map value Boolean, got %v", r.MapValue)
	}
}

func TestRewrite_RecordReadonly(t *testing.T) {
	rec := newRecord().ReadonlyField("id", typ.Number).Build()
	result := Rewrite(rec, replaceNumber(typ.String))
	r, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("expected Record, got %T", result)
	}
	f := r.GetField("id")
	if f == nil || f.Type != typ.String || !f.Readonly {
		t.Fatalf("expected readonly String, got %v", f)
	}
}

func TestRewrite_RecordMetatable(t *testing.T) {
	mt := newRecord().Field("__index", typ.Number).Build()
	rec := newRecord().Field("x", typ.Boolean).Metatable(mt).Build()
	result := Rewrite(rec, replaceNumber(typ.String))
	r, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("expected Record, got %T", result)
	}
	if r.Metatable == nil {
		t.Fatal("expected metatable preserved")
	}
	mtRec := r.Metatable.(*typ.Record)
	f := mtRec.GetField("__index")
	if f == nil || f.Type != typ.String {
		t.Fatalf("expected metatable __index String, got %v", f)
	}
}

func TestRewrite_Alias(t *testing.T) {
	alias := typ.NewAlias("Num", typ.Number)
	result := Rewrite(alias, replaceNumber(typ.String))
	a, ok := result.(*typ.Alias)
	if !ok {
		t.Fatalf("expected Alias, got %T", result)
	}
	if a.Name != "Num" || a.Target != typ.String {
		t.Fatalf("expected Alias Num -> String, got %v -> %v", a.Name, a.Target)
	}
}

func TestRewrite_Instantiated(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	g := typ.NewGeneric("Box", []*typ.TypeParam{tp}, typ.NewArray(tp))
	inst := typ.Instantiate(g, typ.Number)
	result := Rewrite(inst, replaceNumber(typ.String))
	i, ok := result.(*typ.Instantiated)
	if !ok {
		t.Fatalf("expected Instantiated, got %T", result)
	}
	if i.TypeArgs[0] != typ.String {
		t.Fatalf("expected type arg String, got %v", i.TypeArgs[0])
	}
}

func TestRewrite_InstantiatedNoOpReturnsSamePointer(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	g := typ.NewGeneric("Box", []*typ.TypeParam{tp}, typ.NewArray(tp))
	inst := typ.Instantiate(g, typ.String)

	result := Rewrite(inst, replaceNumber(typ.Boolean))
	if result != inst {
		t.Fatal("expected instantiated node to preserve pointer when type args are unchanged")
	}
}

func TestRewrite_Meta(t *testing.T) {
	meta := typ.NewMeta(typ.Number)
	result := Rewrite(meta, replaceNumber(typ.String))
	got, ok := result.(*typ.Meta)
	if !ok {
		t.Fatalf("expected Meta, got %T", result)
	}
	if got.Of != typ.String {
		t.Fatalf("expected meta payload String, got %v", got.Of)
	}
}

func TestRewrite_TypeParamConstraint(t *testing.T) {
	tp := typ.NewTypeParam("T", typ.Number)
	result := Rewrite(tp, replaceNumber(typ.String))
	got, ok := result.(*typ.TypeParam)
	if !ok {
		t.Fatalf("expected TypeParam, got %T", result)
	}
	if got.Constraint != typ.String {
		t.Fatalf("expected constraint String, got %v", got.Constraint)
	}
}

func TestRewrite_GenericBodyAndTypeParamConstraint(t *testing.T) {
	tp := typ.NewTypeParam("T", typ.Number)
	g := typ.NewGeneric("Box", []*typ.TypeParam{tp}, newRecord().Field("value", tp).Build())

	result := Rewrite(g, replaceNumber(typ.String))
	got, ok := result.(*typ.Generic)
	if !ok {
		t.Fatalf("expected Generic, got %T", result)
	}
	if len(got.TypeParams) != 1 || got.TypeParams[0].Constraint != typ.String {
		t.Fatalf("expected rewritten type param constraint, got %#v", got.TypeParams)
	}
	body, ok := got.Body.(*typ.Record)
	if !ok {
		t.Fatalf("expected generic body record, got %T", got.Body)
	}
	field := body.GetField("value")
	if field == nil || field.Type != got.TypeParams[0] {
		t.Fatalf("expected body to reference rewritten type param, got %v", field)
	}
}

func TestRewrite_GenericDescentDoesNotCaptureNestedSameNameBinder(t *testing.T) {
	outer := typ.NewTypeParam("T", nil)
	inner := typ.NewTypeParam("T", nil)
	innerResult := typ.NewGeneric("Result", []*typ.TypeParam{inner}, newRecord().
		Field("ok", typ.Boolean).
		Field("value", inner).
		Build())
	outerRecord := newRecord().
		Field("outer", outer).
		Field("inner", innerResult).
		Build()

	got := Rewrite(outerRecord, func(node typ.Type) (typ.Type, bool) {
		if tp, ok := node.(*typ.TypeParam); ok && tp.Name == "T" {
			return typ.String, true
		}
		return nil, false
	})
	body, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("rewrite result = %T, want record", got)
	}
	outerField := body.GetField("outer")
	if outerField == nil || outerField.Type != typ.String {
		t.Fatalf("outer field = %v, want substituted string", outerField)
	}
	innerField := body.GetField("inner")
	if innerField == nil {
		t.Fatal("missing inner field")
	}
	gotInner, ok := innerField.Type.(*typ.Generic)
	if !ok {
		t.Fatalf("inner field = %T %[1]v, want Result generic", innerField.Type)
	}
	if len(gotInner.TypeParams) != 1 || gotInner.TypeParams[0] != inner {
		t.Fatalf("inner binder changed/captured: %#v, want original inner binder", gotInner.TypeParams)
	}
	gotInnerBody, ok := gotInner.Body.(*typ.Record)
	if !ok {
		t.Fatalf("inner body = %T, want record", gotInner.Body)
	}
	valueField := gotInnerBody.GetField("value")
	if valueField == nil || valueField.Type != inner {
		t.Fatalf("inner value field = %v, want original inner binder", valueField)
	}
}

func TestRewrite_GenericDescentRewritesOuterParamWithoutCapturingNestedBinder(t *testing.T) {
	outer := typ.NewTypeParam("T", nil)
	inner := typ.NewTypeParam("U", outer)
	innerResult := typ.NewGeneric("Result", []*typ.TypeParam{inner}, newRecord().
		Field("value", inner).
		Field("outer", outer).
		Build())
	outerRecord := newRecord().
		Field("payload", outer).
		Field("result", innerResult).
		Build()

	got := Rewrite(outerRecord, func(node typ.Type) (typ.Type, bool) {
		if node == outer {
			return typ.String, true
		}
		return nil, false
	})
	body, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("rewrite result = %T, want record", got)
	}
	payloadField := body.GetField("payload")
	if payloadField == nil || payloadField.Type != typ.String {
		t.Fatalf("payload field = %v, want substituted string", payloadField)
	}
	resultField := body.GetField("result")
	if resultField == nil {
		t.Fatal("missing result field")
	}
	gotInner, ok := resultField.Type.(*typ.Generic)
	if !ok {
		t.Fatalf("result field = %T %[1]v, want Result generic", resultField.Type)
	}
	if len(gotInner.TypeParams) != 1 {
		t.Fatalf("Result type params = %#v, want one owned binder", gotInner.TypeParams)
	}
	gotInnerParam := gotInner.TypeParams[0]
	if gotInnerParam == outer {
		t.Fatal("nested generic binder was captured by outer binder")
	}
	if gotInnerParam.Name != "U" {
		t.Fatalf("nested generic binder name = %q, want U", gotInnerParam.Name)
	}
	if gotInnerParam.Constraint != typ.String {
		t.Fatalf("nested generic binder constraint = %v, want substituted outer string", gotInnerParam.Constraint)
	}
	gotInnerBody, ok := gotInner.Body.(*typ.Record)
	if !ok {
		t.Fatalf("inner body = %T, want record", gotInner.Body)
	}
	valueField := gotInnerBody.GetField("value")
	if valueField == nil || valueField.Type != gotInnerParam {
		t.Fatalf("Result.value = %v, want owned binder %v", valueField, gotInnerParam)
	}
	outerField := gotInnerBody.GetField("outer")
	if outerField == nil || outerField.Type != typ.String {
		t.Fatalf("Result.outer = %v, want substituted outer string", outerField)
	}
}

func TestRewrite_GenericDescentPreservesInstantiatedResultOwnedBinder(t *testing.T) {
	outer := typ.NewTypeParam("T", nil)
	resultParam := typ.NewTypeParam("U", nil)
	result := typ.NewGeneric("Result", []*typ.TypeParam{resultParam}, newRecord().
		Field("ok", typ.Boolean).
		Field("value", resultParam).
		Build())
	callbackParam := typ.NewTypeParam("U", nil)
	callback := typ.Func().
		TypeParamRef(callbackParam).
		Param("input", outer).
		Returns(typ.Instantiate(result, callbackParam)).
		Build()
	container := newRecord().
		Field("source", outer).
		Field("callback", callback).
		Build()

	got := Rewrite(container, func(node typ.Type) (typ.Type, bool) {
		if node == outer {
			return typ.String, true
		}
		return nil, false
	})
	body, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("rewrite result = %T, want record", got)
	}
	source := body.GetField("source")
	if source == nil || source.Type != typ.String {
		t.Fatalf("source field = %v, want substituted string", source)
	}
	callbackField := body.GetField("callback")
	if callbackField == nil {
		t.Fatal("missing callback field")
	}
	gotCallback, ok := callbackField.Type.(*typ.Function)
	if !ok {
		t.Fatalf("callback field = %T %[1]v, want function", callbackField.Type)
	}
	if len(gotCallback.TypeParams) != 1 || gotCallback.TypeParams[0] != callbackParam {
		t.Fatalf("callback binder changed/captured: %#v, want original U binder", gotCallback.TypeParams)
	}
	if gotCallback.Params[0].Type != typ.String {
		t.Fatalf("callback input = %v, want substituted outer string", gotCallback.Params[0].Type)
	}
	gotReturn, ok := gotCallback.Returns[0].(*typ.Instantiated)
	if !ok {
		t.Fatalf("callback return = %T %[1]v, want Result<U>", gotCallback.Returns[0])
	}
	if gotReturn.Generic != result {
		t.Fatalf("callback return generic = %v, want Result", gotReturn.Generic)
	}
	if len(gotReturn.TypeArgs) != 1 || gotReturn.TypeArgs[0] != callbackParam {
		t.Fatalf("callback return args = %#v, want owned U binder", gotReturn.TypeArgs)
	}
}

func TestRewrite_GenericDescentRewritesOwnedBinderConstraintAndReturnUseTogether(t *testing.T) {
	outer := typ.NewTypeParam("T", nil)
	resultParam := typ.NewTypeParam("U", nil)
	result := typ.NewGeneric("Result", []*typ.TypeParam{resultParam}, newRecord().
		Field("ok", typ.Boolean).
		Field("value", resultParam).
		Build())
	callbackParam := typ.NewTypeParam("U", outer)
	callback := typ.Func().
		TypeParamRef(callbackParam).
		Param("input", callbackParam).
		Returns(typ.Instantiate(result, callbackParam)).
		Build()
	container := newRecord().
		Field("payload", outer).
		Field("callback", callback).
		Build()

	got := Rewrite(container, func(node typ.Type) (typ.Type, bool) {
		if node == outer {
			return typ.String, true
		}
		return nil, false
	})
	body, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("rewrite result = %T, want record", got)
	}
	payload := body.GetField("payload")
	if payload == nil || payload.Type != typ.String {
		t.Fatalf("payload field = %v, want substituted string", payload)
	}
	callbackField := body.GetField("callback")
	if callbackField == nil {
		t.Fatal("missing callback field")
	}
	gotCallback, ok := callbackField.Type.(*typ.Function)
	if !ok {
		t.Fatalf("callback field = %T %[1]v, want function", callbackField.Type)
	}
	if len(gotCallback.TypeParams) != 1 {
		t.Fatalf("callback binders = %#v, want one U binder", gotCallback.TypeParams)
	}
	gotCallbackParam := gotCallback.TypeParams[0]
	if gotCallbackParam == callbackParam || gotCallbackParam == outer {
		t.Fatalf("callback binder = %v, want rebuilt owned U binder", gotCallbackParam)
	}
	if gotCallbackParam.Name != "U" || gotCallbackParam.Constraint != typ.String {
		t.Fatalf("callback binder = %v, want U constrained by substituted string", gotCallbackParam)
	}
	if gotCallback.Params[0].Type != gotCallbackParam {
		t.Fatalf("callback param = %v, want rebuilt U binder %v", gotCallback.Params[0].Type, gotCallbackParam)
	}
	gotReturn, ok := gotCallback.Returns[0].(*typ.Instantiated)
	if !ok {
		t.Fatalf("callback return = %T %[1]v, want Result<U>", gotCallback.Returns[0])
	}
	if gotReturn.Generic != result {
		t.Fatalf("callback return generic = %v, want Result", gotReturn.Generic)
	}
	if len(gotReturn.TypeArgs) != 1 || gotReturn.TypeArgs[0] != gotCallbackParam {
		t.Fatalf("callback return args = %#v, want rebuilt owned U binder", gotReturn.TypeArgs)
	}
}

func TestRewrite_FunctionDescentDoesNotCaptureNestedSameNameBinder(t *testing.T) {
	outer := typ.NewTypeParam("T", nil)
	inner := typ.NewTypeParam("T", nil)
	callback := typ.Func().
		TypeParamRef(inner).
		Param("value", inner).
		Returns(inner).
		Build()
	container := newRecord().
		Field("outer", outer).
		Field("callback", callback).
		Build()

	got := Rewrite(container, func(node typ.Type) (typ.Type, bool) {
		if tp, ok := node.(*typ.TypeParam); ok && tp.Name == "T" {
			return typ.String, true
		}
		return nil, false
	})
	body, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("rewrite result = %T, want record", got)
	}
	outerField := body.GetField("outer")
	if outerField == nil || outerField.Type != typ.String {
		t.Fatalf("outer field = %v, want substituted string", outerField)
	}
	callbackField := body.GetField("callback")
	if callbackField == nil {
		t.Fatal("missing callback field")
	}
	gotCallback, ok := callbackField.Type.(*typ.Function)
	if !ok {
		t.Fatalf("callback field = %T %[1]v, want function", callbackField.Type)
	}
	if len(gotCallback.TypeParams) != 1 || gotCallback.TypeParams[0] != inner {
		t.Fatalf("callback binder changed/captured: %#v, want original inner binder", gotCallback.TypeParams)
	}
	if gotCallback.Params[0].Type != inner {
		t.Fatalf("callback param = %v, want original inner binder", gotCallback.Params[0].Type)
	}
	if gotCallback.Returns[0] != inner {
		t.Fatalf("callback return = %v, want original inner binder", gotCallback.Returns[0])
	}
}

func TestRewrite_FunctionTypeParamConstraint(t *testing.T) {
	tp := typ.NewTypeParam("T", typ.Number)
	fn := typ.Func().TypeParamRef(tp).Param("value", tp).Variadic(tp).Returns(tp).Build()
	result := Rewrite(fn, replaceNumber(typ.String))
	got, ok := result.(*typ.Function)
	if !ok {
		t.Fatalf("expected Function, got %T", result)
	}
	if len(got.TypeParams) != 1 || got.TypeParams[0].Constraint != typ.String {
		t.Fatalf("expected rewritten function type param constraint, got %#v", got.TypeParams)
	}
	param, ok := got.Params[0].Type.(*typ.TypeParam)
	if !ok || param != got.TypeParams[0] || param.Constraint != typ.String {
		t.Fatalf("expected rewritten parameter type param, got %v", got.Params[0].Type)
	}
	variadic, ok := got.Variadic.(*typ.TypeParam)
	if !ok || variadic != got.TypeParams[0] {
		t.Fatalf("expected rewritten variadic type param, got %v", got.Variadic)
	}
	ret, ok := got.Returns[0].(*typ.TypeParam)
	if !ok || ret != got.TypeParams[0] {
		t.Fatalf("expected rewritten return type param, got %v", got.Returns[0])
	}
}

func TestRewrite_Interface(t *testing.T) {
	iface := typ.NewInterface("Readable", []typ.Method{
		{Name: "read", Type: typ.Func().Param("self", typ.Self).Returns(typ.Number).Build()},
	})
	result := Rewrite(iface, replaceNumber(typ.String))
	inf, ok := result.(*typ.Interface)
	if !ok {
		t.Fatalf("expected Interface, got %T", result)
	}
	if inf.Methods[0].Type.Returns[0] != typ.String {
		t.Fatalf("expected return String, got %v", inf.Methods[0].Type.Returns[0])
	}
}

func TestRewrite_RecursiveBody(t *testing.T) {
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return newRecord().
			Field("value", typ.Number).
			Field("next", typeexpr.Optional(self)).
			Build()
	})

	result := Rewrite(node, replaceNumber(typ.String))
	got, ok := result.(*typ.Recursive)
	if !ok {
		t.Fatalf("expected Recursive, got %T", result)
	}
	if got == node {
		t.Fatal("expected recursive rewrite to create a new node")
	}
	body, ok := got.Body.(*typ.Record)
	if !ok {
		t.Fatalf("expected recursive body record, got %T", got.Body)
	}
	value := body.GetField("value")
	if value == nil || value.Type != typ.String {
		t.Fatalf("expected rewritten value field, got %v", value)
	}
	next := body.GetField("next")
	opt, ok := next.Type.(*typ.Optional)
	if next == nil || !ok || opt.Inner != got {
		t.Fatalf("expected self-reference to point at rewritten recursive node, got %v", next)
	}
}

func TestRewrite_SelfSubstitution(t *testing.T) {
	rec := newRecord().Field("x", typ.Number).Build()
	fn := typ.Func().Param("self", typ.Self).Returns(typ.Self).Build()
	result := Rewrite(fn, replaceSelf(rec))
	f, ok := result.(*typ.Function)
	if !ok {
		t.Fatalf("expected Function, got %T", result)
	}
	if f.Params[0].Type != rec {
		t.Fatalf("expected param to be record, got %v", f.Params[0].Type)
	}
	if f.Returns[0] != rec {
		t.Fatalf("expected return to be record, got %v", f.Returns[0])
	}
}

func TestRewrite_SelfInOptional(t *testing.T) {
	opt := typeexpr.Optional(typ.Self)
	result := Rewrite(opt, replaceSelf(typ.Number))
	o, ok := result.(*typ.Optional)
	if !ok {
		t.Fatalf("expected Optional, got %T", result)
	}
	if o.Inner != typ.Number {
		t.Fatalf("expected inner Number, got %v", o.Inner)
	}
}

func TestRewrite_SelfInUnion(t *testing.T) {
	u := typeexpr.Union(typ.Self, typ.Boolean)
	result := Rewrite(u, replaceSelf(typ.Number))
	union, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("expected Union, got %T", result)
	}
	found := false
	for _, m := range union.Members {
		if m == typ.Number {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected Number in union, got %v", result)
	}
}

func TestRewrite_EarlyReplacementStopsRecursion(t *testing.T) {
	// Replace the entire array, not its element
	arr := typ.NewArray(typ.Number)
	result := Rewrite(arr, func(t typ.Type) (typ.Type, bool) {
		if _, ok := t.(*typ.Array); ok {
			return typ.String, true
		}
		return nil, false
	})
	if result != typ.String {
		t.Fatalf("expected String (whole array replaced), got %v", result)
	}
}

func TestRewrite_NoOpReturnsSamePointer(t *testing.T) {
	arr := typ.NewArray(typ.String)
	result := Rewrite(arr, replaceNumber(typ.Boolean))
	if result != arr {
		t.Fatal("expected same pointer when nothing matched")
	}
}

func TestRewrite_DepthLimit(t *testing.T) {
	deep := typ.Number
	for i := 0; i < 100; i++ {
		deep = typeexpr.Optional(deep)
	}
	result := Rewrite(deep, replaceNumber(typ.String))
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}
