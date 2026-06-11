package typ

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/kind"
)

// replaceSelf is a test helper that replaces Self with a given type.
func replaceSelf(selfType Type) func(Type) (Type, bool) {
	return func(t Type) (Type, bool) {
		if t.Kind() == kind.Self {
			return selfType, true
		}
		return nil, false
	}
}

// replaceNumber replaces Number with a given type.
func replaceNumber(replacement Type) func(Type) (Type, bool) {
	return func(t Type) (Type, bool) {
		if t == Number {
			return replacement, true
		}
		return nil, false
	}
}

func TestRewrite_Nil(t *testing.T) {
	result := Rewrite(nil, replaceNumber(String))
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
}

func TestRewrite_NoMatch(t *testing.T) {
	result := Rewrite(Boolean, replaceNumber(String))
	if result != Boolean {
		t.Fatalf("expected Boolean unchanged, got %v", result)
	}
}

func TestRewrite_DirectReplacement(t *testing.T) {
	result := Rewrite(Number, replaceNumber(String))
	if result != String {
		t.Fatalf("expected String, got %v", result)
	}
}

func TestRewrite_Optional(t *testing.T) {
	opt := NewOptional(Number)
	result := Rewrite(opt, replaceNumber(String))
	o, ok := result.(*Optional)
	if !ok {
		t.Fatalf("expected Optional, got %T", result)
	}
	if o.Inner != String {
		t.Fatalf("expected inner String, got %v", o.Inner)
	}
}

