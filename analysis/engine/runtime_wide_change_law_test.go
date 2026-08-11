package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// TestWideChangeWakesEachDownstreamRuleOnce is an execution-level law for a
// single source publication that changes every source Factor. Every downstream
// Rule reads every changed source, yet one publication makes each Rule runnable
// once. The asserted value and transfer counts use only the public rule/query
// behavior, not carrier or demand representation details.
func TestWideChangeWakesEachDownstreamRuleOnce(t *testing.T) {
	const width = 4
	composition := NewComposition()
	sourceFactors := make([]*Factor[uint64, uint64], width)
	destinationFactors := make([]*Factor[uint64, uint64], width)
	sourceRules := make([]*Rule[uint64, ruleUnit], width)
	destinationRules := make([]*Rule[uint64, ruleUnit], width)
	sourceRuleWrites := make([]Write[uint64], width)
	destinationRuleReads := make([][]Read[OrderedCells[uint64]], width)
	destinationRuleWrites := make([]Write[uint64], width)
	sourceReads := make([]ReadForm[uint64, OrderedCells[uint64]], width)
	sourceWrites := make([]WriteForm[uint64], width)
	destinationReads := make([]ReadForm[uint64, OrderedCells[uint64]], width)
	destinationWrites := make([]WriteForm[uint64], width)
	for index := 0; index < width; index++ {
		source, sourceOK := DeclareFactor(composition, coldFactorSpec(coldKey(uint64(96_000+index))), func(*Factor[uint64, uint64]) bool { return true })
		destination, destinationOK := DeclareFactor(composition, coldFactorSpec(coldKey(uint64(97_000+index))), func(*Factor[uint64, uint64]) bool { return true })
		if !sourceOK || source == nil || !destinationOK || destination == nil {
			t.Fatalf("Factor %d declaration", index)
		}
		var readsOK, writesOK bool
		sourceReads[index], readsOK = ExactReadForm(source)
		sourceWrites[index], writesOK = ExactWriteForm(source)
		if !readsOK || !writesOK {
			t.Fatalf("source Factor %d forms", index)
		}
		destinationReads[index], readsOK = ExactReadForm(destination)
		destinationWrites[index], writesOK = ExactWriteForm(destination)
		if !readsOK || !writesOK {
			t.Fatalf("destination Factor %d forms", index)
		}
		sourceFactors[index], destinationFactors[index] = source, destination
	}

	sourceTransfers := make([]int, width)
	destinationTransfers := make([]int, width)
	for index := 0; index < width; index++ {
		index := index
		source, declared := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
			Semantic: coldKey(uint64(98_000 + index)), Output: sourceFactors[index].Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](uint64(99_000 + index)),
			Transfer: func(access Access[uint64, ruleUnit]) bool {
				sourceTransfers[index]++
				return Product(access, func(row Row) bool { return StageValue(access, row, 1) })
			},
		}, func(rule *Rule[uint64, ruleUnit]) bool {
			var ok bool
			sourceRuleWrites[index], ok = WriteTo(rule, sourceWrites[index])
			return ok
		})
		if !declared || source == nil {
			t.Fatalf("source Rule %d declaration", index)
		}
		sourceRules[index] = source

		destination, declared := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
			Semantic: coldKey(uint64(100_000 + index)), Output: destinationFactors[index].Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](uint64(101_000 + index)),
			Transfer: func(access Access[uint64, ruleUnit]) bool {
				destinationTransfers[index]++
				return Product(access, func(row Row) bool { return StageValue(access, row, 1) })
			},
		}, func(rule *Rule[uint64, ruleUnit]) bool {
			input, ok := rule.InputAt(0)
			if !ok {
				return false
			}
			destinationRuleReads[index] = make([]Read[OrderedCells[uint64]], 0, len(sourceReads))
			for _, read := range sourceReads {
				token, readOK := ReadFrom(rule, input, read)
				if !readOK {
					return false
				}
				destinationRuleReads[index] = append(destinationRuleReads[index], token)
			}
			var writeOK bool
			destinationRuleWrites[index], writeOK = WriteTo(rule, destinationWrites[index])
			return writeOK
		})
		if !declared || destination == nil {
			t.Fatalf("destination Rule %d declaration", index)
		}
		destinationRules[index] = destination
	}

	var queryRead QueryRead[OrderedCells[uint64]]
	query, declared := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(102_000),
		Project: func(observation Observation) uint64 {
			var value uint64
			rows := 0
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, ok := QueryValue(row, queryRead)
				entry, present, valid := cells.At(0)
				if !ok || !valid || !present {
					return false
				}
				value, rows = entry, rows+1
				return true
			}) || rows != 1 {
				return 0
			}
			return value
		},
		Result: frozenColdResult(coldKey(102_001)),
	}, func(query *Query[uint64]) bool {
		var ok bool
		queryRead, ok = QueryReadFrom(query, destinationReads[0])
		return ok
	})
	if !declared || query == nil || !composition.Seal() {
		t.Fatal("wide-change composition")
	}

	sourceRefs := make([]Ref[uint64], width)
	sourceWriteRefs := make([]Ref[uint64], width)
	destinationRefs := make([]Ref[uint64], width)
	destinationWriteRefs := make([]Ref[uint64], width)
	for index := 0; index < width; index++ {
		var sourceReadOK, sourceWriteOK, destinationReadOK, destinationWriteOK bool
		sourceRefs[index], sourceReadOK = sourceFactors[index].Ref(0)
		sourceWriteRefs[index], sourceWriteOK = sourceFactors[index].Ref(0)
		destinationRefs[index], destinationReadOK = destinationFactors[index].Ref(0)
		destinationWriteRefs[index], destinationWriteOK = destinationFactors[index].Ref(0)
		if !sourceReadOK || !sourceWriteOK || !destinationReadOK || !destinationWriteOK {
			t.Fatalf("factor %d refs", index)
		}
	}
	occurrenceKeys := make([]SemanticKey, 0, 2*width)
	operandKeys := make([]SemanticKey, 0, 2*width)
	occurrenceSites := make([]int, 0, 2*width)
	for index := 0; index < width; index++ {
		occurrenceKeys = append(occurrenceKeys, coldKey(uint64(98_000+index)))
		operandKeys = append(operandKeys, coldKey(uint64(104_000+index)))
		occurrenceSites = append(occurrenceSites, 0)
	}
	for index := 0; index < width; index++ {
		occurrenceKeys = append(occurrenceKeys, coldKey(uint64(100_000+index)))
		operandKeys = append(operandKeys, coldKey(uint64(104_100+index)))
		occurrenceSites = append(occurrenceSites, 1)
	}
	instances := make([]*RuleInstance[uint64, ruleUnit], 2*width)
	for index := 0; index < width; index++ {
		index := index
		var declared bool
		instances[index], declared = NewRuleInstance(sourceRules[index], ruleUnitForSemantic(operandKeys[index]), func(binding *RuleBinding[uint64, ruleUnit]) bool {
			return InstanceWrite(binding, sourceRuleWrites[index], sourceWriteRefs[index])
		})
		if !declared {
			t.Fatalf("source instance %d", index)
		}
		instances[width+index], declared = NewRuleInstance(destinationRules[index], ruleUnitForSemantic(operandKeys[width+index]), func(binding *RuleBinding[uint64, ruleUnit]) bool {
			for readIndex, ref := range sourceRefs {
				if !InstanceRead(binding, destinationRuleReads[index][readIndex], ref) {
					return false
				}
			}
			return InstanceWrite(binding, destinationRuleWrites[index], destinationWriteRefs[index])
		})
		if !declared {
			t.Fatalf("destination instance %d", index)
		}
	}
	batch, sites, occurrences, operands, admitted := lifecycleLawBatchForSites(
		[]SemanticKey{coldKey(103_000), coldKey(103_001)}, occurrenceKeys, instances, occurrenceSites,
		[]equation.InitDisposition{equation.InitPresent, equation.InitAbsent},
	)
	if !admitted {
		t.Fatal("wide-change source batch")
	}
	boundary := equation.BoundaryInput(sites[0], sites[1], coldKey(105_000).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(sites[0].Scope()), equation.TrueExpr())
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		sourcePoint := admitPoint(assembly, sites[0])
		destinationPoint := admitPoint(assembly, sites[1])
		if sourcePoint == nil || destinationPoint == nil {
			return false
		}
		sourceMembers := make([]*assemblyMember, width)
		for index := 0; index < width; index++ {
			sourceMembers[index] = admitInstance(assembly, sourcePoint, occurrences[index], operands[index], instances[index])
			if sourceMembers[index] == nil {
				return false
			}
		}
		if admitGroup(assembly, sourcePoint, sourceMembers...) == nil {
			return false
		}
		destinationMembers := make([]*assemblyMember, width)
		for index := 0; index < width; index++ {
			destinationMembers[index] = admitInstance(assembly, destinationPoint, occurrences[width+index], operands[width+index], instances[width+index])
			if destinationMembers[index] == nil {
				return false
			}
		}
		group := admitGroup(assembly, destinationPoint, destinationMembers...)
		var queryDeclared bool
		queryInstance, queryDeclared = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, queryRead, destinationRefs[0])
		})
		observation := admitQueryAt(assembly, destinationPoint, queryInstance)
		return group != nil && boundary.Available() && admitBoundary(assembly, group, boundary) && queryDeclared && observation != nil
	})
	if !compiled || solver == nil {
		t.Fatal("wide-change Solver assembly")
	}
	state, status := solver.Solve(context.Background())
	if state == nil || status != SolveComplete {
		t.Fatalf("wide-change Solve = state:%v status:%v source-transfers:%v destination-transfers:%v", state, status, sourceTransfers, destinationTransfers)
	}
	receipt, receiptOK := queryInstance.Receipt()
	value, ok := QueryResult(receipt, state)
	if !receiptOK || !ok || value != 1 {
		t.Fatalf("wide-change query = value:%d ok:%t", value, ok)
	}
	for index, transfers := range sourceTransfers {
		if transfers != 1 {
			t.Fatalf("source Rule %d transfers = %d, want one", index, transfers)
		}
	}
	for index, transfers := range destinationTransfers {
		if transfers != 1 {
			t.Fatalf("downstream Rule %d transfers = %d, want one", index, transfers)
		}
	}
}
