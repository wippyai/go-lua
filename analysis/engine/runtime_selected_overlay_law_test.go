package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
)

// selectedOverlayFixture is deliberately the smallest executable structural
// activation: a seed writes source, while the selected FactorEdge transports
// that Factor to target. Both Points carry a Query so the restricted overlay
// path's total-demand premise is real rather than a test-only bypass.
type selectedOverlayFixture struct {
	solver        *Solver
	narrow        equation.AcceptedMember
	broad         equation.AcceptedMember
	targetReceipt QueryReceipt[uint64]
	source        equation.Site
	target        equation.Site
	idle          equation.Site
}

func newSelectedOverlayFixture(t *testing.T, staticForward, dynamicReverse bool) selectedOverlayFixture {
	return newSelectedOverlayActivationFixture(t, staticForward, dynamicReverse, false)
}

func newSelectedOverlayActivationFixture(t *testing.T, staticForward, dynamicReverse, selectTarget bool) selectedOverlayFixture {
	t.Helper()
	composition := NewComposition()
	factor := coldFactor(composition, coldKey(984_001))
	read, readOK := ExactReadForm(factor)
	write, writeOK := ExactWriteForm(factor)
	var seedWrite Write[uint64]
	seed, seedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(984_002), OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Output: factor.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](984_003),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(17)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var declared bool
		seedWrite, declared = WriteTo(rule, write)
		return declared
	})
	family, familyOK := DeclareActivationFamily(composition, coldKey(984_004))
	triggerKey := coldKey(984_005)
	trigger, triggerOK := DeclareActivationRule(composition, ActivationRuleSpec{
		Semantic: triggerKey, Family: family, Inputs: 0,
		Admission: AdmitActivationByTrustedTheorem(coldKey(984_006)),
		Run: func(value Activation) bool {
			return !selectTarget || Activate(value, coldKey(984_019), coldKey(984_015), coldKey(984_016))
		},
	})
	var queryRead QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(984_007),
		Project: func(observation Observation) uint64 {
			var result uint64
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, read := QueryValue(row, queryRead)
				value, present, valid := cells.At(0)
				if !read || !valid {
					return false
				}
				if !present {
					return true
				}
				if value > result {
					result = value
				}
				return true
			}) {
				return 0
			}
			return result
		},
		Result: frozenColdResult(coldKey(984_008)),
	}, func(value *Query[uint64]) bool {
		var declared bool
		queryRead, declared = QueryReadFrom(value, read)
		return declared
	})
	if factor == nil || !readOK || !writeOK || !seedOK || seed == nil || !familyOK || !triggerOK || trigger == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("selected overlay declarations")
	}
	ref, refOK := factor.Ref(0)
	seedInstance, seedInstanceOK := NewRuleInstance(seed, ruleUnitForSemantic(coldKey(984_009)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, seedWrite, ref)
	})
	carry, carryOK := Carry(factor)
	if !refOK || !seedInstanceOK || seedInstance == nil || !carryOK {
		t.Fatal("selected overlay seed")
	}

	build := NewSourceAssembly(composition)
	decision, decisionOK := build.Decision(coldKey(984_010))
	scope, scopeOK := build.Scope(decision)
	truth, truthOK := build.TrueExpr()
	narrow, narrowOK := build.DecisionExpr(decision)
	notNarrow, negatedOK := build.NotExpr(narrow)
	broad, broadOK := build.OrExpr(narrow, notNarrow)
	identity, identityOK := build.IdentityReindex(scope)
	sourceSite, sourceOK := build.Site(coldKey(984_011), scope, truth, true)
	targetSite, targetOK := build.Site(coldKey(984_012), scope, truth, true)
	idleSite, idleOK := build.Site(coldKey(984_020), scope, truth, true)
	triggerOccurrence, triggerOccurrenceOK := build.At(sourceSite)
	seedOccurrence, seedOccurrenceOK := build.At(sourceSite)
	preparedTrigger, preparedTriggerOK := build.PrepareActivation(triggerOccurrence, coldKey(984_013), trigger)
	preparedSeed, preparedSeedOK := build.PrepareInstance(seedOccurrence, seedInstance)
	role := coldKey(984_014)
	dynamicTarget := targetSite
	if dynamicReverse {
		dynamicTarget = sourceSite
	}
	preparedPlan, staged := StageActivationPlan(build, composition, family, []ActivationPlanEntry{{
		Target: coldKey(984_015), Endpoint: coldKey(984_016),
		FactorEdges: []ActivationFactorEdge{{SourceRole: role, TargetSite: dynamicTarget, Factor: carry, Provenance: coldKey(984_017)}},
	}})
	var staticBoundary SourceBoundary
	staticOK := true
	if staticForward {
		staticBoundary, staticOK = build.Boundary(sourceSite, targetSite, coldKey(984_018), truth, identity, truth)
	}
	idleBoundary, idleBoundaryOK := build.Boundary(sourceSite, idleSite, coldKey(984_021), truth, identity, truth)
	sealed := build.Seal()
	plan, finalized := FinalizeActivationPlan(build, preparedPlan)
	if !decisionOK || !scopeOK || !truthOK || !narrowOK || !negatedOK || !broadOK || !identityOK || !sourceOK || !targetOK || !idleOK || !triggerOccurrenceOK || !seedOccurrenceOK || !preparedTriggerOK || !preparedSeedOK || !staged || !staticOK || !idleBoundaryOK || !sealed || !finalized || plan == nil {
		t.Fatal("selected overlay source staging")
	}

	var targetQuery *QueryInstance[uint64]
	solver, assembled := build.Assemble(func(assembly *Assembly) bool {
		source := admitPoint(assembly, sourceSite.value)
		target := admitPoint(assembly, targetSite.value)
		idle := admitPoint(assembly, idleSite.value)
		if source == nil || target == nil || idle == nil {
			return false
		}
		basePoint := source
		if dynamicReverse {
			basePoint = target
		}
		base, baseOK := ActivationBaseAt(assembly, basePoint)
		port, portOK := NewActivationPort(role, base)
		triggerInstance, triggerCreated := NewActivationTrigger(trigger, coldKey(984_019), plan, []*ActivationPort{port}, func(*StructuralBinding) bool { return true })
		seedMember := admitInstance(assembly, source, preparedSeed.occurrence.value, preparedSeed.operand.value, seedInstance)
		triggerMember := admitStructural(assembly, source, preparedTrigger.occurrence.value, preparedTrigger.operand.value, triggerInstance)
		group := admitGroup(assembly, source, seedMember, triggerMember)
		if staticForward && !admitFactorEdge(assembly, target, staticBoundary.descriptor.input, factor.schema) {
			return false
		}
		if !admitFactorEdge(assembly, idle, idleBoundary.descriptor.input, factor.schema) {
			return false
		}
		sourceQuery, sourceQueryOK := NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, queryRead, ref)
		})
		var targetQueryOK bool
		targetQuery, targetQueryOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, queryRead, ref)
		})
		idleQuery, idleQueryOK := NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, queryRead, ref)
		})
		return baseOK && portOK && triggerCreated && seedMember != nil && triggerMember != nil && group != nil &&
			sourceQueryOK && targetQueryOK && idleQueryOK && admitQueryAt(assembly, source, sourceQuery) != nil && admitQueryAt(assembly, target, targetQuery) != nil && admitQueryAt(assembly, idle, idleQuery) != nil
	})
	targetReceipt, targetReceiptOK := targetQuery.Receipt()
	if !assembled || solver == nil || solver.runtime == nil || solver.runtime.graph == nil || !targetReceiptOK {
		t.Fatal("selected overlay assembly")
	}
	triggerMember, found := selectedOverlayTriggerMember(solver.runtime.graph, triggerKey.compositionKey())
	member, selected := solver.compiler.topology.SelectMember(triggerMember.Key(), equation.PairLocator{
		Application: coldKey(984_019).compositionKey(), Target: coldKey(984_015).compositionKey(), Endpoint: coldKey(984_016).compositionKey(),
	})
	narrowAccepted, narrowAcceptedOK := solver.compiler.topology.Accept(member, narrow.value)
	broadAccepted, broadAcceptedOK := solver.compiler.topology.Accept(member, broad.value)
	if !found || !selected || !narrowAcceptedOK || !broadAcceptedOK || !narrowAccepted.Available() || !broadAccepted.Available() {
		t.Fatal("selected overlay acceptance")
	}
	return selectedOverlayFixture{solver: solver, narrow: narrowAccepted, broad: broadAccepted, targetReceipt: targetReceipt, source: sourceSite.value, target: targetSite.value, idle: idleSite.value}
}

