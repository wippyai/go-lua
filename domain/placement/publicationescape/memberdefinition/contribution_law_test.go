package memberdefinition

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	definition "github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementbase "github.com/wippyai/go-lua/domain/placement/memberdefinition"
	publicationescape "github.com/wippyai/go-lua/domain/placement/publicationescape"
)

func TestPublicationEscapeContributionDerivesTheOwnedFoldSignature(t *testing.T) {
	contribution := Contribution()
	if !contribution.Available() || contribution.Axis != "placement" || contribution.Rule != "placement-publication-escape" {
		t.Fatalf("publication escape contribution identity=%q/%q available=%t", contribution.Axis, contribution.Rule, contribution.Available())
	}
	if len(contribution.Relations) != 1 || len(contribution.Projections) != 1 || len(contribution.Reducers) != 1 {
		t.Fatalf("publication escape contribution relations=%d projections=%d reducers=%d, want one route relation, destination, and reducer", len(contribution.Relations), len(contribution.Projections), len(contribution.Reducers))
	}
	relation := contribution.Relations[0]
	if relation.Name != "PublicationRoutes" || relation.Key != "placement/publication-escape/routes" || relation.Subject != "PublicationRouteCarrier" || !relation.CandidateProvider.AxisRelation.Available() {
		t.Fatalf("publication escape route relation=%+v, want the foreign mounted-call candidate authority", relation)
	}
	destination := contribution.Projections[0]
	if destination.Name != "PublicationRouteDestination" || destination.Key != "placement/publication-escape/route-destination" || destination.Relation != "PublicationRoutes" || destination.Role != member.Destination || destination.Result != "PlacementKeyCarrier" || destination.Accessor.Name != "Coordinates" || destination.Accessor.ResultIndex != 1 {
		t.Fatalf("publication escape destination=%+v, want owner-issued route destination column", destination)
	}

	reducer := contribution.Reducers[0]
	if reducer.Key != "placement/publication-escape/reducer" || reducer.Candidate != "" || len(reducer.Inputs) != 1 || len(reducer.Outputs) != 1 {
		t.Fatalf("publication escape reducer shape=%+v", reducer)
	}
	input := reducer.Inputs[0]
	if input.Axis.Key != "placement" || input.Carrier != "PlacementFactCarrier" || input.Form != member.ReadFormSelected ||
		input.Multiplicity != member.MultiplicityOne || input.Tag != "PublicationRequirementCarrier" || input.Route != "" {
		t.Fatalf("publication escape input=%+v, want one selected Placement fact with owner-issued requirement", input)
	}
	if output := reducer.Outputs[0]; output.Axis.Key != "placement" || output.Carrier != "PlacementFactCarrier" {
		t.Fatalf("publication escape output=%+v, want PlacementFactCarrier", output)
	}

	if len(contribution.Carriers) != 3 || contribution.Carriers[0].Name != "PublicationRequirementCarrier" ||
		contribution.Carriers[0].Type.Name != "Placement" || contribution.Carriers[1].Name != "PublicationRouteCarrier" ||
		contribution.Carriers[2].Name != "PublicationRouteTagCarrier" {
		t.Fatalf("publication escape carriers=%+v, want requirement plus route and tag carriers", contribution.Carriers)
	}
	if reducer.Implementation.PackagePath != publicationEscapePackagePath || reducer.Implementation.Name != "PublicationEscapeFold" {
		t.Fatalf("publication escape implementation=%+v", reducer.Implementation)
	}
	var _ func(placementdomain.Placement, placementdomain.Fact) (placementdomain.Fact, structure.ReductionOutcome) = publicationescape.PublicationEscapeFold
}

// TestPublicationEscapeRejectsAWriterAliasForTheRouteDestination keeps the
// destination projection on the route relation that owns its coordinate. A
// writer-shaped alias is hostile even when it uses the same Placement key
// carrier: it would erase the route-to-factor boundary at composition time.
func TestPublicationEscapeRejectsAWriterAliasForTheRouteDestination(t *testing.T) {
	contribution := Contribution()
	contribution.Projections[0].Relation = "StorageRoutes"
	source := definition.Source{
		Package: "placement",
		Name:    "placement",
		Base:    placementbase.Storage(),
		Contributions: []definition.Contribution{
			contribution,
		},
	}
	if _, ok := source.Compose(); ok {
		t.Fatal("publication destination projection accepted a writer relation alias")
	}
}
