package factapply

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
)

// ConcreteNodePointResult is the atomic result of applying every fact owned by
// one CFG node. Cancellation never publishes a prefix: Output is exactly the
// immutable point input whenever Canceled is true.
type ConcreteNodePointResult struct {
	Output   state.State
	Canceled bool
}

// ConcreteNodePointExecutor is the complete concrete node semantic kernel.
// It owns the N0..N6 ordering and the scratch whose lifetime is one prepared
// transfer. Its zero value is not useful; construct it with
// NewConcreteNodePointExecutor. An executor is sequential and not safe for
// concurrent Apply calls, matching transfer.NodeTransfer's ownership model.
//
// The executor is deliberately independent of the operation-plan fast runner.
// That runner may later select rows, but both the existing solver and any
// future runner must execute them through this transaction.
type ConcreteNodePointExecutor struct {
	config                FactsNodeTransferConfig
	expressionRefinements sourcevalue.ExpressionRefinements
	refinedSourceRegistry *axis.Registry
	refinedSources        sourcevalue.SourceValues
	callOutcomeCache      callOutcomeTraversalCache
	rootAssignments       ConcreteRootAssignmentPointExecutor
}

// NewConcreteNodePointExecutor prepares a behavior-neutral node transaction.
func NewConcreteNodePointExecutor(config FactsNodeTransferConfig) *ConcreteNodePointExecutor {
	executor := &ConcreteNodePointExecutor{
		config:                config,
		expressionRefinements: sourcevalue.NewExpressionRefinementsFromReader(config.Facts),
	}
	executor.rootAssignments.presenceCache = &executor.callOutcomeCache
	return executor
}

