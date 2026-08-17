package analysis

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/composite"
	publication "github.com/wippyai/go-lua/analysis/domain/composite/publication"
	"github.com/wippyai/go-lua/analysis/domain/effect/callsite"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
)

// selectedEffectMemberRef resolves the one selected CallEffect member the root
// admitted for this call occurrence. Naming the member is the analyzer root's
// composition step: the composite relation is handed exact coordinates and
// never searches this Plan's private artifact inventory.
func selectedEffectMemberRef(mounts []mountedProgramArtifact, mount, occurrence identity.ContentID) (artifactRuleMemberRef, bool) {
	var found artifactRuleMemberRef
	for _, candidate := range mounts {
		if candidate.moduleKey != mount || !candidate.ruleMembersReady {
			continue
		}
		for _, member := range candidate.ruleMembers {
			if member.role != programartifact.RuleRoleEffectSelected || member.mount != mount || member.occurrence != occurrence {
				continue
			}
			if found.point.Available() {
				return artifactRuleMemberRef{}, false
			}
			found = member
		}
	}
	return found, found.point.Available()
}

func attachSelectedDirectAllocationMembership(
	compilation *engine.ReceiptCompilation,
	binding *composite.ProgramBinding,
	graph *engine.ReceiptGraph,
	mounts []mountedProgramArtifact,
	mount, call identity.ContentID,
) (publication.DirectAllocationMembershipAttachment, bool) {
	if binding == nil {
		return publication.DirectAllocationMembershipAttachment{}, false
	}
	ref, refOK := selectedEffectMemberRef(mounts, mount, call)
	role, roleOK := mountedCapability(binding, programartifact.RuleRoleEffectSelected)
	valueSchema := binding.ValueSchema()
	if !refOK || !roleOK || valueSchema == nil || valueSchema.CoordinateCount() <= 0 || uint64(valueSchema.CoordinateCount()) > uint64(^uint32(0)) {
		return publication.DirectAllocationMembershipAttachment{}, false
	}
	return publication.AttachSelectedDirectAllocationMembership(compilation, binding.ValueQuery(), graph, role, mount, ref.point, call, uint32(valueSchema.CoordinateCount()))
}

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
	event           publication.AllocationContextEvent
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
	attachment publication.DirectAllocationMembershipAttachment,
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
	source, sourceOK := plan.state.binding.RuntimeContexts().Pack().MountedInputSemanticSource(mount, call, subjectSelector)
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
	event, eventOK := publication.NewAllocationContextEvent(attachment, solver, state, transition, correlation, directAdmission, subject, direct, destination, destinationPresent)
	if !correlationOK || !admissionOK || !eventOK || !event.Valid() {
		t.Fatal("publication allocation context event issuance")
	}
	return publicationAllocationContextEventIssue{
		event: event, transition: transition, correlation: correlation, directAdmission: directAdmission,
		subject: subject, direct: direct, destination: destination, hasDestination: destinationPresent, requirement: requirement,
	}
}

