package equation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
)

func premiseTopology(t testing.TB) (*Topology, Member) {
	t.Helper()
	source := &composition.Composition{}
	family, binding, application := boundaryKey(250), boundaryKey(251), boundaryKey(252)
	target, endpoint := boundaryKey(253), boundaryKey(254)
	ambient, ambientOK := NewScope(boundaryDecision(t, 244), boundaryDecision(t, 245))
	if !ambientOK {
		t.Fatal("ambient")
	}
	plan := VariantPlan{data: &variantPlanData{
		source: source,
		family: family,
		key:    boundaryKey(255),
		variants: []sealedVariant{{
			target: target, endpoint: endpoint,
		}},
	}}
	topology := &Topology{source: source, key: boundaryKey(230), bindingAt: map[composition.Key]int{binding: 0}, bindings: []activationBinding{{
		key: binding, family: family, application: application, plan: plan,
		trigger: boundaryKey(241), triggerRule: boundaryKey(242), triggerAdmission: boundaryKey(243), ambient: ambient,
	}}}
	return topology, Member{owner: topology, binding: binding, locator: PairLocator{Application: application, Target: target, Endpoint: endpoint}}
}

func TestAcceptedMemberPremiseMergeLaw(t *testing.T) {
	topology, member := premiseTopology(t)
	leftDecision, rightDecision := boundaryDecision(t, 244), boundaryDecision(t, 245)
	left, leftOK := DecisionExpr(leftDecision)
	right, rightOK := DecisionExpr(rightDecision)
	if !leftOK || !rightOK {
		t.Fatal("premises")
	}
	acceptedLeft, leftAccepted := topology.Accept(member, left)
	acceptedRight, rightAccepted := topology.Accept(member, right)
	merged, mergedOK := topology.MergeAccepted(acceptedLeft, acceptedRight)
	want, wantOK := OrExpr(left, right)
	direct, directOK := topology.Accept(member, want)
	if !leftAccepted || !rightAccepted || !mergedOK || !wantOK || !directOK || !sameExpr(merged.Premise(), want) || merged.Evidence() != direct.Evidence() {
		t.Fatal("same Member did not retain P OR Q")
	}
	mergedRevision, mergedRevisionOK := topology.Revision([]AcceptedMember{merged})
	directRevision, directRevisionOK := topology.Revision([]AcceptedMember{direct})
	if !mergedRevisionOK || !directRevisionOK || mergedRevision != directRevision {
		t.Fatal("merged premise did not retain direct admission revision")
	}
	reversed, reversedOK := topology.MergeAccepted(acceptedRight, acceptedLeft)
	if !reversedOK || !sameExpr(reversed.Premise(), merged.Premise()) || reversed.Evidence() != merged.Evidence() {
		t.Fatal("premise merge was not commutative/canonical")
	}
	idempotent, idempotentOK := topology.MergeAccepted(acceptedLeft, acceptedLeft)
	if !idempotentOK || !sameExpr(idempotent.Premise(), left) {
		t.Fatal("premise merge was not idempotent")
	}
}

func TestAcceptedMemberPremiseRejectsForeignAttachmentScope(t *testing.T) {
	topology, member := premiseTopology(t)
	foreign, foreignOK := DecisionExpr(boundaryDecision(t, 249))
	if !foreignOK {
		t.Fatal("foreign premise")
	}
	if accepted, ok := topology.Accept(member, foreign); ok || accepted.Available() {
		t.Fatal("acceptance admitted a premise outside its attachment ambient scope")
	}
}

func TestPremisedGroupIdentityLaw(t *testing.T) {
	decision := boundaryDecision(t, 246)
	premise, premiseOK := DecisionExpr(decision)
	scope, scopeOK := NewScope(decision)
	batch, _, output, _, _ := boundaryBatch(t, scope)
	if !premiseOK || !scopeOK || batch == nil {
		t.Fatal("premise fixture")
	}
	point, pointOK := derivePoint(output)
	core := groupCore{key: boundaryKey(247)}
	plain, plainOK := derivePremisedGroupKey(core, point, nil, TrueExpr())
	guarded, guardedOK := derivePremisedGroupKey(core, point, nil, premise)
	if !pointOK || !plainOK || !guardedOK || plain == guarded {
		t.Fatal("group identity omitted accepted premise")
	}
}

