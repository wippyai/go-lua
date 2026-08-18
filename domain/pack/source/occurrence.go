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

type mountedSourceRows struct {
	rule   *HotRule
	module identity.ContentID
	rows   map[identity.ContentID]packdomain.SourceResult
}

// MountedIssuer is the exact PackSource substitution authority for one
// mounted ModuleKey.  Equal Program occurrences mounted twice are kept in
// distinct issuers and cannot cross this fence.
type MountedIssuer struct{ rows *mountedSourceRows }

func (rows *mountedSourceRows) valid(rule *HotRule) bool {
	return rule != nil && rows != nil && rows.rule == rule && rows.module.Available() &&
		rule.occurrences != nil && rule.occurrences[rows.module] == rows && rows.rows != nil
}

func (rule *HotRule) sealOccurrenceReceipts() bool {
	if rule == nil || rule.schema == nil {
		return false
	}
	if _, ok := packowner.ResolveRuleImplementation(rule.implementation); !ok {
		return false
	}
	if rule.receiptsSealed {
		return rule.occurrences != nil
	}
	if !rule.schema.LinkOwner().Available() {
		return false
	}
	occurrences := make(map[identity.ContentID]*mountedSourceRows)
	for index := 0; index < rule.schema.SourceOccurrenceCount(); index++ {
		module, id, result, ok := rule.schema.SourceOccurrenceAt(index)
		if !ok || !module.Available() || !id.Available() || !rule.schema.OwnsSourceResult(result) {
			return false
		}
		rows := occurrences[module]
		if rows == nil {
			rows = &mountedSourceRows{rule: rule, module: module, rows: make(map[identity.ContentID]packdomain.SourceResult)}
			occurrences[module] = rows
		}
		if _, duplicate := rows.rows[id]; duplicate {
			return false
		}
		rows.rows[id] = result
	}
	rule.occurrences = occurrences
	rule.receiptsSealed = true
	return len(occurrences) != 0 || rule.schema.SourceOccurrenceCount() == 0
}

// SealOccurrenceReceipts preissues all mounted Pack source receipts after the
// shared binding has sealed. It is the explicit cold lifecycle boundary.
func (rule *HotRule) SealOccurrenceReceipts() bool {
	return rule != nil && rule.sealOccurrenceReceipts()
}

// ForMount returns the exact mounted source issuer for one ModuleKey.
func (rule *HotRule) ForMount(module identity.ContentID) (MountedIssuer, bool) {
	if rule == nil || !rule.receiptsSealed || !module.Available() {
		return MountedIssuer{}, false
	}
	rows := rule.occurrences[module]
	issuer := MountedIssuer{rows: rows}
	return issuer, rows.valid(rule)
}

// ReceiptForOccurrence returns the exact presealed Pack source result in O(1).
func (issuer MountedIssuer) ReceiptForOccurrence(id identity.ContentID) (packdomain.SourceResult, bool) {
	if issuer.rows == nil || !id.Available() || !issuer.rows.valid(issuer.rows.rule) {
		return packdomain.SourceResult{}, false
	}
	result, ok := issuer.rows.rows[id]
	return result, ok && issuer.rows.rule.schema.OwnsSourceResult(result)
}

// SourceForOccurrence projects the typed source descriptor alongside its
// already sealed result for callers that need both hot operands.
func (issuer MountedIssuer) SourceForOccurrence(id identity.ContentID) (packdomain.Source, packdomain.SourceResult, bool) {
	result, ok := issuer.ReceiptForOccurrence(id)
	if !ok {
		return packdomain.Source{}, packdomain.SourceResult{}, false
	}
	source, sourceOK := result.Source()
	return source, result, sourceOK
}

