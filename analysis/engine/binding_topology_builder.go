package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"sync"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

type queuedRuleFinalizer struct {
	mounted RuleSlotCapability
	link    RuleSlotCapability
	run     func() bool
}

// RuleFinalizerFailure is the closed post-source step that rejected a queued
// typed rule row. It identifies only the generic receipt operation; the
// analyzer separately maps the opaque capability back to its domain owner.
type RuleFinalizerFailure uint8

const (
	RuleFinalizerFailureNone RuleFinalizerFailure = iota
	RuleFinalizerFailureBeginDraft
	RuleFinalizerFailureReadPart
	RuleFinalizerFailureCarryPart
	RuleFinalizerFailureWritePart
	RuleFinalizerFailureSupportPart
	RuleFinalizerFailurePrunePart
	RuleFinalizerFailureDraftRead
	RuleFinalizerFailureDraftCarry
	RuleFinalizerFailureDraftWrite
	RuleFinalizerFailureDraftSupport
	RuleFinalizerFailureDraftPrune
	RuleFinalizerFailureAddRuleDraft
	RuleFinalizerFailureIssueRuleRow
	RuleFinalizerFailureAddRuleArguments
	RuleFinalizerFailureAddRuleFence
	RuleFinalizerFailureAddSemanticRule
	RuleFinalizerFailureAddSemanticActivation
)

type receiptSealFailurePhase uint8

const (
	receiptSealFailureNone receiptSealFailurePhase = iota
	receiptSealFailureSources
	receiptSealFailureArtifactRows
	receiptSealFailureRuleFinalizer
	receiptSealFailureQueryBatch
)

// receiptSourceSealFailure is the generic equation Batch predicate that
// rejected source sealing. It is re-exported here so analyzer diagnostics do
// not expose equation internals or source row payloads.
type receiptSourceSealFailure = equation.SealFailure

var (
	receiptSourceSealFailurePrecondition  = equation.SealFailureSourcePrecondition
	receiptSourceSealFailureBatchIdentity = equation.SealFailureSourceBatchIdentity
)

// receiptSealFailure is detached scalar evidence for the first failed source
// seal boundary. Exactly one of the two capability planes is present for a
// finalizer failure; no callback, occurrence, or topology row escapes.
type receiptSealFailure struct {
	phase     receiptSealFailurePhase
	ordinal   uint32
	source    receiptSourceSealFailure
	rule      RuleSourceSealFailure
	finalizer RuleFinalizerFailure
	mounted   RuleSlotCapability
	link      RuleSlotCapability
	artifact  receiptArtifactRowFailure
}

func (failure receiptSealFailure) Phase() receiptSealFailurePhase { return failure.phase }
func (failure receiptSealFailure) Ordinal() uint32                { return failure.ordinal }
func (failure receiptSealFailure) Source() (receiptSourceSealFailure, bool) {
	return failure.source, failure.phase == receiptSealFailureSources && failure.source.Available()
}
func (failure receiptSealFailure) RuleSource() (RuleSourceSealFailure, bool) {
	return failure.rule, failure.phase == receiptSealFailureRuleFinalizer && failure.rule != RuleSourceSealFailureNone
}
func (failure receiptSealFailure) Finalizer() (RuleFinalizerFailure, bool) {
	return failure.finalizer, failure.phase == receiptSealFailureRuleFinalizer && failure.finalizer != RuleFinalizerFailureNone
}
func (failure receiptSealFailure) MountedCapability() (RuleSlotCapability, bool) {
	return failure.mounted, failure.phase == receiptSealFailureRuleFinalizer && failure.mounted.mounted() && !failure.link.available()
}
func (failure receiptSealFailure) LinkCapability() (RuleSlotCapability, bool) {
	return failure.link, failure.phase == receiptSealFailureRuleFinalizer && failure.link.link() && !failure.mounted.available()
}
func (failure receiptSealFailure) ArtifactRow() (receiptArtifactRowFailure, bool) {
	return failure.artifact, failure.phase == receiptSealFailureArtifactRows && failure.artifact != receiptArtifactRowFailureNone
}

// Failure projects the whole seal boundary onto the engine's public failure
// vocabulary. Every scalar coordinate enters the site preimage, so the digest
// separates two seal boundaries that differ in any one of them.
func (failure receiptSealFailure) Failure() SolveFailure {
	if failure.phase == receiptSealFailureNone {
		return SolveFailure{}
	}
	sourceFamily, sourceSite := failure.source.Ordinals()
	return receiptFailure(SolveFailureFamilyCompile, "receipt-seal",
		uint64(failure.phase), uint64(failure.ordinal), sourceFamily, sourceSite,
		uint64(failure.rule), uint64(failure.finalizer), uint64(failure.artifact))
}

type mountedSelectedSurfaceAnchor struct {
	builder    *BindingTopologyBuilder
	occurrence equation.Occurrence
	operand    equation.Operand
	rule       uint64
	index      uint64
	form       equation.SurfaceForm
}

func (builder *BindingTopologyBuilder) claimMountedSurface(surface equation.Surface, anchor mountedSelectedSurfaceAnchor) bool {
	if builder == nil || !surface.Available() {
		return false
	}
	builder.selectedSurfaceMu.Lock()
	defer builder.selectedSurfaceMu.Unlock()
	if _, found := builder.selectedSurfaceAnchor[surface]; found {
		return false
	}
	builder.selectedSurfaceAnchor[surface] = anchor
	return true
}

func (builder *BindingTopologyBuilder) claimMountedSelectedSurface(surface equation.Surface, anchor mountedSelectedSurfaceAnchor) bool {
	return builder.claimMountedSurface(surface, anchor)
}

func (builder *BindingTopologyBuilder) Abort() bool {
	return builder.abort()
}

// QueueMountedRuleFinalizer retains one typed, fully-admitted mounted source row until the
// source Batch seals. ruleSurfaceSource deliberately requires that
// sealed Batch; finalizers therefore run only after lowerArtifactRows opens
// topology construction. The closure remains opaque to the engine and can
// only close over exact owner-issued transactions and implementations.
func (builder *BindingTopologyBuilder) QueueMountedRuleFinalizer(role RuleSlotCapability, finalize func() bool) bool {
	if !role.mounted() {
		return false
	}
	return builder.queueRuleFinalizer(queuedRuleFinalizer{mounted: role, run: finalize})
}

func (builder *BindingTopologyBuilder) QueueLinkRuleFinalizer(role RuleSlotCapability, finalize func() bool) bool {
	if !role.link() {
		return false
	}
	return builder.queueRuleFinalizer(queuedRuleFinalizer{link: role, run: finalize})
}

func (builder *BindingTopologyBuilder) queueRuleFinalizer(finalizer queuedRuleFinalizer) bool {
	if builder == nil || finalizer.run == nil || (finalizer.mounted.mounted() == finalizer.link.link()) {
		return false
	}
	inner, ok := builder.lockSourcesOpen()
	if !ok {
		return false
	}
	inner.mu.Unlock()
	builder.queuedRuleMu.Lock()
	defer builder.queuedRuleMu.Unlock()
	builder.queuedRuleFinalizers = append(builder.queuedRuleFinalizers, finalizer)
	return true
}

