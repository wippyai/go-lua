package factor_test

import (
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"github.com/wippyai/go-lua/domain/composite"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	"github.com/wippyai/go-lua/domain/pack"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	typeauthority "github.com/wippyai/go-lua/domain/type/authority"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

func portableAnyType() schematype.Type {
	value, ok := schematype.NewPrimitive(schematype.PrimitiveAny)
	if !ok {
		panic("portable any type")
	}
	return value
}

type effectFactorFixture struct {
	contract    *target.Contract
	linked      *link.Link
	statics     *staticdomain.Authority
	packs       *pack.Schema
	packMounts  []pack.ArtifactMount
	factor      *effectfactor.Algebra
	mounts      []effectfactor.MountedArtifact
	owner       vocabulary.Operation
	application identity.ContentID
	mountedCall effectfactor.MountedCall
	root        effectfactor.Root
}

func newEffectFactorFixture(t testing.TB, spec target.Spec, source string) effectFactorFixture {
	t.Helper()
	published, err := lower.Lower(lower.Source{Name: "effect_factor_law.lua", Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&spec)
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "effect_factor_law", Program: published}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, ok := composite.Global()
	if !ok {
		t.Fatal("program schema receipt")
	}

	projectMounts := linked.Project().Mounts()
	packMounts := make([]pack.ArtifactMount, projectMounts.Count())
	effectMounts := make([]effectfactor.MountedArtifact, projectMounts.Count())
	staticMounts := make([]staticdomain.MountedArtifact, projectMounts.Count())
	artifacts := make([]*programartifact.Artifact, projectMounts.Count())
	for index := 0; index < projectMounts.Count(); index++ {
		shard, shardOK := projectMounts.At(index)
		program, programOK := projectMounts.Program(shard)
		module, moduleOK := linked.Project().ModuleKey(shard)
		programID, programIDOK := projectMounts.ProgramID(shard)
		if !shardOK || !programOK || program == nil || !moduleOK || !programIDOK {
			t.Fatalf("effect fixture mount %d", index)
		}
		artifact, failure := composite.CompileArtifactDetailed(program, receipt)
		if failure.Available() || artifact == nil || !artifact.Available() {
			t.Fatalf("compile effect artifact %d: %s", index, failure.Error())
		}
		artifacts[index] = artifact
		packMounts[index], ok = pack.NewArtifactMount(snapshottest.MustLower(t, artifact), module, programID)
		if !ok {
			t.Fatalf("pack artifact mount %d", index)
		}
		effectMounts[index] = effectfactor.MountedArtifact{ModuleKey: module, Snapshot: snapshottest.MustLower(t, artifact)}
		staticMounts[index] = staticdomain.MountedArtifact{Artifact: artifact, ModuleID: module, ProgramID: programID, NamespaceID: module}
	}
	types, err := typeauthority.SealArtifactRows(linked.ContentID(), artifacts)
	if err != nil || types == nil {
		t.Fatalf("seal type authority: %v", err)
	}
	statics, _, err := staticdomain.SealMountedArtifacts(staticdomain.MountContext{LinkID: linked.ContentID(), Target: contract}, types, staticMounts)
	if err != nil || statics == nil {
		t.Fatalf("seal static mounts: %v", err)
	}
	packs, ok := pack.SealMountedArtifacts(linked, statics, packMounts)
	if !ok || packs == nil {
		t.Fatal("seal Pack mounts")
	}
	factor, ok := effectfactor.NewWithMountedArtifacts(linked, packs, contract, effectMounts)
	if !ok || factor == nil {
		t.Fatal("seal Effect mounts")
	}
	owner, ok := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"sink"}})
	if !ok {
		t.Fatal("sink operation")
	}
	mountedCall, ok := factor.MountedCallAt(0)
	if !ok {
		t.Fatal("mounted Effect call")
	}
	application, _, _, ok := factor.MountedCallIdentity(mountedCall)
	if !ok {
		t.Fatal("mounted Effect call identity")
	}
	root, ok := factor.RootForMountedCall(mountedCall)
	if !ok {
		t.Fatal("mounted Effect root")
	}
	return effectFactorFixture{contract: contract, linked: linked, statics: statics, packs: packs, packMounts: packMounts, factor: factor, mounts: effectMounts, owner: owner, application: application, mountedCall: mountedCall, root: root}
}

