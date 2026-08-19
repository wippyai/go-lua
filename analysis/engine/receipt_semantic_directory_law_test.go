package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

func issueReceiptAssemblyFixtureRule(t testing.TB, fixture receiptAssemblyRuleFixture) bindingRuleRow {
	t.Helper()
	builder := fixture.assembly
	source, sourceOK := builder.issueRuleSurfaceSource(fixture.sourceSpec())
	draft, draftOK := fixture.implementation.beginBindingRuleRow(source)
	write, writeOK := fixture.implementation.WritePart(source, 0)
	if !sourceOK || !draftOK || !writeOK || !draft.AddWrite(write) {
		t.Fatal("semantic directory Rule draft")
	}
	row, ok := builder.issueRuleRow(draft)
	if !ok {
		t.Fatal("semantic directory Rule receipt")
	}
	return row
}

func TestReceiptAssemblySemanticDirectoryLookupIsExactAndRevisionOwned(t *testing.T) {
	fixture := newReceiptAssemblyRuleFixture(t)
	if fixture.assembly.sealSources().Available() {
		t.Fatal("semantic directory source seal")
	}
	row := issueReceiptAssemblyFixtureRule(t, fixture)
	declaration := topologyDeclaration{
		binding: fixture.assembly.binding, batch: fixture.assembly.inner.batch,
		points:  []declaredPointRow{{ID: receiptAssemblySemanticID(1), Site: fixture.site}},
		members: []declaredMemberRow{{Plane: declaredMemberOwner, ID: receiptAssemblySemanticID(2), Row: row.row}},
	}
	constructed, refusal := constructTopology(declaration)
	topology, issued, ok := constructed.topology, constructed.graph, !refusal.Available() && constructed.Available()
	if !ok || topology == nil || issued == nil {
		t.Fatalf("semantic directory commit stage=%v step=%v", refusal.Stage(), refusal.Step())
	}
	first := CommittedProgramFrom(topology, issued)
	if first == nil {
		t.Fatal("semantic directory committed program")
	}
	point, pointOK := first.lookupPoint(receiptAssemblySemanticID(1))
	member, memberOK := first.lookupRuleMember(receiptAssemblySemanticID(2))
	if !pointOK || !first.graph.OwnsPoint(point) || !memberOK || member.graph != first.graph || member.topology != first.topology || !first.graph.OwnsMember(member.member) {
		t.Fatal("semantic directory exact first-revision receipts")
	}
	if _, found := first.lookupPoint(receiptAssemblySemanticID(99)); found {
		t.Fatal("unknown semantic Point ID resolved")
	}
	if _, found := first.lookupRuleMember(receiptAssemblySemanticID(0)); found {
		t.Fatal("zero semantic member ID resolved")
	}
	second, secondOK := initialCommittedProgram(topology)
	secondPoint, secondPointOK := second.lookupPoint(receiptAssemblySemanticID(1))
	secondMember, secondMemberOK := second.lookupRuleMember(receiptAssemblySemanticID(2))
	if !secondOK || second == nil || second == first || !secondPointOK || !second.graph.OwnsPoint(secondPoint) || !secondMemberOK || secondMember.graph != second.graph || secondMember.topology != second.topology || !second.graph.OwnsMember(secondMember.member) {
		t.Fatal("semantic directory exact second-revision receipts")
	}
	// The directory is the topology's organ: every revision handle resolves
	// one ContentID to the same immutable structural row.
	if secondPoint.Key() != point.Key() || secondMember.member.Key() != member.member.Key() {
		t.Fatal("semantic directory resolved one identity to two rows")
	}
	pointLocator, pointLocatorOK := topology.directory.point(receiptAssemblySemanticID(1))
	memberLocator, memberLocatorOK := topology.directory.member(receiptAssemblySemanticID(2))
	foreign := newReceiptAssemblyRuleFixture(t)
	if foreign.assembly.sealSources().Available() {
		t.Fatal("foreign topology source seal")
	}
	foreignRow := issueReceiptAssemblyFixtureRule(t, foreign)
	foreignDeclaration := topologyDeclaration{
		binding: foreign.assembly.binding, batch: foreign.assembly.inner.batch,
		points:  []declaredPointRow{{ID: receiptAssemblySemanticID(1), Site: foreign.site}},
		members: []declaredMemberRow{{Plane: declaredMemberOwner, ID: receiptAssemblySemanticID(2), Row: foreignRow.row}},
	}
	foreignConstructed, foreignRefusal := constructTopology(foreignDeclaration)
	foreignIssued, foreignOK := foreignConstructed.graph, !foreignRefusal.Available() && foreignConstructed.Available()
	if !pointLocatorOK || !memberLocatorOK || !foreignOK || foreignIssued == nil {
		t.Fatalf("foreign topology commit stage=%v step=%v", foreignRefusal.Stage(), foreignRefusal.Step())
	}
	if _, crossed := pointLocator.Resolve(foreignIssued); crossed {
		t.Fatal("directory Point locator resolved against a foreign topology graph")
	}
	if _, crossed := memberLocator.Resolve(foreignIssued); crossed {
		t.Fatal("directory member locator resolved against a foreign topology graph")
	}
}