func (builder *BindingTopologyBuilder) drainRuleFinalizers() bool {
	builder.queuedRuleMu.Lock()
	finalizers := builder.queuedRuleFinalizers
	builder.queuedRuleFinalizers = nil
	builder.queuedRuleMu.Unlock()
	for index, finalizer := range finalizers {
		builder.recordRuleSourceSealFailure(RuleSourceSealFailureNone)
		builder.recordRuleFinalizerFailure(RuleFinalizerFailureNone)
		if finalizer.run == nil || !finalizer.run() {
			builder.sealFailure = receiptSealFailure{phase: receiptSealFailureRuleFinalizer, ordinal: uint32(index), rule: builder.currentRuleSourceSealFailure(), finalizer: builder.currentRuleFinalizerFailure(), mounted: finalizer.mounted, link: finalizer.link}
			return false
		}
	}
	return true
}

// QueueMountedQueryBatch retains one query batch until the source Batch
// seals. SealSources hands the batch scope back at exactly the point where
// query rows are admissible, so a caller never orders query admission against
// source sealing by hand.
func (builder *BindingTopologyBuilder) QueueMountedQueryBatch(emit func(*MountedQueryBatch) bool) bool {
	if builder == nil || emit == nil {
		return false
	}
	inner, ok := builder.lockSourcesOpen()
	if !ok {
		return false
	}
	inner.mu.Unlock()
	builder.queuedQueryMu.Lock()
	defer builder.queuedQueryMu.Unlock()
	builder.queuedQueryBatches = append(builder.queuedQueryBatches, emit)
	return true
}

func (builder *BindingTopologyBuilder) drainQueryBatches() (uint32, bool) {
	builder.queuedQueryMu.Lock()
	batches := builder.queuedQueryBatches
	builder.queuedQueryBatches = nil
	builder.queuedQueryMu.Unlock()
	for index, emit := range batches {
		batch := &MountedQueryBatch{builder: builder, draining: true}
		admitted := emit != nil && emit(batch)
		batch.draining = false
		if !admitted {
			return uint32(index), false
		}
	}
	return 0, true
}

func (builder *BindingTopologyBuilder) recordRuleSourceSealFailure(failure RuleSourceSealFailure) {
	if builder == nil {
		return
	}
	builder.ruleSourceFailureMu.Lock()
	builder.ruleSourceFailure = failure
	builder.ruleSourceFailureMu.Unlock()
}

func (builder *BindingTopologyBuilder) currentRuleSourceSealFailure() RuleSourceSealFailure {
	if builder == nil {
		return RuleSourceSealFailureNone
	}
	builder.ruleSourceFailureMu.Lock()
	defer builder.ruleSourceFailureMu.Unlock()
	return builder.ruleSourceFailure
}

func (builder *BindingTopologyBuilder) recordRuleFinalizerFailure(failure RuleFinalizerFailure) {
	if builder == nil {
		return
	}
	builder.finalizerFailureMu.Lock()
	builder.finalizerFailure = failure
	builder.finalizerFailureMu.Unlock()
}

func (builder *BindingTopologyBuilder) currentRuleFinalizerFailure() RuleFinalizerFailure {
	if builder == nil {
		return RuleFinalizerFailureNone
	}
	builder.finalizerFailureMu.Lock()
	defer builder.finalizerFailureMu.Unlock()
	return builder.finalizerFailure
}

// SealSources freezes the one exact source Batch owned by this builder. It
// ends source admission while opening topology-row construction against those
// same immutable Site/Occurrence/Operand identities.
func (builder *BindingTopologyBuilder) SealSources() bool {
	if builder == nil {
		return false
	}
	if sourceFailure := builder.sealSources(); sourceFailure.Available() {
		builder.sealFailure = receiptSealFailure{phase: receiptSealFailureSources, source: sourceFailure}
		return false
	}
	artifactFailure, artifactOrdinal, artifactOK := builder.lowerArtifactRows()
	if !artifactOK {
		builder.sealFailure = receiptSealFailure{phase: receiptSealFailureArtifactRows, artifact: artifactFailure, ordinal: artifactOrdinal}
		builder.abort()
		return false
	}
	if !builder.drainRuleFinalizers() {
		builder.abort()
		return false
	}
	if ordinal, drained := builder.drainQueryBatches(); !drained {
		builder.sealFailure = receiptSealFailure{phase: receiptSealFailureQueryBatch, ordinal: ordinal}
		builder.abort()
		return false
	}
	return true
}

func (builder *BindingTopologyBuilder) SealFailure() (receiptSealFailure, bool) {
	if builder == nil || builder.sealFailure.phase == receiptSealFailureNone {
		return receiptSealFailure{}, false
	}
	return builder.sealFailure, true
}

type bindingPointRowReceipt struct {
	builder   *bindingTopologyBuilderState
	state     *schemaBindingState
	authority *schemaBindingAuthority
	ordinal   uint64
	row       equation.PointSpec
}

type bindingPointRowRef struct {
	builder *bindingTopologyBuilderState
	ref     equation.PointRef
}

type bindingQueryRowReceipt struct {
	builder   *bindingTopologyBuilderState
	state     *schemaBindingState
	authority *schemaBindingAuthority
	ordinal   uint64
	issuer    bindingQueryReceipt
	row       equation.QueryInstance
}

type bindingQueryRowRef struct {
	builder *bindingTopologyBuilderState
	ordinal uint64
}

type bindingEnvironmentEdge struct {
	builder   *bindingTopologyBuilderState
	row       equation.EnvironmentEdge
	state     *schemaBindingState
	authority *schemaBindingAuthority
}

type bindingFactorEdge struct {
	builder   *bindingTopologyBuilderState
	row       equation.FactorEdge
	state     *schemaBindingState
	authority *schemaBindingAuthority
	factor    composition.Key
}

type bindingMaterialization struct {
	builder   *bindingTopologyBuilderState
	value     equation.TemplateMaterialization
	base      *equation.Batch
	state     *schemaBindingState
	authority *schemaBindingAuthority
}

// bindingDirectActivation is private builder authority for a direct
// candidate over already-lowered mounted artifact points. Domain code cannot
// manufacture it from transport scalars.
type bindingDirectActivation struct {
	builder   *bindingTopologyBuilderState
	value     equation.DirectActivationCandidate
	base      *equation.Batch
	state     *schemaBindingState
	authority *schemaBindingAuthority
}

// bindingRuleRow is the complete, Binding-issued structural row
// authority. The equation RuleInstance is retained only behind this opaque
// handle; AddRule never accepts a raw row.
type bindingRuleRow struct {
	builder     *bindingTopologyBuilderState
	state       *schemaBindingState
	authority   *schemaBindingAuthority
	ordinal     uint64
	row         equation.RuleInstance
	input       equation.Site
	inputID     identity.ContentID
	stage       rows.ArtifactRuleStage
	predecessor *artifactEnvironmentRow
	routed      bool
}

type bindingRuleRowRef struct {
	builder *bindingTopologyBuilderState
	ref     equation.RuleRef
}

// bindingRuleRowDraft is a typed, cell-owned row assembly handle. Its
// equation row is private and can only be populated through the indexed shape
// methods below.
type bindingRuleRowDraft struct {
	mu             sync.Mutex
	state          *schemaBindingState
	authority      *schemaBindingAuthority
	receipt        bindingRuleReceipt
	source         equation.SurfaceSource
	sourceIdentity equation.SurfaceSource
	row            equation.RuleInstance
	consumed       bool
	builder        *BindingTopologyBuilder
}

