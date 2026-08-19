package analysis

import (
	"context"
	"crypto/sha256"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/effect/callsite"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

func portableAnyType() schematype.Type {
	value, ok := schematype.NewPrimitive(schematype.PrimitiveAny)
	if !ok {
		panic("portable any type")
	}
	return value
}

func publicationTransitionSpec(callback, published, reverseEffects bool) target.Spec {
	sinkBinding := vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"sink"}}
	requireBinding := vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"require"}}
	publication := &vocabulary.PublicationEffectSpec{
		Kind: vocabulary.PublicationEffectSendTransfer, Subject: 0, Destination: vocabulary.PublicationDestinationValueFormal, Context: 1,
		Escape: vocabulary.PublicationEscapeSendTransfer, Mutability: vocabulary.PublicationMutabilityCopyOnWrite, Lifetime: vocabulary.PublicationLifetimePreserve,
	}
	effects := []vocabulary.EffectSpec{{Target: 2, ValueArgs: []vocabulary.ValueFormal{1, 0}}}
	if published {
		returnEffect := vocabulary.EffectSpec{Target: 2, ValueArgs: []vocabulary.ValueFormal{1, 0}, Publication: &vocabulary.PublicationEffectSpec{
			Kind: vocabulary.PublicationEffectReturnEscape, Subject: 0, Destination: vocabulary.PublicationDestinationNone,
			Escape: vocabulary.PublicationEscapeReturn, Mutability: vocabulary.PublicationMutabilityPreserve, Lifetime: vocabulary.PublicationLifetimePreserve,
		}}
		effects[0].Publication = publication
		effects = append(effects, returnEffect)
	}
	if reverseEffects {
		effects[0], effects[1] = effects[1], effects[0]
	}
	owner := vocabulary.OperationSpec{
		Bindings: []vocabulary.BindingSpec{sinkBinding},
		Input:    vocabulary.ValuesSpec{Fixed: []schematype.Type{portableAnyType(), portableAnyType()}, Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Occurrences: effects, Tail: vocabulary.RowClosed},
	}
	if callback {
		callbackEffect := vocabulary.EffectSpec{Target: 2, ValueArgs: []vocabulary.ValueFormal{1, 0}}
		if published {
			callbackEffect.Publication = &vocabulary.PublicationEffectSpec{
				Kind: vocabulary.PublicationEffectCallbackEscape, Subject: 0, Destination: vocabulary.PublicationDestinationNone,
				Escape: vocabulary.PublicationEscapeCallback, Mutability: vocabulary.PublicationMutabilityPreserve, Lifetime: vocabulary.PublicationLifetimePreserve,
			}
		}
		empty := vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}
		owner.Callbacks = []vocabulary.CallbackSpec{{
			Function: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, Admission: schematype.CallableAdmissionOrdinary, Arguments: empty,
			Outcomes:  []vocabulary.TerminalSpec{{Kind: kind.OutcomeNormal, Values: empty}, {Kind: kind.OutcomeReturn, Values: empty}, {Kind: kind.OutcomeThrow, Values: empty}, {Kind: kind.OutcomeYield, Values: empty}, {Kind: kind.OutcomeCancel, Values: empty}},
			Lifecycle: vocabulary.CallbackRetainedOptionalOnce, Effects: vocabulary.RowSpec{Occurrences: []vocabulary.EffectSpec{callbackEffect}, Tail: vocabulary.RowClosed},
		}}
	}
	return target.Spec{
		Semantics: domaincontract.NewSemantics(),
		InitialRoots: []vocabulary.InitialRootSpec{{
			Identity: "GlobalEnvRoot",
			Shape:    vocabulary.BootShapeSpec{Aggregate: vocabulary.BootAggregateTable, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"}},
		}},
		Operations: []vocabulary.OperationSpec{
			owner,
			{Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"effect-target"}}}, Input: vocabulary.ValuesSpec{Fixed: []schematype.Type{portableAnyType(), portableAnyType()}, Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}},
			// Call algebra requires Boundary's scoped loader authority even
			// though this normal-operation fixture never invokes require.
			{Bindings: []vocabulary.BindingSpec{requireBinding}, Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}},
		},
		InitialEntries: []vocabulary.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: vocabulary.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__link_absent"}, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueAbsent}, Mutability: vocabulary.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "sink"}, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueOperation, Operation: sinkBinding}, Mutability: vocabulary.InitialMutable},
		},
		InitialBindings: []vocabulary.InitialBindingSpec{
			{Name: "_G", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}},
			{Name: "__link_absent", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__link_absent"}},
			{Name: "sink", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "sink"}},
		},
	}
}