func TestReceiptAssemblySemanticDirectoryDuplicateZeroAndForeignFailTerminally(t *testing.T) {
	fixture := newReceiptAssemblyRuleFixture(t)
	if fixture.assembly.sealSources().Available() {
		t.Fatal("duplicate semantic source seal")
	}
	point, pointOK := fixture.assembly.issuePointRow(equation.PointSpec{Site: fixture.site})
	if !pointOK {
		t.Fatal("duplicate semantic Point receipt")
	}
	id := receiptAssemblySemanticID(20)
	if _, ok := fixture.assembly.addSemanticPoint(id, point); !ok {
		t.Fatal("first semantic ID")
	}
	row := issueReceiptAssemblyFixtureRule(t, fixture)
	if _, ok := fixture.assembly.addSemanticRule(id, row); ok {
		t.Fatal("duplicate cross-role semantic ID admitted")
	}
	if fixture.assembly.abort() || !fixture.assembly.sealSources().Available() {
		t.Fatal("duplicate semantic ID failure was not terminal")
	}

	fixture = newReceiptAssemblyRuleFixture(t)
	if fixture.assembly.sealSources().Available() {
		t.Fatal("zero semantic source seal")
	}
	point, pointOK = fixture.assembly.issuePointRow(equation.PointSpec{Site: fixture.site})
	if !pointOK {
		t.Fatal("zero semantic Point receipt")
	}
	if _, ok := fixture.assembly.addSemanticPoint(receiptAssemblySemanticID(0), point); ok {
		t.Fatal("zero semantic ID admitted")
	}
	if fixture.assembly.abort() {
		t.Fatal("zero semantic ID failure was not terminal")
	}

	first := newReceiptAssemblyRuleFixture(t)
	second := newReceiptAssemblyRuleFixture(t)
	if first.assembly.sealSources().Available() || second.assembly.sealSources().Available() {
		t.Fatal("foreign semantic source seal")
	}
	foreign, foreignOK := first.assembly.issuePointRow(equation.PointSpec{Site: first.site})
	if !foreignOK {
		t.Fatal("foreign semantic Point receipt")
	}
	if _, ok := second.assembly.addSemanticPoint(receiptAssemblySemanticID(21), foreign); ok {
		t.Fatal("foreign equal-shape Point receipt admitted")
	}
	if second.assembly.abort() || !first.assembly.abort() {
		t.Fatal("foreign semantic failure lifecycle")
	}
}

