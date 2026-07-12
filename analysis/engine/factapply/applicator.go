package factapply

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	valuerefine "github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
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

// ClosedDynamicAllValueInvariant states that every present value reachable via
// Container is a key of Table. The program layer infers these for closed
// reverse-map writer sets; fact application seeds them when the container is
// created as a fresh empty table.
type ClosedDynamicAllValueInvariant struct {
	Container pathdom.Path
	Table     pathdom.Path
}

// FactsNodeTransferConfig configures the generic fact applicator.
type FactsNodeTransferConfig struct {
	Facts                  factflow.Facts
	Sources                sourcevalue.SourceValues
	CallOutcome            callpayload.CallOutcomeProvider
	Visibility             *visibility.Resolver
	ProjectPath            PathTypeProjector
	CovariantWiden         CovariantWiden
	TypeValues             *typevalue.Cache
	ClosedDynamicAllValues []ClosedDynamicAllValueInvariant
}

// FactsEdgeTransferConfig configures the generic edge fact applicator.
type FactsEdgeTransferConfig struct {
	Facts       factflow.Facts
	Sources     sourcevalue.SourceValues
	CallOutcome callpayload.CallOutcomeProvider
	Visibility  *visibility.Resolver
	ProjectPath PathTypeProjector
	TypeValues  *typevalue.Cache
}

type activeBranchRefinement struct {
	targetPath pathdom.Path
	refinement factflow.ValueRefinement
}

