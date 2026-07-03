package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

type normalReturnSummaryLane struct {
	id callboundary.NormalReturnFactLaneID

	empty          func(*callboundary.NormalReturnFacts) bool
	normalize      func(*axis.Registry, *callboundary.NormalReturnFacts, *callboundary.NormalReturnFacts)
	normalizeOwned func(*axis.Registry, *callboundary.NormalReturnFacts, *callboundary.NormalReturnFacts)
	clone          func(*callboundary.NormalReturnFacts, *callboundary.NormalReturnFacts)
	equal          func(*axis.Registry, *callboundary.NormalReturnFacts, *callboundary.NormalReturnFacts) bool
	lessOrEq       func(*axis.Registry, *callboundary.NormalReturnFacts, *callboundary.NormalReturnFacts) bool
	join           func(*axis.Registry, *callboundary.NormalReturnFacts, *callboundary.NormalReturnFacts, *callboundary.NormalReturnFacts)
	widen          func(*axis.Registry, *callboundary.NormalReturnFacts, *callboundary.NormalReturnFacts, *callboundary.NormalReturnFacts)
}

func normalReturnSummarySliceLane[T any](
	id callboundary.NormalReturnFactLaneID,
	get func(*callboundary.NormalReturnFacts) *[]T,
	normalize func(*axis.Registry, []T) []T,
	clone func([]T) []T,
	equal func(*axis.Registry, []T, []T) bool,
	lessOrEq func(*axis.Registry, []T, []T) bool,
	join func(*axis.Registry, []T, []T) []T,
	widen func(*axis.Registry, []T, []T) []T,
) normalReturnSummaryLane {
	return normalReturnSummarySliceLaneOwned(id, get, normalize, normalize, clone, equal, lessOrEq, join, widen)
}

func normalReturnSummarySliceLaneOwned[T any](
	id callboundary.NormalReturnFactLaneID,
	get func(*callboundary.NormalReturnFacts) *[]T,
	normalize func(*axis.Registry, []T) []T,
	normalizeOwned func(*axis.Registry, []T) []T,
	clone func([]T) []T,
	equal func(*axis.Registry, []T, []T) bool,
	lessOrEq func(*axis.Registry, []T, []T) bool,
	join func(*axis.Registry, []T, []T) []T,
	widen func(*axis.Registry, []T, []T) []T,
) normalReturnSummaryLane {
	return normalReturnSummaryLane{
		id: id,
		empty: func(in *callboundary.NormalReturnFacts) bool {
			return len(*get(in)) == 0
		},
		normalize: func(reg *axis.Registry, in *callboundary.NormalReturnFacts, out *callboundary.NormalReturnFacts) {
			*get(out) = normalize(reg, *get(in))
		},
		normalizeOwned: func(reg *axis.Registry, in *callboundary.NormalReturnFacts, out *callboundary.NormalReturnFacts) {
			*get(out) = normalizeOwned(reg, *get(in))
		},
		clone: func(in *callboundary.NormalReturnFacts, out *callboundary.NormalReturnFacts) {
			*get(out) = clone(*get(in))
		},
		equal: func(reg *axis.Registry, a, b *callboundary.NormalReturnFacts) bool {
			return equal(reg, *get(a), *get(b))
		},
		lessOrEq: func(reg *axis.Registry, a, b *callboundary.NormalReturnFacts) bool {
			return lessOrEq(reg, *get(a), *get(b))
		},
		join: func(reg *axis.Registry, a, b *callboundary.NormalReturnFacts, out *callboundary.NormalReturnFacts) {
			*get(out) = join(reg, *get(a), *get(b))
		},
		widen: func(reg *axis.Registry, prev, next *callboundary.NormalReturnFacts, out *callboundary.NormalReturnFacts) {
			*get(out) = widen(reg, *get(prev), *get(next))
		},
	}
}

