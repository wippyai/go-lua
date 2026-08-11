package equation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
)

func ambientPortPoints(t testing.TB, scopes ...Scope) map[PointRef]Point {
	t.Helper()
	batch := NewBatch()
	sites := make([]Site, len(scopes))
	for index, scope := range scopes {
		var ok bool
		sites[index], ok = batch.AdmitSite(boundaryKey(byte(130+index)), scope, TrueExpr(), InitPresent)
		if !ok {
			t.Fatalf("port Site %d", index)
		}
	}
	if !batch.Seal() {
		t.Fatal("port batch")
	}
	points := make(map[PointRef]Point, len(sites))
	for index, site := range sites {
		point, ok := derivePoint(site)
		if !ok {
			t.Fatalf("port Point %d", index)
		}
		points[PointAt(index)] = point
	}
	return points
}

func TestActivationAttachmentAmbientScopeLaw(t *testing.T) {
	role := boundaryKey(140)
	plan := VariantPlan{data: &variantPlanData{
		// The same immutable plan is reused by each attachment below.  No
		// application coordinate can enter this catalog.
		ports:    map[composition.Key]PortMode{role: PortImport},
		variants: []sealedVariant{{}, {}},
	}}
	outer := boundaryDecision(t, 141)
	loop := boundaryDecision(t, 142)
	outerScope, outerOK := NewScope(outer)
	loopScope, loopOK := NewScope(outer, loop)
	if !outerOK || !loopOK {
		t.Fatal("ambient scopes")
	}
	for _, expected := range []Scope{EmptyScope(), outerScope, loopScope} {
		base := ambientPortPoints(t, expected)
		ports, ambient, ok := sealPlanPortBindings(plan, []PortBinding{{Role: role, Base: PointAt(0)}}, base)
		if !ok || !sameScope(ambient, expected) || len(ports) != 1 || ports[role].point.Scope().Key() != expected.Key() {
			t.Fatal("attachment did not retain its exact ambient scope")
		}
		if len(plan.data.variants) != 2 || len(plan.data.ports) != 1 {
			t.Fatal("attachment changed shared endpoint catalog")
		}
	}
}

func TestActivationAttachmentRejectsPortScopeMismatch(t *testing.T) {
	leftRole, rightRole := boundaryKey(143), boundaryKey(144)
	plan := VariantPlan{data: &variantPlanData{ports: map[composition.Key]PortMode{
		leftRole: PortImport, rightRole: PortExport,
	}}}
	outer := boundaryDecision(t, 145)
	loop := boundaryDecision(t, 146)
	left, leftOK := NewScope(outer)
	right, rightOK := NewScope(outer, loop)
	if !leftOK || !rightOK {
		t.Fatal("mismatch scopes")
	}
	base := ambientPortPoints(t, left, right)
	if ports, ambient, ok := sealPlanPortBindings(plan, []PortBinding{
		{Role: rightRole, Base: PointAt(1)},
		{Role: leftRole, Base: PointAt(0)},
	}, base); ok || ports != nil || ambient.Available() {
		t.Fatal("different port worlds were silently unioned")
	}
}

func TestPrototypePortBoundaryIsFormalEmptyScope(t *testing.T) {
	role := boundaryKey(147)
	formal, formalOK := prototypeFragmentScope(FragmentPoint{Port: role}, nil)
	empty := EmptyScope()
	reindex, reindexOK := NewReindex(empty, empty, nil)
	decision := boundaryDecision(t, 148)
	guard, guardOK := DecisionExpr(decision)
	if !formalOK || !reindexOK || !guardOK || !sameScope(formal, empty) {
		t.Fatal("formal port fixture")
	}
	valid := FragmentInput{Provenance: boundaryKey(149), Pre: TrueExpr(), Reindex: reindex, Post: FalseExpr()}
	if !validPrototypeFragmentInput(valid, formal, formal) {
		t.Fatal("terminal port predicate rejected")
	}
	valid.Pre = guard
	if validPrototypeFragmentInput(valid, formal, formal) {
		t.Fatal("port precondition captured an ambient decision")
	}
	valid.Pre = TrueExpr()
	valid.Post = guard
	if validPrototypeFragmentInput(valid, formal, formal) {
		t.Fatal("port postcondition captured an ambient decision")
	}
}

