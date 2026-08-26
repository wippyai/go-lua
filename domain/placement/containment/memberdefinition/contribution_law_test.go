package memberdefinition

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	definition "github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
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
	// its projections, and the operation that publishes its rows. The
	// fold stays the one irreducible judgment beside them.
	if len(contribution.Relations) != 3 || len(contribution.Projections) != 6 || len(contribution.Selections) != 1 || len(contribution.Reducers) != 1 {
		t.Fatalf("containment contribution relations=%d projections=%d selections=%d reducers=%d",
			len(contribution.Relations), len(contribution.Projections), len(contribution.Selections), len(contribution.Reducers))
	}
	var routes definition.Relation
	for _, relation := range contribution.Relations {
		if relation.Name == "ContainmentRoutes" {
			routes = relation
			break
		}
	}
	if routes.Key != "placement/containment/routes" || routes.Subject != "ContainmentRouteCarrier" || !routes.CandidateProvider.Issued() {
		t.Fatalf("containment route relation=%+v, want the issued-candidate route vector", routes)
	}
	roles := map[member.Role]schema.Key{}
	type accessor struct {
		name   string
		result int8
	}
	accessors := map[schema.Key]accessor{}
	for _, projection := range contribution.Projections {
		// The two vector-coordinate projections are declarations for the
		// complete summary relations, not route-row projections. They share
		// the issued candidate authority but must retain their own relation
		// identity; the route checks below apply only to the four route
		// columns.
		if projection.Relation != "ContainmentRoutes" {
			continue
		}
		if !projection.CandidateProvider.Issued() {
			t.Fatalf("containment projection=%+v, want a route projection under the issued candidate", projection)
		}
		roles[projection.Role] = projection.Key
		accessors[projection.Key] = accessor{name: projection.Accessor.Name, result: projection.Accessor.ResultIndex}
	}
	for _, projection := range contribution.Projections {
		if projection.Relation == "ContainmentRoutes" {
			continue
		}
		if !projection.CandidateProvider.Issued() {
			t.Fatalf("containment summary projection=%+v, want the same issued candidate authority", projection)
		}
	}
	if roles[member.Key] != "placement/containment/route-key" || roles[member.Predicate] != "placement/containment/route-tag" ||
		roles[member.Destination] != "placement/containment/route-destination" || roles[member.Attribute] != "placement/containment/route-parent" {
		t.Fatalf("containment route projections=%+v, want key, tag, destination and retained parent", roles)
	}
	if accessors["placement/containment/route-key"] != (accessor{name: "Coordinates", result: 0}) ||
		accessors["placement/containment/route-destination"] != (accessor{name: "Coordinates", result: 1}) ||
		accessors["placement/containment/route-parent"] != (accessor{name: "Parent", result: -1}) {
		t.Fatalf("containment route accessors=%+v, want parent key, child destination, retained parent", accessors)
	}
	if routes.Inputs[0].Carrier != "PlacementFactCarrier" || !routes.Inputs[0].Many || routes.Inputs[0].Form != member.ReadFormComplete ||
		routes.Inputs[1].Carrier != "HeapFactCarrier" || !routes.Inputs[1].Many || routes.Inputs[1].Form != member.ReadFormComplete {
		t.Fatalf("containment route inputs=%+v, want complete Placement and Heap deliveries", routes.Inputs)
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
		reducer.Inputs[1].Form != member.ReadFormSelected || reducer.Inputs[1].Multiplicity != member.MultiplicityOne {
		t.Fatalf("containment inputs=%+v, want selected child and retained parent from one route row", reducer.Inputs)
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

func TestContainmentContributionDeclaresBothSummaryJoinKeys(t *testing.T) {
	contribution := Contribution()
	want := map[schema.Key]string{
		"placement/containment/placement-summary-coordinate": "ContainmentPlacementSummary",
		"heap/containment/heap-summary-coordinate":           "ContainmentHeapSummary",
	}
	for _, projection := range contribution.Projections {
		if relation, ok := want[projection.Key]; ok {
			if projection.Relation != relation || projection.Role != member.Key {
				t.Fatalf("summary key projection=%+v", projection)
			}
			delete(want, projection.Key)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing owner-declared summary keys=%v", want)
	}
}
