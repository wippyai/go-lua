package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/analysis/identity"
)

func selectedOverlaySemanticID(value byte) identity.ContentID {
	var id identity.ContentID
	id[0] = value
	return id
}

// selectedOverlayFixture is one sealed, unsolved program whose first solve
// takes a real activation revision: an accepted direct activation widens demand
// into a body the initial demand closure does not reach. It is the fixture every
// revision law is measured on, because it is the smallest construction whose
// solve cannot complete without one.
type selectedOverlayFixture struct {
	solver            *Solver
	binding           *SchemaBinding
	graph             *CommittedProgram
	construction      *ProgramConstruction
	observation       receiptObservation[uint64]
	triggerID         identity.ContentID
	bodyID            identity.ContentID
	ordinaryTransfers *int
	// declaration is the same sealed program stated as one geometry
	// declaration. The transitional equivalence law folds it through the pure
	// constructor and measures the result against this fixture's committed
	// geometry.
	declaration topologyDeclaration
}

// TestSelectedOverlayWidensUndemandedWTO proves that an accepted direct
// activation can widen demand into a previously inactive body. The trigger
// and target are initially demanded by the observation; the activation's
// transport selects the body, whose ordinary producer then executes through
// the widened recurrence schedule without rebuilding the sealed runtime.
func TestSelectedOverlayWidensUndemandedWTO(t *testing.T) {
	fixture := newSelectedOverlayFixture(t)
	solver, graph, observation := fixture.solver, fixture.graph, fixture.observation
	baseGraph := solver.runtime.graph
	baseProgram := solver.runtime.program
	baseCarrier := solver.runtime.carrier
	initialRelation := solver.relation
	triggerReceipt, triggerReceiptOK := graph.lookupPoint(fixture.triggerID)
	triggerIndex, triggerIndexOK := solver.runtime.graph.PointIndex(triggerReceipt)
	bodyReceipt, bodyReceiptOK := graph.lookupPoint(fixture.bodyID)
	bodyIndex, bodyIndexOK := solver.runtime.graph.PointIndex(bodyReceipt)
	if !triggerReceiptOK || !triggerIndexOK || triggerIndex < 0 || triggerIndex >= len(solver.runtime.activePoints) || !solver.runtime.activePoints[triggerIndex] || !bodyReceiptOK || !bodyIndexOK || bodyIndex < 0 || bodyIndex >= len(solver.runtime.activePoints) || solver.runtime.activePoints[bodyIndex] {
		t.Fatal("selected overlay did not start with an active trigger and undemanded body")
	}
	state, status, report := solver.SolveWithReport(context.Background())
	if status != SolveComplete || state == nil {
		for index := 0; index < solver.runtime.graph.PointCount(); index++ {
			point, _ := solver.runtime.graph.PointAt(schedule.Node(index))
			t.Logf("selected overlay point index=%d key=%v reportKey=%v active=%t", index, point.Key(), reportedSemanticKey(point.Key()), solver.runtime.activePoints[index])
		}
		t.Fatalf("selected overlay solve state=%t status=%v report=%t reason=%v failure=%v point=%v group=%v member=%v rule=%v", state != nil, status, report.Available(), report.Reason(), report.Failure(), report.Point(), report.Group(), report.Member(), report.Rule())
	}
	sealedRelation, sealedRelationOK := solver.runtime.topology.InitialRelation()
	if !sealedRelationOK || !initialRelation.Precedes(solver.relation) || !sealedRelation.Precedes(solver.relation) || solver.runtime.graph != baseGraph || solver.runtime.program != baseProgram || solver.runtime.carrier != baseCarrier || solver.runtime.graph.RegionCount() == 0 || !solver.runtime.activePoints[triggerIndex] || !solver.runtime.activePoints[bodyIndex] {
		t.Fatalf("selected overlay revision=%d advanced=%t graphSame=%t programSame=%t carrierSame=%t triggerActive=%t bodyActive=%t regions=%d", solver.relation.Generation(), initialRelation.Precedes(solver.relation), solver.runtime.graph == baseGraph, solver.runtime.program == baseProgram, solver.runtime.carrier == baseCarrier, triggerIndex < len(solver.runtime.activePoints) && solver.runtime.activePoints[triggerIndex], bodyIndex < len(solver.runtime.activePoints) && solver.runtime.activePoints[bodyIndex], solver.runtime.graph.RegionCount())
	}
	value, readable := testSnapshotObservationValue[uint64](solver, state, observation.id)
	if !readable || value != 1 || *fixture.ordinaryTransfers == 0 {
		t.Fatalf("selected overlay Snapshot value=%d readable=%t ordinaryTransfers=%d", value, readable, *fixture.ordinaryTransfers)
	}
	sealed, sealedOK := solver.PublishedSnapshot(state)
	if !sealedOK {
		t.Fatal("selected overlay has no sealed Snapshot")
	}
	bodyState, bodyReadable := readPointState(sealed.Snapshot(), bodyReceipt.Key())
	if !bodyReadable || !bodyState.Valid() {
		t.Fatalf("selected overlay body PointState readable=%t valid=%t", bodyReadable, bodyState.Valid())
	}
}

