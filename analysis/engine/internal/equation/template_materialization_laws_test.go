package equation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
)

type templateMaterializationFixture struct {
	source                    *composition.Composition
	query                     composition.Key
	formals                   *Batch
	input, output             FormalPort
	local                     Site
	inputs                    []Input
	binding                   TemplateBinding
	actuals                   *Batch
	actualInput, actualOutput Site
}

func newTemplateMaterializationFixture(t testing.TB) templateMaterializationFixture {
	return newTemplateMaterializationFixtureWithMetadata(t, false)
}

func newTemplateMaterializationFixtureWithMetadata(t testing.TB, withMetadata bool) templateMaterializationFixture {
	return newTemplateMaterializationFixtureWithOptions(t, withMetadata, false, false)
}

func newTemplateMaterializationFixtureWithGrammar(t testing.TB, withMetadata, withGrammar bool) templateMaterializationFixture {
	return newTemplateMaterializationFixtureWithOptions(t, withMetadata, withGrammar, false)
}

func newTemplateMaterializationFixtureWithOptions(t testing.TB, withMetadata, withGrammar, collapsedWeakCandidate bool) templateMaterializationFixture {
	t.Helper()
	factor := boundaryKey(201)
	query := boundaryKey(219)
	candidate := composition.Candidate{Factors: []composition.Factor{{Key: factor}}}
	if withMetadata {
		candidate.Factors[0].Forms = []composition.FactorForm{{Kind: composition.FactorSummaryRead, Semantic: boundaryKey(218)}}
		candidate.Queries = []composition.QueryFamily{{Key: query, Freezer: boundaryKey(220), Projections: []composition.QueryProjection{{Kind: composition.QueryFactorSummary, Factor: factor, Normalizer: boundaryKey(218)}}}}
	}
	if withGrammar {
		candidate.Completion = composition.Completion{Semantic: boundaryKey(229), Prune: boundaryKey(230)}
		candidate.Rules = []composition.Rule{{
			Key: boundaryKey(224), OperandFamily: boundaryKey(225),
			Admission:  composition.Admission{Kind: composition.AdmissionTrustedTheorem, Identity: boundaryKey(227)},
			OutputKind: composition.StructuralOutput, Inputs: 1,
			Reads:    []composition.Read{{Kind: composition.ReadExact, Input: 0, Factor: factor}},
			Supports: []composition.Support{{Semantic: boundaryKey(229)}},
			Prunes:   []composition.Prune{{Semantic: boundaryKey(230)}},
		}, {
			Key: boundaryKey(231), OperandFamily: boundaryKey(232),
			Admission:  composition.Admission{Kind: composition.AdmissionTrustedTheorem, Identity: boundaryKey(233)},
			OutputKind: composition.FactorOutput, Output: factor, Inputs: 1,
			Reads:  []composition.Read{{Kind: composition.ReadExact, Input: 0, Factor: factor}},
			Writes: []composition.Write{{Kind: composition.WriteExact, Factor: factor}},
		}}
	}
	source, sourceOK := composition.Seal(candidate)
	if !sourceOK {
		t.Fatal("cold Composition")
	}
	localDecision := boundaryDecision(t, 202)
	localScope, localScopeOK := NewScope(localDecision)
	localInit, localInitOK := DecisionExpr(localDecision)
	if !localScopeOK || !localInitOK {
		t.Fatal("local formal scope")
	}
	formals := NewBatch()
	formalReads := []PortRead{{Role: boundaryKey(204), Surface: Surface{Factor: factor, Form: SurfaceReadExact, Local: 1}}}
	if collapsedWeakCandidate {
		formalReads = append(formalReads, PortRead{Role: boundaryKey(205), Surface: Surface{Factor: factor, Form: SurfaceReadExact, Local: 2}})
	}
	input, inputOK := formals.AdmitFormalPort(boundaryKey(203), PortImport, formalReads)
	local, localOK := formals.AdmitSite(boundaryKey(205), localScope, localInit, InitPresent)
	output, outputOK := formals.AdmitFormalPort(boundaryKey(206), PortExport, nil)
	formalSummary := SummaryMapping{Surface: Surface{Factor: factor, Form: SurfaceReadSummary, Local: 1, Semantic: boundaryKey(218), Normalizer: boundaryKey(218)}, Keys: []uint64{1, 3}}
	formalWeak := WeakTargetMapping{Surface: Surface{Factor: factor, Form: SurfaceWriteExact, Local: 2, Mode: TargetModeWeak}, Candidates: []Surface{{Factor: factor, Form: SurfaceReadExact, Local: 1}}}
	if collapsedWeakCandidate {
		formalWeak.Candidates = append(formalWeak.Candidates, Surface{Factor: factor, Form: SurfaceReadExact, Local: 2})
	}
	metadataSummaryOK, metadataWeakOK := true, true
	if withMetadata {
		metadataSummaryOK = formals.AdmitSummary(formalSummary)
		metadataWeakOK = formals.AdmitWeakTarget(formalWeak)
	}
	if !inputOK || !localOK || !outputOK || !metadataSummaryOK || !metadataWeakOK {
		t.Fatal("formal Batch")
	}
	intoLocal, intoLocalOK := NewReindex(EmptyScope(), localScope, nil)
	fromLocal, fromLocalOK := NewReindex(localScope, EmptyScope(), []DecisionMap{Forget(localDecision)})
	if !intoLocalOK || !fromLocalOK {
		t.Fatal("formal reindexes")
	}
	if withGrammar {
		_, inputPointOK := formals.AdmitPoint(input.Site())
		_, localPointOK := formals.AdmitPoint(local)
		outputPoint, outputPointOK := formals.AdmitPoint(output.Site())
		if !inputPointOK || !localPointOK || !outputPointOK {
			t.Fatal("formal target Points")
		}
		occurrence, occurrenceOK := formals.At(local)
		operand, operandOK := formals.AdmitOperand(occurrence, boundaryKey(223))
		if !occurrenceOK || !operandOK || !formals.AdmitRule(RuleInstance{
			Schema: boundaryKey(224), OperandFamily: boundaryKey(225), Occurrence: occurrence, Operand: operand,
			Reads:    []ResolvedRead{{Index: 0, Surface: Surface{Factor: factor, Form: SurfaceReadExact, Local: 1}}},
			Supports: []ResolvedSupport{{Index: 0, Surface: StructuralSurface{Local: 1, Semantic: boundaryKey(229)}}},
			Prunes:   []ResolvedPrune{{Index: 0, Surface: StructuralSurface{Local: 1, Semantic: boundaryKey(230)}}},
		}) {
			t.Fatal("formal target Rule")
		}
		factorOccurrence, factorOccurrenceOK := formals.At(output.Site())
		factorOperand, factorOperandOK := formals.AdmitOperand(factorOccurrence, boundaryKey(234))
		if !factorOccurrenceOK || !factorOperandOK || !formals.AdmitRule(RuleInstance{
			Schema: boundaryKey(231), OperandFamily: boundaryKey(232), Occurrence: factorOccurrence, Operand: factorOperand,
			Reads:  []ResolvedRead{{Index: 0, Surface: Surface{Factor: factor, Form: SurfaceReadExact, Local: 1}}},
			Writes: []ResolvedWrite{{Index: 0, Surface: Surface{Factor: factor, Form: SurfaceWriteExact, Local: 2, Mode: TargetModeWeak}}},
		}) {
			t.Fatal("formal factor Rule")
		}
		boundaryReindex, reindexOK := NewReindex(EmptyScope(), EmptyScope(), nil)
		if !reindexOK {
			t.Fatal("formal target Reindex")
		}
		targetInput := TargetBoundaryInput(input.Site(), output.Site(), boundaryKey(226), TrueExpr(), boundaryReindex, TrueExpr())
		if !formals.AdmitInput(targetInput) || !formals.AdmitGroup(BatchGroup{
			Members: []RuleRef{RuleAt(0), RuleAt(1)}, Output: outputPoint,
			Inputs: []BatchInput{targetInput},
		}) || !formals.AdmitFactorEdge(BatchFactorEdge{Target: outputPoint, Input: targetInput, Factor: factor}) ||
			!formals.AdmitEnvironmentEdge(BatchEnvironmentEdge{Target: outputPoint, Input: targetInput}) {
			t.Fatal("formal target grammar")
		}
	}
	if !formals.Seal() {
		t.Fatal("formal Batch seal")
	}
	inputs := []Input{
		BoundaryInput(input.Site(), local, boundaryKey(207), TrueExpr(), intoLocal, localInit),
		BoundaryInput(local, output.Site(), boundaryKey(208), localInit, fromLocal, TrueExpr()),
	}
	if !inputs[0].Available() || !inputs[1].Available() {
		t.Fatal("formal Inputs")
	}
	outer := boundaryDecision(t, 209)
	ambient, ambientOK := NewScope(outer)
	if !ambientOK {
		t.Fatal("actual ambient")
	}
	actuals := NewBatch()
	actualInput, actualInputOK := actuals.AdmitSite(boundaryKey(210), ambient, FalseExpr(), InitAbsent)
	actualOutput, actualOutputOK := actuals.AdmitSite(boundaryKey(211), ambient, FalseExpr(), InitAbsent)
	if !actualInputOK || !actualOutputOK || !actuals.Seal() {
		t.Fatal("actual Batch")
	}
	actualReads := []PortRead{{Role: boundaryKey(204), Surface: Surface{Factor: factor, Form: SurfaceReadExact, Local: 101}}}
	if collapsedWeakCandidate {
		actualReads = append(actualReads, PortRead{Role: boundaryKey(205), Surface: Surface{Factor: factor, Form: SurfaceReadExact, Local: 101}})
	}
	binding, bindingOK := SealTemplateBinding(formals, actuals, []FormalPortActual{
		{Role: input, Site: actualInput, Reads: actualReads},
		{Role: output, Site: actualOutput},
	})
	if !bindingOK {
		t.Fatal("TemplateBinding")
	}
	return templateMaterializationFixture{
		source: source, query: query, formals: formals, input: input, output: output, local: local, inputs: inputs,
		binding: binding, actuals: actuals, actualInput: actualInput, actualOutput: actualOutput,
	}
}

