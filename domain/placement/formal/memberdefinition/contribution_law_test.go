package memberdefinition

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/operand"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	memberdefinition "github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/analysis/schema/rule/codegen"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	calldomain "github.com/wippyai/go-lua/domain/call"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	formal "github.com/wippyai/go-lua/domain/placement/formal"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// TestFormalContributionDeclaresOneRouteRelationAndOneReducer states what
// this rule adds to Placement's vocabulary: one dependent route relation over
// Call's mounted-call directory, the three projections that address a row of
// it, and the one reducer the fold is answered through.
func TestFormalContributionDeclaresOneRouteRelationAndOneReducer(t *testing.T) {
	contribution := Contribution()
	if !contribution.Available() {
		t.Fatal("formal contribution is unavailable")
	}
	if contribution.Axis != "placement" || contribution.Rule != "placement-formal" {
		t.Fatalf("contribution identity=%q/%q, want placement/placement-formal", contribution.Axis, contribution.Rule)
	}
	if len(contribution.Carriers) != 6 {
		t.Fatalf("carrier count=%d, want the two Placement carriers, the two Call carriers, and the route row with its tag", len(contribution.Carriers))
	}
	if len(contribution.Relations) != 1 || len(contribution.Projections) != 3 || len(contribution.Reducers) != 1 {
		t.Fatalf("relation/projection/reducer counts=%d/%d/%d, want 1/3/1", len(contribution.Relations), len(contribution.Projections), len(contribution.Reducers))
	}

	relation := contribution.Relations[0]
	provider := mountedCallProvider()
	if relation.Key != "placement/formal/routes" || relation.Subject != "FormalRouteCarrier" ||
		relation.CandidateProvider != member.AxisRelationCandidate(provider) {
		t.Fatalf("relation=%+v, want the formal route set over Call's mounted-call candidates", relation)
	}
	if len(relation.Inputs) != 3 || relation.Inputs[0].Carrier != "CallCoordinateCarrier" ||
		relation.Inputs[1].Carrier != "CallFactCarrier" ||
		relation.Inputs[2].Carrier != "ValueFactCarrier" || !relation.Inputs[2].Many || relation.Inputs[2].Form != member.ReadFormSummary {
		t.Fatalf("relation inputs=%+v, want candidate, call fact, and the whole actual vector", relation.Inputs)
	}
	if relation.MemberParent.Declared() {
		t.Fatalf("relation parent=%+v, want a keyed dependent relation rather than a member set", relation.MemberParent)
	}

	roles := map[member.Role]schemaKey{}
	for _, projection := range contribution.Projections {
		if projection.Relation != "FormalRoutes" || projection.CandidateProvider != member.AxisRelationCandidate(provider) {
			t.Fatalf("projection %q=%+v, want a FormalRoutes row under the same provider", projection.Name, projection)
		}
		if _, duplicate := roles[projection.Role]; duplicate {
			t.Fatalf("projection %q repeats role %v", projection.Name, projection.Role)
		}
		roles[projection.Role] = schemaKey{key: string(projection.Key), result: string(projection.Result)}
	}
	if roles[member.Key] != (schemaKey{key: "placement/formal/route-key", result: "PlacementKeyCarrier"}) ||
		roles[member.Predicate] != (schemaKey{key: "placement/formal/route-tag", result: "FormalRouteTagCarrier"}) ||
		roles[member.Destination] != (schemaKey{key: "placement/formal/route-destination", result: "PlacementKeyCarrier"}) {
		t.Fatalf("projection roles=%+v, want key/predicate/destination addressed separately", roles)
	}

	reducer := contribution.Reducers[0]
	if reducer.Name != "FormalReducer" || reducer.Key != "placement/formal/reducer" || reducer.Candidate != "" {
		t.Fatalf("reducer identity=%q/%q/%q", reducer.Name, reducer.Key, reducer.Candidate)
	}
	if reducer.Implementation.PackagePath != "github.com/wippyai/go-lua/domain/placement/formal" || reducer.Implementation.Name != "FormalFold" || reducer.Implementation.ResultIndex != 0 {
		t.Fatalf("implementation=%+v, want formal.FormalFold result 0", reducer.Implementation)
	}
	if len(reducer.Inputs) != 1 || len(reducer.Outputs) != 1 {
		t.Fatalf("input/output counts=%d/%d, want one/one", len(reducer.Inputs), len(reducer.Outputs))
	}
	input := reducer.Inputs[0]
	if input.Axis != placementAxis() || input.Carrier != "PlacementFactCarrier" || input.Form != member.ReadFormSelected ||
		input.Multiplicity != member.MultiplicityOne || input.Tag != "FormalRouteTagCarrier" || input.Route != "" {
		t.Fatalf("reducer input=%+v, want the selected tagged Placement fact", input)
	}
	if output := reducer.Outputs[0]; output.Axis != placementAxis() || output.Carrier != "PlacementFactCarrier" {
		t.Fatalf("reducer output=%+v, want Placement fact", output)
	}
}

// schemaKey pairs a projection's declared key with the carrier it answers, so
// one comparison states both halves of a role.
type schemaKey struct {
	key    string
	result string
}

// TestFormalContributionDerivesTheAuthoredSymbolSignatures proves the
// declaration alone fixes the shape of every symbol this package still
// authors: the fold, and the three halves of the route derivation.
func TestFormalContributionDerivesTheAuthoredSymbolSignatures(t *testing.T) {
	contribution := Contribution()
	reducer := contribution.Reducers[0]
	carrierDefinition := memberdefinition.Definition{Carriers: contribution.Carriers}
	args, results, ok := carrierDefinition.ReducerSignature(
		reducer,
		memberdefinition.GoType{PackagePath: "github.com/wippyai/go-lua/analysis/schema/structure", Name: "ReductionOutcome"},
		codegen.SelectionCellType,
		codegen.SummaryVectorType,
	)
	if !ok || len(args) != 2 || args[0].Type.Name != "uint64" || args[1].Type.Name != "Fact" ||
		len(results) != 2 || results[0].Name != "Fact" || results[1].Name != "ReductionOutcome" {
		t.Fatalf("derived fold signature args=%+v results=%+v ok=%t", args, results, ok)
	}
	var _ func(uint64, placementdomain.Fact) (placementdomain.Fact, structure.ReductionOutcome) = formal.FormalFold
	var _ func(placementdomain.Schema, *valuedomain.Schema, *calldomain.Algebra, *packdomain.Schema, calldomain.CallCoordinate, calldomain.Value, operand.SummaryVector[valuedomain.Value]) (formal.RoutePlan, bool) = formal.DeriveFormalRoutes
	var _ func(formal.RoutePlan) int = formal.FormalRouteCount
	var _ func(formal.RoutePlan, int) (formal.Route, bool) = formal.FormalRouteAt
}
