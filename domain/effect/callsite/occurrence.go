package callsite

// This file owns the sealed occurrence receipt planes for reusable Call rows.
// Each plane is filled once in Effect's canonical mounted-call order and then
// read by that order's own occurrence inverse, so a redemption reopens neither
// Project, Boundary, Program, nor Flow.

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/identity"
)

// Effect's mounted-call order is the ordinal space of every sealed receipt
// plane here. sealOccurrenceReceipts fills that plane once; redemption is the
// algebra's own occurrence inverse into it.
func (rule *HotRule) sealOccurrenceReceipts() bool {
	if rule == nil || rule.binding == nil || !rule.binding.Sealed() || rule.calls == nil || rule.calls.Algebra() == nil || rule.effects == nil || rule.effects.Algebra() == nil {
		return false
	}
	if rule.receiptsSealed {
		return rule.receipts != nil
	}
	rule.receiptsSealed = true
	effects := rule.effects.Algebra()
	receipts := make([]hotOperand, effects.MountedCallCount())
	for index := range receipts {
		mounted, mountedOK := effects.MountedCallAt(index)
		_, module, id, identityOK := effects.MountedCallIdentity(mounted)
		ordinal, ordinalOK := effects.MountedCallOrdinalForOccurrence(module, id)
		if !mountedOK || !identityOK || !module.Available() || !id.Available() || !ordinalOK || ordinal != index {
			return false
		}
		operand, operandOK := rule.Receipt(mounted)
		if !operandOK || operand.receipt == nil || !operand.receipt.valid() {
			return false
		}
		receipts[index] = operand
	}
	rule.receipts = receipts
	return true
}

// receiptForOccurrence is the one redemption of a sealed Callsite operand.
func (rule *HotRule) receiptForOccurrence(mount, occurrence identity.ContentID) (hotOperand, bool) {
	if rule == nil || !rule.receiptsSealed || rule.effects == nil || rule.effects.Algebra() == nil {
		return hotOperand{}, false
	}
	ordinal, ok := rule.effects.Algebra().MountedCallOrdinalForOccurrence(mount, occurrence)
	if !ok || ordinal < 0 || ordinal >= len(rule.receipts) {
		return hotOperand{}, false
	}
	return rule.receipts[ordinal], true
}

// SealOccurrenceReceipts issues every Selected/Opaque mounted Call receipt
// and closes its module-scoped occurrence inverse.
func (rule *HotRule) SealOccurrenceReceipts() bool {
	return rule != nil && rule.sealOccurrenceReceipts()
}

// MountedSelectedCallEffectStage returns the cold ProgramArtifact stage proof
// for one exact selected Call. The point is derived inside engine from this
// rule's owner capability plus mount/occurrence; callers cannot supply or
// splice a stage point. Opaque Call handling is intentionally a distinct role
// and cannot issue this selected receipt.
func (rule *HotRule) MountedSelectedCallEffectStage(committed *engine.CommittedProgram, mountID, occurrenceID identity.ContentID) (engine.ProgramCallStage, bool) {
	if rule == nil || rule.opaque || committed == nil || !mountID.Available() || !occurrenceID.Available() || rule.implementation == nil {
		return engine.ProgramCallStage{}, false
	}
	_, occurrenceOK := rule.receiptForOccurrence(mountID, occurrenceID)
	capability, capabilityOK := rule.implementation.MountedCapability()
	if !occurrenceOK || !capabilityOK {
		return engine.ProgramCallStage{}, false
	}
	stage, ok := committed.MountedNativeCallStage(capability, mountID, occurrenceID)
	return stage, ok && stage.Kind() == rows.ArtifactRuleStageIssued5
}

func (rule *BodyHotRule) sealOccurrenceReceipts() bool {
	if rule == nil || rule.binding == nil || !rule.binding.Sealed() || rule.calls == nil || rule.calls.Algebra() == nil || rule.effects == nil || rule.effects.Algebra() == nil {
		return false
	}
	if rule.receiptsSealed {
		return rule.receipts != nil
	}
	rule.receiptsSealed = true
	effects := rule.effects.Algebra()
	receipts := make([]hotBodyOperand, effects.MountedCallCount())
	for index := range receipts {
		mounted, mountedOK := effects.MountedCallAt(index)
		_, module, id, identityOK := effects.MountedCallIdentity(mounted)
		ordinal, ordinalOK := effects.MountedCallOrdinalForOccurrence(module, id)
		if !mountedOK || !identityOK || !module.Available() || !id.Available() || !ordinalOK || ordinal != index {
			return false
		}
		operand, operandOK := rule.Receipt(mounted)
		if !operandOK || operand.receipt == nil || !operand.receipt.valid() {
			return false
		}
		receipts[index] = operand
	}
	rule.receipts = receipts
	return true
}

// receiptForOccurrence is the one redemption of a sealed Body operand.
func (rule *BodyHotRule) receiptForOccurrence(mount, occurrence identity.ContentID) (hotBodyOperand, bool) {
	if rule == nil || !rule.receiptsSealed || rule.effects == nil || rule.effects.Algebra() == nil {
		return hotBodyOperand{}, false
	}
	ordinal, ok := rule.effects.Algebra().MountedCallOrdinalForOccurrence(mount, occurrence)
	if !ok || ordinal < 0 || ordinal >= len(rule.receipts) {
		return hotBodyOperand{}, false
	}
	return rule.receipts[ordinal], true
}

// SealOccurrenceReceipts issues every Body mounted Call receipt and closes
// its module-scoped occurrence inverse.
func (rule *BodyHotRule) SealOccurrenceReceipts() bool {
	return rule != nil && rule.sealOccurrenceReceipts()
}
