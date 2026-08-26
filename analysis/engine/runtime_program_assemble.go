package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	programstate "github.com/wippyai/go-lua/analysis/schema/program/state"
)

// MountedProgramRole is one template role bound to this Link's mounted
// capability. Assemble seals the pair into the mount capability map.
type MountedProgramRole struct {
	Scalar     rows.ArtifactScalarRole
	Capability RuleSlotCapability
}

// MountedProgramFactor binds one template Factor identity directly to the
// exact Factor slot of this Link's sealed SchemaBinding.
type MountedProgramFactor struct {
	Scalar     rows.ArtifactScalarFactor
	Capability FactorSlotCapability
}

// MountedProgramArtifact is one sealed template plus this Link's mounted
// role capabilities and mount identity.
type MountedProgramArtifact struct {
	Template *rows.ArtifactScalarTemplate
	Roles    []MountedProgramRole
	Factors  []MountedProgramFactor
	Module   identity.ContentID
	// State is the immutable cold publication this template was lowered from.
	// A rule whose candidates are Program rows is redeemed against it, so the
	// mount carries the publication its ordinals address rather than leaving a
	// consumer to find one by artifact identity.
	State programstate.State
}

// ProgramAdmissionStage names which admission pass refused. The declaration
// handle stays inside ConstructProgram.
type ProgramAdmissionStage uint8

const (
	ProgramAdmissionNone ProgramAdmissionStage = iota
	ProgramAdmissionLink
	ProgramAdmissionMounted
	ProgramAdmissionQuery
	ProgramAdmissionSeal
)

// LinkRuleAdmission is one Link-global occurrence to admit.
type LinkRuleAdmission struct {
	Capability RuleSlotCapability
	Occurrence identity.ContentID
}

// MountedRuleAdmission is one mounted occurrence to admit.
type MountedRuleAdmission struct {
	Capability RuleSlotCapability
	Mount      identity.ContentID
	Point      identity.ContentID
	Occurrence identity.ContentID
}

// MountedPointRuleAdmission is one artifact-independent closure occurrence.
// Construction expands it once over every sealed mounted Point.
type MountedPointRuleAdmission struct {
	Capability RuleSlotCapability
	Occurrence identity.ContentID
}

// programQueryAdmit is the erased mounted query row. Implementations live on
// the sealed query cells; the declaration pass states the row and the
// constructor resolves the Point it is anchored at.
type programQueryAdmit interface {
	declareMountedQuery(state *schemaBindingState, authority *schemaBindingAuthority, context executioncontext.Context, id, mount, point identity.ContentID, writes exactQueryPointWrites) (declaredQueryRow, []*ruleSummaryMapping, bool)
	bindProgramQuery(plane *programPlane, query equation.Query) (queryRow, bool)
}

// ProgramQueryAdmission is one mounted query row to admit.
type ProgramQueryAdmission struct {
	admit   programQueryAdmit
	ID      identity.ContentID
	Mount   identity.ContentID
	Point   identity.ContentID
	Context executioncontext.Context
}

// NewSummaryQueryAdmission seals one summary query row.
func NewSummaryQueryAdmission[V, R any](implementation *SummaryQueryImplementation[V, R], id, mount, point identity.ContentID, context executioncontext.Context) (ProgramQueryAdmission, bool) {
	if implementation == nil || !context.Available() || !id.Available() || !mount.Available() || !point.Available() {
		return ProgramQueryAdmission{}, false
	}
	return ProgramQueryAdmission{admit: implementation, Context: context, ID: id, Mount: mount, Point: point}, true
}

// NewExactQueryAdmission seals one exact query row.
func NewExactQueryAdmission[V, R any](implementation *ExactQueryImplementation[V, R], id, mount, point identity.ContentID, context executioncontext.Context) (ProgramQueryAdmission, bool) {
	if implementation == nil || !context.Available() || !id.Available() || !mount.Available() || !point.Available() {
		return ProgramQueryAdmission{}, false
	}
	return ProgramQueryAdmission{admit: implementation, Context: context, ID: id, Mount: mount, Point: point}, true
}

