package engine

import (
	"context"
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/lattice"
)

// These laws deliberately exercise the public declaration, compilation, solve,
// and query path.  Their callbacks are ordinary lawful lattice and rule
// callbacks; runtime state is not manufactured by the tests.
type wtoFactorLawSpec struct {
	ranked       bool
	narrow       bool
	inflateWiden bool
}

type wtoRuleLawSpec struct {
	factor    int
	output    int
	input     int
	value     uint64
	transfers *int
}

type wtoLawFixture struct {
	solver   *Solver
	queries  []*Query[uint64]
	receipts []QueryReceipt[uint64]
}

type wtoTestHelper interface {
	Helper()
	Fatal(...any)
	Fatalf(string, ...any)
}

// wtoSourceBatch is the test-only source side of the public assembly cut.
// It is deliberately complete before seal: each rule occurrence and operand
// belongs to its output Site, and no test reconstructs a source capability
// from a topology coordinate after the batch closes.
func wtoSourceBatch(siteBase, occurrenceBase int, pointCount int, rules []wtoRuleLawSpec, instances []*RuleInstance[uint64, ruleUnit]) (*equation.Batch, []equation.Site, []equation.Occurrence, []equation.Operand, bool) {
	outputs := make([]int, len(rules))
	for index, rule := range rules {
		outputs[index] = rule.output
	}
	return wtoSourceRows(siteBase, occurrenceBase, pointCount, outputs, instances)
}

