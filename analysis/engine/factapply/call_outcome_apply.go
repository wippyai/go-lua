package factapply

import (
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
)

func applyCallOutcomeFacts(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	widen CovariantWiden,
	typeValues *typevalue.Cache,
	out state.State,
	site factflow.CallSiteView,
	outcome callpayload.CallOutcome,
) state.State {
	bindings := callPlaceholderBindings(facts, resolver, site)
	paramBindings := callArgumentPlaceholderBindings(facts, resolver, site)
	returnBindings := callReturnSlotBindings(site)
	boundaryPaths := callboundary.NewPathBindings(bindings, returnBindings)
	normalReturnFacts := outcome.NormalReturnFacts
	normalApply := normalReturnApplyContext{
		node:                            ctx,
		typeValues:                      typeValues,
		resolver:                        resolver,
		projectPath:                     projectPath,
		point:                           ctx.Point,
		boundaryPaths:                   boundaryPaths,
		normalFacts:                     normalReturnFacts,
		freshDynamicIndexMutationTables: freshDynamicIndexMutationTablesAtCallEntry(ctx, resolver, boundaryPaths, out, normalReturnFacts.DynamicIndexFacts),
	}
	lengthFloors := resolveCallParamLengthFloors(resolver, ctx.Point, out, paramBindings, outcome.ParamLengthFloors)
	for id, object := range outcome.HeapTableObjects {
		out = out.WriteHeapTableObject(ctx.Registry, id, object)
	}
	for id, value := range outcome.Placements {
		out = writeJoinedPlacement(out, id, value)
	}
	out = applyNormalReturnFactPhase(normalApply, normalReturnApplyBeforeParamFacts, out)
	for _, fact := range outcome.ParamPathInvalidations {
		targetPath, ok := fact.Path.Substitute(paramBindings)
		if !ok {
			continue
		}
		out = writePathInvalidationMarker(resolver, ctx.Point, out, targetPath, fact.PreserveStructuralWitness)
		out = applyPathDescendantInvalidation(ctx, resolver, factflow.Facts{}, nil, nil, out, out, factflow.NewPathDescendantInvalidation(targetPath), !fact.PreserveStructuralWitness)
	}
	for _, fact := range outcome.ParamPathWrites {
		targetPath, ok := fact.Path.Substitute(paramBindings)
		if !ok {
			continue
		}
		out = applyValueWriteAt(ctx.Registry, resolver, ctx.Point, out, targetPath, fact.Value)
	}
	for _, fact := range outcome.ParamPathRefinements {
		targetPath, ok := fact.Path.Substitute(paramBindings)
		if !ok {
			continue
		}
		out = applyValueRefinementAt(ctx.Registry, resolver, projectPath, ctx.Point, out, targetPath, factflow.NewValueConstraint(fact.Value))
	}
	out = applyNormalReturnFactPhase(normalApply, normalReturnApplyAfterParamFacts, out)
	for _, fact := range lengthFloors {
		out = applyCallParamLengthFloor(resolver, ctx.Point, out, fact.Path, fact.Floor)
	}
	for _, condition := range outcome.ParamConditions {
		out = applyCallParamCondition(ctx, facts, resolver, projectPath, out, site, condition)
	}
	for _, relation := range outcome.ParamPathRelations {
		out = applyCallParamPathRelation(ctx, resolver, projectPath, out, paramBindings, relation)
	}
	out = applyNormalReturnFactPhase(normalApply, normalReturnApplyAfterParamRelations, out)
	// Apply covariant call-boundary exposures after ordinary post-call facts.
	// Context-specialized summaries may carry precise parameter-member facts such
	// as `$0.x = number`; once the callee has exposed `$0` through a wider mutable
	// view, those facts are stale for the caller and must be invalidated by the
	// shared exposure model.
	out = applyCallParamExposures(ctx, resolver, widen, out, paramBindings, outcome.ParamExposures)
	return out
}
