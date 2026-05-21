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
