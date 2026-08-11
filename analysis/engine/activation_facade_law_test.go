package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

func TestStructuralActivationTemplateAllowsDistinctFactorsOnSameEndpoints(t *testing.T) {
	composition := NewComposition()
	source, target := coldKey(981_030), coldKey(981_031)
	first, second := coldKey(981_032), coldKey(981_033)
	firstFactor := coldFactor(composition, first)
	secondFactor := coldFactor(composition, second)
	firstCarry, firstCarryOK := Carry(firstFactor)
	secondCarry, secondCarryOK := Carry(secondFactor)
	entry := ActivationPlanEntry{FactorEdges: []ActivationFactorEdge{
		{SourceRole: source, TargetRole: target, Factor: firstCarry, Provenance: coldKey(981_034)},
		{SourceRole: source, TargetRole: target, Factor: secondCarry, Provenance: coldKey(981_035)},
	}}
	template, ok := completeStructuralActivationTemplate(entry, nil)
	if !firstCarryOK || !secondCarryOK || !ok || len(template.FactorEdges) != 2 || len(template.Ports) != 2 {
		t.Fatal("distinct same-endpoint factors were collapsed")
	}
	duplicate := entry
	duplicate.FactorEdges = append(append([]ActivationFactorEdge(nil), entry.FactorEdges...), entry.FactorEdges[0])
	if _, ok := completeStructuralActivationTemplate(duplicate, nil); ok {
		t.Fatal("exact duplicate FactorEdge relation was admitted")
	}
}

func TestStructuralActivationPlanRequiresExternalBatchAnchorAndClonesEdges(t *testing.T) {
	composition := NewComposition()
	factor := coldFactor(composition, coldKey(981_036))
	read, readOK := ExactReadForm(factor)
	family, familyOK := DeclareActivationFamily(composition, coldKey(981_037))
	trigger, triggerOK := DeclareActivationRule(composition, ActivationRuleSpec{
		Semantic: coldKey(981_038), Family: family, Inputs: 0, Admission: AdmitActivationByTrustedTheorem(coldKey(981_039)),
		Run: func(Activation) bool { return true },
	})
	query, _, queryOK := declareColdQueryInstance(composition, coldKey(981_040), coldKey(981_041), read)
	if factor == nil || !readOK || !familyOK || !triggerOK || trigger == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("structural anchor declaration")
	}
	carry, carryOK := Carry(factor)
	if !carryOK {
		t.Fatal("structural anchor carry")
	}
	role, targetRole := coldKey(981_042), coldKey(981_043)
	roleOnly := NewSourceAssembly(composition)
	if _, ok := StageActivationPlan(roleOnly, composition, family, []ActivationPlanEntry{{
		Target: coldKey(981_044), Endpoint: coldKey(981_045), FactorEdges: []ActivationFactorEdge{{
			SourceRole: role, TargetRole: targetRole, Factor: carry, Provenance: coldKey(981_046),
		}},
	}}); ok {
		t.Fatal("role-only structural activation had no source batch anchor")
	}

	build := NewSourceAssembly(composition)
	scope, scopeOK := build.Scope()
	truth, truthOK := build.TrueExpr()
	targetSite, targetSiteOK := build.Site(coldKey(981_047), scope, truth, true)
	if !scopeOK || !truthOK || !targetSiteOK {
		t.Fatal("structural clone source")
	}
	edges := []ActivationFactorEdge{{SourceRole: role, TargetSite: targetSite, Factor: carry, Provenance: coldKey(981_048)}}
	prepared, staged := StageActivationPlan(build, composition, family, []ActivationPlanEntry{{
		Target: coldKey(981_049), Endpoint: coldKey(981_050), FactorEdges: edges,
	}})
	edges[0] = ActivationFactorEdge{SourceRole: role, TargetRole: targetRole, Factor: carry, Provenance: coldKey(981_051)}
	if !staged || prepared == nil || !build.Seal() {
		t.Fatal("structural clone staging")
	}
	if _, finalized := FinalizeActivationPlan(build, prepared); !finalized {
		t.Fatal("staged structural edge was aliased to caller storage")
	}
}

