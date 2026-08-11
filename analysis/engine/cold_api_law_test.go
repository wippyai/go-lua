package engine

import (
	"context"
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/lattice"
)

func coldKey[N ~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64](n N) SemanticKey {
	var digest [32]byte
	value := uint64(n)
	for index := 0; index < 8; index++ {
		digest[24+index] = byte(value >> uint((7-index)*8))
	}
	key, ok := NewSemanticKey(digest, 1)
	if !ok {
		panic("cold test semantic key")
	}
	return key
}

func coldUintLattice() lattice.Lattice[uint64] {
	return lattice.Lattice[uint64]{
		Bottom: func() uint64 { return 0 }, Top: func() uint64 { return ^uint64(0) },
		Equal: func(left, right uint64) bool { return left == right }, LessOrEq: func(left, right uint64) bool { return left <= right },
		Join: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
		Widen: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
	}
}

func coldFactorSpec(semantic SemanticKey) FactorSpec[uint64, uint64] {
	return FactorSpec[uint64, uint64]{
		Semantic: semantic, KeyEnd: 2, Lattice: coldUintLattice(), Default: 0,
		AdmitAt: func(uint64, uint64) bool { return true }, Fingerprint: func(value uint64) uint64 { return value },
	}
}

func coldFactor(composition *Composition, semantic SemanticKey) *Factor[uint64, uint64] {
	factor, ok := DeclareFactor(composition, coldFactorSpec(semantic), func(*Factor[uint64, uint64]) bool { return true })
	if !ok {
		return nil
	}
	return factor
}

func TestFactorAdmissionDefaultSweepOccursOnlyAtDeclaration(t *testing.T) {
	composition := NewComposition()
	checks := 0
	spec := coldFactorSpec(coldKey(91_001))
	spec.KeyEnd = 257
	spec.AdmitAt = func(uint64, uint64) bool {
		checks++
		return true
	}
	factor, declared := DeclareFactor(composition, spec, func(*Factor[uint64, uint64]) bool { return true })
	if !declared || factor == nil || checks != int(spec.KeyEnd) {
		t.Fatalf("declaration default checks = %d, want %d", checks, spec.KeyEnd)
	}
	read, readOK := ExactReadForm(factor)
	write, writeOK := ExactWriteForm(factor)
	var ruleWrite Write[uint64]
	rule, ruleOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent, Semantic: coldKey(91_002), Output: factor.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](91_102), Transfer: func(Access[uint64, ruleUnit]) bool { return true }}, func(rule *Rule[uint64, ruleUnit]) bool {
		var ok bool
		ruleWrite, ok = WriteTo(rule, write)
		return ok
	})
	query, queryRead, queryOK := declareColdQueryInstance(composition, coldKey(91_003), coldKey(91_004), read)
	if !readOK || !writeOK || !ruleOK || rule == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("declaration")
	}
	if _, compiled := compileColdExactSolver(composition, factor, rule, ruleWrite, coldKey(91_005), query, queryRead); !compiled {
		t.Fatal("compile")
	}
	if checks != int(spec.KeyEnd) {
		t.Fatalf("compile repeated default admission: checks = %d, want %d", checks, spec.KeyEnd)
	}
}

func TestFactorAlgebraRebindsAcrossCompilations(t *testing.T) {
	composition := NewComposition()
	factor := coldFactor(composition, coldKey(91_101))
	if factor == nil {
		t.Fatal("factor")
	}
	read, readOK := ExactReadForm(factor)
	write, writeOK := ExactWriteForm(factor)
	var ruleWrite Write[uint64]
	rule, ruleOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent, Semantic: coldKey(91_102), Output: factor.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](91_202), Transfer: func(Access[uint64, ruleUnit]) bool { return true }}, func(rule *Rule[uint64, ruleUnit]) bool { var ok bool; ruleWrite, ok = WriteTo(rule, write); return ok })
	query, queryRead, queryOK := declareColdQueryInstance(composition, coldKey(91_103), coldKey(91_104), read)
	if !readOK || !writeOK || !ruleOK || rule == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("declaration")
	}
	first, compiled := compileColdExactSolver(composition, factor, rule, ruleWrite, coldKey(91_105), query, queryRead)
	if !compiled || first == nil {
		t.Fatal("initial compile")
	}
	if second, ok := compileColdExactSolver(composition, factor, rule, ruleWrite, coldKey(91_106), query, queryRead); !ok || second == nil {
		t.Fatal("algebra was not reusable for rebind")
	}
}

