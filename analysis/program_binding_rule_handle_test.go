package analysis

import (
	"github.com/wippyai/go-lua/domain/composite"
	callsite "github.com/wippyai/go-lua/domain/effect/callsite"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
)

// The helpers below recover one rule's own implementation from the sealed
// table so a domain law can state itself against the exact bound rule.
// Production wiring drives the table and never needs them.
func selectedEffectRule(binding *composite.ProgramBinding) *callsite.HotRule {
	rule, _ := composite.RuleHandle[*callsite.HotRule](binding.Rules(), programartifact.RuleRoleEffectSelected)
	return rule
}

func opaqueEffectRule(binding *composite.ProgramBinding) *callsite.HotRule {
	rule, _ := composite.RuleHandle[*callsite.HotRule](binding.Rules(), programartifact.RuleRoleEffectOpaque)
	return rule
}

func bodyEffectRule(binding *composite.ProgramBinding) *callsite.BodyHotRule {
	rule, _ := composite.RuleHandle[*callsite.BodyHotRule](binding.Rules(), programartifact.RuleRoleEffectBody)
	return rule
}
