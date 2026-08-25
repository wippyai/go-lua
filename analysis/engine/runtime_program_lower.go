package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	programstate "github.com/wippyai/go-lua/analysis/schema/program/state"
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
// by constructTopology - and never escapes the declaration fold.
type mountedArtifactRows struct {
	points    []identity.ContentID
	pointMeta map[identity.ContentID]artifactPointMetadata
	sites     map[identity.ContentID]equation.Site
	mounted   map[artifactMountedPoint]equation.Site
	ruleSet   map[artifactMountedRule]artifactMountedRuleSource
	bootstrap *linkBootstrapRows
}

// artifactMountedRuleSource is the candidate row one mounted placement was
// issued for, carried with the immutable state of the Program that issued it.
// State travels with the ordinal because a generated Family holds rows from
// several mounted Programs, so equal ordinals from different mounts are
// different rows.
type artifactMountedRuleSource struct {
	state   programstate.State
	ordinal uint32
	present bool
}

type linkBootstrapRows struct {
	owner   identity.ContentID
	point   LinkBootstrapPoint
	site    equation.Site
	witness LinkBootstrapWitness
}

type artifactMountedPoint struct {
	mount    identity.ContentID
	reusable identity.ContentID
}

// artifactMountedBody is the immutable parent-issued body transport anchor.
// Point IDs remain reusable artifact IDs here; every later use must resolve
// them through the same mount-qualified point row.
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
	stage        schema.Key
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

// programCallRow is a graph-owned handle for one exact
// mounted Call occurrence was attached at its ProgramArtifact-issued native
// stage. It is issued by occurrence alone: callers never submit a reusable
// point to this lookup and therefore cannot splice another artifact point.
type programCallRow struct {
	program *CommittedProgram
	key     artifactMountedRuleOccurrence
	stage   artifactNativeCallStage

	available bool
}

// completeStage proves that the handed key names exactly this stage row in the
// committed program and that the row carries its four authenticated points. A
// committed program is immutable, so the issuer decides this once.
func (handle programCallRow) completeStage() bool {
	if !handle.program.valid() || handle.program.nativeCallStages == nil {
		return false
	}
	row, ok := handle.program.nativeCallStages[handle.key]
	return ok && row == handle.stage && row.point.Available() && row.input.Available() && row.mountedPoint.Available() && row.mountedInput.Available()
}

// Available reports whether this handle was issued against a committed stage
// row. MountedNativeCallStage is the sole issuer and seals the verdict.
func (handle programCallRow) Available() bool { return handle.available }
func (handle programCallRow) Stage() schema.Key {
	if !handle.available {
		return ""
	}
	return handle.stage.stage
}
func (handle programCallRow) MountID() identity.ContentID {
	if !handle.available {
		return identity.ContentID{}
	}
	return handle.key.mount
}
func (handle programCallRow) OccurrenceID() identity.ContentID {
	if !handle.available {
		return identity.ContentID{}
	}
	return handle.key.occurrence
}
func (handle programCallRow) ReusablePointID() identity.ContentID {
	if !handle.available {
		return identity.ContentID{}
	}
	return handle.stage.point
}
func (handle programCallRow) ReusableInputPointID() identity.ContentID {
	if !handle.available {
		return identity.ContentID{}
	}
	return handle.stage.input
}

// RuleMember resolves the already-attached member authenticated by this
// stage proof. The caller cannot substitute another point or occurrence.
func (handle programCallRow) RuleMember() (ProgramMember, bool) {
	if !handle.available {
		return ProgramMember{}, false
	}
	locator, found := handle.program.directory.member(mountedRuleMemberID(handle.key.role, handle.key.mount, handle.stage.point, handle.key.occurrence))
	if !found {
		return ProgramMember{}, false
	}
	member, ok := locator.Resolve(handle.program.graph)
	if !ok || !handle.program.graph.OwnsMember(member) {
		return ProgramMember{}, false
	}
	return ProgramMember{program: handle.program, member: member, locator: locator}, true
}

