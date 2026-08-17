package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"sync"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

// ReceiptGraph is an opaque graph receipt issued only by the exact sealed
// SchemaBinding that owns the graph. Equation graph internals never cross the
// owner boundary.
type ReceiptGraph struct {
	graph     *equation.Graph
	topology  *BindingTopology
	state     *schemaBindingState
	authority *schemaBindingAuthority
}

// BindingTopology is the Binding-issued Link lowering witness. It authenticates
// concrete sealed Factor owners and the exact equation Topology before graph
// attachment is possible.
type BindingTopology struct {
	self              *BindingTopology
	topology          *equation.Topology
	state             *schemaBindingState
	authority         *schemaBindingAuthority
	factors           []schemaFactorBinding
	plan              *bindingTopologyBuilderState
	directory         *semanticDirectory
	artifact          *artifactReceiptTopology
	nativeCallStages  map[artifactMountedRuleOccurrence]artifactNativeCallStage
	artifactFunctions []artifactMountedFunction
	artifactBacked    bool
	bootstrapOwner    identity.ContentID
	bootstrapPoint    identity.ContentID
	bootstrapSemantic identity.ContentID
}

type bindingTopologyBuilderPhase uint8

const (
	bindingTopologyBuilderSourcesOpen bindingTopologyBuilderPhase = iota + 1
	bindingTopologyBuilderTopologyOpen
	bindingTopologyBuilderCommitting
	bindingTopologyBuilderCommitted
	bindingTopologyBuilderAborted
)

type bindingTopologyBuilderState struct {
	mu                  sync.Mutex
	state               *schemaBindingState
	batch               *equation.Batch
	sourceKey           composition.Key
	spec                equation.TopologySpec
	phase               bindingTopologyBuilderPhase
	topology            *equation.Topology
	receipt             *BindingTopology
	authority           *schemaBindingAuthority
	factors             []schemaFactorBinding
	semantic            *bindingSemanticRows
	artifact            *artifactReceiptTopology
	directTransportSets map[directActivationTransportSetKey]equation.DirectActivationTransportSet
}

type bindingSemanticRowKind uint8

const (
	bindingSemanticPoint bindingSemanticRowKind = iota + 1
	bindingSemanticMember
	bindingSemanticQuery
	bindingSemanticActivation
)

type bindingSemanticRows struct {
	ids                   map[identity.ContentID]bindingSemanticRowKind
	points                map[identity.ContentID]equation.PointRef
	pointAt               map[equation.Site]identity.ContentID
	members               map[identity.ContentID]equation.RuleRef
	memberAt              map[equation.RuleRef]identity.ContentID
	queries               map[identity.ContentID]uint64
	activations           map[identity.ContentID]equation.RuleRef
	activationAt          map[equation.RuleRef]identity.ContentID
	materializationAt     map[equation.TemplateMaterialization]equation.RuleRef
	directCandidateAt     map[equation.DirectActivationCandidate]equation.RuleRef
	directCandidateKey    map[composition.Key]equation.RuleRef
	activationCandidates  map[equation.RuleRef]uint64
	activationExpected    map[equation.RuleRef]uint64
	activationApplication map[equation.RuleRef]composition.Key
}

// bindingTopologyBuilder is the only receipt-native Link lowering surface.
// It owns the disposable equation rows until Seal; callers never submit a raw
// TopologySpec or import a prebuilt Topology into this transaction.
type bindingTopologyBuilder struct{ inner *bindingTopologyBuilderState }

// ReceiptAssembly is the production-owned envelope for one sealed Binding
// and its sole topology builder. All lifecycle state lives in the shared
// builder ledger, so copying this capability cannot fork Seal/Commit/Abort.
type ReceiptAssembly struct {
	binding               *SchemaBinding
	builder               *bindingTopologyBuilder
	selectedSurfaceMu     sync.Mutex
	selectedSurfaceAnchor map[equation.Surface]mountedSelectedSurfaceAnchor
	queuedRuleMu          sync.Mutex
	queuedRuleFinalizers  []queuedRuleFinalizer
	ruleSourceFailureMu   sync.Mutex
	ruleSourceFailure     RuleSourceSealFailure
	finalizerFailureMu    sync.Mutex
	finalizerFailure      RuleFinalizerFailure
	sealFailure           ReceiptSealFailure
	commitFailure         ReceiptCommitFailure
}

type ReceiptCommitFailurePhase uint8

type ReceiptTopologyFailure = equation.SealTopologyFailure

const (
	ReceiptCommitFailureNone ReceiptCommitFailurePhase = iota
	ReceiptCommitFailurePrecondition
	ReceiptCommitFailureTopology
	ReceiptCommitFailureGraph
	ReceiptCommitFailureSchedule
	ReceiptCommitFailurePublish
	ReceiptCommitFailureDirectory
)

type ReceiptCommitFailure struct {
	phase        ReceiptCommitFailurePhase
	precondition ReceiptCommitPrecondition
	semanticRows ReceiptCommitSemanticRowsFailure
	topology     equation.SealTopologyFailure
	schedule     ReceiptScheduleFailure
	scheduleRow  uint32
	publish      ReceiptCommitPublishFailure
}

// ReceiptCommitPublishFailure identifies the final generic ownership gate
// that rejected an otherwise constructed receipt pair. It is deliberately
// closed and scalar: callers can distinguish the failed predicate without
// receiving engine/domain state or relying on temporary logging.
type ReceiptCommitPublishFailure uint8

const (
	ReceiptCommitPublishFailureNone ReceiptCommitPublishFailure = iota
	ReceiptCommitPublishFailureRelock
	ReceiptCommitPublishFailureArtifactSeal
	ReceiptCommitPublishFailureBindingTopology
	ReceiptCommitPublishFailureReceiptGraph
)

type ReceiptCommitPrecondition uint8

const (
	ReceiptCommitPreconditionNone ReceiptCommitPrecondition = iota
	ReceiptCommitPreconditionBuilder
	ReceiptCommitPreconditionSourcesOpen
	ReceiptCommitPreconditionPhase
	ReceiptCommitPreconditionBatch
	ReceiptCommitPreconditionBinding
	ReceiptCommitPreconditionAuthority
	ReceiptCommitPreconditionBatchSeal
	ReceiptCommitPreconditionSourceKey
	ReceiptCommitPreconditionSpecBatch
	ReceiptCommitPreconditionSemanticRows
)

type ReceiptCommitSemanticRowsFailure uint8

const (
	ReceiptCommitSemanticRowsFailureNone ReceiptCommitSemanticRowsFailure = iota
	ReceiptCommitSemanticRowsFailureCardinality
	ReceiptCommitSemanticRowsFailureActivation
	ReceiptCommitSemanticRowsFailureMaterialization
	ReceiptCommitSemanticRowsFailureIDs
)

func (failure ReceiptCommitFailure) Schedule() (ReceiptScheduleFailure, bool) {
	return failure.schedule, failure.phase == ReceiptCommitFailureSchedule && failure.schedule != ReceiptScheduleFailureNone
}

func (failure ReceiptCommitFailure) ScheduleOrdinal() (uint32, bool) {
	return failure.scheduleRow, failure.phase == ReceiptCommitFailureSchedule && failure.schedule != ReceiptScheduleFailureNone
}

func (failure ReceiptCommitFailure) Phase() ReceiptCommitFailurePhase { return failure.phase }
func (failure ReceiptCommitFailure) Precondition() (ReceiptCommitPrecondition, bool) {
	return failure.precondition, failure.phase == ReceiptCommitFailurePrecondition && failure.precondition != ReceiptCommitPreconditionNone
}
func (failure ReceiptCommitFailure) SemanticRows() (ReceiptCommitSemanticRowsFailure, bool) {
	return failure.semanticRows, failure.precondition == ReceiptCommitPreconditionSemanticRows && failure.semanticRows != ReceiptCommitSemanticRowsFailureNone
}
func (failure ReceiptCommitFailure) Topology() (equation.SealTopologyFailure, bool) {
	return failure.topology, failure.phase == ReceiptCommitFailureTopology && failure.topology != equation.SealTopologyFailureNone
}
func (failure ReceiptCommitFailure) Publish() (ReceiptCommitPublishFailure, bool) {
	return failure.publish, failure.phase == ReceiptCommitFailurePublish && failure.publish != ReceiptCommitPublishFailureNone
}

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

type ReceiptSealFailurePhase uint8

const (
	ReceiptSealFailureNone ReceiptSealFailurePhase = iota
	ReceiptSealFailureSources
	ReceiptSealFailureArtifactRows
	ReceiptSealFailureRuleFinalizer
)

// ReceiptSourceSealFailure is the generic equation Batch predicate that
// rejected source sealing. It is re-exported here so analyzer diagnostics do
// not expose equation internals or source row payloads.
type ReceiptSourceSealFailure = equation.BatchSealFailure

const (
	ReceiptSourceSealFailureNone                   = equation.BatchSealFailureNone
	ReceiptSourceSealFailurePrecondition           = equation.BatchSealFailurePrecondition
	ReceiptSourceSealFailureSiteRow                = equation.BatchSealFailureSiteRow
	ReceiptSourceSealFailureFormalCoverage         = equation.BatchSealFailureFormalCoverage
	ReceiptSourceSealFailureSiteIdentity           = equation.BatchSealFailureSiteIdentity
	ReceiptSourceSealFailureOccurrenceRow          = equation.BatchSealFailureOccurrenceRow
	ReceiptSourceSealFailureOccurrenceIdentity     = equation.BatchSealFailureOccurrenceIdentity
	ReceiptSourceSealFailureOperandRow             = equation.BatchSealFailureOperandRow
	ReceiptSourceSealFailureOperandIdentity        = equation.BatchSealFailureOperandIdentity
	ReceiptSourceSealFailureBatchIdentity          = equation.BatchSealFailureBatchIdentity
	ReceiptSourceSealFailureTargetRule             = equation.BatchSealFailureTargetRule
	ReceiptSourceSealFailureTargetInput            = equation.BatchSealFailureTargetInput
	ReceiptSourceSealFailureTargetGroup            = equation.BatchSealFailureTargetGroup
	ReceiptSourceSealFailureTargetGroupInput       = equation.BatchSealFailureTargetGroupInput
	ReceiptSourceSealFailureTargetEnvironmentInput = equation.BatchSealFailureTargetEnvironmentInput
	ReceiptSourceSealFailureTargetFactorEdge       = equation.BatchSealFailureTargetFactorEdge
	ReceiptSourceSealFailureTargetEnvironmentEdge  = equation.BatchSealFailureTargetEnvironmentEdge
	ReceiptSourceSealFailureTargetSummary          = equation.BatchSealFailureTargetSummary
	ReceiptSourceSealFailureTargetWeak             = equation.BatchSealFailureTargetWeak
	ReceiptSourceSealFailureTargetState            = equation.BatchSealFailureTargetState
)

// ReceiptSealFailure is detached scalar evidence for the first failed source
// seal boundary. Exactly one of the two capability planes is present for a
// finalizer failure; no callback, occurrence, or topology row escapes.
type ReceiptSealFailure struct {
	phase     ReceiptSealFailurePhase
	ordinal   uint32
	source    ReceiptSourceSealFailure
	rule      RuleSourceSealFailure
	finalizer RuleFinalizerFailure
	mounted   RuleSlotCapability
	link      RuleSlotCapability
	artifact  ReceiptArtifactRowFailure
}

