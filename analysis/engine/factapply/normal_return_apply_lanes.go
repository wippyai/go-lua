package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type normalReturnApplyPhase uint8

const (
	normalReturnApplyBeforeParamFacts normalReturnApplyPhase = iota
	normalReturnApplyAfterParamFacts
	normalReturnApplyAfterParamRelations
	normalReturnApplyFinalWrites
)

type normalReturnApplyContext struct {
	node                            transfer.NodeContext
	typeValues                      *typevalue.Cache
	resolver                        *visibility.Resolver
	projectPath                     PathTypeProjector
	point                           cfg.Point
	boundaryPaths                   callboundary.PathBindings
	normalFacts                     callboundary.NormalReturnFacts
	freshDynamicIndexMutationTables map[keyspace.Key]struct{}
}

func (ctx normalReturnApplyContext) substitute(path pathdom.Path) (pathdom.Path, bool) {
	return ctx.boundaryPaths.Substitute(path)
}

func (ctx normalReturnApplyContext) pathKey(path pathdom.Path) (pathdom.PathKey, bool) {
	return callOutcomePathKeyAt(ctx.resolver, ctx.point, ctx.boundaryPaths, path)
}

func (ctx normalReturnApplyContext) keyspaceKey(path pathdom.Path) (keyspace.Key, bool) {
	return callOutcomeKeyspaceKeyAt(ctx.resolver, ctx.point, ctx.boundaryPaths, path)
}

func (ctx normalReturnApplyContext) stateKey(path pathdom.Path) (pathaddr.StateKey, bool) {
	return callOutcomeStateKeyAt(ctx.resolver, ctx.point, ctx.boundaryPaths, path)
}

func (ctx normalReturnApplyContext) visibleStateKey(path pathdom.Path) (pathaddr.StateKey, bool) {
	return callOutcomeVisibleStateKeyAt(ctx.resolver, ctx.point, ctx.boundaryPaths, path)
}

func (ctx normalReturnApplyContext) relationGraphKey(operand callboundary.RelOperand) (state.RelOperand, bool) {
	targetPath, ok := ctx.substitute(operand.Path)
	if !ok || targetPath.Symbol == 0 {
		return state.RelOperand{}, false
	}
	return relationGraphKeyAt(ctx.resolver, ctx.point, targetPath, operand.IsLength)
}

type normalReturnApplyLaneHandler struct {
	phase normalReturnApplyPhase
	apply func(normalReturnApplyContext, state.State) state.State
}

func applyNormalReturnFactPhase(ctx normalReturnApplyContext, phase normalReturnApplyPhase, out state.State) state.State {
	for _, lane := range normalReturnApplyLanes {
		handler := lane.Value
		if handler.phase == phase {
			out = handler.apply(ctx, out)
		}
	}
	return out
}

var normalReturnApplyLanes = callboundary.BindNormalReturnFactLanes("normal-return apply", map[callboundary.NormalReturnFactLaneID]normalReturnApplyLaneHandler{
	callboundary.LanePathRefinements: {
		phase: normalReturnApplyBeforeParamFacts,
		apply: applyNormalReturnPathRefinements,
	},
	callboundary.LanePathInvalidations: {
		phase: normalReturnApplyAfterParamFacts,
		apply: applyNormalReturnPathInvalidations,
	},
	callboundary.LanePersistentPathWrites: {
		phase: normalReturnApplyFinalWrites,
		apply: applyNormalReturnPersistentPathWrites,
	},
	callboundary.LanePathStaticMembers: {
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnPathStaticMembers,
	},
	callboundary.LanePathPresenceImplications: {
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnPathPresenceImplications,
	},
	callboundary.LaneDynamicIndexFacts: {
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnDynamicIndexFacts,
	},
	callboundary.LaneKeyMemberships: {
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnKeyMemberships,
	},
	callboundary.LaneDynamicValueKeys: {
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnDynamicValueKeys,
	},
	callboundary.LaneDynamicAllValues: {
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnDynamicAllValues,
	},
	callboundary.LaneBranchProofs: {
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnBranchProofs,
	},
	callboundary.LaneNumFloors: {
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnNumFloors,
	},
	callboundary.LaneRelConstraints: {
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnRelConstraints,
	},
	callboundary.LaneChannelSelects: {
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnChannelSelects,
	},
	callboundary.LaneFrozenTables: {
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnFrozenTables,
	},
	callboundary.LaneEffectDeltas: {
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnEffectDeltas,
	},
	callboundary.LaneStoreRelations: {
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnStoreRelations,
	},
	callboundary.LaneLifecycleFacts: {
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnLifecycleFacts,
	},
	callboundary.LaneEscapeEvents: {
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnEscapeEvents,
	},
}, func(handler normalReturnApplyLaneHandler) bool { return handler.apply != nil })

func applyNormalReturnPathRefinements(ctx normalReturnApplyContext, out state.State) state.State {
	for _, fact := range ctx.normalFacts.PathRefinements {
		targetPath, ok := ctx.substitute(fact.Path)
		if !ok {
			continue
		}
		out = applyValueRefinementAtCached(ctx.typeValues, ctx.node.Registry, ctx.resolver, ctx.projectPath, ctx.point, out, targetPath, factflow.NewValueConstraint(fact.Value))
	}
	return out
}

