package contract

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/coverage"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/semanticsource"
)

func TestContractsAreDeterministicCanonicalAndDetached(t *testing.T) {
	factor := packContractFactor(t)
	first, firstOK := Contracts(factor)
	second, secondOK := Contracts(factor)
	sealed, sealedOK := coverage.SealContracts(first)
	if !firstOK || !secondOK || !sealedOK || len(first) != len(second) || len(first) != len(sealed) {
		t.Fatal("canonical Pack contracts")
	}
	for index := range first {
		if first[index] != second[index] || first[index] != sealed[index] {
			t.Fatalf("contract %d changed", index)
		}
	}
	first[0] = coverage.CoverageContract{}
	third, thirdOK := Contracts(factor)
	if !thirdOK || third[0] == (coverage.CoverageContract{}) {
		t.Fatal("Pack contracts retained caller mutation")
	}
}

func TestContractsDeriveDistinctJudgmentConclusions(t *testing.T) {
	contracts, ok := Contracts(packContractFactor(t))
	if !ok {
		t.Fatal("Pack contracts")
	}
	conclusions := make(map[engine.SemanticKey]struct{})
	for _, contract := range contracts {
		if contract.Conclusion == (engine.SemanticKey{}) {
			t.Fatal("zero Pack conclusion")
		}
		conclusions[contract.Conclusion] = struct{}{}
	}
	if len(conclusions) != 7 {
		t.Fatalf("Pack judgment conclusions = %d, want 7", len(conclusions))
	}
	if conclusionFor(t, contracts, semanticsource.OriginProgramFlowValues, 0) !=
		conclusionFor(t, contracts, semanticsource.OriginProgramFlowValues, semanticsource.FacetProgramFlowValueOccurrence) ||
		conclusionFor(t, contracts, semanticsource.OriginProgramFlowValues, 0) ==
			conclusionFor(t, contracts, semanticsource.OriginProgramFlowCall, 0) {
		t.Fatal("Pack judgments were collapsed or split by source row")
	}
}

func TestContractsInventoryExactRelevantSourcesWithoutDuplicates(t *testing.T) {
	contracts, ok := Contracts(packContractFactor(t))
	if !ok {
		t.Fatal("Pack contracts")
	}
	want := map[sourcePair]struct{}{
		{semanticsource.OriginProgramFlowValues, 0}:                                              {},
		{semanticsource.OriginProgramFlowValues, semanticsource.FacetProgramFlowValueOccurrence}: {},
		{semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageAssign}:  {},
		{semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageVararg}:  {},
		{semanticsource.OriginProgramFlowConstructors, 0}:                                        {},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowIndexGet}:     {},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowIndexSet}:     {},
		{semanticsource.OriginProgramFlowCall, 0}:                                                {},
		{semanticsource.OriginProgramFlowOutcome, 0}:                                             {},
		{semanticsource.OriginProgramFlowTransfer, 0}:                                            {},
		{semanticsource.OriginProgramFlowBody, 0}:                                                {},
		{semanticsource.OriginProgramFlowBody, semanticsource.FacetProgramFlowBodyRoots}:         {},
		{semanticsource.OriginTargetContract, 0}:                                                 {},
		{semanticsource.OriginTargetOperation, 0}:                                                {},
		{semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI}:                    {},
		{semanticsource.OriginTargetProtocol, 0}:                                                 {},
		{semanticsource.OriginLinkProjectBaseApplication, 0}:                                     {},
		{semanticsource.OriginLinkBoundary, 0}:                                                   {},
		{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleTransport}:               {},
		{semanticsource.OriginLinkHost, semanticsource.FacetLinkHostEndpointTarget}:              {},
	}
	if len(contracts) != len(want) {
		t.Fatalf("Pack source inventory = %d, want %d", len(contracts), len(want))
	}
	seen := make(map[sourcePair]struct{}, len(contracts))
	for _, contract := range contracts {
		pair := sourcePair{contract.Source.Origin(), contract.Source.Facet()}
		if _, expected := want[pair]; !expected {
			t.Fatalf("unexpected Pack source %#v", pair)
		}
		definition, defined := semanticsource.Definition(pair.origin, pair.facet)
		if !defined || definition.Token() != contract.Source {
			t.Fatalf("Pack source token drift %#v", pair)
		}
		if _, duplicate := seen[pair]; duplicate {
			t.Fatalf("duplicate Pack source %#v", pair)
		}
		seen[pair] = struct{}{}
	}
	if len(seen) != len(want) {
		t.Fatal("Pack source inventory omitted a required relation")
	}
}

func TestContractsRejectUnavailableFactor(t *testing.T) {
	if contracts, ok := Contracts(engine.SemanticKey{}); ok || contracts != nil {
		t.Fatal("unavailable Pack factor received coverage")
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
	t.Fatal("missing Pack source")
	return engine.SemanticKey{}
}

func packContractFactor(t testing.TB) engine.SemanticKey {
	t.Helper()
	var digest [32]byte
	digest[0] = 1
	factor, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		t.Fatal("Pack factor")
	}
	return factor
}