// TestSelectedDirectAllocationMembershipOwnerLaw keeps the first Phase3D
// post-convergence result deliberately small. It attaches the Effect proof and
// Value summary to one open compilation, solves once, and then proves only an
// exact Recent/Summary membership at Value's direct allocation coordinate.
// It neither scans the vector for aliases nor derives a placement result.
func TestSelectedDirectAllocationMembershipOwnerLaw(t *testing.T) {
	plan := publicationSecondCallDirectAllocationTransitionPlan(t, true, true, false)
	defer plan.Close()
	foreignPlan := publicationSecondCallDirectAllocationTransitionPlan(t, true, true, false)
	defer foreignPlan.Close()
	mount, occurrence, secondOccurrence := selectedCallEffectOccurrences(t, plan)
	compilation := publicationTransitionCompilationFor(t, plan, mount, occurrence)
	candidates, candidatesOK := selectedEffectRule(plan.state.binding).AttachMountedPublicationCandidates(compilation, plan.state.graph, plan.state.binding.EffectQuery(), mount, occurrence)
	attachment, attached := attachSelectedDirectAllocationMembership(compilation, plan.state.binding, plan.state.graph, plan.state.artifacts.mounts, mount, occurrence)
	attachmentID, attachmentIDOK := attachment.ContentID()
	if !candidatesOK || !candidates.Available() || candidates.Count() < 2 || !attached || !attachment.Valid() || !attachmentIDOK {
		t.Fatal("direct allocation membership pre-solve attachment")
	}
	if _, duplicate := attachSelectedDirectAllocationMembership(compilation, plan.state.binding, plan.state.graph, plan.state.artifacts.mounts, mount, occurrence); duplicate {
		t.Fatal("direct allocation membership attached more than one Value observation for one selected member")
	}
	if _, accepted := attachSelectedDirectAllocationMembership(compilation, plan.state.binding, plan.state.graph, plan.state.artifacts.mounts, publicationPlacementPolicyID("wrong-membership-mount"), occurrence); accepted {
		t.Fatal("direct allocation membership accepted a foreign mount")
	}
	if _, accepted := attachSelectedDirectAllocationMembership(compilation, plan.state.binding, plan.state.graph, plan.state.artifacts.mounts, mount, publicationPlacementPolicyID("wrong-membership-call")); accepted {
		t.Fatal("direct allocation membership accepted a foreign call")
	}
	var nonSelectedOccurrence identity.ContentID
	for _, mounted := range plan.state.artifacts.mounts {
		if mounted.moduleKey != mount || mounted.artifact == nil {
			continue
		}
		for roleIndex := 0; roleIndex < programartifact.MountedRuleRoleCount() && !nonSelectedOccurrence.Available(); roleIndex++ {
			role, roleOK := programartifact.MountedRuleRoleAt(roleIndex)
			if !roleOK || role == programartifact.RuleRoleEffectSelected || mounted.artifact.RuleOccurrenceCount(role) == 0 {
				continue
			}
			row, rowOK := mounted.artifact.RuleOccurrenceAt(role, 0)
			if rowOK && row.ID().Available() && row.ID() != occurrence {
				nonSelectedOccurrence = row.ID()
			}
		}
	}
	if !nonSelectedOccurrence.Available() {
		t.Fatal("direct allocation membership non-selected role fixture")
	}
	if _, accepted := attachSelectedDirectAllocationMembership(compilation, plan.state.binding, plan.state.graph, plan.state.artifacts.mounts, mount, nonSelectedOccurrence); accepted {
		t.Fatal("direct allocation membership accepted a non-EffectSelected role")
	}
	secondAttachment, secondAttached := attachSelectedDirectAllocationMembership(compilation, plan.state.binding, plan.state.graph, plan.state.artifacts.mounts, mount, secondOccurrence)
	secondAttachmentID, secondAttachmentIDOK := secondAttachment.ContentID()
	if !secondAttached || !secondAttachment.Valid() || !secondAttachmentIDOK || secondAttachmentID == attachmentID {
		t.Fatal("direct allocation membership second observation fixture")
	}
	solver, solverOK := compilation.Solver()
	if !solverOK || solver == nil {
		t.Fatal("direct allocation membership solver")
	}
	// Use a fresh compilation with no prior membership attachment: this proves
	// closure itself rejects first-time attachment, rather than merely relying
	// on the duplicate observation-ID fence in the primary compilation.
	postSolverCompilation := publicationTransitionCompilationFor(t, plan, mount, occurrence)
	postSolver, postSolverOK := postSolverCompilation.Solver()
	if !postSolverOK || postSolver == nil {
		t.Fatal("direct allocation membership post-Solver fixture")
	}
	if _, accepted := attachSelectedDirectAllocationMembership(postSolverCompilation, plan.state.binding, plan.state.graph, plan.state.artifacts.mounts, mount, occurrence); accepted {
		t.Fatal("direct allocation membership attached after Solver closed compilation")
	}
	state, status := solver.Solve(context.Background())
	if status != engine.SolveComplete || state == nil {
		t.Fatalf("direct allocation membership solve=%v state=%t", status, state != nil)
	}

	authority, issuer, runtimeOK := plan.state.binding.RuntimeContexts().Begin(publicationPlacementPolicyID("direct-membership"))
	if !runtimeOK || authority == nil {
		t.Fatal("direct allocation membership runtime issuer")
	}
	defer authority.Close()
	processOwner, processOwnerOK := authority.ProcessOwner(publicationPlacementPolicyID("process"))
	actorOwner, actorOwnerOK := authority.ActorOwner(publicationPlacementPolicyID("actor"))
	processContext, processOK := authority.Process(processOwner)
	actorContext, actorOK := authority.Actor(actorOwner)
	if !processOwnerOK || !actorOwnerOK || !processOK || !actorOK {
		t.Fatal("direct allocation membership runtime contexts")
	}

	transition, transitionOK := candidates.At(0)
	proof, proofFailure := transition.ProveWithFailure(solver, state)
	subjectSelector, selectorOK := proof.SubjectSelector()
	if !transitionOK || proofFailure != callsite.PublicationTransitionProofFailureNone || !proof.Valid() || !proof.MatchesCompletion(solver, state) || !selectorOK {
		t.Fatalf("direct allocation membership transition proof failure=%d", proofFailure)
	}
	source, sourceOK := plan.state.binding.RuntimeContexts().Pack().MountedInputSemanticSource(mount, occurrence, subjectSelector)
	if !sourceOK {
		t.Fatal("direct allocation membership direct source")
	}
	requirement, direct := publicationDirectAllocationSubject(t, plan.state.binding, source)
	subject, subjectAvailability := issuer.BindRuntimeAllocationContext(mount, occurrence, subjectSelector, requirement, processContext)
	if subjectAvailability != packdomain.RuntimeAllocationContextBindingBound || !subject.Valid() {
		t.Fatal("direct allocation membership subject binding")
	}
	destination := packdomain.RuntimeDestinationContextBinding{}
	destinationPresent := false
	if contextSelector, contextRequired := proof.ContextSelector(); contextRequired {
		var destinationAvailability packdomain.RuntimeAllocationContextBindingAvailability
		destination, destinationAvailability = issuer.BindRuntimeDestinationContext(mount, occurrence, contextSelector, actorContext)
		if destinationAvailability != packdomain.RuntimeAllocationContextBindingBound || !destination.Valid() {
			t.Fatal("direct allocation membership destination binding")
		}
		destinationPresent = true
	}
	correlation, correlated := callsite.NewPublicationPlacementCorrelationCandidate(proof, subject, destination, destinationPresent)
	if !correlated || !correlation.Valid() {
		t.Fatal("direct allocation membership correlation")
	}
	membershipProof, proven := attachment.Prove(solver, state, proof, correlation, subject, direct)
	if !proven || !membershipProof.Valid() || membershipProof.Membership() != valuedomain.MembershipRecent && membershipProof.Membership() != valuedomain.MembershipSummary {
		t.Fatal("direct allocation membership proof")
	}
	secondCandidate, secondCandidateOK := candidates.At(1)
	secondProofAtSameCall, secondProofFailure := secondCandidate.ProveWithFailure(solver, state)
	secondSelector, secondSelectorOK := secondProofAtSameCall.SubjectSelector()
	if !secondCandidateOK || secondProofFailure != callsite.PublicationTransitionProofFailureNone || !secondProofAtSameCall.Valid() || !secondSelectorOK {
		t.Fatal("direct allocation membership second publication candidate")
	}
	secondSource, secondSourceOK := plan.state.binding.RuntimeContexts().Pack().MountedInputSemanticSource(mount, occurrence, secondSelector)
	if !secondSourceOK {
		t.Fatal("direct allocation membership second publication source")
	}
	secondRequirement, secondDirect := publicationDirectAllocationSubject(t, plan.state.binding, secondSource)
	secondSubject, secondSubjectAvailability := issuer.BindRuntimeAllocationContext(mount, occurrence, secondSelector, secondRequirement, processContext)
	if secondSubjectAvailability != packdomain.RuntimeAllocationContextBindingBound || !secondSubject.Valid() {
		t.Fatal("direct allocation membership second publication subject")
	}
	secondDestination := packdomain.RuntimeDestinationContextBinding{}
	secondDestinationPresent := false
	if contextSelector, contextRequired := secondProofAtSameCall.ContextSelector(); contextRequired {
		var secondDestinationAvailability packdomain.RuntimeAllocationContextBindingAvailability
		secondDestination, secondDestinationAvailability = issuer.BindRuntimeDestinationContext(mount, occurrence, contextSelector, actorContext)
		if secondDestinationAvailability != packdomain.RuntimeAllocationContextBindingBound || !secondDestination.Valid() {
			t.Fatal("direct allocation membership second publication destination")
		}
		secondDestinationPresent = true
	}
	secondCorrelation, secondCorrelated := callsite.NewPublicationPlacementCorrelationCandidate(secondProofAtSameCall, secondSubject, secondDestination, secondDestinationPresent)
	if !secondCorrelated {
		t.Fatal("direct allocation membership second publication correlation")
	}
	if secondMembership, accepted := attachment.Prove(solver, state, secondProofAtSameCall, secondCorrelation, secondSubject, secondDirect); !accepted || !secondMembership.Valid() {
		t.Fatal("direct allocation membership did not reuse one observation across publication candidates")
	}

	if _, accepted := attachment.Prove(nil, state, proof, correlation, subject, direct); accepted {
		t.Fatal("direct allocation membership accepted a foreign solver")
	}
	if _, accepted := attachment.Prove(solver, state, callsite.PublicationTransitionProof{}, correlation, subject, direct); accepted {
		t.Fatal("direct allocation membership accepted an absent transition target")
	}
	if _, accepted := attachment.Prove(solver, state, proof, callsite.PublicationPlacementCorrelationCandidate{}, subject, direct); accepted {
		t.Fatal("direct allocation membership accepted a spliced correlation")
	}
	if _, accepted := attachment.Prove(solver, state, proof, correlation, packdomain.RuntimeAllocationContextBinding{}, direct); accepted {
		t.Fatal("direct allocation membership accepted a spliced subject")
	}
	if _, accepted := attachment.Prove(solver, state, proof, correlation, subject, valuedomain.DirectAllocationSubject{}); accepted {
		t.Fatal("direct allocation membership accepted a spliced direct identity")
	}

	secondCompilation := publicationTransitionCompilationFor(t, plan, mount, secondOccurrence)
	secondCandidates, secondCandidatesOK := selectedEffectRule(plan.state.binding).AttachMountedPublicationCandidates(secondCompilation, plan.state.graph, plan.state.binding.EffectQuery(), mount, secondOccurrence)
	secondSolver, secondSolverOK := secondCompilation.Solver()
	if !secondCandidatesOK || !secondCandidates.Available() || secondCandidates.Count() == 0 || !secondSolverOK || secondSolver == nil {
		t.Fatal("direct allocation membership second-call fixture")
	}
	secondState, secondStatus := secondSolver.Solve(context.Background())
	secondTransition, secondTransitionOK := secondCandidates.At(0)
	secondProof, secondCallProofFailure := secondTransition.ProveWithFailure(secondSolver, secondState)
	secondCallSelector, secondCallSelectorOK := secondProof.SubjectSelector()
	if secondStatus != engine.SolveComplete || secondState == nil || !secondTransitionOK || secondCallProofFailure != callsite.PublicationTransitionProofFailureNone || !secondProof.Valid() || !secondCallSelectorOK {
		t.Fatal("direct allocation membership second-call proof")
	}
	secondCallSource, secondCallSourceOK := plan.state.binding.RuntimeContexts().Pack().MountedInputSemanticSource(mount, secondOccurrence, secondCallSelector)
	if !secondCallSourceOK {
		t.Fatal("direct allocation membership second-call source")
	}
	secondCallRequirement, secondCallDirect := publicationDirectAllocationSubject(t, plan.state.binding, secondCallSource)
	secondCallSubject, secondCallSubjectAvailability := issuer.BindRuntimeAllocationContext(mount, secondOccurrence, secondCallSelector, secondCallRequirement, processContext)
	if secondCallSubjectAvailability != packdomain.RuntimeAllocationContextBindingBound || !secondCallSubject.Valid() || !secondCallDirect.Valid() {
		t.Fatal("direct allocation membership second-call subject")
	}
	secondCallDestination := packdomain.RuntimeDestinationContextBinding{}
	secondCallDestinationPresent := false
	if contextSelector, contextRequired := secondProof.ContextSelector(); contextRequired {
		var secondCallDestinationAvailability packdomain.RuntimeAllocationContextBindingAvailability
		secondCallDestination, secondCallDestinationAvailability = issuer.BindRuntimeDestinationContext(mount, secondOccurrence, contextSelector, actorContext)
		if secondCallDestinationAvailability != packdomain.RuntimeAllocationContextBindingBound || !secondCallDestination.Valid() {
			t.Fatal("direct allocation membership second-call destination")
		}
		secondCallDestinationPresent = true
	}
	secondCallCorrelation, secondCallCorrelated := callsite.NewPublicationPlacementCorrelationCandidate(secondProof, secondCallSubject, secondCallDestination, secondCallDestinationPresent)
	secondCallMount, secondCallID, secondCallProvenanceOK := secondCallCorrelation.CallProvenance()
	if !secondCallCorrelated || !secondCallProvenanceOK || secondCallMount != mount || secondCallID != secondOccurrence || secondCallID == occurrence {
		t.Fatal("direct allocation membership second-call correlation provenance")
	}
	if _, accepted := attachment.Prove(secondSolver, secondState, secondProof, secondCallCorrelation, secondCallSubject, secondCallDirect); accepted {
		t.Fatal("direct allocation membership accepted a different selected call")
	}

	foreignCompilation, foreignMount, foreignOccurrence := publicationTransitionCompilation(t, foreignPlan)
	foreignCandidates, foreignCandidatesOK := selectedEffectRule(foreignPlan.state.binding).AttachMountedPublicationCandidates(foreignCompilation, foreignPlan.state.graph, foreignPlan.state.binding.EffectQuery(), foreignMount, foreignOccurrence)
	foreignSolver, foreignSolverOK := foreignCompilation.Solver()
	if !foreignCandidatesOK || !foreignCandidates.Available() || foreignCandidates.Count() == 0 || !foreignSolverOK || foreignSolver == nil {
		t.Fatal("direct allocation membership foreign fixture")
	}
	foreignState, foreignStatus := foreignSolver.Solve(context.Background())
	if foreignStatus != engine.SolveComplete || foreignState == nil {
		t.Fatal("direct allocation membership foreign solve")
	}
	foreignTransition, foreignTransitionOK := foreignCandidates.At(0)
	foreignProof, foreignProofFailure := foreignTransition.ProveWithFailure(foreignSolver, foreignState)
	if !foreignTransitionOK || foreignProofFailure != callsite.PublicationTransitionProofFailureNone || !foreignProof.Valid() {
		t.Fatal("direct allocation membership foreign transition")
	}
	foreignAuthority, foreignIssuer, foreignRuntimeOK := foreignPlan.state.binding.RuntimeContexts().Begin(publicationPlacementPolicyID("direct-membership"))
	if !foreignRuntimeOK || foreignAuthority == nil {
		t.Fatal("direct allocation membership foreign runtime issuer")
	}
	defer foreignAuthority.Close()
	foreignProcessOwner, foreignProcessOwnerOK := foreignAuthority.ProcessOwner(publicationPlacementPolicyID("process"))
	foreignProcessContext, foreignProcessOK := foreignAuthority.Process(foreignProcessOwner)
	foreignSelector, foreignSelectorOK := foreignProof.SubjectSelector()
	foreignSource, foreignSourceOK := foreignPlan.state.binding.RuntimeContexts().Pack().MountedInputSemanticSource(foreignMount, foreignOccurrence, foreignSelector)
	if !foreignProcessOwnerOK || !foreignProcessOK || !foreignSelectorOK || !foreignSourceOK {
		t.Fatal("direct allocation membership foreign runtime source")
	}
	foreignRequirement, foreignDirect := publicationDirectAllocationSubject(t, foreignPlan.state.binding, foreignSource)
	foreignSubject, foreignSubjectAvailability := foreignIssuer.BindRuntimeAllocationContext(foreignMount, foreignOccurrence, foreignSelector, foreignRequirement, foreignProcessContext)
	if foreignSubjectAvailability != packdomain.RuntimeAllocationContextBindingBound || !foreignSubject.Valid() || !foreignDirect.Valid() {
		t.Fatal("direct allocation membership foreign subject/direct")
	}
	localDirectID, localDirectIDOK := direct.ContentID()
	foreignDirectID, foreignDirectIDOK := foreignDirect.ContentID()
	if !localDirectIDOK || !foreignDirectIDOK || localDirectID != foreignDirectID {
		t.Fatal("direct allocation membership equal-content foreign direct identity")
	}
	if _, accepted := attachment.Prove(solver, state, proof, correlation, foreignSubject, direct); accepted {
		t.Fatal("direct allocation membership accepted a foreign subject")
	}
	if _, accepted := attachment.Prove(solver, state, proof, correlation, subject, foreignDirect); accepted {
		t.Fatal("direct allocation membership accepted an equal-content foreign direct identity")
	}
	if _, accepted := attachment.Prove(foreignSolver, state, proof, correlation, subject, direct); accepted {
		t.Fatal("direct allocation membership accepted foreign solver with local state")
	}
	if _, accepted := attachment.Prove(solver, foreignState, proof, correlation, subject, direct); accepted {
		t.Fatal("direct allocation membership accepted local solver with foreign state")
	}
	if _, accepted := attachment.Prove(foreignSolver, foreignState, proof, correlation, subject, direct); accepted {
		t.Fatal("direct allocation membership accepted a foreign solver/state")
	}
	if _, accepted := attachment.Prove(foreignSolver, foreignState, foreignProof, correlation, subject, direct); accepted {
		t.Fatal("direct allocation membership accepted a foreign transition/correlation join")
	}

	result, solveStatus := plan.Solve(context.Background())
	if solveStatus != AnalyzeComplete || result == nil {
		t.Fatal("direct allocation membership left the solve incomplete")
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
	candidates, candidatesOK := selectedEffectRule(plan.state.binding).AttachMountedPublicationCandidates(compilation, plan.state.graph, plan.state.binding.EffectQuery(), mount, occurrence)
	attachment, attached := attachSelectedDirectAllocationMembership(compilation, plan.state.binding, plan.state.graph, plan.state.artifacts.mounts, mount, occurrence)
	secondCandidates, secondCandidatesOK := selectedEffectRule(plan.state.binding).AttachMountedPublicationCandidates(compilation, plan.state.graph, plan.state.binding.EffectQuery(), mount, secondOccurrence)
	secondAttachment, secondAttached := attachSelectedDirectAllocationMembership(compilation, plan.state.binding, plan.state.graph, plan.state.artifacts.mounts, mount, secondOccurrence)
	if !candidatesOK || !candidates.Available() || !attached || !attachment.Valid() || !secondCandidatesOK || !secondCandidates.Available() || !secondAttached || !secondAttachment.Valid() {
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

	authority, issuer, authorityOK := plan.state.binding.RuntimeContexts().Begin(publicationPlacementPolicyID("allocation-context-event"))
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
		destinationContext, hasDestination := issued.event.DestinationContext()
		if !idOK || issued.event.SubjectContext().Class() != runtimeCase.class || !hasDestination || destinationContext.Class() != heapdomain.RuntimeAllocationContextActor ||
			issued.event.Kind() != target.PublicationEffectSendTransfer || issued.event.Escape() != target.PublicationEscapeSendTransfer ||
			issued.event.Mutability() != target.PublicationMutabilityCopyOnWrite || issued.event.DeclaredLifetime() != target.PublicationLifetimePreserve {
			t.Fatalf("publication allocation context class=%d evidence", runtimeCase.class)
		}
		authorization, authorizationOK := issued.event.SubjectContext().SharedAuthorizationID()
		if runtimeCase.class == heapdomain.RuntimeAllocationContextShared {
			if !authorizationOK || authorization != sharedContext.SharedAuthorizationID() {
				t.Fatal("publication allocation context shared authorization")
			}
			sharedIssue = issued
		} else if authorizationOK {
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
	releaseDestination, releaseHasDestination := releaseIssue.event.DestinationContext()
	_, releaseHasDestinationBinding := releaseIssue.event.DestinationBindingID()
	_, returnHasDestination := returnIssue.event.DestinationContext()
	if releaseHasDestination || releaseHasDestinationBinding || releaseDestination != (publication.AllocationRuntimeContext{}) ||
		releaseIssue.event.Kind() != target.PublicationEffectCloseRelease || releaseIssue.event.Escape() != target.PublicationEscapeNone ||
		releaseIssue.event.Mutability() != target.PublicationMutabilityPreserve || releaseIssue.event.DeclaredLifetime() != target.PublicationLifetimeRelease ||
		returnHasDestination || returnIssue.event.Kind() != target.PublicationEffectReturnEscape || returnIssue.event.Escape() != target.PublicationEscapeReturn ||
		returnIssue.event.Mutability() != target.PublicationMutabilityPreserve || returnIssue.event.DeclaredLifetime() != target.PublicationLifetimePreserve {
		t.Fatal("destination-free or declared-lifetime event evidence")
	}
	// These are the only lawful Phase3G-A conclusions: Release remains a
	// declared Target transition, and Shared remains a runtime context class.
	// Neither event contains a Dead/LastUse or SharedHeap projection.
	if releaseIssue.event.Membership() != valuedomain.MembershipRecent && releaseIssue.event.Membership() != valuedomain.MembershipSummary ||
		sharedIssue.event.SubjectContext().Class() != heapdomain.RuntimeAllocationContextShared {
		t.Fatal("publication allocation context event narrow conclusion")
	}

	if _, accepted := publication.NewAllocationContextEvent(attachment, solver, state, send, sendIssue.correlation, sendIssue.directAdmission, sendIssue.subject, sendIssue.direct, packdomain.RuntimeDestinationContextBinding{}, false); accepted {
		t.Fatal("context-required publication issued without destination")
	}
	if _, accepted := publication.NewAllocationContextEvent(attachment, solver, state, release, releaseIssue.correlation, releaseIssue.directAdmission, releaseIssue.subject, releaseIssue.direct, sendIssue.destination, true); accepted {
		t.Fatal("destination-free publication accepted an extra context")
	}
	contextSelector, contextSelectorOK := send.ContextSelector()
	wrongDestination, wrongDestinationAvailability := issuer.BindRuntimeDestinationContext(mount, occurrence, contextSelector, threadContext)
	if !contextSelectorOK || wrongDestinationAvailability != packdomain.RuntimeAllocationContextBindingBound || !wrongDestination.Valid() {
		t.Fatal("publication allocation context wrong destination fixture")
	}
	if _, accepted := publication.NewAllocationContextEvent(attachment, solver, state, send, sendIssue.correlation, sendIssue.directAdmission, sendIssue.subject, sendIssue.direct, wrongDestination, true); accepted {
		t.Fatal("publication allocation context accepted a destination splice")
	}
	if _, accepted := publication.NewAllocationContextEvent(attachment, solver, state, send, releaseIssue.correlation, releaseIssue.directAdmission, releaseIssue.subject, releaseIssue.direct, sendIssue.destination, true); accepted {
		t.Fatal("publication allocation context accepted a selector/correlation splice")
	}
	wrongRequirement := publicationDifferentAllocationRequirement(t, plan.state.binding, sendIssue.requirement)
	subjectSelector, subjectSelectorOK := send.SubjectSelector()
	wrongSubject, wrongSubjectAvailability := issuer.BindRuntimeAllocationContext(mount, occurrence, subjectSelector, wrongRequirement, processContext)
	if !subjectSelectorOK || wrongSubjectAvailability != packdomain.RuntimeAllocationContextBindingBound || !wrongSubject.Valid() {
		t.Fatal("publication allocation context wrong requirement fixture")
	}
	if _, accepted := publication.NewAllocationContextEvent(attachment, solver, state, send, sendIssue.correlation, sendIssue.directAdmission, wrongSubject, sendIssue.direct, sendIssue.destination, true); accepted {
		t.Fatal("publication allocation context accepted a requirement splice")
	}
	if _, accepted := publication.NewAllocationContextEvent(attachment, solver, state, send, sendIssue.correlation, sendIssue.directAdmission, sendIssue.subject, valuedomain.DirectAllocationSubject{}, sendIssue.destination, true); accepted {
		t.Fatal("publication allocation context accepted an absent direct receipt")
	}
	if _, accepted := publication.NewAllocationContextEvent(attachment, nil, state, send, sendIssue.correlation, sendIssue.directAdmission, sendIssue.subject, sendIssue.direct, sendIssue.destination, true); accepted {
		t.Fatal("publication allocation context accepted a foreign solver completion")
	}
	if _, accepted := publication.NewAllocationContextEvent(attachment, solver, nil, send, sendIssue.correlation, sendIssue.directAdmission, sendIssue.subject, sendIssue.direct, sendIssue.destination, true); accepted {
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
	if _, accepted := publication.NewAllocationContextEvent(attachment, solver, state, secondSend, secondIssue.correlation, secondIssue.directAdmission, secondIssue.subject, secondIssue.direct, secondIssue.destination, true); accepted {
		t.Fatal("publication allocation context accepted a cross-call membership attachment")
	}
	if _, accepted := publication.NewAllocationContextEvent(attachment, solver, state, send, sendIssue.correlation, sendIssue.directAdmission, sendIssue.subject, secondIssue.direct, sendIssue.destination, true); accepted {
		t.Fatal("publication allocation context accepted a direct-allocation splice")
	}

	foreignAuthority, foreignIssuer, foreignAuthorityOK := plan.state.binding.RuntimeContexts().Begin(publicationPlacementPolicyID("allocation-context-event"))
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
	if _, accepted := publication.NewAllocationContextEvent(attachment, solver, state, send, sendIssue.correlation, sendIssue.directAdmission, sendIssue.subject, sendIssue.direct, foreignDestination, true); accepted {
		t.Fatal("publication allocation context mixed equal-content authorities")
	}
	if _, accepted := publication.NewAllocationContextEvent(attachment, solver, state, send, sendIssue.correlation, sendIssue.directAdmission, foreignSubject, sendIssue.direct, sendIssue.destination, true); accepted {
		t.Fatal("publication allocation context mixed equal-content subject authority")
	}
	foreignAuthority.Close()
	if _, accepted := publication.NewAllocationContextEvent(attachment, solver, state, send, sendIssue.correlation, sendIssue.directAdmission, foreignSubject, sendIssue.direct, foreignDestination, true); accepted {
		t.Fatal("publication allocation context accepted a closed authority")
	}

	authority.Close()
	if _, accepted := publication.NewAllocationContextEvent(attachment, solver, state, send, sendIssue.correlation, sendIssue.directAdmission, sendIssue.subject, sendIssue.direct, sendIssue.destination, true); accepted {
		t.Fatal("publication allocation context issued after authority close")
	}
	if !sendIssue.event.Valid() || !releaseIssue.event.Valid() || !sharedIssue.event.Valid() {
		t.Fatal("detached publication allocation context event did not survive capability release")
	}
}
