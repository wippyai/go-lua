package projectsummary

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

type normalReturnProjectContext struct {
	reg      *axis.Registry
	result   ResultReader
	exit     state.State
	params   []path.Path
	ks       *keyspace.KeySpace
	boundary boundaryPathProjector
}

var normalReturnProjectLanes = callboundary.BindNormalReturnFactLanes("normal-return project", map[callboundary.NormalReturnFactLaneID]func(normalReturnProjectContext, *callboundary.NormalReturnFacts){
	callboundary.LanePathRefinements:          projectNormalReturnPathRefinements,
	callboundary.LanePathInvalidations:        projectNormalReturnPathInvalidations,
	callboundary.LanePersistentPathWrites:     projectNormalReturnPersistentPathWrites,
	callboundary.LanePathStaticMembers:        projectNormalReturnPathStaticMembers,
	callboundary.LanePathStaticMemberDeltas:   projectNormalReturnPathStaticMemberDeltas,
	callboundary.LanePathPresenceImplications: projectNormalReturnPathPresenceImplications,
	callboundary.LaneDynamicIndexFacts:        projectNormalReturnDynamicIndexFacts,
	callboundary.LaneKeyMemberships:           projectNormalReturnKeyMemberships,
	callboundary.LaneDynamicValueKeys:         projectNormalReturnDynamicValueKeys,
	callboundary.LaneDynamicAllValues:         projectNormalReturnDynamicAllValues,
	callboundary.LaneBranchProofs:             projectNormalReturnBranchProofs,
	callboundary.LaneNumFloors:                projectNormalReturnNumFloors,
	callboundary.LaneRelConstraints:           projectNormalReturnRelConstraints,
	callboundary.LaneChannelSelects:           projectNormalReturnChannelSelects,
	callboundary.LaneFrozenTables:             projectNormalReturnFrozenTables,
	callboundary.LaneEffectDeltas:             projectNormalReturnEffectDeltas,
	callboundary.LaneEscapeEvents:             projectNormalReturnEscapeEvents,
	callboundary.LaneStoreRelations:           projectNormalReturnStoreRelations,
	callboundary.LaneLifecycleFacts:           projectNormalReturnLifecycleFacts,
}, func(handler func(normalReturnProjectContext, *callboundary.NormalReturnFacts)) bool {
	return handler != nil
})

func projectNormalReturnPathRefinements(ctx normalReturnProjectContext, out *callboundary.NormalReturnFacts) {
	ctx.exit.ForEachPathRefinement(func(pathKey keyspace.Key, value product.Value) bool {
		value, useful := callboundary.ProjectPathRefinementValue(ctx.reg, value)
		if !useful {
			return true
		}
		target, ok := ctx.boundary.KeyspacePlaceholderPath(pathKey)
		if !ok {
			return true
		}
		out.PathRefinements = append(out.PathRefinements, callboundary.PathValueFact{
			Path:  target,
			Value: value,
		})
		return true
	})
}

func projectNormalReturnPathInvalidations(ctx normalReturnProjectContext, out *callboundary.NormalReturnFacts) {
	out.PathInvalidations = append(out.PathInvalidations, projectAssignmentPathInvalidations(ctx.result, ctx.boundary)...)
	if snapshot := ctx.exit.DynamicIndexFactsSnapshot(); !snapshot.Top {
		for stateKey, stateFact := range snapshot.Facts {
			table, ok := ctx.boundary.KeyspaceStatePath(stateKey.Table)
			if !ok {
				continue
			}
			domain := dynamicindex.Domain(ctx.reg)
			if domain.Equal(stateFact, dynamicindex.Bottom(ctx.reg)) {
				continue
			}
			out.PathInvalidations = append(out.PathInvalidations, callboundary.PathInvalidationFact{
				Path: table,
			})
		}
	}
	if snapshot := ctx.exit.EffectDeltasSnapshot(); !snapshot.Top {
		for stateKey := range snapshot.Deltas {
			target, ok := ctx.boundary.KeyspacePlaceholderPath(stateKey.Target)
			if !ok {
				continue
			}
			if stateKey.Kind == effectdelta.Mutation && callboundary.IsPathInvalidationEffectSite(stateKey.Site) {
				out.PathInvalidations = append(out.PathInvalidations, callboundary.PathInvalidationFact{
					Path: target,
				})
				continue
			}
			if stateKey.Kind == effectdelta.Mutation && callboundary.IsPathStructuralPreservingInvalidationEffectSite(stateKey.Site) {
				out.PathInvalidations = append(out.PathInvalidations, callboundary.PathInvalidationFact{
					Path:                      target,
					PreserveStructuralWitness: true,
				})
			}
		}
	}
}

