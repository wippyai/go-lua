package engine

import "github.com/wippyai/go-lua/analysis/identity"

// AdmitActivationByTrustedTheorem names the reviewed receipt-native theorem
// used by mounted activation Rule slots. It carries no declaration
// authority or Composition dependency.
func AdmitActivationByTrustedTheorem(identity identity.SemanticKey) RuleAdmission[ActivationResult, ruleUnit] {
	return AdmitRuleByTrustedTheorem[ActivationResult, ruleUnit](identity)
}