func selectedOverlayTriggerMember(graph *equation.Graph, rule composition.Key) (equation.RuleMember, bool) {
	if graph == nil || !rule.Available() {
		return equation.RuleMember{}, false
	}
	var result equation.RuleMember
	found := false
	for groupIndex := 0; groupIndex < graph.GroupCount(); groupIndex++ {
		group, groupOK := graph.HyperedgeAt(groupIndex)
		if !groupOK {
			return equation.RuleMember{}, false
		}
		for memberIndex := 0; memberIndex < group.MemberCount(); memberIndex++ {
			member, memberOK := group.MemberAt(memberIndex)
			if !memberOK || !graph.OwnsMember(member) {
				return equation.RuleMember{}, false
			}
			if member.Rule() != rule {
				continue
			}
			if found {
				return equation.RuleMember{}, false
			}
			result, found = member, true
		}
	}
	return result, found
}

func selectedOverlayPointIndex(runtime *solverRuntime, site equation.Site) (int, bool) {
	if runtime == nil || runtime.graph == nil || !site.Available() {
		return 0, false
	}
	for index := 0; index < runtime.graph.PointCount(); index++ {
		point, pointOK := runtime.graph.PointAt(schedule.Node(index))
		if !pointOK || !runtime.graph.OwnsPoint(point) {
			return 0, false
		}
		if point.Site().Same(site) {
			return index, true
		}
	}
	return 0, false
}

