package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice/factset"
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

// returnPresenceRelationLane is the canonical must (intersection) lattice for
// return-slot presence implications: a relation holds at a join only when it
// holds on every path.
var returnPresenceRelationLane = factset.Set[returnPresenceRelationKey, ReturnPresenceRelation]{
	Key: returnPresenceRelationKeyOf,
	EqualFact: func(a, b ReturnPresenceRelation) bool {
		return returnPresenceRelationKeyOf(a) == returnPresenceRelationKeyOf(b)
	},
	Less: returnPresenceRelationLess,
	Valid: func(r ReturnPresenceRelation) bool {
		return r.TriggerIndex >= 0 && r.TargetIndex >= 0 && r.TriggerIndex != r.TargetIndex &&
			!r.TriggerPresence.IsBottom() && !r.TriggerPresence.IsTop() &&
			!r.TargetPresence.IsBottom() && !r.TargetPresence.IsTop()
	},
	Prefer:    func(kept, incoming ReturnPresenceRelation) bool { return true },
	Intersect: true,
}

func returnPresenceRelationKeyOf(relation ReturnPresenceRelation) returnPresenceRelationKey {
	return returnPresenceRelationKey{
		triggerIndex:    relation.TriggerIndex,
		triggerPresence: relation.TriggerPresence,
		targetIndex:     relation.TargetIndex,
		targetPresence:  relation.TargetPresence,
	}
}

func returnPresenceRelationLess(a, b ReturnPresenceRelation) bool {
	left := returnPresenceRelationKeyOf(a)
	right := returnPresenceRelationKeyOf(b)
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
}