func publicationTransitionPlan(t testing.TB, callback, published, reverseEffects bool) *Plan {
	t.Helper()
	return publicationTransitionPlanSource(t, callback, published, reverseEffects, "return sink(function() end, 2)")
}

// publicationSecondCallDirectAllocationTransitionPlan reverses the selected
// effect ABI on purpose: ValueArgs {1,0} makes owner formal 1 the published
// subject, so each literal table allocation is the direct subject while formal
// 0 remains the callable sink receiver. Two calls give the fixture two
// distinct selected occurrences over the same geometry.
func publicationSecondCallDirectAllocationTransitionPlan(t testing.TB, callback, published, reverseEffects bool) *Plan {
	t.Helper()
	return publicationTransitionPlanSource(t, callback, published, reverseEffects, "sink(function() end, {})\nreturn sink(function() end, {})")
}

func publicationTransitionPlanSource(t testing.TB, callback, published, reverseEffects bool, text string) *Plan {
	t.Helper()
	publishedProgram, err := lower.Lower(lower.Source{Name: "publication_transition_law.lua", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	spec := publicationTransitionSpec(callback, published, reverseEffects)
	contract, err := target.Seal(&spec)
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "publication_transition_law", Program: publishedProgram}}})
	if err != nil {
		t.Fatal(err)
	}
	plan, status, diagnostics := CompileWithDiagnostics(linked)
	if status != CompileComplete || plan == nil || plan.state == nil || plan.state.binding == nil {
		t.Fatalf("compile publication transition fixture=%v diagnostics=%+v", status, diagnostics)
	}
	if runtimeDiagnostic, instantiated := plan.state.instantiateRuntimeTopology(); !instantiated || plan.state.committed.program == nil || len(plan.state.querySites) == 0 {
		t.Fatalf("publication transition runtime topology=%+v", runtimeDiagnostic)
	}
	return plan
}

func selectedCallEffectOccurrence(t testing.TB, plan *Plan) (identity.ContentID, identity.ContentID) {
	t.Helper()
	for _, mounted := range plan.state.artifacts.mounts {
		if !mounted.valid() {
			continue
		}
		for index := 0; index < mounted.snapshot.RulePlacementCount(); index++ {
			row, ok := mounted.snapshot.RulePlacementAt(index)
			if !ok || row.Key() != "effect-selected" || !row.OccurrenceID().Available() {
				continue
			}
			return mounted.moduleKey, row.OccurrenceID()
		}
	}
	t.Fatal("fixture has no selected CallEffect occurrence")
	return identity.ContentID{}, identity.ContentID{}
}

func selectedCallEffectOccurrences(t testing.TB, plan *Plan) (identity.ContentID, identity.ContentID, identity.ContentID) {
	t.Helper()
	for _, mounted := range plan.state.artifacts.mounts {
		if !mounted.valid() {
			continue
		}
		var first, second identity.ContentID
		for index := 0; index < mounted.snapshot.RulePlacementCount(); index++ {
			row, ok := mounted.snapshot.RulePlacementAt(index)
			if !ok || row.Key() != "effect-selected" || !row.OccurrenceID().Available() {
				continue
			}
			if !first.Available() {
				first = row.OccurrenceID()
				continue
			}
			if row.OccurrenceID() != first {
				second = row.OccurrenceID()
				break
			}
		}
		if mounted.moduleKey.Available() && first.Available() && second.Available() {
			return mounted.moduleKey, first, second
		}
	}
	t.Fatal("fixture has fewer than two selected CallEffect occurrences")
	return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}
}

