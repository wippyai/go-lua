package analysis

import (
	callsite "github.com/wippyai/go-lua/analysis/domain/effect/callsite"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/target"
)

const publicationAllocationContextEventDomain = "analysis/publication-allocation-context-event/v1"

// publicationAllocationRuntimeContext is a detached description of one exact
// runtime-issued physical context. It is deliberately not Residence.Location
// or placement.Placement: the class only records the runtime context that was
// bound while its short-lived authority was live.
type publicationAllocationRuntimeContext struct {
	id          keyspace.ContentID
	class       heapdomain.RuntimeAllocationContextClass
	isolation   keyspace.ContentID
	sharedBy    keyspace.ContentID
	hasSharedBy bool
}

func publicationAllocationRuntimeContextFor(context heapdomain.RuntimeAllocationContext) (publicationAllocationRuntimeContext, bool) {
	if !context.Valid() {
		return publicationAllocationRuntimeContext{}, false
	}
	result := publicationAllocationRuntimeContext{
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

func (context publicationAllocationRuntimeContext) valid() bool {
	if !context.id.Available() || !context.class.Valid() || !context.isolation.Available() {
		return false
	}
	if context.class == heapdomain.RuntimeAllocationContextShared {
		return context.hasSharedBy && context.sharedBy.Available()
	}
	return !context.hasSharedBy && !context.sharedBy.Available()
}

// PublicationAllocationContextEvent is the detached Analysis-local record of
// one proved publication transition whose subject was the exact direct
// allocation observed at the selected call. It retains scalar identities and
// typed transition/context vocabulary only. In particular, it contains no
// Solver, State, Plan, domain owner, runtime capability, Residence fact,
// Footprint graph, placement class, alias claim, or lifetime-survival claim.
type PublicationAllocationContextEvent struct {
	id keyspace.ContentID

	transition       keyspace.ContentID
	correlation      keyspace.ContentID
	directAdmission  keyspace.ContentID
	direct           keyspace.ContentID
	membershipProof  keyspace.ContentID
	membershipAttach keyspace.ContentID

	mount                keyspace.ContentID
	call                 keyspace.ContentID
	descriptor           keyspace.ContentID
	descriptorOccurrence keyspace.ContentID

	subjectBinding     keyspace.ContentID
	requirement        keyspace.ContentID
	mountedAllocation  keyspace.ContentID
	allocationKey      keyspace.ContentID
	membership         valuedomain.AllocationMembership
	subjectContext     publicationAllocationRuntimeContext
	destinationBinding keyspace.ContentID
	destinationContext publicationAllocationRuntimeContext
	hasDestination     bool

	kind             target.PublicationEffectKind
	escape           target.PublicationEscapeDisposition
	mutability       target.PublicationMutabilityDisposition
	declaredLifetime target.PublicationLifetimeDisposition
}

// newPublicationAllocationContextEvent is the only issuance path. It rebuilds
// the two existing cross-owner admissions from live typed capabilities and
// reruns the Phase3D observation proof. A caller therefore cannot provide a
// detached membership scalar, context ID, Heap key, or target consequence.
func newPublicationAllocationContextEvent(
	attachment selectedDirectAllocationMembershipAttachment,
	solver *engine.Solver,
	state *engine.State,
	transition callsite.PublicationTransitionProof,
	correlation callsite.PublicationPlacementCorrelationCandidate,
	directAdmission callsite.PublicationDirectAllocationSubject,
	subject packdomain.RuntimeAllocationContextBinding,
	direct valuedomain.DirectAllocationSubject,
	destination packdomain.RuntimeDestinationContextBinding,
	destinationPresent bool,
) (PublicationAllocationContextEvent, bool) {
	if !transition.MatchesCompletion(solver, state) || !correlation.Valid() || !directAdmission.Valid() || !subject.Valid() || !direct.Valid() {
		return PublicationAllocationContextEvent{}, false
	}

	rebuiltCorrelation, correlationOK := callsite.NewPublicationPlacementCorrelationCandidate(transition, subject, destination, destinationPresent)
	rebuiltCorrelationID, rebuiltCorrelationIDOK := rebuiltCorrelation.ContentID()
	correlationID, correlationIDOK := correlation.ContentID()
	if !correlationOK || !rebuiltCorrelationIDOK || !correlationIDOK || rebuiltCorrelationID != correlationID {
		return PublicationAllocationContextEvent{}, false
	}
	rebuiltDirectAdmission, admissionOK := callsite.NewPublicationDirectAllocationSubject(rebuiltCorrelation, subject, direct)
	rebuiltAdmissionID, rebuiltAdmissionIDOK := rebuiltDirectAdmission.ContentID()
	directAdmissionID, directAdmissionIDOK := directAdmission.ContentID()
	if !admissionOK || !rebuiltAdmissionIDOK || !directAdmissionIDOK || rebuiltAdmissionID != directAdmissionID {
		return PublicationAllocationContextEvent{}, false
	}

	membershipProof, membershipOK := attachment.prove(solver, state, transition, rebuiltCorrelation, subject, direct)
	if !membershipOK || !membershipProof.valid() || membershipProof.correlation != rebuiltCorrelationID || membershipProof.membership != valuedomain.MembershipRecent && membershipProof.membership != valuedomain.MembershipSummary {
		return PublicationAllocationContextEvent{}, false
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
	subjectContext, subjectContextEvidenceOK := publicationAllocationRuntimeContextFor(subjectRuntime)
	if !transitionIDOK || !descriptorIDOK || !descriptorOccurrenceOK || !directIDOK || !mount.Available() || !call.Available() ||
		!subjectProvenanceOK || subjectMount != mount || subjectCall != call || !mountedOK || !requirementOK ||
		requirement.MountID() != mount || !subjectContextOK || !subjectContextEvidenceOK ||
		membershipProof.direct != directID || membershipProof.attachment != attachment.id {
		return PublicationAllocationContextEvent{}, false
	}

	event := PublicationAllocationContextEvent{
		transition: transitionID, correlation: rebuiltCorrelationID, directAdmission: rebuiltAdmissionID, direct: directID,
		membershipProof: membershipProof.id, membershipAttach: membershipProof.attachment,
		mount: mount, call: call, descriptor: descriptorID, descriptorOccurrence: descriptorOccurrence,
		subjectBinding: subject.ID(), requirement: requirement.ID(), mountedAllocation: mounted.ID(), allocationKey: requirement.KeyID(),
		membership: membershipProof.membership, subjectContext: subjectContext,
		kind: transition.Kind(), escape: transition.Escape(), mutability: transition.Mutability(), declaredLifetime: transition.Lifetime(),
	}
	if destinationPresent {
		destinationRuntime, destinationOK := destination.Context()
		destinationContext, destinationContextOK := publicationAllocationRuntimeContextFor(destinationRuntime)
		if !destinationOK || !destinationContextOK {
			return PublicationAllocationContextEvent{}, false
		}
		event.destinationBinding = destination.ID()
		event.destinationContext = destinationContext
		event.hasDestination = true
	}
	if !event.semanticPayloadValid() {
		return PublicationAllocationContextEvent{}, false
	}
	event.id, _ = publicationAllocationContextEventID(event)
	return event, event.valid()
}

func (event PublicationAllocationContextEvent) semanticPayloadValid() bool {
	if !event.transition.Available() || !event.correlation.Available() || !event.directAdmission.Available() || !event.direct.Available() ||
		!event.membershipProof.Available() || !event.membershipAttach.Available() || !event.mount.Available() || !event.call.Available() ||
		!event.descriptor.Available() || !event.descriptorOccurrence.Available() || !event.subjectBinding.Available() ||
		!event.requirement.Available() || !event.mountedAllocation.Available() || !event.allocationKey.Available() ||
		!event.subjectContext.valid() || event.membership != valuedomain.MembershipRecent && event.membership != valuedomain.MembershipSummary ||
		!publicationAllocationTargetVocabularyValid(event.kind, event.escape, event.mutability, event.declaredLifetime) {
		return false
	}
	if event.hasDestination {
		return event.destinationBinding.Available() && event.destinationContext.valid()
	}
	return !event.destinationBinding.Available() && event.destinationContext == (publicationAllocationRuntimeContext{})
}

func publicationAllocationTargetVocabularyValid(kind target.PublicationEffectKind, escape target.PublicationEscapeDisposition, mutability target.PublicationMutabilityDisposition, lifetime target.PublicationLifetimeDisposition) bool {
	return kind >= target.PublicationEffectSendTransfer && kind <= target.PublicationEffectCloseRelease &&
		escape >= target.PublicationEscapeNone && escape <= target.PublicationEscapeCallback &&
		mutability >= target.PublicationMutabilityPreserve && mutability <= target.PublicationMutabilityCopyOnWrite &&
		lifetime >= target.PublicationLifetimePreserve && lifetime <= target.PublicationLifetimeRelease
}

func publicationAllocationContextEventID(event PublicationAllocationContextEvent) (keyspace.ContentID, bool) {
	if !event.semanticPayloadValid() {
		return keyspace.ContentID{}, false
	}
	presence := byte(0)
	if event.hasDestination {
		presence = 1
	}
	return analysisContentID(
		publicationAllocationContextEventDomain,
		event.transition[:], event.correlation[:], event.directAdmission[:], event.direct[:], event.membershipProof[:], event.membershipAttach[:],
		event.mount[:], event.call[:], event.descriptor[:], event.descriptorOccurrence[:],
		event.subjectBinding[:], event.requirement[:], event.mountedAllocation[:], event.allocationKey[:],
		[]byte{byte(event.membership)},
		event.subjectContext.id[:], []byte{byte(event.subjectContext.class)}, event.subjectContext.isolation[:], event.subjectContext.sharedBy[:], []byte{boolByte(event.subjectContext.hasSharedBy)},
		event.destinationBinding[:], event.destinationContext.id[:], []byte{byte(event.destinationContext.class)}, event.destinationContext.isolation[:], event.destinationContext.sharedBy[:], []byte{boolByte(event.destinationContext.hasSharedBy)}, []byte{presence},
		[]byte{byte(event.kind), byte(event.escape), byte(event.mutability), byte(event.declaredLifetime)},
	)
}

func (event PublicationAllocationContextEvent) valid() bool {
	want, ok := publicationAllocationContextEventID(event)
	return ok && event.id.Available() && event.id == want
}

func (event PublicationAllocationContextEvent) Valid() bool { return event.valid() }

func (event PublicationAllocationContextEvent) ContentID() (keyspace.ContentID, bool) {
	return event.id, event.valid()
}
