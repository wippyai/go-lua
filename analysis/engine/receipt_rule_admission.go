package engine

import (
	"crypto/sha256"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/identity"
)

// RuleOccurrenceReceipt and RuleOperandReceipt are sealed source-batch
// capabilities. Their equation identities remain private to engine; a caller
// can only obtain them from the exact mounted artifact inverse below.
type RuleOccurrenceReceipt struct {
	assembly    *ReceiptAssembly
	role        RuleSlotCapability
	linkRole    RuleSlotCapability
	value       equation.Occurrence
	member      identity.ContentID
	activation  identity.ContentID
	mount       identity.ContentID
	reusable    identity.ContentID
	input       equation.Site
	inputID     identity.ContentID
	stage       rows.ArtifactRuleStage
	predecessor *artifactEnvironmentRow
	routed      bool
}

type RuleOperandReceipt struct {
	assembly   *ReceiptAssembly
	occurrence RuleOccurrenceReceipt
	value      equation.Operand
}

// RuleSurfaceSourceReceipt is the public, opaque source consumed by typed
// RuleImplementation receipt methods. No internal equation type is exposed.
type RuleSurfaceSourceReceipt struct {
	value    equation.RuleSurfaceSourceReceipt
	assembly *ReceiptAssembly
}

// AdmitMountedRuleOccurrence resolves one exact mounted point and occurrence
// identity. The member identity is mount+point+occurrence qualified, so equal
// reusable artifacts and same IDs on different mounts cannot alias.
func (assembly *ReceiptAssembly) AdmitMountedRuleOccurrence(role RuleSlotCapability, mountID, reusablePointID, occurrenceID identity.ContentID) (RuleOccurrenceReceipt, bool) {
	if assembly == nil || assembly.builder == nil || !role.mounted() || !mountID.Available() || !reusablePointID.Available() || !occurrenceID.Available() {
		return RuleOccurrenceReceipt{}, false
	}
	inner, ok := assembly.builder.lockSourcesOpen()
	if !ok {
		return RuleOccurrenceReceipt{}, false
	}
	artifactRows := inner.artifact
	var site equation.Site
	var inputSite equation.Site
	var inputID identity.ContentID
	var stage rows.ArtifactRuleStage
	var predecessor *artifactEnvironmentRow
	var routed bool
	found := false
	if artifactRows != nil {
		site, found = artifactRows.mountedSite(mountID, reusablePointID)
		if found {
			var input artifactRuleInput
			input, found = artifactRows.mountedRule(role, mountID, reusablePointID, occurrenceID)
			inputID, stage, routed = input.point, input.stage, input.routed
			if routed {
				row := input.predecessor
				predecessor = &row
			}
		}
		if found && inputID.Available() {
			inputSite, found = artifactRows.mountedSite(mountID, inputID)
		}
	}
	inner.mu.Unlock()
	if !found {
		return RuleOccurrenceReceipt{}, false
	}
	entity, entityOK := mountedRuleOccurrenceKey(role, occurrenceID)
	occurrence, occurrenceOK := assembly.builder.admitFrom(site, entity)
	member := mountedRuleMemberID(role, mountID, reusablePointID, occurrenceID)
	activation := mountedRuleActivationID(role, mountID, reusablePointID, occurrenceID)
	result := RuleOccurrenceReceipt{assembly: assembly, role: role, value: occurrence, member: member, activation: activation, mount: mountID, reusable: reusablePointID, input: inputSite, inputID: inputID, stage: stage, predecessor: predecessor, routed: routed}
	return result, stage.Valid() && entityOK && occurrenceOK && member.Available() && activation.Available()
}

// AdmitLinkRuleOccurrence resolves only the two Link-global bootstrap roles
// from the sealed witness catalog. It has no mount argument and cannot admit
// an arbitrary site or occurrence ID.
func (assembly *ReceiptAssembly) AdmitLinkRuleOccurrence(role RuleSlotCapability, occurrenceID identity.ContentID) (RuleOccurrenceReceipt, bool) {
	if assembly == nil || assembly.builder == nil || !role.link() || !occurrenceID.Available() || assembly.builder.inner == nil {
		return RuleOccurrenceReceipt{}, false
	}
	inner, ok := assembly.builder.lockSourcesOpen()
	if !ok {
		return RuleOccurrenceReceipt{}, false
	}
	bootstrap := inner.artifact
	// The bootstrap Site is deliberately admitted into the still-open source
	// Batch. Site.Available requires a sealed Batch and therefore cannot be
	// used at this pre-seal admission boundary; admitFrom below authenticates
	// the open-batch capability and preserves the same fence as mounted rows.
	if bootstrap == nil || bootstrap.bootstrap == nil {
		inner.mu.Unlock()
		return RuleOccurrenceReceipt{}, false
	}
	if assignedRole, found := bootstrap.bootstrap.roles[occurrenceID]; !found || assignedRole != role {
		inner.mu.Unlock()
		return RuleOccurrenceReceipt{}, false
	}
	if _, found := bootstrap.bootstrap.claims[occurrenceID]; found {
		inner.mu.Unlock()
		return RuleOccurrenceReceipt{}, false
	}
	bootstrap.bootstrap.claims[occurrenceID] = role
	site := bootstrap.bootstrap.site
	inner.mu.Unlock()
	entity, entityOK := linkRuleOccurrenceKey(role, occurrenceID)
	occurrence, occurrenceOK := assembly.builder.admitFrom(site, entity)
	member := linkRuleMemberID(role, bootstrap.bootstrap.owner, bootstrap.bootstrap.point.PointID, occurrenceID)
	result := RuleOccurrenceReceipt{assembly: assembly, linkRole: role, value: occurrence, member: member}
	return result, entityOK && occurrenceOK && member.Available()
}

