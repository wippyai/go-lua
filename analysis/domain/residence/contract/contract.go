// Package contract declares Residence's cold source-coverage obligations. It
// is not a residence key builder and never materializes a root×boundary table.
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
	callbackConclusion
	moduleConclusion
	suspensionConclusion
	resumeConclusion
	transferConclusion
)

type sourceConclusion struct {
	origin     semanticsource.Origin
	facet      semanticsource.Facet
	conclusion uint16
}

// Residence consumes exact retention boundaries and their Link root identity.
// Target ports matter only through a selected application/boundary, not as a
// pre-expanded application×operation×port topology.
var inventory = [...]sourceConclusion{
	{semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageGlobal, globalConclusion},
	{semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageWrite, storeConclusion},
	{semanticsource.OriginProgramFlowConstructors, 0, allocationConclusion},
	{semanticsource.OriginProgramFlowFunction, 0, captureConclusion},
	{semanticsource.OriginProgramFlowCall, 0, applicationConclusion},
	{semanticsource.OriginProgramFlowOutcome, 0, returnConclusion},
	{semanticsource.OriginProgramFlowTransfer, 0, transferConclusion},
	{semanticsource.OriginProgramModuleImport, 0, moduleConclusion},
	{semanticsource.OriginTargetOperation, 0, applicationConclusion},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetTransfer, transferConclusion},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallback, callbackConclusion},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetSuspension, suspensionConclusion},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetResume, resumeConclusion},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetSpawn, suspensionConclusion},
	{semanticsource.OriginTargetProtocol, 0, suspensionConclusion},
	{semanticsource.OriginLinkProjectShardMount, 0, allocationConclusion},
	{semanticsource.OriginLinkProjectBaseApplication, 0, applicationConclusion},
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
}

// Contracts returns Residence's complete cold coverage declarations for one
// declared Factor. The result retains neither roots nor boundaries and cannot
// be used to construct static freshness or runtime topology.
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
