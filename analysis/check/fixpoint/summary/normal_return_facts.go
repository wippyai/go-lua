package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

func normalizeNormalReturnFacts(reg *axis.Registry, in callboundary.NormalReturnFacts) callboundary.NormalReturnFacts {
	out := callboundary.NormalReturnFacts{
		PathRefinements:   normalizePathValueFacts(reg, in.PathRefinements),
		PathStaticMembers: normalizePathStaticMemberFacts(reg, in.PathStaticMembers),
		PathInvalidations: pathInvalidationLane.Normalize(in.PathInvalidations),
		DynamicIndexFacts: normalizeDynamicIndexFacts(reg, in.DynamicIndexFacts),
		BranchProofs:      branchProofLane.Normalize(in.BranchProofs),
		ChannelSelects:    channelSelectLane.Normalize(in.ChannelSelects),
		FrozenTables:      frozenTableLane.Normalize(in.FrozenTables),
		EffectDeltas:      normalizeEffectDeltas(reg, in.EffectDeltas),
		EscapeEvents:      escapeEventLane.Normalize(in.EscapeEvents),
		StoreRelations:    storeRelationLane.Normalize(in.StoreRelations),
		LifecycleFacts:    lifecycleLane.Normalize(in.LifecycleFacts),
		NumFloors:         normalizeNumFloorFacts(in.NumFloors),
	}
	if normalReturnFactsEmpty(out) {
		return callboundary.NormalReturnFacts{}
	}
	return out
}

func cloneNormalReturnFacts(in callboundary.NormalReturnFacts) callboundary.NormalReturnFacts {
	if normalReturnFactsEmpty(in) {
		return callboundary.NormalReturnFacts{}
	}
	return callboundary.NormalReturnFacts{
		PathRefinements:   clonePathValueFacts(in.PathRefinements),
		PathStaticMembers: clonePathStaticMemberFacts(in.PathStaticMembers),
		PathInvalidations: pathInvalidationLane.Clone(in.PathInvalidations),
		DynamicIndexFacts: cloneDynamicIndexFacts(in.DynamicIndexFacts),
		BranchProofs:      branchProofLane.Clone(in.BranchProofs),
		ChannelSelects:    channelSelectLane.Clone(in.ChannelSelects),
		FrozenTables:      frozenTableLane.Clone(in.FrozenTables),
		EffectDeltas:      cloneEffectDeltas(in.EffectDeltas),
		EscapeEvents:      escapeEventLane.Clone(in.EscapeEvents),
		StoreRelations:    storeRelationLane.Clone(in.StoreRelations),
		LifecycleFacts:    lifecycleLane.Clone(in.LifecycleFacts),
		NumFloors:         cloneNumFloorFacts(in.NumFloors),
	}
}

// CloneNormalReturnFacts returns a defensive copy of normal-return fact lanes.
func CloneNormalReturnFacts(in callboundary.NormalReturnFacts) callboundary.NormalReturnFacts {
	return cloneNormalReturnFacts(in)
}

func normalReturnFactsEqual(reg *axis.Registry, a, b callboundary.NormalReturnFacts) bool {
	a = normalizeNormalReturnFacts(reg, a)
	b = normalizeNormalReturnFacts(reg, b)
	return pathValueFactsEqual(reg, a.PathRefinements, b.PathRefinements) &&
		pathStaticMemberFactsEqual(reg, a.PathStaticMembers, b.PathStaticMembers) &&
		pathInvalidationLane.Equal(a.PathInvalidations, b.PathInvalidations) &&
		dynamicIndexFactsEqual(reg, a.DynamicIndexFacts, b.DynamicIndexFacts) &&
		branchProofLane.Equal(a.BranchProofs, b.BranchProofs) &&
		channelSelectLane.Equal(a.ChannelSelects, b.ChannelSelects) &&
		frozenTableLane.Equal(a.FrozenTables, b.FrozenTables) &&
		effectDeltasEqual(reg, a.EffectDeltas, b.EffectDeltas) &&
		escapeEventLane.Equal(a.EscapeEvents, b.EscapeEvents) &&
		storeRelationLane.Equal(a.StoreRelations, b.StoreRelations) &&
		lifecycleLane.Equal(a.LifecycleFacts, b.LifecycleFacts) &&
		numFloorFactsEqual(a.NumFloors, b.NumFloors)
}

