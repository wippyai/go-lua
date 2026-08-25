package memberdefinition

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

// The suspension consumer's contribution is one rule's statement: the two
// vectors it reads, the operations that publish them, and the fold that draws
// Placement class from them.
func TestSuspensionContributionDeclaresItsVectorsAndOneFold(t *testing.T) {
	contribution := Contribution()
	if !contribution.Available() || contribution.Axis != "placement" || contribution.Rule != "placement-suspension" {
		t.Fatalf("suspension contribution identity=%q/%q available=%t", contribution.Axis, contribution.Rule, contribution.Available())
	}
	if len(contribution.Relations) != 2 || len(contribution.Projections) != 5 || len(contribution.Selections) != 2 || len(contribution.Reducers) != 1 {
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

	// Two inputs, not three: the anchor read states the denominator the source
	// vector is complete against, and a denominator is not a fold input. What
	// the fold takes is the vector and the routed cell it publishes into.
	reducer := contribution.Reducers[0]
	if reducer.Key != "placement/suspension/reducer" || reducer.Candidate != "SubjectLivenessCarrier" ||
		len(reducer.Inputs) != 2 || len(reducer.Outputs) != 1 {
		t.Fatalf("suspension reducer shape=%+v", reducer)
	}
	if reducer.Inputs[0].Form != member.ReadFormSummary || reducer.Inputs[0].Multiplicity != member.MultiplicityMany || reducer.Inputs[0].Tag == "" {
		t.Fatalf("source vector input=%+v, want a tagged whole-vector delivery", reducer.Inputs[0])
	}
	if reducer.Inputs[1].Form != member.ReadFormSelected || reducer.Inputs[1].Carrier != "PlacementFactCarrier" ||
		reducer.Inputs[1].Route != "PlacementKeyCarrier" || reducer.Inputs[1].Tag == "" {
		t.Fatalf("routed cell input=%+v, want the routed Placement cell", reducer.Inputs[1])
	}
	if reducer.Outputs[0].Carrier != "PlacementFactCarrier" {
		t.Fatalf("suspension output carrier=%q, want PlacementFactCarrier", reducer.Outputs[0].Carrier)
	}
	if reducer.Implementation.Name != "SuspensionFold" || strings.Contains(string(reducer.Key), "unknown") {
		t.Fatalf("suspension reducer identity=%+v", reducer.Implementation)
	}
}
