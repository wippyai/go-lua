package program

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

func snapshotWithMaterializedSummaryProofs(
	reg *axis.Registry,
	base summary.Snapshot,
	materialized materializedProgram,
) (summary.Snapshot, bool) {
	entries := base.EntriesOwnedNormalized()
	byKey := make(map[summary.SummaryKey]summary.Summary, len(entries)+1)
	for _, entry := range entries {
		byKey[entry.Key] = entry.Summary
	}
	changed := false
	for result, key := range materialized.resultKey {
		if overlayMaterializedSummaryProofsForResult(reg, byKey, key, result, materialized.projections) {
			changed = true
		}
	}
	if !changed {
		return base, false
	}
	nextEntries := make([]summary.EntrySummary, 0, len(byKey))
	for key, sum := range byKey {
		nextEntries = append(nextEntries, summary.EntrySummary{Key: key, Summary: sum})
	}
	return summary.NewSnapshotOwnedNormalized(reg, nextEntries...), true
}

func materializedCoreProofChangesAffectMaterialization(reg *axis.Registry, before, after summary.Snapshot) bool {
	beforeEntries := summaryEntriesByKey(before)
	for _, entry := range after.EntriesOwnedNormalized() {
		prev := beforeEntries[entry.Key]
		next := entry.Summary
		if !paramObligationsEqual(reg, prev.ParamObligations, next.ParamObligations) ||
			!paramMemberCallObligationsEqual(prev.ParamMemberCallObligations, next.ParamMemberCallObligations) ||
			!summaryLaneEqualNormalized(reg,
				summary.Summary{ReturnParamPathAliases: prev.ReturnParamPathAliases},
				summary.Summary{ReturnParamPathAliases: next.ReturnParamPathAliases},
			) ||
			!summaryLaneEqualNormalized(reg,
				summary.Summary{ReturnFlows: prev.ReturnFlows},
				summary.Summary{ReturnFlows: next.ReturnFlows},
			) ||
			!summaryLaneEqualNormalized(reg,
				summary.Summary{ParamSinkExposures: prev.ParamSinkExposures},
				summary.Summary{ParamSinkExposures: next.ParamSinkExposures},
			) ||
			!returnPresenceRelationsEqual(prev.ReturnPresenceRelations, next.ReturnPresenceRelations) ||
			!returnConditionSlotRefinementsEqual(reg, prev.ReturnConditionSlotRefinements, next.ReturnConditionSlotRefinements) {
			return true
		}
	}
	return false
}

func materializedNormalReturnFactChanges(reg *axis.Registry, before, after summary.Snapshot) bool {
	beforeEntries := summaryEntriesByKey(before)
	for _, entry := range after.EntriesOwnedNormalized() {
		prev := beforeEntries[entry.Key]
		next := entry.Summary
		if !normalReturnFactsMaterializationEqual(reg, prev.NormalReturnFacts, next.NormalReturnFacts) {
			return true
		}
	}
	return false
}

func materializedValueSlotChanges(reg *axis.Registry, before, after summary.Snapshot) bool {
	beforeEntries := summaryEntriesByKey(before)
	for _, entry := range after.EntriesOwnedNormalized() {
		prev := beforeEntries[entry.Key]
		next := entry.Summary
		if !productValueSlicesEqual(reg, prev.Returns, next.Returns) ||
			!productValueSlicesEqual(reg, prev.NormalReturnParams, next.NormalReturnParams) {
			return true
		}
	}
	return false
}

func summaryEntriesByKey(snapshot summary.Snapshot) map[summary.SummaryKey]summary.Summary {
	entries := snapshot.EntriesOwnedNormalized()
	out := make(map[summary.SummaryKey]summary.Summary, len(entries))
	for _, entry := range entries {
		out[entry.Key] = entry.Summary
	}
	return out
}