func TestRewrite_Union(t *testing.T) {
	u := NewUnion(Number, Boolean)
	result := Rewrite(u, replaceNumber(String))
	union, ok := result.(*Union)
	if !ok {
		t.Fatalf("expected Union, got %T", result)
	}
	found := false
	for _, m := range union.Members {
		if m == String {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected String in union, got %v", result)
	}
}

func TestRewrite_Intersection(t *testing.T) {
	rec := newRecord().Field("x", Boolean).Build()
	inter := NewIntersection(Number, rec)
	result := Rewrite(inter, replaceNumber(String))
	intersection, ok := result.(*Intersection)
	if !ok {
		t.Fatalf("expected Intersection, got %T", result)
	}
	found := false
	for _, m := range intersection.Members {
		if m == String {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected String in intersection, got %v", result)
	}
}

func TestRewrite_Array(t *testing.T) {
	arr := NewArray(Number)
	result := Rewrite(arr, replaceNumber(String))
	a, ok := result.(*Array)
	if !ok {
		t.Fatalf("expected Array, got %T", result)
	}
	if a.Element != String {
		t.Fatalf("expected element String, got %v", a.Element)
	}
}

func TestRewrite_Map(t *testing.T) {
	m := NewMap(String, Number)
	result := Rewrite(m, replaceNumber(Integer))
	mp, ok := result.(*Map)
	if !ok {
		t.Fatalf("expected Map, got %T", result)
	}
	if mp.Key != String {
		t.Fatalf("expected key String, got %v", mp.Key)
	}
	if mp.Value != Integer {
		t.Fatalf("expected value Integer, got %v", mp.Value)
	}
}

func TestRewrite_Tuple(t *testing.T) {
	tup := NewTuple(Number, Boolean)
	result := Rewrite(tup, replaceNumber(String))
	tuple, ok := result.(*Tuple)
	if !ok {
		t.Fatalf("expected Tuple, got %T", result)
	}
	if tuple.Elements[0] != String {
		t.Fatalf("expected first element String, got %v", tuple.Elements[0])
	}
	if tuple.Elements[1] != Boolean {
		t.Fatalf("expected second element Boolean, got %v", tuple.Elements[1])
	}
}

func TestRewrite_Function(t *testing.T) {
	fn := Func().Param("a", Number).Returns(Number).Build()
	result := Rewrite(fn, replaceNumber(String))
	f, ok := result.(*Function)
	if !ok {
		t.Fatalf("expected Function, got %T", result)
	}
	if f.Params[0].Type != String {
		t.Fatalf("expected param String, got %v", f.Params[0].Type)
	}
	if f.Returns[0] != String {
		t.Fatalf("expected return String, got %v", f.Returns[0])
	}
}

func TestRewrite_FunctionVariadic(t *testing.T) {
	fn := Func().Variadic(Number).Returns(Boolean).Build()
	result := Rewrite(fn, replaceNumber(String))
	f, ok := result.(*Function)
	if !ok {
		t.Fatalf("expected Function, got %T", result)
	}
	if f.Variadic != String {
		t.Fatalf("expected variadic String, got %v", f.Variadic)
	}
}

func TestRewrite_FunctionPreservesOptionalParam(t *testing.T) {
	fn := Func().OptParam("a", Number).Returns(Boolean).Build()
	result := Rewrite(fn, replaceNumber(String))
	f, ok := result.(*Function)
	if !ok {
		t.Fatalf("expected Function, got %T", result)
	}
	if !f.Params[0].Optional {
		t.Fatal("expected param to remain optional")
	}
}

func TestRewrite_Record(t *testing.T) {
	rec := newRecord().Field("x", Number).OptField("y", Number).Build()
	result := Rewrite(rec, replaceNumber(String))
	r, ok := result.(*Record)
	if !ok {
		t.Fatalf("expected Record, got %T", result)
	}
	x := r.GetField("x")
	if x == nil || x.Type != String {
		t.Fatalf("expected x String, got %v", x)
	}
	y := r.GetField("y")
	if y == nil || y.Type != String || !y.Optional {
		t.Fatalf("expected y optional String, got %v", y)
	}
}

func TestRewrite_RecordReadonly(t *testing.T) {
	rec := newRecord().ReadonlyField("id", Number).Build()
	result := Rewrite(rec, replaceNumber(String))
	r, ok := result.(*Record)
	if !ok {
		t.Fatalf("expected Record, got %T", result)
	}
	f := r.GetField("id")
	if f == nil || f.Type != String || !f.Readonly {
		t.Fatalf("expected readonly String, got %v", f)
	}
}

func TestRewrite_RecordMetatable(t *testing.T) {
	mt := newRecord().Field("__index", Number).Build()
	rec := newRecord().Field("x", Boolean).Metatable(mt).Build()
	result := Rewrite(rec, replaceNumber(String))
	r, ok := result.(*Record)
	if !ok {
		t.Fatalf("expected Record, got %T", result)
	}
	if r.Metatable == nil {
		t.Fatal("expected metatable preserved")
	}
	mtRec := r.Metatable.(*Record)
	f := mtRec.GetField("__index")
	if f == nil || f.Type != String {
		t.Fatalf("expected metatable __index String, got %v", f)
	}
}

func TestRewrite_Alias(t *testing.T) {
	alias := NewAlias("Num", Number)
	result := Rewrite(alias, replaceNumber(String))
	a, ok := result.(*Alias)
	if !ok {
		t.Fatalf("expected Alias, got %T", result)
	}
	if a.Name != "Num" || a.Target != String {
		t.Fatalf("expected Alias Num -> String, got %v -> %v", a.Name, a.Target)
	}
}

func TestRewrite_Instantiated(t *testing.T) {
	tp := NewTypeParam("T", nil)
	g := NewGeneric("Box", []*TypeParam{tp}, NewArray(tp))
	inst := Instantiate(g, Number)
	result := Rewrite(inst, replaceNumber(String))
	i, ok := result.(*Instantiated)
	if !ok {
		t.Fatalf("expected Instantiated, got %T", result)
	}
	if i.TypeArgs[0] != String {
		t.Fatalf("expected type arg String, got %v", i.TypeArgs[0])
	}
}

func TestRewrite_Meta(t *testing.T) {
	meta := NewMeta(Number)
	result := Rewrite(meta, replaceNumber(String))
	got, ok := result.(*Meta)
	if !ok {
		t.Fatalf("expected Meta, got %T", result)
	}
	if got.Of != String {
		t.Fatalf("expected meta payload String, got %v", got.Of)
	}
}

func TestRewrite_TypeParamConstraint(t *testing.T) {
	tp := NewTypeParam("T", Number)
	result := Rewrite(tp, replaceNumber(String))
	got, ok := result.(*TypeParam)
	if !ok {
		t.Fatalf("expected TypeParam, got %T", result)
	}
	if got.Constraint != String {
		t.Fatalf("expected constraint String, got %v", got.Constraint)
	}
}

func TestRewrite_GenericBodyAndTypeParamConstraint(t *testing.T) {
	tp := NewTypeParam("T", Number)
	g := NewGeneric("Box", []*TypeParam{tp}, newRecord().Field("value", tp).Build())

	result := Rewrite(g, replaceNumber(String))
	got, ok := result.(*Generic)
	if !ok {
		t.Fatalf("expected Generic, got %T", result)
	}
	if len(got.TypeParams) != 1 || got.TypeParams[0].Constraint != String {
		t.Fatalf("expected rewritten type param constraint, got %#v", got.TypeParams)
	}
	body, ok := got.Body.(*Record)
	if !ok {
		t.Fatalf("expected generic body record, got %T", got.Body)
	}
	field := body.GetField("value")
	if field == nil || field.Type != got.TypeParams[0] {
		t.Fatalf("expected body to reference rewritten type param, got %v", field)
	}
}

func TestRewrite_FunctionTypeParamConstraint(t *testing.T) {
	tp := NewTypeParam("T", Number)
	fn := Func().TypeParamRef(tp).Param("value", tp).Build()
	result := Rewrite(fn, replaceNumber(String))
	got, ok := result.(*Function)
	if !ok {
		t.Fatalf("expected Function, got %T", result)
	}
	if len(got.TypeParams) != 1 || got.TypeParams[0].Constraint != String {
		t.Fatalf("expected rewritten function type param constraint, got %#v", got.TypeParams)
	}
	param, ok := got.Params[0].Type.(*TypeParam)
	if !ok || param != got.TypeParams[0] || param.Constraint != String {
		t.Fatalf("expected rewritten parameter type param, got %v", got.Params[0].Type)
	}
}

func TestRewrite_Interface(t *testing.T) {
	iface := NewInterface("Readable", []Method{
		{Name: "read", Type: Func().Param("self", Self).Returns(Number).Build()},
	})
	result := Rewrite(iface, replaceNumber(String))
	inf, ok := result.(*Interface)
	if !ok {
		t.Fatalf("expected Interface, got %T", result)
	}
	if inf.Methods[0].Type.Returns[0] != String {
		t.Fatalf("expected return String, got %v", inf.Methods[0].Type.Returns[0])
	}
}

func TestRewrite_RecursiveBody(t *testing.T) {
	node := NewRecursive("Node", func(self Type) Type {
		return newRecord().
			Field("value", Number).
			Field("next", NewOptional(self)).
			Build()
	})

	result := Rewrite(node, replaceNumber(String))
	got, ok := result.(*Recursive)
	if !ok {
		t.Fatalf("expected Recursive, got %T", result)
	}
	if got == node {
		t.Fatal("expected recursive rewrite to create a new node")
	}
	body, ok := got.Body.(*Record)
	if !ok {
		t.Fatalf("expected recursive body record, got %T", got.Body)
	}
	value := body.GetField("value")
	if value == nil || value.Type != String {
		t.Fatalf("expected rewritten value field, got %v", value)
	}
	next := body.GetField("next")
	opt, ok := next.Type.(*Optional)
	if next == nil || !ok || opt.Inner != got {
		t.Fatalf("expected self-reference to point at rewritten recursive node, got %v", next)
	}
}

func TestRewrite_SelfSubstitution(t *testing.T) {
	rec := newRecord().Field("x", Number).Build()
	fn := Func().Param("self", Self).Returns(Self).Build()
	result := Rewrite(fn, replaceSelf(rec))
	f, ok := result.(*Function)
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
	opt := NewOptional(Self)
	result := Rewrite(opt, replaceSelf(Number))
	o, ok := result.(*Optional)
	if !ok {
		t.Fatalf("expected Optional, got %T", result)
	}
	if o.Inner != Number {
		t.Fatalf("expected inner Number, got %v", o.Inner)
	}
}

func TestRewrite_SelfInUnion(t *testing.T) {
	u := NewUnion(Self, Boolean)
	result := Rewrite(u, replaceSelf(Number))
	union, ok := result.(*Union)
	if !ok {
		t.Fatalf("expected Union, got %T", result)
	}
	found := false
	for _, m := range union.Members {
		if m == Number {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected Number in union, got %v", result)
	}
}

func TestRewrite_EarlyReplacementStopsRecursion(t *testing.T) {
	// Replace the entire array, not its element
	arr := NewArray(Number)
	result := Rewrite(arr, func(t Type) (Type, bool) {
		if _, ok := t.(*Array); ok {
			return String, true
		}
		return nil, false
	})
	if result != String {
		t.Fatalf("expected String (whole array replaced), got %v", result)
	}
}

func TestRewrite_NoOpReturnsSamePointer(t *testing.T) {
	arr := NewArray(String)
	result := Rewrite(arr, replaceNumber(Boolean))
	if result != arr {
		t.Fatal("expected same pointer when nothing matched")
	}
}

func TestRewrite_DepthLimit(t *testing.T) {
	deep := Number
	for i := 0; i < 100; i++ {
		deep = NewOptional(deep)
	}
	result := Rewrite(deep, replaceNumber(String))
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}
