package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

func normalizeNormalReturnFacts(reg *axis.Registry, in callboundary.NormalReturnFacts) callboundary.NormalReturnFacts {
	return normalizeNormalReturnFactsWith(reg, in, false)
}

func normalizeOwnedNormalReturnFacts(reg *axis.Registry, in callboundary.NormalReturnFacts) callboundary.NormalReturnFacts {
	return normalizeNormalReturnFactsWith(reg, in, true)
}

func normalizeNormalReturnFactsWith(reg *axis.Registry, in callboundary.NormalReturnFacts, owned bool) callboundary.NormalReturnFacts {
	if in.Empty() {
		return callboundary.NormalReturnFacts{}
	}
	var out callboundary.NormalReturnFacts
	if len(in.PathRefinements) != 0 {
		if owned {
			out.PathRefinements = normalizePathValueFactsOwned(reg, in.PathRefinements)
		} else {
			out.PathRefinements = normalizePathValueFacts(reg, in.PathRefinements)
		}
	}
	if len(in.PersistentPathWrites) != 0 {
		if owned {
			out.PersistentPathWrites = normalizePersistentPathWritesOwned(reg, in.PersistentPathWrites)
		} else {
			out.PersistentPathWrites = normalizePersistentPathWrites(reg, in.PersistentPathWrites)
		}
	}
	if len(in.PathStaticMembers) != 0 {
		if owned {
			out.PathStaticMembers = normalizePathStaticMemberFactsOwned(reg, in.PathStaticMembers)
		} else {
			out.PathStaticMembers = normalizePathStaticMemberFacts(reg, in.PathStaticMembers)
		}
	}
	if len(in.PathInvalidations) != 0 {
		if owned {
			out.PathInvalidations = pathInvalidationLane.NormalizeOwned(in.PathInvalidations)
		} else {
			out.PathInvalidations = pathInvalidationLane.Normalize(in.PathInvalidations)
		}
	}
	if len(in.DynamicIndexFacts) != 0 {
		if owned {
			out.DynamicIndexFacts = normalizeDynamicIndexFactsOwned(reg, in.DynamicIndexFacts)
		} else {
			out.DynamicIndexFacts = normalizeDynamicIndexFacts(reg, in.DynamicIndexFacts)
		}
	}
	if len(in.KeyMemberships) != 0 {
		if owned {
			out.KeyMemberships = keyMembershipLane.NormalizeOwned(in.KeyMemberships)
		} else {
			out.KeyMemberships = keyMembershipLane.Normalize(in.KeyMemberships)
		}
	}
	if len(in.DynamicValueKeys) != 0 {
		if owned {
			out.DynamicValueKeys = dynamicValueKeyMembershipLane.NormalizeOwned(in.DynamicValueKeys)
		} else {
			out.DynamicValueKeys = dynamicValueKeyMembershipLane.Normalize(in.DynamicValueKeys)
		}
	}
	if len(in.DynamicAllValues) != 0 {
		if owned {
			out.DynamicAllValues = dynamicAllValueKeyMembershipLane.NormalizeOwned(in.DynamicAllValues)
		} else {
			out.DynamicAllValues = dynamicAllValueKeyMembershipLane.Normalize(in.DynamicAllValues)
		}
	}
	if len(in.BranchProofs) != 0 {
		if owned {
			out.BranchProofs = branchProofLane.NormalizeOwned(in.BranchProofs)
		} else {
			out.BranchProofs = branchProofLane.Normalize(in.BranchProofs)
		}
	}
	if len(in.ChannelSelects) != 0 {
		if owned {
			out.ChannelSelects = channelSelectLane.NormalizeOwned(in.ChannelSelects)
		} else {
			out.ChannelSelects = channelSelectLane.Normalize(in.ChannelSelects)
		}
	}
	if len(in.FrozenTables) != 0 {
		if owned {
			out.FrozenTables = frozenTableLane.NormalizeOwned(in.FrozenTables)
		} else {
			out.FrozenTables = frozenTableLane.Normalize(in.FrozenTables)
		}
	}
	if len(in.EffectDeltas) != 0 {
		if owned {
			out.EffectDeltas = normalizeEffectDeltasOwned(reg, in.EffectDeltas)
		} else {
			out.EffectDeltas = normalizeEffectDeltas(reg, in.EffectDeltas)
		}
	}
	if len(in.EscapeEvents) != 0 {
		if owned {
			out.EscapeEvents = escapeEventLane.NormalizeOwned(in.EscapeEvents)
		} else {
			out.EscapeEvents = escapeEventLane.Normalize(in.EscapeEvents)
		}
	}
	if len(in.StoreRelations) != 0 {
		if owned {
			out.StoreRelations = storeRelationLane.NormalizeOwned(in.StoreRelations)
		} else {
			out.StoreRelations = storeRelationLane.Normalize(in.StoreRelations)
		}
	}
	if len(in.LifecycleFacts) != 0 {
		if owned {
			out.LifecycleFacts = lifecycleLane.NormalizeOwned(in.LifecycleFacts)
		} else {
			out.LifecycleFacts = lifecycleLane.Normalize(in.LifecycleFacts)
		}
	}
	if len(in.NumFloors) != 0 {
		if owned {
			out.NumFloors = numFloorLane.NormalizeOwned(in.NumFloors)
		} else {
			out.NumFloors = numFloorLane.Normalize(in.NumFloors)
		}
	}
	if len(in.RelConstraints) != 0 {
		if owned {
			out.RelConstraints = relConstraintLane.NormalizeOwned(in.RelConstraints)
		} else {
			out.RelConstraints = relConstraintLane.Normalize(in.RelConstraints)
		}
	}
	if out.Empty() {
		return callboundary.NormalReturnFacts{}
	}
	return out
}

