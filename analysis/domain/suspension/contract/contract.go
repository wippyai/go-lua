// Package contract declares Suspension's cold source-coverage obligations.
// It is not a continuation registry and it has no execution authority.
package contract

import (
	"github.com/wippyai/go-lua/analysis/coverage"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/semanticsource"
)

const revision uint16 = 2

const (
	retainedProjection uint16 = iota + 1
	generationLifecycle
	reentryConsumption
)

type obligation struct {
	origin  semanticsource.Origin
	facet   semanticsource.Facet
	ordinal uint16
}

// obligations names the full planned lifecycle surface, rather than only the
// ModuleInit rows currently executable.  Its ordinals are the three actual
// Suspension judgments; they are not a parallel source-row vocabulary.
var obligations = [...]obligation{
	{semanticsource.OriginProgramFlowValues, semanticsource.FacetProgramFlowValueOccurrence, retainedProjection},
	{semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageCell, retainedProjection},
	{semanticsource.OriginProgramFlowCall, 0, generationLifecycle},
	{semanticsource.OriginProgramFlowControl, 0, retainedProjection},
	{semanticsource.OriginProgramFlowOutcome, 0, generationLifecycle},
	{semanticsource.OriginProgramFlowOutcome, 0, reentryConsumption},
	{semanticsource.OriginTargetOperation, 0, generationLifecycle},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallback, retainedProjection},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallback, generationLifecycle},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetResume, reentryConsumption},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetSpawn, generationLifecycle},
	{semanticsource.OriginLinkProjectBaseApplication, 0, generationLifecycle},
	{semanticsource.OriginLinkBoundary, 0, retainedProjection},
	{semanticsource.OriginLinkBoundary, 0, reentryConsumption},
	{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleTransport, retainedProjection},
	{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleTransport, generationLifecycle},
	// The three module-init rows are the exact operands of Suspension's
	// init/completion/cancel rules; LinkModuleTransport alone is not a
	// lifecycle relation.
	{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitGeneration, generationLifecycle},
	{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitOutcome, generationLifecycle},
	{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitTerminal, reentryConsumption},
}

// Contracts returns Suspension's complete canonical Factor-owned source
// contract.  It fails closed for an unavailable Factor and never emits a
// subset that could conceal a missing lifecycle owner.
func Contracts(factor engine.SemanticKey) ([]coverage.CoverageContract, bool) {
	if !factor.Available() {
		return nil, false
	}
	contracts := make([]coverage.CoverageContract, 0, len(obligations))
	for _, row := range obligations {
		definition, found := semanticsource.Definition(row.origin, row.facet)
		conclusion, derived := coverage.DeriveConclusion(factor, row.ordinal, revision)
		if !found || !derived {
			return nil, false
		}
		contracts = append(contracts, coverage.CoverageContract{
			Source: definition.Token(), Class: coverage.OwnerFactor, Owner: factor, Conclusion: conclusion,
		})
	}
	return coverage.SealContracts(contracts)
}
