package analysis

import (
	callsite "github.com/wippyai/go-lua/analysis/domain/effect/callsite"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema/grammar"
)

// The helpers below recover one rule's own implementation from the sealed
// table so a domain law can state itself against the exact bound rule.
// Production wiring drives the table and never needs them.
func (binding *programBinding) selectedEffectRule() *callsite.HotRule {
	rule, _ := grammar.RuleHandle[*callsite.HotRule](binding.rules, programartifact.RuleRoleEffectSelected)
	return rule
}

func (binding *programBinding) opaqueEffectRule() *callsite.HotRule {
	rule, _ := grammar.RuleHandle[*callsite.HotRule](binding.rules, programartifact.RuleRoleEffectOpaque)
	return rule
}

func (binding *programBinding) bodyEffectRule() *callsite.BodyHotRule {
	rule, _ := grammar.RuleHandle[*callsite.BodyHotRule](binding.rules, programartifact.RuleRoleEffectBody)
	return rule
}
