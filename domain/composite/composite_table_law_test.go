package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

// TestCompositeSurfaceSealsItsEmptyInventory states the composition's half of
// the root's population law: the composite surface is registered and declares
// nothing yet, and that is a sealed surface holding no rows rather than a hole
// in the table.
//
// The emptiness is the deferral documented on compositeSpecs in
// analysis/domain/composite/composite_table.go: no relation is declared as a composite
// until the typed Frame and its admitted write land with the store cut. This
// law is stated against the sealed table, so the day the first composite row is
// authored it is this test that has to be rewritten deliberately.
func TestCompositeSurfaceSealsItsEmptyInventory(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	sealed, failure := Table(compilation)
	if failure.Available() || sealed == nil {
		t.Fatalf("declaration table rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindComposite)
	if !viewOK {
		t.Fatal("sealed table holds no composite surface")
	}
	if !view.Available() || view.Kind() != schema.SurfaceKindComposite {
		t.Fatalf("composite surface sealed as kind %d", view.Kind())
	}
	if view.Count() != 0 {
		t.Fatalf("sealed composite surface holds %d rows; the analyzer declares no composite", view.Count())
	}
	if _, held := view.At(0); held {
		t.Fatal("sealed composite surface answered for a row it does not hold")
	}
}
