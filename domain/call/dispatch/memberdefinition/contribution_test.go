package memberdefinition

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

func TestContributionDeclaresOneCallOwnedRouteAndReducer(t *testing.T) {
	contribution := Contribution()
	if contribution.Axis != "call" || contribution.Rule != "call-dispatch" || len(contribution.Relations) != 1 || len(contribution.Projections) != 3 || len(contribution.Reducers) != 1 {
		t.Fatalf("dispatch contribution = axis:%q rule:%q relations:%d projections:%d reducers:%d", contribution.Axis, contribution.Rule, len(contribution.Relations), len(contribution.Projections), len(contribution.Reducers))
	}
	relation := contribution.Relations[0]
	if relation.Key != "call/dispatch/routes" || len(relation.Inputs) != 2 || len(relation.Derivation.StaticAxes) != 3 || relation.CandidateProvider != member.AxisRelationCandidate(mountedCallProvider()) {
		t.Fatalf("dispatch relation = key:%q inputs:%d static:%d provider:%+v", relation.Key, len(relation.Inputs), len(relation.Derivation.StaticAxes), relation.CandidateProvider)
	}
	reducer := contribution.Reducers[0]
	if reducer.Key != "call/dispatch/reducer" || reducer.Candidate != "CallCoordinateCarrier" || len(reducer.Inputs) != 1 || reducer.Inputs[0].Form != member.ReadFormSelected || reducer.Inputs[0].Tag != "DispatchRouteTagCarrier" || reducer.Inputs[0].Route != "CallKeyCarrier" {
		t.Fatalf("dispatch reducer = %+v", reducer)
	}
}
