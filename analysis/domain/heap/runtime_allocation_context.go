package heap

import (
	"crypto/sha256"
	"encoding/binary"
	"sync"
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
)

// RuntimeAllocationContextClass is the closed physical owner family selected
// by a runtime. It is deliberately not the semantic placement lattice: the
// latter remains domain/placement's Stack/OwnedHeap/SharedHeap authority.
// A context only qualifies an already-derived owned or shared placement with
// the concrete runtime owner that will materialize it.
type RuntimeAllocationContextClass uint8

const (
	RuntimeAllocationContextInvalid RuntimeAllocationContextClass = iota
	RuntimeAllocationContextProcess
	RuntimeAllocationContextActor
	RuntimeAllocationContextShared
	RuntimeAllocationContextThread
)

func (class RuntimeAllocationContextClass) Valid() bool {
	switch class {
	case RuntimeAllocationContextProcess, RuntimeAllocationContextActor, RuntimeAllocationContextShared, RuntimeAllocationContextThread:
		return true
	default:
		return false
	}
}

// RuntimeAllocationContextAuthority is the short-lived runtime issuer for
// one sealed Heap schema and one explicit runtime policy revision. Its private
// generation fence prevents a capability from another equal-content Heap seal,
// another policy revision, or a recreated runtime from being accepted, while
// scalar semantic IDs remain deterministic across equal-content issuers.
//
// It holds only Heap's sealed owner and scalar policy identity. It retains no
// Link, Program, actor, thread, or physical allocator object. Close revokes
// every context it issued; Plan owns that lifecycle when the runtime binding is
// integrated.
type RuntimeAllocationContextAuthority struct {
	mu         sync.RWMutex
	owner      *schema
	policyID   identity.ContentID
	generation *runtimeAllocationGeneration
	closed     bool
}

// runtimeAllocationGeneration is a private issuer epoch. Its non-zero payload
// is essential: Go is permitted to coalesce addresses of distinct zero-sized
// allocations, which would make a pointer-only fence forgeable. The serial is
// deliberately not part of any semantic ContentID; it exists only to fence
// live capability instances.
type runtimeAllocationGeneration struct{ serial uint64 }

var runtimeAllocationGenerationSerial atomic.Uint64

func newRuntimeAllocationGeneration() *runtimeAllocationGeneration {
	serial := runtimeAllocationGenerationSerial.Add(1)
	if serial == 0 {
		// uint64 wrap is not a semantic revision. Skip zero, which is reserved
		// as an invalid private epoch even though practical wrap is unreachable.
		serial = runtimeAllocationGenerationSerial.Add(1)
	}
	return &runtimeAllocationGeneration{serial: serial}
}

// BeginRuntimeAllocationContexts issues a runtime-local context authority.
// policyID is a runtime-owned, content-addressed policy/revision identity; an
// unavailable ID fails closed rather than silently selecting a default policy.
func (schema Schema) BeginRuntimeAllocationContexts(policyID identity.ContentID) (*RuntimeAllocationContextAuthority, bool) {
	if !schema.valid() || !policyID.Available() {
		return nil, false
	}
	return &RuntimeAllocationContextAuthority{owner: schema.owner, policyID: policyID, generation: newRuntimeAllocationGeneration()}, true
}

// OwnsRuntimeAllocationContextAuthority proves that this exact sealed Heap
// owner issued the live runtime authority. ContentID equality is deliberately
// insufficient: independent equal-content Heap seals have equal semantic IDs
// but must not exchange plan-local authority capabilities.
func (schema Schema) OwnsRuntimeAllocationContextAuthority(authority *RuntimeAllocationContextAuthority) bool {
	return schema.valid() && authority != nil && authority.live() && authority.owner == schema.owner &&
		authority.generation != nil && authority.generation.serial != 0
}

func (authority *RuntimeAllocationContextAuthority) live() bool {
	if authority == nil || authority.owner == nil || authority.generation == nil || authority.generation.serial == 0 || !authority.policyID.Available() {
		return false
	}
	authority.mu.RLock()
	defer authority.mu.RUnlock()
	return !authority.closed
}

// Close revokes the authority and every context it issued. It is idempotent so
// Plan.Close can release the runtime even along partial construction paths.
func (authority *RuntimeAllocationContextAuthority) Close() {
	if authority == nil {
		return
	}
	authority.mu.Lock()
	authority.closed = true
	authority.mu.Unlock()
}

