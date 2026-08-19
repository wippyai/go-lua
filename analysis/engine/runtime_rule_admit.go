package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/canonical"
)

// ruleOccurrence and ruleOperand are sealed source-batch
// capabilities. Their equation identities remain private to engine; a caller
// can only obtain them from the exact mounted artifact inverse below.
type ruleOccurrence struct {
	builder     *BindingTopologyBuilder
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

type ruleOperand struct {
	builder    *BindingTopologyBuilder
	occurrence ruleOccurrence
	value      equation.Operand
}

// ruleSurfaceSource is the builder-private sealed source consumed by typed
// RuleImplementation draft methods.
type ruleSurfaceSource struct {
	value   equation.SurfaceSource
	builder *BindingTopologyBuilder
}

// AdmitMountedRuleOccurrence resolves one exact mounted point and occurrence
// identity. The member identity is mount+point+occurrence qualified, so equal
// reusable artifacts and same IDs on different mounts cannot alias.
func (builder *BindingTopologyBuilder) AdmitMountedRuleOccurrence(role RuleSlotCapability, mountID, reusablePointID, occurrenceID identity.ContentID) (ruleOccurrence, bool) {
	if builder == nil || !role.mounted() || !mountID.Available() || !reusablePointID.Available() || !occurrenceID.Available() {
		return ruleOccurrence{}, false
	}
	inner, ok := builder.lockSourcesOpen()
	if !ok {
		return ruleOccurrence{}, false
	}
	artifactRows := builder.mountedRows
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
		return ruleOccurrence{}, false
	}
	entity, entityOK := mountedRuleOccurrenceKey(role, occurrenceID)
	occurrence, occurrenceOK := builder.admitFrom(site, entity)
	member := mountedRuleMemberID(role, mountID, reusablePointID, occurrenceID)
	activation := mountedRuleActivationID(role, mountID, reusablePointID, occurrenceID)
	result := ruleOccurrence{builder: builder, role: role, value: occurrence, member: member, activation: activation, mount: mountID, reusable: reusablePointID, input: inputSite, inputID: inputID, stage: stage, predecessor: predecessor, routed: routed}
	return result, stage.Valid() && entityOK && occurrenceOK && member.Available() && activation.Available()
}

// AdmitLinkRuleOccurrence resolves only the two Link-global bootstrap roles
// from the sealed witness catalog. It has no mount argument and cannot admit
// an arbitrary site or occurrence ID.
func (builder *BindingTopologyBuilder) AdmitLinkRuleOccurrence(role RuleSlotCapability, occurrenceID identity.ContentID) (ruleOccurrence, bool) {
	if builder == nil || !role.link() || !occurrenceID.Available() || builder.inner == nil {
		return ruleOccurrence{}, false
	}
	inner, ok := builder.lockSourcesOpen()
	if !ok {
		return ruleOccurrence{}, false
	}
	bootstrap := builder.mountedRows
	// The bootstrap Site is deliberately admitted into the still-open source
	// Batch. Site.Available requires a sealed Batch and therefore cannot be
	// used at this pre-seal admission boundary; admitFrom below authenticates
	// the open-batch capability and preserves the same fence as mounted rows.
	if bootstrap == nil || bootstrap.bootstrap == nil {
		inner.mu.Unlock()
		return ruleOccurrence{}, false
	}
	if assignedRole, found := bootstrap.bootstrap.roles[occurrenceID]; !found || assignedRole != role {
		inner.mu.Unlock()
		return ruleOccurrence{}, false
	}
	if _, found := bootstrap.bootstrap.claims[occurrenceID]; found {
		inner.mu.Unlock()
		return ruleOccurrence{}, false
	}
	bootstrap.bootstrap.claims[occurrenceID] = role
	site := bootstrap.bootstrap.site
	inner.mu.Unlock()
	entity, entityOK := linkRuleOccurrenceKey(role, occurrenceID)
	occurrence, occurrenceOK := builder.admitFrom(site, entity)
	member := linkRuleMemberID(role, bootstrap.bootstrap.owner, bootstrap.bootstrap.point.PointID, occurrenceID)
	result := ruleOccurrence{builder: builder, linkRole: role, value: occurrence, member: member}
	return result, entityOK && occurrenceOK && member.Available()
}

// AdmitMountedRuleOperand binds one typed issuer's canonical content digest
// to its already-authenticated mounted occurrence. It cannot accept a raw
// occurrence from another assembly or a caller-selected equation identity.
func (builder *BindingTopologyBuilder) AdmitMountedRuleOperand(occurrence ruleOccurrence, digest [32]byte) (ruleOperand, bool) {
	// Rule operands are admitted before source sealing. Occurrence.Available
	// intentionally requires a sealed Batch; SameOpen is the corresponding
	// capability fence for this open-phase transaction.
	if builder == nil || occurrence.builder != builder || !occurrence.value.SameOpen(occurrence.value) {
		return ruleOperand{}, false
	}
	entity, entityOK := operandEntityForContent(digest)
	operand, operandOK := builder.admitOperand(occurrence.value, entity)
	return ruleOperand{builder: builder, occurrence: occurrence, value: operand}, entityOK && operandOK
}