func TestAmbientLiftCarriesGuardThroughLocalFragment(t *testing.T) {
	outer := boundaryDecision(t, 150)
	local := boundaryDecision(t, 151)
	ambient, ambientOK := NewScope(outer)
	localScope, localOK := NewScope(local)
	premise, premiseOK := DecisionExpr(outer)
	if !ambientOK || !localOK || !premiseOK {
		t.Fatal("ambient lift fixture")
	}

	batch := NewBatch()
	portSite, portOK := batch.AdmitSite(boundaryKey(152), ambient, TrueExpr(), InitPresent)
	localSite, localSiteOK := batch.AdmitSite(boundaryKey(153), localScope, FalseExpr(), InitAbsent)
	occurrence, occurrenceOK := batch.At(localSite)
	operand, operandOK := batch.AdmitOperand(occurrence, boundaryKey(154))
	if !portOK || !localSiteOK || !occurrenceOK || !operandOK || !batch.Seal() {
		t.Fatal("ambient lift batch")
	}
	portPoint, portPointOK := derivePoint(portSite)
	localPoint, localPointOK := derivePoint(localSite)
	emptyToLocal, transportOK := NewReindex(EmptyScope(), localScope, nil)
	if !portPointOK || !localPointOK || !transportOK {
		t.Fatal("ambient lift points")
	}

	binding := boundaryKey(155)
	role := boundaryKey(156)
	member := Member{owner: &Topology{}, binding: binding, locator: PairLocator{Application: boundaryKey(157), Target: boundaryKey(158), Endpoint: boundaryKey(159)}}
	template := sealedTemplate{
		key:     boundaryKey(160),
		batch:   batch,
		ambient: ambient,
		value: Template{
			Points: []PointSpec{{Site: localSite}},
			Rules:  []RuleInstance{{Schema: boundaryKey(161), OperandFamily: boundaryKey(162), Occurrence: occurrence, Operand: operand}},
			Groups: []FragmentGroup{{
				Members: []RuleRef{RuleAt(0)},
				Output:  FragmentPoint{Local: PointAt(0)},
				Inputs: []FragmentInput{{
					Point: FragmentPoint{Port: role}, Provenance: boundaryKey(163),
					Pre: TrueExpr(), Reindex: emptyToLocal, Post: TrueExpr(),
				}},
			}},
		},
		instances: []canonicalInstance{{key: boundaryKey(164)}},
		points:    map[PointRef]Point{PointAt(0): localPoint},
		ports: map[composition.Key]sealedPort{
			role: {role: role, base: PointAt(0), point: portPoint, mode: PortImport},
		},
	}
	spec := TopologySpec{Batch: batch, Points: []PointSpec{{Site: portSite}}}
	if !template.appendMember(&spec, binding, member, premise) || len(spec.Groups) != 1 || len(spec.Points) != 2 {
		t.Fatal("ambient fragment did not materialize")
	}
	group := spec.Groups[0]
	output, outputOK := derivePoint(spec.Points[1].Site)
	if !outputOK || !sameExpr(group.premise, premise) || !output.Scope().contains(outer) {
		t.Fatal("accepted ambient premise did not reach dynamic output")
	}
	namespace, namespaceOK := memberNamespace(member)
	alpha, alphaOK := template.decisionAlpha(binding, namespace)
	fresh := alpha[local.Key()]
	if !namespaceOK || !alphaOK || !fresh.Available() || !output.Scope().contains(fresh) || output.Scope().contains(local) {
		t.Fatal("local scope was not alpha-instantiated beneath ambient")
	}
	omega := group.Inputs[0].Reindex()
	mapping, mappingOK := omega.At(0)
	if !mappingOK || omega.Count() != 1 || mapping.Disposition != DecisionIdentity || mapping.Source != outer || mapping.Target != outer || !sameScope(omega.Source(), ambient) || !omega.Target().contains(outer) || !omega.Target().contains(fresh) {
		t.Fatal("port transport did not lift ambient identity")
	}
}

