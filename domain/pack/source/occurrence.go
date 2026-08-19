package source

// This file owns the one Link-local substitution from reusable Program
// source/Values and Call occurrence IDs to Pack's already sealed source
// receipts.  It is intentionally separate from the hot transfer callback:
// the callback sees only the typed receipt and never walks Program or Flow.

import (
	"github.com/wippyai/go-lua/analysis/identity"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	packowner "github.com/wippyai/go-lua/domain/pack/owner"
)

func (rule *HotRule) sealOccurrenceReceipts() bool {
	if rule == nil || rule.schema == nil {
		return false
	}
	if _, ok := packowner.ResolveRuleImplementation(rule.implementation); !ok {
		return false
	}
	if rule.receiptsSealed {
		return true
	}
	if !rule.schema.LinkOwner().Available() {
		return false
	}
	for index := 0; index < rule.schema.SourceOccurrenceCount(); index++ {
		module, id, result, ok := rule.schema.SourceOccurrenceAt(index)
		issued, issuedOK := rule.schema.SourceResultForMountedOccurrence(module, id)
		if !ok || !module.Available() || !id.Available() || !rule.schema.OwnsSourceResult(result) || !issuedOK || issued != result {
			return false
		}
	}
	rule.receiptsSealed = true
	return true
}

// SealOccurrenceReceipts closes Pack's mounted source census against the
// schema's direct occurrence inverse. It is the explicit cold lifecycle
// boundary, so the first engine lookup never has to establish it.
func (rule *HotRule) SealOccurrenceReceipts() bool {
	return rule != nil && rule.sealOccurrenceReceipts()
}

// receiptForOccurrence redeems one presealed Pack source result directly from
// the schema that owns the mounted occurrence census.
func (rule *HotRule) receiptForOccurrence(mount, occurrence identity.ContentID) (packdomain.SourceResult, bool) {
	if rule == nil || !rule.receiptsSealed || rule.schema == nil {
		return packdomain.SourceResult{}, false
	}
	return rule.schema.SourceResultForMountedOccurrence(mount, occurrence)
}
