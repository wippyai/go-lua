package engine

import (
	"context"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// stagedReadScalingMode distinguishes the normal sparse exact locator from
// an intentionally all-target locator. The latter is the honest \"Top-like\"
// case: its semantic result contains every exact target, so R-dependent work
// is expected there rather than hidden in the normal exact route.
type stagedReadScalingMode uint8

const (
	stagedReadScalingExact stagedReadScalingMode = iota + 1
	stagedReadScalingAllTargets
)

const stagedReadScalingOutput uint64 = 97

type stagedReadScalingCounters struct {
	locators       int
	emittedRoutes  int
	selectedRoutes int
	transfers      int
	projects       int
}

func (c stagedReadScalingCounters) same(other stagedReadScalingCounters) bool {
	return c == other
}

type stagedReadScalingMeasurement struct {
	consumers int
	targets   int

	// retained is HeapAlloc after a collection while the assembled Solver is
	// kept live. allocated includes the same assembly transaction's temporary
	// work. Neither observes a runtime private field.
	retained  uint64
	allocated uint64
}

type stagedReadScalingFixture struct {
	mode      stagedReadScalingMode
	consumers int
	targets   int

	composition *Composition
	batch       *equation.Batch
	solver      *Solver
	query       *Query[uint64]
	receipt     QueryReceipt[uint64]
	outputRef   Ref[uint64]
	outputToken QueryRead[OrderedCells[uint64]]

	controlSite, targetSite, outputSite                   equation.Site
	controlOccurrence, targetOccurrence, outputOccurrence equation.Occurrence
	controlOperand, targetOperand, outputOperand          equation.Operand
	controlInstance, targetInstance, outputInstance       *RuleInstance[uint64, ruleUnit]
	controlBoundary, targetBoundary                       equation.Input
	counters                                              *stagedReadScalingCounters
}

// TestStagedExactReadSealedMetadataIsAdditive exercises the assembled
// ReadSelect path with C independent staged-read declarations all targeting
// one Factor with R exact coordinates. The target unit universe is real: the
// fixture seals a Factor of width R and binds every selector through the
// ordinary Assembly transaction. It deliberately does not inspect an engine
// struct field to infer storage shape.
//
// The four measurements vary one dimension at a time. An implementation that
// retained a selector-by-target candidate table would make the combined
// allocation increase dominate the two one-dimensional increases. Additive
// canonical metadata (C + R plus the fixed two input edges) does not.
func TestStagedExactReadSealedMetadataIsAdditive(t *testing.T) {
	const (
		fewConsumers  = 1
		manyConsumers = 16
		fewTargets    = 8
		manyTargets   = 512
	)

	measurements := make(map[[2]int]stagedReadScalingMeasurement, 4)
	for _, dimensions := range [][2]int{
		{fewConsumers, fewTargets},
		{manyConsumers, fewTargets},
		{fewConsumers, manyTargets},
		{manyConsumers, manyTargets},
	} {
		fixture := newStagedReadScalingFixture(t, dimensions[0], dimensions[1], stagedReadScalingExact)
		measurement := fixture.measureAssembly(t)
		fixture.solveAndCheck(t)
		measurements[dimensions] = measurement
	}

	base := measurements[[2]int{fewConsumers, fewTargets}]
	byConsumers := measurements[[2]int{manyConsumers, fewTargets}]
	byTargets := measurements[[2]int{fewConsumers, manyTargets}]
	combined := measurements[[2]int{manyConsumers, manyTargets}]

	assertStagedReadAdditiveAssembly(t, "retained bytes", base.retained, byConsumers.retained, byTargets.retained, combined.retained)
	assertStagedReadAdditiveAssembly(t, "assembly allocated bytes", base.allocated, byConsumers.allocated, byTargets.allocated, combined.allocated)
	t.Logf("staged exact sealed assembly: C=%d,R=%d retained=%d allocated=%d; C=%d,R=%d retained=%d allocated=%d; C=%d,R=%d retained=%d allocated=%d; C=%d,R=%d retained=%d allocated=%d",
		fewConsumers, fewTargets, base.retained, base.allocated,
		manyConsumers, fewTargets, byConsumers.retained, byConsumers.allocated,
		fewConsumers, manyTargets, byTargets.retained, byTargets.allocated,
		manyConsumers, manyTargets, combined.retained, combined.allocated)
}

// TestStagedExactReadRouteWorkStaysSparseAcrossTargetUniverse establishes the
// hot semantic half of the law. Exact locators issue one selected Ref per
// staged declaration at both R values, and the completed warm Solve performs
// no additional locator, selection, or transfer work. In contrast, the
// all-target locator below intentionally emits and exposes R routes: that
// Top-like enumeration is permitted to scale with R.
func TestStagedExactReadRouteWorkStaysSparseAcrossTargetUniverse(t *testing.T) {
	const (
		consumers   = 3
		fewTargets  = 8
		manyTargets = 512
	)

	exactSmall := newStagedReadScalingFixture(t, consumers, fewTargets, stagedReadScalingExact)
	if !exactSmall.assemble(t) {
		t.Fatal("small exact assembly")
	}
	exactSmall.solveAndCheck(t)
	exactLarge := newStagedReadScalingFixture(t, consumers, manyTargets, stagedReadScalingExact)
	if !exactLarge.assemble(t) {
		t.Fatal("large exact assembly")
	}
	exactLarge.solveAndCheck(t)

	if exactSmall.counters.locators == 0 || exactSmall.counters.emittedRoutes == 0 || exactSmall.counters.selectedRoutes == 0 {
		t.Fatalf("small exact route did not execute: %+v", *exactSmall.counters)
	}
	if !exactSmall.counters.same(*exactLarge.counters) {
		t.Fatalf("exact route work changed with target universe: R=%d %+v, R=%d %+v", fewTargets, *exactSmall.counters, manyTargets, *exactLarge.counters)
	}
	if exactLarge.counters.emittedRoutes != exactLarge.counters.locators || exactLarge.counters.selectedRoutes != exactLarge.counters.locators {
		t.Fatalf("exact locator exposed more than one route per invocation: %+v", *exactLarge.counters)
	}

	allSmall := newStagedReadScalingFixture(t, 1, fewTargets, stagedReadScalingAllTargets)
	if !allSmall.assemble(t) {
		t.Fatal("small all-target assembly")
	}
	allSmall.solveAndCheck(t)
	allLarge := newStagedReadScalingFixture(t, 1, manyTargets, stagedReadScalingAllTargets)
	if !allLarge.assemble(t) {
		t.Fatal("large all-target assembly")
	}
	allLarge.solveAndCheck(t)

	if allSmall.counters.locators == 0 || allLarge.counters.locators != allSmall.counters.locators {
		t.Fatalf("all-target locator schedule changed unexpectedly: R=%d %+v, R=%d %+v", fewTargets, *allSmall.counters, manyTargets, *allLarge.counters)
	}
	if allSmall.counters.emittedRoutes != allSmall.counters.locators*fewTargets || allSmall.counters.selectedRoutes != allSmall.counters.emittedRoutes {
		t.Fatalf("small all-target enumeration = %+v, want %d routes per locator", *allSmall.counters, fewTargets)
	}
	if allLarge.counters.emittedRoutes != allLarge.counters.locators*manyTargets || allLarge.counters.selectedRoutes != allLarge.counters.emittedRoutes {
		t.Fatalf("large all-target enumeration = %+v, want %d routes per locator", *allLarge.counters, manyTargets)
	}
	t.Logf("exact route work is R-invariant: %+v; all-target enumeration: R=%d %+v, R=%d %+v", *exactLarge.counters, fewTargets, *allSmall.counters, manyTargets, *allLarge.counters)
}

// assertStagedReadAdditiveAssembly compares the diagonal increase with the
// independent C and R increases. The factor of two tolerates allocator bucket
// rounding and unrelated fixed assembly rows while still rejecting a retained
// C*R candidate surface: at these dimensions its diagonal increase is over an
// order of magnitude larger than the additive deltas.
func assertStagedReadAdditiveAssembly(t testing.TB, name string, base, byConsumers, byTargets, combined uint64) {
	t.Helper()
	if byConsumers < base || byTargets < base || combined < base {
		t.Fatalf("%s did not retain monotone assembled metadata: base=%d consumers=%d targets=%d combined=%d", name, base, byConsumers, byTargets, combined)
	}
	consumerDelta := byConsumers - base
	targetDelta := byTargets - base
	combinedDelta := combined - base
	if consumerDelta == 0 || targetDelta == 0 {
		t.Fatalf("%s did not observe both C and R dimensions: base=%d consumers=%d targets=%d combined=%d", name, base, byConsumers, byTargets, combined)
	}
	if combinedDelta > 2*(consumerDelta+targetDelta) {
		t.Fatalf("%s grows as a cross product: C delta=%d R delta=%d combined delta=%d", name, consumerDelta, targetDelta, combinedDelta)
	}
}

func newStagedReadScalingFixture(t testing.TB, consumers, targets int, mode stagedReadScalingMode) *stagedReadScalingFixture {
	t.Helper()
	if consumers < 1 || targets < 1 || mode != stagedReadScalingExact && mode != stagedReadScalingAllTargets {
		t.Fatal("staged-read scaling dimensions")
	}

	fixture := &stagedReadScalingFixture{consumers: consumers, targets: targets, mode: mode, counters: &stagedReadScalingCounters{}}
	composition := NewComposition()
	controlSpec := coldFactorSpec(stagedReadScalingSemantic(1))
	controlSpec.KeyEnd = 1
	control, controlDeclared := DeclareFactor(composition, controlSpec, func(*Factor[uint64, uint64]) bool { return true })
	targetSpec := coldFactorSpec(stagedReadScalingSemantic(2))
	targetSpec.KeyEnd = uint64(targets)
	target, targetDeclared := DeclareFactor(composition, targetSpec, func(*Factor[uint64, uint64]) bool { return true })
	outputSpec := coldFactorSpec(stagedReadScalingSemantic(3))
	outputSpec.KeyEnd = 1
	output, outputDeclared := DeclareFactor(composition, outputSpec, func(*Factor[uint64, uint64]) bool { return true })
	controlRead, controlReadOK := ExactReadForm(control)
	controlWrite, controlWriteOK := ExactWriteForm(control)
	targetRead, targetReadOK := ExactReadForm(target)
	targetWrite, targetWriteOK := ExactWriteForm(target)
	outputRead, outputReadOK := ExactReadForm(output)
	outputWrite, outputWriteOK := ExactWriteForm(output)
	if !controlDeclared || control == nil || !targetDeclared || target == nil || !outputDeclared || output == nil ||
		!controlReadOK || !controlWriteOK || !targetReadOK || !targetWriteOK || !outputReadOK || !outputWriteOK {
		t.Fatal("staged-read scaling factors/forms")
	}

	var controlWriteToken, targetWriteToken, outputWriteToken Write[uint64]
	controlRule, controlRuleOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: stagedReadScalingSemantic(10), Output: control.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](97_200_011),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var declared bool
		controlWriteToken, declared = WriteTo(rule, controlWrite)
		return declared
	})
	targetRule, targetRuleOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: stagedReadScalingSemantic(12), Output: target.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](97_200_013),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(41)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var declared bool
		targetWriteToken, declared = WriteTo(rule, targetWrite)
		return declared
	})

	var targetRefs []Ref[uint64]
	var predecessor Read[OrderedCells[uint64]]
	selections := make([]Read[Selection[uint64, OrderedCells[uint64]]], consumers)
	outputRule, outputRuleOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: stagedReadScalingSemantic(14), Output: output.Output(), Inputs: 2, Admission: testTrustedTheorem[uint64](97_200_015),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			fixture.counters.transfers++
			return Product(access, func(row Row) bool {
				for selector, selectionRead := range selections {
					selection, selected := ReadValue(access, row, selectionRead)
					if !selected {
						return false
					}
					count, counted := SelectionCount(access, row, selection)
					want := 1
					if fixture.mode == stagedReadScalingAllTargets {
						want = fixture.targets
					}
					if !counted || count != want {
						return false
					}
					for index := 0; index < count; index++ {
						tag, cells, available := SelectionAt(access, row, selection, index)
						if !available || cells.Count() != 1 {
							return false
						}
						value, present, valid := cells.At(0)
						if !valid {
							return false
						}
						if fixture.mode == stagedReadScalingExact {
							if tag != uint64(selector+1) || !present || value != 41 {
								return false
							}
						} else if tag == uint64(fixture.targets/2+1) {
							if !present || value != 41 {
								return false
							}
						} else if present {
							return false
						}
						fixture.counters.selectedRoutes++
					}
				}
				return StageValue(access, row, stagedReadScalingOutput)
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		controlInput, controlInputOK := rule.InputAt(0)
		targetInput, targetInputOK := rule.InputAt(1)
		var predecessorOK, outputWriteOK bool
		predecessor, predecessorOK = ReadFrom(rule, controlInput, controlRead)
		for selector := range selections {
			selector := selector
			selection, declared := SelectRead[uint64, ruleUnit, uint64, OrderedCells[uint64], uint64](rule, targetInput, targetRead, []Dependency{ReadDependency(predecessor)}, func(context SelectorContext, _ ruleUnit) bool {
				fixture.counters.locators++
				cells, readable := SelectorRead(context, predecessor)
				controlValue, present, valid := cells.At(0)
				if !readable || cells.Count() != 1 || !valid || !present || controlValue != 1 {
					return false
				}
				if fixture.mode == stagedReadScalingExact {
					fixture.counters.emittedRoutes++
					return len(targetRefs) == 1 && SelectRoute(context, targetRefs[0], uint64(selector+1))
				}
				for index, ref := range targetRefs {
					fixture.counters.emittedRoutes++
					if !SelectRoute(context, ref, uint64(index+1)) {
						return false
					}
				}
				return true
			})
			if !declared {
				return false
			}
			selections[selector] = selection
		}
		outputWriteToken, outputWriteOK = WriteTo(rule, outputWrite)
		return controlInputOK && targetInputOK && predecessorOK && outputWriteOK
	})
	if !controlRuleOK || controlRule == nil || !targetRuleOK || targetRule == nil || !outputRuleOK || outputRule == nil {
		t.Fatal("staged-read scaling declarations")
	}

	var outputToken QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: stagedReadScalingSemantic(16),
		Project: func(observation Observation) uint64 {
			fixture.counters.projects++
			value, rows := uint64(0), 0
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, readable := QueryValue(row, outputToken)
				entry, present, valid := cells.At(0)
				if !readable || cells.Count() != 1 || !valid || !present {
					return false
				}
				value, rows = entry, rows+1
				return true
			}) || rows != 1 {
				return 0
			}
			return value
		},
		Result: frozenColdResult(stagedReadScalingSemantic(17)),
	}, func(query *Query[uint64]) bool {
		var declared bool
		outputToken, declared = QueryReadFrom(query, outputRead)
		return declared
	})
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("staged-read scaling query")
	}

	selectedTarget := uint64(targets / 2)
	controlRef, controlIssued := control.Ref(0)
	targetRef, targetIssued := target.Ref(selectedTarget)
	outputRef, outputIssued := output.Ref(0)
	if !controlIssued || !targetIssued || !outputIssued {
		t.Fatal("staged-read scaling refs")
	}
	if mode == stagedReadScalingExact {
		targetRefs = []Ref[uint64]{targetRef}
	} else {
		targetRefs = make([]Ref[uint64], targets)
		for index := range targetRefs {
			ref, issued := target.Ref(uint64(index))
			if !issued {
				t.Fatalf("all-target ref %d", index)
			}
			targetRefs[index] = ref
		}
	}

	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	controlSite, controlSiteOK := batch.AdmitSite(stagedReadScalingSemantic(20).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	targetSite, targetSiteOK := batch.AdmitSite(stagedReadScalingSemantic(21).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	outputSite, outputSiteOK := batch.AdmitSite(stagedReadScalingSemantic(22).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
	controlOccurrence, controlOccurrenceOK := batch.At(controlSite)
	targetOccurrence, targetOccurrenceOK := batch.At(targetSite)
	outputOccurrence, outputOccurrenceOK := batch.At(outputSite)
	controlInstance, controlInstanceOK := NewRuleInstance(controlRule, ruleUnitForSemantic(stagedReadScalingSemantic(23)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, controlWriteToken, controlRef)
	})
	targetInstance, targetInstanceOK := NewRuleInstance(targetRule, ruleUnitForSemantic(stagedReadScalingSemantic(24)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, targetWriteToken, targetRef)
	})
	outputInstance, outputInstanceOK := NewRuleInstance(outputRule, ruleUnitForSemantic(stagedReadScalingSemantic(25)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		if !InstanceRead(binding, predecessor, controlRef) {
			return false
		}
		for _, selection := range selections {
			if !InstanceSelectorRead(binding, selection, targetRead) {
				return false
			}
		}
		return InstanceWrite(binding, outputWriteToken, outputRef)
	})
	controlOperand, controlOperandOK := admitInstanceOperand(batch, controlOccurrence, controlInstance)
	targetOperand, targetOperandOK := admitInstanceOperand(batch, targetOccurrence, targetInstance)
	outputOperand, outputOperandOK := admitInstanceOperand(batch, outputOccurrence, outputInstance)
	if !scope.Available() || !controlSiteOK || !targetSiteOK || !outputSiteOK || !controlOccurrenceOK || !targetOccurrenceOK || !outputOccurrenceOK ||
		!controlInstanceOK || !targetInstanceOK || !outputInstanceOK || !controlOperandOK || !targetOperandOK || !outputOperandOK || !batch.Seal() {
		t.Fatal("staged-read scaling source batch")
	}
	controlBoundary := equation.BoundaryInput(controlSite, outputSite, stagedReadScalingSemantic(26).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	targetBoundary := equation.BoundaryInput(targetSite, outputSite, stagedReadScalingSemantic(27).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	if !controlBoundary.Available() || !targetBoundary.Available() {
		t.Fatal("staged-read scaling boundaries")
	}

	fixture.composition, fixture.batch, fixture.query = composition, batch, query
	fixture.outputRef, fixture.outputToken = outputRef, outputToken
	fixture.controlSite, fixture.targetSite, fixture.outputSite = controlSite, targetSite, outputSite
	fixture.controlOccurrence, fixture.targetOccurrence, fixture.outputOccurrence = controlOccurrence, targetOccurrence, outputOccurrence
	fixture.controlOperand, fixture.targetOperand, fixture.outputOperand = controlOperand, targetOperand, outputOperand
	fixture.controlInstance, fixture.targetInstance, fixture.outputInstance = controlInstance, targetInstance, outputInstance
	fixture.controlBoundary, fixture.targetBoundary = controlBoundary, targetBoundary
	return fixture
}

func (fixture *stagedReadScalingFixture) assemble(t testing.TB) bool {
	t.Helper()
	if fixture == nil || fixture.composition == nil || fixture.batch == nil || fixture.solver != nil {
		return false
	}
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(fixture.composition, fixture.batch, func(assembly *Assembly) bool {
		controlPoint := admitPoint(assembly, fixture.controlSite)
		targetPoint := admitPoint(assembly, fixture.targetSite)
		outputPoint := admitPoint(assembly, fixture.outputSite)
		controlMember := admitInstance(assembly, controlPoint, fixture.controlOccurrence, fixture.controlOperand, fixture.controlInstance)
		targetMember := admitInstance(assembly, targetPoint, fixture.targetOccurrence, fixture.targetOperand, fixture.targetInstance)
		outputMember := admitInstance(assembly, outputPoint, fixture.outputOccurrence, fixture.outputOperand, fixture.outputInstance)
		controlGroup := admitGroup(assembly, controlPoint, controlMember)
		targetGroup := admitGroup(assembly, targetPoint, targetMember)
		outputGroup := admitGroup(assembly, outputPoint, outputMember)
		var queryOK bool
		queryInstance, queryOK = NewQueryInstance(fixture.query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, fixture.outputToken, fixture.outputRef)
		})
		return controlPoint != nil && targetPoint != nil && outputPoint != nil && controlMember != nil && targetMember != nil && outputMember != nil &&
			controlGroup != nil && targetGroup != nil && outputGroup != nil && queryOK && queryInstance != nil && admitQueryAt(assembly, outputPoint, queryInstance) != nil &&
			admitBoundary(assembly, outputGroup, fixture.controlBoundary) && admitBoundary(assembly, outputGroup, fixture.targetBoundary)
	})
	if !compiled || solver == nil {
		return false
	}
	receipt, receiptOK := queryInstance.Receipt()
	if !receiptOK {
		return false
	}
	fixture.solver = solver
	fixture.receipt = receipt
	return true
}

