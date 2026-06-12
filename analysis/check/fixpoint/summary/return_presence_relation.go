package summary

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
)

// ReturnPresenceRelation records a must implication between two return slots.
type ReturnPresenceRelation struct {
	TriggerIndex    int
	TriggerPresence presence.Value
	TargetIndex     int
	TargetPresence  presence.Value
}

type returnPresenceRelationKey struct {
	triggerIndex    int
	triggerPresence presence.Value
	targetIndex     int
	targetPresence  presence.Value
}

func normalizeReturnPresenceRelations(in []ReturnPresenceRelation) []ReturnPresenceRelation {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[returnPresenceRelationKey]ReturnPresenceRelation, len(in))
	for _, relation := range in {
		if relation.TriggerIndex < 0 || relation.TargetIndex < 0 ||
			relation.TriggerIndex == relation.TargetIndex ||
			relation.TriggerPresence.IsBottom() || relation.TriggerPresence.IsTop() ||
			relation.TargetPresence.IsBottom() || relation.TargetPresence.IsTop() {
			continue
		}
		seen[returnPresenceRelationKeyOf(relation)] = relation
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]ReturnPresenceRelation, 0, len(seen))
	for _, relation := range seen {
		out = append(out, relation)
	}
	sort.Slice(out, func(i, j int) bool {
		left := returnPresenceRelationKeyOf(out[i])
		right := returnPresenceRelationKeyOf(out[j])
		if left.triggerIndex != right.triggerIndex {
			return left.triggerIndex < right.triggerIndex
		}
		if left.triggerPresence != right.triggerPresence {
			return left.triggerPresence < right.triggerPresence
		}
		if left.targetIndex != right.targetIndex {
			return left.targetIndex < right.targetIndex
		}
		return left.targetPresence < right.targetPresence
	})
	return out
}

func cloneReturnPresenceRelations(in []ReturnPresenceRelation) []ReturnPresenceRelation {
	if len(in) == 0 {
		return nil
	}
	out := make([]ReturnPresenceRelation, len(in))
	copy(out, in)
	return out
}

func returnPresenceRelationsEqual(a, b []ReturnPresenceRelation) bool {
	a = normalizeReturnPresenceRelations(a)
	b = normalizeReturnPresenceRelations(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if returnPresenceRelationKeyOf(a[i]) != returnPresenceRelationKeyOf(b[i]) {
			return false
		}
	}
	return true
}

func returnPresenceRelationsLessOrEq(a, b []ReturnPresenceRelation) bool {
	a = normalizeReturnPresenceRelations(a)
	b = normalizeReturnPresenceRelations(b)
	aByKey := make(map[returnPresenceRelationKey]struct{}, len(a))
	for _, relation := range a {
		aByKey[returnPresenceRelationKeyOf(relation)] = struct{}{}
	}
	for _, relation := range b {
		if _, ok := aByKey[returnPresenceRelationKeyOf(relation)]; !ok {
			return false
		}
	}
	return true
}

func joinReturnPresenceRelations(a, b []ReturnPresenceRelation) []ReturnPresenceRelation {
	a = normalizeReturnPresenceRelations(a)
	b = normalizeReturnPresenceRelations(b)
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	bByKey := make(map[returnPresenceRelationKey]ReturnPresenceRelation, len(b))
	for _, relation := range b {
		bByKey[returnPresenceRelationKeyOf(relation)] = relation
	}
	out := make([]ReturnPresenceRelation, 0, min(len(a), len(b)))
	for _, relation := range a {
		if _, ok := bByKey[returnPresenceRelationKeyOf(relation)]; ok {
			out = append(out, relation)
		}
	}
	return normalizeReturnPresenceRelations(out)
}

func returnPresenceRelationKeyOf(relation ReturnPresenceRelation) returnPresenceRelationKey {
	return returnPresenceRelationKey{
		triggerIndex:    relation.TriggerIndex,
		triggerPresence: relation.TriggerPresence,
		targetIndex:     relation.TargetIndex,
		targetPresence:  relation.TargetPresence,
	}
}
