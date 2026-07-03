package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
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
	bindings := callPlaceholderBindings(facts, site)
	paramBindings := callArgumentPlaceholderBindings(facts, site)
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

func callOutcomeConcreteRootInvalidation(target pathdom.Path) bool {
	return !target.IsPlaceholder() && target.Symbol != 0 && len(target.Segments) == 0
}

// invalidateMutatedFieldSlot drops the caller's stored value for a field slot a
// callee wrote through. A field-level path invalidation (segments > 0) records
// that the callee assigned the slot to a value of its own, wider parameter field
// type, so the slot's confined caller value (a fresh literal's heap static member
// or path-key refinement) is no longer trustworthy. Descendant invalidation alone
// preserves the slot's own value, which would launder the covariant write-through;
// clearing the slot itself makes the later read fall back to structural
// projection, matching how an opaque parameter argument already reflects the
// mutation. Root-targeted invalidations (segments == 0) keep their container value
// and are handled by the descendant pass above.
func invalidateMutatedFieldSlot(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	out state.State,
	targetPath pathdom.Path,
) state.State {
	if len(targetPath.Segments) == 0 {
		return out
	}
	if withHeap, ok := invalidateHeapStaticMemberSubtreeAt(ctx.Registry, out, resolver, ctx.Point, targetPath); ok {
		out = withHeap
	}
	if cleared, ok := invalidatePathSubtreeAt(out, resolver, ctx.Point, targetPath); ok {
		out = cleared
	}
	return out
}

func writePathInvalidationMarker(
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	preserveStructuralWitness bool,
) state.State {
	if targetKey, ok := factKeyspaceKeyAt(resolver, point, targetPath); ok {
		site := callboundary.PathInvalidationEffectSite()
		if preserveStructuralWitness {
			site = callboundary.PathStructuralPreservingInvalidationEffectSite()
		}
		return out.WriteEffectDelta(effectdelta.Key{
			Target: targetKey,
			Site:   site,
			Kind:   effectdelta.Mutation,
		}, effectdelta.Top())
	}
	return out
}

func callOutcomePathKeyAt(
	resolver *visibility.Resolver,
	point cfg.Point,
	boundaryPaths callboundary.PathBindings,
	path pathdom.Path,
) (pathdom.PathKey, bool) {
	targetPath, ok := boundaryPaths.Substitute(path)
	if !ok {
		return "", false
	}
	targetKey := factPathKeyAt(resolver, point, targetPath)
	if targetKey == "" {
		return "", false
	}
	return targetKey, true
}

func callPlaceholderBindings(facts factflow.Facts, site factflow.CallSiteView) []pathdom.Path {
	var bindings []pathdom.Path
	offset := 0
	if receiverPath, ok := site.ReceiverPath(); ok {
		bindings = bindPlaceholderPath(bindings, 0, receiverPath)
		offset = 1
	}
	site.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
		if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
			return true
		}
		sourcePath, ok := facts.ExpressionPathRef(source.ExprRef)
		if !ok || sourcePath.IsEmpty() {
			return true
		}
		bindings = bindPlaceholderPath(bindings, i+offset, sourcePath)
		return true
	})
	return bindings
}

func callArgumentPlaceholderBindings(facts factflow.Facts, site factflow.CallSiteView) []pathdom.Path {
	var bindings []pathdom.Path
	site.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
		if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
			return true
		}
		sourcePath, ok := facts.ExpressionPathRef(source.ExprRef)
		if !ok || sourcePath.IsEmpty() {
			return true
		}
		bindings = bindPlaceholderPath(bindings, i, sourcePath)
		return true
	})
	return bindings
}

func callReturnSlotBindings(site factflow.CallSiteView) []pathdom.Path {
	var bindings []pathdom.Path
	site.ForEachResultTarget(func(target factflow.CallResultTargetView) bool {
		if target.ResultIndex() < 0 || target.TargetPathEmpty() {
			return true
		}
		bindings = bindPlaceholderPath(bindings, target.ResultIndex(), target.TargetPathRef())
		return true
	})
	return bindings
}

func bindPlaceholderPath(bindings []pathdom.Path, index int, p pathdom.Path) []pathdom.Path {
	if index < 0 || p.IsEmpty() {
		return bindings
	}
	for len(bindings) <= index {
		bindings = append(bindings, pathdom.Path{})
	}
	bindings[index] = p
	return bindings
}

func callOutcomeStateKeyAt(
	resolver *visibility.Resolver,
	point cfg.Point,
	boundaryPaths callboundary.PathBindings,
	path pathdom.Path,
) (pathaddr.StateKey, bool) {
	targetPath, ok := boundaryPaths.Substitute(path)
	if !ok {
		return "", false
	}
	if callboundary.IsConcreteSymbolPath(path) {
		return visibility.AddressAt(resolver, point, targetPath).RootOrVisibleStateKey()
	}
	return factStateKeyAt(resolver, point, targetPath)
}
