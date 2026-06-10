package refinement

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestContainsFreeTypeParamTreatsClosedInstantiationAsClosed(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{tp}, typ.NewRecord().Field("value", tp).Build())

	if ContainsFreeTypeParam(typ.Instantiate(box, typ.String)) {
		t.Fatal("closed Box<string> should not report a free type parameter")
	}
	if !ContainsFreeTypeParam(typ.Instantiate(box, tp)) {
		t.Fatal("Box<T> with symbolic argument should report a free type parameter")
	}
}

func TestContainsFreeTypeParamKeepsFunctionOwnedParamsScoped(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	fn := typ.Func().TypeParamRef(tp).Returns(tp).Build()
	closedFunctionWithFreeSibling := typ.NewRecord().
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
	clean := typ.NewRecord().
		Field("value", typ.LiteralString("hello")).
		Field("get", typ.Func().OptParam("self", typ.Any).Returns(typ.String).Build()).
		Build()
	if NeedsSameExpressionFallback(clean) {
		t.Fatalf("clean record reported fallback need: %v", clean)
	}

	open := typ.NewRecord().
		Field("value", typ.LiteralString("hello")).
		Field("get", typ.Func().OptParam("self", typ.Any).Returns(typ.NewRef("", "T")).Build()).
		Build()
	if !NeedsSameExpressionFallback(open) {
		t.Fatalf("open record did not report fallback need: %v", open)
	}
}

func TestNeedsSameExpressionFallbackUsesRecursiveFamilySeen(t *testing.T) {
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("next", typ.NewOptional(self)).
			Field("get", typ.Func().OptParam("self", typ.Any).Returns(self).Build()).
			Build()
	})
	surface := typ.NewRecord().
		Field("left", node).
		Field("right", node).
		Build()

	if NeedsSameExpressionFallback(surface) {
		t.Fatalf("closed recursive surface reported fallback need: %v", surface)
	}

	open := typ.NewRecursive("OpenNode", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("next", typ.NewOptional(self)).
			Field("get", typ.Func().OptParam("self", typ.Any).Returns(typ.NewRef("", "T")).Build()).
			Build()
	})
	if !NeedsSameExpressionFallback(open) {
		t.Fatalf("recursive surface with open return did not report fallback need: %v", open)
	}
}

func TestNeedsSameExpressionFallbackUsesRecursiveAliasFamilySeen(t *testing.T) {
	node := typ.NewRecursivePlaceholder("Node")
	var tower typ.Type = typ.NewAlias("NodeAlias", typ.NewRecord().
		Field("next", node).
		Build())
	for i := 0; i < 512; i++ {
		tower = typ.NewAlias("TowerAlias", typ.NewMap(typ.String, typ.NewOptional(typ.NewUnion(tower, typ.Nil))))
	}
	node.SetBody(typ.NewRecord().
		Field("next", tower).
		Field("hole", typ.Unknown).
		Build())

	if !NeedsSameExpressionFallback(tower) {
		t.Fatal("recursive alias family with an unknown leaf should need fallback")
	}
}

func TestNeedsSameExpressionFallbackWithinReportsIncomplete(t *testing.T) {
	var tower typ.Type = typ.String
	for i := 0; i < 64; i++ {
		tower = typ.NewMap(typ.String, typ.NewOptional(tower))
	}

	needs, complete := NeedsSameExpressionFallbackWithin(tower, 8)
	if needs || complete {
		t.Fatalf("bounded fallback scan = needs %v complete %v, want false/false", needs, complete)
	}
}

func TestRefineWithFallbackRepairsTypeParamLeafAndKeepsLiteral(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	summary := typ.NewRecord().
		Field("value", typ.LiteralString("hello")).
		Field("get", typ.Func().OptParam("self", typ.Any).Returns(tp).Build()).
		Build()
	fallback := typ.NewRecord().
		Field("value", typ.String).
		Field("get", typ.Func().OptParam("self", typ.Self).Returns(typ.String).Build()).
		Build()

	refined, changed := RefineWithFallback(summary, fallback, nil)
	if !changed {
		t.Fatal("RefineWithFallback did not repair open type-param leaf")
	}
	rec, ok := refined.(*typ.Record)
	if !ok {
		t.Fatalf("refined = %T, want record", refined)
	}
	value := rec.GetField("value")
	if value == nil || !typ.TypeEquals(value.Type, typ.LiteralString("hello")) {
		t.Fatalf("value field = %#v, want literal hello", value)
	}
	get := rec.GetField("get")
	if get == nil {
		t.Fatal("missing get field")
	}
	fn, ok := get.Type.(*typ.Function)
	if !ok || len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], typ.String) {
		t.Fatalf("get field = %#v, want function returning string", get)
	}
}