// PolicyID identifies the runtime policy revision that issued this authority.
func (authority *RuntimeAllocationContextAuthority) PolicyID() identity.ContentID {
	if !authority.live() {
		return identity.ContentID{}
	}
	return authority.policyID
}

// RuntimeAllocationOwner is a typed runtime-issued isolation capability. The
// external runtime may name its process, actor, shared pool, or thread with a
// stable ContentID, but no raw ID is accepted by context issuance. That keeps
// a caller from reclassifying an actor ID as a thread or splicing an owner from
// another runtime authority.
type RuntimeAllocationOwner struct {
	authority  *RuntimeAllocationContextAuthority
	generation *runtimeAllocationGeneration
	class      RuntimeAllocationContextClass
	id         identity.ContentID
	sealed     identity.ContentID
}

func runtimeAllocationOwnerID(owner *schema, policy identity.ContentID, class RuntimeAllocationContextClass, id identity.ContentID) identity.ContentID {
	if owner == nil || !owner.id.Available() || !policy.Available() || !class.Valid() || !id.Available() {
		return identity.ContentID{}
	}
	var payload [104]byte
	copy(payload[:32], owner.id[:])
	copy(payload[32:64], policy[:])
	copy(payload[64:96], id[:])
	binary.BigEndian.PutUint64(payload[96:], uint64(class))
	return identity.ContentID(sha256.Sum256(payload[:]))
}

func (owner RuntimeAllocationOwner) valid() bool {
	return owner.authority != nil && owner.authority.live() && owner.generation != nil && owner.generation == owner.authority.generation &&
		owner.class.Valid() && owner.id.Available() && owner.sealed == runtimeAllocationOwnerID(owner.authority.owner, owner.authority.policyID, owner.class, owner.id)
}

func (authority *RuntimeAllocationContextAuthority) issueOwner(class RuntimeAllocationContextClass, id identity.ContentID) (RuntimeAllocationOwner, bool) {
	if !authority.live() || !class.Valid() || !id.Available() {
		return RuntimeAllocationOwner{}, false
	}
	owner := RuntimeAllocationOwner{authority: authority, generation: authority.generation, class: class, id: id,
		sealed: runtimeAllocationOwnerID(authority.owner, authority.policyID, class, id)}
	return owner, owner.valid()
}

func (authority *RuntimeAllocationContextAuthority) ProcessOwner(id identity.ContentID) (RuntimeAllocationOwner, bool) {
	return authority.issueOwner(RuntimeAllocationContextProcess, id)
}
func (authority *RuntimeAllocationContextAuthority) ActorOwner(id identity.ContentID) (RuntimeAllocationOwner, bool) {
	return authority.issueOwner(RuntimeAllocationContextActor, id)
}
func (authority *RuntimeAllocationContextAuthority) SharedOwner(id identity.ContentID) (RuntimeAllocationOwner, bool) {
	return authority.issueOwner(RuntimeAllocationContextShared, id)
}
func (authority *RuntimeAllocationContextAuthority) ThreadOwner(id identity.ContentID) (RuntimeAllocationOwner, bool) {
	return authority.issueOwner(RuntimeAllocationContextThread, id)
}

func (owner RuntimeAllocationOwner) ID() identity.ContentID {
	if !owner.valid() {
		return identity.ContentID{}
	}
	return owner.id
}
func (owner RuntimeAllocationOwner) Class() RuntimeAllocationContextClass {
	if !owner.valid() {
		return RuntimeAllocationContextInvalid
	}
	return owner.class
}

// SharedAllocationAuthorization is an explicit, owner-fenced authorization to
// issue a shared runtime context. A raw ContentID is intentionally insufficient
// to make a shared context: it must first be admitted by this exact authority.
type SharedAllocationAuthorization struct {
	authority  *RuntimeAllocationContextAuthority
	generation *runtimeAllocationGeneration
	id         identity.ContentID
	sealed     identity.ContentID
}

func (authorization SharedAllocationAuthorization) valid() bool {
	if authorization.authority == nil || !authorization.authority.live() || authorization.generation == nil || authorization.generation != authorization.authority.generation || !authorization.id.Available() || !authorization.sealed.Available() {
		return false
	}
	return authorization.sealed == sharedAllocationAuthorizationID(authorization.authority.owner, authorization.authority.policyID, authorization.id)
}

