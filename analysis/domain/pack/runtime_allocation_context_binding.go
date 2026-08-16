package pack

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/program/keyspace"
)

// RuntimeAllocationContextBindingAvailability is the closed outcome of
// binding one Pack input source to one live runtime allocation context. The
// unavailable states are semantic facts about selector precision; they are
// not placement results and never authorize a fallback source.
type RuntimeAllocationContextBindingAvailability uint8

const (
	RuntimeAllocationContextBindingInvalid RuntimeAllocationContextBindingAvailability = iota
	RuntimeAllocationContextBindingBound
	RuntimeAllocationContextBindingUnavailableTail
	RuntimeAllocationContextBindingUnavailableWhole
	RuntimeAllocationContextBindingUnavailableUnknown
)

func (availability RuntimeAllocationContextBindingAvailability) Valid() bool {
	switch availability {
	case RuntimeAllocationContextBindingBound, RuntimeAllocationContextBindingUnavailableTail,
		RuntimeAllocationContextBindingUnavailableWhole, RuntimeAllocationContextBindingUnavailableUnknown:
		return true
	default:
		return false
	}
}

// RuntimeAllocationContextBindingIssuer is the plan-local join authority for
// Pack's exact fixed input source and Heap's live runtime context capability.
// It deliberately retains the two owner capabilities only while the plan is
// live; Result must retain neither this issuer nor the binding it issues.
//
// Heap's authority reissues the mounted receipt during admission, which is the
// generation fence: a context from another authority, including one with equal
// scalar content, cannot be attached here.
type RuntimeAllocationContextBindingIssuer struct {
	pack      *Schema
	heap      heap.Schema
	authority *heap.RuntimeAllocationContextAuthority
}

func (issuer RuntimeAllocationContextBindingIssuer) valid() bool {
	return issuer.pack != nil && issuer.pack.state != nil && issuer.heap.Valid() && issuer.authority != nil &&
		issuer.heap.OwnsRuntimeAllocationContextAuthority(issuer.authority) && issuer.heap.LinkOwner().Matches(issuer.pack.LinkOwner())
}

// NewRuntimeAllocationContextBindingIssuer joins exact Pack and Heap seals
// from one Link with one live runtime authority. It accepts no raw Link,
// module, context, or allocation IDs.
func NewRuntimeAllocationContextBindingIssuer(packSchema *Schema, heapSchema heap.Schema, authority *heap.RuntimeAllocationContextAuthority) (RuntimeAllocationContextBindingIssuer, bool) {
	issuer := RuntimeAllocationContextBindingIssuer{pack: packSchema, heap: heapSchema, authority: authority}
	return issuer, issuer.valid()
}

// RuntimeAllocationContextBinding is one owner-issued fixed-input/runtime
// binding. It identifies the selected mounted call input and exact live Heap
// allocation context without deciding escape, sharing, COW, or placement.
type RuntimeAllocationContextBinding struct {
	issuer   RuntimeAllocationContextBindingIssuer
	mounted  heap.MountedAllocationReceipt
	module   keyspace.ContentID
	call     keyspace.ContentID
	selector InputSelector
	source   SemanticSource
	id       keyspace.ContentID
}

func runtimeAllocationContextBindingID(linkID, mountedID, sourceModule, sourceID, callModule, callID keyspace.ContentID) keyspace.ContentID {
	if !linkID.Available() || !mountedID.Available() || !sourceModule.Available() || !sourceID.Available() || !callModule.Available() || !callID.Available() {
		return keyspace.ContentID{}
	}
	var payload [32 * 6]byte
	copy(payload[0:32], linkID[:])
	copy(payload[32:64], mountedID[:])
	copy(payload[64:96], sourceModule[:])
	copy(payload[96:128], sourceID[:])
	copy(payload[128:160], callModule[:])
	copy(payload[160:192], callID[:])
	return keyspace.ContentID(sha256.Sum256(payload[:]))
}

