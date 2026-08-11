package engine

import (
	"reflect"
	"testing"

	coldcomposition "github.com/wippyai/go-lua/analysis/engine/internal/composition"
)

func TestSemanticReportIsSealedCanonicalAndImmutable(t *testing.T) {
	open := NewComposition()
	if _, ok := open.SemanticReport(); ok {
		t.Fatal("unsealed Composition published a semantic report")
	}

	first := semanticReportFixture(t, false)
	report, reported := first.SemanticReport()
	if !reported {
		t.Fatal("sealed Composition has no semantic report")
	}
	factorA, factorB := coldKey(120_001), coldKey(120_002)
	want := CompositionReport{
		ID:                 first.ID(),
		Completion:         coldKey(120_003),
		CompletionPrune:    coldKey(120_004),
		ActivationFamilies: []SemanticKey{coldKey(120_005)},
		Rules: []RuleSchemaReport{
			{
				Semantic: coldKey(120_006), OperandFamily: unitOperandFamily,
				OutputDisposition: RuleOutputDispositionFactor, OutputFactor: factorA,
				Writes: []RuleWriteReport{{Kind: RuleWriteDispositionExact, Factor: factorA}},
			},
			{
				Semantic: coldKey(120_007), OperandFamily: unitOperandFamily,
				OutputDisposition: RuleOutputDispositionFactor, OutputFactor: factorB, Inputs: 1,
				Reads:   []RuleReadReport{{Kind: RuleReadDispositionExact, Factor: factorA}},
				Carries: []RuleCarryReport{{Factor: factorB}},
				Writes:  []RuleWriteReport{{Kind: RuleWriteDispositionExact, Factor: factorB}},
			},
			{
				Semantic: coldKey(120_008), OperandFamily: unitOperandFamily,
				OutputDisposition: RuleOutputDispositionStructural,
				Supports:          []SemanticKey{coldKey(120_003)}, Prunes: []SemanticKey{coldKey(120_004)},
			},
			{
				Semantic: coldKey(120_009), OperandFamily: unitOperandFamily,
				OutputDisposition: RuleOutputDispositionStructural,
				Activations:       []SemanticKey{coldKey(120_005)},
			},
		},
		Queries: []QuerySchemaReport{{
			Semantic: coldKey(120_010), Freezer: coldKey(120_110),
			Projections: []QueryProjectionReport{{Kind: QueryProjectionDispositionFactorExact, Factor: factorB}},
		}},
		Incidences: []FactorIncidence{
			{Read: factorA, Write: factorB},
			{Read: factorB, Write: factorB},
		},
		Components: []FactorComponent{
			{Factors: []SemanticKey{factorA}, Successors: []SemanticKey{factorB}},
			{Factors: []SemanticKey{factorB}, Successors: []SemanticKey{}},
		},
	}
	if !reflect.DeepEqual(report, want) {
		t.Fatalf("semantic report = %#v, want %#v", report, want)
	}

	second := semanticReportFixture(t, true)
	permuted, reported := second.SemanticReport()
	if !reported || first.ID() != second.ID() || !reflect.DeepEqual(permuted, report) {
		t.Fatal("semantic report or CompositionID changed under declaration permutation")
	}

	report.ActivationFamilies[0] = SemanticKey{}
	report.Rules[0] = RuleSchemaReport{}
	report.Rules[1].Reads[0].Factor = SemanticKey{}
	report.Rules[1].Carries[0].Factor = SemanticKey{}
	report.Rules[1].Writes[0].Factor = SemanticKey{}
	report.Rules[2].Supports[0] = SemanticKey{}
	report.Rules[2].Prunes[0] = SemanticKey{}
	report.Rules[3].Activations[0] = SemanticKey{}
	report.Queries[0].Semantic = SemanticKey{}
	report.Queries[0].Projections[0].Factor = SemanticKey{}
	report.Incidences[0] = FactorIncidence{}
	report.Components[0].Factors[0] = SemanticKey{}
	report.Components[0].Successors[0] = SemanticKey{}
	again, reported := first.SemanticReport()
	if !reported || !reflect.DeepEqual(again, want) {
		t.Fatal("caller mutation changed a later semantic report")
	}
}

