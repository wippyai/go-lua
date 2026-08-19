package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/canonical"
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

// mountedArtifactRows is the construction-local mounted address plane the
// declaration pass reads: the Site each mounted point was admitted under and
// the set of template rule rows an issuance may be declared at. It carries no
// geometry and no dense address - both are derived from the sealed templates
// by constructTopology - and it is released when the declaration ends.
type mountedArtifactRows struct {
	points    []identity.ContentID
	pointMeta map[identity.ContentID]artifactPointMetadata
	sites     map[identity.ContentID]equation.Site
	mounted   map[artifactMountedPoint]equation.Site
	ruleSet   map[artifactMountedRule]struct{}
	bootstrap *linkBootstrapRows
}

type linkBootstrapRows struct {
	owner identity.ContentID
	point LinkBootstrapPoint
	site  equation.Site
	roles map[identity.ContentID]RuleSlotCapability
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

type artifactMountedRule struct {
	role       RuleSlotCapability
	mount      identity.ContentID
	point      identity.ContentID
	occurrence identity.ContentID
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

type artifactPointMetadata struct {
	mount     identity.ContentID
	artifact  identity.ContentID
	reusable  identity.ContentID
	decisions []identity.ContentID
	initial   bool
}

// mountedCallStage is a cold, graph-owned proof that one exact
// mounted Call occurrence was attached at its ProgramArtifact-issued native
// stage. It is issued by occurrence alone: callers never submit a reusable
// point to this lookup and therefore cannot splice another artifact point.
type mountedCallStage struct {
	graph    *equation.Graph
	topology *BindingTopology
	key      artifactMountedRuleOccurrence
	stage    artifactNativeCallStage
}

func (receipt mountedCallStage) row() (artifactNativeCallStage, bool) {
	if receipt.graph == nil || receipt.topology == nil || !receipt.topology.valid() || receipt.topology.nativeCallStages == nil || !receipt.topology.topology.OwnsGraph(receipt.graph) {
		return artifactNativeCallStage{}, false
	}
	row, ok := receipt.topology.nativeCallStages[receipt.key]
	return row, ok && row == receipt.stage && row.stage.NativeCall() && row.point.Available() && row.input.Available() && row.mountedPoint.Available() && row.mountedInput.Available()
}

func (receipt mountedCallStage) Available() bool { _, ok := receipt.row(); return ok }
func (receipt mountedCallStage) Stage() rows.ArtifactRuleStage {
	row, ok := receipt.row()
	if !ok {
		return rows.ArtifactRuleStageInvalid
	}
	return row.stage
}
func (receipt mountedCallStage) MountID() identity.ContentID {
	if _, ok := receipt.row(); !ok {
		return identity.ContentID{}
	}
	return receipt.key.mount
}
func (receipt mountedCallStage) OccurrenceID() identity.ContentID {
	if _, ok := receipt.row(); !ok {
		return identity.ContentID{}
	}
	return receipt.key.occurrence
}
func (receipt mountedCallStage) ReusablePointID() identity.ContentID {
	row, ok := receipt.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.point
}
func (receipt mountedCallStage) ReusableInputPointID() identity.ContentID {
	row, ok := receipt.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.input
}

// RuleMember resolves the already-attached member authenticated by this
// stage proof. The caller cannot substitute another point or occurrence.
func (receipt mountedCallStage) RuleMember() (ProgramMember, bool) {
	row, ok := receipt.row()
	if !ok {
		return ProgramMember{}, false
	}
	if receipt.topology == nil || receipt.topology.directory == nil {
		return ProgramMember{}, false
	}
	locator, found := receipt.topology.directory.member(mountedRuleMemberID(receipt.key.role, receipt.key.mount, row.point, receipt.key.occurrence))
	if !found {
		return ProgramMember{}, false
	}
	member, ok := locator.Resolve(receipt.graph)
	if !ok || !receipt.graph.OwnsMember(member) {
		return ProgramMember{}, false
	}
	return ProgramMember{graph: receipt.graph, topology: receipt.topology, member: member, locator: locator}, true
}

// assembleSealedProgramMounts folds one mounted program. Source admission runs
// through the Binding's row transaction; the geometry those sealed rows fold
// into is derived by the pure constructor, which owns the schedule gate, the
// duplicate-identity refusal and the published directory.
func assembleSealedProgramMounts(binding *SchemaBinding, mounts []sealedProgramMount, admission MountedProgramAdmission, bootstrap LinkBootstrapWitness) (*equation.Graph, *BindingTopology, receiptSealFailure, ProgramAdmissionStage, receiptAssemblyFailure, topologyConstructionRefusal, bool) {
	builder, lowering, assembled := beginMountedProgramAssembly(binding, mounts, bootstrap)
	if !assembled || builder == nil {
		return nil, nil, receiptSealFailure{}, ProgramAdmissionNone, lowering, topologyConstructionRefusal{}, false
	}
	// A mounted program publishes its result through queries; an inventory
	// with none states no observable program at all.
	if len(admission.Queries) == 0 {
		builder.abort()
		return nil, nil, receiptSealFailure{}, ProgramAdmissionQuery, receiptAssemblyFailureNone, topologyConstructionRefusal{}, false
	}
	declaration, seal, stage, declared := declareMountedProgram(builder, mounts, bootstrap, admission)
	if !declared {
		builder.abort()
		return nil, nil, seal, stage, receiptAssemblyFailureNone, topologyConstructionRefusal{}, false
	}
	constructed, refusal := constructTopology(declaration)
	builder.abort()
	if refusal.Available() || !constructed.Available() {
		if !refusal.Available() {
			refusal = refuseTopologySeal(topologyConstructionStepTopologySeal, 0)
		}
		return nil, nil, seal, stage, receiptAssemblyFailureNone, refusal, false
	}
	return constructed.graph, constructed.topology, receiptSealFailure{}, ProgramAdmissionNone, receiptAssemblyFailureNone, topologyConstructionRefusal{}, true
}

type receiptAssemblyFailure uint8

const (
	receiptAssemblyFailureNone receiptAssemblyFailure = iota
	receiptAssemblyFailureInput
	receiptAssemblyFailureSchema
	receiptAssemblyFailureSnapshot
	receiptAssemblyFailureTransaction
	receiptAssemblyFailureStructuralRows
	// Snapshot sub-stages preserve the owner boundary that rejected an
	// otherwise immutable artifact. They are diagnostics only: no partial
	// receipt escapes the assembly.
	receiptAssemblyFailureSnapshotBootstrap
	receiptAssemblyFailureSnapshotMount
	receiptAssemblyFailureSnapshotArtifact
	receiptAssemblyFailureSnapshotNamespace
	receiptAssemblyFailureSnapshotTopology
	receiptAssemblyFailureSnapshotTopologyMount
	receiptAssemblyFailureSnapshotTopologyPoint
	receiptAssemblyFailureSnapshotTopologyBootstrap
	receiptAssemblyFailureSnapshotTopologyRule
)

// Failure projects one lowering boundary onto the engine's public failure
// vocabulary. The ordinal enters the site preimage and never leaves this
// package.
func (failure receiptAssemblyFailure) Failure() SolveFailure {
	if failure == receiptAssemblyFailureNone {
		return SolveFailure{}
	}
	return receiptFailure(SolveFailureFamilyCompile, "receipt-assembly", uint64(failure))
}

func beginMountedProgramMounts(binding *SchemaBinding, mounts []MountedProgramArtifact, bootstrap LinkBootstrapWitness) (*BindingTopologyBuilder, receiptAssemblyFailure, bool) {
	sealed, ok := sealMountedProgramArtifacts(mounts)
	if !ok {
		return nil, receiptAssemblyFailureInput, false
	}
	return beginMountedProgramAssembly(binding, sealed, bootstrap)
}

func beginMountedProgramAssembly(binding *SchemaBinding, mounts []sealedProgramMount, bootstrap LinkBootstrapWitness) (*BindingTopologyBuilder, receiptAssemblyFailure, bool) {
	if binding == nil || !binding.Sealed() || len(mounts) == 0 || !bootstrap.Available() {
		return nil, receiptAssemblyFailureInput, false
	}
	schema := binding.Schema()
	if schema == nil || !schema.Available() {
		return nil, receiptAssemblyFailureSchema, false
	}
	rows, snapshotFailure := buildMountedArtifactRows(mounts, identity.ContentID(schema.ID().Digest()), bootstrap, binding.state)
	if snapshotFailure != receiptAssemblyFailureNone {
		return nil, snapshotFailure, false
	}
	builder, ok := binding.beginBindingTopologyBuilder()
	if !ok {
		return nil, receiptAssemblyFailureTransaction, false
	}
	if !admitMountedArtifactSites(builder, rows) {
		builder.abort()
		return nil, receiptAssemblyFailureStructuralRows, false
	}
	return builder, receiptAssemblyFailureNone, true
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

// artifactSourceKey derives one cold source key from an artifact content
// identity. The namespace enters the preimage, so a point, an edge, and an
// occurrence built from the same content identity name three keys with three
// distinct digests rather than one digest under three version labels.
func artifactSourceKey(domain artifactSourceDomain, id identity.ContentID) (composition.Key, bool) {
	if !id.Available() {
		return composition.Key{}, false
	}
	return framedCompositionKey(domain.name, domain.version, func(writer *canonical.DigestWriter) bool {
		return writer.Bytes(id[:]) == nil
	})
}

func admitMountedArtifactSites(builder *BindingTopologyBuilder, rows *mountedArtifactRows) bool {
	if builder == nil || rows == nil {
		return false
	}
	sites := make(map[identity.ContentID]equation.Site, len(rows.points))
	for _, id := range rows.points {
		source, sourceOK := artifactSourceKey(artifactPointSource, id)
		metadata, metadataOK := rows.pointMeta[id]
		if !metadataOK || !metadata.reusable.Available() {
			return false
		}
		decisions := make([]equation.Decision, len(metadata.decisions))
		for index, semanticID := range metadata.decisions {
			decisionKey := mountedArtifactID("analysis/engine/artifact-decision/v1", metadata.mount, metadata.artifact, semanticID)
			decision, decisionOK := equation.NewDecision(mustArtifactSourceKey(artifactPointSource, decisionKey))
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
		site, siteOK := builder.admitSite(source, scope, init, disposition)
		if !sourceOK || !siteOK {
			return false
		}
		sites[id] = site
	}
	if rows.bootstrap != nil {
		point := rows.bootstrap.point
		source, sourceOK := artifactSourceKey(artifactPointSource, point.PointID)
		decisions := make([]equation.Decision, len(point.DecisionID))
		for index, semanticID := range point.DecisionID {
			decisionKey := mountedArtifactID("analysis/engine/link-bootstrap-decision/v1", rows.bootstrap.owner, rows.bootstrap.owner, semanticID)
			decision, decisionOK := equation.NewDecision(mustArtifactSourceKey(artifactPointSource, decisionKey))
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
		site, siteOK := builder.admitSite(source, scope, init, disposition)
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
	inner, locked := builder.lockSourcesOpen()
	if !locked || builder.mountedRows != nil {
		if locked {
			inner.failLocked()
			inner.mu.Unlock()
		}
		return false
	}
	builder.mountedRows = rows
	inner.mu.Unlock()
	return true
}

// mustArtifactSourceKey returns the unavailable key for an unnameable source
// identity. Every consumer admits the key through a constructor that rejects
// it, so the failure is carried rather than hidden.
func mustArtifactSourceKey(domain artifactSourceDomain, id identity.ContentID) composition.Key {
	key, ok := artifactSourceKey(domain, id)
	if !ok {
		return composition.Key{}
	}
	return key
}

// receiptArtifactRowFailure names the artifact-row boundary a declaration
// could not be addressed through.
type receiptArtifactRowFailure uint8

const (
	receiptArtifactRowFailureNone receiptArtifactRowFailure = iota
	receiptArtifactRowFailureOwner
	receiptArtifactRowFailurePoint
	receiptArtifactRowFailureBootstrap
)

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
	return artifactSourceKey(artifactEdgeSource, id)
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

func buildMountedArtifactRows(mounts []sealedProgramMount, schemaID identity.ContentID, bootstrap LinkBootstrapWitness, bindingState *schemaBindingState) (*mountedArtifactRows, receiptAssemblyFailure) {
	if len(mounts) == 0 || !schemaID.Available() || !bootstrap.Available() || bindingState == nil {
		return nil, receiptAssemblyFailureSnapshotBootstrap
	}
	bootstrapPoint, pointOK := bootstrap.Point()
	if !pointOK {
		return nil, receiptAssemblyFailureSnapshotBootstrap
	}
	occurrences := make(map[identity.ContentID]struct{}, bootstrap.OccurrenceCount())
	for index := 0; index < bootstrap.OccurrenceCount(); index++ {
		id, idOK := bootstrap.OccurrenceAt(index)
		if !idOK {
			return nil, receiptAssemblyFailureSnapshotBootstrap
		}
		occurrences[id] = struct{}{}
	}
	roles := make(map[identity.ContentID]RuleSlotCapability, len(occurrences))
	for id := range occurrences {
		capability, capabilityOK := bootstrap.capabilityFor(id)
		if !capabilityOK || !capability.link() || capability.state != bindingState || capability.authority != bindingState.authority {
			return nil, receiptAssemblyFailureSnapshotBootstrap
		}
		roles[id] = capability
	}
	transports := bootstrap.transportCapabilityCount()
	seenTransportCapabilities := make(map[RuleSlotCapability]struct{}, transports)
	seenTransportFactors := make(map[composition.Key]struct{}, transports)
	if transports != 0 && transports != 2 {
		return nil, receiptAssemblyFailureSnapshotBootstrap
	}
	authorizedTransports, transportsAuthorized := sealedLinkBootstrapTransportPair(bindingState)
	if (transports == 0 && transportsAuthorized) || (transports != 0 && !transportsAuthorized) {
		return nil, receiptAssemblyFailureSnapshotBootstrap
	}
	for index := 0; index < transports; index++ {
		capability, capabilityOK := bootstrap.transportCapabilityAt(index)
		factor, factorOK := linkTransportFactorSemantic(bindingState, capability)
		if !capabilityOK || !factorOK || capability != authorizedTransports[index] {
			return nil, receiptAssemblyFailureSnapshotBootstrap
		}
		if _, duplicate := seenTransportCapabilities[capability]; duplicate {
			return nil, receiptAssemblyFailureSnapshotBootstrap
		}
		if _, duplicate := seenTransportFactors[factor]; duplicate {
			return nil, receiptAssemblyFailureSnapshotBootstrap
		}
		seenTransportCapabilities[capability] = struct{}{}
		seenTransportFactors[factor] = struct{}{}
	}
	result := &mountedArtifactRows{pointMeta: make(map[identity.ContentID]artifactPointMetadata), sites: make(map[identity.ContentID]equation.Site), mounted: make(map[artifactMountedPoint]equation.Site), ruleSet: make(map[artifactMountedRule]struct{}), bootstrap: &linkBootstrapRows{owner: bootstrap.OwnerID(), point: bootstrapPoint, roles: roles}}
	seenMounts := make(map[identity.ContentID]struct{}, len(mounts))
	for _, mount := range mounts {
		if mount.template == nil || !mount.template.Available() || !mount.module.Available() || mount.template.SchemaID() != schemaID {
			return nil, receiptAssemblyFailureSnapshotMount
		}
		template := mount.template
		initialCount := 0
		for index := 0; index < template.PointCount(); index++ {
			point, pointOK := template.PointAt(index)
			if !pointOK {
				return nil, receiptAssemblyFailureSnapshotTopologyPoint
			}
			if point.Initial {
				initialCount++
			}
		}
		if initialCount != 1 {
			return nil, receiptAssemblyFailureSnapshotTopologyPoint
		}
		if _, duplicate := seenMounts[mount.module]; duplicate {
			return nil, receiptAssemblyFailureSnapshotMount
		}
		seenMounts[mount.module] = struct{}{}
		for index := 0; index < template.RuleCount(); index++ {
			rule, ruleOK := template.RuleAt(index)
			if !ruleOK {
				return nil, receiptAssemblyFailureSnapshotNamespace
			}
			capability, capabilityOK := sealedRoleCapability(mount.capabilities, rule.Role)
			if !capabilityOK || capability.state != bindingState || capability.authority != bindingState.authority {
				return nil, receiptAssemblyFailureSnapshotNamespace
			}
		}
		for index := 0; index < template.TransferCount(); index++ {
			transfer, transferOK := template.TransferAt(index)
			if !transferOK {
				return nil, receiptAssemblyFailureSnapshotNamespace
			}
			for _, role := range transfer.Factors {
				capability, capabilityOK := sealedRoleCapability(mount.capabilities, role)
				if !capabilityOK || capability.state != bindingState || capability.authority != bindingState.authority {
					return nil, receiptAssemblyFailureSnapshotNamespace
				}
			}
		}
		if !appendMountedProgramMount(result, mount.module, template, mount.capabilities) {
			return nil, receiptAssemblyFailureSnapshotNamespace
		}
	}
	return result, receiptAssemblyFailureNone
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

func sealedRoleCapability(capabilities map[rows.ArtifactScalarRole]RuleSlotCapability, role rows.ArtifactScalarRole) (RuleSlotCapability, bool) {
	if capabilities == nil || !role.Available() {
		return RuleSlotCapability{}, false
	}
	capability, ok := capabilities[role]
	return capability, ok && capability.mounted()
}

// appendMountedProgramMount admits one sealed scalar template into the
// shared mounted planes. Scalar relations were closed once by
// artifact.NewArtifactScalarTemplate; this pass only resolves Link roles, substitutes
// mount-qualified IDs, and checks that those substitutions stay in the mount.
func appendMountedProgramMount(rows *mountedArtifactRows, mount identity.ContentID, template *rows.ArtifactScalarTemplate, capabilities map[rows.ArtifactScalarRole]RuleSlotCapability) bool {
	if rows == nil || rows.pointMeta == nil || rows.ruleSet == nil || template == nil || !template.Available() || !mount.Available() {
		return false
	}
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

	seenEdgeIDs := make(map[identity.ContentID]struct{}, template.EdgeCount()+template.TransferCount())
	for index := 0; index < template.EdgeCount(); index++ {
		edge, edgeOK := template.EdgeAt(index)
		if !edgeOK {
			return false
		}
		mounted := mountedArtifactID("analysis/engine/artifact-environment-edge/v1", mount, artifactID, edge.ID)
		_, fromOK := points[edge.From]
		_, toOK := points[edge.To]
		if !mounted.Available() || !fromOK || !toOK {
			return false
		}
		if _, duplicate := seenEdgeIDs[mounted]; duplicate {
			return false
		}
		seenEdgeIDs[mounted] = struct{}{}
	}
	for index := 0; index < template.TransferCount(); index++ {
		transfer, transferOK := template.TransferAt(index)
		if !transferOK {
			return false
		}
		for _, role := range transfer.Factors {
			if _, capabilityOK := sealedRoleCapability(capabilities, role); !capabilityOK {
				return false
			}
		}
		mounted := mountedArtifactID("analysis/engine/artifact-environment-edge/v1", mount, artifactID, transfer.ID)
		_, fromOK := points[transfer.From]
		_, toOK := points[transfer.To]
		if !mounted.Available() || !fromOK || !toOK {
			return false
		}
		if _, duplicate := seenEdgeIDs[mounted]; duplicate {
			return false
		}
		seenEdgeIDs[mounted] = struct{}{}
	}
	for index := 0; index < template.RuleCount(); index++ {
		rule, ruleOK := template.RuleAt(index)
		if !ruleOK {
			return false
		}
		capability, capabilityOK := sealedRoleCapability(capabilities, rule.Role)
		_, pointOK := points[rule.Point]
		if !capabilityOK || !rule.Stage.Valid() || !pointOK {
			return false
		}
		if rule.Input.Available() {
			if _, inputOK := points[rule.Input]; !inputOK {
				return false
			}
		}
		key := artifactMountedRule{role: capability, mount: mount, point: rule.Point, occurrence: rule.ID}
		if _, duplicate := rows.ruleSet[key]; duplicate {
			return false
		}
		rows.ruleSet[key] = struct{}{}
	}
	return true
}

// hasMountedSite reports whether the mounted point plane addresses this
// coordinate. It is the artifact-row half of a declared query anchor: the
// dense Point address itself is derived by the constructor.
func (rows *mountedArtifactRows) hasMountedSite(mount, reusable identity.ContentID) bool {
	_, ok := rows.mountedSite(mount, reusable)
	return ok
}

func (rows *mountedArtifactRows) mountedSite(mount, reusable identity.ContentID) (equation.Site, bool) {
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

// mountedRule reports whether the sealed templates carry the exact
// role+mount+point+occurrence rule row one issuance names. The row's own
// coordinates - its stage, its input Point and its route proof - are read from
// the template by the constructor, never copied here.
func (rows *mountedArtifactRows) mountedRule(role RuleSlotCapability, mount, point, occurrence identity.ContentID) bool {
	if rows == nil || rows.ruleSet == nil || !role.mounted() || !mount.Available() || !point.Available() || !occurrence.Available() {
		return false
	}
	_, ok := rows.ruleSet[artifactMountedRule{role: role, mount: mount, point: point, occurrence: occurrence}]
	return ok
}