// AdmitMountedRuleOperand binds one typed issuer's canonical content digest
// to its already-authenticated mounted occurrence. It cannot accept a raw
// occurrence from another assembly or a caller-selected equation identity.
func (assembly *ReceiptAssembly) AdmitMountedRuleOperand(occurrence RuleOccurrenceReceipt, digest [32]byte) (RuleOperandReceipt, bool) {
	// Rule operands are admitted before source sealing. Occurrence.Available
	// intentionally requires a sealed Batch; SameOpen is the corresponding
	// capability fence for this open-phase transaction.
	if assembly == nil || assembly.builder == nil || occurrence.assembly != assembly || !occurrence.value.SameOpen(occurrence.value) {
		return RuleOperandReceipt{}, false
	}
	entity, entityOK := operandEntityForContent(digest)
	operand, operandOK := assembly.builder.admitOperand(occurrence.value, entity)
	return RuleOperandReceipt{assembly: assembly, occurrence: occurrence, value: operand}, entityOK && operandOK
}

// BeginMountedRuleOccurrence is the typed owner bridge: the caller supplies
// the domain operand O, while the exact sealed implementation supplies its
// own canonicalization law. It only admits the operand; surface geometry is
// deliberately a later operation using owner-issued Ref receipts.
func BeginMountedRuleOccurrence[K ~uint32 | ~uint64, V, O any](assembly *ReceiptAssembly, implementation *RuleImplementation[K, V, O], occurrence RuleOccurrenceReceipt, operand O) (RuleOperandReceipt, bool) {
	if assembly == nil || implementation == nil || occurrence.assembly != assembly || !implementation.receipt.valid() || implementation.receipt.cell == nil || implementation.receipt.cell.impl == nil || !occurrenceRoleOwnsSchema(occurrence, implementation.receipt.proof.schema, implementation.receipt.proof.semantic) {
		return RuleOperandReceipt{}, false
	}
	_, digest, contentOK := implementation.receipt.cell.impl.operandContent(operand)
	if !contentOK {
		return RuleOperandReceipt{}, false
	}
	operandReceipt, operandOK := assembly.AdmitMountedRuleOperand(occurrence, digest)
	if !operandOK {
		return RuleOperandReceipt{}, false
	}
	return operandReceipt, true
}

// BeginMountedActivationRuleAdmission is the structural sibling for an
// ActivationRuleImplementation. Activation rows have no typed operand
// callback; their sealed artifact issuer therefore supplies the canonical
// operand digest directly.
func BeginMountedActivationRuleAdmission(assembly *ReceiptAssembly, implementation *ActivationRuleImplementation, occurrence RuleOccurrenceReceipt, digest [32]byte) (*RuleSourceTransaction, bool) {
	if assembly == nil || implementation == nil || occurrence.assembly != assembly || !implementation.receipt.valid() || !occurrenceRoleOwnsSchema(occurrence, implementation.receipt.proof.schema, implementation.receipt.proof.semantic) {
		return nil, false
	}
	operand, ok := assembly.AdmitMountedRuleOperand(occurrence, digest)
	if !ok {
		return nil, false
	}
	return &RuleSourceTransaction{
		assembly:   assembly,
		semantic:   semanticKeyFromComposition(implementation.receipt.proof.semantic),
		family:     semanticKeyFromComposition(implementation.receipt.proof.operandFamily),
		occurrence: occurrence,
		operand:    operand,
	}, true
}

// AddRule admits the row under the exact mounted occurrence authority that
// issued its source. Callers cannot forge or reuse a semantic member ID.
func (assembly *ReceiptAssembly) AddRule(occurrence RuleOccurrenceReceipt, receipt BindingRuleRowReceipt) (BindingRuleRowRef, bool) {
	mounted, link := occurrence.role.mounted(), occurrence.linkRole.link()
	if assembly == nil || assembly.builder == nil || occurrence.assembly != assembly || !occurrence.member.Available() || mounted == link || mounted && (!occurrence.activation.Available() || !occurrence.stage.Valid()) {
		assembly.recordRuleFinalizerFailure(RuleFinalizerFailureAddRuleArguments)
		return BindingRuleRowRef{}, false
	}
	inner := assembly.builder.inner
	if inner == nil || receipt.builder != inner || receipt.state != inner.state || receipt.authority != inner.authority || !receipt.row.Occurrence.Same(occurrence.value) || !receipt.row.Operand.Occurrence().Same(occurrence.value) || !occurrenceRoleOwnsSchema(occurrence, inner.state.schema, receipt.row.Schema) {
		assembly.recordRuleFinalizerFailure(RuleFinalizerFailureAddRuleFence)
		return BindingRuleRowRef{}, false
	}
	receipt.input, receipt.inputID, receipt.stage, receipt.predecessor, receipt.routed = occurrence.input, occurrence.inputID, occurrence.stage, occurrence.predecessor, occurrence.routed
	ref, ok := assembly.builder.addSemanticRule(occurrence.member, receipt)
	if !ok {
		assembly.recordRuleFinalizerFailure(RuleFinalizerFailureAddSemanticRule)
	}
	return ref, ok
}

