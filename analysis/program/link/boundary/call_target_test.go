package boundary

import (
	"testing"

	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
)

func TestCallTargetFormalSeparatesOperationAndLoaderSeeds(t *testing.T) {
	contract := boundaryEndpointTarget(t)
	p := boundaryProgram(t)
	projectDraft, err := linkproject.Build(linkproject.Input{Modules: []linkproject.Module{{Name: "main", Program: p}}, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	project, err := projectDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	draft, err := Build(Input{Project: project, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	component, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	operation, ok := contract.Lookup(target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"op"}})
	if !ok {
		t.Fatal("operation unavailable")
	}
	operationSeed, ok := component.Seeds().ForOperation(operation)
	if !ok {
		t.Fatal("operation seed unavailable")
	}
	formal, ok := component.Seeds().CallTarget(operationSeed)
	if !ok || !formal.Valid() || formal.Kind() != CallTargetFormalSeed {
		t.Fatalf("operation CallTarget = %#v/%t", formal, ok)
	}
	if id, ok := formal.ID(); !ok {
		t.Fatal("operation CallTarget ID unavailable")
	} else if want, wantOK := component.Seeds().ID(operationSeed); !wantOK || id != want {
		t.Fatal("operation CallTarget ID diverged from Seed")
	}
	shard, ok := project.Mounts().At(0)
	if !ok {
		t.Fatal("mount unavailable")
	}
	loader, ok := component.Seeds().ScopedLoader(shard)
	if !ok {
		t.Fatal("loader seed unavailable")
	}
	loaderFormal, ok := component.Seeds().CallTarget(loader)
	if !ok || !loaderFormal.Valid() || loaderFormal.Kind() != CallTargetFormalExternal {
		t.Fatalf("loader CallTarget = %#v/%t", loaderFormal, ok)
	}
	if _, ok := component.Seeds().CallTarget(Seed{}); ok {
		t.Fatal("zero Seed acquired CallTarget formal")
	}
}