func effectFactorSpec(rowTail vocabulary.RowTail, callback bool) target.Spec {
	// Input selectors are intentionally outside the post-cutover Pack surface;
	// retain the descriptor/rule laws with a zero-argument effect occurrence.
	args := vocabulary.EffectSpec{Target: 2}
	owner := vocabulary.OperationSpec{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"sink"}}},
		Input:    vocabulary.ValuesSpec{Fixed: []schematype.Type{portableAnyType()}, Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Occurrences: []vocabulary.EffectSpec{args}, Tail: rowTail},
	}
	if callback {
		empty := vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}
		terminals := []vocabulary.TerminalSpec{
			{Kind: kind.OutcomeNormal, Values: empty}, {Kind: kind.OutcomeReturn, Values: empty},
			{Kind: kind.OutcomeThrow, Values: empty}, {Kind: kind.OutcomeYield, Values: empty},
			{Kind: kind.OutcomeCancel, Values: empty},
		}
		owner.Callbacks = []vocabulary.CallbackSpec{{
			Function: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, Admission: schematype.CallableAdmissionOrdinary,
			Arguments: empty, Outcomes: terminals, Lifecycle: vocabulary.CallbackRetainedOptionalOnce,
			Effects: vocabulary.RowSpec{Occurrences: []vocabulary.EffectSpec{args}, Tail: vocabulary.RowClosed},
		}}
	}
	targetOperation := vocabulary.OperationSpec{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"effect-target"}}},
		Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
	return target.Spec{Semantics: domaincontract.NewSemantics(), Operations: []vocabulary.OperationSpec{owner, targetOperation}}
}

func effectKnownAtom(t testing.TB, fixture effectFactorFixture) effectfactor.Atom {
	t.Helper()
	atom, ok := fixture.factor.CallEffectAtom(fixture.root, fixture.application, fixture.owner, 0)
	if !ok {
		t.Fatal("known ordinary effect")
	}
	return atom
}

func effectAtomID(t testing.TB, algebra *effectfactor.Algebra, atom effectfactor.Atom) identity.ContentID {
	t.Helper()
	id, ok := algebra.AtomID(atom)
	if !ok || !id.Available() {
		t.Fatal("effect atom identity")
	}
	return id
}

func TestEffectFactorMountedOwnerAndRootLaws(t *testing.T) {
	fixture := newEffectFactorFixture(t, effectFactorSpec(vocabulary.RowClosed, false), "local function sink(value) return value end\nsink(1)")
	if !fixture.factor.Valid() || fixture.factor.LinkOwner() != fixture.linked.OwnerCapability() || fixture.factor.Pack().LinkOwner() != fixture.linked.OwnerCapability() {
		t.Fatal("Effect/Pack did not retain the exact Link owner capability")
	}
	if fixture.factor.RootCount() == 0 || fixture.factor.MountedCallCount() == 0 {
		t.Fatal("mounted Effect fixture is empty")
	}
	if got, ok := fixture.factor.RootForCallID(fixture.application); !ok || got != fixture.root {
		t.Fatal("mounted call identity did not resolve its Effect root")
	}
	if !fixture.factor.ContainsCallID(fixture.root, fixture.application) {
		t.Fatal("mounted call did not remain fenced to its root")
	}
	if !fixture.factor.Admit(fixture.root, fixture.factor.Bottom()) {
		t.Fatal("bottom was not admitted at mounted root")
	}
	foreign, ok := effectfactor.NewWithMountedArtifacts(fixture.linked, fixture.packs, fixture.contract, fixture.mounts)
	if !ok || foreign == nil {
		t.Fatal("seal same-Link comparison Effect algebra")
	}
	foreignRoot, ok := foreign.RootAt(0)
	if !ok || fixture.factor.Admit(foreignRoot, fixture.factor.Bottom()) {
		t.Fatal("Effect admitted a same-Link foreign algebra root")
	}
	foreign, ok = effectfactor.NewWithMountedArtifacts(fixture.linked, fixture.packs, fixture.contract, nil)
	if ok || foreign != nil {
		t.Fatal("Effect accepted an incomplete mounted-artifact set")
	}
	foreign, ok = effectfactor.NewWithMountedArtifacts(fixture.linked, fixture.packs, fixture.contract, []effectfactor.MountedArtifact{{ModuleKey: fixture.factor.LinkID(), Snapshot: nil}})
	if ok || foreign != nil {
		t.Fatal("Effect accepted an invalid mounted-artifact receipt")
	}
}

