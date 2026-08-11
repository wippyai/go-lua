package engine

import (
	"context"
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/lattice"
)

// These are first-solve hot-path benchmarks: cold declaration and compilation
// happen outside the timed interval, while Solve crosses only public engine
// boundaries. They intentionally report observable callback work instead of
// imposing performance thresholds.
type waveCReadCase struct {
	name  string
	exact int
	whole bool
}

var waveCReadCases = []waveCReadCase{
	{name: "zero"},
	{name: "exact-1", exact: 1},
	{name: "exact-4", exact: 4},
	// Carry is structural predecessor transport, deliberately distinct from
	// the explicit summary/whole-plane read benchmark added beside this matrix.
	{name: "carry-only", whole: true},
}

type waveCProductCase struct {
	factors int
	arity   int
	reads   waveCReadCase
}

// waveCProductCases is a covering array, not a full width×arity×read
// Cartesian product. Every required width, arity, and read shape appears
// repeatedly, and the final case preserves the widest four-input/four-read
// crossing for regression comparison.
var waveCProductCases = []waveCProductCase{
	{factors: 3, arity: 1, reads: waveCReadCases[0]},
	{factors: 3, arity: 2, reads: waveCReadCases[1]},
	{factors: 3, arity: 4, reads: waveCReadCases[2]},
	{factors: 9, arity: 1, reads: waveCReadCases[3]},
	{factors: 9, arity: 2, reads: waveCReadCases[0]},
	{factors: 9, arity: 4, reads: waveCReadCases[1]},
	{factors: 16, arity: 1, reads: waveCReadCases[2]},
	{factors: 16, arity: 2, reads: waveCReadCases[3]},
	{factors: 16, arity: 4, reads: waveCReadCases[0]},
	{factors: 25, arity: 1, reads: waveCReadCases[1]},
	{factors: 25, arity: 2, reads: waveCReadCases[2]},
	{factors: 25, arity: 4, reads: waveCReadCases[3]},
	{factors: 25, arity: 4, reads: waveCReadCases[2]},
}

type waveCCounters struct {
	equal, order, join, widen, narrow int
	transfers, products, stages       int
}

func (c *waveCCounters) add(other waveCCounters) {
	c.equal += other.equal
	c.order += other.order
	c.join += other.join
	c.widen += other.widen
	c.narrow += other.narrow
	c.transfers += other.transfers
	c.products += other.products
	c.stages += other.stages
}

type waveCHotFixture struct {
	solver   *Solver
	query    *Query[uint64]
	receipt  QueryReceipt[uint64]
	counters *waveCCounters
}