func projectNormalReturnPersistentPathWrites(ctx normalReturnProjectContext, out *callboundary.NormalReturnFacts) {
	out.PersistentPathWrites = append(out.PersistentPathWrites, projectAssignmentPersistentPathWrites(ctx.reg, ctx.result, ctx.exit)...)
	out.PersistentPathWrites = append(out.PersistentPathWrites, projectCallOutcomePersistentPathWrites(ctx.result, ctx.params)...)
}

func projectNormalReturnPathStaticMembers(ctx normalReturnProjectContext, out *callboundary.NormalReturnFacts) {
	bottom := product.Bottom(ctx.reg)
	ctx.exit.ForEachPathStaticMember(func(pathKey keyspace.Key, value product.Value) bool {
		if product.Equal(ctx.reg, value, bottom) {
			return true
		}
		target, ok := ctx.boundary.KeyspaceStatePath(pathKey)
		if !ok {
			return true
		}
		out.PathStaticMembers = append(out.PathStaticMembers, callboundary.PathStaticMemberFact{
			Path:  target,
			Value: value,
		})
		return true
	})
}

func projectNormalReturnPathStaticMemberDeltas(ctx normalReturnProjectContext, out *callboundary.NormalReturnFacts) {
	out.PathStaticMemberDeltas = append(out.PathStaticMemberDeltas, projectAssignmentPathStaticMemberDeltas(ctx.reg, ctx.result, ctx.exit, ctx.params, ctx.boundary)...)
}

func projectNormalReturnPathPresenceImplications(ctx normalReturnProjectContext, out *callboundary.NormalReturnFacts) {
	if snapshot := ctx.exit.PathPresenceImplicationsSnapshot(ctx.ks); !snapshot.Bottom {
		for _, implication := range snapshot.Implications {
			if implication.HasTriggerPathEqual {
				continue
			}
			trigger, ok := ctx.boundary.KeyspaceStatePath(implication.Trigger)
			if !ok {
				continue
			}
			target, ok := ctx.boundary.KeyspaceStatePath(implication.Target)
			if !ok {
				continue
			}
			fact := callboundary.PathPresenceImplicationFact{
				Trigger:         trigger,
				TriggerPresence: implication.TriggerPresence,
				HasTriggerValue: implication.HasTriggerValue,
				Target:          target,
				TargetPresence:  implication.TargetPresence,
				HasTargetValue:  implication.HasTargetValue,
			}
			if implication.HasTriggerValue {
				fact.TriggerValue = portableBoundaryValue(ctx.reg, implication.TriggerValue)
			}
			if implication.HasTargetValue {
				fact.TargetValue = portableBoundaryValue(ctx.reg, implication.TargetValue)
			}
			out.PathPresenceImplications = append(out.PathPresenceImplications, fact)
		}
	}
}

func projectNormalReturnDynamicIndexFacts(ctx normalReturnProjectContext, out *callboundary.NormalReturnFacts) {
	out.DynamicIndexFacts = append(out.DynamicIndexFacts, projectAssignmentDynamicIndexFacts(ctx.reg, ctx.result, ctx.params)...)
	if snapshot := ctx.exit.DynamicIndexFactsSnapshot(); !snapshot.Top {
		for stateKey, stateFact := range snapshot.Facts {
			table, ok := ctx.boundary.KeyspaceStatePath(stateKey.Table)
			if !ok {
				continue
			}
			domain := dynamicindex.Domain(ctx.reg)
			if domain.Equal(stateFact, dynamicindex.Bottom(ctx.reg)) || domain.Equal(stateFact, dynamicindex.Top()) {
				continue
			}
			fact := callboundary.DynamicIndexFact{
				Table: table,
				Site:  stateKey.Site,
				Value: stateFact,
			}
			if keyPath, ok := dynamicIndexSourcePlaceholderPath(ctx.result, ctx.params, stateKey.Site, func(write factflow.DynamicIndexWrite) factflow.ValueSource {
				return write.KeySource()
			}); ok {
				fact.KeyPath = keyPath
			}
			if valuePath, ok := dynamicIndexSourcePlaceholderPath(ctx.result, ctx.params, stateKey.Site, func(write factflow.DynamicIndexWrite) factflow.ValueSource {
				return write.Source()
			}); ok {
				fact.ValuePath = valuePath
			}
			out.DynamicIndexFacts = append(out.DynamicIndexFacts, fact)
		}
	}
}

func projectNormalReturnKeyMemberships(ctx normalReturnProjectContext, out *callboundary.NormalReturnFacts) {
	if snapshot := ctx.exit.KeyMembershipsSnapshot(); !snapshot.Bottom && !snapshot.Top {
		for _, membership := range snapshot.Memberships {
			if membership.Kind != state.KeyMembershipPath {
				continue
			}
			keyPath, ok := ctx.boundary.StatePath(membership.Key)
			if !ok {
				continue
			}
			tablePath, ok := ctx.boundary.StatePath(membership.Table)
			if !ok {
				continue
			}
			out.KeyMemberships = append(out.KeyMemberships, callboundary.KeyMembershipFact{
				Key:   keyPath,
				Table: tablePath,
			})
		}
	}
}