func (binding RuntimeAllocationContextBinding) valid() bool {
	if !binding.issuer.valid() || !binding.mounted.Valid() || !binding.module.Available() || !binding.call.Available() ||
		!binding.issuer.pack.OwnsInputSelector(binding.selector) || binding.selector.kind != inputSelectionScalar || !binding.source.Available() || !binding.id.Available() {
		return false
	}
	requirement, requirementOK := binding.mounted.Requirement()
	context, contextOK := binding.mounted.Context()
	if !requirementOK || !contextOK || requirement.HeapID() != binding.issuer.heap.ContentID() {
		return false
	}
	reissued, mountedOK := binding.issuer.authority.Mount(requirement, context)
	if !mountedOK || reissued.ID() != binding.mounted.ID() {
		return false
	}
	source, sourceOK := binding.issuer.pack.MountedInputSemanticSource(binding.module, binding.call, binding.selector)
	if !sourceOK || !source.Same(binding.source) || source.Module() != binding.module {
		return false
	}
	return binding.id == runtimeAllocationContextBindingID(binding.issuer.pack.LinkOwner().ContentID(), binding.mounted.ID(), binding.source.Module(), binding.source.ID(), binding.module, binding.call)
}

func (binding RuntimeAllocationContextBinding) Valid() bool { return binding.valid() }

// ID is the deterministic scalar identity of this exact mounted call/source
// and mounted allocation/context relation. It excludes private issuer
// generation so equal-content live bindings have equal semantic IDs.
func (binding RuntimeAllocationContextBinding) ID() keyspace.ContentID {
	if !binding.valid() {
		return keyspace.ContentID{}
	}
	return binding.id
}

func (binding RuntimeAllocationContextBinding) Source() (SemanticSource, bool) {
	return binding.source, binding.valid()
}

func (binding RuntimeAllocationContextBinding) MountedAllocation() (heap.MountedAllocationReceipt, bool) {
	return binding.mounted, binding.valid()
}

func (binding RuntimeAllocationContextBinding) CallProvenance() (module, call keyspace.ContentID, ok bool) {
	if !binding.valid() {
		return keyspace.ContentID{}, keyspace.ContentID{}, false
	}
	return binding.module, binding.call, true
}

// MatchesSelector proves that selector is the exact Pack selector bound into
// this receipt. It avoids exposing InputSelector's private owner fields to a
// later cross-domain correlation candidate.
func (binding RuntimeAllocationContextBinding) MatchesSelector(selector InputSelector) bool {
	return binding.valid() && binding.issuer.pack.OwnsInputSelector(selector) && binding.selector == selector
}

// IssuedByPack proves that this live binding came from schema's exact Pack
// seal. It is an owner fence, not a ContentID comparison, so an independently
// resealed equal-content Pack cannot enter a Value direct-identity join.
func (binding RuntimeAllocationContextBinding) IssuedByPack(schema *Schema) bool {
	return binding.valid() && schema != nil && binding.issuer.pack == schema
}

// MatchesAllocationKey proves that key is the exact Heap allocation root
// carried by this live binding's mounted requirement. The key stays owner
// issued; callers cannot substitute a raw key ID or an equal-content Heap
// coordinate from another seal.
func (binding RuntimeAllocationContextBinding) MatchesAllocationKey(key heap.Key) bool {
	if !binding.valid() || !binding.issuer.heap.OwnsKey(key) {
		return false
	}
	mounted, mountedOK := binding.MountedAllocation()
	requirement, requirementOK := mounted.Requirement()
	keyID, keyOK := key.ContentID()
	return mountedOK && requirementOK && keyOK && requirement.KeyID() == keyID
}

// SameRuntimeAllocationContextBindingIssuer proves that the subject-allocation
// and destination-context receipts came from one exact live Pack/Heap issuer.
// Semantic ContentIDs are intentionally insufficient: independently resealed
// equal-content Pack or Heap schemas, or a second runtime authority on the
// same Heap, must not be correlated through their matching scalar IDs.
func SameRuntimeAllocationContextBindingIssuer(subject RuntimeAllocationContextBinding, destination RuntimeDestinationContextBinding) bool {
	return subject.valid() && destination.valid() && subject.issuer.valid() && destination.issuer.valid() &&
		subject.issuer == destination.issuer
}

