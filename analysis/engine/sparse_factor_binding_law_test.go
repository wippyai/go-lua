package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/lattice"
)

type sparseCarryKey uint64

const sparseCarryObservedKey sparseCarryKey = 7

const (
	carriedLaneKey   sparseCarryKey = 3
	unrelatedLaneKey sparseCarryKey = 11
)

// TestSparseTypedCarryPreservesReferencedKey proves that Carry transports the
// complete opaque factor root while binding only the exact graph surfaces that
// can be observed or written. The legal key universe is deliberately much
// larger than the single typed key named by the topology.
func TestSparseTypedCarryPreservesReferencedKey(t *testing.T) {
	_, receipt, solver := sparseCarryFixture(t, 1<<16)
	compiled := solver != nil
	if !compiled || solver == nil {
		t.Fatal("sparse carry solver")
	}
	state, status := solver.Solve(context.Background())
	if status != SolveComplete || state == nil {
		t.Fatalf("sparse carry solve = state:%v status:%v", state, status)
	}
	value, readable := QueryResult(receipt, state)
	if !readable || value != 73 {
		t.Fatalf("sparse carry query = %d readable:%t, want 73:true", value, readable)
	}
}

// TestCarryRecurrenceKeepsDisjointFactorLaneExact places two keys of one
// Factor in one cyclic point region. The back edge carries only lane A from
// its predecessor; lane B is an external head producer. Widen deliberately
// turns the pair (10, Default) into 99, so observing B=10 proves its target
// was not admitted into A's carry-derived recurrence scope.
func TestCarryRecurrenceKeepsDisjointFactorLaneExact(t *testing.T) {
	composition := NewComposition()
	factor, declared := DeclareFactor(composition, FactorSpec[sparseCarryKey, uint64]{
		Semantic: coldKey(98_100), KeyEnd: 32,
		Lattice: lattice.Lattice[uint64]{
			Bottom: func() uint64 { return 0 }, Top: func() uint64 { return ^uint64(0) },
			Equal: func(left, right uint64) bool { return left == right }, LessOrEq: func(left, right uint64) bool { return left <= right },
			Join: func(left, right uint64) uint64 {
				if left > right {
					return left
				}
				return right
			},
			Widen: func(left, right uint64) uint64 {
				if left == 10 && right == 0 {
					return 99
				}
				if left > right {
					return left
				}
				return right
			},
		},
		Default: 0, AdmitAt: func(sparseCarryKey, uint64) bool { return true }, Fingerprint: func(value uint64) uint64 { return value },
		WidenRank: Measure[sparseCarryKey, uint64]{Width: 1, At: func(_ sparseCarryKey, value uint64, _ int) uint64 { return ^value }},
	}, func(*Factor[sparseCarryKey, uint64]) bool { return true })
	if !declared || factor == nil {
		t.Fatal("lane factor")
	}
	read, readOK := ExactReadForm(factor)
	write, writeOK := ExactWriteForm(factor)
	carry, carryOK := Carry(factor)
	if !readOK || !writeOK || !carryOK {
		t.Fatal("lane forms")
	}
	writeRule := func(semantic SemanticKey, theorem, value uint64) (*Rule[uint64, ruleUnit], Write[uint64]) {
		var token Write[uint64]
		rule, ok := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
			Semantic: semantic, Output: factor.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](theorem),
			Transfer: func(access Access[uint64, ruleUnit]) bool {
				return Product(access, func(row Row) bool { return StageValue(access, row, value) })
			},
		}, func(rule *Rule[uint64, ruleUnit]) bool {
			var ok bool
			token, ok = WriteTo(rule, write)
			return ok
		})
		if !ok {
			return nil, Write[uint64]{}
		}
		return rule, token
	}
	laneB, laneBWrite := writeRule(coldKey(98_101), 98_201, 10)
	if laneB == nil {
		t.Fatal("lane B rule")
	}
	var laneAWrite Write[uint64]
	laneA, laneADeclared := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(98_102), Output: factor.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](98_202),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var ok bool
		laneAWrite, ok = WriteTo(rule, write)
		return ok
	})
	if !laneADeclared || laneA == nil {
		t.Fatal("lane A rule")
	}
	carryRule, carryDeclared := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(98_103), Output: factor.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](98_203),
		Transfer: func(access Access[uint64, ruleUnit]) bool { return Product(access, func(Row) bool { return true }) },
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, ok := rule.InputAt(0)
		return ok && CarryFrom(rule, input, carry)
	})
	if !carryDeclared || carryRule == nil {
		t.Fatal("lane carry rule")
	}
	var aToken, bToken QueryRead[OrderedCells[uint64]]
	query, queryDeclared := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(98_104),
		Project: func(observation Observation) uint64 {
			var values [2]uint64
			rows := 0
			if !ProjectRows(observation, func(row QueryRow) bool {
				for index, token := range []QueryRead[OrderedCells[uint64]]{aToken, bToken} {
					cells, ok := QueryValue(row, token)
					if !ok || cells.Count() != 1 {
						return false
					}
					value, present, valid := cells.At(0)
					if !valid || !present {
						return false
					}
					values[index] = value
				}
				rows++
				return true
			}) || rows != 1 {
				return 0
			}
			return values[0]<<8 | values[1]
		},
		Result: frozenColdResult(coldKey(98_105)),
	}, func(query *Query[uint64]) bool {
		var aOK, bOK bool
		aToken, aOK = QueryReadFrom(query, read)
		bToken, bOK = QueryReadFrom(query, read)
		return aOK && bOK
	})
	if !queryDeclared || query == nil || !composition.Seal() {
		t.Fatal("lane query/composition")
	}
	readA, readAOK := factor.Ref(carriedLaneKey)
	readB, readBOK := factor.Ref(unrelatedLaneKey)
	writeA, writeAOK := factor.Ref(carriedLaneKey)
	writeB, writeBOK := factor.Ref(unrelatedLaneKey)
	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	headSite, headSiteOK := batch.AdmitSite(coldKey(98_106).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	laneSite, laneSiteOK := batch.AdmitSite(coldKey(98_107).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
	laneBOccurrence, laneBOccurrenceOK := batch.Relation(headSite, coldKey(98_108).compositionKey())
	laneAOccurrence, laneAOccurrenceOK := batch.Relation(laneSite, coldKey(98_109).compositionKey())
	carryOccurrence, carryOccurrenceOK := batch.Relation(headSite, coldKey(98_110).compositionKey())
	laneBInstance, laneBOK := NewRuleInstance(laneB, ruleUnitForSemantic(coldKey(98_111)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, laneBWrite, writeB)
	})
	laneAInstance, laneAOK := NewRuleInstance(laneA, ruleUnitForSemantic(coldKey(98_112)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, laneAWrite, writeA)
	})
	carryInstance, carryOK := NewRuleInstance(carryRule, ruleUnitForSemantic(coldKey(98_113)), func(*RuleBinding[uint64, ruleUnit]) bool { return true })
	laneBOperand, laneBOperandOK := admitInstanceOperand(batch, laneBOccurrence, laneBInstance)
	laneAOperand, laneAOperandOK := admitInstanceOperand(batch, laneAOccurrence, laneAInstance)
	carryOperand, carryOperandOK := admitInstanceOperand(batch, carryOccurrence, carryInstance)
	if !scope.Available() || !headSiteOK || !laneSiteOK || !laneBOccurrenceOK || !laneAOccurrenceOK || !carryOccurrenceOK ||
		!laneBOK || !laneAOK || !carryOK || !laneBOperandOK || !laneAOperandOK || !carryOperandOK || !readAOK || !readBOK || !writeAOK || !writeBOK || !batch.Seal() {
		t.Fatal("lane source")
	}
	headToLane := equation.BoundaryInput(headSite, laneSite, coldKey(98_114).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	laneToHead := equation.BoundaryInput(laneSite, headSite, coldKey(98_115).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		headPoint := admitPoint(assembly, headSite)
		lanePoint := admitPoint(assembly, laneSite)
		laneBMember := admitInstance(assembly, headPoint, laneBOccurrence, laneBOperand, laneBInstance)
		laneAMember := admitInstance(assembly, lanePoint, laneAOccurrence, laneAOperand, laneAInstance)
		carryMember := admitInstance(assembly, headPoint, carryOccurrence, carryOperand, carryInstance)
		if headPoint == nil || lanePoint == nil || !laneBOK || !laneAOK || !carryOK || laneBMember == nil || laneAMember == nil || carryMember == nil || admitGroup(assembly, headPoint, laneBMember) == nil {
			return false
		}
		laneGroup := admitGroup(assembly, lanePoint, laneAMember)
		carryGroup := admitGroup(assembly, headPoint, carryMember)
		if laneGroup == nil || carryGroup == nil || !admitBoundary(assembly, laneGroup, headToLane) || !admitBoundary(assembly, carryGroup, laneToHead) {
			return false
		}
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, aToken, readA) && InstanceQueryRead(binding, bToken, readB)
		})
		return queryInstanceOK && admitQueryAt(assembly, headPoint, queryInstance) != nil
	})
	if !compiled || solver == nil {
		t.Fatal("lane solver")
	}
	state, status := solver.Solve(context.Background())
	if status != SolveComplete || state == nil {
		t.Fatalf("lane solve = state:%v status:%v", state, status)
	}
	receipt, receiptOK := queryInstance.Receipt()
	if !receiptOK {
		t.Fatal("lane query receipt")
	}
	value, readable := QueryResult(receipt, state)
	if !readable || value != 0x010a {
		t.Fatalf("lane query = %#x readable:%t, want 0x010a:true", value, readable)
	}
}

