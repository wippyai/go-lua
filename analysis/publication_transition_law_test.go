package analysis

import (
	"context"
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect/callsite"
	effectfactor "github.com/wippyai/go-lua/analysis/domain/effect/factor"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
)

func publicationTransitionSpec(callback, published, reverseEffects bool) target.Spec {
	sinkBinding := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"sink"}}
	requireBinding := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"require"}}
	publication := &target.PublicationEffectSpec{
		Kind: target.PublicationEffectSendTransfer, Subject: 0, Destination: target.PublicationDestinationValueFormal, Context: 1,
		Escape: target.PublicationEscapeSendTransfer, Mutability: target.PublicationMutabilityCopyOnWrite, Lifetime: target.PublicationLifetimePreserve,
	}
	effects := []target.EffectSpec{{Target: 2, ValueArgs: []target.ValueFormal{1, 0}}}
	if published {
		returnEffect := target.EffectSpec{Target: 2, ValueArgs: []target.ValueFormal{1, 0}, Publication: &target.PublicationEffectSpec{
			Kind: target.PublicationEffectReturnEscape, Subject: 0, Destination: target.PublicationDestinationNone,
			Escape: target.PublicationEscapeReturn, Mutability: target.PublicationMutabilityPreserve, Lifetime: target.PublicationLifetimePreserve,
		}}
		effects[0].Publication = publication
		effects = append(effects, returnEffect)
	}
	if reverseEffects {
		effects[0], effects[1] = effects[1], effects[0]
	}
	owner := target.OperationSpec{
		Bindings: []target.BindingSpec{sinkBinding},
		Input:    target.ValuesSpec{Fixed: []typ.Type{typ.Any, typ.Any}, Tail: target.ValuesClosed},
		Outcomes: []target.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
		Effects:  target.RowSpec{Occurrences: effects, Tail: target.RowClosed},
	}
	if callback {
		callbackEffect := target.EffectSpec{Target: 2, ValueArgs: []target.ValueFormal{1, 0}}
		if published {
			callbackEffect.Publication = &target.PublicationEffectSpec{
				Kind: target.PublicationEffectCallbackEscape, Subject: 0, Destination: target.PublicationDestinationNone,
				Escape: target.PublicationEscapeCallback, Mutability: target.PublicationMutabilityPreserve, Lifetime: target.PublicationLifetimePreserve,
			}
		}
		empty := target.ValuesSpec{Tail: target.ValuesClosed}
		owner.Callbacks = []target.CallbackSpec{{
			Function: target.InputSource{Kind: target.InputSourceValueFormal}, Admission: target.OrdinaryCallable, Arguments: empty,
			Outcomes:  []target.TerminalSpec{{Kind: kind.OutcomeNormal, Values: empty}, {Kind: kind.OutcomeReturn, Values: empty}, {Kind: kind.OutcomeThrow, Values: empty}, {Kind: kind.OutcomeYield, Values: empty}, {Kind: kind.OutcomeCancel, Values: empty}},
			Lifecycle: target.CallbackRetainedOptionalOnce, Effects: target.RowSpec{Occurrences: []target.EffectSpec{callbackEffect}, Tail: target.RowClosed},
		}}
	}
	return target.Spec{
		InitialRoots: []target.InitialRootSpec{{
			Identity: "GlobalEnvRoot",
			Shape:    target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}},
		}},
		Operations: []target.OperationSpec{
			owner,
			{Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"effect-target"}}}, Input: target.ValuesSpec{Fixed: []typ.Type{typ.Any, typ.Any}, Tail: target.ValuesClosed}, Outcomes: []target.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}}, Effects: target.RowSpec{Tail: target.RowClosed}},
			// Call algebra requires Boundary's scoped loader authority even
			// though this normal-operation fixture never invokes require.
			{Bindings: []target.BindingSpec{requireBinding}, Input: target.ValuesSpec{Tail: target.ValuesClosed}, Outcomes: []target.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}}, Effects: target.RowSpec{Tail: target.RowClosed}},
		},
		InitialEntries: []target.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__link_absent"}, Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "sink"}, Value: target.InitialValueSpec{Kind: target.InitialValueOperation, Operation: sinkBinding}, Mutability: target.InitialMutable},
		},
		InitialBindings: []target.InitialBindingSpec{
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

// publicationDirectAllocationTransitionPlan reverses the selected effect ABI
// on purpose: ValueArgs {1,0} makes owner formal 1 the published subject, so
// this exact table allocation is the direct subject while formal 0 remains the
// callable sink receiver.
func publicationDirectAllocationTransitionPlan(t testing.TB, callback, published, reverseEffects bool) *Plan {
	t.Helper()
	return publicationTransitionPlanSource(t, callback, published, reverseEffects, "return sink(function() end, {})")
}

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
	if runtimeDiagnostic, instantiated := plan.state.instantiateRuntimeTopology(); !instantiated || plan.state.graph == nil || plan.state.queryPlan == nil {
		t.Fatalf("publication transition runtime topology=%+v", runtimeDiagnostic)
	}
	return plan
}