func BenchmarkFactorAlgebraBindingSparseKeyEnd(b *testing.B) {
	for _, keyEnd := range []uint64{2, 1 << 16} {
		b.Run("key_end="+strconv.FormatUint(keyEnd, 10), func(b *testing.B) {
			b.StopTimer()
			composition := NewComposition()
			spec := coldFactorSpec(coldKey(keyEnd + 92_000))
			spec.KeyEnd = keyEnd
			factor, declared := DeclareFactor(composition, spec, func(*Factor[uint64, uint64]) bool { return true })
			if !declared || factor == nil {
				b.Fatal("factor")
			}
			read, readOK := ExactReadForm(factor)
			write, writeOK := ExactWriteForm(factor)
			var ruleWrite Write[uint64]
			rule, ruleOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent, Semantic: coldKey(keyEnd + 93_000), Output: factor.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](keyEnd + 94_000), Transfer: func(Access[uint64, ruleUnit]) bool { return true }}, func(rule *Rule[uint64, ruleUnit]) bool { var ok bool; ruleWrite, ok = WriteTo(rule, write); return ok })
			query, queryRead, queryOK := declareColdQueryInstance(composition, coldKey(keyEnd+95_000), coldKey(keyEnd+96_000), read)
			if !readOK || !writeOK || !ruleOK || rule == nil || !queryOK || query == nil || !composition.Seal() {
				b.Fatal("declaration")
			}
			b.StartTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				if solver, ok := compileColdExactSolver(composition, factor, rule, ruleWrite, coldKey(keyEnd+97_000), query, queryRead); !ok || solver == nil {
					b.Fatal("compile")
				}
			}
		})
	}
}

func frozenColdResult(semantic SemanticKey) FrozenResult[uint64] {
	return FrozenResult[uint64]{
		Semantic: semantic, Freeze: func(value uint64) uint64 { return value }, Clone: func(value uint64) uint64 { return value },
		Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value },
	}
}

func declareColdQuery(composition *Composition, semantic, freezer SemanticKey, form ReadForm[uint64, OrderedCells[uint64]]) (*Query[uint64], bool) {
	query, _, ok := declareColdQueryInstance(composition, semantic, freezer, form)
	return query, ok
}

func declareColdQueryInstance(composition *Composition, semantic, freezer SemanticKey, form ReadForm[uint64, OrderedCells[uint64]]) (*Query[uint64], QueryRead[OrderedCells[uint64]], bool) {
	var token QueryRead[OrderedCells[uint64]]
	query, ok := DeclareQuery(composition, QuerySpec[uint64]{Semantic: semantic, Project: func(Observation) uint64 { return 0 }, Result: frozenColdResult(freezer)}, func(query *Query[uint64]) bool {
		var declared bool
		token, declared = QueryReadFrom(query, form)
		return declared
	})
	return query, token, ok
}

func TestColdCompositionRejectsUnwrittenQuery(t *testing.T) {
	composition := NewComposition()
	factor := coldFactor(composition, coldKey(1))
	if factor == nil {
		t.Fatal("factor declaration")
	}
	read, ok := ExactReadForm(factor)
	if !ok {
		t.Fatal("exact read form")
	}
	if query, ok := declareColdQuery(composition, coldKey(2), coldKey(3), read); !ok || query == nil {
		t.Fatal("query declaration")
	}
	if composition.Seal() || composition.ID().Available() {
		t.Fatal("unwritten query composition sealed")
	}
}

func TestColdFactorCallbackRejectsNestedDeclarations(t *testing.T) {
	tests := []struct {
		name    string
		attempt func(*Composition, *Factor[uint64, uint64]) bool
	}{
		{"Rule", func(c *Composition, f *Factor[uint64, uint64]) bool {
			rule, ok := DeclareRule(c, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent, Semantic: coldKey(72), Output: f.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](1072), Transfer: func(Access[uint64, ruleUnit]) bool { return true }}, func(*Rule[uint64, ruleUnit]) bool { return true })
			return !ok && rule == nil
		}},
		{"Query", func(c *Composition, f *Factor[uint64, uint64]) bool {
			read, ok := ExactReadForm(f)
			if !ok {
				return false
			}
			query, ok := declareColdQuery(c, coldKey(73), coldKey(74), read)
			return !ok && query == nil
		}},
		{"Factor", func(c *Composition, _ *Factor[uint64, uint64]) bool {
			factor, ok := DeclareFactor(c, coldFactorSpec(coldKey(75)), func(*Factor[uint64, uint64]) bool { return true })
			return !ok && factor == nil
		}},
		{"SupportCompletion", func(c *Composition, _ *Factor[uint64, uint64]) bool {
			completion, ok := DeclareSupportCompletion(c, coldKey(76))
			return !ok && completion == (SupportCompletion{})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			composition := NewComposition()
			factor, ok := DeclareFactor(composition, coldFactorSpec(coldKey(71)), func(factor *Factor[uint64, uint64]) bool { return test.attempt(composition, factor) })
			if ok || factor != nil || composition.Seal() {
				t.Fatal("nested declaration did not fail closed")
			}
		})
	}
}