// newWaveCHotFixture has two input provenance shapes. shared reuses one
// predecessor Point at every product port; independent gives each port its
// own Point. The latter is deliberately a topology-level distinction, not a
// fabricated Guard or internal carrier handle.
func newWaveCHotFixture(tb testing.TB, factors, arity int, reads waveCReadCase, keys int, shared, recurrent bool) *waveCHotFixture {
	tb.Helper()
	if factors < 1 || arity < 1 || keys < 1 || reads.whole && reads.exact != 0 {
		tb.Fatal("Wave-C benchmark dimensions")
	}
	composition := NewComposition()
	counters := &waveCCounters{}
	owners := make([]*Factor[uint64, uint64], factors)
	readForms := make([]ReadForm[uint64, OrderedCells[uint64]], factors)
	writeForms := make([]WriteForm[uint64], factors)
	carryForms := make([]CarryForm, factors)
	factorKeys := make([]SemanticKey, factors)
	for index := range owners {
		factorKeys[index] = coldKey(uint64(88_000 + index))
		factor, declared := DeclareFactor(composition, FactorSpec[uint64, uint64]{
			Semantic: factorKeys[index], KeyEnd: uint64(keys),
			Lattice: lattice.Lattice[uint64]{
				Bottom: func() uint64 { return 0 }, Top: func() uint64 { return ^uint64(0) },
				Equal:    func(left, right uint64) bool { counters.equal++; return left == right },
				LessOrEq: func(left, right uint64) bool { counters.order++; return left <= right },
				Join: func(left, right uint64) uint64 {
					counters.join++
					if left > right {
						return left
					}
					return right
				},
				Widen: func(left, right uint64) uint64 {
					counters.widen++
					if left > right {
						return left
					}
					return right
				},
				Narrow: func(left, right uint64) uint64 { counters.narrow++; return right },
			},
			Default: 0, AdmitAt: func(uint64, uint64) bool { return true }, Fingerprint: func(value uint64) uint64 { return value },
			WidenRank:  Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }},
			NarrowRank: Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return value }},
		}, func(*Factor[uint64, uint64]) bool { return true })
		if !declared || factor == nil {
			tb.Fatal("Wave-C Factor declaration")
		}
		read, readOK := ExactReadForm(factor)
		write, writeOK := ExactWriteForm(factor)
		carry, carryOK := Carry(factor)
		if !readOK || !writeOK || !carryOK {
			tb.Fatal("Wave-C Factor forms")
		}
		owners[index], readForms[index], writeForms[index], carryForms[index] = factor, read, write, carry
	}

	rules := make([]*Rule[uint64, ruleUnit], factors)
	ruleReadTokens := make([][]Read[OrderedCells[uint64]], factors)
	ruleWriteTokens := make([][]Write[uint64], factors)
	for factorIndex := range rules {
		factorIndex := factorIndex
		var ruleReads []Read[OrderedCells[uint64]]
		var ruleWrites []Write[uint64]
		rule, declared := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily,
			Semantic: coldKey(uint64(88_100 + factorIndex)), Output: owners[factorIndex].Output(), Inputs: arity,
			OperandContent: ruleUnitContent,
			Admission:      testTrustedTheorem[uint64](uint64(88_200 + factorIndex)),
			Transfer: func(access Access[uint64, ruleUnit]) bool {
				counters.transfers++
				return Product(access, func(row Row) bool {
					counters.products++
					for _, read := range ruleReads {
						cells, ok := ReadValue(access, row, read)
						if !ok || cells.Count() != 1 {
							return false
						}
						_, _, valid := cells.At(0)
						if !valid {
							return false
						}
					}
					counters.stages++
					return StageValue(access, row, 1)
				})
			},
		}, func(rule *Rule[uint64, ruleUnit]) bool {
			for readIndex := 0; readIndex < reads.exact; readIndex++ {
				input, inputOK := rule.InputAt(readIndex % arity)
				inputFactor := factorIndex
				if !shared {
					inputFactor = (factorIndex + readIndex) % factors
				}
				read, readOK := ReadFrom(rule, input, readForms[inputFactor])
				if !inputOK || !readOK {
					return false
				}
				ruleReads = append(ruleReads, read)
			}
			if reads.whole {
				input, inputOK := rule.InputAt(0)
				if !inputOK || !CarryFrom(rule, input, carryForms[factorIndex]) {
					return false
				}
			}
			for key := 0; key < keys; key++ {
				write, writeOK := WriteTo(rule, writeForms[factorIndex])
				if !writeOK {
					return false
				}
				ruleWrites = append(ruleWrites, write)
			}
			return true
		})
		if !declared || rule == nil {
			tb.Fatal("Wave-C Rule declaration")
		}
		rules[factorIndex], ruleReadTokens[factorIndex], ruleWriteTokens[factorIndex] = rule, ruleReads, ruleWrites
	}

	queries := make([]*Query[uint64], factors)
	queryReads := make([]QueryRead[OrderedCells[uint64]], factors)
	for factorIndex := range queries {
		factorIndex := factorIndex
		var token QueryRead[OrderedCells[uint64]]
		query, declared := DeclareQuery(composition, QuerySpec[uint64]{
			Semantic: coldKey(uint64(88_300 + factorIndex)),
			Project: func(observation Observation) uint64 {
				var result uint64
				if !ProjectRows(observation, func(row QueryRow) bool {
					cells, ok := QueryValue(row, token)
					if !ok || cells.Count() != 1 {
						return false
					}
					value, present, valid := cells.At(0)
					if !valid {
						return false
					}
					if present {
						result = value
					}
					return true
				}) {
					return 0
				}
				return result
			},
			Result: frozenColdResult(coldKey(uint64(88_400 + factorIndex))),
		}, func(query *Query[uint64]) bool {
			var ok bool
			token, ok = QueryReadFrom(query, readForms[factorIndex])
			return ok
		})
		if !declared || query == nil {
			tb.Fatal("Wave-C Query declaration")
		}
		queries[factorIndex], queryReads[factorIndex] = query, token
	}
	if !composition.Seal() {
		tb.Fatal("Wave-C Composition seal")
	}

	sourcePoints := arity
	if recurrent {
		sourcePoints = 0
	}
	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	if batch == nil || !scope.Available() {
		tb.Fatal("Wave-C source vocabulary")
	}
	sites := make([]equation.Site, sourcePoints+factors)
	for index := range sites {
		init, disposition := equation.FalseExpr(), equation.InitAbsent
		if index < sourcePoints || recurrent {
			init, disposition = equation.TrueExpr(), equation.InitPresent
		}
		site, admitted := batch.AdmitSite(coldKey(uint64(88_500+index)).compositionKey(), scope, init, disposition)
		if !admitted {
			tb.Fatal("Wave-C source site")
		}
		sites[index] = site
	}
	occurrences := make([]equation.Occurrence, factors)
	operands := make([]equation.Operand, factors)
	instances := make([]*RuleInstance[uint64, ruleUnit], factors)
	instanceOK := make([]bool, factors)
	for factorIndex := range rules {
		factorIndex := factorIndex
		occurrence, occurred := batch.Relation(sites[sourcePoints+factorIndex], coldKey(uint64(88_600+factorIndex)).compositionKey())
		instances[factorIndex], instanceOK[factorIndex] = NewRuleInstance(rules[factorIndex], ruleUnitForSemantic(coldKey(uint64(88_700+factorIndex))), func(binding *RuleBinding[uint64, ruleUnit]) bool {
			for readIndex := 0; readIndex < reads.exact; readIndex++ {
				inputFactor := factorIndex
				if !shared {
					inputFactor = (factorIndex + readIndex) % factors
				}
				ref, issued := owners[inputFactor].Ref(0)
				if !issued || !InstanceRead(binding, ruleReadTokens[factorIndex][readIndex], ref) {
					return false
				}
			}
			for key := 0; key < keys; key++ {
				ref, issued := owners[factorIndex].Ref(uint64(key))
				if !issued || !InstanceWrite(binding, ruleWriteTokens[factorIndex][key], ref) {
					return false
				}
			}
			return true
		})
		operand, admitted := admitInstanceOperand(batch, occurrence, instances[factorIndex])
		if !occurred || !instanceOK[factorIndex] || !admitted {
			tb.Fatal("Wave-C source operand")
		}
		occurrences[factorIndex], operands[factorIndex] = occurrence, operand
	}
	if !batch.Seal() {
		tb.Fatal("Wave-C source seal")
	}
	queryInstances := make([]*QueryInstance[uint64], len(queries))
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		points := make([]*assemblyPoint, len(sites))
		for index, site := range sites {
			points[index] = admitPoint(assembly, site)
			if points[index] == nil {
				tb.Fatal("Wave-C source point")
			}
		}
		for factorIndex := range rules {
			member := admitInstance(assembly, points[sourcePoints+factorIndex], occurrences[factorIndex], operands[factorIndex], instances[factorIndex])
			if !instanceOK[factorIndex] || member == nil {
				tb.Fatal("Wave-C rule assembly")
			}
			group := admitGroup(assembly, points[sourcePoints+factorIndex], member)
			if group == nil {
				tb.Fatal("Wave-C group assembly")
			}
			for inputIndex := 0; inputIndex < arity; inputIndex++ {
				pointIndex := 0
				if recurrent {
					pointIndex = sourcePoints + factorIndex
					if !shared {
						pointIndex = sourcePoints + (factorIndex+inputIndex)%factors
					}
				} else if !shared {
					pointIndex = inputIndex
				}
				boundary := equation.BoundaryInput(sites[pointIndex], sites[sourcePoints+factorIndex], coldKey(uint64(88_800+factorIndex*arity+inputIndex)).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
				if !admitBoundary(assembly, group, boundary) {
					tb.Fatal("Wave-C boundary assembly")
				}
			}
			ref, issued := owners[factorIndex].Ref(0)
			queryInstance, queryInstanceOK := NewQueryInstance(queries[factorIndex], func(binding *QueryBinding[uint64]) bool {
				return issued && InstanceQueryRead(binding, queryReads[factorIndex], ref)
			})
			queryInstances[factorIndex] = queryInstance
			observation := admitQueryAt(assembly, points[sourcePoints+factorIndex], queryInstance)
			if observation == nil || !issued || !queryInstanceOK {
				tb.Fatal("Wave-C query assembly")
			}
		}
		return true
	})
	if !compiled || solver == nil {
		tb.Fatal("Wave-C solver compile")
	}
	receipt, receiptOK := queryInstances[0].Receipt()
	if !receiptOK {
		tb.Fatal("Wave-C query receipt")
	}
	return &waveCHotFixture{solver: solver, query: queries[0], receipt: receipt, counters: counters}
}

