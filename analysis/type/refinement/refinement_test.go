package refinement

import (
	"testing"

	. "github.com/wippyai/go-lua/analysis/type/typ"
)

func TestContainsFreeTypeParamTreatsClosedInstantiationAsClosed(t *testing.T) {
	tp := NewTypeParam("T", nil)
	box := NewGeneric("Box", []*TypeParam{tp}, NewRecord().Field("value", tp).Build())

	if ContainsFreeTypeParam(Instantiate(box, String)) {
		t.Fatal("closed Box<string> should not report a free type parameter")
	}
	if !ContainsFreeTypeParam(Instantiate(box, tp)) {
		t.Fatal("Box<T> with symbolic argument should report a free type parameter")
	}
}

func TestContainsFreeTypeParamKeepsFunctionOwnedParamsScoped(t *testing.T) {
	tp := NewTypeParam("T", nil)
	fn := Func().TypeParamRef(tp).Returns(tp).Build()
	closedFunctionWithFreeSibling := NewRecord().
		Field("call", fn).
		Field("value", tp).
		Build()

	if !ContainsFreeTypeParam(closedFunctionWithFreeSibling) {
		t.Fatal("free sibling type parameter was hidden by function-owned parameter scan")
	}
	if ContainsFreeTypeParam(fn) {
		t.Fatal("function-owned type parameter was reported as free")
	}
}

func TestNeedsSameExpressionFallbackFindsNestedRepairableLeaves(t *testing.T) {
	clean := NewRecord().
		Field("value", LiteralString("hello")).
		Field("get", Func().OptParam("self", Any).Returns(String).Build()).
		Build()
	if NeedsSameExpressionFallback(clean) {
		t.Fatalf("clean record reported fallback need: %v", clean)
	}

	open := NewRecord().
		Field("value", LiteralString("hello")).
		Field("get", Func().OptParam("self", Any).Returns(NewRef("", "T")).Build()).
		Build()
	if !NeedsSameExpressionFallback(open) {
		t.Fatalf("open record did not report fallback need: %v", open)
	}
}

func TestNeedsSameExpressionFallbackUsesRecursiveFamilySeen(t *testing.T) {
	node := NewRecursive("Node", func(self Type) Type {
		return NewRecord().
			Field("next", NewOptional(self)).
			Field("get", Func().OptParam("self", Any).Returns(self).Build()).
			Build()
	})
	surface := NewRecord().
		Field("left", node).
		Field("right", node).
		Build()

	if NeedsSameExpressionFallback(surface) {
		t.Fatalf("closed recursive surface reported fallback need: %v", surface)
	}

	open := NewRecursive("OpenNode", func(self Type) Type {
		return NewRecord().
			Field("next", NewOptional(self)).
			Field("get", Func().OptParam("self", Any).Returns(NewRef("", "T")).Build()).
			Build()
	})
	if !NeedsSameExpressionFallback(open) {
		t.Fatalf("recursive surface with open return did not report fallback need: %v", open)
	}
}

func TestNeedsSameExpressionFallbackUsesRecursiveAliasFamilySeen(t *testing.T) {
	node := NewRecursivePlaceholder("Node")
	var tower Type = NewAlias("NodeAlias", NewRecord().
		Field("next", node).
		Build())
	for i := 0; i < 512; i++ {
		tower = NewAlias("TowerAlias", NewMap(String, NewOptional(NewUnion(tower, Nil))))
	}
	node.SetBody(NewRecord().
		Field("next", tower).
		Field("hole", Unknown).
		Build())

	if !NeedsSameExpressionFallback(tower) {
		t.Fatal("recursive alias family with an unknown leaf should need fallback")
	}
}

func TestNeedsSameExpressionFallbackWithinReportsIncomplete(t *testing.T) {
	var tower Type = String
	for i := 0; i < 64; i++ {
		tower = NewMap(String, NewOptional(tower))
	}

	needs, complete := NeedsSameExpressionFallbackWithin(tower, 8)
	if needs || complete {
		t.Fatalf("bounded fallback scan = needs %v complete %v, want false/false", needs, complete)
	}
}

func TestRefineWithFallbackRepairsTypeParamLeafAndKeepsLiteral(t *testing.T) {
	tp := NewTypeParam("T", nil)
	summary := NewRecord().
		Field("value", LiteralString("hello")).
		Field("get", Func().OptParam("self", Any).Returns(tp).Build()).
		Build()
	fallback := NewRecord().
		Field("value", String).
		Field("get", Func().OptParam("self", Self).Returns(String).Build()).
		Build()

	refined, changed := RefineWithFallback(summary, fallback, nil)
	if !changed {
		t.Fatal("RefineWithFallback did not repair open type-param leaf")
	}
	rec, ok := refined.(*Record)
	if !ok {
		t.Fatalf("refined = %T, want record", refined)
	}
	value := rec.GetField("value")
	if value == nil || !TypeEquals(value.Type, LiteralString("hello")) {
		t.Fatalf("value field = %#v, want literal hello", value)
	}
	get := rec.GetField("get")
	if get == nil {
		t.Fatal("missing get field")
	}
	fn, ok := get.Type.(*Function)
	if !ok || len(fn.Returns) != 1 || !TypeEquals(fn.Returns[0], String) {
		t.Fatalf("get field = %#v, want function returning string", get)
	}
}