func TestActivationAlphaIsFreshAndDeclarationOrderInvariant(t *testing.T) {
	first, second, outer := boundaryDecision(t, 165), boundaryDecision(t, 166), boundaryDecision(t, 167)
	firstScope, firstOK := NewScope(first)
	secondScope, secondOK := NewScope(second)
	ambient, ambientOK := NewScope(outer)
	if !firstOK || !secondOK || !ambientOK {
		t.Fatal("alpha fixture scopes")
	}
	batch := NewBatch()
	firstSite, firstSiteOK := batch.AdmitSite(boundaryKey(168), firstScope, FalseExpr(), InitAbsent)
	secondSite, secondSiteOK := batch.AdmitSite(boundaryKey(169), secondScope, FalseExpr(), InitAbsent)
	if !firstSiteOK || !secondSiteOK || !batch.Seal() {
		t.Fatal("alpha fixture batch")
	}
	binding, memberOne, memberTwo := boundaryKey(170), boundaryKey(171), boundaryKey(172)
	forward := sealedTemplate{key: boundaryKey(173), value: Template{Points: []PointSpec{{Site: firstSite}, {Site: secondSite}}}}
	reversed := sealedTemplate{key: boundaryKey(173), value: Template{Points: []PointSpec{{Site: secondSite}, {Site: firstSite}}}}
	alphaOne, oneOK := forward.decisionAlpha(binding, memberOne)
	alphaReordered, reorderedOK := reversed.decisionAlpha(binding, memberOne)
	alphaTwo, twoOK := forward.decisionAlpha(binding, memberTwo)
	if !oneOK || !reorderedOK || !twoOK || alphaOne[first.Key()] != alphaReordered[first.Key()] || alphaOne[second.Key()] != alphaReordered[second.Key()] || alphaOne[first.Key()] == alphaTwo[first.Key()] {
		t.Fatal("alpha depended on declaration order or leaked across members")
	}
	boundForward, boundForwardOK := boundScope(firstScope, ambient, alphaOne)
	boundAgain, boundAgainOK := boundScope(firstScope, ambient, alphaReordered)
	if !boundForwardOK || !boundAgainOK || !sameScope(boundForward, boundAgain) || !boundForward.contains(outer) || !boundForward.contains(alphaOne[first.Key()]) {
		t.Fatal("ambient/local scope construction was not canonical")
	}
}

func TestEqualLocalUniverseSharesScopeButNotSiteOrPoint(t *testing.T) {
	outer, raw, fresh := boundaryDecision(t, 175), boundaryDecision(t, 176), boundaryDecision(t, 177)
	ambient, ambientOK := NewScope(outer)
	local, localOK := NewScope(raw)
	if !ambientOK || !localOK {
		t.Fatal("equal-universe scopes")
	}
	batch := NewBatch()
	left, leftOK := batch.AdmitSite(boundaryKey(178), local, FalseExpr(), InitAbsent)
	right, rightOK := batch.AdmitSite(boundaryKey(179), local, FalseExpr(), InitAbsent)
	if !leftOK || !rightOK || !batch.Seal() {
		t.Fatal("equal-universe batch")
	}
	table, tableOK := newMemberSiteTable(batch, ambient, decisionAlpha{raw.Key(): fresh}, boundaryKey(180), boundaryKey(181), 2)
	if !tableOK {
		t.Fatal("equal-universe table")
	}
	boundLeft, leftBound := table.bind(left)
	boundRight, rightBound := table.bind(right)
	leftPoint, leftPointOK := derivePoint(boundLeft.site)
	rightPoint, rightPointOK := derivePoint(boundRight.site)
	if !leftBound || !rightBound || !leftPointOK || !rightPointOK || !sameScope(boundLeft.scope, boundRight.scope) || !boundLeft.scope.contains(outer) || !boundLeft.scope.contains(fresh) {
		t.Fatal("equal formal local scopes did not canonically share S union alpha(L)")
	}
	if boundLeft.site.Same(boundRight.site) || leftPoint.Key() == rightPoint.Key() {
		t.Fatal("canonical scope incorrectly collapsed distinct source Sites or Points")
	}
}

func reindexMapping(plan Reindex, source Decision) (DecisionMap, bool) {
	for index := 0; index < plan.Count(); index++ {
		mapping, ok := plan.At(index)
		if ok && mapping.Source == source {
			return mapping, true
		}
	}
	return DecisionMap{}, false
}

