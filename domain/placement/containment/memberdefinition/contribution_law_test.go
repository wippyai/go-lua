package memberdefinition

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	containment "github.com/wippyai/go-lua/domain/placement/containment"
)

func TestContainmentContributionDeclaresItsRouteRowsAndTheIrreducibleFold(t *testing.T) {
	contribution := Contribution()
	if !contribution.Available() || contribution.Axis != "placement" || contribution.Rule != "placement-containment" {
		t.Fatalf("containment contribution identity=%q/%q available=%t", contribution.Axis, contribution.Rule, contribution.Available())
	}
	// The route vector this rule reads is its own statement: one relation,
	// its three projections, and the operation that publishes its rows. The
	// fold stays the one irreducible judgment beside them.
	if len(contribution.Relations) != 1 || len(contribution.Projections) != 3 || len(contribution.Selections) != 1 || len(contribution.Reducers) != 1 {
		t.Fatalf("containment contribution relations=%d projections=%d selections=%d reducers=%d",
			len(contribution.Relations), len(contribution.Projections), len(contribution.Selections), len(contribution.Reducers))
	}
	routes := contribution.Relations[0]
	if routes.Key != "placement/containment/routes" || routes.Subject != "ContainmentRouteCarrier" || !routes.CandidateProvider.Issued() {
		t.Fatalf("containment route relation=%+v, want the issued-candidate route vector", routes)
	}
	roles := map[member.Role]schema.Key{}
	for _, projection := range contribution.Projections {
		if projection.Relation != "ContainmentRoutes" || !projection.CandidateProvider.Issued() {
			t.Fatalf("containment projection=%+v, want a route projection under the issued candidate", projection)
		}
		roles[projection.Role] = projection.Key
	}
	if roles[member.Key] != "placement/containment/route-key" || roles[member.Predicate] != "placement/containment/route-tag" ||
		roles[member.Destination] != "placement/containment/route-destination" {
		t.Fatalf("containment route projections=%+v, want key, tag and destination", roles)
	}
	selection := contribution.Selections[0]
	if selection.Key != "placement/containment/route-selection" || selection.Relation != "ContainmentRoutes" ||
		selection.Tag != "ContainmentRouteTag" || selection.Implementation.Name != "DeriveContainmentRoutes" {
		t.Fatalf("containment selection=%+v, want the declared route operation", selection)
	}

	reducer := contribution.Reducers[0]
	if reducer.Key != "placement/containment/reducer" || reducer.Candidate != "" || len(reducer.Inputs) != 2 || len(reducer.Outputs) != 1 {
		t.Fatalf("containment reducer shape=%+v", reducer)
	}
	if reducer.Inputs[0].Form != member.ReadFormSelected || reducer.Inputs[0].Multiplicity != member.MultiplicityOne ||
		reducer.Inputs[1].Form != member.ReadFormExact || reducer.Inputs[1].Multiplicity != member.MultiplicityOne {
		t.Fatalf("containment inputs=%+v, want selected child then exact parent", reducer.Inputs)
	}
	for index, input := range reducer.Inputs {
		if input.Axis.Key != "placement" || input.Carrier != "PlacementFactCarrier" || input.Tag != "" || input.Route != "" {
			t.Fatalf("containment input %d=%+v, want owner-issued Placement fact without copied route vocabulary", index, input)
		}
	}
	if output := reducer.Outputs[0]; output.Axis.Key != "placement" || output.Carrier != "PlacementFactCarrier" {
		t.Fatalf("containment output=%+v, want PlacementFactCarrier", output)
	}
	if reducer.Implementation.PackagePath != containmentPackagePath || reducer.Implementation.Name != "ContainmentFold" {
		t.Fatalf("containment implementation=%+v", reducer.Implementation)
	}

	var _ func(placementdomain.Fact, placementdomain.Fact) (placementdomain.Fact, structure.ReductionOutcome) = containment.ContainmentFold
}