func newSelectedOverlayFixture(t testing.TB) selectedOverlayFixture {
	t.Helper()
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(949_600))
	transportFactor, transportFactorOK := DeclareFactorSlot[uint64](builder, coldKey(949_618))
	write, writeOK := factor.ExactWrite()
	read, readOK := factor.ExactRead()
	query, queryOK := DeclareQuerySlot[uint64](builder, SchemaQuerySpec{Semantic: coldKey(949_601), Freezer: coldKey(949_602)})
	ordinarySlot, ordinaryOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(949_614), OperandFamily: unitOperandFamily, Inputs: 0,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(949_615)}, Output: factor.Ref(),
	})
	ordinaryWrite, ordinaryWriteOK := SchemaWrite(ordinarySlot, write)
	family, familyOK := DeclareSchemaActivationFamily(builder, coldKey(949_603))
	triggerSlot, triggerOK := DeclareSchemaActivationRule(builder, SchemaStructuralRuleSpec{Semantic: coldKey(949_604), Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(949_605)}, Activation: family})
	if !factorOK || !transportFactorOK || !writeOK || !readOK || !queryOK || !SchemaQueryRead(query, read) || !ordinaryOK || !ordinaryWriteOK || !familyOK || !triggerOK {
		t.Fatal("receipt activation schema")
	}
	schema, schemaOK := builder.Seal()
	if !schemaOK || schema == nil {
		t.Fatal("receipt activation schema seal")
	}
	ordinaryTransfers := new(int)
	querySpec := hotExactQuerySpec()
	querySpec.Project = func(cells OrderedCells[uint64]) uint64 {
		value, present, valid := cells.At(0)
		if !valid || !present || value != 1 {
			return ^uint64(0)
		}
		return value
	}
	querySpec.Result.Semantic = coldKey(949_602)
	binding := NewSchemaBinding(schema)
	ordinarySpec := HotRuleSpec[uint64, ruleUnit]{
		OperandContent: ruleUnitContent,
		Admission:      AdmitRuleByTrustedTheorem[uint64, ruleUnit](coldKey(949_615)),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			*ordinaryTransfers++
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}
	if !BindFactor(binding, factor, hotExactObservationFactorSpec()) || !BindFactor(binding, transportFactor, hotUintFactorSpec()) || !BindRule[uint64, uint64, ruleUnit](binding, ordinarySlot, ordinaryWrite, factor, ordinarySpec, testRuleProjector[ruleUnit]) || !BindExactQuery(binding, query, factor, querySpec) {
		t.Fatal("receipt activation factor/query bind")
	}
	application := coldKey(949_606)
	target := coldKey(949_607)
	endpoint := coldKey(949_608)
	activationSpec := HotActivationSpec{Admission: AdmitActivationByTrustedTheorem(coldKey(949_605)), Run: func(value Activation) bool {
		return Activate(value, application, target, endpoint)
	}}
	if !BindActivationRule(binding, triggerSlot, activationSpec) || !binding.Seal() {
		t.Fatal("receipt activation bind")
	}
	activationImplementation, implementationOK := ActivationRuleImplementationAt(binding, triggerSlot)
	ordinaryImplementation, ordinaryImplementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, ordinarySlot)
	queryImplementation, queryImplementationOK := ExactQueryImplementationAt[uint64, uint64](binding, query)
	assembly, assemblyOK := binding.beginBindingTopologyBuilder()
	if !implementationOK || activationImplementation == nil || !ordinaryImplementationOK || ordinaryImplementation == nil || !queryImplementationOK || queryImplementation == nil || !assemblyOK {
		t.Fatal("receipt activation assembly")
	}
	scope := equation.EmptyScope()
	triggerSite, triggerSiteOK := assembly.admitSite(compositionKeyOf(coldKey(949_609)), scope, equation.TrueExpr(), equation.InitPresent)
	targetSite, targetSiteOK := assembly.admitSite(compositionKeyOf(coldKey(949_610)), scope, equation.TrueExpr(), equation.InitPresent)
	bodySite, bodySiteOK := assembly.admitSite(compositionKeyOf(coldKey(949_619)), scope, equation.TrueExpr(), equation.InitPresent)
	occurrence, occurrenceOK := assembly.admitAt(triggerSite)
	entity, entityOK := operandEntityForContent([32]byte{62})
	operand, operandOK := assembly.admitOperand(occurrence, entity)
	ordinaryOccurrence, ordinaryOccurrenceOK := assembly.admitAt(targetSite)
	bodyOccurrence, bodyOccurrenceOK := assembly.admitAt(bodySite)
	ordinaryOperandValue := ruleUnitForSemantic(coldKey(949_616))
	ordinaryEntity, ordinaryEntityOK := operandEntityForContent(ordinaryOperandValue.content)
	ordinaryOperand, ordinaryOperandOK := assembly.admitOperand(ordinaryOccurrence, ordinaryEntity)
	bodyEntity, bodyEntityOK := operandEntityForContent(ordinaryOperandValue.content)
	bodyOperand, bodyOperandOK := assembly.admitOperand(bodyOccurrence, bodyEntity)
	if !triggerSiteOK || !targetSiteOK || !bodySiteOK || !occurrenceOK || !entityOK || !operandOK || !ordinaryOccurrenceOK || !bodyOccurrenceOK || !ordinaryEntityOK || !ordinaryOperandOK || !bodyEntityOK || !bodyOperandOK || assembly.sealSources().Available() {
		t.Fatal("receipt activation source")
	}
	// The declared point plane is trigger, target, body: the PointRef of the
	// nth declared row is PointAt(n).
	triggerPointRef, targetPointRef, bodyPointRef := equation.PointAt(0), equation.PointAt(1), equation.PointAt(2)
	proof := activationImplementation.binding.proof
	source, sourceOK := assembly.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{Schema: proof.semantic, OperandFamily: proof.operandFamily, Occurrence: occurrence, Operand: operand})
	draft, draftOK := activationImplementation.beginBindingRuleRow(source)
	triggerRow, triggerRowOK := assembly.issueRuleRow(draft)
	ordinaryProof := ordinaryImplementation.binding.proof
	ordinarySource, ordinarySourceOK := assembly.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{
		Schema: ordinaryProof.semantic, OperandFamily: ordinaryProof.operandFamily, Occurrence: ordinaryOccurrence, Operand: ordinaryOperand,
		Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: ordinaryProof.output, Form: equation.SurfaceWriteExact, Local: 7, Mode: equation.TargetModeStrong}}},
	})
	ordinaryDraft, ordinaryDraftOK := ordinaryImplementation.beginBindingRuleRow(ordinarySource)
	ordinaryPart, ordinaryPartOK := ordinaryImplementation.WritePart(ordinarySource, 0)
	ordinaryRow, ordinaryRowOK := bindingRuleRow{}, false
	if ordinaryDraftOK && ordinaryPartOK && ordinaryDraft.AddWrite(ordinaryPart) {
		ordinaryRow, ordinaryRowOK = assembly.issueRuleRow(ordinaryDraft)
	}
	bodySource, bodySourceOK := assembly.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{
		Schema: ordinaryProof.semantic, OperandFamily: ordinaryProof.operandFamily, Occurrence: bodyOccurrence, Operand: bodyOperand,
		Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: ordinaryProof.output, Form: equation.SurfaceWriteExact, Local: 7, Mode: equation.TargetModeStrong}}},
	})
	bodyDraft, bodyDraftOK := ordinaryImplementation.beginBindingRuleRow(bodySource)
	bodyPart, bodyPartOK := ordinaryImplementation.WritePart(bodySource, 0)
	bodyRow, bodyRowOK := bindingRuleRow{}, false
	if bodyDraftOK && bodyPartOK && bodyDraft.AddWrite(bodyPart) {
		bodyRow, bodyRowOK = assembly.issueRuleRow(bodyDraft)
	}
	loop := equation.BoundaryInput(triggerSite, triggerSite, compositionKeyOf(coldKey(949_611)), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	triggerDependency := equation.BoundaryInput(triggerSite, targetSite, compositionKeyOf(coldKey(949_617)), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	if !loop.Available() || !triggerDependency.Available() {
		t.Fatal("receipt activation topology group")
	}
	if !sourceOK || !draftOK || !triggerRowOK || !ordinarySourceOK || !ordinaryDraftOK || !ordinaryPartOK || !ordinaryRowOK || !bodySourceOK || !bodyDraftOK || !bodyPartOK || !bodyRowOK {
		t.Fatal("receipt activation topology rows")
	}
	shape, shapeOK := schema.cold.RuleShapeAt(proof.ordinal)
	activationID := selectedOverlaySemanticID(65)
	transportSet, transportSetOK := equation.NewDirectActivationTransportSet(schema.cold, assembly.inner.batch,
		[]equation.PointRef{targetPointRef}, []equation.PointRef{bodyPointRef},
		[]composition.Key{compositionKeyOf(coldKey(949_618))}, compositionKeyOf(coldKey(949_600)))
	origin := equation.MaterializationOrigin{Family: shape.ActivationFamily, Application: compositionKeyOf(application), Target: compositionKeyOf(target), Endpoint: compositionKeyOf(endpoint), TriggerOrdinal: 0}
	candidate, candidateOK := equation.NewDirectActivationCandidate(schema.cold, assembly.inner.batch, origin, triggerPointRef, transportSet)
	if !shapeOK || !transportSetOK || !candidateOK {
		t.Fatal("receipt activation candidate")
	}
	declaration := topologyDeclaration{
		binding: binding, batch: assembly.inner.batch,
		points: []declaredPointRow{
			{ID: selectedOverlaySemanticID(61), Site: triggerSite},
			{ID: selectedOverlaySemanticID(62), Site: targetSite},
			{ID: selectedOverlaySemanticID(64), Site: bodySite},
		},
		members: []declaredMemberRow{
			{Plane: declaredMemberOwner, ID: selectedOverlaySemanticID(63), ActivationID: activationID, Activation: true, Row: triggerRow.row, EnvironmentInput: loop},
			{Plane: declaredMemberOwner, ID: selectedOverlaySemanticID(67), Row: ordinaryRow.row, EnvironmentInput: triggerDependency},
			{Plane: declaredMemberOwner, ID: selectedOverlaySemanticID(68), Row: bodyRow.row},
		},
		directCandidates:   []equation.DirectActivationCandidate{candidate},
		observationQueries: true,
	}
	constructed, refusal := constructTopology(declaration)
	topology, issued := constructed.topology, constructed.graph
	if refusal.Available() || !constructed.Available() {
		t.Fatalf("receipt activation construction stage=%v step=%v ordinal=%d", refusal.Stage(), refusal.Step(), refusal.Ordinal())
	}
	graph := CommittedProgramFrom(topology, issued)
	if graph == nil {
		t.Fatal("receipt activation committed program")
	}
	_, activationMemberOK := graph.ActivationMember(activationID)
	ordinaryMember, ordinaryMemberOK := graph.RuleMember(selectedOverlaySemanticID(67))
	if !activationMemberOK || !ordinaryMemberOK {
		t.Fatal("receipt activation graph receipts")
	}
	ordinarySurface, ordinarySurfaceOK := ordinaryMember.member.WriteAt(0)
	if !ordinarySurfaceOK || ordinarySurface.Factor != ordinaryProof.output || ordinarySurface.Form != equation.SurfaceWriteExact || ordinarySurface.Mode != equation.TargetModeStrong || ordinarySurface.Local != 7 {
		t.Fatal("receipt activation committed ordinary write coordinate")
	}
	compilation, compilationOK := BeginProgramConstruction(binding, graph)
	if !compilationOK || compilation == nil {
		t.Fatal("receipt activation compilation")
	}
	if !installConstOperandResolver(ordinaryImplementation, ordinaryOperandValue) {
		t.Fatal("receipt ordinary resolver")
	}
	if !AttachRuleMember(compilation, ordinaryImplementation, selectedOverlaySemanticID(67)) {
		t.Fatal("receipt ordinary attachment")
	}
	if !AttachRuleMember(compilation, ordinaryImplementation, selectedOverlaySemanticID(68)) {
		t.Fatal("receipt body attachment")
	}
	if !AttachActivationMember(compilation, activationImplementation, activationID) {
		t.Fatal("receipt activation attachment")
	}
	observation, observationFailure := AttachRuleExactObservationWithFailure(compilation, queryImplementation, selectedOverlaySemanticID(66), ordinaryMember)
	if observationFailure != receiptObservationAttachFailureNone || !observation.Available() {
		t.Fatalf("receipt activation exact observation failure=%v available=%t", observationFailure, observation.Available())
	}
	solver, _, solverOK := compilation.Seal()
	if !solverOK || solver == nil || solver.runtime == nil || solver.runtime.program == nil || solver.runtime.carrier == nil {
		t.Fatal("selected overlay solver")
	}
	return selectedOverlayFixture{
		solver: solver, binding: binding, graph: graph, construction: compilation, observation: observation,
		triggerID: selectedOverlaySemanticID(61), bodyID: selectedOverlaySemanticID(64), ordinaryTransfers: ordinaryTransfers,
		declaration: declaration,
	}
}

func TestReceiptExactObservationRejectsNonExactWriteMetadata(t *testing.T) {
	factor := compositionKeyOf(coldKey(949_620))
	write := equation.Surface{Factor: factor, Form: equation.SurfaceWriteExact, Local: 7, Mode: equation.TargetModeStrong}
	for name, fixture := range map[string]exactObservationWriteFixture{
		"route": {count: 1, surface: write, route: 1},
	} {
		if read, accepted := exactObservationReadSurface(fixture, factor); accepted || read.Available() {
			t.Fatalf("exact observation accepted write metadata %s", name)
		}
	}
}

func initialCommittedProgram(topology *BindingTopology) (*CommittedProgram, bool) {
	if topology == nil || !topology.valid() {
		return nil, false
	}
	relation, ok := topology.topology.InitialRelation()
	if !ok {
		return nil, false
	}
	graph, ok := topology.Graph(relation)
	program := CommittedProgramFrom(topology, graph)
	return program, ok && program != nil
}

// initialEquationGraph resolves the sealed base publication of a Topology and
// the graph issued for it.
func initialEquationGraph(topology *equation.Topology) (*equation.Graph, bool) {
	relation, ok := topology.InitialRelation()
	if !ok {
		return nil, false
	}
	return topology.Graph(relation)
}