func publicationTransitionCompilation(t testing.TB, plan *Plan) (*engine.ProgramConstruction, identity.ContentID, identity.ContentID) {
	t.Helper()
	mount, occurrence := selectedCallEffectOccurrence(t, plan)
	return publicationTransitionCompilationFor(t, plan, mount, occurrence), mount, occurrence
}

func publicationTransitionCompilationFor(t testing.TB, plan *Plan, mount, occurrence identity.ContentID) *engine.ProgramConstruction {
	t.Helper()
	binding := plan.state.binding
	compilation, compiled := plan.state.beginRuntimeConstruction()
	_, witnessOK := linkBootstrapWitness(plan.state, binding)
	sealed, sealedOK := linkArtifactRows(plan.state.artifacts.mounts)
	if !compiled || !witnessOK || !sealedOK || binding.Rules() == nil || !binding.Rules().AttachLinkMembers(compilation) || !binding.Rules().AttachMountedMembers(compilation, sealed) {
		t.Fatal("publication transition receipt compilation")
	}
	if _, attached := binding.AttachQueries(compilation, plan.state.querySites); !attached {
		t.Fatal("publication transition receipt compilation")
	}
	return compilation
}

func publicationPlacementPolicyID(label string) identity.ContentID {
	return identity.ContentID(sha256.Sum256([]byte("publication-placement-policy/" + label)))
}

func publicationPlacementRequirement(t testing.TB, binding *composite.ProgramBinding) heapdomain.AllocationRequirement {
	t.Helper()
	if binding == nil || !binding.RuntimeContexts().Valid() {
		t.Fatal("publication placement runtime binding owner")
	}
	for index := 0; index < binding.RuntimeContexts().Heap().KeyCount(); index++ {
		key, keyOK := binding.RuntimeContexts().Heap().KeyAt(index)
		if !keyOK || key.Kind() != heapdomain.RootAllocation {
			continue
		}
		requirement, requirementOK := binding.RuntimeContexts().Heap().AllocationRequirementForKey(key)
		if requirementOK {
			return requirement
		}
	}
	t.Fatal("publication placement allocation requirement")
	return heapdomain.AllocationRequirement{}
}

func publicationDifferentAllocationRequirement(t testing.TB, binding *composite.ProgramBinding, current heapdomain.AllocationRequirement) heapdomain.AllocationRequirement {
	t.Helper()
	if !current.Valid() || binding == nil || !binding.RuntimeContexts().Valid() {
		t.Fatal("publication different allocation requirement input")
	}
	for index := 0; index < binding.RuntimeContexts().Heap().KeyCount(); index++ {
		key, keyOK := binding.RuntimeContexts().Heap().KeyAt(index)
		if !keyOK || key.Kind() != heapdomain.RootAllocation {
			continue
		}
		candidate, candidateOK := binding.RuntimeContexts().Heap().AllocationRequirementForKey(key)
		if candidateOK && candidate.KeyID() != current.KeyID() {
			return candidate
		}
	}
	t.Fatal("publication different allocation requirement")
	return heapdomain.AllocationRequirement{}
}