func sharedAllocationAuthorizationID(owner *schema, policy, authorization identity.ContentID) identity.ContentID {
	if owner == nil || !owner.id.Available() || !policy.Available() || !authorization.Available() {
		return identity.ContentID{}
	}
	var payload [96]byte
	copy(payload[:32], owner.id[:])
	copy(payload[32:64], policy[:])
	copy(payload[64:96], authorization[:])
	return identity.ContentID(sha256.Sum256(payload[:]))
}

// AuthorizeShared admits a runtime-owned sharing policy identity. It does not
// infer authorization from an allocation form, effect name, or actor label.
func (authority *RuntimeAllocationContextAuthority) AuthorizeShared(id identity.ContentID) (SharedAllocationAuthorization, bool) {
	if !authority.live() || !id.Available() {
		return SharedAllocationAuthorization{}, false
	}
	authorization := SharedAllocationAuthorization{authority: authority, generation: authority.generation, id: id, sealed: sharedAllocationAuthorizationID(authority.owner, authority.policyID, id)}
	return authorization, authorization.valid()
}

// RuntimeAllocationContext is an opaque owner-issued physical allocation
// context. Its isolation owner is itself a typed capability: process, actor,
// and thread identities cannot be retagged or supplied as arbitrary scalar
// IDs. Shared contexts additionally carry the exact sharing authorization
// that admitted them.
type RuntimeAllocationContext struct {
	authority  *RuntimeAllocationContextAuthority
	generation *runtimeAllocationGeneration
	owner      RuntimeAllocationOwner
	sharedBy   identity.ContentID
	id         identity.ContentID
}

func (context RuntimeAllocationContext) valid() bool {
	if context.authority == nil || !context.authority.live() || context.generation == nil || context.generation != context.authority.generation ||
		!context.owner.valid() || context.owner.authority != context.authority || !context.id.Available() {
		return false
	}
	if context.owner.class == RuntimeAllocationContextShared {
		return context.sharedBy.Available() && context.id == runtimeAllocationContextID(context.authority.owner, context.authority.policyID, context.owner.class, context.owner.id, context.sharedBy)
	}
	return !context.sharedBy.Available() && context.id == runtimeAllocationContextID(context.authority.owner, context.authority.policyID, context.owner.class, context.owner.id, identity.ContentID{})
}

// Valid reports whether this exact runtime capability remains live.
func (context RuntimeAllocationContext) Valid() bool { return context.valid() }

func runtimeAllocationContextID(owner *schema, policy identity.ContentID, class RuntimeAllocationContextClass, isolation, shared identity.ContentID) identity.ContentID {
	if owner == nil || !owner.id.Available() || !policy.Available() || !class.Valid() || !isolation.Available() ||
		class == RuntimeAllocationContextShared && !shared.Available() ||
		class != RuntimeAllocationContextShared && shared.Available() {
		return identity.ContentID{}
	}
	var payload [32*4 + 8]byte
	copy(payload[0:32], owner.id[:])
	copy(payload[32:64], policy[:])
	copy(payload[64:96], isolation[:])
	copy(payload[96:128], shared[:])
	binary.BigEndian.PutUint64(payload[128:], uint64(class))
	return identity.ContentID(sha256.Sum256(payload[:]))
}

func (authority *RuntimeAllocationContextAuthority) issue(owner RuntimeAllocationOwner, shared identity.ContentID) (RuntimeAllocationContext, bool) {
	if !authority.live() || !owner.valid() || owner.authority != authority || owner.generation != authority.generation {
		return RuntimeAllocationContext{}, false
	}
	id := runtimeAllocationContextID(authority.owner, authority.policyID, owner.class, owner.id, shared)
	context := RuntimeAllocationContext{authority: authority, generation: authority.generation, owner: owner, sharedBy: shared, id: id}
	return context, context.valid()
}

// Process issues one process-owned allocation context.
func (authority *RuntimeAllocationContextAuthority) Process(owner RuntimeAllocationOwner) (RuntimeAllocationContext, bool) {
	if !owner.valid() || owner.class != RuntimeAllocationContextProcess {
		return RuntimeAllocationContext{}, false
	}
	return authority.issue(owner, identity.ContentID{})
}

