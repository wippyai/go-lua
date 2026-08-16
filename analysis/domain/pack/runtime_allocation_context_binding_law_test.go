package pack_test

import (
	"crypto/sha256"
	"testing"

	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	"github.com/wippyai/go-lua/analysis/domain/type/authority"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/artifact/schemaadapter"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/analysis/schema/grammar"
)

func bindingLawID(text string) identity.ContentID {
	return identity.ContentID(sha256.Sum256([]byte(text)))
}

type runtimeContextBindingFixture struct {
	pack      *packdomain.Schema
	heap      heapdomain.Schema
	module    identity.ContentID
	callID    identity.ContentID
	operation target.Operation
}

func runtimeContextBindingSchema(t testing.TB, contract *target.Contract, operation target.Operation, label string) runtimeContextBindingFixture {
	t.Helper()
	published, err := lower.Lower(lower.Source{Name: "runtime_context_binding_" + label + ".lua", Text: []byte("local receiver = {}\nreceiver:send(1, 2)\n")})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "runtime_context_binding_" + label, Program: published}}})
	if err != nil {
		t.Fatal(err)
	}
	grammar, grammarOK := grammar.Global()
	if !grammarOK {
		t.Fatal("program schema receipt")
	}
	mounted := linked.Project().Mounts()
	shard, shardOK := mounted.At(0)
	program, programOK := mounted.Program(shard)
	module, moduleOK := linked.Project().ModuleKey(shard)
	programID, programIDOK := mounted.ProgramID(shard)
	if mounted.Count() != 1 || !shardOK || !programOK || program == nil || !moduleOK || !programIDOK {
		t.Fatal("binding mount")
	}
	artifact, failure := schemaadapter.CompileDetailed(program.TransformerInput(), grammar)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile binding artifact: %s", failure.Error())
	}
	types, err := typeauthority.SealArtifactRows(linked.ContentID(), []*programartifact.Artifact{artifact})
	if err != nil || types == nil {
		t.Fatalf("seal binding types: %v", err)
	}
	statics, _, err := staticdomain.SealMountedArtifacts(staticdomain.MountContext{LinkID: linked.ContentID(), Target: contract}, types, []staticdomain.MountedArtifact{{Artifact: artifact, ModuleID: module, ProgramID: programID, NamespaceID: module}})
	if err != nil || statics == nil {
		t.Fatalf("seal binding static: %v", err)
	}
	packMount, packMountOK := packdomain.NewArtifactMount(artifact, module, programID)
	heapMount, heapMountOK := heapdomain.NewArtifactMount(artifact, module, programID)
	if !packMountOK || !heapMountOK {
		t.Fatal("binding artifact mounts")
	}
	packSchema, packOK := packdomain.SealMountedArtifacts(linked, statics, []packdomain.ArtifactMount{packMount})
	heapSchema, heapFailure := heapdomain.SealWithArtifacts(linked, []heapdomain.ArtifactMount{heapMount})
	if !packOK || packSchema == nil || heapFailure != heapdomain.SealFailureNone || !heapSchema.Valid() {
		t.Fatal("binding schemas")
	}
	packReceipt, receiptOK := artifact.PackReceipt()
	if !receiptOK {
		t.Fatal("binding Pack receipt")
	}
	for index := 0; index < packReceipt.CallCount(); index++ {
		call, callOK := packReceipt.CallAt(index)
		if callOK && call.Form() == flow.CallFormMethod {
			return runtimeContextBindingFixture{pack: packSchema, heap: heapSchema, module: module, callID: call.ID(), operation: operation}
		}
	}
	t.Fatal("binding method call")
	return runtimeContextBindingFixture{}
}

func (fixture runtimeContextBindingFixture) requirement(t testing.TB) heapdomain.AllocationRequirement {
	t.Helper()
	for index := 0; index < fixture.heap.KeyCount(); index++ {
		key, keyOK := fixture.heap.KeyAt(index)
		if !keyOK || key.Kind() != heapdomain.RootAllocation {
			continue
		}
		requirement, requirementOK := fixture.heap.AllocationRequirementForKey(key)
		if requirementOK {
			return requirement
		}
	}
	t.Fatal("binding allocation requirement")
	return heapdomain.AllocationRequirement{}
}

