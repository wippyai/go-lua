package memberdefinition

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	containment "github.com/wippyai/go-lua/domain/placement/containment"
)

func TestContainmentContributionDeclaresOnlyTheIrreducibleFold(t *testing.T) {
	contribution := Contribution()
	if !contribution.Available() || contribution.Axis != "placement" || contribution.Rule != "placement-containment" {
		t.Fatalf("containment contribution identity=%q/%q available=%t", contribution.Axis, contribution.Rule, contribution.Available())
	}
	if len(contribution.Relations) != 0 || len(contribution.Projections) != 0 || len(contribution.Reducers) != 1 {
		t.Fatalf("containment contribution relations=%d projections=%d reducers=%d, want only one reducer", len(contribution.Relations), len(contribution.Projections), len(contribution.Reducers))
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
