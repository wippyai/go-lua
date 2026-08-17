package bootstrap

import "github.com/wippyai/go-lua/analysis/engine"

func linkCapability(issuer interface {
	LinkCapability() (engine.RuleSlotCapability, bool)
}) engine.RuleSlotCapability {
	capability, _ := issuer.LinkCapability()
	return capability
}