// TestCarryRecurrenceUnionsDirectWriteWithExternalCarry gives one back-edge
// member an external Carry input and a direct output write. The predecessor
// slice owns A while the member owns B. The inner edge carries the previous
// head root, so B advances from 10 to 11 and Widen(10, 11)=99 makes B's
// recurrence membership observable. No Narrow is declared: the completed
// result must nevertheless be the exact B=11 post-fixpoint, and that descent
// must not restart the Carry SCC into an unbounded widen/restart cycle.
func TestCarryRecurrenceUnionsDirectWriteWithExternalCarry(t *testing.T) {
	composition := NewComposition()
	factor, declared := DeclareFactor(composition, FactorSpec[sparseCarryKey, uint64]{
		Semantic: coldKey(98_200), KeyEnd: 32,
		Lattice: lattice.Lattice[uint64]{
			Bottom: func() uint64 { return 0 }, Top: func() uint64 { return ^uint64(0) },
			Equal: func(left, right uint64) bool { return left == right }, LessOrEq: func(left, right uint64) bool { return left <= right },
			Join: func(left, right uint64) uint64 {
				if left > right {
					return left
				}
				return right
			},
			Widen: func(left, right uint64) uint64 {
				if left == 10 && right == 11 {
					return 99
				}
				if left > right {
					return left
				}
				return right
			},
		},
		Default: 0, AdmitAt: func(sparseCarryKey, uint64) bool { return true }, Fingerprint: func(value uint64) uint64 { return value },
		WidenRank: Measure[sparseCarryKey, uint64]{Width: 1, At: func(_ sparseCarryKey, value uint64, _ int) uint64 { return ^value }},
	}, func(*Factor[sparseCarryKey, uint64]) bool { return true })
	if !declared || factor == nil {
		t.Fatal("mixed factor")
	}
	read, readOK := ExactReadForm(factor)
	write, writeOK := ExactWriteForm(factor)
	carry, carryOK := Carry(factor)
	if !readOK || !writeOK || !carryOK {
		t.Fatal("mixed forms")
	}
	seedCalls, backCalls, mixedCalls := 0, 0, 0
	var seedWrite Write[uint64]
	seed, seedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(98_201), Output: factor.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](98_301),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			seedCalls++
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(3)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool { var ok bool; seedWrite, ok = WriteTo(rule, write); return ok })
	if !seedOK || seed == nil {
		t.Fatal("mixed seed")
	}
	back, backOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(98_202), Output: factor.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](98_302),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			backCalls++
			return Product(access, func(Row) bool { return true })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, ok := rule.InputAt(0)
		return ok && CarryFrom(rule, input, carry)
	})
	if !backOK || back == nil {
		t.Fatal("mixed back")
	}
	var innerB Read[OrderedCells[uint64]]
	var mixedWrite Write[uint64]
	mixed, mixedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(98_203), Output: factor.Output(), Inputs: 2, Admission: testTrustedTheorem[uint64](98_303),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			mixedCalls++
			return Product(access, func(row Row) bool {
				cells, ok := ReadValue(access, row, innerB)
				if !ok || cells.Count() != 1 {
					return false
				}
				_, present, valid := cells.At(0)
				if !valid {
					return false
				}
				value := uint64(10)
				if present {
					value = 11
				}
				return StageValue(access, row, value)
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		external, externalOK := rule.InputAt(0)
		internal, internalOK := rule.InputAt(1)
		if !externalOK || !internalOK || !CarryFrom(rule, external, carry) {
			return false
		}
		var readOK bool
		innerB, readOK = ReadFrom(rule, internal, read)
		if !readOK {
			return false
		}
		var ok bool
		mixedWrite, ok = WriteTo(rule, write)
		return ok
	})
	if !mixedOK || mixed == nil {
		t.Fatal("mixed carry/write")
	}
	var aToken, bToken QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(98_204),
		Project: func(observation Observation) uint64 {
			var values [2]uint64
			rows := 0
			if !ProjectRows(observation, func(row QueryRow) bool {
				for index, token := range []QueryRead[OrderedCells[uint64]]{aToken, bToken} {
					cells, ok := QueryValue(row, token)
					if !ok || cells.Count() != 1 {
						return false
					}
					value, present, valid := cells.At(0)
					if !valid || !present {
						return false
					}
					values[index] = value
				}
				rows++
				return true
			}) || rows != 1 {
				return 0
			}
			return values[0]<<8 | values[1]
		},
		Result: frozenColdResult(coldKey(98_205)),
	}, func(query *Query[uint64]) bool {
		var aOK, bOK bool
		aToken, aOK = QueryReadFrom(query, read)
		bToken, bOK = QueryReadFrom(query, read)
		return aOK && bOK
	})
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("mixed query/composition")
	}
	readA, readAOK := factor.Ref(carriedLaneKey)
	readB, readBOK := factor.Ref(unrelatedLaneKey)
	writeA, writeAOK := factor.Ref(carriedLaneKey)
	writeB, writeBOK := factor.Ref(unrelatedLaneKey)
	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	externalSite, externalSiteOK := batch.AdmitSite(coldKey(98_206).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	headSite, headSiteOK := batch.AdmitSite(coldKey(98_207).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	innerSite, innerSiteOK := batch.AdmitSite(coldKey(98_208).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
	seedOccurrence, seedOccurrenceOK := batch.Relation(externalSite, coldKey(98_209).compositionKey())
	backOccurrence, backOccurrenceOK := batch.Relation(innerSite, coldKey(98_210).compositionKey())
	mixedOccurrence, mixedOccurrenceOK := batch.Relation(headSite, coldKey(98_211).compositionKey())
	seedInstance, seedInstanceOK := NewRuleInstance(seed, ruleUnitForSemantic(coldKey(98_212)), func(binding *RuleBinding[uint64, ruleUnit]) bool { return InstanceWrite(binding, seedWrite, writeA) })
	backInstance, backInstanceOK := NewRuleInstance(back, ruleUnitForSemantic(coldKey(98_213)), func(*RuleBinding[uint64, ruleUnit]) bool { return true })
	mixedInstance, mixedInstanceOK := NewRuleInstance(mixed, ruleUnitForSemantic(coldKey(98_214)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceRead(binding, innerB, readB) && InstanceWrite(binding, mixedWrite, writeB)
	})
	seedOperand, seedOperandOK := admitInstanceOperand(batch, seedOccurrence, seedInstance)
	backOperand, backOperandOK := admitInstanceOperand(batch, backOccurrence, backInstance)
	mixedOperand, mixedOperandOK := admitInstanceOperand(batch, mixedOccurrence, mixedInstance)
	if !scope.Available() || !externalSiteOK || !headSiteOK || !innerSiteOK || !seedOccurrenceOK || !backOccurrenceOK || !mixedOccurrenceOK ||
		!seedInstanceOK || !backInstanceOK || !mixedInstanceOK || !seedOperandOK || !backOperandOK || !mixedOperandOK || !readAOK || !readBOK || !writeAOK || !writeBOK || !batch.Seal() {
		t.Fatal("mixed source")
	}
	headToInner := equation.BoundaryInput(headSite, innerSite, coldKey(98_215).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	externalToHead := equation.BoundaryInput(externalSite, headSite, coldKey(98_216).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	innerToHead := equation.BoundaryInput(innerSite, headSite, coldKey(98_217).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		externalPoint := admitPoint(assembly, externalSite)
		headPoint := admitPoint(assembly, headSite)
		innerPoint := admitPoint(assembly, innerSite)
		seedMember := admitInstance(assembly, externalPoint, seedOccurrence, seedOperand, seedInstance)
		backMember := admitInstance(assembly, innerPoint, backOccurrence, backOperand, backInstance)
		mixedMember := admitInstance(assembly, headPoint, mixedOccurrence, mixedOperand, mixedInstance)
		if externalPoint == nil || headPoint == nil || innerPoint == nil || seedMember == nil || backMember == nil || mixedMember == nil || admitGroup(assembly, externalPoint, seedMember) == nil {
			return false
		}
		backGroup := admitGroup(assembly, innerPoint, backMember)
		mixedGroup := admitGroup(assembly, headPoint, mixedMember)
		if backGroup == nil || mixedGroup == nil || !admitBoundary(assembly, backGroup, headToInner) || !admitBoundary(assembly, mixedGroup, externalToHead) || !admitBoundary(assembly, mixedGroup, innerToHead) {
			return false
		}
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, aToken, readA) && InstanceQueryRead(binding, bToken, readB)
		})
		return queryInstanceOK && admitQueryAt(assembly, headPoint, queryInstance) != nil
	})
	if !compiled || solver == nil {
		t.Fatal("mixed solver")
	}
	state, status := solver.Solve(context.Background())
	if status != SolveComplete || state == nil {
		t.Fatalf("mixed solve = state:%v status:%v", state, status)
	}
	receipt, receiptOK := queryInstance.Receipt()
	if !receiptOK {
		t.Fatal("mixed query receipt")
	}
	value, readable := QueryResult(receipt, state)
	if !readable || value != 0x030b {
		t.Fatalf("mixed query = %#x readable:%t, want 0x030b:true", value, readable)
	}
	if seedCalls != 1 || backCalls > 8 || mixedCalls > 8 {
		t.Fatalf("mixed exact descent callbacks = seed:%d back:%d mixed:%d, want seed:1 and finite carry refolds", seedCalls, backCalls, mixedCalls)
	}
}

