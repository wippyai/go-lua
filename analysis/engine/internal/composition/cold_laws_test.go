package composition

import (
	"testing"

	queryschema "github.com/wippyai/go-lua/analysis/schema/query"
)

func coldKey(value byte) Key {
	var id ID
	id[0] = value
	return Key{ID: id, Version: 1}
}

func TestActivationFamilyIsOneSemanticCapability(t *testing.T) {
	first, firstOK := CanonicalActivationFamily(ActivationFamily{Semantic: coldKey(181)})
	second, secondOK := CanonicalActivationFamily(ActivationFamily{Semantic: coldKey(181)})
	if !firstOK || !secondOK || first != second {
		t.Fatal("activation capability was not canonical by semantic identity")
	}
	if family, ok := CanonicalActivationFamily(ActivationFamily{}); ok || family.Semantic.Available() {
		t.Fatal("activation family without semantic identity was accepted")
	}
}

func TestColdSealRejectsOrphanActivationFamily(t *testing.T) {
	factor, family := coldKey(197), coldKey(198)
	base := Candidate{
		Factors: []Factor{{Key: factor}},
		ActivationFamilies: []ActivationFamily{{
			Semantic: family,
		}},
	}
	if sealed, ok := Seal(base); ok || sealed != nil {
		t.Fatal("cold seal accepted an orphan activation family")
	}
	base.Rules = []Rule{{
		Key:           coldKey(199),
		OperandFamily: coldKey(200),
		OutputKind:    StructuralOutput,
		Activations:   []ActivationRange{{Family: family}},
	}}
	if sealed, ok := Seal(base); !ok || sealed == nil {
		t.Fatal("cold seal rejected a structurally declared activation family")
	}
}

func TestColdCompositionIDIgnoresEquationTopology(t *testing.T) {
	factor, rule, read := coldKey(1), coldKey(2), coldKey(3)
	query, freezer := coldKey(4), coldKey(5)
	first, ok := Seal(Candidate{
		Factors: []Factor{{Key: factor}, {Key: read}},
		Rules:   []Rule{{Key: rule, OperandFamily: coldKey(249), OutputKind: FactorOutput, Output: factor, Inputs: 1, Reads: []Read{{Kind: ReadExact, Input: 0, Factor: read}}, Writes: []Write{{Kind: WriteExact, Factor: factor}}}},
		Queries: []QueryFamily{{Key: query, Freezer: freezer, Population: queryschema.PopulationKindSelectedPoint, Projections: []QueryProjection{{Kind: QueryFactorExact, Factor: factor}}}},
	})
	if !ok || first == nil || !first.ID().Available() {
		t.Fatal("cold seal")
	}
	second, ok := Seal(Candidate{
		Factors: []Factor{{Key: read}, {Key: factor}},
		Rules:   []Rule{{Key: rule, OperandFamily: coldKey(249), OutputKind: FactorOutput, Output: factor, Inputs: 1, Reads: []Read{{Kind: ReadExact, Input: 0, Factor: read}}, Writes: []Write{{Kind: WriteExact, Factor: factor}}}},
		Queries: []QueryFamily{{Key: query, Freezer: freezer, Population: queryschema.PopulationKindSelectedPoint, Projections: []QueryProjection{{Kind: QueryFactorExact, Factor: factor}}}},
	})
	if !ok || first.ID() != second.ID() {
		t.Fatal("cold identity depends on declaration order")
	}
	if len(first.Incidence()) != 1 || first.Incidence()[0] != (Incidence{Read: read, Write: factor}) {
		t.Fatal("derived cold incidence")
	}
}

func TestRoutedWriteReadCoordinateEntersCompositionIdentity(t *testing.T) {
	factor, other := coldKey(6), coldKey(7)
	base := func(route uint64) Candidate {
		return Candidate{
			Factors: []Factor{{Key: factor}, {Key: other}},
			Rules: []Rule{{
				Key: coldKey(8), OperandFamily: coldKey(249), OutputKind: FactorOutput, Output: factor, Inputs: 2,
				Reads: []Read{
					{Kind: ReadExact, Input: 0, Factor: factor},
					{Kind: ReadSelect, Input: 0, Factor: factor, Semantic: factor, Dependencies: []uint64{0}},
					{Kind: ReadSelect, Input: 1, Factor: factor, Semantic: factor, Dependencies: []uint64{0}},
				},
				Writes: []Write{{Kind: WriteRoute, Factor: factor, Route: route}},
			}},
			Queries: []QueryFamily{{Key: coldKey(9), Freezer: coldKey(10), Population: queryschema.PopulationKindSelectedPoint, Projections: []QueryProjection{{Kind: QueryFactorExact, Factor: other}}}},
		}
	}
	first, firstOK := Seal(base(2))
	second, secondOK := Seal(base(3))
	if !firstOK || !secondOK || first == nil || second == nil || first.ID() == second.ID() {
		t.Fatalf("routed Write read coordinate identity: first=%t/%t second=%t/%t equal=%t", first != nil, firstOK, second != nil, secondOK, first != nil && second != nil && first.ID() == second.ID())
	}
}