type bindingRuleReadPart struct {
	issuer         bindingRuleReceipt
	sourceIdentity equation.SurfaceSource
	state          *schemaBindingState
	authority      *schemaBindingAuthority
	index          uint64
	surface        equation.Surface
}
type bindingRuleCarryPart struct {
	issuer         bindingRuleReceipt
	sourceIdentity equation.SurfaceSource
	state          *schemaBindingState
	authority      *schemaBindingAuthority
	index          uint64
}
type bindingRuleWritePart struct {
	issuer         bindingRuleReceipt
	sourceIdentity equation.SurfaceSource
	state          *schemaBindingState
	authority      *schemaBindingAuthority
	index          uint64
	value          equation.ResolvedWrite
}
type bindingRuleSupportPart struct {
	issuer         bindingRuleReceipt
	sourceIdentity equation.SurfaceSource
	state          *schemaBindingState
	authority      *schemaBindingAuthority
	index          uint64
	value          equation.ResolvedSupport
}
type bindingRulePrunePart struct {
	issuer         bindingRuleReceipt
	sourceIdentity equation.SurfaceSource
	state          *schemaBindingState
	authority      *schemaBindingAuthority
	index          uint64
	value          equation.ResolvedPrune
}

func (implementation *RuleImplementation[K, V, O]) beginBindingRuleRow(source equation.SurfaceSource) (*bindingRuleRowDraft, bool) {
	state, authority, semantic, ok := implementation.boundTopologyRuleReceipt()
	if !ok || !source.Occurrence().Available() || !source.Operand().Available() || !source.Operand().Occurrence().Same(source.Occurrence()) || source.Rule() != semantic {
		return nil, false
	}
	return &bindingRuleRowDraft{state: state, authority: authority, receipt: implementation, source: source, row: equation.RuleInstance{Schema: semantic, OperandFamily: implementation.binding.proof.operandFamily, Occurrence: source.Occurrence(), Operand: source.Operand()}}, true
}

func (implementation *RuleImplementation[K, V, O]) ReadPart(source equation.SurfaceSource, index uint64) (bindingRuleReadPart, bool) {
	state, authority, _, ok := implementation.boundTopologyRuleReceipt()
	if !ok {
		return bindingRuleReadPart{}, false
	}
	value, ok := source.ReadAt(index)
	if !ok {
		return bindingRuleReadPart{}, false
	}
	return bindingRuleReadPart{issuer: implementation, sourceIdentity: source, state: state, authority: authority, index: index, surface: value.Surface}, true
}

func (implementation *ActivationRuleImplementation) ReadPart(source equation.SurfaceSource, index uint64) (bindingRuleReadPart, bool) {
	state, authority, _, ok := implementation.boundTopologyRuleReceipt()
	if !ok {
		return bindingRuleReadPart{}, false
	}
	value, ok := source.ReadAt(index)
	if !ok {
		return bindingRuleReadPart{}, false
	}
	return bindingRuleReadPart{issuer: implementation, sourceIdentity: source, state: state, authority: authority, index: index, surface: value.Surface}, true
}

func (implementation *RuleImplementation[K, V, O]) CarryPart(source equation.SurfaceSource, index uint64) (bindingRuleCarryPart, bool) {
	state, authority, _, ok := implementation.boundTopologyRuleReceipt()
	if !ok {
		return bindingRuleCarryPart{}, false
	}
	if _, ok := source.CarryAt(index); !ok || source.Rule() != implementation.binding.proof.semantic {
		return bindingRuleCarryPart{}, false
	}
	return bindingRuleCarryPart{issuer: implementation, sourceIdentity: source, state: state, authority: authority, index: index}, true
}

func (implementation *RuleImplementation[K, V, O]) WritePart(source equation.SurfaceSource, index uint64) (bindingRuleWritePart, bool) {
	state, authority, _, ok := implementation.boundTopologyRuleReceipt()
	if !ok {
		return bindingRuleWritePart{}, false
	}
	value, ok := source.WriteAt(index)
	if !ok || source.Rule() != implementation.binding.proof.semantic {
		return bindingRuleWritePart{}, false
	}
	return bindingRuleWritePart{issuer: implementation, sourceIdentity: source, state: state, authority: authority, index: index, value: value}, true
}

func (implementation *RuleImplementation[K, V, O]) SupportPart(source equation.SurfaceSource, index uint64) (bindingRuleSupportPart, bool) {
	state, authority, _, ok := implementation.boundTopologyRuleReceipt()
	value, sourceOK := source.SupportAt(index)
	if !ok || !sourceOK || source.Rule() != implementation.binding.proof.semantic {
		return bindingRuleSupportPart{}, false
	}
	return bindingRuleSupportPart{issuer: implementation, sourceIdentity: source, state: state, authority: authority, index: index, value: value}, true
}

func (implementation *RuleImplementation[K, V, O]) PrunePart(source equation.SurfaceSource, index uint64) (bindingRulePrunePart, bool) {
	state, authority, _, ok := implementation.boundTopologyRuleReceipt()
	value, sourceOK := source.PruneAt(index)
	if !ok || !sourceOK || source.Rule() != implementation.binding.proof.semantic {
		return bindingRulePrunePart{}, false
	}
	return bindingRulePrunePart{issuer: implementation, sourceIdentity: source, state: state, authority: authority, index: index, value: value}, true
}

func (draft *bindingRuleRowDraft) AddRead(receipt bindingRuleReadPart) (ok bool) {
	defer func() {
		if !ok && draft != nil {
			draft.builder.recordRuleFinalizerFailure(RuleFinalizerFailureDraftRead)
		}
	}()
	if draft == nil {
		return false
	}
	draft.mu.Lock()
	defer draft.mu.Unlock()
	if draft.consumed {
		return false
	}
	if draft == nil || draft.state == nil || receipt.issuer != draft.receipt || !receipt.sourceIdentity.Same(draft.source) || receipt.state != draft.state || receipt.authority != draft.authority || receipt.index != uint64(len(draft.row.Reads)) || !receipt.surface.Available() {
		return false
	}
	draft.row.Reads = append(draft.row.Reads, equation.ResolvedRead{Index: receipt.index, Surface: receipt.surface})
	return true
}

func (draft *bindingRuleRowDraft) AddCarry(receipt bindingRuleCarryPart) (ok bool) {
	defer func() {
		if !ok && draft != nil {
			draft.builder.recordRuleFinalizerFailure(RuleFinalizerFailureDraftCarry)
		}
	}()
	if draft == nil {
		return false
	}
	draft.mu.Lock()
	defer draft.mu.Unlock()
	if draft.consumed {
		return false
	}
	if draft == nil || draft.state == nil || receipt.issuer != draft.receipt || !receipt.sourceIdentity.Same(draft.source) || receipt.state != draft.state || receipt.authority != draft.authority || receipt.index != uint64(len(draft.row.Carries)) {
		return false
	}
	draft.row.Carries = append(draft.row.Carries, equation.ResolvedCarry{Index: receipt.index})
	return true
}

func (draft *bindingRuleRowDraft) AddWrite(receipt bindingRuleWritePart) (ok bool) {
	defer func() {
		if !ok && draft != nil {
			draft.builder.recordRuleFinalizerFailure(RuleFinalizerFailureDraftWrite)
		}
	}()
	if draft == nil {
		return false
	}
	draft.mu.Lock()
	defer draft.mu.Unlock()
	if draft.consumed {
		return false
	}
	if draft == nil || draft.state == nil || receipt.issuer != draft.receipt || !receipt.sourceIdentity.Same(draft.source) || receipt.state != draft.state || receipt.authority != draft.authority || receipt.index != uint64(len(draft.row.Writes)) {
		return false
	}
	draft.row.Writes = append(draft.row.Writes, receipt.value)
	return true
}

