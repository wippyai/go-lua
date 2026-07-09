package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// PathValuePresenceImplication publishes a persistent implication at a CFG
// point: when triggerPath is proven to satisfy triggerValue, targetPath has
// targetPresence or targetValue.
type PathValuePresenceImplication struct {
	triggerPath         path.Path
	triggerOtherPath    path.Path
	triggerValue        product.Value
	triggerPresence     presence.Value
	hasTriggerPresence  bool
	hasTriggerPathEqual bool
	targetPath          path.Path
	targetPresence      presence.Value
	targetValue         product.Value
	hasTargetValue      bool
}

// PathValuePresenceImplicationSet groups point-local implication publishes.
type PathValuePresenceImplicationSet struct {
	implications []PathValuePresenceImplication
}

func NewPathValuePresenceImplication(
	triggerPath path.Path,
	triggerValue product.Value,
	targetPath path.Path,
	targetPresence presence.Value,
) PathValuePresenceImplication {
	return PathValuePresenceImplication{
		triggerPath:    triggerPath.Clone(),
		triggerValue:   triggerValue,
		targetPath:     targetPath.Clone(),
		targetPresence: targetPresence,
	}
}

func NewPathValueRefinementImplication(
	triggerPath path.Path,
	triggerValue product.Value,
	targetPath path.Path,
	targetValue product.Value,
) PathValuePresenceImplication {
	return PathValuePresenceImplication{
		triggerPath:    triggerPath.Clone(),
		triggerValue:   triggerValue,
		targetPath:     targetPath.Clone(),
		targetValue:    targetValue,
		hasTargetValue: true,
	}
}

func NewPathTruthyValueRefinementImplication(
	triggerPath path.Path,
	triggerValue product.Value,
	targetPath path.Path,
	targetValue product.Value,
) PathValuePresenceImplication {
	return PathValuePresenceImplication{
		triggerPath:        triggerPath.Clone(),
		triggerValue:       triggerValue,
		triggerPresence:    presence.Present(),
		hasTriggerPresence: true,
		targetPath:         targetPath.Clone(),
		targetValue:        targetValue,
		hasTargetValue:     true,
	}
}

func NewPathEqualityValueRefinementImplication(
	triggerPath path.Path,
	otherPath path.Path,
	targetPath path.Path,
	targetValue product.Value,
) PathValuePresenceImplication {
	return PathValuePresenceImplication{
		triggerPath:         triggerPath.Clone(),
		triggerOtherPath:    otherPath.Clone(),
		hasTriggerPathEqual: true,
		targetPath:          targetPath.Clone(),
		targetValue:         targetValue,
		hasTargetValue:      true,
	}
}

func NewPathValuePresenceImplicationSet(implications ...PathValuePresenceImplication) PathValuePresenceImplicationSet {
	return PathValuePresenceImplicationSet{implications: copyPathValuePresenceImplicationSlice(implications)}
}

func (i PathValuePresenceImplication) TriggerPath() path.Path { return i.triggerPath.Clone() }

// TriggerPathRef returns the trigger path for immediate read-only use.
// Callers must not mutate or retain the returned path.
func (i PathValuePresenceImplication) TriggerPathRef() path.Path { return i.triggerPath }

func (i PathValuePresenceImplication) TriggerOtherPath() path.Path { return i.triggerOtherPath.Clone() }

// TriggerOtherPathRef returns the secondary trigger path for immediate read-only
// use. Callers must not mutate or retain the returned path.
func (i PathValuePresenceImplication) TriggerOtherPathRef() path.Path { return i.triggerOtherPath }

func (i PathValuePresenceImplication) TriggerValue() product.Value { return i.triggerValue }

func (i PathValuePresenceImplication) TriggerPresence() presence.Value { return i.triggerPresence }

func (i PathValuePresenceImplication) HasTriggerPresence() bool { return i.hasTriggerPresence }

func (i PathValuePresenceImplication) HasTriggerPathEqual() bool { return i.hasTriggerPathEqual }

func (i PathValuePresenceImplication) TargetPath() path.Path { return i.targetPath.Clone() }

// TargetPathRef returns the target path for immediate read-only use.
// Callers must not mutate or retain the returned path.
func (i PathValuePresenceImplication) TargetPathRef() path.Path { return i.targetPath }

func (i PathValuePresenceImplication) TargetPresence() presence.Value { return i.targetPresence }

func (i PathValuePresenceImplication) TargetValue() product.Value { return i.targetValue }

func (i PathValuePresenceImplication) HasTargetValue() bool { return i.hasTargetValue }

func (i PathValuePresenceImplication) copy() PathValuePresenceImplication {
	i.triggerPath = i.triggerPath.Clone()
	i.triggerOtherPath = i.triggerOtherPath.Clone()
	i.targetPath = i.targetPath.Clone()
	return i
}

func (s PathValuePresenceImplicationSet) Implications() []PathValuePresenceImplication {
	return copyPathValuePresenceImplicationSlice(s.implications)
}

func (s PathValuePresenceImplicationSet) copy() PathValuePresenceImplicationSet {
	return PathValuePresenceImplicationSet{implications: copyPathValuePresenceImplicationSlice(s.implications)}
}

func copyPathValuePresenceImplicationSlice(in []PathValuePresenceImplication) []PathValuePresenceImplication {
	if len(in) == 0 {
		return nil
	}
	out := make([]PathValuePresenceImplication, len(in))
	for i, fact := range in {
		out[i] = fact.copy()
	}
	return out
}
