package value

import (
	"testing"

	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/typ"
)

func TestFactTypeEqual_IncludesNestedFunctionSpec(t *testing.T) {
	callback := typ.Func().Param("value", typ.String).Build()
	spec := contract.NewSpec().WithCallback(0, (&contract.CallbackSpec{}).WithEnvOverlay(map[string]typ.Type{
		"run": callback,
	}))
	withoutSpec := typ.NewRecord().
		Field("define", typ.Func().Param("fn", callback).Build()).
		Build()
	withSpec := typ.NewRecord().
		Field("define", typ.Func().Param("fn", callback).Spec(spec).Build()).
		Build()

	if !typ.TypeEquals(withoutSpec, withSpec) {
		t.Fatal("ordinary structural equality should ignore function specs")
	}
	if FactTypeEqual(withoutSpec, withSpec) {
		t.Fatal("fact equality must include function specs even when nested")
	}
}

func TestFactTypeEqual_IncludesFunctionSpecThroughAnnotation(t *testing.T) {
	spec := contract.NewSpec().WithCallback(0, &contract.CallbackSpec{Cardinality: contract.CardExactlyOnce})
	withoutSpec := typ.NewAnnotated(typ.Func().Param("fn", typ.Func().Build()).Build(), []typ.Annotation{{Name: "checked"}})
	withSpec := typ.NewAnnotated(typ.Func().Param("fn", typ.Func().Build()).Spec(spec).Build(), []typ.Annotation{{Name: "checked"}})

	if !typ.TypeEquals(withoutSpec, withSpec) {
		t.Fatal("ordinary structural equality should ignore function specs through annotations")
	}
	if FactTypeEqual(withoutSpec, withSpec) {
		t.Fatal("fact equality must include function specs through annotations")
	}
}

func TestFactTypeEqual_IncludesRecordMetatableFactState(t *testing.T) {
	spec := contract.NewSpec().WithCallback(0, &contract.CallbackSpec{Cardinality: contract.CardExactlyOnce})
	method := typ.Func().Returns(typ.String).Build()
	methodWithSpec := typ.Func().Returns(typ.String).Spec(spec).Build()
	metatableWithoutSpec := typ.NewRecord().
		Field("__index", typ.NewRecord().Field("run", method).Build()).
		Build()
	metatableWithSpec := typ.NewRecord().
		Field("__index", typ.NewRecord().Field("run", methodWithSpec).Build()).
		Build()
	withoutSpec := typ.NewRecord().Metatable(metatableWithoutSpec).Build()
	withSpec := typ.NewRecord().Metatable(metatableWithSpec).Build()

	if !typ.TypeEquals(withoutSpec, withSpec) {
		t.Fatal("ordinary structural equality should ignore function specs inside metatables")
	}
	if FactTypeEqual(withoutSpec, withSpec) {
		t.Fatal("fact equality must include metatable fact state")
	}
}

func TestFactTypeEqual_SamePointerIsCompleteProof(t *testing.T) {
	spec := contract.NewSpec().WithCallback(0, &contract.CallbackSpec{Cardinality: contract.CardExactlyOnce})
	rec := typ.NewRecursivePlaceholder("Node")
	body := typ.NewRecord().
		Field("next", rec).
		Field("visit", typ.Func().Param("node", rec).Spec(spec).Build()).
		Build()
	rec.SetBody(body)

	if !FactTypeEqual(rec, rec) {
		t.Fatal("identical fact type pointer should be equal without descending through recursive metadata")
	}
}

func TestFactTypeEqual_RecursiveProductsUseFactRelation(t *testing.T) {
	spec := contract.NewSpec().WithCallback(0, &contract.CallbackSpec{Cardinality: contract.CardExactlyOnce})
	left := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("kind", typ.LiteralString("suite")).
			Field("children", typ.NewArray(self)).
			Field("visit", typ.Func().Param("node", self).Spec(spec).Build()).
			Build()
	})
	right := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("kind", typ.LiteralString("suite")).
			Field("children", typ.NewArray(self)).
			Field("visit", typ.Func().Param("node", self).Spec(spec).Build()).
			Build()
	})
	differentLiteral := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("kind", typ.LiteralString("case")).
			Field("children", typ.NewArray(self)).
			Field("visit", typ.Func().Param("node", self).Spec(spec).Build()).
			Build()
	})
	differentSpec := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("kind", typ.LiteralString("suite")).
			Field("children", typ.NewArray(self)).
			Field("visit", typ.Func().Param("node", self).Build()).
			Build()
	})

	if !FactTypeEqual(left, right) {
		t.Fatal("equivalent recursive fact products should compare equal")
	}
	if FactTypeEqual(left, differentLiteral) {
		t.Fatal("recursive fact equality must still distinguish literal fields")
	}
	if FactTypeEqual(left, differentSpec) {
		t.Fatal("recursive fact equality must still include function specs")
	}
}

func TestFactTypeEqual_TypedNilFunctionSignature(t *testing.T) {
	var nilFunction *typ.Function
	var nilType typ.Type = nilFunction

	if !FactTypeEqual(nilType, nil) {
		t.Fatal("typed nil function should compare as absent fact type")
	}
	if FactTypeEqual(nilType, typ.Func().Build()) {
		t.Fatal("typed nil function should not equal a concrete function")
	}
}