func TestEffectFactorMountedAlgebraLaws(t *testing.T) {
	fixture := newEffectFactorFixture(t, effectFactorSpec(vocabulary.RowClosed, false), "local function sink(value) return value end\nsink(1)")
	known := effectKnownAtom(t, fixture)
	knownValue, ok := fixture.factor.Singleton(known)
	if !ok {
		t.Fatal("known singleton")
	}
	top := fixture.factor.Top()
	if !fixture.factor.LessOrEq(fixture.factor.Bottom(), top) || fixture.factor.LessOrEq(top, knownValue) || !fixture.factor.LessOrEq(knownValue, top) {
		t.Fatal("Effect top/bottom order law")
	}
	joined, ok := fixture.factor.Join(knownValue, top)
	if !ok || !fixture.factor.Equal(joined, top) {
		t.Fatal("Effect top join law")
	}
	if fixture.factor.RootCount() < 2 {
		t.Fatal("fixture lacks transport root")
	}
	destination, ok := fixture.factor.RootAt(1)
	if !ok {
		t.Fatal("transport destination root")
	}
	transported, ok := fixture.factor.TransportAtom(known, destination)
	if !ok || effectAtomID(t, fixture.factor, transported) != effectAtomID(t, fixture.factor, known) {
		t.Fatal("Effect atom transport changed identity")
	}
	transportedValue, ok := fixture.factor.Singleton(transported)
	if !ok || !fixture.factor.Admit(destination, transportedValue) {
		t.Fatal("transported Effect atom was not admitted")
	}
	if fixture.factor.WidenRank(fixture.root, fixture.factor.Top(), 0) != 0 || fixture.factor.WidenRank(fixture.root, fixture.factor.Bottom(), 0) == 0 {
		t.Fatal("Effect rank endpoints")
	}
}

func TestEffectFactorMountedFormalAndOpenRowLaws(t *testing.T) {
	closed := newEffectFactorFixture(t, effectFactorSpec(vocabulary.RowClosed, true), "local function sink(value) return value end\nsink(1)")
	formal, ok := closed.factor.FormalCallEffectAtom(closed.mountedCall, closed.owner, 0)
	if !ok || !formal.Valid() {
		t.Fatal("closed formal ordinary effect")
	}
	binding, ok := closed.factor.BindFormalCallEffectAtom(closed.root, closed.mountedCall, closed.owner, 0, formal)
	if !ok {
		t.Fatal("closed formal effect binding")
	}
	bound, ok := binding.Atom()
	if !ok || effectAtomID(t, closed.factor, bound) != effectAtomID(t, closed.factor, effectKnownAtom(t, closed)) {
		t.Fatal("formal binding did not preserve atom identity")
	}
	callback, ok := closed.contract.CallbackAt(closed.owner, 0)
	if !ok {
		t.Fatal("callback descriptor")
	}
	callbackFormal, ok := closed.factor.FormalCallbackEffectAtom(closed.mountedCall, closed.owner, callback, 0)
	if !ok || !callbackFormal.Valid() {
		t.Fatal("callback formal effect")
	}
	open := newEffectFactorFixture(t, effectFactorSpec(vocabulary.RowClosed, false), "local function sink(value) return value end\nsink(1)")
	opaque, ok := open.contract.Opaque()
	if !ok {
		t.Fatal("opaque operation")
	}
	unknown, ok := open.factor.OpenOperationUnknown(open.root, open.application, opaque)
	if !ok {
		t.Fatal("open-row unknown effect")
	}
	known := effectKnownAtom(t, open)
	knownValue, _ := open.factor.Singleton(known)
	unknownValue, _ := open.factor.Singleton(unknown)
	joined, ok := open.factor.Join(knownValue, unknownValue)
	if !ok || !open.factor.LessOrEq(knownValue, joined) || !open.factor.LessOrEq(unknownValue, joined) {
		t.Fatal("open-row known/unknown join law")
	}
}

