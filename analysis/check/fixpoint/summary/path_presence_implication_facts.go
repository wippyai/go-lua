package summary

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/lattice/factset"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

type pathPresenceImplicationKey struct {
	trigger         pathdom.PathKey
	triggerPresence presence.Value
	triggerValue    product.Value
	hasTriggerValue bool
	target          pathdom.PathKey
	targetPresence  presence.Value
}

var pathPresenceImplicationLane = factset.Set[pathPresenceImplicationKey, callboundary.PathPresenceImplicationFact]{
	Key:       pathPresenceImplicationKeyOf,
	EqualFact: pathPresenceImplicationEqual,
	Less:      pathPresenceImplicationLess,
	Admit:     admitPathPresenceImplication,
	CloneFact: func(f callboundary.PathPresenceImplicationFact) callboundary.PathPresenceImplicationFact {
		f.Trigger = f.Trigger.Clone()
		f.Target = f.Target.Clone()
		return f
	},
	Prefer:    func(kept, incoming callboundary.PathPresenceImplicationFact) bool { return true },
	Intersect: true,
}

func admitPathPresenceImplication(f callboundary.PathPresenceImplicationFact) (callboundary.PathPresenceImplicationFact, bool) {
	if !f.Trigger.IsPlaceholder() || !f.Target.IsPlaceholder() {
		return f, false
	}
	if f.TargetPresence.IsBottom() || f.TargetPresence.IsTop() {
		return f, false
	}
	if !f.HasTriggerValue && (f.TriggerPresence.IsBottom() || f.TriggerPresence.IsTop()) {
		return f, false
	}
	return f, true
}

func pathPresenceImplicationKeyOf(f callboundary.PathPresenceImplicationFact) pathPresenceImplicationKey {
	return pathPresenceImplicationKey{
		trigger:         f.Trigger.Key(),
		triggerPresence: f.TriggerPresence,
		triggerValue:    f.TriggerValue,
		hasTriggerValue: f.HasTriggerValue,
		target:          f.Target.Key(),
		targetPresence:  f.TargetPresence,
	}
}

func pathPresenceImplicationEqual(a, b callboundary.PathPresenceImplicationFact) bool {
	return a.Trigger.Equal(b.Trigger) &&
		a.TriggerPresence == b.TriggerPresence &&
		a.HasTriggerValue == b.HasTriggerValue &&
		(!a.HasTriggerValue || product.Equal(nil, a.TriggerValue, b.TriggerValue)) &&
		a.Target.Equal(b.Target) &&
		a.TargetPresence == b.TargetPresence
}

func pathPresenceImplicationLess(a, b callboundary.PathPresenceImplicationFact) bool {
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
		av := fmt.Sprintf("%#v", ka.triggerValue)
		bv := fmt.Sprintf("%#v", kb.triggerValue)
		if av != bv {
			return av < bv
		}
	}
	if ka.target != kb.target {
		return ka.target < kb.target
	}
	return ka.targetPresence < kb.targetPresence
}