func TestRuleSchemaReportPreservesEveryColdRuleFieldDetached(t *testing.T) {
	cold := func(value uint64) coldcomposition.Key { return coldKey(value).compositionKey() }
	factor, normalizer, selector, transform := cold(121_003), cold(121_004), cold(121_005), cold(121_006)
	rule := coldcomposition.Rule{
		Key: cold(121_001), OperandFamily: cold(121_002), OutputKind: coldcomposition.FactorOutput, Output: factor, Inputs: 2,
		Reads: []coldcomposition.Read{
			{Kind: coldcomposition.ReadExact, Input: 0, Factor: factor},
			{Kind: coldcomposition.ReadSummary, Input: 1, Factor: factor, Semantic: normalizer, Normalizer: normalizer},
			{Kind: coldcomposition.ReadSelect, Input: 1, Factor: factor, Semantic: factor, Dependencies: []uint64{0, 1}},
		},
		Carries: []coldcomposition.Carry{{Input: 1, Factor: factor, Transform: transform}},
		Writes: []coldcomposition.Write{
			{Kind: coldcomposition.WriteExact, Factor: factor},
			{Kind: coldcomposition.WriteSelect, Factor: factor, Semantic: selector, Candidates: []uint64{0, 2}, Dependencies: []coldcomposition.Dependency{{Index: 0}, {Target: true, Index: 0}}},
			{Kind: coldcomposition.WriteRoute, Factor: factor, Route: 3},
		},
		Supports:    []coldcomposition.Support{{Semantic: cold(121_007)}},
		Prunes:      []coldcomposition.Prune{{Semantic: cold(121_008)}},
		Activations: []coldcomposition.ActivationRange{{Family: cold(121_009)}},
	}
	want := RuleSchemaReport{
		Semantic: coldKey(121_001), OperandFamily: coldKey(121_002),
		OutputDisposition: RuleOutputDispositionFactor, OutputFactor: coldKey(121_003), Inputs: 2,
		Reads: []RuleReadReport{
			{Kind: RuleReadDispositionExact, Input: 0, Factor: coldKey(121_003)},
			{Kind: RuleReadDispositionSummary, Input: 1, Factor: coldKey(121_003), Semantic: coldKey(121_004), Normalizer: coldKey(121_004)},
			{Kind: RuleReadDispositionSelect, Input: 1, Factor: coldKey(121_003), Semantic: coldKey(121_003), Dependencies: []uint64{0, 1}},
		},
		Carries: []RuleCarryReport{{Input: 1, Factor: coldKey(121_003), Transform: coldKey(121_006)}},
		Writes: []RuleWriteReport{
			{Kind: RuleWriteDispositionExact, Factor: coldKey(121_003)},
			{Kind: RuleWriteDispositionSelect, Factor: coldKey(121_003), Semantic: coldKey(121_005), Candidates: []uint64{0, 2}, Dependencies: []RuleWriteDependencyReport{{Index: 0}, {Target: true, Index: 0}}},
			{Kind: RuleWriteDispositionRoute, Factor: coldKey(121_003), Route: 3},
		},
		Supports: []SemanticKey{coldKey(121_007)}, Prunes: []SemanticKey{coldKey(121_008)}, Activations: []SemanticKey{coldKey(121_009)},
	}
	report, reported := ruleSchemaReportFromCold(rule)
	if !reported || !reflect.DeepEqual(report, want) {
		t.Fatalf("Rule schema report = %#v, want %#v", report, want)
	}
	report.Reads[2].Dependencies[0] = 99
	report.Writes[1].Candidates[0] = 99
	report.Writes[1].Dependencies[0] = RuleWriteDependencyReport{Target: true, Index: 99}
	again, reported := ruleSchemaReportFromCold(rule)
	if !reported || !reflect.DeepEqual(again, want) {
		t.Fatal("Rule schema report retained caller-owned nested slices")
	}
}

func TestQuerySchemaReportPreservesEveryColdQueryFieldDetached(t *testing.T) {
	cold := func(value uint64) coldcomposition.Key { return coldKey(value).compositionKey() }
	factor, freezer, normalizer := cold(122_003), cold(122_004), cold(122_005)
	query := coldcomposition.QueryFamily{
		Key: cold(122_001), Freezer: freezer,
		Projections: []coldcomposition.QueryProjection{
			{Kind: coldcomposition.QueryFactorExact, Factor: factor},
			{Kind: coldcomposition.QueryFactorSummary, Factor: factor, Normalizer: normalizer},
			{Kind: coldcomposition.QuerySupport},
		},
	}
	want := QuerySchemaReport{
		Semantic: coldKey(122_001), Freezer: coldKey(122_004),
		Projections: []QueryProjectionReport{
			{Kind: QueryProjectionDispositionFactorExact, Factor: coldKey(122_003)},
			{Kind: QueryProjectionDispositionFactorSummary, Factor: coldKey(122_003), Normalizer: coldKey(122_005)},
			{Kind: QueryProjectionDispositionSupport},
		},
	}
	report, reported := querySchemaReportFromCold(query)
	if !reported || !reflect.DeepEqual(report, want) {
		t.Fatalf("Query schema report = %#v, want %#v", report, want)
	}
	report.Projections[0].Factor = SemanticKey{}
	again, reported := querySchemaReportFromCold(query)
	if !reported || !reflect.DeepEqual(again, want) {
		t.Fatal("Query schema report retained caller-owned projections")
	}
}