func publicationEffectFactorSpec(publicationKind vocabulary.PublicationEffectKind, callback bool) target.Spec {
	publication := &vocabulary.PublicationEffectSpec{
		Kind: publicationKind, Subject: 0, Destination: vocabulary.PublicationDestinationNone,
		Escape: vocabulary.PublicationEscapeNone, Mutability: vocabulary.PublicationMutabilityPreserve, Lifetime: vocabulary.PublicationLifetimePreserve,
	}
	switch publicationKind {
	case vocabulary.PublicationEffectSendTransfer:
		publication.Destination, publication.Context = vocabulary.PublicationDestinationValueFormal, 1
		publication.Escape, publication.Mutability = vocabulary.PublicationEscapeSendTransfer, vocabulary.PublicationMutabilityCopyOnWrite
	case vocabulary.PublicationEffectReturnEscape:
		publication.Escape = vocabulary.PublicationEscapeReturn
	case vocabulary.PublicationEffectCallbackEscape:
		publication.Escape = vocabulary.PublicationEscapeCallback
	case vocabulary.PublicationEffectFreezeSeal:
		publication.Mutability = vocabulary.PublicationMutabilitySeal
	case vocabulary.PublicationEffectWriteMutation:
		publication.Mutability = vocabulary.PublicationMutabilityWrite
	case vocabulary.PublicationEffectCloseRelease:
		publication.Lifetime = vocabulary.PublicationLifetimeRelease
	}
	// Deliberately reverse the effect-target inputs. The publication descriptor
	// selects target formals, so a valid binding must map through these ABI
	// positions rather than treating descriptor ordinals as caller formals.
	effect := vocabulary.EffectSpec{Target: 2, ValueArgs: []vocabulary.ValueFormal{1, 0}, Publication: publication}
	owner := vocabulary.OperationSpec{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"sink"}}},
		Input:    vocabulary.ValuesSpec{Fixed: []schematype.Type{portableAnyType(), portableAnyType()}, Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Occurrences: []vocabulary.EffectSpec{effect, {Target: 2, ValueArgs: []vocabulary.ValueFormal{1, 0}}}, Tail: vocabulary.RowClosed},
	}
	if callback {
		empty := vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}
		terminals := []vocabulary.TerminalSpec{{Kind: kind.OutcomeNormal, Values: empty}, {Kind: kind.OutcomeReturn, Values: empty}, {Kind: kind.OutcomeThrow, Values: empty}, {Kind: kind.OutcomeYield, Values: empty}, {Kind: kind.OutcomeCancel, Values: empty}}
		owner.Callbacks = []vocabulary.CallbackSpec{{
			Function: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, Admission: schematype.CallableAdmissionOrdinary,
			Arguments: empty, Outcomes: terminals, Lifecycle: vocabulary.CallbackRetainedOptionalOnce,
			Effects: vocabulary.RowSpec{Occurrences: []vocabulary.EffectSpec{effect}, Tail: vocabulary.RowClosed},
		}}
	}
	targetOperation := vocabulary.OperationSpec{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"effect-target"}}},
		Input:    vocabulary.ValuesSpec{Fixed: []schematype.Type{portableAnyType(), portableAnyType()}, Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
	return target.Spec{Semantics: domaincontract.NewSemantics(), Operations: []vocabulary.OperationSpec{owner, targetOperation}}
}