func occurrenceRoleOwnsSchema(occurrence RuleOccurrenceReceipt, schema *Schema, semantic composition.Key) bool {
	if schema == nil || !semantic.Available() {
		return false
	}
	if _, ok := schema.ruleOrdinalOf(semantic); !ok || occurrence.assembly == nil || occurrence.assembly.binding == nil || occurrence.assembly.binding.state == nil {
		return false
	}
	state := occurrence.assembly.binding.state
	if state.schema != schema {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if occurrence.role.mounted() {
		return state.phase == schemaBindingSealed && state.roleSlots[occurrence.role] == semantic
	}
	return state.phase == schemaBindingSealed && occurrence.linkRole.link() && state.roleSlots[occurrence.linkRole] == semantic
}

// AddRuleFromDraft is the final opaque row bridge for domain wrappers. The
// draft is issued by a typed implementation and is checked against the exact
// mounted occurrence before the private builder mints its sealed row receipt.
func (assembly *ReceiptAssembly) AddRuleFromDraft(occurrence RuleOccurrenceReceipt, draft *BindingRuleRowDraft) (BindingRuleRowRef, bool) {
	if assembly == nil || assembly.builder == nil || assembly.builder.inner == nil || occurrence.assembly != assembly || draft == nil || draft.state != assembly.builder.inner.state || draft.authority != assembly.builder.inner.authority || !draft.source.Occurrence().Same(occurrence.value) || !draft.source.Operand().Occurrence().Same(occurrence.value) {
		assembly.recordRuleFinalizerFailure(RuleFinalizerFailureAddRuleDraft)
		return BindingRuleRowRef{}, false
	}
	receipt, ok := assembly.builder.issueRuleRow(draft)
	if !ok {
		assembly.recordRuleFinalizerFailure(RuleFinalizerFailureIssueRuleRow)
		return BindingRuleRowRef{}, false
	}
	return assembly.AddRule(occurrence, receipt)
}

func (assembly *ReceiptAssembly) AddActivationRule(occurrence RuleOccurrenceReceipt, receipt BindingRuleRowReceipt) bool {
	if assembly == nil || assembly.builder == nil || occurrence.assembly != assembly || !occurrence.role.mounted() || !occurrence.stage.Valid() || !occurrence.member.Available() {
		assembly.recordRuleFinalizerFailure(RuleFinalizerFailureAddRuleArguments)
		return false
	}
	inner := assembly.builder.inner
	if inner == nil || receipt.builder != inner || receipt.state != inner.state || receipt.authority != inner.authority || !receipt.row.Occurrence.Same(occurrence.value) || !receipt.row.Operand.Occurrence().Same(occurrence.value) || !occurrenceRoleOwnsSchema(occurrence, inner.state.schema, receipt.row.Schema) {
		assembly.recordRuleFinalizerFailure(RuleFinalizerFailureAddRuleFence)
		return false
	}
	receipt.input, receipt.inputID, receipt.stage, receipt.predecessor, receipt.routed = occurrence.input, occurrence.inputID, occurrence.stage, occurrence.predecessor, occurrence.routed
	ref, ok := assembly.builder.addSemanticRule(occurrence.member, receipt)
	if !ok {
		assembly.recordRuleFinalizerFailure(RuleFinalizerFailureAddSemanticRule)
		return false
	}
	if !assembly.builder.addSemanticActivation(occurrence.activation, ref) {
		assembly.recordRuleFinalizerFailure(RuleFinalizerFailureAddSemanticActivation)
		return false
	}
	return true
}

func (assembly *ReceiptAssembly) AddActivationRuleFromDraft(occurrence RuleOccurrenceReceipt, draft *BindingRuleRowDraft) bool {
	if assembly == nil || assembly.builder == nil || assembly.builder.inner == nil || occurrence.assembly != assembly || draft == nil || draft.state != assembly.builder.inner.state || draft.authority != assembly.builder.inner.authority || !draft.source.Occurrence().Same(occurrence.value) || !draft.source.Operand().Occurrence().Same(occurrence.value) {
		assembly.recordRuleFinalizerFailure(RuleFinalizerFailureAddRuleDraft)
		return false
	}
	receipt, ok := assembly.builder.issueRuleRow(draft)
	if !ok {
		assembly.recordRuleFinalizerFailure(RuleFinalizerFailureIssueRuleRow)
		return false
	}
	return assembly.AddActivationRule(occurrence, receipt)
}

// AnchoredSelectedReadSurface issues the ReadSelect surface from the exact
// admitted occurrence and operand. ReadSelect has no exact target unit: its
// local is a sealed identity of this occurrence/operand/read proof, not a
// caller-selected Ref.
type AnchoredSelectedReadFailure uint8

const (
	AnchoredSelectedReadFailureNone AnchoredSelectedReadFailure = iota
	AnchoredSelectedReadFailureArguments
	AnchoredSelectedReadFailureReceipt
	AnchoredSelectedReadFailureOwner
	AnchoredSelectedReadFailureSemantic
	AnchoredSelectedReadFailureDependencies
	AnchoredSelectedReadFailureDependencySurface
	AnchoredSelectedReadFailureFactor
	AnchoredSelectedReadFailureDuplicate
	AnchoredSelectedReadFailureClaim
)

func (transaction *RuleSourceTransaction) AnchoredSelectedReadSurface(receipt SchemaSelectedReadReceipt, dependencies []RuleReadSurface) (RuleReadSurface, bool) {
	surface, failure := transaction.AnchoredSelectedReadSurfaceWithFailure(receipt, dependencies)
	return surface, failure == AnchoredSelectedReadFailureNone
}

func (transaction *RuleSourceTransaction) AnchoredSelectedReadSurfaceWithFailure(receipt SchemaSelectedReadReceipt, dependencies []RuleReadSurface) (RuleReadSurface, AnchoredSelectedReadFailure) {
	if transaction == nil || transaction.assembly == nil || transaction.assembly.builder == nil || transaction.assembly.builder.inner == nil || transaction.operand.assembly != transaction.assembly || transaction.occurrence.assembly != transaction.assembly {
		return RuleReadSurface{}, AnchoredSelectedReadFailureArguments
	}
	if !receipt.Valid() || receipt.fence.authority == nil {
		return RuleReadSurface{}, AnchoredSelectedReadFailureReceipt
	}
	if receipt.fence.authority != transaction.assembly.builder.inner.authority || receipt.fence.schema != transaction.assembly.builder.inner.state.schema {
		return RuleReadSurface{}, AnchoredSelectedReadFailureOwner
	}
	if semanticKeyFromComposition(receipt.fence.schema.ruleSemanticAt(receipt.fence.rule)) != transaction.semantic {
		return RuleReadSurface{}, AnchoredSelectedReadFailureSemantic
	}
	if len(dependencies) != int(receipt.dependencyCount) {
		return RuleReadSurface{}, AnchoredSelectedReadFailureDependencies
	}
	factor := receipt.fence.schema.factorSemanticAt(receipt.factor)
	if !factor.Available() {
		return RuleReadSurface{}, AnchoredSelectedReadFailureFactor
	}
	for index, dependency := range dependencies {
		readIndex, ok := receipt.fence.schema.ruleReadDependencyAt(receipt.fence.rule, receipt.read, uint64(index))
		shape, shapeOK := receipt.fence.schema.ruleReadShapeAt(receipt.fence.rule, readIndex)
		if !ok || !shapeOK || dependency.authority != receipt.fence.authority || dependency.value.Mode != equation.TargetModeNone || dependency.value.Factor != shape.Factor || dependency.value.Local == 0 || !validSelectedDependencySurface(shape, dependency.value) {
			return RuleReadSurface{}, AnchoredSelectedReadFailureDependencySurface
		}
	}
	local := anchoredSelectedLocal(transaction.occurrence.value, transaction.operand.value, receipt)
	for _, existing := range transaction.reads {
		if existing.value.Factor == factor && existing.value.Form == equation.SurfaceReadSelect && existing.value.Local == local {
			return RuleReadSurface{}, AnchoredSelectedReadFailureDuplicate
		}
	}
	surface := equation.Surface{Factor: factor, Form: equation.SurfaceReadSelect, Local: local, Semantic: factor}
	anchor := mountedSelectedSurfaceAnchor{assembly: transaction.assembly, occurrence: transaction.occurrence.value, operand: transaction.operand.value, rule: receipt.fence.rule, index: receipt.read, form: equation.SurfaceReadSelect}
	if !transaction.assembly.claimMountedSelectedSurface(surface, anchor) {
		return RuleReadSurface{}, AnchoredSelectedReadFailureClaim
	}
	return RuleReadSurface{value: surface, authority: receipt.fence.authority, anchor: &anchor}, AnchoredSelectedReadFailureNone
}

// AnchoredRouteWriteSurface is the route sibling: the output has no single
// exact Ref because runtime chooses zero-or-many selected targets. Its local
// is tied to the admitted occurrence/operand and sealed route proof.
func (transaction *RuleSourceTransaction) AnchoredRouteWriteSurface(receipt SchemaRouteWriteReceipt) (RuleWriteSurface, bool) {
	if transaction == nil || transaction.assembly == nil || transaction.assembly.builder == nil || transaction.assembly.builder.inner == nil || transaction.operand.assembly != transaction.assembly || transaction.occurrence.assembly != transaction.assembly || !receipt.Valid() || receipt.fence.authority == nil || receipt.fence.authority != transaction.assembly.builder.inner.authority || receipt.fence.schema != transaction.assembly.builder.inner.state.schema || semanticKeyFromComposition(receipt.fence.schema.ruleSemanticAt(receipt.fence.rule)) != transaction.semantic {
		return RuleWriteSurface{}, false
	}
	factor := receipt.fence.schema.factorSemanticAt(receipt.factor)
	if !factor.Available() {
		return RuleWriteSurface{}, false
	}
	local := anchoredRouteLocal(transaction.occurrence.value, transaction.operand.value, receipt)
	surface := equation.Surface{Factor: factor, Form: equation.SurfaceWriteRoute, Local: local}
	anchor := mountedSelectedSurfaceAnchor{assembly: transaction.assembly, occurrence: transaction.occurrence.value, operand: transaction.operand.value, rule: receipt.fence.rule, index: receipt.write, form: equation.SurfaceWriteRoute}
	if !transaction.assembly.claimMountedSurface(surface, anchor) {
		return RuleWriteSurface{}, false
	}
	return RuleWriteSurface{value: surface, authority: receipt.fence.authority, route: &receipt, anchor: &anchor}, true
}

// RuleSourceTransaction is the closed, occurrence-owned source admission
// envelope. It records only owner-issued geometry; no equation coordinates or
// cold factor ordinals can be supplied through this API.
type RuleSourceTransaction struct {
	assembly   *ReceiptAssembly
	semantic   identity.SemanticKey
	family     identity.SemanticKey
	occurrence RuleOccurrenceReceipt
	operand    RuleOperandReceipt
	reads      []RuleReadSurface
	writes     []RuleWriteSurface
	carries    uint64
	sealed     bool
}

// RuleSourceSealFailure is the closed source-seal phase for an already
// admitted mounted rule transaction. It carries no source or topology
// capability and leaves Seal's bool contract unchanged.
type RuleSourceSealFailure uint8

const (
	RuleSourceSealFailureNone RuleSourceSealFailure = iota
	RuleSourceSealFailurePrecondition
	RuleSourceSealFailureColdShape
	RuleSourceSealFailureIssueArguments
	RuleSourceSealFailureIssueTopology
	RuleSourceSealFailureIssueRule
	RuleSourceSealFailureIssueShape
	RuleSourceSealFailureIssueReadAuthority
	RuleSourceSealFailureIssueReadSurface
	RuleSourceSealFailureIssueReadFactor
	RuleSourceSealFailureIssueReadAnchor
	RuleSourceSealFailureIssueWrite
	RuleSourceSealFailureIssueBatch
	RuleSourceSealFailureSummary
)

// RuleSourceIssueFailure is the closed source-issuance predicate after a
// transaction has passed its cold shape check.
type RuleSourceIssueFailure uint8

const (
	RuleSourceIssueFailureNone RuleSourceIssueFailure = iota
	RuleSourceIssueFailureArguments
	RuleSourceIssueFailureTopology
	RuleSourceIssueFailureRule
	RuleSourceIssueFailureShape
	RuleSourceIssueFailureReadAuthority
	RuleSourceIssueFailureReadSurface
	RuleSourceIssueFailureReadFactor
	RuleSourceIssueFailureReadAnchor
	RuleSourceIssueFailureWrite
	RuleSourceIssueFailureBatch
)

// ownsSurfaceAnchor is the transaction fence for occurrence-derived
// ReadSelect/WriteRoute surfaces. The anchor is deliberately private: callers
// can only obtain one from this transaction, and copying it into another
// transaction must fail on the assembly, occurrence, operand, rule, and
// ordinal tuple.
func (transaction *RuleSourceTransaction) ownsSurfaceAnchor(anchor *mountedSelectedSurfaceAnchor, form equation.SurfaceForm, index uint64) bool {
	if transaction == nil || transaction.assembly == nil || transaction.assembly.builder == nil || transaction.assembly.builder.inner == nil || anchor == nil {
		return false
	}
	occurrenceMatches := anchor.occurrence.Same(transaction.occurrence.value)
	if !occurrenceMatches {
		occurrenceMatches = anchor.occurrence.SameOpen(transaction.occurrence.value)
	}
	operandMatches := anchor.operand.Same(transaction.operand.value)
	if !operandMatches {
		operandMatches = anchor.operand.SameOpen(transaction.operand.value)
	}
	if anchor.assembly != transaction.assembly || !occurrenceMatches || !operandMatches || anchor.form != form || anchor.index != index {
		return false
	}
	ordinal, ok := transaction.assembly.builder.inner.state.schema.ruleOrdinalOf(compositionKeyOf(transaction.semantic))
	return ok && anchor.rule == ordinal
}

// BeginMountedRuleAdmission combines typed operand canonicalization with the
// exact implementation's semantic/family proof, while leaving all surfaces
// to the domain owner.
func BeginMountedRuleAdmission[K ~uint32 | ~uint64, V, O any](assembly *ReceiptAssembly, implementation *RuleImplementation[K, V, O], occurrence RuleOccurrenceReceipt, operand O) (*RuleSourceTransaction, bool) {
	operandReceipt, ok := BeginMountedRuleOccurrence(assembly, implementation, occurrence, operand)
	if !ok || implementation == nil || implementation.receipt.proof == nil || !occurrenceRoleOwnsSchema(occurrence, implementation.receipt.proof.schema, implementation.receipt.proof.semantic) {
		return nil, false
	}
	return &RuleSourceTransaction{
		assembly:   assembly,
		semantic:   semanticKeyFromComposition(implementation.receipt.proof.semantic),
		family:     semanticKeyFromComposition(implementation.receipt.proof.operandFamily),
		occurrence: occurrence,
		operand:    operandReceipt,
	}, true
}

func (transaction *RuleSourceTransaction) AddRead(surface RuleReadSurface) bool {
	if transaction == nil || transaction.sealed || !surface.value.Available() {
		return false
	}
	if surface.anchor != nil && !transaction.ownsSurfaceAnchor(surface.anchor, surface.value.Form, uint64(len(transaction.reads))) {
		return false
	}
	transaction.reads = append(transaction.reads, surface)
	return true
}

// AddExactRead is the generic typed convenience wrapper. Go does not permit
// type parameters on methods, so domain packages call this closed function.
func AddExactRead[K ~uint32 | ~uint64](transaction *RuleSourceTransaction, ref Ref[K]) bool {
	surface, ok := ExactReadSurface(ref)
	return ok && transaction != nil && transaction.AddRead(surface)
}

func AddSummaryRead[K ~uint32 | ~uint64](transaction *RuleSourceTransaction, receipt SchemaSummaryReadReceipt, refs *ClosedRefs[K]) bool {
	surface, ok := SummaryReadSurface(receipt, refs)
	return ok && transaction != nil && transaction.AddRead(surface)
}

func AddSelectedRead[K ~uint32 | ~uint64](transaction *RuleSourceTransaction, receipt SchemaSelectedReadReceipt, ref Ref[K], dependencies []RuleReadSurface) bool {
	surface, ok := SelectedReadSurface(receipt, ref, dependencies)
	return ok && transaction != nil && transaction.AddRead(surface)
}

func AddAnchoredSelectedRead(transaction *RuleSourceTransaction, receipt SchemaSelectedReadReceipt, dependencies []RuleReadSurface) bool {
	surface, failure := transaction.AnchoredSelectedReadSurfaceWithFailure(receipt, dependencies)
	return failure == AnchoredSelectedReadFailureNone && transaction.AddRead(surface)
}

func (transaction *RuleSourceTransaction) AddCarry() bool {
	if transaction == nil || transaction.sealed {
		return false
	}
	transaction.carries++
	return true
}

func (transaction *RuleSourceTransaction) AddWrite(surface RuleWriteSurface) bool {
	if transaction == nil || transaction.sealed || !surface.value.Available() {
		return false
	}
	if surface.anchor != nil && !transaction.ownsSurfaceAnchor(surface.anchor, surface.value.Form, uint64(len(transaction.writes))) {
		return false
	}
	transaction.writes = append(transaction.writes, surface)
	return true
}

func AddExactWrite[K ~uint32 | ~uint64](transaction *RuleSourceTransaction, ref Ref[K]) bool {
	surface, ok := ExactWriteSurface(ref)
	return ok && transaction != nil && transaction.AddWrite(surface)
}

func AddSelectorWrite[K ~uint32 | ~uint64](transaction *RuleSourceTransaction, receipt SchemaSelectWriteReceipt, ref Ref[K], targets []Ref[K], relations []SelectorRelation) bool {
	surface, ok := SelectorWriteSurface(receipt, ref, targets, relations)
	return ok && transaction != nil && transaction.AddWrite(surface)
}

func AddRouteWrite[K ~uint32 | ~uint64](transaction *RuleSourceTransaction, receipt SchemaRouteWriteReceipt, ref Ref[K]) bool {
	surface, ok := RouteWriteSurface(receipt, ref)
	return ok && transaction != nil && transaction.AddWrite(surface)
}

func AddAnchoredRouteWrite(transaction *RuleSourceTransaction, receipt SchemaRouteWriteReceipt) bool {
	surface, ok := transaction.AnchoredRouteWriteSurface(receipt)
	return ok && transaction.AddWrite(surface)
}

func (transaction *RuleSourceTransaction) Seal() (RuleSurfaceSourceReceipt, bool) {
	source, failure := transaction.SealWithFailure()
	return source, failure == RuleSourceSealFailureNone
}

// SealWithFailure seals one exact mounted source and returns its closed
// rejected phase when the source cannot be issued.
func (transaction *RuleSourceTransaction) SealWithFailure() (sourceResult RuleSurfaceSourceReceipt, failureResult RuleSourceSealFailure) {
	defer func() {
		if transaction != nil && transaction.assembly != nil {
			transaction.assembly.recordRuleSourceSealFailure(failureResult)
		}
	}()
	if transaction == nil || transaction.sealed {
		return RuleSurfaceSourceReceipt{}, RuleSourceSealFailurePrecondition
	}
	transaction.sealed = true
	if transaction.assembly == nil || transaction.assembly.builder == nil {
		return RuleSurfaceSourceReceipt{}, RuleSourceSealFailurePrecondition
	}
	inner, ok := transaction.assembly.builder.lockTopologyOpen()
	if !ok {
		return RuleSurfaceSourceReceipt{}, RuleSourceSealFailurePrecondition
	}
	ordinal, found := inner.state.schema.cold.RuleIndex(compositionKeyOf(transaction.semantic))
	rule, rowOK := inner.state.schema.cold.RuleAt(ordinal)
	valid := found && rowOK && rule.OperandFamily == compositionKeyOf(transaction.family) && uint64(len(transaction.reads)) == uint64(len(rule.Reads)) && uint64(len(transaction.writes)) == uint64(len(rule.Writes)) && transaction.carries == uint64(len(rule.Carries))
	inner.mu.Unlock()
	if !valid {
		return RuleSurfaceSourceReceipt{}, RuleSourceSealFailureColdShape
	}
	source, issueFailure := transaction.assembly.IssueRuleSourceWithSurfacesWithFailure(transaction.semantic, transaction.family, transaction.occurrence, transaction.operand, transaction.reads, transaction.writes)
	if issueFailure != RuleSourceIssueFailureNone {
		switch issueFailure {
		case RuleSourceIssueFailureArguments:
			return RuleSurfaceSourceReceipt{}, RuleSourceSealFailureIssueArguments
		case RuleSourceIssueFailureTopology:
			return RuleSurfaceSourceReceipt{}, RuleSourceSealFailureIssueTopology
		case RuleSourceIssueFailureRule:
			return RuleSurfaceSourceReceipt{}, RuleSourceSealFailureIssueRule
		case RuleSourceIssueFailureShape:
			return RuleSurfaceSourceReceipt{}, RuleSourceSealFailureIssueShape
		case RuleSourceIssueFailureReadAuthority:
			return RuleSurfaceSourceReceipt{}, RuleSourceSealFailureIssueReadAuthority
		case RuleSourceIssueFailureReadSurface:
			return RuleSurfaceSourceReceipt{}, RuleSourceSealFailureIssueReadSurface
		case RuleSourceIssueFailureReadFactor:
			return RuleSurfaceSourceReceipt{}, RuleSourceSealFailureIssueReadFactor
		case RuleSourceIssueFailureReadAnchor:
			return RuleSurfaceSourceReceipt{}, RuleSourceSealFailureIssueReadAnchor
		case RuleSourceIssueFailureWrite:
			return RuleSurfaceSourceReceipt{}, RuleSourceSealFailureIssueWrite
		case RuleSourceIssueFailureBatch:
			return RuleSurfaceSourceReceipt{}, RuleSourceSealFailureIssueBatch
		default:
			return RuleSurfaceSourceReceipt{}, RuleSourceSealFailureIssueArguments
		}
	}
	for _, read := range transaction.reads {
		if read.summary == nil || read.summary.receipt == nil {
			continue
		}
		if !transaction.assembly.builder.addSummary(read.summary.receipt, equation.SummaryMapping{Surface: read.summary.surface, Keys: read.summary.keys}) {
			return RuleSurfaceSourceReceipt{}, RuleSourceSealFailureSummary
		}
	}
	return source, RuleSourceSealFailureNone
}

// IssueRuleSourceWithSurfaces finalizes a typed source only after every
// occurrence-specific read/write Ref has been supplied by the domain owner.
func (assembly *ReceiptAssembly) IssueRuleSourceWithSurfaces(semantic, operandFamily identity.SemanticKey, occurrence RuleOccurrenceReceipt, operand RuleOperandReceipt, reads []RuleReadSurface, writes []RuleWriteSurface) (RuleSurfaceSourceReceipt, bool) {
	source, failure := assembly.IssueRuleSourceWithSurfacesWithFailure(semantic, operandFamily, occurrence, operand, reads, writes)
	return source, failure == RuleSourceIssueFailureNone
}

// IssueRuleSourceWithSurfacesWithFailure issues a complete typed source and
// returns the closed rejected predicate without exposing equation internals.
func (assembly *ReceiptAssembly) IssueRuleSourceWithSurfacesWithFailure(semantic, operandFamily identity.SemanticKey, occurrence RuleOccurrenceReceipt, operand RuleOperandReceipt, reads []RuleReadSurface, writes []RuleWriteSurface) (RuleSurfaceSourceReceipt, RuleSourceIssueFailure) {
	if assembly == nil || assembly.builder == nil || !semantic.Available() || !operandFamily.Available() || occurrence.assembly != assembly || operand.assembly != assembly || operand.occurrence != occurrence || !occurrence.value.Available() || !operand.value.Available() {
		return RuleSurfaceSourceReceipt{}, RuleSourceIssueFailureArguments
	}
	inner, ok := assembly.builder.lockTopologyOpen()
	if !ok {
		return RuleSurfaceSourceReceipt{}, RuleSourceIssueFailureTopology
	}
	ruleKey := compositionKeyOf(semantic)
	familyKey := compositionKeyOf(operandFamily)
	ruleOrdinal, ruleOK := inner.state.schema.cold.RuleIndex(ruleKey)
	rule, rowOK := inner.state.schema.cold.RuleAt(ruleOrdinal)
	if !ruleOK || !rowOK || rule.OperandFamily != familyKey {
		inner.mu.Unlock()
		return RuleSurfaceSourceReceipt{}, RuleSourceIssueFailureRule
	}
	if len(reads) != len(rule.Reads) || len(writes) != len(rule.Writes) || len(rule.Supports) != 0 || len(rule.Prunes) != 0 {
		inner.mu.Unlock()
		return RuleSurfaceSourceReceipt{}, RuleSourceIssueFailureShape
	}
	resolvedReads := make([]equation.ResolvedRead, len(rule.Reads))
	anchorTransaction := RuleSourceTransaction{assembly: assembly, semantic: semantic, occurrence: occurrence, operand: operand}
	for index, read := range rule.Reads {
		if reads[index].authority != inner.authority {
			inner.mu.Unlock()
			return RuleSurfaceSourceReceipt{}, RuleSourceIssueFailureReadAuthority
		}
		if !reads[index].value.Available() {
			inner.mu.Unlock()
			return RuleSurfaceSourceReceipt{}, RuleSourceIssueFailureReadSurface
		}
		if reads[index].value.Factor != read.Factor {
			inner.mu.Unlock()
			return RuleSurfaceSourceReceipt{}, RuleSourceIssueFailureReadFactor
		}
		if reads[index].anchor != nil && !anchorTransaction.ownsSurfaceAnchor(reads[index].anchor, reads[index].value.Form, uint64(index)) {
			inner.mu.Unlock()
			return RuleSurfaceSourceReceipt{}, RuleSourceIssueFailureReadAnchor
		}
		resolvedReads[index] = equation.ResolvedRead{Index: uint64(index), Surface: reads[index].value}
	}
	carries := make([]equation.ResolvedCarry, len(rule.Carries))
	for index := range carries {
		carries[index] = equation.ResolvedCarry{Index: uint64(index)}
	}
	resolvedWrites := make([]equation.ResolvedWrite, len(rule.Writes))
	for index, write := range rule.Writes {
		if writes[index].authority != inner.authority || !writes[index].value.Available() || writes[index].value.Factor != write.Factor || writes[index].anchor != nil && !anchorTransaction.ownsSurfaceAnchor(writes[index].anchor, writes[index].value.Form, uint64(index)) {
			inner.mu.Unlock()
			return RuleSurfaceSourceReceipt{}, RuleSourceIssueFailureWrite
		}
		resolved := equation.ResolvedWrite{Index: uint64(index), Surface: writes[index].value}
		switch write.Kind {
		case composition.WriteExact:
			if writes[index].route != nil || writes[index].selector != nil || writes[index].value.Form != equation.SurfaceWriteExact || writes[index].value.Mode != equation.TargetModeStrong {
				inner.mu.Unlock()
				return RuleSurfaceSourceReceipt{}, RuleSourceIssueFailureWrite
			}
		case composition.WriteRoute:
			route := writes[index].route
			if route == nil || !route.Valid() || route.fence.authority != inner.authority || route.fence.schema != inner.state.schema || route.write != uint64(index) || writes[index].value.Form != equation.SurfaceWriteRoute || writes[index].value.Mode != equation.TargetModeNone {
				inner.mu.Unlock()
				return RuleSurfaceSourceReceipt{}, RuleSourceIssueFailureWrite
			}
			resolved.Route = route.read + 1
		case composition.WriteSelect:
			selector := writes[index].selector
			if selector == nil || !selector.Valid() || selector.fence.authority != inner.authority || selector.fence.schema != inner.state.schema || selector.write != uint64(index) || writes[index].value.Form != equation.SurfaceWriteSelect || writes[index].value.Mode != equation.TargetModeStrong || len(writes[index].targets) != len(write.Candidates) {
				inner.mu.Unlock()
				return RuleSurfaceSourceReceipt{}, RuleSourceIssueFailureWrite
			}
			resolved.Candidates = append([]uint64(nil), write.Candidates...)
			resolved.TargetCandidates = append([]equation.Surface(nil), writes[index].targets...)
			targetDependencies := make([]uint64, 0, len(write.Dependencies))
			for _, dependency := range write.Dependencies {
				if dependency.Target {
					targetDependencies = append(targetDependencies, dependency.Index)
				}
			}
			if len(writes[index].relations) != len(targetDependencies) {
				inner.mu.Unlock()
				return RuleSurfaceSourceReceipt{}, RuleSourceIssueFailureWrite
			}
			for relationIndex, relation := range writes[index].relations {
				if relation.Prior != targetDependencies[relationIndex] || len(relation.Matches) != len(write.Candidates) {
					inner.mu.Unlock()
					return RuleSurfaceSourceReceipt{}, RuleSourceIssueFailureWrite
				}
			}
			resolved.Relations = append([]equation.CandidateRelation(nil), writes[index].relations...)
		default:
			inner.mu.Unlock()
			return RuleSurfaceSourceReceipt{}, RuleSourceIssueFailureWrite
		}
		resolvedWrites[index] = resolved
	}
	inner.mu.Unlock()
	source, sourceOK := assembly.builder.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{Schema: ruleKey, OperandFamily: familyKey, Occurrence: occurrence.value, Operand: operand.value, Reads: resolvedReads, Carries: carries, Writes: resolvedWrites})
	if !sourceOK {
		return RuleSurfaceSourceReceipt{}, RuleSourceIssueFailureBatch
	}
	return RuleSurfaceSourceReceipt{value: source, assembly: assembly}, RuleSourceIssueFailureNone
}

