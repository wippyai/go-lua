package callsite

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/call"
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	"github.com/wippyai/go-lua/analysis/domain/effect/factor"
	effectowner "github.com/wippyai/go-lua/analysis/domain/effect/owner"
	"github.com/wippyai/go-lua/analysis/domain/pack"
	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

type bodyCallLawFixture struct {
	program      *program.Program
	linked       *link.Link
	contract     *target.Contract
	callsAlg     *call.Algebra
	effectsAlg   *factor.Algebra
	calls        *callowner.Owner
	effects      *effectowner.Owner
	composition  *engine.Composition
	rule         *BodyCallRule
	callSeed     *engine.Rule[call.Value, callsiteTestOperand]
	effectSeed   *engine.Rule[factor.Value, callsiteTestOperand]
	callWrite    engine.Write[call.Value]
	effectWrite  engine.Write[factor.Value]
	query        *engine.Query[uint64]
	queryRead    engine.QueryRead[engine.OrderedCells[factor.Value]]
	application  linkproject.Application
	operand      bodyCallOperand
	summaries    []factor.Value
	summaryRoots []factor.Root
	summaryIndex int
	bodyTargets  []call.Target
	external     call.Target
}

func newBodyCallLawFixture(t testing.TB) bodyCallLawFixture {
	t.Helper()
	programValue, err := lower.Lower(lower.Source{Name: "body_call_law.lua", Text: []byte(`
local function sink(value) return value end
local function first()
  sink(1)
  return 1
end
local function second()
  sink(2)
  return 2
end
local function recursive()
  sink(3)
  recursive()
end
first()
second()
recursive()
`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(func() *target.Spec {
		spec := callsiteTestSpec(callsiteTestMode{ordinaryEffects: true})
		return &spec
	}())
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "body_call_law", Program: programValue}}})
	if err != nil {
		t.Fatal(err)
	}
	types, ok := typeauthority.Seal(linked)
	if !ok {
		t.Fatal("body-call type authority")
	}
	statics, _, err := staticdomain.Seal(linked, types)
	if err != nil {
		t.Fatal(err)
	}
	packs, ok := pack.Seal(linked, statics)
	if !ok {
		t.Fatal("body-call Pack")
	}
	callsAlg, ok := call.New(linked)
	if !ok {
		t.Fatal("body-call Call algebra")
	}
	effectsAlg, ok := factor.New(linked, packs, contract)
	if !ok {
		t.Fatal("body-call Effect algebra")
	}
	bodies := callsAlg.Bodies()
	if bodies.Count() < 2 || bodies.Count() >= effectsAlg.RootCount() {
		t.Fatalf("body-only cursor=%d Effect roots=%d", bodies.Count(), effectsAlg.RootCount())
	}
	applications := linked.Project().Applications().Calls()
	var caller linkproject.Application
	for index := 0; index < applications.Count(); index++ {
		application, present := applications.At(index)
		root, rooted := effectsAlg.RootForCall(application)
		rootIndex, indexed := effectsAlg.RootIndex(root)
		if !present || !rooted || !indexed {
			t.Fatalf("body-call application %d", index)
		}
		if rootIndex == 0 && caller == (linkproject.Application{}) {
			caller = application
		}
	}
	if caller == (linkproject.Application{}) {
		t.Fatal("body-call top-level caller")
	}
	callerRoot, ok := effectsAlg.RootForCall(caller)
	if !ok {
		t.Fatal("body-call caller root")
	}
	operand, ok := newBodyCallOperand(effectsAlg, callsAlg, callerRoot, caller)
	if !ok {
		t.Fatal("body-call operand")
	}

	owner, ok := contract.Lookup(target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"sink"}})
	if !ok {
		t.Fatal("body-call sink operation")
	}
	summaries := make([]factor.Value, bodies.Count())
	summaryRoots := make([]factor.Root, bodies.Count())
	summaryIndex := -1
	for index := 0; index < bodies.Count(); index++ {
		body, present := bodies.At(index)
		shard, term, resolved := callsAlg.ResolveBody(body)
		root, rooted := effectsAlg.RootForBody(shard, term)
		if !present || !resolved || !rooted {
			t.Fatalf("body-call summary root %d", index)
		}
		summaryRoots[index] = root
		var atom factor.Atom
		var found bool
		for applicationIndex := 0; applicationIndex < applications.Count(); applicationIndex++ {
			application, available := applications.At(applicationIndex)
			applicationRoot, applicationRootOK := effectsAlg.RootForCall(application)
			if !available || !applicationRootOK || applicationRoot != root {
				continue
			}
			if candidate, issued := effectsAlg.CallEffectAtom(root, application, owner, 0); issued {
				atom, found = candidate, true
				break
			}
		}
		if !found {
			summaries[index] = effectsAlg.Bottom()
			continue
		}
		summaries[index], ok = effectsAlg.Singleton(atom)
		if !ok {
			t.Fatalf("body-call summary value %d", index)
		}
		if summaryIndex < 0 {
			summaryIndex = index
		}
	}
	if summaryIndex < 0 {
		t.Fatal("body-call fixture has no concrete body summary")
	}

	bodyTargets := make([]call.Target, bodies.Count())
	for index := 0; index < bodies.Count(); index++ {
		body, _ := bodies.At(index)
		for targetIndex := 0; targetIndex < callsAlg.SupportCount(operand.key); targetIndex++ {
			candidate, available := callsAlg.SupportTargetAt(operand.key, targetIndex)
			projected, bodyOK := candidate.Body()
			if available && bodyOK && projected.Same(body) {
				bodyTargets[index] = candidate
				break
			}
		}
		if !bodyTargets[index].Valid() {
			t.Fatalf("body-call target %d", index)
		}
	}
	seed, ok := linked.Boundary().Seeds().ForOperation(owner)
	if !ok {
		t.Fatal("body-call external seed")
	}
	external, ok := callsAlg.TargetForSeed(seed)
	if !ok {
		t.Fatal("body-call external target")
	}
	known, ok := callsAlg.DispatchValue(operand.key, []call.Target{bodyTargets[summaryIndex]}, false)
	if !ok {
		t.Fatal("body-call known source value")
	}

	composition := engine.NewComposition()
	calls, ok := callowner.Declare(composition, callsiteTestKey(100), callsAlg)
	if !ok {
		t.Fatal("body-call Call owner")
	}
	effects, ok := effectowner.Declare(composition, callsiteTestKey(101), callsiteTestKey(102), effectsAlg)
	if !ok {
		t.Fatal("body-call Effect owner")
	}
	rule, ok := DeclareBody(composition, callsiteTestKey(103), callsiteTestKey(104), callsiteTestKey(105), calls, effects)
	if !ok || rule == nil {
		t.Fatal("body-call rule declaration")
	}
	var callWrite engine.Write[call.Value]
	callSeed, ok := engine.DeclareRule(composition, engine.RuleSpec[call.Value, callsiteTestOperand]{
		Semantic: callsiteTestKey(106), OperandFamily: callsiteTestKey(107), OperandContent: callsiteTestOperandContent,
		Output: calls.Output(), Inputs: 0, Admission: engine.AdmitRuleByTrustedTheorem[call.Value, callsiteTestOperand](callsiteTestKey(108)),
		Transfer: func(access engine.Access[call.Value, callsiteTestOperand]) bool {
			return engine.Product(access, func(row engine.Row) bool { return engine.StageValue(access, row, known) })
		},
	}, func(rule *engine.Rule[call.Value, callsiteTestOperand]) bool {
		var declared bool
		callWrite, declared = engine.WriteTo(rule, calls.ExactWrite())
		return declared
	})
	if !ok || callSeed == nil {
		t.Fatal("body-call Call seed")
	}
	var effectWrite engine.Write[factor.Value]
	effectSeed, ok := engine.DeclareRule(composition, engine.RuleSpec[factor.Value, callsiteTestOperand]{
		Semantic: callsiteTestKey(109), OperandFamily: callsiteTestKey(110), OperandContent: callsiteTestOperandContent,
		Output: effects.Output(), Inputs: 0, Admission: engine.AdmitRuleByTrustedTheorem[factor.Value, callsiteTestOperand](callsiteTestKey(111)),
		Transfer: func(access engine.Access[factor.Value, callsiteTestOperand]) bool {
			return engine.Product(access, func(row engine.Row) bool { return engine.StageValue(access, row, summaries[summaryIndex]) })
		},
	}, func(rule *engine.Rule[factor.Value, callsiteTestOperand]) bool {
		var declared bool
		effectWrite, declared = engine.WriteTo(rule, effects.ExactWrite())
		return declared
	})
	if !ok || effectSeed == nil {
		t.Fatal("body-call Effect seed")
	}
	var queryRead engine.QueryRead[engine.OrderedCells[factor.Value]]
	query, ok := engine.DeclareQuery(composition, engine.QuerySpec[uint64]{
		Semantic: callsiteTestKey(112),
		Project: func(observation engine.Observation) uint64 {
			var atoms uint64
			rows := 0
			if !engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				cells, readable := engine.QueryValue(row, queryRead)
				if !readable || cells.Count() != 1 {
					return false
				}
				value, present, available := cells.At(0)
				if !available {
					return false
				}
				if present {
					for index := 0; ; index++ {
						if _, exists := effectsAlg.AtomAt(value, index); !exists {
							break
						}
						atoms++
					}
				}
				rows++
				return true
			}) || rows != 1 {
				return 0
			}
			return atoms
		},
		Result: engine.FrozenResult[uint64]{
			Semantic: callsiteTestKey(113), Freeze: func(value uint64) uint64 { return value }, Clone: func(value uint64) uint64 { return value },
			Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value },
		},
	}, func(query *engine.Query[uint64]) bool {
		var declared bool
		queryRead, declared = engine.QueryReadFrom(query, effects.ExactRead())
		return declared
	})
	if !ok || query == nil || !composition.Seal() {
		t.Fatal("body-call composition")
	}
	return bodyCallLawFixture{
		program: programValue, linked: linked, contract: contract, callsAlg: callsAlg, effectsAlg: effectsAlg,
		calls: calls, effects: effects, composition: composition, rule: rule, application: caller, operand: operand,
		callSeed: callSeed, effectSeed: effectSeed, callWrite: callWrite, effectWrite: effectWrite, query: query, queryRead: queryRead,
		summaries: summaries, summaryRoots: summaryRoots, summaryIndex: summaryIndex, bodyTargets: bodyTargets, external: external,
	}
}