func publicationEffectIndexes(t testing.TB, contract *target.Contract, owner vocabulary.Operation) (publication, generic int) {
	t.Helper()
	publication, generic = -1, -1
	for effect := 0; effect < contract.EffectCount(owner); effect++ {
		if _, published := contract.PublicationEffectDescriptor(owner, effect); published {
			if publication >= 0 {
				t.Fatal("multiple ordinary publication effects")
			}
			publication = effect
			continue
		}
		if generic >= 0 {
			t.Fatal("multiple ordinary generic effects")
		}
		generic = effect
	}
	if publication < 0 || generic < 0 {
		t.Fatal("ordinary publication/generic effect inventory")
	}
	return publication, generic
}

func callbackPublicationEffectIndex(t testing.TB, contract *target.Contract, callback vocabulary.CallbackID) int {
	t.Helper()
	publication := -1
	for effect := 0; effect < contract.CallbackEffectCount(callback); effect++ {
		if _, published := contract.CallbackPublicationEffectDescriptor(callback, effect); published {
			if publication >= 0 {
				t.Fatal("multiple callback publication effects")
			}
			publication = effect
		}
	}
	if publication < 0 {
		t.Fatal("callback publication effect inventory")
	}
	return publication
}

func TestPublicationAtomBindingOwnerLaw(t *testing.T) {
	ordinary := newEffectFactorFixture(t, publicationEffectFactorSpec(vocabulary.PublicationEffectSendTransfer, false), "local function sink(left, right) return left end\nsink(1, 2)")
	publicationEffect, genericEffect := publicationEffectIndexes(t, ordinary.contract, ordinary.owner)
	formal, formalOK := ordinary.factor.FormalCallEffectAtom(ordinary.mountedCall, ordinary.owner, publicationEffect)
	atomBinding, bindingOK := ordinary.factor.BindFormalCallEffectAtom(ordinary.root, ordinary.mountedCall, ordinary.owner, publicationEffect, formal)
	publication, publicationOK := ordinary.factor.PublicationCallEffectBinding(ordinary.root, ordinary.mountedCall, ordinary.owner, publicationEffect, atomBinding)
	if !formalOK || !bindingOK || !publicationOK || !publication.Valid() || publication.Role() != effectfactor.PublicationAtomBindingOrdinary || publication.Kind() != vocabulary.PublicationEffectSendTransfer || publication.Escape() != vocabulary.PublicationEscapeSendTransfer || publication.Mutability() != vocabulary.PublicationMutabilityCopyOnWrite || publication.Lifetime() != vocabulary.PublicationLifetimePreserve {
		t.Fatal("ordinary publication binding")
	}
	descriptorID, descriptorOK := ordinary.contract.PublicationEffectDescriptorID(ordinary.owner, publicationEffect)
	occurrenceID, occurrenceOK := ordinary.contract.PublicationEffectOccurrenceID(ordinary.owner, publicationEffect)
	boundDescriptor, boundDescriptorOK := publication.DescriptorID()
	boundOccurrence, boundOccurrenceOK := publication.OccurrenceID()
	subject, subjectOK := publication.SubjectSelector()
	context, contextOK := publication.ContextSelector()
	expectedSubject, expectedSubjectOK := ordinary.packs.InputSelector(ordinary.owner, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 1})
	expectedContext, expectedContextOK := ordinary.packs.InputSelector(ordinary.owner, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0})
	if !descriptorOK || !occurrenceOK || !boundDescriptorOK || !boundOccurrenceOK || descriptorID != boundDescriptor || occurrenceID != boundOccurrence || !subjectOK || !contextOK || !ordinary.packs.OwnsInputSelector(subject) || !ordinary.packs.OwnsInputSelector(context) || !expectedSubjectOK || !expectedContextOK || subject != expectedSubject || context != expectedContext {
		t.Fatal("ordinary publication descriptor or Pack selectors")
	}
	all, allOK := ordinary.factor.SelectedCallPublicationAtomBindings(ordinary.root, ordinary.mountedCall, ordinary.owner)
	if !allOK || len(all) != 1 || !all[0].Valid() {
		t.Fatal("ordinary publication inventory")
	}
	genericFormal, genericFormalOK := ordinary.factor.FormalCallEffectAtom(ordinary.mountedCall, ordinary.owner, genericEffect)
	genericBinding, genericBindingOK := ordinary.factor.BindFormalCallEffectAtom(ordinary.root, ordinary.mountedCall, ordinary.owner, genericEffect, genericFormal)
	if !genericFormalOK || !genericBindingOK {
		t.Fatal("ordinary generic binding")
	}
	if _, ok := ordinary.factor.PublicationCallEffectBinding(ordinary.root, ordinary.mountedCall, ordinary.owner, genericEffect, genericBinding); ok {
		t.Fatal("generic effect gained publication semantics")
	}
	if _, ok := ordinary.factor.PublicationCallEffectBinding(ordinary.root, ordinary.mountedCall, ordinary.owner, publicationEffect, genericBinding); ok {
		t.Fatal("publication binding admitted mismatched atom")
	}
	foreignOperation, foreignOperationOK := ordinary.contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"effect-target"}})
	if !foreignOperationOK || foreignOperation == ordinary.owner {
		t.Fatal("foreign target operation")
	}
	if _, ok := ordinary.factor.PublicationCallEffectBinding(ordinary.root, ordinary.mountedCall, foreignOperation, publicationEffect, atomBinding); ok {
		t.Fatal("publication binding admitted unselected operation")
	}
	var wrongRoot effectfactor.Root
	wrongRootOK := false
	for index := 0; index < ordinary.factor.RootCount(); index++ {
		candidate, candidateOK := ordinary.factor.RootAt(index)
		if candidateOK && candidate != ordinary.root {
			wrongRoot, wrongRootOK = candidate, true
			break
		}
	}
	if !wrongRootOK {
		t.Fatal("ordinary fixture lacks foreign root")
	}
	if _, ok := ordinary.factor.PublicationCallEffectBinding(wrongRoot, ordinary.mountedCall, ordinary.owner, publicationEffect, atomBinding); ok {
		t.Fatal("publication binding admitted foreign root")
	}
	foreignPacks, foreignPacksOK := pack.SealMountedArtifacts(ordinary.linked, ordinary.statics, ordinary.packMounts)
	foreignFactor, foreignFactorOK := effectfactor.NewWithMountedArtifacts(ordinary.linked, foreignPacks, ordinary.contract, ordinary.mounts)
	foreignMounted, foreignMountedOK := foreignFactor.MountedCallAt(0)
	foreignRoot, foreignRootOK := foreignFactor.RootForMountedCall(foreignMounted)
	foreignFormal, foreignFormalOK := foreignFactor.FormalCallEffectAtom(foreignMounted, ordinary.owner, publicationEffect)
	foreignBinding, foreignBindingOK := foreignFactor.BindFormalCallEffectAtom(foreignRoot, foreignMounted, ordinary.owner, publicationEffect, foreignFormal)
	if !foreignPacksOK || !foreignFactorOK || !foreignMountedOK || !foreignRootOK || !foreignFormalOK || !foreignBindingOK {
		t.Fatal("equal-content foreign Pack/Effect fixture")
	}
	if _, ok := ordinary.factor.PublicationCallEffectBinding(ordinary.root, ordinary.mountedCall, ordinary.owner, publicationEffect, foreignBinding); ok {
		t.Fatal("publication binding admitted foreign Pack/Effect atom")
	}

	callbackFixture := newEffectFactorFixture(t, publicationEffectFactorSpec(vocabulary.PublicationEffectCallbackEscape, true), "local function sink(left, right) return left end\nsink(1, 2)")
	callback, callbackOK := callbackFixture.contract.CallbackAt(callbackFixture.owner, 0)
	callbackEffect := callbackPublicationEffectIndex(t, callbackFixture.contract, callback)
	callbackFormal, callbackFormalOK := callbackFixture.factor.FormalCallbackEffectAtom(callbackFixture.mountedCall, callbackFixture.owner, callback, callbackEffect)
	callbackAtom, callbackAtomOK := callbackFixture.factor.BindFormalCallbackEffectAtom(callbackFixture.root, callbackFixture.mountedCall, callbackFixture.owner, callback, callbackEffect, callbackFormal)
	callbackPublication, callbackPublicationOK := callbackFixture.factor.PublicationCallbackEffectBinding(callbackFixture.root, callbackFixture.mountedCall, callbackFixture.owner, callback, callbackEffect, callbackAtom)
	if !callbackOK || !callbackFormalOK || !callbackAtomOK || !callbackPublicationOK || !callbackPublication.Valid() || callbackPublication.Role() != effectfactor.PublicationAtomBindingCallback || callbackPublication.Kind() != vocabulary.PublicationEffectCallbackEscape || callbackPublication.Escape() != vocabulary.PublicationEscapeCallback {
		t.Fatal("callback publication binding")
	}
	if _, contextOK := callbackPublication.ContextSelector(); contextOK {
		t.Fatal("destination-free callback publication carried context selector")
	}
	callbackDescriptor, callbackDescriptorOK := callbackFixture.contract.CallbackPublicationEffectDescriptorID(callback, callbackEffect)
	callbackOccurrence, callbackOccurrenceOK := callbackFixture.contract.CallbackPublicationEffectOccurrenceID(callback, callbackEffect)
	boundCallbackDescriptor, boundCallbackDescriptorOK := callbackPublication.DescriptorID()
	boundCallbackOccurrence, boundCallbackOccurrenceOK := callbackPublication.OccurrenceID()
	callbackSubject, callbackSubjectOK := callbackPublication.SubjectSelector()
	expectedCallbackSubject, expectedCallbackSubjectOK := callbackFixture.packs.InputSelector(callbackFixture.owner, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 1})
	if !callbackDescriptorOK || !callbackOccurrenceOK || !boundCallbackDescriptorOK || !boundCallbackOccurrenceOK || callbackDescriptor != boundCallbackDescriptor || callbackOccurrence != boundCallbackOccurrence || !callbackSubjectOK || !expectedCallbackSubjectOK || callbackSubject != expectedCallbackSubject {
		t.Fatal("callback publication descriptor or mapped selector")
	}
	_, callbackGenericEffect := publicationEffectIndexes(t, callbackFixture.contract, callbackFixture.owner)
	ordinaryFormal, ordinaryFormalOK := callbackFixture.factor.FormalCallEffectAtom(callbackFixture.mountedCall, callbackFixture.owner, callbackGenericEffect)
	ordinaryAtom, ordinaryAtomOK := callbackFixture.factor.BindFormalCallEffectAtom(callbackFixture.root, callbackFixture.mountedCall, callbackFixture.owner, callbackGenericEffect, ordinaryFormal)
	if !ordinaryFormalOK || !ordinaryAtomOK {
		t.Fatal("callback fixture ordinary binding")
	}
	if _, ok := callbackFixture.factor.PublicationCallbackEffectBinding(callbackFixture.root, callbackFixture.mountedCall, callbackFixture.owner, callback, callbackEffect, ordinaryAtom); ok {
		t.Fatal("callback publication admitted ordinary atom binding")
	}
	first, firstOK := callbackFixture.factor.SelectedCallPublicationAtomBindings(callbackFixture.root, callbackFixture.mountedCall, callbackFixture.owner)
	second, secondOK := callbackFixture.factor.SelectedCallPublicationAtomBindings(callbackFixture.root, callbackFixture.mountedCall, callbackFixture.owner)
	if !firstOK || !secondOK || len(first) != 2 || len(first) != len(second) {
		t.Fatal("callback publication inventory")
	}
	for index := range first {
		firstDescriptor, firstDescriptorOK := first[index].DescriptorID()
		secondDescriptor, secondDescriptorOK := second[index].DescriptorID()
		firstOccurrence, firstOccurrenceOK := first[index].OccurrenceID()
		secondOccurrence, secondOccurrenceOK := second[index].OccurrenceID()
		if !firstDescriptorOK || !secondDescriptorOK || !firstOccurrenceOK || !secondOccurrenceOK || firstDescriptor != secondDescriptor || firstOccurrence != secondOccurrence || first[index].Role() != second[index].Role() {
			t.Fatal("publication inventory changed across repeated issuance")
		}
	}
}