// BeginMountedRuleOccurrence is the typed owner bridge: the caller supplies
// the domain operand O, while the exact sealed implementation supplies its
// own canonicalization law. It only admits the operand; surface geometry is
// deliberately a later operation using owner-issued Ref receipts.
func BeginMountedRuleOccurrence[K ~uint32 | ~uint64, V, O any](builder *BindingTopologyBuilder, implementation *RuleImplementation[K, V, O], occurrence ruleOccurrence, operand O) (ruleOperand, bool) {
	if builder == nil || implementation == nil || occurrence.builder != builder || !implementation.binding.valid() || implementation.binding.cell == nil || implementation.binding.cell.impl == nil || !occurrenceRoleOwnsSchema(occurrence, implementation.binding.proof.schema, implementation.binding.proof.semantic) {
		return ruleOperand{}, false
	}
	_, digest, contentOK := implementation.binding.cell.impl.operandContent(operand)
	if !contentOK {
		return ruleOperand{}, false
	}
	operandReceipt, operandOK := builder.AdmitMountedRuleOperand(occurrence, digest)
	if !operandOK {
		return ruleOperand{}, false
	}
	return operandReceipt, true
}

// BeginMountedActivationRuleAdmission is the structural sibling for an
// ActivationRuleImplementation. Activation rows have no typed operand
// callback; their sealed artifact issuer therefore supplies the canonical
// operand digest directly.
func BeginMountedActivationRuleAdmission(builder *BindingTopologyBuilder, implementation *ActivationRuleImplementation, occurrence ruleOccurrence, digest [32]byte) (*RuleSourceTransaction, bool) {
	if builder == nil || implementation == nil || occurrence.builder != builder || !implementation.binding.valid() || !occurrenceRoleOwnsSchema(occurrence, implementation.binding.proof.schema, implementation.binding.proof.semantic) {
		return nil, false
	}
	operand, ok := builder.AdmitMountedRuleOperand(occurrence, digest)
	if !ok {
		return nil, false
	}
	semantic, semanticOK := semanticKeyFromComposition(implementation.binding.proof.semantic)
	family, familyOK := semanticKeyFromComposition(implementation.binding.proof.operandFamily)
	if !semanticOK || !familyOK {
		return nil, false
	}
	return &RuleSourceTransaction{
		builder:    builder,
		semantic:   semantic,
		family:     family,
		occurrence: occurrence,
		operand:    operand,
	}, true
}

// AddRule admits the row under the exact mounted occurrence authority that
// issued its source. Callers cannot forge or reuse a semantic member ID.
func (builder *BindingTopologyBuilder) AddRule(occurrence ruleOccurrence, receipt bindingRuleRow) (bindingRuleRowRef, bool) {
	mounted, link := occurrence.role.mounted(), occurrence.linkRole.link()
	if builder == nil || occurrence.builder != builder || !occurrence.member.Available() || mounted == link || mounted && (!occurrence.activation.Available() || !occurrence.stage.Valid()) {
		if builder != nil {
			builder.recordRuleFinalizerFailure(RuleFinalizerFailureAddRuleArguments)
		}
		return bindingRuleRowRef{}, false
	}
	inner := builder.inner
	if inner == nil || receipt.builder != inner || receipt.state != inner.state || receipt.authority != inner.authority || !receipt.row.Occurrence.Same(occurrence.value) || !receipt.row.Operand.Occurrence().Same(occurrence.value) || !occurrenceRoleOwnsSchema(occurrence, inner.state.schema, receipt.row.Schema) {
		builder.recordRuleFinalizerFailure(RuleFinalizerFailureAddRuleFence)
		return bindingRuleRowRef{}, false
	}
	receipt.input, receipt.inputID, receipt.stage, receipt.predecessor, receipt.routed = occurrence.input, occurrence.inputID, occurrence.stage, occurrence.predecessor, occurrence.routed
	ref, ok := builder.addSemanticRule(occurrence.member, receipt)
	if !ok {
		builder.recordRuleFinalizerFailure(RuleFinalizerFailureAddSemanticRule)
	}
	return ref, ok
}