// These sealed receipt interfaces are intentionally unnameable outside the
// engine package. Public typed implementations satisfy them, so a builder can
// authenticate exact Link owners without accepting an erased slot or key.
type bindingRuleReceipt interface {
	boundTopologyRuleReceipt() (*schemaBindingState, *schemaBindingAuthority, composition.Key, bool)
}

type bindingFactorReceipt interface {
	boundTopologyFactorReceipt() (*schemaBindingState, *schemaBindingAuthority, composition.Key, bool)
}

func (implementation *RuleImplementation[K, V, O]) boundTopologyRuleReceipt() (*schemaBindingState, *schemaBindingAuthority, composition.Key, bool) {
	if implementation == nil || !implementation.binding.valid() {
		return nil, nil, composition.Key{}, false
	}
	return implementation.binding.state, implementation.binding.authority, implementation.binding.proof.semantic, true
}

func (implementation *ActivationRuleImplementation) boundTopologyRuleReceipt() (*schemaBindingState, *schemaBindingAuthority, composition.Key, bool) {
	if implementation == nil || !implementation.binding.valid() {
		return nil, nil, composition.Key{}, false
	}
	return implementation.binding.state, implementation.binding.authority, implementation.binding.proof.semantic, true
}

// beginBindingRuleRow issues the structural trigger row from the exact sealed
// activation implementation. Candidate materializations are admitted later
// and cannot author or replace this member.
func (implementation *ActivationRuleImplementation) beginBindingRuleRow(source equation.SurfaceSource) (*bindingRuleRowDraft, bool) {
	state, authority, semantic, ok := implementation.boundTopologyRuleReceipt()
	if !ok || !source.Occurrence().Available() || !source.Operand().Available() || !source.Operand().Occurrence().Same(source.Occurrence()) || source.Rule() != semantic {
		return nil, false
	}
	return &bindingRuleRowDraft{state: state, authority: authority, receipt: implementation, source: source, row: equation.RuleInstance{Schema: semantic, OperandFamily: implementation.binding.proof.operandFamily, Occurrence: source.Occurrence(), Operand: source.Operand()}}, true
}

func (implementation *FactorImplementation[K, V]) boundTopologyFactorReceipt() (*schemaBindingState, *schemaBindingAuthority, composition.Key, bool) {
	if implementation == nil || !implementation.binding.validForms() {
		return nil, nil, composition.Key{}, false
	}
	return implementation.binding.state, implementation.binding.authority, implementation.binding.semantic, true
}

func bindingOwnsFactorSchema(schema *Schema, key composition.Key) bool {
	_, ok := schema.factorOrdinalOf(key)
	return ok
}

func bindingOwnsRuleSchema(schema *Schema, key composition.Key) bool {
	_, ok := schema.ruleOrdinalOf(key)
	return ok
}

func bindingOwnsQuerySchema(schema *Schema, key composition.Key) bool {
	if schema == nil || !key.Available() {
		return false
	}
	_, ok := schema.queryOrdinalOf(key)
	return ok
}

func validateBindingRuleRows(schema *Schema, rule equation.RuleInstance) bool {
	if schema == nil || !rule.ValidFor(schema.cold) {
		return false
	}
	ordinal, found := schema.ruleOrdinalOf(rule.Schema)
	shape, shapeOK := schema.ruleShapeAt(ordinal)
	if !found || !shapeOK || rule.OperandFamily != shape.OperandFamily || uint64(len(rule.Reads)) != shape.ReadCount || uint64(len(rule.Carries)) != shape.CarryCount || uint64(len(rule.Writes)) != shape.WriteCount {
		return false
	}
	for index, read := range rule.Reads {
		want, ok := schema.ruleReadShapeAt(ordinal, uint64(index))
		if !ok || read.Index != uint64(index) || !validBindingReadSurface(want, read.Surface) {
			return false
		}
	}
	for index, carry := range rule.Carries {
		want, ok := schema.ruleCarryShapeAt(ordinal, uint64(index))
		if !ok || carry.Index != uint64(index) || !bindingOwnsFactorSchema(schema, want.Factor) {
			return false
		}
	}
	for index, write := range rule.Writes {
		want, ok := schema.ruleWriteShapeAt(ordinal, uint64(index))
		if !ok || write.Index != uint64(index) || !validBindingWriteSurface(want, write) {
			return false
		}
	}
	return true
}

func validBindingReadSurface(shape composition.RuleReadShape, surface equation.Surface) bool {
	if !surface.Available() || surface.Factor != shape.Factor || surface.Mode != equation.TargetModeNone {
		return false
	}
	switch shape.Kind {
	case composition.ReadExact:
		return surface.Form == equation.SurfaceReadExact && !surface.Semantic.Available() && !surface.Normalizer.Available()
	case composition.ReadSummary:
		return surface.Form == equation.SurfaceReadSummary && surface.Semantic == shape.Normalizer && surface.Normalizer == shape.Normalizer && shape.Normalizer.Available()
	case composition.ReadSelect:
		return surface.Form == equation.SurfaceReadSelect && surface.Semantic == shape.Semantic && !surface.Normalizer.Available() && shape.Semantic.Available()
	default:
		return false
	}
}

func validBindingWriteSurface(shape composition.RuleWriteShape, write equation.ResolvedWrite) bool {
	if !write.Surface.Available() || write.Surface.Factor != shape.Factor || write.Route != shape.Route {
		return false
	}
	switch shape.Kind {
	case composition.WriteExact:
		return write.Surface.Form == equation.SurfaceWriteExact
	case composition.WriteRoute:
		return write.Surface.Form == equation.SurfaceWriteRoute
	default:
		return false
	}
}

func bindingOwnsInput(batch *equation.Batch, input equation.Input) bool {
	return batch != nil && input.Available() && batch.OwnsSite(input.Source()) && batch.OwnsSite(input.Target())
}

// CommittedProgramFrom lifts a topology and graph already issued by a
// same-package commit. Production assemble constructs CommittedProgram from
// sealed tables.
func CommittedProgramFrom(topology *BindingTopology, graph *equation.Graph) *CommittedProgram {
	if topology == nil || graph == nil {
		return nil
	}
	return newCommittedProgram(graph, topology, topology.state, topology.authority)
}

// beginBindingTopologyBuilder creates one solve-local topology transaction
// under this exact sealed Binding. A sealed Binding is immutable and reusable;
// every returned builder owns an independent Batch and lifecycle so repeated
// and concurrent Plan solves never rebuild the Link-local factor/rule owners.
func (binding *SchemaBinding) beginBindingTopologyBuilder() (*BindingTopologyBuilder, bool) {
	state := bindingState(binding)
	if state == nil {
		return nil, false
	}
	state.mu.Lock()
	if state.phase != schemaBindingSealed || state.authority == nil || state.schema == nil || len(state.factors) != schemaFactorCount(state.schema) {
		state.mu.Unlock()
		return nil, false
	}
	for _, factor := range state.factors {
		if factor == nil || !factor.schemaFactorComplete() {
			state.mu.Unlock()
			return nil, false
		}
	}
	inner := &bindingTopologyBuilderState{
		state: state, batch: equation.NewBatch(), phase: bindingTopologyBuilderSourcesOpen,
		authority: state.authority,
		semantic: &bindingSemanticRows{
			ids: make(map[identity.ContentID]bindingSemanticRowKind), points: make(map[identity.ContentID]equation.PointRef), pointAt: make(map[equation.Site]identity.ContentID),
			members: make(map[identity.ContentID]equation.RuleRef), memberAt: make(map[equation.RuleRef]identity.ContentID), queries: make(map[identity.ContentID]uint64),
			activations: make(map[identity.ContentID]equation.RuleRef), activationAt: make(map[equation.RuleRef]identity.ContentID),
			materializationAt: make(map[equation.TemplateMaterialization]equation.RuleRef), directCandidateAt: make(map[equation.DirectActivationCandidate]equation.RuleRef), directCandidateKey: make(map[composition.Key]equation.RuleRef), activationCandidates: make(map[equation.RuleRef]uint64), activationExpected: make(map[equation.RuleRef]uint64),
			activationApplication: make(map[equation.RuleRef]composition.Key),
		},
		directTransportSets: make(map[directActivationTransportSetKey]equation.DirectActivationTransportSet),
	}
	inner.spec.Batch = inner.batch
	state.mu.Unlock()
	return &BindingTopologyBuilder{
		inner:                 inner,
		binding:               binding,
		selectedSurfaceAnchor: make(map[equation.Surface]mountedSelectedSurfaceAnchor),
	}, true
}