func semanticReportFixture(t testing.TB, reverse bool) *Composition {
	t.Helper()
	composition := NewComposition()
	factorAKey, factorBKey := coldKey(120_001), coldKey(120_002)
	var factorA, factorB *Factor[uint64, uint64]
	if reverse {
		factorB, factorA = coldFactor(composition, factorBKey), coldFactor(composition, factorAKey)
	} else {
		factorA, factorB = coldFactor(composition, factorAKey), coldFactor(composition, factorBKey)
	}
	if factorA == nil || factorB == nil {
		t.Fatal("factor declaration")
	}
	readA, readAOK := ExactReadForm(factorA)
	readB, readBOK := ExactReadForm(factorB)
	writeA, writeAOK := ExactWriteForm(factorA)
	writeB, writeBOK := ExactWriteForm(factorB)
	carryB, carryBOK := Carry(factorB)
	completion, completionOK := DeclareSupportCompletion(composition, coldKey(120_003))
	prune, pruneOK := DeclarePrune(completion, coldKey(120_004))
	activation, activationOK := DeclareActivationFamily(composition, coldKey(120_005))
	if !readAOK || !readBOK || !writeAOK || !writeBOK || !carryBOK || !completionOK || !pruneOK || !activationOK || prune == (Prune{}) {
		t.Fatal("semantic report child declarations")
	}
	declareSeed := func() bool {
		rule, ok := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
			Semantic: coldKey(120_006), Output: factorA.Output(), Inputs: 0,
			Admission: testTrustedTheorem[uint64](120_106), Transfer: func(Access[uint64, ruleUnit]) bool { return true },
		}, func(rule *Rule[uint64, ruleUnit]) bool {
			_, ok := WriteTo(rule, writeA)
			return ok
		})
		return ok && rule != nil
	}
	declareDependent := func() bool {
		rule, ok := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
			Semantic: coldKey(120_007), Output: factorB.Output(), Inputs: 1,
			Admission: testTrustedTheorem[uint64](120_107), Transfer: func(Access[uint64, ruleUnit]) bool { return true },
		}, func(rule *Rule[uint64, ruleUnit]) bool {
			input, inputOK := rule.InputAt(0)
			if !inputOK {
				return false
			}
			if _, readOK := ReadFrom(rule, input, readA); !readOK {
				return false
			}
			if !CarryFrom(rule, input, carryB) {
				return false
			}
			_, writeOK := WriteTo(rule, writeB)
			return writeOK
		})
		return ok && rule != nil
	}
	declareSupport := func() bool {
		rule, ok := DeclareSupportRule(composition, SupportRuleSpec{
			Semantic: coldKey(120_008), Completion: completion, Prune: prune, Inputs: 0,
			Admission: AdmitSupportByTrustedTheorem(coldKey(120_108)),
			Run:       func(value Support) (Support, bool) { return value, true },
		})
		return ok && rule != nil
	}
	declareActivation := func() bool {
		rule, ok := DeclareActivationRule(composition, ActivationRuleSpec{
			Semantic: coldKey(120_009), Family: activation, Inputs: 0,
			Admission: AdmitActivationByTrustedTheorem(coldKey(120_109)),
			Run:       func(Activation) bool { return true },
		})
		return ok && rule != nil
	}
	if reverse {
		if !declareActivation() || !declareSupport() || !declareDependent() || !declareSeed() {
			t.Fatal("permuted Rule declaration")
		}
	} else if !declareSeed() || !declareDependent() || !declareSupport() || !declareActivation() {
		t.Fatal("Rule declaration")
	}
	query, queryOK := declareColdQuery(composition, coldKey(120_010), coldKey(120_110), readB)
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("semantic report composition did not seal")
	}
	return composition
}