func wtoSourceRows(siteBase, occurrenceBase, pointCount int, outputs []int, instances []*RuleInstance[uint64, ruleUnit]) (*equation.Batch, []equation.Site, []equation.Occurrence, []equation.Operand, bool) {
	if pointCount <= 0 || len(outputs) != len(instances) {
		return nil, nil, nil, nil, false
	}
	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	if batch == nil || !scope.Available() {
		return nil, nil, nil, nil, false
	}
	sites := make([]equation.Site, pointCount)
	for index := range sites {
		site, admitted := batch.AdmitSite(coldKey(siteBase+index).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
		if !admitted {
			return nil, nil, nil, nil, false
		}
		sites[index] = site
	}
	occurrences := make([]equation.Occurrence, len(outputs))
	operands := make([]equation.Operand, len(outputs))
	for index, output := range outputs {
		if output < 0 || output >= len(sites) {
			return nil, nil, nil, nil, false
		}
		occurrence, occurred := batch.Relation(sites[output], coldKey(occurrenceBase+index).compositionKey())
		operand, admitted := admitInstanceOperand(batch, occurrence, instances[index])
		if !occurred || !admitted {
			return nil, nil, nil, nil, false
		}
		occurrences[index], operands[index] = occurrence, operand
	}
	if !batch.Seal() {
		return nil, nil, nil, nil, false
	}
	return batch, sites, occurrences, operands, true
}

func wtoIdentityBoundary(source, target equation.Site, provenance SemanticKey) (equation.Input, bool) {
	if !source.Available() || !target.Available() || source.Scope().Key() != target.Scope().Key() {
		return equation.Input{}, false
	}
	input := equation.BoundaryInput(source, target, provenance.compositionKey(), equation.TrueExpr(), equation.IdentityReindex(source.Scope()), equation.TrueExpr())
	return input, input.Available()
}

func newWTOLawFixture(t wtoTestHelper, factors []wtoFactorLawSpec, rules []wtoRuleLawSpec, pointCount int, queryFactors, queryPoints []int) *wtoLawFixture {
	t.Helper()
	if len(factors) == 0 || len(rules) == 0 || pointCount <= 0 || len(queryFactors) == 0 || len(queryFactors) != len(queryPoints) {
		t.Fatal("invalid WTO law fixture shape")
	}
	composition := NewComposition()
	coldFactors := make([]*Factor[uint64, uint64], len(factors))
	factorKeys := make([]SemanticKey, len(factors))
	reads := make([]ReadForm[uint64, OrderedCells[uint64]], len(factors))
	writes := make([]WriteForm[uint64], len(factors))
	for index, law := range factors {
		factorKeys[index] = coldKey(70_000 + index)
		spec := coldFactorSpec(factorKeys[index])
		spec.KeyEnd = 1
		if law.ranked {
			spec.WidenRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }}
		}
		if law.inflateWiden {
			spec.Lattice.Widen = func(left, right uint64) uint64 {
				if right <= left {
					return left
				}
				return 9
			}
		}
		if law.narrow {
			spec.Lattice.Narrow = func(_ uint64, desired uint64) uint64 { return desired }
			spec.NarrowRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return value }}
		}
		factor, declared := DeclareFactor(composition, spec, func(*Factor[uint64, uint64]) bool { return true })
		if !declared || factor == nil {
			t.Fatalf("factor %d declaration", index)
		}
		read, readOK := ExactReadForm(factor)
		write, writeOK := ExactWriteForm(factor)
		if !readOK || !writeOK {
			t.Fatalf("factor %d forms", index)
		}
		coldFactors[index], reads[index], writes[index] = factor, read, write
	}
	coldRules := make([]*Rule[uint64, ruleUnit], len(rules))
	coldRuleReads := make([]Read[OrderedCells[uint64]], len(rules))
	coldRuleWrites := make([]Write[uint64], len(rules))
	ruleKeys := make([]SemanticKey, len(rules))
	for index, law := range rules {
		if law.factor < 0 || law.factor >= len(coldFactors) || law.output < 0 || law.output >= pointCount || law.input < 0 || law.input >= pointCount {
			t.Fatalf("rule %d shape", index)
		}
		ruleKeys[index] = coldKey(71_000 + index)
		value := law.value
		transfers := law.transfers
		rule, declared := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
			Semantic:  ruleKeys[index],
			Output:    coldFactors[law.factor].Output(),
			Inputs:    1,
			Admission: testTrustedTheorem[uint64](uint64(72_000 + index)),
			Transfer: func(access Access[uint64, ruleUnit]) bool {
				if transfers != nil {
					(*transfers)++
				}
				return Product(access, func(row Row) bool { return StageValue(access, row, value) })
			},
		}, func(rule *Rule[uint64, ruleUnit]) bool {
			input, inputOK := rule.InputAt(0)
			declaredRead, readOK := ReadFrom(rule, input, reads[law.factor])
			declaredWrite, writeOK := WriteTo(rule, writes[law.factor])
			coldRuleReads[index], coldRuleWrites[index] = declaredRead, declaredWrite
			return inputOK && readOK && writeOK
		})
		if !declared || rule == nil {
			t.Fatalf("rule %d declaration", index)
		}
		coldRules[index] = rule
	}
	coldQueries := make([]*Query[uint64], len(queryFactors))
	coldQueryReads := make([]QueryRead[OrderedCells[uint64]], len(queryFactors))
	queryKeys := make([]SemanticKey, len(queryFactors))
	for index, factorIndex := range queryFactors {
		if factorIndex < 0 || factorIndex >= len(coldFactors) || queryPoints[index] < 0 || queryPoints[index] >= pointCount {
			t.Fatalf("query %d shape", index)
		}
		queryKeys[index] = coldKey(73_000 + index)
		var token QueryRead[OrderedCells[uint64]]
		query, declared := DeclareQuery(composition, QuerySpec[uint64]{
			Semantic: queryKeys[index],
			Project: func(observation Observation) uint64 {
				var result uint64
				if !ProjectRows(observation, func(row QueryRow) bool {
					cells, readOK := QueryValue(row, token)
					if !readOK || cells.Count() != 1 {
						return false
					}
					value, present, cellOK := cells.At(0)
					if !cellOK {
						return false
					}
					if present && value > result {
						result = value
					}
					return true
				}) {
					return 0
				}
				return result
			},
			Result: FrozenResult[uint64]{
				Semantic: coldKey(74_000 + index), Freeze: func(value uint64) uint64 { return value }, Clone: func(value uint64) uint64 { return value },
				Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value },
			},
		}, func(query *Query[uint64]) bool {
			declared, okay := QueryReadFrom(query, reads[factorIndex])
			token = declared
			return okay
		})
		if !declared || query == nil {
			t.Fatalf("query %d declaration", index)
		}
		coldQueries[index], coldQueryReads[index] = query, token
	}
	if !composition.Seal() {
		t.Fatal("law composition seal")
	}
	readRefs := make([]Ref[uint64], len(coldFactors))
	writeRefs := make([]Ref[uint64], len(coldFactors))
	for index, factor := range coldFactors {
		var readOK, writeOK bool
		readRefs[index], readOK = factor.Ref(0)
		writeRefs[index], writeOK = factor.Ref(0)
		if !readOK || !writeOK {
			t.Fatalf("factor %d refs", index)
		}
	}
	instances := make([]*RuleInstance[uint64, ruleUnit], len(rules))
	for index, law := range rules {
		instance, instanceOK := NewRuleInstance(coldRules[index], ruleUnitForSemantic(coldKey(86_000+index)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
			return InstanceRead(binding, coldRuleReads[index], readRefs[law.factor]) && InstanceWrite(binding, coldRuleWrites[index], writeRefs[law.factor])
		})
		if !instanceOK {
			t.Fatalf("rule %d instance", index)
		}
		instances[index] = instance
	}
	batch, sites, occurrences, operands, admitted := wtoSourceBatch(75_000, 76_000, pointCount, rules, instances)
	if !admitted {
		t.Fatal("law source batch")
	}
	queryInstances := make([]*QueryInstance[uint64], len(coldQueries))
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		points := make([]*assemblyPoint, pointCount)
		for index := range points {
			points[index] = admitPoint(assembly, sites[index])
			if points[index] == nil {
				t.Fatalf("point %d", index)
			}
		}
		groups := make([][]*assemblyMember, pointCount)
		inputs := make([]int, pointCount)
		for index := range inputs {
			inputs[index] = -1
		}
		for index, law := range rules {
			member := admitInstance(assembly, points[law.output], occurrences[index], operands[index], instances[index])
			if member == nil {
				t.Fatalf("rule %d assembly", index)
			}
			if inputs[law.output] == -1 {
				inputs[law.output] = law.input
			} else if inputs[law.output] != law.input {
				t.Fatalf("rule %d requires a second semantic group at one Point", index)
			}
			groups[law.output] = append(groups[law.output], member)
		}
		for output, members := range groups {
			if len(members) == 0 || inputs[output] < 0 {
				t.Fatalf("point %d group assembly", output)
			}
			boundary, boundaryOK := wtoIdentityBoundary(sites[inputs[output]], sites[output], coldKey(77_000+output))
			group := admitGroup(assembly, points[output], members...)
			if group == nil || !boundaryOK || !admitBoundary(assembly, group, boundary) {
				t.Fatalf("point %d group assembly", output)
			}
		}
		for index, factorIndex := range queryFactors {
			var queryInstanceOK bool
			queryInstance, queryInstanceOK = NewQueryInstance(coldQueries[index], func(binding *QueryBinding[uint64]) bool {
				return InstanceQueryRead(binding, coldQueryReads[index], readRefs[factorIndex])
			})
			queryInstances[index] = queryInstance
			if !queryInstanceOK || admitQueryAt(assembly, points[queryPoints[index]], queryInstance) == nil {
				t.Fatalf("query %d assembly", index)
			}
		}
		return true
	})
	if !compiled || solver == nil {
		t.Fatal("law solver assembly")
	}
	receipts := make([]QueryReceipt[uint64], len(queryInstances))
	for index, instance := range queryInstances {
		var receiptOK bool
		receipts[index], receiptOK = instance.Receipt()
		if !receiptOK {
			t.Fatal("wto query receipt")
		}
	}
	return &wtoLawFixture{solver: solver, queries: coldQueries, receipts: receipts}
}

