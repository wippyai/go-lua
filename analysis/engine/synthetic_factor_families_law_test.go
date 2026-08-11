package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/lattice"
)

// syntheticPackRelation is a deliberately tiny Pack-like relation.  The tail
// identifier is meaningful only to this test: the boundary Rule must retain
// it while changing the head coordinate.
type syntheticPackRelation struct {
	head uint8
	tail uint8
}

// syntheticHeapCell is one field in a partitioned heap root.  Exact Factor
// keys name the fields; a write to one key must preserve the other key's cell.
type syntheticHeapCell struct {
	field uint8
	value uint8
}

func syntheticFlatLattice[V comparable](bottom, top V) lattice.Lattice[V] {
	equal := func(left, right V) bool { return left == right }
	lessOrEq := func(left, right V) bool {
		return left == bottom || right == top || left == right
	}
	join := func(left, right V) V {
		switch {
		case left == bottom:
			return right
		case right == bottom || left == right:
			return left
		default:
			return top
		}
	}
	return lattice.Lattice[V]{
		Bottom: func() V { return bottom }, Top: func() V { return top }, Equal: equal, LessOrEq: lessOrEq,
		Join: join, Widen: join,
	}
}

func syntheticPackFactorSpec(semantic SemanticKey) FactorSpec[uint32, syntheticPackRelation] {
	return FactorSpec[uint32, syntheticPackRelation]{
		Semantic: semantic, KeyEnd: 2,
		Lattice: syntheticFlatLattice(syntheticPackRelation{}, syntheticPackRelation{head: ^uint8(0), tail: ^uint8(0)}),
		Default: syntheticPackRelation{}, AdmitAt: func(uint32, syntheticPackRelation) bool { return true },
		Fingerprint: func(value syntheticPackRelation) uint64 { return uint64(value.head)<<8 | uint64(value.tail) },
	}
}

func syntheticHeapFactorSpec(semantic SemanticKey) FactorSpec[uint64, syntheticHeapCell] {
	return FactorSpec[uint64, syntheticHeapCell]{
		Semantic: semantic, KeyEnd: 2,
		Lattice: syntheticFlatLattice(syntheticHeapCell{}, syntheticHeapCell{field: ^uint8(0), value: ^uint8(0)}),
		Default: syntheticHeapCell{}, AdmitAt: func(uint64, syntheticHeapCell) bool { return true },
		Fingerprint: func(value syntheticHeapCell) uint64 { return uint64(value.field)<<8 | uint64(value.value) },
	}
}

type syntheticFactorFamilyStats struct {
	packSeed, heapFromTail, heapIndependent, packBoundary int
	packSeedRows, heapFromTailRows                        int
	heapIndependentRows, packBoundaryRows                 int
	projects, freezes                                     int
}

type syntheticFactorFamilyFixture struct {
	solver       *Solver
	pack0        *Query[syntheticPackRelation]
	pack1        *Query[syntheticPackRelation]
	heap0        *Query[syntheticHeapCell]
	heap1        *Query[syntheticHeapCell]
	pack0Receipt QueryReceipt[syntheticPackRelation]
	pack1Receipt QueryReceipt[syntheticPackRelation]
	heap0Receipt QueryReceipt[syntheticHeapCell]
	heap1Receipt QueryReceipt[syntheticHeapCell]
	stats        *syntheticFactorFamilyStats
}

func syntheticFactorFamilyKey(offset uint64) SemanticKey { return testSemanticKey(99_000 + offset) }

// TestSyntheticFactorFamiliesPreserveSharedTailAndExactHeapPartitions covers
// the remaining Wave-C Factor-family semantics through the sole public engine
// route.  The pre-existing parametric-boundary fixture owns body transport and
// its two-input resume relation; the activation-SCC fixture owns the
// history-sensitive widening/narrowing episode.  This law supplies only the
// missing shared-tail Pack and exact heap-field independence correspondence.
func TestSyntheticFactorFamiliesPreserveSharedTailAndExactHeapPartitions(t *testing.T) {
	fixture := newSyntheticFactorFamilyFixture(t)
	state, status := fixture.solver.Solve(context.Background())
	if status != SolveComplete || state == nil {
		t.Fatalf("cold Solve = state:%v status:%v callbacks:%+v", state, status, *fixture.stats)
	}
	assertSyntheticFactorFamilyResults(t, fixture, state)
	if got, want := *fixture.stats, (syntheticFactorFamilyStats{packSeed: 1, heapFromTail: 1, heapIndependent: 1, packBoundary: 1, packSeedRows: 1, heapFromTailRows: 1, heapIndependentRows: 1, packBoundaryRows: 1, projects: 4, freezes: 4}); got != want {
		t.Fatalf("cold callbacks = %+v, want %+v", got, want)
	}

	before := *fixture.stats
	state, status = fixture.solver.Solve(context.Background())
	if status != SolveComplete || state == nil {
		t.Fatalf("warm Solve = state:%v status:%v", state, status)
	}
	assertSyntheticFactorFamilyResults(t, fixture, state)
	if got := *fixture.stats; got != before {
		t.Fatalf("warm Solve repeated callbacks: before=%+v after=%+v", before, got)
	}
}