func projectNormalReturnDynamicValueKeys(ctx normalReturnProjectContext, out *callboundary.NormalReturnFacts) {
	out.DynamicValueKeys = append(out.DynamicValueKeys, projectAssignmentDynamicValueKeyMemberships(ctx.result, ctx.params, ctx.boundary)...)
	if snapshot := ctx.exit.KeyMembershipsSnapshot(); !snapshot.Bottom && !snapshot.Top {
		for _, membership := range snapshot.Memberships {
			if membership.Kind != state.KeyMembershipDynamicIndexValue {
				continue
			}
			containerPath, ok := ctx.boundary.KeyspaceStatePath(membership.Container)
			if !ok {
				continue
			}
			tablePath, ok := ctx.boundary.StatePath(membership.Table)
			if !ok {
				continue
			}
			out.DynamicValueKeys = append(out.DynamicValueKeys, callboundary.DynamicValueKeyMembershipFact{
				Container: containerPath,
				Site:      membership.Site,
				Table:     tablePath,
			})
		}
	}
}

func projectNormalReturnDynamicAllValues(ctx normalReturnProjectContext, out *callboundary.NormalReturnFacts) {
	if snapshot := ctx.exit.KeyMembershipsSnapshot(); !snapshot.Bottom && !snapshot.Top {
		for _, membership := range snapshot.Memberships {
			if membership.Kind != state.KeyMembershipDynamicIndexAllValues {
				continue
			}
			containerPath, ok := ctx.boundary.KeyspaceStatePath(membership.Container)
			if !ok {
				continue
			}
			tablePath, ok := ctx.boundary.StatePath(membership.Table)
			if !ok {
				continue
			}
			out.DynamicAllValues = append(out.DynamicAllValues, callboundary.DynamicAllValueKeyMembershipFact{
				Container: containerPath,
				Table:     tablePath,
			})
		}
	}
}

func projectNormalReturnBranchProofs(ctx normalReturnProjectContext, out *callboundary.NormalReturnFacts) {
	if snapshot := ctx.exit.BranchProofsSnapshot(ctx.ks); !snapshot.Bottom && !snapshot.Top {
		for _, stateProof := range snapshot.Proofs {
			target, ok := ctx.boundary.KeyspacePlaceholderPath(stateProof.Path)
			if !ok {
				continue
			}
			kind, ok := projectBranchProofKind(stateProof.Kind)
			if !ok {
				continue
			}
			proof := callboundary.BranchProof{
				Kind: kind,
				Path: target,
			}
			switch kind {
			case pathevidence.BranchProofPathPresence:
				if stateProof.Presence.IsBottom() || stateProof.Presence.IsTop() {
					continue
				}
				proof.Presence = stateProof.Presence
			case pathevidence.BranchProofPathEqual, pathevidence.BranchProofPathNotEqual, pathevidence.BranchProofIndexInRange:
				other, ok := ctx.boundary.KeyspacePlaceholderPath(stateProof.Other)
				if !ok {
					continue
				}
				proof.Other = other
			default:
				continue
			}
			out.BranchProofs = append(out.BranchProofs, proof)
		}
	}
}

func projectNormalReturnNumFloors(ctx normalReturnProjectContext, out *callboundary.NormalReturnFacts) {
	if snapshot := ctx.exit.NumFloorsSnapshot(ctx.ks); !snapshot.Bottom {
		for stateKey, floor := range snapshot.Floors {
			target, ok := ctx.boundary.StatePath(stateKey)
			if !ok {
				continue
			}
			out.NumFloors = append(out.NumFloors, callboundary.NumFloorFact{
				Path:  target,
				Floor: floor,
			})
		}
	}
}

func projectNormalReturnRelConstraints(ctx normalReturnProjectContext, out *callboundary.NormalReturnFacts) {
	if snapshot := ctx.exit.RelConstraints(); !snapshot.Bottom && !snapshot.Top {
		for _, constraint := range snapshot.Constraints {
			projected, ok := ctx.boundary.RelConstraintFact(constraint)
			if !ok {
				continue
			}
			out.RelConstraints = append(out.RelConstraints, projected)
		}
	}
}

