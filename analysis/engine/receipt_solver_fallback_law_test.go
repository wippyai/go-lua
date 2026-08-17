package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

func receiptSolverFallbackSemanticID(value byte) identity.ContentID {
	var id identity.ContentID
	id[0] = value
	return id
}

// TestReceiptSolverFallbackRunsAcceptedActivationThroughWTORevision enters
// Solver.solve with a real accepted activation. The initial graph contains a
// self-loop Region, making the selected overlay ineligible and forcing the
// receipt compiler to resolve all row locators against the distinct revision
// graph before execution continues.
func TestReceiptSolverFallbackRunsAcceptedActivationThroughWTORevision(t *testing.T) {
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(949_600))
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
	if !factorOK || !writeOK || !readOK || !queryOK || !SchemaQueryRead(query, read) || !ordinaryOK || !ordinaryWriteOK || !familyOK || !triggerOK {
		t.Fatal("receipt activation schema")
	}
	schema, schemaOK := builder.Seal()
	if !schemaOK || schema == nil {
		t.Fatal("receipt activation schema seal")
	}
	ordinaryTransfers := 0
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
			ordinaryTransfers++
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}
	if !BindFactor(binding, factor, hotExactObservationFactorSpec()) || !BindRule[uint64, uint64, ruleUnit](binding, ordinarySlot, ordinaryWrite, factor, ordinarySpec) || !BindExactQuery(binding, query, factor, querySpec) {
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
	assembly, assemblyOK := beginReceiptAssembly(binding)
	if !implementationOK || activationImplementation == nil || !ordinaryImplementationOK || ordinaryImplementation == nil || !queryImplementationOK || queryImplementation == nil || !assemblyOK {
		t.Fatal("receipt activation assembly")
	}
	scope := equation.EmptyScope()
	triggerSite, triggerSiteOK := assembly.builder.admitSite(compositionKeyOf(coldKey(949_609)), scope, equation.TrueExpr(), equation.InitPresent)
	targetSite, targetSiteOK := assembly.builder.admitSite(compositionKeyOf(coldKey(949_610)), scope, equation.TrueExpr(), equation.InitPresent)
	occurrence, occurrenceOK := assembly.builder.admitAt(triggerSite)
	entity, entityOK := operandEntityForContent([32]byte{62})
	operand, operandOK := assembly.builder.admitOperand(occurrence, entity)
	ordinaryOccurrence, ordinaryOccurrenceOK := assembly.builder.admitAt(targetSite)
	ordinaryOperandValue := ruleUnitForSemantic(coldKey(949_616))
	ordinaryEntity, ordinaryEntityOK := operandEntityForContent(ordinaryOperandValue.content)
	ordinaryOperand, ordinaryOperandOK := assembly.builder.admitOperand(ordinaryOccurrence, ordinaryEntity)
	if !triggerSiteOK || !targetSiteOK || !occurrenceOK || !entityOK || !operandOK || !ordinaryOccurrenceOK || !ordinaryEntityOK || !ordinaryOperandOK || !assembly.SealSources() {
		t.Fatal("receipt activation source")
	}
	triggerPoint, triggerPointOK := assembly.builder.issuePointRow(equation.PointSpec{Site: triggerSite})
	_, triggerPointSemanticOK := assembly.builder.addSemanticPoint(receiptSolverFallbackSemanticID(61), triggerPoint)
	targetPoint, targetPointOK := assembly.builder.issuePointRow(equation.PointSpec{Site: targetSite})
	_, targetPointSemanticOK := assembly.builder.addSemanticPoint(receiptSolverFallbackSemanticID(62), targetPoint)
	proof := activationImplementation.receipt.proof
	source, sourceOK := assembly.builder.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{Schema: proof.semantic, OperandFamily: proof.operandFamily, Occurrence: occurrence, Operand: operand})
	draft, draftOK := activationImplementation.BeginBindingRuleRow(source)
	triggerRow, triggerRowOK := assembly.builder.issueRuleRow(draft)
	triggerRowRef, triggerRowSemanticOK := assembly.builder.addSemanticRule(receiptSolverFallbackSemanticID(63), triggerRow)
	ordinaryProof := ordinaryImplementation.receipt.proof
	ordinarySource, ordinarySourceOK := assembly.builder.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{
		Schema: ordinaryProof.semantic, OperandFamily: ordinaryProof.operandFamily, Occurrence: ordinaryOccurrence, Operand: ordinaryOperand,
		Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: ordinaryProof.output, Form: equation.SurfaceWriteExact, Local: 7, Mode: equation.TargetModeStrong}}},
	})
	ordinaryDraft, ordinaryDraftOK := ordinaryImplementation.BeginBindingRuleRow(ordinarySource)
	ordinaryPart, ordinaryPartOK := ordinaryImplementation.WritePart(ordinarySource, 0)
	ordinaryRow, ordinaryRowOK := BindingRuleRowReceipt{}, false
	ordinaryRowRef := BindingRuleRowRef{}
	ordinaryRowSemanticOK := false
	if ordinaryDraftOK && ordinaryPartOK && ordinaryDraft.AddWrite(ordinaryPart) {
		ordinaryRow, ordinaryRowOK = assembly.builder.issueRuleRow(ordinaryDraft)
		if ordinaryRowOK {
			ordinaryRowRef, ordinaryRowSemanticOK = assembly.builder.addSemanticRule(receiptSolverFallbackSemanticID(67), ordinaryRow)
		}
	}
	loop := equation.BoundaryInput(triggerSite, triggerSite, compositionKeyOf(coldKey(949_611)), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	triggerDependency := equation.BoundaryInput(triggerSite, targetSite, compositionKeyOf(coldKey(949_617)), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	setGroupEnvironmentInput := func(member BindingRuleRowRef, input equation.Input) bool {
		if member.builder != assembly.builder.inner || !input.Available() {
			return false
		}
		matched := 0
		for index := range assembly.builder.inner.spec.Groups {
			group := &assembly.builder.inner.spec.Groups[index]
			for _, groupMember := range group.Members {
				if groupMember != member.ref {
					continue
				}
				if group.EnvironmentInput.Available() {
					return false
				}
				group.EnvironmentInput = input
				matched++
			}
		}
		return matched == 1
	}
	if len(assembly.builder.inner.spec.Groups) != 2 || !loop.Available() || !triggerDependency.Available() ||
		!setGroupEnvironmentInput(triggerRowRef, loop) || !setGroupEnvironmentInput(ordinaryRowRef, triggerDependency) {
		t.Fatal("receipt activation topology group")
	}
	if !triggerPointOK || !triggerPointSemanticOK || !targetPointOK || !targetPointSemanticOK || !sourceOK || !draftOK || !triggerRowOK || !triggerRowSemanticOK || !ordinarySourceOK || !ordinaryDraftOK || !ordinaryPartOK || !ordinaryRowOK || !ordinaryRowSemanticOK {
		t.Fatal("receipt activation topology rows")
	}
	formals := equation.NewBatch()
	input, inputOK := formals.AdmitFormalPort(compositionKeyOf(coldKey(949_612)), equation.PortImport, nil)
	output, outputOK := formals.AdmitFormalPort(compositionKeyOf(coldKey(949_613)), equation.PortExport, nil)
	if !inputOK || !outputOK || !formals.Seal() {
		t.Fatal("receipt activation formals")
	}
	templateBinding, templateBindingOK := equation.SealTemplateBinding(formals, assembly.builder.inner.batch, []equation.FormalPortActual{{Role: input, Site: triggerSite}, {Role: output, Site: targetSite}})
	materialization, materializationOK := equation.MaterializeTemplateBoundary(schema.cold, templateBinding, []equation.Site{input.Site(), output.Site()}, nil)
	shape, shapeOK := schema.cold.RuleShapeAt(proof.ordinal)
	materialization, originOK := materialization.WithOrigin(equation.MaterializationOrigin{Family: shape.ActivationFamily, Application: compositionKeyOf(application), Target: compositionKeyOf(target), Endpoint: compositionKeyOf(endpoint), TriggerOrdinal: 0})
	activationID := receiptSolverFallbackSemanticID(65)
	materializationReceipt, materializationReceiptOK := assembly.builder.issueMaterialization(materialization)
	if !templateBindingOK || !materializationOK || !shapeOK || !originOK || !assembly.builder.addSemanticActivation(activationID, triggerRowRef) || !materializationReceiptOK || !assembly.builder.addActivationCandidate(materializationReceipt) {
		t.Fatal("receipt activation candidate")
	}
	if _, topologyFailure, topologyOK := equation.SealObservationTopologyWithFailure(schema.cold, assembly.builder.inner.spec); !topologyOK {
		spec := assembly.builder.inner.spec
		shape, _ := schema.cold.RuleShapeAt(proof.ordinal)
		t.Fatalf("receipt activation topology preflight failure=%v rules=%d groups=%d points=%d inputs=%d env=%t shapeInputs=%d", topologyFailure, len(spec.Rules), len(spec.Groups), len(spec.Points), len(spec.Groups[0].Inputs), spec.Groups[0].EnvironmentInput.Available(), shape.Inputs)
	}
	_, graph, committed := assembly.CommitObservationTopology()
	if !committed || graph == nil {
		failure, available := assembly.CommitFailure()
		t.Fatalf("receipt activation commit committed=%t graph=%t failure=%v available=%t", committed, graph != nil, failure, available)
	}
	activationGraph, activationGraphOK := activationReceiptGraph(graph)
	activationMember, activationMemberOK := activationGraph.lookupActivationMember(activationID)
	ordinaryMember, ordinaryMemberOK := graph.RuleMember(receiptSolverFallbackSemanticID(67))
	if !activationGraphOK || !activationMemberOK || !ordinaryMemberOK {
		t.Fatal("receipt activation graph receipts")
	}
	ordinarySurface, ordinarySurfaceOK := ordinaryMember.member.WriteAt(0)
	if !ordinarySurfaceOK || ordinarySurface.Factor != ordinaryProof.output || ordinarySurface.Form != equation.SurfaceWriteExact || ordinarySurface.Mode != equation.TargetModeStrong || ordinarySurface.Local != 7 {
		t.Fatal("receipt activation committed ordinary write coordinate")
	}
	compilation, compilationOK := BeginReceiptActivationCompilation(activationImplementation, graph)
	if !compilationOK || compilation == nil {
		t.Fatal("receipt activation compilation")
	}
	if _, attached := AttachReceiptRuleMember(compilation, ordinaryImplementation, ordinaryMember, ordinaryOperandValue); !attached {
		t.Fatal("receipt ordinary attachment")
	}
	if _, attached := AttachReceiptActivationMember(compilation, activationImplementation, activationMember); !attached {
		t.Fatal("receipt activation attachment")
	}
	observation, observationFailure := AttachRuleExactObservationWithFailure(compilation, queryImplementation, receiptSolverFallbackSemanticID(66), ordinaryMember)
	if observationFailure != ReceiptObservationAttachFailureNone || !observation.Available() {
		t.Fatalf("receipt activation exact observation failure=%v available=%t", observationFailure, observation.Available())
	}
	solver, solverOK := compilation.Solver()
	if !solverOK || solver == nil || solver.compiler == nil {
		t.Fatal("receipt activation solver compiler")
	}
	state, status := solver.Solve(context.Background())
	if status != SolveComplete || state == nil {
		t.Fatalf("receipt activation solve state=%t status=%v", state != nil, status)
	}
	sealedRelation, sealedRelationOK := solver.runtime.topology.InitialRelation()
	if !sealedRelationOK || !sealedRelation.Precedes(solver.relation) || solver.runtime == nil || solver.runtime.graph == graph.graph || solver.runtime.graph.RegionCount() == 0 {
		t.Fatalf("receipt activation revision=%d runtime=%t distinct=%t regions=%d", solver.relation.Generation(), solver.runtime != nil, solver.runtime != nil && solver.runtime.graph != graph.graph, func() int {
			if solver.runtime == nil || solver.runtime.graph == nil {
				return 0
			}
			return solver.runtime.graph.RegionCount()
		}())
	}
	value, readable := ReceiptObservationResult[uint64](observation, solver, state)
	if !readable || value != 1 || ordinaryTransfers == 0 {
		t.Fatalf("exact observation post-revision value=%d readable=%t ordinaryTransfers=%d", value, readable, ordinaryTransfers)
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

// initialReceiptGraph is the test-local spelling of "the sealed base
// publication": the first Relation of a BindingTopology and its graph receipt.
func initialReceiptGraph(topology *BindingTopology) (*ReceiptGraph, bool) {
	if !topology.valid() {
		return nil, false
	}
	relation, ok := topology.topology.InitialRelation()
	if !ok {
		return nil, false
	}
	return topology.Graph(relation)
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