func TestAmbientReindexMatrixLaw(t *testing.T) {
	outer := boundaryDecision(t, 182)
	sourceRaw, targetRaw := boundaryDecision(t, 183), boundaryDecision(t, 184)
	sourceFresh, targetFresh := boundaryDecision(t, 185), boundaryDecision(t, 186)
	ambient, ambientOK := NewScope(outer)
	sourceScope, sourceScopeOK := NewScope(sourceRaw)
	targetScope, targetScopeOK := NewScope(targetRaw)
	if !ambientOK || !sourceScopeOK || !targetScopeOK {
		t.Fatal("reindex matrix scopes")
	}
	batch := NewBatch()
	sourceSite, sourceSiteOK := batch.AdmitSite(boundaryKey(187), sourceScope, FalseExpr(), InitAbsent)
	targetSite, targetSiteOK := batch.AdmitSite(boundaryKey(188), targetScope, FalseExpr(), InitAbsent)
	portLeft, leftOK := batch.AdmitSite(boundaryKey(189), ambient, TrueExpr(), InitPresent)
	portRight, rightOK := batch.AdmitSite(boundaryKey(190), ambient, TrueExpr(), InitPresent)
	externalTarget, externalTargetOK := batch.AdmitSite(boundaryKey(193), EmptyScope(), FalseExpr(), InitAbsent)
	if !sourceSiteOK || !targetSiteOK || !leftOK || !rightOK || !externalTargetOK || !batch.Seal() {
		t.Fatal("reindex matrix batch")
	}
	alpha := decisionAlpha{sourceRaw.Key(): sourceFresh, targetRaw.Key(): targetFresh}
	table, tableOK := newMemberSiteTable(batch, ambient, alpha, boundaryKey(191), boundaryKey(192), 2)
	boundSource, sourceOK := table.bind(sourceSite)
	boundTarget, targetOK := table.bind(targetSite)
	if !tableOK || !sourceOK || !targetOK {
		t.Fatal("reindex matrix dynamic sites")
	}
	localSource := templateResolvedPoint{ref: PointAt(0), site: boundSource.site, scope: boundSource.scope, rawScope: sourceScope, local: true}
	localTarget := templateResolvedPoint{ref: PointAt(1), site: boundTarget.site, scope: boundTarget.scope, rawScope: targetScope, local: true}
	leftPort := templateResolvedPoint{ref: PointAt(2), site: portLeft, scope: ambient, rawScope: EmptyScope()}
	rightPort := templateResolvedPoint{ref: PointAt(3), site: portRight, scope: ambient, rawScope: EmptyScope()}

	forgetRaw, forgetOK := NewReindex(sourceScope, EmptyScope(), []DecisionMap{Forget(sourceRaw)})
	forgotten, forgottenOK := boundReindex(forgetRaw, localSource, rightPort, ambient, alpha)
	forgottenAmbient, forgottenAmbientOK := reindexMapping(forgotten, outer)
	forgottenLocal, forgottenLocalOK := reindexMapping(forgotten, sourceFresh)
	if !forgetOK || !forgottenOK || forgotten.Count() != 2 || !forgottenAmbientOK || forgottenAmbient.Disposition != DecisionIdentity || !forgottenLocalOK || forgottenLocal.Disposition != DecisionForget || !sameScope(forgotten.Source(), localSource.scope) || !sameScope(forgotten.Target(), ambient) {
		t.Fatal("local-to-port Forget failed to preserve ambient identity")
	}

	renameRaw, renameOK := NewReindex(sourceScope, targetScope, []DecisionMap{Rename(sourceRaw, targetRaw)})
	renamed, renamedOK := boundReindex(renameRaw, localSource, localTarget, ambient, alpha)
	renamedAmbient, renamedAmbientOK := reindexMapping(renamed, outer)
	renamedLocal, renamedLocalOK := reindexMapping(renamed, sourceFresh)
	if !renameOK || !renamedOK || renamed.Count() != 2 || !renamedAmbientOK || renamedAmbient.Disposition != DecisionIdentity || !renamedLocalOK || renamedLocal.Disposition != DecisionRename || renamedLocal.Target != targetFresh {
		t.Fatal("local-to-local Rename failed to preserve ambient identity")
	}

	rawTargetExpr, rawTargetExprOK := DecisionExpr(targetRaw)
	substituteRaw, substituteOK := NewReindex(sourceScope, targetScope, []DecisionMap{Substitute(sourceRaw, rawTargetExpr)})
	substituted, substitutedOK := boundReindex(substituteRaw, localSource, localTarget, ambient, alpha)
	substitutedAmbient, substitutedAmbientOK := reindexMapping(substituted, outer)
	substitutedLocal, substitutedLocalOK := reindexMapping(substituted, sourceFresh)
	freshTargetExpr, freshTargetExprOK := DecisionExpr(targetFresh)
	if !rawTargetExprOK || !substituteOK || !substitutedOK || substituted.Count() != 2 || !substitutedAmbientOK || substitutedAmbient.Disposition != DecisionIdentity || !substitutedLocalOK || substitutedLocal.Disposition != DecisionSubstitute || !freshTargetExprOK || !sameExpr(substitutedLocal.Expr, freshTargetExpr) {
		t.Fatal("local-to-local Substitute failed to preserve ambient identity")
	}

	portIdentityRaw, portIdentityOK := NewReindex(EmptyScope(), EmptyScope(), nil)
	portIdentity, portIdentityBound := boundReindex(portIdentityRaw, leftPort, rightPort, ambient, alpha)
	portMap, portMapOK := reindexMapping(portIdentity, outer)
	if !portIdentityOK || !portIdentityBound || portIdentity.Count() != 1 || !portMapOK || portMap.Disposition != DecisionIdentity || !portIdentity.Identity() {
		t.Fatal("port-to-port relation failed to lift the ambient identity")
	}

	external := templateResolvedPoint{ref: PointAt(4), site: externalTarget, scope: EmptyScope(), rawScope: EmptyScope()}
	portToExternalRaw, portToExternalRawOK := NewReindex(EmptyScope(), EmptyScope(), nil)
	portToExternal, portToExternalOK := boundReindex(portToExternalRaw, leftPort, external, ambient, alpha)
	liftedForget, liftedForgetOK := reindexMapping(portToExternal, outer)
	if !portToExternalRawOK || !portToExternalOK || portToExternal.Count() != 1 || !liftedForgetOK || liftedForget.Disposition != DecisionForget {
		t.Fatal("port-to-external relation did not forget its lifted source-only ambient decision")
	}

	authoredSource := templateResolvedPoint{ref: PointAt(5), site: portLeft, scope: ambient, rawScope: ambient}
	authoredRaw, authoredRawOK := NewReindex(ambient, EmptyScope(), []DecisionMap{Forget(outer)})
	authored, authoredOK := boundReindex(authoredRaw, authoredSource, external, ambient, alpha)
	authoredForget, authoredForgetOK := reindexMapping(authored, outer)
	if !authoredRawOK || !authoredOK || authored.Count() != 1 || !authoredForgetOK || authoredForget.Disposition != DecisionForget {
		t.Fatal("authored source-only mapping was duplicated or replaced by the ambient lift")
	}
}

