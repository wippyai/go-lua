package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/identity"
)

// MountedProgramRole is one template role bound to this Link's mounted
// capability. Assemble seals the pair into the mount capability map.
type MountedProgramRole struct {
	Scalar     rows.ArtifactScalarRole
	Capability RuleSlotCapability
}

// MountedProgramArtifact is one sealed template plus this Link's mounted
// role capabilities and mount identity.
type MountedProgramArtifact struct {
	Template *rows.ArtifactScalarTemplate
	Roles    []MountedProgramRole
	Module   identity.ContentID
}

// ProgramAdmissionStage names which admission pass refused. The assembly
// handle stays inside AssembleMountedProgram.
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
	Attach     RuleProgramAttach
	Capability RuleSlotCapability
	Occurrence identity.ContentID
}

// MountedRuleAdmission is one mounted occurrence to admit.
type MountedRuleAdmission struct {
	Attach     RuleProgramAttach
	Capability RuleSlotCapability
	Mount      identity.ContentID
	Point      identity.ContentID
	Occurrence identity.ContentID
}

// programQueryAdmit is the erased mounted query row. Implementations live on
// the sealed query cells; the declaration pass states the row and the
// constructor resolves the Point it is anchored at.
type programQueryAdmit interface {
	declareMountedQuery(state *schemaBindingState, authority *schemaBindingAuthority, id, mount, point identity.ContentID) (declaredQueryRow, *ruleSummaryMapping, bool)
	bindConstruction(*ProgramConstruction, identity.ContentID) bool
}

// ProgramQueryAdmission is one mounted query row to admit.
type ProgramQueryAdmission struct {
	admit programQueryAdmit
	ID    identity.ContentID
	Mount identity.ContentID
	Point identity.ContentID
}

// NewSummaryQueryAdmission seals one summary query row.
func NewSummaryQueryAdmission[V, R any](implementation *SummaryQueryImplementation[V, R], id, mount, point identity.ContentID) (ProgramQueryAdmission, bool) {
	if implementation == nil || !id.Available() || !mount.Available() || !point.Available() {
		return ProgramQueryAdmission{}, false
	}
	return ProgramQueryAdmission{admit: implementation, ID: id, Mount: mount, Point: point}, true
}

// NewExactQueryAdmission seals one exact query row.
func NewExactQueryAdmission[V, R any](implementation *ExactQueryImplementation[V, R], id, mount, point identity.ContentID) (ProgramQueryAdmission, bool) {
	if implementation == nil || !id.Available() || !mount.Available() || !point.Available() {
		return ProgramQueryAdmission{}, false
	}
	return ProgramQueryAdmission{admit: implementation, ID: id, Mount: mount, Point: point}, true
}

// MountedProgramAdmission is the sealed admit inventory for one assemble.
type MountedProgramAdmission struct {
	Link       []LinkRuleAdmission
	Mounted    []MountedRuleAdmission
	Activation []MountedActivationAdmit
	Queries    []ProgramQueryAdmission
}

// sealedProgramMount is one template plus this Link's sealed role
// capabilities and mount identity. Assemble snapshots from this value.
type sealedProgramMount struct {
	template     *rows.ArtifactScalarTemplate
	capabilities map[rows.ArtifactScalarRole]RuleSlotCapability
	module       identity.ContentID
}

// CommittedProgram is the assemble-owned committed handle. Construction
// opens against the sealed equation graph and binding topology.
type CommittedProgram struct {
	graph     *equation.Graph
	topology  *BindingTopology
	state     *schemaBindingState
	authority *schemaBindingAuthority
}

func newCommittedProgram(graph *equation.Graph, topology *BindingTopology, state *schemaBindingState, authority *schemaBindingAuthority) *CommittedProgram {
	committed := &CommittedProgram{graph: graph, topology: topology, state: state, authority: authority}
	if !committed.valid() {
		return nil
	}
	return committed
}

func (committed *CommittedProgram) valid() bool {
	return committed != nil && committed.graph != nil && committed.topology != nil && committed.topology.valid() && committed.state != nil && committed.authority != nil && committed.state == committed.topology.state && committed.authority == committed.topology.authority && committed.state.schema != nil && committed.graph.OwnsComposition(committed.state.schema.cold) && committed.topology.topology.OwnsGraph(committed.graph)
}

// ReleaseArtifact drops the sealed declaration a committed topology was sealed
// from - its source Batch and topology spec - once the runtime is bound. The
// sealed equation Topology and the published directory remain the structural
// authority and stay addressable.
func (committed *CommittedProgram) ReleaseArtifact() bool {
	return committed.valid() && committed.topology.releaseArtifact()
}