func TestPublicationTransitionCandidateOwnerLaw(t *testing.T) {
	plan := publicationTransitionPlan(t, true, true, false)
	defer plan.Close()
	compilation, mount, occurrence := publicationTransitionCompilation(t, plan)
	foreignPlan := publicationTransitionPlan(t, true, true, true)
	defer foreignPlan.Close()
	foreignCompilation, foreignMount, foreignOccurrence := publicationTransitionCompilation(t, foreignPlan)
	if _, accepted := selectedEffectRule(plan.state.binding).AttachMountedPublicationCandidates(compilation, foreignPlan.state.binding.EffectQuery(), mount, occurrence); accepted {
		t.Fatal("foreign Effect query implementation entered candidate attachment")
	}
	if _, accepted := selectedEffectRule(plan.state.binding).AttachMountedPublicationCandidates(foreignCompilation, plan.state.binding.EffectQuery(), foreignMount, foreignOccurrence); accepted {
		t.Fatal("foreign graph entered selected CallEffect candidate attachment")
	}
	candidates, candidatesOK := selectedEffectRule(plan.state.binding).AttachMountedPublicationCandidates(compilation, plan.state.binding.EffectQuery(), mount, occurrence)
	if !candidatesOK || !candidates.Available() || candidates.Count() != 3 {
		t.Fatalf("selected publication candidate inventory=%d ok=%t", candidates.Count(), candidatesOK)
	}
	if _, duplicate := selectedEffectRule(plan.state.binding).AttachMountedPublicationCandidates(compilation, plan.state.binding.EffectQuery(), mount, occurrence); duplicate {
		t.Fatal("candidate attachment issued more than one observation for one selected occurrence")
	}
	if _, opaqueOK := opaqueEffectRule(plan.state.binding).AttachMountedPublicationCandidates(compilation, plan.state.binding.EffectQuery(), mount, occurrence); opaqueOK {
		t.Fatal("opaque CallEffect route issued publication candidates")
	}
	first, firstOK := candidates.At(0)
	second, secondOK := candidates.At(1)
	firstID, firstIDOK := first.ContentID()
	secondID, secondIDOK := second.ContentID()
	preSolve, preSolveFailure := first.ProveWithFailure(nil, nil)
	invalidCandidate := callsite.PublicationTransitionCandidate{}
	_, invalidCandidateFailure := invalidCandidate.ProveWithFailure(nil, nil)
	if !firstOK || !secondOK || !first.Available() || !second.Available() || !firstIDOK || !secondIDOK || firstID == secondID || preSolveFailure != callsite.PublicationTransitionProofFailureInvalidSolverState || preSolve.Valid() || invalidCandidateFailure != callsite.PublicationTransitionProofFailureInvalidCandidate {
		t.Fatal("candidate identity or pre-solve fence")
	}
	solver, _, solverOK := compilation.Seal()
	if !solverOK || solver == nil {
		t.Fatal("publication transition solver")
	}
	state, status := solver.Solve(context.Background())
	if status != engine.SolveComplete || state == nil {
		t.Fatalf("publication transition solve=%v solver=%t state=%t", status, solver != nil, state != nil)
	}
	seenSend, seenReturn, seenCallback := false, false, false
	for index := 0; index < candidates.Count(); index++ {
		candidate, candidateOK := candidates.At(index)
		proof, proofFailure := candidate.ProveWithFailure(solver, state)
		if !candidateOK || proofFailure != callsite.PublicationTransitionProofFailureNone || !proof.Valid() || proof.MountID() != mount || proof.CallOccurrenceID() != occurrence {
			candidateID, candidateIDOK := candidate.ContentID()
			t.Fatalf("completed exact observation did not prove candidate index=%d id=%x idOK=%t failure=%d", index, candidateID, candidateIDOK, proofFailure)
		}
		subject, subjectOK := proof.SubjectSelector()
		if !subjectOK {
			t.Fatal("publication proof lost Pack selector provenance")
		}
		switch proof.Role() {
		case effectfactor.PublicationAtomBindingOrdinary:
			switch proof.Kind() {
			case vocabulary.PublicationEffectSendTransfer:
				contextSelector, contextOK := proof.ContextSelector()
				if proof.Escape() != vocabulary.PublicationEscapeSendTransfer || proof.Mutability() != vocabulary.PublicationMutabilityCopyOnWrite || !contextOK || contextSelector == subject {
					t.Fatal("send publication consequence or context provenance")
				}
				seenSend = true
			case vocabulary.PublicationEffectReturnEscape:
				if proof.Escape() != vocabulary.PublicationEscapeReturn {
					t.Fatal("return publication consequence")
				}
				if _, contextOK := proof.ContextSelector(); contextOK {
					t.Fatal("destination-free return publication issued a context selector")
				}
				seenReturn = true
			default:
				t.Fatal("unexpected ordinary publication kind")
			}
		case effectfactor.PublicationAtomBindingCallback:
			if proof.Kind() != vocabulary.PublicationEffectCallbackEscape || proof.Escape() != vocabulary.PublicationEscapeCallback {
				t.Fatal("callback publication consequence")
			}
			if _, contextOK := proof.ContextSelector(); contextOK {
				t.Fatal("destination-free callback publication issued a context selector")
			}
			seenCallback = true
		default:
			t.Fatal("invalid publication role")
		}
	}
	if !seenSend || !seenReturn || !seenCallback {
		t.Fatal("ordinary/callback publication inventory")
	}
	if _, failure := first.ProveWithFailure(nil, state); failure != callsite.PublicationTransitionProofFailureInvalidSolverState {
		t.Fatal("candidate proved through a foreign solver")
	}
	foreignCandidates, foreignCandidatesOK := selectedEffectRule(foreignPlan.state.binding).AttachMountedPublicationCandidates(foreignCompilation, foreignPlan.state.binding.EffectQuery(), foreignMount, foreignOccurrence)
	foreignSolver, _, foreignSolverOK := foreignCompilation.Seal()
	if !foreignCandidatesOK || !foreignCandidates.Available() || !foreignSolverOK || foreignSolver == nil {
		t.Fatal("foreign candidate fixture")
	}
	if foreignCandidates.Count() != candidates.Count() {
		t.Fatal("independent reseal changed publication candidate inventory")
	}
	for index := 0; index < candidates.Count(); index++ {
		local, localOK := candidates.At(index)
		foreign, foreignOK := foreignCandidates.At(index)
		localID, localIDOK := local.ContentID()
		foreignID, foreignIDOK := foreign.ContentID()
		if !localOK || !foreignOK || !localIDOK || !foreignIDOK || localID != foreignID {
			t.Fatal("independent reseal changed canonical publication candidate order")
		}
	}
	foreignState, foreignStatus := foreignSolver.Solve(context.Background())
	if foreignStatus != engine.SolveComplete || foreignState == nil {
		t.Fatalf("foreign publication transition solve=%v state=%t", foreignStatus, foreignState != nil)
	}
	if _, failure := first.ProveWithFailure(foreignSolver, state); failure != callsite.PublicationTransitionProofFailureUnreadableObservation {
		t.Fatal("candidate proved through a foreign solver with local state")
	}
	if _, failure := first.ProveWithFailure(solver, foreignState); failure != callsite.PublicationTransitionProofFailureUnreadableObservation {
		t.Fatal("candidate proved through local solver and foreign state")
	}
}

