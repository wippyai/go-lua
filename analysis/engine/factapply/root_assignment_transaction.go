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
// by ApplyConcreteRootAssignmentPoint. The assignment is derived from Facts at
// Context.Point so core and presence sidecars cannot observe inconsistent
// assignment descriptors; a missing fact fails closed with Output unchanged.
type ConcreteRootAssignmentPointRequest struct {
	Context                transfer.NodeContext
	Resolver               *visibility.Resolver
	Facts                  factflow.Facts
	Sources                sourcevalue.SourceValues
	Read                   func(cfg.Point) state.State
	Input                  state.State
	Output                 state.State
	CallOutcome            callpayload.CallOutcomeProvider
	ProjectPath            PathTypeProjector
	CovariantWiden         CovariantWiden
	TypeValues             *typevalue.Cache
	ClosedDynamicAllValues []ClosedDynamicAllValueInvariant
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
	return new(ConcreteRootAssignmentPointExecutor).Apply(req)
}

// ConcreteRootAssignmentPointExecutor retains graph traversal acceleration
// across root transactions without leaking cache representation into the
// public request. Its zero value is ready to use. An executor belongs to one
// prepared transfer closure and must not be used concurrently.
type ConcreteRootAssignmentPointExecutor struct {
	presenceCache *callOutcomeTraversalCache
}

// Apply executes one concrete root-assignment transaction. It is semantically
// identical to ApplyConcreteRootAssignmentPoint; the method form only reuses
// private traversal acceleration across calls.
func (e *ConcreteRootAssignmentPointExecutor) Apply(req ConcreteRootAssignmentPointRequest) ConcreteRootAssignmentPointResult {
	assignment, ok := req.Facts.RootAssignment(req.Context.Point)
	if !ok {
		return ConcreteRootAssignmentPointResult{Output: req.Output}
	}
	out, applied := applyRootAssignmentFact(
		req.Context,
		req.Resolver,
		req.Facts,
		req.Sources,
		req.Read,
		req.Input,
		req.Output,
		assignment,
		req.ClosedDynamicAllValues,
		req.TypeValues,
	)
	if !applied {
		return ConcreteRootAssignmentPointResult{Output: out}
	}
	targetPath := assignment.TargetPathRef()
	source := assignment.Source()
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
		e.callOutcomePresenceCache(),
		req.CallOutcome,
		req.Resolver,
		req.Read,
		out,
	)
	return ConcreteRootAssignmentPointResult{Output: out, Applied: true}
}

func (e *ConcreteRootAssignmentPointExecutor) callOutcomePresenceCache() *callOutcomeTraversalCache {
	if e.presenceCache == nil {
		e.presenceCache = &callOutcomeTraversalCache{}
	}
	return e.presenceCache
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
