package factor_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
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
	contract    *contract.Contract
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

func newEffectFactorFixture(t testing.TB, spec declaration.Spec, source string) effectFactorFixture {
	t.Helper()
	published, err := lower.Lower(lower.Source{Name: "effect_factor_law.lua", Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := compiler.Seal(&spec)
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "effect_factor_law", Program: published}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, ok := composite.Build()
	grammar := receipt.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(receipt)
	if !ok || !grammar.Available() || !issuanceOK {
		t.Fatal("program schema receipt")
	}

	projectMounts := linked.Project().Mounts()
	packMounts := make([]pack.ArtifactMount, projectMounts.Count())
	effectMounts := make([]effectfactor.MountedArtifact, projectMounts.Count())
	staticMounts := make([]staticdomain.MountedProgram, projectMounts.Count())
	artifacts := make([]*programartifact.Artifact, projectMounts.Count())
	for index := 0; index < projectMounts.Count(); index++ {
		shard, shardOK := projectMounts.At(index)
		program, programOK := projectMounts.Program(shard)
		module, moduleOK := linked.Project().ModuleKey(shard)
		programID, programIDOK := projectMounts.ProgramID(shard)
		if !shardOK || !programOK || program == nil || !moduleOK || !programIDOK {
			t.Fatalf("effect fixture mount %d", index)
		}
		artifact, failure := artifactcompiler.CompileDetailed(program, grammar, issuance)
		if failure.Available() || artifact == nil || !artifact.Available() {
			t.Fatalf("compile effect artifact %d: %s", index, failure.Error())
		}
		artifacts[index] = artifact
		packMounts[index], ok = pack.NewArtifactMount(snapshottest.MustLower(t, artifact), module, programID)
		if !ok {
			t.Fatalf("pack artifact mount %d", index)
		}
		effectMounts[index] = effectfactor.MountedArtifact{ModuleKey: module, Snapshot: snapshottest.MustLower(t, artifact)}
		staticMounts[index] = staticdomain.MountedProgram{Program: snapshottest.MustMount(t, artifact, module).Program, ModuleID: module, NamespaceID: module}
	}
	programs := make([]programschema.Program, len(artifacts))
	for index, artifact := range artifacts {
		programs[index] = artifact.Program()
	}
	types, err := typeauthority.SealProgramRows(linked.ContentID(), programs)
	if err != nil || types == nil {
		t.Fatalf("seal type authority: %v", err)
	}
	statics, _, err := staticdomain.SealMountedPrograms(staticdomain.MountContext{LinkID: linked.ContentID(), Target: contract}, types, staticMounts)
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
	owner, ok := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"sink"}})
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

func effectFactorSpec(rowTail vocabulary.RowTail, callback bool) declaration.Spec {
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
	return declaration.Spec{Semantics: domaincontract.NewSemantics(), Operations: []vocabulary.OperationSpec{owner, targetOperation}}
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
	callback, ok := closed.contract.Operations.CallbackAt(closed.owner, 0)
	if !ok {
		t.Fatal("callback descriptor")
	}
	callbackFormal, ok := closed.factor.FormalCallbackEffectAtom(closed.mountedCall, closed.owner, callback, 0)
	if !ok || !callbackFormal.Valid() {
		t.Fatal("callback formal effect")
	}
	open := newEffectFactorFixture(t, effectFactorSpec(vocabulary.RowClosed, false), "local function sink(value) return value end\nsink(1)")
	opaque, ok := open.contract.Operations.Opaque()
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

func publicationEffectFactorSpec(publicationKind vocabulary.PublicationEffectKind, callback bool) declaration.Spec {
	publication := &vocabulary.PublicationEffectSpec{
		Kind: publicationKind, Subject: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0}, Destination: vocabulary.PublicationDestinationNone,
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
	return declaration.Spec{Semantics: domaincontract.NewSemantics(), Operations: []vocabulary.OperationSpec{owner, targetOperation}}
}

func publicationEffectIndexes(t testing.TB, contract *contract.Contract, owner vocabulary.Operation) (publication, generic int) {
	t.Helper()
	publication, generic = -1, -1
	for effect := 0; effect < contract.Operations.EffectCount(owner); effect++ {
		if _, published := contract.Operations.EffectPublication(owner, effect); published {
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

func callbackPublicationEffectIndex(t testing.TB, contract *contract.Contract, callback vocabulary.CallbackID) int {
	t.Helper()
	publication := -1
	for effect := 0; effect < contract.Operations.CallbackEffectCount(callback); effect++ {
		if _, published := contract.Operations.CallbackEffectPublication(callback, effect); published {
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

func TestSelectedCallPublicationValidationLaw(t *testing.T) {
	ordinary := newEffectFactorFixture(t, publicationEffectFactorSpec(vocabulary.PublicationEffectSendTransfer, false), "local function sink(left, right) return left end\nsink(1, 2)")
	present, selectedOK := ordinary.factor.SelectedCallPublication(ordinary.root, ordinary.mountedCall, ordinary.owner)
	if !selectedOK || !present {
		t.Fatal("ordinary selected publication")
	}
	foreignOperation, foreignOperationOK := ordinary.contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"effect-target"}})
	if !foreignOperationOK || foreignOperation == ordinary.owner {
		t.Fatal("foreign target operation")
	}
	if foreignPresent, ok := ordinary.factor.SelectedCallPublication(ordinary.root, ordinary.mountedCall, foreignOperation); !ok || foreignPresent {
		t.Fatal("non-publication operation produced a publication occurrence")
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
	if _, ok := ordinary.factor.SelectedCallPublication(wrongRoot, ordinary.mountedCall, ordinary.owner); ok {
		t.Fatal("publication validation admitted foreign root")
	}
	foreignPacks, foreignPacksOK := pack.SealMountedArtifacts(ordinary.linked, ordinary.statics, ordinary.packMounts)
	foreignFactor, foreignFactorOK := effectfactor.NewWithMountedArtifacts(ordinary.linked, foreignPacks, ordinary.contract, ordinary.mounts)
	foreignMounted, foreignMountedOK := foreignFactor.MountedCallAt(0)
	if !foreignPacksOK || !foreignFactorOK || !foreignMountedOK {
		t.Fatal("equal-content foreign Pack/Effect fixture")
	}
	if _, ok := ordinary.factor.SelectedCallPublication(ordinary.root, foreignMounted, ordinary.owner); ok {
		t.Fatal("publication validation admitted foreign Pack/Effect mount")
	}

	callbackFixture := newEffectFactorFixture(t, publicationEffectFactorSpec(vocabulary.PublicationEffectCallbackEscape, true), "local function sink(left, right) return left end\nsink(1, 2)")
	callbackPresent, callbackSelectedOK := callbackFixture.factor.SelectedCallPublication(callbackFixture.root, callbackFixture.mountedCall, callbackFixture.owner)
	if !callbackSelectedOK || !callbackPresent {
		t.Fatal("callback selected publication")
	}
}