func TestRefineWithFallbackDoesNotReplaceWholeConcreteLeaf(t *testing.T) {
	refined, changed := RefineWithFallback(typ.String, typ.LiteralString("signature-only"), nil)
	if changed {
		t.Fatalf("RefineWithFallback changed concrete summary leaf to %v", refined)
	}
	if !typ.TypeEquals(refined, typ.String) {
		t.Fatalf("refined = %v, want original string summary", refined)
	}
}

func TestRefineWithFallbackKeepsConcreteArrayElementOverEmptyFallback(t *testing.T) {
	summaryRecord := typ.NewRecord().
		Field("node_order", typ.NewArray(typ.String)).
		Build()
	fallbackRecord := typ.NewRecord().
		Field("node_order", typ.NewArray(typ.Never)).
		Build()
	summary := typ.NewUnion(typ.Nil, summaryRecord)
	fallback := typ.NewUnion(typ.Nil, fallbackRecord)

	refined, changed := RefineWithFallback(summary, fallback, nil)
	if changed {
		t.Fatalf("RefineWithFallback replaced concrete array element with empty fallback: %v", refined)
	}
	if !typ.TypeEquals(refined, summary) {
		t.Fatalf("refined = %v, want %v", refined, summary)
	}
}

func TestRefineWithFallbackRepairsFunctionReturnDespiteParamShapeMismatch(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	summary := typ.Func().
		OptParam("self", typ.Any).
		Returns(tp).
		Build()
	fallback := typ.Func().
		Param("self", typ.NewRecord().Field("value", typ.String).Build()).
		Returns(typ.String).
		Build()

	refined, changed := RefineWithFallback(summary, fallback, nil)
	if !changed {
		t.Fatal("RefineWithFallback did not repair covariant return")
	}
	fn, ok := refined.(*typ.Function)
	if !ok {
		t.Fatalf("refined = %T, want function", refined)
	}
	if len(fn.Params) != 1 || !fn.Params[0].Optional || !typ.TypeEquals(fn.Params[0].Type, typ.Any) {
		t.Fatalf("param = %#v, want original optional any parameter", fn.Params)
	}
	if len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], typ.String) {
		t.Fatalf("returns = %#v, want string", fn.Returns)
	}
}

func TestRefineWithFallbackRepairsFunctionReturnWithInstantiatedSelfParam(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	container := typ.NewGeneric("Container", []*typ.TypeParam{tp},
		typ.NewRecord().
			Field("value", tp).
			Field("get", typ.Func().Param("self", typ.Instantiate(typ.NewGeneric("Container", []*typ.TypeParam{tp}, typ.NewRecord().Build()), tp)).Returns(tp).Build()).
			Build(),
	)
	summary := typ.NewRecord().
		Field("value", typ.LiteralString("hello")).
		Field("get", typ.Func().OptParam("self", typ.Any).Returns(tp).Build()).
		Build()
	fallback := typ.NewRecord().
		Field("value", typ.String).
		Field("get", typ.Func().Param("self", typ.Instantiate(container, typ.String)).Returns(typ.String).Build()).
		Build()

	refined, changed := RefineWithFallback(summary, fallback, nil)
	if !changed {
		t.Fatal("RefineWithFallback did not repair function return with instantiated self fallback")
	}
	rec, ok := refined.(*typ.Record)
	if !ok {
		t.Fatalf("refined = %T, want record", refined)
	}
	get := rec.GetField("get")
	fn, ok := get.Type.(*typ.Function)
	if get == nil || !ok || len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], typ.String) {
		t.Fatalf("get field = %#v, want function returning string", get)
	}
}

func TestRefineWithFallbackRepairsDeferredRefLeaf(t *testing.T) {
	summary := typ.Func().
		OptParam("self", typ.Any).
		Returns(typ.NewRef("", "T")).
		Build()
	fallback := typ.Func().
		Param("self", typ.NewRecord().Field("value", typ.String).Build()).
		Returns(typ.String).
		Build()

	refined, changed := RefineWithFallback(summary, fallback, nil)
	if !changed {
		t.Fatal("RefineWithFallback did not repair deferred ref leaf")
	}
	fn, ok := refined.(*typ.Function)
	if !ok || len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], typ.String) {
		t.Fatalf("refined = %v, want function returning string", refined)
	}
}

func TestRefineWithFallbackPreservesFunctionOwnedTypeParam(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	summary := typ.Func().TypeParamRef(tp).Param("value", tp).Returns(tp).Build()
	fallback := typ.Func().Param("value", typ.String).Returns(typ.String).Build()

	refined, changed := RefineWithFallback(summary, fallback, nil)
	if changed || refined != summary {
		t.Fatalf("owned type param should not be repaired: %v changed=%v", refined, changed)
	}
}
