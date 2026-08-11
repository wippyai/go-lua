package contract

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/coverage"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/semanticsource"
)

func TestContractsCoverTheCompleteModuleCacheSurfaceCanonically(t *testing.T) {
	factor := factorKey(1)
	first, firstOK := Contracts(factor)
	second, secondOK := Contracts(factor)
	if !firstOK || !secondOK || len(first) != len(obligations) || len(second) != len(first) {
		t.Fatal("module contract cardinality")
	}
	seen := make(map[coverage.CoverageContract]struct{}, len(first))
	for index, contract := range first {
		if contract.Class != coverage.OwnerFactor || contract.Owner != factor || !contract.Conclusion.Available() {
			t.Fatalf("module contract %d authority", index)
		}
		if _, duplicate := seen[contract]; duplicate {
			t.Fatalf("module contract %d duplicate", index)
		}
		seen[contract] = struct{}{}
		if second[index] != contract {
			t.Fatalf("module contract %d unstable", index)
		}
	}
	for _, row := range obligations {
		definition, ok := semanticsource.Definition(row.origin, row.facet)
		if !ok || !hasSource(first, definition.Token()) {
			t.Fatal("module source family omitted")
		}
	}
	if allConclusions(first) != 3 {
		t.Fatal("Module source rows recreated conclusion vocabulary")
	}
	for _, row := range [...]obligation{
		{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitGeneration, cacheTransition},
		{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitOutcome, cacheResult},
		{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitOutcome, cachePublication},
		{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitTerminal, cacheTransition},
	} {
		definition, ok := semanticsource.Definition(row.origin, row.facet)
		if !ok || !hasConclusion(first, definition.Token(), factor, row.ordinal) {
			t.Fatalf("Module lifecycle operand omitted: %#v", row)
		}
	}
	canonical, canonicalOK := coverage.SealContracts(first)
	if !canonicalOK || len(canonical) != len(first) {
		t.Fatal("Module contract was not canonically sealed")
	}
	for index := range first {
		if canonical[index] != first[index] {
			t.Fatal("Module contract order drifted")
		}
	}
}

func TestContractsKeepDistinctBoundaryConclusions(t *testing.T) {
	contracts, contractsOK := Contracts(factorKey(2))
	definition, defined := semanticsource.Definition(semanticsource.OriginLinkBoundary, 0)
	outcomes, outcomesDefined := semanticsource.Definition(semanticsource.OriginProgramFlowOutcome, 0)
	if !contractsOK || !defined || !outcomesDefined {
		t.Fatal("Module transition definitions")
	}
	conclusions := make(map[engine.SemanticKey]struct{})
	for _, contract := range contracts {
		if contract.Source == definition.Token() {
			conclusions[contract.Conclusion] = struct{}{}
		}
	}
	if len(conclusions) != 1 {
		t.Fatal("boundary transport conclusions collapsed")
	}
	if countConclusions(contracts, outcomes.Token()) != 2 {
		t.Fatal("ready throw yield and cancel conclusions collapsed")
	}
}

func TestContractsRejectInvalidFactor(t *testing.T) {
	if contracts, ok := Contracts(engine.SemanticKey{}); ok || contracts != nil {
		t.Fatal("invalid Module Factor produced a partial contract")
	}
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

func factorKey(seed byte) engine.SemanticKey {
	var digest [32]byte
	digest[0] = seed
	key, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		panic("test Factor key")
	}
	return key
}