func (builder *BindingTopologyBuilder) lockPhase(phase bindingTopologyBuilderPhase) (*bindingTopologyBuilderState, bool) {
	if builder == nil || builder.inner == nil {
		return nil, false
	}
	inner := builder.inner
	inner.mu.Lock()
	if inner.phase != phase || inner.batch == nil || inner.state == nil || inner.state.phase != schemaBindingSealed || inner.state.authority != inner.authority {
		inner.mu.Unlock()
		return nil, false
	}
	return inner, true
}

func (builder *BindingTopologyBuilder) lockSourcesOpen() (*bindingTopologyBuilderState, bool) {
	return builder.lockPhase(bindingTopologyBuilderSourcesOpen)
}

func (builder *BindingTopologyBuilder) lockTopologyOpen() (*bindingTopologyBuilderState, bool) {
	inner, ok := builder.lockPhase(bindingTopologyBuilderTopologyOpen)
	if !ok {
		return nil, false
	}
	if !inner.batch.Sealed() || !inner.sourceKey.Available() || inner.batch.Key() != inner.sourceKey || inner.spec.Batch != inner.batch {
		inner.failLocked()
		inner.mu.Unlock()
		return nil, false
	}
	return inner, true
}

func (builder *BindingTopologyBuilder) admitSite(source composition.Key, scope equation.Scope, init equation.Expr, disposition equation.InitDisposition) (equation.Site, bool) {
	inner, ok := builder.lockSourcesOpen()
	if !ok {
		return equation.Site{}, false
	}
	defer inner.mu.Unlock()
	return inner.batch.AdmitSite(source, scope, init, disposition)
}

func (builder *BindingTopologyBuilder) admitAt(site equation.Site) (equation.Occurrence, bool) {
	inner, ok := builder.lockSourcesOpen()
	if !ok {
		return equation.Occurrence{}, false
	}
	defer inner.mu.Unlock()
	return inner.batch.At(site)
}

func (builder *BindingTopologyBuilder) admitFrom(site equation.Site, entity composition.Key) (equation.Occurrence, bool) {
	inner, ok := builder.lockSourcesOpen()
	if !ok {
		return equation.Occurrence{}, false
	}
	defer inner.mu.Unlock()
	return inner.batch.From(site, entity)
}

func (builder *BindingTopologyBuilder) admitOperand(occurrence equation.Occurrence, entity composition.Key) (equation.Operand, bool) {
	inner, ok := builder.lockSourcesOpen()
	if !ok {
		return equation.Operand{}, false
	}
	defer inner.mu.Unlock()
	return inner.batch.AdmitOperand(occurrence, entity)
}

// IssueRuleSurfaceSource authenticates a complete typed row specification
// against this builder's exact sealed Binding composition and source Batch.
// The caller cannot provide an equation.RuleInstance or its private witness;
// the Batch is the only authority that mints the immutable source receipt.
func (builder *BindingTopologyBuilder) issueRuleSurfaceSource(spec equation.RuleSurfaceSourceSpec) (equation.SurfaceSource, bool) {
	inner, ok := builder.lockTopologyOpen()
	if !ok {
		return equation.SurfaceSource{}, false
	}
	defer inner.mu.Unlock()
	return inner.batch.IssueRuleSurfaceSource(inner.state.schema.cold, spec)
}

func (builder *BindingTopologyBuilder) addRow(row func(*equation.TopologySpec) bool) bool {
	inner, ok := builder.lockTopologyOpen()
	if !ok {
		return false
	}
	defer inner.mu.Unlock()
	return row(&inner.spec)
}

func (builder *BindingTopologyBuilder) addPoint(point equation.PointSpec) bool {
	return builder.addRow(func(spec *equation.TopologySpec) bool {
		if !point.Site.Available() || !spec.Batch.OwnsSite(point.Site) {
			return false
		}
		spec.Points = append(spec.Points, point)
		return true
	})
}

func (builder *BindingTopologyBuilder) issuePointRow(point equation.PointSpec) (bindingPointRowReceipt, bool) {
	inner, ok := builder.lockTopologyOpen()
	if !ok {
		return bindingPointRowReceipt{}, false
	}
	valid := point.Site.Available() && inner.batch.OwnsSite(point.Site)
	if !valid {
		inner.failLocked()
		inner.mu.Unlock()
		return bindingPointRowReceipt{}, false
	}
	receipt := bindingPointRowReceipt{builder: inner, state: inner.state, authority: inner.authority, ordinal: uint64(len(inner.spec.Points)), row: point}
	inner.mu.Unlock()
	return receipt, true
}

func claimBindingSemanticID(inner *bindingTopologyBuilderState, id identity.ContentID, kind bindingSemanticRowKind) bool {
	if inner == nil || inner.semantic == nil || !id.Available() || kind < bindingSemanticPoint || kind > bindingSemanticActivation {
		return false
	}
	if _, duplicate := inner.semantic.ids[id]; duplicate {
		return false
	}
	inner.semantic.ids[id] = kind
	return true
}

func (builder *BindingTopologyBuilder) addSemanticPoint(id identity.ContentID, receipt bindingPointRowReceipt) (bindingPointRowRef, bool) {
	inner, ok := builder.lockTopologyOpen()
	if !ok {
		return bindingPointRowRef{}, false
	}
	_, duplicatePoint := inner.semantic.pointAt[receipt.row.Site]
	valid := receipt.builder == inner && receipt.state == inner.state && receipt.authority == inner.authority && receipt.ordinal == uint64(len(inner.spec.Points)) && receipt.row.Site.Available() && inner.batch.OwnsSite(receipt.row.Site) && !duplicatePoint && claimBindingSemanticID(inner, id, bindingSemanticPoint)
	if !valid {
		inner.failLocked()
		inner.mu.Unlock()
		return bindingPointRowRef{}, false
	}
	ref := equation.PointAt(len(inner.spec.Points))
	inner.spec.Points = append(inner.spec.Points, receipt.row)
	inner.semantic.points[id] = ref
	inner.semantic.pointAt[receipt.row.Site] = id
	inner.mu.Unlock()
	return bindingPointRowRef{builder: inner, ref: ref}, true
}

