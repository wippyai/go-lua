package analysis

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect/callsite"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func publicationAllocationContextEventPlan(t testing.TB) *Plan {
	t.Helper()
	program, err := lower.Lower(lower.Source{
		Name: "publication_allocation_context_event_law.lua",
		Text: []byte("sink(function() end, {})\nreturn sink(function() end, {})"),
	})
	if err != nil {
		t.Fatal(err)
	}
	spec := publicationTransitionSpec(true, true, false)
	spec.Operations[0].Effects.Occurrences = append(spec.Operations[0].Effects.Occurrences, target.EffectSpec{
		Target: 2, ValueArgs: []target.ValueFormal{1, 0}, Publication: &target.PublicationEffectSpec{
			Kind: target.PublicationEffectCloseRelease, Subject: 0, Destination: target.PublicationDestinationNone,
			Escape: target.PublicationEscapeNone, Mutability: target.PublicationMutabilityPreserve, Lifetime: target.PublicationLifetimeRelease,
		},
	})
	contract, err := target.Seal(&spec)
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "publication_allocation_context_event_law", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	plan, status, diagnostics := CompileWithDiagnostics(linked)
	if status != CompileComplete || plan == nil || plan.state == nil || plan.state.binding == nil {
		t.Fatalf("compile publication allocation context event fixture=%v diagnostics=%+v", status, diagnostics)
	}
	if runtimeDiagnostic, instantiated := plan.state.instantiateRuntimeTopology(); !instantiated || plan.state.graph == nil || plan.state.queryPlan == nil {
		t.Fatalf("publication allocation context event runtime topology=%+v", runtimeDiagnostic)
	}
	return plan
}

type publicationAllocationContextEventIssue struct {
	event           PublicationAllocationContextEvent
	transition      callsite.PublicationTransitionProof
	correlation     callsite.PublicationPlacementCorrelationCandidate
	directAdmission callsite.PublicationDirectAllocationSubject
	subject         packdomain.RuntimeAllocationContextBinding
	direct          valuedomain.DirectAllocationSubject
	destination     packdomain.RuntimeDestinationContextBinding
	hasDestination  bool
	requirement     heapdomain.AllocationRequirement
}

func issuePublicationAllocationContextEvent(
	t testing.TB,
	plan *Plan,
	attachment selectedDirectAllocationMembershipAttachment,
	solver *engine.Solver,
	state *engine.State,
	issuer packdomain.RuntimeAllocationContextBindingIssuer,
	transition callsite.PublicationTransitionProof,
	subjectContext heapdomain.RuntimeAllocationContext,
	destinationContext heapdomain.RuntimeAllocationContext,
) publicationAllocationContextEventIssue {
	t.Helper()
	mount, call := transition.MountID(), transition.CallOccurrenceID()
	subjectSelector, selectorOK := transition.SubjectSelector()
	source, sourceOK := plan.state.binding.runtimeContexts.pack.MountedInputSemanticSource(mount, call, subjectSelector)
	if !selectorOK || !sourceOK {
		t.Fatal("publication allocation context event subject source")
	}
	requirement, direct := publicationDirectAllocationSubject(t, plan.state.binding, source)
	subject, subjectAvailability := issuer.BindRuntimeAllocationContext(mount, call, subjectSelector, requirement, subjectContext)
	if subjectAvailability != packdomain.RuntimeAllocationContextBindingBound || !subject.Valid() {
		t.Fatal("publication allocation context event subject binding")
	}
	destination := packdomain.RuntimeDestinationContextBinding{}
	destinationPresent := false
	if contextSelector, contextRequired := transition.ContextSelector(); contextRequired {
		var destinationAvailability packdomain.RuntimeAllocationContextBindingAvailability
		destination, destinationAvailability = issuer.BindRuntimeDestinationContext(mount, call, contextSelector, destinationContext)
		if destinationAvailability != packdomain.RuntimeAllocationContextBindingBound || !destination.Valid() {
			t.Fatal("publication allocation context event destination binding")
		}
		destinationPresent = true
	}
	correlation, correlationOK := callsite.NewPublicationPlacementCorrelationCandidate(transition, subject, destination, destinationPresent)
	directAdmission, admissionOK := callsite.NewPublicationDirectAllocationSubject(correlation, subject, direct)
	event, eventOK := newPublicationAllocationContextEvent(attachment, solver, state, transition, correlation, directAdmission, subject, direct, destination, destinationPresent)
	if !correlationOK || !admissionOK || !eventOK || !event.Valid() {
		t.Fatal("publication allocation context event issuance")
	}
	return publicationAllocationContextEventIssue{
		event: event, transition: transition, correlation: correlation, directAdmission: directAdmission,
		subject: subject, direct: direct, destination: destination, hasDestination: destinationPresent, requirement: requirement,
	}
}

