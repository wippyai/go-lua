package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

func TestCanonicalAssemblyBindsStrongExactTopology(t *testing.T) {
	cold := NewComposition()
	factor := coldFactor(cold, coldKey(87_001))
	if factor == nil {
		t.Fatal("factor")
	}
	read, readOK := ExactReadForm(factor)
	write, writeOK := ExactWriteForm(factor)
	var ruleWrite Write[uint64]
	rule, ruleOK := DeclareRule(cold, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(87_002), Output: factor.Output(), Inputs: 0,
		Admission: testTrustedTheorem[uint64](87_002), Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var ok bool
		ruleWrite, ok = WriteTo(rule, write)
		return ok
	})
	var token QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(cold, QuerySpec[uint64]{
		Semantic: coldKey(87_003),
		Project: func(observation Observation) uint64 {
			var projected uint64
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, ok := QueryValue(row, token)
				if !ok || cells.Count() != 1 {
					return false
				}
				value, present, ok := cells.At(0)
				if !ok || !present {
					return false
				}
				projected = value
				return true
			}) {
				return 0
			}
			return projected
		},
		Result: FrozenResult[uint64]{
			Semantic: coldKey(87_004),
			Freeze:   func(value uint64) uint64 { return value }, Clone: func(value uint64) uint64 { return value },
			Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value },
		},
	}, func(query *Query[uint64]) bool {
		var ok bool
		token, ok = QueryReadFrom(query, read)
		return ok
	})
	if !readOK || !writeOK || !ruleOK || !queryOK || rule == nil || query == nil || !cold.Seal() {
		t.Fatal("cold schema")
	}
	scope := equation.EmptyScope()
	batch := equation.NewBatch()
	site, siteOK := batch.AdmitSite(coldKey(87_005).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	occurrence, occurrenceOK := batch.At(site)
	readRef, readRefOK := factor.Ref(0)
	writeRef, writeRefOK := factor.Ref(0)
	instance, instanceOK := NewRuleInstance(rule, ruleUnitForSemantic(coldKey(87_006)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, ruleWrite, writeRef)
	})
	operand, operandOK := admitInstanceOperand(batch, occurrence, instance)
	if !scope.Available() || !siteOK || !occurrenceOK || !instanceOK || !operandOK || !readRefOK || !writeRefOK || !batch.Seal() {
		t.Fatal("source handles")
	}
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(cold, batch, func(assembly *Assembly) bool {
		point := admitPoint(assembly, site)
		member := admitInstance(assembly, point, occurrence, operand, instance)
		if point == nil || !instanceOK || member == nil || admitGroup(assembly, point, member) == nil {
			return false
		}
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, token, readRef)
		})
		return queryInstanceOK && admitQueryAt(assembly, point, queryInstance) != nil
	})
	if !compiled || solver == nil {
		t.Fatal("canonical static compiler")
	}
	state, status := solver.Solve(context.Background())
	if status != SolveComplete || state == nil {
		t.Fatalf("compiled solver did not complete: state=%v status=%v", state, status)
	}
	receipt, receiptOK := queryInstance.Receipt()
	if value, readable := QueryResult(receipt, state); !receiptOK || !readable || value != 1 {
		t.Fatalf("compiled query result = %d, readable=%v", value, readable)
	}
}