func mountedRuleMemberID(role RuleSlotCapability, mount, point, occurrence identity.ContentID) identity.ContentID {
	if !role.mounted() || !mount.Available() || !point.Available() || !occurrence.Available() {
		return identity.ContentID{}
	}
	encoded := []byte("analysis/engine/rule-member/v2")
	encoded = appendRuleSlotCapability(encoded, role)
	encoded = append(encoded, mount[:]...)
	encoded = append(encoded, point[:]...)
	encoded = append(encoded, occurrence[:]...)
	return identity.ContentID(sha256.Sum256(encoded))
}

func mountedRuleActivationID(role RuleSlotCapability, mount, point, occurrence identity.ContentID) identity.ContentID {
	if !role.mounted() || !mount.Available() || !point.Available() || !occurrence.Available() {
		return identity.ContentID{}
	}
	encoded := []byte("analysis/engine/activation-member/v2")
	encoded = appendRuleSlotCapability(encoded, role)
	encoded = append(encoded, mount[:]...)
	encoded = append(encoded, point[:]...)
	encoded = append(encoded, occurrence[:]...)
	return identity.ContentID(sha256.Sum256(encoded))
}

func mountedRuleInputKey(member, input identity.ContentID, slot uint64) (composition.Key, bool) {
	if !member.Available() || !input.Available() {
		return composition.Key{}, false
	}
	encoded := []byte("analysis/engine/rule-input/v1")
	encoded = append(encoded, member[:]...)
	encoded = append(encoded, input[:]...)
	for shift := uint(56); ; shift -= 8 {
		encoded = append(encoded, byte(slot>>shift))
		if shift == 0 {
			break
		}
	}
	return artifactReceiptKey(identity.ContentID(sha256.Sum256(encoded)), artifactOccurrenceSourceVersion)
}

