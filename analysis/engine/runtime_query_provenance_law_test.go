package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// TestMultiReadQueryPreservesExactRepresentativeCorrelation exercises Query
// materialization through declaration, compilation, solve, and retrieval. All
// seven ordered reads must resolve the completed row's exact representative.
func TestMultiReadQueryPreservesExactRepresentativeCorrelation(t *testing.T) {
	composition := NewComposition()
	factor := coldFactor(composition, coldKey(20_118))
	read, readOK := ExactReadForm(factor)
	write, writeOK := ExactWriteForm(factor)
	if factor == nil || !readOK || !writeOK {
		t.Fatal("multi-read query forms")
	}
	var ruleWrite Write[uint64]
	rule, ruleOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(20_120), Output: factor.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](20_121),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(11)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var ok bool
		ruleWrite, ok = WriteTo(rule, write)
		return ok
	})
	if !ruleOK || rule == nil {
		t.Fatal("multi-read query ingress rule")
	}

	forms := [7]ReadForm[uint64, OrderedCells[uint64]]{read, read, read, read, read, read, read}
	var tokens [7]QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(20_124),
		Project: func(observation Observation) uint64 {
			rows := 0
			if !ProjectRows(observation, func(row QueryRow) bool {
				for index := range tokens {
					cells, readable := QueryValue(row, tokens[index])
					if !readable || cells.Count() != 1 {
						return false
					}
					value, present, valid := cells.At(0)
					if !valid || !present || value != 11 {
						return false
					}
				}
				rows++
				return true
			}) || rows != 1 {
				return 0
			}
			return 1
		},
		Result: FrozenResult[uint64]{
			Semantic: coldKey(20_125), Freeze: func(value uint64) uint64 { return value }, Clone: func(value uint64) uint64 { return value },
			Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value },
		},
	}, func(query *Query[uint64]) bool {
		for index := range forms {
			var declared bool
			tokens[index], declared = QueryReadFrom(query, forms[index])
			if !declared {
				return false
			}
		}
		return true
	})
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("multi-read query declaration")
	}
	readRef, readRefOK := factor.Ref(0)
	writeRef, writeRefOK := factor.Ref(0)
	instance, instanceOK := NewRuleInstance(rule, ruleUnitForSemantic(coldKey(20_127)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, ruleWrite, writeRef)
	})
	scope := equation.EmptyScope()
	batch := equation.NewBatch()
	site, siteOK := batch.AdmitSite(coldKey(20_126).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	occurrence, occurrenceOK := batch.At(site)
	operand, operandOK := admitInstanceOperand(batch, occurrence, instance)
	if !scope.Available() || !siteOK || !occurrenceOK || !operandOK || !readRefOK || !writeRefOK || !instanceOK || !batch.Seal() {
		t.Fatal("multi-read query source")
	}
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		point := admitPoint(assembly, site)
		member := admitInstance(assembly, point, occurrence, operand, instance)
		group := admitGroup(assembly, point, member)
		var queryDeclared bool
		queryInstance, queryDeclared = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			for index := range tokens {
				if !InstanceQueryRead(binding, tokens[index], readRef) {
					return false
				}
			}
			return true
		})
		observation := admitQueryAt(assembly, point, queryInstance)
		if point == nil || !queryDeclared || member == nil || group == nil || observation == nil {
			return false
		}
		return true
	})
	if !compiled || solver == nil {
		t.Fatal("multi-read query solver")
	}
	state, status := solver.Solve(context.Background())
	if status != SolveComplete || state == nil {
		t.Fatalf("multi-read query solve = state:%v status:%v", state, status)
	}
	receipt, receiptOK := queryInstance.Receipt()
	result, readable := QueryResult(receipt, state)
	if !receiptOK || !readable || result != 1 {
		t.Fatalf("multi-read query result = %d readable:%t, want 1:true", result, readable)
	}
}
