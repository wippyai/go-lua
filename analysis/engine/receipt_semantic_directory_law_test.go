package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

func issueReceiptAssemblyFixtureRule(t testing.TB, fixture receiptAssemblyRuleFixture) BindingRuleRowReceipt {
	t.Helper()
	builder := fixture.assembly.builder
	source, sourceOK := builder.issueRuleSurfaceSource(fixture.sourceSpec())
	draft, draftOK := fixture.implementation.BeginBindingRuleRow(source)
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
	alias := *fixture.assembly
	if !alias.SealSources() {
		t.Fatal("semantic directory source seal")
	}
	fixture.addTopology(t)
	topology, first, ok := alias.Commit()
	if !ok || topology == nil || first == nil {
		t.Fatal("semantic directory commit")
	}
	point, pointOK := first.lookupPoint(receiptAssemblySemanticID(1))
	member, memberOK := first.lookupRuleMember(receiptAssemblySemanticID(2))
	if !pointOK || point.graph != first || !first.graph.OwnsPoint(point.point) || !memberOK || member.graph != first || !first.graph.OwnsMember(member.member) {
		t.Fatal("semantic directory exact first-revision receipts")
	}
	if _, found := first.lookupPoint(receiptAssemblySemanticID(99)); found {
		t.Fatal("unknown semantic Point ID resolved")
	}
	if _, found := first.lookupRuleMember(receiptAssemblySemanticID(0)); found {
		t.Fatal("zero semantic member ID resolved")
	}
	second, secondOK := initialReceiptGraph(topology)
	secondPoint, secondPointOK := second.lookupPoint(receiptAssemblySemanticID(1))
	secondMember, secondMemberOK := second.lookupRuleMember(receiptAssemblySemanticID(2))
	if !secondOK || second == nil || second == first || !secondPointOK || secondPoint.graph != second || !second.graph.OwnsPoint(secondPoint.point) || !secondMemberOK || secondMember.graph != second || !second.graph.OwnsMember(secondMember.member) {
		t.Fatal("semantic directory exact second-revision receipts")
	}
	// The directory is the topology's organ: every revision handle resolves
	// one ContentID to the same immutable structural row.
	if secondPoint.point.Key() != point.point.Key() || secondMember.member.Key() != member.member.Key() {
		t.Fatal("semantic directory resolved one identity to two rows")
	}
	pointLocator, pointLocatorOK := topology.directory.point(receiptAssemblySemanticID(1))
	memberLocator, memberLocatorOK := topology.directory.member(receiptAssemblySemanticID(2))
	foreign := newReceiptAssemblyRuleFixture(t)
	if !foreign.assembly.SealSources() {
		t.Fatal("foreign topology source seal")
	}
	foreign.addTopology(t)
	_, foreignGraph, foreignOK := foreign.assembly.Commit()
	if !pointLocatorOK || !memberLocatorOK || !foreignOK || foreignGraph == nil {
		t.Fatal("foreign topology commit")
	}
	if _, crossed := pointLocator.Resolve(foreignGraph.graph); crossed {
		t.Fatal("directory Point locator resolved against a foreign topology graph")
	}
	if _, crossed := memberLocator.Resolve(foreignGraph.graph); crossed {
		t.Fatal("directory member locator resolved against a foreign topology graph")
	}
}

func TestReceiptAssemblySemanticDirectoryDuplicateZeroAndForeignFailTerminally(t *testing.T) {
	fixture := newReceiptAssemblyRuleFixture(t)
	if !fixture.assembly.SealSources() {
		t.Fatal("duplicate semantic source seal")
	}
	point, pointOK := fixture.assembly.builder.issuePointRow(equation.PointSpec{Site: fixture.site})
	if !pointOK {
		t.Fatal("duplicate semantic Point receipt")
	}
	id := receiptAssemblySemanticID(20)
	if _, ok := fixture.assembly.builder.addSemanticPoint(id, point); !ok {
		t.Fatal("first semantic ID")
	}
	row := issueReceiptAssemblyFixtureRule(t, fixture)
	if _, ok := fixture.assembly.builder.addSemanticRule(id, row); ok {
		t.Fatal("duplicate cross-role semantic ID admitted")
	}
	if fixture.assembly.Abort() || fixture.assembly.SealSources() {
		t.Fatal("duplicate semantic ID failure was not terminal")
	}

	fixture = newReceiptAssemblyRuleFixture(t)
	if !fixture.assembly.SealSources() {
		t.Fatal("zero semantic source seal")
	}
	point, pointOK = fixture.assembly.builder.issuePointRow(equation.PointSpec{Site: fixture.site})
	if !pointOK {
		t.Fatal("zero semantic Point receipt")
	}
	if _, ok := fixture.assembly.builder.addSemanticPoint(receiptAssemblySemanticID(0), point); ok {
		t.Fatal("zero semantic ID admitted")
	}
	if fixture.assembly.Abort() {
		t.Fatal("zero semantic ID failure was not terminal")
	}

	first := newReceiptAssemblyRuleFixture(t)
	second := newReceiptAssemblyRuleFixture(t)
	if !first.assembly.SealSources() || !second.assembly.SealSources() {
		t.Fatal("foreign semantic source seal")
	}
	foreign, foreignOK := first.assembly.builder.issuePointRow(equation.PointSpec{Site: first.site})
	if !foreignOK {
		t.Fatal("foreign semantic Point receipt")
	}
	if _, ok := second.assembly.builder.addSemanticPoint(receiptAssemblySemanticID(21), foreign); ok {
		t.Fatal("foreign equal-shape Point receipt admitted")
	}
	if second.assembly.Abort() || !first.assembly.Abort() {
		t.Fatal("foreign semantic failure lifecycle")
	}
}