func (failure ReceiptSealFailure) Phase() ReceiptSealFailurePhase { return failure.phase }
func (failure ReceiptSealFailure) Ordinal() uint32                { return failure.ordinal }
func (failure ReceiptSealFailure) Source() (ReceiptSourceSealFailure, bool) {
	return failure.source, failure.phase == ReceiptSealFailureSources && failure.source != ReceiptSourceSealFailureNone
}
func (failure ReceiptSealFailure) RuleSource() (RuleSourceSealFailure, bool) {
	return failure.rule, failure.phase == ReceiptSealFailureRuleFinalizer && failure.rule != RuleSourceSealFailureNone
}
func (failure ReceiptSealFailure) Finalizer() (RuleFinalizerFailure, bool) {
	return failure.finalizer, failure.phase == ReceiptSealFailureRuleFinalizer && failure.finalizer != RuleFinalizerFailureNone
}
func (failure ReceiptSealFailure) MountedCapability() (RuleSlotCapability, bool) {
	return failure.mounted, failure.phase == ReceiptSealFailureRuleFinalizer && failure.mounted.mounted() && !failure.link.available()
}
func (failure ReceiptSealFailure) LinkCapability() (RuleSlotCapability, bool) {
	return failure.link, failure.phase == ReceiptSealFailureRuleFinalizer && failure.link.link() && !failure.mounted.available()
}
func (failure ReceiptSealFailure) ArtifactRow() (ReceiptArtifactRowFailure, bool) {
	return failure.artifact, failure.phase == ReceiptSealFailureArtifactRows && failure.artifact != ReceiptArtifactRowFailureNone
}

type mountedSelectedSurfaceAnchor struct {
	assembly   *ReceiptAssembly
	occurrence equation.Occurrence
	operand    equation.Operand
	rule       uint64
	index      uint64
	form       equation.SurfaceForm
}

// beginReceiptAssembly is deliberately package-private until the sole
// ProgramTransformer lowerer can issue this transaction from its exact parent
// proof. A generic holder of SchemaBinding must not reserve or burn the one
// topology builder.
func beginReceiptAssembly(binding *SchemaBinding) (*ReceiptAssembly, bool) {
	if binding == nil || !binding.Sealed() {
		return nil, false
	}
	builder, ok := binding.beginBindingTopologyBuilder()
	if !ok {
		return nil, false
	}
	return &ReceiptAssembly{binding: binding, builder: builder, selectedSurfaceAnchor: make(map[equation.Surface]mountedSelectedSurfaceAnchor)}, true
}

func (assembly *ReceiptAssembly) claimMountedSurface(surface equation.Surface, anchor mountedSelectedSurfaceAnchor) bool {
	if assembly == nil || !surface.Available() {
		return false
	}
	assembly.selectedSurfaceMu.Lock()
	defer assembly.selectedSurfaceMu.Unlock()
	if previous, found := assembly.selectedSurfaceAnchor[surface]; found {
		_ = previous
		return false
	}
	assembly.selectedSurfaceAnchor[surface] = anchor
	return true
}

func (assembly *ReceiptAssembly) claimMountedSelectedSurface(surface equation.Surface, anchor mountedSelectedSurfaceAnchor) bool {
	return assembly.claimMountedSurface(surface, anchor)
}

func (assembly *ReceiptAssembly) Abort() bool {
	if assembly == nil || assembly.builder == nil {
		return false
	}
	return assembly.builder.abort()
}

// QueueMountedRuleFinalizer retains one typed, fully-admitted mounted source row until the
// source Batch seals. RuleSurfaceSourceReceipt deliberately requires that
// sealed Batch; finalizers therefore run only after lowerArtifactRows opens
// topology construction. The closure remains opaque to the engine and can
// only close over exact owner-issued transactions and implementations.
func (assembly *ReceiptAssembly) QueueMountedRuleFinalizer(role RuleSlotCapability, finalize func() bool) bool {
	if !role.mounted() {
		return false
	}
	return assembly.queueRuleFinalizer(queuedRuleFinalizer{mounted: role, run: finalize})
}

// QueueLinkRuleFinalizer is the distinct Link-global bootstrap ingress.
func (assembly *ReceiptAssembly) QueueLinkRuleFinalizer(role RuleSlotCapability, finalize func() bool) bool {
	if !role.link() {
		return false
	}
	return assembly.queueRuleFinalizer(queuedRuleFinalizer{link: role, run: finalize})
}

func (assembly *ReceiptAssembly) queueRuleFinalizer(finalizer queuedRuleFinalizer) bool {
	if assembly == nil || assembly.builder == nil || finalizer.run == nil ||
		(finalizer.mounted.mounted() == finalizer.link.link()) {
		return false
	}
	inner, ok := assembly.builder.lockSourcesOpen()
	if !ok {
		return false
	}
	inner.mu.Unlock()
	assembly.queuedRuleMu.Lock()
	defer assembly.queuedRuleMu.Unlock()
	assembly.queuedRuleFinalizers = append(assembly.queuedRuleFinalizers, finalizer)
	return true
}

func (assembly *ReceiptAssembly) drainRuleFinalizers() bool {
	assembly.queuedRuleMu.Lock()
	finalizers := assembly.queuedRuleFinalizers
	assembly.queuedRuleFinalizers = nil
	assembly.queuedRuleMu.Unlock()
	for index, finalizer := range finalizers {
		assembly.recordRuleSourceSealFailure(RuleSourceSealFailureNone)
		assembly.recordRuleFinalizerFailure(RuleFinalizerFailureNone)
		if finalizer.run == nil || !finalizer.run() {
			assembly.sealFailure = ReceiptSealFailure{phase: ReceiptSealFailureRuleFinalizer, ordinal: uint32(index), rule: assembly.currentRuleSourceSealFailure(), finalizer: assembly.currentRuleFinalizerFailure(), mounted: finalizer.mounted, link: finalizer.link}
			return false
		}
	}
	return true
}

func (assembly *ReceiptAssembly) recordRuleSourceSealFailure(failure RuleSourceSealFailure) {
	if assembly == nil {
		return
	}
	assembly.ruleSourceFailureMu.Lock()
	assembly.ruleSourceFailure = failure
	assembly.ruleSourceFailureMu.Unlock()
}

func (assembly *ReceiptAssembly) currentRuleSourceSealFailure() RuleSourceSealFailure {
	if assembly == nil {
		return RuleSourceSealFailureNone
	}
	assembly.ruleSourceFailureMu.Lock()
	defer assembly.ruleSourceFailureMu.Unlock()
	return assembly.ruleSourceFailure
}

func (assembly *ReceiptAssembly) recordRuleFinalizerFailure(failure RuleFinalizerFailure) {
	if assembly == nil {
		return
	}
	assembly.finalizerFailureMu.Lock()
	assembly.finalizerFailure = failure
	assembly.finalizerFailureMu.Unlock()
}

func (assembly *ReceiptAssembly) currentRuleFinalizerFailure() RuleFinalizerFailure {
	if assembly == nil {
		return RuleFinalizerFailureNone
	}
	assembly.finalizerFailureMu.Lock()
	defer assembly.finalizerFailureMu.Unlock()
	return assembly.finalizerFailure
}

// SealSources freezes the one exact source Batch owned by this assembly. It
// ends source admission while opening topology-row construction against those
// same immutable Site/Occurrence/Operand identities.
func (assembly *ReceiptAssembly) SealSources() bool {
	if assembly == nil || assembly.builder == nil {
		return false
	}
	if sourceFailure := assembly.builder.sealSources(); sourceFailure != ReceiptSourceSealFailureNone {
		assembly.sealFailure = ReceiptSealFailure{phase: ReceiptSealFailureSources, source: sourceFailure}
		return false
	}
	artifactFailure, artifactOrdinal, artifactOK := assembly.builder.lowerArtifactRows()
	if !artifactOK {
		assembly.sealFailure = ReceiptSealFailure{phase: ReceiptSealFailureArtifactRows, artifact: artifactFailure, ordinal: artifactOrdinal}
		assembly.builder.abort()
		return false
	}
	if !assembly.drainRuleFinalizers() {
		assembly.builder.abort()
		return false
	}
	return true
}

func (assembly *ReceiptAssembly) SealFailure() (ReceiptSealFailure, bool) {
	if assembly == nil || assembly.sealFailure.phase == ReceiptSealFailureNone {
		return ReceiptSealFailure{}, false
	}
	return assembly.sealFailure, true
}

func (assembly *ReceiptAssembly) Commit() (*BindingTopology, *ReceiptGraph, bool) {
	if assembly == nil {
		return nil, nil, false
	}
	if assembly.builder == nil {
		return nil, nil, false
	}
	topology, graph, failure, ok := assembly.builder.commit(false)
	assembly.commitFailure = failure
	return topology, graph, ok
}

// CommitObservationTopology seals a receipt graph with every declared Query
// family deferred to owned solve-local observations. It rejects even one
// ordinary Query row; callers must attach a committed-member observation
// before Solver can derive demand. Ordinary Commit remains strict.
func (assembly *ReceiptAssembly) CommitObservationTopology() (*BindingTopology, *ReceiptGraph, bool) {
	if assembly == nil || assembly.builder == nil {
		return nil, nil, false
	}
	topology, graph, failure, ok := assembly.builder.commit(true)
	assembly.commitFailure = failure
	return topology, graph, ok
}

