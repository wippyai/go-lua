// Package contract declares Effect's cold source-to-conclusion obligations.
//
// It deliberately does not use the legacy effect Label, Row, or Var APIs.
// Those are neither the planned Effect Factor's carrier nor an authority for
// its source coverage.  This fragment names only the existing generated
// Program, Target, and Link relation families that a later Effect Factor must
// interpret.
package contract

import (
	"github.com/wippyai/go-lua/analysis/coverage"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/semanticsource"
)

// revision changes whenever the source-to-judgment inventory changes.  It
// prevents a conclusion produced from the retired aggregate effect row from
// being mistaken for this exact-row contract.
const revision uint16 = 2

// conclusion is Effect's private, closed vocabulary of actual judgments.  It
// is deliberately smaller than the source inventory: the several exact
// operands of one future transfer share one conclusion rather than receiving
// a source-shaped ordinal.  These are not Factor coordinates, Rule IDs, or
// effect-row identities.
type conclusion uint16

const (
	bodyCapability conclusion = iota + 1
	selectedCallRow
	outcomeCapability
	boundaryTransport
	opaqueBoundary
)

type obligation struct {
	origin     semanticsource.Origin
	facet      semanticsource.Facet
	conclusion conclusion
}

// PlanBindings supplies the three already-declared Effect Rules and the exact
// Effect Query. The owner-local obligation classes below decide the lane.
type PlanBindings struct {
	Selected engine.SemanticKey
	Opaque   engine.SemanticKey
	Body     engine.SemanticKey
	Query    engine.SemanticKey
}

// CoveragePlan is Effect's detached source-to-conclusion treatment plan.
type CoveragePlan struct {
	Contracts []coverage.CoverageContract
	Rules     []coverage.RulePlan
	Queries   []coverage.QueryPlan
}

// obligations is the presently representable Effect denominator.
//
// TargetOperationEffect and TargetCallbackEffect are separate source
// relations: their owner and substitutions differ even though both feed the
// same selected-call effect judgment.  They share selectedCallRow with the
// exact call and application operands that make a candidate executable.
// Outcome and boundary rows likewise share their own actual Effect judgments.
// Target's opaque declaration is kept distinct because it contributes
// UnknownExternal, not another explicit template row.
//
// There is intentionally no Program-authored RowSpec entry here: the current
// semantic-source schema has no such relation.  Nor is ProgramStatic's
// FunctionContract facet used as a proxy; it does not declare a typed effect
// row.  Likewise, generic LinkBoundary is the available exact boundary
// correspondence token, but the schema does not yet expose an effect-row
// formal-substitution facet.  Those omissions must be added to the source
// schema before this fragment can claim them.
var obligations = [...]obligation{
	{semanticsource.OriginProgramFlowBody, 0, bodyCapability},
	{semanticsource.OriginProgramFlowBody, semanticsource.FacetProgramFlowBodyRoots, bodyCapability},

	{semanticsource.OriginProgramFlowCall, 0, selectedCallRow},
	{semanticsource.OriginTargetOperation, 0, selectedCallRow},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetOperationEffect, selectedCallRow},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackEffect, selectedCallRow},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallback, selectedCallRow},
	{semanticsource.OriginLinkProjectBaseApplication, 0, selectedCallRow},
	{semanticsource.OriginLinkBoundary, 0, selectedCallRow},

	{semanticsource.OriginProgramFlowOutcome, 0, outcomeCapability},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI, outcomeCapability},
	{semanticsource.OriginLinkBoundary, 0, outcomeCapability},

	{semanticsource.OriginProgramModuleImport, 0, boundaryTransport},
	{semanticsource.OriginProgramModuleImport, semanticsource.FacetProgramModuleRequest, boundaryTransport},
	{semanticsource.OriginLinkBoundary, 0, boundaryTransport},
	{semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleTransport, boundaryTransport},
	{semanticsource.OriginLinkHost, semanticsource.FacetLinkHostEndpointTarget, boundaryTransport},

	{semanticsource.OriginProgramFlowCall, 0, opaqueBoundary},
	{semanticsource.OriginTargetOperation, 0, opaqueBoundary},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetOpaque, opaqueBoundary},
	{semanticsource.OriginLinkProjectBaseApplication, 0, opaqueBoundary},
	{semanticsource.OriginLinkBoundary, 0, opaqueBoundary},
}

