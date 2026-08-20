package equation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
)

// activationRowFixture supplies a formal binding and an ordinary base Batch
// for semantic topology laws.  The fixture deliberately stops at the
// ActivationRowSpec boundary; no materialization or assembly receipt is built
// in the tests that consume it.
type activationRowFixture struct {
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

func newActivationRowFixture(t testing.TB) activationRowFixture {
	return newActivationRowFixtureWithOptions(t, false, false)
}

func newActivationRowFixtureWithGrammar(t testing.TB, withMetadata, withGrammar bool) activationRowFixture {
	return newActivationRowFixtureWithOptions(t, withMetadata, withGrammar)
}

func newActivationRowFixtureWithOptions(t testing.TB, withMetadata, withGrammar bool) activationRowFixture {
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
			OutputKind: composition.StructuralOutput, Inputs: 1,
			Reads:    []composition.Read{{Kind: composition.ReadExact, Input: 0, Factor: factor}},
			Supports: []composition.Support{{Semantic: boundaryKey(229)}},
			Prunes:   []composition.Prune{{Semantic: boundaryKey(230)}},
		}, {
			Key: boundaryKey(231), OperandFamily: boundaryKey(232),
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
	input, inputOK := formals.AdmitFormalPort(boundaryKey(203), PortImport, formalReads)
	local, localOK := formals.AdmitSite(boundaryKey(205), localScope, localInit, InitPresent)
	output, outputOK := formals.AdmitFormalPort(boundaryKey(206), PortExport, nil)
	formalSummary := SummaryMapping{Surface: Surface{Factor: factor, Form: SurfaceReadSummary, Local: 1, Semantic: boundaryKey(218), Normalizer: boundaryKey(218)}, Keys: []uint64{1, 3}}
	formalWeak := WeakTargetMapping{Surface: Surface{Factor: factor, Form: SurfaceWriteExact, Local: 2, Mode: TargetModeWeak}, Candidates: []Surface{{Factor: factor, Form: SurfaceReadExact, Local: 1}}}
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
	binding, bindingOK := SealTemplateBinding(formals, actuals, []FormalPortActual{
		{Role: input, Site: actualInput, Reads: actualReads},
		{Role: output, Site: actualOutput},
	})
	if !bindingOK {
		t.Fatal("TemplateBinding")
	}
	return activationRowFixture{
		source: source, query: query, formals: formals, input: input, output: output, local: local, inputs: inputs,
		binding: binding, actuals: actuals, actualInput: actualInput, actualOutput: actualOutput,
	}
}

func activationRowSpec(fixture activationRowFixture, family, application, target, endpoint composition.Key) ActivationRowSpec {
	return ActivationRowSpec{
		TriggerOrdinal: 0,
		Family:         family,
		Application:    application,
		Target:         target,
		Endpoint:       endpoint,
		Binding:        fixture.binding,
		Sites:          []Site{fixture.input.Site(), fixture.local, fixture.output.Site()},
		Inputs:         append([]Input(nil), fixture.inputs...),
	}
}