func applyNormalReturnPathInvalidations(ctx normalReturnApplyContext, out state.State) state.State {
	dynamicIndexMutationTables := normalReturnDynamicIndexMutationTables(ctx.normalFacts.DynamicIndexFacts)
	for _, fact := range ctx.normalFacts.PathInvalidations {
		targetPath, ok := ctx.substitute(fact.Path)
		if !ok {
			continue
		}
		mutatesDynamicIndex := normalReturnPathMatchesAny(fact.Path, dynamicIndexMutationTables)
		if callOutcomeConcreteRootInvalidation(fact.Path) && !mutatesDynamicIndex {
			out = writeRootSymbol(ctx.node, ctx.resolver, out, targetPath.Symbol, targetPath, product.Top())
			continue
		}
		preserveStructuralWitness := fact.PreserveStructuralWitness || boundaryRootBoundToDescendant(fact.Path, targetPath)
		clearStructuralWitness := !preserveStructuralWitness && !mutatesDynamicIndex
		clearTarget := fact.ClearTarget || (!fact.PreserveStructuralWitness && preserveStructuralWitness)
		out = writePathInvalidationMarker(ctx.resolver, ctx.point, out, targetPath, preserveStructuralWitness)
		out = applyPathDescendantInvalidation(ctx.node, ctx.resolver, factflow.Facts{}, nil, nil, out, out, factflow.NewPathDescendantInvalidation(targetPath), clearStructuralWitness)
		if clearTarget || clearStructuralWitness {
			out = invalidateMutatedFieldSlot(ctx.node, ctx.resolver, out, targetPath)
		}
	}
	return out
}

func boundaryRootBoundToDescendant(factPath, targetPath pathdom.Path) bool {
	if len(factPath.Segments) != 0 || len(targetPath.Segments) == 0 {
		return false
	}
	if factPath.IsPlaceholder() {
		return true
	}
	_, ok := callboundary.ReturnSlotIndex(factPath)
	return ok
}

func applyNormalReturnPersistentPathWrites(ctx normalReturnApplyContext, out state.State) state.State {
	for _, fact := range ctx.normalFacts.PersistentPathWrites {
		targetPath, ok := ctx.substitute(fact.Path)
		if !ok {
			continue
		}
		if callboundary.IsConcreteSymbolPath(fact.Path) && len(targetPath.Segments) == 0 {
			out = writeRootSymbol(ctx.node, ctx.resolver, out, targetPath.Symbol, targetPath, fact.Value)
			continue
		}
		out = applyValueWriteAt(ctx.node.Registry, ctx.resolver, ctx.point, out, targetPath, fact.Value)
	}
	return out
}

func applyNormalReturnPathStaticMembers(ctx normalReturnApplyContext, out state.State) state.State {
	edit := out.Edit(ctx.node.Registry)
	changed := false
	for _, fact := range ctx.normalFacts.PathStaticMembers {
		targetPathKey, ok := ctx.pathKey(fact.Path)
		if !ok {
			continue
		}
		if edit.WritePathStaticMember(ctx.resolver.KeySpace(), targetPathKey, fact.Value) {
			changed = true
		}
	}
	if changed {
		return edit.DoneOn(out)
	}
	return out
}

func applyNormalReturnPathPresenceImplications(ctx normalReturnApplyContext, out state.State) state.State {
	for _, fact := range ctx.normalFacts.PathPresenceImplications {
		trigger, ok := ctx.keyspaceKey(fact.Trigger)
		if !ok {
			continue
		}
		target, ok := ctx.keyspaceKey(fact.Target)
		if !ok {
			continue
		}
		var implication pathevidence.PathPresenceImplication
		if fact.HasTriggerValue {
			implication = pathevidence.NewPathValuePresenceImplication(trigger, fact.TriggerValue, target, fact.TargetPresence)
		} else {
			implication = pathevidence.NewPathPresenceImplication(trigger, fact.TriggerPresence, target, fact.TargetPresence)
		}
		out = out.AddPathPresenceImplication(implication)
	}
	return activatePathPresenceImplications(ctx.node.Registry, ctx.resolver, ctx.point, out)
}

func applyNormalReturnKeyMemberships(ctx normalReturnApplyContext, out state.State) state.State {
	for _, fact := range ctx.normalFacts.KeyMemberships {
		keyStateKey, ok := ctx.stateKey(fact.Key)
		if !ok {
			continue
		}
		tableStateKey, ok := ctx.stateKey(fact.Table)
		if !ok {
			continue
		}
		out = out.AddPathKeyMembership(keyStateKey, tableStateKey)
	}
	return out
}

func applyNormalReturnDynamicValueKeys(ctx normalReturnApplyContext, out state.State) state.State {
	for _, fact := range ctx.normalFacts.DynamicValueKeys {
		containerKey, ok := ctx.keyspaceKey(fact.Container)
		if !ok {
			continue
		}
		tableStateKey, ok := ctx.stateKey(fact.Table)
		if !ok {
			continue
		}
		out = out.AddDynamicIndexValueKeyMembership(containerKey, fact.Site, tableStateKey)
	}
	return out
}

func applyNormalReturnDynamicAllValues(ctx normalReturnApplyContext, out state.State) state.State {
	for _, fact := range ctx.normalFacts.DynamicAllValues {
		containerKey, ok := ctx.keyspaceKey(fact.Container)
		if !ok {
			continue
		}
		tableStateKey, ok := ctx.stateKey(fact.Table)
		if !ok {
			continue
		}
		out = out.AddDynamicIndexAllValuesKeyMembership(containerKey, tableStateKey)
	}
	return out
}