func normalReturnFactsMaterializationEqual(reg *axis.Registry, a, b callboundary.NormalReturnFacts) bool {
	return pathValueFactsEqual(reg, a.PersistentPathWrites, b.PersistentPathWrites) &&
		pathStaticMemberFactsEqual(reg, a.PathStaticMembers, b.PathStaticMembers) &&
		summaryLaneEqualNormalized(reg,
			summary.Summary{NormalReturnFacts: callboundary.NormalReturnFacts{StoreRelations: a.StoreRelations}},
			summary.Summary{NormalReturnFacts: callboundary.NormalReturnFacts{StoreRelations: b.StoreRelations}},
		)
}

func summaryLaneEqualNormalized(reg *axis.Registry, a, b summary.Summary) bool {
	return summary.EqualNormalized(reg, summary.Normalize(reg, a), summary.Normalize(reg, b))
}

func pathValueFactsEqual(reg *axis.Registry, a, b []callboundary.PathValueFact) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Path.Equal(b[i].Path) || !product.Equal(reg, a[i].Value, b[i].Value) {
			return false
		}
	}
	return true
}

func pathStaticMemberFactsEqual(reg *axis.Registry, a, b []callboundary.PathStaticMemberFact) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Path.Equal(b[i].Path) || !product.Equal(reg, a[i].Value, b[i].Value) {
			return false
		}
	}
	return true
}

func productValueSlicesEqual(reg *axis.Registry, a, b []product.Value) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !product.Equal(reg, a[i], b[i]) {
			return false
		}
	}
	return true
}

func overlayMaterializedSummaryProofsForResult(
	reg *axis.Registry,
	entries map[summary.SummaryKey]summary.Summary,
	key summary.SummaryKey,
	result *body.Result,
	projections *resultSummaryProjectionCache,
) bool {
	if reg == nil || entries == nil || result == nil {
		return false
	}
	projected, ok := projections.project(result)
	if !ok {
		return false
	}
	current := entries[key]
	next := current.Clone()
	var changed bool
	if returns, ok := overlayMaterializedValueSlots(reg, next.Returns, projected.Returns, false); ok {
		next.Returns = returns
		changed = true
	}
	if params, ok := overlayMaterializedValueSlots(reg, next.NormalReturnParams, projected.NormalReturnParams, true); ok {
		next.NormalReturnParams = params
		changed = true
	}
	if len(projected.ParamObligations) != 0 &&
		paramObligationsOverlayAllowed(reg, projected.ParamObligations) &&
		!paramObligationsEqual(reg, projected.ParamObligations, current.ParamObligations) {
		next.ParamObligations = append([]product.Value(nil), projected.ParamObligations...)
		changed = true
	}
	if paramMemberCallObligationsSubset(projected.ParamMemberCallObligations, current.ParamMemberCallObligations) &&
		!paramMemberCallObligationsEqual(projected.ParamMemberCallObligations, current.ParamMemberCallObligations) {
		next.ParamMemberCallObligations = append([]summary.ParamMemberCallObligation(nil), projected.ParamMemberCallObligations...)
		changed = true
	}
	if aliases, ok := overlayMaterializedMustSummaryLane(
		reg,
		summary.Summary{ReturnParamPathAliases: current.ReturnParamPathAliases},
		summary.Summary{ReturnParamPathAliases: projected.ReturnParamPathAliases},
	); ok {
		next.ReturnParamPathAliases = aliases.ReturnParamPathAliases
		changed = true
	}
	if flows, ok := overlayMaterializedMustSummaryLane(
		reg,
		summary.Summary{ReturnFlows: current.ReturnFlows},
		summary.Summary{ReturnFlows: projected.ReturnFlows},
	); ok {
		next.ReturnFlows = flows.ReturnFlows
		changed = true
	}
	if sinkExposures, ok := overlayMaterializedMaySummaryLane(
		reg,
		summary.Summary{ParamSinkExposures: current.ParamSinkExposures},
		summary.Summary{ParamSinkExposures: projected.ParamSinkExposures},
	); ok {
		next.ParamSinkExposures = sinkExposures.ParamSinkExposures
		changed = true
	}
	if writes, ok := overlayMaterializedPersistentPathWrites(
		reg,
		current.NormalReturnFacts.PersistentPathWrites,
		projected.NormalReturnFacts.PersistentPathWrites,
	); ok {
		next.NormalReturnFacts.PersistentPathWrites = writes
		changed = true
	}
	if members, ok := overlayMaterializedPathStaticMembers(
		reg,
		current.NormalReturnFacts.PathStaticMembers,
		projected.NormalReturnFacts.PathStaticMembers,
	); ok {
		next.NormalReturnFacts.PathStaticMembers = members
		changed = true
	}
	if storeRelations, ok := overlayMaterializedMustSummaryLane(
		reg,
		summary.Summary{NormalReturnFacts: callboundary.NormalReturnFacts{StoreRelations: current.NormalReturnFacts.StoreRelations}},
		summary.Summary{NormalReturnFacts: callboundary.NormalReturnFacts{StoreRelations: projected.NormalReturnFacts.StoreRelations}},
	); ok {
		next.NormalReturnFacts.StoreRelations = storeRelations.NormalReturnFacts.StoreRelations
		changed = true
	}
	if relations, ok := overlayMaterializedReturnPresenceRelations(reg, current.ReturnPresenceRelations, projected.ReturnPresenceRelations); ok {
		next.ReturnPresenceRelations = relations
		changed = true
	}
	if refinements, ok := overlayMaterializedReturnConditionSlotRefinements(reg, current.ReturnConditionSlotRefinements, projected.ReturnConditionSlotRefinements); ok {
		next.ReturnConditionSlotRefinements = refinements
		changed = true
	}
	if !changed {
		return false
	}
	next = summary.NormalizeOwned(reg, next)
	if summary.EqualNormalized(reg, current, next) {
		return false
	}
	entries[key] = next
	return true
}