func (fixture *stagedReadScalingFixture) measureAssembly(t testing.TB) stagedReadScalingMeasurement {
	t.Helper()
	if fixture == nil {
		t.Fatal("nil staged-read scaling fixture")
	}
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	if !fixture.assemble(t) {
		t.Fatal("staged-read scaling assembly")
	}
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(fixture)
	if after.HeapAlloc < before.HeapAlloc || after.TotalAlloc < before.TotalAlloc {
		t.Fatalf("invalid assembly memory accounting: before=%+v after=%+v", before, after)
	}
	return stagedReadScalingMeasurement{
		consumers: fixture.consumers,
		targets:   fixture.targets,
		retained:  after.HeapAlloc - before.HeapAlloc,
		allocated: after.TotalAlloc - before.TotalAlloc,
	}
}

func (fixture *stagedReadScalingFixture) solveAndCheck(t testing.TB) {
	t.Helper()
	if fixture == nil || fixture.solver == nil || fixture.query == nil || fixture.counters == nil {
		t.Fatal("unassembled staged-read scaling fixture")
	}
	state, status := fixture.solver.Solve(context.Background())
	result, readable := QueryResult(fixture.receipt, state)
	if status != SolveComplete || state == nil || !readable || result != stagedReadScalingOutput || fixture.counters.locators == 0 || fixture.counters.transfers == 0 || fixture.counters.projects != 1 {
		t.Fatalf("staged-read scaling solve = state:%v status:%v result:%d readable:%t counters:%+v", state, status, result, readable, *fixture.counters)
	}
	before := *fixture.counters
	warm, warmStatus := fixture.solver.Solve(context.Background())
	warmResult, warmReadable := QueryResult(fixture.receipt, warm)
	if warmStatus != SolveComplete || warm == nil || !warmReadable || warmResult != stagedReadScalingOutput || !fixture.counters.same(before) {
		t.Fatalf("warmed staged exact route repeated work: state:%v status:%v result:%d readable:%t before:%+v after:%+v", warm, warmStatus, warmResult, warmReadable, before, *fixture.counters)
	}
	allocations := testing.AllocsPerRun(64, func() {
		state, status := fixture.solver.Solve(context.Background())
		if status != SolveComplete || state == nil {
			panic("warmed staged exact Solve")
		}
	})
	if allocations != 0 || !fixture.counters.same(before) {
		t.Fatalf("warmed staged exact route allocations=%v callbacks before:%+v after:%+v", allocations, before, *fixture.counters)
	}
}

func stagedReadScalingSemantic(offset uint64) SemanticKey {
	return coldKey(97_200_000 + offset)
}