func newSyntheticFactorFamilyFixture(t *testing.T) syntheticFactorFamilyFixture {
	t.Helper()
	stats := &syntheticFactorFamilyStats{}
	cold := NewComposition()
	pack, packOK := DeclareFactor(cold, syntheticPackFactorSpec(syntheticFactorFamilyKey(1)), func(*Factor[uint32, syntheticPackRelation]) bool { return true })
	heap, heapOK := DeclareFactor(cold, syntheticHeapFactorSpec(syntheticFactorFamilyKey(2)), func(*Factor[uint64, syntheticHeapCell]) bool { return true })
	packRead, packReadOK := ExactReadForm(pack)
	packWrite, packWriteOK := ExactWriteForm(pack)
	heapRead, heapReadOK := ExactReadForm(heap)
	heapWrite, heapWriteOK := ExactWriteForm(heap)
	heapCarry, heapCarryOK := Carry(heap)
	if !packOK || !heapOK || pack == nil || heap == nil || !packReadOK || !packWriteOK || !heapReadOK || !heapWriteOK || !heapCarryOK {
		t.Fatal("Factor declarations/forms")
	}

	var packSeedWrite Write[syntheticPackRelation]
	packSeed, packSeedOK := DeclareRule(cold, RuleSpec[syntheticPackRelation, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: syntheticFactorFamilyKey(10), Output: pack.Output(), Inputs: 0, Admission: testTrustedTheorem[syntheticPackRelation](99_110),
		Transfer: func(access Access[syntheticPackRelation, ruleUnit]) bool {
			stats.packSeed++
			return Product(access, func(row Row) bool {
				stats.packSeedRows++
				return StageValue(access, row, syntheticPackRelation{head: 1, tail: 7})
			})
		},
	}, func(rule *Rule[syntheticPackRelation, ruleUnit]) bool {
		var ok bool
		packSeedWrite, ok = WriteTo(rule, packWrite)
		return ok
	})

	var packTailRead Read[OrderedCells[syntheticPackRelation]]
	var heapFromTailWrite Write[syntheticHeapCell]
	heapFromTail, heapFromTailOK := DeclareRule(cold, RuleSpec[syntheticHeapCell, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: syntheticFactorFamilyKey(11), Output: heap.Output(), Inputs: 1, Admission: testTrustedTheorem[syntheticHeapCell](99_111),
		Transfer: func(access Access[syntheticHeapCell, ruleUnit]) bool {
			stats.heapFromTail++
			return Product(access, func(row Row) bool {
				stats.heapFromTailRows++
				cells, ok := ReadValue(access, row, packTailRead)
				value, present, cellOK := cells.At(0)
				return ok && cellOK && present && StageValue(access, row, syntheticHeapCell{field: 1, value: value.tail})
			})
		},
	}, func(rule *Rule[syntheticHeapCell, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var readOK, writeOK bool
		packTailRead, readOK = ReadFrom(rule, input, packRead)
		heapFromTailWrite, writeOK = WriteTo(rule, heapWrite)
		return inputOK && readOK && writeOK
	})

	var heapFirstRead Read[OrderedCells[syntheticHeapCell]]
	var heapIndependentWrite Write[syntheticHeapCell]
	heapIndependent, heapIndependentOK := DeclareRule(cold, RuleSpec[syntheticHeapCell, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: syntheticFactorFamilyKey(12), Output: heap.Output(), Inputs: 1, Admission: testTrustedTheorem[syntheticHeapCell](99_112),
		Transfer: func(access Access[syntheticHeapCell, ruleUnit]) bool {
			stats.heapIndependent++
			return Product(access, func(row Row) bool {
				stats.heapIndependentRows++
				cells, ok := ReadValue(access, row, heapFirstRead)
				first, present, cellOK := cells.At(0)
				return ok && cellOK && present && first == (syntheticHeapCell{field: 1, value: 7}) && StageValue(access, row, syntheticHeapCell{field: 2, value: 9})
			})
		},
	}, func(rule *Rule[syntheticHeapCell, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var readOK, writeOK bool
		heapFirstRead, readOK = ReadFrom(rule, input, heapRead)
		heapIndependentWrite, writeOK = WriteTo(rule, heapWrite)
		return inputOK && readOK && writeOK && CarryFrom(rule, input, heapCarry)
	})

	var packSourceRead Read[OrderedCells[syntheticPackRelation]]
	var packBoundaryWrite Write[syntheticPackRelation]
	packBoundary, packBoundaryOK := DeclareRule(cold, RuleSpec[syntheticPackRelation, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: syntheticFactorFamilyKey(13), Output: pack.Output(), Inputs: 1, Admission: testTrustedTheorem[syntheticPackRelation](99_113),
		Transfer: func(access Access[syntheticPackRelation, ruleUnit]) bool {
			stats.packBoundary++
			return Product(access, func(row Row) bool {
				stats.packBoundaryRows++
				cells, ok := ReadValue(access, row, packSourceRead)
				value, present, cellOK := cells.At(0)
				return ok && cellOK && present && StageValue(access, row, syntheticPackRelation{head: 2, tail: value.tail})
			})
		},
	}, func(rule *Rule[syntheticPackRelation, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var readOK, writeOK bool
		packSourceRead, readOK = ReadFrom(rule, input, packRead)
		packBoundaryWrite, writeOK = WriteTo(rule, packWrite)
		return inputOK && readOK && writeOK
	})
	if !packSeedOK || !heapFromTailOK || !heapIndependentOK || !packBoundaryOK || packSeed == nil || heapFromTail == nil || heapIndependent == nil || packBoundary == nil {
		t.Fatal("Rule declarations")
	}

	pack0, pack0Read := declareSyntheticPackQuery(t, cold, syntheticFactorFamilyKey(20), syntheticFactorFamilyKey(30), packRead, stats)
	pack1, pack1Read := declareSyntheticPackQuery(t, cold, syntheticFactorFamilyKey(21), syntheticFactorFamilyKey(31), packRead, stats)
	heap0, heap0Read := declareSyntheticHeapQuery(t, cold, syntheticFactorFamilyKey(22), syntheticFactorFamilyKey(32), heapRead, stats)
	heap1, heap1Read := declareSyntheticHeapQuery(t, cold, syntheticFactorFamilyKey(23), syntheticFactorFamilyKey(33), heapRead, stats)
	if !cold.Seal() {
		t.Fatal("Composition seal")
	}

	solver, pack0Receipt, pack1Receipt, heap0Receipt, heap1Receipt := syntheticFactorFamilyAssembly(t, cold, pack, heap, packSeed, heapFromTail, heapIndependent, packBoundary, packSeedWrite, packTailRead, heapFromTailWrite, heapFirstRead, heapIndependentWrite, packSourceRead, packBoundaryWrite, pack0, pack1, heap0, heap1, pack0Read, pack1Read, heap0Read, heap1Read)
	return syntheticFactorFamilyFixture{solver: solver, pack0: pack0, pack1: pack1, heap0: heap0, heap1: heap1, pack0Receipt: pack0Receipt, pack1Receipt: pack1Receipt, heap0Receipt: heap0Receipt, heap1Receipt: heap1Receipt, stats: stats}
}

func declareSyntheticPackQuery(t *testing.T, cold *Composition, semantic, frozen SemanticKey, form ReadForm[syntheticPackRelation, OrderedCells[syntheticPackRelation]], stats *syntheticFactorFamilyStats) (*Query[syntheticPackRelation], QueryRead[OrderedCells[syntheticPackRelation]]) {
	t.Helper()
	var token QueryRead[OrderedCells[syntheticPackRelation]]
	query, ok := DeclareQuery(cold, QuerySpec[syntheticPackRelation]{
		Semantic: semantic,
		Project: func(observation Observation) syntheticPackRelation {
			stats.projects++
			var result syntheticPackRelation
			rows := 0
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, readable := QueryValue(row, token)
				value, present, cellOK := cells.At(0)
				if !readable || !cellOK || !present {
					return false
				}
				result, rows = value, rows+1
				return true
			}) || rows != 1 {
				return syntheticPackRelation{}
			}
			return result
		},
		Result: FrozenResult[syntheticPackRelation]{Semantic: frozen, Freeze: func(value syntheticPackRelation) syntheticPackRelation { stats.freezes++; return value }, Clone: func(value syntheticPackRelation) syntheticPackRelation { return value }, Equal: func(left, right syntheticPackRelation) bool { return left == right }, Fingerprint: func(value syntheticPackRelation) uint64 { return uint64(value.head)<<8 | uint64(value.tail) }},
	}, func(query *Query[syntheticPackRelation]) bool {
		var declared bool
		token, declared = QueryReadFrom(query, form)
		return declared
	})
	if !ok || query == nil {
		t.Fatal("Pack Query declaration")
	}
	return query, token
}

func declareSyntheticHeapQuery(t *testing.T, cold *Composition, semantic, frozen SemanticKey, form ReadForm[syntheticHeapCell, OrderedCells[syntheticHeapCell]], stats *syntheticFactorFamilyStats) (*Query[syntheticHeapCell], QueryRead[OrderedCells[syntheticHeapCell]]) {
	t.Helper()
	var token QueryRead[OrderedCells[syntheticHeapCell]]
	query, ok := DeclareQuery(cold, QuerySpec[syntheticHeapCell]{
		Semantic: semantic,
		Project: func(observation Observation) syntheticHeapCell {
			stats.projects++
			var result syntheticHeapCell
			rows := 0
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, readable := QueryValue(row, token)
				value, present, cellOK := cells.At(0)
				if !readable || !cellOK || !present {
					return false
				}
				result, rows = value, rows+1
				return true
			}) || rows != 1 {
				return syntheticHeapCell{}
			}
			return result
		},
		Result: FrozenResult[syntheticHeapCell]{Semantic: frozen, Freeze: func(value syntheticHeapCell) syntheticHeapCell { stats.freezes++; return value }, Clone: func(value syntheticHeapCell) syntheticHeapCell { return value }, Equal: func(left, right syntheticHeapCell) bool { return left == right }, Fingerprint: func(value syntheticHeapCell) uint64 { return uint64(value.field)<<8 | uint64(value.value) }},
	}, func(query *Query[syntheticHeapCell]) bool {
		var declared bool
		token, declared = QueryReadFrom(query, form)
		return declared
	})
	if !ok || query == nil {
		t.Fatal("Heap Query declaration")
	}
	return query, token
}