func overlayMaterializedMustSummaryLane(reg *axis.Registry, current, projected summary.Summary) (summary.Summary, bool) {
	current = summary.Normalize(reg, current)
	projected = summary.Normalize(reg, projected)
	if summary.EqualNormalized(reg, current, projected) {
		return current, false
	}
	if !summary.LessOrEq(reg, projected, current) {
		return current, false
	}
	return projected, true
}

func overlayMaterializedMaySummaryLane(reg *axis.Registry, current, projected summary.Summary) (summary.Summary, bool) {
	current = summary.Normalize(reg, current)
	projected = summary.Normalize(reg, projected)
	if summary.EqualNormalized(reg, projected, summary.Summary{}) {
		return current, false
	}
	combined := summary.Join(reg, current, projected)
	if summary.EqualNormalized(reg, current, combined) {
		return current, false
	}
	return combined, true
}

func overlayMaterializedPersistentPathWrites(
	reg *axis.Registry,
	current []callboundary.PathValueFact,
	projected []callboundary.PathValueFact,
) ([]callboundary.PathValueFact, bool) {
	if reg == nil || len(projected) == 0 {
		return current, false
	}
	projectedSummary := summary.Summary{
		NormalReturnFacts: callboundary.NormalReturnFacts{PersistentPathWrites: projected},
	}
	currentSummary := summary.Summary{
		NormalReturnFacts: callboundary.NormalReturnFacts{PersistentPathWrites: current},
	}
	projectedSummary = summary.Normalize(reg, projectedSummary)
	currentSummary = summary.Normalize(reg, currentSummary)
	if !materializedPersistentPathWritesRefineCurrent(
		reg,
		currentSummary.NormalReturnFacts.PersistentPathWrites,
		projectedSummary.NormalReturnFacts.PersistentPathWrites,
	) {
		return current, false
	}
	if summary.EqualNormalized(reg, projectedSummary, currentSummary) {
		return current, false
	}
	return projectedSummary.NormalReturnFacts.PersistentPathWrites, true
}

