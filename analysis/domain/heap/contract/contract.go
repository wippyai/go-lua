// Package contract declares Heap's cold source-to-conclusion obligations.
//
// It is intentionally not a Rule inventory.  In particular, a boundary
// availability row is an operand of Heap's one fresh-root judgment, not proof
// that a Target/Link candidate has executed.  A later activation must prove
// one exact operation and outcome before that judgment can be derived.
package contract

import (
	"github.com/wippyai/go-lua/analysis/coverage"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/semanticsource"
)

const revision uint16 = 1

// conclusion is Heap's private closed vocabulary.  These ordinals are stable
// within this package only; they are neither Rule identities nor source names.
type conclusion uint16

const (
	objectRoot conclusion = iota + 1
	rawAccess
	objectContents
	emptyRoot
	freshRoot
	bootRoot
)

type row struct {
	origin     semanticsource.Origin
	facet      semanticsource.Facet
	conclusion conclusion
}

// PlanBindings names the existing Heap Rule lanes.  The source/conclusion
// ownership remains private to this package; callers only provide identities
// that were already sealed in the production Composition.
type PlanBindings struct {
	Ingress   engine.SemanticKey
	Closed    engine.SemanticKey
	Empty     engine.SemanticKey
	RawSet    engine.SemanticKey
	Bootstrap engine.SemanticKey
}

// CoveragePlan is Heap's detached source-to-Rule treatment plan.
type CoveragePlan struct {
	Contracts []coverage.CoverageContract
	Rules     []coverage.RulePlan
}

// rows are in the generated source-token order.  A repeated source is
// deliberate only when it supplies distinct Heap carrier judgments.
var rows = [...]row{
	{semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageAssign, objectContents},
	{semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageWrite, objectContents},
	{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowIndexSet, rawAccess},
	{semanticsource.OriginProgramFlowConstructors, 0, objectRoot},
	{semanticsource.OriginProgramFlowConstructors, 0, objectContents},
	{semanticsource.OriginProgramFlowConstructors, 0, emptyRoot},
	{semanticsource.OriginProgramFlowFunction, 0, objectRoot},
	{semanticsource.OriginProgramFlowFunction, 0, emptyRoot},
	{semanticsource.OriginProgramFlowCall, 0, freshRoot},
	{semanticsource.OriginProgramFlowOutcome, 0, freshRoot},
	{semanticsource.OriginTargetOperation, 0, freshRoot},
	{semanticsource.OriginTargetBoot, 0, bootRoot},
	{semanticsource.OriginLinkProjectBaseApplication, 0, freshRoot},
	{semanticsource.OriginLinkBoundary, 0, freshRoot},
	{semanticsource.OriginLinkHost, semanticsource.FacetLinkHostBoot, bootRoot},
}

// Contracts returns Heap's complete cold obligations for one already-declared
// Factor semantic identity.  It names no existing Rule, carrier, topology
// row, or runtime fact.  Invalid factor identities fail closed.
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

// BuildPlan preserves Heap's exact executable ownership: table contents go to
// Closed, empty construction, fresh/object roots to Ingress, and boot roots
// to Bootstrap. Raw Value reads are owned by Value's RawGet Rule, while
// indexed writes are owned by Heap's RawSet Rule. No requirement is assigned
// by source order.
func BuildPlan(factor engine.SemanticKey, bindings PlanBindings) (CoveragePlan, bool) {
	if !factor.Available() || !bindings.Ingress.Available() || !bindings.Closed.Available() || !bindings.RawSet.Available() || !bindings.Bootstrap.Available() {
		return CoveragePlan{}, false
	}
	contracts, ok := Contracts(factor)
	if !ok {
		return CoveragePlan{}, false
	}
	// Contracts are sealed in canonical order, so re-deriving every row for
	// every contract is unnecessary. Build the exact requirement relation once
	// and use the sealed contract requirement as its lookup key.
	byRequirement := make(map[coverage.Requirement]conclusion, len(rows))
	for _, declared := range rows {
		definition, found := semanticsource.Definition(declared.origin, declared.facet)
		judgment, derived := coverage.DeriveConclusion(factor, uint16(declared.conclusion), revision)
		if !found || !derived {
			return CoveragePlan{}, false
		}
		contract := coverage.CoverageContract{
			Source:     definition.Token(),
			Class:      coverage.OwnerFactor,
			Owner:      factor,
			Conclusion: judgment,
		}
		requirement := contract.Requirement()
		if _, duplicate := byRequirement[requirement]; duplicate {
			return CoveragePlan{}, false
		}
		byRequirement[requirement] = declared.conclusion
	}
	by := make(map[conclusion][]coverage.Requirement, len(contracts))
	for _, contract := range contracts {
		declared, found := byRequirement[contract.Requirement()]
		if !found {
			return CoveragePlan{}, false
		}
		by[declared] = append(by[declared], contract.Requirement())
	}
	plans := []coverage.RulePlan{
		{Semantic: bindings.Ingress, Covers: append(append([]coverage.Requirement(nil), by[objectRoot]...), by[freshRoot]...)},
		{Semantic: bindings.Closed, Covers: by[objectContents]},
		{Semantic: bindings.Empty, Covers: by[emptyRoot]},
		{Semantic: bindings.RawSet, Covers: by[rawAccess]},
		{Semantic: bindings.Bootstrap, Covers: by[bootRoot]},
	}
	for index := range plans {
		sealed, sealedOK := coverage.SealRequirements(plans[index].Covers)
		if !sealedOK {
			return CoveragePlan{}, false
		}
		plans[index].Covers = sealed
	}
	return CoveragePlan{Contracts: contracts, Rules: plans}, true
}
