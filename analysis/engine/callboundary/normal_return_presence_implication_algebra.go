package callboundary

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice/factset"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

type pathPresenceImplicationKey struct {
	trigger         pathdom.PathKey
	triggerPresence presence.Value
	triggerValue    product.Value
	hasTriggerValue bool
	target          pathdom.PathKey
	targetPresence  presence.Value
	targetValue     product.Value
	hasTargetValue  bool
}

func pathPresenceImplicationLane(reg *axis.Registry) factset.Set[pathPresenceImplicationKey, PathPresenceImplicationFact] {
	return factset.Set[pathPresenceImplicationKey, PathPresenceImplicationFact]{
		Key: pathPresenceImplicationKeyOf,
		EqualFact: func(a, b PathPresenceImplicationFact) bool {
			return pathPresenceImplicationEqual(reg, a, b)
		},
		Less: func(a, b PathPresenceImplicationFact) bool {
			return pathPresenceImplicationLess(reg, a, b)
		},
		Admit:     admitPathPresenceImplication,
		CloneFact: clonePathPresenceImplication,
		Prefer:    func(kept, incoming PathPresenceImplicationFact) bool { return true },
		Intersect: true,
	}
}

func clonePathPresenceImplication(f PathPresenceImplicationFact) PathPresenceImplicationFact {
	f.Trigger = f.Trigger.Clone()
	f.Target = f.Target.Clone()
	return f
}

func clonePathPresenceImplications(in []PathPresenceImplicationFact) []PathPresenceImplicationFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]PathPresenceImplicationFact, len(in))
	for i, fact := range in {
		out[i] = clonePathPresenceImplication(fact)
	}
	return out
}

func admitPathPresenceImplication(f PathPresenceImplicationFact) (PathPresenceImplicationFact, bool) {
	if !f.Trigger.IsPlaceholder() || !f.Target.IsPlaceholder() {
		return f, false
	}
	if !f.HasTargetValue && (f.TargetPresence.IsBottom() || f.TargetPresence.IsTop()) {
		return f, false
	}
	if f.HasTargetValue && f.TargetValue == product.Top() {
		return f, false
	}
	if !f.HasTriggerValue && (f.TriggerPresence.IsBottom() || f.TriggerPresence.IsTop()) {
		return f, false
	}
	return f, true
}

func pathPresenceImplicationKeyOf(f PathPresenceImplicationFact) pathPresenceImplicationKey {
	return pathPresenceImplicationKey{
		trigger:         f.Trigger.Key(),
		triggerPresence: f.TriggerPresence,
		triggerValue:    f.TriggerValue,
		hasTriggerValue: f.HasTriggerValue,
		target:          f.Target.Key(),
		targetPresence:  f.TargetPresence,
		targetValue:     f.TargetValue,
		hasTargetValue:  f.HasTargetValue,
	}
}

func pathPresenceImplicationEqual(reg *axis.Registry, a, b PathPresenceImplicationFact) bool {
	return a.Trigger.Equal(b.Trigger) &&
		a.TriggerPresence == b.TriggerPresence &&
		a.HasTriggerValue == b.HasTriggerValue &&
		(!a.HasTriggerValue || product.Equal(reg, a.TriggerValue, b.TriggerValue)) &&
		a.Target.Equal(b.Target) &&
		a.TargetPresence == b.TargetPresence &&
		a.HasTargetValue == b.HasTargetValue &&
		(!a.HasTargetValue || product.Equal(reg, a.TargetValue, b.TargetValue))
}

func pathPresenceImplicationLess(reg *axis.Registry, a, b PathPresenceImplicationFact) bool {
	ka := pathPresenceImplicationKeyOf(a)
	kb := pathPresenceImplicationKeyOf(b)
	if ka.trigger != kb.trigger {
		return ka.trigger < kb.trigger
	}
	if ka.triggerPresence != kb.triggerPresence {
		return ka.triggerPresence < kb.triggerPresence
	}
	if ka.hasTriggerValue != kb.hasTriggerValue {
		return !ka.hasTriggerValue
	}
	if ka.hasTriggerValue {
		if av, bv := product.Hash(reg, ka.triggerValue), product.Hash(reg, kb.triggerValue); av != bv {
			return av < bv
		}
	}
	if ka.target != kb.target {
		return ka.target < kb.target
	}
	if ka.targetPresence != kb.targetPresence {
		return ka.targetPresence < kb.targetPresence
	}
	if ka.hasTargetValue != kb.hasTargetValue {
		return !ka.hasTargetValue
	}
	if ka.hasTargetValue {
		if av, bv := product.Hash(reg, ka.targetValue), product.Hash(reg, kb.targetValue); av != bv {
			return av < bv
		}
	}
	return false
}
