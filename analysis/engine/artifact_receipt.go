package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

const (
	linkBootstrapTransportDomain = "analysis/engine/link-bootstrap-factor-transfer"

	// artifactContentIdentityVersion versions the mount-qualified content
	// identities below. It is independent of the cold source-key versions: a
	// content identity names artifact content, a source key names one cold
	// namespace that content is admitted into.
	artifactContentIdentityVersion uint64 = 1

	artifactPointSourceVersion      uint64 = 1
	artifactEdgeSourceVersion       uint64 = 2
	artifactOccurrenceSourceVersion uint64 = 3
)

// artifactReceiptTopology is BindingTopology's immutable copy of the exact
// Program artifact structural receipt. It intentionally retains only scalar
// semantic identities and the parent-issued WTO bracket stream; it never
// retains a Program, Flow, callback builder, or alternate topology authority.
type artifactReceiptTopology struct {
	sealed     *BindingTopology
	mounts     []artifactMountReceipt
	points     []identity.ContentID
	pointMeta  map[identity.ContentID]artifactPointMetadata
	sites      map[identity.ContentID]equation.Site
	mounted    map[artifactMountedPoint]equation.Site
	mountedRef map[artifactMountedPoint]equation.PointRef
	bodies     map[artifactMountedBody]artifactBodyTransport
	ruleSet    map[artifactMountedRule]artifactRuleInput
	callStages map[artifactMountedRuleOccurrence]artifactNativeCallStage
	pointRef   map[identity.ContentID]equation.PointRef
	edges      []artifactEnvironmentRow
	regions    []artifactWTORegionRow
	events     []artifactWTOEventRow
	bootstrap  *linkBootstrapReceipt
}

type linkBootstrapReceipt struct {
	owner       identity.ContentID
	point       LinkBootstrapPoint
	site        equation.Site
	occurrences map[identity.ContentID]struct{}
	roles       map[identity.ContentID]RuleSlotCapability
	claims      map[identity.ContentID]RuleSlotCapability
	semantic    identity.ContentID
	ref         equation.PointRef
	transports  []linkBootstrapTransport
}

type linkBootstrapTransport struct {
	capability RuleSlotCapability
	factor     composition.Key
}

type artifactMountedPoint struct {
	mount    identity.ContentID
	reusable identity.ContentID
}

// artifactMountedBody is the immutable parent-issued body transport anchor.
// Point IDs remain reusable artifact IDs here; every later use must resolve
// them through the same mount-qualified point receipt.
type artifactMountedBody struct {
	mount identity.ContentID
	body  identity.ContentID
}

type artifactBodyTransport struct {
	entry []identity.ContentID
	exits []identity.ContentID
}

type artifactMountedRule struct {
	role       RuleSlotCapability
	mount      identity.ContentID
	point      identity.ContentID
	occurrence identity.ContentID
}

type artifactRuleInput struct {
	point        identity.ContentID
	mountedPoint identity.ContentID
	mountedInput identity.ContentID
	stage        rows.ArtifactRuleStage
	predecessor  artifactEnvironmentRow
	routed       bool
}

// artifactMountedRuleOccurrence deliberately omits the reusable point. Native
// Call stages are keyed by the exact mounted occurrence, so a second stage
// point for the same owner/call is an alias and is rejected while snapshotting.
type artifactMountedRuleOccurrence struct {
	role       RuleSlotCapability
	mount      identity.ContentID
	occurrence identity.ContentID
}

type artifactNativeCallStage struct {
	stage        rows.ArtifactRuleStage
	point        identity.ContentID
	input        identity.ContentID
	mountedPoint identity.ContentID
	mountedInput identity.ContentID
}

// ArtifactScalarBinding is the short-lived Link substitution for one reusable
// template. Every declared role is bound exactly once to an exact capability;
// the structural template remains owner-neutral.
type ArtifactScalarBinding struct {
	template     *rows.ArtifactScalarTemplate
	capabilities map[rows.ArtifactScalarRole]RuleSlotCapability
	sealed       bool
}

// ArtifactScalarReceipt is the immutable mounted-input pair. It retains the
// shared Program template plus only Link-local role substitutions.
type ArtifactScalarReceipt struct {
	template     *rows.ArtifactScalarTemplate
	capabilities map[rows.ArtifactScalarRole]RuleSlotCapability
	sealed       bool
}

func NewArtifactScalarBinding(template *rows.ArtifactScalarTemplate) (*ArtifactScalarBinding, bool) {
	if !template.Available() {
		return nil, false
	}
	return &ArtifactScalarBinding{template: template, capabilities: make(map[rows.ArtifactScalarRole]RuleSlotCapability, template.RoleCount())}, true
}

func (binding *ArtifactScalarBinding) BindRole(role rows.ArtifactScalarRole, capability RuleSlotCapability) bool {
	if binding == nil || binding.sealed || !binding.template.OwnsRole(role) || !capability.mounted() {
		return false
	}
	if _, duplicate := binding.capabilities[role]; duplicate {
		return false
	}
	binding.capabilities[role] = capability
	return true
}

func NewArtifactScalarReceipt(binding *ArtifactScalarBinding) (*ArtifactScalarReceipt, bool) {
	if binding == nil || binding.sealed || !binding.template.Available() || len(binding.capabilities) != binding.template.RoleCount() {
		return nil, false
	}
	seenCapabilities := make(map[RuleSlotCapability]struct{}, binding.template.RoleCount())
	for index := 0; index < binding.template.RoleCount(); index++ {
		role, roleOK := binding.template.RoleAt(index)
		if !roleOK {
			return nil, false
		}
		capability, ok := binding.capabilities[role]
		if !ok || !capability.mounted() {
			return nil, false
		}
		if _, duplicate := seenCapabilities[capability]; duplicate {
			return nil, false
		}
		seenCapabilities[capability] = struct{}{}
	}
	binding.sealed = true
	return &ArtifactScalarReceipt{template: binding.template, capabilities: binding.capabilities, sealed: true}, true
}

func (receipt *ArtifactScalarReceipt) capability(role rows.ArtifactScalarRole) (RuleSlotCapability, bool) {
	if receipt == nil || !receipt.sealed || !receipt.template.Available() || !role.Available() {
		return RuleSlotCapability{}, false
	}
	capability, ok := receipt.capabilities[role]
	return capability, ok && capability.mounted()
}

// MountedArtifactReceipt is the opaque Link-owned input row for the sole
// multi-mount lowerer.  MountID is the parent-issued module/shard
// substitution identity, not a Program ID or a caller-selected ordinal.
type MountedArtifactReceipt struct {
	receipt *ArtifactScalarReceipt
	mountID identity.ContentID
}

// NewMountedArtifactReceipt binds one reusable Program artifact to one exact
// Link mount identity.  The fields stay opaque so only the parent assembly
// can preserve mount ordering and feed the lowerer.
func NewMountedArtifactReceipt(receipt *ArtifactScalarReceipt, mountID identity.ContentID) (MountedArtifactReceipt, bool) {
	row := MountedArtifactReceipt{receipt: receipt, mountID: mountID}
	return row, receipt != nil && receipt.sealed && receipt.template.Available() && mountID.Available()
}

type artifactMountReceipt struct {
	mount    identity.ContentID
	artifact identity.ContentID
	program  identity.ContentID
	initial  identity.ContentID
}

type artifactPointMetadata struct {
	mount     identity.ContentID
	artifact  identity.ContentID
	reusable  identity.ContentID
	decisions []identity.ContentID
	initial   bool
}

type artifactEnvironmentRow struct {
	mount    identity.ContentID
	artifact identity.ContentID
	reusable identity.ContentID
	id       identity.ContentID
	from     identity.ContentID
	to       identity.ContentID
	route    identity.ContentID
	guard    identity.ContentID
	decision identity.ContentID
	guarded  bool
	truth    bool
	// component, mu, arm, and reset are parent proof metadata. Equation has
	// no corresponding runtime fields; lowerArtifactRows validates them before
	// admitting the boundary and retains them in the immutable receipt.
	component     identity.ContentID
	mu            identity.ContentID
	reset         identity.ContentID
	hasReset      bool
	resets        []identity.ContentID
	arm           rows.ArtifactStructuralArm
	transportOnly bool
	local         bool
	full          bool
	factorRoles   []RuleSlotCapability
}

type artifactWTORegionRow struct {
	mount    identity.ContentID
	artifact identity.ContentID
	reusable identity.ContentID
	id       identity.ContentID
	head     identity.ContentID
	parent   identity.ContentID
	cyclic   bool
	members  []identity.ContentID
}

type artifactWTOEventRow struct {
	mount    identity.ContentID
	artifact identity.ContentID
	kind     rows.ArtifactEventKind
	region   identity.ContentID
	point    identity.ContentID
}

// ArtifactPointReceipt is a BindingTopology-issued view of one exact Program
// artifact point.  It is not an equation coordinate and cannot be minted from
// a ContentID by callers.
type ArtifactPointReceipt struct {
	topology *BindingTopology
	index    uint32
}

