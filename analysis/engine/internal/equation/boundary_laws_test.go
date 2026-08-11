package equation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
)

func boundaryKey(value byte) composition.Key {
	var id composition.ID
	id[0] = value
	return composition.Key{ID: id, Version: 1}
}

func boundaryDecision(t testing.TB, value byte) Decision {
	t.Helper()
	decision, ok := NewDecision(boundaryKey(value))
	if !ok {
		t.Fatal("decision")
	}
	return decision
}

func boundaryBatch(t testing.TB, scope Scope) (*Batch, Site, Site, Occurrence, Operand) {
	t.Helper()
	batch := NewBatch()
	entry, ok := batch.AdmitSite(boundaryKey(1), scope, TrueExpr(), InitPresent)
	if !ok {
		t.Fatal("entry Site")
	}
	output, ok := batch.AdmitSite(boundaryKey(2), scope, FalseExpr(), InitAbsent)
	if !ok {
		t.Fatal("output Site")
	}
	occurrence, ok := batch.At(output)
	if !ok {
		t.Fatal("occurrence")
	}
	operand, ok := batch.AdmitOperand(occurrence, boundaryKey(3))
	if !ok || !batch.Seal() {
		t.Fatal("operand batch")
	}
	return batch, entry, output, occurrence, operand
}

func TestBatchSealsOnlyOneExactSourceTopology(t *testing.T) {
	scope := EmptyScope()
	batch := NewBatch()
	first, ok := batch.AdmitSite(boundaryKey(10), scope, FalseExpr(), InitAbsent)
	if !ok || first.Available() {
		t.Fatal("open Site capability")
	}
	again, ok := batch.AdmitSite(boundaryKey(10), scope, FalseExpr(), InitAbsent)
	if !ok || again.row != first.row {
		t.Fatal("same exact Site was not canonical")
	}
	if _, ok := batch.AdmitSite(boundaryKey(10), scope, TrueExpr(), InitPresent); ok {
		t.Fatal("divergent Site initialization accepted")
	}
	if batch.Seal() {
		t.Fatal("divergent source topology sealed")
	}

	foreign := NewBatch()
	foreignSite, ok := foreign.AdmitSite(boundaryKey(11), scope, FalseExpr(), InitAbsent)
	if !ok {
		t.Fatal("foreign Site")
	}
	if _, ok := batch.At(foreignSite); ok {
		t.Fatal("foreign Site admitted")
	}
	if _, ok := foreign.AdmitOperand(Occurrence{}, boundaryKey(12)); ok {
		t.Fatal("unsealed non-occurrence admitted")
	}
}

func TestBatchRejectsOccurrenceOperandMismatch(t *testing.T) {
	scope := EmptyScope()
	batch := NewBatch()
	entry, ok := batch.AdmitSite(boundaryKey(20), scope, TrueExpr(), InitPresent)
	if !ok {
		t.Fatal("entry")
	}
	left, ok := batch.AdmitSite(boundaryKey(21), scope, FalseExpr(), InitAbsent)
	if !ok {
		t.Fatal("left")
	}
	right, ok := batch.AdmitSite(boundaryKey(22), scope, FalseExpr(), InitAbsent)
	if !ok {
		t.Fatal("right")
	}
	leftOccurrence, ok := batch.At(left)
	if !ok {
		t.Fatal("left occurrence")
	}
	rightOccurrence, ok := batch.At(right)
	if !ok {
		t.Fatal("right occurrence")
	}
	operand, ok := batch.AdmitOperand(leftOccurrence, boundaryKey(23))
	if !ok || !batch.Seal() {
		t.Fatal("batch")
	}
	if !entry.Available() || !rightOccurrence.Available() || !operand.Available() {
		t.Fatal("sealed capability")
	}
	if !operand.Occurrence().Same(leftOccurrence) || operand.Occurrence().Same(rightOccurrence) {
		t.Fatal("operand occurrence admission")
	}
	if validTopologyBatch(batch, TopologySpec{Batch: batch, Rules: []RuleInstance{{Schema: boundaryKey(24), OperandFamily: boundaryKey(25), Occurrence: rightOccurrence, Operand: operand}}}) {
		t.Fatal("topology admitted an operand at the wrong occurrence")
	}
}

