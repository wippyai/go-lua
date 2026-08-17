package typeauthority

import (
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// Runtime decomposes a declaration into structural rows, so a generic
// declaration's body becomes a row of its own that is open in the declaration's
// formals. Every self reference in that body re-enters the declaration, and the
// row still has to carry a canonical identity.
func TestRuntimeSealsSelfReferentialGenericDeclaration(t *testing.T) {
	formal := typ.NewTypeParam("T", nil)
	declaration := typ.NewGeneric("Container", []*typ.TypeParam{formal}, nil)
	declaration.SetBody(typetable.NewRecord().
		Field("_value", formal).
		Field("get", typ.Func().Param("self", typ.Instantiate(declaration, formal)).Returns(formal).Build()).
		Build())

	runtime := &Runtime{}
	builder := runtimeBuilder{runtime: runtime, byIdentity: make(map[string]RuntimeInner)}
	if err := builder.seedPrimitives(); err != nil {
		t.Fatalf("seedPrimitives: %v", err)
	}
	inner, err := builder.add(runtimePending{value: declaration})
	if err != nil {
		t.Fatalf("Runtime add: %v", err)
	}
	for _, step := range []struct {
		name string
		run  func() error
	}{
		{name: "close", run: builder.close},
		{name: "describe", run: builder.describe},
		{name: "sealCanonical", run: builder.sealCanonical},
		{name: "sealDescriptors", run: builder.sealDescriptors},
		{name: "sealSubtypeRelation", run: builder.sealSubtypeRelation},
	} {
		if err := step.run(); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
	}
	if _, ok := runtime.InnerAtIndex(inner.index); !ok {
		t.Fatal("sealed declaration is not an owned Runtime row")
	}
}
