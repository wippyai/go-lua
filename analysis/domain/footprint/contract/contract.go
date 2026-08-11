// Package contract declares Footprint's cold semantic coverage denominator.
// It is not a Rule registry and it does not describe the current carrier.
package contract

import (
	"github.com/wippyai/go-lua/analysis/coverage"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/semanticsource"
)

const revision uint16 = 2

const (
	allocationObjectGraph uint16 = iota + 1
	boundUncertainty
)

// Contracts declares Footprint's two observable Factor judgments: allocation
// and object-graph structure, plus bounds/uncertainty. The current
// allocation-only carrier is intentionally not used to shrink this denominator.
func Contracts(factor engine.SemanticKey) ([]coverage.CoverageContract, bool) {
	if !factor.Available() {
		return nil, false
	}
	contracts := make([]coverage.CoverageContract, 0, 33)
	if !appendContract(&contracts, factor, semanticsource.OriginProgramFlowLiterals, 0, allocationObjectGraph) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowConstructors, 0, allocationObjectGraph) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowFunction, 0, allocationObjectGraph) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowStorage, 0, allocationObjectGraph) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageCell, allocationObjectGraph) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageRead, allocationObjectGraph) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageAssign, allocationObjectGraph) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageWrite, allocationObjectGraph) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowLens, 0, allocationObjectGraph) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowIndexGet, allocationObjectGraph) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowIndexSet, allocationObjectGraph) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowCall, 0, allocationObjectGraph) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowOutcome, 0, allocationObjectGraph) ||
		!appendContract(&contracts, factor, semanticsource.OriginLinkBoundary, 0, allocationObjectGraph) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, 0, allocationObjectGraph) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetOperationEffect, allocationObjectGraph) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackEffect, allocationObjectGraph) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetSubedge, allocationObjectGraph) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetOpaque, allocationObjectGraph) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetBoot, 0, allocationObjectGraph) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowStorage, 0, boundUncertainty) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageAssign, boundUncertainty) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageWrite, boundUncertainty) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowLens, 0, boundUncertainty) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowUnaryNumeric, boundUncertainty) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowLength, boundUncertainty) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowArithmetic, boundUncertainty) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowBitwise, boundUncertainty) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowConcat, boundUncertainty) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowIndexGet, boundUncertainty) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowIndexSet, boundUncertainty) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowControl, 0, boundUncertainty) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowTransfer, 0, boundUncertainty) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowCall, 0, boundUncertainty) ||
		!appendContract(&contracts, factor, semanticsource.OriginProgramFlowOutcome, 0, boundUncertainty) ||
		!appendContract(&contracts, factor, semanticsource.OriginLinkBoundary, 0, boundUncertainty) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, 0, boundUncertainty) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetOperationEffect, boundUncertainty) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackEffect, boundUncertainty) ||
		!appendContract(&contracts, factor, semanticsource.OriginTargetOperation, semanticsource.FacetTargetOpaque, boundUncertainty) {
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