func linkRuleOccurrenceKey(role RuleSlotCapability, occurrence identity.ContentID) (composition.Key, bool) {
	if !role.link() || !occurrence.Available() {
		return composition.Key{}, false
	}
	encoded := []byte("analysis/engine/link-rule-occurrence/v1")
	encoded = appendRuleSlotCapability(encoded, role)
	encoded = append(encoded, occurrence[:]...)
	return artifactReceiptKey(identity.ContentID(sha256.Sum256(encoded)), artifactOccurrenceSourceVersion)
}

func linkRuleMemberID(role RuleSlotCapability, owner, point, occurrence identity.ContentID) identity.ContentID {
	if !role.link() || !owner.Available() || !point.Available() || !occurrence.Available() {
		return identity.ContentID{}
	}
	encoded := []byte("analysis/engine/link-rule-member/v1")
	encoded = appendRuleSlotCapability(encoded, role)
	encoded = append(encoded, owner[:]...)
	encoded = append(encoded, point[:]...)
	encoded = append(encoded, occurrence[:]...)
	return identity.ContentID(sha256.Sum256(encoded))
}

// mountedRuleOccurrenceKey keeps the Batch occurrence entity family-local.
// One authored occurrence can feed several closed Rule roles; sharing the
// raw artifact ID would alias those independent semantic rows.
func mountedRuleOccurrenceKey(role RuleSlotCapability, occurrence identity.ContentID) (composition.Key, bool) {
	if !role.mounted() || !occurrence.Available() {
		return composition.Key{}, false
	}
	encoded := []byte("analysis/engine/rule-occurrence/v1")
	encoded = appendRuleSlotCapability(encoded, role)
	encoded = append(encoded, occurrence[:]...)
	return artifactReceiptKey(identity.ContentID(sha256.Sum256(encoded)), artifactOccurrenceSourceVersion)
}