func TestColdRuleDeclarationsCompileAndSolve(t *testing.T) {
	composition := NewComposition()
	factorKey, ruleKey, queryKey := coldKey(11), coldKey(12), coldKey(13)
	factor := coldFactor(composition, factorKey)
	if factor == nil {
		t.Fatal("factor declaration")
	}
	write, writeOK := ExactWriteForm(factor)
	read, readOK := ExactReadForm(factor)
	if !writeOK || !readOK {
		t.Fatal("factor forms")
	}
	var ruleWrite Write[uint64]
	rule, ruleOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: ruleKey, Output: factor.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](1012),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(42)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool { var ok bool; ruleWrite, ok = WriteTo(rule, write); return ok })
	if !ruleOK || rule == nil {
		t.Fatal("zero-input rule declaration")
	}
	var token QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: queryKey, Result: frozenColdResult(coldKey(14)),
		Project: func(observation Observation) uint64 {
			var value uint64
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, ok := QueryValue(row, token)
				if !ok {
					return false
				}
				result, present, ok := cells.At(0)
				if !ok || !present {
					return false
				}
				value = result
				return true
			}) {
				return 0
			}
			return value
		},
	}, func(query *Query[uint64]) bool { var ok bool; token, ok = QueryReadFrom(query, read); return ok })
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("ordinary composition did not seal")
	}
	solver, receipt, compiled := compileColdExactSolverWithReceipt(composition, factor, rule, ruleWrite, coldKey(1_015), query, token)
	if !compiled || solver == nil {
		t.Fatal("ordinary composition did not compile")
	}
	state, status := solver.Solve(context.Background())
	if status != SolveComplete || state == nil {
		t.Fatalf("solve = state:%v status:%v", state, status)
	}
	if value, ok := QueryResult(receipt, state); !ok || value != 42 {
		t.Fatalf("query result = %d, readable=%v", value, ok)
	}
}

func TestColdRuleAcceptsArbitraryOrderedInputPorts(t *testing.T) {
	composition := NewComposition()
	factor := coldFactor(composition, coldKey(15))
	if factor == nil {
		t.Fatal("factor declaration")
	}
	read, readOK := ExactReadForm(factor)
	write, writeOK := ExactWriteForm(factor)
	if !readOK || !writeOK {
		t.Fatal("factor forms")
	}
	rule, ok := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent, Semantic: coldKey(16), Output: factor.Output(), Inputs: 3, Admission: testTrustedTheorem[uint64](1016), Transfer: func(Access[uint64, ruleUnit]) bool { return true }}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(2)
		if !inputOK {
			return false
		}
		if _, ok := ReadFrom(rule, input, read); !ok {
			return false
		}
		_, ok := WriteTo(rule, write)
		return ok
	})
	if !ok || rule == nil {
		t.Fatal("third input port rejected")
	}
	if query, ok := declareColdQuery(composition, coldKey(17), coldKey(18), read); !ok || query == nil || !composition.Seal() {
		t.Fatal("three-input declaration did not seal")
	}
}

func TestColdCompositionRejectsForeignOutput(t *testing.T) {
	left, right := NewComposition(), NewComposition()
	leftFactor, rightFactor := coldFactor(left, coldKey(21)), coldFactor(right, coldKey(22))
	if leftFactor == nil || rightFactor == nil {
		t.Fatal("factor declarations")
	}
	rule, ok := DeclareRule(left, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent, Semantic: coldKey(23), Output: rightFactor.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](1023), Transfer: func(Access[uint64, ruleUnit]) bool { return true }}, func(*Rule[uint64, ruleUnit]) bool { return true })
	if ok || rule != nil || left.Seal() {
		t.Fatal("foreign output was accepted")
	}
}

