package engine

import (
	"context"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/lattice"
)

// The route ordinal is intentionally zero-safe: an ordinary write can be
// reconstructed with a Go zero value without becoming a route write.
func TestOutputWriteRuntimeZeroRouteOrdinalIsOrdinary(t *testing.T) {
	projection := &outputRuntime{writes: []outputWriteRuntime{{}}}
	if route, routed := projection.routeRead(); routed || route != 0 {
		t.Fatalf("zero write route = %d/%t, want ordinary", route, routed)
	}
}

type routeWriteScaleMeasurement struct {
	targets   int
	retained  uint64
	allocated uint64
}

// TestRouteWriteRouteUniverseAssemblyScalesLinearly keeps the route universe
// attached once to the output Factor. The two assembled fixtures differ only
// in R. A route implementation that rebuilds/copies a candidate universe per
// member, group, or target incurs a superlinear retained/assembly allocation
// delta and fails this black-box assembly gate.
func TestRouteWriteRouteUniverseAssemblyScalesLinearly(t *testing.T) {
	const (
		smallTargets = 64
		largeTargets = 1024
	)
	small := newRouteWriteScaleFixture(t, smallTargets)
	smallMeasured := small.measureAssembly(t)
	large := newRouteWriteScaleFixture(t, largeTargets)
	largeMeasured := large.measureAssembly(t)
	if largeMeasured.retained > smallMeasured.retained*64 || largeMeasured.allocated > smallMeasured.allocated*64 {
		t.Fatalf("route universe assembly grew superlinearly: small=%+v large=%+v", smallMeasured, largeMeasured)
	}
	small.solveAndCheck(t)
	large.solveAndCheck(t)
	t.Logf("route universe assembly: R=%d retained=%d allocated=%d; R=%d retained=%d allocated=%d", smallMeasured.targets, smallMeasured.retained, smallMeasured.allocated, largeMeasured.targets, largeMeasured.retained, largeMeasured.allocated)
}

// TestRouteWriteRouteUniverseIsNotRetainedPerGroup builds many independent
// route-writing Groups in the same Region and Factor. The route universe is
// authored by the Factor once; every recurrence footprint may retain only the
// owner identity and flags. This white-box shape law catches a regression in
// which each Group copies all R route targets into its footprint. The public
// solve below still checks the joined result and route work.
func TestRouteWriteRouteUniverseIsNotRetainedPerGroup(t *testing.T) {
	const (
		targets = 128
		groups  = 12
	)
	fixture := newRouteWriteScaleFixtureWithGroups(t, targets, groups)
	if !fixture.assemble(t) || fixture.solver == nil || fixture.solver.runtime == nil {
		t.Fatal("route-write grouped assembly")
	}
	routeFootprints, routeFactors := 0, make(map[composition.Key]struct{})
	for _, producer := range fixture.solver.runtime.producers {
		for _, footprint := range producer.footprint {
			if !footprint.route {
				continue
			}
			routeFootprints++
			if footprint.routeFactor == nil || footprint.routeFactor.semantic().compositionKey() != footprint.key {
				t.Fatalf("route footprint lost exact Factor owner: %+v", footprint)
			}
			routeFactors[footprint.key] = struct{}{}
			if len(footprint.targets) != 0 || len(footprint.narrowTargets) != 0 {
				t.Fatalf("route footprint retained Factor universe: key=%v targets=%d narrow=%d", footprint.key, len(footprint.targets), len(footprint.narrowTargets))
			}
		}
	}
	if routeFootprints != groups || len(routeFactors) != 1 {
		t.Fatalf("route footprint shape: groups=%d footprints=%d factors=%d", groups, routeFootprints, len(routeFactors))
	}
	fixture.solveAndCheck(t)
	if fixture.emitted != targets*groups {
		t.Fatalf("grouped route work=%d, want=%d", fixture.emitted, targets*groups)
	}
}

type routeWriteScaleFixture struct {
	targets int
	groups  int

	composition *Composition
	batch       *equation.Batch
	query       *Query[uint64]
	queryRead   QueryRead[OrderedCells[uint64]]
	heapZero    Ref[uint64]

	sourceSite, targetSite                             equation.Site
	controlOccurrence, heapOccurrence, routeOccurrence equation.Occurrence
	controlOperand, heapOperand, routeOperand          equation.Operand
	controlInstance, heapInstance, routeInstance       *RuleInstance[uint64, ruleUnit]
	controlBoundary, heapBoundary                      equation.Input
	routeOccurrences                                   []equation.Occurrence
	routeOperands                                      []equation.Operand
	routeInstances                                     []*RuleInstance[uint64, ruleUnit]
	solver                                             *Solver
	receipt                                            QueryReceipt[uint64]
	emitted                                            int
}

