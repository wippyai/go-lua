package exactfold

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

func TestExactFoldInstallerRequiresASealedValueOwner(t *testing.T) {
	if InstallFamily(nil, nil, absentAuthorities{}) {
		t.Fatal("exact fold installer admitted without a sealed Value owner")
	}
	if executor := (&family{}).NewExecutor(nil); executor != nil {
		t.Fatal("exact fold family minted an executor without sealed state")
	}
}

func TestExactFoldShapeIsOneStrictSharedProgramAtEveryArity(t *testing.T) {
	for arity := 1; arity <= valuedomain.ExactFoldArity; arity++ {
		rule := exactFoldLawRule(t, arity)
		count, ok := exactFoldShape(rule, engineexecution.FormExact)
		if !ok || count != arity {
			t.Fatalf("valid same-axis exact fold shape of arity %d was refused: count=%d ok=%v", arity, count, ok)
		}
	}

	for _, testCase := range []struct {
		name  string
		arity int
		make  func(*generated.CompiledRuleSpec)
	}{
		{name: "read beyond the sealed arity", arity: 3, make: func(spec *generated.CompiledRuleSpec) {
			spec.Reads = append(spec.Reads, spec.Reads[0])
		}},
		{name: "foreign read axis", arity: 2, make: func(spec *generated.CompiledRuleSpec) {
			spec.Reads[1].Axis = 1
		}},
		{name: "foreign read factor", arity: 2, make: func(spec *generated.CompiledRuleSpec) {
			spec.Reads[1].Factor = 1
		}},
		{name: "optional multiplicity", arity: 2, make: func(spec *generated.CompiledRuleSpec) {
			spec.Reads[0].Contract.Multiplicity = ruleprogram.MultiplicityOptional
		}},
		{name: "no carry", arity: 2, make: func(spec *generated.CompiledRuleSpec) {
			spec.Carry = nil
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			candidate := exactFoldRuleWith(t, testCase.arity, testCase.make)
			if _, ok := exactFoldShape(candidate, engineexecution.FormExact); ok {
				t.Fatalf("malformed %s shape admitted", testCase.name)
			}
		})
	}
}

func TestExactFoldMappingRefusesNearestNegativeMembers(t *testing.T) {
	rule := exactFoldLawRule(t, 2)
	mapping := exactFoldLawMapping(rule, 2)
	if !exactFoldMappingMatches(rule, mapping) {
		t.Fatal("valid generated member mapping was refused")
	}
	for _, testCase := range []struct {
		name   string
		mutate func(*valuedomain.ExactFoldMapping)
	}{
		{name: "candidate relation member", mutate: func(mapping *valuedomain.ExactFoldMapping) {
			mapping.CandidateRelationMember++
		}},
		{name: "first read relation member", mutate: func(mapping *valuedomain.ExactFoldMapping) {
			mapping.ReadRelationMember[0]++
		}},
		{name: "first read key member", mutate: func(mapping *valuedomain.ExactFoldMapping) {
			mapping.ReadKeyMember[0]++
		}},
		{name: "second read relation member", mutate: func(mapping *valuedomain.ExactFoldMapping) {
			mapping.ReadRelationMember[1]++
		}},
		{name: "second read key member", mutate: func(mapping *valuedomain.ExactFoldMapping) {
			mapping.ReadKeyMember[1]++
		}},
		{name: "exchanged read positions", mutate: func(mapping *valuedomain.ExactFoldMapping) {
			mapping.ReadKeyMember[0], mapping.ReadKeyMember[1] = mapping.ReadKeyMember[1], mapping.ReadKeyMember[0]
		}},
		{name: "destination projection member", mutate: func(mapping *valuedomain.ExactFoldMapping) {
			mapping.DestinationProjectionMember++
		}},
		{name: "reducer mapping", mutate: func(mapping *valuedomain.ExactFoldMapping) {
			mapping.ReducerOrdinal++
		}},
		{name: "read count", mutate: func(mapping *valuedomain.ExactFoldMapping) {
			mapping.ReadCount--
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			candidate := mapping
			testCase.mutate(&candidate)
			if exactFoldMappingMatches(rule, candidate) {
				t.Fatalf("nearest-negative %s mapping admitted", testCase.name)
			}
		})
	}
}

// TestExactFoldMappingCountMustMatchTheDescriptorReadCount pins the arity half
// of authentication: a mapping that names fewer or more reads than the sealed
// descriptor declares is refused even when every member it does name agrees.
func TestExactFoldMappingCountMustMatchTheDescriptorReadCount(t *testing.T) {
	for arity := 1; arity <= valuedomain.ExactFoldArity; arity++ {
		rule := exactFoldLawRule(t, arity)
		if !exactFoldMappingMatches(rule, exactFoldLawMapping(rule, arity)) {
			t.Fatalf("valid arity %d mapping was refused", arity)
		}
		for _, declared := range []int{0, arity + 1} {
			if declared < 1 || declared > valuedomain.ExactFoldArity {
				continue
			}
			if exactFoldMappingMatches(rule, exactFoldLawMapping(rule, declared)) {
				t.Fatalf("arity %d descriptor admitted a %d-read mapping", arity, declared)
			}
		}
	}
}

