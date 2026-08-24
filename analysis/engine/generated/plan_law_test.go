package generated

import (
	"testing"

	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

func planLawSpec(reads []ReadPlan, carry *CarryPlan, inputCount int) CompiledRuleSpec {
	return CompiledRuleSpec{
		AxisCount:  3,
		InputCount: inputCount,
		Candidate:  ruleplan.RelationAddr{Axis: 0, Member: 0},
		Reducer:    ruleplan.ReducerAddr{Axis: 2, Member: 0},
		Reads:      reads,
		Outputs: []OutputPlan{{
			Factor: 2, Axis: 2, Address: ruleplan.OutputAddr{Axis: 2, Frame: 0},
			Destination: ruleplan.ProjectionAddr{Axis: 0, Member: 0}, Mode: ruleprogram.ModeExact, Exact: true, Strong: true,
		}},
		Carry: carry,
	}
}

func TestPlanExecutorTableAdmitsZeroInputDirectOutput(t *testing.T) {
	descriptor, ok := NewPlanCompiledRule(planLawSpec([]ReadPlan{}, nil, 0))
	if !ok || !descriptor.Available() || descriptor.InputCount() != 0 || descriptor.ReadCount() != 0 || descriptor.OutputCount() != 1 || descriptor.CarryInput() != -1 {
		t.Fatalf("zero-input table = %+v/%t inputs=%d reads=%d outputs=%d carry=%d", descriptor, ok, descriptor.InputCount(), descriptor.ReadCount(), descriptor.OutputCount(), descriptor.CarryInput())
	}
}

func TestPlanExecutorTableRetainsHeterogeneousReadAndOutputFactors(t *testing.T) {
	read := ReadPlan{
		Input: 0, Factor: 1, Axis: 0,
		Relation: ruleplan.RelationAddr{Axis: 0, Member: 0}, Key: ruleplan.ProjectionAddr{Axis: 0, Member: 0},
		Addressing: ruleplan.RelationAddr{Axis: 0, Member: 0}, AddressingPresent: true,
		Form: ruleprogram.Exact, PointBound: ruleprogram.PointBound, Contract: ruleplan.ReadContract{Order: ruleprogram.OrderCanonical, Sparse: ruleprogram.SparseExplicit, OnOpaque: ruleprogram.OnOpaqueRefuse, Multiplicity: ruleprogram.MultiplicityOne},
		RowCapacity: 1, CellCapacity: 1,
	}
	descriptor, ok := NewPlanCompiledRule(planLawSpec([]ReadPlan{read}, &CarryPlan{Input: 0, Factor: 2, Mode: ruleprogram.CarryIdentity, Identity: true}, 1))
	if !ok || !descriptor.Available() || descriptor.ReadFactor() != 1 || descriptor.OutputFactor() != 2 || descriptor.ReadFactor() == descriptor.OutputFactor() {
		t.Fatalf("heterogeneous table = %+v/%t read=%d output=%d", descriptor, ok, descriptor.ReadFactor(), descriptor.OutputFactor())
	}
}

func TestPlanExecutorTableRefusesHoleyInputPorts(t *testing.T) {
	read := ReadPlan{
		Input: 1, Factor: 1, Axis: 0,
		Relation: ruleplan.RelationAddr{Axis: 0, Member: 0}, Key: ruleplan.ProjectionAddr{Axis: 0, Member: 0},
		Addressing: ruleplan.RelationAddr{Axis: 0, Member: 0}, AddressingPresent: true,
		Form: ruleprogram.Exact, PointBound: ruleprogram.PointBound, Contract: ruleplan.ReadContract{Order: ruleprogram.OrderCanonical, Sparse: ruleprogram.SparseExplicit, OnOpaque: ruleprogram.OnOpaqueRefuse, Multiplicity: ruleprogram.MultiplicityOne},
		RowCapacity: 1, CellCapacity: 1,
	}
	if descriptor, ok := NewPlanCompiledRule(planLawSpec([]ReadPlan{read}, &CarryPlan{Input: 1, Factor: 2, Mode: ruleprogram.CarryIdentity, Identity: true}, 2)); ok || descriptor.Available() {
		t.Fatal("holey input prefix admitted")
	}
}

func TestPlanExecutorTableCopiesSealRows(t *testing.T) {
	reads := []ReadPlan{{
		Input: 0, Factor: 1, Axis: 0,
		Relation: ruleplan.RelationAddr{Axis: 0, Member: 0}, Key: ruleplan.ProjectionAddr{Axis: 0, Member: 0},
		Addressing: ruleplan.RelationAddr{Axis: 0, Member: 0}, AddressingPresent: true,
		Form: ruleprogram.Exact, PointBound: ruleprogram.PointBound, Contract: ruleplan.ReadContract{Order: ruleprogram.OrderCanonical, Sparse: ruleprogram.SparseExplicit, OnOpaque: ruleprogram.OnOpaqueRefuse, Multiplicity: ruleprogram.MultiplicityOne},
		RowCapacity: 1, CellCapacity: 1,
	}}
	descriptor, ok := NewPlanCompiledRule(planLawSpec(reads, &CarryPlan{Input: 0, Factor: 2, Mode: ruleprogram.CarryIdentity, Identity: true}, 1))
	if !ok {
		t.Fatal("seal table")
	}
	reads[0].Input = 9
	read, readOK := descriptor.ReadAt(0)
	if !readOK || read.Input != 0 {
		t.Fatalf("sealed read changed through source slice: %+v/%t", read, readOK)
	}
}
