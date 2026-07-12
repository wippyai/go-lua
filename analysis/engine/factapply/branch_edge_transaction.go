package factapply

import (
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
)

// ConcreteBranchEdgePointRequest is the complete concrete transaction for one
// conditional CFG edge. The executor deliberately owns the ordering between
// guard refinement, implication closure, scalar/path facts, evidence, and call
// effects: later phases are allowed to observe every preceding phase.
type ConcreteBranchEdgePointRequest struct {
	Context     transfer.EdgeContext
	Facts       factflow.Facts
	Sources     sourcevalue.SourceValues
	CallOutcome callpayload.CallOutcomeProvider
	Resolver    *visibility.Resolver
	ProjectPath PathTypeProjector
	TypeValues  *typevalue.Cache
	Output      state.State
}

// ConcreteBranchEdgePointResult reports the evolving edge output. Canceled is
// true only for cooperative cancellation; unlike node transactions, an edge
// transaction never rolls completed phases back.
type ConcreteBranchEdgePointResult struct {
	Output   state.State
	Canceled bool
}

// ConcreteBranchEdgePointExecutor retains traversal scratch which is safe to
// reuse across sequential edge applications by one prepared transfer.
type ConcreteBranchEdgePointExecutor struct {
	callOutcomeCache callOutcomeTraversalCache
}

// ApplyConcreteBranchEdgePoint executes one edge with fresh traversal scratch.
func ApplyConcreteBranchEdgePoint(req ConcreteBranchEdgePointRequest) ConcreteBranchEdgePointResult {
	return new(ConcreteBranchEdgePointExecutor).Apply(req)
}