func TestPremisedFactorEdgeCarriesAcceptedRoute(t *testing.T) {
	decision := boundaryDecision(t, 250)
	ambient, ambientOK := NewScope(decision)
	premise, premiseOK := DecisionExpr(decision)
	batch, _, base, occurrence, operand := boundaryBatch(t, EmptyScope())
	point, pointOK := derivePoint(base)
	if !ambientOK || !premiseOK || !pointOK {
		t.Fatal("factor-edge premise fixture")
	}
	topology := &Topology{}
	binding, application := boundaryKey(251), boundaryKey(252)
	member := Member{owner: topology, binding: binding, locator: PairLocator{Application: application, Target: boundaryKey(253), Endpoint: boundaryKey(254)}}
	template := sealedTemplate{
		key:     boundaryKey(255),
		batch:   batch,
		ambient: ambient,
		value: Template{
			Points: []PointSpec{{Site: base}},
			Rules:  []RuleInstance{{Schema: boundaryKey(180), OperandFamily: boundaryKey(181), Occurrence: occurrence, Operand: operand}},
			Groups: []FragmentGroup{{Members: []RuleRef{RuleAt(0)}, Output: FragmentPoint{Local: PointAt(0)}}},
			FactorEdges: []FragmentFactorEdge{{
				Source: FragmentPoint{Local: PointAt(0)}, Target: FragmentPoint{Local: PointAt(0)},
				Factor: boundaryKey(182), Provenance: boundaryKey(183), Pre: TrueExpr(), Reindex: IdentityReindex(EmptyScope()), Post: TrueExpr(),
			}},
		},
		instances: []canonicalInstance{{key: boundaryKey(184)}},
		points:    map[PointRef]Point{PointAt(0): point},
	}
	spec := &TopologySpec{Batch: batch}
	if !template.appendMember(spec, binding, member, premise) || len(spec.FactorEdges) != 1 {
		t.Fatal("premised FactorEdge was not materialized")
	}
	pre := spec.FactorEdges[0].Input.Pre()
	if !pre.Available() || pre.IsTrue() {
		t.Fatal("accepted Member premise was dropped from FactorEdge route")
	}
	seen := false
	for _, got := range pre.Decisions() {
		if got == decision {
			seen = true
		}
	}
	if !seen {
		t.Fatal("FactorEdge precondition omitted selected-member decision")
	}
}

func TestExternalPremisedFactorEdgeKeepsSourcePointAndGuardsTarget(t *testing.T) {
	decision := boundaryDecision(t, 190)
	ambient, ambientOK := NewScope(decision)
	premise, premiseOK := DecisionExpr(decision)
	batch, entry, base, occurrence, operand := boundaryBatch(t, EmptyScope())
	point, pointOK := derivePoint(base)
	if !ambientOK || !premiseOK || !pointOK {
		t.Fatal("external factor-edge premise fixture")
	}
	topology := &Topology{}
	binding := boundaryKey(191)
	member := Member{owner: topology, binding: binding, locator: PairLocator{Application: boundaryKey(192), Target: boundaryKey(193), Endpoint: boundaryKey(194)}}
	template := sealedTemplate{
		key:     boundaryKey(195),
		batch:   batch,
		ambient: ambient,
		value: Template{
			Points: []PointSpec{{Site: base}},
			Rules:  []RuleInstance{{Schema: boundaryKey(196), OperandFamily: boundaryKey(197), Occurrence: occurrence, Operand: operand}},
			Groups: []FragmentGroup{{Members: []RuleRef{RuleAt(0)}, Output: FragmentPoint{Local: PointAt(0)}}},
			FactorEdges: []FragmentFactorEdge{{
				ExternalSource: entry, Target: FragmentPoint{Local: PointAt(0)},
				Factor: boundaryKey(198), Provenance: boundaryKey(199), Pre: TrueExpr(), Reindex: IdentityReindex(EmptyScope()), Post: TrueExpr(),
			}},
		},
		instances: []canonicalInstance{{key: boundaryKey(200)}},
		points:    map[PointRef]Point{PointAt(0): point},
	}
	spec := &TopologySpec{Batch: batch, Points: []PointSpec{{Site: entry}}}
	if !template.appendMember(spec, binding, member, premise) || len(spec.FactorEdges) != 1 {
		t.Fatal("external FactorEdge was not materialized")
	}
	edge := spec.FactorEdges[0]
	if !edge.Input.Source().Same(entry) || edge.Input.Post().IsTrue() {
		t.Fatal("external FactorEdge lost its source or selected guard")
	}
}

