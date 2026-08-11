// Package contract declares Call's cold source-to-conclusion obligations.
//
// Availability in Link is not execution.  The dispatch and outcome obligations
// below deliberately remain unsatisfied until a future activation/outcome
// treatment proves one exact selected execution.
package contract

import (
	"github.com/wippyai/go-lua/analysis/coverage"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/semanticsource"
)

// revision changes with the exact cold source denominator.  In particular,
// operator dispatch is no longer one erased "arm" bucket: each executable
// metamethod family is an independently auditable source.
const revision uint16 = 2

// conclusion is Call's private closed vocabulary.  Its ordinals are not Rule
// IDs and cannot be used as a registry or a dynamic dispatch vocabulary.
type conclusion uint16

const (
	candidate conclusion = iota + 1
	dispatch
	outcome
	callbackResume
)

type row struct {
	origin     semanticsource.Origin
	facet      semanticsource.Facet
	conclusion conclusion
}

// PlanBindings supplies the existing Call Rule identity to the owner-local
// source/conclusion plan.  The contract package does not mint or discover
// semantic identities.
type PlanBindings struct {
	Dispatch engine.SemanticKey
}

// CoveragePlan is Call's detached treatment plan.
type CoveragePlan struct {
	Contracts []coverage.CoverageContract
	Rules     []coverage.RulePlan
}

// rows are source-token ordered.  Target/Link availability rows share Call's
// dispatch conclusion; they are operands of the future selected-call Rule,
// not independent carrier facts or a candidate-product claim.
var rows = [...]row{
	{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowUnaryNumeric, dispatch},
	{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowLength, dispatch},
	{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowArithmetic, dispatch},
	{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowBitwise, dispatch},
	{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowConcat, dispatch},
	{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowEquality, dispatch},
	{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowOrder, dispatch},
	{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowIndexGet, dispatch},
	{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowIndexSet, dispatch},
	{semanticsource.OriginProgramFlowFunction, 0, candidate},
	{semanticsource.OriginProgramFlowCall, 0, dispatch},
	{semanticsource.OriginProgramFlowOutcome, 0, outcome},
	{semanticsource.OriginTargetOperation, 0, dispatch},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI, dispatch},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallback, callbackResume},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetResume, callbackResume},
	{semanticsource.OriginLinkProjectBaseApplication, 0, dispatch},
	{semanticsource.OriginLinkBoundary, 0, dispatch},
	{semanticsource.OriginLinkHost, semanticsource.FacetLinkHostEndpointTarget, candidate},
}

// Contracts returns Call's complete cold obligations for one already-declared
// Factor semantic identity.  The declaration is deterministic and contains no
// Rule identity, candidate product, selected tuple, or runtime fact.
func Contracts(factor engine.SemanticKey) ([]coverage.CoverageContract, bool) {
	if !factor.Available() {
		return nil, false
	}
	contracts := make([]coverage.CoverageContract, 0, len(rows))
	for _, declared := range rows {
		definition, found := semanticsource.Definition(declared.origin, declared.facet)
		conclusion, derived := coverage.DeriveConclusion(factor, uint16(declared.conclusion), revision)
		if !found || !derived {
			return nil, false
		}
		contracts = append(contracts, coverage.CoverageContract{
			Source:     definition.Token(),
			Class:      coverage.OwnerFactor,
			Owner:      factor,
			Conclusion: conclusion,
		})
	}
	return coverage.SealContracts(contracts)
}

// BuildPlan admits only the dispatch conclusion to the declared dispatch
// Rule. Candidate, outcome, and callback/resume conclusions remain outside
// this plan until their exact Rules exist; production coverage does not assign
// them to dispatch by convenience.
func BuildPlan(factor engine.SemanticKey, bindings PlanBindings) (CoveragePlan, bool) {
	if !factor.Available() || !bindings.Dispatch.Available() {
		return CoveragePlan{}, false
	}
	contracts, ok := Contracts(factor)
	if !ok {
		return CoveragePlan{}, false
	}
	dispatchConclusion, dispatchOK := coverage.DeriveConclusion(factor, uint16(dispatch), revision)
	if !dispatchOK {
		return CoveragePlan{}, false
	}
	supported := make([]coverage.CoverageContract, 0, len(contracts))
	for _, contract := range contracts {
		if contract.Conclusion != dispatchConclusion {
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
	return CoveragePlan{Contracts: supported, Rules: []coverage.RulePlan{{Semantic: bindings.Dispatch, Covers: covers}}}, true
}
