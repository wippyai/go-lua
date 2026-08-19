package callsite

// Mounted occurrence issuers are the Link-local inverse for reusable Call
// rows. They are sealed once and then provide O(1) typed receipts without
// reopening Project, Boundary, Program, or Flow.

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/identity"
)

type mountedReceiptRows struct {
	rule   *HotRule
	module identity.ContentID
	rows   map[identity.ContentID]hotOperand
}

func (rule *HotRule) sealOccurrenceReceipts() bool {
	if rule == nil || rule.binding == nil || !rule.binding.Sealed() || rule.calls == nil || rule.calls.Algebra() == nil || rule.effects == nil || rule.effects.Algebra() == nil {
		return false
	}
	if rule.receiptsSealed {
		return rule.occurrences != nil
	}
	rule.receiptsSealed = true
	effects := rule.effects.Algebra()
	occurrences := make(map[identity.ContentID]*mountedReceiptRows)
	for index := 0; index < effects.MountedCallCount(); index++ {
		mounted, mountedOK := effects.MountedCallAt(index)
		_, module, id, identityOK := effects.MountedCallIdentity(mounted)
		if !mountedOK || !identityOK || !module.Available() || !id.Available() {
			return false
		}
		rows := occurrences[module]
		if rows == nil {
			rows = &mountedReceiptRows{rule: rule, module: module, rows: make(map[identity.ContentID]hotOperand)}
			occurrences[module] = rows
		}
		operand, operandOK := rule.Receipt(mounted)
		if !operandOK || operand.receipt == nil || !operand.receipt.valid() {
			return false
		}
		if _, duplicate := rows.rows[id]; duplicate {
			return false
		}
		rows.rows[id] = operand
	}
	rule.occurrences = occurrences
	return len(occurrences) != 0 || effects.MountedCallCount() == 0
}

// SealOccurrenceReceipts issues every Selected/Opaque mounted Call receipt
// and closes its module-scoped occurrence inverse.
func (rule *HotRule) SealOccurrenceReceipts() bool {
	return rule != nil && rule.sealOccurrenceReceipts()
}

// ForMount returns this rule's exact receipt issuer for one mounted module.
func (rule *HotRule) ForMount(module identity.ContentID) (ReceiptIssuer, bool) {
	if rule == nil || !rule.receiptsSealed || !module.Available() {
		return ReceiptIssuer{}, false
	}
	rows := rule.occurrences[module]
	issuer := ReceiptIssuer{rows: rows}
	return issuer, rows != nil && rows.rule == rule && rows.module == module && rows.rows != nil
}

type ReceiptIssuer struct{ rows *mountedReceiptRows }

func (issuer ReceiptIssuer) ReceiptForOccurrence(id identity.ContentID) (hotOperand, bool) {
	if issuer.rows == nil || !id.Available() || issuer.rows.rule == nil || issuer.rows.rule.occurrences[issuer.rows.module] != issuer.rows {
		return hotOperand{}, false
	}
	operand, ok := issuer.rows.rows[id]
	return operand, ok && issuer.rows.rule.accepts(operand)
}

// MountedSelectedCallEffectStage returns the cold ProgramArtifact stage proof
// for one exact selected Call. The point is derived inside engine from this
// rule's owner capability plus mount/occurrence; callers cannot supply or
// splice a stage point. Opaque Call handling is intentionally a distinct role
// and cannot issue this selected receipt.
func (rule *HotRule) MountedSelectedCallEffectStage(compilation *engine.ProgramConstruction, mountID, occurrenceID identity.ContentID) (engine.ProgramCallStage, bool) {
	if rule == nil || rule.opaque || compilation == nil || !mountID.Available() || !occurrenceID.Available() || rule.implementation == nil {
		return engine.ProgramCallStage{}, false
	}
	issuer, issuerOK := rule.ForMount(mountID)
	_, occurrenceOK := issuer.ReceiptForOccurrence(occurrenceID)
	capability, capabilityOK := rule.implementation.MountedCapability()
	if !issuerOK || !occurrenceOK || !capabilityOK {
		return engine.ProgramCallStage{}, false
	}
	stage, ok := compilation.MountedCallStage(capability, mountID, occurrenceID)
	return stage, ok && stage.Kind() == rows.ArtifactRuleStageIssued5
}

func (rule *BodyHotRule) sealOccurrenceReceipts() bool {
	if rule == nil || rule.binding == nil || !rule.binding.Sealed() || rule.calls == nil || rule.calls.Algebra() == nil || rule.effects == nil || rule.effects.Algebra() == nil {
		return false
	}
	if rule.receiptsSealed {
		return rule.occurrences != nil
	}
	rule.receiptsSealed = true
	effects := rule.effects.Algebra()
	occurrences := make(map[identity.ContentID]*mountedBodyReceiptRows)
	for index := 0; index < effects.MountedCallCount(); index++ {
		mounted, mountedOK := effects.MountedCallAt(index)
		_, module, id, identityOK := effects.MountedCallIdentity(mounted)
		if !mountedOK || !identityOK || !module.Available() || !id.Available() {
			return false
		}
		rows := occurrences[module]
		if rows == nil {
			rows = &mountedBodyReceiptRows{rule: rule, module: module, rows: make(map[identity.ContentID]hotBodyOperand)}
			occurrences[module] = rows
		}
		operand, operandOK := rule.Receipt(mounted)
		if !operandOK || operand.receipt == nil || !operand.receipt.valid() {
			return false
		}
		if _, duplicate := rows.rows[id]; duplicate {
			return false
		}
		rows.rows[id] = operand
	}
	rule.occurrences = occurrences
	return len(occurrences) != 0 || effects.MountedCallCount() == 0
}

