package publication

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

func publicationLawID(label string) identity.ContentID {
	return identity.ContentID(sha256.Sum256([]byte("publication-law/" + label)))
}

func sealedAllocationRuntimeContext(label string, class heapdomain.RuntimeAllocationContextClass) AllocationRuntimeContext {
	context := AllocationRuntimeContext{
		id:        publicationLawID(label + "/context"),
		class:     class,
		isolation: publicationLawID(label + "/isolation"),
	}
	if class == heapdomain.RuntimeAllocationContextShared {
		context.sharedBy = publicationLawID(label + "/shared-authorization")
		context.hasSharedBy = true
	}
	return context
}

func sealedAllocationContextEvent(destination bool, subjectClass heapdomain.RuntimeAllocationContextClass) AllocationContextEvent {
	event := AllocationContextEvent{
		transition:           publicationLawID("transition"),
		correlation:          publicationLawID("correlation"),
		directAdmission:      publicationLawID("direct-admission"),
		direct:               publicationLawID("direct"),
		membershipProof:      publicationLawID("membership-proof"),
		membershipAttach:     publicationLawID("membership-attachment"),
		mount:                publicationLawID("mount"),
		call:                 publicationLawID("call"),
		descriptor:           publicationLawID("descriptor"),
		descriptorOccurrence: publicationLawID("descriptor-occurrence"),
		subjectBinding:       publicationLawID("subject-binding"),
		requirement:          publicationLawID("requirement"),
		mountedAllocation:    publicationLawID("mounted-allocation"),
		allocationKey:        publicationLawID("allocation-key"),
		membership:           valuedomain.MembershipRecent,
		subjectContext:       sealedAllocationRuntimeContext("subject", subjectClass),
		kind:                 target.PublicationEffectSendTransfer,
		escape:               target.PublicationEscapeSendTransfer,
		mutability:           target.PublicationMutabilityCopyOnWrite,
		declaredLifetime:     target.PublicationLifetimePreserve,
	}
	if destination {
		event.destinationBinding = publicationLawID("destination-binding")
		event.destinationContext = sealedAllocationRuntimeContext("destination", heapdomain.RuntimeAllocationContextActor)
		event.hasDestination = true
	}
	event.id, _ = allocationContextEventID(event)
	return event
}