func (committed *CommittedProgram) lookupPoint(id identity.ContentID) (equation.Point, bool) {
	if !committed.valid() || !id.Available() || committed.topology.directory == nil {
		return equation.Point{}, false
	}
	locator, found := committed.topology.directory.point(id)
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
	graph    *equation.Graph
	topology *BindingTopology
	identity equation.Query
	locator  equation.QueryRowLocator
	key      identity.ContentID
}

// PublicationKey is the snapshot row identity this query is sealed under.
func (query ProgramQuery) PublicationKey() (identity.ContentID, bool) {
	return query.key, query.key.Available()
}

// ProgramMember is the committed graph-owned rule member handle.
type ProgramMember struct {
	graph    *equation.Graph
	topology *BindingTopology
	member   equation.RuleMember
	locator  equation.RuleMemberRowLocator
}

func (member ProgramMember) ownedBy(graph *equation.Graph, topology *BindingTopology) bool {
	return member.graph != nil && member.topology != nil && graph != nil && topology != nil && member.graph == graph && member.topology == topology
}

func (committed *CommittedProgram) Query(id identity.ContentID) (ProgramQuery, bool) {
	if !committed.valid() || !id.Available() || committed.topology.directory == nil {
		return ProgramQuery{}, false
	}
	locator, found := committed.topology.directory.query(id)
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
	return ProgramQuery{graph: committed.graph, topology: committed.topology, identity: query, locator: locator, key: key}, true
}