func projectNormalReturnChannelSelects(ctx normalReturnProjectContext, out *callboundary.NormalReturnFacts) {
	if snapshot := ctx.exit.ChannelSelectFactsSnapshot(); !snapshot.Bottom && !snapshot.Top {
		for _, stateFact := range snapshot.Facts {
			kind, ok := projectChannelSelectKind(stateFact.Kind)
			if !ok {
				continue
			}
			fact := callboundary.ChannelSelectFact{
				Select:     channelselectfact.ID(stateFact.Select),
				Kind:       kind,
				Index:      stateFact.Index,
				HasDefault: stateFact.HasDefault,
			}
			if stateFact.Result != "" {
				resultPath, ok := ctx.boundary.StatePath(stateFact.Result)
				if !ok {
					continue
				}
				fact.Result = resultPath
			}
			if stateFact.Case != "" {
				casePath, ok := ctx.boundary.StatePath(stateFact.Case)
				if !ok {
					continue
				}
				fact.Case = casePath
			}
			out.ChannelSelects = append(out.ChannelSelects, fact)
		}
	}
}

func projectNormalReturnFrozenTables(ctx normalReturnProjectContext, out *callboundary.NormalReturnFacts) {
	if snapshot := ctx.exit.FrozenTablesSnapshot(); !snapshot.Bottom && !snapshot.Top {
		frozenPaths := frozenTablePlaceholderPaths(ctx.reg, ctx.ks, ctx.exit, ctx.boundary.Params())
		for _, id := range snapshot.Tables {
			for _, target := range frozenPaths[id] {
				out.FrozenTables = append(out.FrozenTables, callboundary.FrozenTableFact{
					Target: target,
				})
			}
		}
	}
	if snapshot := ctx.exit.EffectDeltasSnapshot(); !snapshot.Top {
		for stateKey := range snapshot.Deltas {
			target, ok := ctx.boundary.KeyspacePlaceholderPath(stateKey.Target)
			if !ok {
				continue
			}
			if stateKey.Kind == effectdelta.Freeze && callboundary.IsFrozenTableEffectSite(stateKey.Site) {
				out.FrozenTables = append(out.FrozenTables, callboundary.FrozenTableFact{
					Target: target,
				})
			}
		}
	}
}

func projectNormalReturnEffectDeltas(ctx normalReturnProjectContext, out *callboundary.NormalReturnFacts) {
	if snapshot := ctx.exit.EffectDeltasSnapshot(); !snapshot.Top {
		for stateKey, stateDelta := range snapshot.Deltas {
			target, ok := ctx.boundary.KeyspacePlaceholderPath(stateKey.Target)
			if !ok {
				continue
			}
			if stateKey.Kind == effectdelta.Freeze && callboundary.IsFrozenTableEffectSite(stateKey.Site) {
				continue
			}
			if stateKey.Kind == effectdelta.Mutation &&
				(callboundary.IsPathInvalidationEffectSite(stateKey.Site) ||
					callboundary.IsPathStructuralPreservingInvalidationEffectSite(stateKey.Site)) {
				continue
			}
			delta := callboundary.EffectDelta{
				Target: target,
				Site:   stateKey.Site,
				Kind:   stateKey.Kind,
				Value:  stateDelta,
			}
			domain := effectdelta.Domain(ctx.reg)
			if domain.Equal(delta.Value, domain.Bottom()) || domain.Equal(delta.Value, effectdelta.Top()) {
				continue
			}
			out.EffectDeltas = append(out.EffectDeltas, delta)
		}
	}
}

func projectNormalReturnEscapeEvents(ctx normalReturnProjectContext, out *callboundary.NormalReturnFacts) {
	if snapshot := ctx.exit.EscapeEventsSnapshot(); !snapshot.Bottom && !snapshot.Top {
		for _, event := range snapshot.Facts {
			target, ok := ctx.boundary.StatePath(event.Target)
			if !ok {
				continue
			}
			out.EscapeEvents = append(out.EscapeEvents, callboundary.EscapeEventFact{
				Target:    target,
				Kind:      event.Kind,
				Recursive: event.Recursive,
			})
		}
	}
}

func projectNormalReturnStoreRelations(ctx normalReturnProjectContext, out *callboundary.NormalReturnFacts) {
	out.StoreRelations = append(out.StoreRelations, projectAssignmentStoreRelations(ctx.result, ctx.params)...)
	if snapshot := ctx.exit.StoreRelationsSnapshot(); !snapshot.Bottom && !snapshot.Top {
		for _, relation := range snapshot.Relations {
			source, ok := ctx.boundary.StatePath(relation.Source)
			if !ok {
				continue
			}
			into, ok := ctx.boundary.StatePath(relation.Into)
			if !ok {
				continue
			}
			out.StoreRelations = append(out.StoreRelations, callboundary.StoreRelationFact{
				Source: source,
				Into:   into,
			})
		}
	}
}

func projectNormalReturnLifecycleFacts(ctx normalReturnProjectContext, out *callboundary.NormalReturnFacts) {
	out.LifecycleFacts = append(out.LifecycleFacts, projectCallOutcomeLifecycleFacts(ctx.result, ctx.params)...)
}
