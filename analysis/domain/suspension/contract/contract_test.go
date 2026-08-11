package contract

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/coverage"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/semanticsource"
)

func TestContractsCoverTheCompleteSuspensionLifecycleCanonically(t *testing.T) {
	factor := factorKey(1)
	first, firstOK := Contracts(factor)
	second, secondOK := Contracts(factor)
	if !firstOK || !secondOK || len(first) != len(obligations) || len(second) != len(first) {
		t.Fatal("Suspension contract cardinality")
	}
	seen := make(map[coverage.CoverageContract]struct{}, len(first))
	for index, contract := range first {
		if contract.Class != coverage.OwnerFactor || contract.Owner != factor || !contract.Conclusion.Available() {
			t.Fatalf("Suspension contract %d authority", index)
		}
		if _, duplicate := seen[contract]; duplicate {
			t.Fatalf("Suspension contract %d duplicate", index)
		}
		seen[contract] = struct{}{}
		if second[index] != contract {
			t.Fatalf("Suspension contract %d unstable", index)
		}
	}
	for _, row := range obligations {
		definition, ok := semanticsource.Definition(row.origin, row.facet)
		if !ok || !hasSource(first, definition.Token()) {
			t.Fatal("Suspension source family omitted")
		}
	}
	if allConclusions(first) != 3 {
		t.Fatal("Suspension source rows recreated conclusion vocabulary")
	}
	for _, row := range [...]obligation{
		{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitGeneration, generationLifecycle},
		{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitOutcome, generationLifecycle},
		{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitTerminal, reentryConsumption},
	} {
		definition, ok := semanticsource.Definition(row.origin, row.facet)
		if !ok || !hasConclusion(first, definition.Token(), factor, row.ordinal) {
			t.Fatalf("Suspension lifecycle operand omitted: %#v", row)
		}
	}
	canonical, canonicalOK := coverage.SealContracts(first)
	if !canonicalOK || len(canonical) != len(first) {
		t.Fatal("Suspension contract was not canonically sealed")
	}
	for index := range first {
		if canonical[index] != first[index] {
			t.Fatal("Suspension contract order drifted")
		}
	}
}

func TestContractsKeepYieldTerminalAndResumeConclusionsDistinct(t *testing.T) {
	contracts, contractsOK := Contracts(factorKey(2))
	outcome, outcomeOK := semanticsource.Definition(semanticsource.OriginProgramFlowOutcome, 0)
	boundary, boundaryOK := semanticsource.Definition(semanticsource.OriginLinkBoundary, 0)
	if !contractsOK || !outcomeOK || !boundaryOK {
		t.Fatal("lifecycle definitions")
	}
	if countConclusions(contracts, outcome.Token()) != 2 {
		t.Fatal("yield and terminal conclusions collapsed")
	}
	if countConclusions(contracts, boundary.Token()) != 2 {
		t.Fatal("resume and re-entry conclusions collapsed")
	}
}

func TestContractsRejectInvalidFactor(t *testing.T) {
	if contracts, ok := Contracts(engine.SemanticKey{}); ok || contracts != nil {
		t.Fatal("invalid Suspension Factor produced a partial contract")
	}
}

func countConclusions(contracts []coverage.CoverageContract, source semanticsource.Token) int {
	conclusions := make(map[engine.SemanticKey]struct{})
	for _, contract := range contracts {
		if contract.Source == source {
			conclusions[contract.Conclusion] = struct{}{}
		}
	}
	return len(conclusions)
}

func allConclusions(contracts []coverage.CoverageContract) int {
	conclusions := make(map[engine.SemanticKey]struct{})
	for _, contract := range contracts {
		conclusions[contract.Conclusion] = struct{}{}
	}
	return len(conclusions)
}

func hasSource(contracts []coverage.CoverageContract, source semanticsource.Token) bool {
	for _, contract := range contracts {
		if contract.Source == source {
			return true
		}
	}
	return false
}

func hasConclusion(contracts []coverage.CoverageContract, source semanticsource.Token, factor engine.SemanticKey, ordinal uint16) bool {
	conclusion, ok := coverage.DeriveConclusion(factor, ordinal, revision)
	if !ok {
		return false
	}
	for _, contract := range contracts {
		if contract.Source == source && contract.Conclusion == conclusion {
			return true
		}
	}
	return false
}

func factorKey(seed byte) engine.SemanticKey {
	var digest [32]byte
	digest[0] = seed
	key, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		panic("test Factor key")
	}
	return key
}
