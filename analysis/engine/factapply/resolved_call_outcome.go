package factapply

import (
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// ResolvedCallOutcomeOrdinaryEffectsRequest applies only the ordinary call-node
// effects of an outcome already resolved for Site. It is intentionally not a
// complete "resolve once" call boundary: result publication, filtered
// return-slot facts at later root/path assignments, and return-presence
// implication sidecars keep their existing provider reads because each uses a
// distinct, state-sensitive input.
//
// Provider selection and argument/result materialization remain outside. The
// caller must invoke this helper inside the existing outer node transaction;
// Apply introduces no cancellation boundary and reports no cancellation state.
// It also performs no cloning, freezing, or caching.
type ResolvedCallOutcomeOrdinaryEffectsRequest struct {
	Context     transfer.NodeContext
	Facts       factflow.Facts
	Resolver    *visibility.Resolver
	ProjectPath PathTypeProjector
	Widen       CovariantWiden
	TypeValues  *typevalue.Cache
	Output      state.State
	Site        factflow.CallSiteView
	Outcome     callpayload.CallOutcome
}

// ResolvedCallOutcomeOrdinaryEffectsResult reports whether ordinary effects
// were applied. Identity-keyed heap payload is rejected because the public
// request carries no trusted caller-keyspace provenance; rejection leaves
// Output exactly unchanged.
type ResolvedCallOutcomeOrdinaryEffectsResult struct {
	Output  state.State
	Applied bool
}

// ResolvedCallOutcomeOrdinaryEffectsExecutor is the provider-independent
// concrete ordinary-effects executor. It currently retains no scratch.
type ResolvedCallOutcomeOrdinaryEffectsExecutor struct{}

// ApplyResolvedCallOutcomeOrdinaryEffects applies ordinary effects with a
// fresh executor.
func ApplyResolvedCallOutcomeOrdinaryEffects(req ResolvedCallOutcomeOrdinaryEffectsRequest) ResolvedCallOutcomeOrdinaryEffectsResult {
	if callOutcomeHasIdentityKeyedHeap(req.Outcome) {
		return ResolvedCallOutcomeOrdinaryEffectsResult{Output: req.Output}
	}
	return ResolvedCallOutcomeOrdinaryEffectsResult{
		Output: applyResolvedCallOutcomeFacts(
			req.Context, req.Facts, req.Resolver, req.ProjectPath, req.Widen,
			req.TypeValues, req.Output, req.Site, req.Outcome,
		),
		Applied: true,
	}
}

// Apply executes the legacy ordinary-effect phase order. See the request's
// scope and cancellation contract.
func (*ResolvedCallOutcomeOrdinaryEffectsExecutor) Apply(req ResolvedCallOutcomeOrdinaryEffectsRequest) ResolvedCallOutcomeOrdinaryEffectsResult {
	return ApplyResolvedCallOutcomeOrdinaryEffects(req)
}

func callOutcomeHasIdentityKeyedHeap(outcome callpayload.CallOutcome) bool {
	return len(outcome.HeapTableObjects) != 0 || len(outcome.Placements) != 0
}

// ResolvedCallOutcomeEdgeRequest applies the edge-sensitive lanes of one
// already-resolved outcome at one call site. Provider traversal and the
// callOutcomeEdgeMayUseOutcome eligibility check deliberately remain outside:
// this helper never selects or resolves a provider. ReturnPresenceRelations
// also have a separate assignment sidecar with a distinct provider input; this
// request covers only their edge use and is not a resolve-once call boundary.
// Cancellation remains owned by the surrounding branch-edge executor.
type ResolvedCallOutcomeEdgeRequest struct {
	Context           transfer.EdgeContext
	Facts             factflow.Facts
	Resolver          *visibility.Resolver
	ProjectPath       PathTypeProjector
	BranchRefinements []factflow.BranchRefinement
	Output            state.State
	CallPoint         cfg.Point
	Site              factflow.CallSiteView
	Outcome           callpayload.CallOutcome
}

// ResolvedCallOutcomeEdgeExecutor retains only graph traversal scratch. It is
// safe to reuse sequentially for one prepared transfer; it does not retain an
// outcome or provider result.
type ResolvedCallOutcomeEdgeExecutor struct {
	cache callOutcomeTraversalCache
}

// ApplyResolvedCallOutcomeEdge applies one resolved outcome with fresh
// traversal scratch.
func ApplyResolvedCallOutcomeEdge(req ResolvedCallOutcomeEdgeRequest) state.State {
	return new(ResolvedCallOutcomeEdgeExecutor).Apply(req)
}

// Apply preserves the existing edge lane order: condition refinements,
// condition-slot refinements, then return-presence relations. It introduces no
// cancellation boundary of its own.
func (e *ResolvedCallOutcomeEdgeExecutor) Apply(req ResolvedCallOutcomeEdgeRequest) state.State {
	out := req.Output
	if len(req.Outcome.ReturnConditionRefinements) != 0 {
		out = applyCallReturnConditionRefinements(req.Context, req.Facts, req.Resolver, req.ProjectPath, req.CallPoint, req.Site, req.Outcome, out)
	}
	if len(req.Outcome.ReturnConditionSlots) != 0 {
		out = applyCallReturnConditionSlotRefinements(req.Context, req.Facts, &e.cache, req.Resolver, req.ProjectPath, req.BranchRefinements, req.CallPoint, req.Site, req.Outcome, out)
	}
	if len(req.Outcome.ReturnPresenceRelations) != 0 {
		out = applyCallReturnPresenceRelations(req.Context, req.Facts, &e.cache, req.Resolver, req.ProjectPath, req.BranchRefinements, req.CallPoint, req.Site, req.Outcome, out)
	}
	return out
}