func TestReceiptAssemblySemanticQueryDirectoryUsesExactParentReceipt(t *testing.T) {
	schema, factor, rule, write, query := receiptExactQuerySchemaFixture(t)
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) || !BindRule[uint64, uint64, ruleUnit](binding, rule, write, factor, receiptExactQueryRuleSpec(), testRuleProjector[ruleUnit]) || !BindExactQuery(binding, query, factor, hotExactQuerySpec()) || !binding.Seal() {
		t.Fatal("semantic Query binding")
	}
	ruleImplementation, ruleOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, rule)
	_, queryOK := ExactQueryImplementationAt[uint64, uint64](binding, query)
	assembly, assemblyOK := binding.beginBindingTopologyBuilder()
	if !ruleOK || !queryOK || !assemblyOK {
		t.Fatal("semantic Query implementations")
	}
	site, siteOK := assembly.admitSite(compositionKeyOf(coldKey(949_100)), equation.EmptyScope(), equation.TrueExpr(), equation.InitPresent)
	occurrence, occurrenceOK := assembly.admitAt(site)
	operandValue := ruleUnitForSemantic(coldKey(949_101))
	entity, entityOK := operandEntityForContent(operandValue.content)
	operand, operandOK := assembly.admitOperand(occurrence, entity)
	if !siteOK || !occurrenceOK || !entityOK || !operandOK || assembly.sealSources().Available() {
		t.Fatal("semantic Query source")
	}
	proof := ruleImplementation.binding.proof
	source, sourceOK := assembly.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{
		Schema: proof.semantic, OperandFamily: proof.operandFamily, Occurrence: occurrence, Operand: operand,
		Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: proof.output, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}},
	})
	draft, draftOK := ruleImplementation.beginBindingRuleRow(source)
	writePart, writePartOK := ruleImplementation.WritePart(source, 0)
	if !sourceOK || !draftOK || !writePartOK || !draft.AddWrite(writePart) {
		t.Fatal("semantic Query Rule source")
	}
	ruleRow, ruleRowOK := assembly.issueRuleRow(draft)
	if !ruleRowOK {
		t.Fatal("semantic Query Rule topology")
	}
	queryRow := equation.QueryInstance{Family: schema.querySemanticAt(0), Point: equation.PointAt(0), Surfaces: []equation.Surface{{Factor: proof.output, Form: equation.SurfaceReadExact, Local: 1}}}
	declaration := topologyDeclaration{
		binding: binding, batch: assembly.inner.batch,
		points:  []declaredPointRow{{ID: receiptAssemblySemanticID(30), Site: site}},
		members: []declaredMemberRow{{Plane: declaredMemberOwner, ID: receiptAssemblySemanticID(31), Row: ruleRow.row}},
		queries: []declaredQueryRow{{ID: receiptAssemblySemanticID(32), Row: queryRow}},
	}
	constructed, refusal := constructTopology(declaration)
	topology, issued, committed := constructed.topology, constructed.graph, !refusal.Available() && constructed.Available()
	if !committed || issued == nil {
		t.Fatalf("semantic Query commit stage=%v step=%v", refusal.Stage(), refusal.Step())
	}
	// Mutating the caller's row and the declaration's own row after
	// construction must not reach the sealed geometry: constructTopology
	// clones the Surfaces slice it publishes.
	queryRow.Surfaces[0].Local = 99
	declaration.queries[0].Row.Surfaces[0].Local = 100
	graph := CommittedProgramFrom(topology, issued)
	if graph == nil {
		t.Fatal("semantic Query committed program")
	}
	queryResult, found := graph.Query(receiptAssemblySemanticID(32))
	surfaces := queryResult.identity.Surfaces()
	if !found || queryResult.graph != graph.graph || queryResult.topology != graph.topology || !graph.graph.OwnsQuery(queryResult.identity) || queryResult.identity.Family() != schema.querySemanticAt(0) || len(surfaces) != 1 || surfaces[0].Local != 1 {
		t.Fatal("semantic Query direct lookup")
	}
}

type receiptSemanticActivationFixture struct {
	assembly       *BindingTopologyBuilder
	implementation *ActivationRuleImplementation
	triggerSite    equation.Site
	triggerReceipt bindingRuleRow
	materialized   [2]equation.TemplateMaterialization
}

