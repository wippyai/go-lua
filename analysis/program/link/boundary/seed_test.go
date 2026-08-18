package boundary

import (
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"testing"

	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
)

func TestBoundarySeedViewsRemainCanonicalAndOwnerFenced(t *testing.T) {
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
	operation, ok := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"op"}})
	if !ok {
		t.Fatal("operation unavailable")
	}
	seed, ok := component.Seeds().ForOperation(operation)
	if !ok {
		t.Fatal("operation seed unavailable")
	}
	id, ok := component.Seeds().ID(seed)
	if !ok {
		t.Fatal("operation seed ID unavailable")
	}
	if _, ok := component.Seeds().ForOperation(vocabulary.Operation(0)); ok {
		t.Fatal("zero operation acquired a seed")
	}
	foreignDraft, err := Build(Input{Project: project, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := foreignDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := foreign.Seeds().ID(seed); ok {
		t.Fatal("foreign Boundary accepted Seed handle")
	}
	if found, ok := foreign.Seeds().ForOperation(operation); !ok {
		t.Fatal("foreign Boundary lost canonical operation seed")
	} else if foundID, ok := foreign.Seeds().ID(found); !ok || foundID != id {
		t.Fatal("equivalent Boundary changed portable operation seed ID")
	}
}