func (fixture *wtoLawFixture) solve(t wtoTestHelper) []uint64 {
	t.Helper()
	state, status := fixture.solver.Solve(context.Background())
	if state == nil || status != SolveComplete {
		t.Fatalf("Solve = state:%v status:%v", state, status)
	}
	values := make([]uint64, len(fixture.queries))
	for index := range fixture.queries {
		value, readable := QueryResult(fixture.receipts[index], state)
		if !readable {
			t.Fatalf("query %d unreadable", index)
		}
		values[index] = value
	}
	return values
}

func TestDemandedCycleIgnoresDisconnectedCycle(t *testing.T) {
	factors := []wtoFactorLawSpec{{ranked: true, narrow: true}, {ranked: false, narrow: false}}
	rules := []wtoRuleLawSpec{{factor: 0, output: 0, input: 0, value: 1}, {factor: 1, output: 1, input: 1, value: 1}}
	fixture := newWTOLawFixture(t, factors, rules, 2, []int{0}, []int{0})
	got := fixture.solve(t)
	fresh := newWTOLawFixture(t, factors, rules, 2, []int{0}, []int{0}).solve(t)
	if len(got) != 1 || len(fresh) != 1 || got[0] != 1 || got[0] != fresh[0] {
		t.Fatalf("demanded cycle result = %v, fresh = %v, want [1]", got, fresh)
	}
}

// This measures only public solve behavior. The growing siblings are cold
// declarations that are structurally present but outside the queried cycle;
// no private runtime cache or layout is inspected. A public restart trigger
// is not available through this fixture, so this benchmark honestly covers
// assembly plus first-solve locality for the demanded recurrence slice.
func BenchmarkDemandedCycleSolveLocality(b *testing.B) {
	for _, siblings := range []int{0, 8, 64} {
		siblings := siblings
		b.Run("siblings_"+strconv.Itoa(siblings), func(b *testing.B) {
			var demandedTransfers, siblingTransfers int
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				transfers := make([]int, siblings+1)
				factors := make([]wtoFactorLawSpec, siblings+1)
				rules := make([]wtoRuleLawSpec, siblings+1)
				factors[0] = wtoFactorLawSpec{ranked: true, narrow: true}
				for index := range rules {
					if index != 0 {
						factors[index] = wtoFactorLawSpec{ranked: false, narrow: false}
					}
					rules[index] = wtoRuleLawSpec{factor: index, output: index, input: index, value: uint64(index + 1), transfers: &transfers[index]}
				}
				fixture := newWTOLawFixture(b, factors, rules, siblings+1, []int{0}, []int{0})
				values := fixture.solve(b)
				if len(values) != 1 || values[0] != 1 {
					b.Fatalf("queried recurrence result = %v, want [1]", values)
				}
				demandedTransfers += transfers[0]
				for _, count := range transfers[1:] {
					siblingTransfers += count
				}
			}
			if siblingTransfers != 0 {
				b.Fatalf("disconnected recurrence callbacks = %d, want 0", siblingTransfers)
			}
			b.ReportMetric(float64(demandedTransfers)/float64(b.N), "demanded-transfers/op")
			b.ReportMetric(float64(siblingTransfers)/float64(b.N), "sibling-transfers/op")
		})
	}
}