func normalReturnSummaryLaneNoRegOwned[T any](
	id callboundary.NormalReturnFactLaneID,
	get func(*callboundary.NormalReturnFacts) *[]T,
	normalize func([]T) []T,
	normalizeOwned func([]T) []T,
	clone func([]T) []T,
	equal func([]T, []T) bool,
	lessOrEq func([]T, []T) bool,
	join func([]T, []T) []T,
	widen func([]T, []T) []T,
) normalReturnSummaryLane {
	return normalReturnSummarySliceLaneOwned(id, get,
		func(_ *axis.Registry, in []T) []T { return normalize(in) },
		func(_ *axis.Registry, in []T) []T { return normalizeOwned(in) },
		clone,
		func(_ *axis.Registry, a, b []T) bool { return equal(a, b) },
		func(_ *axis.Registry, a, b []T) bool { return lessOrEq(a, b) },
		func(_ *axis.Registry, a, b []T) []T { return join(a, b) },
		func(_ *axis.Registry, a, b []T) []T { return widen(a, b) },
	)
}

func normalReturnSummaryLaneNoReg[T any](
	id callboundary.NormalReturnFactLaneID,
	get func(*callboundary.NormalReturnFacts) *[]T,
	normalize func([]T) []T,
	clone func([]T) []T,
	equal func([]T, []T) bool,
	lessOrEq func([]T, []T) bool,
	join func([]T, []T) []T,
	widen func([]T, []T) []T,
) normalReturnSummaryLane {
	return normalReturnSummarySliceLane(id, get,
		func(_ *axis.Registry, in []T) []T { return normalize(in) },
		clone,
		func(_ *axis.Registry, a, b []T) bool { return equal(a, b) },
		func(_ *axis.Registry, a, b []T) bool { return lessOrEq(a, b) },
		func(_ *axis.Registry, a, b []T) []T { return join(a, b) },
		func(_ *axis.Registry, a, b []T) []T { return widen(a, b) },
	)
}

func buildNormalReturnSummaryLanes(handlers map[callboundary.NormalReturnFactLaneID]normalReturnSummaryLane) []normalReturnSummaryLane {
	bindings := callboundary.BindNormalReturnFactLanes("normal-return summary", handlers, func(lane normalReturnSummaryLane) bool {
		return lane.id != ""
	})
	out := make([]normalReturnSummaryLane, 0, len(bindings))
	for _, binding := range bindings {
		lane := binding.Value
		if lane.id != binding.ID {
			panic("normal-return summary lane registered with mismatched ID for " + string(binding.ID))
		}
		out = append(out, lane)
	}
	return out
}

