package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// TestAssemblyCarryCyclePreservesOneSemanticRoot is deliberately black-box:
// an SCC containing a carry edge must expose the seed value at a downstream
// observation. It makes no claim about Graph groups, compiler catalogs, or
// any Go representation of the closure.
func TestAssemblyCarryCyclePreservesOneSemanticRoot(t *testing.T) {
	solver, receipt := assemblyCarryFixture(t, 3, []carryEdge{{0, 1}, {1, 2}, {2, 0}}, 2, 98_700)
	state, status := solver.Solve(context.Background())
	value, readable := QueryResult(receipt, state)
	if status != SolveComplete || state == nil || !readable || value != 1 {
		t.Fatalf("carry cycle solve = state:%v status:%v value:%d readable:%t", state, status, value, readable)
	}
}

// TestAssemblyCarryDiamondDeduplicatesAtTheJoin proves the other semantic
// shape: two predecessor routes may join, but one carried root stays one
// factor value at the successor. This replaces direct catalog inspection.
func TestAssemblyCarryDiamondDeduplicatesAtTheJoin(t *testing.T) {
	solver, receipt := assemblyCarryFixture(t, 5, []carryEdge{{0, 1}, {0, 2}, {1, 3}, {2, 3}, {3, 4}}, 4, 98_800)
	state, status := solver.Solve(context.Background())
	value, readable := QueryResult(receipt, state)
	if status != SolveComplete || state == nil || !readable || value != 1 {
		t.Fatalf("carry diamond solve = state:%v status:%v value:%d readable:%t", state, status, value, readable)
	}
}

type carryEdge struct{ source, target int }

