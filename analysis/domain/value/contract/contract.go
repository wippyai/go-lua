// Package contract declares Value's cold source-coverage obligations.
//
// It is intentionally separate from Value's schema and Rules: this package
// names only which sealed source relations require an observable Value
// judgment.  It carries no Value fact, Program row, or execution topology.
package contract

import (
	"github.com/wippyai/go-lua/analysis/coverage"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/semanticsource"
)

const conclusionRevision uint16 = 1

// conclusion is Value's private observable-judgment vocabulary.  These are
// deliberately not Rule identities: several source families can contribute
// the same Value judgment, and a Rule can later discharge several judgments.
type conclusion uint16

const (
	conclusionSourceValue conclusion = iota + 1
	conclusionStorageValue
	conclusionConstructedValue
	conclusionRawGetValue
	conclusionRuntimeTypeValue
	conclusionBootValue
	conclusionHostValue
)

type sourceConclusion struct {
	origin     semanticsource.Origin
	facet      semanticsource.Facet
	conclusion conclusion
}

// PlanBindings are the already-sealed production identities that discharge
// Value's source conclusions.  The contract package owns the source-to-
// conclusion-to-lane mapping; the composition root supplies only the actual
// Rule and Query identities it declared.
type PlanBindings struct {
	Source     engine.SemanticKey
	RawGet     engine.SemanticKey
	Allocation engine.SemanticKey
	Bootstrap  engine.SemanticKey
	Transfer   engine.SemanticKey
	Query      engine.SemanticKey
}

// CoveragePlan is Value's detached treatment plan for one exact Factor.
type CoveragePlan struct {
	Contracts []coverage.CoverageContract
	Rules     []coverage.RulePlan
	Queries   []coverage.QueryPlan
}

// sourceInventory is the complete source denominator interpreted by the
// current Value schema: literal/key source values, storage transport,
// constructed references, raw Value reads, binder-authorized runtime type
// values, and Target/Link boot and host sources.  It intentionally does
// not claim Heap, Call, control, module, or effect relations; their domains
// own those judgments.
var sourceInventory = [...]sourceConclusion{
	{semanticsource.OriginProgramSourceExactKey, 0, conclusionSourceValue},
	{semanticsource.OriginProgramFlowLiterals, 0, conclusionSourceValue},
	{semanticsource.OriginProgramFlowValues, 0, conclusionSourceValue},
	{semanticsource.OriginProgramFlowValues, semanticsource.FacetProgramFlowValueOccurrence, conclusionSourceValue},
	{semanticsource.OriginProgramFlowStorage, 0, conclusionStorageValue},
	{semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageRead, conclusionStorageValue},
	{semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageAssign, conclusionStorageValue},
	{semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageWrite, conclusionStorageValue},
	{semanticsource.OriginProgramFlowConstructors, 0, conclusionConstructedValue},
	{semanticsource.OriginProgramFlowLens, 0, conclusionRawGetValue},
	{semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageGlobal, conclusionRawGetValue},
	{semanticsource.OriginProgramFlowTypeValue, 0, conclusionRuntimeTypeValue},
	{semanticsource.OriginTargetContract, 0, conclusionBootValue},
	{semanticsource.OriginTargetOperation, 0, conclusionBootValue},
	{semanticsource.OriginTargetBoot, 0, conclusionBootValue},
	{semanticsource.OriginLinkProjectShardMount, 0, conclusionSourceValue},
	{semanticsource.OriginLinkHost, 0, conclusionHostValue},
	{semanticsource.OriginLinkHost, semanticsource.FacetLinkHostExposure, conclusionHostValue},
	{semanticsource.OriginLinkHost, semanticsource.FacetLinkHostBoot, conclusionBootValue},
	{semanticsource.OriginLinkHost, semanticsource.FacetLinkHostMember, conclusionHostValue},
	{semanticsource.OriginLinkHost, semanticsource.FacetLinkHostEndpointTarget, conclusionHostValue},
}

