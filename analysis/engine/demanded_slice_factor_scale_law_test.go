package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// TestDemandedSliceFactorScale keeps one exact Factor/query slice fixed while
// growing only disconnected cold declarations and equation rows.  The law is
// intentionally about executed semantic work, not compile latency: binding a
// cold composition necessarily visits every declared Factor.
func TestDemandedSliceFactorScale(t *testing.T) {
	factorCounts := []int{1, 16, 100}
	var baseline demandedSliceObservation
	for _, factorCount := range factorCounts {
		factorCount := factorCount
		t.Run("factors", func(t *testing.T) {
			observed := runDemandedSliceScale(t, factorCount)
			if observed.value != 41 || observed.demandedTransfers != 1 || observed.demandedProducts != 1 || observed.projects != 1 || observed.freezes != 1 {
				t.Fatalf("demanded slice at Factors=%d: value=%d transfers=%d products=%d projects=%d freezes=%d", factorCount, observed.value, observed.demandedTransfers, observed.demandedProducts, observed.projects, observed.freezes)
			}
			if observed.disconnectedTransfers != 0 || observed.disconnectedProducts != 0 {
				t.Fatalf("disconnected slice executed at Factors=%d: transfers=%d products=%d", factorCount, observed.disconnectedTransfers, observed.disconnectedProducts)
			}
			// This is descriptive rather than a performance threshold. Cold
			// declaration/binding is expected to scale with the catalog size.
			t.Logf("cold setup: Factors=%d Rules=%d Points=%d Groups=%d (O(F) setup allowed)", observed.coldFactors, observed.coldRules, observed.coldPoints, observed.coldGroups)
			if factorCount == factorCounts[0] {
				baseline = observed
				return
			}
			if observed.value != baseline.value || observed.demandedTransfers != baseline.demandedTransfers || observed.demandedProducts != baseline.demandedProducts || observed.projects != baseline.projects || observed.freezes != baseline.freezes {
				t.Fatalf("demanded Product/query work changed with disconnected Factors=%d: got=%+v baseline=%+v", factorCount, observed, baseline)
			}
		})
	}
}

type demandedSliceObservation struct {
	value                 uint64
	demandedTransfers     int
	demandedProducts      int
	disconnectedTransfers int
	disconnectedProducts  int
	projects              int
	freezes               int
	coldFactors           int
	coldRules             int
	coldPoints            int
	coldGroups            int
}