// Contracts returns Effect's complete currently representable cold contract
// for one already-declared Factor identity.  It carries no row payload,
// template universe, Rule, Factor, or execution claim; a later selected
// execution treatment must discharge these obligations.
func Contracts(factor engine.SemanticKey) ([]coverage.CoverageContract, bool) {
	if !factor.Available() {
		return nil, false
	}
	contracts := make([]coverage.CoverageContract, 0, len(obligations))
	for _, row := range obligations {
		definition, found := semanticsource.Definition(row.origin, row.facet)
		conclusion, derived := coverage.DeriveConclusion(factor, uint16(row.conclusion), revision)
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

// BuildPlan binds body, selected-call, outcome, and boundary obligations to
// the selected callsite Rule while opaque boundary obligations use the opaque
// Rule. The body root is the one exact observation claimed by Effect's Query.
func BuildPlan(factor engine.SemanticKey, bindings PlanBindings) (CoveragePlan, bool) {
	if !factor.Available() || !bindings.Selected.Available() || !bindings.Opaque.Available() || !bindings.Body.Available() || !bindings.Query.Available() {
		return CoveragePlan{}, false
	}
	contracts, ok := Contracts(factor)
	if !ok {
		return CoveragePlan{}, false
	}
	by := make(map[conclusion][]coverage.Requirement, len(contracts))
	for _, contract := range contracts {
		for _, declared := range obligations {
			definition, defined := semanticsource.Definition(declared.origin, declared.facet)
			judgment, derived := coverage.DeriveConclusion(factor, uint16(declared.conclusion), revision)
			if defined && derived && definition.Token() == contract.Source && judgment == contract.Conclusion {
				by[declared.conclusion] = append(by[declared.conclusion], contract.Requirement())
				break
			}
		}
	}
	queryRequirement, queryOK := oneRequirement(by[bodyCapability], semanticsource.OriginProgramFlowBody, 0)
	if !queryOK {
		return CoveragePlan{}, false
	}
	body := removeRequirement(by[bodyCapability], queryRequirement)
	body, ok = coverage.SealRequirements(body)
	if !ok {
		return CoveragePlan{}, false
	}
	selected := make([]coverage.Requirement, 0, len(contracts))
	for _, kind := range []conclusion{selectedCallRow, outcomeCapability, boundaryTransport} {
		selected = append(selected, by[kind]...)
	}
	selected, ok = coverage.SealRequirements(selected)
	if !ok {
		return CoveragePlan{}, false
	}
	opaque, ok := coverage.SealRequirements(by[opaqueBoundary])
	if !ok {
		return CoveragePlan{}, false
	}
	return CoveragePlan{
		Contracts: contracts,
		Rules:     []coverage.RulePlan{{Semantic: bindings.Body, Covers: body}, {Semantic: bindings.Selected, Covers: selected}, {Semantic: bindings.Opaque, Covers: opaque}},
		Queries:   []coverage.QueryPlan{{Semantic: bindings.Query, Covers: []coverage.Requirement{queryRequirement}}},
	}, true
}

func oneRequirement(requirements []coverage.Requirement, origin semanticsource.Origin, facet semanticsource.Facet) (coverage.Requirement, bool) {
	for _, requirement := range requirements {
		if requirement.Source.Origin() == origin && requirement.Source.Facet() == facet {
			return requirement, true
		}
	}
	return coverage.Requirement{}, false
}

func removeRequirement(requirements []coverage.Requirement, remove coverage.Requirement) []coverage.Requirement {
	result := make([]coverage.Requirement, 0, len(requirements))
	for _, requirement := range requirements {
		if requirement != remove {
			result = append(result, requirement)
		}
	}
	return result
}