// Actor issues one actor-owned allocation context. The caller supplies an
// opaque runtime actor identity; Heap never reads actor names or Link topology.
func (authority *RuntimeAllocationContextAuthority) Actor(owner RuntimeAllocationOwner) (RuntimeAllocationContext, bool) {
	if !owner.valid() || owner.class != RuntimeAllocationContextActor {
		return RuntimeAllocationContext{}, false
	}
	return authority.issue(owner, identity.ContentID{})
}

// Thread issues one thread-owned allocation context. A thread identity is not
// inferred from Target's FreshThread runtime kind: ownership is a runtime fact.
func (authority *RuntimeAllocationContextAuthority) Thread(owner RuntimeAllocationOwner) (RuntimeAllocationContext, bool) {
	if !owner.valid() || owner.class != RuntimeAllocationContextThread {
		return RuntimeAllocationContext{}, false
	}
	return authority.issue(owner, identity.ContentID{})
}

// Shared issues one explicitly authorized shared context. It rejects a valid
// authorization from another runtime authority or a revoked authority.
func (authority *RuntimeAllocationContextAuthority) Shared(owner RuntimeAllocationOwner, authorization SharedAllocationAuthorization) (RuntimeAllocationContext, bool) {
	if !authority.live() || !owner.valid() || owner.class != RuntimeAllocationContextShared || !authorization.valid() || authorization.authority != authority || authorization.generation != authority.generation {
		return RuntimeAllocationContext{}, false
	}
	return authority.issue(owner, authorization.id)
}

func (context RuntimeAllocationContext) Class() RuntimeAllocationContextClass {
	if !context.valid() {
		return RuntimeAllocationContextInvalid
	}
	return context.owner.class
}

func (context RuntimeAllocationContext) IsolationOwnerID() identity.ContentID {
	if !context.valid() {
		return identity.ContentID{}
	}
	return context.owner.id
}

func (context RuntimeAllocationContext) SharedAuthorizationID() identity.ContentID {
	if !context.valid() || context.owner.class != RuntimeAllocationContextShared {
		return identity.ContentID{}
	}
	return context.sharedBy
}

// ContextID is deterministic for the sealed Heap schema, runtime policy,
// context class, owner, and (for shared) explicit authorization.
func (context RuntimeAllocationContext) ContextID() identity.ContentID {
	if !context.valid() {
		return identity.ContentID{}
	}
	return context.id
}

// AllocationRequirement is Heap's symbolic allocation requirement. It names
// an artifact allocation and its mounted substitution, but no runtime context
// or physical object. ProgramArtifact therefore remains reusable across every
// process, actor, shared, and thread context.
type AllocationRequirement struct {
	heapID     identity.ContentID
	keyID      identity.ContentID
	artifactID identity.ContentID
	mount      identity.ContentID
	program    identity.ContentID
	allocation identity.ContentID
	localID    identity.ContentID
	kind       AllocationKind
	form       program.AllocationForm
	id         identity.ContentID
}

func (requirement AllocationRequirement) valid() bool {
	if !requirement.heapID.Available() || !requirement.keyID.Available() || !requirement.artifactID.Available() ||
		!requirement.mount.Available() || !requirement.program.Available() || !requirement.allocation.Available() ||
		!requirement.localID.Available() || !requirement.kind.Valid() || !requirement.form.Valid() || !requirement.id.Available() {
		return false
	}
	return requirement.id == allocationRequirementID(requirement.heapID, requirement.keyID, requirement.artifactID, requirement.mount,
		requirement.program, requirement.allocation, requirement.localID, requirement.kind, requirement.form)
}

func allocationRequirementID(heapID, keyID, artifactID, mount, programID, allocation, localID identity.ContentID, kind AllocationKind, form program.AllocationForm) identity.ContentID {
	if !heapID.Available() || !keyID.Available() || !artifactID.Available() || !mount.Available() || !programID.Available() || !allocation.Available() || !localID.Available() || !kind.Valid() || !form.Valid() {
		return identity.ContentID{}
	}
	var payload [32*7 + 16]byte
	copy(payload[0:32], heapID[:])
	copy(payload[32:64], keyID[:])
	copy(payload[64:96], artifactID[:])
	copy(payload[96:128], mount[:])
	copy(payload[128:160], programID[:])
	copy(payload[160:192], allocation[:])
	copy(payload[192:224], localID[:])
	binary.BigEndian.PutUint64(payload[224:232], uint64(kind))
	binary.BigEndian.PutUint64(payload[232:240], uint64(form))
	return identity.ContentID(sha256.Sum256(payload[:]))
}