type mountedBodyReceiptRows struct {
	rule   *BodyHotRule
	module identity.ContentID
	rows   map[identity.ContentID]hotBodyOperand
}

// SealOccurrenceReceipts issues every Body mounted Call receipt and closes
// its module-scoped occurrence inverse.
func (rule *BodyHotRule) SealOccurrenceReceipts() bool {
	return rule != nil && rule.sealOccurrenceReceipts()
}

// ForMount returns Body's exact mounted receipt issuer.
func (rule *BodyHotRule) ForMount(module identity.ContentID) (BodyReceiptIssuer, bool) {
	if rule == nil || !rule.receiptsSealed || !module.Available() {
		return BodyReceiptIssuer{}, false
	}
	rows := rule.occurrences[module]
	issuer := BodyReceiptIssuer{rows: rows}
	return issuer, rows != nil && rows.rule == rule && rows.module == module && rows.rows != nil
}

type BodyReceiptIssuer struct{ rows *mountedBodyReceiptRows }

// BodyReceiptFinalizationFailure records the first closed source-seal step
// rejected for a mounted Body row. It is retained on the Body owner only as
// scalar diagnostic evidence; no source, graph, or callback capability
// escapes after a failed assembly.
type BodyReceiptFinalizationFailure uint8

const (
	BodyReceiptFinalizationFailureNone BodyReceiptFinalizationFailure = iota
	BodyReceiptFinalizationFailureSourcePrecondition
	BodyReceiptFinalizationFailureSourceColdShape
	BodyReceiptFinalizationFailureSourceIssueArguments
	BodyReceiptFinalizationFailureSourceIssueTopology
	BodyReceiptFinalizationFailureSourceIssueRule
	BodyReceiptFinalizationFailureSourceIssueShape
	BodyReceiptFinalizationFailureSourceIssueReadAuthority
	BodyReceiptFinalizationFailureSourceIssueReadSurface
	BodyReceiptFinalizationFailureSourceIssueReadFactor
	BodyReceiptFinalizationFailureSourceIssueReadAnchor
	BodyReceiptFinalizationFailureSourceIssueWrite
	BodyReceiptFinalizationFailureSourceIssueBatch
	BodyReceiptFinalizationFailureSourceSummary
	BodyReceiptFinalizationFailureDraft
	BodyReceiptFinalizationFailureReadPart
	BodyReceiptFinalizationFailureWritePart
	BodyReceiptFinalizationFailureDraftRead
	BodyReceiptFinalizationFailureDraftWrite
	BodyReceiptFinalizationFailureGraph
)

func (failure BodyReceiptFinalizationFailure) String() string {
	switch failure {
	case BodyReceiptFinalizationFailureNone:
		return "none"
	case BodyReceiptFinalizationFailureSourcePrecondition:
		return "source-precondition"
	case BodyReceiptFinalizationFailureSourceColdShape:
		return "source-cold-shape"
	case BodyReceiptFinalizationFailureSourceIssueArguments:
		return "source-issue-arguments"
	case BodyReceiptFinalizationFailureSourceIssueTopology:
		return "source-issue-topology"
	case BodyReceiptFinalizationFailureSourceIssueRule:
		return "source-issue-rule"
	case BodyReceiptFinalizationFailureSourceIssueShape:
		return "source-issue-shape"
	case BodyReceiptFinalizationFailureSourceIssueReadAuthority:
		return "source-issue-read-authority"
	case BodyReceiptFinalizationFailureSourceIssueReadSurface:
		return "source-issue-read-surface"
	case BodyReceiptFinalizationFailureSourceIssueReadFactor:
		return "source-issue-read-factor"
	case BodyReceiptFinalizationFailureSourceIssueReadAnchor:
		return "source-issue-read-anchor"
	case BodyReceiptFinalizationFailureSourceIssueWrite:
		return "source-issue-write"
	case BodyReceiptFinalizationFailureSourceIssueBatch:
		return "source-issue-batch"
	case BodyReceiptFinalizationFailureSourceSummary:
		return "source-summary"
	case BodyReceiptFinalizationFailureDraft:
		return "draft"
	case BodyReceiptFinalizationFailureReadPart:
		return "read-part"
	case BodyReceiptFinalizationFailureWritePart:
		return "write-part"
	case BodyReceiptFinalizationFailureDraftRead:
		return "draft-read"
	case BodyReceiptFinalizationFailureDraftWrite:
		return "draft-write"
	case BodyReceiptFinalizationFailureGraph:
		return "graph"
	default:
		return "unknown"
	}
}

// FinalizationFailure returns the first Body source-seal rejection, if any.
func (rule *BodyHotRule) FinalizationFailure() BodyReceiptFinalizationFailure {
	if rule == nil {
		return BodyReceiptFinalizationFailureSourcePrecondition
	}
	return rule.finalizationFailure
}

func (rule *BodyHotRule) recordFinalizationFailure(failure BodyReceiptFinalizationFailure) {
	if rule != nil && rule.finalizationFailure == BodyReceiptFinalizationFailureNone {
		rule.finalizationFailure = failure
	}
}

func (issuer BodyReceiptIssuer) ReceiptForOccurrence(id identity.ContentID) (hotBodyOperand, bool) {
	if issuer.rows == nil || !id.Available() || issuer.rows.rule == nil || issuer.rows.rule.occurrences[issuer.rows.module] != issuer.rows {
		return hotBodyOperand{}, false
	}
	operand, ok := issuer.rows.rows[id]
	return operand, ok && issuer.rows.rule.accepts(operand)
}