// MountedProgramAdmission is the sealed admit inventory for one assemble.
type MountedProgramAdmission struct {
	Link         []LinkRuleAdmission
	Mounted      []MountedRuleAdmission
	MountedPoint []MountedPointRuleAdmission
	Activation   []MountedActivationAdmit
	Queries      []ProgramQueryAdmission
}

// sealedProgramMount is one template plus this Link's sealed role
// capabilities and mount identity. Assemble snapshots from this value.
type sealedProgramMount struct {
	template     *rows.ArtifactScalarTemplate
	capabilities map[rows.ArtifactScalarRole]RuleSlotCapability
	factors      map[rows.ArtifactScalarFactor]FactorSlotCapability
	module       identity.ContentID
	state        programstate.State
}

// CommittedProgram is the committed program one construction published: the
// sealed equation Topology and its initial Graph, the directory addressing
// both, and the declared rows a seal binds a runtime from. It is finished at
// construction and never mutated; sealing reads it and mints a Solver.
type CommittedProgram struct {
	self              *CommittedProgram
	graph             *equation.Graph
	topology          *equation.Topology
	relation          equation.Relation
	state             *schemaBindingState
	authority         *schemaBindingAuthority
	directory         *semanticDirectory
	contexts          executioncontext.Directory
	contextIndex      contextfiber.Index
	contextLayout     contextfiber.Layout
	pointOwners       []contextfiber.PointOwner
	nativeCallStages  map[artifactMountedRuleOccurrence]artifactNativeCallStage
	pointTransitions  []ProgramPointTransition
	activations       []programActivationBinding
	members           []programMemberBinding
	queries           []programQueryBinding
	addressed         []composition.Key
	artifactBacked    bool
	bootstrapOwner    identity.ContentID
	bootstrapPoint    identity.ContentID
	bootstrapSemantic identity.ContentID
	// admitted is the ownership verdict sealProgramAdmission reached once, on
	// the finished value. Accessors read it instead of re-deriving the whole
	// topology/context-plane proof - including the Index and Layout owner
	// digests - on every call. The zero value carries the false verdict, and
	// the self fence keeps a copied CommittedProgram out of the verdict.
	admitted bool
}

// sealProgramAdmission takes the committed program's ownership verdict exactly
// once, on the finished value. It is the sole writer of admitted and the sole
// caller of deriveAdmission.
func (committed *CommittedProgram) sealProgramAdmission() bool {
	if committed == nil {
		return false
	}
	committed.admitted = committed.deriveAdmission()
	return committed.admitted
}

// valid reports the verdict sealProgramAdmission already took. The self fence
// stays on the read path so a copy of the sealed value is never admitted.
func (committed *CommittedProgram) valid() bool {
	return committed != nil && committed.self == committed && committed.admitted
}

// deriveAdmission proves the whole committed shape: topology/graph/state
// ownership, the equation Relation's graph, and - for an artifact-backed
// program - the bootstrap identities and the exact compact context plane. It
// runs once, at the seal.
func (committed *CommittedProgram) deriveAdmission() bool {
	if committed == nil || committed.self != committed || committed.graph == nil || committed.topology == nil || committed.state == nil || committed.authority == nil || committed.directory == nil ||
		!committed.directory.ownedBy(committed.topology, committed.state, committed.authority) ||
		committed.state.phase != schemaBindingSealed || committed.state.authority != committed.authority || committed.state.schema == nil ||
		!committed.topology.OwnsComposition(committed.state.schema.cold) || !committed.graph.OwnsComposition(committed.state.schema.cold) ||
		!committed.topology.OwnsGraph(committed.graph) || !committed.relation.OwnedBy(committed.topology) {
		return false
	}
	expectedGraph, graphOK := committed.topology.Graph(committed.relation)
	if !graphOK || expectedGraph != committed.graph {
		return false
	}
	ownerAvailable, pointAvailable, semanticAvailable := committed.bootstrapOwner.Available(), committed.bootstrapPoint.Available(), committed.bootstrapSemantic.Available()
	if !committed.artifactBacked {
		return !committed.contexts.Available() && !committed.contextIndex.Available() && !committed.contextLayout.Available() && len(committed.pointOwners) == 0 &&
			!ownerAvailable && !pointAvailable && !semanticAvailable
	}
	if !ownerAvailable || !pointAvailable || !semanticAvailable || linkBootstrapPointSemanticID(committed.bootstrapOwner, committed.bootstrapPoint) != committed.bootstrapSemantic ||
		!committed.contexts.Available() || committed.contexts.LinkID() != committed.bootstrapOwner || len(committed.pointOwners) != committed.graph.PointCount() ||
		!committed.contextIndex.OwnedBy(committed.contexts, committed.graph.PointCount(), committed.relation.Generation()) ||
		!committed.contextLayout.OwnedBy(committed.contextIndex, committed.contexts, committed.pointOwners, committed.relation.Generation()) {
		return false
	}
	for _, transition := range committed.pointTransitions {
		if !transition.available {
			return false
		}
	}
	_, found := committed.directory.point(committed.bootstrapSemantic)
	return found
}