func TestBatchAdmitAllRejectsWholeSetWithoutPrefix(t *testing.T) {
	batch := NewBatch()
	values := []Admission{
		{Source: boundaryKey(70), Scope: EmptyScope(), Init: FalseExpr(), Disposition: InitAbsent, Kind: OccurrenceAt, Entity: boundaryKey(70), Operand: boundaryKey(71)},
		// The malformed trailing member must not publish the valid first row.
		{Source: boundaryKey(72), Scope: EmptyScope(), Init: FalseExpr(), Disposition: InitAbsent, Kind: OccurrenceAt, Entity: boundaryKey(72)},
	}
	if _, ok := batch.AdmitAll(values); ok {
		t.Fatal("partial source set admitted")
	}
	if batch.phase != batchRejected || len(batch.sites) != 0 || len(batch.occurrences) != 0 || len(batch.operands) != 0 {
		t.Fatalf("rejected atomic admission retained a prefix: phase=%v sites=%d occurrences=%d operands=%d", batch.phase, len(batch.sites), len(batch.occurrences), len(batch.operands))
	}
}

func TestBatchDiscardsAdmissionIndexesAtTerminalPhase(t *testing.T) {
	sealed, _, _, _, _ := boundaryBatch(t, EmptyScope())
	if sealed.siteBySource != nil || sealed.occurrenceAt != nil || sealed.operandAt != nil {
		t.Fatal("sealed Batch retained open-phase admission indexes")
	}

	rejected := NewBatch()
	site, ok := rejected.AdmitSite(boundaryKey(73), EmptyScope(), FalseExpr(), InitAbsent)
	if !ok {
		t.Fatal("rejected Batch fixture site")
	}
	occurrence, ok := rejected.At(site)
	if !ok {
		t.Fatal("rejected Batch fixture occurrence")
	}
	if _, ok := rejected.AdmitOperand(occurrence, boundaryKey(74)); !ok {
		t.Fatal("rejected Batch fixture operand")
	}
	if !rejected.Reject() {
		t.Fatal("open Batch did not reject")
	}
	if len(rejected.sites) != 0 || len(rejected.occurrences) != 0 || len(rejected.operands) != 0 ||
		rejected.siteBySource != nil || rejected.occurrenceAt != nil || rejected.operandAt != nil {
		t.Fatal("rejected Batch retained terminally unreachable admission state")
	}
}

func TestBoundaryRetainsCompleteTransportLaw(t *testing.T) {
	sourceDecision := boundaryDecision(t, 30)
	targetDecision := boundaryDecision(t, 31)
	sourceScope, ok := NewScope(sourceDecision)
	if !ok {
		t.Fatal("source Scope")
	}
	targetScope, ok := NewScope(targetDecision)
	if !ok {
		t.Fatal("target Scope")
	}
	batch := NewBatch()
	entry, ok := batch.AdmitSite(boundaryKey(32), sourceScope, TrueExpr(), InitPresent)
	if !ok {
		t.Fatal("entry")
	}
	output, ok := batch.AdmitSite(boundaryKey(33), targetScope, FalseExpr(), InitAbsent)
	if !ok {
		t.Fatal("output")
	}
	if _, ok := batch.At(output); !ok || !batch.Seal() {
		t.Fatal("batch")
	}
	pre, ok := DecisionExpr(sourceDecision)
	if !ok {
		t.Fatal("pre")
	}
	post, ok := DecisionExpr(targetDecision)
	if !ok {
		t.Fatal("post")
	}
	rename, ok := NewReindex(sourceScope, targetScope, []DecisionMap{Rename(sourceDecision, targetDecision)})
	if !ok {
		t.Fatal("rename")
	}
	boundary := BoundaryInput(entry, output, boundaryKey(34), pre, rename, post)
	if !boundary.Available() || boundary.IdentityTransport() || !boundary.Source().Same(entry) || !boundary.Target().Same(output) || boundary.Provenance() != boundaryKey(34) {
		t.Fatal("boundary law omitted a transport coordinate")
	}
	if wrong := BoundaryInput(entry, entry, boundaryKey(35), pre, rename, post); wrong.Available() {
		t.Fatal("fresh target scope accepted source boundary")
	}

	forget, ok := NewReindex(sourceScope, EmptyScope(), []DecisionMap{Forget(sourceDecision)})
	if !ok {
		t.Fatal("forget")
	}
	forgetBatch := NewBatch()
	forgetSource, ok := forgetBatch.AdmitSite(boundaryKey(35), sourceScope, TrueExpr(), InitPresent)
	if !ok {
		t.Fatal("forget source")
	}
	forgotten, ok := forgetBatch.AdmitSite(boundaryKey(36), EmptyScope(), FalseExpr(), InitAbsent)
	if !ok || !forgetBatch.Seal() {
		t.Fatal("forget target")
	}
	if input := BoundaryInput(forgetSource, forgotten, boundaryKey(37), pre, forget, FalseExpr()); !input.Available() || input.IdentityTransport() {
		t.Fatal("forget boundary")
	}
	if input := BoundaryInput(entry, output, boundaryKey(38), FalseExpr(), rename, FalseExpr()); !input.Available() {
		t.Fatal("false pre/post are semantic filters, not malformed inputs")
	}
}