func TestDeclareBodyUsesOneDistinctRuleFamilyAndRejectsIdentityCollisions(t *testing.T) {
	fixture := newBodyCallLawFixture(t)
	declare := func(semantic, family, evidence engine.SemanticKey) bool {
		composition := engine.NewComposition()
		calls, callsOK := callowner.Declare(composition, callsiteTestKey(160), fixture.callsAlg)
		effects, effectsOK := effectowner.Declare(composition, callsiteTestKey(161), callsiteTestKey(162), fixture.effectsAlg)
		if !callsOK || !effectsOK {
			return false
		}
		body, bodyOK := DeclareBody(composition, semantic, family, evidence, calls, effects)
		return bodyOK && body != nil
	}
	if !declare(callsiteTestKey(163), callsiteTestKey(164), callsiteTestKey(165)) {
		t.Fatal("valid BodyCall declaration rejected")
	}
	if declare(callsiteTestKey(166), callsiteTestKey(166), callsiteTestKey(167)) {
		t.Fatal("BodyCall accepted semantic/family collision")
	}
	if declare(callsiteTestKey(168), callsiteTestKey(169), callsiteTestKey(168)) {
		t.Fatal("BodyCall accepted semantic/evidence collision")
	}
	if declare(callsiteTestKey(170), callsiteTestKey(171), callsiteTestKey(171)) {
		t.Fatal("BodyCall accepted family/evidence collision")
	}
}