func newRouteWriteScaleFixture(t testing.TB, targets int) *routeWriteScaleFixture {
	return newRouteWriteScaleFixtureWithGroups(t, targets, 1)
}

func newRouteWriteScaleFixtureWithGroups(t testing.TB, targets, groups int) *routeWriteScaleFixture {
	t.Helper()
	if targets < 1 || groups < 1 {
		t.Fatal("route-write scale target denominator")
	}
	fixture := &routeWriteScaleFixture{targets: targets, groups: groups}
	composition := NewComposition()
	controlSpec := coldFactorSpec(coldKey(98_100))
	controlSpec.KeyEnd = 1
	control, controlOK := DeclareFactor(composition, controlSpec, func(*Factor[uint64, uint64]) bool { return true })
	heapSpec := coldFactorSpec(coldKey(98_101))
	heapSpec.KeyEnd = uint64(targets)
	heap, heapOK := DeclareFactor(composition, heapSpec, func(*Factor[uint64, uint64]) bool { return true })
	controlRead, controlReadOK := ExactReadForm(control)
	controlWrite, controlWriteOK := ExactWriteForm(control)
	heapRead, heapReadOK := ExactReadForm(heap)
	heapWrite, heapWriteOK := ExactWriteForm(heap)
	if !controlOK || control == nil || !heapOK || heap == nil || !controlReadOK || !controlWriteOK || !heapReadOK || !heapWriteOK {
		t.Fatal("route-write scale factors/forms")
	}

	var controlWriteToken, heapWriteToken Write[uint64]
	controlRule, controlRuleOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(98_102), OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Output: control.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](98_103),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var declared bool
		controlWriteToken, declared = WriteTo(rule, controlWrite)
		return declared
	})
	heapRule, heapRuleOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(98_104), OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Output: heap.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](98_105),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var declared bool
		heapWriteToken, declared = WriteTo(rule, heapWrite)
		return declared
	})

	var trigger Read[OrderedCells[uint64]]
	var selection Read[Selection[uint64, OrderedCells[uint64]]]
	var routeWrite Write[uint64]
	var routeRefs []Ref[uint64]
	routeRule, routeRuleOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(98_106), OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Output: heap.Output(), Inputs: 2, Admission: testTrustedTheorem[uint64](98_107),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool {
				selected, selectedOK := ReadValue(access, row, selection)
				count, counted := SelectionCount(access, row, selected)
				if !selectedOK || !counted || count != len(routeRefs) {
					return false
				}
				return StageSelection(access, row, selected, func(uint64, OrderedCells[uint64]) (uint64, bool) { return 1, true })
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		controlInput, controlInputOK := rule.InputAt(0)
		heapInput, heapInputOK := rule.InputAt(1)
		var triggerOK, selectionOK, writeOK bool
		trigger, triggerOK = ReadFrom(rule, controlInput, controlRead)
		selection, selectionOK = SelectRead[uint64, ruleUnit, uint64, OrderedCells[uint64], uint64](rule, heapInput, heapRead, []Dependency{ReadDependency(trigger)}, func(context SelectorContext, _ ruleUnit) bool {
			cells, readable := SelectorRead(context, trigger)
			if !readable || waveCCell(cells) != 1 || len(routeRefs) != targets {
				return false
			}
			for index, ref := range routeRefs {
				fixture.emitted++
				if !SelectRoute(context, ref, uint64(index+1)) {
					return false
				}
			}
			return true
		})
		routeWrite, writeOK = RouteWrite(rule, selection)
		return controlInputOK && heapInputOK && triggerOK && selectionOK && writeOK
	})
	if !controlRuleOK || controlRule == nil || !heapRuleOK || heapRule == nil || !routeRuleOK || routeRule == nil {
		t.Fatal("route-write scale rules")
	}

	var queryRead QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(98_108),
		Project: func(observation Observation) uint64 {
			var value uint64
			rows := 0
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, readable := QueryValue(row, queryRead)
				value, rows = waveCCell(cells), rows+1
				return readable && value == 1
			}) || rows != 1 {
				return 0
			}
			return value
		},
		Result: frozenColdResult(coldKey(98_109)),
	}, func(query *Query[uint64]) bool {
		var declared bool
		queryRead, declared = QueryReadFrom(query, heapRead)
		return declared
	})
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("route-write scale query/seal")
	}
	controlZero, controlIssued := control.Ref(0)
	heapZero, heapZeroIssued := heap.Ref(0)
	routeRefs = make([]Ref[uint64], targets)
	for index := range routeRefs {
		ref, issued := heap.Ref(uint64(index))
		if !issued {
			t.Fatalf("route-write scale ref %d", index)
		}
		routeRefs[index] = ref
	}
	if !controlIssued || !heapZeroIssued {
		t.Fatal("route-write scale refs")
	}

	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	sourceSite, sourceSiteOK := batch.AdmitSite(coldKey(98_110).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	targetSite, targetSiteOK := batch.AdmitSite(coldKey(98_111).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
	controlOccurrence, controlOccurrenceOK := batch.Relation(sourceSite, coldKey(98_112).compositionKey())
	heapOccurrence, heapOccurrenceOK := batch.Relation(sourceSite, coldKey(98_113).compositionKey())
	routeOccurrence, routeOccurrenceOK := batch.Relation(targetSite, coldKey(98_114).compositionKey())
	controlInstance, controlInstanceOK := NewRuleInstance(controlRule, ruleUnitForSemantic(coldKey(98_115)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, controlWriteToken, controlZero)
	})
	heapInstance, heapInstanceOK := NewRuleInstance(heapRule, ruleUnitForSemantic(coldKey(98_116)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, heapWriteToken, heapZero)
	})
	routeInstance, routeInstanceOK := NewRuleInstance(routeRule, ruleUnitForSemantic(coldKey(98_117)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceRead(binding, trigger, controlZero) && InstanceSelectorRead(binding, selection, heapRead) && InstanceRouteWrite(binding, routeWrite, selection)
	})
	controlOperand, controlOperandOK := admitInstanceOperand(batch, controlOccurrence, controlInstance)
	heapOperand, heapOperandOK := admitInstanceOperand(batch, heapOccurrence, heapInstance)
	routeOperand, routeOperandOK := admitInstanceOperand(batch, routeOccurrence, routeInstance)
	routeOccurrences := make([]equation.Occurrence, fixture.groups)
	routeOperands := make([]equation.Operand, fixture.groups)
	routeInstances := make([]*RuleInstance[uint64, ruleUnit], fixture.groups)
	if fixture.groups > 0 {
		routeOccurrences[0], routeOperands[0], routeInstances[0] = routeOccurrence, routeOperand, routeInstance
	}
	for index := 1; index < fixture.groups; index++ {
		occurrence, occurrenceOK := batch.Relation(targetSite, coldKey(uint64(120_000+index)).compositionKey())
		instance, instanceOK := NewRuleInstance(routeRule, ruleUnitForSemantic(coldKey(uint64(121_000+index))), func(binding *RuleBinding[uint64, ruleUnit]) bool {
			return InstanceRead(binding, trigger, controlZero) && InstanceSelectorRead(binding, selection, heapRead) && InstanceRouteWrite(binding, routeWrite, selection)
		})
		operand, operandOK := admitInstanceOperand(batch, occurrence, instance)
		if !occurrenceOK || !instanceOK || !operandOK {
			t.Fatalf("route-write grouped row %d", index)
		}
		routeOccurrences[index], routeOperands[index], routeInstances[index] = occurrence, operand, instance
	}
	if !scope.Available() || !sourceSiteOK || !targetSiteOK || !controlOccurrenceOK || !heapOccurrenceOK || !routeOccurrenceOK || !controlInstanceOK || !heapInstanceOK || !routeInstanceOK || !controlOperandOK || !heapOperandOK || !routeOperandOK || !batch.Seal() {
		t.Fatal("route-write scale batch")
	}
	controlBoundary := equation.BoundaryInput(sourceSite, targetSite, coldKey(98_118).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	heapBoundary := equation.BoundaryInput(sourceSite, targetSite, coldKey(98_119).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	fixture.composition, fixture.batch, fixture.query, fixture.queryRead, fixture.heapZero = composition, batch, query, queryRead, heapZero
	fixture.sourceSite, fixture.targetSite = sourceSite, targetSite
	fixture.controlOccurrence, fixture.heapOccurrence, fixture.routeOccurrence = controlOccurrence, heapOccurrence, routeOccurrence
	fixture.controlOperand, fixture.heapOperand, fixture.routeOperand = controlOperand, heapOperand, routeOperand
	fixture.controlInstance, fixture.heapInstance, fixture.routeInstance = controlInstance, heapInstance, routeInstance
	fixture.controlBoundary, fixture.heapBoundary = controlBoundary, heapBoundary
	fixture.routeOccurrences, fixture.routeOperands, fixture.routeInstances = routeOccurrences, routeOperands, routeInstances
	return fixture
}

func (fixture *routeWriteScaleFixture) assemble(t testing.TB) bool {
	t.Helper()
	if fixture == nil || fixture.composition == nil || fixture.batch == nil || fixture.solver != nil {
		return false
	}
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(fixture.composition, fixture.batch, func(assembly *Assembly) bool {
		sourcePoint, targetPoint := admitPoint(assembly, fixture.sourceSite), admitPoint(assembly, fixture.targetSite)
		controlMember := admitInstance(assembly, sourcePoint, fixture.controlOccurrence, fixture.controlOperand, fixture.controlInstance)
		heapMember := admitInstance(assembly, sourcePoint, fixture.heapOccurrence, fixture.heapOperand, fixture.heapInstance)
		routeMembers := make([]*assemblyMember, len(fixture.routeOccurrences))
		for index := range routeMembers {
			routeMembers[index] = admitInstance(assembly, targetPoint, fixture.routeOccurrences[index], fixture.routeOperands[index], fixture.routeInstances[index])
		}
		sourceGroup := admitGroup(assembly, sourcePoint, controlMember, heapMember)
		targetGroups := make([]*assemblyGroup, len(routeMembers))
		for index, member := range routeMembers {
			targetGroups[index] = admitGroup(assembly, targetPoint, member)
		}
		var queryOK bool
		queryInstance, queryOK = NewQueryInstance(fixture.query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, fixture.queryRead, fixture.heapZero)
		})
		if sourcePoint == nil || targetPoint == nil || controlMember == nil || heapMember == nil || sourceGroup == nil || queryInstance == nil || !queryOK || admitQueryAt(assembly, targetPoint, queryInstance) == nil || !fixture.controlBoundary.Available() || !fixture.heapBoundary.Available() {
			return false
		}
		for index, group := range targetGroups {
			if routeMembers[index] == nil || group == nil || !admitBoundary(assembly, group, fixture.controlBoundary) || !admitBoundary(assembly, group, fixture.heapBoundary) {
				return false
			}
		}
		return true
	})
	if !compiled || solver == nil {
		return false
	}
	fixture.solver = solver
	var receiptOK bool
	fixture.receipt, receiptOK = queryInstance.Receipt()
	if !receiptOK {
		return false
	}
	return true
}

