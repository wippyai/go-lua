// Package summary defines fixed-point function summaries for analysis checks.
package summary

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

// Summary is the fixed-point analysis summary payload for one function entry.
type Summary struct {
	Returns                     []product.Value
	ParamObligations            []product.Value
	ParamMemberCallObligations  []ParamMemberCallObligation
	ParamMemberReturnSlots      []ParamMemberReturnSlot
	ReturnParamPathAliases      []ReturnParamPathAlias
	ReturnFlows                 []ReturnFlow
	ParamSinkExposures          []ParamSinkExposure
	CapturedPathObligations     []CapturedPathObligation
	NormalReturnParams          []product.Value
	NormalReturnParamConditions []ParamCondition
	NormalReturnParamEqualities []ParamEquality
	NormalReturnFacts           callboundary.NormalReturnFacts
	ProtectedCallTypestate      callboundary.ProtectedCallTypestate
	HeapTableObjects            map[identity.ID]heapidentity.TableObject
	// FreshHeapAllocations identifies callee-created heap objects reachable from
	// a return. Consumers instantiate these templates at the caller's static call
	// site. IDs seeded through parameters, captures, or globals are excluded.
	FreshHeapAllocations            []FreshHeapAllocation
	ReturnConditionParamRefinements []ReturnConditionParamRefinement
	ReturnConditionSlotRefinements  []ReturnConditionSlotRefinement
	ReturnParamLiteralCases         []ReturnParamLiteralCase
	ReturnPresenceRelations         []ReturnPresenceRelation
	MaySuspend                      bool

	// HeapKeySpace is the keyspace under which HeapTableObjects' rootless
	// static-member keys were interned. It is metadata only: it never affects
	// summary equality, ordering, or lattice joins (which always operate within a
	// single producing analysis), and exists so a consumer reading this summary at
	// a call site can rebase the heap keys into its own keyspace. It is nil when
	// the summary carries no heap objects.
	HeapKeySpace *keyspace.KeySpace
}

// FreshHeapAllocation carries one callee-local returned allocation template
// and its proven placement at normal exit. Callers instantiate ID while
// preserving this placement (with Stack promoted across the call boundary).
type FreshHeapAllocation struct {
	ID        identity.ID
	Placement placement.Value
}

// CallerPlacement returns the conservative placement after the allocation
// crosses a normal call boundary.
func (a FreshHeapAllocation) CallerPlacement() placement.Value {
	if a.Placement == placement.Bottom || a.Placement == placement.Stack {
		if a.Placement == placement.Bottom {
			return placement.Unknown
		}
		return placement.OwnedHeap
	}
	return a.Placement
}

// CallerFreshHeapPlacements materializes caller-keyed placement facts without
// exposing placement-domain construction to the call-result adapter.
func CallerFreshHeapPlacements(allocations []FreshHeapAllocation) map[identity.ID]placement.Value {
	if len(allocations) == 0 {
		return nil
	}
	out := make(map[identity.ID]placement.Value, len(allocations))
	for _, allocation := range allocations {
		if allocation.ID != (identity.ID{}) {
			out[allocation.ID] = allocation.CallerPlacement()
		}
	}
	return out
}

// RekeyHeapTableObjects re-interns the rootless static-member keys of every
// heap table object from this summary's producing keyspace into to, so a
// consumer reading the summary at a call site can operate on the objects in its
// own keyspace. Same-space imports validate nested ownership. Nil provenance is
// accepted only when every object is structurally key-free; keyed objects
// without provenance fail closed.
func (s Summary) RekeyHeapTableObjects(to *keyspace.KeySpace) (Summary, error) {
	if to != nil && !to.Valid() || s.HeapKeySpace != nil && !s.HeapKeySpace.Valid() {
		return s, fmt.Errorf("summary rekey: invalid keyspace authority")
	}
	if len(s.HeapTableObjects) == 0 {
		s.HeapKeySpace = nil
		return s, nil
	}
	if heapTableObjectsStructuralKeyFree(s.HeapTableObjects) {
		s.HeapKeySpace = nil
		return s, nil
	}
	if to == nil || s.HeapKeySpace == nil {
		return s, fmt.Errorf("summary rekey: nil or invalid keyspace")
	}
	rekeyed := make(map[identity.ID]heapidentity.TableObject, len(s.HeapTableObjects))
	for id, object := range s.HeapTableObjects {
		next, ok := object.Rekey(s.HeapKeySpace, to)
		if !ok {
			return s, fmt.Errorf("summary rekey: heap table object %v has a foreign structural key", id)
		}
		rekeyed[id] = next
	}
	s.HeapTableObjects = rekeyed
	s.HeapKeySpace = to
	return s, nil
}

// Clone returns an independent copy of s.
func (s Summary) Clone() Summary {
	if summaryBottom(s) {
		return Summary{}
	}
	out := Summary{}
	out.HeapKeySpace = s.HeapKeySpace
	for _, lane := range summaryLanes {
		lane.assignClone(s, &out)
	}
	return out
}
