package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/generated"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

func TestGeneratedRelationOwnerCompletenessWalksEveryRead(t *testing.T) {
	contract := ruleplan.ReadContract{Order: ruleprogram.OrderCanonical, Sparse: ruleprogram.SparseExplicit, OnOpaque: ruleprogram.OnOpaqueRefuse, Multiplicity: ruleprogram.MultiplicityOne}
	descriptor, ok := generated.NewPlanCompiledRule(generated.CompiledRuleSpec{
		AxisCount: 1, InputCount: 1,
		Candidate: ruleplan.RelationAddr{Axis: 0, Member: 1},
		Reducer:   ruleplan.ReducerAddr{Axis: 0, Member: 1},
		Reads: []generated.ReadPlan{
			{Input: 0, Factor: 0, Axis: 0, Relation: ruleplan.RelationAddr{Axis: 0, Member: 1}, Key: ruleplan.ProjectionAddr{Axis: 0, Member: 1}, Addressing: ruleplan.RelationAddr{Axis: 0, Member: 1}, AddressingPresent: true, Form: ruleprogram.Exact, PointBound: ruleprogram.PointBound, Contract: contract, RowCapacity: 1, CellCapacity: 1},
			{Input: 0, Factor: 0, Axis: 0, Relation: ruleplan.RelationAddr{Axis: 0, Member: 2}, Key: ruleplan.ProjectionAddr{Axis: 0, Member: 2}, Addressing: ruleplan.RelationAddr{Axis: 0, Member: 1}, AddressingPresent: true, Form: ruleprogram.Exact, PointBound: ruleprogram.PointBound, Contract: contract, RowCapacity: 1, CellCapacity: 1},
		},
		Outputs: []generated.OutputPlan{{Factor: 0, Axis: 0, Address: ruleplan.OutputAddr{Axis: 0, Frame: 0}, Destination: ruleplan.ProjectionAddr{Axis: 0, Member: 3}, Mode: ruleprogram.ModeExact, Exact: true, Strong: true}},
	})
	if !ok {
		t.Fatal("two-read descriptor")
	}
	axes, ok := generatedRelationOwnerAxes(descriptor)
	if !ok {
		t.Fatal("owner-axis census")
	}
	if len(axes) != 11 {
		t.Fatalf("owner completeness returned %d address axes, want five fixed plus every field of two reads", len(axes))
	}
}

// newExactIdentityDescriptor is a test fixture for the legacy invocation laws.
// Production descriptors now have exactly one schema-owned table constructor;
// the fixture supplies the smallest valid plan tables directly.
func newExactIdentityDescriptor(rule uint32, inputCount, readInput int, readFactor, outputFactor uint32, rowCapacity, cellCapacity int) (generated.CompiledRule, bool) {
	if rowCapacity < 0 || cellCapacity < 0 || rowCapacity > int(^uint16(0)) || cellCapacity > int(^uint16(0)) {
		return generated.CompiledRule{}, false
	}
	return generated.NewPlanCompiledRule(generated.CompiledRuleSpec{
		AxisCount: 1, InputCount: inputCount,
		Candidate: ruleplan.RelationAddr{Axis: 0, Member: 0},
		Reducer:   ruleplan.ReducerAddr{Axis: 0, Member: 0},
		Reads: []generated.ReadPlan{{
			Input: uint32(readInput), Factor: readFactor, Axis: 0,
			Relation:   ruleplan.RelationAddr{Axis: 0, Member: 0},
			Key:        ruleplan.ProjectionAddr{Axis: 0, Member: 0},
			Addressing: ruleplan.RelationAddr{Axis: 0, Member: 0}, AddressingPresent: true,
			Form:        ruleprogram.Exact,
			PointBound:  ruleprogram.PointBound,
			Contract:    ruleplan.ReadContract{Order: ruleprogram.OrderCanonical, Sparse: ruleprogram.SparseExplicit, OnOpaque: ruleprogram.OnOpaqueRefuse, Multiplicity: ruleprogram.MultiplicityOne},
			RowCapacity: uint16(rowCapacity), CellCapacity: uint16(cellCapacity),
		}},
		Outputs: []generated.OutputPlan{{
			Factor: outputFactor, Axis: 0,
			Address:     ruleplan.OutputAddr{Axis: 0, Frame: 0},
			Destination: ruleplan.ProjectionAddr{Axis: 0, Member: 0},
			Mode:        ruleprogram.ModeExact, Exact: true, Strong: true,
		}},
		Carry: &generated.CarryPlan{Input: uint32(readInput), Factor: outputFactor, Mode: ruleprogram.CarryIdentity, Identity: true},
	})
}