func overlayMaterializedPathStaticMembers(
	reg *axis.Registry,
	current []callboundary.PathStaticMemberFact,
	projected []callboundary.PathStaticMemberFact,
) ([]callboundary.PathStaticMemberFact, bool) {
	if reg == nil || len(projected) == 0 {
		return current, false
	}
	projectedSummary := summary.Summary{
		NormalReturnFacts: callboundary.NormalReturnFacts{PathStaticMembers: projected},
	}
	currentSummary := summary.Summary{
		NormalReturnFacts: callboundary.NormalReturnFacts{PathStaticMembers: current},
	}
	projectedSummary = summary.Normalize(reg, projectedSummary)
	currentSummary = summary.Normalize(reg, currentSummary)
	if !materializedPathStaticMembersRefineCurrent(
		reg,
		currentSummary.NormalReturnFacts.PathStaticMembers,
		projectedSummary.NormalReturnFacts.PathStaticMembers,
	) {
		return current, false
	}
	if summary.EqualNormalized(reg, projectedSummary, currentSummary) {
		return current, false
	}
	return projectedSummary.NormalReturnFacts.PathStaticMembers, true
}

func overlayMaterializedReturnPresenceRelations(
	reg *axis.Registry,
	current []summary.ReturnPresenceRelation,
	projected []summary.ReturnPresenceRelation,
) ([]summary.ReturnPresenceRelation, bool) {
	if len(projected) == 0 {
		return current, false
	}
	currentSummary := summary.Normalize(reg, summary.Summary{ReturnPresenceRelations: current})
	combined := make([]summary.ReturnPresenceRelation, 0, len(current)+len(projected))
	combined = append(combined, current...)
	combined = append(combined, projected...)
	combinedSummary := summary.Normalize(reg, summary.Summary{ReturnPresenceRelations: combined})
	if summary.EqualNormalized(reg, currentSummary, combinedSummary) {
		return current, false
	}
	return combinedSummary.ReturnPresenceRelations, true
}

func overlayMaterializedReturnConditionSlotRefinements(
	reg *axis.Registry,
	current []summary.ReturnConditionSlotRefinement,
	projected []summary.ReturnConditionSlotRefinement,
) ([]summary.ReturnConditionSlotRefinement, bool) {
	if len(projected) == 0 {
		return current, false
	}
	currentSummary := summary.Normalize(reg, summary.Summary{ReturnConditionSlotRefinements: current})
	combined := make([]summary.ReturnConditionSlotRefinement, 0, len(current)+len(projected))
	combined = append(combined, current...)
	combined = append(combined, projected...)
	combinedSummary := summary.Normalize(reg, summary.Summary{ReturnConditionSlotRefinements: combined})
	if summary.EqualNormalized(reg, currentSummary, combinedSummary) {
		return current, false
	}
	return combinedSummary.ReturnConditionSlotRefinements, true
}

func materializedPersistentPathWritesRefineCurrent(
	reg *axis.Registry,
	current []callboundary.PathValueFact,
	projected []callboundary.PathValueFact,
) bool {
	if len(projected) == 0 {
		return len(current) == 0
	}
	projectedByPath := make(map[path.PathKey]product.Value, len(projected))
	for _, fact := range projected {
		if fact.Path.IsEmpty() {
			continue
		}
		projectedByPath[fact.Path.Key()] = fact.Value
	}
	for _, fact := range current {
		value, ok := projectedByPath[fact.Path.Key()]
		if !ok || !product.LessOrEq(reg, value, fact.Value) {
			return false
		}
	}
	return true
}

func materializedPathStaticMembersRefineCurrent(
	reg *axis.Registry,
	current []callboundary.PathStaticMemberFact,
	projected []callboundary.PathStaticMemberFact,
) bool {
	if len(projected) == 0 {
		return len(current) == 0
	}
	projectedByPath := make(map[path.PathKey]product.Value, len(projected))
	for _, fact := range projected {
		if fact.Path.IsEmpty() {
			continue
		}
		projectedByPath[fact.Path.Key()] = fact.Value
	}
	for _, fact := range current {
		value, ok := projectedByPath[fact.Path.Key()]
		if !ok || !product.LessOrEq(reg, value, fact.Value) {
			return false
		}
	}
	return true
}