func TestStructuralActivationPlanSelectedExternalTargetLaw(t *testing.T) {
	composition := NewComposition()
	factorKey, seedKey, queryKey := coldKey(981_040), coldKey(981_041), coldKey(981_042)
	factor := coldFactor(composition, factorKey)
	read, readOK := ExactReadForm(factor)
	write, writeOK := ExactWriteForm(factor)
	var seedWrite Write[uint64]
	seed, seedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: seedKey, OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Output: factor.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](981_141),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(7)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var ok bool
		seedWrite, ok = WriteTo(rule, write)
		return ok
	})
	family, familyOK := DeclareActivationFamily(composition, coldKey(981_043))
	application, selectedTarget, unselectedTarget := coldKey(981_044), coldKey(981_045), coldKey(981_046)
	selectedEndpoint, unselectedEndpoint := coldKey(981_047), coldKey(981_048)
	role := coldKey(981_049)
	activationRuns := 0
	trigger, triggerOK := DeclareActivationRule(composition, ActivationRuleSpec{
		Semantic: coldKey(981_050), Family: family, Inputs: 0, Admission: AdmitActivationByTrustedTheorem(coldKey(981_150)),
		Run: func(value Activation) bool {
			activationRuns++
			boundApplication, projected := ActivationApplication(value)
			return projected && boundApplication == application && Activate(value, application, selectedTarget, selectedEndpoint)
		},
	})
	query, queryRead, queryOK := declareColdQueryInstance(composition, queryKey, coldKey(981_052), read)
	if factor == nil || !readOK || !writeOK || !seedOK || seed == nil || !familyOK || !triggerOK || trigger == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("structural activation declarations")
	}
	seedRef, seedRefOK := factor.Ref(1)
	seedInstance, seedInstanceOK := NewRuleInstance(seed, ruleUnitForSemantic(coldKey(981_053)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, seedWrite, seedRef)
	})
	if !seedRefOK || !seedInstanceOK || seedInstance == nil {
		t.Fatal("seed instance")
	}
	carry, carryOK := Carry(factor)
	if !carryOK {
		t.Fatal("structural activation carry")
	}
	build := NewSourceAssembly(composition)
	scope, scopeOK := build.Scope()
	truth, truthOK := build.TrueExpr()
	triggerSite, triggerSiteOK := build.Site(coldKey(981_054), scope, truth, true)
	selectedSite, selectedSiteOK := build.Site(coldKey(981_055), scope, truth, true)
	unselectedSite, unselectedSiteOK := build.Site(coldKey(981_056), scope, truth, true)
	triggerOccurrence, triggerOccurrenceOK := build.At(triggerSite)
	triggerOperand, triggerOperandOK := build.Operand(triggerOccurrence, coldKey(981_057))
	seedOccurrence, seedOccurrenceOK := build.At(triggerSite)
	seedPrepared, seedOperandOK := build.PrepareInstance(seedOccurrence, seedInstance)
	prepared, staged := StageActivationPlan(build, composition, family, []ActivationPlanEntry{
		{Target: selectedTarget, Endpoint: selectedEndpoint, FactorEdges: []ActivationFactorEdge{{SourceRole: role, TargetSite: selectedSite, Factor: carry, Provenance: coldKey(981_058)}}},
		{Target: unselectedTarget, Endpoint: unselectedEndpoint, FactorEdges: []ActivationFactorEdge{{SourceRole: role, TargetSite: unselectedSite, Factor: carry, Provenance: coldKey(981_059)}}},
	})
	sealed := build.Seal()
	if !scopeOK || !truthOK || !triggerSiteOK || !selectedSiteOK || !unselectedSiteOK || !triggerOccurrenceOK || !triggerOperandOK || !seedOccurrenceOK || !seedOperandOK || !staged || !sealed {
		t.Fatal("structural activation staging")
	}
	opposite, oppositeOK := completeStructuralActivationTemplate(ActivationPlanEntry{FactorEdges: []ActivationFactorEdge{{
		SourceSite: triggerSite, TargetRole: role, Factor: carry, Provenance: coldKey(981_060),
	}}}, composition)
	if !oppositeOK || len(opposite.FactorEdges) != 1 || !opposite.FactorEdges[0].ExternalSource.Available() || opposite.FactorEdges[0].Target.Port != role.compositionKey() {
		t.Fatal("external source/dynamic target structural transport")
	}
	plan, finalized := FinalizeActivationPlan(build, prepared)
	if !finalized || plan == nil || plan.EndpointCount() != 2 {
		t.Fatal("structural activation finalization")
	}
	foreignBuild := NewSourceAssembly(composition)
	foreignScope, foreignScopeOK := foreignBuild.Scope()
	foreignTruth, foreignTruthOK := foreignBuild.TrueExpr()
	_, foreignSiteOK := foreignBuild.Site(coldKey(981_061), foreignScope, foreignTruth, true)
	foreignSealed := foreignBuild.Seal()
	foreignAssembly := newAssembly(composition, foreignBuild.state.batch)
	if !foreignScopeOK || !foreignTruthOK || !foreignSiteOK || !foreignSealed || foreignAssembly == nil {
		t.Fatal("foreign activation assembly")
	}
	foreignAssembly.sourceAssembly = foreignBuild
	if plan.value.bind(foreignAssembly) || plan.value.assembly != nil {
		t.Fatal("foreign activation plan claim poisoned the owner")
	}

	solver, assembled := assemble(composition, build.state.batch, func(assembly *Assembly) bool {
		assembly.sourceAssembly = build
		triggerPoint := admitPoint(assembly, triggerSite.value)
		selectedPoint := admitPoint(assembly, selectedSite.value)
		unselectedPoint := admitPoint(assembly, unselectedSite.value)
		if triggerPoint == nil || selectedPoint == nil || unselectedPoint == nil {
			return false
		}
		base, baseOK := ActivationBaseAt(assembly, triggerPoint)
		port, portOK := NewActivationPort(role, base)
		triggerInstance, triggerCreated := NewActivationTrigger(trigger, application, plan, []*ActivationPort{port}, func(*StructuralBinding) bool { return true })
		seedMember := admitInstance(assembly, triggerPoint, seedPrepared.occurrence.value, seedPrepared.operand.value, seedInstance)
		triggerMember := admitStructural(assembly, triggerPoint, triggerOccurrence.value, triggerOperand.value, triggerInstance)
		if !baseOK || !portOK || !triggerCreated || seedMember == nil || triggerMember == nil || admitGroup(assembly, triggerPoint, seedMember, triggerMember) == nil {
			return false
		}
		queryInstance, queryCreated := NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, queryRead, seedRef)
		})
		return queryCreated && admitQueryAt(assembly, triggerPoint, queryInstance) != nil
	})
	if !assembled || solver == nil {
		t.Fatal("structural activation assembly")
	}
	state, status := solver.Solve(context.Background())
	if status != SolveComplete || state == nil || activationRuns == 0 || len(solver.accepted) != 1 {
		t.Fatalf("structural activation solve status=%v state=%v runs=%d accepted=%d", status, state != nil, activationRuns, len(solver.accepted))
	}
	t.Run("semantic activation delta", func(t *testing.T) {
		topology := solver.compiler.topology
		member := solver.accepted[0].Member()
		knownBroad, broadOK := topology.Accept(member, equation.TrueExpr())
		candidateSubsumed, subsumedOK := topology.Accept(member, equation.FalseExpr())
		delta, deltaOK := subtractAcceptedActivations(topology, []equation.AcceptedMember{candidateSubsumed}, []equation.AcceptedMember{knownBroad})
		if !broadOK || !subsumedOK || !deltaOK || len(delta) != 0 {
			t.Fatalf("subsumed activation requested revision broad=%t candidate=%t delta=%t count=%d", broadOK, subsumedOK, deltaOK, len(delta))
		}
		knownNarrow, narrowOK := topology.Accept(member, equation.FalseExpr())
		candidateExpansion, expansionOK := topology.Accept(member, equation.TrueExpr())
		delta, deltaOK = subtractAcceptedActivations(topology, []equation.AcceptedMember{candidateExpansion}, []equation.AcceptedMember{knownNarrow})
		if !narrowOK || !expansionOK || !deltaOK || len(delta) != 1 || delta[0].Evidence() != candidateExpansion.Evidence() {
			t.Fatalf("genuine activation expansion was lost narrow=%t candidate=%t delta=%t count=%d", narrowOK, expansionOK, deltaOK, len(delta))
		}
	})
	graph, graphOK := solver.compiler.topology.Graph(solver.accepted)
	if !graphOK || graph == nil || graph.FactorEdgeTotal() != 1 {
		t.Fatalf("selected structural edge count=%d", graph.FactorEdgeTotal())
	}
	edge, edgeOK := graph.FactorEdgeAtIndex(0)
	if !edgeOK || !edge.Target().Site().Same(selectedSite.value) || edge.Factor() != factor.schema.semantic.compositionKey() {
		t.Fatal("selected structural edge did not retain exact external target")
	}
}

