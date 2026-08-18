// runtime_program_construction.go is the first-construction attachment ledger
// and the public Solver mint. A construction binds one runtime, records every
// attached member, query and observation, and seals them once. Activation
// revisions never retain or replay this ledger.

package engine

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

// programConstruction is the private ledger of one first construction. It
// embeds the plane it attaches to so a bind reads the same Factor universe a
// later revision will rebind against.
type programConstruction struct {
	*programPlane
	mu             sync.Mutex
	members        map[composition.Key]runtimeMember
	queries        map[composition.Key]runtimeQuery
	observations   []runtimeObservation
	observationIDs map[identity.ContentID]struct{}
	closed         bool
}

// ProgramConstruction is the opaque first-construction handle. Callers attach
// typed members, queries and observations, then Seal the Solver. The handle
// cannot observe slots, callbacks, graph mutation, or carrier coordinates.
type ProgramConstruction struct {
	inner     *programConstruction
	graph     *ReceiptGraph
	addressed []composition.Key
}

// BeginProgramConstruction opens the first-construction ledger over one sealed
// binding and the graph that binding committed. The plane is bound here; attach
// writes into the ledger; Seal mints the Solver.
func BeginProgramConstruction(binding *SchemaBinding, graph *ReceiptGraph) (*ProgramConstruction, bool) {
	if binding == nil || graph == nil || !binding.Sealed() || !graph.valid() || graph.state != binding.state {
		return nil, false
	}
	addressed, addressedOK := graph.publishedQueryKeys()
	inner, ok := beginProgramConstruction(binding, graph.graph)
	if !addressedOK || !ok || inner == nil {
		return nil, false
	}
	return &ProgramConstruction{inner: inner, graph: graph, addressed: addressed}, true
}

// The direct constructor consumes this opaque graph view.  Keeping the
// ReceiptGraph projection here leaves the direct row binder independent of the
// compile-side graph declaration and gives the eventual geometry cut one
// narrow implementation point to remove.
func (receipt *ReceiptGraph) directProgramGraphValid(binding *SchemaBinding) bool {
	return receipt != nil && binding != nil && receipt.valid() && receipt.state == binding.state
}

func (receipt *ReceiptGraph) directProgramGraphState() *schemaBindingState {
	if receipt == nil {
		return nil
	}
	return receipt.state
}

func (receipt *ReceiptGraph) directProgramGraphValue() *equation.Graph {
	if receipt == nil {
		return nil
	}
	return receipt.graph
}

func (receipt *ReceiptGraph) directProgramTopology() *equation.Topology {
	if receipt == nil || receipt.topology == nil {
		return nil
	}
	return receipt.topology.topology
}

func (receipt *ReceiptGraph) directPublishedQueryKeys() ([]composition.Key, bool) {
	if receipt == nil {
		return nil, false
	}
	return receipt.publishedQueryKeys()
}

func (receipt *ReceiptGraph) directMountedRuleMember(role RuleSlotCapability, mount, point, occurrence identity.ContentID) (equation.RuleMember, bool) {
	if receipt == nil {
		return equation.RuleMember{}, false
	}
	row, ok := receipt.MountedRuleMember(role, mount, point, occurrence)
	if !ok {
		return equation.RuleMember{}, false
	}
	return row.member, true
}

func (receipt *ReceiptGraph) directLinkRuleMember(role RuleSlotCapability, occurrence identity.ContentID) (equation.RuleMember, bool) {
	if receipt == nil {
		return equation.RuleMember{}, false
	}
	row, ok := receipt.LinkRuleMember(role, occurrence)
	if !ok {
		return equation.RuleMember{}, false
	}
	return row.member, true
}

func (receipt *ReceiptGraph) directMountedActivationMember(role RuleSlotCapability, mount, point, occurrence identity.ContentID) (equation.RuleMember, bool) {
	if receipt == nil {
		return equation.RuleMember{}, false
	}
	row, ok := receipt.MountedActivationMember(role, mount, point, occurrence)
	if !ok {
		return equation.RuleMember{}, false
	}
	return row.member, true
}

func (receipt *ReceiptGraph) directQuery(id identity.ContentID) (equation.Query, bool) {
	if receipt == nil {
		return equation.Query{}, false
	}
	row, ok := receipt.Query(id)
	if !ok {
		return equation.Query{}, false
	}
	return row.identity, true
}

