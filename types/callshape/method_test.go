package callshape

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestHasExplicitSelfSimpleRejectsTopLikeFirstParam(t *testing.T) {
	receiver := typ.NewRecord().Field("id", typ.String).Build()

	anyFirst := typ.Func().
		Param("options", typ.Any).
		Returns(typ.Boolean).
		Build()
	if HasExplicitSelfSimple(anyFirst, receiver) {
		t.Fatal("any first param should not be treated as explicit self")
	}

	unknownFirst := typ.Func().
		Param("value", typ.Unknown).
		Returns(typ.Boolean).
		Build()
	if HasExplicitSelfSimple(unknownFirst, receiver) {
		t.Fatal("unknown first param should not be treated as explicit self")
	}
}

func TestHasExplicitSelfSimpleAcceptsExplicitPatterns(t *testing.T) {
	receiver := typ.NewRecord().Field("id", typ.String).Build()

	namedSelf := typ.Func().
		Param("self", typ.Any).
		Returns(typ.Boolean).
		Build()
	if !HasExplicitSelfSimple(namedSelf, receiver) {
		t.Fatal("parameter named self should be treated as explicit self")
	}

	matchingType := typ.Func().
		Param("receiver", receiver).
		Returns(typ.Boolean).
		Build()
	if !HasExplicitSelfSimple(matchingType, receiver) {
		t.Fatal("receiver-compatible first param should be treated as explicit self")
	}
}

func TestHasExplicitSelfSimpleNamedSelfSkipsRecursiveReceiverNormalization(t *testing.T) {
	receiver := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("next", self).
			Field("callback", typ.Func().Param("node", self).Returns(self).Build()).
			Build()
	})
	fn := typ.Func().
		Param("self", typ.Any).
		Param("name", typ.String).
		Returns(typ.Boolean).
		Build()
	if !HasExplicitSelfSimple(fn, receiver) {
		t.Fatal("parameter named self should not require recursive receiver normalization")
	}
}

func TestHasExplicitSelfSimpleRejectsTopLikeReceiver(t *testing.T) {
	receiver := typ.Any
	optionsType := typ.NewRecord().
		Field("count", typ.NewOptional(typ.Number)).
		Build()

	optionsFirst := typ.Func().
		Param("options", typ.NewOptional(optionsType)).
		Returns(typ.Boolean).
		Build()
	if HasExplicitSelfSimple(optionsFirst, receiver) {
		t.Fatal("top-like receiver should not trigger explicit self inference")
	}

	tpOptions := &typ.TypeParam{Name: "T", Constraint: typ.NewOptional(optionsType)}
	genericFirst := typ.Func().
		Param("options", tpOptions).
		Returns(typ.Boolean).
		Build()
	if HasExplicitSelfSimple(genericFirst, receiver) {
		t.Fatal("top-like receiver should not trigger explicit self inference for constrained type params")
	}
}

func TestHasExplicitSelfSimpleAcceptsLiteralReceiverAgainstPrimitiveParam(t *testing.T) {
	receiver := typ.LiteralString("hello")
	fn := typ.Func().
		Param("s", typ.String).
		Param("start", typ.Integer).
		Returns(typ.String).
		Build()

	if !HasExplicitSelfSimple(fn, receiver) {
		t.Fatal("literal string receiver should match explicit primitive self param")
	}
}
