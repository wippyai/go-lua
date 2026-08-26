package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

// The evidence fold writes an evidence Factor, but its routed destination is
// still the Placement-owned coordinate of the canonical SuspensionRoutes row.
// This is an intentional cross-axis use of one owner-issued address; creating
// an evidence-side route destination would duplicate the route identity.
func TestSuspensionEvidenceUsesTheCanonicalPlacementRouteDestination(t *testing.T) {
	output := SuspensionEvidence().Fold.Outputs[0]
	want := member.ProjectionRef{Axis: placementAxis(), Member: suspensionRouteDestination}
	if output.Destination != want {
		t.Fatalf("evidence destination=%+v, want canonical Placement route projection %+v", output.Destination, want)
	}

	// Nearest negative: the evidence output column is a writer fact, not an
	// address projection. Substituting it would cross the route owner fence.
	foreign := member.ProjectionRef{
		Axis:   axisReference("placement-suspension-evidence"),
		Member: schema.Key("placement/suspension-evidence/facts"),
	}
	if output.Destination == foreign {
		t.Fatal("evidence destination reused its writer fact as a route address")
	}
}
