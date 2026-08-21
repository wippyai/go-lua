package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
)

// An engine-published axis declares a coordinate space whose column the engine
// fills itself: no factor cell holds it and no rule writes it, so it declares
// no cold fragment, no binding, and no algebra. The reachability publication is
// one, and the laws below state that such an axis reaches the composition's own
// cold and hot passes rather than only the surface that admits it.

// enginePublishedProbe is one engine-published axis declaration in the shape a
// domain would register. It names the composition's own Link input record, so
// what the passes below run on is a production-shaped inventory row.
func enginePublishedProbe(t *testing.T, key, semantic schema.Key) *axisTemplate {
	t.Helper()
	template, ok := axis.New(axis.Spec[LinkInputs]{
		Key:         key,
		Storage:     axis.StorageEngine,
		Cardinality: axis.CardinalitySparse,
		Lifetime:    axis.LifetimeProgram,
		Mutability:  axis.MutabilityFrozen,
		Concurrency: axis.ConcurrencyShared,
		Frame:       axis.Frame{Outputs: []axis.Output{{Key: key + "/facts", Writer: key}}},
		Semantic:    semantic,
	})
	if !ok {
		t.Fatalf("engine-published axis %q was not admitted", key)
	}
	return template
}

// TestEnginePublishedAxisPassesThroughTheColdAndHotPasses states the passes'
// side of the storage declaration: an axis with no factor binding is passed
// over by both, and the coverage law reads the declared storage rather than
// demanding a cell from every row.
func TestEnginePublishedAxisPassesThroughTheColdAndHotPasses(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	state := compilation.catalog
	roles, rolesOK := SemanticRoles(compilation)
	if !rolesOK {
		t.Fatal("semantic role vocabulary")
	}
	entries := []*axisTemplate{enginePublishedProbe(t, "reachability", "semantic/factor/value/summary-identity")}
	builder := engine.NewSchema()
	fragments, failedAxis, declared := declareAxisInventory(state, entries, builder, roles)
	if !declared {
		t.Fatalf("cold pass rejected an engine-published inventory at axis %v", failedAxis)
	}
	if !fragments.available(entries) {
		t.Fatal("cold pass over an engine-published inventory reported incomplete coverage")
	}
	if fragments[1].Available() {
		t.Fatal("cold pass recorded a fragment for an axis that declares none")
	}
	// The hot pass is a transaction on one sealed binding. The composition's
	// own sealed catalog supplies it: an engine-published axis instantiates
	// nothing on it, which is exactly what the pass has to show.
	binding := engine.NewSchemaBinding(compilation.Schema())
	if binding == nil {
		t.Fatal("the sealed catalog published no binding")
	}
	bound, failedAxis, boundOK := bindAxisInventory(state, entries, binding, fragments, LinkInputs{})
	if !boundOK {
		t.Fatalf("hot pass rejected an engine-published inventory at axis %v", failedAxis)
	}
	if bound[1].Available() {
		t.Fatal("hot pass bound an axis that declares no binding")
	}
}

// TestBoundAxisWithoutItsCellLeavesThePassIncomplete is the other half: the
// coverage the storage declaration removes for an engine-published axis is
// still demanded of a bound one, so passing over a row is a property of the
// declaration rather than of the pass.
func TestBoundAxisWithoutItsCellLeavesThePassIncomplete(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	state := compilation.catalog
	if state == nil || state.sealed == nil {
		t.Fatal("declaration table did not seal")
	}
	empty := newAxisCells(state.axes)
	if empty.available(state.axes) {
		t.Fatal("an empty pass over the bound production inventory reported complete coverage")
	}
}