func TestReceiptAssemblySemanticQueryDirectoryUsesExactParentReceipt(t *testing.T) {
	schema, factor, rule, write, query := receiptExactQuerySchemaFixture(t)
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) || !BindRule[uint64, uint64, ruleUnit](binding, rule, write, factor, receiptExactQueryRuleSpec()) || !BindExactQuery(binding, query, factor, hotExactQuerySpec()) || !binding.Seal() {
		t.Fatal("semantic Query binding")
	}
	ruleImplementation, ruleOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, rule)
	queryImplementation, queryOK := ExactQueryImplementationAt[uint64, uint64](binding, query)
	assembly, assemblyOK := beginReceiptAssembly(binding)
	if !ruleOK || !queryOK || !assemblyOK {
		t.Fatal("semantic Query implementations")
	}
	site, siteOK := assembly.builder.admitSite(coldKey(949_100).compositionKey(), equation.EmptyScope(), equation.TrueExpr(), equation.InitPresent)
	occurrence, occurrenceOK := assembly.builder.admitAt(site)
	operandValue := ruleUnitForSemantic(coldKey(949_101))
	entity, entityOK := operandEntityForContent(operandValue.content)
	operand, operandOK := assembly.builder.admitOperand(occurrence, entity)
	if !siteOK || !occurrenceOK || !entityOK || !operandOK || !assembly.SealSources() {
		t.Fatal("semantic Query source")
	}
	pointReceipt, pointReceiptOK := assembly.builder.issuePointRow(equation.PointSpec{Site: site})
	point, pointOK := assembly.builder.addSemanticPoint(receiptAssemblySemanticID(30), pointReceipt)
	proof := ruleImplementation.receipt.proof
	source, sourceOK := assembly.builder.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{
		Schema: proof.semantic, OperandFamily: proof.operandFamily, Occurrence: occurrence, Operand: operand,
		Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: proof.output, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}},
	})
	draft, draftOK := ruleImplementation.BeginBindingRuleRow(source)
	writePart, writePartOK := ruleImplementation.WritePart(source, 0)
	if !pointReceiptOK || !pointOK || !sourceOK || !draftOK || !writePartOK || !draft.AddWrite(writePart) {
		t.Fatal("semantic Query Rule source")
	}
	ruleReceipt, ruleReceiptOK := assembly.builder.issueRuleRow(draft)
	if _, memberOK := assembly.builder.addSemanticRule(receiptAssemblySemanticID(31), ruleReceipt); !ruleReceiptOK || !memberOK {
		t.Fatal("semantic Query Rule topology")
	}
	queryRow := equation.QueryInstance{Family: schema.querySemanticAt(0), Point: point.ref, Surfaces: []equation.Surface{{Factor: proof.output, Form: equation.SurfaceReadExact, Local: 1}}}
	queryReceipt, queryReceiptOK := assembly.builder.issueQueryRow(queryImplementation, queryRow)
	if !queryReceiptOK {
		t.Fatal("semantic Query row receipt")
	}
	if _, ok := assembly.builder.addSemanticQuery(receiptAssemblySemanticID(32), queryReceipt); !ok {
		t.Fatal("semantic Query registration")
	}
	queryRow.Surfaces[0].Local = 99
	queryReceipt.row.Surfaces[0].Local = 100
	_, graph, committed := assembly.Commit()
	if !committed || graph == nil {
		t.Fatal("semantic Query commit")
	}
	queryResult, found := graph.lookupQuery(receiptAssemblySemanticID(32))
	surfaces := queryResult.identity.Surfaces()
	if !found || queryResult.graph != graph || !graph.graph.OwnsQuery(queryResult.identity) || queryResult.identity.Family() != schema.querySemanticAt(0) || len(surfaces) != 1 || surfaces[0].Local != 1 {
		t.Fatal("semantic Query direct lookup")
	}
}