func TestAmbientScopeSurvivesPlanSealAndSelectedGraph(t *testing.T) {
	factor, secondary := boundaryKey(180), boundaryKey(179)
	templateRule, templateFamily, templateProof := boundaryKey(181), boundaryKey(182), boundaryKey(183)
	triggerRule, triggerFamily, triggerProof := boundaryKey(184), boundaryKey(185), boundaryKey(186)
	activationFamily := boundaryKey(187)
	query, freezer := boundaryKey(188), boundaryKey(189)
	source, sourceOK := composition.Seal(composition.Candidate{
		Factors:            []composition.Factor{{Key: factor}, {Key: secondary}},
		ActivationFamilies: []composition.ActivationFamily{{Semantic: activationFamily}},
		Rules: []composition.Rule{
			{
				Key: templateRule, OperandFamily: templateFamily,
				Admission:  composition.Admission{Kind: composition.AdmissionTrustedTheorem, Identity: templateProof},
				OutputKind: composition.FactorOutput, Output: factor, Inputs: 2,
				Reads: []composition.Read{
					{Kind: composition.ReadExact, Input: 1, Factor: factor},
					{Kind: composition.ReadExact, Input: 1, Factor: secondary},
				},
				Writes: []composition.Write{{Kind: composition.WriteExact, Factor: factor}},
			},
			{
				Key: triggerRule, OperandFamily: triggerFamily,
				Admission:   composition.Admission{Kind: composition.AdmissionTrustedTheorem, Identity: triggerProof},
				OutputKind:  composition.StructuralOutput,
				Activations: []composition.ActivationRange{{Family: activationFamily}},
			},
		},
		Queries: []composition.QueryFamily{{
			Key: query, Freezer: freezer,
			Projections: []composition.QueryProjection{{Kind: composition.QueryFactorExact, Factor: factor}},
		}},
	})
	if !sourceOK || source == nil {
		t.Fatal("ambient source composition")
	}

	outer := boundaryDecision(t, 190)
	local := boundaryDecision(t, 191)
	ambient, ambientOK := NewScope(outer)
	localScope, localOK := NewScope(local)
	premise, premiseOK := DecisionExpr(outer)
	if !ambientOK || !localOK || !premiseOK {
		t.Fatal("ambient source scopes")
	}
	batch := NewBatch()
	portSite, portOK := batch.AdmitSite(boundaryKey(192), ambient, TrueExpr(), InitPresent)
	triggerSite, triggerSiteOK := batch.AdmitSite(boundaryKey(193), ambient, TrueExpr(), InitPresent)
	localSite, localSiteOK := batch.AdmitSite(boundaryKey(194), localScope, TrueExpr(), InitPresent)
	triggerOccurrence, triggerOccurrenceOK := batch.At(triggerSite)
	localOccurrence, localOccurrenceOK := batch.At(localSite)
	triggerOperand, triggerOperandOK := batch.AdmitOperand(triggerOccurrence, boundaryKey(195))
	localOperand, localOperandOK := batch.AdmitOperand(localOccurrence, boundaryKey(196))
	if !portOK || !triggerSiteOK || !localSiteOK || !triggerOccurrenceOK || !localOccurrenceOK || !triggerOperandOK || !localOperandOK || !batch.Seal() {
		t.Fatal("ambient source batch")
	}

	role, readSlot, secondarySlot := boundaryKey(197), boundaryKey(207), boundaryKey(209)
	forgetLocal, forgetOK := NewReindex(localScope, EmptyScope(), []DecisionMap{Forget(local)})
	emptyIdentity, emptyIdentityOK := NewReindex(EmptyScope(), EmptyScope(), nil)
	read := Surface{Factor: factor, Form: SurfaceReadExact, Local: 1}
	callerRead := Surface{Factor: factor, Form: SurfaceReadExact, Local: 2}
	secondaryRead := Surface{Factor: secondary, Form: SurfaceReadExact, Local: 1}
	callerSecondaryRead := Surface{Factor: secondary, Form: SurfaceReadExact, Local: 2}
	write := Surface{Factor: factor, Form: SurfaceWriteExact, Local: 1, Mode: TargetModeWeak}
	if !forgetOK || !emptyIdentityOK {
		t.Fatal("local-to-port transport")
	}
	templateValue := Template{
		Rules: []RuleInstance{{
			Schema: templateRule, OperandFamily: templateFamily, Occurrence: localOccurrence, Operand: localOperand,
			Reads: []ResolvedRead{{Index: 0, Surface: read}, {Index: 1, Surface: secondaryRead}}, Writes: []ResolvedWrite{{Index: 0, Surface: write}},
		}},
		Points: []PointSpec{{Site: localSite}},
		Ports: []Port{{Role: role, Mode: PortImportExport, Reads: []PortRead{
			{Role: readSlot, Surface: read}, {Role: secondarySlot, Surface: secondaryRead},
		}}},
		Groups: []FragmentGroup{{
			Members: []RuleRef{RuleAt(0)}, Output: FragmentPoint{Port: role},
			Inputs: []FragmentInput{
				{Point: FragmentPoint{Local: PointAt(0)}, Provenance: boundaryKey(200), Pre: TrueExpr(), Reindex: forgetLocal, Post: TrueExpr()},
				{Point: FragmentPoint{Port: role}, Provenance: boundaryKey(208), Pre: TrueExpr(), Reindex: emptyIdentity, Post: TrueExpr()},
			},
		}},
		WeakTargets: []WeakTargetMapping{{Surface: write, Candidates: []Surface{read, {Factor: factor, Form: SurfaceReadExact, Local: 3}}}},
	}
	plan, planOK := NewVariantPlan(source, activationFamily, []Variant{{
		Target: boundaryKey(198), Endpoint: boundaryKey(199), Template: templateValue,
	}})
	if !planOK {
		t.Fatal("scope-polymorphic variant plan")
	}
	wrongRoute := copyTemplate(templateValue)
	wrongRoute.Groups[0].Inputs[0], wrongRoute.Groups[0].Inputs[1] = wrongRoute.Groups[0].Inputs[1], wrongRoute.Groups[0].Inputs[0]
	if rejected, accepted := NewVariantPlan(source, activationFamily, []Variant{{Target: boundaryKey(198), Endpoint: boundaryKey(199), Template: wrongRoute}}); accepted || rejected != (VariantPlan{}) {
		t.Fatal("prototype read crossed to a different fragment input")
	}
	unusedSlot := copyTemplate(templateValue)
	unusedSlot.Ports[0].Reads = append(unusedSlot.Ports[0].Reads, PortRead{Role: boundaryKey(210), Surface: callerRead})
	if rejected, accepted := NewVariantPlan(source, activationFamily, []Variant{{Target: boundaryKey(198), Endpoint: boundaryKey(199), Template: unusedSlot}}); accepted || rejected != (VariantPlan{}) {
		t.Fatal("unused prototype read slot sealed")
	}
	topology, topologyOK := SealTopology(source, TopologySpec{
		Batch:   batch,
		Rules:   []RuleInstance{{Schema: triggerRule, OperandFamily: triggerFamily, Occurrence: triggerOccurrence, Operand: triggerOperand}},
		Points:  []PointSpec{{Site: portSite}, {Site: triggerSite}},
		Groups:  []Group{{Members: []RuleRef{RuleAt(0)}, Output: PointAt(1)}},
		Queries: []QueryInstance{{Family: query, Point: PointAt(1), Surfaces: []Surface{read}}},
		ActivationBindings: []ActivationBinding{{
			Family: activationFamily, Trigger: RuleAt(0), Application: boundaryKey(201), Plan: plan,
			PortBindings: []PortBinding{{Role: role, Base: PointAt(0), Reads: []PortRead{
				{Role: secondarySlot, Surface: callerSecondaryRead}, {Role: readSlot, Surface: callerRead},
			}}},
		}},
	})
	if !topologyOK || topology == nil || len(topology.bindings) != 1 || !sameScope(topology.bindings[0].ambient, ambient) {
		t.Fatal("ambient binding did not seal")
	}
	member, memberOK := topology.SelectMember(topology.bindings[0].trigger, PairLocator{Application: boundaryKey(201), Target: boundaryKey(198), Endpoint: boundaryKey(199)})
	accepted, acceptedOK := topology.Accept(member, premise)
	graph, graphOK := topology.Graph([]AcceptedMember{accepted})
	if !memberOK || !acceptedOK || !graphOK || graph == nil {
		t.Fatal("selected ambient fragment did not compile")
	}
	seen := false
	for index := 0; index < graph.GroupCount(); index++ {
		group, groupOK := graph.HyperedgeAt(index)
		if !groupOK || group.MemberCount() != 1 {
			continue
		}
		member, dynamic := group.MemberAt(0)
		if !dynamic {
			continue
		}
		if _, selected := member.ActivationMember(); !selected {
			continue
		}
		seen = true
		boundRead, readOK := member.ReadAt(0)
		boundSecondaryRead, secondaryReadOK := member.ReadAt(1)
		if !readOK || !secondaryReadOK || boundRead != callerRead || boundSecondaryRead != callerSecondaryRead {
			t.Fatal("selected member did not receive its declared caller read slots")
		}
		if !sameExpr(group.Premise(), premise) || !group.Output().Scope().contains(outer) {
			t.Fatal("selected group dropped its ambient premise")
		}
		input, inputOK := group.InputAt(0)
		if !inputOK || input.Reindex().Count() != 2 {
			t.Fatal("selected local-to-port reindex omitted ambient identity")
		}
	}
	if count, weakOK := graph.WeakTargetCandidateCount(write); !weakOK || count != 2 {
		t.Fatal("weak candidate substitution did not preserve set coverage")
	} else {
		candidate, candidateOK := graph.WeakTargetCandidateAt(write, 0)
		static, staticOK := graph.WeakTargetCandidateAt(write, 1)
		if !candidateOK || !staticOK || candidate != callerRead || static != (Surface{Factor: factor, Form: SurfaceReadExact, Local: 3}) {
			t.Fatal("weak candidates did not receive canonical caller substitution")
		}
	}
	if !seen {
		t.Fatal("selected dynamic group missing from graph")
	}
}