// MountedNativeCallStageReceipt is a cold, graph-owned proof that one exact
// mounted Call occurrence was attached at its ProgramArtifact-issued native
// stage. It is issued by occurrence alone: callers never submit a reusable
// point to this lookup and therefore cannot splice another artifact point.
type MountedNativeCallStageReceipt struct {
	graph *ReceiptGraph
	key   artifactMountedRuleOccurrence
	stage artifactNativeCallStage
}

func (receipt MountedNativeCallStageReceipt) row() (artifactNativeCallStage, bool) {
	if receipt.graph == nil || !receipt.graph.valid() || receipt.graph.topology == nil || receipt.graph.topology.nativeCallStages == nil {
		return artifactNativeCallStage{}, false
	}
	row, ok := receipt.graph.topology.nativeCallStages[receipt.key]
	return row, ok && row == receipt.stage && row.stage.NativeCall() && row.point.Available() && row.input.Available() && row.mountedPoint.Available() && row.mountedInput.Available()
}

func (receipt MountedNativeCallStageReceipt) Available() bool { _, ok := receipt.row(); return ok }
func (receipt MountedNativeCallStageReceipt) Stage() rows.ArtifactRuleStage {
	row, ok := receipt.row()
	if !ok {
		return rows.ArtifactRuleStageInvalid
	}
	return row.stage
}
func (receipt MountedNativeCallStageReceipt) MountID() identity.ContentID {
	if _, ok := receipt.row(); !ok {
		return identity.ContentID{}
	}
	return receipt.key.mount
}
func (receipt MountedNativeCallStageReceipt) OccurrenceID() identity.ContentID {
	if _, ok := receipt.row(); !ok {
		return identity.ContentID{}
	}
	return receipt.key.occurrence
}
func (receipt MountedNativeCallStageReceipt) ReusablePointID() identity.ContentID {
	row, ok := receipt.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.point
}
func (receipt MountedNativeCallStageReceipt) ReusableInputPointID() identity.ContentID {
	row, ok := receipt.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.input
}

// RuleMember resolves the already-attached member authenticated by this
// stage proof. The caller cannot substitute another point or occurrence.
func (receipt MountedNativeCallStageReceipt) RuleMember() (ReceiptRuleMember, bool) {
	row, ok := receipt.row()
	if !ok {
		return ReceiptRuleMember{}, false
	}
	return receipt.graph.lookupRuleMember(mountedRuleMemberID(receipt.key.role, receipt.key.mount, row.point, receipt.key.occurrence))
}

// MountedNativeCallStage resolves the exact native stage by owner capability,
// mount, and occurrence. Point identity is output-only proof material.
func (receipt *ReceiptGraph) MountedNativeCallStage(role RuleSlotCapability, mount, occurrence identity.ContentID) (MountedNativeCallStageReceipt, bool) {
	if receipt == nil || !receipt.valid() || !role.mounted() || role.state != receipt.state || role.authority != receipt.authority || !mount.Available() || !occurrence.Available() || receipt.topology.nativeCallStages == nil {
		return MountedNativeCallStageReceipt{}, false
	}
	key := artifactMountedRuleOccurrence{role: role, mount: mount, occurrence: occurrence}
	stage, ok := receipt.topology.nativeCallStages[key]
	result := MountedNativeCallStageReceipt{graph: receipt, key: key, stage: stage}
	return result, ok && result.Available()
}

func (point ArtifactPointReceipt) Available() bool {
	return point.topology != nil && point.topology.valid() && point.topology.artifact != nil && uint64(point.index) < uint64(len(point.topology.artifact.points))
}

// ArtifactEnvironmentReceipt is one exact parent structural edge retained by
// BindingTopology.  It exposes scalar artifact semantics, never the private
// equation Input or PointRef used to lower it.
type ArtifactEnvironmentReceipt struct {
	topology *BindingTopology
	index    uint32
}

func (edge ArtifactEnvironmentReceipt) row() (artifactEnvironmentRow, bool) {
	if edge.topology == nil || !edge.topology.valid() || edge.topology.artifact == nil || uint64(edge.index) >= uint64(len(edge.topology.artifact.edges)) {
		return artifactEnvironmentRow{}, false
	}
	return edge.topology.artifact.edges[edge.index], true
}
func (edge ArtifactEnvironmentReceipt) Available() bool { _, ok := edge.row(); return ok }

// ArtifactWTOEventReceipt is one exact parent-issued WTO bracket/point row.
// Region and Point are a closed sum: callers cannot manufacture a mixed row.
type ArtifactWTOEventReceipt struct {
	topology *BindingTopology
	index    uint32
}

func (event ArtifactWTOEventReceipt) row() (artifactWTOEventRow, bool) {
	if event.topology == nil || !event.topology.valid() || event.topology.artifact == nil || uint64(event.index) >= uint64(len(event.topology.artifact.events)) {
		return artifactWTOEventRow{}, false
	}
	return event.topology.artifact.events[event.index], true
}
func (event ArtifactWTOEventReceipt) Available() bool { _, ok := event.row(); return ok }
func (event ArtifactWTOEventReceipt) Kind() rows.ArtifactEventKind {
	row, ok := event.row()
	if !ok {
		return rows.ArtifactEventInvalid
	}
	return row.kind
}

// ArtifactWTORegionReceipt is one exact parent-issued WTO region. Member
// order is preserved verbatim from the artifact and remains separate from
// Equation's private point coordinates.
type ArtifactWTORegionReceipt struct {
	topology *BindingTopology
	index    uint32
}

func (region ArtifactWTORegionReceipt) row() (artifactWTORegionRow, bool) {
	if region.topology == nil || !region.topology.valid() || region.topology.artifact == nil || uint64(region.index) >= uint64(len(region.topology.artifact.regions)) {
		return artifactWTORegionRow{}, false
	}
	return region.topology.artifact.regions[region.index], true
}
func (region ArtifactWTORegionReceipt) Available() bool { _, ok := region.row(); return ok }

// AssembleMountedArtifactReceipt owns the whole receipt assembly transaction.
// It lowers the mounts, hands the open assembly to populate, and terminalizes
// on populate's answer: true commits, false aborts. The assembly never escapes
// this scope, so begin can no longer be paired with a hand-placed abort and
// every rejected path releases the binding's one topology builder. Exactly one
// failure is available on a rejection: lowering when the mounts did not lower,
// commit when the populated assembly did not commit, and neither when populate
// itself rejected under its own vocabulary.
func AssembleMountedArtifactReceipt(binding *SchemaBinding, mounts []MountedArtifactReceipt, populate func(*ReceiptAssembly) bool, bootstrap ...LinkBootstrapWitness) (*BindingTopology, *ReceiptGraph, ReceiptAssemblyFailure, ReceiptCommitFailure, bool) {
	if populate == nil {
		return nil, nil, ReceiptAssemblyFailureInput, ReceiptCommitFailure{}, false
	}
	assembly, lowering, assembled := BeginMountedArtifactReceiptAssemblyWithFailure(binding, mounts, bootstrap...)
	if !assembled {
		return nil, nil, lowering, ReceiptCommitFailure{}, false
	}
	if !populate(assembly) {
		assembly.Abort()
		return nil, nil, ReceiptAssemblyFailureNone, ReceiptCommitFailure{}, false
	}
	topology, graph, committed := assembly.Commit()
	if !committed || topology == nil || graph == nil {
		failure, _ := assembly.CommitFailure()
		return nil, nil, ReceiptAssemblyFailureNone, failure, false
	}
	return topology, graph, ReceiptAssemblyFailureNone, ReceiptCommitFailure{}, true
}

// BeginMountedArtifactReceiptAssembly is the sole Link-cardinality-aware
// ProgramArtifact structural lowerer. It consumes every ordered mount into
// one Binding-owned Batch and Topology; duplicate reusable Program artifacts
// remain distinct because every lowered identity is mount-qualified.
func BeginMountedArtifactReceiptAssembly(binding *SchemaBinding, mounts []MountedArtifactReceipt, bootstrap ...LinkBootstrapWitness) (*ReceiptAssembly, bool) {
	assembly, _, ok := BeginMountedArtifactReceiptAssemblyWithFailure(binding, mounts, bootstrap...)
	return assembly, ok
}

type ReceiptAssemblyFailure uint8

