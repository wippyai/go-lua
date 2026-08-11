// Package contract declares Ownership's cold source-coverage obligations. It
// has no Program/Link scan and does not form an ownership origin or a duty.
package contract

import (
	"github.com/wippyai/go-lua/analysis/coverage"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/semanticsource"
)

const revision uint16 = 2

const (
	allocationConclusion uint16 = iota + 1
	captureConclusion
	storeConclusion
	returnConclusion
	globalConclusion
	applicationConclusion
	inputConclusion
	callbackConclusion
	moduleConclusion
	suspensionConclusion
	resumeConclusion
	transferConclusion
	bootConclusion
	hostConclusion
)

type sourceConclusion struct {
	origin     semanticsource.Origin
	facet      semanticsource.Facet
	conclusion uint16
}

// This is the source denominator for Ownership's exact duties: allocation
// roots, retained Program boundaries, selected operation ports, module and
// provider boundaries, and continuation/transfer dispositions. It deliberately
// has no synthetic allocation×application or static-freshness relation.
var inventory = [...]sourceConclusion{
	{semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageGlobal, globalConclusion},
	{semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageWrite, storeConclusion},
	{semanticsource.OriginProgramFlowConstructors, 0, allocationConclusion},
	{semanticsource.OriginProgramFlowFunction, 0, captureConclusion},
	{semanticsource.OriginProgramFlowOutcome, 0, returnConclusion},
	{semanticsource.OriginProgramFlowTransfer, 0, transferConclusion},
	{semanticsource.OriginProgramModuleImport, 0, moduleConclusion},
	{semanticsource.OriginTargetOperation, 0, applicationConclusion},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI, inputConclusion},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetOutcome, returnConclusion},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetTransfer, transferConclusion},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetTransferOutcome, transferConclusion},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallback, callbackConclusion},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetBinding, inputConclusion},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetSuspension, suspensionConclusion},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetResume, resumeConclusion},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetSpawn, suspensionConclusion},
	{semanticsource.OriginTargetProtocol, 0, suspensionConclusion},
	{semanticsource.OriginTargetBoot, 0, bootConclusion},
	{semanticsource.OriginLinkProjectShardMount, 0, allocationConclusion},
	{semanticsource.OriginLinkProjectBaseApplication, 0, applicationConclusion},
	{semanticsource.OriginLinkBoundary, 0, returnConclusion},
	{semanticsource.OriginLinkBoundary, 0, applicationConclusion},
	{semanticsource.OriginLinkBoundary, 0, callbackConclusion},
	{semanticsource.OriginLinkBoundary, 0, suspensionConclusion},
	{semanticsource.OriginLinkBoundary, 0, resumeConclusion},
	{semanticsource.OriginLinkBoundary, 0, transferConclusion},
	{semanticsource.OriginLinkModule, 0, moduleConclusion},
	{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleCache, moduleConclusion},
	{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleRepresentative, moduleConclusion},
	{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleTransport, moduleConclusion},
	{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleAnalysisRoot, moduleConclusion},
	{semanticsource.OriginLinkHost, 0, hostConclusion},
	{semanticsource.OriginLinkHost, semanticsource.FacetLinkHostExposure, hostConclusion},
	{semanticsource.OriginLinkHost, semanticsource.FacetLinkHostBoot, bootConclusion},
}

// Contracts returns Ownership's complete cold coverage declarations for one
// declared Factor. Its private conclusion ordinals remain Factor-scoped and
// cannot become source names, Rule IDs, or a second ownership authority.
func Contracts(factor engine.SemanticKey) ([]coverage.CoverageContract, bool) {
	if !factor.Available() {
		return nil, false
	}
	contracts := make([]coverage.CoverageContract, 0, len(inventory))
	for _, item := range inventory {
		definition, found := semanticsource.Definition(item.origin, item.facet)
		conclusion, derived := coverage.DeriveConclusion(factor, item.conclusion, revision)
		if !found || !derived {
			return nil, false
		}
		contracts = append(contracts, coverage.CoverageContract{
			Source: definition.Token(), Class: coverage.OwnerFactor, Owner: factor, Conclusion: conclusion,
		})
	}
	return coverage.SealContracts(contracts)
}