func selectedCallEffectOccurrence(t testing.TB, plan *Plan) (identity.ContentID, identity.ContentID) {
	t.Helper()
	for _, mounted := range plan.state.artifacts.mounts {
		if mounted.artifact == nil || mounted.artifact.RuleOccurrenceCount(programartifact.RuleRoleEffectSelected) == 0 {
			continue
		}
		row, ok := mounted.artifact.RuleOccurrenceAt(programartifact.RuleRoleEffectSelected, 0)
		if !ok || !row.ID().Available() {
			t.Fatal("selected CallEffect occurrence")
		}
		return mounted.moduleKey, row.ID()
	}
	t.Fatal("fixture has no selected CallEffect occurrence")
	return identity.ContentID{}, identity.ContentID{}
}

func selectedCallEffectOccurrences(t testing.TB, plan *Plan) (identity.ContentID, identity.ContentID, identity.ContentID) {
	t.Helper()
	for _, mounted := range plan.state.artifacts.mounts {
		if mounted.artifact == nil || mounted.artifact.RuleOccurrenceCount(programartifact.RuleRoleEffectSelected) < 2 {
			continue
		}
		first, firstOK := mounted.artifact.RuleOccurrenceAt(programartifact.RuleRoleEffectSelected, 0)
		second, secondOK := mounted.artifact.RuleOccurrenceAt(programartifact.RuleRoleEffectSelected, 1)
		if !firstOK || !secondOK || !mounted.moduleKey.Available() || !first.ID().Available() || !second.ID().Available() || first.ID() == second.ID() {
			t.Fatal("distinct selected CallEffect occurrences")
		}
		return mounted.moduleKey, first.ID(), second.ID()
	}
	t.Fatal("fixture has fewer than two selected CallEffect occurrences")
	return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}
}

func publicationTransitionCompilation(t testing.TB, plan *Plan) (*engine.ReceiptCompilation, identity.ContentID, identity.ContentID) {
	t.Helper()
	mount, occurrence := selectedCallEffectOccurrence(t, plan)
	return publicationTransitionCompilationFor(t, plan, mount, occurrence), mount, occurrence
}

func publicationTransitionCompilationFor(t testing.TB, plan *Plan, mount, occurrence identity.ContentID) *engine.ReceiptCompilation {
	t.Helper()
	binding, graph := plan.state.binding, plan.state.graph
	compilation, compiled := engine.BeginReceiptTopologyCompilation(binding.binding, graph)
	valueIDs, heapIDs, _, witnessOK := linkBootstrapWitness(plan.state, binding)
	if !compiled || !witnessOK || !binding.attachLinkBootstrapMembers(compilation, graph, valueIDs, heapIDs) || !binding.attachArtifactRuleMembers(compilation, graph, plan.state.artifacts.mounts) || !plan.state.queryPlan.Attach(compilation, graph, binding) {
		t.Fatal("publication transition receipt compilation")
	}
	return compilation
}

func publicationPlacementPolicyID(label string) identity.ContentID {
	return identity.ContentID(sha256.Sum256([]byte("publication-placement-policy/" + label)))
}

func publicationPlacementRequirement(t testing.TB, binding *programBinding) heapdomain.AllocationRequirement {
	t.Helper()
	if binding == nil || !binding.runtimeContexts.valid() {
		t.Fatal("publication placement runtime binding owner")
	}
	for index := 0; index < binding.runtimeContexts.heap.KeyCount(); index++ {
		key, keyOK := binding.runtimeContexts.heap.KeyAt(index)
		if !keyOK || key.Kind() != heapdomain.RootAllocation {
			continue
		}
		requirement, requirementOK := binding.runtimeContexts.heap.AllocationRequirementForKey(key)
		if requirementOK {
			return requirement
		}
	}
	t.Fatal("publication placement allocation requirement")
	return heapdomain.AllocationRequirement{}
}

