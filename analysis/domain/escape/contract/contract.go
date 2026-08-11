// Package contract declares Escape's cold source-coverage obligations.  It
// does not inspect Program rows, bind a Link, or create an Escape coordinate.
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
	callConclusion
	inputConclusion
	outcomeConclusion
	callbackConclusion
	moduleConclusion
	transferConclusion
	suspensionConclusion
	resumeConclusion
)

type sourceConclusion struct {
	origin     semanticsource.Origin
	facet      semanticsource.Facet
	conclusion uint16
}

// The inventory follows Escape's boundary algebra, not its old schema-building
// loops.  In particular, an operation is useful only with its exact Link
// application/boundary identity; this list is not a static operation topology.
var inventory = [...]sourceConclusion{
	{semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageGlobal, globalConclusion},
	{semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageWrite, storeConclusion},
	{semanticsource.OriginProgramFlowConstructors, 0, allocationConclusion},
	{semanticsource.OriginProgramFlowFunction, 0, captureConclusion},
	{semanticsource.OriginProgramFlowCall, 0, callConclusion},
	{semanticsource.OriginProgramFlowOutcome, 0, returnConclusion},
	{semanticsource.OriginProgramFlowTransfer, 0, transferConclusion},
	{semanticsource.OriginProgramModuleImport, 0, moduleConclusion},
	{semanticsource.OriginTargetOperation, 0, callConclusion},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI, inputConclusion},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetOutcome, outcomeConclusion},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetTransfer, transferConclusion},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetTransferOutcome, transferConclusion},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallback, callbackConclusion},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetSuspension, suspensionConclusion},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetResume, resumeConclusion},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetSpawn, suspensionConclusion},
	{semanticsource.OriginTargetProtocol, 0, suspensionConclusion},
	{semanticsource.OriginLinkProjectShardMount, 0, allocationConclusion},
	{semanticsource.OriginLinkProjectBaseApplication, 0, callConclusion},
	{semanticsource.OriginLinkBoundary, 0, callConclusion},
	{semanticsource.OriginLinkBoundary, 0, transferConclusion},
	{semanticsource.OriginLinkBoundary, 0, suspensionConclusion},
	{semanticsource.OriginLinkModule, 0, moduleConclusion},
	{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleCache, moduleConclusion},
	{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleTransport, moduleConclusion},
}

// Contracts returns Escape's complete cold coverage declarations for one
// declared Factor. The returned slice is caller-owned; no registry, Rule ID,
// source name, Program row, or runtime topology is retained here.
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