func activeBranchRefinementsForEdge(refinements []factflow.BranchRefinement, cond bool) []activeBranchRefinement {
	if len(refinements) == 0 {
		return nil
	}
	out := make([]activeBranchRefinement, 0, len(refinements))
	for _, fact := range refinements {
		refinement, ok := fact.ValueForEdge(cond)
		if !ok {
			continue
		}
		out = append(out, activeBranchRefinement{
			targetPath: fact.TargetPathRef(),
			refinement: refinement,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return len(out[i].targetPath.Segments) < len(out[j].targetPath.Segments)
	})
	return out
}

func activeBranchRefinementHasStrictPrefix(refinements []activeBranchRefinement, target pathdom.Path) bool {
	if target.Symbol == 0 || len(target.Segments) == 0 {
		return false
	}
	for _, fact := range refinements {
		if fact.targetPath.Symbol != target.Symbol || len(fact.targetPath.Segments) >= len(target.Segments) {
			continue
		}
		if branchRefinementSegmentsHavePrefix(target.Segments, fact.targetPath.Segments) {
			return true
		}
	}
	return false
}

func branchRefinementSegmentsHavePrefix(target []segment.Segment, prefix []segment.Segment) bool {
	if len(prefix) > len(target) {
		return false
	}
	for i := range prefix {
		if target[i] != prefix[i] {
			return false
		}
	}
	return true
}

// NewFactsNodeTransfer returns a generic node transfer that applies point-local
// transfer facts. It intentionally handles only root assignment, member/path
// assignment, descendant path invalidation, call return-slot production, and
// return-slot facts; branch-edge refinements are handled by NewFactsEdgeTransfer.
func NewFactsNodeTransfer(config FactsNodeTransferConfig) transfer.NodeTransfer {
	expressionRefinements := sourcevalue.NewExpressionRefinementsFromReader(config.Facts)
	callOutcomeCache := &callOutcomeTraversalCache{}
	var refinedSourceRegistry *axis.Registry
	var refinedSources sourcevalue.SourceValues
	return func(ctx transfer.NodeContext, in state.State) state.State {
		if ctx.Session != nil && ctx.Session.Token().Canceled() {
			return in
		}
		facts := config.Facts
		sources := config.Sources
		callOutcome := config.CallOutcome
		var callResults *lazyCallResultReader
		ensureCallResults := func() *lazyCallResultReader {
			if callResults == nil {
				callResults = &lazyCallResultReader{
					ctx:             ctx,
					facts:           facts,
					sources:         sources,
					outcomeProvider: callOutcome,
					resolver:        config.Visibility,
					projectPath:     config.ProjectPath,
					widen:           config.CovariantWiden,
					typeValues:      config.TypeValues,
				}
			}
			return callResults
		}

		out := in
		if sources != nil {
			if refinedSources == nil || refinedSourceRegistry != ctx.Registry {
				refinedSources = expressionRefinements.Bind(ctx.Registry, sources)
				refinedSourceRegistry = ctx.Registry
			}
			sources = refinedSources
		}
		if nodeHasCallMaterializationFacts(facts, ctx.Point) {
			out = ensureCallResults().Materialize(ctx.Point, in)
		}
		if facts.NoNormalReturn(ctx.Point) {
			return state.State{}
		}
		pathImplicationsPending := false
		flushPathImplications := func() {
			if !pathImplicationsPending {
				return
			}
			out = activatePathPresenceImplicationsWithToken(ctx.Registry, config.Visibility, ctx.Point, out, tokenOf(ctx.Session))
			pathImplicationsPending = false
		}
		poll := cancellation.NewPoller(tokenOf(ctx.Session), cancellation.EveryCheap)
		for _, fact := range facts.PathValuePresenceImplications(ctx.Point) {
			if poll.Poll() {
				return in
			}
			implication, ok := pathValuePresenceImplicationAt(ctx, config.Visibility, fact)
			if !ok {
				continue
			}
			if presenceImplicationTargetInvalidatesDescendants(implication) {
				flushPathImplications()
				out = out.AddPathPresenceImplication(implication)
				out = activatePathPresenceImplicationsWithToken(ctx.Registry, config.Visibility, ctx.Point, out, tokenOf(ctx.Session))
				continue
			}
			out = out.AddPathPresenceImplication(implication)
			pathImplicationsPending = true
		}
		flushPathImplications()
		if fact, ok := facts.PathDescendantInvalidation(ctx.Point); ok {
			_, directDynamicWrite := facts.DynamicIndexWrite(ctx.Point)
			out = applyPathDescendantInvalidation(ctx, config.Visibility, facts, sources, ensureCallResults().ReadLazy(), in, out, fact, !directDynamicWrite)
		}
		for _, fact := range facts.PostconditionRefinements(ctx.Point) {
			if poll.Poll() {
				return in
			}
			out = applyValueRefinementAtCached(config.TypeValues, ctx.Registry, config.Visibility, config.ProjectPath, ctx.Point, out, fact.TargetPathRef(), fact.Value())
		}
		for _, fact := range facts.PostconditionPathRelations(ctx.Point) {
			if poll.Poll() {
				return in
			}
			out = applyPostconditionPathRelation(ctx, config.Visibility, config.ProjectPath, out, fact)
		}
		for _, fact := range facts.ChannelSelects(ctx.Point) {
			if poll.Poll() {
				return in
			}
			out = applyChannelSelect(ctx, config.Visibility, out, fact)
		}
		if sources == nil {
			return out
		}
		if fact, ok := facts.DynamicIndexWrite(ctx.Point); ok {
			out = applyDynamicIndexWrite(ctx, config.Visibility, facts, sources, ensureCallResults().ReadLazy(), in, out, fact)
		}
		if fact, ok := facts.RootAssignment(ctx.Point); ok {
			var applied bool
			out, applied = applyRootAssignmentFact(ctx, config.Visibility, facts, sources, ensureCallResults().ReadLazy(), in, out, fact, config.ClosedDynamicAllValues, config.TypeValues)
			if applied {
				out = applyCallOutcomeReturnSlotFactsAfterRootAssignment(ctx, facts, callOutcome, config.Visibility, config.ProjectPath, config.CovariantWiden, config.TypeValues, ensureCallResults().ReadLazy(), in, out, fact.TargetPathRef(), fact.Source())
				out = applyCallOutcomePresenceRelationPublishes(ctx, facts, callOutcomeCache, callOutcome, config.Visibility, ensureCallResults().ReadLazy(), out)
			}
		}
		if fact, ok := facts.PathAssignment(ctx.Point); ok {
			var applied bool
			out, applied = ApplyConcretePathAssignment(ConcretePathAssignmentRequest{
				Context:    ctx,
				Resolver:   config.Visibility,
				Facts:      facts,
				Sources:    sources,
				Read:       ensureCallResults().ReadLazy(),
				Input:      in,
				Output:     out,
				Assignment: fact,
			})
			if applied {
				out = applyObjectLiteralEntries(ctx, config.Visibility, facts, sources, ensureCallResults().ReadLazy(), in, out, fact.TargetPathRef(), fact.Source(), config.TypeValues)
				out = applyCallOutcomePresenceRelationPublishes(ctx, facts, callOutcomeCache, callOutcome, config.Visibility, ensureCallResults().ReadLazy(), out)
			}
		}
		if fact, ok := facts.PathStaticMemberWrite(ctx.Point); ok {
			out = applyPathStaticMemberWrite(ctx, config.Visibility, facts, sources, ensureCallResults().ReadLazy(), in, out, fact)
		}
		if fact, ok := facts.Return(ctx.Point); ok {
			out = applyReturn(ctx, facts, sources, ensureCallResults().ReadLazy(), in, out, fact, config.Visibility, config.ProjectPath, config.TypeValues)
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
		if ctx.Session != nil && ctx.Session.Token().Canceled() {
			return out
		}
		if !ctx.HasCond {
			return out
		}
		if config.Facts.BranchEdgeUnreachable(ctx.Edge.From, ctx.Edge.Cond) {
			return unreachableState(ctx.Registry)
		}
		if branchConditionEdgeUnreachable(ctx, config.Facts, config.Sources, out) {
			return unreachableState(ctx.Registry)
		}
		branchRefinements := config.Facts.BranchRefinements(ctx.Edge.From)
		unreachable := false
		poll := cancellation.NewPoller(tokenOf(ctx.Session), cancellation.EveryCheap)
		config.Facts.ForEachBranchPathEvidence(ctx.Edge.From, func(proof factflow.BranchPathEvidence) bool {
			if poll.Poll() {
				return false
			}
			if proof.Kind() == factflow.BranchPathEvidenceTruthy &&
				proof.ActiveOnEdge(ctx.Edge.Cond) &&
				branchTruthyEvidenceContradictsCurrentValue(config.TypeValues, ctx.Registry, config.Visibility, config.ProjectPath, ctx.Edge.From, out, proof.PathRef()) {
				unreachable = true
				return false
			}
			if proof.Kind() == factflow.BranchPathEvidenceTruthy &&
				proof.ActiveOnEdge(!ctx.Edge.Cond) &&
				!proof.ActiveOnEdge(ctx.Edge.Cond) &&
				proof.OppositeEdgeImpliesFalsy() &&
				branchFalsyEvidenceContradictsCurrentValue(config.TypeValues, ctx.Registry, config.Visibility, config.ProjectPath, ctx.Edge.From, out, proof.PathRef()) {
				unreachable = true
				return false
			}
			return true
		})
		if unreachable {
			return unreachableState(ctx.Registry)
		}
		activeRefinements := activeBranchRefinementsForEdge(branchRefinements, ctx.Edge.Cond)
		for _, fact := range activeRefinements {
			if poll.Poll() {
				return out
			}
			targetPath := fact.targetPath
			if activeBranchRefinementHasStrictPrefix(activeRefinements, targetPath) {
				if invalidated, ok := invalidatePathSubtreeAt(out, config.Visibility, ctx.Edge.From, targetPath); ok {
					out = invalidated
				}
			}
			out = applyBranchRefinementCached(config.TypeValues, ctx, config.Visibility, config.ProjectPath, out, targetPath, fact.refinement)
			if stateIsBottom(ctx.Registry, out) {
				return out
			}
		}
		out = activatePathPresenceImplicationsWithToken(ctx.Registry, config.Visibility, ctx.Edge.From, out, tokenOf(ctx.Session))
		for _, fact := range config.Facts.BranchLenRefinements(ctx.Edge.From) {
			if poll.Poll() {
				return out
			}
			if fact.Cond() != ctx.Edge.Cond {
				continue
			}
			out = applyBranchLenRefinement(ctx, config.Visibility, out, fact)
		}
		for _, fact := range config.Facts.BranchNumFloorRefinements(ctx.Edge.From) {
			if poll.Poll() {
				return out
			}
			if fact.Cond() != ctx.Edge.Cond {
				continue
			}
			out = applyBranchNumFloorRefinement(ctx, config.Visibility, out, fact)
		}
		for _, fact := range config.Facts.BranchNumCeilRefinements(ctx.Edge.From) {
			if poll.Poll() {
				return out
			}
			if fact.Cond() != ctx.Edge.Cond {
				continue
			}
			out = applyBranchNumCeilRefinement(ctx, config.Visibility, out, fact)
		}
		for _, fact := range config.Facts.BranchDiffConstraints(ctx.Edge.From) {
			if poll.Poll() {
				return out
			}
			if fact.Cond() != ctx.Edge.Cond {
				continue
			}
			out = applyBranchDiffConstraint(ctx, config.Visibility, out, fact)
		}
		for _, relation := range config.Facts.BranchPresenceRelations(ctx.Edge.From) {
			if poll.Poll() {
				return out
			}
			refinement, ok := branchPresenceRelationRefinement(config.TypeValues, ctx, config.Visibility, config.ProjectPath, out, branchRefinements, relation)
			if !ok {
				continue
			}
			out = applyBranchRefinementCached(config.TypeValues, ctx, config.Visibility, config.ProjectPath, out, relation.TargetPathRef(), refinement)
		}
		for _, relation := range config.Facts.BranchPathRelations(ctx.Edge.From) {
			if poll.Poll() {
				return out
			}
			if !relation.ActiveOnEdge(ctx.Edge.Cond) {
				continue
			}
			out = ApplyConcreteBranchPathRelation(ConcreteBranchPathRelationRequest{
				Context:     ctx,
				Resolver:    config.Visibility,
				ProjectPath: config.ProjectPath,
				TypeValues:  config.TypeValues,
				Output:      out,
				Kind:        relation.Kind(),
				LeftPath:    relation.LeftPath(),
				RightPath:   relation.RightPath(),
			})
			if stateIsBottom(ctx.Registry, out) {
				return out
			}
		}
		config.Facts.ForEachBranchPathEvidence(ctx.Edge.From, func(proof factflow.BranchPathEvidence) bool {
			if poll.Poll() {
				return false
			}
			if proof.Kind() == factflow.BranchPathEvidenceTruthy &&
				proof.ActiveOnEdge(!ctx.Edge.Cond) &&
				!proof.ActiveOnEdge(ctx.Edge.Cond) &&
				proof.OppositeEdgeImpliesFalsy() {
				out = applyDescendantTruthyOppositeRootOriginRefinement(
					config.TypeValues,
					ctx.Registry,
					config.Visibility,
					ctx.Edge.From,
					out,
					proof.PathRef(),
				)
			}
			if !proof.ActiveOnEdge(ctx.Edge.Cond) {
				return true
			}
			out = applyBranchIndexStaticLengthCeil(config.TypeValues, ctx, config.Visibility, config.ProjectPath, out, proof)
			out = applyBranchPathEvidence(config.TypeValues, ctx, config.Visibility, config.ProjectPath, out, proof)
			return !stateIsBottom(ctx.Registry, out)
		})
		if stateIsBottom(ctx.Registry, out) {
			return out
		}
		out = applyCallOutcomeEdgeFacts(ctx, config.Facts, callOutcomeCache, config.CallOutcome, config.Visibility, config.ProjectPath, branchRefinements, out)
		return out
	}
}

func tokenOf(session *cancellation.Session) *cancellation.Token {
	if session == nil {
		return nil
	}
	return session.Token()
}

func branchConditionEdgeUnreachable(
	ctx transfer.EdgeContext,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	in state.State,
) bool {
	if sources == nil {
		return false
	}
	source, ok := facts.BranchConditionSource(ctx.Edge.From)
	if !ok {
		return false
	}
	value, ok := sources.ValueOfSource(ctx.Edge.From, source, in, ctx.Read)
	if !ok {
		return false
	}
	if product.Equal(ctx.Registry, value, product.Bottom(ctx.Registry)) {
		return false
	}
	if ctx.Edge.Cond {
		return !valuerefine.CanBeTruthy(ctx.Registry, value)
	}
	return !valuerefine.CanBeFalsy(ctx.Registry, value)
}

func stateIsBottom(reg *axis.Registry, st state.State) bool {
	return state.IsBottom(reg, st)
}

func unreachableState(reg *axis.Registry) state.State {
	return state.Domain(reg).Bottom()
}

func branchTruthyEvidenceContradictsCurrentValue(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
) bool {
	current, ok := resolvePathValueAtCached(typeValues, reg, resolver, point, out, targetPath, projectPath)
	if !ok || product.Equal(reg, current.value, product.Bottom(reg)) {
		return false
	}
	return !valuerefine.CanBeTruthy(reg, current.value)
}

func branchFalsyEvidenceContradictsCurrentValue(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
) bool {
	current, ok := resolvePathValueAtCached(typeValues, reg, resolver, point, out, targetPath, projectPath)
	if !ok || product.Equal(reg, current.value, product.Bottom(reg)) {
		return false
	}
	return !valuerefine.CanBeFalsy(reg, current.value)
}
