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
	"github.com/wippyai/go-lua/analysis/type/typ"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/link"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

// These tests deliberately publish a Call through the real engine source
// facade. The source rule is only a fixture seed; callsite.Rule.Instance is
// the rule under test and its output must pass Solver's local admission.
type callsiteTestOperand struct{ digest [32]byte }

func callsiteTestKey(value byte) engine.SemanticKey {
	var digest [32]byte
	digest[31] = value
	key, ok := engine.NewSemanticKey(digest, 0xCA11)
	if !ok {
		panic("callsite test semantic key")
	}
	return key
}

func callsiteTestOperandContent(value callsiteTestOperand) (callsiteTestOperand, [32]byte, bool) {
	return value, value.digest, value.digest != [32]byte{}
}

type callsiteTestMode struct {
	ordinaryEffects bool
	callback        bool
	callbackOpen    bool
	ownerTail       target.RowTail
	rowArgs         bool
	rowVariable     bool
	opaqueRule      bool
	callOpaque      bool
	callTop         bool
	selectOpaque    bool
	endpointAliases int
	tamperMode      uint8
}

type callsiteTestFixture struct {
	contract    *target.Contract
	linked      *link.Link
	packs       *pack.Schema
	callsAlg    *call.Algebra
	effectsAlg  *factor.Algebra
	calls       *callowner.Owner
	effects     *effectowner.Owner
	composition *engine.Composition
	callRule    *engine.Rule[call.Value, callsiteTestOperand]
	effectSeed  *engine.Rule[factor.Value, callsiteTestOperand]
	callsite    *Rule
	callWrite   engine.Write[call.Value]
	effectWrite engine.Write[factor.Value]
	query       *engine.Query[uint64]
	queryRead   engine.QueryRead[engine.OrderedCells[factor.Value]]
	application linkproject.Application
	root        factor.Root
	owner       target.Operation
	key         call.Key
	value       call.Value
}

func callsiteTestSpec(mode callsiteTestMode) target.Spec {
	ownerTail := mode.ownerTail
	if ownerTail == 0 {
		ownerTail = target.RowClosed
	}
	ownerRowFormals := uint32(0)
	var ownerRowVar target.RowVar
	if mode.rowVariable || mode.rowArgs {
		ownerTail = target.RowVariable
		ownerRowFormals = 1
		ownerRowVar = 0
	}

	ordinary := make([]target.EffectSpec, 0, 1)
	if mode.ordinaryEffects {
		effect := target.EffectSpec{Target: 2, ValueArgs: []target.ValueFormal{0}}
		if mode.rowArgs {
			effect.RowArgs = []target.RowVar{0}
		}
		ordinary = append(ordinary, effect)
	}

	bindings := []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"sink"}}}
	if mode.endpointAliases != 0 {
		bindings = append(bindings, target.BindingSpec{Namespace: target.BindingProvider, Owner: []string{"test"}, Member: []string{"sink"}})
	}
	owner := target.OperationSpec{
		Bindings:   bindings,
		Input:      target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesClosed},
		Outcomes:   []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
		RowFormals: ownerRowFormals,
		Effects:    target.RowSpec{Occurrences: ordinary, Tail: ownerTail, Var: ownerRowVar},
	}
	if mode.callback {
		callbackTail := target.RowClosed
		if mode.callbackOpen {
			callbackTail = target.RowUnknownOpen
		}
		empty := target.ValuesSpec{Tail: target.ValuesClosed}
		terminals := []target.TerminalSpec{
			{Kind: flowkind.OutcomeNormal, Values: empty}, {Kind: flowkind.OutcomeReturn, Values: empty},
			{Kind: flowkind.OutcomeThrow, Values: empty}, {Kind: flowkind.OutcomeYield, Values: empty},
			{Kind: flowkind.OutcomeCancel, Values: empty},
		}
		owner.Callbacks = []target.CallbackSpec{
			{Function: target.InputSource{Kind: target.InputSourceValueFormal}, Admission: target.OrdinaryCallable,
				Arguments: empty, Outcomes: terminals, Lifecycle: target.CallbackRetainedOptionalOnce,
				Effects: target.RowSpec{Occurrences: []target.EffectSpec{{Target: 3, ValueArgs: []target.ValueFormal{0}}}, Tail: callbackTail}},
		}
	}

	targetOperation := target.OperationSpec{
		Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"effect-target"}}},
		RowFormals: func() uint32 {
			if mode.rowArgs {
				return 1
			}
			return 0
		}(),
		Input:    target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesClosed},
		Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
		Effects:  target.RowSpec{Tail: target.RowClosed},
	}
	callbackTarget := target.OperationSpec{
		Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"callback-effect-target"}}},
		Input:    target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesClosed},
		Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
		Effects:  target.RowSpec{Tail: target.RowClosed},
	}
	return target.Spec{Operations: []target.OperationSpec{owner, targetOperation, callbackTarget}}
}

