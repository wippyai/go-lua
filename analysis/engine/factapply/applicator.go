package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// PathTypeProjector projects a path through a structural root type. Engine
// packages receive this from the language/check layer so they stay syntax-free.
type PathTypeProjector func(root typ.Type, path pathdom.Path) (typ.Type, bool)

// CovariantWiden rebuilds a covariantly-exposed object's witness type. Given the
// object's current witness type, the exposure contract type, and the source path
// segments locating the exposed sub-object under its ancestor symbol, it returns
// the ancestor witness type with every strictly-wider field widened to the
// contract and the deduplicated top segment of every widened leaf (so the engine
// can drop the precise per-field facts beneath it). It returns ok=false when no
// field widens. Engine packages receive this from the check layer to keep the
// subtype/unwrap reasoning out of the engine.
type CovariantWiden func(sourceWitness, contract typ.Type, segments []segment.Segment) (widened typ.Type, topWidenedSegments [][]segment.Segment, ok bool)

// FactsNodeTransferConfig configures the generic fact applicator.
type FactsNodeTransferConfig struct {
	Facts          factflow.Facts
	Sources        sourcevalue.SourceValues
	CallOutcome    callpayload.CallOutcomeProvider
	Visibility     *visibility.Resolver
	ProjectPath    PathTypeProjector
	CovariantWiden CovariantWiden
	TypeValues     *typevalue.Cache
}

// FactsEdgeTransferConfig configures the generic edge fact applicator.
type FactsEdgeTransferConfig struct {
	Facts       factflow.Facts
	CallOutcome callpayload.CallOutcomeProvider
	Visibility  *visibility.Resolver
	ProjectPath PathTypeProjector
	TypeValues  *typevalue.Cache
}

// NewFactsNodeTransfer returns a generic node transfer that applies point-local
// transfer facts. It intentionally handles only root assignment, member/path
// assignment, descendant path invalidation, call return-slot production, and
// return-slot facts; branch-edge refinements are handled by NewFactsEdgeTransfer.
func NewFactsNodeTransfer(config FactsNodeTransferConfig) transfer.NodeTransfer {
	expressionRefinements := config.Facts.ExpressionRefinements()
	callOutcomeCache := &callOutcomeTraversalCache{}
	return func(ctx transfer.NodeContext, in state.State) state.State {
		facts := config.Facts
		sources := config.Sources
		callOutcome := config.CallOutcome
		read, materialize := callResultReader(ctx, facts, sources, callOutcome, config.Visibility, config.ProjectPath, config.CovariantWiden, config.TypeValues)

		out := materialize(ctx.Point, in)
		if facts.NoNormalReturn(ctx.Point) {
			return state.State{}
		}
		if fact, ok := facts.PathDescendantInvalidation(ctx.Point); ok {
			_, directDynamicWrite := facts.DynamicIndexWrite(ctx.Point)
			out = applyPathDescendantInvalidation(ctx, config.Visibility, out, fact, !directDynamicWrite)
		}
		for _, fact := range facts.PostconditionRefinements(ctx.Point) {
			out = applyValueRefinementAtCached(config.TypeValues, ctx.Registry, config.Visibility, config.ProjectPath, ctx.Point, out, fact.TargetPath(), fact.Value())
		}
		for _, fact := range facts.PostconditionPathRelations(ctx.Point) {
			out = applyPostconditionPathRelation(ctx, config.Visibility, config.ProjectPath, out, fact)
		}
		for _, fact := range facts.ChannelSelects(ctx.Point) {
			out = applyChannelSelect(ctx, config.Visibility, out, fact)
		}
		if sources == nil {
			return out
		}
		sources = sourcevalue.WithExpressionRefinements(ctx.Registry, sources, expressionRefinements)
		if fact, ok := facts.DynamicIndexWrite(ctx.Point); ok {
			out = applyDynamicIndexWrite(ctx, config.Visibility, facts, sources, read, in, out, fact)
		}
		if fact, ok := facts.RootAssignment(ctx.Point); ok {
			var applied bool
			out, applied = applyRootAssignmentFact(ctx, config.Visibility, facts, sources, read, in, out, fact)
			if applied {
				out = applyCallOutcomePresenceRelationPublishes(ctx, facts, callOutcomeCache, callOutcome, config.Visibility, read, out)
			}
		}
		if fact, ok := facts.PathAssignment(ctx.Point); ok {
			var applied bool
			out, applied = applyPathAssignment(ctx, config.Visibility, sources, read, in, out, fact)
			if applied {
				out = applyObjectLiteralEntries(ctx, config.Visibility, facts, sources, read, in, out, fact.TargetPath(), fact.Source())
				out = applyCallOutcomePresenceRelationPublishes(ctx, facts, callOutcomeCache, callOutcome, config.Visibility, read, out)
			}
		}
		if fact, ok := facts.PathStaticMemberWrite(ctx.Point); ok {
			out = applyPathStaticMemberWrite(ctx, config.Visibility, facts, sources, read, in, out, fact)
		}
		if fact, ok := facts.Return(ctx.Point); ok {
			out = applyReturn(ctx, facts, sources, read, in, out, fact, config.Visibility, config.ProjectPath, config.TypeValues)
		}
		out = applyCovariantExposures(ctx, config.Visibility, config.CovariantWiden, facts, out)
		return out
	}
}