func (builder *BindingTopologyBuilder) issueRuleRow(draft *bindingRuleRowDraft) (bindingRuleRow, bool) {
	inner, ok := builder.lockTopologyOpen()
	if !ok {
		return bindingRuleRow{}, false
	}
	if draft == nil {
		inner.mu.Unlock()
		return bindingRuleRow{}, false
	}
	draft.mu.Lock()
	if draft.consumed {
		draft.mu.Unlock()
		inner.mu.Unlock()
		return bindingRuleRow{}, false
	}
	state, authority, semantic, receiptOK := draft.receipt.boundTopologyRuleReceipt()
	row := draft.row
	valid := draft.state == inner.state && draft.authority == inner.authority && draft.source.ValidFor(inner.state.schema.cold, inner.batch, row.Schema) && receiptOK && state == inner.state && authority == inner.authority && semantic == row.Schema && row.Schema.Available() && row.OperandFamily.Available() && row.Occurrence.Available() && row.Operand.Available() && inner.batch.OwnsOccurrence(row.Occurrence) && inner.batch.OwnsOperand(row.Operand) && row.Operand.Occurrence().Same(row.Occurrence) && bindingOwnsRuleSchema(inner.state.schema, row.Schema) && validateBindingRuleRows(inner.state.schema, row)
	ordinal := uint64(len(inner.spec.Rules))
	if valid {
		draft.consumed = true
	}
	draft.mu.Unlock()
	inner.mu.Unlock()
	if !valid {
		return bindingRuleRow{}, false
	}
	return bindingRuleRow{builder: builder.inner, state: state, authority: authority, ordinal: ordinal, row: cloneBindingRuleRow(row)}, true
}

func (builder *BindingTopologyBuilder) addSemanticRule(id identity.ContentID, receipt bindingRuleRow) (bindingRuleRowRef, bool) {
	inner, ok := builder.lockTopologyOpen()
	if !ok {
		return bindingRuleRowRef{}, false
	}
	ref := equation.RuleAt(len(inner.spec.Rules))
	_, duplicateMember := inner.semantic.memberAt[ref]
	valid := receipt.builder == inner && receipt.state == inner.state && receipt.authority == inner.authority && receipt.ordinal == uint64(len(inner.spec.Rules)) && validateBindingRuleRows(inner.state.schema, receipt.row) && !duplicateMember && claimBindingSemanticID(inner, id, bindingSemanticMember)
	if !valid {
		inner.failLocked()
		inner.mu.Unlock()
		return bindingRuleRowRef{}, false
	}
	pointID, found := inner.semantic.pointAt[receipt.row.Occurrence.Site()]
	output, pointOK := inner.semantic.points[pointID]
	ordinal, shapeOK := inner.state.schema.ruleOrdinalOf(receipt.row.Schema)
	shape, shapeOK := inner.state.schema.ruleShapeAt(ordinal)
	inputs := make([]equation.Input, 0, shape.Inputs)
	if shapeOK && shape.Inputs != 0 {
		source := receipt.input
		target := receipt.row.Occurrence.Site()
		shapeOK = receipt.inputID.Available()
		for input := uint64(0); input < shape.Inputs; input++ {
			provenance, provenanceOK := mountedRuleInputKey(id, receipt.inputID, input)
			var boundary equation.Input
			boundaryOK := false
			if receipt.routed {
				if receipt.predecessor != nil {
					boundary, boundaryOK = artifactPredecessorRuleInput(builder.mountedRows, *receipt.predecessor, source, target, pointID, provenance)
				}
			} else {
				reindex, reindexOK := ruleInputReindex(source.Scope(), target.Scope())
				boundary = equation.BoundaryInput(source, target, provenance, equation.TrueExpr(), reindex, equation.TrueExpr())
				boundaryOK = reindexOK && boundary.Available()
			}
			if !shapeOK || !provenanceOK || !boundaryOK || !boundary.Available() {
				shapeOK = false
				break
			}
			inputs = append(inputs, boundary)
		}
	}
	group := equation.Group{Members: []equation.RuleRef{ref}, Output: output, Inputs: inputs}
	if !found || !pointOK || !shapeOK || uint64(len(inputs)) != shape.Inputs || !validBindingGroup(inner.batch, group) {
		inner.failLocked()
		inner.mu.Unlock()
		return bindingRuleRowRef{}, false
	}
	inner.spec.Rules = append(inner.spec.Rules, cloneBindingRuleRow(receipt.row))
	inner.spec.Groups = append(inner.spec.Groups, group)
	inner.semantic.members[id] = ref
	inner.semantic.memberAt[ref] = id
	inner.mu.Unlock()
	return bindingRuleRowRef{builder: inner, ref: ref}, true
}

func artifactPredecessorRuleInput(rows *mountedArtifactRows, edge artifactEnvironmentRow, source, target equation.Site, targetPoint identity.ContentID, provenance composition.Key) (equation.Input, bool) {
	if rows == nil || !validArtifactRouteProof(edge) || !edge.route.Available() || !provenance.Available() || !source.Available() || !target.Available() || !targetPoint.Available() {
		return equation.Input{}, false
	}
	wantSource, sourceOK := rows.sites[edge.to]
	_, targetOK := rows.pointMeta[targetPoint]
	reindex, reindexOK := ruleInputReindex(source.Scope(), target.Scope())
	input := equation.BoundaryInput(source, target, provenance, equation.TrueExpr(), reindex, equation.TrueExpr())
	if !sourceOK || !source.Same(wantSource) || !targetOK {
		return equation.Input{}, false
	}
	return input, reindexOK && input.Available()
}

func ruleInputReindex(source, target equation.Scope) (equation.Reindex, bool) {
	if !source.Available() || !target.Available() {
		return equation.Reindex{}, false
	}
	targets := make(map[composition.Key]equation.Decision, target.Count())
	for index := 0; index < target.Count(); index++ {
		decision, ok := target.At(index)
		if !ok || !decision.Available() {
			return equation.Reindex{}, false
		}
		targets[decision.Key()] = decision
	}
	maps := make([]equation.DecisionMap, source.Count())
	for index := range maps {
		decision, ok := source.At(index)
		if !ok || !decision.Available() {
			return equation.Reindex{}, false
		}
		if targetDecision, retained := targets[decision.Key()]; retained {
			maps[index] = equation.Identity(decision)
			if targetDecision != decision {
				maps[index] = equation.Rename(decision, targetDecision)
			}
		} else {
			maps[index] = equation.Forget(decision)
		}
	}
	return equation.NewReindex(source, target, maps)
}

func validBindingGroup(batch *equation.Batch, group equation.Group) bool {
	if batch == nil || len(group.Members) == 0 || group.Output == 0 {
		return false
	}
	for _, input := range group.Inputs {
		if !bindingOwnsInput(batch, input) {
			return false
		}
	}
	return !group.EnvironmentInput.Available() || bindingOwnsInput(batch, group.EnvironmentInput)
}

func cloneBindingRuleRow(row equation.RuleInstance) equation.RuleInstance {
	row.Reads = append([]equation.ResolvedRead(nil), row.Reads...)
	row.Carries = append([]equation.ResolvedCarry(nil), row.Carries...)
	row.Writes = append([]equation.ResolvedWrite(nil), row.Writes...)
	row.Supports = append([]equation.ResolvedSupport(nil), row.Supports...)
	row.Prunes = append([]equation.ResolvedPrune(nil), row.Prunes...)
	return row
}

