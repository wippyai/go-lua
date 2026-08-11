package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// TestSyntheticParametricBoundaryLaw uses normal, throw, and yield only as
// synthetic coordinate labels.  Its boundary is an ordinary equation Reindex:
// two call decisions substitute the same target expression, local freshness is
// a Rename, and each outcome keeps its own renamed target coordinate.
func TestSyntheticParametricBoundaryLaw(t *testing.T) {
	stats := &parametricBoundaryStats{}
	fixture := newParametricBoundaryFixture(t, stats)
	first, status := fixture.solver.Solve(context.Background())
	if status != SolveComplete || first == nil {
		t.Fatalf("first Solve = state:%v status:%v", first, status)
	}
	assertParametricBoundaryResults(t, fixture.receipts, first)
	if stats.transfers != [7]int{1, 1, 1, 1, 1, 1, 1} || stats.projects != [4]int{1, 1, 1, 1} || stats.freezes != [4]int{1, 1, 1, 1} {
		t.Fatalf("cold callbacks = transfers:%v projects:%v freezes:%v", stats.transfers, stats.projects, stats.freezes)
	}
	second, status := fixture.solver.Solve(context.Background())
	if status != SolveComplete || second == nil {
		t.Fatalf("warm Solve = state:%v status:%v", second, status)
	}
	assertParametricBoundaryResults(t, fixture.receipts, second)
	if stats.transfers != [7]int{1, 1, 1, 1, 1, 1, 1} || stats.projects != [4]int{1, 1, 1, 1} || stats.freezes != [4]int{1, 1, 1, 1} {
		t.Fatalf("warm Solve repeated callbacks: transfers:%v projects:%v freezes:%v", stats.transfers, stats.projects, stats.freezes)
	}
}

type parametricBoundaryStats struct {
	transfers         [7]int
	projects, freezes [4]int
}

type parametricBoundaryFixture struct {
	solver   *Solver
	queries  [4]*Query[uint64]
	receipts [4]QueryReceipt[uint64]
}

func parametricBoundaryKey(offset uint64) SemanticKey { return testSemanticKey(39_000 + offset) }