func TestRuleOperandFamilyIsMandatoryAndCanonical(t *testing.T) {
	factor, rule, query, freezer := coldKey(201), coldKey(202), coldKey(203), coldKey(204)
	base := func() Candidate {
		return Candidate{
			Factors: []Factor{{Key: factor}},
			Rules: []Rule{{
				Key: rule, OperandFamily: coldKey(249), OutputKind: FactorOutput, Output: factor,
				Writes: []Write{{Kind: WriteExact, Factor: factor}},
			}},
			Queries: []QueryFamily{{Key: query, Freezer: freezer, Population: queryschema.PopulationKindSelectedPoint, Projections: []QueryProjection{{Kind: QueryFactorExact, Factor: factor}}}},
		}
	}
	first, firstOK := Seal(base())
	if !firstOK || first == nil {
		t.Fatal("cold rule with operand family rejected")
	}
	missingOperandFamily := base()
	missingOperandFamily.Rules[0].OperandFamily = Key{}
	if sealed, ok := Seal(missingOperandFamily); ok || sealed != nil {
		t.Fatal("cold rule without operand family was accepted")
	}
	changedOperandFamily := base()
	changedOperandFamily.Rules[0].OperandFamily = coldKey(206)
	third, thirdOK := Seal(changedOperandFamily)
	if !thirdOK || third == nil || first.ID() == third.ID() {
		t.Fatal("operand family was omitted from cold composition identity")
	}
}

func TestTransformedCarryFormIsValidatedAndChangesColdIdentity(t *testing.T) {
	factor, rule, query, freezer, transform := coldKey(211), coldKey(212), coldKey(213), coldKey(214), coldKey(215)
	base := func() Candidate {
		return Candidate{
			Factors: []Factor{{Key: factor}},
			Rules: []Rule{{
				Key: rule, OperandFamily: coldKey(249), OutputKind: FactorOutput, Output: factor, Inputs: 1,
				Carries: []Carry{{Input: 0, Factor: factor}},
			}},
			Queries: []QueryFamily{{Key: query, Freezer: freezer, Population: queryschema.PopulationKindSelectedPoint, Projections: []QueryProjection{{Kind: QueryFactorExact, Factor: factor}}}},
		}
	}
	ordinary, ordinaryOK := Seal(base())
	if !ordinaryOK || ordinary == nil {
		t.Fatal("ordinary carry composition")
	}
	mapped := base()
	mapped.Rules[0].Carries[0].Transform = transform
	transformed, transformedOK := Seal(mapped)
	if !transformedOK || transformed == nil || ordinary.ID() == transformed.ID() {
		t.Fatal("transformed carry was not a distinct cold term")
	}
	duplicate := base()
	duplicate.Rules[0].Carries[0].Transform = query
	if sealed, ok := Seal(duplicate); ok || sealed != nil {
		t.Fatal("carry transform reused another cold identity")
	}
}

func TestColdQueryFamilyRetainsFreezerButRejectsTopology(t *testing.T) {
	factor, completion, prune := coldKey(11), coldKey(12), coldKey(13)
	family, freezer := coldKey(14), coldKey(15)
	sealed, ok := Seal(Candidate{Factors: []Factor{{Key: factor}}, Completion: Completion{Semantic: completion, Prune: prune}, Queries: []QueryFamily{{Key: family, Freezer: freezer, Population: queryschema.PopulationKindSelectedPoint, Projections: []QueryProjection{{Kind: QuerySupport}}}}})
	if !ok || sealed == nil {
		t.Fatal("support family")
	}
	families := sealed.Queries()
	if len(families) != 1 || families[0].Freezer != freezer {
		t.Fatal("freezer identity was lost")
	}
	registered, registeredOK := sealed.Completion()
	if !registeredOK || registered.Semantic != completion || registered.Prune != prune || len(sealed.Incidence()) != 0 || len(sealed.Components()) != 1 || len(sealed.Components()[0].Factors) != 1 || sealed.Components()[0].Factors[0] != factor {
		t.Fatal("Composition support completion entered Factor graph state")
	}
}

