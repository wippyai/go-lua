package coverage

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/lattice"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/semanticsource"
)

func TestSourceCatalogUsesEveryIssuedTokenInCanonicalOrder(t *testing.T) {
	catalog, ok := NewSourceCatalog(lawPublications(t))
	if !ok || catalog.Count() != semanticsource.CatalogSchema().Count() {
		t.Fatal("canonical source catalog")
	}
	sawZero := false
	for index := 0; index < catalog.Count(); index++ {
		token, exists := catalog.TokenAt(index)
		measure, measured := catalog.MeasureAt(index)
		if !exists || !measured || !availableToken(token) || measure.Token() != token {
			t.Fatalf("source token %d", index)
		}
		if index != 0 {
			previous, _ := catalog.TokenAt(index - 1)
			if compareToken(previous, token) >= 0 {
				t.Fatalf("source catalog order at %d", index)
			}
		}
		sawZero = sawZero || measure.Count() == 0
	}
	if !sawZero {
		t.Fatal("zero-count source family was lost")
	}
}

func TestFreezeAcceptsExactContractsAndMultiRequirementRule(t *testing.T) {
	catalog := lawCatalog(t)
	composition, ruleA, ruleB, query := lawComposition(t)
	requirements := lawRequirements(t, catalog)
	first, second := splitRequirements(requirements)
	ledger, result := Freeze(catalog, lawContracts(requirements),
		[]RulePlan{{Semantic: ruleA, Covers: first}, {Semantic: ruleB, Covers: second[:1]}},
		[]QueryPlan{{Semantic: query, Covers: second[1:]}},
		nil, composition,
	)
	if !result.Valid() || !ledger.valid || ledger.compositionID != composition.ID() {
		t.Fatal("exact coverage contract was rejected")
	}
	if len(first) < 2 {
		t.Fatal("law catalog did not exercise a multi-requirement Rule")
	}
}

func TestFreezeAcceptsExplicitStructuralTreatments(t *testing.T) {
	catalog := lawCatalog(t)
	composition, ruleA, ruleB, query := lawComposition(t)
	requirements := lawRequirements(t, catalog)
	authority := keyspace.ContentID{0: 0xA0}
	requirements[0] = Requirement{Source: requirements[0].Source, Class: OwnerStructural, Authority: authority, AuthorityKind: StructuralAuthoritySource}
	if len(requirements) < 5 {
		t.Fatal("law source denominator")
	}
	_, result := Freeze(catalog, lawContracts(requirements),
		[]RulePlan{{Semantic: ruleA, Covers: []Requirement{requirements[2]}}, {Semantic: ruleB, Covers: []Requirement{requirements[1]}}},
		[]QueryPlan{{Semantic: query, Covers: requirements[3:]}},
		[]StructuralPlan{{Authority: authority, AuthorityKind: StructuralAuthoritySource, Covers: []Requirement{requirements[0]}}},
		composition,
	)
	if !result.Valid() {
		t.Fatal("explicit structural disposition was rejected")
	}
}

func TestFreezeBindsStructuralTreatmentToExactOwnerAuthority(t *testing.T) {
	catalog := lawCatalog(t)
	composition, ruleA, ruleB, query := lawComposition(t)
	requirements := lawRequirements(t, catalog)
	authority := keyspace.ContentID{0: 0xA1}
	requirements[0] = Requirement{
		Source: requirements[0].Source, Class: OwnerStructural,
		Authority: authority, AuthorityKind: StructuralAuthoritySource,
	}
	contracts := lawContracts(requirements)
	contracts[0] = CoverageContract{
		Source: requirements[0].Source, Class: OwnerStructural,
		Authority: authority, AuthorityKind: StructuralAuthoritySource,
	}
	_, result := Freeze(catalog, contracts,
		[]RulePlan{{Semantic: ruleA, Covers: []Requirement{requirements[2]}}, {Semantic: ruleB, Covers: []Requirement{requirements[1]}}},
		[]QueryPlan{{Semantic: query, Covers: requirements[3:]}},
		[]StructuralPlan{{Authority: authority, AuthorityKind: StructuralAuthoritySource, Covers: []Requirement{requirements[0]}}},
		composition,
	)
	if !result.Valid() {
		t.Fatalf("typed structural authority was rejected: %#v", result.Issues)
	}
}

