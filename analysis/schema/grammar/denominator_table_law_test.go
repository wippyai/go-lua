package grammar

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

// TestDenominatorTableCoversEveryAxis is the drift law of this inventory: the
// closed worlds the analyzer quantifies over are the coordinate populations of
// its axes, so one denominator is declared per axis and each one is owned by
// the axis it describes.
func TestDenominatorTableCoversEveryAxis(t *testing.T) {
	sealed, failure := Table()
	if failure.Available() || sealed == nil {
		t.Fatalf("declaration table rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	axes, axesOK := sealed.Surface(schema.SurfaceKindAxis)
	view, viewOK := sealed.Surface(schema.SurfaceKindDenominator)
	if !axesOK || !viewOK {
		t.Fatal("sealed table holds no axis or denominator surface")
	}
	if view.Count() != axes.Count() {
		t.Fatalf("sealed denominator surface holds %d closed worlds over %d axes", view.Count(), axes.Count())
	}
	universes := make(map[identity.ContentID]schema.Key, view.Count())
	for position := 0; position < view.Count(); position++ {
		row, rowOK := view.At(position)
		entry, entryOK := row.(*denominator.Entry)
		if !rowOK || !entryOK {
			t.Fatalf("denominator row %d is not a denominator entry", position)
		}
		if entry.Owner().Surface != schema.SurfaceKindAxis {
			t.Fatalf("denominator %q is owned by surface %d, not the axis surface", entry.Key(), entry.Owner().Surface)
		}
		if _, declared := axes.ByID(schema.NewEntryID(schema.SurfaceKindAxis, entry.Owner().Entry)); !declared {
			t.Fatalf("denominator %q names an axis that is not declared", entry.Key())
		}
		// A universe closed at publication is one whose members the solver
		// derives, which is what an axis's coordinate population is.
		if entry.Phase() != denominator.PhasePublication {
			t.Fatalf("denominator %q closes at phase %d, not at publication", entry.Key(), entry.Phase())
		}
		if prior, duplicate := universes[entry.Universe()]; duplicate {
			t.Fatalf("denominators %q and %q describe one universe", prior, entry.Key())
		}
		universes[entry.Universe()] = entry.Key()
	}
}

// TestDenominatorSurfaceIsSealedAfterItsOwners states the phase law for this
// surface: a denominator resolves its owner against a surface sealed below it.
func TestDenominatorSurfaceIsSealedAfterItsOwners(t *testing.T) {
	if schema.SurfaceKindDenominator <= schema.SurfaceKindAxis {
		t.Fatalf("denominator catalog ordinal %d does not follow the axis ordinal %d", schema.SurfaceKindDenominator, schema.SurfaceKindAxis)
	}
}
