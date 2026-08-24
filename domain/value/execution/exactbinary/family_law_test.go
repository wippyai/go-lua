package exactbinary

import (
	"testing"

	engineexecution "github.com/wippyai/go-lua/analysis/engine/execution"
	"github.com/wippyai/go-lua/analysis/engine/generated"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

type absentAuthorities struct{}

func (absentAuthorities) ValueAuthority() *valueowner.HotOwner { return nil }
func (absentAuthorities) ValueSchema() *valuedomain.Schema     { return nil }

func TestExactBinaryInstallerRequiresASealedValueOwner(t *testing.T) {
	if InstallFamily(nil, nil, absentAuthorities{}) {
		t.Fatal("exact-binary installer admitted without a sealed Value owner")
	}
	if executor := (&family{}).NewExecutor(nil); executor != nil {
		t.Fatal("exact-binary family minted an executor without sealed state")
	}
}

func TestExactBinaryShapeIsOneStrictSharedProgram(t *testing.T) {
	rule := exactBinaryLawRule(t)
	if !exactBinaryShape(rule, engineexecution.FormExact) {
		t.Fatal("valid same-axis exact-binary shape was refused")
	}

	for _, testCase := range []struct {
		name string
		make func(*generated.CompiledRuleSpec)
	}{
		{name: "one read", make: func(spec *generated.CompiledRuleSpec) {
			spec.Reads = spec.Reads[:1]
		}},
		{name: "foreign read axis", make: func(spec *generated.CompiledRuleSpec) {
			spec.Reads[1].Axis = 1
		}},
		{name: "optional multiplicity", make: func(spec *generated.CompiledRuleSpec) {
			spec.Reads[0].Contract.Multiplicity = ruleprogram.MultiplicityOptional
		}},
		{name: "no carry", make: func(spec *generated.CompiledRuleSpec) {
			spec.Carry = nil
		}},
	} {
		candidate := exactBinaryRuleWith(t, testCase.make)
		if exactBinaryShape(candidate, engineexecution.FormExact) {
			t.Fatalf("malformed %s shape admitted", testCase.name)
		}
	}
}

func TestExactBinaryShapeHotProbeAllocatesNothing(t *testing.T) {
	rule := exactBinaryLawRule(t)
	allocations := testing.AllocsPerRun(200, func() {
		if !exactBinaryShape(rule, engineexecution.FormExact) {
			t.Fatal("valid shape became unavailable")
		}
	})
	if allocations != 0 {
		t.Fatalf("warm exact-binary shape probe allocated %v times", allocations)
	}
}

func exactBinaryLawRule(t *testing.T) generated.CompiledRule {
	t.Helper()
	return exactBinaryRuleWith(t, func(*generated.CompiledRuleSpec) {})
}

func exactBinaryRuleWith(t *testing.T, amend func(*generated.CompiledRuleSpec)) generated.CompiledRule {
	t.Helper()
	contract := ruleplan.ReadContract{
		Order:        ruleprogram.OrderCanonical,
		Sparse:       ruleprogram.SparseExplicit,
		OnOpaque:     ruleprogram.OnOpaqueRefuse,
		Multiplicity: ruleprogram.MultiplicityOne,
	}
	axis := uint32(2)
	spec := generated.CompiledRuleSpec{
		Ordinal: 11, AxisCount: 3, InputCount: 1,
		Candidate: ruleplan.RelationAddr{Axis: axis, Member: 0},
		Reducer:   ruleplan.ReducerAddr{Axis: axis, Member: 3},
		Reads: []generated.ReadPlan{
			{
				Input: 0, Factor: axis, Axis: axis,
				Relation: ruleplan.RelationAddr{Axis: axis, Member: 0}, Key: ruleplan.ProjectionAddr{Axis: axis, Member: 0},
				Form: ruleprogram.Exact, Contract: contract, RowCapacity: 2, CellCapacity: 1,
			},
			{
				Input: 0, Factor: axis, Axis: axis,
				Relation: ruleplan.RelationAddr{Axis: axis, Member: 1}, Key: ruleplan.ProjectionAddr{Axis: axis, Member: 1},
				Form: ruleprogram.Exact, Contract: contract, RowCapacity: 2, CellCapacity: 1,
			},
		},
		Outputs: []generated.OutputPlan{{
			Factor: axis, Axis: axis, Address: ruleplan.OutputAddr{Axis: axis, Frame: 0},
			Destination: ruleplan.ProjectionAddr{Axis: axis, Member: 2}, Mode: ruleprogram.ModeExact, Slot: 0, Exact: true, Strong: true,
		}},
		Carry: &generated.CarryPlan{Input: 0, Factor: axis, Mode: ruleprogram.CarryIdentity, Identity: true},
	}
	amend(&spec)
	rule, ok := generated.NewPlanCompiledRule(spec)
	if !ok {
		t.Fatal("sealed exact-binary law descriptor")
	}
	return rule
}
