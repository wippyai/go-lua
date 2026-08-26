package summary

import (
	heapsummary "github.com/wippyai/go-lua/analysis/domain/heap/relation/summary"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// AllocationEvidenceForMetadata is the reusable semantic join for one Heap
// allocation-root row and one complete Placement factor cell.
//
// Heap has already authenticated the allocation coordinate and its source
// metadata before producing metadata. Placement authenticates the factor at
// this boundary, projects Heap's closed source vocabulary into its own
// allocation-kind vocabulary, and then asks Placement's canonical evidence
// constructor to build the base evidence. No Heap Schema/Key lookup is
// repeated here, and no default or Unknown evidence is manufactured when a
// source is unavailable.
//
// The result is deliberately only Placement's base evidence. It is not a
// summary AllocationRow and has no encoder, so an incomplete metadata join
// cannot be mistaken for a publishable child result. The final summary
// operation composes every producer-owned proof first and is the sole caller
// allowed to construct the terminal AllocationRow.
func AllocationEvidenceForMetadata(metadata heapsummary.AllocationRow, fact placementdomain.Fact) (placementdomain.AllocationEvidence, bool) {
	if !metadata.Valid() {
		return placementdomain.AllocationEvidence{}, false
	}
	authenticated, ok := placementdomain.AuthenticateFactCell(fact, true, true)
	if !ok {
		return placementdomain.AllocationEvidence{}, false
	}
	kind, ok := allocationKindForSource(metadata.Origin())
	if !ok {
		return placementdomain.AllocationEvidence{}, false
	}
	evidence, ok := placementdomain.NewAllocationEvidence(metadata.ID(), kind, authenticated)
	if !ok {
		return placementdomain.AllocationEvidence{}, false
	}
	return evidence, true
}

// allocationKindForSource is the only closed vocabulary projection in this
// bridge. A malformed or future Heap source refuses; it cannot be silently
// represented as Placement's Unknown kind because that would turn missing
// producer evidence into an authenticated result.
func allocationKindForSource(source heapsummary.Source, sourceOK bool) (placementdomain.AllocationKind, bool) {
	if !sourceOK {
		return placementdomain.AllocationKindUnknown, false
	}
	switch source.Kind() {
	case heapsummary.SourceProgram:
		origin, ok := source.ProgramOrigin()
		if !ok {
			return placementdomain.AllocationKindUnknown, false
		}
		switch origin.Kind {
		case heapdomain.AllocationTable:
			return placementdomain.AllocationKindTable, true
		case heapdomain.AllocationClosure:
			return placementdomain.AllocationKindClosure, true
		default:
			return placementdomain.AllocationKindUnknown, false
		}
	case heapsummary.SourceFresh:
		if _, ok := source.FreshResult(); !ok {
			return placementdomain.AllocationKindUnknown, false
		}
		return placementdomain.AllocationKindManifest, true
	default:
		return placementdomain.AllocationKindUnknown, false
	}
}
