package apply

import (
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
)

// FactsNodeTransferConfig configures the generic fact applicator.
type FactsNodeTransferConfig struct {
	Facts       factflow.Facts
	Sources     sourcevalue.SourceValues
	CallResults CallResultProvider
	Visibility  *visibility.Resolver
}

// FactsEdgeTransferConfig configures the generic edge fact applicator.
type FactsEdgeTransferConfig struct {
	Facts      factflow.Facts
	Visibility *visibility.Resolver
}

// NewFactsNodeTransfer returns a generic node transfer that applies point-local
// transfer facts. It intentionally handles only root assignment, member/path
// assignment, descendant path invalidation, call return-slot production, and
// return-slot facts; branch-edge refinements are handled by NewFactsEdgeTransfer.
func NewFactsNodeTransfer(config FactsNodeTransferConfig) transfer.NodeTransfer {
	return func(ctx transfer.NodeContext, in state.State) state.State {
		facts := config.Facts
		sources := config.Sources
		callResults := config.CallResults
		read, materialize := callResultReader(ctx, facts, callResults)

		out := materialize(ctx.Point, in)
		if fact, ok := facts.PathDescendantInvalidation(ctx.Point); ok {
			out = applyPathDescendantInvalidation(ctx, config.Visibility, out, fact)
		}
		if sources == nil {
			return out
		}
		sources = sourcevalue.WithValueOverlays(ctx.Registry, sources, facts.ValueOverlays())
		if fact, ok := facts.LocalAssignment(ctx.Point); ok {
			out = applyRootAssignmentFact(ctx, config.Visibility, facts, sources, read, in, out, fact)
		}
		if fact, ok := facts.OrdinaryAssignment(ctx.Point); ok {
			out = applyRootAssignmentFact(ctx, config.Visibility, facts, sources, read, in, out, fact)
		}
		if fact, ok := facts.PathAssignment(ctx.Point); ok {
			var applied bool
			out, applied = applyPathAssignment(ctx, config.Visibility, sources, read, in, out, fact)
			if applied {
				out = applyObjectLiteralEntries(ctx, config.Visibility, facts, sources, read, in, out, fact.TargetPath(), fact.Source())
			}
		}
		if fact, ok := facts.Return(ctx.Point); ok {
			out = applyReturn(ctx, sources, read, in, out, fact)
		}
		return out
	}
}

// NewFactsEdgeTransfer returns a generic edge transfer that applies
// branch refinements for the selected branch edge.
func NewFactsEdgeTransfer(config FactsEdgeTransferConfig) transfer.EdgeTransfer {
	return func(ctx transfer.EdgeContext, out state.State) state.State {
		if !ctx.HasCond {
			return out
		}
		branchRefinements := config.Facts.BranchRefinements(ctx.Edge.From)
		for _, fact := range branchRefinements {
			refinement, ok := fact.ValueForEdge(ctx.Edge.Cond)
			if !ok {
				continue
			}
			out = applyBranchRefinement(ctx, config.Visibility, out, fact.TargetPath(), refinement)
		}
		for _, relation := range config.Facts.BranchPresenceRelations(ctx.Edge.From) {
			refinement, ok := branchPresenceRelationRefinement(ctx, branchRefinements, relation)
			if !ok {
				continue
			}
			out = applyBranchRefinement(ctx, config.Visibility, out, relation.TargetPath(), refinement)
		}
		return out
	}
}