func appendRuleSlotCapability(encoded []byte, capability RuleSlotCapability) []byte {
	encoded = append(encoded, byte(capability.kind), byte(boolByte(capability.activation)))
	for shift := uint(56); ; shift -= 8 {
		encoded = append(encoded, byte(capability.ordinal>>shift))
		if shift == 0 {
			break
		}
	}
	return append(encoded, 0)
}

func boolByte(value bool) uint8 {
	if value {
		return 1
	}
	return 0
}

// Typed implementation adapters consume only the opaque source receipt.
func (implementation *RuleImplementation[K, V, O]) BeginReceiptRuleRow(source RuleSurfaceSourceReceipt) (*BindingRuleRowDraft, bool) {
	draft, ok := implementation.BeginBindingRuleRow(source.value)
	if !ok {
		source.assembly.recordRuleFinalizerFailure(RuleFinalizerFailureBeginDraft)
		return nil, false
	}
	draft.assembly = source.assembly
	return draft, true
}

func (implementation *ActivationRuleImplementation) BeginReceiptRuleRow(source RuleSurfaceSourceReceipt) (*BindingRuleRowDraft, bool) {
	draft, ok := implementation.BeginBindingRuleRow(source.value)
	if !ok {
		source.assembly.recordRuleFinalizerFailure(RuleFinalizerFailureBeginDraft)
		return nil, false
	}
	draft.assembly = source.assembly
	return draft, true
}

