package factor_test

import (
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"testing"

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

type effectMembershipFixture struct {
	algebra *effectfactor.Algebra
	root    effectfactor.Root
	mounted effectfactor.MountedCall
	owner   vocabulary.Operation
}

func newEffectMembershipFixture(t testing.TB) effectMembershipFixture {
	t.Helper()
	published, err := lower.Lower(lower.Source{Name: "effect_observation_law.lua", Text: []byte("local function sink(value) return value end\nsink(1)")})
	if err != nil {
		t.Fatal(err)
	}
	args := vocabulary.EffectSpec{Target: 2}
	foreignArgs := vocabulary.EffectSpec{Target: 3}
	contract, err := target.Seal(&target.Spec{Semantics: domaincontract.NewSemantics(), Operations: []vocabulary.OperationSpec{
		{Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"sink"}}}, Input: vocabulary.ValuesSpec{Fixed: []schematype.Type{portableAnyType()}, Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Occurrences: []vocabulary.EffectSpec{args, foreignArgs}, Tail: vocabulary.RowClosed}},
		{Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"effect-target"}}}, Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}},
		{Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"foreign-effect-target"}}}, Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "effect_observation_law", Program: published}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, ok := composite.Global()
	if !ok {
		t.Fatal("program schema receipt")
	}
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	program, programOK := mounts.Program(shard)
	module, moduleOK := linked.Project().ModuleKey(shard)
	programID, programIDOK := mounts.ProgramID(shard)
	if !shardOK || !programOK || program == nil || !moduleOK || !programIDOK {
		t.Fatal("effect observation mount")
	}
	artifact, failure := composite.CompileArtifactDetailed(program, receipt)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile artifact: %s", failure.Error())
	}
	types, err := typeauthority.SealArtifactRows(linked.ContentID(), []*programartifact.Artifact{artifact})
	if err != nil || types == nil {
		t.Fatalf("seal types: %v", err)
	}
	statics, _, err := staticdomain.SealMountedArtifacts(staticdomain.MountContext{LinkID: linked.ContentID(), Target: contract}, types, []staticdomain.MountedArtifact{{Artifact: artifact, ModuleID: module, ProgramID: programID, NamespaceID: module}})
	if err != nil || statics == nil {
		t.Fatalf("seal statics: %v", err)
	}
	packMount, packOK := pack.NewArtifactMount(snapshottest.MustLower(t, artifact), module, programID)
	packs, packsOK := pack.SealMountedArtifacts(linked, statics, []pack.ArtifactMount{packMount})
	algebra, algebraOK := effectfactor.NewWithMountedArtifacts(linked, packs, contract, []effectfactor.MountedArtifact{{ModuleKey: module, Snapshot: snapshottest.MustLower(t, artifact)}})
	owner, ownerOK := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"sink"}})
	mounted, mountedOK := algebra.MountedCallAt(0)
	root, rootOK := algebra.RootForMountedCall(mounted)
	if !packOK || !packsOK || packs == nil || !algebraOK || algebra == nil || !ownerOK || !mountedOK || !rootOK {
		t.Fatal("effect observation authorities")
	}
	return effectMembershipFixture{algebra: algebra, root: root, mounted: mounted, owner: owner}
}

func TestEffectObservationProvesOnlyAuthenticatedAtomBinding(t *testing.T) {
	fixture := newEffectMembershipFixture(t)
	formal, formalOK := fixture.algebra.FormalCallEffectAtom(fixture.mounted, fixture.owner, 0)
	binding, bindingOK := fixture.algebra.BindFormalCallEffectAtom(fixture.root, fixture.mounted, fixture.owner, 0, formal)
	atom, atomOK := binding.Atom()
	value, valueOK := fixture.algebra.Singleton(atom)
	observation, accumulated := effectfactor.AccumulateEffect(fixture.algebra, effectfactor.BeginEffect(fixture.algebra), value, true, true)
	if !formalOK || !bindingOK || !atomOK || !valueOK || !accumulated || !observation.ProvesAtomBinding(binding) {
		t.Fatal("exact observed atom did not prove its owned binding")
	}
	cloned := effectfactor.CloneEffect(observation)
	if !cloned.ProvesAtomBinding(binding) {
		t.Fatal("frozen observation lost its private membership seal")
	}
	foreignFormal, foreignFormalOK := fixture.algebra.FormalCallEffectAtom(fixture.mounted, fixture.owner, 1)
	foreignBinding, foreignBindingOK := fixture.algebra.BindFormalCallEffectAtom(fixture.root, fixture.mounted, fixture.owner, 1, foreignFormal)
	if !foreignFormalOK || !foreignBindingOK || observation.ProvesAtomBinding(foreignBinding) {
		t.Fatal("observation accepted an unobserved owned binding")
	}
	tampered := effectfactor.CloneEffect(observation)
	tampered.Atoms[0][0] ^= 0xFF
	if tampered.ProvesAtomBinding(binding) {
		t.Fatal("observation accepted a public atom projection that disagrees with its seal")
	}
	foreignAtom, foreignAtomOK := foreignBinding.Atom()
	twoAtoms, twoAtomsOK := fixture.algebra.FromAtoms([]effectfactor.Atom{atom, foreignAtom})
	twoAtomObservation, twoAtomAccumulated := effectfactor.AccumulateEffect(fixture.algebra, effectfactor.BeginEffect(fixture.algebra), twoAtoms, true, true)
	if !foreignAtomOK || !twoAtomsOK || !twoAtomAccumulated || len(twoAtomObservation.Atoms) != 2 {
		t.Fatal("two-atom observation fixture")
	}
	earliest := binding
	if !binding.MatchesCertificate(twoAtomObservation.Atoms[0]) {
		earliest = foreignBinding
	}
	if !twoAtomObservation.ProvesAtomBinding(earliest) {
		t.Fatal("two-atom observation did not prove its first owned binding")
	}
	suffixTampered := effectfactor.CloneEffect(twoAtomObservation)
	suffixTampered.Atoms[len(suffixTampered.Atoms)-1][0] ^= 0xFF
	if suffixTampered.ProvesAtomBinding(earliest) {
		t.Fatal("membership accepted when a later public atom disagreed with its seal")
	}
	top, topOK := effectfactor.AccumulateEffect(fixture.algebra, effectfactor.BeginEffect(fixture.algebra), fixture.algebra.Top(), true, true)
	absent, absentOK := effectfactor.AccumulateEffect(fixture.algebra, effectfactor.BeginEffect(fixture.algebra), fixture.algebra.Bottom(), false, true)
	multiple := effectfactor.CloneEffect(observation)
	multiple.Rows = 2
	invalid := effectfactor.CloneEffect(observation)
	invalid.Valid = false
	if !topOK || !absentOK || top.ProvesAtomBinding(binding) || absent.ProvesAtomBinding(binding) || multiple.ProvesAtomBinding(binding) || invalid.ProvesAtomBinding(binding) {
		t.Fatal("Top, absent, or multi-row Effect observation proved a publication binding")
	}
}