// BenchmarkSparseFactorBindingKeyEnd measures compilation with a fixed single
// typed key surface as KeyEnd grows. Factor declaration intentionally remains
// outside the timed section because its universal Default-admission validation
// is a distinct cold schema law.
func BenchmarkSparseFactorBindingKeyEnd(b *testing.B) {
	for _, keyEnd := range []uint64{32, 1 << 18} {
		b.Run("key-end="+itoaUint(keyEnd), func(b *testing.B) {
			b.ReportAllocs()
			for iteration := 0; iteration < b.N; iteration++ {
				_, _, solver := sparseCarryFixture(b, keyEnd)
				if solver == nil {
					b.Fatal("sparse binding compile")
				}
			}
		})
	}
}

// BenchmarkCarryClosureTopologyScale records bounded cold compilation of one
// Factor's cyclic Carry region. The graph-owned (Factor, Point) closure is
// condensed once; every carrying member consumes that immutable result with no
// recursive predecessor walk, cap, or runtime topology search.
func BenchmarkCarryClosureTopologyScale(b *testing.B) {
	for _, carries := range []int{1, 8, 32, 128} {
		b.Run("carries="+itoaUint(uint64(carries)), func(b *testing.B) {
			b.ReportAllocs()
			for iteration := 0; iteration < b.N; iteration++ {
				solver := sparseCarryTopologyScaleFixture(b, carries)
				if solver == nil {
					b.Fatal("carry topology compile")
				}
			}
		})
	}
}