func (implementation *ActivationRuleImplementation) ReceiptReadPart(source RuleSurfaceSourceReceipt, index uint64) (BindingRuleReadPartReceipt, bool) {
	part, ok := implementation.ReadPart(source.value, index)
	if !ok {
		source.assembly.recordRuleFinalizerFailure(RuleFinalizerFailureReadPart)
	}
	return part, ok
}

func (implementation *RuleImplementation[K, V, O]) ReceiptReadPart(source RuleSurfaceSourceReceipt, index uint64) (BindingRuleReadPartReceipt, bool) {
	part, ok := implementation.ReadPart(source.value, index)
	if !ok {
		source.assembly.recordRuleFinalizerFailure(RuleFinalizerFailureReadPart)
	}
	return part, ok
}

func (implementation *RuleImplementation[K, V, O]) ReceiptCarryPart(source RuleSurfaceSourceReceipt, index uint64) (BindingRuleCarryPartReceipt, bool) {
	part, ok := implementation.CarryPart(source.value, index)
	if !ok {
		source.assembly.recordRuleFinalizerFailure(RuleFinalizerFailureCarryPart)
	}
	return part, ok
}

func (implementation *RuleImplementation[K, V, O]) ReceiptWritePart(source RuleSurfaceSourceReceipt, index uint64) (BindingRuleWritePartReceipt, bool) {
	part, ok := implementation.WritePart(source.value, index)
	if !ok {
		source.assembly.recordRuleFinalizerFailure(RuleFinalizerFailureWritePart)
	}
	return part, ok
}

func (implementation *RuleImplementation[K, V, O]) ReceiptSupportPart(source RuleSurfaceSourceReceipt, index uint64) (BindingRuleSupportPartReceipt, bool) {
	part, ok := implementation.SupportPart(source.value, index)
	if !ok {
		source.assembly.recordRuleFinalizerFailure(RuleFinalizerFailureSupportPart)
	}
	return part, ok
}

func (implementation *RuleImplementation[K, V, O]) ReceiptPrunePart(source RuleSurfaceSourceReceipt, index uint64) (BindingRulePrunePartReceipt, bool) {
	part, ok := implementation.PrunePart(source.value, index)
	if !ok {
		source.assembly.recordRuleFinalizerFailure(RuleFinalizerFailurePrunePart)
	}
	return part, ok
}
