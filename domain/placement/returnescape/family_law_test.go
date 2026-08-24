package returnescape

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/generated"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

func returnEscapeFamilyLawSpec() generated.CompiledRuleSpec {
	valueContract := ruleplan.ReadContract{
		Order:        ruleprogram.OrderCanonical,
		Sparse:       ruleprogram.SparseDefault,
		OnOpaque:     ruleprogram.OnOpaquePropagateAuthenticated,
		Multiplicity: ruleprogram.MultiplicityOne,
	}
	placementContract := valueContract
	placementContract.OnOpaque = ruleprogram.OnOpaqueRefuse
	return generated.CompiledRuleSpec{
		Ordinal:    7,
		AxisCount:  3,
		InputCount: 1,
		Candidate:  ruleplan.RelationAddr{Axis: 1, Member: 0},
		Reducer:    ruleplan.ReducerAddr{Axis: 2, Member: 0},
		Reads: []generated.ReadPlan{
			{
				Input: 0, Factor: 1, Axis: 1,
				Sources:     ruleplan.Span{Start: 0, Count: 1},
				Relation:    ruleplan.RelationAddr{Axis: 1, Member: 1},
				Key:         ruleplan.ProjectionAddr{Axis: 1, Member: 1},
				Form:        ruleprogram.Exact,
				Contract:    valueContract,
				Denominator: ruleplan.DenominatorAddr{Ordinal: 0, Present: true},
				RowCapacity: 1, CellCapacity: 1,
			},
			{
				Input: 0, Factor: 1, Axis: 1,
				Sources:     ruleplan.Span{Start: 1, Count: 1},
				Relation:    ruleplan.RelationAddr{Axis: 1, Member: 2},
				Key:         ruleplan.ProjectionAddr{Axis: 1, Member: 2},
				Form:        ruleprogram.Selected,
				Contract:    valueContract,
				Denominator: ruleplan.DenominatorAddr{Ordinal: 1, Present: true},
				RowCapacity: 1, CellCapacity: 1,
			},
			{
				Input: 0, Factor: 2, Axis: 2,
				Sources:     ruleplan.Span{Start: 2, Count: 1},
				Relation:    ruleplan.RelationAddr{Axis: 2, Member: 4},
				Key:         ruleplan.ProjectionAddr{Axis: 2, Member: 4},
				Form:        ruleprogram.Selected,
				Contract:    placementContract,
				Denominator: ruleplan.DenominatorAddr{Ordinal: 2, Present: true},
				RowCapacity: 1, CellCapacity: 1,
			},
		},
		Outputs: []generated.OutputPlan{{
			Factor: 2, Axis: 2,
			Address:     ruleplan.OutputAddr{Axis: 2, Frame: 0},
			Destination: ruleplan.ProjectionAddr{Axis: 2, Member: 6},
			Mode:        ruleprogram.ModeRoute, Slot: 0,
			RouteJoin: 2, RouteJoinPresent: true,
		}},
	}
}

func TestReturnEscapeFamilyClaimsOnlyTheNoCarryRouteShape(t *testing.T) {
	spec := returnEscapeFamilyLawSpec()
	rule, ok := generated.NewPlanCompiledRule(spec)
	if !ok || !returnEscapeRuleShape(rule) {
		t.Fatalf("canonical ReturnEscape descriptor refused: %+v/%t", rule, ok)
	}

	spec.Carry = &generated.CarryPlan{Input: 0, Factor: 2, Mode: ruleprogram.CarryIdentity, Identity: true}
	withCarry, carryOK := generated.NewPlanCompiledRule(spec)
	if !carryOK || returnEscapeRuleShape(withCarry) {
		t.Fatalf("identity carry was admitted by ReturnEscape family: %+v/%t", withCarry, carryOK)
	}

	spec = returnEscapeFamilyLawSpec()
	spec.Outputs[0].RouteJoin = 1
	spec.Outputs[0].Destination.Axis = 1
	wrongRoute, wrongRouteOK := generated.NewPlanCompiledRule(spec)
	if !wrongRouteOK || returnEscapeRuleShape(wrongRoute) {
		t.Fatalf("route join other than join2 was admitted: %+v/%t", wrongRoute, wrongRouteOK)
	}
}