func sparseCarryTopologyScaleFixture(tb testing.TB, carries int) *Solver {
	tb.Helper()
	if carries < 1 {
		tb.Fatal("carry count")
	}
	composition := NewComposition()
	factor, declared := DeclareFactor(composition, FactorSpec[sparseCarryKey, uint64]{
		Semantic: coldKey(98_500), KeyEnd: 32, Lattice: coldUintLattice(), Default: 0,
		AdmitAt: func(sparseCarryKey, uint64) bool { return true }, Fingerprint: func(value uint64) uint64 { return value },
		WidenRank: Measure[sparseCarryKey, uint64]{Width: 1, At: func(_ sparseCarryKey, value uint64, _ int) uint64 { return ^value }},
	}, func(*Factor[sparseCarryKey, uint64]) bool { return true })
	if !declared || factor == nil {
		tb.Fatal("carry scale factor")
	}
	read, readOK := ExactReadForm(factor)
	write, writeOK := ExactWriteForm(factor)
	carry, carryOK := Carry(factor)
	if !readOK || !writeOK || !carryOK {
		tb.Fatal("carry scale forms")
	}
	var seedWrite Write[uint64]
	seed, seedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(98_501), Output: factor.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](98_601),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool { var ok bool; seedWrite, ok = WriteTo(rule, write); return ok })
	if !seedOK || seed == nil {
		tb.Fatal("carry scale seed")
	}
	carryRule, carryRuleOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(98_502), Output: factor.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](98_602),
		Transfer: func(access Access[uint64, ruleUnit]) bool { return Product(access, func(Row) bool { return true }) },
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, ok := rule.InputAt(0)
		return ok && CarryFrom(rule, input, carry)
	})
	if !carryRuleOK || carryRule == nil {
		tb.Fatal("carry scale rule")
	}
	var token QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(98_503),
		Project: func(observation Observation) uint64 {
			var value uint64
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, ok := QueryValue(row, token)
				if !ok || cells.Count() != 1 {
					return false
				}
				entry, present, valid := cells.At(0)
				if !valid || !present {
					return false
				}
				value = entry
				return true
			}) {
				return 0
			}
			return value
		},
		Result: frozenColdResult(coldKey(98_504)),
	}, func(query *Query[uint64]) bool {
		var ok bool
		token, ok = QueryReadFrom(query, read)
		return ok
	})
	if !queryOK || query == nil || !composition.Seal() {
		tb.Fatal("carry scale query/composition")
	}
	readRef, readRefOK := factor.Ref(0)
	writeRef, writeRefOK := factor.Ref(0)
	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	if !scope.Available() || !readRefOK || !writeRefOK {
		tb.Fatal("carry scale source vocabulary")
	}
	sites := make([]equation.Site, carries)
	for index := range sites {
		init, disposition := equation.FalseExpr(), equation.InitAbsent
		if index == 0 {
			init, disposition = equation.TrueExpr(), equation.InitPresent
		}
		site, admitted := batch.AdmitSite(coldKey(uint64(98_700+index)).compositionKey(), scope, init, disposition)
		if !admitted {
			tb.Fatal("carry scale site")
		}
		sites[index] = site
	}
	seedOccurrence, seedOccurrenceOK := batch.Relation(sites[0], coldKey(98_800).compositionKey())
	seedInstance, seedInstanceOK := NewRuleInstance(seed, ruleUnitForSemantic(coldKey(98_900)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, seedWrite, writeRef)
	})
	seedOperand, seedOperandOK := admitInstanceOperand(batch, seedOccurrence, seedInstance)
	carryOccurrences := make([]equation.Occurrence, carries)
	carryOperands := make([]equation.Operand, carries)
	carryInstances := make([]*RuleInstance[uint64, ruleUnit], carries)
	for index := range carryOccurrences {
		output := (index + 1) % carries
		occurrence, occurrenceOK := batch.Relation(sites[output], coldKey(uint64(98_810+index)).compositionKey())
		instance, instanceOK := NewRuleInstance(carryRule, ruleUnitForSemantic(coldKey(uint64(98_910+index))), func(*RuleBinding[uint64, ruleUnit]) bool { return true })
		operand, operandOK := admitInstanceOperand(batch, occurrence, instance)
		if !occurrenceOK || !instanceOK || !operandOK {
			tb.Fatal("carry scale occurrence")
		}
		carryOccurrences[index], carryOperands[index], carryInstances[index] = occurrence, operand, instance
	}
	if !seedOccurrenceOK || !seedInstanceOK || !seedOperandOK || !batch.Seal() {
		tb.Fatal("carry scale batch")
	}
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		points := make([]*assemblyPoint, carries)
		for index, site := range sites {
			points[index] = admitPoint(assembly, site)
			if points[index] == nil {
				tb.Fatal("carry scale point")
			}
		}
		seedMember := admitInstance(assembly, points[0], seedOccurrence, seedOperand, seedInstance)
		if seedMember == nil || admitGroup(assembly, points[0], seedMember) == nil {
			return false
		}
		for index := range carryOccurrences {
			output := (index + 1) % carries
			member := admitInstance(assembly, points[output], carryOccurrences[index], carryOperands[index], carryInstances[index])
			group := admitGroup(assembly, points[output], member)
			boundary := equation.BoundaryInput(sites[index], sites[output], coldKey(uint64(98_950+index)).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
			if member == nil || group == nil || !admitBoundary(assembly, group, boundary) {
				return false
			}
		}
		queryInstance, queryInstanceOK := NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, token, readRef)
		})
		return queryInstanceOK && admitQueryAt(assembly, points[0], queryInstance) != nil
	})
	if !compiled {
		return nil
	}
	return solver
}

