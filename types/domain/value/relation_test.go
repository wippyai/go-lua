package value

import (
	"testing"

	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/typ"
)

// TestSameConvergedFact_AcyclicDistinguishesCallbackEnvOverlay proves the
// value-domain convergence relation (and the shape interning that uses it) keeps
// two same-signature acyclic functions distinct when their callback EnvOverlays
// differ. Collapsing them would drop one overlay, hiding a global that the larger
// overlay declares (e.g. after_each) when a prior compilation interns the smaller
// one first.
func TestSameConvergedFact_AcyclicDistinguishesCallbackEnvOverlay(t *testing.T) {
	callbackParam := func() *typ.FunctionBuilder {
		return typ.Func().Param("fn", typ.Func().Build()).Returns(typ.Func().Build())
	}

	small := contract.NewSpec().WithCallback(0, (&contract.CallbackSpec{Cardinality: contract.CardExactlyOnce}).WithEnvOverlay(map[string]typ.Type{
		"describe": typ.Func().Build(),
		"it":       typ.Func().Build(),
	}))
	large := contract.NewSpec().WithCallback(0, (&contract.CallbackSpec{Cardinality: contract.CardExactlyOnce}).WithEnvOverlay(map[string]typ.Type{
		"describe":   typ.Func().Build(),
		"it":         typ.Func().Build(),
		"after_each": typ.Func().Build(),
	}))

	smallFn := callbackParam().Spec(small).Build()
	largeFn := callbackParam().Spec(large).Build()

	if SameConvergedFact(smallFn, largeFn) {
		t.Fatal("functions with different callback EnvOverlays must not be the same converged fact")
	}
	if !SameConvergedFact(largeFn, largeFn) {
		t.Fatal("a function must be the same converged fact as itself")
	}
	noSpec := callbackParam().Build()
	if SameConvergedFact(largeFn, noSpec) {
		t.Fatal("a spec-carrying function must not converge with a spec-less one of the same signature")
	}
}

func TestCovers_RecursiveProductAdmitsCoveredObservation(t *testing.T) {
	suite := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Field("full_path", typ.String).
			Build()
	})
	observation := typ.NewRecord().
		Field("name", typ.String).
		Field("children", typ.NewArray(suite)).
		Field("full_path", typ.String).
		Build()

	if !Covers(suite, observation) {
		t.Fatalf("recursive product should cover its unfolded observation")
	}
}

func TestCovers_RecursiveUnionAdmitsCoveredObservation(t *testing.T) {
	suite := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	observation := typ.NewRecord().
		Field("name", typ.String).
		Field("children", typ.NewArray(suite)).
		Build()
	upper := typ.NewUnion(suite, typ.Boolean)

	if !Covers(upper, observation) {
		t.Fatalf("recursive union should cover observation through its recursive member")
	}
}

func TestCovers_AcyclicUsesSubtype(t *testing.T) {
	upper := typ.Number
	observation := typ.Integer
	if !Covers(upper, observation) {
		t.Fatalf("number should cover integer through acyclic subtype")
	}
	if Covers(observation, upper) {
		t.Fatalf("integer must not cover number")
	}
}