func TestColdQueryReadOrderChangesIdentityButNotFactorGraph(t *testing.T) {
	left, right, rule := coldKey(31), coldKey(32), coldKey(33)
	family, freezer := coldKey(34), coldKey(35)
	first, firstOK := Seal(Candidate{
		Factors: []Factor{{Key: left}, {Key: right}},
		Rules:   []Rule{{Key: rule, OperandFamily: coldKey(249), OutputKind: FactorOutput, Output: left, Inputs: 1, Reads: []Read{{Kind: ReadExact, Input: 0, Factor: right}}, Writes: []Write{{Kind: WriteExact, Factor: left}}}},
		Queries: []QueryFamily{{Key: family, Freezer: freezer, Population: queryschema.PopulationKindSelectedPoint, Projections: []QueryProjection{{Kind: QueryFactorExact, Factor: left}, {Kind: QueryFactorExact, Factor: right}}}},
	})
	second, secondOK := Seal(Candidate{
		Factors: []Factor{{Key: right}, {Key: left}},
		Rules:   []Rule{{Key: rule, OperandFamily: coldKey(249), OutputKind: FactorOutput, Output: left, Inputs: 1, Reads: []Read{{Kind: ReadExact, Input: 0, Factor: right}}, Writes: []Write{{Kind: WriteExact, Factor: left}}}},
		Queries: []QueryFamily{{Key: family, Freezer: freezer, Population: queryschema.PopulationKindSelectedPoint, Projections: []QueryProjection{{Kind: QueryFactorExact, Factor: right}, {Kind: QueryFactorExact, Factor: left}}}},
	})
	if !firstOK || !secondOK || first == nil || second == nil || first.ID() == second.ID() {
		t.Fatal("ordered query projections were not part of cold identity")
	}
	if len(first.Incidence()) != len(second.Incidence()) || len(first.Components()) != len(second.Components()) {
		t.Fatal("query projection order changed the Factor SCC graph")
	}
}

func TestColdQueryScalarShapesPreserveOrderedProjectionProof(t *testing.T) {
	left, right := coldKey(41), coldKey(42)
	family, freezer, normalizer := coldKey(43), coldKey(44), coldKey(45)
	sealed, ok := Seal(Candidate{
		Factors: []Factor{{Key: left}, {Key: right, Forms: []FactorForm{{Kind: FactorSummaryRead, Semantic: normalizer}}}},
		Queries: []QueryFamily{{Key: family, Freezer: freezer, Population: queryschema.PopulationKindSelectedPoint, Projections: []QueryProjection{
			{Kind: QueryFactorExact, Factor: left},
			{Kind: QueryFactorSummary, Factor: right, Normalizer: normalizer},
		}}},
	})
	if !ok || sealed == nil {
		t.Fatal("query shape fixture")
	}
	index, found := sealed.QueryIndex(family)
	shape, shaped := sealed.QueryShapeAt(index)
	first, firstOK := sealed.QueryProjectionShapeAt(index, 0)
	second, secondOK := sealed.QueryProjectionShapeAt(index, 1)
	if !found || !shaped || shape != (QueryShape{Freezer: freezer, Population: queryschema.PopulationKindSelectedPoint, ProjectionCount: 2}) ||
		!firstOK || first != (QueryProjectionShape{Kind: QueryFactorExact, Factor: left}) ||
		!secondOK || second != (QueryProjectionShape{Kind: QueryFactorSummary, Factor: right, Normalizer: normalizer}) {
		t.Fatal("scalar query shape projection disagrees with sealed order")
	}
	detached := sealed.Queries()
	detached[0].Projections[0].Factor = right
	again, againOK := sealed.QueryProjectionShapeAt(index, 0)
	if !againOK || again != first {
		t.Fatal("detached query copy mutated the sealed scalar proof")
	}
	if _, present := sealed.QueryProjectionShapeAt(index, 2); present {
		t.Fatal("out-of-range query projection was admitted")
	}
}