func TestRuntimeAllocationContextBindingFencesAndAvailability(t *testing.T) {
	contract, operation := selectorLawContract(t)
	fixture := runtimeContextBindingSchema(t, contract, operation, "primary")
	fixed, fixedOK := fixture.pack.InputSelector(operation, target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: 1})
	tail, tailOK := fixture.pack.InputSelector(operation, target.InputSource{Kind: target.InputSourceValuesVar, Ordinal: 0})
	opaque, opaqueOK := contract.Opaque()
	whole, wholeOK := fixture.pack.InputSelector(opaque, target.InputSource{Kind: target.InputSourceAllInputs})
	if !fixedOK || !tailOK || !opaqueOK || !wholeOK {
		t.Fatal("binding selector inventory")
	}
	requirement := fixture.requirement(t)
	authority, authorityOK := fixture.heap.BeginRuntimeAllocationContexts(bindingLawID("policy"))
	if !authorityOK {
		t.Fatal("binding runtime authority")
	}
	issuer, issuerOK := packdomain.NewRuntimeAllocationContextBindingIssuer(fixture.pack, fixture.heap, authority)
	if !issuerOK {
		t.Fatal("binding issuer")
	}

	type runtimeCase struct {
		class   heapdomain.RuntimeAllocationContextClass
		context heapdomain.RuntimeAllocationContext
	}
	processOwner, processOwnerOK := authority.ProcessOwner(bindingLawID("process"))
	actorOwner, actorOwnerOK := authority.ActorOwner(bindingLawID("actor"))
	threadOwner, threadOwnerOK := authority.ThreadOwner(bindingLawID("thread"))
	sharedOwner, sharedOwnerOK := authority.SharedOwner(bindingLawID("shared"))
	process, processOK := authority.Process(processOwner)
	actor, actorOK := authority.Actor(actorOwner)
	thread, threadOK := authority.Thread(threadOwner)
	sharedAuthorization, sharedAuthorizationOK := authority.AuthorizeShared(bindingLawID("shared-policy"))
	shared, sharedOK := authority.Shared(sharedOwner, sharedAuthorization)
	if !processOwnerOK || !actorOwnerOK || !threadOwnerOK || !sharedOwnerOK || !processOK || !actorOK || !threadOK || !sharedAuthorizationOK || !sharedOK {
		t.Fatal("binding runtime contexts")
	}
	cases := []runtimeCase{{heapdomain.RuntimeAllocationContextProcess, process}, {heapdomain.RuntimeAllocationContextActor, actor}, {heapdomain.RuntimeAllocationContextShared, shared}, {heapdomain.RuntimeAllocationContextThread, thread}}
	ids := make(map[identity.ContentID]struct{}, len(cases))
	destinationIDs := make(map[identity.ContentID]struct{}, len(cases))
	for _, candidate := range cases {
		binding, availability := issuer.BindRuntimeAllocationContext(fixture.module, fixture.callID, fixed, requirement, candidate.context)
		if availability != packdomain.RuntimeAllocationContextBindingBound || !binding.Valid() {
			t.Fatalf("context %v did not bind", candidate.class)
		}
		mounted, mountedOK := binding.MountedAllocation()
		context, contextOK := mounted.Context()
		if !mountedOK || !contextOK || context.Class() != candidate.class {
			t.Fatalf("context %v provenance", candidate.class)
		}
		if _, duplicate := ids[binding.ID()]; duplicate {
			t.Fatal("runtime contexts aliased binding identity")
		}
		ids[binding.ID()] = struct{}{}

		destination, destinationAvailability := issuer.BindRuntimeDestinationContext(fixture.module, fixture.callID, fixed, candidate.context)
		if destinationAvailability != packdomain.RuntimeAllocationContextBindingBound || !destination.Valid() {
			t.Fatalf("destination context %v did not bind", candidate.class)
		}
		if context, contextOK := destination.Context(); !contextOK || context.Class() != candidate.class {
			t.Fatalf("destination context %v provenance", candidate.class)
		}
		if _, duplicate := destinationIDs[destination.ID()]; duplicate {
			t.Fatal("runtime contexts aliased destination binding identity")
		}
		destinationIDs[destination.ID()] = struct{}{}
	}
	if _, availability := issuer.BindRuntimeAllocationContext(fixture.module, fixture.callID, tail, requirement, actor); availability != packdomain.RuntimeAllocationContextBindingUnavailableTail {
		t.Fatal("tail did not remain explicitly unavailable")
	}
	if _, availability := issuer.BindRuntimeAllocationContext(fixture.module, fixture.callID, whole, requirement, actor); availability != packdomain.RuntimeAllocationContextBindingUnavailableWhole {
		t.Fatal("whole did not remain explicitly unavailable")
	}
	if _, availability := issuer.BindRuntimeDestinationContext(fixture.module, fixture.callID, tail, actor); availability != packdomain.RuntimeAllocationContextBindingUnavailableTail {
		t.Fatal("destination tail did not remain explicitly unavailable")
	}
	if _, availability := issuer.BindRuntimeDestinationContext(fixture.module, fixture.callID, whole, actor); availability != packdomain.RuntimeAllocationContextBindingUnavailableWhole {
		t.Fatal("destination whole did not remain explicitly unavailable")
	}

	foreignAuthority, foreignAuthorityOK := fixture.heap.BeginRuntimeAllocationContexts(bindingLawID("foreign-policy"))
	foreignOwner, foreignOwnerOK := foreignAuthority.ActorOwner(bindingLawID("actor"))
	foreignContext, foreignContextOK := foreignAuthority.Actor(foreignOwner)
	if !foreignAuthorityOK || !foreignOwnerOK || !foreignContextOK {
		t.Fatal("foreign authority")
	}
	if _, availability := issuer.BindRuntimeAllocationContext(fixture.module, fixture.callID, fixed, requirement, foreignContext); availability != packdomain.RuntimeAllocationContextBindingInvalid {
		t.Fatal("foreign authority context accepted")
	}
	if _, availability := issuer.BindRuntimeDestinationContext(fixture.module, fixture.callID, fixed, foreignContext); availability != packdomain.RuntimeAllocationContextBindingInvalid {
		t.Fatal("foreign authority destination context accepted")
	}
	foreign := runtimeContextBindingSchema(t, contract, operation, "foreign")
	foreignSelector, foreignSelectorOK := foreign.pack.InputSelector(operation, target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: 1})
	if !foreignSelectorOK {
		t.Fatal("foreign selector")
	}
	if _, availability := issuer.BindRuntimeAllocationContext(fixture.module, fixture.callID, foreignSelector, requirement, actor); availability != packdomain.RuntimeAllocationContextBindingInvalid {
		t.Fatal("foreign selector accepted")
	}
	if _, availability := issuer.BindRuntimeDestinationContext(fixture.module, fixture.callID, foreignSelector, actor); availability != packdomain.RuntimeAllocationContextBindingInvalid {
		t.Fatal("foreign destination selector accepted")
	}
	if _, availability := issuer.BindRuntimeAllocationContext(foreign.module, foreign.callID, fixed, requirement, actor); availability != packdomain.RuntimeAllocationContextBindingUnavailableUnknown {
		t.Fatal("foreign mounted call did not remain unavailable")
	}
	if _, availability := issuer.BindRuntimeDestinationContext(foreign.module, foreign.callID, fixed, actor); availability != packdomain.RuntimeAllocationContextBindingUnavailableUnknown {
		t.Fatal("foreign destination mounted call did not remain unavailable")
	}
	bound, boundAvailability := issuer.BindRuntimeAllocationContext(fixture.module, fixture.callID, fixed, requirement, actor)
	if boundAvailability != packdomain.RuntimeAllocationContextBindingBound || !bound.Valid() {
		t.Fatal("close invalidation setup")
	}
	destination, destinationAvailability := issuer.BindRuntimeDestinationContext(fixture.module, fixture.callID, fixed, actor)
	if destinationAvailability != packdomain.RuntimeAllocationContextBindingBound || !destination.Valid() {
		t.Fatal("destination close invalidation setup")
	}
	if !packdomain.SameRuntimeAllocationContextBindingIssuer(bound, destination) {
		t.Fatal("same issuer bindings did not correlate")
	}
	if packdomain.RuntimeDestinationContextBindingAbsent(destination) || !packdomain.RuntimeDestinationContextBindingAbsent(packdomain.RuntimeDestinationContextBinding{}) {
		t.Fatal("destination absence did not require the canonical zero representation")
	}
	secondAuthority, secondAuthorityOK := fixture.heap.BeginRuntimeAllocationContexts(bindingLawID("second-authority-policy"))
	secondOwner, secondOwnerOK := secondAuthority.ActorOwner(bindingLawID("actor"))
	secondActor, secondActorOK := secondAuthority.Actor(secondOwner)
	secondIssuer, secondIssuerOK := packdomain.NewRuntimeAllocationContextBindingIssuer(fixture.pack, fixture.heap, secondAuthority)
	secondSubject, secondSubjectAvailability := secondIssuer.BindRuntimeAllocationContext(fixture.module, fixture.callID, fixed, requirement, secondActor)
	secondDestination, secondDestinationAvailability := secondIssuer.BindRuntimeDestinationContext(fixture.module, fixture.callID, fixed, secondActor)
	if !secondAuthorityOK || !secondOwnerOK || !secondActorOK || !secondIssuerOK || secondSubjectAvailability != packdomain.RuntimeAllocationContextBindingBound || secondDestinationAvailability != packdomain.RuntimeAllocationContextBindingBound || !secondSubject.Valid() || !secondDestination.Valid() {
		t.Fatal("second runtime authority binding setup")
	}
	if packdomain.SameRuntimeAllocationContextBindingIssuer(bound, secondDestination) || packdomain.SameRuntimeAllocationContextBindingIssuer(secondSubject, destination) {
		t.Fatal("different live runtime authorities correlated equal-content bindings")
	}
	authority.Close()
	if bound.Valid() {
		t.Fatal("closed runtime authority left binding valid")
	}
	if destination.Valid() {
		t.Fatal("closed runtime authority left destination binding valid")
	}
	if packdomain.SameRuntimeAllocationContextBindingIssuer(bound, destination) {
		t.Fatal("closed runtime authority left cross-binding issuer relation valid")
	}
	if _, availability := issuer.BindRuntimeAllocationContext(fixture.module, fixture.callID, fixed, requirement, actor); availability != packdomain.RuntimeAllocationContextBindingInvalid {
		t.Fatal("closed runtime authority left issuer usable")
	}
}

