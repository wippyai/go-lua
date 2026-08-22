package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/query"
)

// Query construction enumerates the sealed query table. Family names stay on
// their owning registrations.

func TestQueryConstructionUsesSealedQueryFamilies(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	issued := QueryIssuance(compilation)
	pins := queryIssuancePins()
	if len(issued) != len(pins) {
		t.Fatalf("sealed table issues %d query families, want %d", len(issued), len(pins))
	}
	seen := make(map[string]bool, len(issued))
	for position, family := range issued {
		if !family.Family.Available() || !family.Authority.Available() || !family.Population.Available() || !family.Projection.Available() {
			t.Fatalf("issued family %q is not a complete sealed query identity", family.Family)
		}
		if family.Family != pins[position] {
			t.Fatalf("issued family at position %d is %q, want %q", position, family.Family, pins[position])
		}
		if family.Authority != family.Family {
			t.Fatalf("family %q publishes authority %q, not its declaration key", family.Family, family.Authority)
		}
		switch family.Population {
		case query.PopulationSelectedPoint:
			if family.Projection != query.ProjectionSummary && family.Projection != query.ProjectionExact {
				t.Fatalf("family %q declares projection %q", family.Family, family.Projection)
			}
		case query.PopulationObservation:
			if family.Family != QueryFamilyCallCalleeSet || family.Projection != query.ProjectionExact {
				t.Fatalf("observation family %q declares population/projection %q/%q", family.Family, family.Population, family.Projection)
			}
		default:
			t.Fatalf("family %q has unknown population %q", family.Family, family.Population)
		}
		if seen[string(family.Family)] {
			t.Fatalf("family %q is issued twice", family.Family)
		}
		seen[string(family.Family)] = true
	}
	selected, selectedOK := selectedPointQueryIssuance(compilation.catalog)
	if !selectedOK || len(selected) != len(queryPositionPins()) {
		t.Fatalf("selected query issuance holds %d families, want %d", len(selected), len(queryPositionPins()))
	}
	for position, family := range selected {
		if family.Family != queryPositionPins()[position] || family.Population != query.PopulationSelectedPoint {
			t.Fatalf("selected query family at position %d is %q/%q", position, family.Family, family.Population)
		}
	}
}