func (fixture *waveCHotFixture) solve(tb testing.TB) {
	tb.Helper()
	state, status := fixture.solver.Solve(context.Background())
	if status != SolveComplete || state == nil {
		tb.Fatal("Wave-C solve")
	}
	if value, readable := QueryResult(fixture.receipt, state); !readable || value != 1 {
		tb.Fatal("Wave-C query")
	}
}

func waveCReport(b *testing.B, counters waveCCounters) {
	divisor := float64(b.N)
	b.ReportMetric(float64(counters.equal)/divisor, "equal-calls/op")
	b.ReportMetric(float64(counters.order)/divisor, "order-calls/op")
	b.ReportMetric(float64(counters.join)/divisor, "join-calls/op")
	b.ReportMetric(float64(counters.widen)/divisor, "widen-calls/op")
	b.ReportMetric(float64(counters.narrow)/divisor, "narrow-calls/op")
	b.ReportMetric(float64(counters.transfers)/divisor, "transfer-calls/op")
	b.ReportMetric(float64(counters.products)/divisor, "product-rows/op")
	b.ReportMetric(float64(counters.stages)/divisor, "stage-calls/op")
}

func BenchmarkWaveCHotProductMatrix(b *testing.B) {
	for _, matrix := range waveCProductCases {
		matrix := matrix
		name := "factors=" + strconv.Itoa(matrix.factors) + "/arity=" + strconv.Itoa(matrix.arity) + "/reads=" + matrix.reads.name
		b.Run(name, func(b *testing.B) {
			var total waveCCounters
			b.ReportAllocs()
			for iteration := 0; iteration < b.N; iteration++ {
				b.StopTimer()
				fixture := newWaveCHotFixture(b, matrix.factors, matrix.arity, matrix.reads, 1, true, false)
				b.StartTimer()
				fixture.solve(b)
				total.add(*fixture.counters)
			}
			b.StopTimer()
			b.ReportMetric(float64(matrix.factors), "factor-width/op")
			b.ReportMetric(float64(matrix.arity), "arity/op")
			b.ReportMetric(float64(matrix.reads.exact), "exact-reads/op")
			b.ReportMetric(boolMetric(matrix.reads.whole), "carry-only/op")
			waveCReport(b, total)
		})
	}
}