func newReceiptSemanticActivationFixture(t testing.TB) receiptSemanticActivationFixture {
	t.Helper()
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(949_199))
	family, familyOK := DeclareSchemaActivationFamily(builder, coldKey(949_200))
	triggerSlot, triggerOK := DeclareSchemaActivationRule(builder, SchemaStructuralRuleSpec{
		Semantic: coldKey(949_201), Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(949_202)}, Activation: family,
	})
	schema, schemaOK := builder.Seal()
	if !factorOK || !familyOK || !triggerOK || triggerSlot == nil || !schemaOK {
		t.Fatal("semantic Activation schema")
	}
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) || !BindActivationRule(binding, triggerSlot, HotActivationSpec{Admission: AdmitActivationByTrustedTheorem(coldKey(949_202)), Run: func(Activation) bool { return true }}) || !binding.Seal() {
		t.Fatal("semantic Activation binding")
	}
	implementation, implementationOK := ActivationRuleImplementationAt(binding, triggerSlot)
	assembly, assemblyOK := binding.beginBindingTopologyBuilder()
	if !implementationOK || !assemblyOK {
		t.Fatal("semantic Activation implementation")
	}
	scope := equation.EmptyScope()
	triggerSite, triggerSiteOK := assembly.admitSite(compositionKeyOf(coldKey(949_203)), scope, equation.TrueExpr(), equation.InitPresent)
	targetSite, targetSiteOK := assembly.admitSite(compositionKeyOf(coldKey(949_204)), scope, equation.TrueExpr(), equation.InitPresent)
	occurrence, occurrenceOK := assembly.admitAt(triggerSite)
	entity, entityOK := operandEntityForContent([32]byte{31})
	operand, operandOK := assembly.admitOperand(occurrence, entity)
	if !triggerSiteOK || !targetSiteOK || !occurrenceOK || !entityOK || !operandOK || assembly.sealSources().Available() {
		t.Fatal("semantic Activation source")
	}
	proof := implementation.binding.proof
	source, sourceOK := assembly.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{Schema: proof.semantic, OperandFamily: proof.operandFamily, Occurrence: occurrence, Operand: operand})
	draft, draftOK := implementation.beginBindingRuleRow(source)
	ruleReceipt, ruleReceiptOK := assembly.issueRuleRow(draft)
	if !sourceOK || !draftOK || !ruleReceiptOK {
		t.Fatal("semantic Activation trigger member")
	}

	formals := equation.NewBatch()
	input, inputOK := formals.AdmitFormalPort(compositionKeyOf(coldKey(949_205)), equation.PortImport, nil)
	output, outputOK := formals.AdmitFormalPort(compositionKeyOf(coldKey(949_206)), equation.PortExport, nil)
	if !inputOK || !outputOK || !formals.Seal() {
		t.Fatal("semantic Activation formal ports")
	}
	templateBinding, templateBindingOK := equation.SealTemplateBinding(formals, assembly.inner.batch, []equation.FormalPortActual{{Role: input, Site: triggerSite}, {Role: output, Site: targetSite}})
	first, firstOK := equation.MaterializeTemplateBoundary(schema.cold, templateBinding, []equation.Site{input.Site(), output.Site()}, nil)
	second, secondOK := equation.MaterializeTemplateBoundary(schema.cold, templateBinding, []equation.Site{output.Site(), input.Site()}, nil)
	shape, shapeOK := schema.cold.RuleShapeAt(proof.ordinal)
	first, firstOriginOK := first.WithOrigin(equation.MaterializationOrigin{Family: shape.ActivationFamily, Application: compositionKeyOf(coldKey(949_207)), Target: compositionKeyOf(coldKey(949_208)), Endpoint: compositionKeyOf(coldKey(949_209)), TriggerOrdinal: 0})
	second, secondOriginOK := second.WithOrigin(equation.MaterializationOrigin{Family: shape.ActivationFamily, Application: compositionKeyOf(coldKey(949_207)), Target: compositionKeyOf(coldKey(949_210)), Endpoint: compositionKeyOf(coldKey(949_211)), TriggerOrdinal: 0})
	if !templateBindingOK || !firstOK || !secondOK || !shapeOK || shape.OutputKind != composition.StructuralOutput || !firstOriginOK || !secondOriginOK || first.Key() == second.Key() || first.Batch() == second.Batch() {
		t.Fatal("semantic Activation materializations")
	}
	return receiptSemanticActivationFixture{assembly: assembly, implementation: implementation, triggerSite: triggerSite, triggerReceipt: ruleReceipt, materialized: [2]equation.TemplateMaterialization{first, second}}
}