func (fixture *routeWriteScaleFixture) measureAssembly(t testing.TB) routeWriteScaleMeasurement {
	t.Helper()
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	if !fixture.assemble(t) {
		t.Fatal("route-write scale assembly")
	}
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(fixture)
	if after.HeapAlloc < before.HeapAlloc || after.TotalAlloc < before.TotalAlloc {
		t.Fatalf("route-write scale memory accounting before=%+v after=%+v", before, after)
	}
	return routeWriteScaleMeasurement{targets: fixture.targets, retained: after.HeapAlloc - before.HeapAlloc, allocated: after.TotalAlloc - before.TotalAlloc}
}

func (fixture *routeWriteScaleFixture) solveAndCheck(t testing.TB) {
	t.Helper()
	if fixture == nil || fixture.solver == nil || fixture.query == nil {
		t.Fatal("unassembled route-write scale fixture")
	}
	state, status := fixture.solver.Solve(context.Background())
	value, readable := QueryResult(fixture.receipt, state)
	wantRoutes := fixture.targets * fixture.groups
	if status != SolveComplete || state == nil || !readable || value != 1 || fixture.emitted != wantRoutes {
		t.Fatalf("route-write scale solve state:%v status:%v value:%d readable:%t routes:%d want:%d", state, status, value, readable, fixture.emitted, wantRoutes)
	}
	before := fixture.emitted
	warm, warmStatus := fixture.solver.Solve(context.Background())
	warmValue, warmReadable := QueryResult(fixture.receipt, warm)
	if warmStatus != SolveComplete || warm == nil || !warmReadable || warmValue != 1 || fixture.emitted != before {
		t.Fatalf("warmed route-write scale solve state:%v status:%v value:%d readable:%t routes:%d want:%d", warm, warmStatus, warmValue, warmReadable, fixture.emitted, before)
	}
}