// assemblyCarryFixture writes one root, derives all carry transport solely
// from the declared Carry form, and returns the only public observation. A
// point may have several groups (the cycle head and diamond join), which is
// ordinary Assembly geometry rather than a test-only topology escape hatch.
func assemblyCarryFixture(t testing.TB, points int, edges []carryEdge, observed int, base uint64) (*Solver, QueryReceipt[uint64]) {
	t.Helper()
	if points < 2 || len(edges) == 0 || observed < 0 || observed >= points {
		t.Fatal("carry fixture shape")
	}
	composition := NewComposition()
	factorSpec := coldFactorSpec(coldKey(base))
	// A Carry SCC is admitted only with this Factor's well-founded widening
	// witness. The test deliberately exercises the recursive path, so using
	// the rankless acyclic convenience fixture here must fail closed.
	factorSpec.WidenRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }}
	factor, factorDeclared := DeclareFactor(composition, factorSpec, func(*Factor[uint64, uint64]) bool { return true })
	if !factorDeclared || factor == nil {
		t.Fatal("carry factor")
	}
	read, readOK := ExactReadForm(factor)
	write, writeOK := ExactWriteForm(factor)
	carry, carryOK := Carry(factor)
	if !readOK || !writeOK || !carryOK {
		t.Fatal("carry forms")
	}
	var seedWrite Write[uint64]
	seed, seedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent, Semantic: coldKey(base + 1), Output: factor.Output(), Inputs: 0,
		Admission: testTrustedTheorem[uint64](base + 2),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var written bool
		seedWrite, written = WriteTo(rule, write)
		return written
	})
	carryRule, carryRuleOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent, Semantic: coldKey(base + 3), Output: factor.Output(), Inputs: 1,
		Admission: testTrustedTheorem[uint64](base + 4),
		Transfer:  func(access Access[uint64, ruleUnit]) bool { return Product(access, func(Row) bool { return true }) },
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		return inputOK && CarryFrom(rule, input, carry)
	})
	var token QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(base + 5),
		Project: func(observation Observation) uint64 {
			var result uint64
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, resolved := QueryValue(row, token)
				value, present, valid := cells.At(0)
				if !resolved || !valid || !present || cells.Count() != 1 {
					return false
				}
				result = value
				return true
			}) {
				return 0
			}
			return result
		},
		Result: frozenColdResult(coldKey(base + 6)),
	}, func(query *Query[uint64]) bool {
		var declared bool
		token, declared = QueryReadFrom(query, read)
		return declared
	})
	if !seedOK || seed == nil || !carryRuleOK || carryRule == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("carry declarations")
	}
	ref, issued := factor.Ref(0)
	seedInstance, seedDeclared := NewRuleInstance(seed, ruleUnitForSemantic(coldKey(base+21)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, seedWrite, ref)
	})
	carryInstances := make([]*RuleInstance[uint64, ruleUnit], len(edges))
	for index := range carryInstances {
		var declared bool
		carryInstances[index], declared = NewRuleInstance(carryRule, ruleUnitForSemantic(coldKey(base+40+uint64(index))), func(*RuleBinding[uint64, ruleUnit]) bool { return true })
		if !declared {
			t.Fatal("carry instance")
		}
	}
	if !issued || !seedDeclared {
		t.Fatal("carry source instances")
	}

	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	sites := make([]equation.Site, points)
	for index := range sites {
		init, disposition := equation.FalseExpr(), equation.InitAbsent
		if index == 0 {
			init, disposition = equation.TrueExpr(), equation.InitPresent
		}
		site, admitted := batch.AdmitSite(coldKey(base+10+uint64(index)).compositionKey(), scope, init, disposition)
		if !admitted {
			t.Fatal("carry source site")
		}
		sites[index] = site
	}
	seedOccurrence, seedOccurred := batch.Relation(sites[0], coldKey(base+20).compositionKey())
	seedOperand, seedOperandOK := admitInstanceOperand(batch, seedOccurrence, seedInstance)
	occurrences := make([]equation.Occurrence, len(edges))
	operands := make([]equation.Operand, len(edges))
	for index, edge := range edges {
		if edge.source < 0 || edge.source >= points || edge.target < 0 || edge.target >= points {
			t.Fatal("carry edge")
		}
		occurrence, occurred := batch.Relation(sites[edge.target], coldKey(base+30+uint64(index)).compositionKey())
		operand, admitted := admitInstanceOperand(batch, occurrence, carryInstances[index])
		if !occurred || !admitted {
			t.Fatal("carry source operand")
		}
		occurrences[index], operands[index] = occurrence, operand
	}
	if !scope.Available() || !seedOccurred || !seedOperandOK || !batch.Seal() {
		t.Fatal("carry source batch")
	}

	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		assemblyPoints := make([]*assemblyPoint, len(sites))
		for index, site := range sites {
			assemblyPoints[index] = admitPoint(assembly, site)
			if assemblyPoints[index] == nil {
				return false
			}
		}
		seedMember := admitInstance(assembly, assemblyPoints[0], seedOccurrence, seedOperand, seedInstance)
		if seedMember == nil || admitGroup(assembly, assemblyPoints[0], seedMember) == nil {
			return false
		}
		for index, edge := range edges {
			member := admitInstance(assembly, assemblyPoints[edge.target], occurrences[index], operands[index], carryInstances[index])
			group := admitGroup(assembly, assemblyPoints[edge.target], member)
			boundary := equation.BoundaryInput(sites[edge.source], sites[edge.target], coldKey(base+50+uint64(index)).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
			if member == nil || group == nil || !admitBoundary(assembly, group, boundary) {
				return false
			}
		}
		var queryDeclared bool
		queryInstance, queryDeclared = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, token, ref)
		})
		observation := admitQueryAt(assembly, assemblyPoints[observed], queryInstance)
		return queryDeclared && observation != nil
	})
	if !compiled || solver == nil {
		t.Fatal("carry solver compile")
	}
	receipt, receiptOK := queryInstance.Receipt()
	if !receiptOK {
		t.Fatal("carry query receipt")
	}
	return solver, receipt
}
