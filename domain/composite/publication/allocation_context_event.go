package publication

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target"
	callsite "github.com/wippyai/go-lua/domain/effect/callsite"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

const allocationContextEventDomain = "analysis/publication-allocation-context-event/v1"

// AllocationRuntimeContext is a detached description of one exact
// runtime-issued physical context. It is deliberately not Residence.Location
// or placement.Placement: the class only records the runtime context that was
// bound while its short-lived authority was live.
type AllocationRuntimeContext struct {
	id          identity.ContentID
	class       heapdomain.RuntimeAllocationContextClass
	isolation   identity.ContentID
	sharedBy    identity.ContentID
	hasSharedBy bool
}

func allocationRuntimeContextFor(context heapdomain.RuntimeAllocationContext) (AllocationRuntimeContext, bool) {
	if !context.Valid() {
		return AllocationRuntimeContext{}, false
	}
	result := AllocationRuntimeContext{
		id:        context.ContextID(),
		class:     context.Class(),
		isolation: context.IsolationOwnerID(),
	}
	if result.class == heapdomain.RuntimeAllocationContextShared {
		result.sharedBy = context.SharedAuthorizationID()
		result.hasSharedBy = true
	}
	return result, result.valid()
}

func (context AllocationRuntimeContext) valid() bool {
	if !context.id.Available() || !context.class.Valid() || !context.isolation.Available() {
		return false
	}
	if context.class == heapdomain.RuntimeAllocationContextShared {
		return context.hasSharedBy && context.sharedBy.Available()
	}
	return !context.hasSharedBy && !context.sharedBy.Available()
}

func (context AllocationRuntimeContext) Valid() bool { return context.valid() }

func (context AllocationRuntimeContext) ContextID() identity.ContentID { return context.id }

func (context AllocationRuntimeContext) Class() heapdomain.RuntimeAllocationContextClass {
	return context.class
}

func (context AllocationRuntimeContext) IsolationOwnerID() identity.ContentID {
	return context.isolation
}

// SharedAuthorizationID reports the authorization only for the Shared class.
// Every other class carries none, which is the class distinction itself rather
// than an absent field.
func (context AllocationRuntimeContext) SharedAuthorizationID() (identity.ContentID, bool) {
	if !context.hasSharedBy {
		return identity.ContentID{}, false
	}
	return context.sharedBy, context.sharedBy.Available()
}

// AllocationContextEvent is the detached record of one proved publication
// transition whose subject was the exact direct allocation observed at the
// selected call. It retains scalar identities and typed transition/context
// vocabulary only. In particular, it contains no Solver, State, Plan, domain
// owner, runtime capability, Residence fact, Footprint graph, placement class,
// alias claim, or lifetime-survival claim.
type AllocationContextEvent struct {
	id identity.ContentID

	transition       identity.ContentID
	correlation      identity.ContentID
	directAdmission  identity.ContentID
	direct           identity.ContentID
	membershipProof  identity.ContentID
	membershipAttach identity.ContentID

	mount                identity.ContentID
	call                 identity.ContentID
	descriptor           identity.ContentID
	descriptorOccurrence identity.ContentID

	subjectBinding     identity.ContentID
	requirement        identity.ContentID
	mountedAllocation  identity.ContentID
	allocationKey      identity.ContentID
	membership         valuedomain.AllocationMembership
	subjectContext     AllocationRuntimeContext
	destinationBinding identity.ContentID
	destinationContext AllocationRuntimeContext
	hasDestination     bool

	kind             target.PublicationEffectKind
	escape           target.PublicationEscapeDisposition
	mutability       target.PublicationMutabilityDisposition
	declaredLifetime target.PublicationLifetimeDisposition
}