func TestRefineWithFallbackDoesNotReplaceWholeConcreteLeaf(t *testing.T) {
	refined, changed := RefineWithFallback(String, LiteralString("signature-only"), nil)
	if changed {
		t.Fatalf("RefineWithFallback changed concrete summary leaf to %v", refined)
	}
	if !TypeEquals(refined, String) {
		t.Fatalf("refined = %v, want original string summary", refined)
	}
}

func TestRefineWithFallbackKeepsConcreteArrayElementOverEmptyFallback(t *testing.T) {
	summaryRecord := NewRecord().
		Field("node_order", NewArray(String)).
		Build()
	fallbackRecord := NewRecord().
		Field("node_order", NewArray(Never)).
		Build()
	summary := NewUnion(Nil, summaryRecord)
	fallback := NewUnion(Nil, fallbackRecord)

	refined, changed := RefineWithFallback(summary, fallback, nil)
	if changed {
		t.Fatalf("RefineWithFallback replaced concrete array element with empty fallback: %v", refined)
	}
	if !TypeEquals(refined, summary) {
		t.Fatalf("refined = %v, want %v", refined, summary)
	}
}

func TestRefineWithFallbackRepairsFunctionReturnDespiteParamShapeMismatch(t *testing.T) {
	tp := NewTypeParam("T", nil)
	summary := Func().
		OptParam("self", Any).
		Returns(tp).
		Build()
	fallback := Func().
		Param("self", NewRecord().Field("value", String).Build()).
		Returns(String).
		Build()

	refined, changed := RefineWithFallback(summary, fallback, nil)
	if !changed {
		t.Fatal("RefineWithFallback did not repair covariant return")
	}
	fn, ok := refined.(*Function)
	if !ok {
		t.Fatalf("refined = %T, want function", refined)
	}
	if len(fn.Params) != 1 || !fn.Params[0].Optional || !TypeEquals(fn.Params[0].Type, Any) {
		t.Fatalf("param = %#v, want original optional any parameter", fn.Params)
	}
	if len(fn.Returns) != 1 || !TypeEquals(fn.Returns[0], String) {
		t.Fatalf("returns = %#v, want string", fn.Returns)
	}
}

func TestRefineWithFallbackRepairsFunctionReturnWithInstantiatedSelfParam(t *testing.T) {
	tp := NewTypeParam("T", nil)
	container := NewGeneric("Container", []*TypeParam{tp},
		NewRecord().
			Field("value", tp).
			Field("get", Func().Param("self", Instantiate(NewGeneric("Container", []*TypeParam{tp}, NewRecord().Build()), tp)).Returns(tp).Build()).
			Build(),
	)
	summary := NewRecord().
		Field("value", LiteralString("hello")).
		Field("get", Func().OptParam("self", Any).Returns(tp).Build()).
		Build()
	fallback := NewRecord().
		Field("value", String).
		Field("get", Func().Param("self", Instantiate(container, String)).Returns(String).Build()).
		Build()

	refined, changed := RefineWithFallback(summary, fallback, nil)
	if !changed {
		t.Fatal("RefineWithFallback did not repair function return with instantiated self fallback")
	}
	rec, ok := refined.(*Record)
	if !ok {
		t.Fatalf("refined = %T, want record", refined)
	}
	get := rec.GetField("get")
	fn, ok := get.Type.(*Function)
	if get == nil || !ok || len(fn.Returns) != 1 || !TypeEquals(fn.Returns[0], String) {
		t.Fatalf("get field = %#v, want function returning string", get)
	}
}

func TestRefineWithFallbackRepairsDeferredRefLeaf(t *testing.T) {
	summary := Func().
		OptParam("self", Any).
		Returns(NewRef("", "T")).
		Build()
	fallback := Func().
		Param("self", NewRecord().Field("value", String).Build()).
		Returns(String).
		Build()

	refined, changed := RefineWithFallback(summary, fallback, nil)
	if !changed {
		t.Fatal("RefineWithFallback did not repair deferred ref leaf")
	}
	fn, ok := refined.(*Function)
	if !ok || len(fn.Returns) != 1 || !TypeEquals(fn.Returns[0], String) {
		t.Fatalf("refined = %v, want function returning string", refined)
	}
}

func TestRefineWithFallbackPreservesFunctionOwnedTypeParam(t *testing.T) {
	tp := NewTypeParam("T", nil)
	summary := Func().TypeParamRef(tp).Param("value", tp).Returns(tp).Build()
	fallback := Func().Param("value", String).Returns(String).Build()

	refined, changed := RefineWithFallback(summary, fallback, nil)
	if changed || refined != summary {
		t.Fatalf("owned type param should not be repaired: %v changed=%v", refined, changed)
	}
}
