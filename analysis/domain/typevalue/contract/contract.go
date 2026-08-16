// Package contract declares TypeValue's cold source-coverage obligations.
package contract

import (
	"github.com/wippyai/go-lua/analysis/coverage"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/semanticsource"
)

const conclusionRevision uint16 = 2

type conclusion uint16

const (
	conclusionTypeValueSource conclusion = iota + 1
	conclusionTypeValueTarget
	conclusionTypeValueResolution
)

type sourceConclusion struct {
	origin     semanticsource.Origin
	facet      semanticsource.Facet
	conclusion conclusion
}

// TypeValue interprets only binder-authorized runtime TypeValue sources and
// their exact authored static descriptor/resolution.  Heap allocation roots
// remain Heap-owned, and ordinary Value/call flow cannot manufacture a
// TypeValue source.
var sourceInventory = [...]sourceConclusion{
	{semanticsource.OriginProgramFlowTypeValue, 0, conclusionTypeValueSource},
	{semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeValueTarget, conclusionTypeValueTarget},
	{semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeRef, conclusionTypeValueTarget},
	{semanticsource.OriginLinkProjectShardMount, 0, conclusionTypeValueResolution},
	{semanticsource.OriginLinkStatic, 0, conclusionTypeValueResolution},
}

// Contracts returns TypeValue's canonically ordered Factor-owned obligations.
func Contracts(factor engine.SemanticKey) ([]coverage.CoverageContract, bool) {
	if !factor.Available() {
		return nil, false
	}
	contracts := make([]coverage.CoverageContract, 0, len(sourceInventory))
	for _, item := range sourceInventory {
		definition, found := semanticsource.Definition(item.origin, item.facet)
		judgment, valid := coverage.DeriveConclusion(factor, uint16(item.conclusion), conclusionRevision)
		if !found || !valid {
			return nil, false
		}
		contracts = append(contracts, coverage.CoverageContract{Source: definition.Token(), Class: coverage.OwnerFactor, Owner: factor, Conclusion: judgment})
	}
	return coverage.SealContracts(contracts)
}
