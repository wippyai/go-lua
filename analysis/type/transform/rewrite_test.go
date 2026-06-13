package transform

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
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

func (b *testRecordBuilder) ReadonlyField(name string, t typ.Type) *testRecordBuilder {
	b.parts.Fields = append(b.parts.Fields, typ.Field{Name: name, Type: t, Readonly: true})
	return b
}

func (b *testRecordBuilder) Metatable(t typ.Type) *testRecordBuilder {
	b.parts.Metatable = t
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
	opt := typ.NewOptional(typ.Number)
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
	u := typ.NewUnion(typ.Number, typ.Boolean)
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

func TestRewrite_Intersection(t *testing.T) {
	rec := newRecord().Field("x", typ.Boolean).Build()
	inter := typ.NewIntersection(typ.Number, rec)
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

func TestRewrite_FunctionTypeParamConstraint(t *testing.T) {
	tp := typ.NewTypeParam("T", typ.Number)
	fn := typ.Func().TypeParamRef(tp).Param("value", tp).Build()
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
			Field("next", typ.NewOptional(self)).
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
	opt := typ.NewOptional(typ.Self)
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
	u := typ.NewUnion(typ.Self, typ.Boolean)
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
		deep = typ.NewOptional(deep)
	}
	result := Rewrite(deep, replaceNumber(typ.String))
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}
