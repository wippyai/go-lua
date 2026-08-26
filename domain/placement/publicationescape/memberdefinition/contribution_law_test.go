package memberdefinition

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	publicationescape "github.com/wippyai/go-lua/domain/placement/publicationescape"
)

func TestPublicationEscapeContributionDeclaresItsVectorsAndTheOwnedFold(t *testing.T) {
	contribution := Contribution()
	if !contribution.Available() || contribution.Axis != "placement" || contribution.Rule != "placement-publication-escape" {
		t.Fatalf("publication escape contribution identity=%q/%q available=%t", contribution.Axis, contribution.Rule, contribution.Available())
	}
	// The two vectors this rule reads are its own statement: the Effect source
	// rows, the Placement route rows, the projections each is addressed by,
	// and the operations that publish them. The fold stays the one irreducible
	// judgment beside them.
	if len(contribution.Relations) != 2 || len(contribution.Projections) != 5 || len(contribution.Selections) != 2 || len(contribution.Reducers) != 1 {
		t.Fatalf("publication escape contribution relations=%d projections=%d selections=%d reducers=%d",
			len(contribution.Relations), len(contribution.Projections), len(contribution.Selections), len(contribution.Reducers))
	}
	relations := map[string]string{}
	for _, relation := range contribution.Relations {
		if relation.CandidateProvider.AxisRelation.Axis.Key != "effect" {
			t.Fatalf("relation %q hangs off %+v, want Effect's own mounted call row", relation.Name, relation.CandidateProvider)
		}
		relations[relation.Name] = string(relation.Key)
	}
	if relations["PublicationSources"] != "effect/mounted-publication/sources" ||
		relations["PublicationRoutes"] != "placement/publication-escape/routes" {
		t.Fatalf("publication escape relations=%v", relations)
	}
	// A selection names no operation of its own: it publishes into a relation
	// this rule declares and stamps that relation's tag projection.
	selections := map[string]string{}
	for _, selection := range contribution.Selections {
		if _, declared := relations[string(selection.Relation)]; !declared {
			t.Fatalf("selection %q publishes into undeclared relation %q", selection.Name, selection.Relation)
		}
		stamped := false
		for _, projection := range contribution.Projections {
			if projection.Name == selection.Tag && projection.Relation == selection.Relation {
				stamped = true
			}
		}
		if !stamped {
			t.Fatalf("selection %q stamps no declared tag projection %q", selection.Name, selection.Tag)
		}
		selections[string(selection.Key)] = string(selection.Relation)
	}
	if selections["effect/publication-escape/source-selection"] != "PublicationSources" ||
		selections["placement/publication-escape/route-selection"] != "PublicationRoutes" {
		t.Fatalf("publication escape selections=%v", selections)
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

	// The requirement carrier is this rule's own; the rest are the carriers
	// its declared rows are typed in, which composition folds against the
	// axes that already publish them.
	requirement := false
	for _, carrier := range contribution.Carriers {
		if carrier.Name == "PublicationRequirementCarrier" && carrier.Type.Name == "Placement" {
			requirement = true
		}
	}
	if !requirement {
		t.Fatalf("publication escape carriers=%+v, want its owned requirement carrier", contribution.Carriers)
	}
	if reducer.Implementation.PackagePath != publicationEscapePackagePath || reducer.Implementation.Name != "PublicationEscapeFold" {
		t.Fatalf("publication escape implementation=%+v", reducer.Implementation)
	}
	var _ func(placementdomain.Placement, placementdomain.Fact) (placementdomain.Fact, structure.ReductionOutcome) = publicationescape.PublicationEscapeFold
}