// BindRuntimeAllocationContext attaches a fixed Pack input to the exact Heap
// authority that issued its context. Tail and whole selectors are explicitly
// unavailable; they never receive a fabricated semantic source. A fixed
// selector whose mounted call source is absent is likewise unavailable.
func (issuer RuntimeAllocationContextBindingIssuer) BindRuntimeAllocationContext(module, callID keyspace.ContentID, selector InputSelector, requirement heap.AllocationRequirement, context heap.RuntimeAllocationContext) (RuntimeAllocationContextBinding, RuntimeAllocationContextBindingAvailability) {
	if !issuer.valid() || !module.Available() || !callID.Available() || !issuer.pack.OwnsInputSelector(selector) {
		return RuntimeAllocationContextBinding{}, RuntimeAllocationContextBindingInvalid
	}
	mounted, mountedOK := issuer.authority.Mount(requirement, context)
	if !mountedOK || requirement.HeapID() != issuer.heap.ContentID() {
		return RuntimeAllocationContextBinding{}, RuntimeAllocationContextBindingInvalid
	}
	switch selector.kind {
	case inputSelectionTail:
		return RuntimeAllocationContextBinding{}, RuntimeAllocationContextBindingUnavailableTail
	case inputSelectionWhole:
		return RuntimeAllocationContextBinding{}, RuntimeAllocationContextBindingUnavailableWhole
	case inputSelectionScalar:
		source, sourceOK := issuer.pack.MountedInputSemanticSource(module, callID, selector)
		if !sourceOK {
			return RuntimeAllocationContextBinding{}, RuntimeAllocationContextBindingUnavailableUnknown
		}
		id := runtimeAllocationContextBindingID(issuer.pack.LinkOwner().ContentID(), mounted.ID(), source.Module(), source.ID(), module, callID)
		binding := RuntimeAllocationContextBinding{issuer: issuer, mounted: mounted, module: module, call: callID, selector: selector, source: source, id: id}
		if !binding.valid() {
			return RuntimeAllocationContextBinding{}, RuntimeAllocationContextBindingInvalid
		}
		return binding, RuntimeAllocationContextBindingBound
	default:
		return RuntimeAllocationContextBinding{}, RuntimeAllocationContextBindingInvalid
	}
}

// RuntimeDestinationContextBinding is a separate fixed-input/runtime-context
// receipt. It deliberately has no AllocationRequirement or mounted allocation
// field: a destination context is a call semantic input, never evidence about
// the subject allocation's placement.
type RuntimeDestinationContextBinding struct {
	issuer   RuntimeAllocationContextBindingIssuer
	context  heap.RuntimeAllocationContext
	module   keyspace.ContentID
	call     keyspace.ContentID
	selector InputSelector
	source   SemanticSource
	id       keyspace.ContentID
}

// RuntimeDestinationContextBindingAbsent is the only accepted absent value for
// a destination-free publication transition. Callers must pass an explicit
// presence bit beside the binding; an invalid or stale non-zero capability is
// not interchangeable with absence.
func RuntimeDestinationContextBindingAbsent(binding RuntimeDestinationContextBinding) bool {
	return binding == (RuntimeDestinationContextBinding{})
}

func runtimeDestinationContextBindingID(linkID, contextID, sourceModule, sourceID, callModule, callID keyspace.ContentID, class heap.RuntimeAllocationContextClass) keyspace.ContentID {
	if !linkID.Available() || !contextID.Available() || !sourceModule.Available() || !sourceID.Available() || !callModule.Available() || !callID.Available() || !class.Valid() {
		return keyspace.ContentID{}
	}
	var payload [32*6 + 1]byte
	copy(payload[0:32], linkID[:])
	copy(payload[32:64], contextID[:])
	copy(payload[64:96], sourceModule[:])
	copy(payload[96:128], sourceID[:])
	copy(payload[128:160], callModule[:])
	copy(payload[160:192], callID[:])
	payload[192] = byte(class)
	return keyspace.ContentID(sha256.Sum256(payload[:]))
}