// Rule callbacks are the exact demanded-work counter.  Factor Join is only
// observable for authored overlap (plus Join-stability admission); this
// one-writer chain folds an empty-authorship base with one authored candidate,
// so right-only folds may carry their terminal without invoking Join.  Keep a
// nonnegative upper bound rather than asserting an implementation-specific
// exact Join count.
func TestDemandedPostfixAvoidsUnconditionalAcyclicCompletionRefold(t *testing.T) {
	for _, points := range []int{2, 9, 33} {
		points := points
		t.Run("points", func(t *testing.T) {
			value, transfers, joins := solveSparsePostfixChain(t, points)
			fresh, _, _ := solveSparsePostfixChain(t, points)
			if value != fresh || transfers != points-1 || joins < 0 || joins > points-1 {
				t.Fatalf("points=%d: value=%d fresh=%d transfers=%d joins=%d, want transfers=%d and 0<=joins<=%d", points, value, fresh, transfers, joins, points-1, points-1)
			}
		})
	}
}

func solveSparsePostfixChain(t *testing.T, points int) (uint64, int, int) {
	t.Helper()
	if points < 2 {
		t.Fatal("sparse postfix point count")
	}
	composition := NewComposition()
	joins := 0
	factorKey := coldKey(77_000 + points)
	spec := coldFactorSpec(factorKey)
	spec.KeyEnd = 1
	join := spec.Lattice.Join
	spec.Lattice.Join = func(left, right uint64) uint64 { joins++; return join(left, right) }
	factor, declared := DeclareFactor(composition, spec, func(*Factor[uint64, uint64]) bool { return true })
	if !declared || factor == nil {
		t.Fatal("sparse postfix factor")
	}
	read, readOK := ExactReadForm(factor)
	write, writeOK := ExactWriteForm(factor)
	if !readOK || !writeOK {
		t.Fatal("sparse postfix forms")
	}
	// Assembly requires every source point to carry one completed group. The
	// entry group deliberately contributes no factor value; the chain rules
	// remain the only measured transfer work in this law.
	var seedWrite Write[uint64]
	seed, seedDeclared := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent, Semantic: coldKey(77_050 + points), Output: factor.Output(), Inputs: 0,
		Admission: testTrustedTheorem[uint64](uint64(77_060 + points)),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return NoCandidate(access, row) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var bound bool
		seedWrite, bound = WriteTo(rule, write)
		return bound
	})
	if !seedDeclared || seed == nil {
		t.Fatal("sparse postfix seed")
	}
	transfers := 0
	rules := make([]*Rule[uint64, ruleUnit], points-1)
	ruleReads := make([]Read[OrderedCells[uint64]], points-1)
	ruleWrites := make([]Write[uint64], points-1)
	ruleKeys := make([]SemanticKey, points-1)
	for index := range rules {
		index := index
		ruleKeys[index] = coldKey(77_100 + points*100 + index)
		rule, declared := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
			Semantic: ruleKeys[index], Output: factor.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](uint64(77_200 + points*100 + index)),
			Transfer: func(access Access[uint64, ruleUnit]) bool {
				transfers++
				return Product(access, func(row Row) bool { return StageValue(access, row, 1) })
			},
		}, func(rule *Rule[uint64, ruleUnit]) bool {
			input, inputOK := rule.InputAt(0)
			declaredRead, readOK := ReadFrom(rule, input, read)
			declaredWrite, writeOK := WriteTo(rule, write)
			ruleReads[index], ruleWrites[index] = declaredRead, declaredWrite
			return inputOK && readOK && writeOK
		})
		if !declared || rule == nil {
			t.Fatalf("sparse postfix rule %d", index)
		}
		rules[index] = rule
	}
	queryKey := coldKey(77_300 + points)
	var token QueryRead[OrderedCells[uint64]]
	query, declared := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: queryKey,
		Project: func(observation Observation) uint64 {
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
		Result: FrozenResult[uint64]{Semantic: coldKey(77_400 + points), Freeze: func(value uint64) uint64 { return value }, Clone: func(value uint64) uint64 { return value }, Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value }},
	}, func(query *Query[uint64]) bool { var ok bool; token, ok = QueryReadFrom(query, read); return ok })
	if !declared || query == nil || !composition.Seal() {
		t.Fatal("sparse postfix query/composition")
	}
	readRef, readIssued := factor.Ref(0)
	writeRef, writeIssued := factor.Ref(0)
	if !readIssued || !writeIssued {
		t.Fatal("sparse postfix refs")
	}
	outputs := make([]int, points)
	for index := 1; index < len(outputs); index++ {
		outputs[index] = index
	}
	instances := make([]*RuleInstance[uint64, ruleUnit], points)
	operandBase := 77_700 + points*100
	seedInstance, seedInstanceOK := NewRuleInstance(seed, ruleUnitForSemantic(coldKey(operandBase)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, seedWrite, writeRef)
	})
	if !seedInstanceOK {
		t.Fatal("sparse postfix seed instance")
	}
	instances[0] = seedInstance
	for index, rule := range rules {
		instance, instanceOK := NewRuleInstance(rule, ruleUnitForSemantic(coldKey(operandBase+index+1)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
			return InstanceRead(binding, ruleReads[index], readRef) && InstanceWrite(binding, ruleWrites[index], writeRef)
		})
		if !instanceOK {
			t.Fatalf("sparse postfix rule %d instance", index)
		}
		instances[index+1] = instance
	}
	batch, sites, occurrences, operands, admitted := wtoSourceRows(77_500+points*100, 77_600+points*100, points, outputs, instances)
	if !admitted {
		t.Fatal("sparse postfix source batch")
	}
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		assemblyPoints := make([]*assemblyPoint, points)
		for index := range assemblyPoints {
			assemblyPoints[index] = admitPoint(assembly, sites[index])
			if assemblyPoints[index] == nil {
				t.Fatalf("sparse postfix point %d", index)
			}
			if index == 0 {
				member := admitInstance(assembly, assemblyPoints[index], occurrences[0], operands[0], instances[0])
				if member == nil || admitGroup(assembly, assemblyPoints[index], member) == nil {
					t.Fatal("sparse postfix seed assembly")
				}
				continue
			}
			ruleIndex := index - 1
			member := admitInstance(assembly, assemblyPoints[index], occurrences[index], operands[index], instances[index])
			boundary, boundaryOK := wtoIdentityBoundary(sites[index-1], sites[index], coldKey(77_800+points*100+ruleIndex))
			group := admitGroup(assembly, assemblyPoints[index], member)
			if member == nil || group == nil || !boundaryOK || !admitBoundary(assembly, group, boundary) {
				t.Fatalf("sparse postfix rule %d assembly", ruleIndex)
			}
		}
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, token, readRef)
		})
		if !queryInstanceOK || admitQueryAt(assembly, assemblyPoints[len(assemblyPoints)-1], queryInstance) == nil {
			t.Fatal("sparse postfix query assembly")
		}
		return true
	})
	if !compiled || solver == nil {
		t.Fatal("sparse postfix solver")
	}
	joins, transfers = 0, 0
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	value, readable := QueryResult(receipt, state)
	if status != SolveComplete || !receiptOK || !readable {
		t.Fatalf("sparse postfix Solve = state:%v status:%v readable:%v", state, status, readable)
	}
	return value, transfers, joins
}

