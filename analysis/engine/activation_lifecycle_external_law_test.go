package engine_test

import (
	"context"
	"testing"

	engine "github.com/wippyai/go-lua/analysis/engine"
)

// TestExternalActivationLifecycleUsesOneOpaqueLateBoundPath proves that a
// caller can prepare the source operand before sealing, finalize the shared
// structural plan after sealing, and then attach the trigger through only the
// public AssemblyPoint/ActivationBase/ActivationMember facade. The caller owns
// a guard decision absent from the static callee target, so revision compilation
// must forget that Port-lifted coordinate at the ordinary FactorEdge boundary.
func TestExternalActivationLifecycleUsesOneOpaqueLateBoundPath(t *testing.T) {
	composition := engine.NewComposition()
	factorKey := facadeKey(101)
	factor, factorOK := engine.DeclareFactor(composition, engine.FactorSpec[uint64, uint64]{
		Semantic: factorKey, KeyEnd: 1, Lattice: facadeLattice(), Default: 0,
		AdmitAt: func(uint64, uint64) bool { return true }, Fingerprint: func(value uint64) uint64 { return value },
		WidenRank:  engine.Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }},
		NarrowRank: engine.Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return value }},
	}, func(*engine.Factor[uint64, uint64]) bool { return true })
	read, readOK := engine.ExactReadForm(factor)
	write, writeOK := engine.ExactWriteForm(factor)
	var seedWrite engine.Write[uint64]
	seed, seedOK := engine.DeclareRule(composition, engine.RuleSpec[uint64, facadeUnit]{
		Semantic: facadeKey(102), OperandFamily: facadeKey(103), OperandContent: facadeUnitContent,
		Output: factor.Output(), Inputs: 0,
		Admission: engine.AdmitRuleByTrustedTheorem[uint64, facadeUnit](facadeKey(104)),
		Transfer: func(value engine.Access[uint64, facadeUnit]) bool {
			return engine.Product(value, func(row engine.Row) bool { return engine.StageValue(value, row, 1) })
		},
	}, func(rule *engine.Rule[uint64, facadeUnit]) bool {
		var ok bool
		seedWrite, ok = engine.WriteTo(rule, write)
		return ok
	})
	family, familyOK := engine.DeclareActivationFamily(composition, facadeKey(105))
	application, selectedTarget, unselectedTarget := facadeKey(106), facadeKey(107), facadeKey(108)
	selectedEndpoint, unselectedEndpoint, role := facadeKey(109), facadeKey(110), facadeKey(111)
	runs := 0
	trigger, triggerOK := engine.DeclareActivationRule(composition, engine.ActivationRuleSpec{
		Semantic: facadeKey(112), Family: family, Inputs: 0,
		Admission: engine.AdmitActivationByTrustedTheorem(facadeKey(113)),
		Run: func(value engine.Activation) bool {
			runs++
			boundApplication, projected := engine.ActivationApplication(value)
			return projected && boundApplication == application && engine.Activate(value, boundApplication, selectedTarget, selectedEndpoint)
		},
	})
	var queryRead engine.QueryRead[engine.OrderedCells[uint64]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[uint64]{
		Semantic: facadeKey(121), Project: func(engine.Observation) uint64 { return 0 },
		Result: engine.FrozenResult[uint64]{
			Semantic: facadeKey(122), Freeze: func(value uint64) uint64 { return value }, Clone: func(value uint64) uint64 { return value },
			Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value },
		},
	}, func(value *engine.Query[uint64]) bool {
		var ok bool
		queryRead, ok = engine.QueryReadFrom(value, read)
		return ok
	})
	if factor == nil || !factorOK || !readOK || !writeOK || seed == nil || !seedOK || !familyOK || !triggerOK || trigger == nil || query == nil || !queryOK || !composition.Seal() {
		t.Fatalf("activation lifecycle declarations factor=%t factorNil=%t read=%t write=%t seed=%t seedOK=%t family=%t trigger=%t triggerNil=%t query=%t queryNil=%t", factorOK, factor == nil, readOK, writeOK, seed != nil, seedOK, familyOK, triggerOK, trigger == nil, queryOK, query == nil)
	}
	seedRef, seedRefOK := factor.Ref(0)
	carry, carryOK := engine.Carry(factor)
	seedInstance, seedInstanceOK := engine.NewRuleInstance(seed, facadeUnitFor(facadeKey(114)), func(binding *engine.RuleBinding[uint64, facadeUnit]) bool {
		return engine.InstanceWrite(binding, seedWrite, seedRef)
	})
	if !seedRefOK || !seedInstanceOK || seedInstance == nil {
		t.Fatal("activation lifecycle seed")
	}

	source := engine.NewSourceAssembly(composition)
	callerDecision, callerDecisionOK := source.Decision(facadeKey(124))
	callerScope, callerScopeOK := source.Scope(callerDecision)
	calleeScope, calleeScopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	triggerSite, triggerSiteOK := source.Site(facadeKey(115), callerScope, truth, true)
	selectedSite, selectedSiteOK := source.Site(facadeKey(116), calleeScope, truth, true)
	unselectedSite, unselectedSiteOK := source.Site(facadeKey(117), calleeScope, truth, true)
	triggerOccurrence, triggerOccurrenceOK := source.At(triggerSite)
	activationPrepared, activationPreparedOK := source.PrepareActivation(triggerOccurrence, facadeKey(118), trigger)
	seedOccurrence, seedOccurrenceOK := source.At(triggerSite)
	seedPrepared, seedPreparedOK := source.PrepareInstance(seedOccurrence, seedInstance)
	activationInputs, activationInputsOK := source.InputCount(activationPrepared)
	prepared, staged := engine.StageActivationPlan(source, composition, family, []engine.ActivationPlanEntry{
		{Target: selectedTarget, Endpoint: selectedEndpoint, FactorEdges: []engine.ActivationFactorEdge{{
			SourceRole: role, TargetSite: selectedSite, Factor: carry, Provenance: facadeKey(119),
		}}},
		{Target: unselectedTarget, Endpoint: unselectedEndpoint, FactorEdges: []engine.ActivationFactorEdge{{
			SourceRole: role, TargetSite: unselectedSite, Factor: carry, Provenance: facadeKey(120),
		}}},
	})
	if !activationInputsOK || activationInputs != 0 {
		t.Fatalf("late-bound activation input count=%d ok=%t", activationInputs, activationInputsOK)
	}
	if activationPrepared.Available() {
		t.Fatal("late-bound capability became available before seal")
	}
	sealed := source.Seal()
	plan, finalized := engine.FinalizeActivationPlan(source, prepared)
	if !callerDecisionOK || !callerScopeOK || !calleeScopeOK || !truthOK || !triggerSiteOK || !selectedSiteOK || !unselectedSiteOK || !triggerOccurrenceOK || !activationPreparedOK || !carryOK ||
		!seedOccurrenceOK || !seedPreparedOK || !staged || !sealed || !finalized || plan == nil || !activationPrepared.Available() || !seedPrepared.Available() {
		t.Fatal("activation lifecycle source stages")
	}
	foreign := engine.NewSourceAssembly(composition)
	foreignScope, foreignScopeOK := foreign.Scope()
	foreignTruth, foreignTruthOK := foreign.TrueExpr()
	foreignSite, foreignSiteOK := foreign.Site(facadeKey(123), foreignScope, foreignTruth, true)
	foreignSealed := foreign.Seal()
	foreignTriggerCreated := false
	_, _ = foreign.Assemble(func(value *engine.Assembly) bool {
		foreignPoint, pointOK := value.Point(foreignSite)
		foreignBase, baseOK := value.ActivationBase(foreignPoint)
		foreignPort, portOK := engine.NewActivationPort(role, foreignBase)
		_, foreignTriggerCreated = engine.NewActivationTrigger(trigger, application, plan, []*engine.ActivationPort{foreignPort}, func(*engine.StructuralBinding) bool { return true })
		return pointOK && baseOK && portOK
	})
	if !foreignScopeOK || !foreignTruthOK || !foreignSiteOK || !foreignSealed || foreignTriggerCreated {
		t.Fatal("foreign same-composition activation plan claim")
	}

	var pointOK, targetPointOK, baseOK, portOK, triggerOK2, seedMemberOK, activationMemberOK, groupOK bool
	solver, assembled := source.Assemble(func(value *engine.Assembly) bool {
		triggerPoint, triggerPointOK := value.Point(triggerSite)
		var targetPoint engine.AssemblyPoint
		targetPoint, targetPointOK = value.Point(selectedSite)
		var base engine.ActivationBase
		base, baseOK = value.ActivationBase(triggerPoint)
		var port *engine.ActivationPort
		port, portOK = engine.NewActivationPort(role, base)
		var triggerInstance *engine.StructuralInstance
		var created bool
		triggerInstance, created = engine.NewActivationTrigger(trigger, application, plan, []*engine.ActivationPort{port}, func(*engine.StructuralBinding) bool { return true })
		var seedMember engine.AssemblyMember
		seedMember, seedMemberOK = value.Member(triggerPoint, seedPrepared)
		var activationMember engine.AssemblyMember
		activationMember, activationMemberOK = value.ActivationMember(triggerPoint, activationPrepared, triggerInstance)
		_, groupOK = value.Group(triggerPoint, seedMember, activationMember)
		pointOK = triggerPointOK
		_ = targetPoint
		_ = targetPointOK
		triggerOK2 = created
		queryInstance, queryCreated := engine.NewQueryInstance(query, func(binding *engine.QueryBinding[uint64]) bool {
			return engine.InstanceQueryRead(binding, queryRead, seedRef)
		})
		_, queryAttached := value.Query(triggerPoint, queryInstance)
		return pointOK && targetPointOK && baseOK && portOK && triggerOK2 && seedMemberOK && activationMemberOK && groupOK && queryCreated && queryAttached
	})
	state, status := (*engine.State)(nil), engine.SolveStatus(0)
	if solver != nil {
		state, status = solver.Solve(context.Background())
	}
	if !assembled || solver == nil || state == nil || status != engine.SolveComplete || runs == 0 || !pointOK || !targetPointOK || !baseOK || !portOK ||
		!triggerOK2 || !seedMemberOK || !activationMemberOK || !groupOK {
		t.Fatalf("activation lifecycle assembled=%t solver=%p state=%t status=%v runs=%d point=%t target=%t base=%t port=%t trigger=%t seed=%t activation=%t group=%t",
			assembled, solver, state != nil, status, runs, pointOK, targetPointOK, baseOK, portOK, triggerOK2, seedMemberOK, activationMemberOK, groupOK)
	}
	if activationPrepared.Available() {
		t.Fatal("late-bound activation capability was reusable")
	}
}