// Apply executes the exact concrete branch-edge sequence.
func (e *ConcreteBranchEdgePointExecutor) Apply(req ConcreteBranchEdgePointRequest) ConcreteBranchEdgePointResult {
	ctx := req.Context
	out := req.Output
	token := tokenOf(ctx.Session)
	canceled := func() ConcreteBranchEdgePointResult {
		return ConcreteBranchEdgePointResult{Output: out, Canceled: true}
	}
	done := func() ConcreteBranchEdgePointResult {
		return ConcreteBranchEdgePointResult{Output: out}
	}
	if token != nil && token.Canceled() {
		return canceled()
	}
	if !ctx.HasCond {
		return done()
	}
	if req.Facts.BranchEdgeUnreachable(ctx.Edge.From, ctx.Edge.Cond) ||
		branchConditionEdgeUnreachable(ctx, req.Facts, req.Sources, out) {
		out = unreachableState(ctx.Registry)
		return done()
	}

	branchRefinements := req.Facts.BranchRefinements(ctx.Edge.From)
	unreachable := false
	interrupted := false
	poll := cancellation.NewPoller(token, cancellation.EveryCheap)
	req.Facts.ForEachBranchPathEvidence(ctx.Edge.From, func(proof factflow.BranchPathEvidence) bool {
		if poll.Poll() {
			interrupted = true
			return false
		}
		if proof.Kind() == factflow.BranchPathEvidenceTruthy &&
			proof.ActiveOnEdge(ctx.Edge.Cond) &&
			branchTruthyEvidenceContradictsCurrentValue(req.TypeValues, ctx.Registry, req.Resolver, req.ProjectPath, ctx.Edge.From, out, proof.PathRef()) {
			unreachable = true
			return false
		}
		if proof.Kind() == factflow.BranchPathEvidenceTruthy &&
			proof.ActiveOnEdge(!ctx.Edge.Cond) &&
			!proof.ActiveOnEdge(ctx.Edge.Cond) &&
			proof.OppositeEdgeImpliesFalsy() &&
			branchFalsyEvidenceContradictsCurrentValue(req.TypeValues, ctx.Registry, req.Resolver, req.ProjectPath, ctx.Edge.From, out, proof.PathRef()) {
			unreachable = true
			return false
		}
		return true
	})
	if interrupted {
		return canceled()
	}
	if unreachable {
		out = unreachableState(ctx.Registry)
		return done()
	}

	activeRefinements := activeBranchRefinementsForEdge(branchRefinements, ctx.Edge.Cond)
	for _, fact := range activeRefinements {
		if poll.Poll() {
			return canceled()
		}
		targetPath := fact.targetPath
		if activeBranchRefinementHasStrictPrefix(activeRefinements, targetPath) {
			if invalidated, ok := invalidatePathSubtreeAt(out, req.Resolver, ctx.Edge.From, targetPath); ok {
				out = invalidated
			}
		}
		out = applyBranchRefinementCached(req.TypeValues, ctx, req.Resolver, req.ProjectPath, out, targetPath, fact.refinement)
		if stateIsBottom(ctx.Registry, out) {
			return done()
		}
	}
	implicationResult := ApplyConcretePresenceImplications(ConcretePresenceImplicationRequest{
		Registry: ctx.Registry,
		Resolver: req.Resolver,
		Point:    ctx.Edge.From,
		Output:   out,
		Token:    token,
	})
	out = implicationResult.Output
	if implicationResult.Canceled {
		return canceled()
	}

	for _, fact := range req.Facts.BranchLenRefinements(ctx.Edge.From) {
		if poll.Poll() {
			return canceled()
		}
		if fact.Cond() == ctx.Edge.Cond {
			out = applyBranchLenRefinement(ctx, req.Resolver, out, fact)
		}
	}
	for _, fact := range req.Facts.BranchNumFloorRefinements(ctx.Edge.From) {
		if poll.Poll() {
			return canceled()
		}
		if fact.Cond() == ctx.Edge.Cond {
			out = applyBranchNumFloorRefinement(ctx, req.Resolver, out, fact)
		}
	}
	for _, fact := range req.Facts.BranchNumCeilRefinements(ctx.Edge.From) {
		if poll.Poll() {
			return canceled()
		}
		if fact.Cond() == ctx.Edge.Cond {
			out = applyBranchNumCeilRefinement(ctx, req.Resolver, out, fact)
		}
	}
	for _, fact := range req.Facts.BranchDiffConstraints(ctx.Edge.From) {
		if poll.Poll() {
			return canceled()
		}
		if fact.Cond() == ctx.Edge.Cond {
			out = applyBranchDiffConstraint(ctx, req.Resolver, out, fact)
		}
	}
	for _, relation := range req.Facts.BranchPresenceRelations(ctx.Edge.From) {
		if poll.Poll() {
			return canceled()
		}
		refinement, ok := branchPresenceRelationRefinement(req.TypeValues, ctx, req.Resolver, req.ProjectPath, out, branchRefinements, relation)
		if ok {
			out = applyBranchRefinementCached(req.TypeValues, ctx, req.Resolver, req.ProjectPath, out, relation.TargetPathRef(), refinement)
		}
	}
	for _, relation := range req.Facts.BranchPathRelations(ctx.Edge.From) {
		if poll.Poll() {
			return canceled()
		}
		if !relation.ActiveOnEdge(ctx.Edge.Cond) {
			continue
		}
		out = ApplyConcreteBranchPathRelation(ConcreteBranchPathRelationRequest{
			Context: ctx, Resolver: req.Resolver, ProjectPath: req.ProjectPath,
			TypeValues: req.TypeValues, Output: out, Kind: relation.Kind(),
			LeftPath: relation.LeftPath(), RightPath: relation.RightPath(),
		})
		if stateIsBottom(ctx.Registry, out) {
			return done()
		}
	}

	interrupted = false
	req.Facts.ForEachBranchPathEvidence(ctx.Edge.From, func(proof factflow.BranchPathEvidence) bool {
		if poll.Poll() {
			interrupted = true
			return false
		}
		if proof.Kind() == factflow.BranchPathEvidenceTruthy &&
			proof.ActiveOnEdge(!ctx.Edge.Cond) && !proof.ActiveOnEdge(ctx.Edge.Cond) &&
			proof.OppositeEdgeImpliesFalsy() {
			out = applyDescendantTruthyOppositeRootOriginRefinement(req.TypeValues, ctx.Registry, req.Resolver, ctx.Edge.From, out, proof.PathRef())
		}
		if !proof.ActiveOnEdge(ctx.Edge.Cond) {
			return true
		}
		out = applyBranchIndexStaticLengthCeil(req.TypeValues, ctx, req.Resolver, req.ProjectPath, out, proof)
		out = applyBranchPathEvidence(req.TypeValues, ctx, req.Resolver, req.ProjectPath, out, proof)
		return !stateIsBottom(ctx.Registry, out)
	})
	if interrupted {
		return canceled()
	}
	if stateIsBottom(ctx.Registry, out) {
		return done()
	}
	out = applyCallOutcomeEdgeFacts(ctx, req.Facts, &e.callOutcomeCache, req.CallOutcome, req.Resolver, req.ProjectPath, branchRefinements, out)
	return done()
}