func (committed *CommittedProgram) lookupPoint(id identity.ContentID) (equation.Point, bool) {
	if !committed.valid() || !id.Available() || committed.directory == nil {
		return equation.Point{}, false
	}
	locator, found := committed.directory.point(id)
	if !found {
		return equation.Point{}, false
	}
	point, ok := locator.Resolve(committed.graph)
	if !ok || !committed.graph.OwnsPoint(point) {
		return equation.Point{}, false
	}
	return point, true
}

// ProgramQuery is the committed graph-owned Query handle.
type ProgramQuery struct {
	program  *CommittedProgram
	identity equation.Query
	locator  equation.QueryRowLocator
	key      identity.ContentID
}

// ContextID returns the exact execution context that owns this query's
// retained equation identity. Context is part of the equation query key, so
// two otherwise equal observations in different contexts remain distinct
// published rows.
func (query ProgramQuery) ContextID() identity.ContentID {
	if query.program == nil || !query.program.valid() || !query.identity.Key().Available() {
		return identity.ContentID{}
	}
	return query.identity.ContextID()
}

// PublicationKey is the snapshot row identity this query is sealed under.
func (query ProgramQuery) PublicationKey() (identity.ContentID, bool) {
	return query.key, query.key.Available()
}

// StateOrdinal returns the compact execution-state row retained for this
// exact query Context and graph Point. It is a read-only projection of the
// committed Link context plane; callers cannot mint or substitute a state
// row, and no Context is inferred from the mounted Point owner.
func (query ProgramQuery) StateOrdinal() (uint64, bool) {
	if query.program == nil || !query.program.valid() {
		return 0, false
	}
	state, ok := queryStateOrdinalOwned(query.program.graph, query.identity, query.program.contextIndex, query.program.contextLayout)
	if !ok {
		return 0, false
	}
	return uint64(state), true
}

// ProgramMember is the committed graph-owned rule member handle.
type ProgramMember struct {
	program *CommittedProgram
	member  equation.RuleMember
	locator equation.RuleMemberRowLocator
}

func (member ProgramMember) ownedBy(committed *CommittedProgram) bool {
	return member.program != nil && committed != nil && member.program == committed
}

func (committed *CommittedProgram) Query(id identity.ContentID) (ProgramQuery, bool) {
	if !committed.valid() || !id.Available() || committed.directory == nil {
		return ProgramQuery{}, false
	}
	locator, found := committed.directory.query(id)
	if !found {
		return ProgramQuery{}, false
	}
	query, ok := locator.Resolve(committed.graph)
	if !ok || !committed.graph.OwnsQuery(query) {
		return ProgramQuery{}, false
	}
	key := solvedRowKey(query.Key())
	if !key.Available() {
		return ProgramQuery{}, false
	}
	return ProgramQuery{program: committed, identity: query, locator: locator, key: key}, true
}

