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

func TestExactBinaryMappingRefusesNearestNegativeMembers(t *testing.T) {
	rule := exactBinaryLawRule(t)
	mapping := valuedomain.ExactBinaryMapping{
		ReducerOrdinal:              rule.Reducer().Member,
		CandidateRelationMember:     rule.CandidateRelation().Member,
		Read0RelationMember:         0,
		Read0KeyMember:              0,
		Read1RelationMember:         1,
		Read1KeyMember:              1,
		DestinationProjectionMember: 2,
	}
	if !exactBinaryMappingMatches(rule, mapping) {
		t.Fatal("valid generated member mapping was refused")
	}
	for _, testCase := range []struct {
		name   string
		mutate func(*valuedomain.ExactBinaryMapping)
	}{
		{name: "candidate relation member", mutate: func(mapping *valuedomain.ExactBinaryMapping) {
			mapping.CandidateRelationMember++
		}},
		{name: "first read relation member", mutate: func(mapping *valuedomain.ExactBinaryMapping) {
			mapping.Read0RelationMember++
		}},
		{name: "first read key member", mutate: func(mapping *valuedomain.ExactBinaryMapping) {
			mapping.Read0KeyMember++
		}},
		{name: "second read relation member", mutate: func(mapping *valuedomain.ExactBinaryMapping) {
			mapping.Read1RelationMember++
		}},
		{name: "second read key member", mutate: func(mapping *valuedomain.ExactBinaryMapping) {
			mapping.Read1KeyMember++
		}},
		{name: "destination projection member", mutate: func(mapping *valuedomain.ExactBinaryMapping) {
			mapping.DestinationProjectionMember++
		}},
		{name: "reducer mapping", mutate: func(mapping *valuedomain.ExactBinaryMapping) {
			mapping.ReducerOrdinal++
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			candidate := mapping
			testCase.mutate(&candidate)
			if exactBinaryMappingMatches(rule, candidate) {
				t.Fatalf("nearest-negative %s mapping admitted", testCase.name)
			}
		})
	}
}

func TestExactBinaryShapeRefusesNearestNegativeFactorAndOutputStrength(t *testing.T) {
	for _, testCase := range []struct {
		name string
		make func(*generated.CompiledRuleSpec)
	}{
		{name: "factor", make: func(spec *generated.CompiledRuleSpec) {
			spec.Reads[0].Factor = 1
		}},
		{name: "output slot", make: func(spec *generated.CompiledRuleSpec) {
			spec.Outputs[0].Slot = 1
		}},
		{name: "output strength", make: func(spec *generated.CompiledRuleSpec) {
			spec.Outputs[0].Strong = false
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			spec := exactBinaryLawSpec()
			testCase.make(&spec)
			candidate, sealed := generated.NewPlanCompiledRule(spec)
			if testCase.name == "factor" {
				if !sealed || exactBinaryShape(candidate, engineexecution.FormExact) {
					t.Fatalf("nearest-negative %s admitted", testCase.name)
				}
				return
			}
			if sealed {
				t.Fatalf("nearest-negative %s admitted", testCase.name)
			}
		})
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
	spec := exactBinaryLawSpec()
	amend(&spec)
	rule, ok := generated.NewPlanCompiledRule(spec)
	if !ok {
		t.Fatal("sealed exact-binary law descriptor")
	}
	return rule
}

func exactBinaryLawSpec() generated.CompiledRuleSpec {
	contract := ruleplan.ReadContract{
		Order:        ruleprogram.OrderCanonical,
		Sparse:       ruleprogram.SparseExplicit,
		OnOpaque:     ruleprogram.OnOpaqueRefuse,
		Multiplicity: ruleprogram.MultiplicityOne,
	}
	axis := uint32(2)
	return generated.CompiledRuleSpec{
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
}