func TestColdCompletionChangesIdentityOutsideFactorGraph(t *testing.T) {
	factor := coldKey(21)
	first, firstOK := Seal(Candidate{
		Factors:    []Factor{{Key: factor}},
		Completion: Completion{Semantic: coldKey(22), Prune: coldKey(23)},
	})
	second, secondOK := Seal(Candidate{
		Factors:    []Factor{{Key: factor}},
		Completion: Completion{Semantic: coldKey(22), Prune: coldKey(24)},
	})
	if !firstOK || !secondOK || first == nil || second == nil || first.ID() == second.ID() {
		t.Fatal("top-level support completion law was omitted from cold identity")
	}
	if len(first.Factors()) != 1 || len(first.Incidence()) != 0 || len(first.Components()) != 1 || len(first.Components()[0].Factors) != 1 || first.Components()[0].Factors[0] != factor {
		t.Fatal("support completion law changed the Factor graph")
	}
}

func TestDeclaredFactorFormsAreFrozenHashedAndRequiredBySummaryReads(t *testing.T) {
	factor, firstForm, secondForm := coldKey(31), coldKey(32), coldKey(33)
	first, firstOK := Seal(Candidate{Factors: []Factor{{Key: factor, Forms: []FactorForm{{Kind: FactorSummaryRead, Semantic: firstForm}}}}})
	second, secondOK := Seal(Candidate{Factors: []Factor{{Key: factor, Forms: []FactorForm{{Kind: FactorSummaryRead, Semantic: secondForm}}}}})
	if !firstOK || first == nil || !secondOK || second == nil || first.ID() == second.ID() {
		t.Fatal("declared Factor form was omitted from cold composition identity")
	}
	forms := first.Factors()
	if len(forms) != 1 || len(forms[0].Forms) != 1 || forms[0].Forms[0] != (FactorForm{Kind: FactorSummaryRead, Semantic: firstForm}) {
		t.Fatal("declared Factor form did not survive cold sealing")
	}
	forms[0].Forms[0].Semantic = secondForm
	if again := first.Factors(); len(again) != 1 || len(again[0].Forms) != 1 || again[0].Forms[0].Semantic != firstForm {
		t.Fatal("Factor form escaped immutable cold schema")
	}

	if sealed, ok := Seal(Candidate{Factors: []Factor{{Key: factor, Forms: []FactorForm{{Kind: FactorSummaryRead, Semantic: firstForm}, {Kind: FactorSummaryRead, Semantic: firstForm}}}}}); ok || sealed != nil {
		t.Fatal("duplicate Factor form semantic was admitted")
	}
	if sealed, ok := Seal(Candidate{Factors: []Factor{{Key: factor, Forms: []FactorForm{{Kind: FactorSummaryRead, Semantic: factor}}}}}); ok || sealed != nil {
		t.Fatal("Factor form reused its owner semantic")
	}
	if sealed, ok := Seal(Candidate{Factors: []Factor{{Key: factor, Forms: []FactorForm{{Kind: FactorFormKind(99), Semantic: firstForm}}}}}); ok || sealed != nil {
		t.Fatal("unknown Factor form kind was admitted")
	}

	query := QueryFamily{Key: coldKey(34), Freezer: coldKey(35), Population: queryschema.PopulationKindSelectedPoint, Projections: []QueryProjection{{Kind: QueryFactorSummary, Factor: factor, Normalizer: secondForm}}}
	if sealed, ok := Seal(Candidate{Factors: []Factor{{Key: factor, Forms: []FactorForm{{Kind: FactorSummaryRead, Semantic: firstForm}}}}, Queries: []QueryFamily{query}}); ok || sealed != nil {
		t.Fatal("summary query accepted a normalizer not declared by its Factor")
	}
	query.Projections[0].Normalizer = firstForm
	if sealed, ok := Seal(Candidate{Factors: []Factor{{Key: factor, Forms: []FactorForm{{Kind: FactorSummaryRead, Semantic: firstForm}}}}, Queries: []QueryFamily{query}}); !ok || sealed == nil {
		t.Fatal("summary query rejected its Factor-declared normalizer")
	}
}

