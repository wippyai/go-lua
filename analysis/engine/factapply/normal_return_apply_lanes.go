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
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
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

type normalReturnApplyLane struct {
	id    callboundary.NormalReturnFactLaneID
	phase normalReturnApplyPhase
	apply func(normalReturnApplyContext, state.State) state.State
}

type normalReturnApplyLaneHandler struct {
	phase normalReturnApplyPhase
	apply func(normalReturnApplyContext, state.State) state.State
}

func applyNormalReturnFactPhase(ctx normalReturnApplyContext, phase normalReturnApplyPhase, out state.State) state.State {
	for _, lane := range normalReturnApplyLanes {
		if lane.phase == phase {
			out = lane.apply(ctx, out)
		}
	}
	return out
}

var normalReturnApplyLanes = buildNormalReturnApplyLanes(map[callboundary.NormalReturnFactLaneID]normalReturnApplyLaneHandler{
	callboundary.LanePathRefinements: {
		phase: normalReturnApplyBeforeParamFacts,
		apply: applyNormalReturnPathRefinements,
	},
	callboundary.LanePathInvalidations: {
		phase: normalReturnApplyAfterParamFacts,
		apply: applyNormalReturnPathInvalidations,
	},
	callboundary.LanePersistentPathWrites: {
		phase: normalReturnApplyAfterParamFacts,
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
})

func buildNormalReturnApplyLanes(handlers map[callboundary.NormalReturnFactLaneID]normalReturnApplyLaneHandler) []normalReturnApplyLane {
	bindings := callboundary.BindNormalReturnFactLanes("normal-return apply", handlers, func(handler normalReturnApplyLaneHandler) bool {
		return handler.apply != nil
	})
	out := make([]normalReturnApplyLane, 0, len(bindings))
	for _, binding := range bindings {
		handler := binding.Value
		out = append(out, normalReturnApplyLane{
			id:    binding.ID,
			phase: handler.phase,
			apply: handler.apply,
		})
	}
	return out
}

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

func applyNormalReturnBranchProofs(ctx normalReturnApplyContext, out state.State) state.State {
	for _, proof := range ctx.normalFacts.BranchProofs {
		stateProof, ok := callBranchProofAt(ctx, proof)
		if !ok {
			continue
		}
		out = out.AddBranchProof(stateProof)
		out = applyNormalReturnPathRelationProof(ctx, out, proof)
	}
	return out
}

func callBranchProofAt(
	ctx normalReturnApplyContext,
	proof callboundary.BranchProof,
) (pathevidence.BranchProof, bool) {
	path, ok := ctx.keyspaceKey(proof.Path)
	if !ok {
		return pathevidence.BranchProof{}, false
	}
	switch proof.Kind {
	case pathevidence.BranchProofPathPresence:
		return pathevidence.BranchProof{
			Kind:     pathevidence.BranchProofPathPresence,
			Path:     path,
			Presence: proof.Presence,
		}, true
	case pathevidence.BranchProofPathEqual, pathevidence.BranchProofPathNotEqual, pathevidence.BranchProofIndexInRange:
		other, ok := ctx.keyspaceKey(proof.Other)
		if !ok {
			return pathevidence.BranchProof{}, false
		}
		return pathevidence.BranchProof{
			Kind:  proof.Kind,
			Path:  path,
			Other: other,
		}, true
	default:
		return pathevidence.BranchProof{}, false
	}
}

func applyNormalReturnPathRelationProof(ctx normalReturnApplyContext, out state.State, proof callboundary.BranchProof) state.State {
	switch proof.Kind {
	case pathevidence.BranchProofPathEqual, pathevidence.BranchProofPathNotEqual:
	default:
		return out
	}
	leftPath, ok := ctx.substitute(proof.Path)
	if !ok {
		return out
	}
	rightPath, ok := ctx.substitute(proof.Other)
	if !ok {
		return out
	}
	edgeCtx := transfer.EdgeContext{
		Registry: ctx.node.Registry,
		Edge: cfg.Edge{
			From: ctx.point,
			Cond: true,
		},
	}
	switch proof.Kind {
	case pathevidence.BranchProofPathEqual:
		return applyBranchPathEquality(ctx.typeValues, edgeCtx, ctx.resolver, ctx.projectPath, out, leftPath, rightPath)
	case pathevidence.BranchProofPathNotEqual:
		return applyBranchPathInequality(ctx.typeValues, edgeCtx, ctx.resolver, ctx.projectPath, out, leftPath, rightPath)
	default:
		return out
	}
}

func applyNormalReturnNumFloors(ctx normalReturnApplyContext, out state.State) state.State {
	for _, fact := range ctx.normalFacts.NumFloors {
		out = applyNormalReturnNumFloor(ctx, out, fact)
	}
	return out
}

func applyNormalReturnNumFloor(
	ctx normalReturnApplyContext,
	out state.State,
	fact callboundary.NumFloorFact,
) state.State {
	targetPath, ok := ctx.substitute(fact.Path)
	if !ok || targetPath.Symbol == 0 {
		return out
	}
	pathKey, ok := visibility.AddressAt(ctx.resolver, ctx.point, targetPath).RootOrVisibleStateKey()
	if !ok {
		return out
	}
	return out.WriteNumFloor(ctx.resolver.KeySpace(), pathKey, fact.Floor)
}

func applyNormalReturnRelConstraints(ctx normalReturnApplyContext, out state.State) state.State {
	for _, fact := range ctx.normalFacts.RelConstraints {
		out = applyNormalReturnRelConstraint(ctx, out, fact)
	}
	return out
}

func applyNormalReturnRelConstraint(
	ctx normalReturnApplyContext,
	out state.State,
	fact callboundary.RelConstraintFact,
) state.State {
	aKey, ok := ctx.relationGraphKey(fact.A)
	if !ok {
		return out
	}
	cKey, ok := ctx.relationGraphKey(fact.C)
	if !ok {
		return out
	}
	var bKey state.RelOperand
	coB := fact.CoB
	if coB != 0 && !fact.B.Path.IsEmpty() {
		bKey, ok = ctx.relationGraphKey(fact.B)
		if !ok {
			return out
		}
	} else {
		coB = 0
	}
	return out.WriteScaledConstraint(fact.CoA, aKey, coB, bKey, cKey, fact.K)
}

func applyNormalReturnFrozenTables(ctx normalReturnApplyContext, out state.State) state.State {
	for _, fact := range ctx.normalFacts.FrozenTables {
		targetPath, ok := ctx.substitute(fact.Target)
		if !ok {
			continue
		}
		if targetKey, ok := factKeyspaceKeyAt(ctx.resolver, ctx.point, targetPath); ok {
			out = out.WriteEffectDelta(effectdelta.Key{
				Target: targetKey,
				Site:   callboundary.FrozenTableEffectSite(),
				Kind:   effectdelta.Freeze,
			}, effectdelta.Top())
		}
		out = applyFrozenTableFact(ctx.node.Registry, ctx.resolver, ctx.projectPath, ctx.point, out, targetPath)
	}
	return out
}

func applyNormalReturnEffectDeltas(ctx normalReturnApplyContext, out state.State) state.State {
	for _, delta := range ctx.normalFacts.EffectDeltas {
		targetKey, ok := ctx.keyspaceKey(delta.Target)
		if !ok {
			continue
		}
		out = out.WriteEffectDelta(effectdelta.Key{
			Target: targetKey,
			Site:   delta.Site,
			Kind:   delta.Kind,
		}, delta.Value)
	}
	return out
}

func applyNormalReturnStoreRelations(ctx normalReturnApplyContext, out state.State) state.State {
	for _, relation := range ctx.normalFacts.StoreRelations {
		sourceStateKey, ok := ctx.stateKey(relation.Source)
		if !ok {
			continue
		}
		intoStateKey, ok := ctx.stateKey(relation.Into)
		if !ok {
			continue
		}
		out = out.AddStoreRelation(state.StoreRelation{
			Source: sourceStateKey,
			Into:   intoStateKey,
		})
	}
	return out
}

func applyNormalReturnLifecycleFacts(ctx normalReturnApplyContext, out state.State) state.State {
	for _, fact := range ctx.normalFacts.LifecycleFacts {
		targetStateKey, ok := ctx.visibleStateKey(fact.Target)
		if !ok || fact.Protocol == "" {
			continue
		}
		resource := out.CanonicalTypestateResource(ctx.resolver.KeySpace(), targetStateKey, fact.Protocol)
		switch fact.Kind {
		case callboundary.LifecycleAcquire:
			out = out.AcquireTypestate(resource, fact.To, fact.Obligation)
		case callboundary.LifecycleTransition:
			out = out.TransitionTypestate(resource, fact.From, fact.To)
		case callboundary.LifecycleEscape:
			out = out.EscapeTypestate(resource)
		}
	}
	return out
}

func applyNormalReturnEscapeEvents(ctx normalReturnApplyContext, out state.State) state.State {
	for _, event := range ctx.normalFacts.EscapeEvents {
		targetPath, ok := ctx.substitute(event.Target)
		if !ok {
			continue
		}
		targetStateKey, ok := ctx.visibleStateKey(event.Target)
		if !ok {
			continue
		}
		out = out.AddEscapeEvent(state.EscapeEvent{
			Target:    targetStateKey,
			Kind:      event.Kind,
			Recursive: event.Recursive,
		})
		out = applyEscapeEventPlacement(ctx.node.Registry, ctx.resolver, ctx.projectPath, ctx.point, out, targetPath, event)
	}
	return out
}
