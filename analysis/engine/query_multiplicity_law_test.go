package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// TestOneColdQueryFamilyPublishesExactInstanceReceipts proves that one cold
// Query schema can be attached at two concrete points without either schema
// duplication or family-only result ambiguity.
func TestOneColdQueryFamilyPublishesExactInstanceReceipts(t *testing.T) {
	composition := NewComposition()
	factor := coldFactor(composition, coldKey(94_001))
	read, readOK := ExactReadForm(factor)
	write, writeOK := ExactWriteForm(factor)
	var ruleWrite Write[uint64]
	rule, ruleOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(94_002), OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Output: factor.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](94_003),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, 17) })
		},
	}, func(value *Rule[uint64, ruleUnit]) bool {
		var ok bool
		ruleWrite, ok = WriteTo(value, write)
		return ok
	})
	var queryRead QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(94_004),
		Project: func(observation Observation) uint64 {
			var result uint64
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, ok := QueryValue(row, queryRead)
				value, present, valid := cells.At(0)
				if !ok || !valid || !present {
					return false
				}
				result = value
				return true
			}) {
				return 0
			}
			return result
		},
		Result: FrozenResult[uint64]{
			Semantic: coldKey(94_005), Freeze: func(value uint64) uint64 { return value }, Clone: func(value uint64) uint64 { return value },
			Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value },
		},
	}, func(value *Query[uint64]) bool {
		var ok bool
		queryRead, ok = QueryReadFrom(value, read)
		return ok
	})
	if factor == nil || !readOK || !writeOK || !ruleOK || rule == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("multiplicity cold declaration")
	}
	ref, refOK := factor.Ref(0)
	if !refOK {
		t.Fatal("multiplicity ref")
	}
	instances := make([]*RuleInstance[uint64, ruleUnit], 2)
	for index := range instances {
		instance, ok := NewRuleInstance(rule, ruleUnitForSemantic(coldKey(uint64(94_010+index))), func(binding *RuleBinding[uint64, ruleUnit]) bool {
			return InstanceWrite(binding, ruleWrite, ref)
		})
		if !ok {
			t.Fatal("multiplicity rule instance")
		}
		instances[index] = instance
	}
	scope := equation.EmptyScope()
	batch := equation.NewBatch()
	sites := make([]equation.Site, len(instances))
	occurrences := make([]equation.Occurrence, len(instances))
	operands := make([]equation.Operand, len(instances))
	for index, instance := range instances {
		site, siteOK := batch.AdmitSite(coldKey(uint64(94_020+index)).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
		occurrence, occurrenceOK := batch.At(site)
		operand, operandOK := admitInstanceOperand(batch, occurrence, instance)
		if !siteOK || !occurrenceOK || !operandOK {
			t.Fatal("multiplicity source row")
		}
		sites[index], occurrences[index], operands[index] = site, occurrence, operand
	}
	if !scope.Available() || !batch.Seal() {
		t.Fatal("multiplicity source seal")
	}
	queryInstances := make([]*QueryInstance[uint64], len(instances))
	solver, assembled := assemble(composition, batch, func(assembly *Assembly) bool {
		for index, instance := range instances {
			point := admitPoint(assembly, sites[index])
			member := admitInstance(assembly, point, occurrences[index], operands[index], instance)
			group := admitGroup(assembly, point, member)
			queryInstance, queryInstanceOK := NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
				return InstanceQueryRead(binding, queryRead, ref)
			})
			queryInstances[index] = queryInstance
			if point == nil || member == nil || group == nil || !queryInstanceOK || admitQueryAt(assembly, point, queryInstance) == nil {
				return false
			}
		}
		return true
	})
	if !assembled || solver == nil {
		t.Fatal("multiplicity assembly")
	}
	first, firstOK := queryInstances[0].Receipt()
	second, secondOK := queryInstances[1].Receipt()
	if !firstOK || !secondOK || first == second {
		t.Fatal("distinct exact receipts")
	}
	state, status := solver.Solve(context.Background())
	if status != SolveComplete || state == nil {
		t.Fatalf("multiplicity solve = state:%v status:%v", state, status)
	}
	for index, receipt := range []QueryReceipt[uint64]{first, second} {
		value, readable := QueryResult(receipt, state)
		if !readable || value != 17 {
			t.Fatalf("receipt result[%d] = %d/%v, want 17/true", index, value, readable)
		}
	}
}
