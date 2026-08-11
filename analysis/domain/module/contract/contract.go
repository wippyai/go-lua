// Package contract declares Module's cold source-coverage obligations.
//
// It deliberately contains no Rule identity, source row payload, registry, or
// execution wiring.  The rows state only which generated source families can
// require a distinct Module-cache conclusion.  Rule and query plans discharge
// these obligations later at the sealed composition cut.
package contract

import (
	"github.com/wippyai/go-lua/analysis/coverage"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/semanticsource"
)

const revision uint16 = 2

const (
	cacheTransition uint16 = iota + 1
	cacheResult
	cachePublication
)

// obligation is intentionally numeric: source-vocabulary names, rule IDs,
// and package paths are not semantic contract inputs.
type obligation struct {
	origin  semanticsource.Origin
	facet   semanticsource.Facet
	ordinal uint16
}

// obligations covers the complete Module cache contract.  Ordinals identify
// only the three Module judgments, never source rows: every Program, Target,
// and Link operand contributing to one cache judgment shares its conclusion.
var obligations = [...]obligation{
	{semanticsource.OriginProgramFlowCall, 0, cacheTransition},
	{semanticsource.OriginProgramFlowCall, 0, cacheResult},
	{semanticsource.OriginProgramFlowOutcome, 0, cacheTransition},
	{semanticsource.OriginProgramFlowOutcome, 0, cachePublication},
	{semanticsource.OriginProgramModuleImport, 0, cacheTransition},
	{semanticsource.OriginProgramModuleImport, semanticsource.FacetProgramModuleRequest, cacheTransition},
	{semanticsource.OriginTargetContract, 0, cacheTransition},
	{semanticsource.OriginTargetOperation, 0, cacheTransition},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetResume, cacheTransition},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetSpawn, cacheTransition},
	{semanticsource.OriginLinkProjectBaseApplication, 0, cacheTransition},
	{semanticsource.OriginLinkBoundary, 0, cacheTransition},
	{semanticsource.OriginLinkModule, 0, cacheTransition},
	{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleCache, cacheTransition},
	{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleRepresentative, cacheTransition},
	{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleTransport, cacheTransition},
	{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleAnalysisRoot, cacheTransition},
	// Cache transition, successful publication, and failed restoration consume
	// these three exact Link-owned module-init correspondences, rather than
	// treating callback metadata as a cache transition.
	{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitGeneration, cacheTransition},
	{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitOutcome, cacheResult},
	{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitOutcome, cachePublication},
	{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitTerminal, cacheTransition},
	{semanticsource.OriginLinkHost, semanticsource.FacetLinkHostExposure, cacheResult},
	{semanticsource.OriginLinkHost, semanticsource.FacetLinkHostExposure, cachePublication},
}

// Contracts returns Module's complete canonical Factor-owned source contract.
// An unavailable Factor cannot own a conclusion, so it returns no partial
// contract.  The result is detached and canonically ordered.
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
