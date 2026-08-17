package callsite

// Mounted occurrence issuers are the Link-local inverse for reusable Call
// rows. They are sealed once and then provide O(1) typed receipts without
// reopening Project, Boundary, Program, or Flow.

import (
	effectowner "github.com/wippyai/go-lua/analysis/domain/effect/owner"
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
func (rule *HotRule) MountedSelectedCallEffectStage(graph *engine.ReceiptGraph, mountID, occurrenceID identity.ContentID) (engine.MountedNativeCallStageReceipt, bool) {
	if rule == nil || rule.opaque || graph == nil || !mountID.Available() || !occurrenceID.Available() || rule.implementation == nil {
		return engine.MountedNativeCallStageReceipt{}, false
	}
	issuer, issuerOK := rule.ForMount(mountID)
	_, occurrenceOK := issuer.ReceiptForOccurrence(occurrenceID)
	capability, capabilityOK := rule.implementation.MountedCapability()
	if !issuerOK || !occurrenceOK || !capabilityOK {
		return engine.MountedNativeCallStageReceipt{}, false
	}
	receipt, ok := graph.MountedNativeCallStage(capability, mountID, occurrenceID)
	return receipt, ok && receipt.Stage() == rows.ArtifactRuleStageCallEffect
}

// AttachMountedOccurrence admits one Selected/Opaque artifact Call row with
// exact Call predecessor and Effect output surfaces.
func (rule *HotRule) AttachMountedOccurrence(assembly *engine.ReceiptAssembly, mountID, reusablePointID, occurrenceID identity.ContentID) (engine.BindingRuleRowRef, bool) {
	if rule == nil || rule.implementation == nil || rule.calls == nil || rule.effects == nil || assembly == nil {
		return engine.BindingRuleRowRef{}, false
	}
	issuer, ok := rule.ForMount(mountID)
	if !ok {
		return engine.BindingRuleRowRef{}, false
	}
	operand, ok := issuer.ReceiptForOccurrence(occurrenceID)
	if !ok {
		return engine.BindingRuleRowRef{}, false
	}
	capability, capabilityOK := rule.implementation.MountedCapability()
	if !capabilityOK {
		return engine.BindingRuleRowRef{}, false
	}
	occurrence, ok := assembly.AdmitMountedRuleOccurrence(capability, mountID, reusablePointID, occurrenceID)
	if !ok {
		return engine.BindingRuleRowRef{}, false
	}
	implementation, implementationOK := effectowner.ResolveRuleImplementationFor(rule.effects, rule.implementation)
	transaction, ok := engine.BeginMountedRuleAdmission(assembly, implementation, occurrence, operand)
	if !implementationOK || !ok {
		return engine.BindingRuleRowRef{}, false
	}
	readRef, readOK := rule.calls.Ref(operand.key)
	writeRef, writeOK := rule.effects.Ref(operand.root)
	if !readOK || !writeOK || !engine.AddExactRead(transaction, readRef) || !engine.AddExactWrite(transaction, writeRef) {
		return engine.BindingRuleRowRef{}, false
	}
	queued := assembly.QueueMountedRuleFinalizer(capability, func() bool {
		sourceReceipt, sourceOK := transaction.Seal()
		if !sourceOK {
			return false
		}
		draft, draftOK := implementation.BeginReceiptRuleRow(sourceReceipt)
		readPart, readPartOK := implementation.ReceiptReadPart(sourceReceipt, 0)
		writePart, writePartOK := implementation.ReceiptWritePart(sourceReceipt, 0)
		if !draftOK || !readPartOK || !writePartOK || !draft.AddRead(readPart) || !draft.AddWrite(writePart) {
			return false
		}
		_, added := assembly.AddRuleFromDraft(occurrence, draft)
		return added
	})
	return engine.BindingRuleRowRef{}, queued
}

// AttachMountedReceiptMember resolves and attaches one exact Selected/Opaque
// graph member using the preissued typed callsite operand.
func (rule *HotRule) AttachMountedReceiptMember(compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, mountID, reusablePointID, occurrenceID identity.ContentID) (*engine.ReceiptMember, bool) {
	if rule == nil || compilation == nil || graph == nil || rule.implementation == nil {
		return nil, false
	}
	capability, capabilityOK := rule.implementation.MountedCapability()
	if !capabilityOK {
		return nil, false
	}
	member, ok := graph.MountedRuleMember(capability, mountID, reusablePointID, occurrenceID)
	if !ok {
		return nil, false
	}
	issuer, ok := rule.ForMount(mountID)
	if !ok {
		return nil, false
	}
	operand, ok := issuer.ReceiptForOccurrence(occurrenceID)
	if !ok {
		return nil, false
	}
	implementation, ok := effectowner.ResolveRuleImplementationFor(rule.effects, rule.implementation)
	if !ok {
		return nil, false
	}
	return engine.AttachReceiptRuleMember(compilation, implementation, member, operand)
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

// BodyReceiptAttachFailure is the closed admission boundary for one mounted
// Body Effect row. It is diagnostic-only: callers still receive no partial
// member when attachment rejects an owner or source capability.
type BodyReceiptAttachFailure uint8

const (
	BodyReceiptAttachFailureNone BodyReceiptAttachFailure = iota
	BodyReceiptAttachFailureArguments
	BodyReceiptAttachFailureIssuer
	BodyReceiptAttachFailureOperand
	BodyReceiptAttachFailureOccurrence
	BodyReceiptAttachFailureImplementation
	BodyReceiptAttachFailureTransaction
	BodyReceiptAttachFailureCallRef
	BodyReceiptAttachFailureEffectRef
	BodyReceiptAttachFailureCallSurface
	BodyReceiptAttachFailureSelectedAdmissionReceipt
	BodyReceiptAttachFailureExactRead
	BodyReceiptAttachFailureSelectedArguments
	BodyReceiptAttachFailureSelectedReceipt
	BodyReceiptAttachFailureSelectedOwner
	BodyReceiptAttachFailureSelectedSemantic
	BodyReceiptAttachFailureSelectedDependencies
	BodyReceiptAttachFailureSelectedDependencySurface
	BodyReceiptAttachFailureSelectedFactor
	BodyReceiptAttachFailureSelectedDuplicate
	BodyReceiptAttachFailureSelectedClaim
	BodyReceiptAttachFailureExactWrite
	BodyReceiptAttachFailureFinalizer
)

func (failure BodyReceiptAttachFailure) String() string {
	switch failure {
	case BodyReceiptAttachFailureNone:
		return "none"
	case BodyReceiptAttachFailureArguments:
		return "arguments"
	case BodyReceiptAttachFailureIssuer:
		return "issuer"
	case BodyReceiptAttachFailureOperand:
		return "operand"
	case BodyReceiptAttachFailureOccurrence:
		return "occurrence"
	case BodyReceiptAttachFailureImplementation:
		return "implementation"
	case BodyReceiptAttachFailureTransaction:
		return "transaction"
	case BodyReceiptAttachFailureCallRef:
		return "call-ref"
	case BodyReceiptAttachFailureEffectRef:
		return "effect-ref"
	case BodyReceiptAttachFailureCallSurface:
		return "call-surface"
	case BodyReceiptAttachFailureSelectedAdmissionReceipt:
		return "selected-admission-receipt"
	case BodyReceiptAttachFailureExactRead:
		return "exact-read"
	case BodyReceiptAttachFailureSelectedArguments:
		return "selected-arguments"
	case BodyReceiptAttachFailureSelectedReceipt:
		return "selected-receipt"
	case BodyReceiptAttachFailureSelectedOwner:
		return "selected-owner"
	case BodyReceiptAttachFailureSelectedSemantic:
		return "selected-semantic"
	case BodyReceiptAttachFailureSelectedDependencies:
		return "selected-dependencies"
	case BodyReceiptAttachFailureSelectedDependencySurface:
		return "selected-dependency-surface"
	case BodyReceiptAttachFailureSelectedFactor:
		return "selected-factor"
	case BodyReceiptAttachFailureSelectedDuplicate:
		return "selected-duplicate"
	case BodyReceiptAttachFailureSelectedClaim:
		return "selected-claim"
	case BodyReceiptAttachFailureExactWrite:
		return "exact-write"
	case BodyReceiptAttachFailureFinalizer:
		return "finalizer"
	default:
		return "unknown"
	}
}

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

func (rule *BodyHotRule) AttachMountedOccurrence(assembly *engine.ReceiptAssembly, mountID, reusablePointID, occurrenceID identity.ContentID) (engine.BindingRuleRowRef, bool) {
	row, failure := rule.AttachMountedOccurrenceWithFailure(assembly, mountID, reusablePointID, occurrenceID)
	return row, failure == BodyReceiptAttachFailureNone
}

// AttachMountedOccurrenceWithFailure admits one mount-qualified Body row and
// returns its closed failed capability phase when the exact receipt cannot be
// attached.
func (rule *BodyHotRule) AttachMountedOccurrenceWithFailure(assembly *engine.ReceiptAssembly, mountID, reusablePointID, occurrenceID identity.ContentID) (engine.BindingRuleRowRef, BodyReceiptAttachFailure) {
	if rule == nil || rule.implementation == nil || rule.calls == nil || rule.effects == nil || assembly == nil {
		return engine.BindingRuleRowRef{}, BodyReceiptAttachFailureArguments
	}
	issuer, ok := rule.ForMount(mountID)
	if !ok {
		return engine.BindingRuleRowRef{}, BodyReceiptAttachFailureIssuer
	}
	operand, ok := issuer.ReceiptForOccurrence(occurrenceID)
	if !ok {
		return engine.BindingRuleRowRef{}, BodyReceiptAttachFailureOperand
	}
	capability, capabilityOK := rule.implementation.MountedCapability()
	if !capabilityOK {
		return engine.BindingRuleRowRef{}, BodyReceiptAttachFailureOccurrence
	}
	occurrence, ok := assembly.AdmitMountedRuleOccurrence(capability, mountID, reusablePointID, occurrenceID)
	if !ok {
		return engine.BindingRuleRowRef{}, BodyReceiptAttachFailureOccurrence
	}
	implementation, implementationOK := effectowner.ResolveRuleImplementationFor(rule.effects, rule.implementation)
	if !implementationOK {
		return engine.BindingRuleRowRef{}, BodyReceiptAttachFailureImplementation
	}
	transaction, ok := engine.BeginMountedRuleAdmission(assembly, implementation, occurrence, operand)
	if !ok {
		return engine.BindingRuleRowRef{}, BodyReceiptAttachFailureTransaction
	}
	callRef, callOK := rule.calls.Ref(operand.key)
	effectRef, effectOK := rule.effects.Ref(operand.root)
	callSurface, callSurfaceOK := engine.ExactReadSurface(callRef)
	// Body's first cold read is the exact Call predecessor; its dependent
	// selected Effect summary is the second read declared by BodySchema.
	selectedReceipt, selectedOK := implementation.SelectedReadReceipt(1)
	if !callOK {
		return engine.BindingRuleRowRef{}, BodyReceiptAttachFailureCallRef
	}
	if !effectOK {
		return engine.BindingRuleRowRef{}, BodyReceiptAttachFailureEffectRef
	}
	if !callSurfaceOK {
		return engine.BindingRuleRowRef{}, BodyReceiptAttachFailureCallSurface
	}
	if !selectedOK {
		return engine.BindingRuleRowRef{}, BodyReceiptAttachFailureSelectedReceipt
	}
	if !engine.AddExactRead(transaction, callRef) {
		return engine.BindingRuleRowRef{}, BodyReceiptAttachFailureExactRead
	}
	selectedSurface, selectedFailure := transaction.AnchoredSelectedReadSurfaceWithFailure(selectedReceipt, []engine.RuleReadSurface{callSurface})
	if selectedFailure != engine.AnchoredSelectedReadFailureNone {
		switch selectedFailure {
		case engine.AnchoredSelectedReadFailureArguments:
			return engine.BindingRuleRowRef{}, BodyReceiptAttachFailureSelectedArguments
		case engine.AnchoredSelectedReadFailureReceipt:
			return engine.BindingRuleRowRef{}, BodyReceiptAttachFailureSelectedAdmissionReceipt
		case engine.AnchoredSelectedReadFailureOwner:
			return engine.BindingRuleRowRef{}, BodyReceiptAttachFailureSelectedOwner
		case engine.AnchoredSelectedReadFailureSemantic:
			return engine.BindingRuleRowRef{}, BodyReceiptAttachFailureSelectedSemantic
		case engine.AnchoredSelectedReadFailureDependencies:
			return engine.BindingRuleRowRef{}, BodyReceiptAttachFailureSelectedDependencies
		case engine.AnchoredSelectedReadFailureDependencySurface:
			return engine.BindingRuleRowRef{}, BodyReceiptAttachFailureSelectedDependencySurface
		case engine.AnchoredSelectedReadFailureFactor:
			return engine.BindingRuleRowRef{}, BodyReceiptAttachFailureSelectedFactor
		case engine.AnchoredSelectedReadFailureDuplicate:
			return engine.BindingRuleRowRef{}, BodyReceiptAttachFailureSelectedDuplicate
		case engine.AnchoredSelectedReadFailureClaim:
			return engine.BindingRuleRowRef{}, BodyReceiptAttachFailureSelectedClaim
		default:
			return engine.BindingRuleRowRef{}, BodyReceiptAttachFailureSelectedArguments
		}
	}
	if !transaction.AddRead(selectedSurface) {
		return engine.BindingRuleRowRef{}, BodyReceiptAttachFailureSelectedArguments
	}
	if !engine.AddExactWrite(transaction, effectRef) {
		return engine.BindingRuleRowRef{}, BodyReceiptAttachFailureExactWrite
	}
	queued := assembly.QueueMountedRuleFinalizer(capability, func() bool {
		sourceReceipt, sourceFailure := transaction.SealWithFailure()
		if sourceFailure != engine.RuleSourceSealFailureNone {
			switch sourceFailure {
			case engine.RuleSourceSealFailurePrecondition:
				rule.recordFinalizationFailure(BodyReceiptFinalizationFailureSourcePrecondition)
			case engine.RuleSourceSealFailureColdShape:
				rule.recordFinalizationFailure(BodyReceiptFinalizationFailureSourceColdShape)
			case engine.RuleSourceSealFailureIssueArguments:
				rule.recordFinalizationFailure(BodyReceiptFinalizationFailureSourceIssueArguments)
			case engine.RuleSourceSealFailureIssueTopology:
				rule.recordFinalizationFailure(BodyReceiptFinalizationFailureSourceIssueTopology)
			case engine.RuleSourceSealFailureIssueRule:
				rule.recordFinalizationFailure(BodyReceiptFinalizationFailureSourceIssueRule)
			case engine.RuleSourceSealFailureIssueShape:
				rule.recordFinalizationFailure(BodyReceiptFinalizationFailureSourceIssueShape)
			case engine.RuleSourceSealFailureIssueReadAuthority:
				rule.recordFinalizationFailure(BodyReceiptFinalizationFailureSourceIssueReadAuthority)
			case engine.RuleSourceSealFailureIssueReadSurface:
				rule.recordFinalizationFailure(BodyReceiptFinalizationFailureSourceIssueReadSurface)
			case engine.RuleSourceSealFailureIssueReadFactor:
				rule.recordFinalizationFailure(BodyReceiptFinalizationFailureSourceIssueReadFactor)
			case engine.RuleSourceSealFailureIssueReadAnchor:
				rule.recordFinalizationFailure(BodyReceiptFinalizationFailureSourceIssueReadAnchor)
			case engine.RuleSourceSealFailureIssueWrite:
				rule.recordFinalizationFailure(BodyReceiptFinalizationFailureSourceIssueWrite)
			case engine.RuleSourceSealFailureIssueBatch:
				rule.recordFinalizationFailure(BodyReceiptFinalizationFailureSourceIssueBatch)
			case engine.RuleSourceSealFailureSummary:
				rule.recordFinalizationFailure(BodyReceiptFinalizationFailureSourceSummary)
			}
			return false
		}
		draft, draftOK := implementation.BeginReceiptRuleRow(sourceReceipt)
		firstRead, firstReadOK := implementation.ReceiptReadPart(sourceReceipt, 0)
		secondRead, secondReadOK := implementation.ReceiptReadPart(sourceReceipt, 1)
		writePart, writePartOK := implementation.ReceiptWritePart(sourceReceipt, 0)
		if !draftOK {
			rule.recordFinalizationFailure(BodyReceiptFinalizationFailureDraft)
			return false
		}
		if !firstReadOK || !secondReadOK {
			rule.recordFinalizationFailure(BodyReceiptFinalizationFailureReadPart)
			return false
		}
		if !writePartOK {
			rule.recordFinalizationFailure(BodyReceiptFinalizationFailureWritePart)
			return false
		}
		if !draft.AddRead(firstRead) || !draft.AddRead(secondRead) {
			rule.recordFinalizationFailure(BodyReceiptFinalizationFailureDraftRead)
			return false
		}
		if !draft.AddWrite(writePart) {
			rule.recordFinalizationFailure(BodyReceiptFinalizationFailureDraftWrite)
			return false
		}
		_, added := assembly.AddRuleFromDraft(occurrence, draft)
		if !added {
			rule.recordFinalizationFailure(BodyReceiptFinalizationFailureGraph)
		}
		return added
	})
	if !queued {
		return engine.BindingRuleRowRef{}, BodyReceiptAttachFailureFinalizer
	}
	return engine.BindingRuleRowRef{}, BodyReceiptAttachFailureNone
}

// AttachMountedReceiptMember resolves and attaches one exact Body graph
// member using its preissued body-call operand.
func (rule *BodyHotRule) AttachMountedReceiptMember(compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, mountID, reusablePointID, occurrenceID identity.ContentID) (*engine.ReceiptMember, bool) {
	if rule == nil || compilation == nil || graph == nil || rule.implementation == nil {
		return nil, false
	}
	capability, capabilityOK := rule.implementation.MountedCapability()
	if !capabilityOK {
		return nil, false
	}
	member, ok := graph.MountedRuleMember(capability, mountID, reusablePointID, occurrenceID)
	if !ok {
		return nil, false
	}
	issuer, ok := rule.ForMount(mountID)
	if !ok {
		return nil, false
	}
	operand, ok := issuer.ReceiptForOccurrence(occurrenceID)
	if !ok {
		return nil, false
	}
	implementation, ok := effectowner.ResolveRuleImplementationFor(rule.effects, rule.implementation)
	if !ok {
		return nil, false
	}
	return engine.AttachReceiptRuleMember(compilation, implementation, member, operand)
}