// beginProgramConstruction binds one plane and opens the attachment ledger
// against it. Same-package tests that exercise a raw graph use this entry;
// production goes through BeginProgramConstruction so the committed topology
// is the one Seal reads.
func beginProgramConstruction(binding *SchemaBinding, graph *equation.Graph) (*programConstruction, bool) {
	plane, ok := bindProgramPlane(bindingState(binding), graph)
	if !ok || plane == nil {
		return nil, false
	}
	return &programConstruction{
		programPlane:   plane,
		members:        make(map[composition.Key]runtimeMember),
		queries:        make(map[composition.Key]runtimeQuery),
		observationIDs: make(map[identity.ContentID]struct{}),
	}, true
}

// Close terminally closes this construction. Copies of the opaque handle share
// the same ledger and therefore cannot attach after close.
func (construction *ProgramConstruction) Close() bool {
	if construction == nil || construction.inner == nil {
		return false
	}
	construction.inner.mu.Lock()
	defer construction.inner.mu.Unlock()
	if construction.inner.closed {
		return false
	}
	construction.inner.closed = true
	return true
}

// Seal is the sole Solver constructor. It requires a structurally valid
// topology, total published query and observation tables, and a bound plane,
// then mints the Solver through mintProgramSolver. The construction is
// terminal: the ledger is released before this returns.
func (construction *ProgramConstruction) Seal() (*Solver, SolveFailure, bool) {
	if construction == nil || construction.inner == nil || construction.graph == nil || construction.graph.graph == nil || construction.graph.topology == nil || !construction.graph.topology.valid() {
		return nil, ProgramConstructionFailure(ProgramConstructionStageAdmission), false
	}
	topology := construction.graph.topology
	graph := construction.graph.graph
	inner := construction.inner
	inner.mu.Lock()
	if inner.closed || inner.programPlane == nil || inner.carrier == nil || inner.members == nil || inner.queries == nil || inner.observationIDs == nil {
		inner.mu.Unlock()
		return nil, ProgramConstructionFailure(ProgramConstructionStageAdmission), false
	}
	inner.closed = true
	drafts := make([]runtimeMember, 0, len(inner.members))
	for _, draft := range inner.members {
		if draft == nil {
			inner.mu.Unlock()
			return nil, ProgramConstructionFailure(ProgramConstructionStageMemberBind), false
		}
		drafts = append(drafts, draft)
	}
	queries, queriesOK := bindProgramQueryTable(construction.addressed, graph, inner.queries)
	if !queriesOK {
		inner.mu.Unlock()
		return nil, ProgramConstructionFailure(ProgramConstructionStageQueryAddress), false
	}
	observations, observationsOK := bindProgramObservationTable(inner.observations, len(inner.observationIDs))
	if !observationsOK {
		inner.mu.Unlock()
		return nil, ProgramConstructionFailure(ProgramConstructionStageObservationAddress), false
	}
	plane := inner.programPlane
	state, authority := plane.runtime.state, plane.runtime.authority
	inner.mu.Unlock()

	runtime, assembled := assembleReceiptRuntime(state, authority, graph, plane.carrier, plane.byKey, drafts, queries, observations)
	if !assembled || runtime == nil {
		return nil, ProgramConstructionFailure(ProgramConstructionStageProgramSeal), false
	}
	runtime.topology = topology.topology
	if !plane.releaseColdFactorBindings() {
		return nil, ProgramConstructionFailure(ProgramConstructionStageFactorBind), false
	}
	inner.mu.Lock()
	inner.programPlane = nil
	inner.members = nil
	inner.queries = nil
	inner.observations = nil
	inner.observationIDs = nil
	inner.mu.Unlock()
	solver, failure, minted := mintProgramSolver(runtime)
	if !minted {
		if failure.Available() {
			return nil, failure, false
		}
		return nil, ProgramConstructionFailure(ProgramConstructionStageSolverMint), false
	}
	return solver, SolveFailure{}, true
}

// AttachRuleMember attaches the member this construction's graph publishes
// under id. Lookup is graph-local: a foreign graph's handle is not an input.
func AttachRuleMember[K ~uint32 | ~uint64, V, O any](construction *ProgramConstruction, implementation *RuleImplementation[K, V, O], id identity.ContentID) bool {
	if construction == nil || construction.graph == nil || !id.Available() {
		return false
	}
	member, ok := construction.graph.RuleMember(id)
	if !ok {
		return false
	}
	operand, resolved := implementation.resolveOperand(OperandCoords{Member: id})
	if !resolved {
		return false
	}
	return attachResolvedRuleMember(construction, implementation, member.member, operand)
}