// AllocationRequirementForKey projects an already-sealed Heap allocation root
// into its one symbolic reusable requirement. Every pointer-bearing Heap Key
// and Artifact receipt is authenticated here and then discarded: the returned
// requirement retains only scalar semantic identities, never a Schema, Key,
// Artifact, Link, Program, or runtime capability.
func (schema Schema) AllocationRequirementForKey(key Key) (AllocationRequirement, bool) {
	if !schema.valid() || !schema.OwnsKey(key) || key.Kind() != RootAllocation {
		return AllocationRequirement{}, false
	}
	receipt, ok := key.AllocationReceipt()
	if !ok {
		return AllocationRequirement{}, false
	}
	keyID, keyOK := schema.KeyID(key)
	localID, localOK := schema.AllocationRootValueID(key)
	artifactID := receipt.artifact.ID()
	if !keyOK || !localOK || !artifactID.Available() {
		return AllocationRequirement{}, false
	}
	id := allocationRequirementID(schema.id(), keyID, artifactID, receipt.Module(), receipt.ProgramID(), receipt.AllocationID(), localID, receipt.Kind(), receipt.Form())
	requirement := AllocationRequirement{heapID: schema.id(), keyID: keyID, artifactID: artifactID, mount: receipt.Module(), program: receipt.ProgramID(), allocation: receipt.AllocationID(), localID: localID, kind: receipt.Kind(), form: receipt.Form(), id: id}
	return requirement, requirement.valid()
}

func (schema Schema) id() identity.ContentID { return schema.ContentID() }

func (requirement AllocationRequirement) Valid() bool { return requirement.valid() }
func (requirement AllocationRequirement) ID() identity.ContentID {
	if !requirement.valid() {
		return identity.ContentID{}
	}
	return requirement.id
}
func (requirement AllocationRequirement) HeapID() identity.ContentID {
	if !requirement.valid() {
		return identity.ContentID{}
	}
	return requirement.heapID
}
func (requirement AllocationRequirement) KeyID() identity.ContentID {
	if !requirement.valid() {
		return identity.ContentID{}
	}
	return requirement.keyID
}
func (requirement AllocationRequirement) ArtifactID() identity.ContentID {
	if !requirement.valid() {
		return identity.ContentID{}
	}
	return requirement.artifactID
}
func (requirement AllocationRequirement) AllocationID() identity.ContentID {
	if !requirement.valid() {
		return identity.ContentID{}
	}
	return requirement.allocation
}
func (requirement AllocationRequirement) ProgramID() identity.ContentID {
	if !requirement.valid() {
		return identity.ContentID{}
	}
	return requirement.program
}
func (requirement AllocationRequirement) MountID() identity.ContentID {
	if !requirement.valid() {
		return identity.ContentID{}
	}
	return requirement.mount
}
func (requirement AllocationRequirement) LocalID() identity.ContentID {
	if !requirement.valid() {
		return identity.ContentID{}
	}
	return requirement.localID
}
func (requirement AllocationRequirement) Kind() AllocationKind {
	if !requirement.valid() {
		return AllocationInvalid
	}
	return requirement.kind
}
func (requirement AllocationRequirement) Form() program.AllocationForm {
	if !requirement.valid() {
		return program.AllocationFormInvalid
	}
	return requirement.form
}

// MountedAllocationReceipt is the owner-fenced runtime binding of a symbolic
// allocation requirement to one live runtime context. It is not a solved
// placement fact: callers must obtain a later semantic proof before publishing
// a Placement result.
type MountedAllocationReceipt struct {
	requirement AllocationRequirement
	context     RuntimeAllocationContext
	id          identity.ContentID
}

func mountedAllocationID(requirement, context identity.ContentID) identity.ContentID {
	if !requirement.Available() || !context.Available() {
		return identity.ContentID{}
	}
	var payload [72]byte
	copy(payload[:32], requirement[:])
	copy(payload[32:64], context[:])
	binary.BigEndian.PutUint64(payload[64:], 0x686561702d6d6f75) // heap-mou
	return identity.ContentID(sha256.Sum256(payload[:]))
}

