package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

// TestReceiptCompilerThreadsExactAndSummaryReadThroughProductEvidencePatch
// exercises the combined receipt lane used by a closed heterogeneous rule.
// The graph member consumes two forms of the same sealed Factor: an exact
// predecessor and a summary predecessor.  Both are issued once by the Schema
// binding and then reach Product, evidence, and the ordinary carry/write patch.
func TestReceiptCompilerThreadsExactAndSummaryReadThroughProductEvidencePatch(t *testing.T) {
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(948_001))
	exactForm, exactFormOK := factor.ExactRead()
	summaryForm, summaryFormOK := factor.SummaryRead(coldKey(948_002))
	writeForm, writeFormOK := factor.ExactWrite()
	source, sourceOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(948_003), OperandFamily: unitOperandFamily, Inputs: 0,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(948_004)}, Output: factor.Ref(),
	})
	sourceWrite, sourceWriteOK := SchemaWrite(source, writeForm)
	reader, readerOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(948_005), OperandFamily: unitOperandFamily, Inputs: 1,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisDerivation, Identity: coldKey(948_006)}, Output: factor.Ref(),
	})
	input, inputOK := reader.Input(0)
	readerExact, readerExactOK := SchemaRead(reader, exactForm, input)
	readerSummary, readerSummaryOK := SchemaRead(reader, summaryForm, input)
	readerCarry, readerCarryOK := SchemaCarryFrom(reader, input, factor.Ref())
	readerWrite, readerWriteOK := SchemaWrite(reader, writeForm)
	schema, schemaOK := builder.Seal()
	if !factorOK || !exactFormOK || !summaryFormOK || !writeFormOK || !sourceOK || !sourceWriteOK || !readerOK || !inputOK || !readerExactOK || !readerSummaryOK || !readerCarryOK || !readerWriteOK || !schemaOK {
		t.Fatal("exact-summary receipt schema")
	}

	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) || !BindIdentitySummaryReadForFactor[uint64, uint64](binding, factor, summaryForm) {
		t.Fatal("exact-summary Factor receipt")
	}

	sourceOperand := ruleUnitForSemantic(coldKey(948_007))
	readerOperand := ruleUnitForSemantic(coldKey(948_008))
	sourceHot := HotRuleSpec[uint64, ruleUnit]{
		OperandContent: ruleUnitContent,
		Admission:      AdmitRuleByTrustedTheorem[uint64, ruleUnit](coldKey(948_004)),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(7)) })
		},
	}
	if !BindRule[uint64, uint64, ruleUnit](binding, source, sourceWrite, factor, sourceHot, testRuleProjector[ruleUnit]) {
		t.Fatal("exact-summary source receipt")
	}
	var exactRuntime Read[OrderedCells[uint64]]
	var summaryRuntime Read[OrderedCells[uint64]]
	var summaryRefs *ClosedRefs[uint64]
	readerHot := HotRuleSpec[uint64, ruleUnit]{
		OperandContent: ruleUnitContent,
		Admission: AdmitRuleByDerivation(coldKey(948_006), func(derivation RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) {
			disposition, dispositionOK := derivation.DispositionAt(0)
			exactCells, exactOK := DerivationDispositionReadValue(derivation, disposition, exactRuntime)
			summaryCells, summaryOK := DerivationDispositionReadValue(derivation, disposition, summaryRuntime)
			exact, exactPresent, exactAvailable := exactCells.At(0)
			summary, summaryPresent, summaryAvailable := summaryCells.At(0)
			if !dispositionOK || !exactOK || !summaryOK || exactCells.Count() != 1 || summaryCells.Count() != 1 || !exactAvailable || !summaryAvailable || !exactPresent || !summaryPresent || exact != 7 || summary != 7 || !DerivationReadMatchesSummaryRefs(derivation, summaryRuntime, summaryRefs) {
				return RuleEvidence{}, false
			}
			return derivation.Accept()
		}),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool {
				exactCells, exactOK := ReadValue(access, row, exactRuntime)
				summaryCells, summaryOK := ReadValue(access, row, summaryRuntime)
				exact, exactPresent, exactAvailable := exactCells.At(0)
				summary, summaryPresent, summaryAvailable := summaryCells.At(0)
				return exactOK && summaryOK && exactAvailable && summaryAvailable && exactPresent && summaryPresent && exact == 7 && summary == 7 && StageValue(access, row, exact+1)
			})
		},
	}
	var bindExact Read[OrderedCells[uint64]]
	var bindSummary Read[OrderedCells[uint64]]
	bindExact, bindSummary, bindOK := BindRuleWithExactAndSummaryReadAndCarry[uint64, uint64, ruleUnit, uint64, uint64, uint64, uint64, OrderedCells[uint64]](binding, reader, readerExact, factor, readerSummary, factor, summaryForm, readerCarry, readerWrite, factor, readerHot, HotCarrySpec[uint64, ruleUnit]{}, func(ruleUnit) (uint64, bool) { return 1, true }, func(ruleUnit) (uint64, bool) { return 2, true })
	exactRuntime, summaryRuntime = bindExact, bindSummary
	if !bindOK || !binding.Seal() {
		t.Fatal("exact-summary reader receipt")
	}
	factorImplementation, factorImplementationOK := FactorImplementationAt[uint64, uint64](binding, factor)
	ref, refOK := factorImplementation.Ref(0)
	refs := factorImplementation.NewClosedRefs()
	if !factorImplementationOK || !refOK || refs == nil || !refs.Append(ref) || !refs.Close() {
		t.Fatal("exact-summary refs")
	}
	summaryRefs = refs
	sourceImplementation, sourceImplementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, source)
	readerImplementation, readerImplementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, reader)
	if !sourceImplementationOK || !readerImplementationOK {
		t.Fatal("exact-summary Rule implementations")
	}

	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	sourceSite, sourceSiteOK := batch.AdmitSite(compositionKeyOf(coldKey(948_009)), scope, equation.TrueExpr(), equation.InitPresent)
	readerSite, readerSiteOK := batch.AdmitSite(compositionKeyOf(coldKey(948_010)), scope, equation.TrueExpr(), equation.InitPresent)
	sourceOccurrence, sourceOccurrenceOK := batch.At(sourceSite)
	readerOccurrence, readerOccurrenceOK := batch.At(readerSite)
	sourceEntity, sourceEntityOK := operandEntityForContent(sourceOperand.content)
	readerEntity, readerEntityOK := operandEntityForContent(readerOperand.content)
	sourceOperandRow, sourceOperandOK := batch.AdmitOperand(sourceOccurrence, sourceEntity)
	readerOperandRow, readerOperandOK := batch.AdmitOperand(readerOccurrence, readerEntity)
	if !sourceSiteOK || !readerSiteOK || !sourceOccurrenceOK || !readerOccurrenceOK || !sourceEntityOK || !readerEntityOK || !sourceOperandOK || !readerOperandOK || !batch.Seal() {
		t.Fatal("exact-summary batch")
	}
	boundary := equation.BoundaryInput(sourceSite, readerSite, compositionKeyOf(coldKey(948_011)), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	if !boundary.Available() {
		t.Fatal("exact-summary boundary")
	}
	exactSurface := equation.Surface{Factor: factorImplementation.binding.semantic, Form: equation.SurfaceReadExact, Local: 1}
	summarySurface := equation.Surface{Factor: factorImplementation.binding.semantic, Form: equation.SurfaceReadSummary, Local: 1, Semantic: compositionKeyOf(coldKey(948_002)), Normalizer: compositionKeyOf(coldKey(948_002))}
	topology, topologyOK := equation.SealTopology(schema.cold, equation.TopologySpec{
		Batch: batch,
		Rules: []equation.RuleInstance{
			{Schema: sourceImplementation.binding.proof.semantic, OperandFamily: compositionKeyOf(unitOperandFamily), Occurrence: sourceOccurrence, Operand: sourceOperandRow, Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: factorImplementation.binding.semantic, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}}},
			{Schema: readerImplementation.binding.proof.semantic, OperandFamily: compositionKeyOf(unitOperandFamily), Occurrence: readerOccurrence, Operand: readerOperandRow, Reads: []equation.ResolvedRead{{Index: 0, Surface: exactSurface}, {Index: 1, Surface: summarySurface}}, Carries: []equation.ResolvedCarry{{Index: 0}}, Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: factorImplementation.binding.semantic, Form: equation.SurfaceWriteExact, Local: 2, Mode: equation.TargetModeStrong}}}},
		},
		Points:    []equation.PointSpec{{Site: sourceSite}, {Site: readerSite}},
		Groups:    []equation.Group{{Members: []equation.RuleRef{equation.RuleAt(0)}, Output: equation.PointAt(0)}, {Members: []equation.RuleRef{equation.RuleAt(1)}, Output: equation.PointAt(1), Inputs: []equation.Input{boundary}}},
		Summaries: []equation.SummaryMapping{{Surface: summarySurface, Keys: []uint64{0}}},
	})
	if !topologyOK || topology == nil {
		t.Fatal("exact-summary topology")
	}
	graph, graphOK := initialEquationGraph(topology)
	if !graphOK || graph == nil {
		t.Fatal("exact-summary graph")
	}
	var sourceMember, readerMember equation.RuleMember
	for groupIndex := 0; groupIndex < graph.GroupCount(); groupIndex++ {
		group, ok := graph.HyperedgeAt(groupIndex)
		if !ok {
			t.Fatal("exact-summary group")
		}
		for memberIndex := 0; memberIndex < group.MemberCount(); memberIndex++ {
			member, ok := group.MemberAt(memberIndex)
			if !ok {
				t.Fatal("exact-summary member")
			}
			switch member.Rule() {
			case sourceImplementation.binding.proof.semantic:
				sourceMember = member
			case readerImplementation.binding.proof.semantic:
				readerMember = member
			}
		}
	}
	compilation, compiled := beginProgramConstruction(binding, graph)
	sourceRow, sourceRowOK := attachProgramRuleMember(compilation, sourceImplementation, sourceMember, sourceOperand)
	readerRow, readerRowOK := attachProgramRuleMember(compilation, readerImplementation, readerMember, readerOperand)
	sourceSlot, sourceSlotOK := sourceRow.outputSlot()
	readerSlot, readerSlotOK := readerRow.outputSlot()
	sourcePlan, sourcePlanOK := compilation.carrier.SealContribution(0, []shape.Slot{sourceSlot}, nil, false)
	readerPlan, readerPlanOK := compilation.carrier.SealContribution(1, []shape.Slot{readerSlot}, []carrier.ContributionSource{{Slot: readerSlot, Input: 0}}, false)
	work, workOK := compilation.carrier.NewWork()
	whole, wholeOK := support.True(compilation.runtime.guards)
	sourceBase, sourceBaseOK := work.BeginRuleContribution(sourcePlan, compilation.carrier.Scope(), nil, whole)
	sourceResult := sourceRow.execute(work, sourceBase, nil, whole)
	sourceContribution, sourceFinished := work.FinishRuleContribution(sourceBase, []carrier.Patch{sourceResult.patch})
	sourcePoint, sourcePointOK := work.PointStateFromRuleContribution(sourceContribution)
	readerBase, readerBaseOK := work.BeginRuleContribution(readerPlan, compilation.carrier.Scope(), []carrier.PointState{sourcePoint}, whole)
	readerResult := readerRow.execute(work, readerBase, []carrier.State{sourcePoint.State()}, whole)
	readerContribution, readerFinished := work.FinishRuleContribution(readerBase, []carrier.Patch{readerResult.patch})
	if !compiled || !sourceRowOK || !readerRowOK || !sourceSlotOK || !readerSlotOK || !sourcePlanOK || !readerPlanOK || !workOK || !wholeOK || !sourceBaseOK || !sourceResult.valid || !sourceResult.wrote || !sourceFinished || !sourceContribution.Valid() || !sourcePointOK || !readerBaseOK || !readerResult.valid || !readerResult.wrote || !readerFinished || !readerContribution.Valid() {
		t.Fatal("exact-summary Product/evidence/publication")
	}
	if exactRuntime.origin == nil || exactRuntime.origin.kind != composition.ReadExact || summaryRuntime.origin == nil || summaryRuntime.origin.kind != composition.ReadSummary || summaryRuntime.origin.semantic != compositionKeyOf(coldKey(948_002)) {
		t.Fatal("exact-summary read origin fence")
	}

	foreign := NewSchemaBinding(schema)
	foreignFactorOK := BindFactor(foreign, factor, hotUintFactorSpec())
	foreignSummaryOK := BindIdentitySummaryReadForFactor[uint64, uint64](foreign, factor, summaryForm)
	foreignSourceOK := BindRule[uint64, uint64, ruleUnit](foreign, source, sourceWrite, factor, sourceHot, testRuleProjector[ruleUnit])
	_, _, foreignReaderOK := BindRuleWithExactAndSummaryReadAndCarry[uint64, uint64, ruleUnit, uint64, uint64, uint64, uint64, OrderedCells[uint64]](foreign, reader, readerExact, factor, readerSummary, factor, summaryForm, readerCarry, readerWrite, factor, readerHot, HotCarrySpec[uint64, ruleUnit]{}, func(ruleUnit) (uint64, bool) { return 1, true }, func(ruleUnit) (uint64, bool) { return 2, true })
	if !foreignFactorOK || !foreignSummaryOK || !foreignSourceOK || !foreignReaderOK || !foreign.Seal() {
		t.Fatal("foreign exact-summary binding")
	}
	foreignReader, foreignReaderOK := RuleImplementationAt[uint64, uint64, ruleUnit](foreign, reader)
	if !foreignReaderOK || foreignReader == nil {
		t.Fatal("foreign exact-summary implementation")
	}
	if _, accepted := attachProgramRuleMember(compilation, foreignReader, readerMember, readerOperand); accepted {
		t.Fatal("equal-Schema foreign exact-summary member crossed authority")
	}
	if _, duplicate := attachProgramRuleMember(compilation, readerImplementation, readerMember, readerOperand); duplicate {
		t.Fatal("duplicate exact-summary member admitted")
	}
}