func selectedOverlayTargetValue(epoch *executorEpoch, target int) (uint64, bool) {
	if epoch == nil || epoch.runtime == nil || epoch.work == nil || target < 0 || target >= len(epoch.points) {
		return 0, false
	}
	for _, row := range epoch.runtime.queries {
		if row == nil {
			return 0, false
		}
		pointIndex, indexed := epoch.runtime.graph.PointIndex(row.query().Point())
		if !indexed || pointIndex != target {
			continue
		}
		result, materialized := row.materialize(epoch.work, epoch.points[pointIndex].State())
		if !materialized || result == nil {
			return 0, false
		}
		value, typed := result.value.(*typedFrozenValue[uint64])
		if !typed || value == nil {
			return 0, false
		}
		return value.value, true
	}
	return 0, false
}

func selectedOverlayEpoch(t *testing.T, runtime *solverRuntime, accepted []equation.AcceptedMember) *executorEpoch {
	t.Helper()
	epoch, opened := newRuntimeEpoch(runtime, accepted, context.Background())
	if !opened || epoch == nil || !epoch.run() {
		if epoch != nil {
			epoch.discard()
		}
		t.Fatal("selected overlay epoch")
	}
	return epoch
}

func TestSelectedFactorOverlayInstallsAndWakesTargetLaw(t *testing.T) {
	fixture := newSelectedOverlayFixture(t, false, false)
	runtime, _, compiled := fixture.solver.compiler.compile(nil)
	if !compiled || runtime == nil || !runtimeSelectedOverlayEligible(runtime) {
		t.Fatal("selected overlay base runtime")
	}
	target, indexed := selectedOverlayPointIndex(runtime, fixture.target)
	idle, idleIndexed := selectedOverlayPointIndex(runtime, fixture.idle)
	if !indexed || !idleIndexed || len(runtime.factorIncoming[idle]) == 0 {
		t.Fatal("selected overlay target")
	}
	untouchedIncoming := runtime.factorIncoming[idle]
	epoch := selectedOverlayEpoch(t, runtime, nil)
	defer epoch.discard()
	overlay, prepared := runtime.prepareSelectedFactorOverlay([]equation.AcceptedMember{fixture.narrow})
	if !prepared || overlay == nil || len(overlay.additions) != 1 || len(overlay.replacements) != 0 || !overlay.dependencyChanged {
		t.Fatalf("selected overlay preparation additions=%d replacements=%d dependency=%t prepared=%t", len(overlay.additions), len(overlay.replacements), overlay != nil && overlay.dependencyChanged, prepared)
	}
	if !epoch.installSelectedFactorOverlay(overlay) || !epoch.run() {
		t.Fatal("selected overlay installation")
	}
	if len(runtime.factorEdges) != overlay.previousEdgeCount+1 || len(runtime.factorIncoming[target]) != 1 || len(runtime.overlay.factorOutgoing) != runtime.graph.PointCount() {
		t.Fatalf("selected overlay rows edges=%d incoming=%d outgoing=%d", len(runtime.factorEdges), len(runtime.factorIncoming[target]), len(runtime.overlay.factorOutgoing))
	}
	if len(runtime.factorIncoming[idle]) != len(untouchedIncoming) || &runtime.factorIncoming[idle][0] != &untouchedIncoming[0] {
		t.Fatal("selected overlay replaced untouched incoming CSR row")
	}
	if value, readable := selectedOverlayTargetValue(epoch, target); !readable || value != 17 {
		t.Fatalf("selected overlay target=%d/%t", value, readable)
	}
}

func TestSelectedFactorOverlayRejectsCombinedCycleLaw(t *testing.T) {
	fixture := newSelectedOverlayFixture(t, true, true)
	runtime, _, compiled := fixture.solver.compiler.compile(nil)
	if !compiled || runtime == nil || !runtimeSelectedOverlayEligible(runtime) {
		t.Fatal("selected cycle base runtime")
	}
	edges, generation := len(runtime.factorEdges), runtime.overlay.generation
	if overlay, prepared := runtime.prepareSelectedFactorOverlay([]equation.AcceptedMember{fixture.narrow}); prepared || overlay != nil {
		t.Fatal("selected cyclic delta was installed")
	}
	if len(runtime.factorEdges) != edges || runtime.overlay.generation != generation {
		t.Fatalf("cycle preparation mutated runtime edges=%d/%d generation=%d/%d", len(runtime.factorEdges), edges, runtime.overlay.generation, generation)
	}
}