func (binding RuntimeDestinationContextBinding) valid() bool {
	if !binding.issuer.valid() || !binding.issuer.authority.OwnsRuntimeAllocationContext(binding.context) || !binding.module.Available() || !binding.call.Available() ||
		!binding.issuer.pack.OwnsInputSelector(binding.selector) || binding.selector.kind != inputSelectionScalar || !binding.source.Available() || !binding.id.Available() {
		return false
	}
	source, sourceOK := binding.issuer.pack.MountedInputSemanticSource(binding.module, binding.call, binding.selector)
	if !sourceOK || !source.Same(binding.source) || source.Module() != binding.module {
		return false
	}
	return binding.id == runtimeDestinationContextBindingID(binding.issuer.pack.LinkOwner().ContentID(), binding.context.ContextID(), binding.source.Module(), binding.source.ID(), binding.module, binding.call, binding.context.Class())
}

func (binding RuntimeDestinationContextBinding) Valid() bool { return binding.valid() }
func (binding RuntimeDestinationContextBinding) ID() keyspace.ContentID {
	if !binding.valid() {
		return keyspace.ContentID{}
	}
	return binding.id
}
func (binding RuntimeDestinationContextBinding) Source() (SemanticSource, bool) {
	return binding.source, binding.valid()
}
func (binding RuntimeDestinationContextBinding) Context() (heap.RuntimeAllocationContext, bool) {
	return binding.context, binding.valid()
}
func (binding RuntimeDestinationContextBinding) CallProvenance() (module, call keyspace.ContentID, ok bool) {
	if !binding.valid() {
		return keyspace.ContentID{}, keyspace.ContentID{}, false
	}
	return binding.module, binding.call, true
}
func (binding RuntimeDestinationContextBinding) MatchesSelector(selector InputSelector) bool {
	return binding.valid() && binding.issuer.pack.OwnsInputSelector(selector) && binding.selector == selector
}

// BindRuntimeDestinationContext binds only an exact fixed Pack input to the
// same live authority's destination context. It has no allocation parameter by
// design. Tail, whole and unavailable fixed sources remain explicit
// unavailable outcomes.
func (issuer RuntimeAllocationContextBindingIssuer) BindRuntimeDestinationContext(module, callID keyspace.ContentID, selector InputSelector, context heap.RuntimeAllocationContext) (RuntimeDestinationContextBinding, RuntimeAllocationContextBindingAvailability) {
	if !issuer.valid() || !module.Available() || !callID.Available() || !issuer.pack.OwnsInputSelector(selector) || !issuer.authority.OwnsRuntimeAllocationContext(context) {
		return RuntimeDestinationContextBinding{}, RuntimeAllocationContextBindingInvalid
	}
	switch selector.kind {
	case inputSelectionTail:
		return RuntimeDestinationContextBinding{}, RuntimeAllocationContextBindingUnavailableTail
	case inputSelectionWhole:
		return RuntimeDestinationContextBinding{}, RuntimeAllocationContextBindingUnavailableWhole
	case inputSelectionScalar:
		source, sourceOK := issuer.pack.MountedInputSemanticSource(module, callID, selector)
		if !sourceOK {
			return RuntimeDestinationContextBinding{}, RuntimeAllocationContextBindingUnavailableUnknown
		}
		id := runtimeDestinationContextBindingID(issuer.pack.LinkOwner().ContentID(), context.ContextID(), source.Module(), source.ID(), module, callID, context.Class())
		binding := RuntimeDestinationContextBinding{issuer: issuer, context: context, module: module, call: callID, selector: selector, source: source, id: id}
		if !binding.valid() {
			return RuntimeDestinationContextBinding{}, RuntimeAllocationContextBindingInvalid
		}
		return binding, RuntimeAllocationContextBindingBound
	default:
		return RuntimeDestinationContextBinding{}, RuntimeAllocationContextBindingInvalid
	}
}