func TestPremisedFactorEdgeKeepsExternalTargetAndGuardsSource(t *testing.T) {
	decision := boundaryDecision(t, 201)
	ambient, ambientOK := NewScope(decision)
	premise, premiseOK := DecisionExpr(decision)
	batch := NewBatch()
	externalTarget, targetOK := batch.AdmitSite(boundaryKey(202), ambient, TrueExpr(), InitPresent)
	base, baseOK := batch.AdmitSite(boundaryKey(203), EmptyScope(), FalseExpr(), InitAbsent)
	occurrence, occurrenceOK := batch.At(base)
	operand, operandOK := batch.AdmitOperand(occurrence, boundaryKey(204))
	if !ambientOK || !premiseOK || !targetOK || !baseOK || !occurrenceOK || !operandOK || !batch.Seal() {
		t.Fatal("external target factor-edge premise fixture")
	}
	point, pointOK := derivePoint(base)
	if !pointOK {
		t.Fatal("source point")
	}
	reindex, reindexOK := NewReindex(EmptyScope(), ambient, nil)
	if !reindexOK {
		t.Fatal("external target reindex")
	}
	topology := &Topology{}
	binding := boundaryKey(205)
	member := Member{owner: topology, binding: binding, locator: PairLocator{Application: boundaryKey(206), Target: boundaryKey(207), Endpoint: boundaryKey(208)}}
	template := sealedTemplate{
		key:     boundaryKey(209),
		batch:   batch,
		ambient: ambient,
		value: Template{
			Points: []PointSpec{{Site: base}},
			Rules:  []RuleInstance{{Schema: boundaryKey(210), OperandFamily: boundaryKey(211), Occurrence: occurrence, Operand: operand}},
			Groups: []FragmentGroup{{Members: []RuleRef{RuleAt(0)}, Output: FragmentPoint{Local: PointAt(0)}}},
			FactorEdges: []FragmentFactorEdge{{
				Source: FragmentPoint{Local: PointAt(0)}, ExternalTarget: externalTarget,
				Factor: boundaryKey(212), Provenance: boundaryKey(213), Pre: TrueExpr(), Reindex: reindex, Post: TrueExpr(),
			}},
		},
		instances: []canonicalInstance{{key: boundaryKey(214)}},
		points:    map[PointRef]Point{PointAt(0): point},
	}
	spec := &TopologySpec{Batch: batch, Points: []PointSpec{{Site: externalTarget}}}
	if !template.appendMember(spec, binding, member, premise) || len(spec.FactorEdges) != 1 {
		t.Fatal("external target FactorEdge was not materialized")
	}
	edge := spec.FactorEdges[0]
	if !edge.Input.Target().Same(externalTarget) || edge.Input.Pre().IsTrue() {
		t.Fatal("external target FactorEdge lost target or selected guard")
	}
}

func TestExternalSourceAmbientIdentityIsNotDuplicated(t *testing.T) {
	decision := boundaryDecision(t, 215)
	ambient, ambientOK := NewScope(decision)
	premise, premiseOK := DecisionExpr(decision)
	batch := NewBatch()
	externalSource, sourceOK := batch.AdmitSite(boundaryKey(216), ambient, TrueExpr(), InitPresent)
	base, baseOK := batch.AdmitSite(boundaryKey(217), EmptyScope(), FalseExpr(), InitAbsent)
	occurrence, occurrenceOK := batch.At(base)
	operand, operandOK := batch.AdmitOperand(occurrence, boundaryKey(218))
	if !ambientOK || !premiseOK || !sourceOK || !baseOK || !occurrenceOK || !operandOK || !batch.Seal() {
		t.Fatal("ambient collision fixture")
	}
	point, pointOK := derivePoint(base)
	if !pointOK {
		t.Fatal("target point")
	}
	topology := &Topology{}
	binding := boundaryKey(219)
	member := Member{owner: topology, binding: binding, locator: PairLocator{Application: boundaryKey(220), Target: boundaryKey(221), Endpoint: boundaryKey(222)}}
	forget, forgetOK := NewReindex(ambient, EmptyScope(), []DecisionMap{Forget(decision)})
	if !forgetOK {
		t.Fatal("formal external reindex")
	}
	template := sealedTemplate{
		key:     boundaryKey(223),
		batch:   batch,
		ambient: ambient,
		value: Template{
			Points: []PointSpec{{Site: base}},
			Rules:  []RuleInstance{{Schema: boundaryKey(224), OperandFamily: boundaryKey(225), Occurrence: occurrence, Operand: operand}},
			Groups: []FragmentGroup{{Members: []RuleRef{RuleAt(0)}, Output: FragmentPoint{Local: PointAt(0)}}},
			FactorEdges: []FragmentFactorEdge{{
				ExternalSource: externalSource, Target: FragmentPoint{Local: PointAt(0)},
				Factor: boundaryKey(226), Provenance: boundaryKey(227), Pre: TrueExpr(), Reindex: forget, Post: TrueExpr(),
			}},
		},
		instances: []canonicalInstance{{key: boundaryKey(228)}},
		points:    map[PointRef]Point{PointAt(0): point},
	}
	spec := &TopologySpec{Batch: batch, Points: []PointSpec{{Site: externalSource}}}
	if !template.appendMember(spec, binding, member, premise) || len(spec.FactorEdges) != 1 {
		t.Fatal("ambient external FactorEdge was not materialized")
	}
	if !spec.FactorEdges[0].Input.Reindex().Identity() {
		t.Fatal("ambient source/port collision did not resolve to one identity map")
	}
}