func TestColdWriteSelectorRejectsEmptyAndUnorderedCandidates(t *testing.T) {
	for _, unordered := range []bool{false, true} {
		composition := NewComposition()
		var selector WriteForm[uint64]
		selectorOK := false
		factor, ok := DeclareFactor(composition, coldFactorSpec(coldKey(31)), func(f *Factor[uint64, uint64]) bool {
			selector, selectorOK = DeclareWriteSelector(f, coldKey(32))
			return selectorOK
		})
		if !ok || factor == nil {
			t.Fatal("write selector factor")
		}
		read, _ := ExactReadForm(factor)
		rule, ok := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent, Semantic: coldKey(33), Output: factor.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](1033), Transfer: func(Access[uint64, ruleUnit]) bool { return true }}, func(rule *Rule[uint64, ruleUnit]) bool {
			input, ok := rule.InputAt(0)
			if !ok {
				return false
			}
			left, ok := ReadFrom(rule, input, read)
			if !ok {
				return false
			}
			right, ok := ReadFrom(rule, input, read)
			if !ok {
				return false
			}
			candidates := []Read[OrderedCells[uint64]]{}
			if unordered {
				candidates = []Read[OrderedCells[uint64]]{right, left}
			}
			_, ok = SelectWrite(rule, selector, candidates, []Dependency{ReadDependency(left)}, func(SelectorContext) bool { return true })
			return ok
		})
		if ok || rule != nil || composition.Seal() {
			t.Fatal("invalid write selector sequence was accepted")
		}
	}
}

func TestColdFactorRejectsUnrankedNarrowing(t *testing.T) {
	composition := NewComposition()
	_, ok := DeclareFactor(composition, FactorSpec[uint64, uint64]{Semantic: coldKey(41), KeyEnd: 1, Lattice: lattice.Lattice[uint64]{Bottom: func() uint64 { return 0 }, Top: func() uint64 { return ^uint64(0) }, Equal: func(left, right uint64) bool { return left == right }, LessOrEq: func(left, right uint64) bool { return left <= right }, Join: func(_, right uint64) uint64 { return right }, Widen: func(_, right uint64) uint64 { return right }, Narrow: func(_, right uint64) uint64 { return right }}, Default: 0, AdmitAt: func(uint64, uint64) bool { return true }, Fingerprint: func(uint64) uint64 { return 0 }}, func(*Factor[uint64, uint64]) bool { return true })
	if ok || composition.Seal() {
		t.Fatal("unranked narrowing was admitted")
	}
}

func TestColdDeclaredNormalizerSurvivesSeal(t *testing.T) {
	composition := NewComposition()
	var normalizer Normalizer[uint64, uint64]
	normalizerOK := false
	factor, ok := DeclareFactor(composition, coldFactorSpec(coldKey(51)), func(f *Factor[uint64, uint64]) bool {
		normalizer, normalizerOK = DeclareNormalizer(f, coldKey(52), func(OrderedCells[uint64]) uint64 { return 0 }, func(left, right uint64) bool { return left == right }, func(value uint64) uint64 { return value })
		return normalizerOK
	})
	if !ok || factor == nil {
		t.Fatal("normalizer declaration")
	}
	read, _ := ExactReadForm(factor)
	write, _ := ExactWriteForm(factor)
	rule, ok := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent, Semantic: coldKey(53), Output: factor.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](1053), Transfer: func(Access[uint64, ruleUnit]) bool { return true }}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, ok := rule.InputAt(0)
		if !ok {
			return false
		}
		if _, ok = ReadFrom(rule, input, read); !ok {
			return false
		}
		_, ok = WriteTo(rule, write)
		return ok
	})
	if !ok || rule == nil {
		t.Fatal("rule declaration")
	}
	if query, ok := declareColdQuery(composition, coldKey(54), coldKey(55), read); !ok || query == nil || !composition.Seal() {
		t.Fatal("normalizer composition did not seal")
	}
	if summary, ok := SummaryReadForm(normalizer); !ok || !summary.valid() {
		t.Fatal("sealed normalizer capability is unavailable")
	}
}

func TestColdSupportQueryDeclarationsSeal(t *testing.T) {
	composition := NewComposition()
	factor := coldFactor(composition, coldKey(61))
	if factor == nil {
		t.Fatal("factor declaration")
	}
	read, readOK := ExactReadForm(factor)
	if !readOK {
		t.Fatal("support read form")
	}
	completion, completionOK := DeclareSupportCompletion(composition, coldKey(62))
	prune, pruneOK := DeclarePrune(completion, coldKey(63))
	if !completionOK || !pruneOK {
		t.Fatal("support lifecycle")
	}
	rule, ruleOK := DeclareSupportRule(composition, SupportRuleSpec{
		Semantic: coldKey(64), Completion: completion, Prune: prune, Inputs: 3,
		Admission: testTrustedTheorem[Support](1064),
		Declare: func(rule *SupportRule) bool {
			input, ok := rule.InputAt(2)
			if !ok {
				return false
			}
			_, ok = ReadFrom(rule, input, read)
			return ok
		},
		Run: func(value Support) (Support, bool) { return value, true },
	})
	query, queryOK := DeclareSupportQuery(composition, coldKey(65), func(SupportObservation) uint64 { return 0 }, frozenColdResult(coldKey(66)))
	if !ruleOK || rule == nil || !queryOK || query == nil || !composition.Seal() || !composition.ID().Available() {
		t.Fatal("support query composition did not seal")
	}
	inventory, ok := composition.RuleAdmissionInventory()
	if !ok || len(inventory.Rules) != 1 || inventory.Rules[0].Rule != coldKey(64) {
		t.Fatal("support rule missing from public inventory")
	}
}