func occurrenceRoleOwnsSchema(occurrence ruleOccurrence, schema *Schema, semantic composition.Key) bool {
	if schema == nil || !semantic.Available() {
		return false
	}
	if _, ok := schema.ruleOrdinalOf(semantic); !ok || occurrence.builder == nil || occurrence.builder.binding == nil || occurrence.builder.binding.state == nil {
		return false
	}
	state := occurrence.builder.binding.state
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
func (builder *BindingTopologyBuilder) AddRuleFromDraft(occurrence ruleOccurrence, draft *bindingRuleRowDraft) (bindingRuleRowRef, bool) {
	if builder == nil || builder.inner == nil || occurrence.builder != builder || draft == nil || draft.state != builder.inner.state || draft.authority != builder.inner.authority || !draft.source.Occurrence().Same(occurrence.value) || !draft.source.Operand().Occurrence().Same(occurrence.value) {
		if builder != nil {
			builder.recordRuleFinalizerFailure(RuleFinalizerFailureAddRuleDraft)
		}
		return bindingRuleRowRef{}, false
	}
	receipt, ok := builder.issueRuleRow(draft)
	if !ok {
		builder.recordRuleFinalizerFailure(RuleFinalizerFailureIssueRuleRow)
		return bindingRuleRowRef{}, false
	}
	return builder.AddRule(occurrence, receipt)
}

func (builder *BindingTopologyBuilder) AddActivationRule(occurrence ruleOccurrence, receipt bindingRuleRow) bool {
	if builder == nil || occurrence.builder != builder || !occurrence.role.mounted() || !occurrence.stage.Valid() || !occurrence.member.Available() {
		if builder != nil {
			builder.recordRuleFinalizerFailure(RuleFinalizerFailureAddRuleArguments)
		}
		return false
	}
	inner := builder.inner
	if inner == nil || receipt.builder != inner || receipt.state != inner.state || receipt.authority != inner.authority || !receipt.row.Occurrence.Same(occurrence.value) || !receipt.row.Operand.Occurrence().Same(occurrence.value) || !occurrenceRoleOwnsSchema(occurrence, inner.state.schema, receipt.row.Schema) {
		builder.recordRuleFinalizerFailure(RuleFinalizerFailureAddRuleFence)
		return false
	}
	receipt.input, receipt.inputID, receipt.stage, receipt.predecessor, receipt.routed = occurrence.input, occurrence.inputID, occurrence.stage, occurrence.predecessor, occurrence.routed
	ref, ok := builder.addSemanticRule(occurrence.member, receipt)
	if !ok {
		builder.recordRuleFinalizerFailure(RuleFinalizerFailureAddSemanticRule)
		return false
	}
	if !builder.addSemanticActivation(occurrence.activation, ref) {
		builder.recordRuleFinalizerFailure(RuleFinalizerFailureAddSemanticActivation)
		return false
	}
	return true
}

func (builder *BindingTopologyBuilder) AddActivationRuleFromDraft(occurrence ruleOccurrence, draft *bindingRuleRowDraft) bool {
	if builder == nil || builder.inner == nil || occurrence.builder != builder || draft == nil || draft.state != builder.inner.state || draft.authority != builder.inner.authority || !draft.source.Occurrence().Same(occurrence.value) || !draft.source.Operand().Occurrence().Same(occurrence.value) {
		if builder != nil {
			builder.recordRuleFinalizerFailure(RuleFinalizerFailureAddRuleDraft)
		}
		return false
	}
	receipt, ok := builder.issueRuleRow(draft)
	if !ok {
		builder.recordRuleFinalizerFailure(RuleFinalizerFailureIssueRuleRow)
		return false
	}
	return builder.AddActivationRule(occurrence, receipt)
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

func (transaction *RuleSourceTransaction) AnchoredSelectedReadSurface(receipt schemaSelectedRead, dependencies []RuleReadSurface) (RuleReadSurface, bool) {
	surface, failure := transaction.AnchoredSelectedReadSurfaceWithFailure(receipt, dependencies)
	return surface, failure == AnchoredSelectedReadFailureNone
}

func (transaction *RuleSourceTransaction) AnchoredSelectedReadSurfaceWithFailure(receipt schemaSelectedRead, dependencies []RuleReadSurface) (RuleReadSurface, AnchoredSelectedReadFailure) {
	if transaction == nil || transaction.builder == nil || transaction.builder.inner == nil || transaction.operand.builder != transaction.builder || transaction.occurrence.builder != transaction.builder {
		return RuleReadSurface{}, AnchoredSelectedReadFailureArguments
	}
	if !receipt.Valid() || receipt.fence.authority == nil {
		return RuleReadSurface{}, AnchoredSelectedReadFailureReceipt
	}
	if receipt.fence.authority != transaction.builder.inner.authority || receipt.fence.schema != transaction.builder.inner.state.schema {
		return RuleReadSurface{}, AnchoredSelectedReadFailureOwner
	}
	ruleSemantic, ruleSemanticOK := semanticKeyFromComposition(receipt.fence.schema.ruleSemanticAt(receipt.fence.rule))
	if !ruleSemanticOK || ruleSemantic != transaction.semantic {
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
		if !ok || !shapeOK || dependency.authority != receipt.fence.authority || dependency.value.Mode != equation.TargetModeNone || dependency.value.Factor != shape.Factor || !dependency.value.LocalAvailable() || !validSelectedDependencySurface(shape, dependency.value) {
			return RuleReadSurface{}, AnchoredSelectedReadFailureDependencySurface
		}
	}
	content, contentOK := anchoredSelectedContent(transaction.occurrence.value, transaction.operand.value, receipt)
	if !contentOK {
		return RuleReadSurface{}, AnchoredSelectedReadFailureReceipt
	}
	for _, existing := range transaction.reads {
		if existing.value.Factor == factor && existing.value.Form == equation.SurfaceReadSelect && existing.value.Content == content {
			return RuleReadSurface{}, AnchoredSelectedReadFailureDuplicate
		}
	}
	surface := equation.Surface{Factor: factor, Form: equation.SurfaceReadSelect, Content: content, Semantic: factor}
	anchor := mountedSelectedSurfaceAnchor{builder: transaction.builder, occurrence: transaction.occurrence.value, operand: transaction.operand.value, rule: receipt.fence.rule, index: receipt.read, form: equation.SurfaceReadSelect}
	if !transaction.builder.claimMountedSelectedSurface(surface, anchor) {
		return RuleReadSurface{}, AnchoredSelectedReadFailureClaim
	}
	return RuleReadSurface{value: surface, authority: receipt.fence.authority, anchor: &anchor}, AnchoredSelectedReadFailureNone
}

// AnchoredRouteWriteSurface is the route sibling: the output has no single
// exact Ref because runtime chooses zero-or-many selected targets. Its local
// is tied to the admitted occurrence/operand and sealed route proof.
func (transaction *RuleSourceTransaction) AnchoredRouteWriteSurface(receipt schemaRouteWrite) (RuleWriteSurface, bool) {
	if transaction == nil || transaction.builder == nil || transaction.builder.inner == nil || transaction.operand.builder != transaction.builder || transaction.occurrence.builder != transaction.builder || !receipt.Valid() || receipt.fence.authority == nil || receipt.fence.authority != transaction.builder.inner.authority || receipt.fence.schema != transaction.builder.inner.state.schema {
		return RuleWriteSurface{}, false
	}
	ruleSemantic, ruleSemanticOK := semanticKeyFromComposition(receipt.fence.schema.ruleSemanticAt(receipt.fence.rule))
	if !ruleSemanticOK || ruleSemantic != transaction.semantic {
		return RuleWriteSurface{}, false
	}
	factor := receipt.fence.schema.factorSemanticAt(receipt.factor)
	if !factor.Available() {
		return RuleWriteSurface{}, false
	}
	content, contentOK := anchoredRouteContent(transaction.occurrence.value, transaction.operand.value, receipt)
	if !contentOK {
		return RuleWriteSurface{}, false
	}
	surface := equation.Surface{Factor: factor, Form: equation.SurfaceWriteRoute, Content: content}
	anchor := mountedSelectedSurfaceAnchor{builder: transaction.builder, occurrence: transaction.occurrence.value, operand: transaction.operand.value, rule: receipt.fence.rule, index: receipt.write, form: equation.SurfaceWriteRoute}
	if !transaction.builder.claimMountedSurface(surface, anchor) {
		return RuleWriteSurface{}, false
	}
	return RuleWriteSurface{value: surface, authority: receipt.fence.authority, route: &receipt, anchor: &anchor}, true
}

// RuleSourceTransaction is the closed, occurrence-owned source admission
// envelope. It records only owner-issued geometry; no equation coordinates or
// cold factor ordinals can be supplied through this API.
type RuleSourceTransaction struct {
	builder    *BindingTopologyBuilder
	semantic   identity.SemanticKey
	family     identity.SemanticKey
	occurrence ruleOccurrence
	operand    ruleOperand
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
	if transaction == nil || transaction.builder == nil || transaction.builder.inner == nil || anchor == nil {
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
	if anchor.builder != transaction.builder || !occurrenceMatches || !operandMatches || anchor.form != form || anchor.index != index {
		return false
	}
	ordinal, ok := transaction.builder.inner.state.schema.ruleOrdinalOf(compositionKeyOf(transaction.semantic))
	return ok && anchor.rule == ordinal
}

// AdmitMountedRule owns one mounted rule's source scope. It begins the
// admission, hands the open transaction to admit for surface placement, and
// queues capability's finalizer, which seals the transaction and hands the
// sealed source to issue. The transaction stays inside this scope, so an
// admitted rule always carries its finalizer.
func AdmitMountedRule[K ~uint32 | ~uint64, V, O any](builder *BindingTopologyBuilder, implementation *RuleImplementation[K, V, O], capability RuleSlotCapability, occurrence ruleOccurrence, operand O, admit func(*RuleSourceTransaction) bool) bool {
	if builder == nil || implementation == nil {
		return false
	}
	return admitRuleScope(builder, implementation, occurrence, operand, admit, func(source ruleSurfaceSource) bool {
		return implementation.issueDraft(builder, occurrence, source)
	}, nil, builder.QueueMountedRuleFinalizer, capability)
}

// AdmitMountedRuleWithFailure is AdmitMountedRule's diagnostic form: seal
// receives the closed seal phase that rejected the queued finalizer, so an
// owner keeps its own failure vocabulary without holding the transaction.
func AdmitMountedRuleWithFailure[K ~uint32 | ~uint64, V, O any](builder *BindingTopologyBuilder, implementation *RuleImplementation[K, V, O], capability RuleSlotCapability, occurrence ruleOccurrence, operand O, admit func(*RuleSourceTransaction) bool, seal func(RuleSourceSealFailure)) bool {
	if builder == nil || implementation == nil {
		return false
	}
	return admitRuleScope(builder, implementation, occurrence, operand, admit, func(source ruleSurfaceSource) bool {
		return implementation.issueDraft(builder, occurrence, source)
	}, seal, builder.QueueMountedRuleFinalizer, capability)
}

// AdmitLinkRule is the Link-cardinality sibling of AdmitMountedRule.
func AdmitLinkRule[K ~uint32 | ~uint64, V, O any](builder *BindingTopologyBuilder, implementation *RuleImplementation[K, V, O], capability RuleSlotCapability, occurrence ruleOccurrence, operand O, admit func(*RuleSourceTransaction) bool) bool {
	if builder == nil || implementation == nil {
		return false
	}
	return admitRuleScope(builder, implementation, occurrence, operand, admit, func(source ruleSurfaceSource) bool {
		return implementation.issueDraft(builder, occurrence, source)
	}, nil, builder.QueueLinkRuleFinalizer, capability)
}

// admitRuleScope is the single admission scope shared by the mounted and link
// entries. queue selects the cardinality-specific finalizer ingress.
func admitRuleScope[K ~uint32 | ~uint64, V, O any](builder *BindingTopologyBuilder, implementation *RuleImplementation[K, V, O], occurrence ruleOccurrence, operand O, admit func(*RuleSourceTransaction) bool, issue func(ruleSurfaceSource) bool, seal func(RuleSourceSealFailure), queue func(RuleSlotCapability, func() bool) bool, capability RuleSlotCapability) bool {
	if builder == nil || admit == nil || issue == nil {
		return false
	}
	transaction, ok := beginMountedRuleAdmission(builder, implementation, occurrence, operand)
	if !ok {
		return false
	}
	if !admit(transaction) {
		return false
	}
	return queue(capability, func() bool {
		source, failure := transaction.SealWithFailure()
		if failure != RuleSourceSealFailureNone {
			if seal != nil {
				seal(failure)
			}
			return false
		}
		return issue(source)
	})
}

// beginMountedRuleAdmission combines typed operand canonicalization with the
// exact implementation's semantic/family proof, while leaving all surfaces
// to the domain owner.
func beginMountedRuleAdmission[K ~uint32 | ~uint64, V, O any](builder *BindingTopologyBuilder, implementation *RuleImplementation[K, V, O], occurrence ruleOccurrence, operand O) (*RuleSourceTransaction, bool) {
	operandReceipt, ok := BeginMountedRuleOccurrence(builder, implementation, occurrence, operand)
	if !ok || implementation == nil || implementation.binding.proof == nil || !occurrenceRoleOwnsSchema(occurrence, implementation.binding.proof.schema, implementation.binding.proof.semantic) {
		return nil, false
	}
	semantic, semanticOK := semanticKeyFromComposition(implementation.binding.proof.semantic)
	family, familyOK := semanticKeyFromComposition(implementation.binding.proof.operandFamily)
	if !semanticOK || !familyOK {
		return nil, false
	}
	return &RuleSourceTransaction{
		builder:    builder,
		semantic:   semantic,
		family:     family,
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

func AddSummaryRead[K ~uint32 | ~uint64](transaction *RuleSourceTransaction, receipt schemaSummaryRead, refs *ClosedRefs[K]) bool {
	surface, ok := SummaryReadSurface(receipt, refs)
	return ok && transaction != nil && transaction.AddRead(surface)
}

type summaryReadRefs interface {
	placeSummaryRead(transaction *RuleSourceTransaction, receipt schemaSummaryRead) bool
}

func (refs *ClosedRefs[K]) placeSummaryRead(transaction *RuleSourceTransaction, receipt schemaSummaryRead) bool {
	return AddSummaryRead(transaction, receipt, refs)
}

func addSummaryReadRefs(transaction *RuleSourceTransaction, receipt schemaSummaryRead, refs any) bool {
	placed, ok := refs.(summaryReadRefs)
	return ok && placed.placeSummaryRead(transaction, receipt)
}

func AddSelectedRead[K ~uint32 | ~uint64](transaction *RuleSourceTransaction, receipt schemaSelectedRead, ref Ref[K], dependencies []RuleReadSurface) bool {
	surface, ok := SelectedReadSurface(receipt, ref, dependencies)
	return ok && transaction != nil && transaction.AddRead(surface)
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

func AddAnchoredRouteWrite(transaction *RuleSourceTransaction, receipt schemaRouteWrite) bool {
	surface, ok := transaction.AnchoredRouteWriteSurface(receipt)
	return ok && transaction.AddWrite(surface)
}

func (transaction *RuleSourceTransaction) Seal() (ruleSurfaceSource, bool) {
	source, failure := transaction.SealWithFailure()
	return source, failure == RuleSourceSealFailureNone
}

// SealWithFailure seals one exact mounted source and returns its closed
// rejected phase when the source cannot be issued.
func (transaction *RuleSourceTransaction) SealWithFailure() (sourceResult ruleSurfaceSource, failureResult RuleSourceSealFailure) {
	defer func() {
		if transaction != nil && transaction.builder != nil {
			transaction.builder.recordRuleSourceSealFailure(failureResult)
		}
	}()
	if transaction == nil || transaction.sealed {
		return ruleSurfaceSource{}, RuleSourceSealFailurePrecondition
	}
	transaction.sealed = true
	if transaction.builder == nil {
		return ruleSurfaceSource{}, RuleSourceSealFailurePrecondition
	}
	inner, ok := transaction.builder.lockTopologyOpen()
	if !ok {
		return ruleSurfaceSource{}, RuleSourceSealFailurePrecondition
	}
	ordinal, found := inner.state.schema.cold.RuleIndex(compositionKeyOf(transaction.semantic))
	rule, rowOK := inner.state.schema.cold.RuleAt(ordinal)
	valid := found && rowOK && rule.OperandFamily == compositionKeyOf(transaction.family) && uint64(len(transaction.reads)) == uint64(len(rule.Reads)) && uint64(len(transaction.writes)) == uint64(len(rule.Writes)) && transaction.carries == uint64(len(rule.Carries))
	inner.mu.Unlock()
	if !valid {
		return ruleSurfaceSource{}, RuleSourceSealFailureColdShape
	}
	source, issueFailure := transaction.builder.IssueRuleSourceWithSurfacesWithFailure(transaction.semantic, transaction.family, transaction.occurrence, transaction.operand, transaction.reads, transaction.writes)
	if issueFailure != RuleSourceIssueFailureNone {
		switch issueFailure {
		case RuleSourceIssueFailureArguments:
			return ruleSurfaceSource{}, RuleSourceSealFailureIssueArguments
		case RuleSourceIssueFailureTopology:
			return ruleSurfaceSource{}, RuleSourceSealFailureIssueTopology
		case RuleSourceIssueFailureRule:
			return ruleSurfaceSource{}, RuleSourceSealFailureIssueRule
		case RuleSourceIssueFailureShape:
			return ruleSurfaceSource{}, RuleSourceSealFailureIssueShape
		case RuleSourceIssueFailureReadAuthority:
			return ruleSurfaceSource{}, RuleSourceSealFailureIssueReadAuthority
		case RuleSourceIssueFailureReadSurface:
			return ruleSurfaceSource{}, RuleSourceSealFailureIssueReadSurface
		case RuleSourceIssueFailureReadFactor:
			return ruleSurfaceSource{}, RuleSourceSealFailureIssueReadFactor
		case RuleSourceIssueFailureReadAnchor:
			return ruleSurfaceSource{}, RuleSourceSealFailureIssueReadAnchor
		case RuleSourceIssueFailureWrite:
			return ruleSurfaceSource{}, RuleSourceSealFailureIssueWrite
		case RuleSourceIssueFailureBatch:
			return ruleSurfaceSource{}, RuleSourceSealFailureIssueBatch
		default:
			return ruleSurfaceSource{}, RuleSourceSealFailureIssueArguments
		}
	}
	for _, read := range transaction.reads {
		if read.summary == nil || read.summary.receipt == nil {
			continue
		}
		if !transaction.builder.addSummary(read.summary.receipt, equation.SummaryMapping{Surface: read.summary.surface, Keys: read.summary.keys}) {
			return ruleSurfaceSource{}, RuleSourceSealFailureSummary
		}
	}
	return source, RuleSourceSealFailureNone
}

// IssueRuleSourceWithSurfaces finalizes a typed source only after every
// occurrence-specific read/write Ref has been supplied by the domain owner.
func (builder *BindingTopologyBuilder) IssueRuleSourceWithSurfaces(semantic, operandFamily identity.SemanticKey, occurrence ruleOccurrence, operand ruleOperand, reads []RuleReadSurface, writes []RuleWriteSurface) (ruleSurfaceSource, bool) {
	source, failure := builder.IssueRuleSourceWithSurfacesWithFailure(semantic, operandFamily, occurrence, operand, reads, writes)
	return source, failure == RuleSourceIssueFailureNone
}

// IssueRuleSourceWithSurfacesWithFailure issues a complete typed source and
// returns the closed rejected predicate without exposing equation internals.
func (builder *BindingTopologyBuilder) IssueRuleSourceWithSurfacesWithFailure(semantic, operandFamily identity.SemanticKey, occurrence ruleOccurrence, operand ruleOperand, reads []RuleReadSurface, writes []RuleWriteSurface) (ruleSurfaceSource, RuleSourceIssueFailure) {
	if builder == nil || !semantic.Available() || !operandFamily.Available() || occurrence.builder != builder || operand.builder != builder || operand.occurrence != occurrence || !occurrence.value.Available() || !operand.value.Available() {
		return ruleSurfaceSource{}, RuleSourceIssueFailureArguments
	}
	inner, ok := builder.lockTopologyOpen()
	if !ok {
		return ruleSurfaceSource{}, RuleSourceIssueFailureTopology
	}
	ruleKey := compositionKeyOf(semantic)
	familyKey := compositionKeyOf(operandFamily)
	ruleOrdinal, ruleOK := inner.state.schema.cold.RuleIndex(ruleKey)
	rule, rowOK := inner.state.schema.cold.RuleAt(ruleOrdinal)
	if !ruleOK || !rowOK || rule.OperandFamily != familyKey {
		inner.mu.Unlock()
		return ruleSurfaceSource{}, RuleSourceIssueFailureRule
	}
	if len(reads) != len(rule.Reads) || len(writes) != len(rule.Writes) || len(rule.Supports) != 0 || len(rule.Prunes) != 0 {
		inner.mu.Unlock()
		return ruleSurfaceSource{}, RuleSourceIssueFailureShape
	}
	resolvedReads := make([]equation.ResolvedRead, len(rule.Reads))
	anchorTransaction := RuleSourceTransaction{builder: builder, semantic: semantic, occurrence: occurrence, operand: operand}
	for index, read := range rule.Reads {
		if reads[index].authority != inner.authority {
			inner.mu.Unlock()
			return ruleSurfaceSource{}, RuleSourceIssueFailureReadAuthority
		}
		if !reads[index].value.Available() {
			inner.mu.Unlock()
			return ruleSurfaceSource{}, RuleSourceIssueFailureReadSurface
		}
		if reads[index].value.Factor != read.Factor {
			inner.mu.Unlock()
			return ruleSurfaceSource{}, RuleSourceIssueFailureReadFactor
		}
		if reads[index].anchor != nil && !anchorTransaction.ownsSurfaceAnchor(reads[index].anchor, reads[index].value.Form, uint64(index)) {
			inner.mu.Unlock()
			return ruleSurfaceSource{}, RuleSourceIssueFailureReadAnchor
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
			return ruleSurfaceSource{}, RuleSourceIssueFailureWrite
		}
		resolved := equation.ResolvedWrite{Index: uint64(index), Surface: writes[index].value}
		switch write.Kind {
		case composition.WriteExact:
			if writes[index].route != nil || writes[index].value.Form != equation.SurfaceWriteExact || writes[index].value.Mode != equation.TargetModeStrong {
				inner.mu.Unlock()
				return ruleSurfaceSource{}, RuleSourceIssueFailureWrite
			}
		case composition.WriteRoute:
			route := writes[index].route
			if route == nil || !route.Valid() || route.fence.authority != inner.authority || route.fence.schema != inner.state.schema || route.write != uint64(index) || writes[index].value.Form != equation.SurfaceWriteRoute || writes[index].value.Mode != equation.TargetModeNone {
				inner.mu.Unlock()
				return ruleSurfaceSource{}, RuleSourceIssueFailureWrite
			}
			resolved.Route = route.read + 1
		default:
			inner.mu.Unlock()
			return ruleSurfaceSource{}, RuleSourceIssueFailureWrite
		}
		resolvedWrites[index] = resolved
	}
	inner.mu.Unlock()
	source, sourceOK := builder.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{Schema: ruleKey, OperandFamily: familyKey, Occurrence: occurrence.value, Operand: operand.value, Reads: resolvedReads, Carries: carries, Writes: resolvedWrites})
	if !sourceOK {
		return ruleSurfaceSource{}, RuleSourceIssueFailureBatch
	}
	return ruleSurfaceSource{value: source, builder: builder}, RuleSourceIssueFailureNone
}

const (
	mountedRuleMemberDomain     = "analysis/engine/rule-member"
	mountedRuleActivationDomain = "analysis/engine/activation-member"
	mountedRuleInputDomain      = "analysis/engine/rule-input"
	mountedRuleOccurrenceDomain = "analysis/engine/rule-occurrence"
	linkRuleOccurrenceDomain    = "analysis/engine/link-rule-occurrence"
	linkRuleMemberDomain        = "analysis/engine/link-rule-member"

	ruleSourceIdentityVersion uint64 = 3
)

func mountedRuleMemberID(role RuleSlotCapability, mount, point, occurrence identity.ContentID) identity.ContentID {
	if !role.mounted() || !mount.Available() || !point.Available() || !occurrence.Available() {
		return identity.ContentID{}
	}
	return framedContentID(mountedRuleMemberDomain, ruleSourceIdentityVersion, func(writer *canonical.DigestWriter) bool {
		return writeRuleSlotCapability(writer, role) && writeContentIDs(writer, mount, point, occurrence)
	})
}

func mountedRuleActivationID(role RuleSlotCapability, mount, point, occurrence identity.ContentID) identity.ContentID {
	if !role.mounted() || !mount.Available() || !point.Available() || !occurrence.Available() {
		return identity.ContentID{}
	}
	return framedContentID(mountedRuleActivationDomain, ruleSourceIdentityVersion, func(writer *canonical.DigestWriter) bool {
		return writeRuleSlotCapability(writer, role) && writeContentIDs(writer, mount, point, occurrence)
	})
}

func mountedRuleInputKey(member, input identity.ContentID, slot uint64) (composition.Key, bool) {
	if !member.Available() || !input.Available() {
		return composition.Key{}, false
	}
	id := framedContentID(mountedRuleInputDomain, ruleSourceIdentityVersion, func(writer *canonical.DigestWriter) bool {
		return writeContentIDs(writer, member, input) && writer.Uint(slot) == nil
	})
	if !id.Available() {
		return composition.Key{}, false
	}
	return artifactSourceKey(artifactOccurrenceSource, id)
}

func linkRuleOccurrenceKey(role RuleSlotCapability, occurrence identity.ContentID) (composition.Key, bool) {
	if !role.link() || !occurrence.Available() {
		return composition.Key{}, false
	}
	id := framedContentID(linkRuleOccurrenceDomain, ruleSourceIdentityVersion, func(writer *canonical.DigestWriter) bool {
		return writeRuleSlotCapability(writer, role) && writeContentIDs(writer, occurrence)
	})
	if !id.Available() {
		return composition.Key{}, false
	}
	return artifactSourceKey(artifactOccurrenceSource, id)
}

func linkRuleMemberID(role RuleSlotCapability, owner, point, occurrence identity.ContentID) identity.ContentID {
	if !role.link() || !owner.Available() || !point.Available() || !occurrence.Available() {
		return identity.ContentID{}
	}
	return framedContentID(linkRuleMemberDomain, ruleSourceIdentityVersion, func(writer *canonical.DigestWriter) bool {
		return writeRuleSlotCapability(writer, role) && writeContentIDs(writer, owner, point, occurrence)
	})
}

// mountedRuleOccurrenceKey keeps the Batch occurrence entity family-local.
// One authored occurrence can feed several closed Rule roles; sharing the
// raw artifact ID would alias those independent semantic rows.
func mountedRuleOccurrenceKey(role RuleSlotCapability, occurrence identity.ContentID) (composition.Key, bool) {
	if !role.mounted() || !occurrence.Available() {
		return composition.Key{}, false
	}
	id := framedContentID(mountedRuleOccurrenceDomain, ruleSourceIdentityVersion, func(writer *canonical.DigestWriter) bool {
		return writeRuleSlotCapability(writer, role) && writeContentIDs(writer, occurrence)
	})
	if !id.Available() {
		return composition.Key{}, false
	}
	return artifactSourceKey(artifactOccurrenceSource, id)
}

// Typed implementation adapters consume only the opaque source receipt.
func (implementation *RuleImplementation[K, V, O]) beginReceiptRuleRow(source ruleSurfaceSource) (*bindingRuleRowDraft, bool) {
	draft, ok := implementation.beginBindingRuleRow(source.value)
	if !ok {
		source.builder.recordRuleFinalizerFailure(RuleFinalizerFailureBeginDraft)
		return nil, false
	}
	draft.builder = source.builder
	return draft, true
}

func (implementation *ActivationRuleImplementation) beginReceiptRuleRow(source ruleSurfaceSource) (*bindingRuleRowDraft, bool) {
	draft, ok := implementation.beginBindingRuleRow(source.value)
	if !ok {
		source.builder.recordRuleFinalizerFailure(RuleFinalizerFailureBeginDraft)
		return nil, false
	}
	draft.builder = source.builder
	return draft, true
}

func (implementation *ActivationRuleImplementation) receiptReadPart(source ruleSurfaceSource, index uint64) (bindingRuleReadPart, bool) {
	part, ok := implementation.ReadPart(source.value, index)
	if !ok {
		source.builder.recordRuleFinalizerFailure(RuleFinalizerFailureReadPart)
	}
	return part, ok
}

func (implementation *RuleImplementation[K, V, O]) receiptReadPart(source ruleSurfaceSource, index uint64) (bindingRuleReadPart, bool) {
	part, ok := implementation.ReadPart(source.value, index)
	if !ok {
		source.builder.recordRuleFinalizerFailure(RuleFinalizerFailureReadPart)
	}
	return part, ok
}

func (implementation *RuleImplementation[K, V, O]) receiptCarryPart(source ruleSurfaceSource, index uint64) (bindingRuleCarryPart, bool) {
	part, ok := implementation.CarryPart(source.value, index)
	if !ok {
		source.builder.recordRuleFinalizerFailure(RuleFinalizerFailureCarryPart)
	}
	return part, ok
}

func (implementation *RuleImplementation[K, V, O]) receiptWritePart(source ruleSurfaceSource, index uint64) (bindingRuleWritePart, bool) {
	part, ok := implementation.WritePart(source.value, index)
	if !ok {
		source.builder.recordRuleFinalizerFailure(RuleFinalizerFailureWritePart)
	}
	return part, ok
}

func (implementation *RuleImplementation[K, V, O]) receiptSupportPart(source ruleSurfaceSource, index uint64) (bindingRuleSupportPart, bool) {
	part, ok := implementation.SupportPart(source.value, index)
	if !ok {
		source.builder.recordRuleFinalizerFailure(RuleFinalizerFailureSupportPart)
	}
	return part, ok
}

func (implementation *RuleImplementation[K, V, O]) receiptPrunePart(source ruleSurfaceSource, index uint64) (bindingRulePrunePart, bool) {
	part, ok := implementation.PrunePart(source.value, index)
	if !ok {
		source.builder.recordRuleFinalizerFailure(RuleFinalizerFailurePrunePart)
	}
	return part, ok
}