func TestColdDeclaredFormsObeyPublicGlobalClaimLaw(t *testing.T) {
	factor, other, form := coldKey(36), coldKey(37), coldKey(38)
	base := func(semantic Key) Candidate {
		return Candidate{Factors: []Factor{{Key: factor, Forms: []FactorForm{{Kind: FactorSummaryRead, Semantic: semantic}}}}}
	}
	for _, scenario := range []struct {
		name      string
		candidate Candidate
	}{
		{
			name: "cross-factor-form",
			candidate: Candidate{Factors: []Factor{
				{Key: factor, Forms: []FactorForm{{Kind: FactorSummaryRead, Semantic: form}}},
				{Key: other, Forms: []FactorForm{{Kind: FactorSummaryRead, Semantic: form}}},
			}},
		},
		{
			name:      "other-factor-key",
			candidate: Candidate{Factors: []Factor{{Key: factor, Forms: []FactorForm{{Kind: FactorSummaryRead, Semantic: other}}}, {Key: other}}},
		},
		{
			name: "rule-key",
			candidate: func() Candidate {
				candidate := base(coldKey(39))
				candidate.Rules = []Rule{{Key: coldKey(39), OperandFamily: coldKey(40), OutputKind: FactorOutput, Output: factor, Writes: []Write{{Kind: WriteExact, Factor: factor}}}}
				return candidate
			}(),
		},
		{
			name: "query-key",
			candidate: func() Candidate {
				candidate := base(coldKey(41))
				candidate.Queries = []QueryFamily{{Key: coldKey(41), Freezer: coldKey(42), Population: queryschema.PopulationKindSelectedPoint, Projections: []QueryProjection{{Kind: QueryFactorExact, Factor: factor}}}}
				return candidate
			}(),
		},
		{
			name: "query-freezer",
			candidate: func() Candidate {
				candidate := base(coldKey(43))
				candidate.Queries = []QueryFamily{{Key: coldKey(44), Freezer: coldKey(43), Population: queryschema.PopulationKindSelectedPoint, Projections: []QueryProjection{{Kind: QueryFactorExact, Factor: factor}}}}
				return candidate
			}(),
		},
		{
			name: "activation",
			candidate: func() Candidate {
				candidate := base(coldKey(45))
				candidate.ActivationFamilies = []ActivationFamily{{Semantic: coldKey(45)}}
				return candidate
			}(),
		},
		{
			name: "completion",
			candidate: func() Candidate {
				candidate := base(coldKey(46))
				candidate.Completion = Completion{Semantic: coldKey(46), Prune: coldKey(47)}
				return candidate
			}(),
		},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			if sealed, ok := Seal(scenario.candidate); ok || sealed != nil {
				t.Fatal("cold composition admitted a public semantic-claim collision")
			}
		})
	}
}

func TestRuleOutputDispositionIsCanonicalAndExclusive(t *testing.T) {
	factor, input := coldKey(41), coldKey(42)
	rule, query, freezer := coldKey(43), coldKey(44), coldKey(45)
	base := Candidate{
		Factors: []Factor{{Key: factor}, {Key: input}},
		Queries: []QueryFamily{{Key: query, Freezer: freezer, Population: queryschema.PopulationKindSelectedPoint, Projections: []QueryProjection{{Kind: QueryFactorExact, Factor: factor}}}},
	}
	factorRule := Rule{
		Key: rule, OperandFamily: coldKey(249), OutputKind: FactorOutput, Output: factor, Inputs: 1,
		Reads:  []Read{{Kind: ReadExact, Input: 0, Factor: input}},
		Writes: []Write{{Kind: WriteExact, Factor: factor}},
	}
	base.Rules = []Rule{factorRule}
	sealed, ok := Seal(base)
	if !ok || sealed == nil || len(sealed.Incidence()) != 1 || sealed.Incidence()[0] != (Incidence{Read: input, Write: factor}) {
		t.Fatal("Factor output did not own its one incidence target")
	}

	untagged := base
	untagged.Rules[0].OutputKind = OutputInvalid
	if sealed, ok := Seal(untagged); ok || sealed != nil {
		t.Fatal("untagged output was admitted")
	}

	multipleCarries := base
	multipleCarries.Rules[0].Carries = []Carry{{Input: 0, Factor: factor}, {Input: 0, Factor: factor}}
	if sealed, ok := Seal(multipleCarries); ok || sealed != nil {
		t.Fatal("Factor Rule owned more than one whole-Factor carry")
	}

	completion, prune := coldKey(46), coldKey(47)
	structural := Candidate{
		Factors:    []Factor{{Key: factor}},
		Completion: Completion{Semantic: completion, Prune: prune},
		Rules: []Rule{{
			Key: rule, OperandFamily: coldKey(249), OutputKind: StructuralOutput, Inputs: 1,
			Supports: []Support{{Semantic: completion}}, Prunes: []Prune{{Semantic: prune}},
		}},
		Queries: []QueryFamily{{Key: query, Freezer: freezer, Population: queryschema.PopulationKindSelectedPoint, Projections: []QueryProjection{{Kind: QuerySupport}}}},
	}
	sealed, ok = Seal(structural)
	if !ok || sealed == nil || len(sealed.Incidence()) != 0 {
		t.Fatal("structural output entered the Factor graph")
	}
	if sealed.Rules()[0].OutputKind != StructuralOutput {
		t.Fatal("structural output disposition was not retained")
	}

	structural.Rules[0].Writes = []Write{{Kind: WriteExact, Factor: factor}}
	if sealed, ok := Seal(structural); ok || sealed != nil {
		t.Fatal("structural output owned a Factor write")
	}
}

