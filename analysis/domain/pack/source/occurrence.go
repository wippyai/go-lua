package source

// This file owns the one Link-local substitution from reusable Program
// source/Values and Call occurrence IDs to Pack's already sealed source
// receipts.  It is intentionally separate from the hot transfer callback:
// the callback sees only the typed receipt and never walks Program or Flow.

import (
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	packowner "github.com/wippyai/go-lua/analysis/domain/pack/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
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
	if len(occurrences) == 0 {
		return false
	}
	rule.occurrences = occurrences
	rule.receiptsSealed = true
	return true
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

// AttachMountedOccurrence admits one artifact PackSource row and supplies its
// exact Pack output surface. The caller chooses no factor ordinal: Root and
// its Ref are issued by Pack's sealed owner.
func (rule *HotRule) AttachMountedOccurrence(assembly *engine.ReceiptAssembly, mountID, reusablePointID, occurrenceID identity.ContentID) (engine.BindingRuleRowRef, bool) {
	if rule == nil || rule.owner == nil || assembly == nil {
		return engine.BindingRuleRowRef{}, false
	}
	issuer, ok := rule.ForMount(mountID)
	if !ok {
		return engine.BindingRuleRowRef{}, false
	}
	_, result, ok := issuer.SourceForOccurrence(occurrenceID)
	if !ok {
		return engine.BindingRuleRowRef{}, false
	}
	source, sourceOK := result.Source()
	root, rootOK := result.Root()
	if !sourceOK || !rootOK || !rule.schema.OwnsSourceResult(result) {
		return engine.BindingRuleRowRef{}, false
	}
	capability := mountedCapability(rule.implementation)
	occurrence, ok := assembly.AdmitMountedRuleOccurrence(capability, mountID, reusablePointID, occurrenceID)
	if !ok {
		return engine.BindingRuleRowRef{}, false
	}
	implementation, implementationOK := packowner.ResolveRuleImplementation(rule.implementation)
	transaction, ok := engine.BeginMountedRuleAdmission(assembly, implementation, occurrence, source)
	if !implementationOK || !ok {
		return engine.BindingRuleRowRef{}, false
	}
	ref, ok := rule.owner.Ref(root)
	if !ok || !engine.AddExactWrite(transaction, ref) {
		return engine.BindingRuleRowRef{}, false
	}
	queued := assembly.QueueMountedRuleFinalizer(capability, func() bool {
		sourceReceipt, sourceOK := transaction.Seal()
		if !sourceOK {
			return false
		}
		draft, draftOK := implementation.BeginReceiptRuleRow(sourceReceipt)
		writePart, writePartOK := implementation.ReceiptWritePart(sourceReceipt, 0)
		if !draftOK || !writePartOK || !draft.AddWrite(writePart) {
			return false
		}
		_, added := assembly.AddRuleFromDraft(occurrence, draft)
		return added
	})
	return engine.BindingRuleRowRef{}, queued
}

// AttachMountedReceiptMember resolves and attaches the exact graph member
// after receipt compilation has committed its graph. It is the post-commit
// counterpart to AttachMountedOccurrence and retains no equation handle.
func (rule *HotRule) AttachMountedReceiptMember(compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, mountID, reusablePointID, occurrenceID identity.ContentID) (*engine.ReceiptMember, bool) {
	if rule == nil || compilation == nil || graph == nil || rule.implementation == nil {
		return nil, false
	}
	member, ok := graph.MountedRuleMember(mountedCapability(rule.implementation), mountID, reusablePointID, occurrenceID)
	if !ok {
		return nil, false
	}
	issuer, ok := rule.ForMount(mountID)
	if !ok {
		return nil, false
	}
	source, _, ok := issuer.SourceForOccurrence(occurrenceID)
	if !ok {
		return nil, false
	}
	implementation, ok := packowner.ResolveRuleImplementation(rule.implementation)
	if !ok {
		return nil, false
	}
	return engine.AttachReceiptRuleMember(compilation, implementation, member, source)
}