// NewAllocationContextEvent is the only issuance path. It rebuilds the two
// existing cross-owner admissions from live typed capabilities and reruns the
// Phase3D observation proof. A caller therefore cannot provide a detached
// membership scalar, context ID, Heap key, or target consequence.
func NewAllocationContextEvent(
	attachment DirectAllocationMembershipAttachment,
	solver *engine.Solver,
	state *engine.State,
	transition callsite.PublicationTransitionProof,
	correlation callsite.PublicationPlacementCorrelationCandidate,
	directAdmission callsite.PublicationDirectAllocationSubject,
	subject packdomain.RuntimeAllocationContextBinding,
	direct valuedomain.DirectAllocationSubject,
	destination packdomain.RuntimeDestinationContextBinding,
	destinationPresent bool,
) (AllocationContextEvent, bool) {
	if !transition.MatchesCompletion(solver, state) || !correlation.Valid() || !directAdmission.Valid() || !subject.Valid() || !direct.Valid() {
		return AllocationContextEvent{}, false
	}

	rebuiltCorrelation, correlationOK := callsite.NewPublicationPlacementCorrelationCandidate(transition, subject, destination, destinationPresent)
	rebuiltCorrelationID, rebuiltCorrelationIDOK := rebuiltCorrelation.ContentID()
	correlationID, correlationIDOK := correlation.ContentID()
	if !correlationOK || !rebuiltCorrelationIDOK || !correlationIDOK || rebuiltCorrelationID != correlationID {
		return AllocationContextEvent{}, false
	}
	rebuiltDirectAdmission, admissionOK := callsite.NewPublicationDirectAllocationSubject(rebuiltCorrelation, subject, direct)
	rebuiltAdmissionID, rebuiltAdmissionIDOK := rebuiltDirectAdmission.ContentID()
	directAdmissionID, directAdmissionIDOK := directAdmission.ContentID()
	if !admissionOK || !rebuiltAdmissionIDOK || !directAdmissionIDOK || rebuiltAdmissionID != directAdmissionID {
		return AllocationContextEvent{}, false
	}

	membershipProof, membershipOK := attachment.Prove(solver, state, transition, rebuiltCorrelation, subject, direct)
	if !membershipOK || !membershipProof.valid() || membershipProof.correlation != rebuiltCorrelationID || membershipProof.membership != valuedomain.MembershipRecent && membershipProof.membership != valuedomain.MembershipSummary {
		return AllocationContextEvent{}, false
	}

	transitionID, transitionIDOK := transition.ContentID()
	descriptorID, descriptorIDOK := transition.DescriptorID()
	descriptorOccurrence, descriptorOccurrenceOK := transition.OccurrenceID()
	directID, directIDOK := direct.ContentID()
	mount, call := transition.MountID(), transition.CallOccurrenceID()
	subjectMount, subjectCall, subjectProvenanceOK := subject.CallProvenance()
	mounted, mountedOK := subject.MountedAllocation()
	requirement, requirementOK := mounted.Requirement()
	subjectRuntime, subjectContextOK := mounted.Context()
	subjectContext, subjectContextEvidenceOK := allocationRuntimeContextFor(subjectRuntime)
	if !transitionIDOK || !descriptorIDOK || !descriptorOccurrenceOK || !directIDOK || !mount.Available() || !call.Available() ||
		!subjectProvenanceOK || subjectMount != mount || subjectCall != call || !mountedOK || !requirementOK ||
		requirement.MountID() != mount || !subjectContextOK || !subjectContextEvidenceOK ||
		membershipProof.direct != directID || membershipProof.attachment != attachment.id {
		return AllocationContextEvent{}, false
	}

	event := AllocationContextEvent{
		transition: transitionID, correlation: rebuiltCorrelationID, directAdmission: rebuiltAdmissionID, direct: directID,
		membershipProof: membershipProof.id, membershipAttach: membershipProof.attachment,
		mount: mount, call: call, descriptor: descriptorID, descriptorOccurrence: descriptorOccurrence,
		subjectBinding: subject.ID(), requirement: requirement.ID(), mountedAllocation: mounted.ID(), allocationKey: requirement.KeyID(),
		membership: membershipProof.membership, subjectContext: subjectContext,
		kind: transition.Kind(), escape: transition.Escape(), mutability: transition.Mutability(), declaredLifetime: transition.Lifetime(),
	}
	if destinationPresent {
		destinationRuntime, destinationOK := destination.Context()
		destinationContext, destinationContextOK := allocationRuntimeContextFor(destinationRuntime)
		if !destinationOK || !destinationContextOK {
			return AllocationContextEvent{}, false
		}
		event.destinationBinding = destination.ID()
		event.destinationContext = destinationContext
		event.hasDestination = true
	}
	if !event.semanticPayloadValid() {
		return AllocationContextEvent{}, false
	}
	event.id, _ = allocationContextEventID(event)
	return event, event.valid()
}