const (
	ReceiptAssemblyFailureNone ReceiptAssemblyFailure = iota
	ReceiptAssemblyFailureInput
	ReceiptAssemblyFailureSchema
	ReceiptAssemblyFailureSnapshot
	ReceiptAssemblyFailureTransaction
	ReceiptAssemblyFailureStructuralRows
	// Snapshot sub-stages preserve the owner boundary that rejected an
	// otherwise immutable artifact. They are diagnostics only: no partial
	// receipt escapes the assembly.
	ReceiptAssemblyFailureSnapshotBootstrap
	ReceiptAssemblyFailureSnapshotMount
	ReceiptAssemblyFailureSnapshotArtifact
	ReceiptAssemblyFailureSnapshotNamespace
	ReceiptAssemblyFailureSnapshotTopology
	ReceiptAssemblyFailureSnapshotTopologyMount
	ReceiptAssemblyFailureSnapshotTopologyPoint
	ReceiptAssemblyFailureSnapshotTopologyRegion
	ReceiptAssemblyFailureSnapshotTopologyEdge
	ReceiptAssemblyFailureSnapshotTopologyEvent
	ReceiptAssemblyFailureSnapshotTopologyBootstrap
	ReceiptAssemblyFailureSnapshotTopologySchedule
	ReceiptAssemblyFailureSnapshotTopologyRule
)

// Failure projects one lowering boundary onto the engine's public failure
// vocabulary. The ordinal enters the site preimage and never leaves this
// package.
func (failure ReceiptAssemblyFailure) Failure() SolveFailure {
	if failure == ReceiptAssemblyFailureNone {
		return SolveFailure{}
	}
	return receiptFailure(SolveFailureFamilyCompile, "receipt-assembly", uint64(failure))
}

// BeginMountedArtifactReceiptAssemblyWithFailure exposes only the first
// closed assembly phase. It is the permanent diagnostic entrypoint used by
// production; it never returns a partial snapshot or mutable row.
func BeginMountedArtifactReceiptAssemblyWithFailure(binding *SchemaBinding, mounts []MountedArtifactReceipt, bootstrap ...LinkBootstrapWitness) (*ReceiptAssembly, ReceiptAssemblyFailure, bool) {
	if binding == nil || !binding.Sealed() || len(mounts) == 0 || len(bootstrap) != 1 || !bootstrap[0].Available() {
		return nil, ReceiptAssemblyFailureInput, false
	}
	schema := binding.Schema()
	if schema == nil || !schema.Available() {
		return nil, ReceiptAssemblyFailureSchema, false
	}
	rows, snapshotFailure := snapshotMountedArtifactReceipts(mounts, identity.ContentID(schema.ID().Digest()), bootstrap[0], binding.state)
	if snapshotFailure != ReceiptAssemblyFailureNone {
		return nil, snapshotFailure, false
	}
	assembly, ok := beginReceiptAssembly(binding)
	if !ok {
		return nil, ReceiptAssemblyFailureTransaction, false
	}
	if !lowerArtifactReceipt(assembly, rows) {
		assembly.Abort()
		return nil, ReceiptAssemblyFailureStructuralRows, false
	}
	return assembly, ReceiptAssemblyFailureNone, true
}

// artifactSourceDomain pairs one cold source namespace with its semantic
// version. Pairing them keeps a caller from separating two namespaces by the
// version field alone.
type artifactSourceDomain struct {
	name    string
	version uint64
}

var (
	artifactPointSource      = artifactSourceDomain{name: "analysis/engine/artifact-point-source", version: artifactPointSourceVersion}
	artifactEdgeSource       = artifactSourceDomain{name: "analysis/engine/artifact-edge-source", version: artifactEdgeSourceVersion}
	artifactOccurrenceSource = artifactSourceDomain{name: "analysis/engine/artifact-occurrence-source", version: artifactOccurrenceSourceVersion}
)

// artifactReceiptKey derives one cold source key from an artifact content
// identity. The namespace enters the preimage, so a point, an edge, and an
// occurrence built from the same content identity name three keys with three
// distinct digests rather than one digest under three version labels.
func artifactReceiptKey(domain artifactSourceDomain, id identity.ContentID) (composition.Key, bool) {
	if !id.Available() {
		return composition.Key{}, false
	}
	return framedCompositionKey(domain.name, domain.version, func(writer *canonical.DigestWriter) bool {
		return writer.Bytes(id[:]) == nil
	})
}

func lowerArtifactReceipt(assembly *ReceiptAssembly, rows *artifactReceiptTopology) bool {
	if assembly == nil || assembly.builder == nil || rows == nil {
		return false
	}
	sites := make(map[identity.ContentID]equation.Site, len(rows.points))
	for _, id := range rows.points {
		source, sourceOK := artifactReceiptKey(artifactPointSource, id)
		metadata, metadataOK := rows.pointMeta[id]
		if !metadataOK || !metadata.reusable.Available() {
			return false
		}
		decisions := make([]equation.Decision, len(metadata.decisions))
		for index, semanticID := range metadata.decisions {
			decisionKey := mountedArtifactID("analysis/engine/artifact-decision/v1", metadata.mount, metadata.artifact, semanticID)
			decision, decisionOK := equation.NewDecision(mustArtifactReceiptKey(artifactPointSource, decisionKey))
			if !decisionOK {
				return false
			}
			decisions[index] = decision
		}
		scope, scopeOK := equation.NewScope(decisions...)
		if !scopeOK {
			return false
		}
		init := equation.FalseExpr()
		disposition := equation.InitAbsent
		if metadata.initial {
			init = equation.TrueExpr()
			disposition = equation.InitPresent
		}
		site, siteOK := assembly.builder.admitSite(source, scope, init, disposition)
		if !sourceOK || !siteOK {
			return false
		}
		sites[id] = site
	}
	if rows.bootstrap != nil {
		point := rows.bootstrap.point
		source, sourceOK := artifactReceiptKey(artifactPointSource, point.PointID)
		decisions := make([]equation.Decision, len(point.DecisionID))
		for index, semanticID := range point.DecisionID {
			decisionKey := mountedArtifactID("analysis/engine/link-bootstrap-decision/v1", rows.bootstrap.owner, rows.bootstrap.owner, semanticID)
			decision, decisionOK := equation.NewDecision(mustArtifactReceiptKey(artifactPointSource, decisionKey))
			if !decisionOK {
				return false
			}
			decisions[index] = decision
		}
		scope, scopeOK := equation.NewScope(decisions...)
		init := equation.FalseExpr()
		disposition := equation.InitAbsent
		if point.Initial {
			init = equation.TrueExpr()
			disposition = equation.InitPresent
		}
		if !sourceOK || !scopeOK {
			return false
		}
		site, siteOK := assembly.builder.admitSite(source, scope, init, disposition)
		if !siteOK {
			return false
		}
		rows.bootstrap.site = site
		sites[point.PointID] = site
	}
	rows.sites = sites
	rows.mounted = make(map[artifactMountedPoint]equation.Site, len(rows.pointMeta))
	for id, metadata := range rows.pointMeta {
		site, siteOK := sites[id]
		if !siteOK {
			return false
		}
		key := artifactMountedPoint{mount: metadata.mount, reusable: metadata.reusable}
		if _, duplicate := rows.mounted[key]; duplicate {
			return false
		}
		rows.mounted[key] = site
	}
	inner, locked := assembly.builder.lockSourcesOpen()
	if !locked || inner.artifact != nil {
		if locked {
			inner.failLocked()
			inner.mu.Unlock()
		}
		return false
	}
	inner.artifact = rows
	inner.mu.Unlock()
	return true
}

// mustArtifactReceiptKey returns the unavailable key for an unnameable source
// identity. Every consumer admits the key through a constructor that rejects
// it, so the failure is carried rather than hidden.
func mustArtifactReceiptKey(domain artifactSourceDomain, id identity.ContentID) composition.Key {
	key, ok := artifactReceiptKey(domain, id)
	if !ok {
		return composition.Key{}
	}
	return key
}

// lowerArtifactRows completes the artifact's point/edge rows after all
// typed operand rows have been admitted to the open source Batch.  Keeping
// this commit after SealSources preserves one canonical Batch key while
// allowing the analysis dispatcher to issue every exact mounted operand.
type ReceiptArtifactRowFailure uint8

const (
	ReceiptArtifactRowFailureNone ReceiptArtifactRowFailure = iota
	ReceiptArtifactRowFailureOwner
	ReceiptArtifactRowFailurePoint
	ReceiptArtifactRowFailureBootstrap
	ReceiptArtifactRowFailureEdgeMetadata
	ReceiptArtifactRowFailureEdgeProof
	ReceiptArtifactRowFailureEdgeReset
	ReceiptArtifactRowFailureEdgeDecision
	ReceiptArtifactRowFailureEdgeReindex
	ReceiptArtifactRowFailureEdgeGuard
	ReceiptArtifactRowFailureEdgeReceipt
	ReceiptArtifactRowFailureEvent
	ReceiptArtifactRowFailureSchedule
)