// TestPublicationAllocationContextEventOwnerLaw proves only the exact detached
// event boundary. Runtime context classes remain physical qualifiers, and the
// Target Release disposition remains a declared transition rather than a
// Residence Dead/LastUse or placement conclusion.
func TestPublicationAllocationContextEventOwnerLaw(t *testing.T) {
	plan := publicationAllocationContextEventPlan(t)
	defer plan.Close()
	mount, occurrence, secondOccurrence := selectedCallEffectOccurrences(t, plan)
	compilation := publicationTransitionCompilationFor(t, plan, mount, occurrence)
	candidates, candidatesOK := plan.state.binding.effectSelected.AttachMountedPublicationCandidates(compilation, plan.state.graph, plan.state.binding.effectQuery, mount, occurrence)
	attachment, attached := attachSelectedDirectAllocationMembership(compilation, plan.state.binding, plan.state.graph, plan.state.artifacts.mounts, mount, occurrence)
	secondCandidates, secondCandidatesOK := plan.state.binding.effectSelected.AttachMountedPublicationCandidates(compilation, plan.state.graph, plan.state.binding.effectQuery, mount, secondOccurrence)
	secondAttachment, secondAttached := attachSelectedDirectAllocationMembership(compilation, plan.state.binding, plan.state.graph, plan.state.artifacts.mounts, mount, secondOccurrence)
	if !candidatesOK || !candidates.Available() || !attached || !attachment.valid() || !secondCandidatesOK || !secondCandidates.Available() || !secondAttached || !secondAttachment.valid() {
		t.Fatal("publication allocation context event pre-solve attachments")
	}
	solver, solverOK := compilation.Solver()
	if !solverOK || solver == nil {
		t.Fatal("publication allocation context event solver")
	}
	state, status := solver.Solve(context.Background())
	if status != engine.SolveComplete || state == nil {
		t.Fatalf("publication allocation context event solve=%v state=%t", status, state != nil)
	}
	proofs := make(map[target.PublicationEffectKind]callsite.PublicationTransitionProof)
	for index := 0; index < candidates.Count(); index++ {
		candidate, candidateOK := candidates.At(index)
		proof, failure := candidate.ProveWithFailure(solver, state)
		if !candidateOK || failure != callsite.PublicationTransitionProofFailureNone || !proof.Valid() {
			t.Fatalf("publication allocation context candidate index=%d failure=%d", index, failure)
		}
		proofs[proof.Kind()] = proof
	}
	send, sendOK := proofs[target.PublicationEffectSendTransfer]
	release, releaseOK := proofs[target.PublicationEffectCloseRelease]
	returned, returnOK := proofs[target.PublicationEffectReturnEscape]
	if !sendOK || !releaseOK || !returnOK {
		t.Fatal("publication allocation context event transition inventory")
	}

	authority, issuer, authorityOK := plan.state.binding.runtimeContexts.Begin(publicationPlacementPolicyID("allocation-context-event"))
	if !authorityOK || authority == nil {
		t.Fatal("publication allocation context event runtime issuer")
	}
	processOwner, processOwnerOK := authority.ProcessOwner(publicationPlacementPolicyID("event-process"))
	actorOwner, actorOwnerOK := authority.ActorOwner(publicationPlacementPolicyID("event-actor"))
	threadOwner, threadOwnerOK := authority.ThreadOwner(publicationPlacementPolicyID("event-thread"))
	sharedOwner, sharedOwnerOK := authority.SharedOwner(publicationPlacementPolicyID("event-shared"))
	processContext, processOK := authority.Process(processOwner)
	actorContext, actorOK := authority.Actor(actorOwner)
	threadContext, threadOK := authority.Thread(threadOwner)
	sharedAuthorization, sharedAuthorizationOK := authority.AuthorizeShared(publicationPlacementPolicyID("event-shared-authorization"))
	sharedContext, sharedOK := authority.Shared(sharedOwner, sharedAuthorization)
	if !processOwnerOK || !actorOwnerOK || !threadOwnerOK || !sharedOwnerOK || !processOK || !actorOK || !threadOK || !sharedAuthorizationOK || !sharedOK {
		t.Fatal("publication allocation context event runtime contexts")
	}

	contextCases := []struct {
		class   heapdomain.RuntimeAllocationContextClass
		context heapdomain.RuntimeAllocationContext
	}{
		{heapdomain.RuntimeAllocationContextProcess, processContext},
		{heapdomain.RuntimeAllocationContextActor, actorContext},
		{heapdomain.RuntimeAllocationContextThread, threadContext},
		{heapdomain.RuntimeAllocationContextShared, sharedContext},
	}
	issuedIDs := make(map[[32]byte]struct{}, len(contextCases))
	var sendIssue, sharedIssue publicationAllocationContextEventIssue
	for _, runtimeCase := range contextCases {
		issued := issuePublicationAllocationContextEvent(t, plan, attachment, solver, state, issuer, send, runtimeCase.context, actorContext)
		id, idOK := issued.event.ContentID()
		if !idOK || issued.event.subjectContext.class != runtimeCase.class || !issued.event.hasDestination || issued.event.destinationContext.class != heapdomain.RuntimeAllocationContextActor ||
			issued.event.kind != target.PublicationEffectSendTransfer || issued.event.escape != target.PublicationEscapeSendTransfer ||
			issued.event.mutability != target.PublicationMutabilityCopyOnWrite || issued.event.declaredLifetime != target.PublicationLifetimePreserve {
			t.Fatalf("publication allocation context class=%d evidence", runtimeCase.class)
		}
		if runtimeCase.class == heapdomain.RuntimeAllocationContextShared {
			if !issued.event.subjectContext.hasSharedBy || issued.event.subjectContext.sharedBy != sharedContext.SharedAuthorizationID() {
				t.Fatal("publication allocation context shared authorization")
			}
			sharedIssue = issued
		} else if issued.event.subjectContext.hasSharedBy || issued.event.subjectContext.sharedBy.Available() {
			t.Fatalf("publication allocation context class=%d acquired shared authorization", runtimeCase.class)
		}
		if _, duplicate := issuedIDs[id]; duplicate {
			t.Fatal("publication allocation context classes aliased event identity")
		}
		issuedIDs[id] = struct{}{}
		if runtimeCase.class == heapdomain.RuntimeAllocationContextProcess {
			sendIssue = issued
		}
	}

	releaseIssue := issuePublicationAllocationContextEvent(t, plan, attachment, solver, state, issuer, release, processContext, heapdomain.RuntimeAllocationContext{})
	returnIssue := issuePublicationAllocationContextEvent(t, plan, attachment, solver, state, issuer, returned, processContext, heapdomain.RuntimeAllocationContext{})
	if releaseIssue.event.hasDestination || releaseIssue.event.destinationBinding.Available() || releaseIssue.event.destinationContext != (publicationAllocationRuntimeContext{}) ||
		releaseIssue.event.kind != target.PublicationEffectCloseRelease || releaseIssue.event.escape != target.PublicationEscapeNone ||
		releaseIssue.event.mutability != target.PublicationMutabilityPreserve || releaseIssue.event.declaredLifetime != target.PublicationLifetimeRelease ||
		returnIssue.event.hasDestination || returnIssue.event.kind != target.PublicationEffectReturnEscape || returnIssue.event.escape != target.PublicationEscapeReturn ||
		returnIssue.event.mutability != target.PublicationMutabilityPreserve || returnIssue.event.declaredLifetime != target.PublicationLifetimePreserve {
		t.Fatal("destination-free or declared-lifetime event evidence")
	}
	// These are the only lawful Phase3G-A conclusions: Release remains a
	// declared Target transition, and Shared remains a runtime context class.
	// Neither event contains a Dead/LastUse or SharedHeap projection.
	if releaseIssue.event.membership != valuedomain.MembershipRecent && releaseIssue.event.membership != valuedomain.MembershipSummary ||
		sharedIssue.event.subjectContext.class != heapdomain.RuntimeAllocationContextShared {
		t.Fatal("publication allocation context event narrow conclusion")
	}

	if _, accepted := newPublicationAllocationContextEvent(attachment, solver, state, send, sendIssue.correlation, sendIssue.directAdmission, sendIssue.subject, sendIssue.direct, packdomain.RuntimeDestinationContextBinding{}, false); accepted {
		t.Fatal("context-required publication issued without destination")
	}
	if _, accepted := newPublicationAllocationContextEvent(attachment, solver, state, release, releaseIssue.correlation, releaseIssue.directAdmission, releaseIssue.subject, releaseIssue.direct, sendIssue.destination, true); accepted {
		t.Fatal("destination-free publication accepted an extra context")
	}
	contextSelector, contextSelectorOK := send.ContextSelector()
	wrongDestination, wrongDestinationAvailability := issuer.BindRuntimeDestinationContext(mount, occurrence, contextSelector, threadContext)
	if !contextSelectorOK || wrongDestinationAvailability != packdomain.RuntimeAllocationContextBindingBound || !wrongDestination.Valid() {
		t.Fatal("publication allocation context wrong destination fixture")
	}
	if _, accepted := newPublicationAllocationContextEvent(attachment, solver, state, send, sendIssue.correlation, sendIssue.directAdmission, sendIssue.subject, sendIssue.direct, wrongDestination, true); accepted {
		t.Fatal("publication allocation context accepted a destination splice")
	}
	if _, accepted := newPublicationAllocationContextEvent(attachment, solver, state, send, releaseIssue.correlation, releaseIssue.directAdmission, releaseIssue.subject, releaseIssue.direct, sendIssue.destination, true); accepted {
		t.Fatal("publication allocation context accepted a selector/correlation splice")
	}
	wrongRequirement := publicationDifferentAllocationRequirement(t, plan.state.binding, sendIssue.requirement)
	subjectSelector, subjectSelectorOK := send.SubjectSelector()
	wrongSubject, wrongSubjectAvailability := issuer.BindRuntimeAllocationContext(mount, occurrence, subjectSelector, wrongRequirement, processContext)
	if !subjectSelectorOK || wrongSubjectAvailability != packdomain.RuntimeAllocationContextBindingBound || !wrongSubject.Valid() {
		t.Fatal("publication allocation context wrong requirement fixture")
	}
	if _, accepted := newPublicationAllocationContextEvent(attachment, solver, state, send, sendIssue.correlation, sendIssue.directAdmission, wrongSubject, sendIssue.direct, sendIssue.destination, true); accepted {
		t.Fatal("publication allocation context accepted a requirement splice")
	}
	if _, accepted := newPublicationAllocationContextEvent(attachment, solver, state, send, sendIssue.correlation, sendIssue.directAdmission, sendIssue.subject, valuedomain.DirectAllocationSubject{}, sendIssue.destination, true); accepted {
		t.Fatal("publication allocation context accepted an absent direct receipt")
	}
	if _, accepted := newPublicationAllocationContextEvent(attachment, nil, state, send, sendIssue.correlation, sendIssue.directAdmission, sendIssue.subject, sendIssue.direct, sendIssue.destination, true); accepted {
		t.Fatal("publication allocation context accepted a foreign solver completion")
	}
	if _, accepted := newPublicationAllocationContextEvent(attachment, solver, nil, send, sendIssue.correlation, sendIssue.directAdmission, sendIssue.subject, sendIssue.direct, sendIssue.destination, true); accepted {
		t.Fatal("publication allocation context accepted a foreign state completion")
	}

	secondProofs := make(map[target.PublicationEffectKind]callsite.PublicationTransitionProof)
	for index := 0; index < secondCandidates.Count(); index++ {
		candidate, candidateOK := secondCandidates.At(index)
		proof, failure := candidate.ProveWithFailure(solver, state)
		if !candidateOK || failure != callsite.PublicationTransitionProofFailureNone || !proof.Valid() {
			t.Fatal("publication allocation context second-call proof")
		}
		secondProofs[proof.Kind()] = proof
	}
	secondSend, secondSendOK := secondProofs[target.PublicationEffectSendTransfer]
	if !secondSendOK {
		t.Fatal("publication allocation context second-call send")
	}
	secondIssue := issuePublicationAllocationContextEvent(t, plan, secondAttachment, solver, state, issuer, secondSend, processContext, actorContext)
	if _, accepted := newPublicationAllocationContextEvent(attachment, solver, state, secondSend, secondIssue.correlation, secondIssue.directAdmission, secondIssue.subject, secondIssue.direct, secondIssue.destination, true); accepted {
		t.Fatal("publication allocation context accepted a cross-call membership attachment")
	}
	if _, accepted := newPublicationAllocationContextEvent(attachment, solver, state, send, sendIssue.correlation, sendIssue.directAdmission, sendIssue.subject, secondIssue.direct, sendIssue.destination, true); accepted {
		t.Fatal("publication allocation context accepted a direct-allocation splice")
	}

	foreignAuthority, foreignIssuer, foreignAuthorityOK := plan.state.binding.runtimeContexts.Begin(publicationPlacementPolicyID("allocation-context-event"))
	if !foreignAuthorityOK || foreignAuthority == nil {
		t.Fatal("publication allocation context equal-content authority")
	}
	foreignProcessOwner, foreignProcessOwnerOK := foreignAuthority.ProcessOwner(publicationPlacementPolicyID("event-process"))
	foreignActorOwner, foreignActorOwnerOK := foreignAuthority.ActorOwner(publicationPlacementPolicyID("event-actor"))
	foreignProcess, foreignProcessOK := foreignAuthority.Process(foreignProcessOwner)
	foreignActor, foreignActorOK := foreignAuthority.Actor(foreignActorOwner)
	foreignSubject, foreignSubjectAvailability := foreignIssuer.BindRuntimeAllocationContext(mount, occurrence, subjectSelector, sendIssue.requirement, foreignProcess)
	foreignDestination, foreignDestinationAvailability := foreignIssuer.BindRuntimeDestinationContext(mount, occurrence, contextSelector, foreignActor)
	if !foreignProcessOwnerOK || !foreignActorOwnerOK || !foreignProcessOK || !foreignActorOK ||
		foreignSubjectAvailability != packdomain.RuntimeAllocationContextBindingBound || foreignDestinationAvailability != packdomain.RuntimeAllocationContextBindingBound ||
		!foreignSubject.Valid() || !foreignDestination.Valid() || foreignSubject.ID() != sendIssue.subject.ID() || foreignDestination.ID() != sendIssue.destination.ID() {
		t.Fatal("publication allocation context equal-content authority fixture")
	}
	if _, accepted := newPublicationAllocationContextEvent(attachment, solver, state, send, sendIssue.correlation, sendIssue.directAdmission, sendIssue.subject, sendIssue.direct, foreignDestination, true); accepted {
		t.Fatal("publication allocation context mixed equal-content authorities")
	}
	if _, accepted := newPublicationAllocationContextEvent(attachment, solver, state, send, sendIssue.correlation, sendIssue.directAdmission, foreignSubject, sendIssue.direct, sendIssue.destination, true); accepted {
		t.Fatal("publication allocation context mixed equal-content subject authority")
	}
	foreignAuthority.Close()
	if _, accepted := newPublicationAllocationContextEvent(attachment, solver, state, send, sendIssue.correlation, sendIssue.directAdmission, foreignSubject, sendIssue.direct, foreignDestination, true); accepted {
		t.Fatal("publication allocation context accepted a closed authority")
	}

	mutations := []PublicationAllocationContextEvent{
		func() PublicationAllocationContextEvent {
			value := sendIssue.event
			value.id = publicationPlacementPolicyID("mutated-event-id")
			return value
		}(),
		func() PublicationAllocationContextEvent {
			value := sendIssue.event
			value.transition = publicationPlacementPolicyID("mutated-transition")
			return value
		}(),
		func() PublicationAllocationContextEvent {
			value := sendIssue.event
			value.correlation = publicationPlacementPolicyID("mutated-correlation")
			return value
		}(),
		func() PublicationAllocationContextEvent {
			value := sendIssue.event
			value.directAdmission = publicationPlacementPolicyID("mutated-admission")
			return value
		}(),
		func() PublicationAllocationContextEvent {
			value := sendIssue.event
			value.membership = valuedomain.MembershipMixedOrUnknown
			return value
		}(),
		func() PublicationAllocationContextEvent {
			value := sendIssue.event
			value.allocationKey = publicationPlacementPolicyID("mutated-key")
			return value
		}(),
		func() PublicationAllocationContextEvent {
			value := sendIssue.event
			value.subjectContext.class = heapdomain.RuntimeAllocationContextThread
			return value
		}(),
		func() PublicationAllocationContextEvent {
			value := sendIssue.event
			value.destinationContext.isolation = publicationPlacementPolicyID("mutated-destination")
			return value
		}(),
		func() PublicationAllocationContextEvent {
			value := sendIssue.event
			value.hasDestination = false
			return value
		}(),
		func() PublicationAllocationContextEvent {
			value := sendIssue.event
			value.declaredLifetime = target.PublicationLifetimeRelease
			return value
		}(),
	}
	for index, mutated := range mutations {
		if mutated.Valid() {
			t.Fatalf("publication allocation context scalar mutation=%d survived", index)
		}
	}

	authority.Close()
	if _, accepted := newPublicationAllocationContextEvent(attachment, solver, state, send, sendIssue.correlation, sendIssue.directAdmission, sendIssue.subject, sendIssue.direct, sendIssue.destination, true); accepted {
		t.Fatal("publication allocation context issued after authority close")
	}
	if !sendIssue.event.Valid() || !releaseIssue.event.Valid() || !sharedIssue.event.Valid() {
		t.Fatal("detached publication allocation context event did not survive capability release")
	}
}