func TestColdQueryReadsMustBeDeclaredDuringCallback(t *testing.T) {
	composition := NewComposition()
	factor := coldFactor(composition, coldKey(81))
	if factor == nil {
		t.Fatal("factor declaration")
	}
	read, _ := ExactReadForm(factor)
	write, _ := ExactWriteForm(factor)
	rule, ok := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent, Semantic: coldKey(82), Output: factor.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](1082), Transfer: func(Access[uint64, ruleUnit]) bool { return true }}, func(rule *Rule[uint64, ruleUnit]) bool { _, ok := WriteTo(rule, write); return ok })
	if !ok || rule == nil {
		t.Fatal("rule")
	}
	query, ok := declareColdQuery(composition, coldKey(83), coldKey(84), read)
	if !ok || query == nil {
		t.Fatal("query")
	}
	if _, ok := QueryReadFrom(query, read); ok || composition.Seal() {
		t.Fatal("late query read was accepted")
	}
}

func compileColdExactSolver(composition *Composition, factor *Factor[uint64, uint64], rule *Rule[uint64, ruleUnit], ruleWrite Write[uint64], sourceID SemanticKey, query *Query[uint64], queryRead QueryRead[OrderedCells[uint64]]) (*Solver, bool) {
	solver, _, ok := compileColdExactSolverWithReceipt(composition, factor, rule, ruleWrite, sourceID, query, queryRead)
	return solver, ok
}

func compileColdExactSolverWithReceipt(composition *Composition, factor *Factor[uint64, uint64], rule *Rule[uint64, ruleUnit], ruleWrite Write[uint64], sourceID SemanticKey, query *Query[uint64], queryRead QueryRead[OrderedCells[uint64]]) (*Solver, QueryReceipt[uint64], bool) {
	if composition == nil || factor == nil || rule == nil || query == nil {
		return nil, QueryReceipt[uint64]{}, false
	}
	read, readOK := factor.Ref(0)
	write, writeOK := factor.Ref(0)
	if !readOK || !writeOK {
		return nil, QueryReceipt[uint64]{}, false
	}
	instance, instanceOK := NewRuleInstance(rule, ruleUnitForSemantic(sourceID), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, ruleWrite, write)
	})
	batch, site, occurrence, operand, admitted := coldSourceBatch(sourceID, coldKey(90_002), instance)
	if !instanceOK || !admitted {
		return nil, QueryReceipt[uint64]{}, false
	}
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		point := admitPoint(assembly, site)
		member := admitInstance(assembly, point, occurrence, operand, instance)
		if !instanceOK || member == nil || admitGroup(assembly, point, member) == nil {
			return false
		}
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, queryRead, read)
		})
		return queryInstanceOK && admitQueryAt(assembly, point, queryInstance) != nil
	})
	if !compiled || solver == nil {
		return nil, QueryReceipt[uint64]{}, false
	}
	receipt, receiptOK := queryInstance.Receipt()
	return solver, receipt, receiptOK
}

// coldSourceBatch is the test-only Program/Link boundary spelling used by the
// cold and WTO laws.  The source Batch owns every source Site, occurrence,
// and operand before assembly begins; tests never manufacture engine anchors.
func coldSourceBatch(siteID, occurrenceID SemanticKey, instance *RuleInstance[uint64, ruleUnit]) (*equation.Batch, equation.Site, equation.Occurrence, equation.Operand, bool) {
	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	if batch == nil || !scope.Available() {
		return nil, equation.Site{}, equation.Occurrence{}, equation.Operand{}, false
	}
	site, siteOK := batch.AdmitSite(siteID.compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	occurrence, occurrenceOK := batch.Relation(site, occurrenceID.compositionKey())
	operand, operandOK := admitInstanceOperand(batch, occurrence, instance)
	if !siteOK || !occurrenceOK || !operandOK || !batch.Seal() {
		return nil, equation.Site{}, equation.Occurrence{}, equation.Operand{}, false
	}
	return batch, site, occurrence, operand, true
}