// Contracts returns the canonical, duplicate-free coverage obligations for
// one sealed Value Factor.  A missing or forged Factor identity is rejected
// before any conclusion identity is derived.
func Contracts(factor engine.SemanticKey) ([]coverage.CoverageContract, bool) {
	if !factor.Available() {
		return nil, false
	}
	contracts := make([]coverage.CoverageContract, 0, len(sourceInventory))
	for _, item := range sourceInventory {
		definition, found := semanticsource.Definition(item.origin, item.facet)
		if !found {
			return nil, false
		}
		judgment, valid := coverage.DeriveConclusion(factor, uint16(item.conclusion), conclusionRevision)
		if !valid {
			return nil, false
		}
		contracts = append(contracts, coverage.CoverageContract{
			Source: definition.Token(), Class: coverage.OwnerFactor, Owner: factor, Conclusion: judgment,
		})
	}
	return coverage.SealContracts(contracts)
}

// BuildPlan binds each private Value conclusion to one existing production
// lane.  It deliberately has no fallback assignment: a missing binding or a
// conclusion with no named lane fails the whole plan.
func BuildPlan(factor engine.SemanticKey, bindings PlanBindings) (CoveragePlan, bool) {
	if !factor.Available() || !available(bindings) {
		return CoveragePlan{}, false
	}
	contracts, ok := Contracts(factor)
	if !ok {
		return CoveragePlan{}, false
	}
	byConclusion := make(map[conclusion][]coverage.Requirement, len(sourceInventory))
	for _, contract := range contracts {
		for _, item := range sourceInventory {
			definition, defined := semanticsource.Definition(item.origin, item.facet)
			judgment, derived := coverage.DeriveConclusion(factor, uint16(item.conclusion), conclusionRevision)
			if !defined || !derived || definition.Token() != contract.Source || judgment != contract.Conclusion {
				continue
			}
			byConclusion[item.conclusion] = append(byConclusion[item.conclusion], contract.Requirement())
			break
		}
	}
	// The source Rule is the exact zero-input Value treatment used by literal
	// returns. Control geometry is owned by Program Flow and is intentionally
	// absent here; a structural Source/Flow/Module plan discharges it at the
	// sealed owner boundary.
	rules := []coverage.RulePlan{
		{Semantic: bindings.Source, Covers: byConclusion[conclusionSourceValue]},
		{Semantic: bindings.RawGet, Covers: byConclusion[conclusionRawGetValue]},
		{Semantic: bindings.Allocation, Covers: byConclusion[conclusionConstructedValue]},
		{Semantic: bindings.Bootstrap, Covers: append(append([]coverage.Requirement(nil), byConclusion[conclusionBootValue]...), byConclusion[conclusionHostValue]...)},
		{Semantic: bindings.Transfer, Covers: byConclusion[conclusionStorageValue]},
	}
	// Runtime type values remain a Value-owned judgment. Operators and claims
	// are deliberately not admitted until their dedicated Rules are declared;
	// this Value Rule therefore leaves those source relations unclaimed.
	rules[0].Covers = append(rules[0].Covers, byConclusion[conclusionRuntimeTypeValue]...)
	queryRequirement, queryOK := oneRequirement(byConclusion[conclusionSourceValue], semanticsource.OriginProgramFlowValues, 0)
	if !queryOK {
		return CoveragePlan{}, false
	}
	rules[0].Covers = removeRequirement(rules[0].Covers, queryRequirement)
	queries := []coverage.QueryPlan{{Semantic: bindings.Query, Covers: []coverage.Requirement{queryRequirement}}}
	for index := range rules {
		sealed, sealedOK := coverage.SealRequirements(rules[index].Covers)
		if !sealedOK {
			return CoveragePlan{}, false
		}
		rules[index].Covers = sealed
	}
	return CoveragePlan{Contracts: contracts, Rules: rules, Queries: queries}, true
}

func available(bindings PlanBindings) bool {
	keys := []engine.SemanticKey{bindings.Source, bindings.RawGet, bindings.Allocation, bindings.Bootstrap, bindings.Transfer, bindings.Query}
	for _, key := range keys {
		if !key.Available() {
			return false
		}
	}
	return true
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
