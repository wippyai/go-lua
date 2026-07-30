package callboundary

import (
	"github.com/wippyai/go-lua/__legacy/analysis/domain/lattice/factmap"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

type normalReturnSummaryLane struct {
	empty          func(*NormalReturnFacts) bool
	normalize      func(*axis.Registry, *NormalReturnFacts, *NormalReturnFacts)
	normalizeOwned func(*axis.Registry, *NormalReturnFacts, *NormalReturnFacts)
	clone          func(*NormalReturnFacts, *NormalReturnFacts)
	equal          func(*axis.Registry, *NormalReturnFacts, *NormalReturnFacts) bool
	lessOrEq       func(*axis.Registry, *NormalReturnFacts, *NormalReturnFacts) bool
	join           func(*axis.Registry, *NormalReturnFacts, *NormalReturnFacts, *NormalReturnFacts)
	widen          func(*axis.Registry, *NormalReturnFacts, *NormalReturnFacts, *NormalReturnFacts)
}

func normalReturnSummarySliceLaneOwned[T any](
	get func(*NormalReturnFacts) *[]T,
	normalize func(*axis.Registry, []T) []T,
	normalizeOwned func(*axis.Registry, []T) []T,
	clone func([]T) []T,
	equal func(*axis.Registry, []T, []T) bool,
	lessOrEq func(*axis.Registry, []T, []T) bool,
	join func(*axis.Registry, []T, []T) []T,
	widen func(*axis.Registry, []T, []T) []T,
) normalReturnSummaryLane {
	return normalReturnSummaryLane{
		empty: func(in *NormalReturnFacts) bool {
			return len(*get(in)) == 0
		},
		normalize: func(reg *axis.Registry, in *NormalReturnFacts, out *NormalReturnFacts) {
			*get(out) = normalize(reg, *get(in))
		},
		normalizeOwned: func(reg *axis.Registry, in *NormalReturnFacts, out *NormalReturnFacts) {
			*get(out) = normalizeOwned(reg, *get(in))
		},
		clone: func(in *NormalReturnFacts, out *NormalReturnFacts) {
			*get(out) = clone(*get(in))
		},
		equal: func(reg *axis.Registry, a, b *NormalReturnFacts) bool {
			return equal(reg, *get(a), *get(b))
		},
		lessOrEq: func(reg *axis.Registry, a, b *NormalReturnFacts) bool {
			return lessOrEq(reg, *get(a), *get(b))
		},
		join: func(reg *axis.Registry, a, b *NormalReturnFacts, out *NormalReturnFacts) {
			*get(out) = join(reg, *get(a), *get(b))
		},
		widen: func(reg *axis.Registry, prev, next *NormalReturnFacts, out *NormalReturnFacts) {
			*get(out) = widen(reg, *get(prev), *get(next))
		},
	}
}

func normalReturnSummaryLaneNoRegOwned[T any](
	get func(*NormalReturnFacts) *[]T,
	normalize func([]T) []T,
	normalizeOwned func([]T) []T,
	clone func([]T) []T,
	equal func([]T, []T) bool,
	lessOrEq func([]T, []T) bool,
	join func([]T, []T) []T,
	widen func([]T, []T) []T,
) normalReturnSummaryLane {
	return normalReturnSummarySliceLaneOwned(get,
		func(_ *axis.Registry, in []T) []T { return normalize(in) },
		func(_ *axis.Registry, in []T) []T { return normalizeOwned(in) },
		clone,
		func(_ *axis.Registry, a, b []T) bool { return equal(a, b) },
		func(_ *axis.Registry, a, b []T) bool { return lessOrEq(a, b) },
		func(_ *axis.Registry, a, b []T) []T { return join(a, b) },
		func(_ *axis.Registry, a, b []T) []T { return widen(a, b) },
	)
}

func normalReturnSummaryFactMapLane[K comparable, F any, V any](
	get func(*NormalReturnFacts) *[]F,
	makeMap func(*axis.Registry) factmap.Map[K, F, V],
	clone func([]F) []F,
) normalReturnSummaryLane {
	return normalReturnSummarySliceLaneOwned(get,
		func(reg *axis.Registry, in []F) []F { return makeMap(reg).Normalize(in) },
		func(reg *axis.Registry, in []F) []F { return makeMap(reg).NormalizeOwned(in) },
		clone,
		func(reg *axis.Registry, a, b []F) bool { return makeMap(reg).Equal(a, b) },
		func(reg *axis.Registry, a, b []F) bool { return makeMap(reg).LessOrEq(a, b) },
		func(reg *axis.Registry, a, b []F) []F { return makeMap(reg).Join(a, b) },
		func(reg *axis.Registry, a, b []F) []F { return makeMap(reg).Widen(a, b) },
	)
}

func normalReturnSummaryLaneValid(lane normalReturnSummaryLane) bool {
	return lane.empty != nil &&
		lane.normalize != nil &&
		lane.normalizeOwned != nil &&
		lane.clone != nil &&
		lane.equal != nil &&
		lane.lessOrEq != nil &&
		lane.join != nil &&
		lane.widen != nil
}

var normalReturnSummaryLanes = BindNormalReturnFactLanes("normal-return summary", map[NormalReturnFactLaneID]normalReturnSummaryLane{
	LanePathRefinements: normalReturnSummaryFactMapLane(
		func(f *NormalReturnFacts) *[]PathValueFact { return &f.PathRefinements },
		pathValueMap, clonePathValueFacts),
	LanePersistentPathWrites: normalReturnSummaryFactMapLane(
		func(f *NormalReturnFacts) *[]PathValueFact { return &f.PersistentPathWrites },
		persistentPathWriteMap, clonePathValueFacts),
	LanePathStaticMembers: normalReturnSummaryFactMapLane(
		func(f *NormalReturnFacts) *[]PathStaticMemberFact {
			return &f.PathStaticMembers
		},
		pathStaticMemberMap, clonePathStaticMemberFacts),
	LanePathStaticMemberDeltas: normalReturnSummarySliceLaneOwned(
		func(f *NormalReturnFacts) *[]PathStaticMemberDeltaFact {
			return &f.PathStaticMemberDeltas
		},
		normalizePathStaticMemberDeltas,
		normalizePathStaticMemberDeltasOwned,
		clonePathStaticMemberDeltaFacts,
		equalPathStaticMemberDeltas,
		lessOrEqPathStaticMemberDeltas,
		joinPathStaticMemberDeltas,
		widenPathStaticMemberDeltas),
	LanePathInvalidations: normalReturnSummaryLaneNoRegOwned(
		func(f *NormalReturnFacts) *[]PathInvalidationFact {
			return &f.PathInvalidations
		},
		pathInvalidationLane.Normalize, pathInvalidationLane.NormalizeOwned, pathInvalidationLane.Clone, pathInvalidationLane.Equal, pathInvalidationLane.LessOrEq, pathInvalidationLane.Join, pathInvalidationLane.Widen),
	LaneDynamicIndexFacts: normalReturnSummaryFactMapLane(
		func(f *NormalReturnFacts) *[]DynamicIndexFact { return &f.DynamicIndexFacts },
		dynamicIndexMap, cloneDynamicIndexFacts),
	LaneKeyMemberships: normalReturnSummaryLaneNoRegOwned(
		func(f *NormalReturnFacts) *[]KeyMembershipFact { return &f.KeyMemberships },
		keyMembershipLane.Normalize, keyMembershipLane.NormalizeOwned, keyMembershipLane.Clone, keyMembershipLane.Equal, keyMembershipLane.LessOrEq, keyMembershipLane.Join, keyMembershipLane.Widen),
	LaneDynamicValueKeys: normalReturnSummaryLaneNoRegOwned(
		func(f *NormalReturnFacts) *[]DynamicValueKeyMembershipFact {
			return &f.DynamicValueKeys
		},
		dynamicValueKeyMembershipLane.Normalize, dynamicValueKeyMembershipLane.NormalizeOwned, dynamicValueKeyMembershipLane.Clone, dynamicValueKeyMembershipLane.Equal, dynamicValueKeyMembershipLane.LessOrEq, dynamicValueKeyMembershipLane.Join, dynamicValueKeyMembershipLane.Widen),
	LaneDynamicAllValues: normalReturnSummaryLaneNoRegOwned(
		func(f *NormalReturnFacts) *[]DynamicAllValueKeyMembershipFact {
			return &f.DynamicAllValues
		},
		dynamicAllValueKeyMembershipLane.Normalize, dynamicAllValueKeyMembershipLane.NormalizeOwned, dynamicAllValueKeyMembershipLane.Clone, dynamicAllValueKeyMembershipLane.Equal, dynamicAllValueKeyMembershipLane.LessOrEq, dynamicAllValueKeyMembershipLane.Join, dynamicAllValueKeyMembershipLane.Widen),
	LaneBranchProofs: normalReturnSummaryLaneNoRegOwned(
		func(f *NormalReturnFacts) *[]BranchProof { return &f.BranchProofs },
		branchProofLane.Normalize, branchProofLane.NormalizeOwned, branchProofLane.Clone, branchProofLane.Equal, branchProofLane.LessOrEq, branchProofLane.Join, branchProofLane.Join),
	LanePathPresenceImplications: normalReturnSummarySliceLaneOwned(
		func(f *NormalReturnFacts) *[]PathPresenceImplicationFact {
			return &f.PathPresenceImplications
		},
		func(reg *axis.Registry, in []PathPresenceImplicationFact) []PathPresenceImplicationFact {
			return pathPresenceImplicationLane(reg).Normalize(in)
		},
		func(reg *axis.Registry, in []PathPresenceImplicationFact) []PathPresenceImplicationFact {
			return pathPresenceImplicationLane(reg).NormalizeOwned(in)
		},
		clonePathPresenceImplications,
		func(reg *axis.Registry, a, b []PathPresenceImplicationFact) bool {
			return pathPresenceImplicationLane(reg).Equal(a, b)
		},
		func(reg *axis.Registry, a, b []PathPresenceImplicationFact) bool {
			return pathPresenceImplicationLane(reg).LessOrEq(a, b)
		},
		func(reg *axis.Registry, a, b []PathPresenceImplicationFact) []PathPresenceImplicationFact {
			return pathPresenceImplicationLane(reg).Join(a, b)
		},
		func(reg *axis.Registry, prev, next []PathPresenceImplicationFact) []PathPresenceImplicationFact {
			return pathPresenceImplicationLane(reg).Widen(prev, next)
		}),
	LaneChannelSelects: normalReturnSummaryLaneNoRegOwned(
		func(f *NormalReturnFacts) *[]ChannelSelectFact { return &f.ChannelSelects },
		channelSelectLane.Normalize, channelSelectLane.NormalizeOwned, channelSelectLane.Clone, channelSelectLane.Equal, channelSelectLane.LessOrEq, channelSelectLane.Join, channelSelectLane.Join),
	LaneFrozenTables: normalReturnSummaryLaneNoRegOwned(
		func(f *NormalReturnFacts) *[]FrozenTableFact { return &f.FrozenTables },
		frozenTableLane.Normalize, frozenTableLane.NormalizeOwned, frozenTableLane.Clone, frozenTableLane.Equal, frozenTableLane.LessOrEq, frozenTableLane.Join, frozenTableLane.Join),
	LaneEffectDeltas: normalReturnSummaryFactMapLane(
		func(f *NormalReturnFacts) *[]EffectDelta { return &f.EffectDeltas },
		effectDeltaMap, cloneEffectDeltas),
	LaneEscapeEvents: normalReturnSummaryLaneNoRegOwned(
		func(f *NormalReturnFacts) *[]EscapeEventFact { return &f.EscapeEvents },
		escapeEventLane.Normalize, escapeEventLane.NormalizeOwned, escapeEventLane.Clone, escapeEventLane.Equal, escapeEventLane.LessOrEq, escapeEventLane.Join, escapeEventLane.Widen),
	LaneStoreRelations: normalReturnSummaryLaneNoRegOwned(
		func(f *NormalReturnFacts) *[]StoreRelationFact { return &f.StoreRelations },
		storeRelationLane.Normalize, storeRelationLane.NormalizeOwned, storeRelationLane.Clone, storeRelationLane.Equal, storeRelationLane.LessOrEq, storeRelationLane.Join, storeRelationLane.Widen),
	LaneLifecycleFacts: normalReturnSummaryLaneNoRegOwned(
		func(f *NormalReturnFacts) *[]LifecycleFact { return &f.LifecycleFacts },
		lifecycleLane.Normalize, lifecycleLane.NormalizeOwned, lifecycleLane.Clone, lifecycleLane.Equal, lifecycleLane.LessOrEq, lifecycleLane.Join, lifecycleLane.Widen),
	LaneNumFloors: normalReturnSummaryLaneNoRegOwned(
		func(f *NormalReturnFacts) *[]NumFloorFact { return &f.NumFloors },
		numFloorLane.Normalize, numFloorLane.NormalizeOwned, numFloorLane.Clone, numFloorLane.Equal, numFloorLane.LessOrEq, numFloorLane.Join, numFloorLane.Widen),
	LaneNumCeils: normalReturnSummaryLaneNoRegOwned(
		func(f *NormalReturnFacts) *[]NumCeilFact { return &f.NumCeils },
		numCeilLane.Normalize, numCeilLane.NormalizeOwned, numCeilLane.Clone, numCeilLane.Equal, numCeilLane.LessOrEq, numCeilLane.Join, numCeilLane.Widen),
	LaneRelConstraints: normalReturnSummaryLaneNoRegOwned(
		func(f *NormalReturnFacts) *[]RelConstraintFact { return &f.RelConstraints },
		relConstraintLane.Normalize, relConstraintLane.NormalizeOwned, relConstraintLane.Clone, relConstraintLane.Equal, relConstraintLane.LessOrEq, relConstraintLane.Join, relConstraintLane.Widen),
}, normalReturnSummaryLaneValid)
