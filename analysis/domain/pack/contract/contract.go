// Package contract declares Pack's cold source-coverage obligations.
//
// The declarations are deliberately only Source × Pack-judgment pairs. They
// do not name Rules, recreate Pack rows, or select any Target/Link instance.
package contract

import (
	"github.com/wippyai/go-lua/analysis/coverage"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/semanticsource"
)

const contractRevision uint16 = 2

// These are the owner-local Pack judgments represented by the generated row
// table. The production composition currently declares only the Source Rule;
// the remaining conclusions stay in the detached inventory but are not
// admitted by BuildPlan until their exact Rules exist.
const (
	packSchemaConclusion    uint16 = 1
	packSourceConclusion    uint16 = 2
	packBindConclusion      uint16 = 3
	packSpliceConclusion    uint16 = 4
	packEntryConclusion     uint16 = 5
	packOutcomeConclusion   uint16 = 6
	packTransportConclusion uint16 = 7
)

// PlanBindings names Pack's one declared source Rule.
type PlanBindings struct {
	Source engine.SemanticKey
}

// CoveragePlan is Pack's detached treatment plan.
type CoveragePlan struct {
	Contracts []coverage.CoverageContract
	Rules     []coverage.RulePlan
}

// Contracts returns Pack's canonical source coverage claims for factor.
//
// The private ordinals identify judgments rather than implementation Rules:
// schema, authored lists, binding/vararg lists, list operations, call
// adjustment, outcome lists, and boundary transport.  The returned slice is
// freshly allocated and canonically sealed.
func Contracts(factor engine.SemanticKey) ([]coverage.CoverageContract, bool) {
	if !factor.Available() {
		return nil, false
	}
	rows := []row{
		{semanticsource.OriginProgramFlowValues, 0, packSourceConclusion},
		{semanticsource.OriginProgramFlowValues, semanticsource.FacetProgramFlowValueOccurrence, packSourceConclusion},
		{semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageAssign, packBindConclusion},
		{semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageVararg, packBindConclusion},
		{semanticsource.OriginProgramFlowConstructors, 0, packSpliceConclusion},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowIndexGet, packSpliceConclusion},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowIndexSet, packSpliceConclusion},
		{semanticsource.OriginProgramFlowCall, 0, packEntryConclusion},
		{semanticsource.OriginProgramFlowOutcome, 0, packOutcomeConclusion},
		{semanticsource.OriginProgramFlowTransfer, 0, packTransportConclusion},
		{semanticsource.OriginProgramFlowBody, 0, packSchemaConclusion},
		{semanticsource.OriginProgramFlowBody, semanticsource.FacetProgramFlowBodyRoots, packSchemaConclusion},
		{semanticsource.OriginTargetContract, 0, packEntryConclusion},
		{semanticsource.OriginTargetOperation, 0, packEntryConclusion},
		{semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI, packEntryConclusion},
		{semanticsource.OriginTargetProtocol, 0, packOutcomeConclusion},
		{semanticsource.OriginLinkProjectBaseApplication, 0, packEntryConclusion},
		{semanticsource.OriginLinkBoundary, 0, packTransportConclusion},
		{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleTransport, packTransportConclusion},
		{semanticsource.OriginLinkHost, semanticsource.FacetLinkHostEndpointTarget, packTransportConclusion},
	}
	contracts := make([]coverage.CoverageContract, 0, len(rows))
	for _, row := range rows {
		definition, present := semanticsource.Definition(row.origin, row.facet)
		conclusion, derived := coverage.DeriveConclusion(factor, row.ordinal, contractRevision)
		if !present || !derived {
			return nil, false
		}
		contracts = append(contracts, coverage.CoverageContract{
			Source: definition.Token(), Class: coverage.OwnerFactor, Owner: factor, Conclusion: conclusion,
		})
	}
	return coverage.SealContracts(contracts)
}

// BuildPlan admits only Pack's authored-list Source conclusion to the one
// existing source Rule. Bind, Splice, BodyEntry/BodyReturn, and transport
// conclusions remain outside this plan until their exact Rules are declared;
// production coverage does not overclaim them here.
func BuildPlan(factor engine.SemanticKey, bindings PlanBindings) (CoveragePlan, bool) {
	if !factor.Available() || !bindings.Source.Available() {
		return CoveragePlan{}, false
	}
	contracts, ok := Contracts(factor)
	if !ok {
		return CoveragePlan{}, false
	}
	sourceConclusion, sourceConclusionOK := coverage.DeriveConclusion(factor, packSourceConclusion, contractRevision)
	if !sourceConclusionOK {
		return CoveragePlan{}, false
	}
	supported := make([]coverage.CoverageContract, 0, len(contracts))
	for _, contract := range contracts {
		if contract.Conclusion != sourceConclusion {
			continue
		}
		supported = append(supported, contract)
	}
	if len(supported) == 0 {
		return CoveragePlan{}, false
	}
	covers := make([]coverage.Requirement, 0, len(supported))
	for _, contract := range supported {
		covers = append(covers, contract.Requirement())
	}
	covers, ok = coverage.SealRequirements(covers)
	if !ok {
		return CoveragePlan{}, false
	}
	return CoveragePlan{Contracts: supported, Rules: []coverage.RulePlan{{Semantic: bindings.Source, Covers: covers}}}, true
}

type row struct {
	origin  semanticsource.Origin
	facet   semanticsource.Facet
	ordinal uint16
}
