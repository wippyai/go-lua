// Package contract declares Static's cold source-coverage obligations.
package contract

import (
	"github.com/wippyai/go-lua/analysis/coverage"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/semanticsource"
)

const conclusionRevision uint16 = 2

type conclusion uint16

const (
	conclusionStaticSyntax conclusion = iota + 1
	conclusionRuntimeSubject
	conclusionTargetType
	conclusionStaticResolution
)

type sourceConclusion struct {
	origin     semanticsource.Origin
	facet      semanticsource.Facet
	conclusion conclusion
}

// Static consumes the complete authored static surface, the runtime Value
// subject only at a typeof frontier, Target's declared type-bearing operation
// surface, and Link's static namespace/resolution relation.  It does not
// interpret runtime storage, heap, calls, effects, or provider execution.
var sourceInventory = [...]sourceConclusion{
	{semanticsource.OriginProgramFlowValues, 0, conclusionRuntimeSubject},
	{semanticsource.OriginProgramStatic, 0, conclusionStaticSyntax},
	{semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticFunctionContract, conclusionStaticSyntax},
	{semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticCallTypeArguments, conclusionStaticSyntax},
	{semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticCellDeclaredType, conclusionStaticSyntax},
	{semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticClaimTarget, conclusionStaticSyntax},
	{semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeValueTarget, conclusionStaticSyntax},
	{semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeof, conclusionStaticSyntax},
	{semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticAnnotation, conclusionStaticSyntax},
	{semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticPublication, conclusionStaticSyntax},
	{semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeRef, conclusionStaticSyntax},
	{semanticsource.OriginProgramModuleImport, 0, conclusionStaticResolution},
	{semanticsource.OriginProgramModuleImport, semanticsource.FacetProgramModuleRequest, conclusionStaticResolution},
	{semanticsource.OriginTargetContract, 0, conclusionTargetType},
	{semanticsource.OriginTargetOperation, 0, conclusionTargetType},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI, conclusionTargetType},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetSubedge, conclusionTargetType},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallback, conclusionTargetType},
	{semanticsource.OriginTargetOperation, semanticsource.FacetTargetResume, conclusionTargetType},
	{semanticsource.OriginTargetProtocol, 0, conclusionTargetType},
	{semanticsource.OriginLinkProjectShardMount, 0, conclusionStaticResolution},
	{semanticsource.OriginLinkStatic, 0, conclusionStaticResolution},
	{semanticsource.OriginLinkStatic, semanticsource.FacetLinkStaticResolution, conclusionStaticResolution},
}

// Contracts returns Static's canonically ordered Factor-owned obligations.
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
