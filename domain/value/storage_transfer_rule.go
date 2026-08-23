package value

import "github.com/wippyai/go-lua/analysis/schema/rule"

// StorageTransferRuleIssues is Value's canonical mounted issuance geometry.
// Every consumer of Value's sealed StorageTransfer relation subscribes to this
// one declaration; callers receive a fresh slice so no rule can mutate the
// owner’s canonical list for another consumer.
func StorageTransferRuleIssues() []rule.Issuance {
	return []rule.Issuance{
		{Occurrence: "occurrence/storage-read", Requirement: "program-requirement/unrestricted", Form: "program-form/local-entry"},
		{Occurrence: "occurrence/storage-bind-transfer", Requirement: "program-requirement/unrestricted", Form: "program-form/local-successor"},
		{Occurrence: "occurrence/storage-bind-transfer", Requirement: "program-requirement/tail-transfer-result", Form: "program-form/call-effect"},
		{Occurrence: "occurrence/storage-write", Requirement: "program-requirement/unrestricted", Form: "program-form/local-predecessor"},
	}
}
