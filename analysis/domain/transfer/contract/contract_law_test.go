package contract

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/coverage"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/semanticsource"
)

func TestContractsCoverTheCompleteSenderProof(t *testing.T) {
	factor := lawFactor(t, 1)
	contracts, ok := Contracts(factor)
	if !ok || len(contracts) != 15 {
		t.Fatalf("contracts=%d valid=%t want=15", len(contracts), ok)
	}
	seen := make(map[coverage.CoverageContract]struct{}, len(contracts))
	for _, contract := range contracts {
		if contract.Class != coverage.OwnerFactor || contract.Owner != factor {
			t.Fatal("Transfer contract escaped its Factor owner")
		}
		if _, ok := contract.Source.Identity(); !ok || !contract.Conclusion.Available() {
			t.Fatal("Transfer contract has no issued source or conclusion")
		}
		if _, duplicate := seen[contract]; duplicate {
			t.Fatal("duplicate Transfer coverage contract")
		}
		seen[contract] = struct{}{}
	}
	requireCanonical(t, contracts)
	if !hasContract(contracts, factor, semanticsource.OriginTargetOperation, 0, senderTransferRelation) ||
		!hasContract(contracts, factor, semanticsource.OriginTargetOperation, 0, senderOutcomeArms) ||
		!hasContract(contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetTransfer, senderTransferRelation) ||
		!hasContract(contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetOutcome, senderOutcomeArms) ||
		!hasContract(contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetTransferOutcome, senderOutcomeArms) ||
		!hasContract(contracts, factor, semanticsource.OriginProgramFlowTransfer, 0, senderTransferRelation) ||
		!hasContract(contracts, factor, semanticsource.OriginProgramFlowTransfer, 0, senderOutcomeArms) {
		t.Fatal("Transfer source operands were not grouped by actual Factor judgment")
	}
	other, otherOK := Contracts(lawFactor(t, 2))
	if !otherOK || len(other) != len(contracts) || other[0].Conclusion == contracts[0].Conclusion {
		t.Fatal("Transfer conclusion was not Factor-scoped")
	}
	if contracts, ok := Contracts(engine.SemanticKey{}); ok || contracts != nil {
		t.Fatal("unavailable Factor received Transfer coverage")
	}
}

func TestExactTransferRowsIssueNewConclusionIdentities(t *testing.T) {
	factor := lawFactor(t, 3)
	current, currentOK := coverage.DeriveConclusion(factor, senderTransferRelation, revision)
	retired, retiredOK := coverage.DeriveConclusion(factor, senderTransferRelation, revision-1)
	if !currentOK || !retiredOK || current == retired {
		t.Fatal("exact transfer contract did not retire its aggregate conclusion identity")
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
		t.Fatal("Transfer contracts did not seal canonically")
	}
	for index := range contracts {
		if normalized[index] != contracts[index] {
			t.Fatal("Transfer Contracts did not return canonical order")
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