// HasMountedRuleMember reports whether this construction's committed graph
// publishes the authored mounted occurrence. Domain observation attach uses
// this instead of taking a ReceiptGraph.
func (construction *ProgramConstruction) HasMountedRuleMember(role RuleSlotCapability, mount, point, occurrence identity.ContentID) bool {
	if construction == nil || construction.graph == nil {
		return false
	}
	_, ok := construction.graph.MountedRuleMember(role, mount, point, occurrence)
	return ok
}

// MountedNativeCallStage resolves the exact native Call stage published by
// this construction's committed graph.
func (construction *ProgramConstruction) MountedNativeCallStage(role RuleSlotCapability, mount, occurrence identity.ContentID) (MountedNativeCallStageReceipt, bool) {
	if construction == nil || construction.graph == nil {
		return MountedNativeCallStageReceipt{}, false
	}
	return construction.graph.MountedNativeCallStage(role, mount, occurrence)
}

// AttachMountedRuleMember attaches the mounted occurrence this construction's
// graph publishes under the authored mount/point/occurrence coordinates.
func AttachMountedRuleMember[K ~uint32 | ~uint64, V, O any](construction *ProgramConstruction, implementation *RuleImplementation[K, V, O], role RuleSlotCapability, mount, point, occurrence identity.ContentID) bool {
	if construction == nil || construction.graph == nil {
		return false
	}
	member, ok := construction.graph.MountedRuleMember(role, mount, point, occurrence)
	if !ok {
		return false
	}
	operand, resolved := implementation.resolveOperand(OperandCoords{Mount: mount, Point: point, Occurrence: occurrence})
	if !resolved {
		return false
	}
	return attachResolvedRuleMember(construction, implementation, member.member, operand)
}

// AttachLinkRuleMember attaches the Link-global occurrence this construction's
// graph publishes under role and occurrence.
func AttachLinkRuleMember[K ~uint32 | ~uint64, V, O any](construction *ProgramConstruction, implementation *RuleImplementation[K, V, O], role RuleSlotCapability, occurrence identity.ContentID) bool {
	if construction == nil || construction.graph == nil {
		return false
	}
	member, ok := construction.graph.LinkRuleMember(role, occurrence)
	if !ok {
		return false
	}
	operand, resolved := implementation.resolveOperand(OperandCoords{Occurrence: occurrence})
	if !resolved {
		return false
	}
	return attachResolvedRuleMember(construction, implementation, member.member, operand)
}

func attachResolvedRuleMember[K ~uint32 | ~uint64, V, O any](construction *ProgramConstruction, implementation *RuleImplementation[K, V, O], member equation.RuleMember, operand O) bool {
	if construction == nil || construction.inner == nil {
		return false
	}
	construction.inner.mu.Lock()
	row, ok := attachProgramRuleMemberLocked(construction.inner, implementation, member, operand)
	if !ok || row == nil {
		construction.inner.mu.Unlock()
		return false
	}
	construction.inner.mu.Unlock()
	return true
}

func attachProgramQueryLocked(inner *programConstruction, key composition.Key, row runtimeQuery) bool {
	if inner == nil {
		return false
	}
	if inner.closed || inner.queries == nil || !key.Available() {
		return false
	}
	if _, duplicate := inner.queries[key]; duplicate {
		return false
	}
	inner.queries[key] = row
	return true
}

func attachResolvedQuery[V, R any](construction *ProgramConstruction, identity equation.Query, bind func(*programPlane, equation.Query) (runtimeQuery, bool)) bool {
	if construction == nil || construction.inner == nil || construction.graph == nil || !identity.Key().Available() {
		return false
	}
	construction.inner.mu.Lock()
	row, ok := bind(construction.inner.programPlane, identity)
	if !ok || !attachProgramQueryLocked(construction.inner, identity.Key(), row) {
		construction.inner.mu.Unlock()
		return false
	}
	construction.inner.mu.Unlock()
	return true
}