func (builder *bindingTopologyBuilder) lowerArtifactRows() (ReceiptArtifactRowFailure, uint32, bool) {
	if builder == nil {
		return ReceiptArtifactRowFailureOwner, 0, false
	}
	inner, locked := builder.lockTopologyOpen()
	if !locked {
		return ReceiptArtifactRowFailureOwner, 0, false
	}
	artifactRows := inner.artifact
	inner.mu.Unlock()
	if artifactRows == nil {
		return ReceiptArtifactRowFailureNone, 0, true
	}
	sites := artifactRows.sites
	refs := make(map[identity.ContentID]equation.PointRef, len(artifactRows.points))
	pointDecisions := make(map[identity.ContentID]map[identity.ContentID]equation.Decision, len(artifactRows.points))
	for pointIndex, id := range artifactRows.points {
		site, siteOK := sites[id]
		row, issued := builder.issuePointRow(equation.PointSpec{Site: site})
		ref, added := builder.addSemanticPoint(id, row)
		if !siteOK || !issued || !added {
			return ReceiptArtifactRowFailurePoint, uint32(pointIndex), false
		}
		refs[id] = ref.ref
		metadata, metadataOK := artifactRows.pointMeta[id]
		if !metadataOK {
			return ReceiptArtifactRowFailurePoint, uint32(pointIndex), false
		}
		decisions := make(map[identity.ContentID]equation.Decision, len(metadata.decisions))
		for _, semanticID := range metadata.decisions {
			decisionKey := mountedArtifactID("analysis/engine/artifact-decision/v1", metadata.mount, metadata.artifact, semanticID)
			decision, decisionOK := equation.NewDecision(mustArtifactReceiptKey(artifactPointSource, decisionKey))
			if !decisionOK {
				return ReceiptArtifactRowFailurePoint, uint32(pointIndex), false
			}
			decisions[semanticID] = decision
		}
		pointDecisions[id] = decisions
	}
	if artifactRows.bootstrap != nil {
		point := artifactRows.bootstrap.point.PointID
		row, issued := builder.issuePointRow(equation.PointSpec{Site: artifactRows.sites[point]})
		semanticID := linkBootstrapPointSemanticID(artifactRows.bootstrap.owner, point)
		ref, added := builder.addSemanticPoint(semanticID, row)
		if !issued || !added || !semanticID.Available() {
			return ReceiptArtifactRowFailureBootstrap, 0, false
		}
		artifactRows.bootstrap.semantic = semanticID
		artifactRows.bootstrap.ref = ref.ref
	}
	for edgeIndex, edge := range artifactRows.edges {
		from, fromOK := sites[edge.from]
		to, toOK := sites[edge.to]
		target, targetOK := refs[edge.to]
		provenance, provenanceOK := artifactReceiptKey(artifactEdgeSource, edge.id)
		fromMetadata, fromMetadataOK := artifactRows.pointMeta[edge.from]
		_, toMetadataOK := artifactRows.pointMeta[edge.to]
		fromDecisions, fromDecisionsOK := pointDecisions[edge.from]
		toDecisions, toDecisionsOK := pointDecisions[edge.to]
		if !fromMetadataOK || !toMetadataOK || !fromDecisionsOK || !toDecisionsOK {
			return ReceiptArtifactRowFailureEdgeMetadata, uint32(edgeIndex), false
		}
		maps := make([]equation.DecisionMap, len(fromMetadata.decisions))
		resetSet := make(map[identity.ContentID]struct{}, len(edge.resets))
		for _, resetID := range edge.resets {
			// A recurrence reset is conditional on the decision being live at
			// this source Point. Parent reset receipts may contain decisions
			// outside the current lexical scope; clearing absent information is
			// an exact no-op, not malformed evidence.
			resetSet[resetID] = struct{}{}
		}
		for index, semanticID := range fromMetadata.decisions {
			decision, decisionOK := fromDecisions[semanticID]
			if !decisionOK {
				return ReceiptArtifactRowFailureEdgeDecision, uint32(edgeIndex), false
			}
			if _, forgotten := resetSet[semanticID]; forgotten {
				maps[index] = equation.Forget(decision)
			} else {
				targetDecision, retained := toDecisions[semanticID]
				if !retained {
					// Leaving a decision scope is an ordinary exact projection,
					// independent of recurrence reset. A reset is parent proof
					// that a still-scoped decision must be forgotten; absence
					// from the target Point is already the parent-owned proof that
					// this route leaves that decision's lexical scope.
					maps[index] = equation.Forget(decision)
					continue
				}
				maps[index] = equation.Identity(decision)
				if targetDecision != decision {
					maps[index] = equation.Rename(decision, targetDecision)
				}
			}
		}
		sourceScope := from.Scope()
		targetScope := to.Scope()
		omega, omegaOK := equation.NewReindex(sourceScope, targetScope, maps)
		if !omegaOK {
			return ReceiptArtifactRowFailureEdgeReindex, uint32(edgeIndex), false
		}
		pre := equation.TrueExpr()
		if edge.guarded {
			decision, decisionOK := fromDecisions[edge.decision]
			if !decisionOK {
				return ReceiptArtifactRowFailureEdgeGuard, uint32(edgeIndex), false
			}
			pre, decisionOK = equation.DecisionExpr(decision)
			if !decisionOK {
				return ReceiptArtifactRowFailureEdgeGuard, uint32(edgeIndex), false
			}
			if !edge.truth {
				pre, decisionOK = equation.NotExpr(pre)
				if !decisionOK {
					return ReceiptArtifactRowFailureEdgeGuard, uint32(edgeIndex), false
				}
			}
		}
		input := equation.BoundaryInput(from, to, provenance, pre, omega, equation.TrueExpr())
		if !fromOK || !toOK || !targetOK || !provenanceOK || !input.Available() {
			return ReceiptArtifactRowFailureEdgeReceipt, uint32(edgeIndex), false
		}
		if edge.local && !edge.full {
			seenFactors := make(map[composition.Key]struct{}, len(edge.factorRoles))
			for _, role := range edge.factorRoles {
				factor, semantic, factorOK := artifactTransportFactor(builder.inner, role)
				if _, duplicate := seenFactors[semantic]; duplicate {
					factorOK = false
				}
				if !factorOK {
					return ReceiptArtifactRowFailureEdgeReceipt, uint32(edgeIndex), false
				}
				receipt, issued := builder.issueFactorEdge(factor, equation.FactorEdge{Target: target, Input: input, Factor: semantic})
				if !issued || !builder.addFactorEdge(receipt) {
					return ReceiptArtifactRowFailureEdgeReceipt, uint32(edgeIndex), false
				}
				seenFactors[semantic] = struct{}{}
			}
			continue
		}
		receipt, issued := builder.issueEnvironmentEdge(equation.EnvironmentEdge{Target: target, Input: input, TransportOnly: edge.transportOnly})
		if !issued || !builder.addEnvironmentEdge(receipt) {
			return ReceiptArtifactRowFailureEdgeReceipt, uint32(edgeIndex), false
		}
	}
	if artifactRows.bootstrap != nil {
		for transportIndex, transport := range artifactRows.bootstrap.transports {
			factor, semantic, factorOK := bootstrapTransportFactor(builder.inner, transport.capability)
			if !factorOK || semantic != transport.factor {
				return ReceiptArtifactRowFailureBootstrap, uint32(transportIndex), false
			}
			for _, mount := range artifactRows.mounts {
				targetID := mount.initial
				metadata, metadataOK := artifactRows.pointMeta[targetID]
				if !metadataOK || !metadata.initial || metadata.mount != mount.mount || metadata.artifact != mount.artifact {
					return ReceiptArtifactRowFailureBootstrap, uint32(transportIndex), false
				}
				targetSite, siteOK := artifactRows.sites[targetID]
				target, targetOK := refs[targetID]
				provenance, provenanceOK := linkBootstrapTransportKey(artifactRows.bootstrap.owner, metadata, semantic)
				reindex, reindexOK := ruleInputReindex(artifactRows.bootstrap.site.Scope(), targetSite.Scope())
				input := equation.BoundaryInput(artifactRows.bootstrap.site, targetSite, provenance, equation.TrueExpr(), reindex, equation.TrueExpr())
				if !siteOK || !targetOK || !provenanceOK || !reindexOK || !input.Available() {
					return ReceiptArtifactRowFailureBootstrap, uint32(transportIndex), false
				}
				receipt, issued := builder.issueFactorEdge(factor, equation.FactorEdge{Target: target, Input: input, Factor: semantic})
				if !issued || !builder.addFactorEdge(receipt) {
					return ReceiptArtifactRowFailureBootstrap, uint32(transportIndex), false
				}
			}
		}
	}
	// The artifact WTO stream is the parent-issued semantic order.  Convert
	// its point events to dense ranks aligned with the equation PointSpec
	// artifactRows; point IDs are mounted and therefore cannot be used as tie-breaks.
	eventRank := make(map[identity.ContentID]int, len(artifactRows.points))
	for eventIndex, event := range artifactRows.events {
		if event.kind != rows.ArtifactEventPoint {
			continue
		}
		if !event.point.Available() {
			return ReceiptArtifactRowFailureEvent, uint32(eventIndex), false
		}
		if _, duplicate := eventRank[event.point]; duplicate {
			return ReceiptArtifactRowFailureEvent, uint32(eventIndex), false
		}
		eventRank[event.point] = len(eventRank)
	}
	if len(eventRank) != len(artifactRows.points) {
		return ReceiptArtifactRowFailureEvent, uint32(len(artifactRows.events)), false
	}
	artifactRows.pointRef = refs
	artifactRows.mountedRef = make(map[artifactMountedPoint]equation.PointRef, len(artifactRows.pointMeta))
	for id, metadata := range artifactRows.pointMeta {
		ref, refOK := refs[id]
		if !refOK {
			return ReceiptArtifactRowFailurePoint, 0, false
		}
		artifactRows.mountedRef[artifactMountedPoint{mount: metadata.mount, reusable: metadata.reusable}] = ref
	}
	inner, locked = builder.lockTopologyOpen()
	if !locked {
		return ReceiptArtifactRowFailureSchedule, 0, false
	}
	pointRanks := make([]int, len(inner.spec.Points))
	bootstrapOffset := 0
	if artifactRows.bootstrap != nil {
		bootstrapOffset = 1
	}
	if len(pointRanks) != len(artifactRows.points)+bootstrapOffset {
		inner.failLocked()
		inner.mu.Unlock()
		return ReceiptArtifactRowFailureSchedule, 0, false
	}
	for index, point := range artifactRows.points {
		rank, rankOK := eventRank[point]
		if !rankOK {
			inner.failLocked()
			inner.mu.Unlock()
			return ReceiptArtifactRowFailureSchedule, uint32(index), false
		}
		pointRanks[index] = rank + bootstrapOffset
	}
	if artifactRows.bootstrap != nil {
		// Link bootstrap is intentionally the deterministic rank-zero anchor;
		// all mounted artifact points retain their local WTO order after it.
		pointRanks[len(artifactRows.points)] = 0
	}
	inner.spec.PointRanks = pointRanks
	inner.mu.Unlock()
	return ReceiptArtifactRowFailureNone, 0, true
}