func TestFreezeRetainsDistinctConclusionsForOneSourceAndFactor(t *testing.T) {
	catalog := lawCatalog(t)
	composition, ruleA, ruleB, query := lawComposition(t)
	requirements := lawRequirements(t, catalog)
	extra := requirements[0]
	extra.Conclusion = lawKey(200)
	requirements = append(requirements, extra)
	_, result := Freeze(catalog, lawContracts(requirements),
		[]RulePlan{{Semantic: ruleA, Covers: []Requirement{requirements[0], extra}}, {Semantic: ruleB, Covers: []Requirement{requirements[1]}}},
		[]QueryPlan{{Semantic: query, Covers: requirements[2 : len(requirements)-1]}},
		nil, composition,
	)
	if !result.Valid() {
		t.Fatal("two conclusions for one source and Factor were collapsed")
	}
}

func TestFreezeRejectsMissingDuplicateReuseAndUnclaimedComposition(t *testing.T) {
	catalog := lawCatalog(t)
	composition, ruleA, ruleB, query := lawComposition(t)
	requirements := lawRequirements(t, catalog)
	contracts := lawContracts(requirements)
	contracts = append(contracts[:len(contracts)-1], contractFor(requirements[0])) // omit final; duplicate first.
	unknown := Requirement{Source: requirements[0].Source, Class: OwnerFactor, Owner: lawKey(100), Conclusion: lawKey(250)}
	_, result := Freeze(catalog, contracts,
		[]RulePlan{{Semantic: ruleA, Covers: []Requirement{requirements[0]}}, {Semantic: ruleA, Covers: []Requirement{requirements[0]}}},
		[]QueryPlan{{Semantic: query, Covers: []Requirement{unknown}}},
		nil, composition,
	)
	if !hasIssue(result, IssueDuplicateRequirement) ||
		!hasIssue(result, IssueDuplicateTreatmentSemantic) || !hasIssue(result, IssueUnknownTreatmentRequirement) ||
		!hasIssue(result, IssueMissingRequirement) || !hasIssue(result, IssueUnclaimedCompositionRule) {
		t.Fatal("incomplete coverage was accepted")
	}
	_ = ruleB
}

func TestFreezeRejectsNonCanonicalCoversAndCompositionKindDrift(t *testing.T) {
	catalog := lawCatalog(t)
	composition, ruleA, ruleB, _ := lawComposition(t)
	requirements := lawRequirements(t, catalog)
	first, second := splitRequirements(requirements)
	authority := keyspace.ContentID{0: 0xA3}
	_, result := Freeze(catalog, lawContracts(requirements),
		[]RulePlan{{Semantic: ruleA, Covers: []Requirement{first[1], first[0]}}},
		[]QueryPlan{{Semantic: ruleB, Covers: second}},
		[]StructuralPlan{{Authority: authority, AuthorityKind: StructuralAuthoritySource, Covers: []Requirement{first[0]}}}, composition,
	)
	if !hasIssue(result, IssueNonCanonicalCovers) || !hasIssue(result, IssueTreatmentKindMismatch) || !hasIssue(result, IssueTreatmentReuse) {
		t.Fatal("noncanonical or kind-drifted treatment was accepted")
	}
}