func TestBodyCallUsesTargetScopedSelector(t *testing.T) {
	fixture := newBodyCallLawFixture(t)
	first, firstOK := fixture.rule.Instance(fixture.application)
	second, secondOK := fixture.rule.Instance(fixture.application)
	if !firstOK || !secondOK || first == nil || second == nil || first == second {
		t.Fatal("body-call instance")
	}
	if fixture.rule.bodies.Count() == 0 || fixture.rule.bodies.Count() >= fixture.effectsAlg.RootCount() {
		t.Fatal("body-call selector lost its body denominator")
	}
}

func TestBodyCallWithNoFunctionBodiesIssuesNoInstance(t *testing.T) {
	programValue, err := lower.Lower(lower.Source{Name: "body_call_empty.lua", Text: []byte(`sink()`)})
	if err != nil {
		t.Fatal(err)
	}
	spec := callsiteTestSpec(callsiteTestMode{ordinaryEffects: true})
	sink := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"sink"}}
	stringKey := func(value string) keyspace.LiteralValue {
		return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}
	}
	spec.InitialRoots = []target.InitialRootSpec{{Identity: "GlobalEnvRoot", Shape: target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}}}}
	spec.InitialEntries = []target.InitialEntrySpec{
		{Root: "GlobalEnvRoot", Key: stringKey("_G"), Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable},
		{Root: "GlobalEnvRoot", Key: stringKey("sink"), Value: target.InitialValueSpec{Kind: target.InitialValueOperation, Operation: sink}, Mutability: target.InitialMutable},
		{Root: "GlobalEnvRoot", Key: stringKey("__link_absent"), Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
	}
	spec.InitialBindings = []target.InitialBindingSpec{{Name: "_G", Root: "GlobalEnvRoot", Key: stringKey("_G")}, {Name: "sink", Root: "GlobalEnvRoot", Key: stringKey("sink")}, {Name: "__link_absent", Root: "GlobalEnvRoot", Key: stringKey("__link_absent")}}
	contract, err := target.Seal(&spec)
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "body_call_empty", Program: programValue}}})
	if err != nil {
		t.Fatal(err)
	}
	types, ok := typeauthority.Seal(linked)
	if !ok {
		t.Fatal("empty body-call type authority")
	}
	statics, _, err := staticdomain.Seal(linked, types)
	if err != nil {
		t.Fatal(err)
	}
	packs, ok := pack.Seal(linked, statics)
	if !ok {
		t.Fatal("empty body-call Pack")
	}
	callsAlg, callsOK := call.New(linked)
	effectsAlg, effectsOK := factor.New(linked, packs, contract)
	composition := engine.NewComposition()
	calls, callsOwnerOK := callowner.Declare(composition, callsiteTestKey(130), callsAlg)
	effects, effectsOwnerOK := effectowner.Declare(composition, callsiteTestKey(131), callsiteTestKey(132), effectsAlg)
	rule, ruleOK := DeclareBody(composition, callsiteTestKey(133), callsiteTestKey(134), callsiteTestKey(135), calls, effects)
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[uint64]{
		Semantic: callsiteTestKey(136), Project: func(engine.Observation) uint64 { return 0 },
		Result: engine.FrozenResult[uint64]{
			Semantic: callsiteTestKey(137), Freeze: func(value uint64) uint64 { return value }, Clone: func(value uint64) uint64 { return value },
			Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value },
		},
	}, func(query *engine.Query[uint64]) bool {
		_, declared := engine.QueryReadFrom(query, effects.ExactRead())
		return declared
	})
	applications := linked.Project().Applications().Calls()
	application, applicationOK := applications.At(0)
	sealed := composition.Seal()
	if !callsOK || !effectsOK || !callsOwnerOK || !effectsOwnerOK || !ruleOK || rule == nil || !queryOK || query == nil || !applicationOK || callsAlg.Bodies().Count() != 0 || !sealed {
		t.Fatalf("empty body-call declaration: calls=%t effects=%t callOwner=%t effectOwner=%t rule=%t query=%t application=%t bodies=%d sealed=%t", callsOK, effectsOK, callsOwnerOK, effectsOwnerOK, ruleOK, queryOK, applicationOK, callsAlg.Bodies().Count(), sealed)
	}
	if instance, ok := rule.Instance(application); ok || instance != nil {
		t.Fatal("empty body cursor issued a body-call instance")
	}
}