func TestTemplateMaterializationReissuesOneOrdinaryTargetBatch(t *testing.T) {
	fixture := newTemplateMaterializationFixture(t)
	materialized, ok := MaterializeTemplateBoundary(
		fixture.source,
		fixture.binding,
		[]Site{fixture.local, fixture.output.Site(), fixture.input.Site()},
		fixture.inputs,
	)
	if !ok || !materialized.Available() || materialized.Batch() == nil || !materialized.Batch().Sealed() || materialized.InputCount() != 2 {
		t.Fatal("target materialization")
	}
	inputSite, inputOK := materialized.Site(fixture.input.Site())
	localSite, localOK := materialized.Site(fixture.local)
	outputSite, outputOK := materialized.Site(fixture.output.Site())
	if !inputOK || !localOK || !outputOK || !materialized.Batch().OwnsSite(inputSite) || !materialized.Batch().OwnsSite(localSite) || !materialized.Batch().OwnsSite(outputSite) {
		t.Fatal("target Site ownership")
	}
	if inputSite.Same(fixture.actualInput) || outputSite.Same(fixture.actualOutput) || inputSite.Key() != fixture.actualInput.Key() || outputSite.Key() != fixture.actualOutput.Key() {
		t.Fatal("concrete Sites were retained instead of target-reissued")
	}
	if localSite.Scope().Count() != 2 || fixture.local.Scope().Count() != 1 || fixture.input.Site().Scope().Count() != 0 {
		t.Fatal("local alpha/ambient scope")
	}
	localInit, disposition, initialized := localSite.Init()
	if !initialized || disposition != InitPresent || len(localInit.Decisions()) != 1 || fixture.formals.OwnsSite(localSite) {
		t.Fatal("local target initialization")
	}
	first, firstOK := materialized.InputAt(0)
	second, secondOK := materialized.InputAt(1)
	if !firstOK || !secondOK || !first.Source().Same(inputSite) || !first.Target().Same(localSite) || !second.Source().Same(localSite) || !second.Target().Same(outputSite) {
		t.Fatal("ordinary target Inputs")
	}
	if first.Reindex().Count() != 1 || second.Reindex().Count() != 2 {
		t.Fatal("ambient/local relation was not lowered by the canonical binder")
	}
	forgotten := 0
	for index := 0; index < second.Reindex().Count(); index++ {
		mapping, present := second.Reindex().At(index)
		if !present {
			t.Fatal("materialized Reindex row")
		}
		if mapping.Disposition == DecisionForget {
			forgotten++
		}
	}
	if forgotten != 1 {
		t.Fatal("local decision was not forgotten at the export")
	}
	resolved, reads, resolvedOK := materialized.ResolveImport(fixture.input)
	if !resolvedOK || !resolved.Same(inputSite) || len(reads) != 1 || reads[0].Surface.Local != 101 {
		t.Fatal("authenticated read-slot resolution")
	}
	if resolved, resolvedOK := materialized.ResolveExport(fixture.output); !resolvedOK || !resolved.Same(outputSite) {
		t.Fatal("authenticated export resolution")
	}
	if validTopologyBatch(materialized.Batch(), TopologySpec{Batch: materialized.Batch(), Points: []PointSpec{{Site: inputSite}, {Site: localSite}, {Site: outputSite}}}) == false {
		t.Fatal("materialized Sites are not ordinary topology Sites")
	}
}