func TestMixedOptionalNarrowKeepsBothCoordinatesQueryable(t *testing.T) {
	fixture := newWTOLawFixture(t,
		[]wtoFactorLawSpec{{ranked: true, narrow: true, inflateWiden: true}, {ranked: true, narrow: false, inflateWiden: true}},
		[]wtoRuleLawSpec{{factor: 0, output: 0, input: 0, value: 2}, {factor: 1, output: 0, input: 0, value: 2}},
		1, []int{0, 1}, []int{0, 0})
	values := fixture.solve(t)
	if values[0] != 2 || values[1] != 2 {
		t.Fatalf("mixed optional narrow results = %v, want [2 2]", values)
	}
}

// TestNarrowPropagationRejectsIncomparableCyclicCandidate proves that the
// narrow-phase exception for a region-local wake admits only a real descent.
// One self-loop ascends 1 -> 3 and widens to 7 while retaining exact 3. The
// exact narrow publication then descends 7 -> 3, whose self-wake deliberately
// produces 4. Since 3 and 4 are incomparable in the subset lattice, the public
// Solve must fail closed rather than complete with that candidate.
func TestNarrowPropagationRejectsIncomparableCyclicCandidate(t *testing.T) {
	composition := NewComposition()
	factor, factorOK := DeclareFactor(composition, FactorSpec[uint64, uint64]{
		Semantic: coldKey(77_900), KeyEnd: 1,
		Lattice: lattice.Lattice[uint64]{
			Bottom: func() uint64 { return 0 }, Top: func() uint64 { return ^uint64(0) },
			Equal:    func(left, right uint64) bool { return left == right },
			LessOrEq: func(left, right uint64) bool { return left&right == left },
			Join:     func(left, right uint64) uint64 { return left | right },
			Widen: func(left, right uint64) uint64 {
				if left == 1 && right == 3 {
					return 7
				}
				return left | right
			},
		},
		Default: 0, AdmitAt: func(uint64, uint64) bool { return true }, Fingerprint: func(value uint64) uint64 { return value },
		WidenRank: Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }},
	}, func(*Factor[uint64, uint64]) bool { return true })
	if !factorOK || factor == nil {
		t.Fatal("incomparable factor")
	}
	read, readOK := ExactReadForm(factor)
	write, writeOK := ExactWriteForm(factor)
	if !readOK || !writeOK {
		t.Fatal("incomparable forms")
	}

	var selfRead Read[OrderedCells[uint64]]
	var selfWrite Write[uint64]
	incomparableProduced := false
	transfers := 0
	selfRule, ruleOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(77_901), Output: factor.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](77_911),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			transfers++
			return Product(access, func(row Row) bool {
				cells, ok := ReadValue(access, row, selfRead)
				if !ok || cells.Count() != 1 {
					return false
				}
				value, present, valid := cells.At(0)
				if !valid {
					return false
				}
				output := uint64(1)
				if present {
					switch value {
					case 1, 7:
						output = 3
					case 3:
						output = 4
						incomparableProduced = true
					}
				}
				return StageValue(access, row, output)
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var bound, targetOK bool
		selfRead, bound = ReadFrom(rule, input, read)
		selfWrite, targetOK = WriteTo(rule, write)
		return inputOK && bound && targetOK
	})
	if !ruleOK || selfRule == nil {
		t.Fatal("incomparable self Rule")
	}

	var queryRead QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(77_903), Project: func(observation Observation) uint64 {
			var result uint64
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, ok := QueryValue(row, queryRead)
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
		}, Result: frozenColdResult(coldKey(77_904)),
	}, func(query *Query[uint64]) bool {
		var declared bool
		queryRead, declared = QueryReadFrom(query, read)
		return declared
	})
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("incomparable query/composition")
	}
	readRef, readIssued := factor.Ref(0)
	writeRef, writeIssued := factor.Ref(0)
	instance, instanceOK := NewRuleInstance(selfRule, ruleUnitForSemantic(coldKey(77_907)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceRead(binding, selfRead, readRef) && InstanceWrite(binding, selfWrite, writeRef)
	})
	batch, sites, occurrences, operands, admitted := wtoSourceRows(77_905, 77_906, 1, []int{0}, []*RuleInstance[uint64, ruleUnit]{instance})
	if !readIssued || !writeIssued || !instanceOK || !admitted {
		t.Fatal("incomparable exact operands")
	}
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		point := admitPoint(assembly, sites[0])
		member := admitInstance(assembly, point, occurrences[0], operands[0], instance)
		boundary, boundaryOK := wtoIdentityBoundary(sites[0], sites[0], coldKey(77_908))
		group := admitGroup(assembly, point, member)
		if !instanceOK || member == nil || group == nil || !boundaryOK || !admitBoundary(assembly, group, boundary) {
			t.Fatal("incomparable assembly")
		}
		queryInstance, queryInstanceOK := NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, queryRead, readRef)
		})
		if !queryInstanceOK || admitQueryAt(assembly, point, queryInstance) == nil {
			t.Fatal("incomparable query assembly")
		}
		return true
	})
	if !compiled || solver == nil {
		t.Fatal("incomparable solver")
	}
	state, status := solver.Solve(context.Background())
	if state != nil || status != SolveIncomplete || !incomparableProduced || transfers != 4 {
		t.Fatalf("incomparable narrow propagation = state:%v status:%v produced:%t transfers:%d, want nil/incomplete/true/4", state, status, incomparableProduced, transfers)
	}
}