func publicationDirectAllocationSubject(t testing.TB, binding *programBinding, source packdomain.SemanticSource) (heapdomain.AllocationRequirement, valuedomain.DirectAllocationSubject) {
	t.Helper()
	if binding == nil || binding.value == nil || binding.value.Schema() == nil || !binding.runtimeContexts.valid() || !source.Available() {
		t.Fatal("publication direct allocation subject input")
	}
	values := binding.value.Schema()
	for index := 0; index < binding.runtimeContexts.heap.KeyCount(); index++ {
		key, keyOK := binding.runtimeContexts.heap.KeyAt(index)
		if !keyOK || key.Kind() != heapdomain.RootAllocation {
			continue
		}
		allocation, allocationOK := values.AllocationResultFor(key)
		direct, directOK := values.DirectAllocationSubjectFor(binding.runtimeContexts.pack, source, allocation)
		requirement, requirementOK := binding.runtimeContexts.heap.AllocationRequirementForKey(key)
		if allocationOK && directOK && requirementOK {
			return requirement, direct
		}
	}
	t.Fatal("publication direct allocation subject")
	return heapdomain.AllocationRequirement{}, valuedomain.DirectAllocationSubject{}
}

func publicationDifferentAllocationRequirement(t testing.TB, binding *programBinding, current heapdomain.AllocationRequirement) heapdomain.AllocationRequirement {
	t.Helper()
	if !current.Valid() || binding == nil || !binding.runtimeContexts.valid() {
		t.Fatal("publication different allocation requirement input")
	}
	for index := 0; index < binding.runtimeContexts.heap.KeyCount(); index++ {
		key, keyOK := binding.runtimeContexts.heap.KeyAt(index)
		if !keyOK || key.Kind() != heapdomain.RootAllocation {
			continue
		}
		candidate, candidateOK := binding.runtimeContexts.heap.AllocationRequirementForKey(key)
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
	if _, accepted := plan.state.binding.selectedEffectRule().AttachMountedPublicationCandidates(compilation, plan.state.graph, foreignPlan.state.binding.effectQuery, mount, occurrence); accepted {
		t.Fatal("foreign Effect query implementation entered candidate attachment")
	}
	if _, accepted := plan.state.binding.selectedEffectRule().AttachMountedPublicationCandidates(foreignCompilation, foreignPlan.state.graph, plan.state.binding.effectQuery, foreignMount, foreignOccurrence); accepted {
		t.Fatal("foreign graph entered selected CallEffect candidate attachment")
	}
	candidates, candidatesOK := plan.state.binding.selectedEffectRule().AttachMountedPublicationCandidates(compilation, plan.state.graph, plan.state.binding.effectQuery, mount, occurrence)
	if !candidatesOK || !candidates.Available() || candidates.Count() != 3 {
		t.Fatalf("selected publication candidate inventory=%d ok=%t", candidates.Count(), candidatesOK)
	}
	if _, duplicate := plan.state.binding.selectedEffectRule().AttachMountedPublicationCandidates(compilation, plan.state.graph, plan.state.binding.effectQuery, mount, occurrence); duplicate {
		t.Fatal("candidate attachment issued more than one observation for one selected occurrence")
	}
	if _, opaqueOK := plan.state.binding.opaqueEffectRule().AttachMountedPublicationCandidates(compilation, plan.state.graph, plan.state.binding.effectQuery, mount, occurrence); opaqueOK {
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
	solver, solverOK := compilation.Solver()
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
			case target.PublicationEffectSendTransfer:
				contextSelector, contextOK := proof.ContextSelector()
				if proof.Escape() != target.PublicationEscapeSendTransfer || proof.Mutability() != target.PublicationMutabilityCopyOnWrite || !contextOK || contextSelector == subject {
					t.Fatal("send publication consequence or context provenance")
				}
				seenSend = true
			case target.PublicationEffectReturnEscape:
				if proof.Escape() != target.PublicationEscapeReturn {
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
			if proof.Kind() != target.PublicationEffectCallbackEscape || proof.Escape() != target.PublicationEscapeCallback {
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
	foreignCandidates, foreignCandidatesOK := foreignPlan.state.binding.selectedEffectRule().AttachMountedPublicationCandidates(foreignCompilation, foreignPlan.state.graph, foreignPlan.state.binding.effectQuery, foreignMount, foreignOccurrence)
	foreignSolver, foreignSolverOK := foreignCompilation.Solver()
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
	candidates, candidatesOK := plan.state.binding.selectedEffectRule().AttachMountedPublicationCandidates(compilation, plan.state.graph, plan.state.binding.effectQuery, mount, occurrence)
	solver, solverOK := compilation.Solver()
	if !candidatesOK || !candidates.Available() || candidates.Count() == 0 || !solverOK || solver == nil {
		t.Fatal("publication placement candidate fixture")
	}
	state, status := solver.Solve(context.Background())
	if status != engine.SolveComplete || state == nil {
		t.Fatalf("publication placement solve=%v state=%t", status, state != nil)
	}

	authority, issuer, runtimeOK := plan.state.binding.runtimeContexts.Begin(publicationPlacementPolicyID("plan-owner"))
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

// TestPublicationDirectAllocationSubjectPlanOwnerLaw is the first real
// Phase3C identity admission. The fixture deliberately passes a literal table
// as the second sink input: the selected publication ABI reverses ValueArgs,
// so its subject formal resolves directly to that table's allocation root.
// This proves identity only. The Heap authority remains explicitly
// FactorsUnbound and the public Result retains no placement projection.
func TestPublicationDirectAllocationSubjectPlanOwnerLaw(t *testing.T) {
	plan := publicationSecondCallDirectAllocationTransitionPlan(t, true, true, false)
	defer plan.Close()
	// The independent comparison must have exactly the same artifact geometry
	// as this two-call Plan. Its scalar direct receipt should therefore match,
	// while its private Value/Pack/Heap owners remain foreign.
	foreignPlan := publicationSecondCallDirectAllocationTransitionPlan(t, true, true, false)
	defer foreignPlan.Close()
	mount, occurrence, secondOccurrence := selectedCallEffectOccurrences(t, plan)
	compilation := publicationTransitionCompilationFor(t, plan, mount, occurrence)
	candidates, candidatesOK := plan.state.binding.selectedEffectRule().AttachMountedPublicationCandidates(compilation, plan.state.graph, plan.state.binding.effectQuery, mount, occurrence)
	solver, solverOK := compilation.Solver()
	if !candidatesOK || !candidates.Available() || candidates.Count() == 0 || !solverOK || solver == nil {
		t.Fatal("publication direct allocation candidate fixture")
	}
	state, status := solver.Solve(context.Background())
	if status != engine.SolveComplete || state == nil {
		t.Fatalf("publication direct allocation solve=%v state=%t", status, state != nil)
	}
	secondCompilation := publicationTransitionCompilationFor(t, plan, mount, secondOccurrence)
	secondCandidates, secondCandidatesOK := plan.state.binding.selectedEffectRule().AttachMountedPublicationCandidates(secondCompilation, plan.state.graph, plan.state.binding.effectQuery, mount, secondOccurrence)
	secondSolver, secondSolverOK := secondCompilation.Solver()
	if !secondCandidatesOK || !secondCandidates.Available() || secondCandidates.Count() == 0 || !secondSolverOK || secondSolver == nil {
		t.Fatal("publication second-call candidate fixture")
	}
	secondState, secondStatus := secondSolver.Solve(context.Background())
	if secondStatus != engine.SolveComplete || secondState == nil {
		t.Fatalf("publication second-call solve=%v state=%t", secondStatus, secondState != nil)
	}
	foreignCompilation, foreignMount, foreignOccurrence := publicationTransitionCompilation(t, foreignPlan)
	foreignCandidates, foreignCandidatesOK := foreignPlan.state.binding.selectedEffectRule().AttachMountedPublicationCandidates(foreignCompilation, foreignPlan.state.graph, foreignPlan.state.binding.effectQuery, foreignMount, foreignOccurrence)
	foreignSolver, foreignSolverOK := foreignCompilation.Solver()
	if !foreignCandidatesOK || !foreignCandidates.Available() || foreignCandidates.Count() == 0 || !foreignSolverOK || foreignSolver == nil {
		t.Fatal("publication foreign direct candidate fixture")
	}
	foreignState, foreignStatus := foreignSolver.Solve(context.Background())
	if foreignStatus != engine.SolveComplete || foreignState == nil {
		t.Fatalf("publication foreign direct solve=%v state=%t", foreignStatus, foreignState != nil)
	}

	authority, issuer, runtimeOK := plan.state.binding.runtimeContexts.Begin(publicationPlacementPolicyID("direct-identity"))
	if !runtimeOK || authority == nil {
		t.Fatal("publication direct allocation runtime issuer")
	}
	defer authority.Close()
	processOwner, processOwnerOK := authority.ProcessOwner(publicationPlacementPolicyID("process"))
	actorOwner, actorOwnerOK := authority.ActorOwner(publicationPlacementPolicyID("actor"))
	processContext, processOK := authority.Process(processOwner)
	actorContext, actorOK := authority.Actor(actorOwner)
	if !processOwnerOK || !actorOwnerOK || !processOK || !actorOK {
		t.Fatal("publication direct allocation runtime contexts")
	}

	admitted := 0
	var firstCorrelation callsite.PublicationPlacementCorrelationCandidate
	var firstSubject packdomain.RuntimeAllocationContextBinding
	var firstDirect valuedomain.DirectAllocationSubject
	for index := 0; index < candidates.Count(); index++ {
		transition, transitionOK := candidates.At(index)
		proof, proofFailure := transition.ProveWithFailure(solver, state)
		subjectSelector, subjectOK := proof.SubjectSelector()
		if !transitionOK || proofFailure != callsite.PublicationTransitionProofFailureNone || !proof.Valid() || !subjectOK {
			t.Fatalf("publication direct allocation proof index=%d failure=%d", index, proofFailure)
		}
		source, sourceOK := plan.state.binding.runtimeContexts.pack.MountedInputSemanticSource(mount, occurrence, subjectSelector)
		if !sourceOK {
			t.Fatalf("publication direct allocation source index=%d", index)
		}
		requirement, direct := publicationDirectAllocationSubject(t, plan.state.binding, source)
		subject, subjectAvailability := issuer.BindRuntimeAllocationContext(mount, occurrence, subjectSelector, requirement, processContext)
		if subjectAvailability != packdomain.RuntimeAllocationContextBindingBound || !subject.Valid() {
			t.Fatalf("publication direct allocation subject binding index=%d", index)
		}

		destination := packdomain.RuntimeDestinationContextBinding{}
		destinationPresent := false
		if contextSelector, contextRequired := proof.ContextSelector(); contextRequired {
			var destinationAvailability packdomain.RuntimeAllocationContextBindingAvailability
			destination, destinationAvailability = issuer.BindRuntimeDestinationContext(mount, occurrence, contextSelector, actorContext)
			if destinationAvailability != packdomain.RuntimeAllocationContextBindingBound || !destination.Valid() {
				t.Fatalf("publication direct allocation destination binding index=%d", index)
			}
			destinationPresent = true
		}
		correlation, correlated := callsite.NewPublicationPlacementCorrelationCandidate(proof, subject, destination, destinationPresent)
		if !correlated || !correlation.Valid() {
			t.Fatalf("publication direct allocation correlation index=%d", index)
		}
		identity, identified := callsite.NewPublicationDirectAllocationSubject(correlation, subject, direct)
		if !identified || !identity.Valid() {
			t.Fatalf("publication direct allocation identity index=%d", index)
		}
		directID, directOK := direct.ContentID()
		identityDirectID, identityDirectOK := identity.DirectAllocationSubjectID()
		if !directOK || !identityDirectOK || directID != identityDirectID {
			t.Fatalf("publication direct allocation scalar identity index=%d", index)
		}
		mounted, mountedOK := subject.MountedAllocation()
		unavailable, unavailableOK := authority.Unavailable(mounted)
		if !mountedOK || !unavailableOK || !unavailable.Valid() || unavailable.Availability() != heapdomain.PlacementFactorsUnbound {
			t.Fatalf("publication direct allocation availability index=%d", index)
		}
		wrongRequirement := publicationDifferentAllocationRequirement(t, plan.state.binding, requirement)
		wrongSubject, wrongSubjectAvailability := issuer.BindRuntimeAllocationContext(mount, occurrence, subjectSelector, wrongRequirement, processContext)
		if wrongSubjectAvailability != packdomain.RuntimeAllocationContextBindingBound || !wrongSubject.Valid() {
			t.Fatalf("publication direct allocation wrong requirement setup index=%d", index)
		}
		if _, spliced := callsite.NewPublicationDirectAllocationSubject(correlation, wrongSubject, direct); spliced {
			t.Fatalf("publication direct allocation correlation/subject splice index=%d", index)
		}
		wrongCorrelation, wrongCorrelated := callsite.NewPublicationPlacementCorrelationCandidate(proof, wrongSubject, destination, destinationPresent)
		if !wrongCorrelated || !wrongCorrelation.Valid() {
			t.Fatalf("publication direct allocation wrong correlation setup index=%d", index)
		}
		if _, spliced := callsite.NewPublicationDirectAllocationSubject(wrongCorrelation, wrongSubject, direct); spliced {
			t.Fatalf("publication direct allocation requirement splice index=%d", index)
		}
		if admitted == 0 {
			firstCorrelation, firstSubject, firstDirect = correlation, subject, direct
		}
		admitted++
	}
	if admitted == 0 {
		t.Fatal("publication direct allocation did not admit a direct allocation")
	}
	foreignTransition, foreignTransitionOK := foreignCandidates.At(0)
	foreignProof, foreignProofFailure := foreignTransition.ProveWithFailure(foreignSolver, foreignState)
	foreignSelector, foreignSelectorOK := foreignProof.SubjectSelector()
	if !foreignTransitionOK || foreignProofFailure != callsite.PublicationTransitionProofFailureNone || !foreignProof.Valid() || !foreignSelectorOK {
		t.Fatal("publication foreign direct proof")
	}
	foreignSource, foreignSourceOK := foreignPlan.state.binding.runtimeContexts.pack.MountedInputSemanticSource(foreignMount, foreignOccurrence, foreignSelector)
	if !foreignSourceOK {
		t.Fatal("publication foreign direct source")
	}
	_, foreignDirect := publicationDirectAllocationSubject(t, foreignPlan.state.binding, foreignSource)
	localDirectID, localDirectIDOK := firstDirect.ContentID()
	foreignDirectID, foreignDirectIDOK := foreignDirect.ContentID()
	if !localDirectIDOK || !foreignDirectIDOK || localDirectID != foreignDirectID {
		t.Fatal("equal-content foreign direct semantic identity")
	}
	if _, accepted := callsite.NewPublicationDirectAllocationSubject(firstCorrelation, firstSubject, foreignDirect); accepted {
		t.Fatal("foreign equal-content direct identity entered local live subject")
	}
	secondTransition, secondTransitionOK := secondCandidates.At(0)
	secondProof, secondProofFailure := secondTransition.ProveWithFailure(secondSolver, secondState)
	if !secondTransitionOK || secondProofFailure != callsite.PublicationTransitionProofFailureNone || !secondProof.Valid() {
		t.Fatal("publication second-call proof")
	}
	crossSelector, crossSelectorOK := secondProof.SubjectSelector()
	crossSource, crossSourceOK := plan.state.binding.runtimeContexts.pack.MountedInputSemanticSource(mount, occurrence, crossSelector)
	if !crossSelectorOK || !crossSourceOK {
		t.Fatal("publication cross-call subject source")
	}
	crossRequirement, _ := publicationDirectAllocationSubject(t, plan.state.binding, crossSource)
	crossSubject, crossSubjectAvailability := issuer.BindRuntimeAllocationContext(mount, occurrence, crossSelector, crossRequirement, processContext)
	if crossSubjectAvailability != packdomain.RuntimeAllocationContextBindingBound || !crossSubject.Valid() {
		t.Fatal("publication cross-call subject binding")
	}
	crossMount, crossCall, crossProvenanceOK := crossSubject.CallProvenance()
	proofMount, proofCall := secondProof.MountID(), secondProof.CallOccurrenceID()
	if !crossProvenanceOK || crossMount != mount || crossCall != occurrence || proofMount != mount || proofCall != secondOccurrence || crossCall == proofCall {
		t.Fatal("publication cross-call provenance setup")
	}
	crossDestination := packdomain.RuntimeDestinationContextBinding{}
	crossDestinationPresent := false
	if contextSelector, contextRequired := secondProof.ContextSelector(); contextRequired {
		var crossDestinationAvailability packdomain.RuntimeAllocationContextBindingAvailability
		crossDestination, crossDestinationAvailability = issuer.BindRuntimeDestinationContext(mount, occurrence, contextSelector, actorContext)
		if crossDestinationAvailability != packdomain.RuntimeAllocationContextBindingBound || !crossDestination.Valid() {
			t.Fatal("publication cross-call destination binding")
		}
		crossDestinationPresent = true
	}
	if _, accepted := callsite.NewPublicationPlacementCorrelationCandidate(secondProof, crossSubject, crossDestination, crossDestinationPresent); accepted {
		t.Fatal("different call/mount proof entered local correlation")
	}
	authority.Close()
	if _, accepted := callsite.NewPublicationDirectAllocationSubject(firstCorrelation, firstSubject, firstDirect); accepted {
		t.Fatal("closed runtime authority left direct identity admission usable")
	}
	result, solveStatus := plan.Solve(context.Background())
	if solveStatus != AnalyzeComplete || result == nil || result.placement != nil {
		t.Fatal("direct allocation identity emitted a placement result")
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
	candidates, candidatesOK := plan.state.binding.selectedEffectRule().AttachMountedPublicationCandidates(compilation, plan.state.graph, plan.state.binding.effectQuery, mount, occurrence)
	attachment, attached := attachSelectedDirectAllocationMembership(compilation, plan.state.binding, plan.state.graph, plan.state.artifacts.mounts, mount, occurrence)
	if !candidatesOK || !candidates.Available() || candidates.Count() < 2 || !attached || !attachment.valid() {
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
	if !secondAttached || !secondAttachment.valid() || secondAttachment.id == attachment.id {
		t.Fatal("direct allocation membership second observation fixture")
	}
	swappedAttachment := attachment
	swappedAttachment.observation = secondAttachment.observation
	if swappedAttachment.valid() {
		t.Fatal("direct allocation membership accepted a swapped real Engine observation handle")
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

	authority, issuer, runtimeOK := plan.state.binding.runtimeContexts.Begin(publicationPlacementPolicyID("direct-membership"))
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
	source, sourceOK := plan.state.binding.runtimeContexts.pack.MountedInputSemanticSource(mount, occurrence, subjectSelector)
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
	membershipProof, proven := attachment.prove(solver, state, proof, correlation, subject, direct)
	if !proven || !membershipProof.valid() || membershipProof.membership != valuedomain.MembershipRecent && membershipProof.membership != valuedomain.MembershipSummary {
		t.Fatal("direct allocation membership proof")
	}
	for _, mutated := range []selectedDirectAllocationMembershipAttachment{
		func() selectedDirectAllocationMembershipAttachment {
			value := attachment
			value.id = publicationPlacementPolicyID("membership-mutated-id")
			return value
		}(),
		func() selectedDirectAllocationMembershipAttachment {
			value := attachment
			value.mount = secondOccurrence
			return value
		}(),
		func() selectedDirectAllocationMembershipAttachment {
			value := attachment
			value.point = publicationPlacementPolicyID("membership-mutated-point")
			return value
		}(),
		func() selectedDirectAllocationMembershipAttachment {
			value := attachment
			value.call = secondOccurrence
			return value
		}(),
		func() selectedDirectAllocationMembershipAttachment { value := attachment; value.width++; return value }(),
	} {
		if mutated.valid() {
			t.Fatal("direct allocation membership attachment scalar mutation survived")
		}
	}
	for _, mutated := range []directAllocationMembershipProof{
		func() directAllocationMembershipProof {
			value := membershipProof
			value.id = publicationPlacementPolicyID("membership-proof-mutated-id")
			return value
		}(),
		func() directAllocationMembershipProof {
			value := membershipProof
			value.attachment = publicationPlacementPolicyID("membership-proof-mutated-attachment")
			return value
		}(),
		func() directAllocationMembershipProof {
			value := membershipProof
			value.correlation = publicationPlacementPolicyID("membership-proof-mutated-correlation")
			return value
		}(),
		func() directAllocationMembershipProof {
			value := membershipProof
			value.direct = publicationPlacementPolicyID("membership-proof-mutated-direct")
			return value
		}(),
		func() directAllocationMembershipProof {
			value := membershipProof
			value.membership = valuedomain.MembershipMixedOrUnknown
			return value
		}(),
	} {
		if mutated.valid() {
			t.Fatal("direct allocation membership proof scalar mutation survived")
		}
	}
	secondCandidate, secondCandidateOK := candidates.At(1)
	secondProofAtSameCall, secondProofFailure := secondCandidate.ProveWithFailure(solver, state)
	secondSelector, secondSelectorOK := secondProofAtSameCall.SubjectSelector()
	if !secondCandidateOK || secondProofFailure != callsite.PublicationTransitionProofFailureNone || !secondProofAtSameCall.Valid() || !secondSelectorOK {
		t.Fatal("direct allocation membership second publication candidate")
	}
	secondSource, secondSourceOK := plan.state.binding.runtimeContexts.pack.MountedInputSemanticSource(mount, occurrence, secondSelector)
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
	if secondMembership, accepted := attachment.prove(solver, state, secondProofAtSameCall, secondCorrelation, secondSubject, secondDirect); !accepted || !secondMembership.valid() {
		t.Fatal("direct allocation membership did not reuse one observation across publication candidates")
	}

	if _, accepted := attachment.prove(nil, state, proof, correlation, subject, direct); accepted {
		t.Fatal("direct allocation membership accepted a foreign solver")
	}
	if _, accepted := attachment.prove(solver, state, callsite.PublicationTransitionProof{}, correlation, subject, direct); accepted {
		t.Fatal("direct allocation membership accepted an absent transition target")
	}
	if _, accepted := attachment.prove(solver, state, proof, callsite.PublicationPlacementCorrelationCandidate{}, subject, direct); accepted {
		t.Fatal("direct allocation membership accepted a spliced correlation")
	}
	if _, accepted := attachment.prove(solver, state, proof, correlation, packdomain.RuntimeAllocationContextBinding{}, direct); accepted {
		t.Fatal("direct allocation membership accepted a spliced subject")
	}
	if _, accepted := attachment.prove(solver, state, proof, correlation, subject, valuedomain.DirectAllocationSubject{}); accepted {
		t.Fatal("direct allocation membership accepted a spliced direct identity")
	}

	secondCompilation := publicationTransitionCompilationFor(t, plan, mount, secondOccurrence)
	secondCandidates, secondCandidatesOK := plan.state.binding.selectedEffectRule().AttachMountedPublicationCandidates(secondCompilation, plan.state.graph, plan.state.binding.effectQuery, mount, secondOccurrence)
	secondSolver, secondSolverOK := secondCompilation.Solver()
	if !secondCandidatesOK || !secondCandidates.Available() || secondCandidates.Count() == 0 || !secondSolverOK || secondSolver == nil {
		t.Fatal("direct allocation membership second-call fixture")
	}
	secondState, secondStatus := secondSolver.Solve(context.Background())
	secondTransition, secondTransitionOK := secondCandidates.At(0)
	secondProof, secondProofFailure := secondTransition.ProveWithFailure(secondSolver, secondState)
	secondCallSelector, secondCallSelectorOK := secondProof.SubjectSelector()
	if secondStatus != engine.SolveComplete || secondState == nil || !secondTransitionOK || secondProofFailure != callsite.PublicationTransitionProofFailureNone || !secondProof.Valid() || !secondCallSelectorOK {
		t.Fatal("direct allocation membership second-call proof")
	}
	secondCallSource, secondCallSourceOK := plan.state.binding.runtimeContexts.pack.MountedInputSemanticSource(mount, secondOccurrence, secondCallSelector)
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
	if _, accepted := attachment.prove(secondSolver, secondState, secondProof, secondCallCorrelation, secondCallSubject, secondCallDirect); accepted {
		t.Fatal("direct allocation membership accepted a different selected call")
	}

	foreignCompilation, foreignMount, foreignOccurrence := publicationTransitionCompilation(t, foreignPlan)
	foreignCandidates, foreignCandidatesOK := foreignPlan.state.binding.selectedEffectRule().AttachMountedPublicationCandidates(foreignCompilation, foreignPlan.state.graph, foreignPlan.state.binding.effectQuery, foreignMount, foreignOccurrence)
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
	foreignAuthority, foreignIssuer, foreignRuntimeOK := foreignPlan.state.binding.runtimeContexts.Begin(publicationPlacementPolicyID("direct-membership"))
	if !foreignRuntimeOK || foreignAuthority == nil {
		t.Fatal("direct allocation membership foreign runtime issuer")
	}
	defer foreignAuthority.Close()
	foreignProcessOwner, foreignProcessOwnerOK := foreignAuthority.ProcessOwner(publicationPlacementPolicyID("process"))
	foreignProcessContext, foreignProcessOK := foreignAuthority.Process(foreignProcessOwner)
	foreignSelector, foreignSelectorOK := foreignProof.SubjectSelector()
	foreignSource, foreignSourceOK := foreignPlan.state.binding.runtimeContexts.pack.MountedInputSemanticSource(foreignMount, foreignOccurrence, foreignSelector)
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
	if _, accepted := attachment.prove(solver, state, proof, correlation, foreignSubject, direct); accepted {
		t.Fatal("direct allocation membership accepted a foreign subject")
	}
	if _, accepted := attachment.prove(solver, state, proof, correlation, subject, foreignDirect); accepted {
		t.Fatal("direct allocation membership accepted an equal-content foreign direct identity")
	}
	if _, accepted := attachment.prove(foreignSolver, state, proof, correlation, subject, direct); accepted {
		t.Fatal("direct allocation membership accepted foreign solver with local state")
	}
	if _, accepted := attachment.prove(solver, foreignState, proof, correlation, subject, direct); accepted {
		t.Fatal("direct allocation membership accepted local solver with foreign state")
	}
	if _, accepted := attachment.prove(foreignSolver, foreignState, proof, correlation, subject, direct); accepted {
		t.Fatal("direct allocation membership accepted a foreign solver/state")
	}
	if _, accepted := attachment.prove(foreignSolver, foreignState, foreignProof, correlation, subject, direct); accepted {
		t.Fatal("direct allocation membership accepted a foreign transition/correlation join")
	}

	result, solveStatus := plan.Solve(context.Background())
	if solveStatus != AnalyzeComplete || result == nil || result.placement != nil {
		t.Fatal("direct allocation membership emitted a placement result")
	}
}

func TestPublicationTransitionCandidatesOmitGenericEffects(t *testing.T) {
	plan := publicationTransitionPlan(t, false, false, false)
	defer plan.Close()
	compilation, mount, occurrence := publicationTransitionCompilation(t, plan)
	candidates, ok := plan.state.binding.selectedEffectRule().AttachMountedPublicationCandidates(compilation, plan.state.graph, plan.state.binding.effectQuery, mount, occurrence)
	if !ok || !candidates.Available() || candidates.Count() != 0 {
		t.Fatal("generic selected effect issued a publication candidate")
	}
}
