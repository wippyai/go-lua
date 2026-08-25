package memberdefinition

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	publicationescape "github.com/wippyai/go-lua/domain/placement/publicationescape"
)

func TestPublicationEscapeContributionDerivesTheOwnedFoldSignature(t *testing.T) {
	contribution := Contribution()
	if !contribution.Available() || contribution.Axis != "placement" || contribution.Rule != "placement-publication-escape" {
		t.Fatalf("publication escape contribution identity=%q/%q available=%t", contribution.Axis, contribution.Rule, contribution.Available())
	}
	if len(contribution.Relations) != 0 || len(contribution.Projections) != 0 || len(contribution.Reducers) != 1 {
		t.Fatalf("publication escape contribution relations=%d projections=%d reducers=%d, want only one reducer", len(contribution.Relations), len(contribution.Projections), len(contribution.Reducers))
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

	if len(contribution.Carriers) != 1 || contribution.Carriers[0].Name != "PublicationRequirementCarrier" ||
		contribution.Carriers[0].Type.Name != "Placement" {
		t.Fatalf("publication escape carriers=%+v, want only its owned requirement carrier", contribution.Carriers)
	}
	if reducer.Implementation.PackagePath != publicationEscapePackagePath || reducer.Implementation.Name != "PublicationEscapeFold" {
		t.Fatalf("publication escape implementation=%+v", reducer.Implementation)
	}
	var _ func(placementdomain.Placement, placementdomain.Fact) (placementdomain.Fact, structure.ReductionOutcome) = publicationescape.PublicationEscapeFold
}
