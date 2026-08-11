package engine

import (
	"context"
	"testing"
)

// TestActivationFrontierBatchesIndependentGroupsAndKeepsMixedOutput proves
// that activation evidence is collected while each fixed Group still finishes
// its ordinary candidate. Both independent triggers must reach one compiler
// revision, and the mixed seed+activation Group must retain its seed output.
func TestActivationFrontierBatchesIndependentGroupsAndKeepsMixedOutput(t *testing.T) {
	composition := NewComposition()
	factor := coldFactor(composition, coldKey(983_001))
	read, readOK := ExactReadForm(factor)
	write, writeOK := ExactWriteForm(factor)
	var seedWrite Write[uint64]
	seedRuns := 0
	seed, seedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(983_002), OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Output: factor.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](983_003),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			seedRuns++
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(17)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var ok bool
		seedWrite, ok = WriteTo(rule, write)
		return ok
	})
	family, familyOK := DeclareActivationFamily(composition, coldKey(983_004))
	applicationA, applicationB := coldKey(983_005), coldKey(983_006)
	targetA, targetB := coldKey(983_007), coldKey(983_008)
	endpointA, endpointB := coldKey(983_009), coldKey(983_010)
	activationRuns := 0
	triggerA, triggerAOK := DeclareActivationRule(composition, ActivationRuleSpec{
		Semantic: coldKey(983_011), Family: family, Inputs: 0,
		Admission: AdmitActivationByTrustedTheorem(coldKey(983_012)),
		Run: func(value Activation) bool {
			activationRuns++
			application, projected := ActivationApplication(value)
			return projected && application == applicationA && Activate(value, application, targetA, endpointA)
		},
	})
	triggerB, triggerBOK := DeclareActivationRule(composition, ActivationRuleSpec{
		Semantic: coldKey(983_013), Family: family, Inputs: 0,
		Admission: AdmitActivationByTrustedTheorem(coldKey(983_014)),
		Run: func(value Activation) bool {
			activationRuns++
			application, projected := ActivationApplication(value)
			return projected && application == applicationB && Activate(value, application, targetB, endpointB)
		},
	})
	var queryRead QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(983_015),
		Project: func(observation Observation) uint64 {
			var result uint64
			ProjectRows(observation, func(row QueryRow) bool {
				cells, ok := QueryValue(row, queryRead)
				value, present, valid := cells.At(0)
				if !ok || !valid || !present {
					return false
				}
				result = value
				return true
			})
			return result
		},
		Result: frozenColdResult(coldKey(983_016)),
	}, func(value *Query[uint64]) bool {
		var ok bool
		queryRead, ok = QueryReadFrom(value, read)
		return ok
	})
	if factor == nil || !readOK || !writeOK || !seedOK || seed == nil || !familyOK || !triggerAOK || triggerA == nil || !triggerBOK || triggerB == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("activation frontier declarations")
	}
	seedRef, seedRefOK := factor.Ref(0)
	seedInstance, seedInstanceOK := NewRuleInstance(seed, ruleUnitForSemantic(coldKey(983_017)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, seedWrite, seedRef)
	})
	seedInstanceB, seedInstanceBOK := NewRuleInstance(seed, ruleUnitForSemantic(coldKey(983_027)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, seedWrite, seedRef)
	})
	carry, carryOK := Carry(factor)
	if !seedRefOK || !seedInstanceOK || seedInstance == nil || !seedInstanceBOK || seedInstanceB == nil || !carryOK {
		t.Fatal("activation frontier seed")
	}

	build := NewSourceAssembly(composition)
	scope, scopeOK := build.Scope()
	truth, truthOK := build.TrueExpr()
	pointASite, pointAOK := build.Site(coldKey(983_018), scope, truth, true)
	pointBSite, pointBOK := build.Site(coldKey(983_019), scope, truth, true)
	targetASite, targetAOK := build.Site(coldKey(983_020), scope, truth, true)
	targetBSite, targetBOK := build.Site(coldKey(983_021), scope, truth, true)
	occurrenceA, occurrenceAOK := build.At(pointASite)
	occurrenceB, occurrenceBOK := build.At(pointBSite)
	seedOccurrence, seedOccurrenceOK := build.At(pointASite)
	seedOccurrenceB, seedOccurrenceBOK := build.At(pointBSite)
	preparedA, preparedAOK := build.PrepareActivation(occurrenceA, coldKey(983_022), triggerA)
	preparedB, preparedBOK := build.PrepareActivation(occurrenceB, coldKey(983_023), triggerB)
	seedPrepared, seedOperandOK := build.PrepareInstance(seedOccurrence, seedInstance)
	seedPreparedB, seedOperandBOK := build.PrepareInstance(seedOccurrenceB, seedInstanceB)
	role := coldKey(983_024)
	preparedPlan, staged := StageActivationPlan(build, composition, family, []ActivationPlanEntry{
		{Target: targetA, Endpoint: endpointA, FactorEdges: []ActivationFactorEdge{{SourceRole: role, TargetSite: targetASite, Factor: carry, Provenance: coldKey(983_025)}}},
		{Target: targetB, Endpoint: endpointB, FactorEdges: []ActivationFactorEdge{{SourceRole: role, TargetSite: targetBSite, Factor: carry, Provenance: coldKey(983_026)}}},
	})
	sealed := build.Seal()
	plan, finalized := FinalizeActivationPlan(build, preparedPlan)
	if !scopeOK || !truthOK || !pointAOK || !pointBOK || !targetAOK || !targetBOK || !occurrenceAOK || !occurrenceBOK || !seedOccurrenceOK || !seedOccurrenceBOK || !preparedAOK || !preparedBOK || !seedOperandOK || !seedOperandBOK || !staged || !sealed || !finalized || plan == nil {
		t.Fatal("activation frontier source staging")
	}

	var queryInstanceA *QueryInstance[uint64]
	solver, assembled := build.Assemble(func(assembly *Assembly) bool {
		pointA := admitPoint(assembly, pointASite.value)
		pointB := admitPoint(assembly, pointBSite.value)
		targetPointA := admitPoint(assembly, targetASite.value)
		targetPointB := admitPoint(assembly, targetBSite.value)
		if pointA == nil || pointB == nil || targetPointA == nil || targetPointB == nil {
			return false
		}
		baseA, baseAOK := ActivationBaseAt(assembly, pointA)
		baseB, baseBOK := ActivationBaseAt(assembly, pointB)
		portA, portAOK := NewActivationPort(role, baseA)
		portB, portBOK := NewActivationPort(role, baseB)
		instanceA, instanceAOK := NewActivationTrigger(triggerA, applicationA, plan, []*ActivationPort{portA}, func(*StructuralBinding) bool { return true })
		instanceB, instanceBOK := NewActivationTrigger(triggerB, applicationB, plan, []*ActivationPort{portB}, func(*StructuralBinding) bool { return true })
		activationMemberA := admitStructural(assembly, pointA, preparedA.occurrence.value, preparedA.operand.value, instanceA)
		activationMemberB := admitStructural(assembly, pointB, preparedB.occurrence.value, preparedB.operand.value, instanceB)
		seedMember := admitInstance(assembly, pointA, seedPrepared.occurrence.value, seedPrepared.operand.value, seedInstance)
		seedMemberB := admitInstance(assembly, pointB, seedPreparedB.occurrence.value, seedPreparedB.operand.value, seedInstanceB)
		groupA := admitGroup(assembly, pointA, seedMember, activationMemberA)
		groupB := admitGroup(assembly, pointB, seedMemberB, activationMemberB)
		var queryAOK bool
		queryInstanceA, queryAOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, queryRead, seedRef)
		})
		queryInstanceB, queryBOK := NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, queryRead, seedRef)
		})
		queryAttachedA := admitQueryAt(assembly, pointA, queryInstanceA)
		queryAttachedB := admitQueryAt(assembly, pointB, queryInstanceB)
		return baseAOK && baseBOK && portAOK && portBOK && instanceAOK && instanceBOK && activationMemberA != nil && activationMemberB != nil && seedMember != nil && seedMemberB != nil && groupA != nil && groupB != nil && queryAOK && queryBOK && queryAttachedA != nil && queryAttachedB != nil
	})
	if !assembled || solver == nil || queryInstanceA == nil {
		t.Fatal("activation frontier assembly")
	}
	receipt, receiptOK := queryInstanceA.Receipt()
	state, status, report := solver.SolveWithReport(context.Background())
	result, readable := QueryResult(receipt, state)
	if !receiptOK || status != SolveComplete || state == nil || !readable || result != 17 {
		t.Fatalf("mixed activation result=%d/%t status=%v state=%t receipt=%t reason=%v phase=%v seedRuns=%d activationRuns=%d revision=%d accepted=%d", result, readable, status, state != nil, receiptOK, report.Reason(), report.Phase(), seedRuns, activationRuns, solver.revision, len(solver.accepted))
	}
	if activationRuns == 0 || solver.revision != 1 || len(solver.accepted) != 2 {
		t.Fatalf("activation frontier runs=%d revision=%d accepted=%d", activationRuns, solver.revision, len(solver.accepted))
	}
	seenA, seenB := false, false
	for _, accepted := range solver.accepted {
		locator, locatorOK := accepted.Member().Locator()
		if !locatorOK {
			continue
		}
		if locator.Application == applicationA.compositionKey() && locator.Target == targetA.compositionKey() && locator.Endpoint == endpointA.compositionKey() {
			seenA = true
		}
		if locator.Application == applicationB.compositionKey() && locator.Target == targetB.compositionKey() && locator.Endpoint == endpointB.compositionKey() {
			seenB = true
		}
	}
	if !seenA || !seenB || solver.accepted[0].Member().Same(solver.accepted[1].Member()) {
		t.Fatalf("activation frontier members seenA=%t seenB=%t same=%t", seenA, seenB, solver.accepted[0].Member().Same(solver.accepted[1].Member()))
	}
	if _, graphOK := solver.compiler.topology.Graph(solver.accepted); !graphOK {
		t.Fatal("accepted activation graph")
	}
}