func TestTemplateMaterializationPreservesFormalMetadata(t *testing.T) {
	fixture := newTemplateMaterializationFixtureWithMetadata(t, true)
	materialized, ok := MaterializeTemplateBoundary(fixture.source, fixture.binding,
		[]Site{fixture.input.Site(), fixture.local, fixture.output.Site()}, fixture.inputs)
	if !ok {
		t.Fatal("materialization")
	}
	spec, projected := MaterializeTargetBatch(materialized.Batch())
	if !projected || len(spec.Summaries) != 1 || len(spec.WeakTargets) != 1 {
		t.Fatal("formal metadata was dropped")
	}
	if spec.Summaries[0].Surface.Form != SurfaceReadSummary || spec.Summaries[0].Keys[0] != 1 ||
		spec.WeakTargets[0].Surface.Mode != TargetModeWeak || len(spec.WeakTargets[0].Candidates) != 1 ||
		spec.WeakTargets[0].Candidates[0].Factor != boundaryKey(201) || spec.WeakTargets[0].Candidates[0].Local != 101 {
		t.Fatal("formal metadata changed during materialization")
	}
}

func TestTemplateMaterializationCompleteGrammarParityAndDuplicateFence(t *testing.T) {
	fixture := newTemplateMaterializationFixtureWithGrammar(t, true, true)
	sites := []Site{fixture.input.Site(), fixture.local, fixture.output.Site()}
	materialized, ok := MaterializeTemplateBoundary(fixture.source, fixture.binding, sites, fixture.inputs)
	if !ok {
		t.Fatal("complete grammar materialization")
	}
	spec, projected := MaterializeTargetBatch(materialized.Batch())
	if !projected || len(spec.Points) != 3 || len(spec.Rules) != 2 || len(spec.Groups) != 1 || len(spec.FactorEdges) != 1 || len(spec.EnvironmentEdges) != 1 || len(spec.Summaries) != 1 || len(spec.WeakTargets) != 1 {
		t.Fatal("complete grammar rows were not preserved")
	}
	if spec.Groups[0].Output != PointAt(2) || spec.Groups[0].Output == PointAt(0) || len(spec.Groups[0].Inputs) != 1 {
		t.Fatal("non-self group output was rewritten to its input")
	}
	groupInput := spec.Groups[0].Inputs[0]
	if !groupInput.Source().Available() || !groupInput.Target().Available() || groupInput.Source().Key() == groupInput.Target().Key() ||
		groupInput.Source().batch != materialized.Batch() || groupInput.Target().batch != materialized.Batch() {
		t.Fatal("group boundary was not reissued into target ownership")
	}
	assembly, assembled := SealTopologyAssembly(fixture.actuals, []TemplateMaterialization{materialized})
	if !assembled || !assembly.Available() || len(assembly.Targets()) != 1 {
		t.Fatal("complete grammar assembly")
	}
	assembledTarget := assembly.Targets()[0]
	if len(assembledTarget.Groups) != 1 || assembledTarget.Groups[0].Output != PointAt(2) || len(assembledTarget.Summaries) != 1 || len(assembledTarget.WeakTargets) != 1 {
		t.Fatal("assembly changed the formal target catalog")
	}
	topology, sealed := SealTopology(fixture.source, TopologySpec{
		Batch: fixture.actuals, Materializations: []TemplateMaterialization{materialized},
		Queries: []QueryInstance{{Family: fixture.query, Point: PointAt(0), Surfaces: []Surface{{Factor: boundaryKey(201), Form: SurfaceReadSummary, Local: 1, Semantic: boundaryKey(218), Normalizer: boundaryKey(218)}}}},
	})
	if !sealed || topology == nil {
		t.Fatal("complete grammar did not reach final topology/buildInstances")
	}
	duplicate, duplicateOK := MaterializeTemplateBoundary(fixture.source, fixture.binding, sites, fixture.inputs)
	if !duplicateOK || duplicate.Key() != materialized.Key() || duplicate.Batch() == materialized.Batch() {
		t.Fatal("replay did not preserve canonical receipt identity")
	}
	if _, accepted := SealTopologyAssembly(fixture.actuals, []TemplateMaterialization{materialized, duplicate}); accepted {
		t.Fatal("identical metadata mappings were admitted as duplicate materializations")
	}
	foreign := newTemplateMaterializationFixtureWithGrammar(t, true, true)
	foreignSites := []Site{fixture.input.Site(), fixture.local, foreign.output.Site()}
	if _, accepted := MaterializeTemplateBoundary(fixture.source, fixture.binding, foreignSites, fixture.inputs); accepted {
		t.Fatal("foreign formal endpoint crossed the Batch fence")
	}
	collapsed := newTemplateMaterializationFixtureWithOptions(t, true, true, true)
	if _, accepted := MaterializeTemplateBoundary(collapsed.source, collapsed.binding, []Site{collapsed.input.Site(), collapsed.local, collapsed.output.Site()}, collapsed.inputs); accepted {
		t.Fatal("collapsed weak candidates were silently coalesced")
	}
	swapped := BoundaryInput(fixture.output.Site(), fixture.local, boundaryKey(228), TrueExpr(), func() Reindex {
		value, valid := NewReindex(EmptyScope(), fixture.local.Scope(), nil)
		if !valid {
			t.Fatal("swapped endpoint Reindex")
		}
		return value
	}(), TrueExpr())
	if _, accepted := MaterializeTemplateBoundary(fixture.source, fixture.binding, sites, []Input{swapped, fixture.inputs[1]}); accepted {
		t.Fatal("swapped import/export endpoint was accepted")
	}
}