func (event AllocationContextEvent) semanticPayloadValid() bool {
	if !event.transition.Available() || !event.correlation.Available() || !event.directAdmission.Available() || !event.direct.Available() ||
		!event.membershipProof.Available() || !event.membershipAttach.Available() || !event.mount.Available() || !event.call.Available() ||
		!event.descriptor.Available() || !event.descriptorOccurrence.Available() || !event.subjectBinding.Available() ||
		!event.requirement.Available() || !event.mountedAllocation.Available() || !event.allocationKey.Available() ||
		!event.subjectContext.valid() || event.membership != valuedomain.MembershipRecent && event.membership != valuedomain.MembershipSummary ||
		!targetVocabularyValid(event.kind, event.escape, event.mutability, event.declaredLifetime) {
		return false
	}
	if event.hasDestination {
		return event.destinationBinding.Available() && event.destinationContext.valid()
	}
	return !event.destinationBinding.Available() && event.destinationContext == (AllocationRuntimeContext{})
}

func targetVocabularyValid(kind target.PublicationEffectKind, escape target.PublicationEscapeDisposition, mutability target.PublicationMutabilityDisposition, lifetime target.PublicationLifetimeDisposition) bool {
	return kind >= target.PublicationEffectSendTransfer && kind <= target.PublicationEffectCloseRelease &&
		escape >= target.PublicationEscapeNone && escape <= target.PublicationEscapeCallback &&
		mutability >= target.PublicationMutabilityPreserve && mutability <= target.PublicationMutabilityCopyOnWrite &&
		lifetime >= target.PublicationLifetimePreserve && lifetime <= target.PublicationLifetimeRelease
}

func allocationContextEventID(event AllocationContextEvent) (identity.ContentID, bool) {
	if !event.semanticPayloadValid() {
		return identity.ContentID{}, false
	}
	presence := byte(0)
	if event.hasDestination {
		presence = 1
	}
	return identity.DeriveContentID(
		allocationContextEventDomain,
		event.transition[:], event.correlation[:], event.directAdmission[:], event.direct[:], event.membershipProof[:], event.membershipAttach[:],
		event.mount[:], event.call[:], event.descriptor[:], event.descriptorOccurrence[:],
		event.subjectBinding[:], event.requirement[:], event.mountedAllocation[:], event.allocationKey[:],
		[]byte{byte(event.membership)},
		event.subjectContext.id[:], []byte{byte(event.subjectContext.class)}, event.subjectContext.isolation[:], event.subjectContext.sharedBy[:], []byte{boolByte(event.subjectContext.hasSharedBy)},
		event.destinationBinding[:], event.destinationContext.id[:], []byte{byte(event.destinationContext.class)}, event.destinationContext.isolation[:], event.destinationContext.sharedBy[:], []byte{boolByte(event.destinationContext.hasSharedBy)}, []byte{presence},
		[]byte{byte(event.kind), byte(event.escape), byte(event.mutability), byte(event.declaredLifetime)},
	)
}

func boolByte(value bool) byte {
	if value {
		return 1
	}
	return 0
}

func (event AllocationContextEvent) valid() bool {
	want, ok := allocationContextEventID(event)
	return ok && event.id.Available() && event.id == want
}

func (event AllocationContextEvent) Valid() bool { return event.valid() }

func (event AllocationContextEvent) ContentID() (identity.ContentID, bool) {
	return event.id, event.valid()
}

func (event AllocationContextEvent) MountID() identity.ContentID { return event.mount }

func (event AllocationContextEvent) CallOccurrenceID() identity.ContentID { return event.call }

func (event AllocationContextEvent) Kind() target.PublicationEffectKind { return event.kind }

func (event AllocationContextEvent) Escape() target.PublicationEscapeDisposition {
	return event.escape
}

func (event AllocationContextEvent) Mutability() target.PublicationMutabilityDisposition {
	return event.mutability
}

// DeclaredLifetime is the Target disposition the transition declared. Release
// remains a declared transition and never a Residence Dead or LastUse claim.
func (event AllocationContextEvent) DeclaredLifetime() target.PublicationLifetimeDisposition {
	return event.declaredLifetime
}

func (event AllocationContextEvent) Membership() valuedomain.AllocationMembership {
	return event.membership
}

func (event AllocationContextEvent) SubjectContext() AllocationRuntimeContext {
	return event.subjectContext
}

// DestinationContext reports the destination only for a context-required
// transition. A destination-free publication carries none.
func (event AllocationContextEvent) DestinationContext() (AllocationRuntimeContext, bool) {
	if !event.hasDestination {
		return AllocationRuntimeContext{}, false
	}
	return event.destinationContext, true
}

func (event AllocationContextEvent) DestinationBindingID() (identity.ContentID, bool) {
	if !event.hasDestination {
		return identity.ContentID{}, false
	}
	return event.destinationBinding, event.destinationBinding.Available()
}