func TestBoundaryIdentityFastPathIsExactAndAllocationFree(t *testing.T) {
	scope := EmptyScope()
	_, entry, output, _, _ := boundaryBatch(t, scope)
	identity := IdentityReindex(scope)
	input := BoundaryInput(entry, output, boundaryKey(40), TrueExpr(), identity, TrueExpr())
	if !input.Available() || !input.IdentityTransport() {
		t.Fatal("exact identity boundary")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		if !input.IdentityTransport() {
			t.Fatal("identity transport")
		}
	}); allocations != 0 {
		t.Fatalf("identity transport allocated %g times", allocations)
	}
	filter := BoundaryInput(entry, output, boundaryKey(41), FalseExpr(), identity, TrueExpr())
	if !filter.Available() || filter.IdentityTransport() {
		t.Fatal("filter-only boundary reused a plane root")
	}
}

func TestMultiInputBoundaryIntersectionAndMuReinstallIdentity(t *testing.T) {
	scope := EmptyScope()
	batch := NewBatch()
	left, ok := batch.AdmitSite(boundaryKey(50), scope, TrueExpr(), InitPresent)
	if !ok {
		t.Fatal("left")
	}
	right, ok := batch.AdmitSite(boundaryKey(51), scope, TrueExpr(), InitPresent)
	if !ok {
		t.Fatal("right")
	}
	output, ok := batch.AdmitSite(boundaryKey(52), scope, FalseExpr(), InitAbsent)
	if !ok {
		t.Fatal("output")
	}
	occurrence, ok := batch.At(output)
	if !ok {
		t.Fatal("occurrence")
	}
	operand, ok := batch.AdmitOperand(occurrence, boundaryKey(53))
	if !ok || !batch.Seal() {
		t.Fatal("batch")
	}
	if !operand.Occurrence().Same(occurrence) {
		t.Fatal("operand lane")
	}
	identity := IdentityReindex(scope)
	first := BoundaryInput(left, output, boundaryKey(54), TrueExpr(), identity, TrueExpr())
	second := BoundaryInput(right, output, boundaryKey(55), FalseExpr(), identity, TrueExpr())
	if !first.Available() || !second.Available() || first.Key() == second.Key() {
		t.Fatal("ordered multi-input transport")
	}
	// The recurrence compiler must retain the boundary law: replacing only a
	// precondition produces a distinct finite group identity, so killing and
	// reinstalling a Mu edge cannot reuse its former transfer state.
	core, ok := deriveGroupCore([]canonicalInstance{{key: boundaryKey(56)}})
	if !ok {
		t.Fatal("group core")
	}
	point, ok := derivePoint(output)
	if !ok {
		t.Fatal("point")
	}
	first.point, second.point = point, point
	before, ok := deriveGroupKey(core, point, []Input{first, second})
	if !ok {
		t.Fatal("first Mu")
	}
	reinstalled := BoundaryInput(right, output, boundaryKey(55), TrueExpr(), identity, TrueExpr())
	reinstalled.point = point
	after, ok := deriveGroupKey(core, point, []Input{first, reinstalled})
	if !ok || before == after {
		t.Fatal("Mu boundary reinstall retained killed transport identity")
	}
}