func TestTemplateMaterializationIsCanonicalAndOwnerFenced(t *testing.T) {
	fixture := newTemplateMaterializationFixture(t)
	left, leftOK := MaterializeTemplateBoundary(fixture.source, fixture.binding,
		[]Site{fixture.input.Site(), fixture.local, fixture.output.Site()}, fixture.inputs)
	right, rightOK := MaterializeTemplateBoundary(fixture.source, fixture.binding,
		[]Site{fixture.output.Site(), fixture.input.Site(), fixture.local}, []Input{fixture.inputs[1], fixture.inputs[0]})
	if !leftOK || !rightOK || left.Key() != right.Key() || left.Same(right) {
		t.Fatal("canonical replay identity/exact authority")
	}
	leftLocal, _ := left.Site(fixture.local)
	rightLocal, _ := right.Site(fixture.local)
	if leftLocal.Key() != rightLocal.Key() || leftLocal.Same(rightLocal) {
		t.Fatal("materialized Site replay")
	}
	foreign := newTemplateMaterializationFixture(t)
	if _, accepted := left.Site(foreign.local); accepted {
		t.Fatal("foreign equal formal Site crossed materialization owner")
	}
	if _, accepted := MaterializeTemplateBoundary(fixture.source, fixture.binding,
		[]Site{fixture.input.Site(), fixture.local}, fixture.inputs); accepted {
		t.Fatal("partial formal Site denominator accepted")
	}
}