// assembleSealedProgramMounts folds one mounted program. Source admission runs
// through the constructor's private row workspace; the geometry those sealed
// rows fold into is derived by the pure constructor, which owns the schedule
// gate, the duplicate-identity refusal and the published directory.
func assembleSealedProgramMounts(binding *SchemaBinding, mounts []sealedProgramMount, contexts executioncontext.Directory, admission MountedProgramAdmission, pointTransitions []ProgramPointTransitionAdmission, bootstrap LinkBootstrapWitness) (*CommittedProgram, programSealFailure, ProgramAdmissionStage, programAssemblyFailure, topologyConstructionRefusal, bool) {
	if !contexts.Available() || !bootstrap.Available() || contexts.LinkID() != bootstrap.OwnerID() {
		return nil, programSealFailure{}, ProgramAdmissionNone, programAssemblyFailureInput, topologyConstructionRefusal{}, false
	}
	rowsWorkspace, lowering, assembled := assembleProgramRows(binding, mounts, bootstrap)
	if !assembled || rowsWorkspace == nil {
		return nil, programSealFailure{}, ProgramAdmissionNone, lowering, topologyConstructionRefusal{}, false
	}
	// A mounted program publishes its result through queries; an inventory
	// with none states no observable program at all.
	if len(admission.Queries) == 0 {
		return nil, programSealFailure{}, ProgramAdmissionQuery, programAssemblyFailureNone, topologyConstructionRefusal{}, false
	}
	declaration, seal, stage, declared := declareMountedProgram(rowsWorkspace, mounts, contexts, bootstrap, admission, pointTransitions)
	if !declared {
		return nil, seal, stage, programAssemblyFailureNone, topologyConstructionRefusal{}, false
	}
	constructed, refusal := constructTopology(declaration)
	if refusal.Available() || !constructed.Available() {
		if !refusal.Available() {
			refusal = refuseTopologySeal(topologyConstructionStepTopologySeal, 0)
		}
		return nil, seal, stage, programAssemblyFailureNone, refusal, false
	}
	return constructed.program, programSealFailure{}, ProgramAdmissionNone, programAssemblyFailureNone, topologyConstructionRefusal{}, true
}

type programAssemblyFailure uint8

const (
	programAssemblyFailureNone programAssemblyFailure = iota
	programAssemblyFailureInput
	programAssemblyFailureSchema
	programAssemblyFailureSnapshot
	programAssemblyFailureRows
	programAssemblyFailureStructuralRows
	// Snapshot sub-stages preserve the owner boundary that rejected an
	// otherwise immutable artifact. They are diagnostics only: no partial
	// no partial row escapes the assembly.
	programAssemblyFailureSnapshotTransport
	programAssemblyFailureSnapshotMount
	programAssemblyFailureSnapshotArtifact
	programAssemblyFailureSnapshotNamespace
	programAssemblyFailureSnapshotTopology
	programAssemblyFailureSnapshotTopologyMount
	programAssemblyFailureSnapshotTopologyPoint
	programAssemblyFailureSnapshotBootstrap
	programAssemblyFailureSnapshotTopologyRule
)

// Failure projects one lowering boundary onto the engine's public failure
// vocabulary. The ordinal enters the site preimage and never leaves this
// package.
func (failure programAssemblyFailure) Failure() SolveFailure {
	if failure == programAssemblyFailureNone {
		return SolveFailure{}
	}
	return boundaryFailure(SolveFailureFamilyCompile, "program-assembly", uint64(failure))
}

