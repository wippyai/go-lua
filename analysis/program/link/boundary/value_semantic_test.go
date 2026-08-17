package boundary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
)

func TestBoundaryMountedSemanticDirectoryReturnsIssuedValues(t *testing.T) {
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
	seen := 0
	if !component.Values().VisitMountedSemantics(func(moduleID, occurrenceID identity.ContentID, value Value) bool {
		if !moduleID.Available() || !occurrenceID.Available() {
			t.Fatal("semantic directory returned unavailable identity")
		}
		if _, ok := component.Values().ID(value); !ok {
			t.Fatal("semantic directory returned an unissued Value")
		}
		seen++
		return true
	}) {
		t.Fatal("semantic directory traversal aborted")
	}
	if seen == 0 {
		t.Fatal("mounted Program published no semantic Value rows")
	}
}
