package contract

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/coverage"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/semanticsource"
)

func TestContractsCoverAllocationMutationLoopAndOperation(t *testing.T) {
	factor := lawFactor(t, 1)
	contracts, ok := Contracts(factor)
	if !ok || len(contracts) != 40 {
		t.Fatalf("contracts=%d valid=%t want=40", len(contracts), ok)
	}
	seen := make(map[coverage.CoverageContract]struct{}, len(contracts))
	for _, contract := range contracts {
		if contract.Class != coverage.OwnerFactor || contract.Owner != factor {
			t.Fatal("Footprint contract escaped its Factor owner")
		}
		if _, ok := contract.Source.Identity(); !ok || !contract.Conclusion.Available() {
			t.Fatal("Footprint contract has no issued source or conclusion")
		}
		if _, duplicate := seen[contract]; duplicate {
			t.Fatal("duplicate Footprint coverage contract")
		}
		seen[contract] = struct{}{}
	}
	requireCanonical(t, contracts)
	if !hasContract(contracts, factor, semanticsource.OriginProgramFlowConstructors, 0, allocationObjectGraph) ||
		!hasContract(contracts, factor, semanticsource.OriginProgramFlowTransfer, 0, boundUncertainty) ||
		!hasContract(contracts, factor, semanticsource.OriginTargetOperation, 0, allocationObjectGraph) ||
		!hasContract(contracts, factor, semanticsource.OriginTargetOperation, 0, boundUncertainty) ||
		!hasContract(contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetOperationEffect, allocationObjectGraph) ||
		!hasContract(contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackEffect, boundUncertainty) {
		t.Fatal("Footprint source operands were not grouped by actual Factor judgment")
	}
	if again, againOK := Contracts(factor); !againOK || len(again) != len(contracts) {
		t.Fatal("Footprint coverage contract was not deterministic")
	}
	if contracts, ok := Contracts(engine.SemanticKey{}); ok || contracts != nil {
		t.Fatal("unavailable Factor received Footprint coverage")
	}
}

func requireCanonical(t testing.TB, contracts []coverage.CoverageContract) {
	t.Helper()
	permuted := append([]coverage.CoverageContract(nil), contracts...)
	for left, right := 0, len(permuted)-1; left < right; left, right = left+1, right-1 {
		permuted[left], permuted[right] = permuted[right], permuted[left]
	}
	normalized, ok := coverage.SealContracts(permuted)
	if !ok || len(normalized) != len(contracts) {
		t.Fatal("Footprint contracts did not seal canonically")
	}
	for index := range contracts {
		if normalized[index] != contracts[index] {
			t.Fatal("Footprint Contracts did not return canonical order")
		}
	}
}

func hasContract(contracts []coverage.CoverageContract, factor engine.SemanticKey, origin semanticsource.Origin, facet semanticsource.Facet, ordinal uint16) bool {
	definition, found := semanticsource.Definition(origin, facet)
	conclusion, derived := coverage.DeriveConclusion(factor, ordinal, revision)
	if !found || !derived {
		return false
	}
	want := coverage.CoverageContract{Source: definition.Token(), Class: coverage.OwnerFactor, Owner: factor, Conclusion: conclusion}
	for _, contract := range contracts {
		if contract == want {
			return true
		}
	}
	return false
}

func lawFactor(t testing.TB, word byte) engine.SemanticKey {
	t.Helper()
	digest := [32]byte{word}
	factor, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		t.Fatal("law Factor")
	}
	return factor
}
