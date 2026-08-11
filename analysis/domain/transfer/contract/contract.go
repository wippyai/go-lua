// Package contract declares Transfer's cold semantic coverage denominator.
// It is not a Rule registry and it does not describe the current carrier.
package contract

import (
	"github.com/wippyai/go-lua/analysis/coverage"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/semanticsource"
)

// revision changes with the exact Target transfer descriptor inventory.  A
// conclusion from the retired aggregate effect source is never compatible
// with this transfer-specific contract.
const revision uint16 = 2

const (
	senderTransferRelation uint16 = iota + 1
	senderOutcomeArms
)

// Contracts declares Transfer's two observable Factor judgments: the
// sender-side transfer relation and its delivery/rejection outcome arms.
// Source phases remain operands of those judgments, never conclusions.
func Contracts(factor engine.SemanticKey) ([]coverage.CoverageContract, bool) {
	if !factor.Available() {
		return nil, false
	}
	contracts := make([]coverage.CoverageContract, 0, 15)
	if !appendContract(&contracts, factor, semanticsource.OriginProgramFlowCall, 0, senderTransferRelation) ||
		!appendContract(&contracts, factor, semanticsource.OriginLinkBoundary, 0, senderTransferRelation) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowTransfer, 0, senderTransferRelation) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowValues, 0, senderTransferRelation) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowValues, semanticsource.FacetProgramFlowValueOccurrence, senderTransferRelation) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, 0, senderTransferRelation) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetTransfer, senderTransferRelation) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetSubedge, senderTransferRelation) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetOpaque, senderTransferRelation) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowOutcome, 0, senderOutcomeArms) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowTransfer, 0, senderOutcomeArms) ||
		!appendContract(&contracts, factor, semanticsource.OriginLinkBoundary, 0, senderOutcomeArms) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, 0, senderOutcomeArms) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetOutcome, senderOutcomeArms) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetTransferOutcome, senderOutcomeArms) {
		return nil, false
	}
	return coverage.SealContracts(contracts)
}

func appendContract(dst *[]coverage.CoverageContract, factor engine.SemanticKey, origin semanticsource.Origin, facet semanticsource.Facet, ordinal uint16) bool {
	definition, found := semanticsource.Definition(origin, facet)
	conclusion, derived := coverage.DeriveConclusion(factor, ordinal, revision)
	if !found || !derived {
		return false
	}
	*dst = append(*dst, coverage.CoverageContract{
		Source: definition.Token(), Class: coverage.OwnerFactor, Owner: factor, Conclusion: conclusion,
	})
	return true
}