// NewFactsEdgeTransfer returns a generic edge transfer that applies
// branch refinements for the selected branch edge, including root-origin
// narrowing recovered from descendant path refinements when flow state carries
// the structural root type.
func NewFactsEdgeTransfer(config FactsEdgeTransferConfig) transfer.EdgeTransfer {
	callOutcomeCache := &callOutcomeTraversalCache{}
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
			out = applyBranchRefinementCached(config.TypeValues, ctx, config.Visibility, config.ProjectPath, out, fact.TargetPath(), refinement)
		}
		for _, fact := range config.Facts.BranchLenRefinements(ctx.Edge.From) {
			if fact.Cond() != ctx.Edge.Cond {
				continue
			}
			out = applyBranchLenRefinement(ctx, config.Visibility, out, fact)
		}
		for _, fact := range config.Facts.BranchNumFloorRefinements(ctx.Edge.From) {
			if fact.Cond() != ctx.Edge.Cond {
				continue
			}
			out = applyBranchNumFloorRefinement(ctx, config.Visibility, out, fact)
		}
		for _, fact := range config.Facts.BranchDiffConstraints(ctx.Edge.From) {
			if fact.Cond() != ctx.Edge.Cond {
				continue
			}
			out = applyBranchDiffConstraint(ctx, config.Visibility, out, fact)
		}
		for _, relation := range config.Facts.BranchPresenceRelations(ctx.Edge.From) {
			refinement, ok := branchPresenceRelationRefinement(config.TypeValues, ctx, config.Visibility, config.ProjectPath, out, branchRefinements, relation)
			if !ok {
				continue
			}
			out = applyBranchRefinementCached(config.TypeValues, ctx, config.Visibility, config.ProjectPath, out, relation.TargetPath(), refinement)
		}
		for _, relation := range config.Facts.BranchPathRelations(ctx.Edge.From) {
			if !relation.ActiveOnEdge(ctx.Edge.Cond) {
				continue
			}
			out = applyBranchPathRelation(config.TypeValues, ctx, config.Visibility, config.ProjectPath, out, relation)
		}
		for _, proof := range config.Facts.BranchPathEvidence(ctx.Edge.From) {
			if proof.Kind() == factflow.BranchPathEvidenceTruthy && proof.ActiveOnEdge(!ctx.Edge.Cond) && !proof.ActiveOnEdge(ctx.Edge.Cond) {
				out = applyDescendantTruthyOppositeRootOriginRefinement(
					config.TypeValues,
					ctx.Registry,
					config.Visibility,
					ctx.Edge.From,
					out,
					proof.Path(),
				)
			}
			if !proof.ActiveOnEdge(ctx.Edge.Cond) {
				continue
			}
			out = applyBranchPathEvidence(ctx, config.Visibility, config.ProjectPath, out, proof)
		}
		out = applyCallOutcomeEdgeFacts(ctx, config.Facts, callOutcomeCache, config.CallOutcome, config.Visibility, config.ProjectPath, branchRefinements, out)
		return out
	}
}