func newCallsiteTestFixture(t testing.TB, mode callsiteTestMode) callsiteTestFixture {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "callsite_law.lua", Text: []byte("local function sink(value) return value end\nsink(1)")})
	if err != nil {
		t.Fatal(err)
	}
	spec := callsiteTestSpec(mode)
	contract, err := target.Seal(&spec)
	if err != nil {
		t.Fatal(err)
	}
	endpointRequests := make([]linkboundary.EndpointRequest, 0, mode.endpointAliases)
	for i := 0; i < mode.endpointAliases; i++ {
		endpointRequests = append(endpointRequests, linkboundary.EndpointRequest{Identity: "alias-" + string(rune('a'+i)), Binding: target.BindingSpec{Namespace: target.BindingProvider, Owner: []string{"test"}, Member: []string{"sink"}}})
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "callsite_law", Program: program}}, EndpointRequests: endpointRequests})
	if err != nil {
		t.Fatal(err)
	}
	types, ok := typeauthority.Seal(linked)
	if !ok {
		t.Fatal("seal type authority")
	}
	statics, _, err := staticdomain.Seal(linked, types)
	if err != nil {
		t.Fatal(err)
	}
	packs, ok := pack.Seal(linked, statics)
	if !ok {
		t.Fatal("seal Pack")
	}
	effectsAlg, ok := factor.New(linked, packs, contract)
	if !ok {
		t.Fatal("seal Effect factor")
	}
	callsAlg, ok := call.New(linked)
	if !ok {
		t.Fatal("seal Call factor")
	}
	owner, ok := contract.Lookup(target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"sink"}})
	if !ok {
		t.Fatal("sink operation")
	}
	if mode.selectOpaque {
		owner, ok = contract.Opaque()
		if !ok {
			t.Fatal("opaque operation")
		}
	}
	applications := linked.Project().Applications().Calls()
	application, ok := applications.At(0)
	if !ok {
		t.Fatal("Call application")
	}
	root, ok := effectsAlg.RootForCall(application)
	if !ok {
		t.Fatal("Effect root")
	}
	key, ok := callsAlg.KeyForApplication(application)
	if !ok {
		t.Fatal("Call key")
	}
	capabilities := make([]call.Target, 0, 1)
	if mode.endpointAliases == 0 {
		seed, ok := linked.Boundary().Seeds().ForOperation(owner)
		if !ok {
			t.Fatal("operation seed")
		}
		capability, ok := callsAlg.TargetForSeed(seed)
		if !ok {
			t.Fatal("Call target capability")
		}
		capabilities = append(capabilities, capability)
	} else {
		endpoints := linked.Boundary().Endpoints()
		if endpoints.Count() != mode.endpointAliases {
			t.Fatal("endpoint alias count")
		}
		for i := 0; i < endpoints.Count(); i++ {
			endpoint, ok := endpoints.At(i)
			if !ok {
				t.Fatal("endpoint alias")
			}
			seed, ok := endpoints.Seed(endpoint)
			if !ok {
				t.Fatal("endpoint seed")
			}
			capability, ok := callsAlg.TargetForSeed(seed)
			if !ok {
				t.Fatal("endpoint Call target")
			}
			capabilities = append(capabilities, capability)
		}
	}
	var value call.Value
	if mode.callTop {
		value = callsAlg.Top()
	} else {
		value, ok = callsAlg.DispatchValue(key, capabilities, mode.callOpaque)
		if !ok {
			t.Fatal("Call dispatch value")
		}
	}

	composition := engine.NewComposition()
	callsOwner, ok := callowner.Declare(composition, callsiteTestKey(1), callsAlg)
	if !ok {
		t.Fatal("Call owner declaration")
	}
	effectsOwner, ok := effectowner.Declare(composition, callsiteTestKey(2), callsiteTestKey(22), effectsAlg)
	if !ok {
		t.Fatal("Effect owner declaration")
	}
	callsiteSemantic, family, evidence := callsiteTestKey(3), callsiteTestKey(4), callsiteTestKey(5)
	var callsiteRule *Rule
	if mode.tamperMode == 0 {
		if mode.opaqueRule {
			callsiteRule, ok = DeclareOpaque(composition, callsiteSemantic, family, evidence, callsOwner, effectsOwner)
		} else {
			callsiteRule, ok = DeclareSelected(composition, callsiteSemantic, family, evidence, callsOwner, effectsOwner)
		}
		if !ok || callsiteRule == nil {
			t.Fatal("callsite rule declaration")
		}
	}

	var effectSeed *engine.Rule[factor.Value, callsiteTestOperand]
	var effectWrite engine.Write[factor.Value]
	if mode.tamperMode != 0 {
		known, knownOK := effectsAlg.CallEffectAtom(root, application, owner, 0)
		if !knownOK {
			t.Fatal("tamper effect seed atom")
		}
		seedValue, seedOK := effectsAlg.Singleton(known)
		if !seedOK {
			t.Fatal("tamper effect seed value")
		}
		var seedWrite engine.Write[factor.Value]
		effectSeed, ok = engine.DeclareRule(composition, engine.RuleSpec[factor.Value, callsiteTestOperand]{
			Semantic: callsiteTestKey(16), OperandFamily: callsiteTestKey(17), OperandContent: callsiteTestOperandContent,
			Output: effectsOwner.Output(), Inputs: 0, Admission: engine.AdmitRuleByTrustedTheorem[factor.Value, callsiteTestOperand](callsiteTestKey(18)),
			Transfer: func(access engine.Access[factor.Value, callsiteTestOperand]) bool {
				return engine.Product(access, func(row engine.Row) bool { return engine.StageValue(access, row, seedValue) })
			},
		}, func(rule *engine.Rule[factor.Value, callsiteTestOperand]) bool {
			var declared bool
			seedWrite, declared = engine.WriteTo(rule, effectsOwner.ExactWrite())
			return declared
		})
		if !ok || effectSeed == nil {
			t.Fatal("tamper effect seed declaration")
		}

		callsiteRule = &Rule{semantic: callsiteSemantic, calls: callsOwner, effects: effectsOwner}
		custom := callsiteRule
		var customRule *engine.Rule[factor.Value, operand]
		customRule, ok = engine.DeclareRule(composition, engine.RuleSpec[factor.Value, operand]{
			Semantic: callsiteSemantic, OperandFamily: family, OperandContent: operandContent,
			Output: effectsOwner.Output(), Inputs: 1, Admission: engine.AdmitRuleByDerivation[factor.Value, operand](evidence, custom.check),
			Transfer: func(access engine.Access[factor.Value, operand]) bool {
				return engine.Product(access, func(row engine.Row) bool {
					if mode.tamperMode == 2 {
						return engine.StageTransform(access, row)
					}
					return engine.StageValue(access, row, seedValue)
				})
			},
		}, func(rule *engine.Rule[factor.Value, operand]) bool {
			input, inputOK := rule.InputAt(0)
			read, readOK := engine.ReadFrom(rule, input, callsOwner.ExactRead())
			write, writeOK := engine.WriteTo(rule, effectsOwner.ExactWrite())
			carry := effectsOwner.Carry()
			if !inputOK || !readOK || !writeOK || !engine.TransformCarryFrom(rule, input, carry, callsiteTestKey(19), func(_ operand, value factor.Value) (factor.Value, bool) {
				return value, true
			}) {
				return false
			}
			custom.rule, custom.read, custom.write = rule, read, write
			return true
		})
		if !ok || customRule == nil {
			t.Fatal("tamper callsite declaration")
		}
		effectWrite = seedWrite
	}

	var callWrite engine.Write[call.Value]
	callSource, ok := engine.DeclareRule(composition, engine.RuleSpec[call.Value, callsiteTestOperand]{
		Semantic: callsiteTestKey(6), OperandFamily: callsiteTestKey(7), OperandContent: callsiteTestOperandContent,
		Output: callsOwner.Output(), Inputs: 0, Admission: engine.AdmitRuleByTrustedTheorem[call.Value, callsiteTestOperand](callsiteTestKey(8)),
		Transfer: func(access engine.Access[call.Value, callsiteTestOperand]) bool {
			return engine.Product(access, func(row engine.Row) bool { return engine.StageValue(access, row, value) })
		},
	}, func(rule *engine.Rule[call.Value, callsiteTestOperand]) bool {
		var declared bool
		callWrite, declared = engine.WriteTo(rule, callsOwner.ExactWrite())
		return declared
	})
	if !ok || callSource == nil {
		t.Fatal("Call source rule declaration")
	}

	var queryRead engine.QueryRead[engine.OrderedCells[factor.Value]]
	query, ok := engine.DeclareQuery(composition, engine.QuerySpec[uint64]{
		Semantic: callsiteTestKey(9),
		Project: func(observation engine.Observation) uint64 {
			result, rows := uint64(0), 0
			if !engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				cells, readable := engine.QueryValue(row, queryRead)
				if !readable || cells.Count() != 1 {
					return false
				}
				value, present, cell := cells.At(0)
				if !cell {
					return false
				}
				if present {
					if effectsAlg.Equal(value, effectsAlg.Top()) {
						result = ^uint64(0)
					} else {
						for index := 0; ; index++ {
							if _, exists := effectsAlg.AtomAt(value, index); !exists {
								break
							}
							result++
						}
					}
				}
				rows++
				return true
			}) || rows != 1 {
				return 0
			}
			return result
		},
		Result: engine.FrozenResult[uint64]{Semantic: callsiteTestKey(10), Freeze: func(value uint64) uint64 { return value }, Clone: func(value uint64) uint64 { return value }, Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value }},
	}, func(query *engine.Query[uint64]) bool {
		var declared bool
		queryRead, declared = engine.QueryReadFrom(query, effectsOwner.ExactRead())
		return declared
	})
	if !ok || query == nil || !composition.Seal() {
		t.Fatal("callsite composition seal")
	}

	return callsiteTestFixture{contract: contract, linked: linked, packs: packs, callsAlg: callsAlg, effectsAlg: effectsAlg, calls: callsOwner, effects: effectsOwner, composition: composition, callRule: callSource, effectSeed: effectSeed, callsite: callsiteRule, callWrite: callWrite, effectWrite: effectWrite, query: query, queryRead: queryRead, application: application, root: root, owner: owner, key: key, value: value}
}