func (committed *CommittedProgram) lookupRuleMember(id identity.ContentID) (ProgramMember, bool) {
	if !committed.valid() || !id.Available() || committed.topology.directory == nil {
		return ProgramMember{}, false
	}
	locator, found := committed.topology.directory.member(id)
	if !found {
		return ProgramMember{}, false
	}
	member, ok := locator.Resolve(committed.graph)
	if !ok || !committed.graph.OwnsMember(member) {
		return ProgramMember{}, false
	}
	return ProgramMember{graph: committed.graph, topology: committed.topology, member: member, locator: locator}, true
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

func (committed *CommittedProgram) LinkRuleMember(role RuleSlotCapability, occurrence identity.ContentID) (ProgramMember, bool) {
	if !role.link() || !committed.valid() || role.state != committed.state || role.authority != committed.authority || !committed.topology.bootstrapOwner.Available() || !committed.topology.bootstrapPoint.Available() {
		return ProgramMember{}, false
	}
	return committed.lookupRuleMember(linkRuleMemberID(role, committed.topology.bootstrapOwner, committed.topology.bootstrapPoint, occurrence))
}

func (committed *CommittedProgram) MountedNativeCallStage(role RuleSlotCapability, mount, occurrence identity.ContentID) (ProgramCallStage, bool) {
	if !committed.valid() || !role.mounted() || role.state != committed.state || role.authority != committed.authority || !mount.Available() || !occurrence.Available() || committed.topology.nativeCallStages == nil {
		return ProgramCallStage{}, false
	}
	key := artifactMountedRuleOccurrence{role: role, mount: mount, occurrence: occurrence}
	stage, ok := committed.topology.nativeCallStages[key]
	result := ProgramCallStage{receipt: mountedCallStage{graph: committed.graph, topology: committed.topology, key: key, stage: stage}}
	return result, ok && result.Available()
}

// ActivationMember is one exact activation member from the committed graph.
// It is distinct from ProgramMember because activation compilation needs the
// activation-row locator as well as the graph member.
type ActivationMember struct {
	graph    *equation.Graph
	topology *BindingTopology
	member   equation.RuleMember
	locator  equation.ActivationMemberRowLocator
}

func (committed *CommittedProgram) lookupActivationMember(id identity.ContentID) (ActivationMember, bool) {
	if !committed.valid() || !id.Available() || committed.topology.directory == nil {
		return ActivationMember{}, false
	}
	locator, found := committed.topology.directory.activation(id)
	if !found {
		return ActivationMember{}, false
	}
	member, ok := locator.Resolve(committed.graph)
	if !ok || !committed.graph.OwnsMember(member) {
		return ActivationMember{}, false
	}
	return ActivationMember{graph: committed.graph, topology: committed.topology, member: member, locator: locator}, true
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
	if !committed.valid() || committed.topology.directory == nil {
		return nil, false
	}
	directory := committed.topology.directory
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

// ProgramAssembleRefusal is the closed assemble refusal. Receipt failure
// types stay inside the engine; callers read stage and SolveFailure only.
type ProgramAssembleRefusal struct {
	stage        ProgramAdmissionStage
	lowering     receiptAssemblyFailure
	construction topologyConstructionRefusal
	seal         receiptSealFailure
}

func (refusal ProgramAssembleRefusal) Stage() ProgramAdmissionStage { return refusal.stage }

func (refusal ProgramAssembleRefusal) Lowered() bool {
	return refusal.lowering != receiptAssemblyFailureNone
}

func (refusal ProgramAssembleRefusal) LoweringFailure() SolveFailure {
	return refusal.lowering.Failure()
}

func (refusal ProgramAssembleRefusal) Seal() SolveFailure { return refusal.seal.Failure() }

func (refusal ProgramAssembleRefusal) ArtifactRowOrdinal() (uint32, bool) {
	if refusal.seal.Phase() != receiptSealFailureArtifactRows {
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

// Commit projects the geometry construction refusal onto the public failure
// vocabulary. A construction that published no geometry carries the stage it
// refused at; a construction that never ran carries nothing.
func (refusal ProgramAssembleRefusal) Commit() SolveFailure { return refusal.construction.Failure() }

// ScheduleRow is the ordinal of the composition-schedule row a refused
// construction stopped on. It is zero for every other refusal.
func (refusal ProgramAssembleRefusal) ScheduleRow() uint32 {
	if refusal.construction.Step() != topologyConstructionStepSchedule {
		return 0
	}
	return refusal.construction.Ordinal()
}

// ObservationAttachArguments is the closed observation-attach input refusal.
func ObservationAttachArguments() SolveFailure {
	return receiptObservationAttachFailureArguments.Failure()
}

// ObservationAttachPoint is the closed observation-attach point refusal.
func ObservationAttachPoint() SolveFailure {
	return receiptObservationAttachFailurePoint.Failure()
}

// ProgramBootstrap is the Link-lane witness assemble consumes. Callers supply
// owner, point, and capability catalogs; they never name the receipt witness.
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

// BeginMountedProgram opens one sealed-template construction transaction.
// Production assemble commits this same builder; tests that place owner-issued
// seed rows hold the open handle until Seal/Commit.
func BeginMountedProgram(binding *SchemaBinding, mounts []MountedProgramArtifact, bootstrap ProgramBootstrap) (*BindingTopologyBuilder, ProgramAssembleRefusal, bool) {
	if !bootstrap.witness.Available() {
		return nil, ProgramAssembleRefusal{lowering: receiptAssemblyFailureInput}, false
	}
	builder, lowering, ok := beginMountedProgramMounts(binding, mounts, bootstrap.witness)
	if !ok {
		return nil, ProgramAssembleRefusal{lowering: lowering}, false
	}
	return builder, ProgramAssembleRefusal{}, true
}

// AssembleMountedProgram snapshots sealed templates into one binding topology
// and commits the equation graph. Callers supply templates and role
// capabilities; they never mint a receipt row.
func AssembleMountedProgram(binding *SchemaBinding, mounts []MountedProgramArtifact, admission MountedProgramAdmission, bootstrap ProgramBootstrap) (*CommittedProgram, ProgramAssembleRefusal, bool) {
	if binding == nil || !binding.Sealed() {
		return nil, ProgramAssembleRefusal{lowering: receiptAssemblyFailureInput}, false
	}
	sealed, sealedOK := sealMountedProgramArtifacts(mounts)
	if !sealedOK {
		return nil, ProgramAssembleRefusal{lowering: receiptAssemblyFailureInput}, false
	}
	graph, topology, seal, stage, lowering, construction, committed := assembleSealedProgramMounts(binding, sealed, admission, bootstrap.witness)
	if !committed {
		return nil, ProgramAssembleRefusal{stage: stage, lowering: lowering, construction: construction, seal: seal}, false
	}
	program := newCommittedProgram(graph, topology, binding.state, binding.state.authority)
	if program == nil {
		return nil, ProgramAssembleRefusal{stage: stage, construction: construction, seal: seal}, false
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
			sealed = append(sealed, sealedProgramMount{template: existing.template, capabilities: existing.capabilities, module: mount.Module})
			continue
		}
		if len(mount.Roles) != mount.Template.RoleCount() {
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
		row := sealedProgramMount{template: mount.Template, capabilities: capabilities, module: mount.Module}
		byArtifact[artifactID] = row
		sealed = append(sealed, row)
	}
	return sealed, true
}