func TestStructuralOutputRuleEntersCompositionIdentity(t *testing.T) {
	factor := coldKey(51)
	completion, prune := coldKey(52), coldKey(53)
	query, freezer := coldKey(54), coldKey(55)
	structural := Candidate{
		Factors:    []Factor{{Key: factor}},
		Completion: Completion{Semantic: completion, Prune: prune},
		Rules:      []Rule{{Key: coldKey(56), OperandFamily: coldKey(249), OutputKind: StructuralOutput, Inputs: 1, Supports: []Support{{Semantic: completion}}, Prunes: []Prune{{Semantic: prune}}}},
		Queries:    []QueryFamily{{Key: query, Freezer: freezer, Population: queryschema.PopulationKindSelectedPoint, Projections: []QueryProjection{{Kind: QuerySupport}}}},
	}
	first, firstOK := Seal(structural)
	if !firstOK || first == nil {
		t.Fatal("structural output composition")
	}
	structural.Rules[0].Key = coldKey(57)
	second, secondOK := Seal(structural)
	if !secondOK || second == nil || first.ID() == second.ID() {
		t.Fatal("structural output Rule was omitted from composition identity")
	}
}

func TestFactorRuleRetainsArbitraryOrderedInputArity(t *testing.T) {
	factor, input := coldKey(61), coldKey(62)
	rule, query, freezer := coldKey(63), coldKey(64), coldKey(65)
	candidate := Candidate{
		Factors: []Factor{{Key: factor}, {Key: input}},
		Rules: []Rule{{
			Key: rule, OperandFamily: coldKey(249), OutputKind: FactorOutput, Output: factor, Inputs: 3,
			Reads:   []Read{{Kind: ReadExact, Input: 2, Factor: input}},
			Carries: []Carry{{Input: 1, Factor: factor}},
			Writes:  []Write{{Kind: WriteExact, Factor: factor}},
		}},
		Queries: []QueryFamily{{Key: query, Freezer: freezer, Population: queryschema.PopulationKindSelectedPoint, Projections: []QueryProjection{{Kind: QueryFactorExact, Factor: factor}}}},
	}
	sealed, ok := Seal(candidate)
	if !ok || sealed == nil || sealed.Rules()[0].Inputs != 3 {
		t.Fatal("finite ordered three-input Factor Rule was rejected")
	}

	candidate.Rules[0].Reads[0].Input = 3
	if sealed, ok := Seal(candidate); ok || sealed != nil {
		t.Fatal("out-of-range input port was admitted")
	}
}

func TestFactorRuleAdmitsZeroInputIngress(t *testing.T) {
	factor, rule, query, freezer := coldKey(66), coldKey(67), coldKey(68), coldKey(69)
	sealed, ok := Seal(Candidate{
		Factors: []Factor{{Key: factor}},
		Rules: []Rule{{
			Key: rule, OperandFamily: coldKey(249), OutputKind: FactorOutput, Output: factor, Inputs: 0,
			Writes: []Write{{Kind: WriteExact, Factor: factor}},
		}},
		Queries: []QueryFamily{{Key: query, Freezer: freezer, Population: queryschema.PopulationKindSelectedPoint, Projections: []QueryProjection{{Kind: QueryFactorExact, Factor: factor}}}},
	})
	if !ok || sealed == nil || len(sealed.Rules()) != 1 || sealed.Rules()[0].Inputs != 0 {
		t.Fatal("zero-input Factor ingress Rule was rejected")
	}
}