// TestPublicationPlacementCorrelationPlanOwnerLaw is the first positive
// end-to-end admission of the Phase3B static correlation. It uses the actual
// solved Effect observation, the exact Pack/Heap pair retained by this Plan,
// and runtime-issued process/actor contexts. It does not derive a placement
// lattice row: alias, freeze/COW, lifetime and residence remain unbound.
func TestPublicationPlacementCorrelationPlanOwnerLaw(t *testing.T) {
	plan := publicationTransitionPlan(t, true, true, false)
	defer plan.Close()
	compilation, mount, occurrence := publicationTransitionCompilation(t, plan)
	candidates, candidatesOK := selectedEffectRule(plan.state.binding).AttachMountedPublicationCandidates(compilation, plan.state.binding.EffectQuery(), mount, occurrence)
	solver, _, solverOK := compilation.Seal()
	if !candidatesOK || !candidates.Available() || candidates.Count() == 0 || !solverOK || solver == nil {
		t.Fatal("publication placement candidate fixture")
	}
	state, status := solver.Solve(context.Background())
	if status != engine.SolveComplete || state == nil {
		t.Fatalf("publication placement solve=%v state=%t", status, state != nil)
	}

	authority, issuer, runtimeOK := plan.state.binding.RuntimeContexts().Begin(publicationPlacementPolicyID("plan-owner"))
	if !runtimeOK || authority == nil {
		t.Fatal("publication placement runtime issuer")
	}
	defer authority.Close()
	processOwner, processOwnerOK := authority.ProcessOwner(publicationPlacementPolicyID("process"))
	actorOwner, actorOwnerOK := authority.ActorOwner(publicationPlacementPolicyID("actor"))
	processContext, processOK := authority.Process(processOwner)
	actorContext, actorOK := authority.Actor(actorOwner)
	if !processOwnerOK || !actorOwnerOK || !processOK || !actorOK {
		t.Fatal("publication placement runtime contexts")
	}
	requirement := publicationPlacementRequirement(t, plan.state.binding)
	seenDestination, seenDestinationFree := false, false
	for index := 0; index < candidates.Count(); index++ {
		transition, transitionOK := candidates.At(index)
		proof, proofFailure := transition.ProveWithFailure(solver, state)
		subjectSelector, subjectOK := proof.SubjectSelector()
		if !transitionOK || proofFailure != callsite.PublicationTransitionProofFailureNone || !proof.Valid() || !subjectOK {
			t.Fatalf("publication placement proof index=%d failure=%d", index, proofFailure)
		}
		subject, subjectAvailability := issuer.BindRuntimeAllocationContext(mount, occurrence, subjectSelector, requirement, processContext)
		if subjectAvailability != packdomain.RuntimeAllocationContextBindingBound || !subject.Valid() {
			t.Fatalf("publication placement subject binding index=%d", index)
		}

		destination := packdomain.RuntimeDestinationContextBinding{}
		destinationPresent := false
		if contextSelector, contextRequired := proof.ContextSelector(); contextRequired {
			var destinationAvailability packdomain.RuntimeAllocationContextBindingAvailability
			destination, destinationAvailability = issuer.BindRuntimeDestinationContext(mount, occurrence, contextSelector, actorContext)
			if destinationAvailability != packdomain.RuntimeAllocationContextBindingBound || !destination.Valid() {
				t.Fatalf("publication placement destination binding index=%d", index)
			}
			destinationPresent = true
		}
		correlation, correlated := callsite.NewPublicationPlacementCorrelationCandidate(proof, subject, destination, destinationPresent)
		if !correlated || !correlation.Valid() || correlation.Kind() != proof.Kind() || correlation.Escape() != proof.Escape() || correlation.Mutability() != proof.Mutability() || correlation.Lifetime() != proof.Lifetime() {
			t.Fatalf("publication placement correlation index=%d", index)
		}
		if subjectID, subjectIDOK := correlation.SubjectBindingID(); !subjectIDOK || subjectID != subject.ID() {
			t.Fatalf("publication placement subject correlation index=%d", index)
		}
		if destinationPresent {
			if destinationID, destinationIDOK := correlation.DestinationBindingID(); !destinationIDOK || destinationID != destination.ID() {
				t.Fatalf("publication placement destination correlation index=%d", index)
			}
			seenDestination = true
		} else {
			if _, destinationIDOK := correlation.DestinationBindingID(); destinationIDOK {
				t.Fatalf("destination-free publication acquired context index=%d", index)
			}
			seenDestinationFree = true
		}
	}
	if !seenDestination || !seenDestinationFree {
		t.Fatal("publication placement did not cover destination and destination-free transitions")
	}
}

func TestPublicationTransitionCandidatesOmitGenericEffects(t *testing.T) {
	plan := publicationTransitionPlan(t, false, false, false)
	defer plan.Close()
	compilation, mount, occurrence := publicationTransitionCompilation(t, plan)
	candidates, ok := selectedEffectRule(plan.state.binding).AttachMountedPublicationCandidates(compilation, plan.state.binding.EffectQuery(), mount, occurrence)
	if !ok || !candidates.Available() || candidates.Count() != 0 {
		t.Fatal("generic selected effect issued a publication candidate")
	}
}