func TestRuntimeAllocationContextBindingEqualContentSemanticID(t *testing.T) {
	contract, operation := selectorLawContract(t)
	left := runtimeContextBindingSchema(t, contract, operation, "equal")
	right := runtimeContextBindingSchema(t, contract, operation, "equal")
	if left.heap.ContentID() != right.heap.ContentID() {
		t.Fatal("equal heap content")
	}
	leftSelector, leftSelectorOK := left.pack.InputSelector(operation, target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: 1})
	rightSelector, rightSelectorOK := right.pack.InputSelector(operation, target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: 1})
	leftAuthority, leftAuthorityOK := left.heap.BeginRuntimeAllocationContexts(bindingLawID("equal-policy"))
	rightAuthority, rightAuthorityOK := right.heap.BeginRuntimeAllocationContexts(bindingLawID("equal-policy"))
	leftOwner, leftOwnerOK := leftAuthority.ActorOwner(bindingLawID("equal-actor"))
	rightOwner, rightOwnerOK := rightAuthority.ActorOwner(bindingLawID("equal-actor"))
	leftContext, leftContextOK := leftAuthority.Actor(leftOwner)
	rightContext, rightContextOK := rightAuthority.Actor(rightOwner)
	leftIssuer, leftIssuerOK := packdomain.NewRuntimeAllocationContextBindingIssuer(left.pack, left.heap, leftAuthority)
	rightIssuer, rightIssuerOK := packdomain.NewRuntimeAllocationContextBindingIssuer(right.pack, right.heap, rightAuthority)
	if !leftSelectorOK || !rightSelectorOK || !leftAuthorityOK || !rightAuthorityOK || !leftOwnerOK || !rightOwnerOK || !leftContextOK || !rightContextOK || !leftIssuerOK || !rightIssuerOK {
		t.Fatal("equal binding setup")
	}
	if _, spliced := packdomain.NewRuntimeAllocationContextBindingIssuer(left.pack, left.heap, rightAuthority); spliced {
		t.Fatal("equal-content foreign authority issuer accepted")
	}
	leftBinding, leftAvailability := leftIssuer.BindRuntimeAllocationContext(left.module, left.callID, leftSelector, left.requirement(t), leftContext)
	rightBinding, rightAvailability := rightIssuer.BindRuntimeAllocationContext(right.module, right.callID, rightSelector, right.requirement(t), rightContext)
	if leftAvailability != packdomain.RuntimeAllocationContextBindingBound || rightAvailability != packdomain.RuntimeAllocationContextBindingBound || !leftBinding.Valid() || !rightBinding.Valid() || leftBinding.ID() != rightBinding.ID() {
		t.Fatal("equal content binding semantic identity")
	}
	if _, availability := leftIssuer.BindRuntimeAllocationContext(left.module, left.callID, leftSelector, left.requirement(t), rightContext); availability != packdomain.RuntimeAllocationContextBindingInvalid {
		t.Fatal("equal-content foreign authority context accepted")
	}
	leftDestination, leftDestinationAvailability := leftIssuer.BindRuntimeDestinationContext(left.module, left.callID, leftSelector, leftContext)
	rightDestination, rightDestinationAvailability := rightIssuer.BindRuntimeDestinationContext(right.module, right.callID, rightSelector, rightContext)
	if leftDestinationAvailability != packdomain.RuntimeAllocationContextBindingBound || rightDestinationAvailability != packdomain.RuntimeAllocationContextBindingBound || !leftDestination.Valid() || !rightDestination.Valid() || leftDestination.ID() != rightDestination.ID() {
		t.Fatal("equal content destination binding semantic identity")
	}
	if packdomain.SameRuntimeAllocationContextBindingIssuer(leftBinding, rightDestination) || packdomain.SameRuntimeAllocationContextBindingIssuer(rightBinding, leftDestination) {
		t.Fatal("equal-content foreign issuers correlated")
	}
	if _, availability := leftIssuer.BindRuntimeDestinationContext(left.module, left.callID, leftSelector, rightContext); availability != packdomain.RuntimeAllocationContextBindingInvalid {
		t.Fatal("equal-content foreign authority destination context accepted")
	}
}