func (committed *CommittedProgram) lookupRuleMember(id identity.ContentID) (ProgramMember, bool) {
	if !committed.valid() || !id.Available() || committed.directory == nil {
		return ProgramMember{}, false
	}
	locator, found := committed.directory.member(id)
	if !found {
		return ProgramMember{}, false
	}
	member, ok := locator.Resolve(committed.graph)
	if !ok || !committed.graph.OwnsMember(member) {
		return ProgramMember{}, false
	}
	return ProgramMember{program: committed, member: member, locator: locator}, true
}

func (committed *CommittedProgram) RuleMember(id identity.ContentID) (ProgramMember, bool) {
	return committed.lookupRuleMember(id)
}

func (committed *CommittedProgram) MountedRuleMember(role RuleSlotCapability, mount, point, occurrence identity.ContentID) (ProgramMember, bool) {
	if !role.mounted() || !committed.valid() {
		return ProgramMember{}, false
	}
	return committed.lookupRuleMember(mountedRuleMemberID(role, mount, point, occurrence))
}

// ownsObservationContext authenticates the typed Context carried by an
// observation admission against this committed program's exact Link-owned
// directory and mounted module. A Context with a matching module but a foreign
// Link, a zero row, or an identity absent from the directory is refused; no
// directory position or first same-module row is substituted.
func (committed *CommittedProgram) ownsObservationContext(context executioncontext.Context, mount identity.ContentID) bool {
	if committed == nil || !committed.valid() || !committed.artifactBacked || !committed.contexts.Available() || !context.Available() || !mount.Available() ||
		context.LinkID() != committed.contexts.LinkID() || context.ModuleKey() != mount {
		return false
	}
	canonical, ok := committed.contexts.Context(context.ID())
	return ok && canonical.Available() && canonical.ID() == context.ID() && canonical.LinkID() == context.LinkID() && canonical.ModuleKey() == context.ModuleKey()
}

func (committed *CommittedProgram) LinkRuleMember(role RuleSlotCapability, occurrence identity.ContentID) (ProgramMember, bool) {
	if !role.link() || !committed.valid() || role.state != committed.state || role.authority != committed.authority || !committed.bootstrapOwner.Available() || !committed.bootstrapPoint.Available() {
		return ProgramMember{}, false
	}
	return committed.lookupRuleMember(linkRuleMemberID(role, committed.bootstrapOwner, committed.bootstrapPoint, occurrence))
}

// MountedPointRuleMember resolves one artifact-independent closure member at
// its exact mounted Point.
func (committed *CommittedProgram) MountedPointRuleMember(role RuleSlotCapability, mount, point, occurrence identity.ContentID) (ProgramMember, bool) {
	if committed == nil || !role.mountedPoint() {
		return ProgramMember{}, false
	}
	return committed.lookupRuleMember(mountedPointRuleMemberID(role, mount, point, occurrence))
}

func (committed *CommittedProgram) MountedNativeCallStage(role RuleSlotCapability, mount, occurrence identity.ContentID) (ProgramCallStage, bool) {
	if !committed.valid() || !role.mounted() || role.state != committed.state || role.authority != committed.authority || !mount.Available() || !occurrence.Available() || committed.nativeCallStages == nil {
		return ProgramCallStage{}, false
	}
	key := artifactMountedRuleOccurrence{role: role, mount: mount, occurrence: occurrence}
	stage, ok := committed.nativeCallStages[key]
	handle := programCallRow{program: committed, key: key, stage: stage}
	handle.available = ok && handle.completeStage()
	return ProgramCallStage{handle: handle}, handle.available
}

// ActivationMember is one exact activation member from the committed graph.
// It is distinct from ProgramMember because activation compilation needs the
// activation-row locator as well as the graph member.
type ActivationMember struct {
	program *CommittedProgram
	member  equation.RuleMember
	locator equation.ActivationMemberRowLocator
}

func (committed *CommittedProgram) lookupActivationMember(id identity.ContentID) (ActivationMember, bool) {
	if !committed.valid() || !id.Available() || committed.directory == nil {
		return ActivationMember{}, false
	}
	locator, found := committed.directory.activation(id)
	if !found {
		return ActivationMember{}, false
	}
	member, ok := locator.Resolve(committed.graph)
	if !ok || !committed.graph.OwnsMember(member) {
		return ActivationMember{}, false
	}
	return ActivationMember{program: committed, member: member, locator: locator}, true
}