func overlayMaterializedValueSlots(reg *axis.Registry, current, projected []product.Value, requireUseful bool) ([]product.Value, bool) {
	if reg == nil || len(projected) == 0 {
		return current, false
	}
	out := current
	changed := false
	copied := false
	for i, value := range projected {
		if product.Equal(reg, value, product.Bottom(reg)) {
			continue
		}
		if requireUseful && !summary.UsefulNormalReturnParam(reg, value) {
			continue
		}
		existing := product.Bottom(reg)
		if i < len(current) {
			existing = current[i]
		}
		if !materializedSlotRefines(reg, value, existing) {
			continue
		}
		if product.Equal(reg, existing, value) {
			continue
		}
		if i >= len(out) {
			next := make([]product.Value, i+1)
			copy(next, out)
			for j := len(out); j < len(next); j++ {
				next[j] = product.Bottom(reg)
			}
			out = next
			copied = true
		} else if !copied {
			out = append([]product.Value(nil), current...)
			copied = true
		}
		out[i] = value
		changed = true
	}
	return out, changed
}

func materializedSlotRefines(reg *axis.Registry, projected, current product.Value) bool {
	if product.Equal(reg, current, product.Bottom(reg)) || product.Equal(reg, current, product.Top()) {
		return true
	}
	if materializedSlotTrusted(reg, current) && materializedSlotUntrustedTop(reg, projected) {
		return false
	}
	return product.LessOrEq(reg, projected, current)
}

func materializedSlotTrusted(reg *axis.Registry, value product.Value) bool {
	ev := product.Get(reg, value, evidence.Key)
	return !ev.IsExplicitTop() && !ev.IsGradualTop()
}

func materializedSlotUntrustedTop(reg *axis.Registry, value product.Value) bool {
	ev := product.Get(reg, value, evidence.Key)
	return ev.IsExplicitTop() || ev.IsGradualTop()
}

func returnPresenceRelationsEqual(a, b []summary.ReturnPresenceRelation) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func returnConditionSlotRefinementsEqual(reg *axis.Registry, a, b []summary.ReturnConditionSlotRefinement) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ReturnIndex != b[i].ReturnIndex ||
			a[i].ReturnValue != b[i].ReturnValue ||
			a[i].TargetIndex != b[i].TargetIndex ||
			!product.Equal(reg, a[i].Value, b[i].Value) {
			return false
		}
	}
	return true
}

func paramMemberCallObligationsEqual(a, b []summary.ParamMemberCallObligation) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func paramMemberCallObligationsSubset(projected, current []summary.ParamMemberCallObligation) bool {
	if len(projected) > len(current) {
		return false
	}
	if len(projected) == 0 {
		return true
	}
	seen := make(map[summary.ParamMemberCallObligation]struct{}, len(current))
	for _, obligation := range current {
		seen[obligation] = struct{}{}
	}
	for _, obligation := range projected {
		if _, ok := seen[obligation]; !ok {
			return false
		}
	}
	return true
}

func paramObligationsOverlayAllowed(reg *axis.Registry, projected []product.Value) bool {
	if reg == nil {
		return false
	}
	bottom := product.Bottom(reg)
	for _, value := range projected {
		if product.Equal(reg, value, bottom) {
			return false
		}
	}
	return true
}

func paramObligationsEqual(reg *axis.Registry, a, b []product.Value) bool {
	if reg == nil {
		return len(a) == len(b)
	}
	n := max(len(a), len(b))
	top := product.Top()
	for i := range n {
		left := top
		if i < len(a) {
			left = a[i]
		}
		right := top
		if i < len(b) {
			right = b[i]
		}
		if !product.Equal(reg, left, right) {
			return false
		}
	}
	return true
}