func TestFreezeRejectsStructuralCrossClassCoverage(t *testing.T) {
	catalog := lawCatalog(t)
	composition, ruleA, ruleB, query := lawComposition(t)
	requirements := lawRequirements(t, catalog)
	authority := keyspace.ContentID{0: 0xA4}
	_, result := Freeze(catalog, lawContracts(requirements),
		[]RulePlan{{Semantic: ruleA, Covers: []Requirement{requirements[2]}}, {Semantic: ruleB, Covers: []Requirement{requirements[3]}}},
		[]QueryPlan{{Semantic: query, Covers: requirements[4:]}},
		[]StructuralPlan{{Authority: authority, AuthorityKind: StructuralAuthoritySource, Covers: []Requirement{requirements[0]}}},
		composition,
	)
	if !hasIssue(result, IssueTreatmentKindMismatch) {
		t.Fatal("structural plan covered a Factor requirement")
	}
}

func TestFreezeRejectsSemanticOnlyStructuralClaim(t *testing.T) {
	catalog := lawCatalog(t)
	composition, ruleA, ruleB, query := lawComposition(t)
	requirements := lawRequirements(t, catalog)
	// A semantic-looking Owner/Conclusion pair is not a structural authority.
	// Structural coverage must carry the exact sealed owner ID and kind.
	requirements[0] = Requirement{Source: requirements[0].Source, Class: OwnerStructural, Owner: lawKey(242), Conclusion: lawKey(243)}
	_, result := Freeze(catalog, lawContracts(requirements),
		[]RulePlan{{Semantic: ruleA, Covers: []Requirement{requirements[2]}}, {Semantic: ruleB, Covers: []Requirement{requirements[3]}}},
		[]QueryPlan{{Semantic: query, Covers: requirements[1:2]}},
		[]StructuralPlan{{Covers: []Requirement{requirements[0]}}}, composition,
	)
	if result.Valid() || !hasIssue(result, IssueInvalidRequirement) || !hasIssue(result, IssueInvalidTreatment) {
		t.Fatalf("semantic-only structural claim was accepted: %#v", result.Issues)
	}
}

func lawCatalog(t testing.TB) SourceCatalog {
	t.Helper()
	catalog, ok := NewSourceCatalog(lawPublications(t))
	if !ok || catalog.Count() < 4 {
		t.Fatal("law catalog")
	}
	return catalog
}

func lawRequirements(t testing.TB, catalog SourceCatalog) []Requirement {
	t.Helper()
	requirements := make([]Requirement, catalog.Count())
	for index := range requirements {
		token, ok := catalog.TokenAt(index)
		if !ok {
			t.Fatal("law token")
		}
		requirements[index] = Requirement{Source: token, Class: OwnerFactor, Owner: lawKey(100), Conclusion: lawKey(byte(index + 1))}
	}
	return requirements
}

func lawContracts(requirements []Requirement) []CoverageContract {
	contracts := make([]CoverageContract, len(requirements))
	for index, requirement := range requirements {
		contracts[index] = contractFor(requirement)
	}
	return contracts
}

func contractFor(requirement Requirement) CoverageContract {
	return CoverageContract{
		Source: requirement.Source, Class: requirement.Class,
		Owner: requirement.Owner, Conclusion: requirement.Conclusion,
		Authority: requirement.Authority, AuthorityKind: requirement.AuthorityKind,
	}
}

func lawPublications(t testing.TB) semanticsource.Publications {
	t.Helper()
	schema := semanticsource.CatalogSchema()
	publisher, err := semanticsource.NewPublisher(schema)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < schema.Count(); index++ {
		definition, ok := schema.DefinitionAt(index)
		if !ok {
			t.Fatalf("source definition %d", index)
		}
		count := 0
		if index%3 == 1 {
			count = index + 1
		}
		publication, err := semanticsource.SealPublication(definition, count)
		if err != nil || publisher.Accept(publication) != nil {
			t.Fatalf("source publication %d", index)
		}
	}
	publications, err := publisher.Seal()
	if err != nil {
		t.Fatal(err)
	}
	return publications
}

func splitRequirements(requirements []Requirement) ([]Requirement, []Requirement) {
	cut := len(requirements) / 2
	if cut < 2 {
		cut = 2
	}
	return requirements[:cut], requirements[cut:]
}

