package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
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

func normalReturnDynamicIndexMutationTables(facts []callboundary.DynamicIndexFact) []pathdom.Path {
	if len(facts) == 0 {
		return nil
	}
	out := make([]pathdom.Path, 0, len(facts))
	for _, fact := range facts {
		if fact.Table.IsEmpty() {
			continue
		}
		out = append(out, fact.Table)
	}
	return out
}

func freshDynamicIndexMutationTablesAtCallEntry(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	boundaryPaths callboundary.PathBindings,
	in state.State,
	facts []callboundary.DynamicIndexFact,
) map[keyspace.Key]struct{} {
	if resolver == nil || len(facts) == 0 {
		return nil
	}
	counts := make(map[keyspace.Key]int, len(facts))
	paths := make(map[keyspace.Key]pathdom.Path, len(facts))
	for _, fact := range facts {
		tableKey, ok := callOutcomeKeyspaceKeyAt(resolver, ctx.Point, boundaryPaths, fact.Table)
		if !ok {
			continue
		}
		tablePath, ok := boundaryPaths.Substitute(fact.Table)
		if !ok {
			continue
		}
		counts[tableKey]++
		paths[tableKey] = tablePath
	}
	out := make(map[keyspace.Key]struct{})
	for tableKey, count := range counts {
		if count != 1 {
			continue
		}
		if rootPathHasFreshEmptyTable(ctx.Registry, in, paths[tableKey]) {
			out[tableKey] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func addDynamicIndexValueKeyMembershipsFromPath(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	out state.State,
	sourcePath pathdom.Path,
	container keyspace.Key,
	site dynamicindex.Site,
) state.State {
	if resolver == nil || sourcePath.IsEmpty() || sourcePath.Symbol == 0 {
		return out
	}
	sourceKey, ok := visibility.AddressAt(resolver, ctx.Point, sourcePath).VisibleStateKey()
	if !ok {
		return out
	}
	for _, table := range out.PathKeyMembershipTables(sourceKey) {
		out = out.AddDynamicIndexValueKeyMembership(container, site, table)
	}
	return out
}

func callOutcomePathMatchesAny(target pathdom.Path, candidates []pathdom.Path) bool {
	for _, candidate := range candidates {
		if target.Equal(candidate) {
			return true
		}
	}
	return false
}

// applyCallParamExposures eager-widens each argument object the callee exposes
// through a wider mutable view. Each exposure declares that the callee aliases the
// argument (or a member sub-path of it), at the wider mutable contract carried by
// Contract, into a slot the callee returns, stores into another argument, or
// retains in a captured sink; a write through that wider view can launder a wider
// value back into the argument object, so a later narrow read of the argument is
// no longer trustworthy. It reuses the single covariant widening routine
// (applyCovariantExposure) by rebasing the exposure's callee-relative Source
// placeholder onto the concrete argument path, so the widen and per-field fact
// invalidation match every other mutable-exposure site. The widen no-ops when the
// argument's current type is not strictly narrower than the contract.
func applyCallParamExposures(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	widen CovariantWiden,
	out state.State,
	paramBindings []pathdom.Path,
	exposures []callpayload.CallParamExposure,
) state.State {
	if widen == nil || len(exposures) == 0 {
		return out
	}
	for _, exposure := range exposures {
		argPath, ok := exposure.Source.Substitute(paramBindings)
		if !ok || argPath.Symbol == 0 {
			continue
		}
		out = applyCovariantExposure(ctx, resolver, widen, out, factflow.NewCovariantExposure(argPath, exposure.Contract, exposure.Kind))
	}
	return out
}

type resolvedCallParamLengthFloor struct {
	Path  pathdom.Path
	Floor int64
}

func resolveCallParamLengthFloors(
	resolver *visibility.Resolver,
	point cfg.Point,
	in state.State,
	paramBindings []pathdom.Path,
	facts []callpayload.CallParamLengthFloor,
) []resolvedCallParamLengthFloor {
	if len(facts) == 0 {
		return nil
	}
	out := make([]resolvedCallParamLengthFloor, 0, len(facts))
	for _, fact := range facts {
		targetPath, ok := fact.Path.Substitute(paramBindings)
		if !ok {
			continue
		}
		floor := fact.Floor
		// Current param length floors are lowered from positive LengthChange
		// effects, so Floor is the minimum delta contributed by the call.
		// Resolve it against the incoming floor before mutation invalidations
		// clear stale descendants, then publish the post-call lower bound.
		if existing, ok := readCallParamLengthFloor(resolver, point, in, targetPath); ok {
			floor += existing
		}
		out = append(out, resolvedCallParamLengthFloor{Path: targetPath, Floor: floor})
	}
	return out
}

func readCallParamLengthFloor(
	resolver *visibility.Resolver,
	point cfg.Point,
	in state.State,
	targetPath pathdom.Path,
) (int64, bool) {
	if resolver == nil || targetPath.Symbol == 0 {
		return 0, false
	}
	pathKey, ok := visibility.AddressAt(resolver, point, targetPath).VisibleStateKey()
	if !ok {
		return 0, false
	}
	return in.ReadLenFloor(resolver.KeySpace(), pathKey)
}

func applyCallParamLengthFloor(
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	floor int64,
) state.State {
	if resolver == nil || targetPath.Symbol == 0 || floor <= 0 {
		return out
	}
	pathKey, ok := visibility.AddressAt(resolver, point, targetPath).VisibleStateKey()
	if !ok {
		return out
	}
	return out.WriteLenFloor(resolver.KeySpace(), pathKey, floor)
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

func applyCallParamCondition(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	out state.State,
	site factflow.CallSiteView,
	condition callpayload.CallParamCondition,
) state.State {
	arg, ok := site.ArgumentSourceAt(condition.ParamIndex)
	if !ok {
		return out
	}
	if arg.Kind != factflow.ValueSourceExpression || !arg.HasExpr {
		return out
	}
	expressionCondition, ok := facts.ExpressionCondition(arg.ExprRef)
	if !ok {
		return out
	}
	selectedFacts := expressionCondition.FactsForValue(condition.Value)
	for _, refinement := range selectedFacts.Refinements() {
		out = applyValueRefinementAt(ctx.Registry, resolver, projectPath, ctx.Point, out, refinement.TargetPathRef(), refinement.Value())
	}
	for _, relation := range selectedFacts.PathRelations() {
		out = applyPostconditionPathRelation(ctx, resolver, projectPath, out, relation)
	}
	return out
}

func applyCallParamPathRelation(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	out state.State,
	bindings []pathdom.Path,
	relation callpayload.CallParamPathRelation,
) state.State {
	switch relation.Kind {
	case callpayload.CallPathRelationEqual:
		left, ok := relation.Left.Substitute(bindings)
		if !ok {
			return out
		}
		right, ok := relation.Right.Substitute(bindings)
		if !ok || left.Equal(right) {
			return out
		}
		return applyPathEqualityAt(ctx.Registry, resolver, projectPath, ctx.Point, out, left, right)
	default:
		return out
	}
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