func TestSealTopologyDeduplicatesSharedExportReverseIncidence(t *testing.T) {
	factorA, factorB := boundaryKey(240), boundaryKey(241)
	activationFamily := boundaryKey(242)
	triggerRule, triggerFamily, triggerProof := boundaryKey(243), boundaryKey(244), boundaryKey(245)
	query, freezer := boundaryKey(220), boundaryKey(221)
	source, sourceOK := composition.Seal(composition.Candidate{
		Factors:            []composition.Factor{{Key: factorA}, {Key: factorB}},
		ActivationFamilies: []composition.ActivationFamily{{Semantic: activationFamily}},
		Rules: []composition.Rule{{
			Key: triggerRule, OperandFamily: triggerFamily,
			Admission:   composition.Admission{Kind: composition.AdmissionTrustedTheorem, Identity: triggerProof},
			OutputKind:  composition.StructuralOutput,
			Activations: []composition.ActivationRange{{Family: activationFamily}},
		}},
		Queries: []composition.QueryFamily{{
			Key: query, Freezer: freezer,
			Projections: []composition.QueryProjection{{Kind: composition.QueryFactorExact, Factor: factorA}},
		}},
	})
	if !sourceOK || source == nil {
		t.Fatal("shared-export source")
	}

	batch := NewBatch()
	sharedPort, sharedPortOK := batch.AdmitSite(boundaryKey(246), EmptyScope(), TrueExpr(), InitPresent)
	triggerSite, triggerSiteOK := batch.AdmitSite(boundaryKey(247), EmptyScope(), TrueExpr(), InitPresent)
	sourceA, sourceAOK := batch.AdmitSite(boundaryKey(248), EmptyScope(), TrueExpr(), InitPresent)
	sourceB, sourceBOK := batch.AdmitSite(boundaryKey(249), EmptyScope(), TrueExpr(), InitPresent)
	occurrence, occurrenceOK := batch.At(triggerSite)
	operand, operandOK := batch.AdmitOperand(occurrence, boundaryKey(250))
	identity, identityOK := NewReindex(EmptyScope(), EmptyScope(), nil)
	if !sharedPortOK || !triggerSiteOK || !sourceAOK || !sourceBOK || !occurrenceOK || !operandOK || !identityOK || !batch.Seal() {
		t.Fatal("shared-export batch")
	}

	leftRole, rightRole := boundaryKey(251), boundaryKey(252)
	template := Template{
		Ports: []Port{{Role: leftRole, Mode: PortExport}, {Role: rightRole, Mode: PortExport}},
		FactorEdges: []FragmentFactorEdge{
			{ExternalSource: sourceA, Target: FragmentPoint{Port: leftRole}, Factor: factorA, Provenance: boundaryKey(253), Pre: TrueExpr(), Reindex: identity, Post: TrueExpr()},
			{ExternalSource: sourceB, Target: FragmentPoint{Port: rightRole}, Factor: factorB, Provenance: boundaryKey(254), Pre: TrueExpr(), Reindex: identity, Post: TrueExpr()},
		},
	}
	plan, planOK := NewVariantPlan(source, activationFamily, []Variant{{
		Target: boundaryKey(255), Endpoint: boundaryKey(1), Template: template,
	}})
	if !planOK {
		t.Fatal("shared-export plan")
	}
	topology, topologyOK := SealTopology(source, TopologySpec{
		Batch:   batch,
		Rules:   []RuleInstance{{Schema: triggerRule, OperandFamily: triggerFamily, Occurrence: occurrence, Operand: operand}},
		Points:  []PointSpec{{Site: sharedPort}, {Site: triggerSite}, {Site: sourceA}, {Site: sourceB}},
		Groups:  []Group{{Members: []RuleRef{RuleAt(0)}, Output: PointAt(1)}},
		Queries: []QueryInstance{{Family: query, Point: PointAt(1), Surfaces: []Surface{{Factor: factorA, Form: SurfaceReadExact, Local: 1}}}},
		ActivationBindings: []ActivationBinding{{
			Family: activationFamily, Trigger: RuleAt(0), Application: boundaryKey(2), Plan: plan,
			PortBindings: []PortBinding{{Role: leftRole, Base: PointAt(0)}, {Role: rightRole, Base: PointAt(0)}},
		}},
	})
	if !topologyOK || topology == nil {
		t.Fatalf("shared export topology seal failed: ok=%v nil=%v", topologyOK, topology == nil)
	}
	if len(topology.reverses) != 1 {
		t.Fatalf("shared export reverse rows=%d, want 1", len(topology.reverses))
	}
	graph, graphOK := topology.Graph(nil)
	if !graphOK || graph == nil {
		t.Fatal("shared export base graph")
	}
	incidences := 0
	for _, triggers := range graph.activationReverses {
		incidences += len(triggers)
	}
	if incidences != 1 {
		t.Fatalf("shared export reverse incidence count=%d, want 1", incidences)
	}
}
