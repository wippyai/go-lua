package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// BranchPresenceRelation describes one branch-triggered presence implication:
// when triggerPath is refined to triggerPresence on an edge, targetPath may be
// refined to targetPresence on the same edge.
type BranchPresenceRelation struct {
	triggerPath     path.Path
	triggerPresence presence.Value
	targetPath      path.Path
	targetPresence  presence.Value
}

// BranchPresenceRelationSet groups branch-triggered presence relations emitted
// at the same CFG branch point.
type BranchPresenceRelationSet struct {
	relations []BranchPresenceRelation
}

// NewBranchPresenceRelation creates a branch presence implication.
func NewBranchPresenceRelation(
	triggerPath path.Path,
	triggerPresence presence.Value,
	targetPath path.Path,
	targetPresence presence.Value,
) BranchPresenceRelation {
	return BranchPresenceRelation{
		triggerPath:     triggerPath.Clone(),
		triggerPresence: triggerPresence,
		targetPath:      targetPath.Clone(),
		targetPresence:  targetPresence,
	}
}

// NewBranchPresenceRelationSet creates a relation set.
func NewBranchPresenceRelationSet(relations ...BranchPresenceRelation) BranchPresenceRelationSet {
	return BranchPresenceRelationSet{relations: copyBranchPresenceRelationSlice(relations)}
}

// TriggerPath returns the branch-refined path that activates the implication.
func (r BranchPresenceRelation) TriggerPath() path.Path { return r.triggerPath.Clone() }

// TriggerPresence returns the triggering presence state.
func (r BranchPresenceRelation) TriggerPresence() presence.Value { return r.triggerPresence }

// TargetPath returns the path refined when the implication activates.
func (r BranchPresenceRelation) TargetPath() path.Path { return r.targetPath.Clone() }

// TargetPresence returns the target presence state.
func (r BranchPresenceRelation) TargetPresence() presence.Value { return r.targetPresence }

func (r BranchPresenceRelation) copy() BranchPresenceRelation {
	r.triggerPath = r.triggerPath.Clone()
	r.targetPath = r.targetPath.Clone()
	return r
}

// Relations returns the branch presence relations in deterministic order.
func (s BranchPresenceRelationSet) Relations() []BranchPresenceRelation {
	return copyBranchPresenceRelationSlice(s.relations)
}

func (s BranchPresenceRelationSet) copy() BranchPresenceRelationSet {
	return BranchPresenceRelationSet{relations: copyBranchPresenceRelationSlice(s.relations)}
}

func copyBranchPresenceRelationMap(in map[cfg.Point]BranchPresenceRelationSet) map[cfg.Point]BranchPresenceRelationSet {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]BranchPresenceRelationSet, len(in))
	for point, set := range in {
		out[point] = set.copy()
	}
	return out
}

func mergeBranchPresenceRelationMap(
	base map[cfg.Point]BranchPresenceRelationSet,
	added map[cfg.Point]BranchPresenceRelationSet,
) map[cfg.Point]BranchPresenceRelationSet {
	if len(base) == 0 {
		return copyBranchPresenceRelationMap(added)
	}
	out := copyBranchPresenceRelationMap(base)
	for point, set := range added {
		relations := out[point].Relations()
		relations = append(relations, set.Relations()...)
		out[point] = NewBranchPresenceRelationSet(relations...)
	}
	return out
}

func copyBranchPresenceRelationSlice(in []BranchPresenceRelation) []BranchPresenceRelation {
	if len(in) == 0 {
		return nil
	}
	out := make([]BranchPresenceRelation, len(in))
	for i, fact := range in {
		out[i] = fact.copy()
	}
	return out
}