func AttachExactQuery[V, R any](construction *ProgramConstruction, implementation *ExactQueryImplementation[V, R], id identity.ContentID) bool {
	if construction == nil || construction.graph == nil || implementation == nil || !id.Available() {
		return false
	}
	query, ok := construction.graph.Query(id)
	if !ok {
		return false
	}
	return attachResolvedQuery[V, R](construction, query.identity, func(plane *programPlane, identity equation.Query) (runtimeQuery, bool) {
		return bindReceiptExactQueryRuntime[V, R](plane, implementation, identity)
	})
}

func AttachSummaryQuery[V, R any](construction *ProgramConstruction, implementation *SummaryQueryImplementation[V, R], id identity.ContentID) bool {
	if construction == nil || construction.graph == nil || implementation == nil || !id.Available() {
		return false
	}
	query, ok := construction.graph.Query(id)
	if !ok {
		return false
	}
	return attachResolvedQuery[V, R](construction, query.identity, func(plane *programPlane, identity equation.Query) (runtimeQuery, bool) {
		return bindReceiptSummaryQueryRuntime[V, R](plane, implementation, identity)
	})
}

// AttachActivationMember attaches the activation this construction's graph
// publishes under id.
func AttachActivationMember(construction *ProgramConstruction, implementation *ActivationRuleImplementation, id identity.ContentID) bool {
	if construction == nil || construction.graph == nil || !id.Available() {
		return false
	}
	member, ok := construction.graph.ActivationMember(id)
	if !ok {
		return false
	}
	return attachResolvedActivationMember(construction, implementation, member.member)
}

// AttachMountedActivationMember attaches the mounted activation this
// construction's graph publishes under the authored coordinates.
func AttachMountedActivationMember(construction *ProgramConstruction, implementation *ActivationRuleImplementation, role RuleSlotCapability, mount, point, occurrence identity.ContentID) bool {
	if construction == nil || construction.graph == nil {
		return false
	}
	member, ok := construction.graph.MountedActivationMember(role, mount, point, occurrence)
	if !ok {
		return false
	}
	return attachResolvedActivationMember(construction, implementation, member.member)
}

func attachResolvedActivationMember(construction *ProgramConstruction, implementation *ActivationRuleImplementation, member equation.RuleMember) bool {
	if construction == nil || construction.inner == nil || construction.graph == nil || construction.graph.topology == nil || implementation == nil || !implementation.receipt.valid() {
		return false
	}
	inner := construction.inner
	inner.mu.Lock()
	defer inner.mu.Unlock()
	if inner.closed || !inner.frozen || inner.runtime == nil || inner.runtime.mode != runtimeBindingReceipt || implementation.receipt.state != inner.runtime.state || implementation.receipt.authority != inner.runtime.authority || !construction.graph.graph.OwnsMember(member) || !member.Key().Available() {
		return false
	}
	if _, duplicate := inner.members[member.Key()]; duplicate {
		return false
	}
	row, ok := bindActivationMemberReceipt(member, implementation, construction.graph.topology.topology, member.Key(), construction.graph.graph, inner.byKey)
	if !ok || row == nil || row.member().Key() != member.Key() {
		return false
	}
	inner.members[member.Key()] = row
	return true
}

// attachProgramRuleMember consumes one cell-issued Rule implementation and one
// exact graph member. A member enters this ledger only through the Binding
// authority that already owns its output Factor implementation.
func attachProgramRuleMember[K ~uint32 | ~uint64, V, O any](construction *programConstruction, implementation *RuleImplementation[K, V, O], member equation.RuleMember, operand O) (runtimeMember, bool) {
	if construction == nil {
		return nil, false
	}
	construction.mu.Lock()
	defer construction.mu.Unlock()
	return attachProgramRuleMemberLocked(construction, implementation, member, operand)
}

func attachProgramRuleMemberLocked[K ~uint32 | ~uint64, V, O any](construction *programConstruction, implementation *RuleImplementation[K, V, O], member equation.RuleMember, operand O) (runtimeMember, bool) {
	if construction.closed || construction.members == nil {
		return nil, false
	}
	if _, duplicate := construction.members[member.Key()]; duplicate {
		return nil, false
	}
	row, ok := bindProgramRuleMember(construction.programPlane, implementation, member, operand)
	if !ok || row == nil {
		return nil, false
	}
	construction.members[member.Key()] = row
	return row, true
}
