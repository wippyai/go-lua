package query_test

import (
	"testing"

	effectfactor "github.com/wippyai/go-lua/analysis/domain/effect/factor"
	"github.com/wippyai/go-lua/analysis/domain/pack"
	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	"github.com/wippyai/go-lua/analysis/domain/type/authority"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/artifact/schemaadapter"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/analysis/schema/grammar"
	"github.com/wippyai/go-lua/analysis/schema/query"
)

type effectMembershipFixture struct {
	algebra *effectfactor.Algebra
	root    effectfactor.Root
	mounted effectfactor.MountedCall
	owner   target.Operation
}

func newEffectMembershipFixture(t testing.TB) effectMembershipFixture {
	t.Helper()
	published, err := lower.Lower(lower.Source{Name: "effect_observation_law.lua", Text: []byte("local function sink(value) return value end\nsink(1)")})
	if err != nil {
		t.Fatal(err)
	}
	args := target.EffectSpec{Target: 2}
	foreignArgs := target.EffectSpec{Target: 3}
	contract, err := target.Seal(&target.Spec{Operations: []target.OperationSpec{
		{Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"sink"}}}, Input: target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesClosed}, Outcomes: []target.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}}, Effects: target.RowSpec{Occurrences: []target.EffectSpec{args, foreignArgs}, Tail: target.RowClosed}},
		{Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"effect-target"}}}, Input: target.ValuesSpec{Tail: target.ValuesClosed}, Outcomes: []target.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}}, Effects: target.RowSpec{Tail: target.RowClosed}},
		{Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"foreign-effect-target"}}}, Input: target.ValuesSpec{Tail: target.ValuesClosed}, Outcomes: []target.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}}, Effects: target.RowSpec{Tail: target.RowClosed}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "effect_observation_law", Program: published}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, ok := grammar.Global()
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
	artifact, failure := schemaadapter.CompileDetailed(program.TransformerInput(), receipt)
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
	packMount, packOK := pack.NewArtifactMount(artifact, module, programID)
	packs, packsOK := pack.SealMountedArtifacts(linked, statics, []pack.ArtifactMount{packMount})
	algebra, algebraOK := effectfactor.NewWithMountedArtifacts(linked, packs, contract, []effectfactor.MountedArtifact{{ModuleKey: module, Artifact: artifact}})
	owner, ownerOK := contract.Lookup(target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"sink"}})
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
	observation, accumulated := query.AccumulateEffect(fixture.algebra, query.BeginEffect(fixture.algebra), value, true, true)
	if !formalOK || !bindingOK || !atomOK || !valueOK || !accumulated || !observation.ProvesAtomBinding(binding) {
		t.Fatal("exact observed atom did not prove its owned binding")
	}
	cloned := query.CloneEffect(observation)
	if !cloned.ProvesAtomBinding(binding) {
		t.Fatal("frozen observation lost its private membership certificate")
	}
	foreignFormal, foreignFormalOK := fixture.algebra.FormalCallEffectAtom(fixture.mounted, fixture.owner, 1)
	foreignBinding, foreignBindingOK := fixture.algebra.BindFormalCallEffectAtom(fixture.root, fixture.mounted, fixture.owner, 1, foreignFormal)
	if !foreignFormalOK || !foreignBindingOK || observation.ProvesAtomBinding(foreignBinding) {
		t.Fatal("observation accepted an unobserved owned binding")
	}
	tampered := query.CloneEffect(observation)
	tampered.Atoms[0][0] ^= 0xFF
	if tampered.ProvesAtomBinding(binding) {
		t.Fatal("observation accepted a public atom projection that disagrees with its certificate")
	}
	foreignAtom, foreignAtomOK := foreignBinding.Atom()
	twoAtoms, twoAtomsOK := fixture.algebra.FromAtoms([]effectfactor.Atom{atom, foreignAtom})
	twoAtomObservation, twoAtomAccumulated := query.AccumulateEffect(fixture.algebra, query.BeginEffect(fixture.algebra), twoAtoms, true, true)
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
	suffixTampered := query.CloneEffect(twoAtomObservation)
	suffixTampered.Atoms[len(suffixTampered.Atoms)-1][0] ^= 0xFF
	if suffixTampered.ProvesAtomBinding(earliest) {
		t.Fatal("membership accepted when a later public atom disagreed with its certificate")
	}
	top, topOK := query.AccumulateEffect(fixture.algebra, query.BeginEffect(fixture.algebra), fixture.algebra.Top(), true, true)
	absent, absentOK := query.AccumulateEffect(fixture.algebra, query.BeginEffect(fixture.algebra), fixture.algebra.Bottom(), false, true)
	multiple := query.CloneEffect(observation)
	multiple.Rows = 2
	invalid := query.CloneEffect(observation)
	invalid.Valid = false
	if !topOK || !absentOK || top.ProvesAtomBinding(binding) || absent.ProvesAtomBinding(binding) || multiple.ProvesAtomBinding(binding) || invalid.ProvesAtomBinding(binding) {
		t.Fatal("Top, absent, or multi-row Effect observation proved a publication binding")
	}
}
