package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

func normalizeNormalReturnFacts(reg *axis.Registry, in callboundary.NormalReturnFacts) callboundary.NormalReturnFacts {
	out := callboundary.NormalReturnFacts{
		PathRefinements:   normalizePathValueFacts(reg, in.PathRefinements),
		PathStaticMembers: normalizePathStaticMemberFacts(reg, in.PathStaticMembers),
		PathInvalidations: normalizePathInvalidationFacts(in.PathInvalidations),
		DynamicIndexFacts: normalizeDynamicIndexFacts(reg, in.DynamicIndexFacts),
		BranchProofs:      normalizeBranchProofs(in.BranchProofs),
		ChannelSelects:    normalizeChannelSelectFacts(in.ChannelSelects),
		FrozenTables:      normalizeFrozenTableFacts(in.FrozenTables),
		EffectDeltas:      normalizeEffectDeltas(reg, in.EffectDeltas),
		EscapeEvents:      normalizeEscapeEventFacts(in.EscapeEvents),
		StoreRelations:    normalizeStoreRelationFacts(in.StoreRelations),
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
		PathInvalidations: clonePathInvalidationFacts(in.PathInvalidations),
		DynamicIndexFacts: cloneDynamicIndexFacts(in.DynamicIndexFacts),
		BranchProofs:      cloneBranchProofs(in.BranchProofs),
		ChannelSelects:    cloneChannelSelectFacts(in.ChannelSelects),
		FrozenTables:      cloneFrozenTableFacts(in.FrozenTables),
		EffectDeltas:      cloneEffectDeltas(in.EffectDeltas),
		EscapeEvents:      cloneEscapeEventFacts(in.EscapeEvents),
		StoreRelations:    cloneStoreRelationFacts(in.StoreRelations),
	}
}

func normalReturnFactsEqual(reg *axis.Registry, a, b callboundary.NormalReturnFacts) bool {
	a = normalizeNormalReturnFacts(reg, a)
	b = normalizeNormalReturnFacts(reg, b)
	return pathValueFactsEqual(reg, a.PathRefinements, b.PathRefinements) &&
		pathStaticMemberFactsEqual(reg, a.PathStaticMembers, b.PathStaticMembers) &&
		pathInvalidationFactsEqual(a.PathInvalidations, b.PathInvalidations) &&
		dynamicIndexFactsEqual(reg, a.DynamicIndexFacts, b.DynamicIndexFacts) &&
		branchProofsEqual(a.BranchProofs, b.BranchProofs) &&
		channelSelectFactsEqual(a.ChannelSelects, b.ChannelSelects) &&
		frozenTableFactsEqual(a.FrozenTables, b.FrozenTables) &&
		effectDeltasEqual(reg, a.EffectDeltas, b.EffectDeltas) &&
		escapeEventFactsEqual(a.EscapeEvents, b.EscapeEvents) &&
		storeRelationFactsEqual(a.StoreRelations, b.StoreRelations)
}

func normalReturnFactsLessOrEq(reg *axis.Registry, a, b callboundary.NormalReturnFacts) bool {
	a = normalizeNormalReturnFacts(reg, a)
	b = normalizeNormalReturnFacts(reg, b)
	return pathValueFactsLessOrEq(reg, a.PathRefinements, b.PathRefinements) &&
		pathStaticMemberFactsLessOrEq(reg, a.PathStaticMembers, b.PathStaticMembers) &&
		pathInvalidationFactsLessOrEq(a.PathInvalidations, b.PathInvalidations) &&
		dynamicIndexFactsLessOrEq(reg, a.DynamicIndexFacts, b.DynamicIndexFacts) &&
		branchProofsLessOrEq(a.BranchProofs, b.BranchProofs) &&
		channelSelectFactsLessOrEq(a.ChannelSelects, b.ChannelSelects) &&
		frozenTableFactsLessOrEq(a.FrozenTables, b.FrozenTables) &&
		effectDeltasLessOrEq(reg, a.EffectDeltas, b.EffectDeltas) &&
		escapeEventFactsLessOrEq(a.EscapeEvents, b.EscapeEvents) &&
		storeRelationFactsLessOrEq(a.StoreRelations, b.StoreRelations)
}

func joinNormalReturnFacts(reg *axis.Registry, a, b callboundary.NormalReturnFacts) callboundary.NormalReturnFacts {
	return normalizeNormalReturnFacts(reg, callboundary.NormalReturnFacts{
		PathRefinements:   joinPathValueFacts(reg, a.PathRefinements, b.PathRefinements),
		PathStaticMembers: joinPathStaticMemberFacts(reg, a.PathStaticMembers, b.PathStaticMembers),
		PathInvalidations: joinPathInvalidationFacts(a.PathInvalidations, b.PathInvalidations),
		DynamicIndexFacts: joinDynamicIndexFacts(reg, a.DynamicIndexFacts, b.DynamicIndexFacts),
		BranchProofs:      joinBranchProofs(a.BranchProofs, b.BranchProofs),
		ChannelSelects:    joinChannelSelectFacts(a.ChannelSelects, b.ChannelSelects),
		FrozenTables:      joinFrozenTableFacts(a.FrozenTables, b.FrozenTables),
		EffectDeltas:      joinEffectDeltas(reg, a.EffectDeltas, b.EffectDeltas),
		EscapeEvents:      joinEscapeEventFacts(a.EscapeEvents, b.EscapeEvents),
		StoreRelations:    joinStoreRelationFacts(a.StoreRelations, b.StoreRelations),
	})
}

func widenNormalReturnFacts(reg *axis.Registry, prev, next callboundary.NormalReturnFacts) callboundary.NormalReturnFacts {
	return normalizeNormalReturnFacts(reg, callboundary.NormalReturnFacts{
		PathRefinements:   widenPathValueFacts(reg, prev.PathRefinements, next.PathRefinements),
		PathStaticMembers: widenPathStaticMemberFacts(reg, prev.PathStaticMembers, next.PathStaticMembers),
		PathInvalidations: widenPathInvalidationFacts(prev.PathInvalidations, next.PathInvalidations),
		DynamicIndexFacts: widenDynamicIndexFacts(reg, prev.DynamicIndexFacts, next.DynamicIndexFacts),
		BranchProofs:      joinBranchProofs(prev.BranchProofs, next.BranchProofs),
		ChannelSelects:    joinChannelSelectFacts(prev.ChannelSelects, next.ChannelSelects),
		FrozenTables:      joinFrozenTableFacts(prev.FrozenTables, next.FrozenTables),
		EffectDeltas:      widenEffectDeltas(reg, prev.EffectDeltas, next.EffectDeltas),
		EscapeEvents:      widenEscapeEventFacts(prev.EscapeEvents, next.EscapeEvents),
		StoreRelations:    widenStoreRelationFacts(prev.StoreRelations, next.StoreRelations),
	})
}

func normalReturnFactsEmpty(facts callboundary.NormalReturnFacts) bool {
	return len(facts.PathRefinements) == 0 &&
		len(facts.PathStaticMembers) == 0 &&
		len(facts.PathInvalidations) == 0 &&
		len(facts.DynamicIndexFacts) == 0 &&
		len(facts.BranchProofs) == 0 &&
		len(facts.ChannelSelects) == 0 &&
		len(facts.FrozenTables) == 0 &&
		len(facts.EffectDeltas) == 0 &&
		len(facts.EscapeEvents) == 0 &&
		len(facts.StoreRelations) == 0
}