func cloneNormalReturnFacts(in callboundary.NormalReturnFacts) callboundary.NormalReturnFacts {
	if in.Empty() {
		return callboundary.NormalReturnFacts{}
	}
	out := callboundary.NormalReturnFacts{
		PathRefinements:      clonePathValueFacts(in.PathRefinements),
		PersistentPathWrites: clonePathValueFacts(in.PersistentPathWrites),
		PathStaticMembers:    clonePathStaticMemberFacts(in.PathStaticMembers),
		PathInvalidations:    pathInvalidationLane.Clone(in.PathInvalidations),
		DynamicIndexFacts:    cloneDynamicIndexFacts(in.DynamicIndexFacts),
		KeyMemberships:       keyMembershipLane.Clone(in.KeyMemberships),
		DynamicValueKeys:     dynamicValueKeyMembershipLane.Clone(in.DynamicValueKeys),
		DynamicAllValues:     dynamicAllValueKeyMembershipLane.Clone(in.DynamicAllValues),
		BranchProofs:         branchProofLane.Clone(in.BranchProofs),
		ChannelSelects:       channelSelectLane.Clone(in.ChannelSelects),
		FrozenTables:         frozenTableLane.Clone(in.FrozenTables),
		EffectDeltas:         cloneEffectDeltas(in.EffectDeltas),
		EscapeEvents:         escapeEventLane.Clone(in.EscapeEvents),
		StoreRelations:       storeRelationLane.Clone(in.StoreRelations),
		LifecycleFacts:       lifecycleLane.Clone(in.LifecycleFacts),
		NumFloors:            numFloorLane.Clone(in.NumFloors),
		RelConstraints:       relConstraintLane.Clone(in.RelConstraints),
	}
	return out
}

// CloneNormalReturnFacts returns a defensive copy of normal-return fact lanes.
func CloneNormalReturnFacts(in callboundary.NormalReturnFacts) callboundary.NormalReturnFacts {
	return cloneNormalReturnFacts(in)
}

func normalReturnFactsEqual(reg *axis.Registry, a, b callboundary.NormalReturnFacts) bool {
	a = normalizeNormalReturnFacts(reg, a)
	b = normalizeNormalReturnFacts(reg, b)
	return normalReturnFactsEqualNormalized(reg, a, b)
}

func normalReturnFactsEqualNormalized(reg *axis.Registry, a, b callboundary.NormalReturnFacts) bool {
	for _, lane := range normalReturnSummaryLanes {
		if lane.empty(&a) && lane.empty(&b) {
			continue
		}
		if !lane.equal(reg, &a, &b) {
			return false
		}
	}
	return true
}

func normalReturnFactsLessOrEq(reg *axis.Registry, a, b callboundary.NormalReturnFacts) bool {
	a = normalizeNormalReturnFacts(reg, a)
	b = normalizeNormalReturnFacts(reg, b)
	for _, lane := range normalReturnSummaryLanes {
		if lane.empty(&a) && lane.empty(&b) {
			continue
		}
		if !lane.lessOrEq(reg, &a, &b) {
			return false
		}
	}
	return true
}

func joinNormalReturnFacts(reg *axis.Registry, a, b callboundary.NormalReturnFacts) callboundary.NormalReturnFacts {
	var out callboundary.NormalReturnFacts
	for _, lane := range normalReturnSummaryLanes {
		if lane.empty(&a) && lane.empty(&b) {
			continue
		}
		lane.join(reg, &a, &b, &out)
	}
	return normalizeNormalReturnFacts(reg, out)
}

func widenNormalReturnFacts(reg *axis.Registry, prev, next callboundary.NormalReturnFacts) callboundary.NormalReturnFacts {
	var out callboundary.NormalReturnFacts
	for _, lane := range normalReturnSummaryLanes {
		if lane.empty(&prev) && lane.empty(&next) {
			continue
		}
		lane.widen(reg, &prev, &next, &out)
	}
	return normalizeNormalReturnFacts(reg, out)
}