type callsiteTestRun struct {
	assembled bool
	state     *engine.State
	status    engine.SolveStatus
	value     uint64
	readable  bool
}

func runCallsiteTestFixture(t testing.TB, fixture callsiteTestFixture) callsiteTestRun {
	t.Helper()
	callRef, ok := fixture.calls.Locate(fixture.key)
	if !ok {
		t.Fatal("Call source ref")
	}
	sourceOperand := callsiteTestOperand{digest: callsiteTestKey(11).Digest()}
	sourceInstance, ok := engine.NewRuleInstance(fixture.callRule, sourceOperand, func(binding *engine.RuleBinding[call.Value, callsiteTestOperand]) bool {
		return engine.InstanceWrite(binding, fixture.callWrite, callRef)
	})
	if !ok {
		t.Fatal("Call source instance")
	}
	var effectSeedInstance *engine.RuleInstance[factor.Value, callsiteTestOperand]
	var effectSeedOperand callsiteTestOperand
	if fixture.effectSeed != nil {
		effectSeedOperand = callsiteTestOperand{digest: callsiteTestKey(20).Digest()}
		effectRef, effectOK := fixture.effects.Locate(fixture.root)
		if !effectOK {
			t.Fatal("Effect seed ref")
		}
		var seedOK bool
		effectSeedInstance, seedOK = engine.NewRuleInstance(fixture.effectSeed, effectSeedOperand, func(binding *engine.RuleBinding[factor.Value, callsiteTestOperand]) bool {
			return engine.InstanceWrite(binding, fixture.effectWrite, effectRef)
		})
		if !seedOK {
			t.Fatal("Effect seed instance")
		}
	}
	callsiteInstance, ok := fixture.callsite.Instance(fixture.application)
	if !ok {
		t.Fatal("callsite instance")
	}

	effectRef, ok := fixture.effects.Locate(fixture.root)
	if !ok {
		t.Fatal("Effect ref")
	}
	source := engine.NewSourceAssembly(fixture.composition)
	if source == nil {
		t.Fatal("source assembly")
	}
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	falsity, falsityOK := source.FalseExpr()
	sourceSite, sourceSiteOK := source.Site(callsiteTestKey(12), scope, truth, true)
	targetSite, targetSiteOK := source.Site(callsiteTestKey(13), scope, falsity, false)
	sourceOccurrence, sourceOccurrenceOK := source.At(sourceSite)
	effectSeedOccurrence, effectSeedOccurrenceOK := source.Relation(sourceSite, callsiteTestKey(21))
	targetOccurrence, targetOccurrenceOK := source.Relation(targetSite, callsiteTestKey(14))
	sourcePrepared, sourceOperandOK := source.PrepareInstance(sourceOccurrence, sourceInstance)
	effectSeedPrepared, effectSeedOperandOK := source.PrepareInstance(effectSeedOccurrence, effectSeedInstance)
	targetPrepared, targetOperandOK := source.PrepareInstance(targetOccurrence, callsiteInstance)
	reindex, reindexOK := source.IdentityReindex(scope)
	boundary, boundaryOK := source.Boundary(sourceSite, targetSite, callsiteTestKey(15), truth, reindex, truth)
	sealed := source.Seal()
	if !scopeOK || !truthOK || !falsityOK || !sourceSiteOK || !targetSiteOK || !sourceOccurrenceOK || !targetOccurrenceOK || !sourceOperandOK || !targetOperandOK || (fixture.effectSeed != nil && (!effectSeedOccurrenceOK || !effectSeedOperandOK)) || !sealed || !reindexOK || !boundaryOK {
		t.Fatal("source stage")
	}

	var assembled bool
	var queryInstance *engine.QueryInstance[uint64]
	solver, compiled := source.Assemble(func(value *engine.Assembly) bool {
		sourcePoint, sourcePointOK := value.Point(sourceSite)
		targetPoint, targetPointOK := value.Point(targetSite)
		sourceMember, sourceMemberOK := value.Member(sourcePoint, sourcePrepared)
		var effectSeedMember engine.AssemblyMember
		var effectSeedMemberOK bool
		if effectSeedInstance != nil {
			effectSeedMember, effectSeedMemberOK = value.Member(sourcePoint, effectSeedPrepared)
		} else {
			effectSeedMemberOK = true
		}
		targetMember, targetMemberOK := value.Member(targetPoint, targetPrepared)
		var sourceGroup engine.AssemblyGroup
		var sourceGroupOK bool
		if effectSeedInstance != nil {
			sourceGroup, sourceGroupOK = value.Group(sourcePoint, sourceMember, effectSeedMember)
		} else {
			sourceGroup, sourceGroupOK = value.Group(sourcePoint, sourceMember)
		}
		targetGroup, targetGroupOK := value.Group(targetPoint, targetMember)
		var queryOK bool
		queryInstance, queryOK = engine.NewQueryInstance(fixture.query, func(binding *engine.QueryBinding[uint64]) bool {
			return engine.InstanceQueryRead(binding, fixture.queryRead, effectRef)
		})
		_, queryAttached := value.Query(targetPoint, queryInstance)
		boundaryAttached := value.Boundary(targetGroup, boundary)
		assembled = sourcePointOK && targetPointOK && sourceMemberOK && effectSeedMemberOK && targetMemberOK && sourceGroupOK && targetGroupOK && sourceGroup.Available() && queryOK && queryAttached && boundaryAttached
		return assembled
	})
	if !compiled || solver == nil || !assembled {
		return callsiteTestRun{assembled: assembled}
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	var result uint64
	var readable bool
	if receiptOK {
		result, readable = engine.QueryResult(receipt, state)
	}
	return callsiteTestRun{assembled: true, state: state, status: status, value: result, readable: readable}
}

// runCallsiteSelfRecurrenceFixture puts the Call producer and the callsite
// rule at one point, with only the callsite group on an identity self-edge.
// The first self-edge execution can therefore have an empty Product before
// the Call seed is published; the same read observation must wake it for the
// later nonempty Call value. This is the real SourceAssembly/Solver path used
// by the production recurrent Effect topology.
func runCallsiteSelfRecurrenceFixture(t testing.TB, fixture callsiteTestFixture) callsiteTestRun {
	t.Helper()
	callRef, ok := fixture.calls.Locate(fixture.key)
	if !ok {
		t.Fatal("Call self-recurrence source ref")
	}
	sourceOperand := callsiteTestOperand{digest: callsiteTestKey(23).Digest()}
	sourceInstance, ok := engine.NewRuleInstance(fixture.callRule, sourceOperand, func(binding *engine.RuleBinding[call.Value, callsiteTestOperand]) bool {
		return engine.InstanceWrite(binding, fixture.callWrite, callRef)
	})
	if !ok {
		t.Fatal("Call self-recurrence source instance")
	}
	callsiteInstance, ok := fixture.callsite.Instance(fixture.application)
	if !ok {
		t.Fatal("Call self-recurrence callsite instance")
	}
	effectRef, ok := fixture.effects.Locate(fixture.root)
	if !ok {
		t.Fatal("Call self-recurrence Effect ref")
	}

	source := engine.NewSourceAssembly(fixture.composition)
	if source == nil {
		t.Fatal("Call self-recurrence source assembly")
	}
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	site, siteOK := source.Site(callsiteTestKey(24), scope, truth, true)
	sourceOccurrence, sourceOccurrenceOK := source.At(site)
	targetOccurrence, targetOccurrenceOK := source.Relation(site, callsiteTestKey(25))
	sourcePrepared, sourcePreparedOK := source.PrepareInstance(sourceOccurrence, sourceInstance)
	targetPrepared, targetPreparedOK := source.PrepareInstance(targetOccurrence, callsiteInstance)
	reindex, reindexOK := source.IdentityReindex(scope)
	boundary, boundaryOK := source.Boundary(site, site, callsiteTestKey(26), truth, reindex, truth)
	if !scopeOK || !truthOK || !siteOK || !sourceOccurrenceOK || !targetOccurrenceOK || !sourcePreparedOK || !targetPreparedOK || !reindexOK || !boundaryOK || !source.Seal() {
		t.Fatal("Call self-recurrence source schema")
	}

	var queryInstance *engine.QueryInstance[uint64]
	assembled := false
	solver, compiled := source.Assemble(func(assembly *engine.Assembly) bool {
		point, pointOK := assembly.Point(site)
		sourceMember, sourceMemberOK := assembly.Member(point, sourcePrepared)
		targetMember, targetMemberOK := assembly.Member(point, targetPrepared)
		sourceGroup, sourceGroupOK := assembly.Group(point, sourceMember)
		targetGroup, targetGroupOK := assembly.Group(point, targetMember)
		var queryOK bool
		queryInstance, queryOK = engine.NewQueryInstance(fixture.query, func(binding *engine.QueryBinding[uint64]) bool {
			return engine.InstanceQueryRead(binding, fixture.queryRead, effectRef)
		})
		_, queryAttached := assembly.Query(point, queryInstance)
		boundaryAttached := assembly.Boundary(targetGroup, boundary)
		assembled = pointOK && sourceMemberOK && targetMemberOK && sourceGroupOK && targetGroupOK && sourceGroup.Available() && queryOK && queryAttached && boundaryAttached
		return assembled
	})
	if !compiled || solver == nil || !assembled {
		return callsiteTestRun{assembled: assembled}
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	var result uint64
	var readable bool
	if receiptOK {
		result, readable = engine.QueryResult(receipt, state)
	}
	return callsiteTestRun{assembled: true, state: state, status: status, value: result, readable: readable}
}

func TestCallsiteSelectedOrdinaryAndCallbackRunsThroughSolver(t *testing.T) {
	run := runCallsiteTestFixture(t, newCallsiteTestFixture(t, callsiteTestMode{ordinaryEffects: true, callback: true}))
	if !run.assembled || run.status != engine.SolveComplete || run.state == nil || !run.readable || run.value != 2 {
		t.Fatalf("selected ordinary+callback: assembled=%t state=%v status=%v readable=%t atoms=%d", run.assembled, run.state, run.status, run.readable, run.value)
	}
}

func TestCallsiteSelectedAliasTargetsQuotientBeforeCapacity(t *testing.T) {
	fixture := newCallsiteTestFixture(t, callsiteTestMode{ordinaryEffects: true, endpointAliases: 3})
	if fixture.value.KnownTargetCount() != 3 {
		t.Fatalf("known endpoint aliases = %d, want 3", fixture.value.KnownTargetCount())
	}
	for i := 0; i < fixture.value.KnownTargetCount(); i++ {
		target, ok := fixture.value.KnownTargetAt(i)
		if !ok {
			t.Fatal("known endpoint target")
		}
		operation, ok := target.Operation()
		if !ok || operation != fixture.owner {
			t.Fatal("endpoint target did not select the shared operation")
		}
	}
	if rank := fixture.effectsAlg.WidenRank(fixture.root, fixture.effectsAlg.Bottom(), 0); rank != 3 {
		t.Fatalf("small Effect capacity rank = %d, want 3 (capacity two)", rank)
	}
	run := runCallsiteTestFixture(t, fixture)
	if !run.assembled || run.status != engine.SolveComplete || run.state == nil || !run.readable || run.value != 1 {
		t.Fatalf("alias quotient result: assembled=%t state=%v status=%v readable=%t atoms=%d", run.assembled, run.state, run.status, run.readable, run.value)
	}
}

func TestCallsiteOpaqueCallAndExplicitUnknownOpenRowRunThroughSolver(t *testing.T) {
	fixture := newCallsiteTestFixture(t, callsiteTestMode{ordinaryEffects: true, opaqueRule: true, callOpaque: true, selectOpaque: true})
	tail, _, ok := fixture.contract.EffectTail(fixture.owner)
	if !ok || tail != target.RowUnknownOpen || !fixture.value.HasOpaqueAlternative() {
		t.Fatal("opaque/open-row fixture lost its explicit evidence")
	}
	run := runCallsiteTestFixture(t, fixture)
	if !run.assembled || run.status != engine.SolveComplete || run.state == nil || !run.readable || run.value != 1 {
		t.Fatalf("opaque/open-row result: assembled=%t state=%v status=%v readable=%t atoms=%d", run.assembled, run.state, run.status, run.readable, run.value)
	}
}

func TestCallsiteTopCallRetainsOpaqueBoundary(t *testing.T) {
	fixture := newCallsiteTestFixture(t, callsiteTestMode{ordinaryEffects: true, opaqueRule: true, callTop: true})
	if !fixture.value.IsTop() || !fixture.value.HasOpaqueAlternative() {
		t.Fatal("Call.Top did not retain its opaque boundary")
	}
	run := runCallsiteTestFixture(t, fixture)
	if !run.assembled || run.status != engine.SolveComplete || run.state == nil || !run.readable || run.value != ^uint64(0) {
		t.Fatalf("Call.Top result: assembled=%t state=%v status=%v readable=%t atoms=%d", run.assembled, run.state, run.status, run.readable, run.value)
	}
}

func TestCallsiteClosedEmptyEffectSettlesNoCandidate(t *testing.T) {
	run := runCallsiteTestFixture(t, newCallsiteTestFixture(t, callsiteTestMode{}))
	if !run.assembled || run.status != engine.SolveComplete || run.state == nil || !run.readable || run.value != 0 {
		t.Fatalf("closed empty result: assembled=%t state=%v status=%v readable=%t atoms=%d", run.assembled, run.state, run.status, run.readable, run.value)
	}
}

func TestCallsiteSelectedSelfRecurrenceAcceptsEmptyFirstProductAndWakes(t *testing.T) {
	run := runCallsiteSelfRecurrenceFixture(t, newCallsiteTestFixture(t, callsiteTestMode{ordinaryEffects: true}))
	if !run.assembled || run.status != engine.SolveComplete || run.state == nil || !run.readable || run.value != 1 {
		t.Fatalf("selected self recurrence: assembled=%t state=%v status=%v readable=%t atoms=%d", run.assembled, run.state, run.status, run.readable, run.value)
	}
}

func TestCallsiteOpaqueSelfRecurrenceAcceptsEmptyFirstProductAndWakes(t *testing.T) {
	run := runCallsiteSelfRecurrenceFixture(t, newCallsiteTestFixture(t, callsiteTestMode{ordinaryEffects: true, opaqueRule: true, callOpaque: true, selectOpaque: true}))
	if !run.assembled || run.status != engine.SolveComplete || run.state == nil || !run.readable || run.value != 1 {
		t.Fatalf("opaque self recurrence: assembled=%t state=%v status=%v readable=%t atoms=%d", run.assembled, run.state, run.status, run.readable, run.value)
	}
}

func TestCallsiteRejectsUnsupportedRowVariableAndRowArguments(t *testing.T) {
	for name, mode := range map[string]callsiteTestMode{
		"row-variable":  {ordinaryEffects: true, rowVariable: true},
		"row-arguments": {ordinaryEffects: true, rowArgs: true},
	} {
		t.Run(name, func(t *testing.T) {
			run := runCallsiteTestFixture(t, newCallsiteTestFixture(t, mode))
			if !run.assembled || run.state != nil || run.status != engine.SolveIncomplete {
				t.Fatalf("unsupported row was admitted: assembled=%t state=%v status=%v", run.assembled, run.state, run.status)
			}
		})
	}
}

func TestCallsiteRejectsForeignApplicationAndSameContentForeignEffectOwner(t *testing.T) {
	fixture := newCallsiteTestFixture(t, callsiteTestMode{ordinaryEffects: true})
	foreign := newCallsiteTestFixture(t, callsiteTestMode{ordinaryEffects: true})
	if _, ok := fixture.callsite.Instance(foreign.application); ok {
		t.Fatal("foreign application crossed callsite owner fence")
	}

	foreignAlgebra, ok := factor.New(fixture.linked, fixture.packs, fixture.contract)
	if !ok {
		t.Fatal("same-content foreign Effect algebra")
	}
	foreignRoot, ok := foreignAlgebra.RootForCall(fixture.application)
	if !ok {
		t.Fatal("same-content foreign Effect root")
	}
	foreignOperand, ok := newOperand(foreignAlgebra, fixture.callsAlg, foreignRoot, fixture.application)
	if !ok || fixture.callsite.validOperand(foreignOperand) {
		t.Fatal("same-content foreign Effect owner was accepted")
	}
}

func TestCallsiteDerivationRejectsCarryTransform(t *testing.T) {
	run := runCallsiteTestFixture(t, newCallsiteTestFixture(t, callsiteTestMode{ordinaryEffects: true, tamperMode: 1}))
	if !run.assembled || run.state != nil || run.status != engine.SolveIncomplete {
		t.Fatalf("CarryTransform tamper was admitted: assembled=%t state=%v status=%v", run.assembled, run.state, run.status)
	}
}

func TestCallsiteDerivationRejectsTransformOnly(t *testing.T) {
	run := runCallsiteTestFixture(t, newCallsiteTestFixture(t, callsiteTestMode{ordinaryEffects: true, tamperMode: 2}))
	if !run.assembled || run.state != nil || run.status != engine.SolveIncomplete {
		t.Fatalf("TransformOnly tamper was admitted: assembled=%t state=%v status=%v", run.assembled, run.state, run.status)
	}
}
