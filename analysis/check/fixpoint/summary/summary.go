// Package summary defines fixed-point function summaries for analysis checks.
package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

// Summary is the fixed-point analysis summary payload for one function entry.
type Summary struct {
	Returns                         []product.Value
	ParamObligations                []product.Value
	ParamMemberCallObligations      []ParamMemberCallObligation
	ParamMemberReturnSlots          []ParamMemberReturnSlot
	ReturnParamPathAliases          []ReturnParamPathAlias
	ReturnFlows                     []ReturnFlow
	ParamSinkExposures              []ParamSinkExposure
	CapturedPathObligations         []CapturedPathObligation
	NormalReturnParams              []product.Value
	NormalReturnParamConditions     []ParamCondition
	NormalReturnParamEqualities     []ParamEquality
	NormalReturnFacts               callboundary.NormalReturnFacts
	HeapTableObjects                map[identity.ID]heapidentity.TableObject
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

// RekeyHeapTableObjects re-interns the rootless static-member keys of every
// heap table object from this summary's producing keyspace into to, so a
// consumer reading the summary at a call site can operate on the objects in its
// own keyspace. It is a no-op when the summary carries no heap objects, has no
// producing keyspace, or to equals the producing keyspace.
func (s Summary) RekeyHeapTableObjects(to *keyspace.KeySpace) Summary {
	if len(s.HeapTableObjects) == 0 || s.HeapKeySpace == nil || to == nil || s.HeapKeySpace == to {
		return s
	}
	rekeyed := make(map[identity.ID]heapidentity.TableObject, len(s.HeapTableObjects))
	for id, object := range s.HeapTableObjects {
		rekeyed[id] = object.Rekey(s.HeapKeySpace, to)
	}
	s.HeapTableObjects = rekeyed
	s.HeapKeySpace = to
	return s
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