func (committed *CommittedProgram) ActivationMember(id identity.ContentID) (ActivationMember, bool) {
	return committed.lookupActivationMember(id)
}

func (committed *CommittedProgram) MountedActivationMember(role RuleSlotCapability, mount, point, occurrence identity.ContentID) (ActivationMember, bool) {
	if !role.mounted() || !committed.valid() {
		return ActivationMember{}, false
	}
	return committed.lookupActivationMember(mountedRuleActivationID(role, mount, point, occurrence))
}

func (committed *CommittedProgram) publishedQueryKeys() ([]composition.Key, bool) {
	if !committed.valid() || committed.directory == nil {
		return nil, false
	}
	directory := committed.directory
	addressed := make([]composition.Key, 0, len(directory.queryOrder))
	for ordinal, id := range directory.queryOrder {
		entry, found := directory.resolve(id)
		locator, located := directory.query(id)
		resolved, resolvedOK := locator.Resolve(committed.graph)
		if !found || entry.kind != bindingSemanticQuery || int(entry.slot) != ordinal || !located || !resolvedOK {
			return nil, false
		}
		addressed = append(addressed, resolved.Key())
	}
	return addressed, true
}

// ProgramAssembleRefusal is the closed assemble refusal. Internal failure
// types stay inside the engine; callers read stage and SolveFailure only.
type ProgramAssembleRefusal struct {
	stage        ProgramAdmissionStage
	lowering     programAssemblyFailure
	construction topologyConstructionRefusal
	seal         programSealFailure
}

func (refusal ProgramAssembleRefusal) Stage() ProgramAdmissionStage { return refusal.stage }

func (refusal ProgramAssembleRefusal) Lowered() bool {
	return refusal.lowering != programAssemblyFailureNone
}

func (refusal ProgramAssembleRefusal) LoweringFailure() SolveFailure {
	return refusal.lowering.Failure()
}

func (refusal ProgramAssembleRefusal) Seal() SolveFailure { return refusal.seal.Failure() }

func (refusal ProgramAssembleRefusal) ArtifactRowOrdinal() (uint32, bool) {
	if refusal.seal.Phase() != programSealFailureArtifactRows {
		return 0, false
	}
	return refusal.seal.Ordinal(), true
}

func (refusal ProgramAssembleRefusal) MountedRole() (RuleSlotCapability, bool) {
	return refusal.seal.MountedCapability()
}

func (refusal ProgramAssembleRefusal) LinkRole() (RuleSlotCapability, bool) {
	return refusal.seal.LinkCapability()
}

// AdmissionRow is the ordinal of the declared Link, Mounted, or Query
// admission row a refused ProgramAdmissionLink, ProgramAdmissionMounted, or
// ProgramAdmissionQuery stage rejected. The boundary itself travels as the
// Seal identity; the row is data published beside it. It is absent for every
// other seal phase, including one that reached no admission row at all.
func (refusal ProgramAssembleRefusal) AdmissionRow() (uint32, bool) {
	switch refusal.seal.Phase() {
	case programSealFailureLinkIssuance, programSealFailureMountedIssuance,
		programSealFailureActivationIssuance, programSealFailureQueryBatch:
		return refusal.seal.Ordinal(), true
	default:
		return 0, false
	}
}

// Commit projects the geometry construction refusal onto the public failure
// vocabulary. A construction that published no geometry carries the stage it
// refused at; a construction that never ran carries nothing.
func (refusal ProgramAssembleRefusal) Commit() SolveFailure {
	return refusal.construction.Failure()
}

// ConstructionRow is the ordinal of the declared row a refused construction
// stopped on. The boundary itself travels as the Commit identity, so the row is
// published as the data beside it and never as a second coordinate of that
// identity.
func (refusal ProgramAssembleRefusal) ConstructionRow() (uint32, bool) {
	if !refusal.construction.Available() {
		return 0, false
	}
	return refusal.construction.Ordinal(), true
}

