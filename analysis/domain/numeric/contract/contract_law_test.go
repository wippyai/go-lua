package contract

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/coverage"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/semanticsource"
)

func TestContractsAreDeterministicCanonicalAndDetached(t *testing.T) {
	factor := numericContractFactor(t)
	first, firstOK := Contracts(factor)
	second, secondOK := Contracts(factor)
	sealed, sealedOK := coverage.SealContracts(first)
	if !firstOK || !secondOK || !sealedOK || len(first) != len(second) || len(first) != len(sealed) {
		t.Fatal("canonical Numeric contracts")
	}
	for index := range first {
		if first[index] != second[index] || first[index] != sealed[index] {
			t.Fatalf("contract %d changed", index)
		}
	}
	first[0] = coverage.CoverageContract{}
	third, thirdOK := Contracts(factor)
	if !thirdOK || third[0] == (coverage.CoverageContract{}) {
		t.Fatal("Numeric contracts retained caller mutation")
	}
}

func TestContractsDeriveDistinctJudgmentConclusions(t *testing.T) {
	contracts, ok := Contracts(numericContractFactor(t))
	if !ok {
		t.Fatal("Numeric contracts")
	}
	conclusions := make(map[engine.SemanticKey]struct{})
	for _, contract := range contracts {
		if contract.Conclusion == (engine.SemanticKey{}) {
			t.Fatal("zero Numeric conclusion")
		}
		conclusions[contract.Conclusion] = struct{}{}
	}
	if len(conclusions) != 3 {
		t.Fatalf("Numeric judgment conclusions = %d, want 3", len(conclusions))
	}
	if conclusionFor(t, contracts, semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowArithmetic) ==
		conclusionFor(t, contracts, semanticsource.OriginProgramFlowControl, 0) {
		t.Fatal("Numeric judgments were collapsed or split by source row")
	}
}

func TestContractsInventoryExactRelevantSourcesWithoutDuplicates(t *testing.T) {
	contracts, ok := Contracts(numericContractFactor(t))
	if !ok {
		t.Fatal("Numeric contracts")
	}
	want := map[sourcePair]struct{}{
		{semanticsource.OriginProgramFlowLiterals, 0}:                                            {},
		{semanticsource.OriginProgramFlowValues, semanticsource.FacetProgramFlowValueOccurrence}: {},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowUnaryNumeric}: {},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowLength}:       {},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowArithmetic}:   {},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowBitwise}:      {},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowEquality}:     {},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowOrder}:        {},
		{semanticsource.OriginProgramFlowControl, 0}:                                             {},
		{semanticsource.OriginProgramFlowBody, 0}:                                                {},
		{semanticsource.OriginProgramFlowBody, semanticsource.FacetProgramFlowBodyRoots}:         {},
	}
	if len(contracts) != len(want) {
		t.Fatalf("Numeric source inventory = %d, want %d", len(contracts), len(want))
	}
	seen := make(map[sourcePair]struct{}, len(contracts))
	for _, contract := range contracts {
		pair := sourcePair{contract.Source.Origin(), contract.Source.Facet()}
		if _, expected := want[pair]; !expected {
			t.Fatalf("unexpected Numeric source %#v", pair)
		}
		definition, defined := semanticsource.Definition(pair.origin, pair.facet)
		if !defined || definition.Token() != contract.Source {
			t.Fatalf("Numeric source token drift %#v", pair)
		}
		if _, duplicate := seen[pair]; duplicate {
			t.Fatalf("duplicate Numeric source %#v", pair)
		}
		seen[pair] = struct{}{}
	}
	if len(seen) != len(want) {
		t.Fatal("Numeric source inventory omitted a required relation")
	}
}

func TestContractsRejectUnavailableFactor(t *testing.T) {
	if contracts, ok := Contracts(engine.SemanticKey{}); ok || contracts != nil {
		t.Fatal("unavailable Numeric factor received coverage")
	}
}

type sourcePair struct {
	origin semanticsource.Origin
	facet  semanticsource.Facet
}

func conclusionFor(t testing.TB, contracts []coverage.CoverageContract, origin semanticsource.Origin, facet semanticsource.Facet) engine.SemanticKey {
	t.Helper()
	for _, contract := range contracts {
		if contract.Source.Origin() == origin && contract.Source.Facet() == facet {
			return contract.Conclusion
		}
	}
	t.Fatal("missing Numeric source")
	return engine.SemanticKey{}
}

func numericContractFactor(t testing.TB) engine.SemanticKey {
	t.Helper()
	var digest [32]byte
	digest[0] = 2
	factor, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		t.Fatal("Numeric factor")
	}
	return factor
}