func TestDynamicAlphaRewritesPrePostAndReindexTogether(t *testing.T) {
	raw := boundaryDecision(t, 60)
	fresh := boundaryDecision(t, 61)
	scope, ok := NewScope(raw)
	if !ok {
		t.Fatal("raw Scope")
	}
	formula, ok := DecisionExpr(raw)
	if !ok {
		t.Fatal("formula")
	}
	reindex := IdentityReindex(scope)
	alpha := decisionAlpha{raw.Key(): fresh}
	pre, ok := boundExpr(formula, alpha)
	if !ok || len(pre.Decisions()) != 1 || pre.Decisions()[0] != fresh {
		t.Fatal("dynamic pre alpha")
	}
	post, ok := boundExpr(formula, alpha)
	if !ok || len(post.Decisions()) != 1 || post.Decisions()[0] != fresh {
		t.Fatal("dynamic post alpha")
	}
	batch := NewBatch()
	base, ok := batch.AdmitSite(boundaryKey(62), scope, FalseExpr(), InitAbsent)
	baseOccurrence, occurred := batch.At(base)
	baseOperand, attached := batch.AdmitOperand(baseOccurrence, boundaryKey(66))
	if !ok || !occurred || !attached || !batch.Seal() {
		t.Fatal("dynamic base")
	}
	table, ok := newMemberSiteTable(batch, EmptyScope(), alpha, boundaryKey(63), boundaryKey(64), 1)
	if !ok {
		t.Fatal("member Site table")
	}
	boundSite, ok := table.bind(base)
	if !ok {
		t.Fatal("dynamic Site")
	}
	dynamic := boundSite.site
	decision, decisionOK := dynamic.Scope().At(0)
	if !decisionOK || decision != fresh {
		t.Fatal("dynamic Site scope")
	}
	if _, ok := batch.At(base); ok {
		t.Fatal("post-seal raw occurrence admission")
	}
	// The activation overlay itself is derived only from sealed base rows.  It
	// includes the binding/member identities at every semantic layer, so two
	// dynamic members cannot reuse one another's Site or Operand capability.
	dynamicOccurrence, ok := table.bindOccurrence(baseOccurrence, boundaryKey(68))
	if !ok || !dynamicOccurrence.occurrence.Site().Same(dynamic) {
		t.Fatal("dynamic occurrence")
	}
	dynamicOperand, ok := table.bindOperand(baseOperand, dynamicOccurrence)
	if !ok || !dynamicOperand.Occurrence().Same(dynamicOccurrence.occurrence) || dynamicOperand.Key() == baseOperand.Key() {
		t.Fatal("dynamic Operand")
	}
	source := templateResolvedPoint{ref: PointAt(1), site: dynamic, scope: dynamic.Scope(), rawScope: scope, local: true}
	target := templateResolvedPoint{ref: PointAt(2), site: dynamic, scope: dynamic.Scope(), rawScope: scope, local: true}
	bound, ok := boundReindex(reindex, source, target, EmptyScope(), alpha)
	if !ok || !bound.Identity() {
		t.Fatal("dynamic reindex alpha")
	}
}

func TestMemberAlphaUsesOneSiteForPointAndRuleRoles(t *testing.T) {
	raw := boundaryDecision(t, 70)
	scope, ok := NewScope(raw)
	if !ok {
		t.Fatal("scope")
	}
	initial, ok := DecisionExpr(raw)
	if !ok {
		t.Fatal("initial formula")
	}
	batch := NewBatch()
	base, ok := batch.AdmitSite(boundaryKey(72), scope, initial, InitPresent)
	if !ok {
		t.Fatal("base Site")
	}
	occurrence, ok := batch.At(base)
	if !ok {
		t.Fatal("base occurrence")
	}
	operand, ok := batch.AdmitOperand(occurrence, boundaryKey(73))
	if !ok || !batch.Seal() {
		t.Fatal("base Operand")
	}
	point, ok := derivePoint(base)
	if !ok {
		t.Fatal("base Point")
	}
	binding := boundaryKey(74)
	member := Member{owner: &Topology{}, binding: binding, locator: PairLocator{Application: boundaryKey(76), Target: boundaryKey(77), Endpoint: boundaryKey(78)}}
	template := sealedTemplate{
		key:     boundaryKey(79),
		batch:   batch,
		ambient: EmptyScope(),
		value: Template{
			Points: []PointSpec{{Site: base}},
			Rules:  []RuleInstance{{Schema: boundaryKey(80), OperandFamily: boundaryKey(81), Occurrence: occurrence, Operand: operand}},
		},
		instances: []canonicalInstance{{key: boundaryKey(82)}},
		points:    map[PointRef]Point{PointAt(0): point},
	}
	spec := TopologySpec{Batch: batch}
	if !template.appendMember(&spec, binding, member, TrueExpr()) || len(spec.Points) != 1 || len(spec.Rules) != 1 {
		t.Fatal("member template expansion")
	}
	pointSite, ruleSite := spec.Points[0].Site, spec.Rules[0].Occurrence.Site()
	if !pointSite.Same(ruleSite) || pointSite.dynamic == nil || pointSite.dynamic != ruleSite.dynamic || !sameScope(pointSite.Scope(), ruleSite.Scope()) {
		t.Fatal("one source acquired multiple member-local Site capabilities")
	}
	namespace, namespaceOK := memberNamespace(member)
	if !namespaceOK {
		t.Fatal("member namespace")
	}
	alpha, ok := template.decisionAlpha(binding, namespace)
	if !ok {
		t.Fatal("alpha")
	}
	decision, decisionOK := pointSite.Scope().At(0)
	if !decisionOK || decision != alpha[raw.Key()] {
		t.Fatal("member Site did not retain the one alpha-renamed scope")
	}

	table, ok := newMemberSiteTable(batch, EmptyScope(), alpha, binding, namespace, 1)
	if !ok {
		t.Fatal("member Site table")
	}
	first, ok := table.bind(base)
	second, again := table.bind(occurrence.Site())
	if !ok || !again || len(table.rows) != 1 || first.site.dynamic != second.site.dynamic {
		t.Fatal("duplicate role binding was not canonical")
	}
	foreignBatch := NewBatch()
	foreign, admitted := foreignBatch.AdmitSite(boundaryKey(83), scope, initial, InitPresent)
	if !admitted || !foreignBatch.Seal() {
		t.Fatal("foreign Site")
	}
	if _, accepted := table.bind(foreign); accepted {
		t.Fatal("foreign Site entered member-local binding")
	}
	if _, accepted := table.admit(base, EmptyScope(), FalseExpr(), InitPresent); accepted {
		t.Fatal("divergent Site rebinding entered member-local binding")
	}
}

