// Package contract declares Numeric's cold source-coverage obligations.
//
// The declarations are Source × Numeric-judgment pairs only.  They neither
// encode a Rule inventory nor interpret Program, Target, or Link rows.
package contract

import (
	"github.com/wippyai/go-lua/analysis/coverage"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/semanticsource"
)

const contractRevision uint16 = 2

// Contracts returns Numeric's canonical source coverage claims for factor.
//
// The private ordinals identify the smallest Numeric judgments: scalar
// projection/literal facts, primitive operation facts, and truth-branch
// refinement. The returned slice is freshly
// allocated and canonically sealed.
func Contracts(factor engine.SemanticKey) ([]coverage.CoverageContract, bool) {
	if !factor.Available() {
		return nil, false
	}
	rows := []row{
		{semanticsource.OriginProgramFlowLiterals, 0, 1},
		{semanticsource.OriginProgramFlowValues, semanticsource.FacetProgramFlowValueOccurrence, 1},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowUnaryNumeric, 2},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowLength, 2},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowArithmetic, 2},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowBitwise, 2},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowEquality, 3},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowOrder, 3},
		{semanticsource.OriginProgramFlowControl, 0, 3},
		{semanticsource.OriginProgramFlowBody, 0, 1},
		{semanticsource.OriginProgramFlowBody, semanticsource.FacetProgramFlowBodyRoots, 1},
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

type row struct {
	origin  semanticsource.Origin
	facet   semanticsource.Facet
	ordinal uint16
}
