// Package contract declares Typestate's cold semantic coverage denominator.
// It is not a Rule registry and it does not describe the current carrier.
package contract

import (
	"github.com/wippyai/go-lua/analysis/coverage"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/semanticsource"
)

const revision uint16 = 2

const (
	protocolState uint16 = iota + 1
	holderRelation
	cleanupDuty
)

// Contracts declares Typestate's three observable Factor judgments. Each
// judgment collects its jointly necessary source operands; it deliberately
// does not turn source phases into fake conclusion identities.
func Contracts(factor engine.SemanticKey) ([]coverage.CoverageContract, bool) {
	if !factor.Available() {
		return nil, false
	}
	contracts := make([]coverage.CoverageContract, 0, 24)
	if !appendContract(&contracts, factor, semanticsource.OriginProgramFlowCall, 0, protocolState) ||
		!appendContract(&contracts, factor, semanticsource.OriginLinkBoundary, 0, protocolState) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowOutcome, 0, protocolState) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, 0, protocolState) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetSubedge, protocolState) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetProtocol, 0, protocolState) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetOpaque, protocolState) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallback, holderRelation) ||
		// Callback owns retention; CallbackRelease is the separate causal
		// holder-consumption relation.  The former aggregate lifecycle source
		// hid that distinction.
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackRelease, holderRelation) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetResume, holderRelation) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetSpawn, holderRelation) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowStorage, 0, holderRelation) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowTransfer, 0, holderRelation) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetOpaque, holderRelation) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetOperationEffect, holderRelation) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackEffect, holderRelation) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowOutcome, 0, cleanupDuty) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, 0, cleanupDuty) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackRelease, cleanupDuty) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetResume, cleanupDuty) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowTransfer, 0, cleanupDuty) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetOpaque, cleanupDuty) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetOperationEffect, cleanupDuty) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackEffect, cleanupDuty) {
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