func TestActivationPremiseRejectsForeignOutputScope(t *testing.T) {
	raw := boundaryDecision(t, 248)
	scope, scopeOK := NewScope(raw)
	premise, premiseOK := DecisionExpr(raw)
	batch, _, base, occurrence, operand := boundaryBatch(t, scope)
	point, pointOK := derivePoint(base)
	binding := boundaryKey(249)
	member := Member{owner: &Topology{}, binding: binding, locator: PairLocator{Application: boundaryKey(238), Target: boundaryKey(239), Endpoint: boundaryKey(240)}}
	template := sealedTemplate{
		key:     boundaryKey(237),
		batch:   batch,
		ambient: EmptyScope(),
		value: Template{
			Points: []PointSpec{{Site: base}},
			Rules:  []RuleInstance{{Schema: boundaryKey(236), OperandFamily: boundaryKey(235), Occurrence: occurrence, Operand: operand}},
			Groups: []FragmentGroup{{Members: []RuleRef{RuleAt(0)}, Output: FragmentPoint{Local: PointAt(0)}}},
		},
		instances: []canonicalInstance{{key: boundaryKey(234)}},
		points:    map[PointRef]Point{PointAt(0): point},
	}
	if !scopeOK || !premiseOK || !pointOK {
		t.Fatal("foreign-scope fixture")
	}
	if template.appendMember(&TopologySpec{Batch: batch}, binding, member, premise) {
		t.Fatal("unbound premise escaped into alpha-renamed dynamic output scope")
	}
}

func TestExprDAGCancellationDropsPremise(t *testing.T) {
	left, right := boundaryDecision(t, 232), boundaryDecision(t, 233)
	rows := []ExprNode{{Decision: right, Low: 0, High: 1}, {Decision: left, Low: 0, High: 2}}
	checks := 0
	if expression, accepted := NewExprDAGWithCheckpoint(rows, 3, func() bool {
		checks++
		return checks < 2
	}); accepted || expression.Available() {
		t.Fatal("canceled premise DAG was retained")
	}
	if expression, accepted := NewExprDAG(rows, 3); !accepted || !expression.Available() {
		t.Fatal("valid premise DAG rejected")
	}
}

func TestExprDAGCanonicalizesReachableLayoutAndRejectsResidue(t *testing.T) {
	first, second, third := boundaryDecision(t, 220), boundaryDecision(t, 221), boundaryDecision(t, 222)
	left, leftOK := NewExprDAG([]ExprNode{
		{Decision: second, Low: 0, High: 1},
		{Decision: third, Low: 0, High: 1},
		{Decision: first, Low: 2, High: 3},
	}, 4)
	right, rightOK := NewExprDAG([]ExprNode{
		{Decision: third, Low: 0, High: 1},
		{Decision: second, Low: 0, High: 1},
		{Decision: first, Low: 3, High: 2},
	}, 4)
	if !leftOK || !rightOK || !sameExpr(left, right) {
		t.Fatal("equivalent reachable layouts did not seal canonically")
	}
	if residue, accepted := NewExprDAG([]ExprNode{
		{Decision: second, Low: 0, High: 1},
		{Decision: third, Low: 0, High: 1},
	}, 2); accepted || residue.Available() {
		t.Fatal("unreachable raw DAG row was accepted")
	}
}