// The event carries every scalar of the admission it records, so the seal is
// the only thing standing between a detached record and a fabricated one. This
// law walks each field individually rather than sampling: a field the identity
// forgets is a field a caller may rewrite after issuance.
func TestAllocationContextEventScalarSealLaw(t *testing.T) {
	event := sealedAllocationContextEvent(true, heapdomain.RuntimeAllocationContextProcess)
	id, idOK := event.ContentID()
	if !event.Valid() || !idOK || !id.Available() {
		t.Fatal("sealed allocation context event invalid")
	}

	foreign := publicationLawID("foreign")
	mutations := map[string]func(AllocationContextEvent) AllocationContextEvent{
		"id":          func(value AllocationContextEvent) AllocationContextEvent { value.id = foreign; return value },
		"transition":  func(value AllocationContextEvent) AllocationContextEvent { value.transition = foreign; return value },
		"correlation": func(value AllocationContextEvent) AllocationContextEvent { value.correlation = foreign; return value },
		"direct-admission": func(value AllocationContextEvent) AllocationContextEvent {
			value.directAdmission = foreign
			return value
		},
		"direct": func(value AllocationContextEvent) AllocationContextEvent { value.direct = foreign; return value },
		"membership-proof": func(value AllocationContextEvent) AllocationContextEvent {
			value.membershipProof = foreign
			return value
		},
		"membership-attachment": func(value AllocationContextEvent) AllocationContextEvent {
			value.membershipAttach = foreign
			return value
		},
		"mount":      func(value AllocationContextEvent) AllocationContextEvent { value.mount = foreign; return value },
		"call":       func(value AllocationContextEvent) AllocationContextEvent { value.call = foreign; return value },
		"descriptor": func(value AllocationContextEvent) AllocationContextEvent { value.descriptor = foreign; return value },
		"descriptor-occurrence": func(value AllocationContextEvent) AllocationContextEvent {
			value.descriptorOccurrence = foreign
			return value
		},
		"subject-binding": func(value AllocationContextEvent) AllocationContextEvent {
			value.subjectBinding = foreign
			return value
		},
		"requirement": func(value AllocationContextEvent) AllocationContextEvent { value.requirement = foreign; return value },
		"mounted-allocation": func(value AllocationContextEvent) AllocationContextEvent {
			value.mountedAllocation = foreign
			return value
		},
		"allocation-key": func(value AllocationContextEvent) AllocationContextEvent { value.allocationKey = foreign; return value },
		"membership": func(value AllocationContextEvent) AllocationContextEvent {
			value.membership = valuedomain.MembershipSummary
			return value
		},
		"subject-context-id": func(value AllocationContextEvent) AllocationContextEvent {
			value.subjectContext.id = foreign
			return value
		},
		"subject-context-class": func(value AllocationContextEvent) AllocationContextEvent {
			value.subjectContext.class = heapdomain.RuntimeAllocationContextThread
			return value
		},
		"subject-context-isolation": func(value AllocationContextEvent) AllocationContextEvent {
			value.subjectContext.isolation = foreign
			return value
		},
		"destination-binding": func(value AllocationContextEvent) AllocationContextEvent {
			value.destinationBinding = foreign
			return value
		},
		"destination-context-id": func(value AllocationContextEvent) AllocationContextEvent {
			value.destinationContext.id = foreign
			return value
		},
		"destination-context-class": func(value AllocationContextEvent) AllocationContextEvent {
			value.destinationContext.class = heapdomain.RuntimeAllocationContextThread
			return value
		},
		"destination-context-isolation": func(value AllocationContextEvent) AllocationContextEvent {
			value.destinationContext.isolation = foreign
			return value
		},
		"destination-presence": func(value AllocationContextEvent) AllocationContextEvent {
			value.hasDestination = false
			return value
		},
		"kind": func(value AllocationContextEvent) AllocationContextEvent {
			value.kind = target.PublicationEffectReturnEscape
			return value
		},
		"escape": func(value AllocationContextEvent) AllocationContextEvent {
			value.escape = target.PublicationEscapeReturn
			return value
		},
		"mutability": func(value AllocationContextEvent) AllocationContextEvent {
			value.mutability = target.PublicationMutabilityPreserve
			return value
		},
		"declared-lifetime": func(value AllocationContextEvent) AllocationContextEvent {
			value.declaredLifetime = target.PublicationLifetimeRelease
			return value
		},
	}
	for name, mutate := range mutations {
		mutated := mutate(event)
		if mutated.Valid() {
			t.Fatalf("allocation context event scalar mutation survived field=%s", name)
		}
		if _, ok := mutated.ContentID(); ok {
			t.Fatalf("mutated allocation context event published an identity field=%s", name)
		}
	}
}

// A destination-free publication must carry no destination at all. Holding a
// zero binding beside a present flag, or a bound destination beside an absent
// flag, are both half states the seal refuses.
func TestAllocationContextEventDestinationPresenceLaw(t *testing.T) {
	event := sealedAllocationContextEvent(false, heapdomain.RuntimeAllocationContextProcess)
	if !event.Valid() {
		t.Fatal("destination-free allocation context event invalid")
	}
	if context, ok := event.DestinationContext(); ok || context != (AllocationRuntimeContext{}) {
		t.Fatal("destination-free event published a destination context")
	}
	if _, ok := event.DestinationBindingID(); ok {
		t.Fatal("destination-free event published a destination binding")
	}

	claimed := event
	claimed.hasDestination = true
	claimed.id, _ = allocationContextEventID(claimed)
	if claimed.Valid() {
		t.Fatal("destination presence without a bound destination sealed")
	}
	orphan := event
	orphan.destinationBinding = publicationLawID("orphan-destination-binding")
	orphan.id, _ = allocationContextEventID(orphan)
	if orphan.Valid() {
		t.Fatal("destination binding without declared presence sealed")
	}
	orphanContext := event
	orphanContext.destinationContext = sealedAllocationRuntimeContext("orphan", heapdomain.RuntimeAllocationContextActor)
	orphanContext.id, _ = allocationContextEventID(orphanContext)
	if orphanContext.Valid() {
		t.Fatal("destination context without declared presence sealed")
	}
}