// ScheduleRow is the ordinal of the composition-schedule row a refused
// construction stopped on. It is zero for every other refusal.
func (refusal ProgramAssembleRefusal) ScheduleRow() uint32 {
	if refusal.construction.Step() != topologyConstructionStepSchedule {
		return 0
	}
	return refusal.construction.Ordinal()
}

// ObservationSealArguments is the closed observation input refusal.
func ObservationSealArguments() SolveFailure {
	return observationSealFailureArguments.Failure()
}

// ObservationSealPoint is the closed observation point refusal.
func ObservationSealPoint() SolveFailure {
	return observationSealFailurePoint.Failure()
}

// ProgramBootstrap is the Link-lane witness assemble consumes. Callers supply
// owner, point, and capability catalogs; they never name internal witness state.
type ProgramBootstrap struct {
	witness LinkBootstrapWitness
}

// ProgramBootstrapCatalog is one Link-lane occurrence inventory.
type ProgramBootstrapCatalog struct {
	Capability  RuleSlotCapability
	Occurrences []identity.ContentID
}

// NewProgramBootstrap seals the Link bootstrap witness from capability catalogs.
func NewProgramBootstrap(owner, pointID identity.ContentID, catalogs ...ProgramBootstrapCatalog) (ProgramBootstrap, bool) {
	if !owner.Available() || !pointID.Available() {
		return ProgramBootstrap{}, false
	}
	if len(catalogs) == 0 {
		witness, ok := NewLinkBootstrapWitness(owner, LinkBootstrapPoint{PointID: pointID, Known: true}, nil)
		if !ok {
			return ProgramBootstrap{}, false
		}
		return ProgramBootstrap{witness: witness}, true
	}
	mapped := make([]LinkBootstrapCatalog, len(catalogs))
	for index, catalog := range catalogs {
		mapped[index] = LinkBootstrapCatalog{Capability: catalog.Capability, Occurrences: catalog.Occurrences}
	}
	witness, ok := NewLinkBootstrapWitnessByCapability(owner, LinkBootstrapPoint{PointID: pointID, Known: true, Initial: true}, mapped...)
	if !ok {
		return ProgramBootstrap{}, false
	}
	return ProgramBootstrap{witness: witness}, true
}

// ProgramDeclaration is the sealed construction input: one sealed Binding, the
// artifact templates this Link mounts with their role capabilities, the Link
// bootstrap witness, the required Link execution-context directory, and the
// admission inventory the owners sealed. Every field is finished before the
// construction reads it; the committed program retains the directory and its
// graph-aligned compact context layout as immutable owner state.
type ProgramDeclaration struct {
	Binding   *SchemaBinding
	Mounts    []MountedProgramArtifact
	Bootstrap ProgramBootstrap
	// Contexts is the required Link-owned execution-context directory for an
	// artifact-backed construction. It is never inferred from mounts or
	// replaced with a synthetic single context.
	Contexts  executioncontext.Directory
	Admission MountedProgramAdmission
	// PointTransitions is the sole schema-row admission for cross-module point
	// geometry. Each pair carries the exact ModuleCallTransition and its
	// authenticated InitGeneration; ConstructProgram resolves all graph points
	// from those rows after the semantic directory is sealed.
	PointTransitions []ProgramPointTransitionAdmission
}