type lawOperand struct{}

func lawComposition(t testing.TB) (*engine.Composition, engine.SemanticKey, engine.SemanticKey, engine.SemanticKey) {
	t.Helper()
	composition := engine.NewComposition()
	factor, declared := engine.DeclareFactor(composition, engine.FactorSpec[uint64, uint64]{
		Semantic: lawKey(100), KeyEnd: 1, Default: 0, Lattice: lawLattice(),
		AdmitAt: func(uint64, uint64) bool { return true }, Fingerprint: func(value uint64) uint64 { return value },
	}, func(*engine.Factor[uint64, uint64]) bool { return true })
	if !declared || factor == nil {
		t.Fatal("law Factor")
	}
	read, readOK := engine.ExactReadForm(factor)
	write, writeOK := engine.ExactWriteForm(factor)
	ruleA, ruleAOK := lawDeclareRule(composition, factor, write, 110)
	ruleB, ruleBOK := lawDeclareRule(composition, factor, write, 120)
	query, queryOK := lawDeclareQuery(composition, read, 130, 131)
	if !readOK || !writeOK || !ruleAOK || !ruleBOK || !queryOK || !composition.Seal() {
		t.Fatal("law composition declaration")
	}
	return composition, ruleA, ruleB, query
}

func lawDeclareRule(composition *engine.Composition, factor *engine.Factor[uint64, uint64], write engine.WriteForm[uint64], seed byte) (engine.SemanticKey, bool) {
	semantic := lawKey(seed)
	rule, declared := engine.DeclareRule(composition, engine.RuleSpec[uint64, lawOperand]{
		Semantic: semantic, OperandFamily: lawKey(seed + 1), OperandContent: lawOperandContent,
		Output: factor.Output(), Inputs: 0, Admission: engine.AdmitRuleByTrustedTheorem[uint64, lawOperand](lawKey(seed + 2)),
		Transfer: func(engine.Access[uint64, lawOperand]) bool { return true },
	}, func(rule *engine.Rule[uint64, lawOperand]) bool {
		_, wrote := engine.WriteTo(rule, write)
		return wrote
	})
	return semantic, declared && rule != nil
}

func lawDeclareQuery(composition *engine.Composition, read engine.ReadForm[uint64, engine.OrderedCells[uint64]], semantic, freezer byte) (engine.SemanticKey, bool) {
	key := lawKey(semantic)
	query, declared := engine.DeclareQuery(composition, engine.QuerySpec[uint64]{
		Semantic: key, Project: func(engine.Observation) uint64 { return 0 },
		Result: engine.FrozenResult[uint64]{
			Semantic: lawKey(freezer), Freeze: func(value uint64) uint64 { return value }, Clone: func(value uint64) uint64 { return value },
			Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value },
		},
	}, func(query *engine.Query[uint64]) bool {
		_, readOK := engine.QueryReadFrom(query, read)
		return readOK
	})
	return key, declared && query != nil
}

func lawKey(seed byte) engine.SemanticKey {
	var digest [32]byte
	digest[31] = seed
	key, valid := engine.NewSemanticKey(digest, 1)
	if !valid {
		panic("law semantic key")
	}
	return key
}

func lawOperandContent(value lawOperand) (lawOperand, [32]byte, bool) {
	return value, [32]byte{1}, true
}

func lawLattice() lattice.Lattice[uint64] {
	return lattice.Lattice[uint64]{
		Bottom: func() uint64 { return 0 }, Top: func() uint64 { return ^uint64(0) },
		Equal: func(left, right uint64) bool { return left == right }, LessOrEq: func(left, right uint64) bool { return left <= right },
		Join: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
		Widen: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
	}
}

func hasIssue(result Result, want IssueKind) bool {
	for _, issue := range result.Issues {
		if issue.Kind == want {
			return true
		}
	}
	return false
}