// TestNestedWTORegionEpisodesPreserveResidualQueries is a public semantic
// regression law for a nested recurrence.  Its residual relation is:
//
//	seed = 1
//	outer = max(seed, min(inner, 3))
//	inner = max(outer, min(inner+1, 3))
//
// The chain is 0 <= 1 <= 2 <= 3 <= 8.  Widening a strict ascent yields the
// sentinel 8, while narrowing returns the exact RHS.  Consequently the outer
// head first widens to 8 and later narrows to 3.  That descent changes the
// inner region's external face, so the inner recurrence has to begin a fresh
// ascent from its base while the enclosing region is still narrowing.  The
// law observes none of that runtime bookkeeping: it independently solves the
// residual relation and compares only completed public query results.
func TestNestedWTORegionEpisodesPreserveResidualQueries(t *testing.T) {
	want := nestedWTOResidual(t)
	var first []uint64
	for _, spelling := range []struct {
		name          string
		semanticOrder []int
		topologyOrder []int
	}{
		{name: "canonical", semanticOrder: []int{0, 1, 2, 3, 4}, topologyOrder: []int{0, 1, 2, 3, 4}},
		// Semantic identities and disposable topology rows are deliberately
		// permuted independently.  The relation, its residual solution, and
		// the public observations remain the same.
		{name: "permuted", semanticOrder: []int{3, 0, 4, 1, 2}, topologyOrder: []int{4, 2, 0, 3, 1}},
	} {
		spelling := spelling
		t.Run(spelling.name, func(t *testing.T) {
			got := solveNestedWTORegionLaw(t, spelling.semanticOrder, spelling.topologyOrder)
			if len(got) != len(want) {
				t.Fatalf("query result count = %d, want %d", len(got), len(want))
			}
			for index := range want {
				if got[index] != want[index] {
					t.Fatalf("query %d = %d, want independently solved residual %d", index, got[index], want[index])
				}
				if got[index] == 8 {
					t.Fatalf("query %d retained widened sentinel 8", index)
				}
			}
			if first == nil {
				first = append([]uint64(nil), got...)
				return
			}
			for index := range first {
				if got[index] != first[index] {
					t.Fatalf("permuted public query %d = %d, canonical = %d", index, got[index], first[index])
				}
			}
		})
	}
}