// ConstructProgram folds one sealed declaration into the committed program:
// the sealed equation Topology, its initial Graph, the directory addressing
// both, and the rows a seal binds a runtime from. It is the sole entry from a
// declaration to a program; callers never mint a construction row.
func ConstructProgram(declaration ProgramDeclaration) (*CommittedProgram, ProgramAssembleRefusal, bool) {
	if declaration.Binding == nil || !declaration.Binding.Sealed() {
		return nil, ProgramAssembleRefusal{lowering: programAssemblyFailureInput}, false
	}
	// ConstructProgram currently admits artifact-backed programs only. Keep
	// the directory boundary explicit here so a missing or foreign directory
	// cannot be mistaken for a later topology failure or silently defaulted.
	if len(declaration.Mounts) != 0 && (!declaration.Contexts.Available() || !declaration.Bootstrap.witness.Available() || declaration.Contexts.LinkID() != declaration.Bootstrap.witness.OwnerID()) {
		return nil, ProgramAssembleRefusal{lowering: programAssemblyFailureInput}, false
	}
	sealed, sealedOK := sealMountedProgramArtifacts(declaration.Mounts)
	if !sealedOK {
		return nil, ProgramAssembleRefusal{lowering: programAssemblyFailureInput}, false
	}
	program, seal, stage, lowering, construction, committed := assembleSealedProgramMounts(declaration.Binding, sealed, declaration.Contexts, declaration.Admission, declaration.PointTransitions, declaration.Bootstrap.witness)
	if !committed || program == nil {
		return nil, ProgramAssembleRefusal{stage: stage, lowering: lowering, construction: construction, seal: seal}, false
	}
	return program, ProgramAssembleRefusal{}, true
}

func sealMountedProgramArtifacts(mounts []MountedProgramArtifact) ([]sealedProgramMount, bool) {
	if len(mounts) == 0 {
		return nil, false
	}
	sealed := make([]sealedProgramMount, 0, len(mounts))
	byArtifact := make(map[identity.ContentID]sealedProgramMount, len(mounts))
	for _, mount := range mounts {
		if mount.Template == nil || !mount.Template.Available() || !mount.Module.Available() {
			return nil, false
		}
		artifactID := mount.Template.ArtifactID()
		if existing, have := byArtifact[artifactID]; have {
			if existing.template != mount.Template {
				return nil, false
			}
			sealed = append(sealed, sealedProgramMount{template: existing.template, capabilities: existing.capabilities, factors: existing.factors, module: mount.Module, state: existing.state})
			continue
		}
		if len(mount.Roles) != mount.Template.RoleCount() {
			return nil, false
		}
		if len(mount.Factors) != mount.Template.FactorCount() {
			return nil, false
		}
		capabilities := make(map[rows.ArtifactScalarRole]RuleSlotCapability, mount.Template.RoleCount())
		seenCapabilities := make(map[RuleSlotCapability]struct{}, mount.Template.RoleCount())
		for _, role := range mount.Roles {
			if !mount.Template.OwnsRole(role.Scalar) || !role.Capability.mounted() {
				return nil, false
			}
			if _, duplicate := capabilities[role.Scalar]; duplicate {
				return nil, false
			}
			if _, duplicate := seenCapabilities[role.Capability]; duplicate {
				return nil, false
			}
			capabilities[role.Scalar] = role.Capability
			seenCapabilities[role.Capability] = struct{}{}
		}
		for index := 0; index < mount.Template.RoleCount(); index++ {
			declared, declaredOK := mount.Template.RoleAt(index)
			if !declaredOK {
				return nil, false
			}
			capability, ok := capabilities[declared]
			if !ok || !capability.mounted() {
				return nil, false
			}
		}
		factors := make(map[rows.ArtifactScalarFactor]FactorSlotCapability, mount.Template.FactorCount())
		seenFactors := make(map[FactorSlotCapability]struct{}, mount.Template.FactorCount())
		for _, factor := range mount.Factors {
			if !mount.Template.OwnsFactor(factor.Scalar) || !factor.Capability.Available() {
				return nil, false
			}
			if _, duplicate := factors[factor.Scalar]; duplicate {
				return nil, false
			}
			if _, duplicate := seenFactors[factor.Capability]; duplicate {
				return nil, false
			}
			factors[factor.Scalar] = factor.Capability
			seenFactors[factor.Capability] = struct{}{}
		}
		for index := 0; index < mount.Template.FactorCount(); index++ {
			declared, declaredOK := mount.Template.FactorAt(index)
			capability, ok := factors[declared]
			if !declaredOK || !ok || !capability.Available() {
				return nil, false
			}
		}
		row := sealedProgramMount{template: mount.Template, capabilities: capabilities, factors: factors, module: mount.Module, state: mount.State}
		byArtifact[artifactID] = row
		sealed = append(sealed, row)
	}
	return sealed, true
}
