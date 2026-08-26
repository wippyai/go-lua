package memberdefinition

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/operand"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
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
	// The selection carries no symbol of its own: the operation that computes
	// these rows is the derivation ContainmentRoutes declares. What the row
	// states is which relation it publishes into and which tag it stamps, and
	// both are rows this rule declares itself.
	selection := contribution.Selections[0]
	if selection.Key != "placement/containment/route-selection" || selection.Relation != routes.Name ||
		selection.Tag != "ContainmentRouteTag" {
		t.Fatalf("containment selection=%+v, want the route vector published under its tag", selection)
	}
	stamped := false
	for _, projection := range contribution.Projections {
		if projection.Name == selection.Tag {
			stamped = projection.Relation == selection.Relation && projection.Role == member.Predicate
		}
	}
	if !stamped {
		t.Fatalf("containment selection stamps %q, which is not the predicate projection of %q", selection.Tag, selection.Relation)
	}

	// The fold takes the reads its Program folds: the two complete vectors
	// that closed the parent denominator, and the routed child cell it
	// publishes at, which carries the coordinate and the tag it was
	// correlated by.
	reducer := contribution.Reducers[0]
	if reducer.Key != "placement/containment/reducer" || reducer.Candidate != "" || len(reducer.Inputs) != 3 || len(reducer.Outputs) != 1 {
		t.Fatalf("containment reducer shape=%+v", reducer)
	}
	if reducer.Inputs[0].Form != member.ReadFormComplete || reducer.Inputs[0].Multiplicity != member.MultiplicityMany ||
		reducer.Inputs[0].Axis.Key != "placement" || reducer.Inputs[0].Carrier != "PlacementFactCarrier" {
		t.Fatalf("containment parent vector=%+v, want the complete Placement denominator", reducer.Inputs[0])
	}
	if reducer.Inputs[1].Form != member.ReadFormComplete || reducer.Inputs[1].Multiplicity != member.MultiplicityMany ||
		reducer.Inputs[1].Axis.Key != "heap" || reducer.Inputs[1].Carrier != "HeapFactCarrier" {
		t.Fatalf("containment heap vector=%+v, want the complete Heap denominator", reducer.Inputs[1])
	}
	if reducer.Inputs[2].Form != member.ReadFormSelected || reducer.Inputs[2].Multiplicity != member.MultiplicityOne ||
		reducer.Inputs[2].Axis.Key != "placement" || reducer.Inputs[2].Carrier != "PlacementFactCarrier" ||
		reducer.Inputs[2].Tag != "ContainmentRouteTagCarrier" || reducer.Inputs[2].Route != "PlacementKeyCarrier" {
		t.Fatalf("containment routed cell=%+v, want the routed child with its coordinate and tag", reducer.Inputs[2])
	}
	for _, vector := range reducer.Inputs[:2] {
		if vector.Tag != "" || vector.Route != "" {
			t.Fatalf("containment vector=%+v, want a whole-vector delivery without route vocabulary", vector)
		}
	}
	if output := reducer.Outputs[0]; output.Axis.Key != "placement" || output.Carrier != "PlacementFactCarrier" {
		t.Fatalf("containment output=%+v, want PlacementFactCarrier", output)
	}
	if reducer.Implementation.PackagePath != containmentPackagePath || reducer.Implementation.Name != "ContainmentFold" {
		t.Fatalf("containment implementation=%+v", reducer.Implementation)
	}

	var _ func(operand.SummaryVector[placementdomain.Fact], operand.SummaryVector[heapdomain.Value], heapdomain.Key, uint64, placementdomain.Fact) (placementdomain.Fact, structure.ReductionOutcome) = containment.ContainmentFold
}