func newParametricBoundaryFixture(t *testing.T, stats *parametricBoundaryStats) parametricBoundaryFixture {
	t.Helper()
	if stats == nil {
		t.Fatal("callback stats")
	}
	cold := NewComposition()
	var factors [4]*Factor[uint64, uint64]
	var reads [4]ReadForm[uint64, OrderedCells[uint64]]
	var writes [4]WriteForm[uint64]
	for index := range factors {
		spec := coldFactorSpec(parametricBoundaryKey(uint64(index + 1)))
		spec.KeyEnd = 1
		factor, ok := DeclareFactor(cold, spec, func(*Factor[uint64, uint64]) bool { return true })
		if !ok || factor == nil {
			t.Fatal("Factor declaration")
		}
		factors[index] = factor
		var readOK, writeOK bool
		reads[index], readOK = ExactReadForm(factor)
		writes[index], writeOK = ExactWriteForm(factor)
		if !readOK || !writeOK {
			t.Fatal("Factor forms")
		}
	}

	var rules [7]*Rule[uint64, ruleUnit]
	var ruleReads [7][]Read[OrderedCells[uint64]]
	var ruleWrites [7]Write[uint64]
	for index, value := range []uint64{11, 22, 33} {
		index, value := index, value
		rule, ok := DeclareRule(cold, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
			Semantic: parametricBoundaryKey(uint64(20 + index)), Output: factors[index].Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](39_020 + uint64(index)),
			Transfer: func(access Access[uint64, ruleUnit]) bool {
				stats.transfers[index]++
				return Product(access, func(row Row) bool { return StageValue(access, row, value) })
			},
		}, func(rule *Rule[uint64, ruleUnit]) bool {
			var ok bool
			ruleWrites[index], ok = WriteTo(rule, writes[index])
			return ok
		})
		if !ok || rule == nil {
			t.Fatal("outcome ingress Rule")
		}
		rules[index] = rule
	}
	for index := 0; index < 3; index++ {
		index := index
		var read Read[OrderedCells[uint64]]
		rule, ok := DeclareRule(cold, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
			Semantic: parametricBoundaryKey(uint64(30 + index)), Output: factors[index].Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](39_030 + uint64(index)),
			Transfer: func(access Access[uint64, ruleUnit]) bool {
				stats.transfers[3+index]++
				return Product(access, func(row Row) bool {
					cells, ok := ReadValue(access, row, read)
					if !ok || cells.Count() != 1 {
						return false
					}
					value, present, ok := cells.At(0)
					return ok && present && StageValue(access, row, value)
				})
			},
		}, func(rule *Rule[uint64, ruleUnit]) bool {
			input, inputOK := rule.InputAt(0)
			var readOK, writeOK bool
			read, readOK = ReadFrom(rule, input, reads[index])
			ruleWrites[3+index], writeOK = WriteTo(rule, writes[index])
			return inputOK && readOK && writeOK
		})
		if !ok || rule == nil {
			t.Fatal("boundary transport Rule")
		}
		rules[3+index], ruleReads[3+index] = rule, []Read[OrderedCells[uint64]]{read}
	}
	var normalRead, throwRead Read[OrderedCells[uint64]]
	resume, ok := DeclareRule(cold, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: parametricBoundaryKey(40), Output: factors[3].Output(), Inputs: 2, Admission: testTrustedTheorem[uint64](39_040),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			stats.transfers[6]++
			return Product(access, func(row Row) bool {
				normal, normalOK := ReadValue(access, row, normalRead)
				thrown, throwOK := ReadValue(access, row, throwRead)
				if !normalOK || !throwOK || normal.Count() != 1 || thrown.Count() != 1 {
					return false
				}
				normalValue, normalPresent, normalCellOK := normal.At(0)
				throwValue, throwPresent, throwCellOK := thrown.At(0)
				return normalCellOK && normalPresent && throwCellOK && throwPresent && StageValue(access, row, normalValue*100+throwValue)
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		normalInput, normalInputOK := rule.InputAt(0)
		throwInput, throwInputOK := rule.InputAt(1)
		var normalOK, throwOK, writeOK bool
		normalRead, normalOK = ReadFrom(rule, normalInput, reads[0])
		throwRead, throwOK = ReadFrom(rule, throwInput, reads[1])
		ruleWrites[6], writeOK = WriteTo(rule, writes[3])
		return normalInputOK && throwInputOK && normalOK && throwOK && writeOK
	})
	if !ok || resume == nil {
		t.Fatal("two-input resume Rule")
	}
	rules[6], ruleReads[6] = resume, []Read[OrderedCells[uint64]]{normalRead, throwRead}

	var queries [4]*Query[uint64]
	var queryReads [4]QueryRead[OrderedCells[uint64]]
	for index := range queries {
		index := index
		var token QueryRead[OrderedCells[uint64]]
		query, ok := DeclareQuery(cold, QuerySpec[uint64]{
			Semantic: parametricBoundaryKey(uint64(50 + index)),
			Project: func(observation Observation) uint64 {
				stats.projects[index]++
				var value uint64
				rows := 0
				if !ProjectRows(observation, func(row QueryRow) bool {
					cells, ok := QueryValue(row, token)
					if !ok || cells.Count() != 1 {
						return false
					}
					entry, present, cellOK := cells.At(0)
					if !cellOK || !present {
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
				Semantic: parametricBoundaryKey(uint64(60 + index)),
				Freeze:   func(value uint64) uint64 { stats.freezes[index]++; return value }, Clone: func(value uint64) uint64 { return value },
				Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value },
			},
		}, func(query *Query[uint64]) bool {
			var declared bool
			token, declared = QueryReadFrom(query, reads[index])
			return declared
		})
		if !ok || query == nil {
			t.Fatal("Query declaration")
		}
		queries[index], queryReads[index] = query, token
	}
	if !cold.Seal() {
		t.Fatal("Composition seal")
	}
	decision := func(offset uint64) equation.Decision {
		value, ok := equation.NewDecision(parametricBoundaryKey(offset).compositionKey())
		if !ok {
			t.Fatal("Decision")
		}
		return value
	}
	callLeft, callRight, localSource := decision(100), decision(101), decision(102)
	normalSource, throwSource, yieldSource := decision(103), decision(104), decision(105)
	resumeTarget, localTarget := decision(106), decision(107)
	normalTarget, throwTarget, yieldTarget := decision(108), decision(109), decision(110)
	source, sourceOK := equation.NewScope(callLeft, callRight, localSource, normalSource, throwSource, yieldSource)
	target, targetOK := equation.NewScope(resumeTarget, localTarget, normalTarget, throwTarget, yieldTarget)
	resumeExpr, exprOK := equation.DecisionExpr(resumeTarget)
	boundary, boundaryOK := equation.NewReindex(source, target, []equation.DecisionMap{
		equation.Substitute(callLeft, resumeExpr), equation.Substitute(callRight, resumeExpr),
		equation.Rename(localSource, localTarget), equation.Rename(normalSource, normalTarget), equation.Rename(throwSource, throwTarget), equation.Rename(yieldSource, yieldTarget),
	})
	identity := equation.IdentityReindex(target)
	if !sourceOK || !targetOK || !exprOK || !boundaryOK || !identity.Available() {
		t.Fatal("boundary Scope/Reindex/Expr")
	}
	var instances [7]*RuleInstance[uint64, ruleUnit]
	for index, rule := range rules {
		index, rule := index, rule
		writeIndex := index
		if index >= 3 && index < 6 {
			writeIndex = index - 3
		} else if index == 6 {
			writeIndex = 3
		}
		write, writeOK := factors[writeIndex].Ref(0)
		instance, instanceOK := NewRuleInstance(rule, ruleUnitForSemantic(parametricBoundaryKey(uint64(140+index))), func(binding *RuleBinding[uint64, ruleUnit]) bool {
			if index >= 3 && index < 6 {
				read, readOK := factors[index-3].Ref(0)
				if !readOK || !InstanceRead(binding, ruleReads[index][0], read) {
					return false
				}
			}
			if index == 6 {
				for _, factorIndex := range []int{0, 1} {
					read, readOK := factors[factorIndex].Ref(0)
					if !readOK || !InstanceRead(binding, ruleReads[index][factorIndex], read) {
						return false
					}
				}
			}
			return writeOK && InstanceWrite(binding, ruleWrites[index], write)
		})
		if !writeOK || !instanceOK {
			t.Fatalf("boundary rule instance %d", index)
		}
		instances[index] = instance
	}

	batch := equation.NewBatch()
	var sites [7]equation.Site
	var occurrences [7]equation.Occurrence
	var operands [7]equation.Operand
	for index := range sites {
		scope := target
		init, disposition := equation.FalseExpr(), equation.InitAbsent
		if index < 3 {
			scope, init, disposition = source, equation.TrueExpr(), equation.InitPresent
		}
		site, siteOK := batch.AdmitSite(parametricBoundaryKey(uint64(120+index)).compositionKey(), scope, init, disposition)
		occurrence, occurrenceOK := batch.At(site)
		operand, operandOK := admitInstanceOperand(batch, occurrence, instances[index])
		if !siteOK || !occurrenceOK || !operandOK {
			t.Fatalf("boundary source row %d", index)
		}
		sites[index], occurrences[index], operands[index] = site, occurrence, operand
	}
	if !batch.Seal() {
		t.Fatal("boundary source batch")
	}
	queryInstances := [4]*QueryInstance[uint64]{}
	solver, compiled := assemble(cold, batch, func(assembly *Assembly) bool {
		points := [7]*assemblyPoint{}
		groups := [7]*assemblyGroup{}
		for index := range rules {
			point := admitPoint(assembly, sites[index])
			instance := instances[index]
			member := admitInstance(assembly, point, occurrences[index], operands[index], instance)
			if point == nil || instance == nil || member == nil {
				t.Fatalf("boundary rule assembly %d", index)
			}
			group := admitGroup(assembly, point, member)
			if group == nil {
				t.Fatalf("boundary write assembly %d", index)
			}
			points[index] = point
			groups[index] = group
		}
		for index := 0; index < 3; index++ {
			boundary := equation.BoundaryInput(sites[index], sites[3+index], parametricBoundaryKey(uint64(160+index)).compositionKey(), equation.TrueExpr(), boundary, equation.TrueExpr())
			if !boundary.Available() || !admitBoundary(assembly, groups[3+index], boundary) {
				t.Fatalf("boundary transport %d", index)
			}
		}
		for _, index := range []int{3, 4} {
			input := equation.BoundaryInput(sites[index], sites[6], parametricBoundaryKey(uint64(170+index)).compositionKey(), equation.TrueExpr(), identity, equation.TrueExpr())
			if !input.Available() || !admitBoundary(assembly, groups[6], input) {
				t.Fatalf("resume boundary %d", index)
			}
		}
		for index, query := range queries {
			read, readOK := factors[index].Ref(0)
			queryInstance, queryInstanceOK := NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
				return readOK && InstanceQueryRead(binding, queryReads[index], read)
			})
			queryInstances[index] = queryInstance
			observation := admitQueryAt(assembly, points[3+index], queryInstance)
			if observation == nil || !queryInstanceOK || !readOK {
				t.Fatalf("boundary query assembly %d", index)
			}
		}
		return true
	})
	if !compiled || solver == nil {
		t.Fatal("Solver compilation")
	}
	receipts := [4]QueryReceipt[uint64]{}
	for index, instance := range queryInstances {
		var receiptOK bool
		receipts[index], receiptOK = instance.Receipt()
		if !receiptOK {
			t.Fatalf("boundary query receipt %d", index)
		}
	}
	return parametricBoundaryFixture{solver: solver, queries: queries, receipts: receipts}
}

func assertParametricBoundaryResults(t *testing.T, receipts [4]QueryReceipt[uint64], state *State) {
	t.Helper()
	for index, want := range [4]uint64{11, 22, 33, 1122} {
		got, readable := QueryResult(receipts[index], state)
		if !readable || got != want {
			t.Fatalf("Query %d = value:%d readable:%t, want:%d", index, got, readable, want)
		}
	}
}
