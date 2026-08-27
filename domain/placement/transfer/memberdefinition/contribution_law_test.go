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
	transfer "github.com/wippyai/go-lua/domain/placement/transfer"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// TestTransferContributionDeclaresOneRouteRelationAndOneReducer states what
// this rule adds to Placement's vocabulary: one dependent route relation over
// Call's mounted-call directory, the three projections that address a row of
// it, and the one reducer the fold is answered through.
func TestTransferContributionDeclaresOneRouteRelationAndOneReducer(t *testing.T) {
	contribution := Contribution()
	if !contribution.Available() {
		t.Fatal("transfer contribution is unavailable")
	}
	if contribution.Axis != "placement" || contribution.Rule != "placement-transfer" {
		t.Fatalf("contribution identity=%q/%q, want placement/placement-transfer", contribution.Axis, contribution.Rule)
	}
	if len(contribution.Carriers) != 4 || len(contribution.CarrierRefs) != 2 {
		t.Fatalf("carrier authorities=%d/imports=%d, want four local Placement carriers and two imported Call carriers", len(contribution.Carriers), len(contribution.CarrierRefs))
	}
	for index, want := range []struct {
		name string
		key  string
	}{
		{name: "CallCoordinateCarrier", key: "carrier/call/mounted-call"},
		{name: "CallFactCarrier", key: "carrier/call/fact"},
	} {
		ref := contribution.CarrierRefs[index]
		if ref.Name != want.name || string(ref.Key) != want.key || ref.Ref.Owner.Key != "call" || string(ref.Ref.Carrier) != want.key {
			t.Fatalf("carrier ref[%d]=%+v, want Call-owned %s/%s", index, ref, want.name, want.key)
		}
	}
	if len(contribution.Relations) != 1 || len(contribution.Projections) != 3 || len(contribution.Reducers) != 1 {
		t.Fatalf("relation/projection/reducer counts=%d/%d/%d, want 1/3/1", len(contribution.Relations), len(contribution.Projections), len(contribution.Reducers))
	}

	relation := contribution.Relations[0]
	provider := mountedCallProvider()
	if relation.Key != "placement/transfer/routes" || relation.Subject != "TransferRouteCarrier" ||
		relation.CandidateProvider != member.AxisRelationCandidate(provider) {
		t.Fatalf("relation=%+v, want the transfer route set over Call's mounted-call candidates", relation)
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
		if projection.Relation != "TransferRoutes" || projection.CandidateProvider != member.AxisRelationCandidate(provider) {
			t.Fatalf("projection %q=%+v, want a TransferRoutes row under the same provider", projection.Name, projection)
		}
		if _, duplicate := roles[projection.Role]; duplicate {
			t.Fatalf("projection %q repeats role %v", projection.Name, projection.Role)
		}
		roles[projection.Role] = schemaKey{key: string(projection.Key), result: string(projection.Result)}
	}
	if roles[member.Key] != (schemaKey{key: "placement/transfer/route-key", result: "PlacementKeyCarrier"}) ||
		roles[member.Predicate] != (schemaKey{key: "placement/transfer/route-tag", result: "TransferRouteTagCarrier"}) ||
		roles[member.Destination] != (schemaKey{key: "placement/transfer/route-destination", result: "PlacementKeyCarrier"}) {
		t.Fatalf("projection roles=%+v, want key/predicate/destination addressed separately", roles)
	}

	reducer := contribution.Reducers[0]
	if reducer.Name != "TransferReducer" || reducer.Key != "placement/transfer/reducer" || reducer.Candidate != "" {
		t.Fatalf("reducer identity=%q/%q/%q", reducer.Name, reducer.Key, reducer.Candidate)
	}
	if reducer.Implementation.PackagePath != "github.com/wippyai/go-lua/domain/placement/transfer" || reducer.Implementation.Name != "TransferFold" || reducer.Implementation.ResultIndex != 0 {
		t.Fatalf("implementation=%+v, want transfer.TransferFold result 0", reducer.Implementation)
	}
	if len(reducer.Inputs) != 1 || len(reducer.Outputs) != 1 {
		t.Fatalf("input/output counts=%d/%d, want one/one", len(reducer.Inputs), len(reducer.Outputs))
	}
	input := reducer.Inputs[0]
	if input.Axis != placementAxis() || input.Carrier != "PlacementFactCarrier" || input.Form != member.ReadFormSelected ||
		input.Multiplicity != member.MultiplicityOne || input.Tag != "TransferRouteTagCarrier" || input.Route != "" {
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

// TestTransferContributionDerivesTheAuthoredSymbolSignatures proves the
// declaration alone fixes the shape of every symbol this package still
// authors: the fold, and the three halves of the route derivation.
func TestTransferContributionDerivesTheAuthoredSymbolSignatures(t *testing.T) {
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
	var _ func(uint64, placementdomain.Fact) (placementdomain.Fact, structure.ReductionOutcome) = transfer.TransferFold
	var _ func(placementdomain.Schema, *valuedomain.Schema, *calldomain.Algebra, *packdomain.Schema, calldomain.CallCoordinate, calldomain.Value, operand.SummaryVector[valuedomain.Value]) (transfer.RoutePlan, bool) = transfer.DeriveTransferRoutes
	var _ func(transfer.RoutePlan) int = transfer.TransferRouteCount
	var _ func(transfer.RoutePlan, int) (transfer.Route, bool) = transfer.TransferRouteAt
}
