package factapply

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	valueref "github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/factquery"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
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
		value := paramPathWriteValueWithDeclaredContract(ctx, facts, targetPath, fact.Value)
		out = applyValueWriteAt(ctx.Registry, resolver, ctx.Point, out, targetPath, value)
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
	out = applyNormalReturnFactPhase(normalApply, normalReturnApplyFinalWrites, out)
	// Apply covariant call-boundary exposures after ordinary post-call facts.
	// Context-specialized summaries may carry precise parameter-member facts such
	// as `$0.x = number`; once the callee has exposed `$0` through a wider mutable
	// view, those facts are stale for the caller and must be invalidated by the
	// shared exposure model.
	out = applyCallParamExposures(ctx, resolver, widen, out, paramBindings, outcome.ParamExposures)
	return applyProtectedCallTypestate(out, outcome.ProtectedCallTypestate)
}

func paramPathWriteValueWithDeclaredContract(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	targetPath path.Path,
	value product.Value,
) product.Value {
	if ctx.Registry == nil || ctx.Graph == nil || targetPath.Symbol == 0 {
		return value
	}
	declSource, ok := factquery.DominatingPathRootDeclarationSource(ctx.Point, targetPath, facts, ctx.Graph)
	if !ok {
		return value
	}
	assign, ok := facts.RootAssignment(declSource.Point)
	if !ok {
		return value
	}
	declared, ok := assign.DeclaredValue()
	if !ok || (!assign.DeclaredValueContracts() && !assign.DeclaredValueOverlays()) {
		return value
	}
	projected, ok := declaredValueForParamWritePath(ctx.Registry, declared, targetPath)
	if !ok {
		return value
	}
	return mergeParamWriteDeclaredContract(ctx.Registry, value, projected)
}

func declaredValueForParamWritePath(reg *axis.Registry, declared product.Value, targetPath path.Path) (product.Value, bool) {
	if len(targetPath.Segments) == 0 {
		return declared, true
	}
	declaredType, ok := typevalue.TypeOf(reg, declared)
	if !ok || declaredType == nil {
		return product.Value{}, false
	}
	projected, ok := luatypeprojection.ApplyConstructorSegments(declaredType, targetPath.Segments)
	if !ok || projected == nil {
		return product.Value{}, false
	}
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, projected), projected)
	if claim := product.Get(reg, declared, assertion.Key); !claim.IsTop() && !claim.IsBottom() {
		value = product.Set(reg, value, assertion.Key, claim)
	}
	return value, true
}

func mergeParamWriteDeclaredContract(reg *axis.Registry, value, declared product.Value) product.Value {
	value = valueref.MergeDeclaredContract(reg, value, declared)
	if declaredClaim := product.Get(reg, declared, assertion.Key); !declaredClaim.IsBottom() && !declaredClaim.IsTop() {
		currentClaim := product.Get(reg, value, assertion.Key)
		value = product.Set(reg, value, assertion.Key, assertion.Combine(currentClaim, declaredClaim))
	}
	return value
}