func (assembly *ReceiptAssembly) CommitFailure() (ReceiptCommitFailure, bool) {
	if assembly == nil || assembly.commitFailure.phase == ReceiptCommitFailureNone {
		return ReceiptCommitFailure{}, false
	}
	return assembly.commitFailure, true
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

type BindingEnvironmentEdgeReceipt struct {
	builder   *bindingTopologyBuilderState
	row       equation.EnvironmentEdge
	state     *schemaBindingState
	authority *schemaBindingAuthority
}

type BindingFactorEdgeReceipt struct {
	builder   *bindingTopologyBuilderState
	row       equation.FactorEdge
	state     *schemaBindingState
	authority *schemaBindingAuthority
	factor    composition.Key
}

type BindingMaterializationReceipt struct {
	builder   *bindingTopologyBuilderState
	value     equation.TemplateMaterialization
	base      *equation.Batch
	state     *schemaBindingState
	authority *schemaBindingAuthority
}

// BindingDirectActivationReceipt is private builder authority for a direct
// candidate over already-lowered mounted artifact points. Domain code cannot
// manufacture it from transport scalars.
type BindingDirectActivationReceipt struct {
	builder   *bindingTopologyBuilderState
	value     equation.DirectActivationCandidate
	base      *equation.Batch
	state     *schemaBindingState
	authority *schemaBindingAuthority
}

// BindingRuleRowReceipt is the complete, Binding-issued structural row
// authority. The equation RuleInstance is retained only behind this opaque
// handle; AddRule never accepts a raw row.
type BindingRuleRowReceipt struct {
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

type BindingRuleRowRef struct {
	builder *bindingTopologyBuilderState
	ref     equation.RuleRef
}

// BindingRuleRowDraft is a typed, cell-owned row assembly handle. Its
// equation row is private and can only be populated through the indexed shape
// methods below.
type BindingRuleRowDraft struct {
	mu             sync.Mutex
	state          *schemaBindingState
	authority      *schemaBindingAuthority
	receipt        bindingRuleReceipt
	source         equation.RuleSurfaceSourceReceipt
	sourceIdentity equation.RuleSurfaceSourceReceipt
	row            equation.RuleInstance
	consumed       bool
	assembly       *ReceiptAssembly
}

type BindingRuleReadPartReceipt struct {
	issuer         bindingRuleReceipt
	sourceIdentity equation.RuleSurfaceSourceReceipt
	state          *schemaBindingState
	authority      *schemaBindingAuthority
	index          uint64
	surface        equation.Surface
}
type BindingRuleCarryPartReceipt struct {
	issuer         bindingRuleReceipt
	sourceIdentity equation.RuleSurfaceSourceReceipt
	state          *schemaBindingState
	authority      *schemaBindingAuthority
	index          uint64
}
type BindingRuleWritePartReceipt struct {
	issuer         bindingRuleReceipt
	sourceIdentity equation.RuleSurfaceSourceReceipt
	state          *schemaBindingState
	authority      *schemaBindingAuthority
	index          uint64
	value          equation.ResolvedWrite
}
type BindingRuleSupportPartReceipt struct {
	issuer         bindingRuleReceipt
	sourceIdentity equation.RuleSurfaceSourceReceipt
	state          *schemaBindingState
	authority      *schemaBindingAuthority
	index          uint64
	value          equation.ResolvedSupport
}
type BindingRulePrunePartReceipt struct {
	issuer         bindingRuleReceipt
	sourceIdentity equation.RuleSurfaceSourceReceipt
	state          *schemaBindingState
	authority      *schemaBindingAuthority
	index          uint64
	value          equation.ResolvedPrune
}

func (implementation *RuleImplementation[K, V, O]) BeginBindingRuleRow(source equation.RuleSurfaceSourceReceipt) (*BindingRuleRowDraft, bool) {
	state, authority, semantic, ok := implementation.boundTopologyRuleReceipt()
	if !ok || !source.Occurrence().Available() || !source.Operand().Available() || !source.Operand().Occurrence().Same(source.Occurrence()) || source.Rule() != semantic {
		return nil, false
	}
	return &BindingRuleRowDraft{state: state, authority: authority, receipt: implementation, source: source, row: equation.RuleInstance{Schema: semantic, OperandFamily: implementation.receipt.proof.operandFamily, Occurrence: source.Occurrence(), Operand: source.Operand()}}, true
}

func (implementation *RuleImplementation[K, V, O]) ReadPart(source equation.RuleSurfaceSourceReceipt, index uint64) (BindingRuleReadPartReceipt, bool) {
	state, authority, _, ok := implementation.boundTopologyRuleReceipt()
	if !ok {
		return BindingRuleReadPartReceipt{}, false
	}
	value, ok := source.ReadAt(index)
	if !ok {
		return BindingRuleReadPartReceipt{}, false
	}
	return BindingRuleReadPartReceipt{issuer: implementation, sourceIdentity: source, state: state, authority: authority, index: index, surface: value.Surface}, true
}

func (implementation *ActivationRuleImplementation) ReadPart(source equation.RuleSurfaceSourceReceipt, index uint64) (BindingRuleReadPartReceipt, bool) {
	state, authority, _, ok := implementation.boundTopologyRuleReceipt()
	if !ok {
		return BindingRuleReadPartReceipt{}, false
	}
	value, ok := source.ReadAt(index)
	if !ok {
		return BindingRuleReadPartReceipt{}, false
	}
	return BindingRuleReadPartReceipt{issuer: implementation, sourceIdentity: source, state: state, authority: authority, index: index, surface: value.Surface}, true
}

func (implementation *RuleImplementation[K, V, O]) CarryPart(source equation.RuleSurfaceSourceReceipt, index uint64) (BindingRuleCarryPartReceipt, bool) {
	state, authority, _, ok := implementation.boundTopologyRuleReceipt()
	if !ok {
		return BindingRuleCarryPartReceipt{}, false
	}
	if _, ok := source.CarryAt(index); !ok || source.Rule() != implementation.receipt.proof.semantic {
		return BindingRuleCarryPartReceipt{}, false
	}
	return BindingRuleCarryPartReceipt{issuer: implementation, sourceIdentity: source, state: state, authority: authority, index: index}, true
}

func (implementation *RuleImplementation[K, V, O]) WritePart(source equation.RuleSurfaceSourceReceipt, index uint64) (BindingRuleWritePartReceipt, bool) {
	state, authority, _, ok := implementation.boundTopologyRuleReceipt()
	if !ok {
		return BindingRuleWritePartReceipt{}, false
	}
	value, ok := source.WriteAt(index)
	if !ok || source.Rule() != implementation.receipt.proof.semantic {
		return BindingRuleWritePartReceipt{}, false
	}
	return BindingRuleWritePartReceipt{issuer: implementation, sourceIdentity: source, state: state, authority: authority, index: index, value: value}, true
}

func (implementation *RuleImplementation[K, V, O]) SupportPart(source equation.RuleSurfaceSourceReceipt, index uint64) (BindingRuleSupportPartReceipt, bool) {
	state, authority, _, ok := implementation.boundTopologyRuleReceipt()
	value, sourceOK := source.SupportAt(index)
	if !ok || !sourceOK || source.Rule() != implementation.receipt.proof.semantic {
		return BindingRuleSupportPartReceipt{}, false
	}
	return BindingRuleSupportPartReceipt{issuer: implementation, sourceIdentity: source, state: state, authority: authority, index: index, value: value}, true
}

func (implementation *RuleImplementation[K, V, O]) PrunePart(source equation.RuleSurfaceSourceReceipt, index uint64) (BindingRulePrunePartReceipt, bool) {
	state, authority, _, ok := implementation.boundTopologyRuleReceipt()
	value, sourceOK := source.PruneAt(index)
	if !ok || !sourceOK || source.Rule() != implementation.receipt.proof.semantic {
		return BindingRulePrunePartReceipt{}, false
	}
	return BindingRulePrunePartReceipt{issuer: implementation, sourceIdentity: source, state: state, authority: authority, index: index, value: value}, true
}

func (draft *BindingRuleRowDraft) AddRead(receipt BindingRuleReadPartReceipt) (ok bool) {
	defer func() {
		if !ok && draft != nil {
			draft.assembly.recordRuleFinalizerFailure(RuleFinalizerFailureDraftRead)
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

func (draft *BindingRuleRowDraft) AddCarry(receipt BindingRuleCarryPartReceipt) (ok bool) {
	defer func() {
		if !ok && draft != nil {
			draft.assembly.recordRuleFinalizerFailure(RuleFinalizerFailureDraftCarry)
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

func (draft *BindingRuleRowDraft) AddWrite(receipt BindingRuleWritePartReceipt) (ok bool) {
	defer func() {
		if !ok && draft != nil {
			draft.assembly.recordRuleFinalizerFailure(RuleFinalizerFailureDraftWrite)
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
	if implementation == nil || !implementation.receipt.valid() {
		return nil, nil, composition.Key{}, false
	}
	return implementation.receipt.state, implementation.receipt.authority, implementation.receipt.proof.semantic, true
}

func (implementation *ActivationRuleImplementation) boundTopologyRuleReceipt() (*schemaBindingState, *schemaBindingAuthority, composition.Key, bool) {
	if implementation == nil || !implementation.receipt.valid() {
		return nil, nil, composition.Key{}, false
	}
	return implementation.receipt.state, implementation.receipt.authority, implementation.receipt.proof.semantic, true
}

// BeginBindingRuleRow issues the structural trigger row from the exact sealed
// activation implementation. Candidate materializations are admitted later
// and cannot author or replace this member.
func (implementation *ActivationRuleImplementation) BeginBindingRuleRow(source equation.RuleSurfaceSourceReceipt) (*BindingRuleRowDraft, bool) {
	state, authority, semantic, ok := implementation.boundTopologyRuleReceipt()
	if !ok || !source.Occurrence().Available() || !source.Operand().Available() || !source.Operand().Occurrence().Same(source.Occurrence()) || source.Rule() != semantic {
		return nil, false
	}
	return &BindingRuleRowDraft{state: state, authority: authority, receipt: implementation, source: source, row: equation.RuleInstance{Schema: semantic, OperandFamily: implementation.receipt.proof.operandFamily, Occurrence: source.Occurrence(), Operand: source.Operand()}}, true
}

func (implementation *FactorImplementation[K, V]) boundTopologyFactorReceipt() (*schemaBindingState, *schemaBindingAuthority, composition.Key, bool) {
	if implementation == nil || !implementation.receipt.validForms() {
		return nil, nil, composition.Key{}, false
	}
	return implementation.receipt.state, implementation.receipt.authority, implementation.receipt.semantic, true
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
	if !write.Surface.Available() || write.Surface.Factor != shape.Factor || write.Route != shape.Route || uint64(len(write.Candidates)) != shape.CandidateCount || uint64(len(write.Relations)) != shape.DependencyCount {
		return false
	}
	switch shape.Kind {
	case composition.WriteExact:
		return write.Surface.Form == equation.SurfaceWriteExact
	case composition.WriteSelect:
		return write.Surface.Form == equation.SurfaceWriteSelect
	case composition.WriteRoute:
		return write.Surface.Form == equation.SurfaceWriteRoute
	default:
		return false
	}
}

func bindingOwnsInput(batch *equation.Batch, input equation.Input) bool {
	return batch != nil && input.Available() && batch.OwnsSite(input.Source()) && batch.OwnsSite(input.Target())
}

// ReceiptRuleMember is an opaque graph-owned member receipt. It can be used
// only with the ReceiptGraph that issued it.
type ReceiptRuleMember struct {
	graph   *ReceiptGraph
	member  equation.RuleMember
	locator equation.RuleMemberRowLocator
}

type receiptPoint struct {
	graph *ReceiptGraph
	point equation.Point
}

// ActivationReceiptGraph is the opaque activation attachment witness.  It
// binds one graph receipt to the exact sealed Topology that materialized that
// graph; callers cannot substitute an equal graph or retain either equation
// authority directly.
type ActivationReceiptGraph struct {
	receipt  *ReceiptGraph
	topology *equation.Topology
}

// ActivationReceiptMember is one exact activation member from an
// ActivationReceiptGraph.  It is intentionally distinct from ReceiptRuleMember
// because activation compilation needs the topology's binding proof as well
// as the graph member.
type ActivationReceiptMember struct {
	graph   *ActivationReceiptGraph
	member  equation.RuleMember
	locator equation.ActivationMemberRowLocator
}

func (receipt *ReceiptGraph) valid() bool {
	return receipt != nil && receipt.graph != nil && receipt.topology != nil && receipt.topology.valid() && receipt.state == receipt.topology.state && receipt.authority == receipt.topology.authority && receipt.graph.OwnsComposition(receipt.state.schema.cold) && receipt.topology.topology.OwnsGraph(receipt.graph)
}

func (receipt *BindingTopology) valid() bool {
	if receipt == nil || receipt.self != receipt || receipt.topology == nil || receipt.state == nil || receipt.authority == nil || receipt.plan == nil || (receipt.artifact != nil && !receipt.artifact.valid(receipt)) || !receipt.directory.ownedBy(receipt.topology, receipt.state, receipt.authority) || receipt.state.phase != schemaBindingSealed || receipt.state.authority != receipt.authority || receipt.state.schema == nil || !receipt.topology.OwnsComposition(receipt.state.schema.cold) || receipt.plan.state != receipt.state || receipt.plan.authority != receipt.authority || receipt.plan.topology != receipt.topology || receipt.plan.phase != bindingTopologyBuilderCommitted || !receipt.plan.sourceKey.Available() {
		return false
	}
	ownerAvailable, pointAvailable, semanticAvailable := receipt.bootstrapOwner.Available(), receipt.bootstrapPoint.Available(), receipt.bootstrapSemantic.Available()
	if receipt.artifactBacked {
		semantic := linkBootstrapPointSemanticID(receipt.bootstrapOwner, receipt.bootstrapPoint)
		if !ownerAvailable || !pointAvailable || !semanticAvailable || semantic != receipt.bootstrapSemantic || receipt.artifactFunctions == nil {
			return false
		}
		if _, found := receipt.directory.point(receipt.bootstrapSemantic); !found {
			return false
		}
	} else if receipt.artifact != nil || receipt.artifactFunctions != nil || ownerAvailable || pointAvailable || semanticAvailable {
		return false
	}
	if receipt.artifact != nil && (receipt.artifact.bootstrap == nil || receipt.bootstrapOwner != receipt.artifact.bootstrap.owner || receipt.bootstrapPoint != receipt.artifact.bootstrap.point.PointID || receipt.bootstrapSemantic != receipt.artifact.bootstrap.semantic) {
		return false
	}
	if receipt.plan.batch == nil {
		return receipt.plan.spec.Batch == nil
	}
	return receipt.plan.batch.Sealed() && receipt.plan.batch.Key() == receipt.plan.sourceKey && receipt.plan.spec.Batch == receipt.plan.batch
}

func (receipt *ActivationReceiptGraph) valid() bool {
	return receipt != nil && receipt.receipt != nil && receipt.receipt.valid() && receipt.topology != nil &&
		receipt.receipt.topology.topology == receipt.topology && receipt.topology.OwnsComposition(receipt.receipt.state.schema.cold) && receipt.topology.OwnsGraph(receipt.receipt.graph)
}

// beginBindingTopologyBuilder creates one solve-local topology transaction
// under this exact sealed Binding. A sealed Binding is immutable and reusable;
// every returned builder owns an independent Batch and lifecycle so repeated
// and concurrent Plan solves never rebuild the Link-local factor/rule owners.
// The constructor remains reachable only through the ReceiptAssembly
// capability envelope.
func (binding *SchemaBinding) beginBindingTopologyBuilder() (*bindingTopologyBuilder, bool) {
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
		authority: state.authority, factors: append([]schemaFactorBinding(nil), state.factors...),
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
	return &bindingTopologyBuilder{inner: inner}, true
}

func (builder *bindingTopologyBuilder) lockPhase(phase bindingTopologyBuilderPhase) (*bindingTopologyBuilderState, bool) {
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

func (builder *bindingTopologyBuilder) lockSourcesOpen() (*bindingTopologyBuilderState, bool) {
	return builder.lockPhase(bindingTopologyBuilderSourcesOpen)
}

func (builder *bindingTopologyBuilder) lockTopologyOpen() (*bindingTopologyBuilderState, bool) {
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

func (builder *bindingTopologyBuilder) admitSite(source composition.Key, scope equation.Scope, init equation.Expr, disposition equation.InitDisposition) (equation.Site, bool) {
	inner, ok := builder.lockSourcesOpen()
	if !ok {
		return equation.Site{}, false
	}
	defer inner.mu.Unlock()
	return inner.batch.AdmitSite(source, scope, init, disposition)
}

func (builder *bindingTopologyBuilder) admitAt(site equation.Site) (equation.Occurrence, bool) {
	inner, ok := builder.lockSourcesOpen()
	if !ok {
		return equation.Occurrence{}, false
	}
	defer inner.mu.Unlock()
	return inner.batch.At(site)
}

func (builder *bindingTopologyBuilder) admitFrom(site equation.Site, entity composition.Key) (equation.Occurrence, bool) {
	inner, ok := builder.lockSourcesOpen()
	if !ok {
		return equation.Occurrence{}, false
	}
	defer inner.mu.Unlock()
	return inner.batch.From(site, entity)
}

func (builder *bindingTopologyBuilder) admitOperand(occurrence equation.Occurrence, entity composition.Key) (equation.Operand, bool) {
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
func (builder *bindingTopologyBuilder) issueRuleSurfaceSource(spec equation.RuleSurfaceSourceSpec) (equation.RuleSurfaceSourceReceipt, bool) {
	inner, ok := builder.lockTopologyOpen()
	if !ok {
		return equation.RuleSurfaceSourceReceipt{}, false
	}
	defer inner.mu.Unlock()
	return inner.batch.IssueRuleSurfaceSource(inner.state.schema.cold, spec)
}

func (builder *bindingTopologyBuilder) addRow(row func(*equation.TopologySpec) bool) bool {
	inner, ok := builder.lockTopologyOpen()
	if !ok {
		return false
	}
	defer inner.mu.Unlock()
	return row(&inner.spec)
}

func (builder *bindingTopologyBuilder) addPoint(point equation.PointSpec) bool {
	return builder.addRow(func(spec *equation.TopologySpec) bool {
		if !point.Site.Available() || !spec.Batch.OwnsSite(point.Site) {
			return false
		}
		spec.Points = append(spec.Points, point)
		return true
	})
}

func (builder *bindingTopologyBuilder) issuePointRow(point equation.PointSpec) (bindingPointRowReceipt, bool) {
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

func (builder *bindingTopologyBuilder) addSemanticPoint(id identity.ContentID, receipt bindingPointRowReceipt) (bindingPointRowRef, bool) {
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

func (builder *bindingTopologyBuilder) issueRuleRow(draft *BindingRuleRowDraft) (BindingRuleRowReceipt, bool) {
	inner, ok := builder.lockTopologyOpen()
	if !ok {
		return BindingRuleRowReceipt{}, false
	}
	if draft == nil {
		inner.mu.Unlock()
		return BindingRuleRowReceipt{}, false
	}
	draft.mu.Lock()
	if draft.consumed {
		draft.mu.Unlock()
		inner.mu.Unlock()
		return BindingRuleRowReceipt{}, false
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
		return BindingRuleRowReceipt{}, false
	}
	return BindingRuleRowReceipt{builder: builder.inner, state: state, authority: authority, ordinal: ordinal, row: cloneBindingRuleRow(row)}, true
}

func (builder *bindingTopologyBuilder) addSemanticRule(id identity.ContentID, receipt BindingRuleRowReceipt) (BindingRuleRowRef, bool) {
	inner, ok := builder.lockTopologyOpen()
	if !ok {
		return BindingRuleRowRef{}, false
	}
	ref := equation.RuleAt(len(inner.spec.Rules))
	_, duplicateMember := inner.semantic.memberAt[ref]
	valid := receipt.builder == inner && receipt.state == inner.state && receipt.authority == inner.authority && receipt.ordinal == uint64(len(inner.spec.Rules)) && validateBindingRuleRows(inner.state.schema, receipt.row) && !duplicateMember && claimBindingSemanticID(inner, id, bindingSemanticMember)
	if !valid {
		inner.failLocked()
		inner.mu.Unlock()
		return BindingRuleRowRef{}, false
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
					boundary, boundaryOK = artifactPredecessorRuleInput(inner.artifact, *receipt.predecessor, source, target, pointID, provenance)
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
		return BindingRuleRowRef{}, false
	}
	inner.spec.Rules = append(inner.spec.Rules, cloneBindingRuleRow(receipt.row))
	inner.spec.Groups = append(inner.spec.Groups, group)
	inner.semantic.members[id] = ref
	inner.semantic.memberAt[ref] = id
	inner.mu.Unlock()
	return BindingRuleRowRef{builder: inner, ref: ref}, true
}

func artifactPredecessorRuleInput(rows *artifactReceiptTopology, edge artifactEnvironmentRow, source, target equation.Site, targetPoint identity.ContentID, provenance composition.Key) (equation.Input, bool) {
	if rows == nil || !validArtifactRouteProof(edge) || !edge.route.Available() || !provenance.Available() || !source.Available() || !target.Available() || !targetPoint.Available() {
		return equation.Input{}, false
	}
	wantSource, sourceOK := rows.sites[edge.from]
	sourceMeta, sourceMetaOK := rows.pointMeta[edge.from]
	targetMeta, targetMetaOK := rows.pointMeta[targetPoint]
	if !sourceOK || !source.Same(wantSource) || !sourceMetaOK || !targetMetaOK {
		return equation.Input{}, false
	}
	sourceDecisions, sourceDecisionsOK := artifactRulePointDecisions(sourceMeta, source)
	targetDecisions, targetDecisionsOK := artifactRulePointDecisions(targetMeta, target)
	if !sourceDecisionsOK || !targetDecisionsOK {
		return equation.Input{}, false
	}
	resetSet := make(map[identity.ContentID]struct{}, len(edge.resets))
	for _, reset := range edge.resets {
		if _, exists := sourceDecisions[reset]; !exists {
			return equation.Input{}, false
		}
		resetSet[reset] = struct{}{}
	}
	maps := make([]equation.DecisionMap, len(sourceMeta.decisions))
	for index, semanticID := range sourceMeta.decisions {
		decision, exists := sourceDecisions[semanticID]
		if !exists {
			return equation.Input{}, false
		}
		if _, reset := resetSet[semanticID]; reset {
			maps[index] = equation.Forget(decision)
			continue
		}
		targetDecision, retained := targetDecisions[semanticID]
		if !retained {
			return equation.Input{}, false
		}
		maps[index] = equation.Identity(decision)
		if targetDecision != decision {
			maps[index] = equation.Rename(decision, targetDecision)
		}
	}
	reindex, reindexOK := equation.NewReindex(source.Scope(), target.Scope(), maps)
	pre := equation.TrueExpr()
	if edge.guarded {
		decision, decisionOK := sourceDecisions[edge.decision]
		if !decisionOK {
			return equation.Input{}, false
		}
		pre, decisionOK = equation.DecisionExpr(decision)
		if decisionOK && !edge.truth {
			pre, decisionOK = equation.NotExpr(pre)
		}
		if !decisionOK {
			return equation.Input{}, false
		}
	}
	input := equation.BoundaryInput(source, target, provenance, pre, reindex, equation.TrueExpr())
	return input, reindexOK && input.Available()
}

func artifactRulePointDecisions(metadata artifactPointMetadata, site equation.Site) (map[identity.ContentID]equation.Decision, bool) {
	scope := site.Scope()
	if !scope.Available() || len(metadata.decisions) != scope.Count() {
		return nil, false
	}
	result := make(map[identity.ContentID]equation.Decision, len(metadata.decisions))
	for index, semanticID := range metadata.decisions {
		decision, ok := scope.At(index)
		if !ok || !semanticID.Available() || !decision.Available() {
			return nil, false
		}
		result[semanticID] = decision
	}
	return result, len(result) == len(metadata.decisions)
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
	for index := range row.Writes {
		row.Writes[index].Candidates = append([]uint64(nil), row.Writes[index].Candidates...)
		row.Writes[index].TargetCandidates = append([]equation.Surface(nil), row.Writes[index].TargetCandidates...)
		row.Writes[index].Relations = cloneBindingCandidateRelations(row.Writes[index].Relations)
	}
	row.Supports = append([]equation.ResolvedSupport(nil), row.Supports...)
	row.Prunes = append([]equation.ResolvedPrune(nil), row.Prunes...)
	return row
}

func cloneBindingCandidateRelations(rows []equation.CandidateRelation) []equation.CandidateRelation {
	result := make([]equation.CandidateRelation, len(rows))
	for index, row := range rows {
		result[index] = equation.CandidateRelation{Prior: row.Prior, Matches: make([][]uint64, len(row.Matches))}
		for current, matches := range row.Matches {
			result[index].Matches[current] = append([]uint64(nil), matches...)
		}
	}
	return result
}

func (builder *bindingTopologyBuilder) issueQueryRow(receipt bindingQueryReceipt, query equation.QueryInstance) (bindingQueryRowReceipt, bool) {
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

func (builder *bindingTopologyBuilder) addSemanticQuery(id identity.ContentID, receipt bindingQueryRowReceipt) (bindingQueryRowRef, bool) {
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

func (builder *bindingTopologyBuilder) issueEnvironmentEdge(edge equation.EnvironmentEdge) (BindingEnvironmentEdgeReceipt, bool) {
	inner, ok := builder.lockTopologyOpen()
	if !ok || edge.Target == 0 || !bindingOwnsInput(inner.batch, edge.Input) {
		if ok {
			inner.mu.Unlock()
		}
		return BindingEnvironmentEdgeReceipt{}, false
	}
	inner.mu.Unlock()
	return BindingEnvironmentEdgeReceipt{builder: builder.inner, row: edge, state: builder.inner.state, authority: builder.inner.authority}, true
}

func (builder *bindingTopologyBuilder) addEnvironmentEdge(receipt BindingEnvironmentEdgeReceipt) bool {
	return builder.addRow(func(spec *equation.TopologySpec) bool {
		if receipt.builder != builder.inner || receipt.state != builder.inner.state || receipt.authority != builder.inner.authority || receipt.row.Target == 0 || !bindingOwnsInput(spec.Batch, receipt.row.Input) {
			return false
		}
		spec.EnvironmentEdges = append(spec.EnvironmentEdges, receipt.row)
		return true
	})
}

func (builder *bindingTopologyBuilder) issueFactorEdge(factor bindingFactorReceipt, edge equation.FactorEdge) (BindingFactorEdgeReceipt, bool) {
	inner, ok := builder.lockTopologyOpen()
	if !ok {
		return BindingFactorEdgeReceipt{}, false
	}
	state, authority, semantic, receiptOK := factor.boundTopologyFactorReceipt()
	valid := receiptOK && state == inner.state && authority == inner.authority && semantic == edge.Factor && edge.Target != 0 && edge.Factor.Available() && bindingOwnsInput(inner.batch, edge.Input)
	inner.mu.Unlock()
	if !valid {
		return BindingFactorEdgeReceipt{}, false
	}
	return BindingFactorEdgeReceipt{builder: builder.inner, row: edge, state: state, authority: authority, factor: semantic}, true
}

func (builder *bindingTopologyBuilder) addFactorEdge(receipt BindingFactorEdgeReceipt) bool {
	return builder.addRow(func(spec *equation.TopologySpec) bool {
		if receipt.builder != builder.inner || receipt.state != builder.inner.state || receipt.authority != builder.inner.authority || receipt.factor != receipt.row.Factor || receipt.row.Target == 0 || !bindingOwnsInput(spec.Batch, receipt.row.Input) || !bindingOwnsFactorSchema(builder.inner.state.schema, receipt.row.Factor) {
			return false
		}
		spec.FactorEdges = append(spec.FactorEdges, receipt.row)
		return true
	})
}

func (builder *bindingTopologyBuilder) addSummary(receipt bindingSummarySurfaceReceipt, summary equation.SummaryMapping) bool {
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

func (builder *bindingTopologyBuilder) issueMaterialization(value equation.TemplateMaterialization) (BindingMaterializationReceipt, bool) {
	inner, ok := builder.lockTopologyOpen()
	if !ok || !value.OwnedBy(inner.state.schema.cold, inner.batch) {
		if ok {
			inner.mu.Unlock()
		}
		return BindingMaterializationReceipt{}, false
	}
	inner.mu.Unlock()
	return BindingMaterializationReceipt{builder: builder.inner, value: value, base: builder.inner.batch, state: builder.inner.state, authority: builder.inner.authority}, true
}

func (builder *bindingTopologyBuilder) issueDirectActivationCandidate(value equation.DirectActivationCandidate) (BindingDirectActivationReceipt, bool) {
	inner, ok := builder.lockTopologyOpen()
	if !ok || !value.OwnedBy(inner.state.schema.cold, inner.batch) {
		if ok {
			inner.mu.Unlock()
		}
		return BindingDirectActivationReceipt{}, false
	}
	inner.mu.Unlock()
	return BindingDirectActivationReceipt{builder: builder.inner, value: value, base: builder.inner.batch, state: builder.inner.state, authority: builder.inner.authority}, true
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
func (builder *bindingTopologyBuilder) addSemanticActivation(id identity.ContentID, member BindingRuleRowRef) bool {
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
func (builder *bindingTopologyBuilder) addActivationCandidate(receipt BindingMaterializationReceipt) bool {
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
func (builder *bindingTopologyBuilder) addDirectActivationCandidate(receipt BindingDirectActivationReceipt) bool {
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

func completeBindingSemanticRows(inner *bindingTopologyBuilderState) bool {
	return completeBindingSemanticRowsWithFailure(inner) == ReceiptCommitSemanticRowsFailureNone
}

func completeBindingSemanticRowsWithFailure(inner *bindingTopologyBuilderState) ReceiptCommitSemanticRowsFailure {
	if inner == nil || inner.semantic == nil || len(inner.semantic.points) != len(inner.spec.Points) || len(inner.semantic.pointAt) != len(inner.semantic.points) || len(inner.semantic.members) != len(inner.spec.Rules) || len(inner.semantic.memberAt) != len(inner.semantic.members) || len(inner.semantic.queries) != len(inner.spec.Queries) || len(inner.semantic.activations) != len(inner.semantic.activationAt) || len(inner.semantic.materializationAt) != len(inner.spec.Materializations) || len(inner.semantic.directCandidateAt) != len(inner.spec.DirectCandidates) || len(inner.semantic.directCandidateKey) != len(inner.spec.DirectCandidates) || len(inner.semantic.activationCandidates) != len(inner.semantic.activations) || len(inner.semantic.activationExpected) != len(inner.semantic.activations) || len(inner.semantic.activationApplication) != len(inner.semantic.activations) {
		return ReceiptCommitSemanticRowsFailureCardinality
	}
	for ref := range inner.semantic.activationAt {
		actual, counted := inner.semantic.activationCandidates[ref]
		expected, completed := inner.semantic.activationExpected[ref]
		if !counted || !completed || actual != expected || !inner.semantic.activationApplication[ref].Available() {
			return ReceiptCommitSemanticRowsFailureActivation
		}
	}
	for _, value := range inner.spec.Materializations {
		origin, ok := value.Origin()
		if !ok || origin.TriggerOrdinal < 0 || origin.TriggerOrdinal >= len(inner.spec.Rules) {
			return ReceiptCommitSemanticRowsFailureMaterialization
		}
		ref := equation.RuleAt(origin.TriggerOrdinal)
		owned, found := inner.semantic.materializationAt[value]
		if !found || owned != ref {
			return ReceiptCommitSemanticRowsFailureMaterialization
		}
		if _, registered := inner.semantic.activationAt[ref]; !registered {
			return ReceiptCommitSemanticRowsFailureMaterialization
		}
		if inner.semantic.activationApplication[ref] != origin.Application {
			return ReceiptCommitSemanticRowsFailureMaterialization
		}
	}
	for _, value := range inner.spec.DirectCandidates {
		origin, ok := value.Origin()
		if !ok || origin.TriggerOrdinal < 0 || origin.TriggerOrdinal >= len(inner.spec.Rules) {
			return ReceiptCommitSemanticRowsFailureMaterialization
		}
		ref := equation.RuleAt(origin.TriggerOrdinal)
		owned, found := inner.semantic.directCandidateAt[value]
		key := value.Key()
		keyOwner, keyFound := inner.semantic.directCandidateKey[key]
		if !found || owned != ref || !key.Available() || !keyFound || keyOwner != ref {
			return ReceiptCommitSemanticRowsFailureMaterialization
		}
		if _, registered := inner.semantic.activationAt[ref]; !registered || inner.semantic.activationApplication[ref] != origin.Application {
			return ReceiptCommitSemanticRowsFailureMaterialization
		}
	}
	total := len(inner.semantic.points) + len(inner.semantic.members) + len(inner.semantic.queries) + len(inner.semantic.activations)
	if total != len(inner.semantic.ids) {
		return ReceiptCommitSemanticRowsFailureIDs
	}
	return ReceiptCommitSemanticRowsFailureNone
}

// SealSources closes source admission on the exact Batch retained by this
// builder. The Batch remains the sole source identity authority for the
// topology phase; no rows are copied into a second admission plane.
func (builder *bindingTopologyBuilder) sealSources() ReceiptSourceSealFailure {
	inner, ok := builder.lockSourcesOpen()
	if !ok {
		return ReceiptSourceSealFailurePrecondition
	}
	if failure := inner.batch.SealWithFailure(); failure != ReceiptSourceSealFailureNone {
		inner.failLocked()
		inner.mu.Unlock()
		return failure
	}
	key := inner.batch.Key()
	if !inner.batch.Sealed() || !key.Available() || inner.spec.Batch != inner.batch {
		inner.failLocked()
		inner.mu.Unlock()
		return ReceiptSourceSealFailureBatchIdentity
	}
	inner.sourceKey = key
	inner.phase = bindingTopologyBuilderTopologyOpen
	inner.mu.Unlock()
	return ReceiptSourceSealFailureNone
}

func (inner *bindingTopologyBuilderState) failLocked() {
	if inner == nil {
		return
	}
	inner.phase = bindingTopologyBuilderAborted
	inner.batch = nil
	inner.sourceKey = composition.Key{}
	inner.topology = nil
	inner.receipt = nil
	inner.factors = nil
	inner.semantic = nil
	inner.artifact = nil
	inner.directTransportSets = nil
	inner.spec = equation.TopologySpec{}
}

// Commit materializes exactly one topology and its initial graph from the
// already-sealed source Batch. Neither receipt is published until both have
// been validated, so every failed commit is terminal and fail-closed.
func (builder *bindingTopologyBuilder) commit(deferredQueries bool) (*BindingTopology, *ReceiptGraph, ReceiptCommitFailure, bool) {
	if builder == nil || builder.inner == nil {
		return nil, nil, ReceiptCommitFailure{phase: ReceiptCommitFailurePrecondition, precondition: ReceiptCommitPreconditionBuilder}, false
	}
	inner := builder.inner
	inner.mu.Lock()
	if inner.phase == bindingTopologyBuilderSourcesOpen {
		inner.batch.Reject()
		inner.failLocked()
		inner.mu.Unlock()
		return nil, nil, ReceiptCommitFailure{phase: ReceiptCommitFailurePrecondition, precondition: ReceiptCommitPreconditionSourcesOpen}, false
	}
	// Candidate admission is intentionally open until Commit.  Freeze the
	// observed per-trigger denominator only at this terminal boundary so the
	// semantic-row check can distinguish a complete candidate set from an
	// attachment that is still being assembled.
	if inner.phase == bindingTopologyBuilderTopologyOpen && inner.semantic != nil {
		for ref := range inner.semantic.activationAt {
			inner.semantic.activationExpected[ref] = inner.semantic.activationCandidates[ref]
		}
	}
	precondition := ReceiptCommitPreconditionNone
	switch {
	case inner.phase != bindingTopologyBuilderTopologyOpen:
		precondition = ReceiptCommitPreconditionPhase
	case inner.batch == nil:
		precondition = ReceiptCommitPreconditionBatch
	case inner.state == nil || inner.state.phase != schemaBindingSealed:
		precondition = ReceiptCommitPreconditionBinding
	case inner.state.authority != inner.authority:
		precondition = ReceiptCommitPreconditionAuthority
	case !inner.batch.Sealed():
		precondition = ReceiptCommitPreconditionBatchSeal
	case !inner.sourceKey.Available() || inner.batch.Key() != inner.sourceKey:
		precondition = ReceiptCommitPreconditionSourceKey
	case inner.spec.Batch != inner.batch:
		precondition = ReceiptCommitPreconditionSpecBatch
	case !completeBindingSemanticRows(inner):
		precondition = ReceiptCommitPreconditionSemanticRows
	}
	if precondition != ReceiptCommitPreconditionNone {
		if inner.phase == bindingTopologyBuilderTopologyOpen {
			inner.failLocked()
		}
		inner.mu.Unlock()
		return nil, nil, ReceiptCommitFailure{phase: ReceiptCommitFailurePrecondition, precondition: precondition, semanticRows: completeBindingSemanticRowsWithFailure(inner)}, false
	}
	inner.phase = bindingTopologyBuilderCommitting
	spec := inner.spec
	// Candidate rows retain their immutable transport-set capability. The
	// build-only mount/body cache must not survive into the published plan.
	inner.directTransportSets = nil
	semantic, artifact := inner.semantic, inner.artifact
	state, authority, factors := inner.state, inner.authority, append([]schemaFactorBinding(nil), inner.factors...)
	inner.mu.Unlock()
	var topology *equation.Topology
	var topologyFailure equation.SealTopologyFailure
	var ok bool
	if deferredQueries {
		topology, topologyFailure, ok = equation.SealObservationTopologyWithFailure(state.schema.cold, spec)
	} else {
		topology, topologyFailure, ok = equation.SealTopologyWithFailure(state.schema.cold, spec)
	}
	var graph *equation.Graph
	if ok && topology != nil && topology.OwnsComposition(state.schema.cold) {
		var relation equation.Relation
		relation, ok = topology.InitialRelation()
		if ok {
			graph, ok = topology.Graph(relation)
		}
	} else {
		if ok {
			topologyFailure = equation.SealTopologyFailureInput
		}
		ok = false
	}
	failure := ReceiptCommitFailure{}
	if !ok {
		failure = ReceiptCommitFailure{phase: ReceiptCommitFailureTopology, topology: topologyFailure}
	} else if graph == nil {
		failure = ReceiptCommitFailure{phase: ReceiptCommitFailureGraph}
	}
	if ok && artifact != nil {
		scheduleFailure, scheduleRow, scheduleOK := validateMountedArtifactSchedule(artifact, topology, graph)
		if !scheduleOK {
			ok = false
			failure = ReceiptCommitFailure{phase: ReceiptCommitFailureSchedule, schedule: scheduleFailure, scheduleRow: scheduleRow}
		}
	}
	inner.mu.Lock()
	relockOK := ok && graph != nil && inner.phase == bindingTopologyBuilderCommitting && state.phase == schemaBindingSealed && state.authority == authority && inner.batch != nil && inner.batch.Sealed() && inner.batch.Key() == inner.sourceKey && topology.OwnsGraph(graph)
	if !relockOK {
		inner.failLocked()
		inner.mu.Unlock()
		if failure.phase == ReceiptCommitFailureNone {
			failure = ReceiptCommitFailure{phase: ReceiptCommitFailurePublish, publish: ReceiptCommitPublishFailureRelock}
		}
		return nil, nil, failure, false
	}
	directory, directoryOK := sealSemanticDirectory(topology, state, authority, semantic)
	if !directoryOK {
		inner.failLocked()
		inner.mu.Unlock()
		return nil, nil, ReceiptCommitFailure{phase: ReceiptCommitFailureDirectory}, false
	}
	nativeCallStages, callStagesOK := sealNativeCallStageDirectory(artifact, directory)
	if !callStagesOK {
		inner.failLocked()
		inner.mu.Unlock()
		return nil, nil, ReceiptCommitFailure{phase: ReceiptCommitFailureDirectory}, false
	}
	artifactFunctions, functionsOK := sealArtifactFunctionDirectory(artifact)
	if !functionsOK {
		inner.failLocked()
		inner.mu.Unlock()
		return nil, nil, ReceiptCommitFailure{phase: ReceiptCommitFailureDirectory}, false
	}
	receipt := &BindingTopology{topology: topology, state: state, authority: authority, factors: factors, plan: inner, directory: directory, artifact: artifact, nativeCallStages: nativeCallStages, artifactFunctions: artifactFunctions, artifactBacked: artifact != nil}
	receipt.self = receipt
	inner.topology, inner.receipt, inner.phase = topology, receipt, bindingTopologyBuilderCommitted
	inner.semantic = nil
	if artifact != nil && !artifact.seal(receipt) {
		inner.failLocked()
		inner.mu.Unlock()
		return nil, nil, ReceiptCommitFailure{phase: ReceiptCommitFailurePublish, publish: ReceiptCommitPublishFailureArtifactSeal}, false
	}
	if artifact != nil {
		receipt.bootstrapOwner = artifact.bootstrap.owner
		receipt.bootstrapPoint = artifact.bootstrap.point.PointID
		receipt.bootstrapSemantic = artifact.bootstrap.semantic
	}
	graphReceipt := &ReceiptGraph{graph: graph, topology: receipt, state: state, authority: authority}
	if !receipt.valid() {
		inner.failLocked()
		inner.mu.Unlock()
		return nil, nil, ReceiptCommitFailure{phase: ReceiptCommitFailurePublish, publish: ReceiptCommitPublishFailureBindingTopology}, false
	}
	if !graphReceipt.valid() {
		inner.failLocked()
		inner.mu.Unlock()
		return nil, nil, ReceiptCommitFailure{phase: ReceiptCommitFailurePublish, publish: ReceiptCommitPublishFailureReceiptGraph}, false
	}
	inner.mu.Unlock()
	return receipt, graphReceipt, ReceiptCommitFailure{}, true
}

// Abort terminally consumes an abandoned construction plan. Copies share the
// same ledger, so only one of Commit or Abort can win.
func (builder *bindingTopologyBuilder) abort() bool {
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

// Graph issues the opaque graph receipt for one published activation
// relation, only through this exact construction witness. Foreign topologies
// and equal-schema graphs cannot enter this path.
func (receipt *BindingTopology) Graph(relation equation.Relation) (*ReceiptGraph, bool) {
	if !receipt.valid() {
		return nil, false
	}
	graph, ok := receipt.topology.Graph(relation)
	if !ok {
		return nil, false
	}
	result := &ReceiptGraph{graph: graph, topology: receipt, state: receipt.state, authority: receipt.authority}
	return result, result.valid()
}

func (receipt *ReceiptGraph) lookupPoint(id identity.ContentID) (receiptPoint, bool) {
	if !receipt.valid() {
		return receiptPoint{}, false
	}
	locator, found := receipt.topology.directory.point(id)
	if !found {
		return receiptPoint{}, false
	}
	point, ok := locator.Resolve(receipt.graph)
	if !ok || !receipt.graph.OwnsPoint(point) {
		return receiptPoint{}, false
	}
	return receiptPoint{graph: receipt, point: point}, true
}

func (receipt *ReceiptGraph) lookupRuleMember(id identity.ContentID) (ReceiptRuleMember, bool) {
	if !receipt.valid() {
		return ReceiptRuleMember{}, false
	}
	locator, found := receipt.topology.directory.member(id)
	if !found {
		return ReceiptRuleMember{}, false
	}
	member, ok := locator.Resolve(receipt.graph)
	if !ok || !receipt.graph.OwnsMember(member) {
		return ReceiptRuleMember{}, false
	}
	return ReceiptRuleMember{graph: receipt, member: member, locator: locator}, true
}

// RuleMember returns the exact graph-owned member admitted under id.  The
// returned receipt is foreign-graph resistant and is accepted only by the
// matching typed ReceiptCompilation transaction.
func (receipt *ReceiptGraph) RuleMember(id identity.ContentID) (ReceiptRuleMember, bool) {
	return receipt.lookupRuleMember(id)
}

// MountedRuleMember resolves an authored mounted occurrence through the
// sealed artifact role directory. Callers cannot forge the member identity or
// bypass the mount/point/role fence with a raw ContentID.
func (receipt *ReceiptGraph) MountedRuleMember(role RuleSlotCapability, mount, reusablePoint, occurrence identity.ContentID) (ReceiptRuleMember, bool) {
	if !role.mounted() || !receipt.valid() || receipt.topology == nil {
		return ReceiptRuleMember{}, false
	}
	return receipt.lookupRuleMember(mountedRuleMemberID(role, mount, reusablePoint, occurrence))
}

// LinkRuleMember resolves one Link-global bootstrap row through the sealed
// witness catalog. The caller supplies only the closed role and witness
// occurrence; owner/point identity remains engine-private.
func (receipt *ReceiptGraph) LinkRuleMember(role RuleSlotCapability, occurrence identity.ContentID) (ReceiptRuleMember, bool) {
	if !role.link() || !receipt.valid() || receipt.topology == nil || role.state != receipt.state || role.authority != receipt.authority || !receipt.topology.bootstrapOwner.Available() || !receipt.topology.bootstrapPoint.Available() {
		return ReceiptRuleMember{}, false
	}
	return receipt.lookupRuleMember(linkRuleMemberID(role, receipt.topology.bootstrapOwner, receipt.topology.bootstrapPoint, occurrence))
}

// ReleaseArtifactReceipt drops the temporary expanded mounted-row snapshot
// after commit. Equation topology plus its semantic directory remain the sole
// immutable structural authority; future attachment calls resolve admitted
// mounted identities directly from that directory.
func (receipt *ReceiptGraph) ReleaseArtifactReceipt() bool {
	if receipt == nil || !receipt.valid() || receipt.topology == nil {
		return false
	}
	topology := receipt.topology
	if topology.artifact == nil {
		return true
	}
	if topology.artifact.bootstrap == nil || topology.bootstrapOwner != topology.artifact.bootstrap.owner || topology.bootstrapPoint != topology.artifact.bootstrap.point.PointID || topology.bootstrapSemantic != topology.artifact.bootstrap.semantic {
		return false
	}
	topology.artifact = nil
	topology.factors = nil
	if topology.plan != nil {
		topology.plan.mu.Lock()
		topology.plan.artifact = nil
		// The equation Topology and semantic directory are already sealed. The
		// construction Batch/spec are a second expanded representation and have
		// no post-commit readers; retain only the source key and terminal phase.
		topology.plan.batch = nil
		topology.plan.spec = equation.TopologySpec{}
		topology.plan.factors = nil
		topology.plan.mu.Unlock()
	}
	return true
}

func (receipt *ReceiptGraph) ActivationMember(id identity.ContentID) (ActivationReceiptMember, bool) {
	graph, ok := receipt.ActivationGraph()
	if !ok {
		return ActivationReceiptMember{}, false
	}
	return graph.lookupActivationMember(id)
}

func (receipt *ReceiptGraph) MountedActivationMember(role RuleSlotCapability, mount, reusablePoint, occurrence identity.ContentID) (ActivationReceiptMember, bool) {
	graph, ok := receipt.ActivationGraph()
	if !ok {
		return ActivationReceiptMember{}, false
	}
	return graph.MountedRuleMember(role, mount, reusablePoint, occurrence)
}

func (receipt *ReceiptGraph) lookupQuery(id identity.ContentID) (ReceiptQuery, bool) {
	if !receipt.valid() {
		return ReceiptQuery{}, false
	}
	locator, found := receipt.topology.directory.query(id)
	if !found {
		return ReceiptQuery{}, false
	}
	query, ok := locator.Resolve(receipt.graph)
	if !ok || !receipt.graph.OwnsQuery(query) {
		return ReceiptQuery{}, false
	}
	return ReceiptQuery{graph: receipt, identity: query, locator: locator}, true
}

func activationReceiptGraph(receipt *ReceiptGraph) (*ActivationReceiptGraph, bool) {
	if !receipt.valid() || receipt.topology == nil || receipt.topology.topology == nil {
		return nil, false
	}
	result := &ActivationReceiptGraph{receipt: receipt, topology: receipt.topology.topology}
	return result, result.valid()
}

// ActivationGraph projects one committed ReceiptGraph into the exact
// activation attachment witness.  The projection is sealed to the graph's
// Binding authority and cannot be forged from an equal topology.
func (receipt *ReceiptGraph) ActivationGraph() (*ActivationReceiptGraph, bool) {
	return activationReceiptGraph(receipt)
}

func (receipt *ActivationReceiptGraph) lookupActivationMember(id identity.ContentID) (ActivationReceiptMember, bool) {
	if !receipt.valid() {
		return ActivationReceiptMember{}, false
	}
	locator, found := receipt.receipt.topology.directory.activation(id)
	if !found {
		return ActivationReceiptMember{}, false
	}
	member, ok := locator.Resolve(receipt.receipt.graph)
	if !ok || !receipt.receipt.graph.OwnsMember(member) {
		return ActivationReceiptMember{}, false
	}
	return ActivationReceiptMember{graph: receipt, member: member, locator: locator}, true
}

// MountedRuleMember resolves the activation member paired with one exact
// mounted semantic occurrence. The role-qualified identity is issued by the
// same artifact directory used for ordinary Rule members.
func (receipt *ActivationReceiptGraph) MountedRuleMember(role RuleSlotCapability, mount, reusablePoint, occurrence identity.ContentID) (ActivationReceiptMember, bool) {
	if !role.mounted() || !receipt.valid() || receipt.receipt == nil || receipt.receipt.topology == nil {
		return ActivationReceiptMember{}, false
	}
	return receipt.lookupActivationMember(mountedRuleActivationID(role, mount, reusablePoint, occurrence))
}

// solverCompiler is the private activation-revision lowering seam. Receipt
// compilations rebuild graph-owned runtime state without a cold Composition.
type solverCompiler interface {
	compile(equation.Relation) (*solverRuntime, SolveFailurePhase, bool)
}

func validAcceptedActivations(topology *equation.Topology, accepted []equation.AcceptedMember) bool {
	if topology == nil {
		return false
	}
	// Publish repeats this fail-closed validation at the authority boundary. The
	// compiler keeps the same predicate here so an untrusted caller cannot get as
	// far as carrier allocation with a foreign Member. It is membership only: no
	// structural digest is derived for a set that is not being published.
	return topology.ValidAccepted(accepted)
}

// receiptFactorCompilation is the private transaction state between Factor
// binding and the future Rule/Query receipt compiler. It retains the pinned
// runtime binding and its guards; callers cannot receive orphaned factors
// after the owner is discarded.
type receiptFactorCompilation struct {
	mu                  sync.Mutex
	runtime             *runtimeBinding
	factors             []runtimeFactor
	byKey               map[composition.Key]runtimeFactor
	carrier             *carrier.Composition
	ordered             []runtimeFactor
	members             map[composition.Key]runtimeMember
	queries             map[composition.Key]runtimeQuery
	observations        []runtimeObservation
	observationIDs      map[identity.ContentID]struct{}
	observationPoints   map[composition.Key]equation.Point
	memberBuilders      []receiptMemberBuilder
	queryBuilders       []receiptQueryBuilder
	observationBuilders []receiptObservationBuilder
	frozen              bool
	closed              bool
}

// The receipt transaction retains only typed, one-shot rebinding closures
// until Solver is sealed.  They let a later accepted activation revision
// rebuild its graph-owned runtime without retaining a cold Composition or
// exposing erased callback state.
type receiptMemberBuilder func(*receiptFactorCompilation) (runtimeMember, bool)
type receiptQueryBuilder func(*receiptFactorCompilation) (runtimeQuery, bool)
type receiptObservationBuilder func(*receiptFactorCompilation) (runtimeObservation, bool)

type receiptSolverCompiler struct {
	state               *schemaBindingState
	topology            *equation.Topology
	memberBuilders      []receiptMemberBuilder
	queryBuilders       []receiptQueryBuilder
	observationBuilders []receiptObservationBuilder
}

func (compiler receiptSolverCompiler) compile(relation equation.Relation) (*solverRuntime, SolveFailurePhase, bool) {
	if compiler.state == nil || compiler.topology == nil || !relation.OwnedBy(compiler.topology) {
		return nil, SolveFailurePhaseCompileValidation, false
	}
	graph, ok := compiler.topology.Graph(relation)
	if !ok || graph == nil {
		return nil, SolveFailurePhaseCompileValidation, false
	}
	binding := &SchemaBinding{state: compiler.state}
	compiled, ok := compileReceiptFactors(binding, graph)
	if !ok || compiled == nil {
		return nil, SolveFailurePhaseCompileComposition, false
	}
	rows := make([]runtimeMember, 0, len(compiler.memberBuilders))
	for _, build := range compiler.memberBuilders {
		if build == nil {
			return nil, SolveFailurePhaseCompileMemberBinding, false
		}
		row, built := build(compiled)
		if !built || row == nil {
			return nil, SolveFailurePhaseCompileMemberBinding, false
		}
		rows = append(rows, row)
	}
	queryByKey := make(map[composition.Key]runtimeQuery, len(compiler.queryBuilders))
	for _, build := range compiler.queryBuilders {
		if build == nil {
			return nil, SolveFailurePhaseCompileQueryBinding, false
		}
		row, built := build(compiled)
		if !built || row == nil {
			return nil, SolveFailurePhaseCompileQueryBinding, false
		}
		if _, duplicate := queryByKey[row.query().Key()]; duplicate {
			return nil, SolveFailurePhaseCompileQueryBinding, false
		}
		queryByKey[row.query().Key()] = row
	}
	queries := make([]runtimeQuery, graph.QueryCount())
	for index := 0; index < graph.QueryCount(); index++ {
		identity, indexed := graph.QueryAt(index)
		row, present := queryByKey[identity.Key()]
		if !indexed || !present || row == nil || row.query().Key() != identity.Key() {
			return nil, SolveFailurePhaseCompileQueryBinding, false
		}
		queries[index] = row
	}
	observations := make([]runtimeObservation, 0, len(compiler.observationBuilders))
	for _, build := range compiler.observationBuilders {
		if build == nil {
			return nil, SolveFailurePhaseCompileRuntimeAssembly, false
		}
		row, built := build(compiled)
		if !built || row == nil {
			return nil, SolveFailurePhaseCompileRuntimeAssembly, false
		}
		observations = append(observations, row)
	}
	runtime, ok := assembleReceiptRuntime(compiled.runtime.state, compiled.runtime.authority, graph, compiled.carrier, compiled.byKey, rows, queries, observations)
	if !ok || runtime == nil {
		return nil, SolveFailurePhaseCompileRuntimeAssembly, false
	}
	runtime.factors = append([]runtimeFactor(nil), compiled.ordered...)
	runtime.topology = compiler.topology
	for _, factor := range compiled.ordered {
		if factor == nil {
			return nil, SolveFailurePhaseCompileComposition, false
		}
		factor.releaseColdBindings()
	}
	return runtime, SolveFailurePhaseNone, true
}

// ReceiptCompilation is the opaque receipt-native Rule attachment transaction.
// It owns the sealed Factor carrier and graph catalog; callers can only add a
// typed Rule implementation/member pair and cannot observe slots, callbacks,
// graph mutation, or carrier coordinates.
type ReceiptCompilation struct {
	inner *receiptFactorCompilation
	graph *ReceiptGraph
}

// ReceiptMember is an opaque graph-owned Rule member attached to a
// ReceiptCompilation. Runtime execution remains engine-owned.
type ReceiptMember struct {
	inner runtimeMember
}

// ReceiptQuery is an opaque graph-owned Query identity used by the common
// receipt-native solver transaction.
type ReceiptQuery struct {
	graph    *ReceiptGraph
	identity equation.Query
	locator  equation.QueryRowLocator
}

// ActivationReceiptCompilation is the closed receipt-native attachment
// transaction for structural activation members.  It uses the same Factor
// carrier and runtime member machinery as ReceiptCompilation, but can only
// be started through an ActivationReceiptGraph's topology proof.
type ActivationReceiptCompilation struct {
	inner *receiptFactorCompilation
	graph *ActivationReceiptGraph
}

// ActivationReceiptMember is an opaque successfully-attached activation
// runtime member.  Its internals remain engine-owned.
type AttachedActivationReceiptMember struct {
	inner runtimeMember
}

// Close terminally seals this receipt attachment transaction. Copies of the
// opaque handle share the same ledger and therefore cannot attach after close.
func (compilation *ReceiptCompilation) Close() bool {
	if compilation == nil || compilation.inner == nil {
		return false
	}
	compilation.inner.mu.Lock()
	defer compilation.inner.mu.Unlock()
	if compilation.inner.closed {
		return false
	}
	compilation.inner.closed = true
	return true
}

// Close terminally closes an activation receipt transaction.
func (compilation *ActivationReceiptCompilation) Close() bool {
	if compilation == nil || compilation.inner == nil {
		return false
	}
	compilation.inner.mu.Lock()
	defer compilation.inner.mu.Unlock()
	if compilation.inner.closed {
		return false
	}
	compilation.inner.closed = true
	return true
}

// Solver seals the receipt compilation and assembles its attached members
// into the normal executable runtime. The returned Solver follows the same
// Product/evidence/patch path; no receipt-only
// execution shortcut exists. Query-bearing graphs require their typed query
// receipt lane and are rejected until that lane is attached.
func (compilation *ReceiptCompilation) Solver() (*Solver, bool) {
	if compilation == nil || compilation.inner == nil || compilation.graph == nil {
		return nil, false
	}
	inner := compilation.inner
	inner.mu.Lock()
	if inner.closed || inner.runtime == nil || inner.carrier == nil || inner.members == nil || inner.queries == nil || inner.observationIDs == nil || compilation.graph.graph == nil {
		inner.mu.Unlock()
		return nil, false
	}
	inner.closed = true
	rows := make([]runtimeMember, 0, len(inner.members))
	for _, row := range inner.members {
		if row == nil {
			inner.mu.Unlock()
			return nil, false
		}
		rows = append(rows, row)
	}
	directory := compilation.graph.topology.directory
	if !directory.ownedBy(compilation.graph.topology.topology, compilation.graph.state, compilation.graph.authority) {
		inner.mu.Unlock()
		return nil, false
	}
	queries := make([]runtimeQuery, compilation.graph.graph.QueryCount())
	for index := 0; index < compilation.graph.graph.QueryCount(); index++ {
		identity, ok := compilation.graph.graph.QueryAt(index)
		if !ok || !identity.Key().Available() {
			inner.mu.Unlock()
			return nil, false
		}
		row, ok := inner.queries[identity.Key()]
		if !ok || row == nil {
			inner.mu.Unlock()
			return nil, false
		}
		queries[index] = row
	}
	observations := append([]runtimeObservation(nil), inner.observations...)
	for _, observation := range observations {
		if observation == nil {
			inner.mu.Unlock()
			return nil, false
		}
	}
	carrier := inner.carrier
	graph := compilation.graph.graph
	ordered := append([]runtimeFactor(nil), inner.ordered...)
	byKey := inner.byKey
	state, authority := inner.runtime.state, inner.runtime.authority
	compiler := receiptSolverCompiler{
		state: state, topology: compilation.graph.topology.topology,
		memberBuilders:      append([]receiptMemberBuilder(nil), inner.memberBuilders...),
		queryBuilders:       append([]receiptQueryBuilder(nil), inner.queryBuilders...),
		observationBuilders: append([]receiptObservationBuilder(nil), inner.observationBuilders...),
	}
	inner.mu.Unlock()

	runtime, ok := assembleReceiptRuntime(state, authority, graph, carrier, byKey, rows, queries, observations)
	if !ok || runtime == nil {
		return nil, false
	}
	runtime.factors = append([]runtimeFactor(nil), ordered...)
	runtime.topology = compilation.graph.topology.topology
	for _, factor := range ordered {
		if factor == nil {
			return nil, false
		}
		factor.releaseColdBindings()
	}
	inner.mu.Lock()
	inner.runtime = nil
	inner.factors = nil
	inner.byKey = nil
	inner.ordered = nil
	inner.members = nil
	inner.queries = nil
	inner.observations = nil
	inner.observationIDs = nil
	inner.observationPoints = nil
	inner.carrier = nil
	inner.mu.Unlock()
	relation, relationOK := runtime.topology.InitialRelation()
	store, storeOK := solverStores.issue()
	if !relationOK || !storeOK {
		return nil, false
	}
	return &Solver{runtime: runtime, compiler: compiler, store: store, relation: relation}, true
}

// BeginReceiptCompilation starts a receipt compiler from one exact sealed Rule
// receipt and graph. The Rule receipt supplies the SchemaBinding authority, so
// an equal-but-foreign binding or an unsealed implementation cannot enter.
func BeginReceiptCompilation[K ~uint32 | ~uint64, V, O any](implementation *RuleImplementation[K, V, O], graph *ReceiptGraph) (*ReceiptCompilation, bool) {
	if implementation == nil || !implementation.receipt.valid() || !graph.valid() || implementation.receipt.state != graph.state || implementation.receipt.authority != graph.authority {
		return nil, false
	}
	binding := &SchemaBinding{state: graph.state}
	compiled, ok := compileReceiptFactors(binding, graph.graph)
	if !ok || compiled == nil {
		return nil, false
	}
	return &ReceiptCompilation{inner: compiled, graph: graph}, true
}

// BeginReceiptActivationCompilation opens the ordinary ReceiptCompilation
// for a structural activation implementation. Activations and factor Rules
// now share one graph, carrier, member catalog, and Solver terminal path.
func BeginReceiptActivationCompilation(implementation *ActivationRuleImplementation, graph *ReceiptGraph) (*ReceiptCompilation, bool) {
	if implementation == nil || !implementation.receipt.valid() || !graph.valid() || implementation.receipt.state != graph.state || implementation.receipt.authority != graph.authority {
		return nil, false
	}
	binding := &SchemaBinding{state: graph.state}
	compiled, ok := compileReceiptFactors(binding, graph.graph)
	if !ok || compiled == nil {
		return nil, false
	}
	return &ReceiptCompilation{inner: compiled, graph: graph}, true
}

// AttachRuleMember attaches one exact graph-owned member through the existing
// typed receipt binder. The generic boundary preserves the Rule operand type;
// no erased operand, Factor slot, callback, or raw Ref is accepted.
func AttachReceiptRuleMember[K ~uint32 | ~uint64, V, O any](compilation *ReceiptCompilation, implementation *RuleImplementation[K, V, O], member ReceiptRuleMember, operand O) (*ReceiptMember, bool) {
	if compilation == nil || compilation.inner == nil || compilation.graph == nil || member.graph != compilation.graph {
		return nil, false
	}
	locator := member.locator
	resolved, located := locator.Resolve(compilation.graph.graph)
	if !located || resolved.Key() != member.member.Key() {
		return nil, false
	}
	compilation.inner.mu.Lock()
	row, ok := bindReceiptRuleMemberLocked(compilation.inner, implementation, member.member, operand)
	if !ok || row == nil {
		compilation.inner.mu.Unlock()
		return nil, false
	}
	compilation.inner.memberBuilders = append(compilation.inner.memberBuilders, func(next *receiptFactorCompilation) (runtimeMember, bool) {
		resolved, ok := locator.Resolve(next.runtime.graph)
		if !ok {
			return nil, false
		}
		return bindReceiptRuleMember(next, implementation, resolved, operand)
	})
	compilation.inner.mu.Unlock()
	return &ReceiptMember{inner: row}, true
}

func attachReceiptQueryLocked(inner *receiptFactorCompilation, query ReceiptQuery, row runtimeQuery) bool {
	if inner == nil {
		return false
	}
	if inner.closed || inner.queries == nil || !query.identity.Key().Available() {
		return false
	}
	if _, duplicate := inner.queries[query.identity.Key()]; duplicate {
		return false
	}
	inner.queries[query.identity.Key()] = row
	return true
}

func AttachReceiptExactQuery[V, R any](compilation *ReceiptCompilation, implementation *ExactQueryImplementation[V, R], query ReceiptQuery) bool {
	if compilation == nil || compilation.inner == nil || compilation.graph == nil || query.graph != compilation.graph || implementation == nil {
		return false
	}
	locator := query.locator
	resolved, located := locator.Resolve(compilation.graph.graph)
	if !located || resolved.Key() != query.identity.Key() {
		return false
	}
	compilation.inner.mu.Lock()
	row, ok := bindReceiptExactQueryRuntime[V, R](compilation.inner, implementation, query.identity)
	if !ok || !attachReceiptQueryLocked(compilation.inner, query, row) {
		compilation.inner.mu.Unlock()
		return false
	}
	compilation.inner.queryBuilders = append(compilation.inner.queryBuilders, func(next *receiptFactorCompilation) (runtimeQuery, bool) {
		identity, ok := locator.Resolve(next.runtime.graph)
		if !ok {
			return nil, false
		}
		return bindReceiptExactQueryRuntime[V, R](next, implementation, identity)
	})
	compilation.inner.mu.Unlock()
	return true
}

func AttachReceiptSummaryQuery[V, R any](compilation *ReceiptCompilation, implementation *SummaryQueryImplementation[V, R], query ReceiptQuery) bool {
	if compilation == nil || compilation.inner == nil || compilation.graph == nil || query.graph != compilation.graph || implementation == nil {
		return false
	}
	locator := query.locator
	resolved, located := locator.Resolve(compilation.graph.graph)
	if !located || resolved.Key() != query.identity.Key() {
		return false
	}
	compilation.inner.mu.Lock()
	row, ok := bindReceiptSummaryQueryRuntime[V, R](compilation.inner, implementation, query.identity)
	if !ok || !attachReceiptQueryLocked(compilation.inner, query, row) {
		compilation.inner.mu.Unlock()
		return false
	}
	compilation.inner.queryBuilders = append(compilation.inner.queryBuilders, func(next *receiptFactorCompilation) (runtimeQuery, bool) {
		identity, ok := locator.Resolve(next.runtime.graph)
		if !ok {
			return nil, false
		}
		return bindReceiptSummaryQueryRuntime[V, R](next, implementation, identity)
	})
	compilation.inner.mu.Unlock()
	return true
}

// AttachReceiptActivationMember attaches an activation into the ordinary
// ReceiptCompilation. The activation graph must be the projection of that
// compilation's exact ReceiptGraph; equal-but-foreign graphs are rejected.
func AttachReceiptActivationMember(compilation *ReceiptCompilation, implementation *ActivationRuleImplementation, member ActivationReceiptMember) (*AttachedActivationReceiptMember, bool) {
	if compilation == nil || compilation.inner == nil || compilation.graph == nil || member.graph == nil || member.graph.receipt != compilation.graph || !member.graph.valid() || implementation == nil || !implementation.receipt.valid() {
		return nil, false
	}
	inner := compilation.inner
	inner.mu.Lock()
	defer inner.mu.Unlock()
	if inner.closed || !inner.frozen || inner.runtime == nil || inner.runtime.mode != runtimeBindingReceipt || implementation.receipt.state != inner.runtime.state || implementation.receipt.authority != inner.runtime.authority || !compilation.graph.graph.OwnsMember(member.member) || !member.member.Key().Available() {
		return nil, false
	}
	if _, duplicate := inner.members[member.member.Key()]; duplicate {
		return nil, false
	}
	locator := member.locator
	resolved, located := locator.Resolve(compilation.graph.graph)
	if !located || resolved.Key() != member.member.Key() {
		return nil, false
	}
	row, ok := bindActivationMemberReceipt(member.member, implementation, compilation.graph.topology.topology, member.member.Key(), compilation.graph.graph, inner.byKey)
	if !ok || row == nil || row.member().Key() != member.member.Key() {
		return nil, false
	}
	inner.members[member.member.Key()] = row
	inner.memberBuilders = append(inner.memberBuilders, func(next *receiptFactorCompilation) (runtimeMember, bool) {
		resolved, ok := locator.Resolve(next.runtime.graph)
		if !ok {
			return nil, false
		}
		return bindActivationMemberReceipt(resolved, implementation, compilation.graph.topology.topology, resolved.Key(), next.runtime.graph, next.byKey)
	})
	return &AttachedActivationReceiptMember{inner: row}, true
}

// compileReceiptFactors is the sealed Factor-only compiler entry. It uses the
// same graph catalog and boundFactor/runtimeFactor path as the
// compiler, but enumerates the private SchemaBinding cells by canonical
// ordinal. It deliberately stops before Rule/Query lowering, whose callback
// contracts remain receipt-owned.
func compileReceiptFactors(binding *SchemaBinding, graph *equation.Graph) (*receiptFactorCompilation, bool) {
	runtime, ok := newReceiptRuntimeBinding(binding, graph)
	if !ok || runtime == nil {
		return nil, false
	}
	factors, byKey, ok := bindReceiptFactors(binding, runtime)
	if !ok || !runtime.freezeCatalog() {
		return nil, false
	}
	prepared, ordered, ok := prepareRuntimeComposition(factors, runtime.guards)
	if !ok || prepared == nil {
		return nil, false
	}
	attached, ok := prepared.Attach()
	if !ok || attached == nil {
		return nil, false
	}
	for _, factor := range ordered {
		preparer, preparable := factor.(interface{ prepareRouteTransformClosure() bool })
		if !preparable || !preparer.prepareRouteTransformClosure() {
			return nil, false
		}
	}
	return &receiptFactorCompilation{
		runtime: runtime, factors: factors, byKey: byKey, carrier: attached, ordered: ordered,
		members: make(map[composition.Key]runtimeMember), queries: make(map[composition.Key]runtimeQuery),
		observationIDs: make(map[identity.ContentID]struct{}), frozen: true,
	}, true
}

// bindRuleMember consumes one cell-issued Rule implementation and one exact
// graph member. Receipt compilation has no fallback: a migrated member
// can enter this transaction only through the Binding authority that already
// owns its output Factor implementation.
func bindReceiptRuleMember[K ~uint32 | ~uint64, V, O any](compilation *receiptFactorCompilation, implementation *RuleImplementation[K, V, O], member equation.RuleMember, operand O) (runtimeMember, bool) {
	if compilation == nil {
		return nil, false
	}
	compilation.mu.Lock()
	defer compilation.mu.Unlock()
	return bindReceiptRuleMemberLocked(compilation, implementation, member, operand)
}

func bindReceiptRuleMemberLocked[K ~uint32 | ~uint64, V, O any](compilation *receiptFactorCompilation, implementation *RuleImplementation[K, V, O], member equation.RuleMember, operand O) (runtimeMember, bool) {
	if compilation.closed || !compilation.frozen || compilation.runtime == nil || compilation.runtime.mode != runtimeBindingReceipt || compilation.runtime.graph == nil || compilation.carrier == nil || compilation.members == nil || implementation == nil || !implementation.receipt.valid() || implementation.receipt.state != compilation.runtime.state || implementation.receipt.authority != compilation.runtime.authority || !compilation.runtime.graph.OwnsMember(member) || !member.Key().Available() {
		return nil, false
	}
	if _, duplicate := compilation.members[member.Key()]; duplicate {
		return nil, false
	}
	output, present := compilation.byKey[implementation.receipt.proof.output]
	if !present || output == nil {
		return nil, false
	}
	row, ok := bindSchemaRuleMember(implementation, member, operand, output, compilation.byKey)
	if !ok || row == nil || row.member().Key() != member.Key() {
		return nil, false
	}
	compilation.members[member.Key()] = row
	return row, true
}

func bindReceiptFactors(binding *SchemaBinding, runtime *runtimeBinding) ([]runtimeFactor, map[composition.Key]runtimeFactor, bool) {
	state := bindingState(binding)
	if state == nil || runtime == nil || runtime.mode != runtimeBindingReceipt || runtime.state != state || runtime.authority == nil || !runtime.valid() {
		return nil, nil, false
	}
	state.mu.Lock()
	if state.phase != schemaBindingSealed || state.authority != runtime.authority || state.schema != runtime.schema {
		state.mu.Unlock()
		return nil, nil, false
	}
	cells := append([]schemaFactorBinding(nil), state.factors...)
	schema := state.schema
	state.mu.Unlock()
	if len(cells) != schemaFactorCount(schema) {
		return nil, nil, false
	}
	factors := make([]runtimeFactor, len(cells))
	byKey := make(map[composition.Key]runtimeFactor, len(cells))
	for ordinal, cell := range cells {
		if cell == nil || cell.schemaFactorOrdinal() != uint64(ordinal) || cell.schemaFactorSchema() != schema || !cell.schemaFactorComplete() {
			return nil, nil, false
		}
		factor, bound := cell.schemaFactorRuntimeBinding(runtime)
		key := schema.factorSemanticAt(uint64(ordinal))
		if !bound || factor == nil || !key.Available() || compositionKeyOf(factor.semantic()) != key {
			return nil, nil, false
		}
		if _, duplicate := byKey[key]; duplicate {
			return nil, nil, false
		}
		factors[ordinal], byKey[key] = factor, factor
	}
	return factors, byKey, true
}