func TestMemberAlphaRejectsOccurrenceOperandSplices(t *testing.T) {
	raw := boundaryDecision(t, 90)
	fresh := boundaryDecision(t, 91)
	scope, ok := NewScope(raw)
	if !ok {
		t.Fatal("scope")
	}
	batch := NewBatch()
	left, ok := batch.AdmitSite(boundaryKey(92), scope, FalseExpr(), InitAbsent)
	if !ok {
		t.Fatal("left Site")
	}
	right, ok := batch.AdmitSite(boundaryKey(93), scope, FalseExpr(), InitAbsent)
	if !ok {
		t.Fatal("right Site")
	}
	leftOccurrence, ok := batch.At(left)
	if !ok {
		t.Fatal("left occurrence")
	}
	rightOccurrence, ok := batch.At(right)
	if !ok {
		t.Fatal("right occurrence")
	}
	leftOperand, ok := batch.AdmitOperand(leftOccurrence, boundaryKey(94))
	if !ok {
		t.Fatal("left Operand")
	}
	rightOperand, ok := batch.AdmitOperand(rightOccurrence, boundaryKey(95))
	if !ok || !batch.Seal() {
		t.Fatal("right Operand")
	}
	table, ok := newMemberSiteTable(batch, EmptyScope(), decisionAlpha{raw.Key(): fresh}, boundaryKey(96), boundaryKey(97), 2)
	if !ok {
		t.Fatal("member Site table")
	}
	boundLeft, ok := table.bindOccurrence(leftOccurrence, boundaryKey(98))
	if !ok || !boundLeft.occurrence.Site().Source().Available() || boundLeft.occurrence.Site().Source() != left.Source() {
		t.Fatal("left lineage")
	}
	boundRight, ok := table.bindOccurrence(rightOccurrence, boundaryKey(99))
	if !ok || boundLeft.occurrence.Site().Same(boundRight.occurrence.Site()) {
		t.Fatal("occurrence Site lineage")
	}
	if _, accepted := table.bindOperand(rightOperand, boundLeft); accepted {
		t.Fatal("Operand spliced across admitted occurrences")
	}
	spliced := boundLeft
	spliced.site = boundRight.site
	if _, accepted := table.bindOperand(leftOperand, spliced); accepted {
		t.Fatal("Occurrence spliced onto unrelated bound Site")
	}
	if _, accepted := table.bindOperand(leftOperand, boundRight); accepted {
		t.Fatal("Operand accepted a foreign dynamic occurrence")
	}
	forgedOccurrence := boundLeft.occurrence
	forgedOccurrenceOverlay := *forgedOccurrence.dynamic
	forgedOccurrenceOverlay.site = boundRight.occurrence.Site()
	forgedOccurrence.dynamic = &forgedOccurrenceOverlay
	if forgedOccurrence.Available() || batch.ownsOccurrence(forgedOccurrence) {
		t.Fatal("forged occurrence Site splice remained topology-valid")
	}
	boundOperand, ok := table.bindOperand(leftOperand, boundLeft)
	if !ok {
		t.Fatal("bound Operand")
	}
	forgedOperand := boundOperand
	forgedOperandOverlay := *forgedOperand.dynamic
	forgedOperandOverlay.occurrence = boundRight.occurrence
	forgedOperand.dynamic = &forgedOperandOverlay
	if forgedOperand.Available() || batch.ownsOperand(forgedOperand) {
		t.Fatal("forged Operand occurrence splice remained topology-valid")
	}
}