func validArtifactRouteProof(edge artifactEnvironmentRow) bool {
	if edge.local {
		if !edge.id.Available() || !edge.from.Available() || !edge.to.Available() || edge.from == edge.to || edge.full == (len(edge.factorRoles) != 0) || edge.transportOnly != edge.full ||
			edge.route.Available() || edge.guard.Available() || edge.decision.Available() || edge.guarded || edge.truth ||
			edge.component.Available() || edge.mu.Available() || edge.reset.Available() || edge.hasReset || len(edge.resets) != 0 || edge.arm != rows.ArtifactStructuralArmInvalid {
			return false
		}
		seen := make(map[RuleSlotCapability]struct{}, len(edge.factorRoles))
		for _, role := range edge.factorRoles {
			if !role.mounted() {
				return false
			}
			if _, duplicate := seen[role]; duplicate {
				return false
			}
			seen[role] = struct{}{}
		}
		return true
	}
	if edge.transportOnly != (edge.from == edge.to && !edge.component.Available() && !edge.mu.Available() && !edge.hasReset) {
		return false
	}
	// transportOnly is the only lowering-derived field on a structural edge;
	// the rest of the route proof is the scalar property the artifact owner
	// already sealed, so it is proved once, there.
	return rows.ValidArtifactScalarEdgeProof(rows.ArtifactScalarEdge{
		ID: edge.id, From: edge.from, To: edge.to, Route: edge.route,
		Guard: edge.guard, Decision: edge.decision,
		Component: edge.component, Mu: edge.mu, Reset: edge.reset,
		Resets: edge.resets, Arm: edge.arm,
		Guarded: edge.guarded, Truth: edge.truth, HasReset: edge.hasReset,
	})
}

func artifactTransportFactor(inner *bindingTopologyBuilderState, role RuleSlotCapability) (bindingFactorReceipt, composition.Key, bool) {
	if !role.mounted() {
		return nil, composition.Key{}, false
	}
	return transportFactor(inner, role)
}

func bootstrapTransportFactor(inner *bindingTopologyBuilderState, role RuleSlotCapability) (bindingFactorReceipt, composition.Key, bool) {
	if !role.link() {
		return nil, composition.Key{}, false
	}
	return transportFactor(inner, role)
}

func transportFactor(inner *bindingTopologyBuilderState, role RuleSlotCapability) (bindingFactorReceipt, composition.Key, bool) {
	if inner == nil || inner.state == nil || inner.state.schema == nil || !role.available() || role.state != inner.state || role.authority != inner.authority {
		return nil, composition.Key{}, false
	}
	rule, roleOK := inner.state.roleSlots[role]
	ruleOrdinal, ruleOK := inner.state.schema.ruleOrdinalOf(rule)
	shape, shapeOK := inner.state.schema.ruleShapeAt(ruleOrdinal)
	factorOrdinal, factorOK := inner.state.schema.factorOrdinalOf(shape.Output)
	if !roleOK || !ruleOK || !shapeOK || shape.OutputKind != composition.FactorOutput || !factorOK || factorOrdinal >= uint64(len(inner.factors)) {
		return nil, composition.Key{}, false
	}
	factor := inner.factors[factorOrdinal]
	if factor == nil {
		return nil, composition.Key{}, false
	}
	return factor, shape.Output, true
}

func linkBootstrapTransportKey(owner identity.ContentID, target artifactPointMetadata, factor composition.Key) (composition.Key, bool) {
	if !owner.Available() || !target.mount.Available() || !target.artifact.Available() || !target.reusable.Available() || !factor.Available() {
		return composition.Key{}, false
	}
	id := framedContentID(linkBootstrapTransportDomain, artifactContentIdentityVersion, func(writer *canonical.DigestWriter) bool {
		return writeContentIDs(writer, owner, target.mount, target.artifact, target.reusable) &&
			writer.Bytes(factor.ID[:]) == nil && writer.Uint(factor.Version) == nil
	})
	if !id.Available() {
		return composition.Key{}, false
	}
	return artifactReceiptKey(artifactEdgeSource, id)
}

// mountedArtifactID derives one mount-qualified artifact identity. Each part
// is length-framed under the caller's domain, so two different mount/artifact
// splits of the same bytes cannot reach the same identity.
func mountedArtifactID(domain string, mount, artifact, id identity.ContentID) identity.ContentID {
	if !mount.Available() || !artifact.Available() || !id.Available() {
		return identity.ContentID{}
	}
	return framedContentID(domain, artifactContentIdentityVersion, func(writer *canonical.DigestWriter) bool {
		return writeContentIDs(writer, mount, artifact, id)
	})
}

func linkBootstrapPointSemanticID(owner, point identity.ContentID) identity.ContentID {
	return mountedArtifactID("analysis/engine/link-bootstrap-point/v1", owner, owner, point)
}