// nestedWTOResidual is deliberately not an execution-model simulation: it
// performs ordinary exact least-fixed-point iteration over the stated finite
// relation, with no scheduling, widening, narrowing, or runtime state.
func nestedWTOResidual(t wtoTestHelper) []uint64 {
	t.Helper()
	outer, inner := uint64(0), uint64(0)
	for iteration := 0; iteration != 8; iteration++ {
		nextOuter := uint64(1) // seed
		if capped := minNestedWTOValue(inner, 3); capped > nextOuter {
			nextOuter = capped
		}
		nextInner := outer
		if stepped := minNestedWTOValue(inner+1, 3); stepped > nextInner {
			nextInner = stepped
		}
		if nextOuter == outer && nextInner == inner {
			return []uint64{outer, inner}
		}
		outer, inner = nextOuter, nextInner
	}
	t.Fatal("nested WTO residual did not converge on its finite chain")
	return nil
}

func minNestedWTOValue(left, right uint64) uint64 {
	if left < right {
		return left
	}
	return right
}

type nestedWTORuleLawSpec struct {
	output int
	inputs []int
	apply  func([]uint64) uint64
}

// solveNestedWTORegionLaw uses only cold declarations and public solver/query
// APIs.  semanticOrder controls semantic keys assigned to the five relation
// rules; topologyOrder controls the otherwise-disposable Rule and Group row
// spelling.  Neither is consulted after compilation.
func solveNestedWTORegionLaw(t wtoTestHelper, semanticOrder, topologyOrder []int) []uint64 {
	t.Helper()
	if len(semanticOrder) != 5 || len(topologyOrder) != 5 {
		t.Fatal("nested WTO permutation shape")
	}
	seenSemantic, seenTopology := make([]bool, 5), make([]bool, 5)
	for index := range semanticOrder {
		if semanticOrder[index] < 0 || semanticOrder[index] >= 5 || topologyOrder[index] < 0 || topologyOrder[index] >= 5 || seenSemantic[semanticOrder[index]] || seenTopology[topologyOrder[index]] {
			t.Fatal("nested WTO permutation")
		}
		seenSemantic[semanticOrder[index]], seenTopology[topologyOrder[index]] = true, true
	}

	composition := NewComposition()
	factorKey := coldKey(78_000)
	factorSpec := coldFactorSpec(factorKey)
	factorSpec.KeyEnd = 1
	factorSpec.Lattice.Widen = func(left, right uint64) uint64 {
		if right <= left {
			return left
		}
		return 8
	}
	factorSpec.Lattice.Narrow = func(_ uint64, exact uint64) uint64 { return exact }
	factorSpec.WidenRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }}
	factorSpec.NarrowRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return value }}
	factor, factorOK := DeclareFactor(composition, factorSpec, func(*Factor[uint64, uint64]) bool { return true })
	if !factorOK || factor == nil {
		t.Fatal("nested WTO factor")
	}
	read, readOK := ExactReadForm(factor)
	write, writeOK := ExactWriteForm(factor)
	if !readOK || !writeOK {
		t.Fatal("nested WTO forms")
	}

	// The five points spell the residual relation without a duplicate writer
	// at one point: seed, capped-inner, outer merge, outer projection, and
	// stepped-inner. The two-port merge is the explicit join of seed and the
	// inner contribution; the source Batch retains those two boundaries.
	rules := []nestedWTORuleLawSpec{
		{output: 0, apply: func([]uint64) uint64 { return 1 }}, // seed
		{output: 1, inputs: []int{4}, apply: func(values []uint64) uint64 { return minNestedWTOValue(values[0], 3) }},
		{output: 2, inputs: []int{0, 1}, apply: func(values []uint64) uint64 {
			if values[0] > values[1] {
				return values[0]
			}
			return values[1]
		}},
		{output: 3, inputs: []int{2}, apply: func(values []uint64) uint64 { return values[0] }},
		{output: 4, inputs: []int{3}, apply: func(values []uint64) uint64 { return minNestedWTOValue(values[0]+1, 3) }},
	}
	ruleKeys := make([]SemanticKey, len(rules))
	declaredRules := make([]*Rule[uint64, ruleUnit], len(rules))
	declaredReads := make([][]Read[OrderedCells[uint64]], len(rules))
	declaredWrites := make([]Write[uint64], len(rules))
	for role, law := range rules {
		role, law := role, law
		ruleKeys[role] = coldKey(78_100 + semanticOrder[role])
		incoming := make([]Read[OrderedCells[uint64]], len(law.inputs))
		rule, declared := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
			Semantic: ruleKeys[role], Output: factor.Output(), Inputs: len(law.inputs), Admission: testTrustedTheorem[uint64](uint64(78_200 + role)),
			Transfer: func(access Access[uint64, ruleUnit]) bool {
				return Product(access, func(row Row) bool {
					values := make([]uint64, len(incoming))
					for index, token := range incoming {
						cells, readOK := ReadValue(access, row, token)
						if !readOK || cells.Count() != 1 {
							return false
						}
						observed, present, cellOK := cells.At(0)
						if !cellOK {
							return false
						}
						if present {
							values[index] = observed
						}
					}
					return StageValue(access, row, law.apply(values))
				})
			},
		}, func(rule *Rule[uint64, ruleUnit]) bool {
			for index := range law.inputs {
				input, inputOK := rule.InputAt(index)
				if !inputOK {
					return false
				}
				var readBound bool
				incoming[index], readBound = ReadFrom(rule, input, read)
				if !readBound {
					return false
				}
			}
			declaredWrite, writeOK := WriteTo(rule, write)
			declaredWrites[role] = declaredWrite
			return writeOK
		})
		if !declared || rule == nil {
			t.Fatalf("nested WTO rule %d", role)
		}
		declaredRules[role], declaredReads[role] = rule, incoming
	}

	queries := make([]*Query[uint64], 2)
	queryReads := make([]QueryRead[OrderedCells[uint64]], 2)
	queryPoints := []int{2, 4}
	for index, point := range queryPoints {
		index, point := index, point
		var token QueryRead[OrderedCells[uint64]]
		query, declared := DeclareQuery(composition, QuerySpec[uint64]{
			Semantic: coldKey(78_300 + index),
			Project: func(observation Observation) uint64 {
				value, rows := uint64(0), 0
				if !ProjectRows(observation, func(row QueryRow) bool {
					cells, readOK := QueryValue(row, token)
					entry, present, cellOK := cells.At(0)
					if !readOK || !cellOK || !present || cells.Count() != 1 {
						return false
					}
					value, rows = entry, rows+1
					return true
				}) || rows != 1 {
					return 0
				}
				return value
			},
			Result: frozenColdResult(coldKey(78_400 + index)),
		}, func(query *Query[uint64]) bool {
			var readBound bool
			token, readBound = QueryReadFrom(query, read)
			return readBound
		})
		if !declared || query == nil {
			t.Fatalf("nested WTO query %d", point)
		}
		queries[index], queryReads[index] = query, token
	}
	if !composition.Seal() {
		t.Fatal("nested WTO composition seal")
	}
	readRef, readRefOK := factor.Ref(0)
	writeRef, writeRefOK := factor.Ref(0)
	if !readRefOK || !writeRefOK {
		t.Fatal("nested WTO factor refs")
	}

	outputs := make([]int, len(rules))
	for role, law := range rules {
		outputs[role] = law.output
	}
	instances := make([]*RuleInstance[uint64, ruleUnit], len(rules))
	for role := range rules {
		role := role
		instance, instanceOK := NewRuleInstance(declaredRules[role], ruleUnitForSemantic(coldKey(78_700+role)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
			for _, declaredRead := range declaredReads[role] {
				if !InstanceRead(binding, declaredRead, readRef) {
					return false
				}
			}
			return InstanceWrite(binding, declaredWrites[role], writeRef)
		})
		if !instanceOK {
			t.Fatalf("nested WTO rule %d instance", role)
		}
		instances[role] = instance
	}
	batch, sites, occurrences, operands, admitted := wtoSourceRows(78_500, 78_600, len(rules), outputs, instances)
	if !admitted {
		t.Fatal("nested WTO source batch")
	}
	queryInstances := make([]*QueryInstance[uint64], len(queries))
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		points := make([]*assemblyPoint, len(rules))
		for index := range points {
			points[index] = admitPoint(assembly, sites[index])
			if points[index] == nil {
				t.Fatalf("nested WTO point %d", index)
			}
		}
		for _, role := range topologyOrder {
			law := rules[role]
			member := admitInstance(assembly, points[law.output], occurrences[role], operands[role], instances[role])
			if member == nil {
				t.Fatalf("nested WTO rule %d assembly", role)
			}
			group := admitGroup(assembly, points[law.output], member)
			if group == nil {
				t.Fatalf("nested WTO rule %d group", role)
			}
			for edge, input := range law.inputs {
				boundary, boundaryOK := wtoIdentityBoundary(sites[input], sites[law.output], coldKey(78_800+role*10+edge))
				if !boundaryOK || !admitBoundary(assembly, group, boundary) {
					t.Fatalf("nested WTO rule %d boundary", role)
				}
			}
		}
		for index, query := range queries {
			var queryInstanceOK bool
			queryInstance, queryInstanceOK := NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
				return InstanceQueryRead(binding, queryReads[index], readRef)
			})
			queryInstances[index] = queryInstance
			if !queryInstanceOK || admitQueryAt(assembly, points[queryPoints[index]], queryInstance) == nil {
				t.Fatalf("nested WTO query %d assembly", index)
			}
		}
		return true
	})
	if !compiled || solver == nil {
		t.Fatal("nested WTO solver assembly")
	}
	state, status := solver.Solve(context.Background())
	if state == nil || status != SolveComplete {
		t.Fatalf("nested WTO Solve = state:%v status:%v", state, status)
	}
	receipts := make([]QueryReceipt[uint64], len(queryInstances))
	for index, instance := range queryInstances {
		var receiptOK bool
		receipts[index], receiptOK = instance.Receipt()
		if !receiptOK {
			t.Fatalf("nested WTO query receipt %d", index)
		}
	}
	results := make([]uint64, len(queries))
	for index := range queries {
		value, readable := QueryResult(receipts[index], state)
		if !readable {
			t.Fatalf("nested WTO query %d has incomplete evidence", index)
		}
		results[index] = value
	}
	return results
}
