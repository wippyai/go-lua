package factapply

import (
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// ConcreteRootAssignmentPointRequest is the complete concrete transaction for
// a root assignment at one CFG point. Input is the immutable point-entry
// snapshot used by source and call-outcome providers. Output is the evolving
// state after earlier operations at the same point.
//
// The distinction is semantic: callers must not replace Input with Output.
// Object-literal sibling reads and return-slot outcome reads use Input, while
// the assignment and its sidecars publish onto Output in the order documented
// by ApplyConcreteRootAssignmentPoint.
type ConcreteRootAssignmentPointRequest struct {
	Context                transfer.NodeContext
	Resolver               *visibility.Resolver
	Facts                  factflow.Facts
	Sources                sourcevalue.SourceValues
	Read                   func(cfg.Point) state.State
	Input                  state.State
	Output                 state.State
	Assignment             factflow.RootAssignment
	CallOutcome            callpayload.CallOutcomeProvider
	ProjectPath            PathTypeProjector
	CovariantWiden         CovariantWiden
	TypeValues             *typevalue.Cache
	ClosedDynamicAllValues []ClosedDynamicAllValueInvariant

	// presenceCache is closure-owned acceleration only. It has no semantic
	// effect and is deliberately not part of the public transaction contract.
	presenceCache *callOutcomeTraversalCache
}

// ConcreteRootAssignmentPointResult reports the transaction's published state
// and whether the root core resolved a value and performed the assignment.
// Applied=false can still carry the current dynamic-key-membership evidence
// delta; assignment sidecars remain gated exactly as in the legacy transfer.
type ConcreteRootAssignmentPointResult struct {
	Output  state.State
	Applied bool
}

// ApplyConcreteRootAssignmentPoint applies one root assignment as an atomic
// semantic unit in the established concrete order:
//
//  1. root core and root evidence lanes,
//  2. object-literal heap and entry sidecars,
//  3. call-return-slot facts, and
//  4. call-return presence-relation publishes.
//
// Covariant exposure is intentionally not part of this transaction. It is a
// whole-point finalizer and must run after path assignment, static-member write,
// and return operations have completed.
//
// This extraction adds no cancellation boundary. In particular, provider and
// object-entry callbacks do not yet poll a token; adding cancellation here
// requires whole-node rollback rather than returning a partially published
// transaction.
func ApplyConcreteRootAssignmentPoint(req ConcreteRootAssignmentPointRequest) ConcreteRootAssignmentPointResult {
	out, applied := applyRootAssignmentFact(
		req.Context,
		req.Resolver,
		req.Facts,
		req.Sources,
		req.Read,
		req.Input,
		req.Output,
		req.Assignment,
		req.ClosedDynamicAllValues,
		req.TypeValues,
	)
	if !applied {
		return ConcreteRootAssignmentPointResult{Output: out}
	}
	targetPath := req.Assignment.TargetPathRef()
	source := req.Assignment.Source()
	out = applyCallOutcomeReturnSlotFactsAfterRootAssignment(
		req.Context,
		req.Facts,
		req.CallOutcome,
		req.Resolver,
		req.ProjectPath,
		req.CovariantWiden,
		req.TypeValues,
		req.Read,
		req.Input,
		out,
		targetPath,
		source,
	)
	out = applyCallOutcomePresenceRelationPublishes(
		req.Context,
		req.Facts,
		req.presenceCache,
		req.CallOutcome,
		req.Resolver,
		req.Read,
		out,
	)
	return ConcreteRootAssignmentPointResult{Output: out, Applied: true}
}

// ConcretePointFinalizerRequest contains the operations that observe the
// completed point rather than one assignment transaction.
type ConcretePointFinalizerRequest struct {
	Context        transfer.NodeContext
	Resolver       *visibility.Resolver
	Facts          factflow.Facts
	CovariantWiden CovariantWiden
	Output         state.State
}

// FinalizeConcretePoint applies whole-point effects after all point-local
// assignments, writes, and return facts. Keeping this phase separate prevents
// a root-only reusable transaction from moving covariant widening ahead of a
// later same-point operation.
func FinalizeConcretePoint(req ConcretePointFinalizerRequest) state.State {
	return applyCovariantExposures(req.Context, req.Resolver, req.CovariantWiden, req.Facts, req.Output)
}