type receiptSemanticActivationFixture struct {
	assembly       *ReceiptAssembly
	implementation *ActivationRuleImplementation
	trigger        BindingRuleRowRef
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
	assembly, assemblyOK := beginReceiptAssembly(binding)
	if !implementationOK || !assemblyOK {
		t.Fatal("semantic Activation implementation")
	}
	scope := equation.EmptyScope()
	triggerSite, triggerSiteOK := assembly.builder.admitSite(coldKey(949_203).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	targetSite, targetSiteOK := assembly.builder.admitSite(coldKey(949_204).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	occurrence, occurrenceOK := assembly.builder.admitAt(triggerSite)
	entity, entityOK := operandEntityForContent([32]byte{31})
	operand, operandOK := assembly.builder.admitOperand(occurrence, entity)
	if !triggerSiteOK || !targetSiteOK || !occurrenceOK || !entityOK || !operandOK || !assembly.SealSources() {
		t.Fatal("semantic Activation source")
	}
	pointReceipt, pointReceiptOK := assembly.builder.issuePointRow(equation.PointSpec{Site: triggerSite})
	_, pointOK := assembly.builder.addSemanticPoint(receiptAssemblySemanticID(40), pointReceipt)
	proof := implementation.receipt.proof
	source, sourceOK := assembly.builder.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{Schema: proof.semantic, OperandFamily: proof.operandFamily, Occurrence: occurrence, Operand: operand})
	draft, draftOK := implementation.BeginBindingRuleRow(source)
	ruleReceipt, ruleReceiptOK := assembly.builder.issueRuleRow(draft)
	trigger, triggerRefOK := assembly.builder.addSemanticRule(receiptAssemblySemanticID(41), ruleReceipt)
	if !pointReceiptOK || !pointOK || !sourceOK || !draftOK || !ruleReceiptOK || !triggerRefOK {
		t.Fatal("semantic Activation trigger member")
	}

	formals := equation.NewBatch()
	input, inputOK := formals.AdmitFormalPort(coldKey(949_205).compositionKey(), equation.PortImport, nil)
	output, outputOK := formals.AdmitFormalPort(coldKey(949_206).compositionKey(), equation.PortExport, nil)
	if !inputOK || !outputOK || !formals.Seal() {
		t.Fatal("semantic Activation formal ports")
	}
	templateBinding, templateBindingOK := equation.SealTemplateBinding(formals, assembly.builder.inner.batch, []equation.FormalPortActual{{Role: input, Site: triggerSite}, {Role: output, Site: targetSite}})
	first, firstOK := equation.MaterializeTemplateBoundary(schema.cold, templateBinding, []equation.Site{input.Site(), output.Site()}, nil)
	second, secondOK := equation.MaterializeTemplateBoundary(schema.cold, templateBinding, []equation.Site{output.Site(), input.Site()}, nil)
	shape, shapeOK := schema.cold.RuleShapeAt(proof.ordinal)
	first, firstOriginOK := first.WithOrigin(equation.MaterializationOrigin{Family: shape.ActivationFamily, Application: coldKey(949_207).compositionKey(), Target: coldKey(949_208).compositionKey(), Endpoint: coldKey(949_209).compositionKey(), TriggerOrdinal: 0})
	second, secondOriginOK := second.WithOrigin(equation.MaterializationOrigin{Family: shape.ActivationFamily, Application: coldKey(949_207).compositionKey(), Target: coldKey(949_210).compositionKey(), Endpoint: coldKey(949_211).compositionKey(), TriggerOrdinal: 0})
	if !templateBindingOK || !firstOK || !secondOK || !shapeOK || shape.OutputKind != composition.StructuralOutput || !firstOriginOK || !secondOriginOK || first.Key() == second.Key() || first.Batch() == second.Batch() {
		t.Fatal("semantic Activation materializations")
	}
	return receiptSemanticActivationFixture{assembly: assembly, implementation: implementation, trigger: trigger, materialized: [2]equation.TemplateMaterialization{first, second}}
}

func TestReceiptAssemblySemanticActivationOwnsOneMemberIDAndManyCandidates(t *testing.T) {
	fixture := newReceiptSemanticActivationFixture(t)
	activationID := receiptAssemblySemanticID(42)
	if !fixture.assembly.builder.addSemanticActivation(activationID, fixture.trigger) {
		t.Fatal("semantic Activation member registration")
	}
	for _, value := range fixture.materialized {
		receipt, issued := fixture.assembly.builder.issueMaterialization(value)
		if !issued || !fixture.assembly.builder.addActivationCandidate(receipt) {
			t.Fatal("semantic Activation candidate registration")
		}
	}
	topology, graph, committed := fixture.assembly.Commit()
	if !committed || topology == nil || graph == nil {
		t.Fatal("semantic Activation commit")
	}
	activationGraph, activationGraphOK := activationReceiptGraph(graph)
	activationMember, activationMemberOK := activationGraph.lookupActivationMember(activationID)
	ordinaryMember, ordinaryMemberOK := graph.lookupRuleMember(receiptAssemblySemanticID(41))
	if !activationGraphOK || !activationMemberOK || !ordinaryMemberOK || activationMember.graph != activationGraph || activationMember.member.Key() != ordinaryMember.member.Key() {
		t.Fatal("semantic Activation stable trigger lookup")
	}
	secondGraph, secondGraphOK := initialReceiptGraph(topology)
	secondActivationGraph, secondActivationGraphOK := activationReceiptGraph(secondGraph)
	secondActivationMember, secondActivationMemberOK := secondActivationGraph.lookupActivationMember(activationID)
	if !secondGraphOK || !secondActivationGraphOK || !secondActivationMemberOK || secondGraph == graph || secondActivationMember.graph != secondActivationGraph || secondActivationMember.member.Key() != activationMember.member.Key() {
		t.Fatal("semantic Activation receipt crossed graph revision")
	}
	locator, locatorOK := topology.directory.activation(activationID)
	foreign := newReceiptSemanticActivationFixture(t)
	if !foreign.assembly.builder.addSemanticActivation(activationID, foreign.trigger) {
		t.Fatal("foreign semantic Activation registration")
	}
	for _, value := range foreign.materialized {
		receipt, issued := foreign.assembly.builder.issueMaterialization(value)
		if !issued || !foreign.assembly.builder.addActivationCandidate(receipt) {
			t.Fatal("foreign semantic Activation candidate")
		}
	}
	_, foreignGraph, foreignCommitted := foreign.assembly.Commit()
	if !locatorOK || !foreignCommitted || foreignGraph == nil {
		t.Fatal("foreign semantic Activation commit")
	}
	if _, crossed := locator.Resolve(foreignGraph.graph); crossed {
		t.Fatal("directory Activation locator resolved against a foreign topology graph")
	}

	duplicate := newReceiptSemanticActivationFixture(t)
	if !duplicate.assembly.builder.addSemanticActivation(receiptAssemblySemanticID(42), duplicate.trigger) {
		t.Fatal("semantic Activation duplicate setup")
	}
	if duplicate.assembly.builder.addSemanticActivation(receiptAssemblySemanticID(43), duplicate.trigger) {
		t.Fatal("one trigger received multiple semantic Activation IDs")
	}
	if duplicate.assembly.Abort() {
		t.Fatal("duplicate semantic Activation failure was not terminal")
	}

	mixedApplication := newReceiptSemanticActivationFixture(t)
	if !mixedApplication.assembly.builder.addSemanticActivation(receiptAssemblySemanticID(42), mixedApplication.trigger) {
		t.Fatal("mixed-application semantic Activation setup")
	}
	firstReceipt, firstIssued := mixedApplication.assembly.builder.issueMaterialization(mixedApplication.materialized[0])
	if !firstIssued || !mixedApplication.assembly.builder.addActivationCandidate(firstReceipt) {
		t.Fatal("mixed-application first candidate")
	}
	origin, originOK := mixedApplication.materialized[1].Origin()
	origin.Application = coldKey(949_212).compositionKey()
	foreignApplication, rebound := mixedApplication.materialized[1].WithOrigin(origin)
	foreignReceipt, foreignIssued := mixedApplication.assembly.builder.issueMaterialization(foreignApplication)
	if !originOK || !rebound || !foreignIssued {
		t.Fatal("mixed-application hostile candidate setup")
	}
	if mixedApplication.assembly.builder.addActivationCandidate(foreignReceipt) {
		t.Fatal("one trigger admitted candidates from multiple applications")
	}
	if mixedApplication.assembly.Abort() {
		t.Fatal("mixed-application failure was not terminal")
	}
}
