package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
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

type normalReturnApplyLane struct {
	id    callboundary.NormalReturnFactLaneID
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

var normalReturnApplyLanes = []normalReturnApplyLane{
	{
		id:    callboundary.LanePathRefinements,
		phase: normalReturnApplyBeforeParamFacts,
		apply: applyNormalReturnPathRefinements,
	},
	{
		id:    callboundary.LanePathInvalidations,
		phase: normalReturnApplyAfterParamFacts,
		apply: applyNormalReturnPathInvalidations,
	},
	{
		id:    callboundary.LanePersistentPathWrites,
		phase: normalReturnApplyAfterParamFacts,
		apply: applyNormalReturnPersistentPathWrites,
	},
	{
		id:    callboundary.LanePathStaticMembers,
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnPathStaticMembers,
	},
	{
		id:    callboundary.LanePathPresenceImplications,
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnPathPresenceImplications,
	},
	{
		id:    callboundary.LaneDynamicIndexFacts,
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnDynamicIndexFacts,
	},
	{
		id:    callboundary.LaneKeyMemberships,
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnKeyMemberships,
	},
	{
		id:    callboundary.LaneDynamicValueKeys,
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnDynamicValueKeys,
	},
	{
		id:    callboundary.LaneDynamicAllValues,
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnDynamicAllValues,
	},
	{
		id:    callboundary.LaneBranchProofs,
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnBranchProofs,
	},
	{
		id:    callboundary.LaneNumFloors,
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnNumFloors,
	},
	{
		id:    callboundary.LaneRelConstraints,
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnRelConstraints,
	},
	{
		id:    callboundary.LaneChannelSelects,
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnChannelSelects,
	},
	{
		id:    callboundary.LaneFrozenTables,
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnFrozenTables,
	},
	{
		id:    callboundary.LaneEffectDeltas,
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnEffectDeltas,
	},
	{
		id:    callboundary.LaneStoreRelations,
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnStoreRelations,
	},
	{
		id:    callboundary.LaneLifecycleFacts,
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnLifecycleFacts,
	},
	{
		id:    callboundary.LaneEscapeEvents,
		phase: normalReturnApplyAfterParamRelations,
		apply: applyNormalReturnEscapeEvents,
	},
}

func applyNormalReturnPathRefinements(ctx normalReturnApplyContext, out state.State) state.State {
	for _, fact := range ctx.normalFacts.PathRefinements {
		targetPath, ok := ctx.boundaryPaths.Substitute(fact.Path)
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
		targetPath, ok := ctx.boundaryPaths.Substitute(fact.Path)
		if !ok {
			continue
		}
		mutatesDynamicIndex := callOutcomePathMatchesAny(fact.Path, dynamicIndexMutationTables)
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
		targetPath, ok := ctx.boundaryPaths.Substitute(fact.Path)
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
		targetPathKey, ok := callOutcomePathKeyAt(ctx.resolver, ctx.point, ctx.boundaryPaths, fact.Path)
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
		trigger, ok := callOutcomeKeyspaceKeyAt(ctx.resolver, ctx.point, ctx.boundaryPaths, fact.Trigger)
		if !ok {
			continue
		}
		target, ok := callOutcomeKeyspaceKeyAt(ctx.resolver, ctx.point, ctx.boundaryPaths, fact.Target)
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

func applyNormalReturnDynamicIndexFacts(ctx normalReturnApplyContext, out state.State) state.State {
	edit := out.Edit(ctx.node.Registry)
	priorAllValueTables := make(map[keyspace.Key][]pathaddr.StateKey)
	if len(ctx.normalFacts.DynamicIndexFacts) != 0 {
		clearedContainers := make(map[keyspace.Key]struct{}, len(ctx.normalFacts.DynamicIndexFacts))
		for _, fact := range ctx.normalFacts.DynamicIndexFacts {
			tableKey, ok := callOutcomeKeyspaceKeyAt(ctx.resolver, ctx.point, ctx.boundaryPaths, fact.Table)
			if !ok {
				continue
			}
			if _, seen := clearedContainers[tableKey]; seen {
				continue
			}
			clearedContainers[tableKey] = struct{}{}
			priorAllValueTables[tableKey] = out.DynamicIndexAllValuesKeyMembershipTables(tableKey)
			out = out.ClearDynamicIndexValueKeyMembershipsForContainer(tableKey)
		}
	}
	dynamicChanged := false
	for _, fact := range ctx.normalFacts.DynamicIndexFacts {
		tableKey, ok := callOutcomeKeyspaceKeyAt(ctx.resolver, ctx.point, ctx.boundaryPaths, fact.Table)
		if !ok {
			continue
		}
		tablePath, ok := ctx.boundaryPaths.Substitute(fact.Table)
		if !ok {
			continue
		}
		key := dynamicindex.Key{
			Table: tableKey,
			Site:  fact.Site,
		}
		valuePath, hasValuePath := ctx.boundaryPaths.Substitute(fact.ValuePath)
		if edit.WriteDynamicIndexFact(key, fact.Value) {
			dynamicChanged = true
		}
		if hasValuePath {
			out = addDynamicIndexValueKeyMembershipsFromPath(ctx.node, ctx.resolver, out, valuePath, tableKey, key.Site)
		}
		out = writeHeapTableDynamicIndexFact(ctx.node, ctx.resolver, out, tablePath, key, fact.Value)
		if hasValuePath {
			out = addNormalReturnDynamicAllValueMembershipsFromPath(ctx, out, tablePath, tableKey, valuePath, fact.Value, priorAllValueTables[tableKey])
		}
	}
	if dynamicChanged {
		return edit.DoneOn(out)
	}
	return out
}

func addNormalReturnDynamicAllValueMembershipsFromPath(
	ctx normalReturnApplyContext,
	out state.State,
	tablePath pathdom.Path,
	tableKey keyspace.Key,
	valuePath pathdom.Path,
	value dynamicindex.Fact,
	priorTables []pathaddr.StateKey,
) state.State {
	if ctx.resolver == nil || valuePath.IsEmpty() || valuePath.Symbol == 0 {
		return out
	}
	sourceKey, ok := visibility.AddressAt(ctx.resolver, ctx.point, valuePath).VisibleStateKey()
	if !ok {
		return out
	}
	sourceTables := out.PathKeyMembershipTables(sourceKey)
	if len(sourceTables) == 0 {
		return out
	}
	if dynamicIndexFactDefinitelyAbsent(ctx.node.Registry, value) {
		for _, table := range priorTables {
			out = out.AddDynamicIndexAllValuesKeyMembership(tableKey, table)
		}
		return out
	}
	for _, table := range priorTables {
		if stateKeyIn(sourceTables, table) {
			out = out.AddDynamicIndexAllValuesKeyMembership(tableKey, table)
		}
	}
	if _, freshAtCallEntry := ctx.freshDynamicIndexMutationTables[tableKey]; freshAtCallEntry {
		for _, table := range sourceTables {
			out = out.AddDynamicIndexAllValuesKeyMembership(tableKey, table)
		}
	}
	for _, table := range rootPathDynamicValueKeyMembershipTables(ctx.node.Registry, out, tablePath, tableKey) {
		out = out.AddDynamicIndexAllValuesKeyMembership(tableKey, table)
	}
	return out
}

func applyNormalReturnKeyMemberships(ctx normalReturnApplyContext, out state.State) state.State {
	for _, fact := range ctx.normalFacts.KeyMemberships {
		keyStateKey, ok := callOutcomeStateKeyAt(ctx.resolver, ctx.point, ctx.boundaryPaths, fact.Key)
		if !ok {
			continue
		}
		tableStateKey, ok := callOutcomeStateKeyAt(ctx.resolver, ctx.point, ctx.boundaryPaths, fact.Table)
		if !ok {
			continue
		}
		out = out.AddPathKeyMembership(keyStateKey, tableStateKey)
	}
	return out
}

func applyNormalReturnDynamicValueKeys(ctx normalReturnApplyContext, out state.State) state.State {
	for _, fact := range ctx.normalFacts.DynamicValueKeys {
		containerKey, ok := callOutcomeKeyspaceKeyAt(ctx.resolver, ctx.point, ctx.boundaryPaths, fact.Container)
		if !ok {
			continue
		}
		tableStateKey, ok := callOutcomeStateKeyAt(ctx.resolver, ctx.point, ctx.boundaryPaths, fact.Table)
		if !ok {
			continue
		}
		out = out.AddDynamicIndexValueKeyMembership(containerKey, fact.Site, tableStateKey)
	}
	return out
}

func applyNormalReturnDynamicAllValues(ctx normalReturnApplyContext, out state.State) state.State {
	for _, fact := range ctx.normalFacts.DynamicAllValues {
		containerKey, ok := callOutcomeKeyspaceKeyAt(ctx.resolver, ctx.point, ctx.boundaryPaths, fact.Container)
		if !ok {
			continue
		}
		tableStateKey, ok := callOutcomeStateKeyAt(ctx.resolver, ctx.point, ctx.boundaryPaths, fact.Table)
		if !ok {
			continue
		}
		out = out.AddDynamicIndexAllValuesKeyMembership(containerKey, tableStateKey)
	}
	return out
}

func applyNormalReturnBranchProofs(ctx normalReturnApplyContext, out state.State) state.State {
	for _, proof := range ctx.normalFacts.BranchProofs {
		stateProof, ok := callBranchProofAt(ctx.resolver, ctx.point, ctx.boundaryPaths, proof)
		if !ok {
			continue
		}
		out = out.AddBranchProof(stateProof)
		out = applyNormalReturnPathRelationProof(ctx, out, proof)
	}
	return out
}

func applyNormalReturnPathRelationProof(ctx normalReturnApplyContext, out state.State, proof callboundary.BranchProof) state.State {
	switch proof.Kind {
	case pathevidence.BranchProofPathEqual, pathevidence.BranchProofPathNotEqual:
	default:
		return out
	}
	leftPath, ok := ctx.boundaryPaths.Substitute(proof.Path)
	if !ok {
		return out
	}
	rightPath, ok := ctx.boundaryPaths.Substitute(proof.Other)
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
		out = applyNormalReturnNumFloor(ctx.resolver, ctx.point, ctx.boundaryPaths, out, fact)
	}
	return out
}

func applyNormalReturnRelConstraints(ctx normalReturnApplyContext, out state.State) state.State {
	for _, fact := range ctx.normalFacts.RelConstraints {
		out = applyNormalReturnRelConstraint(ctx.resolver, ctx.point, ctx.boundaryPaths, out, fact)
	}
	return out
}

func applyNormalReturnChannelSelects(ctx normalReturnApplyContext, out state.State) state.State {
	for _, event := range ctx.normalFacts.ChannelSelects {
		fact, ok := callChannelSelectFactAt(ctx.resolver, ctx.point, ctx.boundaryPaths, event)
		if !ok {
			continue
		}
		out = out.AddChannelSelectFact(fact)
	}
	return out
}

func applyNormalReturnFrozenTables(ctx normalReturnApplyContext, out state.State) state.State {
	for _, fact := range ctx.normalFacts.FrozenTables {
		targetPath, ok := ctx.boundaryPaths.Substitute(fact.Target)
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
		targetKey, ok := callOutcomeKeyspaceKeyAt(ctx.resolver, ctx.point, ctx.boundaryPaths, delta.Target)
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
		sourceStateKey, ok := callOutcomeStateKeyAt(ctx.resolver, ctx.point, ctx.boundaryPaths, relation.Source)
		if !ok {
			continue
		}
		intoStateKey, ok := callOutcomeStateKeyAt(ctx.resolver, ctx.point, ctx.boundaryPaths, relation.Into)
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
		targetStateKey, ok := callOutcomeVisibleStateKeyAt(ctx.resolver, ctx.point, ctx.boundaryPaths, fact.Target)
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
		targetPath, ok := ctx.boundaryPaths.Substitute(event.Target)
		if !ok {
			continue
		}
		targetStateKey, ok := factStateKeyAt(ctx.resolver, ctx.point, targetPath)
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

func callOutcomeVisibleStateKeyAt(
	resolver *visibility.Resolver,
	point cfg.Point,
	boundaryPaths callboundary.PathBindings,
	path pathdom.Path,
) (pathaddr.StateKey, bool) {
	targetPath, ok := boundaryPaths.Substitute(path)
	if !ok {
		return "", false
	}
	return factStateKeyAt(resolver, point, targetPath)
}