// TestActivationFacadeOneInstancePathLaw exercises the only legal source
// path: one typed instance is admitted, placed in the paired Template row,
// attached once, and later expanded from the trigger's exact selection.  The
// two port reads have different typed Factor handles.
func TestActivationFacadeOneInstancePathLaw(t *testing.T) {
	composition := NewComposition()
	left := coldFactor(composition, coldKey(981_001))
	right, rightOK := DeclareFactor(composition, FactorSpec[uint32, uint64]{
		Semantic: coldKey(981_002), KeyEnd: 2, Lattice: coldUintLattice(), Default: 0,
		AdmitAt: func(uint32, uint64) bool { return true }, Fingerprint: func(value uint64) uint64 { return value },
	}, func(*Factor[uint32, uint64]) bool { return true })
	leftRead, leftReadOK := ExactReadForm(left)
	rightRead, rightReadOK := ExactReadForm(right)
	leftWrite, leftWriteOK := ExactWriteForm(left)
	if left == nil || !rightOK || right == nil || !leftReadOK || !rightReadOK || !leftWriteOK {
		t.Fatal("factor declarations")
	}

	var prototype *Rule[uint64, ruleUnit]
	var prototypeLeftRead, prototypeRightRead Read[OrderedCells[uint64]]
	var prototypeWriteToken Write[uint64]
	prototype, prototypeOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(981_003), OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Output: left.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](981_103), Transfer: func(Access[uint64, ruleUnit]) bool { return true },
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		if !inputOK {
			return false
		}
		var leftOK, rightOK, writeOK bool
		prototypeLeftRead, leftOK = ReadFrom(rule, input, leftRead)
		prototypeRightRead, rightOK = ReadFrom(rule, input, rightRead)
		prototypeWriteToken, writeOK = WriteTo(rule, leftWrite)
		return leftOK && rightOK && writeOK
	})
	if !prototypeOK || prototype == nil {
		t.Fatal("prototype rule")
	}
	var seedWrite Write[uint64]
	seed, seedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(981_020), OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Output: left.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](981_120),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var declared bool
		seedWrite, declared = WriteTo(rule, leftWrite)
		return declared
	})
	if !seedOK || seed == nil {
		t.Fatal("activation demand seed")
	}
	family, familyOK := DeclareActivationFamily(composition, coldKey(981_004))
	application, target, endpoint := coldKey(981_005), coldKey(981_006), coldKey(981_007)
	activationRuns := 0
	activationSelected := false
	trigger, triggerOK := DeclareActivationRule(composition, ActivationRuleSpec{
		Semantic: coldKey(981_008), Family: family, Inputs: 0, Admission: AdmitActivationByTrustedTheorem(coldKey(981_108)),
		Run: func(value Activation) bool {
			activationRuns++
			selectedApplication, projected := ActivationApplication(value)
			activationSelected = projected && selectedApplication == application && Activate(value, selectedApplication, target, endpoint)
			return activationSelected
		},
	})
	if !familyOK || !triggerOK || trigger == nil {
		t.Fatal("activation rule")
	}
	query, queryRead, queryOK := declareColdQueryInstance(composition, coldKey(981_009), coldKey(981_109), leftRead)
	if !queryOK || query == nil {
		t.Fatal("query")
	}

	if !composition.Seal() {
		t.Fatal("composition seal")
	}
	leftRef, leftRefOK := left.Ref(1)
	rightRef, rightRefOK := right.Ref(1)
	prototypeLeftRef, prototypeLeftRefOK := left.Ref(0)
	role, leftSlot, rightSlot := coldKey(981_015), coldKey(981_016), coldKey(981_017)
	payload, payloadOK := NewActivationPrototypeInstance(prototype, ruleUnitForSemantic(coldKey(981_010)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstancePortRead(binding, prototypeLeftRead, role, leftSlot) &&
			InstancePortRead(binding, prototypeRightRead, role, rightSlot) &&
			InstanceWrite(binding, prototypeWriteToken, prototypeLeftRef)
	})
	seedInstance, seedInstanceOK := NewRuleInstance(seed, ruleUnitForSemantic(coldKey(981_021)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, seedWrite, leftRef)
	})
	if !leftRefOK || !rightRefOK || !prototypeLeftRefOK || !payloadOK || payload == nil || !seedInstanceOK || seedInstance == nil {
		t.Fatal("typed source instances")
	}
	build := NewSourceAssembly(composition)
	batch := build.state.batch
	scope, scopeOK := build.Scope()
	truth, truthOK := build.TrueExpr()
	falsity, falsityOK := build.FalseExpr()
	portSite, portOK := build.Site(coldKey(981_011), scope, truth, true)
	triggerSite, triggerSiteOK := build.Site(coldKey(981_012), scope, truth, true)
	localSite, localSiteOK := build.Site(coldKey(981_013), scope, falsity, false)
	triggerOccurrence, triggerOccurrenceOK := build.At(triggerSite)
	triggerOperand, triggerOperandOK := build.Operand(triggerOccurrence, coldKey(981_014))
	seedOccurrence, seedOccurrenceOK := build.Relation(triggerSite, coldKey(981_022))
	seedPrepared, seedOperandOK := build.PrepareInstance(seedOccurrence, seedInstance)
	admission, admissionOK := ActivationPrototypeAdmissionFor(coldKey(981_013), payload)
	prepared, staged := StageActivationPlan(build, composition, family, []ActivationPlanEntry{{
		Target: target, Endpoint: endpoint, PortRole: role, Provenance: coldKey(981_018), Prototype: admission,
	}})
	if premature, finalized := FinalizeActivationPlan(build, prepared); finalized || premature != nil {
		t.Fatal("activation plan finalized before its source build sealed")
	}
	foreignBuild := NewSourceAssembly(composition)
	if foreign, finalized := FinalizeActivationPlan(foreignBuild, prepared); finalized || foreign != nil {
		t.Fatal("activation plan finalized from a foreign source build")
	}
	sealed := build.Seal()
	if !scopeOK || !truthOK || !falsityOK || !portOK || !triggerSiteOK || !localSiteOK || !triggerOccurrenceOK || !triggerOperandOK || !seedOccurrenceOK || !seedOperandOK || !admissionOK || !staged || !sealed {
		t.Fatalf("source-build admission/seal port=%t trigger=%t local=%t triggerOccurrence=%t triggerOperand=%t seedOccurrence=%t seedOperand=%t admission=%t staged=%t sealed=%t", portOK, triggerSiteOK, localSiteOK, triggerOccurrenceOK, triggerOperandOK, seedOccurrenceOK, seedOperandOK, admissionOK, staged, sealed)
	}
	plan, planOK := FinalizeActivationPlan(build, prepared)
	if !planOK || plan == nil || plan.EndpointCount() != 1 {
		t.Fatalf("sealed activation plan ok=%t plan=%p prototype=%t row=%t finalized=%t", planOK, plan, payload.state.activationPrototype != nil, payload.state.activationRow.Available(), prepared.finalized)
	}
	if gotTarget, gotEndpoint, endpointOK := plan.EndpointAt(0); !endpointOK || gotTarget != target || gotEndpoint != endpoint {
		t.Fatal("static endpoint route")
	}
	if replay, replayOK := FinalizeActivationPlan(build, prepared); replayOK || replay != nil {
		t.Fatal("prepared activation plan finalized twice")
	}

	solver, assembled := assemble(composition, batch, func(assembly *Assembly) bool {
		assembly.sourceAssembly = build
		// This unbound point deliberately precedes the imported payload point.
		// The port must follow its issued ActivationBase, not a caller-predicted
		// declaration ordinal.
		prefixPoint := admitPoint(assembly, localSite.value)
		portPoint := admitPoint(assembly, portSite.value)
		triggerPoint := admitPoint(assembly, triggerSite.value)
		base, baseOK := ActivationBaseAt(assembly, portPoint)
		port, portCreated := NewActivationPort(role, base)
		readsBound := portCreated && AddActivationPortRead(port, rightSlot, right, rightRef) && AddActivationPortRead(port, leftSlot, left, leftRef)
		triggerInstance, triggerCreated := NewActivationTrigger(trigger, application, plan, []*ActivationPort{port}, func(*StructuralBinding) bool { return true })
		if !leftRefOK || !rightRefOK || !baseOK || !readsBound || !triggerCreated || triggerInstance == nil {
			return false
		}
		seedMember := admitInstance(assembly, triggerPoint, seedPrepared.occurrence.value, seedPrepared.operand.value, seedInstance)
		member := admitStructural(assembly, triggerPoint, triggerOccurrence.value, triggerOperand.value, triggerInstance)
		if prefixPoint == nil || portPoint == nil || triggerPoint == nil {
			t.Fatal("assembly points")
		}
		if seedMember == nil || member == nil {
			t.Fatal("structural trigger")
		}
		if admitGroup(assembly, triggerPoint, seedMember, member) == nil {
			t.Fatal("trigger group")
		}
		queryInstance, queryInstanceOK := NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, queryRead, leftRef)
		})
		return queryInstanceOK && admitQueryAt(assembly, triggerPoint, queryInstance) != nil
	})
	if !assembled || solver == nil {
		t.Fatal("activation façade assembly")
	}
	if state, status := solver.Solve(context.Background()); status != SolveComplete || state == nil || len(solver.accepted) != 1 {
		t.Fatalf("activation solve status=%v accepted=%d runs=%d selected=%t", status, len(solver.accepted), activationRuns, activationSelected)
	}
	graph, graphOK := solver.compiler.topology.Graph(solver.accepted)
	if !graphOK || graph == nil {
		t.Fatal("accepted activation graph")
	}
	callerLeft := equation.Surface{Factor: left.schema.semantic.compositionKey(), Form: equation.SurfaceReadExact, Local: 2}
	callerRight := equation.Surface{Factor: right.schema.semantic.compositionKey(), Form: equation.SurfaceReadExact, Local: 2}
	seen := false
	for index := 0; index < graph.GroupCount(); index++ {
		group, groupOK := graph.HyperedgeAt(index)
		if !groupOK || group.MemberCount() != 1 {
			continue
		}
		member, memberOK := group.MemberAt(0)
		if !memberOK {
			continue
		}
		if _, selected := member.ActivationMember(); !selected {
			continue
		}
		first, firstOK := member.ReadAt(0)
		second, secondOK := member.ReadAt(1)
		if !firstOK || !secondOK || first != callerLeft || second != callerRight {
			t.Fatal("selected member did not receive canonical typed port reads")
		}
		seen = true
	}
	if !seen {
		t.Fatal("selected activation member")
	}
}