func TestTemplateMaterializationRejectsColdFactorAndDirectionMismatch(t *testing.T) {
	fixture := newTemplateMaterializationFixture(t)
	foreignSource, sealed := composition.Seal(composition.Candidate{Factors: []composition.Factor{{Key: boundaryKey(212)}}})
	if !sealed {
		t.Fatal("foreign cold Composition")
	}
	allSites := []Site{fixture.input.Site(), fixture.local, fixture.output.Site()}
	if _, accepted := MaterializeTemplateBoundary(foreignSource, fixture.binding, allSites, fixture.inputs); accepted {
		t.Fatal("formal read Factor absent from exact cold Composition")
	}
	wrongDirection, ok := NewReindex(EmptyScope(), fixture.local.Scope(), nil)
	if !ok {
		t.Fatal("wrong-direction relation")
	}
	bad := BoundaryInput(fixture.output.Site(), fixture.local, boundaryKey(213), TrueExpr(), wrongDirection, TrueExpr())
	if !bad.Available() {
		t.Fatal("formal wrong-direction fixture")
	}
	if _, accepted := MaterializeTemplateBoundary(fixture.source, fixture.binding, allSites, []Input{bad}); accepted {
		t.Fatal("export-only formal port consumed as an import")
	}
}

func TestTemplateMaterializationRejectsDistinctConcreteWorlds(t *testing.T) {
	fixture := newTemplateMaterializationFixture(t)
	leftDecision := boundaryDecision(t, 214)
	rightDecision := boundaryDecision(t, 215)
	leftScope, leftOK := NewScope(leftDecision)
	rightScope, rightOK := NewScope(rightDecision)
	if !leftOK || !rightOK {
		t.Fatal("distinct actual scopes")
	}
	actuals := NewBatch()
	left, leftOK := actuals.AdmitSite(boundaryKey(216), leftScope, FalseExpr(), InitAbsent)
	right, rightOK := actuals.AdmitSite(boundaryKey(217), rightScope, FalseExpr(), InitAbsent)
	if !leftOK || !rightOK || !actuals.Seal() {
		t.Fatal("distinct-world actual Batch")
	}
	binding, bound := SealTemplateBinding(fixture.formals, actuals, []FormalPortActual{
		{Role: fixture.input, Site: left, Reads: []PortRead{{Role: boundaryKey(204), Surface: Surface{Factor: boundaryKey(201), Form: SurfaceReadExact, Local: 101}}}},
		{Role: fixture.output, Site: right},
	})
	if !bound {
		t.Fatal("structural binding should retain each exact directional transport")
	}
	if _, accepted := MaterializeTemplateBoundary(fixture.source, binding,
		[]Site{fixture.input.Site(), fixture.local, fixture.output.Site()}, fixture.inputs); accepted {
		t.Fatal("distinct concrete worlds were silently unioned")
	}
}