func TestReceiptAssemblySemanticActivationOwnsOneMemberIDAndManyCandidates(t *testing.T) {
	fixture := newReceiptSemanticActivationFixture(t)
	activationID := receiptAssemblySemanticID(42)
	declaration := topologyDeclaration{
		binding: fixture.assembly.binding, batch: fixture.assembly.inner.batch,
		points: []declaredPointRow{{ID: receiptAssemblySemanticID(40), Site: fixture.triggerSite}},
		members: []declaredMemberRow{
			{Plane: declaredMemberOwner, ID: receiptAssemblySemanticID(41), ActivationID: activationID, Activation: true, Row: fixture.triggerReceipt.row},
		},
		materializations: []equation.TemplateMaterialization{fixture.materialized[0], fixture.materialized[1]},
	}
	constructed, refusal := constructTopology(declaration)
	topology, issuedGraph, committed := constructed.topology, constructed.graph, !refusal.Available() && constructed.Available()
	if !committed || topology == nil || issuedGraph == nil {
		t.Fatalf("semantic Activation commit stage=%v step=%v", refusal.Stage(), refusal.Step())
	}
	graph := CommittedProgramFrom(topology, issuedGraph)
	if graph == nil {
		t.Fatal("semantic Activation committed program")
	}
	activationMember, activationMemberOK := graph.ActivationMember(activationID)
	ordinaryMember, ordinaryMemberOK := graph.lookupRuleMember(receiptAssemblySemanticID(41))
	if !activationMemberOK || !ordinaryMemberOK || activationMember.graph != graph.graph || activationMember.topology != graph.topology || activationMember.member.Key() != ordinaryMember.member.Key() {
		t.Fatal("semantic Activation stable trigger lookup")
	}
	secondGraph, secondGraphOK := initialCommittedProgram(topology)
	secondActivationMember, secondActivationMemberOK := secondGraph.ActivationMember(activationID)
	if !secondGraphOK || !secondActivationMemberOK || secondGraph == graph || secondActivationMember.graph != secondGraph.graph || secondActivationMember.topology != secondGraph.topology || secondActivationMember.member.Key() != activationMember.member.Key() {
		t.Fatal("semantic Activation receipt crossed graph revision")
	}
	locator, locatorOK := topology.directory.activation(activationID)
	foreign := newReceiptSemanticActivationFixture(t)
	foreignDeclaration := topologyDeclaration{
		binding: foreign.assembly.binding, batch: foreign.assembly.inner.batch,
		points: []declaredPointRow{{ID: receiptAssemblySemanticID(40), Site: foreign.triggerSite}},
		members: []declaredMemberRow{
			{Plane: declaredMemberOwner, ID: receiptAssemblySemanticID(41), ActivationID: activationID, Activation: true, Row: foreign.triggerReceipt.row},
		},
		materializations: []equation.TemplateMaterialization{foreign.materialized[0], foreign.materialized[1]},
	}
	foreignConstructed, foreignRefusal := constructTopology(foreignDeclaration)
	foreignGraph, foreignCommitted := foreignConstructed.graph, !foreignRefusal.Available() && foreignConstructed.Available()
	if !locatorOK || !foreignCommitted || foreignGraph == nil {
		t.Fatalf("foreign semantic Activation commit stage=%v step=%v", foreignRefusal.Stage(), foreignRefusal.Step())
	}
	if _, crossed := locator.Resolve(foreignGraph); crossed {
		t.Fatal("directory Activation locator resolved against a foreign topology graph")
	}

	duplicate := newReceiptSemanticActivationFixture(t)
	duplicatePoint, duplicatePointOK := duplicate.assembly.issuePointRow(equation.PointSpec{Site: duplicate.triggerSite})
	_, duplicatePointSemanticOK := duplicate.assembly.addSemanticPoint(receiptAssemblySemanticID(40), duplicatePoint)
	duplicateTrigger, duplicateTriggerOK := duplicate.assembly.addSemanticRule(receiptAssemblySemanticID(41), duplicate.triggerReceipt)
	if !duplicatePointOK || !duplicatePointSemanticOK || !duplicateTriggerOK {
		t.Fatal("duplicate semantic Activation trigger member")
	}
	if !duplicate.assembly.addSemanticActivation(receiptAssemblySemanticID(42), duplicateTrigger) {
		t.Fatal("semantic Activation duplicate setup")
	}
	if duplicate.assembly.addSemanticActivation(receiptAssemblySemanticID(43), duplicateTrigger) {
		t.Fatal("one trigger received multiple semantic Activation IDs")
	}
	if duplicate.assembly.abort() {
		t.Fatal("duplicate semantic Activation failure was not terminal")
	}

	mixedApplication := newReceiptSemanticActivationFixture(t)
	mixedPoint, mixedPointOK := mixedApplication.assembly.issuePointRow(equation.PointSpec{Site: mixedApplication.triggerSite})
	_, mixedPointSemanticOK := mixedApplication.assembly.addSemanticPoint(receiptAssemblySemanticID(40), mixedPoint)
	mixedTrigger, mixedTriggerOK := mixedApplication.assembly.addSemanticRule(receiptAssemblySemanticID(41), mixedApplication.triggerReceipt)
	if !mixedPointOK || !mixedPointSemanticOK || !mixedTriggerOK {
		t.Fatal("mixed-application semantic Activation trigger member")
	}
	if !mixedApplication.assembly.addSemanticActivation(receiptAssemblySemanticID(42), mixedTrigger) {
		t.Fatal("mixed-application semantic Activation setup")
	}
	firstReceipt, firstIssued := mixedApplication.assembly.issueMaterialization(mixedApplication.materialized[0])
	if !firstIssued || !mixedApplication.assembly.addActivationCandidate(firstReceipt) {
		t.Fatal("mixed-application first candidate")
	}
	origin, originOK := mixedApplication.materialized[1].Origin()
	origin.Application = compositionKeyOf(coldKey(949_212))
	foreignApplication, rebound := mixedApplication.materialized[1].WithOrigin(origin)
	foreignReceipt, foreignMaterialized := mixedApplication.assembly.issueMaterialization(foreignApplication)
	if !originOK || !rebound || !foreignMaterialized {
		t.Fatal("mixed-application hostile candidate setup")
	}
	if mixedApplication.assembly.addActivationCandidate(foreignReceipt) {
		t.Fatal("one trigger admitted candidates from multiple applications")
	}
	if mixedApplication.assembly.abort() {
		t.Fatal("mixed-application failure was not terminal")
	}
}
