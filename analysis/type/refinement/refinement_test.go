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