func (receipt MountedAllocationReceipt) valid() bool {
	return receipt.requirement.valid() && receipt.context.valid() && receipt.requirement.heapID == receipt.context.authority.owner.id &&
		receipt.id == mountedAllocationID(receipt.requirement.id, receipt.context.id)
}

// Mount binds a symbolic allocation to a context without recompiling or
// reopening its ProgramArtifact. Foreign, stale, or spliced inputs fail.
func (authority *RuntimeAllocationContextAuthority) Mount(requirement AllocationRequirement, context RuntimeAllocationContext) (MountedAllocationReceipt, bool) {
	if !authority.live() || !requirement.valid() || !context.valid() || context.authority != authority || requirement.heapID != authority.owner.id {
		return MountedAllocationReceipt{}, false
	}
	receipt := MountedAllocationReceipt{requirement: requirement, context: context, id: mountedAllocationID(requirement.id, context.id)}
	return receipt, receipt.valid()
}

// OwnsRuntimeAllocationContext proves that this exact live authority issued
// context. Unlike Mount it needs no allocation requirement, so Pack can bind
// a destination runtime context without accidentally turning it into a
// subject-allocation placement receipt.
func (authority *RuntimeAllocationContextAuthority) OwnsRuntimeAllocationContext(context RuntimeAllocationContext) bool {
	return authority.live() && context.valid() && context.authority == authority && context.generation == authority.generation &&
		context.owner.authority == authority && context.owner.generation == authority.generation
}

func (receipt MountedAllocationReceipt) Valid() bool { return receipt.valid() }
func (receipt MountedAllocationReceipt) ID() identity.ContentID {
	if !receipt.valid() {
		return identity.ContentID{}
	}
	return receipt.id
}
func (receipt MountedAllocationReceipt) Requirement() (AllocationRequirement, bool) {
	if !receipt.valid() {
		return AllocationRequirement{}, false
	}
	return receipt.requirement, true
}
func (receipt MountedAllocationReceipt) Context() (RuntimeAllocationContext, bool) {
	if !receipt.valid() {
		return RuntimeAllocationContext{}, false
	}
	return receipt.context, true
}

// PlacementAvailability is the closed availability state for the placement
// plane. It is not the domain/placement lattice: availability states explain
// why no lattice fact may be published at all.
type PlacementAvailability uint8

const (
	PlacementAvailabilityInvalid PlacementAvailability = iota
	// PlacementFactorsUnbound means Phase 2 has no converged closed transfer
	// joining Value allocation identity, Heap frozen/shape, Residence lifetime,
	// Footprint substitution, and the sole Effect transition. In particular,
	// alias/uniqueness, escape, mutability, sharing/COW authorization,
	// lifetime, publication and invalidation are absent, so no placement class
	// can be inferred from allocation geometry or runtime context.
	PlacementFactorsUnbound
)

func (availability PlacementAvailability) Valid() bool {
	return availability == PlacementFactorsUnbound
}

// PlacementUnavailable records the only lawful current outcome for a mounted
// requirement while the semantic placement factors are unbound. It is
// intentionally not a Result placement row: the corpus oracle must report
// Unsupported instead of treating structural allocation geometry as a solved
// memory decision.
type PlacementUnavailable struct {
	mounted MountedAllocationReceipt
	state   PlacementAvailability
}

func (authority *RuntimeAllocationContextAuthority) Unavailable(mounted MountedAllocationReceipt) (PlacementUnavailable, bool) {
	if !authority.live() || !mounted.valid() || mounted.context.authority != authority {
		return PlacementUnavailable{}, false
	}
	return PlacementUnavailable{mounted: mounted, state: PlacementFactorsUnbound}, true
}

func (unavailable PlacementUnavailable) Valid() bool {
	return unavailable.mounted.valid() && unavailable.state.Valid()
}
func (unavailable PlacementUnavailable) Availability() PlacementAvailability {
	if !unavailable.Valid() {
		return PlacementAvailabilityInvalid
	}
	return unavailable.state
}
func (unavailable PlacementUnavailable) MountedID() identity.ContentID {
	if !unavailable.Valid() {
		return identity.ContentID{}
	}
	return unavailable.mounted.ID()
}