var normalReturnSummaryLanes = buildNormalReturnSummaryLanes(map[callboundary.NormalReturnFactLaneID]normalReturnSummaryLane{
	callboundary.LanePathRefinements: normalReturnSummarySliceLaneOwned(callboundary.LanePathRefinements,
		func(f *callboundary.NormalReturnFacts) *[]callboundary.PathValueFact { return &f.PathRefinements },
		normalizePathValueFacts, normalizePathValueFactsOwned, clonePathValueFacts, pathValueFactsEqual, pathValueFactsLessOrEq, joinPathValueFacts, widenPathValueFacts),
	callboundary.LanePersistentPathWrites: normalReturnSummarySliceLaneOwned(callboundary.LanePersistentPathWrites,
		func(f *callboundary.NormalReturnFacts) *[]callboundary.PathValueFact { return &f.PersistentPathWrites },
		normalizePersistentPathWrites, normalizePersistentPathWritesOwned, clonePathValueFacts, persistentPathWritesEqual, persistentPathWritesLessOrEq, joinPersistentPathWrites, widenPersistentPathWrites),
	callboundary.LanePathStaticMembers: normalReturnSummarySliceLaneOwned(callboundary.LanePathStaticMembers,
		func(f *callboundary.NormalReturnFacts) *[]callboundary.PathStaticMemberFact {
			return &f.PathStaticMembers
		},
		normalizePathStaticMemberFacts, normalizePathStaticMemberFactsOwned, clonePathStaticMemberFacts, pathStaticMemberFactsEqual, pathStaticMemberFactsLessOrEq, joinPathStaticMemberFacts, widenPathStaticMemberFacts),
	callboundary.LanePathInvalidations: normalReturnSummaryLaneNoRegOwned(callboundary.LanePathInvalidations,
		func(f *callboundary.NormalReturnFacts) *[]callboundary.PathInvalidationFact {
			return &f.PathInvalidations
		},
		pathInvalidationLane.Normalize, pathInvalidationLane.NormalizeOwned, pathInvalidationLane.Clone, pathInvalidationLane.Equal, pathInvalidationLane.LessOrEq, pathInvalidationLane.Join, pathInvalidationLane.Widen),
	callboundary.LaneDynamicIndexFacts: normalReturnSummarySliceLaneOwned(callboundary.LaneDynamicIndexFacts,
		func(f *callboundary.NormalReturnFacts) *[]callboundary.DynamicIndexFact { return &f.DynamicIndexFacts },
		normalizeDynamicIndexFacts, normalizeDynamicIndexFactsOwned, cloneDynamicIndexFacts, dynamicIndexFactsEqual, dynamicIndexFactsLessOrEq, joinDynamicIndexFacts, widenDynamicIndexFacts),
	callboundary.LaneKeyMemberships: normalReturnSummaryLaneNoRegOwned(callboundary.LaneKeyMemberships,
		func(f *callboundary.NormalReturnFacts) *[]callboundary.KeyMembershipFact { return &f.KeyMemberships },
		keyMembershipLane.Normalize, keyMembershipLane.NormalizeOwned, keyMembershipLane.Clone, keyMembershipLane.Equal, keyMembershipLane.LessOrEq, keyMembershipLane.Join, keyMembershipLane.Widen),
	callboundary.LaneDynamicValueKeys: normalReturnSummaryLaneNoRegOwned(callboundary.LaneDynamicValueKeys,
		func(f *callboundary.NormalReturnFacts) *[]callboundary.DynamicValueKeyMembershipFact {
			return &f.DynamicValueKeys
		},
		dynamicValueKeyMembershipLane.Normalize, dynamicValueKeyMembershipLane.NormalizeOwned, dynamicValueKeyMembershipLane.Clone, dynamicValueKeyMembershipLane.Equal, dynamicValueKeyMembershipLane.LessOrEq, dynamicValueKeyMembershipLane.Join, dynamicValueKeyMembershipLane.Widen),
	callboundary.LaneDynamicAllValues: normalReturnSummaryLaneNoRegOwned(callboundary.LaneDynamicAllValues,
		func(f *callboundary.NormalReturnFacts) *[]callboundary.DynamicAllValueKeyMembershipFact {
			return &f.DynamicAllValues
		},
		dynamicAllValueKeyMembershipLane.Normalize, dynamicAllValueKeyMembershipLane.NormalizeOwned, dynamicAllValueKeyMembershipLane.Clone, dynamicAllValueKeyMembershipLane.Equal, dynamicAllValueKeyMembershipLane.LessOrEq, dynamicAllValueKeyMembershipLane.Join, dynamicAllValueKeyMembershipLane.Widen),
	callboundary.LaneBranchProofs: normalReturnSummaryLaneNoRegOwned(callboundary.LaneBranchProofs,
		func(f *callboundary.NormalReturnFacts) *[]callboundary.BranchProof { return &f.BranchProofs },
		branchProofLane.Normalize, branchProofLane.NormalizeOwned, branchProofLane.Clone, branchProofLane.Equal, branchProofLane.LessOrEq, branchProofLane.Join, branchProofLane.Join),
	callboundary.LanePathPresenceImplications: normalReturnSummaryLaneNoRegOwned(callboundary.LanePathPresenceImplications,
		func(f *callboundary.NormalReturnFacts) *[]callboundary.PathPresenceImplicationFact {
			return &f.PathPresenceImplications
		},
		pathPresenceImplicationLane.Normalize,
		pathPresenceImplicationLane.NormalizeOwned,
		pathPresenceImplicationLane.Clone,
		pathPresenceImplicationLane.Equal,
		pathPresenceImplicationLane.LessOrEq,
		pathPresenceImplicationLane.Join,
		pathPresenceImplicationLane.Widen),
	callboundary.LaneChannelSelects: normalReturnSummaryLaneNoRegOwned(callboundary.LaneChannelSelects,
		func(f *callboundary.NormalReturnFacts) *[]callboundary.ChannelSelectFact { return &f.ChannelSelects },
		channelSelectLane.Normalize, channelSelectLane.NormalizeOwned, channelSelectLane.Clone, channelSelectLane.Equal, channelSelectLane.LessOrEq, channelSelectLane.Join, channelSelectLane.Join),
	callboundary.LaneFrozenTables: normalReturnSummaryLaneNoRegOwned(callboundary.LaneFrozenTables,
		func(f *callboundary.NormalReturnFacts) *[]callboundary.FrozenTableFact { return &f.FrozenTables },
		frozenTableLane.Normalize, frozenTableLane.NormalizeOwned, frozenTableLane.Clone, frozenTableLane.Equal, frozenTableLane.LessOrEq, frozenTableLane.Join, frozenTableLane.Join),
	callboundary.LaneEffectDeltas: normalReturnSummarySliceLaneOwned(callboundary.LaneEffectDeltas,
		func(f *callboundary.NormalReturnFacts) *[]callboundary.EffectDelta { return &f.EffectDeltas },
		normalizeEffectDeltas, normalizeEffectDeltasOwned, cloneEffectDeltas, effectDeltasEqual, effectDeltasLessOrEq, joinEffectDeltas, widenEffectDeltas),
	callboundary.LaneEscapeEvents: normalReturnSummaryLaneNoRegOwned(callboundary.LaneEscapeEvents,
		func(f *callboundary.NormalReturnFacts) *[]callboundary.EscapeEventFact { return &f.EscapeEvents },
		escapeEventLane.Normalize, escapeEventLane.NormalizeOwned, escapeEventLane.Clone, escapeEventLane.Equal, escapeEventLane.LessOrEq, escapeEventLane.Join, escapeEventLane.Widen),
	callboundary.LaneStoreRelations: normalReturnSummaryLaneNoRegOwned(callboundary.LaneStoreRelations,
		func(f *callboundary.NormalReturnFacts) *[]callboundary.StoreRelationFact { return &f.StoreRelations },
		storeRelationLane.Normalize, storeRelationLane.NormalizeOwned, storeRelationLane.Clone, storeRelationLane.Equal, storeRelationLane.LessOrEq, storeRelationLane.Join, storeRelationLane.Widen),
	callboundary.LaneLifecycleFacts: normalReturnSummaryLaneNoRegOwned(callboundary.LaneLifecycleFacts,
		func(f *callboundary.NormalReturnFacts) *[]callboundary.LifecycleFact { return &f.LifecycleFacts },
		lifecycleLane.Normalize, lifecycleLane.NormalizeOwned, lifecycleLane.Clone, lifecycleLane.Equal, lifecycleLane.LessOrEq, lifecycleLane.Join, lifecycleLane.Widen),
	callboundary.LaneNumFloors: normalReturnSummaryLaneNoRegOwned(callboundary.LaneNumFloors,
		func(f *callboundary.NormalReturnFacts) *[]callboundary.NumFloorFact { return &f.NumFloors },
		numFloorLane.Normalize, numFloorLane.NormalizeOwned, numFloorLane.Clone, numFloorLane.Equal, numFloorLane.LessOrEq, numFloorLane.Join, numFloorLane.Widen),
	callboundary.LaneRelConstraints: normalReturnSummaryLaneNoRegOwned(callboundary.LaneRelConstraints,
		func(f *callboundary.NormalReturnFacts) *[]callboundary.RelConstraintFact { return &f.RelConstraints },
		relConstraintLane.Normalize, relConstraintLane.NormalizeOwned, relConstraintLane.Clone, relConstraintLane.Equal, relConstraintLane.LessOrEq, relConstraintLane.Join, relConstraintLane.Widen),
})
