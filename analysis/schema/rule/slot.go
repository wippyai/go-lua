package rule

import "github.com/wippyai/go-lua/analysis/engine"

// RegisterMountedSlot performs the short-lived pre-seal owner handoff for a
// mounted-lane rule. The capability is not retained past the pairing pass:
// once SchemaBinding seals, callers resolve the exact capability from its
// semantic directory.
//
// It is the Register hook of every mounted rule, so it lives with the surface
// that declares the hook rather than being restated by each owning domain.
func RegisterMountedSlot[V, O any](binding *engine.SchemaBinding, slot *engine.RuleSlot[V, O]) (engine.RuleSlotCapability, bool) {
	capability, ok := engine.IssueMountedRuleCapability(binding, slot)
	if !ok || !engine.RegisterRuleSlot(binding, slot, capability) {
		return engine.RuleSlotCapability{}, false
	}
	return capability, true
}

// RegisterLinkSlot is the same handoff on the Link lane.
func RegisterLinkSlot[V, O any](binding *engine.SchemaBinding, slot *engine.RuleSlot[V, O]) (engine.RuleSlotCapability, bool) {
	capability, ok := engine.IssueLinkRuleCapability(binding, slot)
	if !ok || !engine.RegisterRuleSlot(binding, slot, capability) {
		return engine.RuleSlotCapability{}, false
	}
	return capability, true
}
