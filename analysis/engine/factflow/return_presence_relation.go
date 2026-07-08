package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
)

// ReturnPresenceRelation describes one return-slot presence implication at a
// return point. When TriggerIndex has TriggerPresence, TargetIndex may be
// refined to TargetPresence by callers that branch on the trigger slot.
type ReturnPresenceRelation struct {
	triggerIndex    int
	triggerPresence presence.Value
	targetIndex     int
	targetPresence  presence.Value
}

// ReturnPresenceRelationSet groups return-presence relations emitted at the
// same return point.
type ReturnPresenceRelationSet struct {
	relations []ReturnPresenceRelation
}

// NewReturnPresenceRelation creates a return-slot presence implication.
func NewReturnPresenceRelation(
	triggerIndex int,
	triggerPresence presence.Value,
	targetIndex int,
	targetPresence presence.Value,
) ReturnPresenceRelation {
	return ReturnPresenceRelation{
		triggerIndex:    triggerIndex,
		triggerPresence: triggerPresence,
		targetIndex:     targetIndex,
		targetPresence:  targetPresence,
	}
}

// NewReturnPresenceRelationSet creates a relation set.
func NewReturnPresenceRelationSet(relations ...ReturnPresenceRelation) ReturnPresenceRelationSet {
	return ReturnPresenceRelationSet{relations: copyReturnPresenceRelationSlice(relations)}
}

func (r ReturnPresenceRelation) TriggerIndex() int { return r.triggerIndex }

func (r ReturnPresenceRelation) TriggerPresence() presence.Value { return r.triggerPresence }

func (r ReturnPresenceRelation) TargetIndex() int { return r.targetIndex }

func (r ReturnPresenceRelation) TargetPresence() presence.Value { return r.targetPresence }

func (r ReturnPresenceRelation) copy() ReturnPresenceRelation { return r }

func (s ReturnPresenceRelationSet) Relations() []ReturnPresenceRelation {
	return copyReturnPresenceRelationSlice(s.relations)
}

func (s ReturnPresenceRelationSet) copy() ReturnPresenceRelationSet {
	return ReturnPresenceRelationSet{relations: copyReturnPresenceRelationSlice(s.relations)}
}

func copyReturnPresenceRelationSlice(in []ReturnPresenceRelation) []ReturnPresenceRelation {
	if len(in) == 0 {
		return nil
	}
	out := make([]ReturnPresenceRelation, len(in))
	for i, relation := range in {
		out[i] = relation.copy()
	}
	return out
}
