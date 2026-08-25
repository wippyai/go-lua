package memberdefinition

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

// TestContributionDeclaresOneExactCallOwnedFold states the shape dispatch
// folds through: the mounted application it is indexed by, the one exact Value
// image of that application's callee, and the Call fact it answers. The
// alternatives that callee reaches are the judgment's own, so the declaration
// carries no route relation, predicate tag or destination projection - the
// three authorities the judgment rests on are named as its sealed state.
func TestContributionDeclaresOneExactCallOwnedFold(t *testing.T) {
	contribution := Contribution()
	if contribution.Axis != "call" || contribution.Rule != "call-dispatch" ||
		len(contribution.Relations) != 0 || len(contribution.Projections) != 0 || len(contribution.Reducers) != 1 {
		t.Fatalf("dispatch contribution = axis:%q rule:%q relations:%d projections:%d reducers:%d",
			contribution.Axis, contribution.Rule, len(contribution.Relations), len(contribution.Projections), len(contribution.Reducers))
	}
	reducer := contribution.Reducers[0]
	if reducer.Key != "call/dispatch/reducer" || reducer.Candidate != "CallCoordinateCarrier" || len(reducer.Inputs) != 1 {
		t.Fatalf("dispatch reducer = %+v", reducer)
	}
	input := reducer.Inputs[0]
	if input.Form != member.ReadFormExact || input.Multiplicity != member.MultiplicityOne || input.Carrier != "ValueFactCarrier" {
		t.Fatalf("dispatch reducer input = %+v", input)
	}
	if input.Tag != "" || input.Route != "" {
		t.Fatalf("an exact dispatch fold declares a tag or route carrier: %+v", input)
	}
	if len(reducer.Derivation.StaticAxes) != 3 || reducer.Derivation.State.Name != "Judgment" || reducer.Derivation.Build.Name != "Derive" {
		t.Fatalf("dispatch judgment state = %+v", reducer.Derivation)
	}
	if reducer.Implementation.Name != "Dispatch" || reducer.Implementation.Receiver.Name != "Judgment" {
		t.Fatalf("dispatch implementation = %+v", reducer.Implementation)
	}
}