func runDemandedSliceScale(t *testing.T, factorCount int) demandedSliceObservation {
	t.Helper()
	if factorCount < 1 {
		t.Fatal("Factor count")
	}
	composition := NewComposition()
	factors := make([]*Factor[uint64, uint64], factorCount)
	reads := make([]ReadForm[uint64, OrderedCells[uint64]], factorCount)
	writes := make([]WriteForm[uint64], factorCount)
	for index := range factors {
		semantic := demandedSliceFactorSemantic(index)
		factor, declared := DeclareFactor(composition, coldFactorSpec(semantic), func(*Factor[uint64, uint64]) bool { return true })
		if !declared || factor == nil {
			t.Fatalf("Factor %d declaration", index)
		}
		read, readOK := ExactReadForm(factor)
		write, writeOK := ExactWriteForm(factor)
		if !readOK || !writeOK {
			t.Fatalf("Factor %d forms", index)
		}
		factors[index], reads[index], writes[index] = factor, read, write
	}

	stats := &demandedSliceObservation{coldFactors: factorCount, coldRules: factorCount, coldPoints: factorCount, coldGroups: factorCount}
	rules := make([]*Rule[uint64, ruleUnit], factorCount)
	writeTokens := make([]Write[uint64], factorCount)
	for index := range rules {
		index := index
		semantic := demandedSliceRuleSemantic(index)
		rule, declared := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
			Semantic: semantic, Output: factors[index].Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](uint64(91_000 + index)),
			Transfer: func(access Access[uint64, ruleUnit]) bool {
				if index == 0 {
					stats.demandedTransfers++
				} else {
					stats.disconnectedTransfers++
				}
				return Product(access, func(row Row) bool {
					if index == 0 {
						stats.demandedProducts++
					} else {
						stats.disconnectedProducts++
					}
					return StageValue(access, row, uint64(41))
				})
			},
		}, func(rule *Rule[uint64, ruleUnit]) bool {
			var ok bool
			writeTokens[index], ok = WriteTo(rule, writes[index])
			return ok
		})
		if !declared || rule == nil {
			t.Fatalf("Rule %d declaration", index)
		}
		rules[index] = rule
	}

	var token QueryRead[OrderedCells[uint64]]
	query, declared := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: demandedSliceQuerySemantic(),
		Project: func(observation Observation) uint64 {
			stats.projects++
			var value uint64
			rows := 0
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, ok := QueryValue(row, token)
				if !ok || cells.Count() != 1 {
					return false
				}
				entry, present, valid := cells.At(0)
				if !valid || !present {
					return false
				}
				value, rows = entry, rows+1
				return true
			}) || rows != 1 {
				return 0
			}
			return value
		},
		Result: FrozenResult[uint64]{
			Semantic: demandedSliceFrozenSemantic(),
			Freeze:   func(value uint64) uint64 { stats.freezes++; return value }, Clone: func(value uint64) uint64 { return value },
			Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value },
		},
	}, func(query *Query[uint64]) bool {
		var ok bool
		token, ok = QueryReadFrom(query, reads[0])
		return ok
	})
	if !declared || query == nil || !composition.Seal() {
		t.Fatal("demanded Query/composition declaration")
	}

	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	sites := make([]equation.Site, factorCount)
	occurrences := make([]equation.Occurrence, factorCount)
	operands := make([]equation.Operand, factorCount)
	instances := make([]*RuleInstance[uint64, ruleUnit], factorCount)
	for index := range rules {
		write, writeOK := factors[index].Ref(0)
		instance, instanceOK := NewRuleInstance(rules[index], ruleUnitForSemantic(demandedSliceRuleSemantic(index)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
			return InstanceWrite(binding, writeTokens[index], write)
		})
		site, siteOK := batch.AdmitSite(demandedSliceSourceSemantic(index).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
		occurrence, occurrenceOK := batch.At(site)
		operand, operandOK := admitInstanceOperand(batch, occurrence, instance)
		if !writeOK || !instanceOK || !siteOK || !occurrenceOK || !operandOK {
			t.Fatalf("demanded source row %d", index)
		}
		sites[index], occurrences[index], operands[index], instances[index] = site, occurrence, operand, instance
	}
	if !batch.Seal() {
		t.Fatal("demanded source batch")
	}
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		for index := range rules {
			point := admitPoint(assembly, sites[index])
			member := admitInstance(assembly, point, occurrences[index], operands[index], instances[index])
			if point == nil || member == nil || admitGroup(assembly, point, member) == nil {
				t.Fatalf("demanded assembly row %d", index)
			}
			if index == 0 {
				read, readOK := factors[0].Ref(0)
				var queryInstanceOK bool
				queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
					return InstanceQueryRead(binding, token, read)
				})
				observation := admitQueryAt(assembly, point, queryInstance)
				if !readOK || !queryInstanceOK || observation == nil {
					t.Fatal("demanded query assembly")
				}
			}
		}
		return true
	})
	if !compiled || solver == nil {
		t.Fatal("demanded scale Solver assembly")
	}
	state, status := solver.Solve(context.Background())
	if state == nil || status != SolveComplete {
		t.Fatalf("demanded scale Solve = state:%v status:%v", state, status)
	}
	receipt, receiptOK := queryInstance.Receipt()
	value, projected := QueryResult(receipt, state)
	if !receiptOK || !projected {
		t.Fatal("demanded exact Query result")
	}
	stats.value = value
	return *stats
}

func demandedSliceFactorSemantic(index int) SemanticKey { return coldKey(90_000 + index) }
func demandedSliceRuleSemantic(index int) SemanticKey   { return coldKey(91_000 + index) }
func demandedSliceSourceSemantic(index int) SemanticKey { return coldKey(92_000 + index) }
func demandedSliceQuerySemantic() SemanticKey           { return coldKey(93_000) }
func demandedSliceFrozenSemantic() SemanticKey          { return coldKey(93_001) }