// Apply executes the exact established node sequence:
//
//	N0 call/channel materialization
//	N1 no-normal-return
//	N2 implication publication and closure
//	N3 descendant invalidation, postconditions, and channel facts
//	N4 dynamic/root/path/static writes and their composite sidecars
//	N5 return publication
//	N6 covariant finalization
//
// Input remains the source/provider snapshot throughout; Output evolves between
// barriers. The lazy call reader and its recursive materialization cache are
// created per Apply, exactly as before this extraction. Existing cooperative
// cancellation points roll the whole node back to Input; callback-boundary
// hardening is intentionally a separately tested semantic change.
func (e *ConcreteNodePointExecutor) Apply(ctx transfer.NodeContext, in state.State) ConcreteNodePointResult {
	if e == nil {
		return ConcreteNodePointResult{Output: in}
	}
	config := e.config
	facts := config.Facts
	sources := config.Sources
	callOutcome := config.CallOutcome
	token := tokenOf(ctx.Session)
	canceled := func() ConcreteNodePointResult {
		return ConcreteNodePointResult{Output: in, Canceled: true}
	}
	done := func(out state.State) ConcreteNodePointResult {
		return ConcreteNodePointResult{Output: out}
	}
	if token != nil && token.Canceled() {
		return canceled()
	}

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
		if e.refinedSources == nil || e.refinedSourceRegistry != ctx.Registry {
			e.refinedSources = e.expressionRefinements.Bind(ctx.Registry, sources)
			e.refinedSourceRegistry = ctx.Registry
		}
		sources = e.refinedSources
	}

	// N0: materialize calls before any node-local consumer observes their
	// return slots. The materializer owns one cache for this Apply only.
	if nodeHasCallMaterializationFacts(facts, ctx.Point) {
		out = ensureCallResults().Materialize(ctx.Point, in)
	}

	// N1 terminates the normal path only after N0 provider effects have run.
	if facts.NoNormalReturn(ctx.Point) {
		return done(state.State{})
	}

	// N2 publishes direct implications transactionally and closes them at the
	// descendant-invalidation barriers established by the legacy applicator.
	var pathImplicationPublications []pathevidence.PathPresenceImplication
	poll := cancellation.NewPoller(token, cancellation.EveryCheap)
	for _, fact := range facts.PathValuePresenceImplications(ctx.Point) {
		if poll.Poll() {
			return canceled()
		}
		implication, ok := pathValuePresenceImplicationAt(ctx, config.Visibility, fact)
		if ok {
			pathImplicationPublications = append(pathImplicationPublications, implication)
		}
	}
	if len(pathImplicationPublications) != 0 {
		result := ApplyConcretePresenceImplications(ConcretePresenceImplicationRequest{
			Registry:     ctx.Registry,
			Resolver:     config.Visibility,
			Point:        ctx.Point,
			Input:        in,
			Output:       out,
			Publications: pathImplicationPublications,
			Token:        token,
			Cancellation: ConcretePresenceImplicationRollbackNode,
			Barriers:     ConcretePresenceImplicationDescendantInvalidationBarriers,
		})
		if result.Canceled {
			return canceled()
		}
		out = result.Output
	}

	// N3: invalidation precedes refinements, relations, and channel evidence.
	if fact, ok := facts.PathDescendantInvalidation(ctx.Point); ok {
		_, directDynamicWrite := facts.DynamicIndexWrite(ctx.Point)
		out = applyPathDescendantInvalidation(ctx, config.Visibility, facts, sources, ensureCallResults().ReadLazy(), in, out, fact, !directDynamicWrite)
	}
	for _, fact := range facts.PostconditionRefinements(ctx.Point) {
		if poll.Poll() {
			return canceled()
		}
		out = applyValueRefinementAtCached(config.TypeValues, ctx.Registry, config.Visibility, config.ProjectPath, ctx.Point, out, fact.TargetPathRef(), fact.Value())
	}
	for _, fact := range facts.PostconditionPathRelations(ctx.Point) {
		if poll.Poll() {
			return canceled()
		}
		out = applyPostconditionPathRelation(ctx, config.Visibility, config.ProjectPath, out, fact)
	}
	for _, fact := range facts.ChannelSelects(ctx.Point) {
		if poll.Poll() {
			return canceled()
		}
		out = applyChannelSelect(ctx, config.Visibility, out, fact)
	}

	// Preserve the established nil-source boundary: write/return/finalizer
	// phases were not entered without a source provider.
	if sources == nil {
		return done(out)
	}

	read := ensureCallResults().ReadLazy()
	// N4: every source resolves against immutable Input; publications compose
	// onto evolving Output in this precise order.
	if fact, ok := facts.DynamicIndexWrite(ctx.Point); ok {
		out = applyDynamicIndexWrite(ctx, config.Visibility, facts, sources, read, in, out, fact)
	}
	if _, ok := facts.RootAssignment(ctx.Point); ok {
		result := e.rootAssignments.Apply(ConcreteRootAssignmentPointRequest{
			Context:                ctx,
			Resolver:               config.Visibility,
			Facts:                  facts,
			Sources:                sources,
			Read:                   read,
			Input:                  in,
			Output:                 out,
			CallOutcome:            callOutcome,
			ProjectPath:            config.ProjectPath,
			CovariantWiden:         config.CovariantWiden,
			TypeValues:             config.TypeValues,
			ClosedDynamicAllValues: config.ClosedDynamicAllValues,
		})
		out = result.Output
	}
	if fact, ok := facts.PathAssignment(ctx.Point); ok {
		var applied bool
		out, applied = ApplyConcretePathAssignment(ConcretePathAssignmentRequest{
			Context: ctx, Resolver: config.Visibility, Facts: facts,
			Sources: sources, Read: read, Input: in, Output: out,
			Assignment: fact,
		})
		if applied {
			out = applyObjectLiteralEntries(ctx, config.Visibility, facts, sources, read, in, out, fact.TargetPathRef(), fact.Source(), config.TypeValues)
			out = applyCallOutcomePresenceRelationPublishes(ctx, facts, &e.callOutcomeCache, callOutcome, config.Visibility, read, out)
		}
	}
	if fact, ok := facts.PathStaticMemberWrite(ctx.Point); ok {
		out = applyPathStaticMemberWrite(ctx, config.Visibility, facts, sources, read, in, out, fact)
	}

	// N5: returns see the same immutable source snapshot and all prior writes.
	if fact, ok := facts.Return(ctx.Point); ok {
		out = applyReturn(ctx, facts, sources, read, in, out, fact, config.Visibility, config.ProjectPath, config.TypeValues)
	}

	// N6 observes the completed node and is itself covered by node rollback.
	out = FinalizeConcretePoint(ConcretePointFinalizerRequest{
		Context: ctx, Resolver: config.Visibility, Facts: facts,
		CovariantWiden: config.CovariantWiden, Output: out,
	})
	return done(out)
}