func snapshotMountedArtifactReceipts(mounts []MountedArtifactReceipt, schemaID identity.ContentID, bootstrap LinkBootstrapWitness, bindingState *schemaBindingState) (*artifactReceiptTopology, ReceiptAssemblyFailure) {
	if len(mounts) == 0 || !schemaID.Available() || !bootstrap.Available() || bindingState == nil {
		return nil, ReceiptAssemblyFailureSnapshotBootstrap
	}
	bootstrapPoint, pointOK := bootstrap.Point()
	if !pointOK {
		return nil, ReceiptAssemblyFailureSnapshotBootstrap
	}
	occurrences := make(map[identity.ContentID]struct{}, bootstrap.OccurrenceCount())
	for index := 0; index < bootstrap.OccurrenceCount(); index++ {
		id, idOK := bootstrap.OccurrenceAt(index)
		if !idOK {
			return nil, ReceiptAssemblyFailureSnapshotBootstrap
		}
		occurrences[id] = struct{}{}
	}
	roles := make(map[identity.ContentID]RuleSlotCapability, len(occurrences))
	for id := range occurrences {
		capability, capabilityOK := bootstrap.capabilityFor(id)
		if !capabilityOK || !capability.link() || capability.state != bindingState || capability.authority != bindingState.authority {
			return nil, ReceiptAssemblyFailureSnapshotBootstrap
		}
		roles[id] = capability
	}
	transports := make([]linkBootstrapTransport, bootstrap.transportCapabilityCount())
	seenTransportCapabilities := make(map[RuleSlotCapability]struct{}, len(transports))
	seenTransportFactors := make(map[composition.Key]struct{}, len(transports))
	if len(transports) != 0 && len(transports) != 2 {
		return nil, ReceiptAssemblyFailureSnapshotBootstrap
	}
	authorizedTransports, transportsAuthorized := sealedLinkBootstrapTransportPair(bindingState)
	if (len(transports) == 0 && transportsAuthorized) || (len(transports) != 0 && !transportsAuthorized) {
		return nil, ReceiptAssemblyFailureSnapshotBootstrap
	}
	for index := range transports {
		capability, capabilityOK := bootstrap.transportCapabilityAt(index)
		factor, factorOK := linkTransportFactorSemantic(bindingState, capability)
		if !capabilityOK || !factorOK || capability != authorizedTransports[index] {
			return nil, ReceiptAssemblyFailureSnapshotBootstrap
		}
		if _, duplicate := seenTransportCapabilities[capability]; duplicate {
			return nil, ReceiptAssemblyFailureSnapshotBootstrap
		}
		if _, duplicate := seenTransportFactors[factor]; duplicate {
			return nil, ReceiptAssemblyFailureSnapshotBootstrap
		}
		seenTransportCapabilities[capability] = struct{}{}
		seenTransportFactors[factor] = struct{}{}
		transports[index] = linkBootstrapTransport{capability: capability, factor: factor}
	}
	result := &artifactReceiptTopology{pointMeta: make(map[identity.ContentID]artifactPointMetadata), sites: make(map[identity.ContentID]equation.Site), mounted: make(map[artifactMountedPoint]equation.Site), mountedRef: make(map[artifactMountedPoint]equation.PointRef), bodies: make(map[artifactMountedBody]artifactBodyTransport), ruleSet: make(map[artifactMountedRule]artifactRuleInput), callStages: make(map[artifactMountedRuleOccurrence]artifactNativeCallStage), pointRef: make(map[identity.ContentID]equation.PointRef), bootstrap: &linkBootstrapReceipt{owner: bootstrap.OwnerID(), point: bootstrapPoint, occurrences: occurrences, roles: roles, claims: make(map[identity.ContentID]RuleSlotCapability), transports: transports}}
	seenMounts := make(map[identity.ContentID]struct{}, len(mounts))
	for _, mount := range mounts {
		if mount.receipt == nil || !mount.receipt.sealed || !mount.receipt.template.Available() || !mount.mountID.Available() || mount.receipt.template.SchemaID() != schemaID {
			return nil, ReceiptAssemblyFailureSnapshotMount
		}
		template := mount.receipt.template
		initialCount := 0
		for index := 0; index < template.PointCount(); index++ {
			point, pointOK := template.PointAt(index)
			if !pointOK {
				return nil, ReceiptAssemblyFailureSnapshotTopologyPoint
			}
			if point.Initial {
				initialCount++
			}
		}
		if initialCount != 1 {
			return nil, ReceiptAssemblyFailureSnapshotTopologyPoint
		}
		if _, duplicate := seenMounts[mount.mountID]; duplicate {
			return nil, ReceiptAssemblyFailureSnapshotMount
		}
		seenMounts[mount.mountID] = struct{}{}
		for index := 0; index < template.RuleCount(); index++ {
			rule, ruleOK := template.RuleAt(index)
			if !ruleOK {
				return nil, ReceiptAssemblyFailureSnapshotNamespace
			}
			capability, capabilityOK := mount.receipt.capability(rule.Role)
			if !capabilityOK || capability.state != bindingState || capability.authority != bindingState.authority {
				return nil, ReceiptAssemblyFailureSnapshotNamespace
			}
		}
		for index := 0; index < template.TransferCount(); index++ {
			transfer, transferOK := template.TransferAt(index)
			if !transferOK {
				return nil, ReceiptAssemblyFailureSnapshotNamespace
			}
			for _, role := range transfer.Factors {
				capability, capabilityOK := mount.receipt.capability(role)
				if !capabilityOK || capability.state != bindingState || capability.authority != bindingState.authority {
					return nil, ReceiptAssemblyFailureSnapshotNamespace
				}
			}
		}
		if !appendMountedArtifactReceipt(result, mount.mountID, mount.receipt) {
			return nil, ReceiptAssemblyFailureSnapshotNamespace
		}
	}
	return result, ReceiptAssemblyFailureNone
}