// TestDemandedRankedCycleSkipsUnrankedSibling is the recurrence counterpart
// of the scale law.  It deliberately uses no solver-runtime fields: the cold
// APIs plus a disposable equation.TopologySpec can express both disconnected
// cycles, and Solve/QueryResult are the only execution observations.
func TestDemandedRankedCycleSkipsUnrankedSibling(t *testing.T) {
	composition := NewComposition()
	rankedSpec := coldFactorSpec(coldKey(94_000))
	rankedSpec.KeyEnd = 1
	rankedSpec.WidenRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }}
	rankedSpec.Lattice.Narrow = func(_ uint64, desired uint64) uint64 { return desired }
	rankedSpec.NarrowRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return value }}
	ranked, rankedDeclared := DeclareFactor(composition, rankedSpec, func(*Factor[uint64, uint64]) bool { return true })
	unranked, unrankedDeclared := DeclareFactor(composition, coldFactorSpec(coldKey(94_001)), func(*Factor[uint64, uint64]) bool { return true })
	if !rankedDeclared || ranked == nil || !unrankedDeclared || unranked == nil {
		t.Fatal("ranked/unranked Factor declaration")
	}
	rankedRead, rankedReadOK := ExactReadForm(ranked)
	rankedWrite, rankedWriteOK := ExactWriteForm(ranked)
	unrankedRead, unrankedReadOK := ExactReadForm(unranked)
	unrankedWrite, unrankedWriteOK := ExactWriteForm(unranked)
	if !rankedReadOK || !rankedWriteOK || !unrankedReadOK || !unrankedWriteOK {
		t.Fatal("ranked/unranked Factor forms")
	}

	transfers := [2]int{}
	products := [2]int{}
	rules := make([]*Rule[uint64, ruleUnit], 2)
	ruleReads := make([]Read[OrderedCells[uint64]], len(rules))
	ruleWrites := make([]Write[uint64], len(rules))
	for index, output := range []Output[uint64]{ranked.Output(), unranked.Output()} {
		index, output := index, output
		read, write := rankedRead, rankedWrite
		if index == 1 {
			read, write = unrankedRead, unrankedWrite
		}
		rule, declared := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
			Semantic: coldKey(94_010 + index), Output: output, Inputs: 1, Admission: testTrustedTheorem[uint64](uint64(94_020 + index)),
			Transfer: func(access Access[uint64, ruleUnit]) bool {
				transfers[index]++
				return Product(access, func(row Row) bool {
					products[index]++
					return StageValue(access, row, uint64(7))
				})
			},
		}, func(rule *Rule[uint64, ruleUnit]) bool {
			input, inputOK := rule.InputAt(0)
			var readOK, writeOK bool
			ruleReads[index], readOK = ReadFrom(rule, input, read)
			ruleWrites[index], writeOK = WriteTo(rule, write)
			return inputOK && readOK && writeOK
		})
		if !declared || rule == nil {
			t.Fatalf("cycle Rule %d declaration", index)
		}
		rules[index] = rule
	}

	projects, freezes := 0, 0
	var token QueryRead[OrderedCells[uint64]]
	query, declared := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(94_030),
		Project: func(observation Observation) uint64 {
			projects++
			var value uint64
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, ok := QueryValue(row, token)
				entry, present, valid := cells.At(0)
				if !ok || !valid || !present {
					return false
				}
				value = entry
				return true
			}) {
				return 0
			}
			return value
		},
		Result: FrozenResult[uint64]{
			Semantic: coldKey(94_031), Freeze: func(value uint64) uint64 { freezes++; return value }, Clone: func(value uint64) uint64 { return value },
			Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value },
		},
	}, func(query *Query[uint64]) bool {
		var ok bool
		token, ok = QueryReadFrom(query, rankedRead)
		return ok
	})
	if !declared || query == nil || !composition.Seal() {
		t.Fatal("cycle Query/composition declaration")
	}
	scope := equation.EmptyScope()
	batch := equation.NewBatch()
	sites := [2]equation.Site{}
	occurrences := [2]equation.Occurrence{}
	operands := [2]equation.Operand{}
	instances := [2]*RuleInstance[uint64, ruleUnit]{}
	factorSet := []*Factor[uint64, uint64]{ranked, unranked}
	for index := range rules {
		read, readOK := factorSet[index].Ref(0)
		write, writeOK := factorSet[index].Ref(0)
		semantic := coldKey(94_050 + index)
		instance, instanceOK := NewRuleInstance(rules[index], ruleUnitForSemantic(semantic), func(binding *RuleBinding[uint64, ruleUnit]) bool {
			return InstanceRead(binding, ruleReads[index], read) && InstanceWrite(binding, ruleWrites[index], write)
		})
		site, siteOK := batch.AdmitSite(coldKey(94_040+index).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
		occurrence, occurrenceOK := batch.At(site)
		operand, operandOK := admitInstanceOperand(batch, occurrence, instance)
		if !readOK || !writeOK || !instanceOK || !siteOK || !occurrenceOK || !operandOK {
			t.Fatalf("cycle source row %d", index)
		}
		sites[index], occurrences[index], operands[index], instances[index] = site, occurrence, operand, instance
	}
	if !scope.Available() || !batch.Seal() {
		t.Fatal("cycle source batch")
	}
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		for index := range rules {
			point := admitPoint(assembly, sites[index])
			read, _ := factorSet[index].Ref(0)
			member := admitInstance(assembly, point, occurrences[index], operands[index], instances[index])
			boundary := equation.BoundaryInput(sites[index], sites[index], coldKey(94_060+index).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
			group := admitGroup(assembly, point, member)
			if point == nil || member == nil || group == nil || !admitBoundary(assembly, group, boundary) {
				t.Fatalf("cycle assembly row %d", index)
			}
			if index == 0 {
				var queryInstanceOK bool
				queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
					return InstanceQueryRead(binding, token, read)
				})
				observation := admitQueryAt(assembly, point, queryInstance)
				if !queryInstanceOK || observation == nil {
					t.Fatal("cycle query assembly")
				}
			}
		}
		return true
	})
	if !compiled || solver == nil {
		t.Fatal("cycle Solver assembly")
	}
	state, status := solver.Solve(context.Background())
	if state == nil || status != SolveComplete {
		t.Fatalf("ranked demanded cycle Solve = state:%v status:%v", state, status)
	}
	receipt, receiptOK := queryInstance.Receipt()
	value, projected := QueryResult(receipt, state)
	if !receiptOK || !projected || value != 7 || transfers[0] == 0 || products[0] == 0 || projects != 1 || freezes != 1 {
		t.Fatalf("ranked demanded cycle = value:%d projected:%t transfers:%v products:%v projects:%d freezes:%d", value, projected, transfers, products, projects, freezes)
	}
	if transfers[1] != 0 || products[1] != 0 {
		t.Fatalf("unranked disconnected cycle executed: transfers=%d products=%d", transfers[1], products[1])
	}
}