// Shared is the only runtime class that carries an authorization. Every other
// class carrying one, or Shared carrying none, is a fabricated context rather
// than a runtime-issued one.
func TestAllocationRuntimeContextClassLaw(t *testing.T) {
	shared := sealedAllocationContextEvent(true, heapdomain.RuntimeAllocationContextShared)
	authorization, authorizationOK := shared.SubjectContext().SharedAuthorizationID()
	if !shared.Valid() || !authorizationOK || !authorization.Available() {
		t.Fatal("shared subject context authorization")
	}
	process := sealedAllocationContextEvent(true, heapdomain.RuntimeAllocationContextProcess)
	if _, ok := process.SubjectContext().SharedAuthorizationID(); ok {
		t.Fatal("process subject context published a shared authorization")
	}

	unauthorized := shared
	unauthorized.subjectContext.sharedBy = identity.ContentID{}
	unauthorized.subjectContext.hasSharedBy = false
	unauthorized.id, _ = allocationContextEventID(unauthorized)
	if unauthorized.Valid() {
		t.Fatal("shared runtime class sealed without an authorization")
	}
	overreaching := process
	overreaching.subjectContext.sharedBy = publicationLawID("overreaching-authorization")
	overreaching.subjectContext.hasSharedBy = true
	overreaching.id, _ = allocationContextEventID(overreaching)
	if overreaching.Valid() {
		t.Fatal("non-shared runtime class sealed with an authorization")
	}
	incomplete := AllocationRuntimeContext{class: heapdomain.RuntimeAllocationContextProcess, isolation: publicationLawID("isolation")}
	if incomplete.Valid() {
		t.Fatal("runtime context without a context identity sealed")
	}
}

// Membership is the whole conclusion this event is allowed to make, so it must
// remain an exact Recent or Summary cell. Mixed or unknown is not a weaker
// answer, it is no answer.
func TestAllocationContextEventMembershipLaw(t *testing.T) {
	for _, membership := range []valuedomain.AllocationMembership{valuedomain.MembershipRecent, valuedomain.MembershipSummary} {
		event := sealedAllocationContextEvent(true, heapdomain.RuntimeAllocationContextProcess)
		event.membership = membership
		event.id, _ = allocationContextEventID(event)
		if !event.Valid() || event.Membership() != membership {
			t.Fatalf("exact membership=%d sealed", membership)
		}
	}
	event := sealedAllocationContextEvent(true, heapdomain.RuntimeAllocationContextProcess)
	event.membership = valuedomain.MembershipMixedOrUnknown
	event.id, _ = allocationContextEventID(event)
	if event.Valid() {
		t.Fatal("mixed or unknown membership sealed")
	}
}

// The typed transition vocabulary is bounded by Target. A value outside the
// declared range is not an unfamiliar disposition to pass through, it is a
// value Target never issued.
func TestAllocationContextEventTargetVocabularyLaw(t *testing.T) {
	event := sealedAllocationContextEvent(true, heapdomain.RuntimeAllocationContextProcess)
	beyond := []func(AllocationContextEvent) AllocationContextEvent{
		func(value AllocationContextEvent) AllocationContextEvent {
			value.kind = target.PublicationEffectCloseRelease + 1
			return value
		},
		func(value AllocationContextEvent) AllocationContextEvent {
			value.escape = target.PublicationEscapeCallback + 1
			return value
		},
		func(value AllocationContextEvent) AllocationContextEvent {
			value.mutability = target.PublicationMutabilityCopyOnWrite + 1
			return value
		},
		func(value AllocationContextEvent) AllocationContextEvent {
			value.declaredLifetime = target.PublicationLifetimeRelease + 1
			return value
		},
	}
	for index, mutate := range beyond {
		mutated := mutate(event)
		mutated.id, _ = allocationContextEventID(mutated)
		if _, ok := allocationContextEventID(mutated); ok || mutated.Valid() {
			t.Fatalf("out-of-range target vocabulary sealed index=%d", index)
		}
	}
	if !targetVocabularyValid(target.PublicationEffectCloseRelease, target.PublicationEscapeNone, target.PublicationMutabilityPreserve, target.PublicationLifetimeRelease) {
		t.Fatal("declared target vocabulary rejected")
	}
}
