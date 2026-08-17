package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

// TestDenominatorTableCoversEveryAxis is the drift law of the coordinate half
// of this inventory: one of the closed worlds the analyzer quantifies over is
// the coordinate population of an axis, so one denominator is declared per axis
// and each one is owned by the axis it describes. The surface hosts more than
// this family - relation-family row populations are declared on it too - so
// what the count states is that every axis has its world, not that the surface
// holds no other.
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
	// Every declared universe is unique across the whole surface, not only
	// within one family: two closed worlds under one description would make a
	// totality claim depend on which name it was made under.
	universes := make(map[identity.ContentID]schema.Key, view.Count())
	owned := 0
	for position := 0; position < view.Count(); position++ {
		row, rowOK := view.At(position)
		entry, entryOK := row.(*denominator.Entry)
		if !rowOK || !entryOK {
			t.Fatalf("denominator row %d is not a denominator entry", position)
		}
		if prior, duplicate := universes[entry.Universe()]; duplicate {
			t.Fatalf("denominators %q and %q describe one universe", prior, entry.Key())
		}
		universes[entry.Universe()] = entry.Key()
		if entry.Owner().Surface != schema.SurfaceKindAxis {
			continue
		}
		if _, declared := axes.ByID(schema.NewEntryID(schema.SurfaceKindAxis, entry.Owner().Entry)); !declared {
			t.Fatalf("denominator %q names an axis that is not declared", entry.Key())
		}
		// A universe closed at publication is one whose members the solver
		// derives, which is what an axis's coordinate population is.
		if entry.Phase() != denominator.PhasePublication {
			t.Fatalf("denominator %q closes at phase %d, not at publication", entry.Key(), entry.Phase())
		}
		owned++
	}
	if owned != axes.Count() {
		t.Fatalf("sealed denominator surface holds %d coordinate worlds over %d axes", owned, axes.Count())
	}
}
