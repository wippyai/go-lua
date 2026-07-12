package callresult

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
)

// instantiateReturnedAllocations atomically substitutes callee allocation
// templates with caller-static-site identities before outcome lowering. The
// substitution is built once and then applied to every identity-bearing lane.
func instantiateReturnedAllocations(ctx transfer.NodeContext, got summary.Summary) summary.Summary {
	if ctx.Registry == nil || ctx.Graph == nil || len(got.FreshHeapAllocations) == 0 {
		return got
	}
	substitution := make(map[identity.ID]identity.ID, len(got.FreshHeapAllocations))
	for _, template := range got.FreshHeapAllocations {
		instantiated := identity.ReturnedAllocation(template, ctx.Graph.ID(), uint64(ctx.Point))
		if instantiated != (identity.ID{}) {
			substitution[template] = instantiated
		}
	}
	if len(substitution) == 0 {
		return got
	}
	mapValue := func(value product.Value) product.Value {
		id, ok := product.Get(ctx.Registry, value, identity.Key).ID()
		if !ok {
			return value
		}
		next, ok := substitution[id]
		if !ok {
			return value
		}
		return product.Set(ctx.Registry, value, identity.Key, identity.Singleton(next))
	}
	mapValues := func(values []product.Value) {
		for i := range values {
			values[i] = mapValue(values[i])
		}
	}

	// The provider already owns got at this point. Clone only map-backed heap
	// storage before changing keys; all slice-backed summary lanes are owned.
	mapValues(got.Returns)
	mapValues(got.ParamObligations)
	mapValues(got.NormalReturnParams)
	for i := range got.ParamSinkExposures {
		got.ParamSinkExposures[i].Contract = mapValue(got.ParamSinkExposures[i].Contract)
	}
	for i := range got.CapturedPathObligations {
		got.CapturedPathObligations[i].Value = mapValue(got.CapturedPathObligations[i].Value)
	}
	for i := range got.ReturnConditionParamRefinements {
		got.ReturnConditionParamRefinements[i].Value = mapValue(got.ReturnConditionParamRefinements[i].Value)
	}
	for i := range got.ReturnConditionSlotRefinements {
		got.ReturnConditionSlotRefinements[i].Value = mapValue(got.ReturnConditionSlotRefinements[i].Value)
	}
	for i := range got.ReturnParamLiteralCases {
		got.ReturnParamLiteralCases[i].Value = mapValue(got.ReturnParamLiteralCases[i].Value)
	}

	facts := &got.NormalReturnFacts
	for i := range facts.PathRefinements {
		facts.PathRefinements[i].Value = mapValue(facts.PathRefinements[i].Value)
	}
	for i := range facts.PersistentPathWrites {
		facts.PersistentPathWrites[i].Value = mapValue(facts.PersistentPathWrites[i].Value)
	}
	for i := range facts.PathStaticMembers {
		facts.PathStaticMembers[i].Value = mapValue(facts.PathStaticMembers[i].Value)
	}
	for i := range facts.PathStaticMemberDeltas {
		facts.PathStaticMemberDeltas[i].Value = mapValue(facts.PathStaticMemberDeltas[i].Value)
	}
	for i := range facts.DynamicIndexFacts {
		facts.DynamicIndexFacts[i].Value.KeyValue = mapValue(facts.DynamicIndexFacts[i].Value.KeyValue)
		facts.DynamicIndexFacts[i].Value.Value = mapValue(facts.DynamicIndexFacts[i].Value.Value)
	}
	for i := range facts.PathPresenceImplications {
		facts.PathPresenceImplications[i].TriggerValue = mapValue(facts.PathPresenceImplications[i].TriggerValue)
		facts.PathPresenceImplications[i].TargetValue = mapValue(facts.PathPresenceImplications[i].TargetValue)
	}

	if len(got.HeapTableObjects) != 0 {
		objects := make(map[identity.ID]heapidentity.TableObject, len(got.HeapTableObjects))
		for id, object := range got.HeapTableObjects {
			nextID := id
			if replacement, ok := substitution[id]; ok {
				nextID = replacement
			}
			objects[nextID] = object.MapValues(ctx.Registry, mapValue)
		}
		got.HeapTableObjects = objects
	}
	for i, template := range got.FreshHeapAllocations {
		if replacement, ok := substitution[template]; ok {
			got.FreshHeapAllocations[i] = replacement
		}
	}
	return got
}