func assembleProgramRows(binding *SchemaBinding, mounts []sealedProgramMount, bootstrap LinkBootstrapWitness) (*programRows, programAssemblyFailure, bool) {
	if binding == nil || !binding.Sealed() || len(mounts) == 0 || !bootstrap.Available() {
		return nil, programAssemblyFailureInput, false
	}
	schema := binding.Schema()
	if schema == nil || !schema.Available() {
		return nil, programAssemblyFailureSchema, false
	}
	artifactSchema, artifactSchemaOK := bindingArtifactSchemaID(binding)
	if !artifactSchemaOK {
		return nil, programAssemblyFailureSchema, false
	}
	rows, snapshotFailure := buildMountedArtifactRows(mounts, artifactSchema, bootstrap, binding.state)
	if snapshotFailure != programAssemblyFailureNone {
		return nil, snapshotFailure, false
	}
	rowsWorkspace, ok := newProgramRows(binding)
	if !ok {
		return nil, programAssemblyFailureRows, false
	}
	if !admitMountedArtifactSites(rowsWorkspace, rows) {
		return nil, programAssemblyFailureStructuralRows, false
	}
	return rowsWorkspace, programAssemblyFailureNone, true
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

func admitMountedArtifactSites(rowsWorkspace *programRows, rows *mountedArtifactRows) bool {
	if rowsWorkspace == nil || rows == nil {
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
		site, siteOK := rowsWorkspace.admitSite(source, scope, init, disposition)
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
		site, siteOK := rowsWorkspace.admitSite(source, scope, init, disposition)
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
	if rowsWorkspace.mountedRows != nil || rowsWorkspace.batch == nil || rowsWorkspace.batch.Sealed() {
		return false
	}
	return rowsWorkspace.setMountedRows(rows)
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

// programArtifactRowFailure names the artifact-row boundary a declaration
// could not be addressed through.
type programArtifactRowFailure uint8

const (
	programArtifactRowFailureNone programArtifactRowFailure = iota
	programArtifactRowFailureOwner
	programArtifactRowFailurePoint
	programArtifactRowFailureBootstrap
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

// sealedLinkCapabilityInventory is the schema-owned Link input inventory. The
// capability directory is keyed by opaque capabilities, so its map order is
// not semantic; schema rule ordinal is the canonical order of the inventory.
func sealedLinkCapabilityInventory(state *schemaBindingState) ([]RuleSlotCapability, bool) {
	if state == nil {
		return nil, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingSealed || state.authority == nil || state.schema == nil || !state.schema.Available() {
		return nil, false
	}
	count := state.schema.ruleCount()
	result := make([]RuleSlotCapability, 0)
	for ordinal := uint64(0); ordinal < count; ordinal++ {
		capability := RuleSlotCapability{state: state, authority: state.authority, ordinal: ordinal, kind: ruleCapabilityLink}
		semantic, registered := state.roleSlots[capability]
		if !registered {
			continue
		}
		if !capability.link() || semantic != state.schema.ruleSemanticAt(ordinal) {
			return nil, false
		}
		result = append(result, capability)
	}
	for capability, semantic := range state.roleSlots {
		if capability.kind != ruleCapabilityLink {
			continue
		}
		if !capability.link() || capability.state != state || capability.authority != state.authority || capability.ordinal >= count || semantic != state.schema.ruleSemanticAt(capability.ordinal) {
			return nil, false
		}
	}
	return result, true
}

// validateSealedLinkBootstrapCatalog authenticates one complete Link catalog
// against the sealed schema inventory. The schema's rule ordinals remain the
// only ordering authority; the witness contributes an unordered namespace per
// capability. A duplicate, missing, or foreign capability therefore fails at
// the Snapshot boundary without making caller order semantic.
func validateSealedLinkBootstrapCatalog(state *schemaBindingState, witness LinkBootstrapWitness) bool {
	expected, expectedOK := sealedLinkCapabilityInventory(state)
	if !expectedOK || !witness.Available() || witness.catalogCapabilityCount() != len(expected) {
		return false
	}
	if len(witness.byCapability) != len(expected) {
		return false
	}
	expectedSet := make(map[RuleSlotCapability]struct{}, len(expected))
	for _, capability := range expected {
		expectedSet[capability] = struct{}{}
		if _, present := witness.byCapability[capability]; !present {
			return false
		}
	}
	seenCapabilities := make(map[RuleSlotCapability]struct{}, witness.catalogCapabilityCount())
	for index := 0; index < witness.catalogCapabilityCount(); index++ {
		capability, capabilityOK := witness.catalogCapabilityAt(index)
		if _, expectedCapability := expectedSet[capability]; !capabilityOK || !expectedCapability {
			return false
		}
		if _, duplicate := seenCapabilities[capability]; duplicate {
			return false
		}
		seenCapabilities[capability] = struct{}{}
	}
	seenClaims := make(map[linkBootstrapClaim]struct{}, witness.claimCount())
	for index := 0; index < witness.claimCount(); index++ {
		capability, occurrence, claimOK := witness.claimAt(index)
		if !claimOK || !capability.link() || !occurrence.Available() {
			return false
		}
		claim := linkBootstrapClaim{capability: capability, occurrence: occurrence}
		if _, duplicate := seenClaims[claim]; duplicate {
			return false
		}
		seenClaims[claim] = struct{}{}
	}
	for capability, occurrences := range witness.byCapability {
		for occurrence := range occurrences {
			if _, present := seenClaims[linkBootstrapClaim{capability: capability, occurrence: occurrence}]; !present {
				return false
			}
		}
	}
	return true
}

func buildMountedArtifactRows(mounts []sealedProgramMount, schemaID identity.ContentID, bootstrap LinkBootstrapWitness, bindingState *schemaBindingState) (*mountedArtifactRows, programAssemblyFailure) {
	if len(mounts) == 0 || !schemaID.Available() || !bootstrap.Available() || bindingState == nil {
		return nil, programAssemblyFailureSnapshotBootstrap
	}
	if !validateSealedLinkBootstrapCatalog(bindingState, bootstrap) {
		return nil, programAssemblyFailureSnapshotBootstrap
	}
	bootstrapPoint, pointOK := bootstrap.Point()
	if !pointOK {
		return nil, programAssemblyFailureSnapshotBootstrap
	}
	for index := 0; index < bootstrap.claimCount(); index++ {
		capability, occurrence, claimOK := bootstrap.claimAt(index)
		if !claimOK || !capability.link() || capability.state != bindingState || capability.authority != bindingState.authority || !occurrence.Available() {
			return nil, programAssemblyFailureSnapshotBootstrap
		}
	}
	authorizedTransports, transportsAuthorized := sealedLinkBootstrapTransports(bindingState)
	seenTransportCapabilities := make(map[RuleSlotCapability]struct{}, len(authorizedTransports))
	seenTransportFactors := make(map[composition.Key]struct{}, len(authorizedTransports))
	if transportsAuthorized {
		for _, capability := range authorizedTransports {
			factor, factorOK := linkTransportFactorSemantic(bindingState, capability)
			if !factorOK {
				return nil, programAssemblyFailureSnapshotBootstrap
			}
			if _, duplicate := seenTransportCapabilities[capability]; duplicate {
				return nil, programAssemblyFailureSnapshotBootstrap
			}
			if _, duplicate := seenTransportFactors[factor]; duplicate {
				return nil, programAssemblyFailureSnapshotBootstrap
			}
			seenTransportCapabilities[capability] = struct{}{}
			seenTransportFactors[factor] = struct{}{}
		}
	}
	result := &mountedArtifactRows{pointMeta: make(map[identity.ContentID]artifactPointMetadata), sites: make(map[identity.ContentID]equation.Site), mounted: make(map[artifactMountedPoint]equation.Site), ruleSet: make(map[artifactMountedRule]artifactMountedRuleSource), bootstrap: &linkBootstrapRows{owner: bootstrap.OwnerID(), point: bootstrapPoint, witness: bootstrap}}
	seenMounts := make(map[identity.ContentID]struct{}, len(mounts))
	for _, mount := range mounts {
		if mount.template == nil || !mount.template.Available() || !mount.module.Available() || mount.template.SchemaID() != schemaID {
			return nil, programAssemblyFailureSnapshotMount
		}
		template := mount.template
		initialCount := 0
		for index := 0; index < template.PointCount(); index++ {
			point, pointOK := template.PointAt(index)
			if !pointOK {
				return nil, programAssemblyFailureSnapshotTopologyPoint
			}
			if point.Initial {
				initialCount++
			}
		}
		if initialCount != 1 {
			return nil, programAssemblyFailureSnapshotTopologyPoint
		}
		if _, duplicate := seenMounts[mount.module]; duplicate {
			return nil, programAssemblyFailureSnapshotMount
		}
		seenMounts[mount.module] = struct{}{}
		for index := 0; index < template.RuleCount(); index++ {
			rule, ruleOK := template.RuleAt(index)
			if !ruleOK {
				return nil, programAssemblyFailureSnapshotNamespace
			}
			capability, capabilityOK := sealedRoleCapability(mount.capabilities, rule.Role)
			if !capabilityOK || capability.state != bindingState || capability.authority != bindingState.authority {
				return nil, programAssemblyFailureSnapshotNamespace
			}
		}
		for index := 0; index < template.TransferCount(); index++ {
			transfer, transferOK := template.TransferAt(index)
			if !transferOK {
				return nil, programAssemblyFailureSnapshotNamespace
			}
			for _, factor := range transfer.Factors {
				capability, capabilityOK := mount.factors[factor]
				if _, factorOK := capability.semantic(bindingState, bindingState.authority); !capabilityOK || !factorOK {
					return nil, programAssemblyFailureSnapshotNamespace
				}
			}
		}
		if !appendMountedProgramMount(result, mount.module, template, mount.capabilities, mount.factors, mount.state) {
			return nil, programAssemblyFailureSnapshotNamespace
		}
	}
	return result, programAssemblyFailureNone
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
func appendMountedProgramMount(rows *mountedArtifactRows, mount identity.ContentID, template *rows.ArtifactScalarTemplate, capabilities map[rows.ArtifactScalarRole]RuleSlotCapability, factors map[rows.ArtifactScalarFactor]FactorSlotCapability, state programstate.State) bool {
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
		for _, factor := range transfer.Factors {
			capability, capabilityOK := factors[factor]
			if !capabilityOK || !capability.Available() {
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
		if !capabilityOK || !rule.Stage.Available() || !pointOK {
			return false
		}
		inputCount := rule.InputPointCount()
		if inputCount < 0 || inputCount > len(rule.Inputs) {
			return false
		}
		for inputIndex := 0; inputIndex < inputCount; inputIndex++ {
			input, inputOK := rule.InputPointAt(inputIndex)
			if !inputOK || !input.Available() {
				return false
			}
			if _, pointOK := points[input]; !pointOK {
				return false
			}
		}
		key := artifactMountedRule{role: capability, mount: mount, point: rule.Point, occurrence: rule.ID}
		if _, duplicate := rows.ruleSet[key]; duplicate {
			return false
		}
		// A placement that names a candidate row is only admissible from a
		// Program whose state this Link mounted: the ordinal addresses that
		// publication and nothing else.
		if rule.SourcePresent && !state.Available() {
			return false
		}
		rows.ruleSet[key] = artifactMountedRuleSource{state: state, ordinal: rule.Source, present: rule.SourcePresent}
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

// mountedRuleSource returns the candidate row this mounted placement was
// issued for. The second result distinguishes an admitted placement carrying
// no candidate row from a coordinate this plane does not address at all.
func (rows *mountedArtifactRows) mountedRuleSource(role RuleSlotCapability, mount, point, occurrence identity.ContentID) (artifactMountedRuleSource, bool) {
	if rows == nil || rows.ruleSet == nil {
		return artifactMountedRuleSource{}, false
	}
	source, ok := rows.ruleSet[artifactMountedRule{role: role, mount: mount, point: point, occurrence: occurrence}]
	return source, ok
}
