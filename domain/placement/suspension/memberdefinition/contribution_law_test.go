package memberdefinition

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

// The suspension consumer owns one source/route derivation and one scalar
// fold. The route derivation consumes the complete Value vector; its fold
// receives only the scalars retained by that route row.
func TestSuspensionContributionDeclaresOneVectorAuthorityAndOneScalarFold(t *testing.T) {
	contribution := Contribution()
	if !contribution.Available() || contribution.Axis != "placement" || contribution.Rule != "placement-suspension" {
		t.Fatalf("suspension contribution identity=%q/%q available=%t", contribution.Axis, contribution.Rule, contribution.Available())
	}
	if len(contribution.Relations) != 3 || len(contribution.Projections) != 7 || len(contribution.Selections) != 2 || len(contribution.Reducers) != 1 {
		t.Fatalf("suspension contribution relations=%d projections=%d selections=%d reducers=%d",
			len(contribution.Relations), len(contribution.Projections), len(contribution.Selections), len(contribution.Reducers))
	}
	for _, relation := range contribution.Relations {
		if !relation.CandidateProvider.Issued() {
			t.Fatalf("relation %q does not hang off the issued Program row", relation.Name)
		}
	}
	for _, selection := range contribution.Selections {
		if !selection.Implementation.Available() || !strings.HasPrefix(selection.Implementation.Name, "Derive") {
			t.Fatalf("selection %q names no owner judgment: %+v", selection.Name, selection.Implementation)
		}
	}

	var routesFound bool
	for _, relation := range contribution.Relations {
		if relation.Name != "SuspensionRoutes" {
			continue
		}
		routesFound = true
		if len(relation.Inputs) != 2 || relation.Inputs[0].Carrier != "SubjectLivenessCarrier" ||
			relation.Inputs[1].Carrier != "ValueFactCarrier" || !relation.Inputs[1].Many || relation.Inputs[1].Form != member.ReadFormComplete {
			t.Fatalf("route inputs=%+v, want candidate plus complete Value vector", relation.Inputs)
		}
	}
	if !routesFound {
		t.Fatal("suspension contribution has no canonical route relation")
	}

	summaryCount := 0
	for _, projection := range contribution.Projections {
		if projection.Name != "SuspensionRouteSourceSummary" {
			continue
		}
		if projection.Role != member.Attribute || projection.Relation != "SuspensionRoutes" || projection.Result != "SourceSummaryCarrier" {
			t.Fatalf("source summary projection=%+v", projection)
		}
		summaryCount++
	}
	if summaryCount != 1 {
		t.Fatalf("canonical suspension SourceSummary projections=%d, want one owner authority", summaryCount)
	}

	// Candidate is implicit. The four explicit inputs are all scalar columns
	// of the one selected route row: summary, key, tag, and routed fact.
	reducer := contribution.Reducers[0]
	if reducer.Key != "placement/suspension/reducer" || reducer.Candidate != "SubjectLivenessCarrier" ||
		len(reducer.Inputs) != 4 || len(reducer.Outputs) != 1 {
		t.Fatalf("suspension reducer shape=%+v", reducer)
	}
	wantCarriers := []string{"SourceSummaryCarrier", "PlacementKeyCarrier", "SuspensionRouteTagCarrier", "PlacementFactCarrier"}
	for index, input := range reducer.Inputs {
		if input.Carrier != wantCarriers[index] || input.Form != member.ReadFormSelected || input.Multiplicity != member.MultiplicityOne || input.Tag == "" {
			t.Fatalf("scalar route input %d=%+v, want selected scalar %s", index, input, wantCarriers[index])
		}
	}
	if reducer.Inputs[3].Route != "PlacementKeyCarrier" {
		t.Fatalf("routed cell input=%+v, want the routed Placement cell", reducer.Inputs[3])
	}
	if reducer.Outputs[0].Carrier != "PlacementFactCarrier" {
		t.Fatalf("suspension output carrier=%q, want PlacementFactCarrier", reducer.Outputs[0].Carrier)
	}
	if reducer.Implementation.Name != "SuspensionFold" || strings.Contains(string(reducer.Key), "unknown") {
		t.Fatalf("suspension reducer identity=%+v", reducer.Implementation)
	}
}

func TestSuspensionContributionDeclaresTheAnchorJoinKey(t *testing.T) {
	contribution := Contribution()
	for _, projection := range contribution.Projections {
		if projection.Key == "value/suspension/anchor-key" {
			if projection.Relation != "SuspensionAnchors" || projection.Role != member.Key {
				t.Fatalf("anchor key projection=%+v", projection)
			}
			return
		}
	}
	t.Fatal("suspension anchor join key is not owner-declared")
}