func TestSelectedFactorOverlayPremiseWideningMatchesColdCanonicalLaw(t *testing.T) {
	fixture := newSelectedOverlayFixture(t, false, false)
	runtime, _, compiled := fixture.solver.compiler.compile(nil)
	if !compiled || runtime == nil {
		t.Fatal("selected widening base runtime")
	}
	target, indexed := selectedOverlayPointIndex(runtime, fixture.target)
	if !indexed {
		t.Fatal("selected widening target")
	}
	epoch := selectedOverlayEpoch(t, runtime, nil)
	defer epoch.discard()
	first, prepared := runtime.prepareSelectedFactorOverlay([]equation.AcceptedMember{fixture.narrow})
	if !prepared || first == nil || len(first.additions) != 1 || !epoch.installSelectedFactorOverlay(first) || !epoch.run() {
		t.Fatal("selected widening first premise")
	}
	broadened, prepared := runtime.prepareSelectedFactorOverlay([]equation.AcceptedMember{fixture.broad})
	if !prepared || broadened == nil || len(broadened.additions) != 0 || len(broadened.replacements) != 1 {
		t.Fatalf("selected widening shape additions=%d replacements=%d prepared=%t", len(broadened.additions), len(broadened.replacements), prepared)
	}
	oldCount := len(runtime.factorEdges)
	if !epoch.installSelectedFactorOverlay(broadened) || !epoch.run() || len(runtime.factorEdges) != oldCount {
		t.Fatal("selected widening install")
	}
	live, readable := selectedOverlayTargetValue(epoch, target)
	if !readable || live != 17 {
		t.Fatalf("selected widening live=%d/%t", live, readable)
	}

	cold, _, compiled := fixture.solver.compiler.compile([]equation.AcceptedMember{fixture.broad})
	if !compiled || cold == nil {
		t.Fatal("selected widening cold compilation")
	}
	coldTarget, indexed := selectedOverlayPointIndex(cold, fixture.target)
	if !indexed {
		t.Fatal("selected widening cold target")
	}
	coldEpoch := selectedOverlayEpoch(t, cold, []equation.AcceptedMember{fixture.broad})
	defer coldEpoch.discard()
	coldValue, coldReadable := selectedOverlayTargetValue(coldEpoch, coldTarget)
	if !coldReadable || coldValue != live {
		t.Fatalf("selected widening cold=%d/%t live=%d", coldValue, coldReadable, live)
	}
}

func TestSolverResumesSettledEpochWithSelectedFactorOverlayLaw(t *testing.T) {
	fixture := newSelectedOverlayActivationFixture(t, false, false, true)
	initialRuntime := fixture.solver.runtime
	initialEdges := len(initialRuntime.factorEdges)
	state, status, report := fixture.solver.SolveWithReport(context.Background())
	value, readable := QueryResult(fixture.targetReceipt, state)
	if status != SolveComplete || state == nil || report.Available() || !readable || value != 17 {
		t.Fatalf("selected live solve status=%v state=%t report=%t value=%d/%t", status, state != nil, report.Available(), value, readable)
	}
	if fixture.solver.runtime != initialRuntime || fixture.solver.revision != 1 || len(fixture.solver.accepted) != 1 || initialRuntime.overlay.generation != 2 || len(initialRuntime.factorEdges) != initialEdges+1 {
		t.Fatalf("selected live ownership same=%t revision=%d accepted=%d generation=%d edges=%d/%d", fixture.solver.runtime == initialRuntime, fixture.solver.revision, len(fixture.solver.accepted), initialRuntime.overlay.generation, len(initialRuntime.factorEdges), initialEdges)
	}
}

func TestSolverFallsBackToColdRevisionWhenSelectedEdgeClosesCycleLaw(t *testing.T) {
	fixture := newSelectedOverlayActivationFixture(t, true, true, true)
	initialRuntime := fixture.solver.runtime
	state, status, report := fixture.solver.SolveWithReport(context.Background())
	value, readable := QueryResult(fixture.targetReceipt, state)
	if status != SolveComplete || state == nil || report.Available() || !readable || value != 17 {
		t.Fatalf("selected cycle fallback status=%v state=%t report=%t value=%d/%t", status, state != nil, report.Available(), value, readable)
	}
	if fixture.solver.runtime == initialRuntime || fixture.solver.revision != 1 || len(fixture.solver.accepted) != 1 || fixture.solver.runtime.graph.RegionCount() == 0 {
		t.Fatalf("selected cycle fallback same=%t revision=%d accepted=%d regions=%d", fixture.solver.runtime == initialRuntime, fixture.solver.revision, len(fixture.solver.accepted), fixture.solver.runtime.graph.RegionCount())
	}
}