func TestBodyCallSelectorKnownOpenExternalAndTopSemantics(t *testing.T) {
	fixture := newBodyCallLawFixture(t)
	known, ok := fixture.callsAlg.DispatchValue(fixture.operand.key, []call.Target{fixture.bodyTargets[fixture.summaryIndex]}, false)
	if !ok {
		t.Fatal("known body Call")
	}
	open, ok := fixture.callsAlg.DispatchValue(fixture.operand.key, []call.Target{fixture.bodyTargets[fixture.summaryIndex]}, true)
	if !ok || !open.HasOpaqueAlternative() {
		t.Fatal("open body Call")
	}
	external, ok := fixture.callsAlg.DispatchValue(fixture.operand.key, []call.Target{fixture.external}, false)
	if !ok {
		t.Fatal("external Call")
	}

	knownRoutes, knownOK := fixture.rule.expectedRoutes(known)
	openRoutes, openOK := fixture.rule.expectedRoutes(open)
	externalRoutes, externalOK := fixture.rule.expectedRoutes(external)
	topRoutes, topOK := fixture.rule.expectedRoutes(fixture.callsAlg.Top())
	if !knownOK || !openOK || !externalOK || !topOK {
		t.Fatal("body-call selector projection")
	}
	if len(knownRoutes) != 1 || len(openRoutes) != 1 || len(externalRoutes) != 0 || len(topRoutes) != fixture.rule.bodies.Count() {
		t.Fatalf("selector route counts: known=%d open=%d external=%d top=%d bodies=%d", len(knownRoutes), len(openRoutes), len(externalRoutes), len(topRoutes), fixture.rule.bodies.Count())
	}
	if knownRoutes[0].root != fixture.summaryRoots[fixture.summaryIndex] || openRoutes[0].root != knownRoutes[0].root {
		t.Fatal("selector did not preserve the explicit body target")
	}
	seen := make(map[uint64]bool, len(topRoutes))
	for _, route := range topRoutes {
		if seen[route.tag] {
			t.Fatal("Call Top selector duplicated a body route")
		}
		seen[route.tag] = true
		if _, ok := fixture.effects.Locate(route.root); !ok {
			t.Fatal("Call Top selector issued a foreign Effect root")
		}
	}
}