func assertSyntheticFactorFamilyResults(t *testing.T, fixture syntheticFactorFamilyFixture, state *State) {
	t.Helper()
	pack0, pack0OK := QueryResult(fixture.pack0Receipt, state)
	pack1, pack1OK := QueryResult(fixture.pack1Receipt, state)
	heap0, heap0OK := QueryResult(fixture.heap0Receipt, state)
	heap1, heap1OK := QueryResult(fixture.heap1Receipt, state)
	if !pack0OK || !pack1OK || !heap0OK || !heap1OK || pack0 != (syntheticPackRelation{head: 1, tail: 7}) || pack1 != (syntheticPackRelation{head: 2, tail: 7}) || heap0 != (syntheticHeapCell{field: 1, value: 7}) || heap1 != (syntheticHeapCell{field: 2, value: 9}) {
		t.Fatalf("Factor-family results = pack0:%+v/%t pack1:%+v/%t heap0:%+v/%t heap1:%+v/%t", pack0, pack0OK, pack1, pack1OK, heap0, heap0OK, heap1, heap1OK)
	}
}

func syntheticFactorFamilyAssembly(t *testing.T, cold *Composition, pack *Factor[uint32, syntheticPackRelation], heap *Factor[uint64, syntheticHeapCell], packSeed *Rule[syntheticPackRelation, ruleUnit], heapFromTail, heapIndependent *Rule[syntheticHeapCell, ruleUnit], packBoundary *Rule[syntheticPackRelation, ruleUnit], packSeedWrite Write[syntheticPackRelation], packTailRead Read[OrderedCells[syntheticPackRelation]], heapFromTailWrite Write[syntheticHeapCell], heapFirstRead Read[OrderedCells[syntheticHeapCell]], heapIndependentWrite Write[syntheticHeapCell], packSourceRead Read[OrderedCells[syntheticPackRelation]], packBoundaryWrite Write[syntheticPackRelation], pack0, pack1 *Query[syntheticPackRelation], heap0, heap1 *Query[syntheticHeapCell], pack0Read, pack1Read QueryRead[OrderedCells[syntheticPackRelation]], heap0Read, heap1Read QueryRead[OrderedCells[syntheticHeapCell]]) (*Solver, QueryReceipt[syntheticPackRelation], QueryReceipt[syntheticPackRelation], QueryReceipt[syntheticHeapCell], QueryReceipt[syntheticHeapCell]) {
	t.Helper()
	packRead0, packRead0OK := pack.Ref(0)
	packRead1, packRead1OK := pack.Ref(1)
	packWrite0, packWrite0OK := pack.Ref(0)
	packWrite1, packWrite1OK := pack.Ref(1)
	heapRead0, heapRead0OK := heap.Ref(0)
	heapRead1, heapRead1OK := heap.Ref(1)
	heapWrite0, heapWrite0OK := heap.Ref(0)
	heapWrite1, heapWrite1OK := heap.Ref(1)
	if !packRead0OK || !packRead1OK || !packWrite0OK || !packWrite1OK || !heapRead0OK || !heapRead1OK || !heapWrite0OK || !heapWrite1OK {
		t.Fatal("surface references")
	}
	packSeedInstance, packSeedOK := NewRuleInstance(packSeed, ruleUnitForSemantic(syntheticFactorFamilyKey(60)), func(binding *RuleBinding[syntheticPackRelation, ruleUnit]) bool {
		return InstanceWrite(binding, packSeedWrite, packWrite0)
	})
	heapFromTailInstance, heapFromTailOK := NewRuleInstance(heapFromTail, ruleUnitForSemantic(syntheticFactorFamilyKey(61)), func(binding *RuleBinding[syntheticHeapCell, ruleUnit]) bool {
		return InstanceRead(binding, packTailRead, packRead0) && InstanceWrite(binding, heapFromTailWrite, heapWrite0)
	})
	heapIndependentInstance, heapIndependentOK := NewRuleInstance(heapIndependent, ruleUnitForSemantic(syntheticFactorFamilyKey(62)), func(binding *RuleBinding[syntheticHeapCell, ruleUnit]) bool {
		return InstanceRead(binding, heapFirstRead, heapRead0) && InstanceWrite(binding, heapIndependentWrite, heapWrite1)
	})
	packBoundaryInstance, packBoundaryOK := NewRuleInstance(packBoundary, ruleUnitForSemantic(syntheticFactorFamilyKey(63)), func(binding *RuleBinding[syntheticPackRelation, ruleUnit]) bool {
		return InstanceRead(binding, packSourceRead, packRead0) && InstanceWrite(binding, packBoundaryWrite, packWrite1)
	})
	if !packSeedOK || !heapFromTailOK || !heapIndependentOK || !packBoundaryOK {
		t.Fatal("rule instances")
	}
	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	identity := equation.IdentityReindex(scope)
	if batch == nil || !scope.Available() || !identity.Available() {
		t.Fatal("source scope")
	}
	sites := [4]equation.Site{}
	for index := range sites {
		init, disposition := equation.FalseExpr(), equation.InitAbsent
		if index == 0 {
			init, disposition = equation.TrueExpr(), equation.InitPresent
		}
		site, admitted := batch.AdmitSite(syntheticFactorFamilyKey(uint64(40+index)).compositionKey(), scope, init, disposition)
		if !admitted {
			t.Fatal("source site")
		}
		sites[index] = site
	}
	occurrences := [4]equation.Occurrence{}
	operands := [4]equation.Operand{}
	for index := range sites {
		occurrence, occurred := batch.Relation(sites[index], syntheticFactorFamilyKey(uint64(50+index)).compositionKey())
		var operand equation.Operand
		var admitted bool
		switch index {
		case 0:
			operand, admitted = admitInstanceOperand(batch, occurrence, packSeedInstance)
		case 1:
			operand, admitted = admitInstanceOperand(batch, occurrence, heapFromTailInstance)
		case 2:
			operand, admitted = admitInstanceOperand(batch, occurrence, heapIndependentInstance)
		case 3:
			operand, admitted = admitInstanceOperand(batch, occurrence, packBoundaryInstance)
		}
		if !occurred || !admitted {
			t.Fatal("source occurrence")
		}
		occurrences[index], operands[index] = occurrence, operand
	}
	if !batch.Seal() {
		t.Fatal("source seal")
	}
	var pack0Instance *QueryInstance[syntheticPackRelation]
	var pack1Instance *QueryInstance[syntheticPackRelation]
	var heap0Instance *QueryInstance[syntheticHeapCell]
	var heap1Instance *QueryInstance[syntheticHeapCell]
	solver, compiled := assemble(cold, batch, func(assembly *Assembly) bool {
		points := [4]*assemblyPoint{}
		for index := range sites {
			points[index] = admitPoint(assembly, sites[index])
			if points[index] == nil {
				t.Fatal("source point")
			}
		}
		packSeedMember := admitInstance(assembly, points[0], occurrences[0], operands[0], packSeedInstance)
		heapFromTailMember := admitInstance(assembly, points[1], occurrences[1], operands[1], heapFromTailInstance)
		heapIndependentMember := admitInstance(assembly, points[2], occurrences[2], operands[2], heapIndependentInstance)
		packBoundaryMember := admitInstance(assembly, points[3], occurrences[3], operands[3], packBoundaryInstance)
		if !packSeedOK || !heapFromTailOK || !heapIndependentOK || !packBoundaryOK || packSeedMember == nil || heapFromTailMember == nil || heapIndependentMember == nil || packBoundaryMember == nil ||
			admitGroup(assembly, points[0], packSeedMember) == nil {
			t.Fatal("rule assembly")
		}
		heapFromTailGroup := admitGroup(assembly, points[1], heapFromTailMember)
		heapIndependentGroup := admitGroup(assembly, points[2], heapIndependentMember)
		packBoundaryGroup := admitGroup(assembly, points[3], packBoundaryMember)
		seedToHeap := equation.BoundaryInput(sites[0], sites[1], syntheticFactorFamilyKey(70).compositionKey(), equation.TrueExpr(), identity, equation.TrueExpr())
		heapToHeap := equation.BoundaryInput(sites[1], sites[2], syntheticFactorFamilyKey(71).compositionKey(), equation.TrueExpr(), identity, equation.TrueExpr())
		seedToPack := equation.BoundaryInput(sites[0], sites[3], syntheticFactorFamilyKey(72).compositionKey(), equation.TrueExpr(), identity, equation.TrueExpr())
		if heapFromTailGroup == nil || heapIndependentGroup == nil || packBoundaryGroup == nil ||
			!admitBoundary(assembly, heapFromTailGroup, seedToHeap) || !admitBoundary(assembly, heapIndependentGroup, heapToHeap) || !admitBoundary(assembly, packBoundaryGroup, seedToPack) {
			t.Fatal("group assembly")
		}
		var pack0InstanceOK bool
		pack0Instance, pack0InstanceOK = NewQueryInstance(pack0, func(binding *QueryBinding[syntheticPackRelation]) bool {
			return InstanceQueryRead(binding, pack0Read, packRead0)
		})
		var pack1InstanceOK bool
		pack1Instance, pack1InstanceOK = NewQueryInstance(pack1, func(binding *QueryBinding[syntheticPackRelation]) bool {
			return InstanceQueryRead(binding, pack1Read, packRead1)
		})
		var heap0InstanceOK bool
		heap0Instance, heap0InstanceOK = NewQueryInstance(heap0, func(binding *QueryBinding[syntheticHeapCell]) bool {
			return InstanceQueryRead(binding, heap0Read, heapRead0)
		})
		var heap1InstanceOK bool
		heap1Instance, heap1InstanceOK = NewQueryInstance(heap1, func(binding *QueryBinding[syntheticHeapCell]) bool {
			return InstanceQueryRead(binding, heap1Read, heapRead1)
		})
		pack0Observation := admitQueryAt(assembly, points[0], pack0Instance)
		pack1Observation := admitQueryAt(assembly, points[3], pack1Instance)
		heap0Observation := admitQueryAt(assembly, points[2], heap0Instance)
		heap1Observation := admitQueryAt(assembly, points[2], heap1Instance)
		if pack0Observation == nil || pack1Observation == nil || heap0Observation == nil || heap1Observation == nil ||
			!pack0InstanceOK || !pack1InstanceOK || !heap0InstanceOK || !heap1InstanceOK {
			t.Fatal("query assembly")
		}
		return true
	})
	if !compiled || solver == nil {
		t.Fatal("Solver compilation")
	}
	pack0Receipt, pack0OK := pack0Instance.Receipt()
	pack1Receipt, pack1OK := pack1Instance.Receipt()
	heap0Receipt, heap0OK := heap0Instance.Receipt()
	heap1Receipt, heap1OK := heap1Instance.Receipt()
	if !pack0OK || !pack1OK || !heap0OK || !heap1OK {
		t.Fatal("query receipts")
	}
	return solver, pack0Receipt, pack1Receipt, heap0Receipt, heap1Receipt
}