func (builder *BindingTopologyBuilder) issueQueryRow(receipt bindingQueryReceipt, query equation.QueryInstance) (bindingQueryRowReceipt, bool) {
	inner, ok := builder.lockTopologyOpen()
	if !ok {
		return bindingQueryRowReceipt{}, false
	}
	state, authority, family, ordinal, receiptOK := receipt.boundTopologyQueryReceipt()
	valid := receiptOK && state == inner.state && authority == inner.authority && query.Family.Available() && query.Family == family && bindingOwnsQuerySchema(inner.state.schema, query.Family) && ordinal < inner.state.schema.queryCount() && validBindingQueryInstance(inner.state.schema, ordinal, query) && !duplicateBindingQuery(inner.spec.Queries, query)
	if !valid {
		inner.failLocked()
		inner.mu.Unlock()
		return bindingQueryRowReceipt{}, false
	}
	query.Surfaces = append([]equation.Surface(nil), query.Surfaces...)
	result := bindingQueryRowReceipt{builder: inner, state: state, authority: authority, ordinal: uint64(len(inner.spec.Queries)), issuer: receipt, row: query}
	inner.mu.Unlock()
	return result, true
}

func (builder *BindingTopologyBuilder) addSemanticQuery(id identity.ContentID, receipt bindingQueryRowReceipt) (bindingQueryRowRef, bool) {
	inner, ok := builder.lockTopologyOpen()
	if !ok {
		return bindingQueryRowRef{}, false
	}
	state, authority, family, queryOrdinal, receiptOK := receipt.issuer.boundTopologyQueryReceipt()
	valid := receiptOK && receipt.builder == inner && receipt.state == inner.state && receipt.authority == inner.authority && state == inner.state && authority == inner.authority && receipt.ordinal == uint64(len(inner.spec.Queries)) && receipt.row.Family == family && validBindingQueryInstance(inner.state.schema, queryOrdinal, receipt.row) && !duplicateBindingQuery(inner.spec.Queries, receipt.row) && claimBindingSemanticID(inner, id, bindingSemanticQuery)
	if !valid {
		inner.failLocked()
		inner.mu.Unlock()
		return bindingQueryRowRef{}, false
	}
	row := receipt.row
	row.Surfaces = append([]equation.Surface(nil), receipt.row.Surfaces...)
	inner.spec.Queries = append(inner.spec.Queries, row)
	inner.semantic.queries[id] = receipt.ordinal
	inner.mu.Unlock()
	return bindingQueryRowRef{builder: inner, ordinal: receipt.ordinal}, true
}

func (builder *BindingTopologyBuilder) issueEnvironmentEdge(edge equation.EnvironmentEdge) (bindingEnvironmentEdge, bool) {
	inner, ok := builder.lockTopologyOpen()
	if !ok || edge.Target == 0 || !bindingOwnsInput(inner.batch, edge.Input) {
		if ok {
			inner.mu.Unlock()
		}
		return bindingEnvironmentEdge{}, false
	}
	inner.mu.Unlock()
	return bindingEnvironmentEdge{builder: builder.inner, row: edge, state: builder.inner.state, authority: builder.inner.authority}, true
}

func (builder *BindingTopologyBuilder) addEnvironmentEdge(receipt bindingEnvironmentEdge) bool {
	return builder.addRow(func(spec *equation.TopologySpec) bool {
		if receipt.builder != builder.inner || receipt.state != builder.inner.state || receipt.authority != builder.inner.authority || receipt.row.Target == 0 || !bindingOwnsInput(spec.Batch, receipt.row.Input) {
			return false
		}
		spec.EnvironmentEdges = append(spec.EnvironmentEdges, receipt.row)
		return true
	})
}

func (builder *BindingTopologyBuilder) issueFactorEdge(factor bindingFactorReceipt, edge equation.FactorEdge) (bindingFactorEdge, bool) {
	inner, ok := builder.lockTopologyOpen()
	if !ok {
		return bindingFactorEdge{}, false
	}
	state, authority, semantic, receiptOK := factor.boundTopologyFactorReceipt()
	valid := receiptOK && state == inner.state && authority == inner.authority && semantic == edge.Factor && edge.Target != 0 && edge.Factor.Available() && bindingOwnsInput(inner.batch, edge.Input)
	inner.mu.Unlock()
	if !valid {
		return bindingFactorEdge{}, false
	}
	return bindingFactorEdge{builder: builder.inner, row: edge, state: state, authority: authority, factor: semantic}, true
}

func (builder *BindingTopologyBuilder) addFactorEdge(receipt bindingFactorEdge) bool {
	return builder.addRow(func(spec *equation.TopologySpec) bool {
		if receipt.builder != builder.inner || receipt.state != builder.inner.state || receipt.authority != builder.inner.authority || receipt.factor != receipt.row.Factor || receipt.row.Target == 0 || !bindingOwnsInput(spec.Batch, receipt.row.Input) || !bindingOwnsFactorSchema(builder.inner.state.schema, receipt.row.Factor) {
			return false
		}
		spec.FactorEdges = append(spec.FactorEdges, receipt.row)
		return true
	})
}

func (builder *BindingTopologyBuilder) addSummary(receipt bindingSummarySurfaceReceipt, summary equation.SummaryMapping) bool {
	return builder.addRow(func(spec *equation.TopologySpec) bool {
		if !validateSummarySurfaceReceipt(receipt, builder.inner.state, builder.inner.authority, summary.Surface) || len(summary.Keys) == 0 {
			return false
		}
		for _, existing := range spec.Summaries {
			if existing.Surface != summary.Surface {
				continue
			}
			if len(existing.Keys) != len(summary.Keys) {
				return false
			}
			for index := range existing.Keys {
				if existing.Keys[index] != summary.Keys[index] {
					return false
				}
			}
			return true
		}
		summary.Keys = append([]uint64(nil), summary.Keys...)
		spec.Summaries = append(spec.Summaries, summary)
		return true
	})
}

func (builder *BindingTopologyBuilder) issueMaterialization(value equation.TemplateMaterialization) (bindingMaterialization, bool) {
	inner, ok := builder.lockTopologyOpen()
	if !ok || !value.OwnedBy(inner.state.schema.cold, inner.batch) {
		if ok {
			inner.mu.Unlock()
		}
		return bindingMaterialization{}, false
	}
	inner.mu.Unlock()
	return bindingMaterialization{builder: builder.inner, value: value, base: builder.inner.batch, state: builder.inner.state, authority: builder.inner.authority}, true
}

func (builder *BindingTopologyBuilder) issueDirectActivationCandidate(value equation.DirectActivationCandidate) (bindingDirectActivation, bool) {
	inner, ok := builder.lockTopologyOpen()
	if !ok || !value.OwnedBy(inner.state.schema.cold, inner.batch) {
		if ok {
			inner.mu.Unlock()
		}
		return bindingDirectActivation{}, false
	}
	inner.mu.Unlock()
	return bindingDirectActivation{builder: builder.inner, value: value, base: builder.inner.batch, state: builder.inner.state, authority: builder.inner.authority}, true
}

func bindingActivationRuleShape(inner *bindingTopologyBuilderState, ref equation.RuleRef) (composition.RuleShape, bool) {
	index := int(uint64(ref)) - 1
	if inner == nil || inner.state == nil || inner.state.schema == nil || inner.state.schema.cold == nil || index < 0 || index >= len(inner.spec.Rules) {
		return composition.RuleShape{}, false
	}
	ordinal, found := inner.state.schema.cold.RuleIndex(inner.spec.Rules[index].Schema)
	if !found {
		return composition.RuleShape{}, false
	}
	return inner.state.schema.cold.RuleShapeAt(ordinal)
}