func TestBodyCallSelectorRejectsForeignAndZeroBodyCapabilities(t *testing.T) {
	fixture := newBodyCallLawFixture(t)
	foreignCalls, ok := call.New(fixture.linked)
	if !ok {
		t.Fatal("foreign Call algebra")
	}
	foreignBody, bodyOK := foreignCalls.Bodies().At(0)
	if !bodyOK {
		t.Fatal("foreign body capability")
	}
	if _, routed := fixture.rule.routeForBody(foreignBody); routed {
		t.Fatal("BodyCall accepted a same-Link foreign body")
	}
	if _, routed := fixture.rule.routeForBody(call.Body{}); routed {
		t.Fatal("BodyCall accepted a zero body capability")
	}
	if _, routed := fixture.rule.routeAt(-1); routed {
		t.Fatal("BodyCall accepted a negative body coordinate")
	}
	if _, routed := fixture.rule.routeAt(fixture.rule.bodies.Count()); routed {
		t.Fatal("BodyCall accepted a body coordinate at the cursor end")
	}
}

func TestBodyCallColdEvidenceTransportsExactBodySummary(t *testing.T) {
	fixture := newBodyCallLawFixture(t)
	callRef, callRefOK := fixture.calls.Locate(fixture.operand.key)
	summaryRef, summaryRefOK := fixture.effects.Locate(fixture.summaryRoots[fixture.summaryIndex])
	outputRef, outputRefOK := fixture.effects.Locate(fixture.operand.root)
	if !callRefOK || !summaryRefOK || !outputRefOK {
		t.Fatal("body-call cold refs")
	}
	callOperand := callsiteTestOperand{digest: callsiteTestKey(114).Digest()}
	effectOperand := callsiteTestOperand{digest: callsiteTestKey(115).Digest()}
	callInstance, callInstanceOK := engine.NewRuleInstance(fixture.callSeed, callOperand, func(binding *engine.RuleBinding[call.Value, callsiteTestOperand]) bool {
		return engine.InstanceWrite(binding, fixture.callWrite, callRef)
	})
	effectInstance, effectInstanceOK := engine.NewRuleInstance(fixture.effectSeed, effectOperand, func(binding *engine.RuleBinding[factor.Value, callsiteTestOperand]) bool {
		return engine.InstanceWrite(binding, fixture.effectWrite, summaryRef)
	})
	bodyInstance, bodyInstanceOK := fixture.rule.Instance(fixture.application)
	var secondApplication linkproject.Application
	applications := fixture.linked.Project().Applications().Calls()
	for index := 0; index < applications.Count(); index++ {
		application, available := applications.At(index)
		root, rooted := fixture.effectsAlg.RootForCall(application)
		if available && rooted && root != fixture.operand.root {
			secondApplication = application
			break
		}
	}
	secondBodyInstance, secondBodyInstanceOK := fixture.rule.Instance(secondApplication)
	if !callInstanceOK || !effectInstanceOK || !bodyInstanceOK || secondApplication == (linkproject.Application{}) || !secondBodyInstanceOK {
		t.Fatal("body-call cold instances")
	}

	source := engine.NewSourceAssembly(fixture.composition)
	if source == nil {
		t.Fatal("body-call source assembly")
	}
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	falsity, falsityOK := source.FalseExpr()
	sourceSite, sourceSiteOK := source.Site(callsiteTestKey(116), scope, truth, true)
	targetSite, targetSiteOK := source.Site(callsiteTestKey(117), scope, falsity, false)
	callOccurrence, callOccurrenceOK := source.At(sourceSite)
	effectOccurrence, effectOccurrenceOK := source.Relation(sourceSite, callsiteTestKey(118))
	bodyOccurrence, bodyOccurrenceOK := source.Relation(targetSite, callsiteTestKey(119))
	secondBodyOccurrence, secondBodyOccurrenceOK := source.Relation(targetSite, callsiteTestKey(121))
	callPrepared, callPreparedOK := source.PrepareInstance(callOccurrence, callInstance)
	effectPrepared, effectPreparedOK := source.PrepareInstance(effectOccurrence, effectInstance)
	bodyPrepared, bodyPreparedOK := source.PrepareInstance(bodyOccurrence, bodyInstance)
	secondBodyPrepared, secondBodyPreparedOK := source.PrepareInstance(secondBodyOccurrence, secondBodyInstance)
	reindex, reindexOK := source.IdentityReindex(scope)
	boundary, boundaryOK := source.Boundary(sourceSite, targetSite, callsiteTestKey(120), truth, reindex, truth)
	if !scopeOK || !truthOK || !falsityOK || !sourceSiteOK || !targetSiteOK || !callOccurrenceOK || !effectOccurrenceOK || !bodyOccurrenceOK || !secondBodyOccurrenceOK ||
		!callPreparedOK || !effectPreparedOK || !bodyPreparedOK || !secondBodyPreparedOK || !reindexOK || !boundaryOK || !source.Seal() {
		t.Fatal("body-call source schema")
	}

	var queryInstance *engine.QueryInstance[uint64]
	assembled := false
	solver, compiled := source.Assemble(func(assembly *engine.Assembly) bool {
		sourcePoint, sourcePointOK := assembly.Point(sourceSite)
		targetPoint, targetPointOK := assembly.Point(targetSite)
		callMember, callMemberOK := assembly.Member(sourcePoint, callPrepared)
		effectMember, effectMemberOK := assembly.Member(sourcePoint, effectPrepared)
		bodyMember, bodyMemberOK := assembly.Member(targetPoint, bodyPrepared)
		secondBodyMember, secondBodyMemberOK := assembly.Member(targetPoint, secondBodyPrepared)
		sourceGroup, sourceGroupOK := assembly.Group(sourcePoint, callMember, effectMember)
		targetGroup, targetGroupOK := assembly.Group(targetPoint, bodyMember)
		secondTargetGroup, secondTargetGroupOK := assembly.Group(targetPoint, secondBodyMember)
		var queryOK bool
		queryInstance, queryOK = engine.NewQueryInstance(fixture.query, func(binding *engine.QueryBinding[uint64]) bool {
			return engine.InstanceQueryRead(binding, fixture.queryRead, outputRef)
		})
		_, queryAttached := assembly.Query(targetPoint, queryInstance)
		boundaryAttached := assembly.Boundary(targetGroup, boundary)
		secondBoundaryAttached := assembly.Boundary(secondTargetGroup, boundary)
		assembled = sourcePointOK && targetPointOK && callMemberOK && effectMemberOK && bodyMemberOK && secondBodyMemberOK && sourceGroupOK && targetGroupOK && secondTargetGroupOK && sourceGroup.Available() && queryOK && queryAttached && boundaryAttached && secondBoundaryAttached
		return assembled
	})
	if !compiled || solver == nil || !assembled {
		t.Fatal("body-call cold assembly")
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	atoms, readable := engine.QueryResult(receipt, state)
	if status != engine.SolveComplete || state == nil || !receiptOK || !readable || atoms != 1 {
		t.Fatalf("body-call cold solve: status=%v receipt=%t readable=%t atoms=%d", status, receiptOK, readable, atoms)
	}
}

func TestBodyCallRecursiveSelfSCCConvergesInEngineWTO(t *testing.T) {
	fixture := newBodyCallLawFixture(t)
	recursiveIndex := -1
	for index := len(fixture.summaries) - 1; index >= 0; index-- {
		if !fixture.effectsAlg.Equal(fixture.summaries[index], fixture.effectsAlg.Bottom()) {
			recursiveIndex = index
			break
		}
	}
	if recursiveIndex < 0 {
		t.Fatal("recursive body summary")
	}
	recursiveRoot := fixture.summaryRoots[recursiveIndex]
	applications := fixture.linked.Project().Applications().Calls()
	var recursiveCall linkproject.Application
	for index := 0; index < applications.Count(); index++ {
		application, available := applications.At(index)
		root, rooted := fixture.effectsAlg.RootForCall(application)
		if available && rooted && root == recursiveRoot {
			recursiveCall = application
		}
	}
	if recursiveCall == (linkproject.Application{}) {
		t.Fatal("recursive body application")
	}
	recursiveKey, keyOK := fixture.callsAlg.KeyForApplication(recursiveCall)
	recursiveValue, valueOK := fixture.callsAlg.DispatchValue(recursiveKey, []call.Target{fixture.bodyTargets[recursiveIndex]}, false)
	if !keyOK || !valueOK {
		t.Fatal("recursive Call value")
	}

	composition := engine.NewComposition()
	calls, callsOK := callowner.Declare(composition, callsiteTestKey(140), fixture.callsAlg)
	effects, effectsOK := effectowner.Declare(composition, callsiteTestKey(141), callsiteTestKey(142), fixture.effectsAlg)
	bodyRule, bodyRuleOK := DeclareBody(composition, callsiteTestKey(143), callsiteTestKey(144), callsiteTestKey(145), calls, effects)
	var callWrite engine.Write[call.Value]
	callSeed, callSeedOK := engine.DeclareRule(composition, engine.RuleSpec[call.Value, callsiteTestOperand]{
		Semantic: callsiteTestKey(146), OperandFamily: callsiteTestKey(147), OperandContent: callsiteTestOperandContent,
		Output: calls.Output(), Inputs: 0, Admission: engine.AdmitRuleByTrustedTheorem[call.Value, callsiteTestOperand](callsiteTestKey(148)),
		Transfer: func(access engine.Access[call.Value, callsiteTestOperand]) bool {
			return engine.Product(access, func(row engine.Row) bool { return engine.StageValue(access, row, recursiveValue) })
		},
	}, func(rule *engine.Rule[call.Value, callsiteTestOperand]) bool {
		var declared bool
		callWrite, declared = engine.WriteTo(rule, calls.ExactWrite())
		return declared
	})
	var effectWrite engine.Write[factor.Value]
	effectSeed, effectSeedOK := engine.DeclareRule(composition, engine.RuleSpec[factor.Value, callsiteTestOperand]{
		Semantic: callsiteTestKey(149), OperandFamily: callsiteTestKey(150), OperandContent: callsiteTestOperandContent,
		Output: effects.Output(), Inputs: 0, Admission: engine.AdmitRuleByTrustedTheorem[factor.Value, callsiteTestOperand](callsiteTestKey(151)),
		Transfer: func(access engine.Access[factor.Value, callsiteTestOperand]) bool {
			return engine.Product(access, func(row engine.Row) bool { return engine.StageValue(access, row, fixture.summaries[recursiveIndex]) })
		},
	}, func(rule *engine.Rule[factor.Value, callsiteTestOperand]) bool {
		var declared bool
		effectWrite, declared = engine.WriteTo(rule, effects.ExactWrite())
		return declared
	})
	var queryRead engine.QueryRead[engine.OrderedCells[factor.Value]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[uint64]{
		Semantic: callsiteTestKey(152),
		Project: func(observation engine.Observation) uint64 {
			var result uint64
			if !engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				cells, readable := engine.QueryValue(row, queryRead)
				value, present, available := cells.At(0)
				if !readable || cells.Count() != 1 || !available || !present {
					return false
				}
				for index := 0; ; index++ {
					if _, exists := fixture.effectsAlg.AtomAt(value, index); !exists {
						break
					}
					result++
				}
				return true
			}) {
				return 0
			}
			return result
		},
		Result: engine.FrozenResult[uint64]{
			Semantic: callsiteTestKey(153), Freeze: func(value uint64) uint64 { return value }, Clone: func(value uint64) uint64 { return value },
			Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value },
		},
	}, func(query *engine.Query[uint64]) bool {
		var declared bool
		queryRead, declared = engine.QueryReadFrom(query, effects.ExactRead())
		return declared
	})
	if !callsOK || !effectsOK || !bodyRuleOK || bodyRule == nil || !callSeedOK || callSeed == nil || !effectSeedOK || effectSeed == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("recursive body composition")
	}
	callRef, callRefOK := calls.Locate(recursiveKey)
	effectRef, effectRefOK := effects.Locate(recursiveRoot)
	callOperand := callsiteTestOperand{digest: callsiteTestKey(154).Digest()}
	effectOperand := callsiteTestOperand{digest: callsiteTestKey(155).Digest()}
	callInstance, callInstanceOK := engine.NewRuleInstance(callSeed, callOperand, func(binding *engine.RuleBinding[call.Value, callsiteTestOperand]) bool {
		return engine.InstanceWrite(binding, callWrite, callRef)
	})
	effectInstance, effectInstanceOK := engine.NewRuleInstance(effectSeed, effectOperand, func(binding *engine.RuleBinding[factor.Value, callsiteTestOperand]) bool {
		return engine.InstanceWrite(binding, effectWrite, effectRef)
	})
	bodyInstance, bodyInstanceOK := bodyRule.Instance(recursiveCall)
	if !callRefOK || !effectRefOK || !callInstanceOK || !effectInstanceOK || !bodyInstanceOK {
		t.Fatal("recursive body instances")
	}

	source := engine.NewSourceAssembly(composition)
	if source == nil {
		t.Fatal("recursive body source assembly")
	}
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	site, siteOK := source.Site(callsiteTestKey(156), scope, truth, true)
	callOccurrence, callOccurrenceOK := source.At(site)
	effectOccurrence, effectOccurrenceOK := source.Relation(site, callsiteTestKey(157))
	bodyOccurrence, bodyOccurrenceOK := source.Relation(site, callsiteTestKey(158))
	callPrepared, callPreparedOK := source.PrepareInstance(callOccurrence, callInstance)
	effectPrepared, effectPreparedOK := source.PrepareInstance(effectOccurrence, effectInstance)
	bodyPrepared, bodyPreparedOK := source.PrepareInstance(bodyOccurrence, bodyInstance)
	reindex, reindexOK := source.IdentityReindex(scope)
	boundary, boundaryOK := source.Boundary(site, site, callsiteTestKey(159), truth, reindex, truth)
	if !scopeOK || !truthOK || !siteOK || !callOccurrenceOK || !effectOccurrenceOK || !bodyOccurrenceOK || !callPreparedOK || !effectPreparedOK || !bodyPreparedOK || !reindexOK || !boundaryOK || !source.Seal() {
		t.Fatal("recursive body source")
	}
	var queryInstance *engine.QueryInstance[uint64]
	assembled := false
	solver, compiled := source.Assemble(func(assembly *engine.Assembly) bool {
		point, pointOK := assembly.Point(site)
		callMember, callMemberOK := assembly.Member(point, callPrepared)
		effectMember, effectMemberOK := assembly.Member(point, effectPrepared)
		bodyMember, bodyMemberOK := assembly.Member(point, bodyPrepared)
		sourceGroup, sourceGroupOK := assembly.Group(point, callMember, effectMember)
		targetGroup, targetGroupOK := assembly.Group(point, bodyMember)
		var instanceOK bool
		queryInstance, instanceOK = engine.NewQueryInstance(query, func(binding *engine.QueryBinding[uint64]) bool {
			return engine.InstanceQueryRead(binding, queryRead, effectRef)
		})
		_, queryAttached := assembly.Query(point, queryInstance)
		boundaryAttached := assembly.Boundary(targetGroup, boundary)
		assembled = pointOK && callMemberOK && effectMemberOK && bodyMemberOK && sourceGroupOK && targetGroupOK && sourceGroup.Available() && instanceOK && queryAttached && boundaryAttached
		return assembled
	})
	if !compiled || solver == nil || !assembled {
		t.Fatal("recursive body assembly")
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	atoms, readable := engine.QueryResult(receipt, state)
	if status != engine.SolveComplete || state == nil || !receiptOK || !readable || atoms != 1 {
		t.Fatalf("recursive body solve: status=%v receipt=%t readable=%t atoms=%d", status, receiptOK, readable, atoms)
	}
}
