package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// TestMultiReadProductPreservesExactRowCorrelation exercises Product
// provenance only through the public cold declarations and completed solver
// result. Seven independent inputs deliberately carry distinct values; the
// Product callback must resolve every ordered Read on the same completed row,
// stage their positional checksum once, and survive final row coalescing.
func TestMultiReadProductPreservesExactRowCorrelation(t *testing.T) {
	const readCount = 7
	composition := NewComposition()
	var factors [readCount + 1]*Factor[uint64, uint64]
	var reads [readCount + 1]ReadForm[uint64, OrderedCells[uint64]]
	var writes [readCount + 1]WriteForm[uint64]
	for index := range factors {
		factor := coldFactor(composition, coldKey(20_130+index))
		read, readOK := ExactReadForm(factor)
		write, writeOK := ExactWriteForm(factor)
		if factor == nil || !readOK || !writeOK {
			t.Fatalf("multi-read Product Factor %d", index)
		}
		factors[index], reads[index], writes[index] = factor, read, write
	}

	var ingress [readCount]*Rule[uint64, ruleUnit]
	var ingressWrites [readCount]Write[uint64]
	var ingressTransfers, ingressRows [readCount]int
	for index := range ingress {
		index := index
		rule, declared := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
			Semantic: coldKey(20_140 + index), Output: factors[index].Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](uint64(20_150 + index)),
			Transfer: func(access Access[uint64, ruleUnit]) bool {
				ingressTransfers[index]++
				return Product(access, func(row Row) bool {
					ingressRows[index]++
					return StageValue(access, row, uint64(index+1))
				})
			},
		}, func(rule *Rule[uint64, ruleUnit]) bool {
			var ok bool
			ingressWrites[index], ok = WriteTo(rule, writes[index])
			return ok
		})
		if !declared || rule == nil {
			t.Fatalf("multi-read Product ingress Rule %d", index)
		}
		ingress[index] = rule
	}

	var tokens [readCount]Read[OrderedCells[uint64]]
	var correlateWrite Write[uint64]
	productTransfers, productRows := 0, 0
	correlate, correlateOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(20_160), Output: factors[readCount].Output(), Inputs: readCount, Admission: testTrustedTheorem[uint64](20_161),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			productTransfers++
			return Product(access, func(row Row) bool {
				productRows++
				var positional uint64
				for index := range tokens {
					cells, readable := ReadValue(access, row, tokens[index])
					if !readable || cells.Count() != 1 {
						return false
					}
					value, present, valid := cells.At(0)
					if !valid || !present || value != uint64(index+1) {
						return false
					}
					positional = positional*10 + value
				}
				return StageValue(access, row, positional)
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		for index := range tokens {
			input, inputOK := rule.InputAt(index)
			var readOK bool
			tokens[index], readOK = ReadFrom(rule, input, reads[index])
			if !inputOK || !readOK {
				return false
			}
		}
		var writeOK bool
		correlateWrite, writeOK = WriteTo(rule, writes[readCount])
		return writeOK
	})
	if !correlateOK || correlate == nil {
		t.Fatal("multi-read Product correlation Rule")
	}

	projects, freezes := 0, 0
	var queryRead QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(20_162),
		Project: func(observation Observation) uint64 {
			projects++
			rows, result := 0, uint64(0)
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, readable := QueryValue(row, queryRead)
				value, present, valid := cells.At(0)
				if !readable || cells.Count() != 1 || !valid || !present {
					return false
				}
				rows, result = rows+1, value
				return true
			}) || rows != 1 {
				return 0
			}
			return result
		},
		Result: FrozenResult[uint64]{
			Semantic: coldKey(20_163), Freeze: func(value uint64) uint64 { freezes++; return value }, Clone: func(value uint64) uint64 { return value },
			Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value },
		},
	}, func(query *Query[uint64]) bool {
		var declared bool
		queryRead, declared = QueryReadFrom(query, reads[readCount])
		return declared
	})
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("multi-read Product Query/composition")
	}
	var refs [readCount + 1]Ref[uint64]
	var instances [readCount + 1]*RuleInstance[uint64, ruleUnit]
	for index := range refs {
		ref, refOK := factors[index].Ref(0)
		if !refOK {
			t.Fatalf("multi-read Product Ref %d", index)
		}
		refs[index] = ref
		var instanceOK bool
		if index < readCount {
			index := index
			instances[index], instanceOK = NewRuleInstance(ingress[index], ruleUnitForSemantic(coldKey(20_180+index)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
				return InstanceWrite(binding, ingressWrites[index], refs[index])
			})
		} else {
			instances[index], instanceOK = NewRuleInstance(correlate, ruleUnitForSemantic(coldKey(20_180+index)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
				for readIndex := range tokens {
					if !InstanceRead(binding, tokens[readIndex], refs[readIndex]) {
						return false
					}
				}
				return InstanceWrite(binding, correlateWrite, refs[readCount])
			})
		}
		if !instanceOK {
			t.Fatalf("multi-read Product instance %d", index)
		}
	}

	scope := equation.EmptyScope()
	batch := equation.NewBatch()
	var sites [readCount + 1]equation.Site
	var occurrences [readCount + 1]equation.Occurrence
	var operands [readCount + 1]equation.Operand
	for index := range sites {
		site, siteOK := batch.AdmitSite(coldKey(20_170+index).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
		occurrence, occurrenceOK := batch.At(site)
		operand, operandOK := admitInstanceOperand(batch, occurrence, instances[index])
		if !siteOK || !occurrenceOK || !operandOK {
			t.Fatalf("multi-read Product source row %d", index)
		}
		sites[index], occurrences[index], operands[index] = site, occurrence, operand
	}
	if !scope.Available() || !batch.Seal() {
		t.Fatal("multi-read Product source batch")
	}
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		points := [readCount + 1]*assemblyPoint{}
		for index := range ingress {
			point := admitPoint(assembly, sites[index])
			member := admitInstance(assembly, point, occurrences[index], operands[index], instances[index])
			if point == nil || member == nil || admitGroup(assembly, point, member) == nil {
				t.Fatalf("multi-read Product ingress assembly %d", index)
			}
			points[index] = point
		}
		output := admitPoint(assembly, sites[readCount])
		member := admitInstance(assembly, output, occurrences[readCount], operands[readCount], instances[readCount])
		group := admitGroup(assembly, output, member)
		if output == nil || member == nil || group == nil {
			t.Fatal("multi-read Product output assembly")
		}
		for index := range ingress {
			boundary := equation.BoundaryInput(sites[index], sites[readCount], coldKey(20_190+index).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
			if !boundary.Available() || !admitBoundary(assembly, group, boundary) {
				t.Fatalf("multi-read Product boundary %d", index)
			}
		}
		points[readCount] = output
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, queryRead, refs[readCount])
		})
		observation := admitQueryAt(assembly, points[readCount], queryInstance)
		if !queryInstanceOK || observation == nil {
			t.Fatal("multi-read Product query assembly")
		}
		return true
	})
	if !compiled || solver == nil {
		t.Fatal("multi-read Product Solver compilation")
	}
	state, status := solver.Solve(context.Background())
	if status != SolveComplete || state == nil {
		t.Fatalf("multi-read Product Solve = state:%v status:%v", state, status)
	}
	receipt, receiptOK := queryInstance.Receipt()
	result, readable := QueryResult(receipt, state)
	if !receiptOK || !readable || result != 1_234_567 {
		t.Fatalf("multi-read Product result = %d readable:%t, want 1234567:true", result, readable)
	}
	for index := range ingress {
		if ingressTransfers[index] != 1 || ingressRows[index] != 1 {
			t.Fatalf("ingress %d callbacks = transfers:%d rows:%d, want 1:1", index, ingressTransfers[index], ingressRows[index])
		}
	}
	if productTransfers != 1 || productRows != 1 || projects != 1 || freezes != 1 {
		t.Fatalf("correlation callbacks = transfers:%d rows:%d projects:%d freezes:%d, want 1:1:1:1", productTransfers, productRows, projects, freezes)
	}
}