func sparseCarryFixture(tb testing.TB, keyEnd uint64) (*Query[uint64], QueryReceipt[uint64], *Solver) {
	tb.Helper()
	if keyEnd <= uint64(sparseCarryObservedKey) {
		tb.Fatal("sparse key end")
	}
	composition := NewComposition()
	factor, declared := DeclareFactor(composition, FactorSpec[sparseCarryKey, uint64]{
		Semantic: coldKey(98_000), KeyEnd: keyEnd, Lattice: coldUintLattice(), Default: 0,
		AdmitAt: func(sparseCarryKey, uint64) bool { return true }, Fingerprint: func(value uint64) uint64 { return value },
	}, func(*Factor[sparseCarryKey, uint64]) bool { return true })
	if !declared || factor == nil {
		tb.Fatal("sparse factor")
	}
	read, readOK := ExactReadForm(factor)
	write, writeOK := ExactWriteForm(factor)
	carry, carryOK := Carry(factor)
	if !readOK || !writeOK || !carryOK {
		tb.Fatal("sparse factor forms")
	}
	var ingressWrite Write[uint64]
	ingress, ingressOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(98_001), Output: factor.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](98_101),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(73)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var ok bool
		ingressWrite, ok = WriteTo(rule, write)
		return ok
	})
	if !ingressOK || ingress == nil {
		tb.Fatal("sparse ingress rule")
	}
	carryRule, carryDeclared := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(98_002), Output: factor.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](98_102),
		Transfer: func(access Access[uint64, ruleUnit]) bool { return Product(access, func(Row) bool { return true }) },
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, ok := rule.InputAt(0)
		return ok && CarryFrom(rule, input, carry)
	})
	if !carryDeclared || carryRule == nil {
		tb.Fatal("sparse carry rule")
	}
	var token QueryRead[OrderedCells[uint64]]
	query, queryDeclared := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(98_003),
		Project: func(observation Observation) uint64 {
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
		Result: frozenColdResult(coldKey(98_004)),
	}, func(query *Query[uint64]) bool {
		var ok bool
		token, ok = QueryReadFrom(query, read)
		return ok
	})
	if !queryDeclared || query == nil || !composition.Seal() {
		tb.Fatal("sparse query/composition")
	}
	readRef, readRefOK := factor.Ref(sparseCarryObservedKey)
	writeRef, writeRefOK := factor.Ref(sparseCarryObservedKey)
	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	ingressSite, ingressSiteOK := batch.AdmitSite(coldKey(98_005).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	carrySite, carrySiteOK := batch.AdmitSite(coldKey(98_006).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
	ingressOccurrence, ingressOccurrenceOK := batch.Relation(ingressSite, coldKey(98_007).compositionKey())
	carryOccurrence, carryOccurrenceOK := batch.Relation(carrySite, coldKey(98_008).compositionKey())
	ingressInstance, ingressInstanceOK := NewRuleInstance(ingress, ruleUnitForSemantic(coldKey(98_009)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, ingressWrite, writeRef)
	})
	carryInstance, carryInstanceOK := NewRuleInstance(carryRule, ruleUnitForSemantic(coldKey(98_010)), func(*RuleBinding[uint64, ruleUnit]) bool { return true })
	ingressOperand, ingressOperandOK := admitInstanceOperand(batch, ingressOccurrence, ingressInstance)
	carryOperand, carryOperandOK := admitInstanceOperand(batch, carryOccurrence, carryInstance)
	if !scope.Available() || !ingressSiteOK || !carrySiteOK || !ingressOccurrenceOK || !carryOccurrenceOK ||
		!ingressInstanceOK || !carryInstanceOK || !ingressOperandOK || !carryOperandOK || !readRefOK || !writeRefOK || !batch.Seal() {
		tb.Fatal("sparse source")
	}
	boundary := equation.BoundaryInput(ingressSite, carrySite, coldKey(98_011).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		ingressPoint := admitPoint(assembly, ingressSite)
		carryPoint := admitPoint(assembly, carrySite)
		ingressMember := admitInstance(assembly, ingressPoint, ingressOccurrence, ingressOperand, ingressInstance)
		carryMember := admitInstance(assembly, carryPoint, carryOccurrence, carryOperand, carryInstance)
		if ingressPoint == nil || carryPoint == nil || ingressMember == nil || carryMember == nil || admitGroup(assembly, ingressPoint, ingressMember) == nil {
			return false
		}
		carryGroup := admitGroup(assembly, carryPoint, carryMember)
		if carryGroup == nil || !admitBoundary(assembly, carryGroup, boundary) {
			return false
		}
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, token, readRef)
		})
		return queryInstanceOK && admitQueryAt(assembly, carryPoint, queryInstance) != nil
	})
	if !compiled {
		return query, QueryReceipt[uint64]{}, nil
	}
	receipt, receiptOK := queryInstance.Receipt()
	if !receiptOK {
		tb.Fatal("sparse query receipt")
	}
	return query, receipt, solver
}

func itoaUint(value uint64) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	index := len(digits)
	for value != 0 {
		index--
		digits[index] = byte(value%10) + '0'
		value /= 10
	}
	return string(digits[index:])
}