// This engine matrix varies Point provenance only. Actual shared/independent
// Guard-region coverage lives in carrier/wave_c_operations_benchmark_test.go.
func BenchmarkWaveCHotKeyPointMatrix(b *testing.B) {
	for _, keys := range []int{1, 16, 128} {
		for _, shared := range []bool{true, false} {
			name := "keys=" + strconv.Itoa(keys) + "/points=" + guardShapeName(shared)
			b.Run(name, func(b *testing.B) {
				var total waveCCounters
				b.ReportAllocs()
				for iteration := 0; iteration < b.N; iteration++ {
					b.StopTimer()
					fixture := newWaveCHotFixture(b, 9, 4, waveCReadCase{name: "exact-4", exact: 4}, keys, shared, false)
					b.StartTimer()
					fixture.solve(b)
					total.add(*fixture.counters)
				}
				b.StopTimer()
				b.ReportMetric(float64(keys), "populated-keys/op")
				b.ReportMetric(boolMetric(shared), "shared-points/op")
				b.ReportMetric(boolMetric(!shared), "independent-points/op")
				waveCReport(b, total)
			})
		}
	}
}

// BenchmarkWaveCHotExplicitWholePlaneRead is intentionally separate from
// Carry-only: it binds a declared SummaryReadForm over every populated key.
func BenchmarkWaveCHotExplicitWholePlaneRead(b *testing.B) {
	for _, keys := range []int{1, 16, 128} {
		b.Run("keys="+strconv.Itoa(keys), func(b *testing.B) {
			b.ReportAllocs()
			for iteration := 0; iteration < b.N; iteration++ {
				b.StopTimer()
				fixture := newWaveCSummaryFixture(b, keys)
				b.StartTimer()
				fixture.solve(b)
			}
			b.StopTimer()
			b.ReportMetric(float64(keys), "summary-keys/op")
			b.ReportMetric(1, "whole-plane-read/op")
		})
	}
}

