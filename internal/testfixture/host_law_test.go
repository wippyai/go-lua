package testfixture

import (
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/type/ambient"
	"github.com/wippyai/go-lua/domain/type/channelselect"
	"testing"
)

func TestPlacementHostFixturesLinkAgainstCanonicalTarget(t *testing.T) {
	repository, err := RepositoryRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	corpus, err := LoadCorpus(repository)
	if err != nil {
		t.Fatal(err)
	}
	target, err := StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"placement/alias-send",
		"placement/select-receive-local",
		"placement/select-receive-shared-store",
	} {
		t.Run(name, func(t *testing.T) {
			project, err := corpus.Project(name)
			if err != nil {
				t.Fatal(err)
			}
			linked, err := SealCorpusProject(target, project)
			if err != nil {
				t.Fatal(err)
			}
			if linked.Project().Mounts().Count() != project.FileCount() {
				t.Fatalf("mount count = %d, want %d", linked.Project().Mounts().Count(), project.FileCount())
			}
		})
	}
}

func TestStandardLibraryTargetProcessSendCarriesTransferAndFormalSend(t *testing.T) {
	contract, err := StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	op, ok := contract.Operations.Lookup(vocabulary.BindingSpec{
		Namespace: vocabulary.BindingModule,
		Owner:     []string{"process"},
		Member:    []string{"send"},
	})
	if !ok {
		t.Fatal("process.send is not a target operation")
	}
	if got := contract.Operations.TransferCount(op); got != 1 {
		t.Fatalf("process.send transfer count = %d, want 1", got)
	}
	transfer, ok := contract.Operations.TransferIDAt(op, 0)
	if !ok {
		t.Fatal("process.send transfer identity unavailable")
	}
	endpoint, payload, alias, identity, capabilities, ok := contract.Operations.TransferDeclaration(transfer)
	if !ok {
		t.Fatal("process.send transfer declaration unavailable")
	}
	if endpoint != (vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointExternal}) {
		t.Fatalf("process.send endpoint = %#v, want external", endpoint)
	}
	wantSource := vocabulary.InputSource{Kind: vocabulary.InputSourceValuesVar, Ordinal: 0}
	if payload != wantSource || alias != wantSource {
		t.Fatalf("process.send payload/alias = %#v/%#v, want %#v", payload, alias, wantSource)
	}
	if identity != vocabulary.TransferIdentityUnspecified || capabilities != vocabulary.TransferCapabilitiesUnspecified {
		t.Fatalf("process.send relation = identity %d, capabilities %d", identity, capabilities)
	}
	if got := contract.Operations.TransferOutcomeCount(op, 0); got != 2 {
		t.Fatalf("process.send transfer outcome count = %d, want 2", got)
	}
	if _, possibility, ok := contract.Operations.TransferOutcomeAt(op, 0, 0); !ok || possibility != vocabulary.TransferMayDeliver {
		t.Fatalf("process.send normal transfer outcome = %d/%v, want deliver", possibility, ok)
	}
	if _, possibility, ok := contract.Operations.TransferOutcomeAt(op, 0, 1); !ok || possibility != vocabulary.TransferMayReject {
		t.Fatalf("process.send throw transfer outcome = %d/%v, want reject", possibility, ok)
	}
	if got := contract.Operations.FormalEffectCount(op); got != 1 {
		t.Fatalf("process.send formal effect count = %d, want 1", got)
	}
	formal, ok := contract.Operations.FormalEffectAt(op, 0)
	if !ok || formal.Kind != vocabulary.FormalEffectSendSuffix || formal.FromParam != 2 {
		t.Fatalf("process.send formal effect = %#v/%v, want send suffix from 2", formal, ok)
	}
	if got := contract.Operations.EffectCount(op); got != 1 {
		t.Fatalf("process.send publication effect count = %d, want 1", got)
	}
	publication, ok := contract.Operations.EffectPublication(op, 0)
	if !ok || !publication.Valid() || publication.Kind() != vocabulary.PublicationEffectSendTransfer || publication.Subject() != (vocabulary.InputSource{Kind: vocabulary.InputSourceValuesVar, Ordinal: 0}) || publication.Context() != 0 {
		t.Fatalf("process.send publication = %#v/%v, want ValuesVar payload with pid context", publication, ok)
	}
}

func TestStandardLibraryTargetChannelSurfacePreservesSelectAndReceiveKinds(t *testing.T) {
	contract, err := StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	op, ok := contract.Operations.Lookup(vocabulary.BindingSpec{
		Namespace: vocabulary.BindingModule,
		Owner:     []string{channelselect.ModuleName},
		Member:    []string{"select"},
	})
	if !ok {
		t.Fatal("channel.select is not a target operation")
	}
	if _, ok := contract.Operations.Input(op); !ok {
		t.Fatal("channel.select input Values is unavailable")
	}
	if got := contract.Operations.OutcomeCount(op); got != 2 {
		t.Fatalf("channel.select outcomes = %d, want normal and throw", got)
	}
	selectType := channelselect.SelectFunction()
	if !channelselect.IsSelectFunction(selectType) {
		t.Fatal("channel.select declaration lost its canonical select type")
	}
	channel := ambient.ChannelGeneric()
	if channel == nil {
		t.Fatal("ambient channel generic unavailable")
	}
	newOp, ok := contract.Operations.Lookup(vocabulary.BindingSpec{
		Namespace: vocabulary.BindingModule,
		Owner:     []string{channelselect.ModuleName},
		Member:    []string{"new"},
	})
	if !ok {
		t.Fatal("channel.new is not a target operation")
	}
	if contract.Operations.OutcomeCount(newOp) != 2 {
		t.Fatalf("channel.new outcomes = %d, want normal and throw", contract.Operations.OutcomeCount(newOp))
	}
}
