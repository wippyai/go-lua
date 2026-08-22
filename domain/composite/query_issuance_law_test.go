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
	if len(issued) == 0 {
		t.Fatal("sealed table declares no query families")
	}
	seen := make(map[string]bool, len(issued))
	for _, family := range issued {
		if !family.Family.Available() || !family.Authority.Available() || !family.Population.Available() || !family.Projection.Available() {
			t.Fatalf("issued family %q is not a complete sealed query identity", family.Family)
		}
		if family.Authority != family.Family {
			t.Fatalf("family %q publishes authority %q, not its declaration key", family.Family, family.Authority)
		}
		if family.Population != query.PopulationSelectedPoint {
			t.Fatalf("family %q is asked at %q, not the selected-point population", family.Family, family.Population)
		}
		if family.Projection != query.ProjectionSummary && family.Projection != query.ProjectionExact {
			t.Fatalf("family %q declares projection %q", family.Family, family.Projection)
		}
		if seen[string(family.Family)] {
			t.Fatalf("family %q is issued twice", family.Family)
		}
		seen[string(family.Family)] = true
	}
}