func newWaveCSummaryFixture(tb testing.TB, keys int) *waveCHotFixture {
	tb.Helper()
	composition := NewComposition()
	var summary ReadForm[uint64, uint64]
	source, sourceOK := DeclareFactor(composition, FactorSpec[uint64, uint64]{
		Semantic: coldKey(89_000), KeyEnd: uint64(keys), Lattice: coldUintLattice(), Default: 0,
		AdmitAt: func(uint64, uint64) bool { return true }, Fingerprint: func(value uint64) uint64 { return value },
	}, func(factor *Factor[uint64, uint64]) bool {
		normalizer, normalizerOK := DeclareNormalizer(factor, coldKey(89_001), func(cells OrderedCells[uint64]) uint64 { return uint64(cells.Count()) }, func(left, right uint64) bool { return left == right }, func(value uint64) uint64 { return value })
		if !normalizerOK {
			return false
		}
		var summaryOK bool
		summary, summaryOK = SummaryReadForm(normalizer)
		return summaryOK
	})
	sink, sinkOK := DeclareFactor(composition, FactorSpec[uint64, uint64]{
		Semantic: coldKey(89_002), KeyEnd: 1, Lattice: coldUintLattice(), Default: 0,
		AdmitAt: func(uint64, uint64) bool { return true }, Fingerprint: func(value uint64) uint64 { return value },
	}, func(*Factor[uint64, uint64]) bool { return true })
	if !sourceOK || !sinkOK || source == nil || sink == nil {
		tb.Fatal("Wave-C summary Factors")
	}
	_, sourceReadOK := ExactReadForm(source)
	sourceWrite, sourceWriteOK := ExactWriteForm(source)
	sinkRead, sinkReadOK := ExactReadForm(sink)
	sinkWrite, sinkWriteOK := ExactWriteForm(sink)
	if !sourceReadOK || !sourceWriteOK || !sinkReadOK || !sinkWriteOK {
		tb.Fatal("Wave-C summary forms")
	}
	seedWrites := make([]Write[uint64], 0, keys)
	seed, seedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent, Semantic: coldKey(89_010), Output: source.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](89_110), Transfer: func(access Access[uint64, ruleUnit]) bool {
		return Product(access, func(row Row) bool { return StageValue(access, row, 1) })
	}}, func(rule *Rule[uint64, ruleUnit]) bool {
		for key := 0; key < keys; key++ {
			write, ok := WriteTo(rule, sourceWrite)
			if !ok {
				return false
			}
			seedWrites = append(seedWrites, write)
		}
		return true
	})
	var summaryRead Read[uint64]
	var projectWrite Write[uint64]
	project, projectOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent, Semantic: coldKey(89_011), Output: sink.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](89_111), Transfer: func(access Access[uint64, ruleUnit]) bool {
		return Product(access, func(row Row) bool {
			value, ok := ReadValue(access, row, summaryRead)
			return ok && value == uint64(keys) && StageValue(access, row, 1)
		})
	}}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var readOK bool
		summaryRead, readOK = ReadFrom(rule, input, summary)
		var writeOK bool
		projectWrite, writeOK = WriteTo(rule, sinkWrite)
		return inputOK && readOK && writeOK
	})
	var token QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{Semantic: coldKey(89_012), Project: func(observation Observation) uint64 {
		var value uint64
		if !ProjectRows(observation, func(row QueryRow) bool {
			cells, ok := QueryValue(row, token)
			entry, present, valid := cells.At(0)
			if !ok || cells.Count() != 1 || !valid || !present {
				return false
			}
			value = entry
			return true
		}) {
			return 0
		}
		return value
	}, Result: frozenColdResult(coldKey(89_112))}, func(query *Query[uint64]) bool { var ok bool; token, ok = QueryReadFrom(query, sinkRead); return ok })
	if !seedOK || !projectOK || !queryOK || seed == nil || project == nil || query == nil || !composition.Seal() {
		tb.Fatal("Wave-C summary declarations")
	}
	sourceRefs := make([]Ref[uint64], keys)
	for key := range sourceRefs {
		ref, issued := source.Ref(uint64(key))
		if !issued {
			tb.Fatal("Wave-C summary source reference")
		}
		sourceRefs[key] = ref
	}
	sinkRef, sinkRefOK := sink.Ref(0)
	closed := source.NewClosedRefs()
	if closed == nil {
		tb.Fatal("Wave-C summary closed references")
	}
	for _, ref := range sourceRefs {
		if !closed.Append(ref) {
			tb.Fatal("Wave-C summary closed reference")
		}
	}
	if !closed.Close() {
		tb.Fatal("Wave-C summary coverage")
	}
	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	seedSite, seedSiteOK := batch.AdmitSite(coldKey(89_020).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	projectSite, projectSiteOK := batch.AdmitSite(coldKey(89_021).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
	seedOccurrence, seedOccurrenceOK := batch.Relation(seedSite, coldKey(89_022).compositionKey())
	projectOccurrence, projectOccurrenceOK := batch.Relation(projectSite, coldKey(89_023).compositionKey())
	seedInstance, seedInstanceOK := NewRuleInstance(seed, ruleUnitForSemantic(coldKey(89_024)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		for index, ref := range sourceRefs {
			if !InstanceWrite(binding, seedWrites[index], ref) {
				return false
			}
		}
		return true
	})
	projectInstance, projectInstanceOK := NewRuleInstance(project, ruleUnitForSemantic(coldKey(89_025)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceSummaryRead(binding, summaryRead, summary, closed) && InstanceWrite(binding, projectWrite, sinkRef)
	})
	seedOperand, seedOperandOK := admitInstanceOperand(batch, seedOccurrence, seedInstance)
	projectOperand, projectOperandOK := admitInstanceOperand(batch, projectOccurrence, projectInstance)
	if !scope.Available() || !sinkRefOK || !seedSiteOK || !projectSiteOK || !seedOccurrenceOK || !projectOccurrenceOK ||
		!seedInstanceOK || !projectInstanceOK || !seedOperandOK || !projectOperandOK || !batch.Seal() {
		tb.Fatal("Wave-C summary source")
	}
	boundary := equation.BoundaryInput(seedSite, projectSite, coldKey(89_026).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		seedPoint := admitPoint(assembly, seedSite)
		projectPoint := admitPoint(assembly, projectSite)
		seedMember := admitInstance(assembly, seedPoint, seedOccurrence, seedOperand, seedInstance)
		projectMember := admitInstance(assembly, projectPoint, projectOccurrence, projectOperand, projectInstance)
		if seedPoint == nil || projectPoint == nil || !seedInstanceOK || !projectInstanceOK || seedMember == nil || projectMember == nil || admitGroup(assembly, seedPoint, seedMember) == nil {
			tb.Fatal("Wave-C summary form assembly")
		}
		projectGroup := admitGroup(assembly, projectPoint, projectMember)
		if projectGroup == nil || !admitBoundary(assembly, projectGroup, boundary) {
			tb.Fatal("Wave-C summary group assembly")
		}
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, token, sinkRef)
		})
		observation := admitQueryAt(assembly, projectPoint, queryInstance)
		if observation == nil || !queryInstanceOK {
			tb.Fatal("Wave-C summary query assembly")
		}
		return true
	})
	if !compiled || solver == nil {
		tb.Fatal("Wave-C summary compile")
	}
	receipt, receiptOK := queryInstance.Receipt()
	if !receiptOK {
		tb.Fatal("Wave-C summary query receipt")
	}
	return &waveCHotFixture{solver: solver, query: query, receipt: receipt, counters: &waveCCounters{}}
}

// There is no public delta-publication counter. stage-calls/op is the closest
// lawful public observation: a successful StageValue must be admitted before
// the completed QueryResult can expose its recurrence output.
func BenchmarkWaveCHotRecurrentSolve(b *testing.B) {
	for _, factors := range []int{3, 9, 16, 25} {
		for _, keys := range []int{1, 16, 128} {
			name := "factors=" + strconv.Itoa(factors) + "/keys=" + strconv.Itoa(keys)
			b.Run(name, func(b *testing.B) {
				var total waveCCounters
				b.ReportAllocs()
				for iteration := 0; iteration < b.N; iteration++ {
					b.StopTimer()
					fixture := newWaveCHotFixture(b, factors, 1, waveCReadCase{name: "exact-1", exact: 1}, keys, true, true)
					b.StartTimer()
					fixture.solve(b)
					total.add(*fixture.counters)
				}
				b.StopTimer()
				b.ReportMetric(float64(factors), "factor-width/op")
				b.ReportMetric(float64(keys), "populated-keys/op")
				b.ReportMetric(1, "recurrent-solve/op")
				waveCReport(b, total)
			})
		}
	}
}

func guardShapeName(shared bool) string {
	if shared {
		return "shared"
	}
	return "independent"
}

func boolMetric(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