func linkTransportFactorSemantic(state *schemaBindingState, capability RuleSlotCapability) (composition.Key, bool) {
	if state == nil || state.schema == nil || state.phase != schemaBindingSealed || !capability.link() || capability.state != state || capability.authority != state.authority {
		return composition.Key{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	rule, roleOK := state.roleSlots[capability]
	ruleOrdinal, ruleOK := state.schema.ruleOrdinalOf(rule)
	shape, shapeOK := state.schema.ruleShapeAt(ruleOrdinal)
	if !roleOK || !ruleOK || !shapeOK || shape.OutputKind != composition.FactorOutput || !shape.Output.Available() {
		return composition.Key{}, false
	}
	return shape.Output, true
}

// appendMountedArtifactReceipt admits one already-sealed scalar template into
// the shared mounted planes. Scalar relations were closed once by
// artifact.NewArtifactScalarTemplate; this pass only resolves Link roles, substitutes
// mount-qualified IDs, and checks that those substitutions stay in the mount.
func appendMountedArtifactReceipt(rows *artifactReceiptTopology, mount identity.ContentID, receipt *ArtifactScalarReceipt) bool {
	if rows == nil || rows.pointMeta == nil || rows.bodies == nil || rows.ruleSet == nil || rows.callStages == nil || receipt == nil || !receipt.sealed || !receipt.template.Available() || !mount.Available() {
		return false
	}
	template := receipt.template
	artifactID := template.ArtifactID()
	points := make(map[identity.ContentID]identity.ContentID, template.PointCount())
	var initial identity.ContentID
	for index := 0; index < template.PointCount(); index++ {
		point, pointOK := template.PointAt(index)
		if !pointOK {
			return false
		}
		mounted := mountedArtifactID("analysis/engine/artifact-point/v1", mount, artifactID, point.ID)
		if !mounted.Available() {
			return false
		}
		if _, duplicate := points[point.ID]; duplicate {
			return false
		}
		if _, duplicate := rows.pointMeta[mounted]; duplicate {
			return false
		}
		points[point.ID] = mounted
		rows.points = append(rows.points, mounted)
		rows.pointMeta[mounted] = artifactPointMetadata{mount: mount, artifact: artifactID, reusable: point.ID, decisions: point.Decisions, initial: point.Initial}
		if point.Initial {
			if initial.Available() {
				return false
			}
			initial = mounted
		}
	}
	if !initial.Available() {
		return false
	}

	regions := make(map[identity.ContentID]identity.ContentID, template.RegionCount())
	seenRegionIDs := make(map[identity.ContentID]struct{}, len(rows.regions)+template.RegionCount())
	for _, prior := range rows.regions {
		seenRegionIDs[prior.id] = struct{}{}
	}
	regionOffset := len(rows.regions)
	for index := 0; index < template.RegionCount(); index++ {
		region, regionOK := template.RegionAt(index)
		if !regionOK {
			return false
		}
		mounted := mountedArtifactID("analysis/engine/artifact-wto-region/v1", mount, artifactID, region.ID)
		if !mounted.Available() {
			return false
		}
		if _, duplicate := regions[region.ID]; duplicate {
			return false
		}
		if _, duplicate := seenRegionIDs[mounted]; duplicate {
			return false
		}
		regions[region.ID] = mounted
		seenRegionIDs[mounted] = struct{}{}
		rows.regions = append(rows.regions, artifactWTORegionRow{mount: mount, artifact: artifactID, reusable: region.ID, id: mounted, cyclic: region.Cyclic})
	}
	for index := 0; index < template.RegionCount(); index++ {
		region, regionOK := template.RegionAt(index)
		if !regionOK {
			return false
		}
		row := &rows.regions[regionOffset+index]
		head, headOK := points[region.Head]
		if !headOK {
			return false
		}
		row.head = head
		if region.Parent.Available() {
			parent, parentOK := regions[region.Parent]
			if !parentOK {
				return false
			}
			row.parent = parent
		}
		row.members = make([]identity.ContentID, len(region.Members))
		for memberIndex, member := range region.Members {
			mounted, memberOK := points[member]
			if !memberOK {
				return false
			}
			row.members[memberIndex] = mounted
		}
	}

	routes := make(map[identity.ContentID]artifactEnvironmentRow, template.EdgeCount())
	seenEdgeIDs := make(map[identity.ContentID]struct{}, len(rows.edges)+template.EdgeCount()+template.TransferCount())
	for _, prior := range rows.edges {
		seenEdgeIDs[prior.id] = struct{}{}
	}
	for index := 0; index < template.EdgeCount(); index++ {
		edge, edgeOK := template.EdgeAt(index)
		if !edgeOK {
			return false
		}
		mounted := mountedArtifactID("analysis/engine/artifact-environment-edge/v1", mount, artifactID, edge.ID)
		from, fromOK := points[edge.From]
		to, toOK := points[edge.To]
		if !mounted.Available() || !fromOK || !toOK {
			return false
		}
		if _, duplicate := seenEdgeIDs[mounted]; duplicate {
			return false
		}
		row := artifactEnvironmentRow{mount: mount, artifact: artifactID, reusable: edge.ID, id: mounted, from: from, to: to, route: edge.Route, guard: edge.Guard, decision: edge.Decision, guarded: edge.Guarded, truth: edge.Truth, component: edge.Component, mu: edge.Mu, reset: edge.Reset, hasReset: edge.HasReset, resets: edge.Resets, arm: edge.Arm, transportOnly: edge.From == edge.To && !edge.Component.Available() && !edge.HasReset && !edge.Mu.Available()}
		rows.edges = append(rows.edges, row)
		seenEdgeIDs[mounted] = struct{}{}
		if edge.Route.Available() {
			if _, duplicate := routes[edge.Route]; !duplicate {
				routes[edge.Route] = row
			}
		}
	}
	for index := 0; index < template.TransferCount(); index++ {
		edge, edgeOK := template.TransferAt(index)
		if !edgeOK {
			return false
		}
		capabilities := make([]RuleSlotCapability, len(edge.Factors))
		for factorIndex, role := range edge.Factors {
			capability, capabilityOK := receipt.capability(role)
			if !capabilityOK {
				return false
			}
			capabilities[factorIndex] = capability
		}
		row := artifactEnvironmentRow{mount: mount, artifact: artifactID, reusable: edge.ID, from: points[edge.From], to: points[edge.To], id: mountedArtifactID("analysis/engine/artifact-environment-edge/v1", mount, artifactID, edge.ID), transportOnly: edge.Full, local: true, full: edge.Full, factorRoles: capabilities}
		if !row.id.Available() || !row.from.Available() || !row.to.Available() {
			return false
		}
		if _, duplicate := seenEdgeIDs[row.id]; duplicate {
			return false
		}
		rows.edges = append(rows.edges, row)
		seenEdgeIDs[row.id] = struct{}{}
	}

	for index := 0; index < template.EventCount(); index++ {
		event, eventOK := template.EventAt(index)
		if !eventOK {
			return false
		}
		row := artifactWTOEventRow{mount: mount, artifact: artifactID, kind: event.Kind}
		if event.Region.Available() {
			region, regionOK := regions[event.Region]
			if !regionOK {
				return false
			}
			row.region = region
		}
		if event.Point.Available() {
			point, pointOK := points[event.Point]
			if !pointOK {
				return false
			}
			row.point = point
		}
		rows.events = append(rows.events, row)
	}

	for index := 0; index < template.RuleCount(); index++ {
		rule, ruleOK := template.RuleAt(index)
		if !ruleOK {
			return false
		}
		capability, capabilityOK := receipt.capability(rule.Role)
		mountedPoint, pointOK := points[rule.Point]
		if !capabilityOK || !rule.Stage.Valid() || !pointOK {
			return false
		}
		input := identity.ContentID{}
		if rule.Input.Available() {
			var inputOK bool
			input, inputOK = points[rule.Input]
			if !inputOK {
				return false
			}
		}
		key := artifactMountedRule{role: capability, mount: mount, point: rule.Point, occurrence: rule.ID}
		if _, duplicate := rows.ruleSet[key]; duplicate {
			return false
		}
		bound := artifactRuleInput{point: rule.Input, mountedPoint: mountedPoint, mountedInput: input, stage: rule.Stage}
		if rule.Route.Available() {
			predecessor, predecessorOK := routes[rule.Route]
			if !predecessorOK || predecessor.from != input {
				return false
			}
			bound.predecessor, bound.routed = predecessor, true
		}
		rows.ruleSet[key] = bound
		if rule.Stage.NativeCall() {
			callKey := artifactMountedRuleOccurrence{role: capability, mount: mount, occurrence: rule.ID}
			if _, duplicate := rows.callStages[callKey]; duplicate {
				return false
			}
			rows.callStages[callKey] = artifactNativeCallStage{stage: rule.Stage, point: rule.Point, input: rule.Input, mountedPoint: mountedPoint, mountedInput: input}
		}
	}
	for index := 0; index < template.BodyCount(); index++ {
		body, bodyOK := template.BodyAt(index)
		if !bodyOK {
			return false
		}
		key := artifactMountedBody{mount: mount, body: body.ID}
		if _, duplicate := rows.bodies[key]; duplicate {
			return false
		}
		// ArtifactScalarReceipt owns these slices and seals them before this
		// pass. Body transports intentionally retain reusable point IDs; the
		// mounted point inverse resolves them later.
		rows.bodies[key] = artifactBodyTransport{entry: body.Entry, exits: body.Exits}
	}
	rows.mounts = append(rows.mounts, artifactMountReceipt{mount: mount, artifact: artifactID, program: template.ProgramID(), initial: initial})
	return true
}

// sealNativeCallStageDirectory retains only the compact native-stage inverse
// needed after the expanded artifact snapshot is released. Every entry must
// already have an attached semantic member under the exact
// role+mount+point+occurrence identity.
func sealNativeCallStageDirectory(rows *artifactReceiptTopology, directory *semanticDirectory) (map[artifactMountedRuleOccurrence]artifactNativeCallStage, bool) {
	if rows == nil {
		return nil, true
	}
	if directory == nil || rows.callStages == nil {
		return nil, false
	}
	result := make(map[artifactMountedRuleOccurrence]artifactNativeCallStage, len(rows.callStages))
	for key, stage := range rows.callStages {
		rule, found := rows.ruleSet[artifactMountedRule{role: key.role, mount: key.mount, point: stage.point, occurrence: key.occurrence}]
		memberID := mountedRuleMemberID(key.role, key.mount, stage.point, key.occurrence)
		if !found || !stage.stage.NativeCall() || rule.stage != stage.stage || rule.point != stage.input || rule.mountedPoint != stage.mountedPoint || rule.mountedInput != stage.mountedInput || !memberID.Available() {
			return nil, false
		}
		if _, attached := directory.member(memberID); !attached {
			return nil, false
		}
		if _, duplicate := result[key]; duplicate {
			return nil, false
		}
		result[key] = stage
	}
	return result, true
}

// validPayload checks only mount substitution and publication ownership. The
// scalar artifact relations were admitted once by artifact.NewArtifactScalarTemplate;
// repeating their WTO/edge proofs here only re-proves immutable input.
func (rows *artifactReceiptTopology) validPayload(topology *BindingTopology) bool {
	if rows == nil || len(rows.mounts) == 0 || len(rows.points) == 0 || len(rows.events) == 0 || len(rows.pointMeta) != len(rows.points) || topology != nil && len(rows.pointRef) != len(rows.points) {
		return false
	}
	if rows.bootstrap == nil || !rows.bootstrap.owner.Available() || !rows.bootstrap.point.Known || !rows.bootstrap.point.PointID.Available() || len(rows.bootstrap.transports) != 0 && len(rows.bootstrap.transports) != 2 {
		return false
	}
	if topology != nil {
		authorizedTransports, transportsAuthorized := sealedLinkBootstrapTransportPair(topology.state)
		if (len(rows.bootstrap.transports) == 0 && transportsAuthorized) || (len(rows.bootstrap.transports) != 0 && !transportsAuthorized) {
			return false
		}
		seenCapabilities := make(map[RuleSlotCapability]struct{}, len(rows.bootstrap.transports))
		seenFactors := make(map[composition.Key]struct{}, len(rows.bootstrap.transports))
		for index, transport := range rows.bootstrap.transports {
			factor, factorOK := linkTransportFactorSemantic(topology.state, transport.capability)
			if !factorOK || factor != transport.factor || transport.capability.authority != topology.authority || transport.capability != authorizedTransports[index] {
				return false
			}
			if _, duplicate := seenCapabilities[transport.capability]; duplicate {
				return false
			}
			if _, duplicate := seenFactors[transport.factor]; duplicate {
				return false
			}
			seenCapabilities[transport.capability] = struct{}{}
			seenFactors[transport.factor] = struct{}{}
		}
	}
	mounts := make(map[identity.ContentID]artifactMountReceipt, len(rows.mounts))
	for _, mount := range rows.mounts {
		metadata, initialOK := rows.pointMeta[mount.initial]
		if !mount.mount.Available() || !mount.artifact.Available() || !mount.program.Available() || !mount.initial.Available() || !initialOK || !metadata.initial || metadata.mount != mount.mount || metadata.artifact != mount.artifact {
			return false
		}
		if _, duplicate := mounts[mount.mount]; duplicate {
			return false
		}
		mounts[mount.mount] = mount
	}
	points := make(map[identity.ContentID]struct{}, len(rows.points))
	pointMounts := make(map[identity.ContentID]identity.ContentID, len(rows.points))
	sourcePoints := make(map[artifactMountedPoint]struct{}, len(rows.points))
	initialCounts := make(map[identity.ContentID]int, len(rows.mounts))
	for _, id := range rows.points {
		if !id.Available() {
			return false
		}
		if _, duplicate := points[id]; duplicate {
			return false
		}
		points[id] = struct{}{}
		metadata, metadataOK := rows.pointMeta[id]
		mount, mountOK := mounts[metadata.mount]
		if !metadataOK || !metadata.reusable.Available() || !mountOK || mount.artifact != metadata.artifact {
			return false
		}
		if metadata.initial {
			if id != mount.initial {
				return false
			}
			initialCounts[metadata.mount]++
		}
		pointMounts[id] = metadata.mount
		sourcePoints[artifactMountedPoint{mount: metadata.mount, reusable: metadata.reusable}] = struct{}{}
	}
	for mount := range mounts {
		if initialCounts[mount] != 1 {
			return false
		}
	}
	regions := make(map[identity.ContentID]struct{}, len(rows.regions))
	regionMounts := make(map[identity.ContentID]identity.ContentID, len(rows.regions))
	for _, region := range rows.regions {
		mount, mountOK := mounts[region.mount]
		if !region.id.Available() || !region.head.Available() || !mountOK || mount.artifact != region.artifact || !region.reusable.Available() {
			return false
		}
		if _, duplicate := regions[region.id]; duplicate {
			return false
		}
		headMount, headMountOK := pointMounts[region.head]
		if !headMountOK || headMount != region.mount {
			return false
		}
		for _, member := range region.members {
			memberMount, pointOK := pointMounts[member]
			if !pointOK || memberMount != region.mount {
				return false
			}
		}
		regions[region.id] = struct{}{}
		regionMounts[region.id] = region.mount
	}
	for _, region := range rows.regions {
		if region.parent.Available() {
			parentMount, exists := regionMounts[region.parent]
			if !exists || parentMount != region.mount || region.parent == region.id {
				return false
			}
		}
	}
	edges := make(map[identity.ContentID]struct{}, len(rows.edges))
	for _, edge := range rows.edges {
		mount, mountOK := mounts[edge.mount]
		if !mountOK || mount.artifact != edge.artifact || !edge.reusable.Available() || !edge.id.Available() {
			return false
		}
		fromMount, fromOK := pointMounts[edge.from]
		if !fromOK || fromMount != edge.mount {
			return false
		}
		toMount, toOK := pointMounts[edge.to]
		if !toOK || toMount != edge.mount {
			return false
		}
		if _, duplicate := edges[edge.id]; duplicate {
			return false
		}
		edges[edge.id] = struct{}{}
	}
	for _, event := range rows.events {
		mount, mountOK := mounts[event.mount]
		if !mountOK || mount.artifact != event.artifact {
			return false
		}
		if event.region.Available() {
			regionMount, regionOK := regionMounts[event.region]
			if !regionOK || regionMount != event.mount {
				return false
			}
		}
		if event.point.Available() {
			pointMount, pointOK := pointMounts[event.point]
			if !pointOK || pointMount != event.mount {
				return false
			}
		}
	}
	for key, input := range rows.ruleSet {
		mount, mountOK := mounts[key.mount]
		if !mountOK || !key.role.mounted() || !input.stage.Valid() || topology != nil && (key.role.state != topology.state || key.role.authority != topology.authority) || !key.point.Available() || !key.occurrence.Available() {
			return false
		}
		if _, pointOK := sourcePoints[artifactMountedPoint{mount: key.mount, reusable: key.point}]; !pointOK || !input.mountedPoint.Available() || pointMounts[input.mountedPoint] != key.mount {
			return false
		}
		if input.point.Available() {
			if _, inputOK := sourcePoints[artifactMountedPoint{mount: key.mount, reusable: input.point}]; !inputOK || !input.mountedInput.Available() || pointMounts[input.mountedInput] != key.mount {
				return false
			}
		} else if input.mountedInput.Available() {
			return false
		}
		if input.routed {
			if input.predecessor.mount != key.mount || input.predecessor.artifact != mount.artifact {
				return false
			}
			fromMount, fromOK := pointMounts[input.predecessor.from]
			toMount, toOK := pointMounts[input.predecessor.to]
			if !fromOK || !toOK || fromMount != key.mount || toMount != key.mount {
				return false
			}
		}
	}
	nativeCount := 0
	for key, input := range rows.ruleSet {
		if !input.stage.NativeCall() {
			continue
		}
		nativeCount++
		callKey := artifactMountedRuleOccurrence{role: key.role, mount: key.mount, occurrence: key.occurrence}
		stage, found := rows.callStages[callKey]
		if !found || stage.stage != input.stage || stage.point != key.point || stage.input != input.point || stage.mountedPoint != input.mountedPoint || stage.mountedInput != input.mountedInput {
			return false
		}
	}
	if nativeCount != len(rows.callStages) {
		return false
	}
	for key, transport := range rows.bodies {
		if _, mountOK := mounts[key.mount]; !mountOK || !key.body.Available() || len(transport.entry) == 0 || len(transport.exits) == 0 {
			return false
		}
		for _, reusable := range transport.entry {
			if _, pointOK := sourcePoints[artifactMountedPoint{mount: key.mount, reusable: reusable}]; !pointOK {
				return false
			}
		}
		for _, reusable := range transport.exits {
			if _, pointOK := sourcePoints[artifactMountedPoint{mount: key.mount, reusable: reusable}]; !pointOK {
				return false
			}
		}
	}
	if topology != nil {
		expectedPoints := len(rows.points)
		if rows.bootstrap != nil {
			expectedPoints++
		}
		expectedEnvironmentEdges, expectedFactorEdges := 0, 0
		for _, edge := range rows.edges {
			if edge.local && !edge.full {
				expectedFactorEdges += len(edge.factorRoles)
			} else {
				expectedEnvironmentEdges++
			}
		}
		expectedFactorEdges += len(rows.mounts) * len(rows.bootstrap.transports)
		if topology.plan == nil || len(topology.plan.spec.Points) != expectedPoints || len(topology.plan.spec.EnvironmentEdges) != expectedEnvironmentEdges || len(topology.plan.spec.FactorEdges) != expectedFactorEdges {
			return false
		}
		for id, ref := range rows.pointRef {
			if !id.Available() || ref == 0 {
				return false
			}
			if _, found := topology.directory.point(id); !found {
				return false
			}
		}
		if rows.bootstrap != nil && (rows.bootstrap.ref == 0 || !rows.bootstrap.semantic.Available()) {
			return false
		}
		if rows.bootstrap != nil {
			if _, found := topology.directory.point(rows.bootstrap.semantic); !found {
				return false
			}
		}
	}
	return true
}

// valid keeps the open construction gates explicit. A non-nil topology is a
// published-owner query and must use the constant-time sealed owner fence.
func (rows *artifactReceiptTopology) valid(topology *BindingTopology) bool {
	if topology == nil {
		return rows != nil && rows.sealed == nil
	}
	return rows != nil && rows.sealed == topology && topology.artifact == rows
}

// seal performs the sole complete proof after the equation topology and its
// semantic directory exist. The private receipt planes are immutable after
// this point, so all later access is authenticated by exact owner identity.
func (rows *artifactReceiptTopology) seal(topology *BindingTopology) bool {
	if rows == nil || topology == nil || rows.sealed != nil || topology.artifact != rows || !rows.validPayload(topology) {
		return false
	}
	rows.sealed = topology
	return true
}

func (rows *artifactReceiptTopology) mountedSite(mount, reusable identity.ContentID) (equation.Site, bool) {
	if rows == nil || rows.mounted == nil || !mount.Available() || !reusable.Available() {
		return equation.Site{}, false
	}
	site, ok := rows.mounted[artifactMountedPoint{mount: mount, reusable: reusable}]
	// Mounted rule occurrence admission happens while the source Batch is
	// still open. Site.Available is intentionally a post-seal capability;
	// the caller holds the assembly's source lock and Batch.From authenticates
	// this exact mapped Site in the open phase.
	return site, ok
}

func (rows *artifactReceiptTopology) mountedPoint(mount, reusable identity.ContentID) (equation.Site, equation.PointRef, bool) {
	if rows == nil || rows.mounted == nil || rows.mountedRef == nil || !mount.Available() || !reusable.Available() {
		return equation.Site{}, 0, false
	}
	key := artifactMountedPoint{mount: mount, reusable: reusable}
	site, siteOK := rows.mounted[key]
	ref, refOK := rows.mountedRef[key]
	return site, ref, siteOK && refOK && ref != 0
}

func (rows *artifactReceiptTopology) mountedBody(mount, body identity.ContentID) (artifactBodyTransport, bool) {
	if rows == nil || rows.bodies == nil || !mount.Available() || !body.Available() {
		return artifactBodyTransport{}, false
	}
	value, ok := rows.bodies[artifactMountedBody{mount: mount, body: body}]
	return value, ok && len(value.entry) != 0 && len(value.exits) != 0
}

func (rows *artifactReceiptTopology) mountedRule(role RuleSlotCapability, mount, point, occurrence identity.ContentID) (artifactRuleInput, bool) {
	if rows == nil || rows.ruleSet == nil || !role.mounted() || !mount.Available() || !point.Available() || !occurrence.Available() {
		return artifactRuleInput{}, false
	}
	input, ok := rows.ruleSet[artifactMountedRule{role: role, mount: mount, point: point, occurrence: occurrence}]
	return input, ok
}