// addSemanticActivation registers the one stable structural-member identity
// for an already-admitted trigger Rule. Candidate materializations are a
// separate denominator: one trigger/member may own many exact target tuples,
// but it can never receive two stable activation IDs.
func (builder *BindingTopologyBuilder) addSemanticActivation(id identity.ContentID, member bindingRuleRowRef) bool {
	inner, ok := builder.lockTopologyOpen()
	if !ok {
		return false
	}
	index := int(uint64(member.ref)) - 1
	_, registeredMember := inner.semantic.memberAt[member.ref]
	_, duplicateActivation := inner.semantic.activationAt[member.ref]
	shape, shapeOK := bindingActivationRuleShape(inner, member.ref)
	valid := member.builder == inner && index >= 0 && index < len(inner.spec.Rules) && registeredMember && !duplicateActivation && shapeOK && shape.ActivationCount == 1 && shape.ActivationFamily.Available() && claimBindingSemanticID(inner, id, bindingSemanticActivation)
	if !valid {
		inner.failLocked()
		inner.mu.Unlock()
		return false
	}
	inner.semantic.activations[id] = member.ref
	inner.semantic.activationAt[member.ref] = id
	inner.mu.Unlock()
	return true
}

// addActivationCandidate associates one exact materialization receipt with
// its already-registered trigger member. The target tuple remains sealed in
// the materialization; the stable member directory never aliases one trigger
// under multiple IDs and never reconstructs candidates from coordinates.
func (builder *BindingTopologyBuilder) addActivationCandidate(receipt bindingMaterialization) bool {
	inner, ok := builder.lockTopologyOpen()
	if !ok {
		return false
	}
	origin, originOK := receipt.value.Origin()
	trigger := equation.RuleRef(0)
	if originOK && origin.TriggerOrdinal >= 0 {
		trigger = equation.RuleAt(origin.TriggerOrdinal)
	}
	_, registeredActivation := inner.semantic.activationAt[trigger]
	_, duplicateCandidate := inner.semantic.materializationAt[receipt.value]
	application, hasApplication := inner.semantic.activationApplication[trigger]
	_, completed := inner.semantic.activationExpected[trigger]
	valid := receipt.builder == inner && receipt.state == inner.state && receipt.authority == inner.authority && receipt.base == inner.batch && receipt.value.OwnedBy(inner.state.schema.cold, inner.batch) && originOK && origin.TriggerOrdinal >= 0 && origin.TriggerOrdinal < len(inner.spec.Rules) && registeredActivation && !completed && !duplicateCandidate && (!hasApplication || application == origin.Application)
	if valid {
		shape, shaped := bindingActivationRuleShape(inner, trigger)
		valid = shaped && shape.ActivationCount == 1 && shape.ActivationFamily == origin.Family
	}
	if !valid {
		inner.failLocked()
		inner.mu.Unlock()
		return false
	}
	inner.spec.Materializations = append(inner.spec.Materializations, receipt.value)
	inner.semantic.materializationAt[receipt.value] = trigger
	inner.semantic.activationCandidates[trigger]++
	inner.semantic.activationApplication[trigger] = origin.Application
	inner.mu.Unlock()
	return true
}

// addDirectActivationCandidate follows the same activation denominator law as
// template materializations, but the candidate's factor transport stays out
// of the static topology until its exact Member is accepted.
func (builder *BindingTopologyBuilder) addDirectActivationCandidate(receipt bindingDirectActivation) bool {
	inner, ok := builder.lockTopologyOpen()
	if !ok {
		return false
	}
	origin, originOK := receipt.value.Origin()
	trigger := equation.RuleRef(0)
	if originOK && origin.TriggerOrdinal >= 0 {
		trigger = equation.RuleAt(origin.TriggerOrdinal)
	}
	_, registeredActivation := inner.semantic.activationAt[trigger]
	_, duplicateCandidate := inner.semantic.directCandidateAt[receipt.value]
	key := receipt.value.Key()
	_, duplicateKey := inner.semantic.directCandidateKey[key]
	application, hasApplication := inner.semantic.activationApplication[trigger]
	_, completed := inner.semantic.activationExpected[trigger]
	valid := receipt.builder == inner && receipt.state == inner.state && receipt.authority == inner.authority && receipt.base == inner.batch && receipt.value.OwnedBy(inner.state.schema.cold, inner.batch) && key.Available() && originOK && origin.TriggerOrdinal >= 0 && origin.TriggerOrdinal < len(inner.spec.Rules) && registeredActivation && !completed && !duplicateCandidate && !duplicateKey && (!hasApplication || application == origin.Application)
	if valid {
		shape, shaped := bindingActivationRuleShape(inner, trigger)
		valid = shaped && shape.ActivationCount == 1 && shape.ActivationFamily == origin.Family
	}
	if !valid {
		inner.failLocked()
		inner.mu.Unlock()
		return false
	}
	inner.spec.DirectCandidates = append(inner.spec.DirectCandidates, receipt.value)
	inner.semantic.directCandidateAt[receipt.value] = trigger
	inner.semantic.directCandidateKey[key] = trigger
	inner.semantic.activationCandidates[trigger]++
	inner.semantic.activationApplication[trigger] = origin.Application
	inner.mu.Unlock()
	return true
}

// SealSources closes source admission on the exact Batch retained by this
// builder. The Batch remains the sole source identity authority for the
// topology phase; no rows are copied into a second admission plane.
func (builder *BindingTopologyBuilder) sealSources() receiptSourceSealFailure {
	inner, ok := builder.lockSourcesOpen()
	if !ok {
		return receiptSourceSealFailurePrecondition
	}
	if failure := inner.batch.SealWithFailure(); failure.Available() {
		inner.failLocked()
		inner.mu.Unlock()
		return failure
	}
	key := inner.batch.Key()
	if !inner.batch.Sealed() || !key.Available() || inner.spec.Batch != inner.batch {
		inner.failLocked()
		inner.mu.Unlock()
		return receiptSourceSealFailureBatchIdentity
	}
	inner.sourceKey = key
	inner.phase = bindingTopologyBuilderTopologyOpen
	inner.mu.Unlock()
	return equation.SealFailure{}
}

func (inner *bindingTopologyBuilderState) failLocked() {
	if inner == nil {
		return
	}
	inner.phase = bindingTopologyBuilderAborted
	inner.batch = nil
	inner.sourceKey = composition.Key{}
	inner.semantic = nil
	inner.directTransportSets = nil
	inner.spec = equation.TopologySpec{}
}

// Abort terminally consumes an abandoned construction plan. Copies share the
// same ledger, so only one of Commit or Abort can win.
func (builder *BindingTopologyBuilder) abort() bool {
	if builder == nil || builder.inner == nil {
		return false
	}
	inner := builder.inner
	inner.mu.Lock()
	defer inner.mu.Unlock()
	if inner.phase != bindingTopologyBuilderSourcesOpen && inner.phase != bindingTopologyBuilderTopologyOpen {
		return false
	}
	if inner.phase == bindingTopologyBuilderSourcesOpen {
		inner.batch.Reject()
	}
	inner.failLocked()
	return true
}

// Graph issues the equation graph for one published activation relation,
// only through this exact construction witness. Foreign topologies and
// equal-schema graphs cannot enter this path.
func (binding *BindingTopology) Graph(relation equation.Relation) (*equation.Graph, bool) {
	if !binding.valid() {
		return nil, false
	}
	return binding.topology.Graph(relation)
}