// TestRouteWritePreservesCarriesJoinsDuplicateRoutesAndBindsEvidence drives
// the one production route-write path. Two distinct tags select the same
// Heap coordinate, so the Factor's Join must combine their values before the
// sole strong Set. A third route targets B, while Carry keeps C intact. The
// checker replays every exact selected route and its ticketed target/value
// pair. A downstream Carry and return Carry form an SCC, so this is also a
// black-box law for route-scope propagation through recursive Carry closure.
func TestRouteWritePreservesCarriesJoinsDuplicateRoutesAndBindsEvidence(t *testing.T) {
	composition := NewComposition()
	controlSpec := coldFactorSpec(coldKey(98_000))
	controlSpec.KeyEnd = 1
	control, controlOK := DeclareFactor(composition, controlSpec, func(*Factor[uint64, uint64]) bool { return true })
	heapSpec := coldFactorSpec(coldKey(98_001))
	heapSpec.KeyEnd = 3
	heapSpec.Lattice = lattice.Lattice[uint64]{
		Bottom: func() uint64 { return 0 }, Top: func() uint64 { return ^uint64(0) },
		Equal:    func(left, right uint64) bool { return left == right },
		LessOrEq: func(left, right uint64) bool { return left|right == right },
		Join:     func(left, right uint64) uint64 { return left | right },
		Widen:    func(left, right uint64) uint64 { return left | right },
	}
	// The return Carry below creates one semantic SCC. Its rank is a
	// well-founded lattice witness, not a convergence budget.
	heapSpec.WidenRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }}
	heap, heapOK := DeclareFactor(composition, heapSpec, func(*Factor[uint64, uint64]) bool { return true })
	controlRead, controlReadOK := ExactReadForm(control)
	controlWrite, controlWriteOK := ExactWriteForm(control)
	heapRead, heapReadOK := ExactReadForm(heap)
	heapWrite, heapWriteOK := ExactWriteForm(heap)
	carry, carryOK := Carry(heap)
	if !controlOK || control == nil || !heapOK || heap == nil || !controlReadOK || !controlWriteOK || !heapReadOK || !heapWriteOK || !carryOK {
		t.Fatal("route-write factors/forms")
	}

	var controlWriteToken Write[uint64]
	controlSeed, controlSeedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(98_002), OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Output: control.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](98_003),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var declared bool
		controlWriteToken, declared = WriteTo(rule, controlWrite)
		return declared
	})

	var heapZeroWrite, heapOneWrite, heapTwoWrite Write[uint64]
	heapSeed, heapSeedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(98_004), OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Output: heap.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](98_005),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var zeroOK, oneOK, twoOK bool
		heapZeroWrite, zeroOK = WriteTo(rule, heapWrite)
		heapOneWrite, oneOK = WriteTo(rule, heapWrite)
		heapTwoWrite, twoOK = WriteTo(rule, heapWrite)
		return zeroOK && oneOK && twoOK
	})

	var trigger Read[OrderedCells[uint64]]
	var selection Read[Selection[uint64, OrderedCells[uint64]]]
	var routeWrite Write[uint64]
	var heapZero, heapOne Ref[uint64]
	var staleDerivation RuleDerivation[uint64, ruleUnit]
	var staleDisposition RuleDisposition[uint64]
	var staleOutput RuleOutput[uint64]
	var capturedEvidence bool
	transferCalls, evidenceCalls := 0, 0
	routeRule, routeRuleOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(98_006), OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Output: heap.Output(), Inputs: 2,
		Admission: AdmitRuleByDerivation(coldKey(98_007), func(derivation RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) {
			evidenceCalls++
			if derivation.ReadCount() != 3 || derivation.DispositionCount() != 1 {
				return RuleEvidence{}, false
			}
			disposition, present := derivation.DispositionAt(0)
			if !present || disposition.Kind() != RuleDispositionStaged || disposition.TargetCount() != 0 || disposition.OutputCount() != 3 {
				return RuleEvidence{}, false
			}
			if _, available := disposition.Value(); available {
				return RuleEvidence{}, false
			}
			count, counted := DerivationDispositionSelectionCount[uint64, ruleUnit, uint64, OrderedCells[uint64]](derivation, disposition, selection)
			if !counted || count != 3 {
				return RuleEvidence{}, false
			}
			for index, want := range []struct {
				tag, output uint64
				target      Ref[uint64]
			}{{tag: 3, output: 2, target: heapZero}, {tag: 8, output: 4, target: heapZero}, {tag: 9, output: 8, target: heapOne}} {
				tag, cells, selected := DerivationDispositionSelectionAt[uint64, ruleUnit, uint64, OrderedCells[uint64]](derivation, disposition, selection, index)
				output, outputOK := disposition.OutputAt(index)
				routeTag, routeCells, routed := DerivationDispositionRouteValue[uint64, ruleUnit, uint64, OrderedCells[uint64]](derivation, disposition, selection, output)
				if !selected || !outputOK || !routed || tag != want.tag || routeTag != want.tag || waveCCell(cells) != 1 || waveCCell(routeCells) != 1 || output.Value() != want.output || !TargetMatchesRef(output.Target(), want.target) {
					return RuleEvidence{}, false
				}
			}
			first, firstOK := disposition.OutputAt(0)
			second, secondOK := disposition.OutputAt(1)
			if !firstOK || !secondOK {
				return RuleEvidence{}, false
			}
			// No cross-derivation/row/ordinal pairing is accepted, even when the
			// target happens to be identical and V is not comparable.
			foreignDerivation := derivation
			foreignDerivation.identity = coldKey(98_008)
			foreignDisposition := RuleDisposition[uint64]{}
			swapped := first
			swapped.ordinal = 1
			foreignOutput := first
			foreignOutput.witness = second.witness
			if _, _, accepted := DerivationDispositionRouteValue[uint64, ruleUnit, uint64, OrderedCells[uint64]](foreignDerivation, disposition, selection, first); accepted {
				return RuleEvidence{}, false
			}
			if _, _, accepted := DerivationDispositionRouteValue[uint64, ruleUnit, uint64, OrderedCells[uint64]](derivation, foreignDisposition, selection, first); accepted {
				return RuleEvidence{}, false
			}
			if _, _, accepted := DerivationDispositionRouteValue[uint64, ruleUnit, uint64, OrderedCells[uint64]](derivation, disposition, Read[Selection[uint64, OrderedCells[uint64]]]{}, first); accepted {
				return RuleEvidence{}, false
			}
			if _, _, accepted := DerivationDispositionRouteValue[uint64, ruleUnit, uint64, OrderedCells[uint64]](derivation, disposition, selection, swapped); accepted {
				return RuleEvidence{}, false
			}
			if _, _, accepted := DerivationDispositionRouteValue[uint64, ruleUnit, uint64, OrderedCells[uint64]](derivation, disposition, selection, foreignOutput); accepted {
				return RuleEvidence{}, false
			}
			staleDerivation, staleDisposition, staleOutput, capturedEvidence = derivation, disposition, first, true
			return derivation.Accept()
		}),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			transferCalls++
			return Product(access, func(row Row) bool {
				cells, readable := ReadValue(access, row, trigger)
				selected, selectedOK := ReadValue(access, row, selection)
				count, counted := SelectionCount(access, row, selected)
				if !readable || waveCCell(cells) != 1 || !selectedOK || !counted || count != 3 {
					return false
				}
				return StageSelection(access, row, selected, func(tag uint64, selected OrderedCells[uint64]) (uint64, bool) {
					if waveCCell(selected) != 1 {
						return 0, false
					}
					switch tag {
					case 3:
						return 2, true
					case 8:
						return 4, true
					case 9:
						return 8, true
					default:
						return 0, false
					}
				})
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		controlInput, controlInputOK := rule.InputAt(0)
		heapInput, heapInputOK := rule.InputAt(1)
		var triggerOK, selectionOK, writeOK bool
		trigger, triggerOK = ReadFrom(rule, controlInput, controlRead)
		selection, selectionOK = SelectRead[uint64, ruleUnit, uint64, OrderedCells[uint64], uint64](rule, heapInput, heapRead, []Dependency{ReadDependency(trigger)}, func(context SelectorContext, _ ruleUnit) bool {
			cells, readable := SelectorRead(context, trigger)
			return readable && waveCCell(cells) == 1 && SelectRoute(context, heapZero, uint64(3)) && SelectRoute(context, heapZero, uint64(8)) && SelectRoute(context, heapOne, uint64(9))
		})
		routeWrite, writeOK = RouteWrite(rule, selection)
		return controlInputOK && heapInputOK && triggerOK && selectionOK && writeOK && CarryFrom(rule, heapInput, carry)
	})
	if !controlSeedOK || controlSeed == nil || !heapSeedOK || heapSeed == nil || !routeRuleOK || routeRule == nil {
		t.Fatal("route-write rule declarations")
	}
	carryRule, carryRuleOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(98_021), OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Output: heap.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](98_022),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(Row) bool { return true })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		return inputOK && CarryFrom(rule, input, carry)
	})
	if !carryRuleOK || carryRule == nil {
		t.Fatal("route-write carry declaration")
	}

	var zeroRead, oneRead, twoRead QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(98_009),
		Project: func(observation Observation) uint64 {
			result, rows := uint64(0), 0
			complete := ProjectRows(observation, func(row QueryRow) bool {
				zeroCells, zeroOK := QueryValue(row, zeroRead)
				oneCells, oneOK := QueryValue(row, oneRead)
				twoCells, twoOK := QueryValue(row, twoRead)
				zero, zeroPresent, zeroValid := zeroCells.At(0)
				one, onePresent, oneValid := oneCells.At(0)
				two, twoPresent, twoValid := twoCells.At(0)
				if !zeroOK || !oneOK || !twoOK || zeroCells.Count() != 1 || oneCells.Count() != 1 || twoCells.Count() != 1 || !zeroValid || !oneValid || !twoValid || !zeroPresent || !onePresent || !twoPresent {
					return false
				}
				rows++
				result = zero<<16 | one<<8 | two
				return true
			})
			if !complete || rows != 1 {
				return 0
			}
			return result
		},
		Result: frozenColdResult(coldKey(98_010)),
	}, func(query *Query[uint64]) bool {
		var zeroOK, oneOK, twoOK bool
		zeroRead, zeroOK = QueryReadFrom(query, heapRead)
		oneRead, oneOK = QueryReadFrom(query, heapRead)
		twoRead, twoOK = QueryReadFrom(query, heapRead)
		return zeroOK && oneOK && twoOK
	})
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("route-write query/seal")
	}
	controlZero, controlIssued := control.Ref(0)
	heapZero, heapZeroIssued := heap.Ref(0)
	heapOne, heapOneIssued := heap.Ref(1)
	heapTwo, heapTwoIssued := heap.Ref(2)
	if !controlIssued || !heapZeroIssued || !heapOneIssued || !heapTwoIssued {
		t.Fatal("route-write refs")
	}

	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	sourceSite, sourceSiteOK := batch.AdmitSite(coldKey(98_011).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	targetSite, targetSiteOK := batch.AdmitSite(coldKey(98_012).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
	downstreamSite, downstreamSiteOK := batch.AdmitSite(coldKey(98_030).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
	controlOccurrence, controlOccurrenceOK := batch.Relation(sourceSite, coldKey(98_013).compositionKey())
	heapOccurrence, heapOccurrenceOK := batch.Relation(sourceSite, coldKey(98_014).compositionKey())
	routeOccurrence, routeOccurrenceOK := batch.Relation(targetSite, coldKey(98_015).compositionKey())
	returnOccurrence, returnOccurrenceOK := batch.Relation(targetSite, coldKey(98_031).compositionKey())
	downstreamOccurrence, downstreamOccurrenceOK := batch.Relation(downstreamSite, coldKey(98_032).compositionKey())
	controlInstance, controlInstanceOK := NewRuleInstance(controlSeed, ruleUnitForSemantic(coldKey(98_016)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, controlWriteToken, controlZero)
	})
	heapInstance, heapInstanceOK := NewRuleInstance(heapSeed, ruleUnitForSemantic(coldKey(98_017)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, heapZeroWrite, heapZero) && InstanceWrite(binding, heapOneWrite, heapOne) && InstanceWrite(binding, heapTwoWrite, heapTwo)
	})
	routeInstance, routeInstanceOK := NewRuleInstance(routeRule, ruleUnitForSemantic(coldKey(98_018)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceRead(binding, trigger, controlZero) && InstanceSelectorRead(binding, selection, heapRead) && InstanceRouteWrite(binding, routeWrite, selection)
	})
	returnInstance, returnInstanceOK := NewRuleInstance(carryRule, ruleUnitForSemantic(coldKey(98_033)), func(*RuleBinding[uint64, ruleUnit]) bool { return true })
	downstreamInstance, downstreamInstanceOK := NewRuleInstance(carryRule, ruleUnitForSemantic(coldKey(98_034)), func(*RuleBinding[uint64, ruleUnit]) bool { return true })
	controlOperand, controlOperandOK := admitInstanceOperand(batch, controlOccurrence, controlInstance)
	heapOperand, heapOperandOK := admitInstanceOperand(batch, heapOccurrence, heapInstance)
	routeOperand, routeOperandOK := admitInstanceOperand(batch, routeOccurrence, routeInstance)
	returnOperand, returnOperandOK := admitInstanceOperand(batch, returnOccurrence, returnInstance)
	downstreamOperand, downstreamOperandOK := admitInstanceOperand(batch, downstreamOccurrence, downstreamInstance)
	if !scope.Available() || !sourceSiteOK || !targetSiteOK || !downstreamSiteOK || !controlOccurrenceOK || !heapOccurrenceOK || !routeOccurrenceOK || !returnOccurrenceOK || !downstreamOccurrenceOK ||
		!controlInstanceOK || !heapInstanceOK || !routeInstanceOK || !returnInstanceOK || !downstreamInstanceOK || !controlOperandOK || !heapOperandOK || !routeOperandOK || !returnOperandOK || !downstreamOperandOK || !batch.Seal() {
		t.Fatal("route-write batch")
	}
	controlBoundary := equation.BoundaryInput(sourceSite, targetSite, coldKey(98_019).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	heapBoundary := equation.BoundaryInput(sourceSite, targetSite, coldKey(98_020).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	returnBoundary := equation.BoundaryInput(downstreamSite, targetSite, coldKey(98_035).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	downstreamBoundary := equation.BoundaryInput(targetSite, downstreamSite, coldKey(98_036).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		sourcePoint, targetPoint, downstreamPoint := admitPoint(assembly, sourceSite), admitPoint(assembly, targetSite), admitPoint(assembly, downstreamSite)
		controlMember := admitInstance(assembly, sourcePoint, controlOccurrence, controlOperand, controlInstance)
		heapMember := admitInstance(assembly, sourcePoint, heapOccurrence, heapOperand, heapInstance)
		routeMember := admitInstance(assembly, targetPoint, routeOccurrence, routeOperand, routeInstance)
		returnMember := admitInstance(assembly, targetPoint, returnOccurrence, returnOperand, returnInstance)
		downstreamMember := admitInstance(assembly, downstreamPoint, downstreamOccurrence, downstreamOperand, downstreamInstance)
		sourceGroup := admitGroup(assembly, sourcePoint, controlMember, heapMember)
		targetGroup := admitGroup(assembly, targetPoint, routeMember)
		returnGroup := admitGroup(assembly, targetPoint, returnMember)
		downstreamGroup := admitGroup(assembly, downstreamPoint, downstreamMember)
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, zeroRead, heapZero) && InstanceQueryRead(binding, oneRead, heapOne) && InstanceQueryRead(binding, twoRead, heapTwo)
		})
		observation := admitQueryAt(assembly, downstreamPoint, queryInstance)
		return sourcePoint != nil && targetPoint != nil && downstreamPoint != nil && controlMember != nil && heapMember != nil && routeMember != nil && returnMember != nil && downstreamMember != nil &&
			sourceGroup != nil && targetGroup != nil && returnGroup != nil && downstreamGroup != nil && queryInstanceOK && observation != nil && controlBoundary.Available() && heapBoundary.Available() && returnBoundary.Available() && downstreamBoundary.Available() &&
			admitBoundary(assembly, targetGroup, controlBoundary) && admitBoundary(assembly, targetGroup, heapBoundary) && admitBoundary(assembly, returnGroup, returnBoundary) && admitBoundary(assembly, downstreamGroup, downstreamBoundary)
	})
	if !compiled || solver == nil {
		t.Fatal("route-write assembly")
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	if !receiptOK {
		t.Fatal("route-write query receipt")
	}
	result, readable := QueryResult(receipt, state)
	if status != SolveComplete || state == nil || !readable || result != 0x060801 || !capturedEvidence || transferCalls != 1 || evidenceCalls != 1 {
		t.Fatalf("route-write solve = state:%v status:%v result:%#x readable:%t evidence:%t transfer:%d checker:%d", state, status, result, readable, capturedEvidence, transferCalls, evidenceCalls)
	}
	if _, _, live := DerivationDispositionRouteValue[uint64, ruleUnit, uint64, OrderedCells[uint64]](staleDerivation, staleDisposition, selection, staleOutput); live {
		t.Fatal("route evidence remained live after checker admission")
	}
	allocations := testing.AllocsPerRun(64, func() {
		warm, warmStatus := solver.Solve(context.Background())
		if warmStatus != SolveComplete || warm == nil {
			panic("warmed route-write solve")
		}
	})
	if allocations != 0 {
		t.Fatalf("warmed route-write solve allocations=%v, want 0", allocations)
	}
}