func TestExactFoldShapeRefusesNearestNegativeFactorAndOutputStrength(t *testing.T) {
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
		{name: "no read", make: func(spec *generated.CompiledRuleSpec) {
			spec.Reads = nil
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			spec := exactFoldLawSpec(2)
			testCase.make(&spec)
			candidate, sealed := generated.NewPlanCompiledRule(spec)
			if testCase.name == "factor" {
				if _, ok := exactFoldShape(candidate, engineexecution.FormExact); !sealed || ok {
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

func TestExactFoldShapeHotProbeAllocatesNothing(t *testing.T) {
	for arity := 1; arity <= valuedomain.ExactFoldArity; arity++ {
		rule := exactFoldLawRule(t, arity)
		mapping := exactFoldLawMapping(rule, arity)
		allocations := testing.AllocsPerRun(200, func() {
			count, ok := exactFoldShape(rule, engineexecution.FormExact)
			if !ok || count != arity || !exactFoldMappingMatches(rule, mapping) {
				t.Fatal("valid shape became unavailable")
			}
		})
		if allocations != 0 {
			t.Fatalf("warm exact fold probe at arity %d allocated %v times", arity, allocations)
		}
	}
}

func exactFoldLawRule(t *testing.T, arity int) generated.CompiledRule {
	t.Helper()
	return exactFoldRuleWith(t, arity, func(*generated.CompiledRuleSpec) {})
}

func exactFoldRuleWith(t *testing.T, arity int, amend func(*generated.CompiledRuleSpec)) generated.CompiledRule {
	t.Helper()
	spec := exactFoldLawSpec(arity)
	amend(&spec)
	rule, ok := generated.NewPlanCompiledRule(spec)
	if !ok {
		t.Fatal("sealed exact fold law descriptor")
	}
	return rule
}

// exactFoldLawMapping is the owner-issued mapping the law's descriptor states,
// derived from that descriptor rather than restated by hand.
func exactFoldLawMapping(rule generated.CompiledRule, reads int) valuedomain.ExactFoldMapping {
	mapping := valuedomain.ExactFoldMapping{
		ReducerOrdinal:          rule.Reducer().Member,
		CandidateRelationMember: rule.CandidateRelation().Member,
		ReadCount:               uint32(reads),
	}
	if output, ok := rule.OutputAt(0); ok {
		mapping.DestinationProjectionMember = output.Destination.Member
	}
	for join := 0; join < reads && join < valuedomain.ExactFoldArity; join++ {
		read, ok := rule.ReadAt(join)
		if !ok {
			continue
		}
		mapping.ReadRelationMember[join] = read.Relation.Member
		mapping.ReadKeyMember[join] = read.Key.Member
	}
	return mapping
}

func exactFoldLawSpec(arity int) generated.CompiledRuleSpec {
	contract := ruleplan.ReadContract{
		Order:        ruleprogram.OrderCanonical,
		Sparse:       ruleprogram.SparseExplicit,
		OnOpaque:     ruleprogram.OnOpaqueRefuse,
		Multiplicity: ruleprogram.MultiplicityOne,
	}
	axis := uint32(2)
	spec := generated.CompiledRuleSpec{
		AxisCount: 3, InputCount: 1,
		Candidate: ruleplan.RelationAddr{Axis: axis, Member: 0},
		Reducer:   ruleplan.ReducerAddr{Axis: axis, Member: 3},
		Outputs: []generated.OutputPlan{{
			Factor: axis, Axis: axis, Address: ruleplan.OutputAddr{Axis: axis, Frame: 0},
			Destination: ruleplan.ProjectionAddr{Axis: axis, Member: 9}, Mode: ruleprogram.ModeExact, Slot: 0, Exact: true, Strong: true,
		}},
		Carry: &generated.CarryPlan{Input: 0, Factor: axis, Mode: ruleprogram.CarryIdentity, Identity: true},
	}
	for join := 0; join < arity; join++ {
		spec.Reads = append(spec.Reads, generated.ReadPlan{
			Input: 0, Factor: axis, Axis: axis,
			Relation: ruleplan.RelationAddr{Axis: axis, Member: uint32(join)}, Key: ruleplan.ProjectionAddr{Axis: axis, Member: uint32(join)},
			Addressing: ruleplan.RelationAddr{Axis: axis, Member: 0}, AddressingPresent: true,
			Form: ruleprogram.Exact, PointBound: ruleprogram.PointBound, Contract: contract, RowCapacity: 2, CellCapacity: 1,
		})
	}
	return spec
}
