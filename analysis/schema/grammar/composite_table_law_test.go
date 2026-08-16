package grammar

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

// TestCompositeSurfaceSealsItsEmptyInventory states the composition's half of
// the root's population law: the composite surface is registered and declares
// nothing yet, and that is a sealed surface holding no rows rather than a hole
// in the table.
func TestCompositeSurfaceSealsItsEmptyInventory(t *testing.T) {
	sealed, failure := Table()
	if failure.Available() || sealed == nil {
		t.Fatalf("declaration table rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindComposite)
	if !viewOK {
		t.Fatal("sealed table holds no composite surface")
	}
	if view.Count() != len(compositeSpecs()) {
		t.Fatalf("sealed composite surface holds %d of %d authored rows", view.Count(), len(compositeSpecs()))
	}
}

// TestCompositeSurfaceIsSealedAfterTheAxisSurface states the phase law for this
// surface: a composite resolves every axis it names against the sealed axis
// inventory, so it is sealed after it.
func TestCompositeSurfaceIsSealedAfterTheAxisSurface(t *testing.T) {
	if schema.SurfaceKindComposite <= schema.SurfaceKindAxis {
		t.Fatalf("composite catalog ordinal %d does not follow the axis ordinal %d", schema.SurfaceKindComposite, schema.SurfaceKindAxis)
	}
}
