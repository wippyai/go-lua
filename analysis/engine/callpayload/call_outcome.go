// Package callpayload defines generic call-boundary payload DTOs.
package callpayload

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// CallResult is one indexed abstract result produced by a call.
type CallResult struct {
	Index int
	Value product.Value
}

// CallOutcomeProvider resolves rich call-site evidence into one generic call
// outcome payload.
type CallOutcomeProvider func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) CallOutcome

// CallOutcome is the generic payload produced at a call boundary. It carries
// return-slot values plus normal-return facts expressed over placeholder paths
// such as $0 and $1. Fact application rebases those paths at the caller.
type CallOutcome struct {
	Results []CallResult

	// PostReturnAuthority means this outcome matched a semantic provider that
	// owns result slots and normal-return state facts for the call. Supplemental
	// providers may still add diagnostic ParamObligations, but must not publish
	// weaker return or post-return facts through this call.
	PostReturnAuthority bool

	NormalReturnFacts          callboundary.NormalReturnFacts
	HeapTableObjects           map[identity.ID]heapidentity.TableObject
	Placements                 map[identity.ID]placement.Value
	ParamObligations           []CallParamObligation
	ParamPathRefinements       []CallParamPathRefinement
	ParamLengthFloors          []CallParamLengthFloor
	ParamPathInvalidations     []CallParamPathInvalidation
	ParamConditions            []CallParamCondition
	ParamPathRelations         []CallParamPathRelation
	ReturnConditionRefinements []CallReturnConditionRefinement
	ReturnPresenceRelations    []CallReturnPresenceRelation
}

// CallParamObligation records a pre-call value constraint for one explicit
// argument. It is diagnostic evidence only and is not applied as a normal-return
// refinement to caller state.
type CallParamObligation struct {
	ParamIndex int
	Value      product.Value
}

// CallParamPathRefinement records a normal-return value constraint for a
// parameter placeholder path. Parameter placeholders are indexed by explicit
// argument position and do not include the receiver slot.
type CallParamPathRefinement struct {
	Path  pathdom.Path
	Value product.Value
}

// CallParamLengthFloor records a normal-return lower bound on len(param path).
type CallParamLengthFloor struct {
	Path  pathdom.Path
	Floor int64
}

// CallParamPathInvalidation records that descendants below a parameter
// placeholder path were invalidated by a normal-returning call.
type CallParamPathInvalidation struct {
	Path pathdom.Path
}

// CallParamCondition records that a normal return selects the truthiness facts
// for one call argument expression.
type CallParamCondition struct {
	ParamIndex int
	Value      bool
}

// CallPathRelationKind classifies a normal-return relation over placeholder
// paths.
type CallPathRelationKind uint8

const (
	CallPathRelationEqual CallPathRelationKind = iota + 1
)

// CallParamPathRelation records a normal-return relation over parameter
// placeholder paths. Parameter placeholders are indexed by explicit argument
// position and do not include the receiver slot.
type CallParamPathRelation struct {
	Kind  CallPathRelationKind
	Left  pathdom.Path
	Right pathdom.Path
}

// CallReturnConditionRefinement records a parameter-relative value refinement
// selected by the boolean value of one call return slot.
type CallReturnConditionRefinement struct {
	ReturnIndex int
	ReturnValue bool
	Target      pathdom.Path
	Value       product.Value
}

// CallReturnPresenceRelation records a must implication between two call
// return slots.
type CallReturnPresenceRelation struct {
	TriggerIndex    int
	TriggerPresence presence.Value
	TargetIndex     int
	TargetPresence  presence.Value
}