func normalReturnFactsLessOrEq(reg *axis.Registry, a, b callboundary.NormalReturnFacts) bool {
	a = normalizeNormalReturnFacts(reg, a)
	b = normalizeNormalReturnFacts(reg, b)
	return pathValueFactsLessOrEq(reg, a.PathRefinements, b.PathRefinements) &&
		pathStaticMemberFactsLessOrEq(reg, a.PathStaticMembers, b.PathStaticMembers) &&
		pathInvalidationLane.LessOrEq(a.PathInvalidations, b.PathInvalidations) &&
		dynamicIndexFactsLessOrEq(reg, a.DynamicIndexFacts, b.DynamicIndexFacts) &&
		branchProofLane.LessOrEq(a.BranchProofs, b.BranchProofs) &&
		channelSelectLane.LessOrEq(a.ChannelSelects, b.ChannelSelects) &&
		frozenTableLane.LessOrEq(a.FrozenTables, b.FrozenTables) &&
		effectDeltasLessOrEq(reg, a.EffectDeltas, b.EffectDeltas) &&
		escapeEventLane.LessOrEq(a.EscapeEvents, b.EscapeEvents) &&
		storeRelationLane.LessOrEq(a.StoreRelations, b.StoreRelations) &&
		lifecycleLane.LessOrEq(a.LifecycleFacts, b.LifecycleFacts) &&
		numFloorFactsLessOrEq(a.NumFloors, b.NumFloors)
}

func joinNormalReturnFacts(reg *axis.Registry, a, b callboundary.NormalReturnFacts) callboundary.NormalReturnFacts {
	return normalizeNormalReturnFacts(reg, callboundary.NormalReturnFacts{
		PathRefinements:   joinPathValueFacts(reg, a.PathRefinements, b.PathRefinements),
		PathStaticMembers: joinPathStaticMemberFacts(reg, a.PathStaticMembers, b.PathStaticMembers),
		PathInvalidations: pathInvalidationLane.Join(a.PathInvalidations, b.PathInvalidations),
		DynamicIndexFacts: joinDynamicIndexFacts(reg, a.DynamicIndexFacts, b.DynamicIndexFacts),
		BranchProofs:      branchProofLane.Join(a.BranchProofs, b.BranchProofs),
		ChannelSelects:    channelSelectLane.Join(a.ChannelSelects, b.ChannelSelects),
		FrozenTables:      frozenTableLane.Join(a.FrozenTables, b.FrozenTables),
		EffectDeltas:      joinEffectDeltas(reg, a.EffectDeltas, b.EffectDeltas),
		EscapeEvents:      escapeEventLane.Join(a.EscapeEvents, b.EscapeEvents),
		StoreRelations:    storeRelationLane.Join(a.StoreRelations, b.StoreRelations),
		LifecycleFacts:    lifecycleLane.Join(a.LifecycleFacts, b.LifecycleFacts),
		NumFloors:         joinNumFloorFacts(a.NumFloors, b.NumFloors),
	})
}

func widenNormalReturnFacts(reg *axis.Registry, prev, next callboundary.NormalReturnFacts) callboundary.NormalReturnFacts {
	return normalizeNormalReturnFacts(reg, callboundary.NormalReturnFacts{
		PathRefinements:   widenPathValueFacts(reg, prev.PathRefinements, next.PathRefinements),
		PathStaticMembers: widenPathStaticMemberFacts(reg, prev.PathStaticMembers, next.PathStaticMembers),
		PathInvalidations: pathInvalidationLane.Widen(prev.PathInvalidations, next.PathInvalidations),
		DynamicIndexFacts: widenDynamicIndexFacts(reg, prev.DynamicIndexFacts, next.DynamicIndexFacts),
		BranchProofs:      branchProofLane.Join(prev.BranchProofs, next.BranchProofs),
		ChannelSelects:    channelSelectLane.Join(prev.ChannelSelects, next.ChannelSelects),
		FrozenTables:      frozenTableLane.Join(prev.FrozenTables, next.FrozenTables),
		EffectDeltas:      widenEffectDeltas(reg, prev.EffectDeltas, next.EffectDeltas),
		EscapeEvents:      escapeEventLane.Widen(prev.EscapeEvents, next.EscapeEvents),
		StoreRelations:    storeRelationLane.Widen(prev.StoreRelations, next.StoreRelations),
		LifecycleFacts:    lifecycleLane.Widen(prev.LifecycleFacts, next.LifecycleFacts),
		NumFloors:         joinNumFloorFacts(prev.NumFloors, next.NumFloors),
	})
}

func normalReturnFactsEmpty(facts callboundary.NormalReturnFacts) bool {
	return facts.Empty()
}
