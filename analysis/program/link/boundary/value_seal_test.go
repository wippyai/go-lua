package boundary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
)

func TestBoundaryValueSealTracksMountedProgramRelation(t *testing.T) {
	contract := boundaryEndpointTarget(t)
	build := func(text string) *Component {
		p, err := lower.Lower(lower.Source{Name: "boundary-value-seal", Text: []byte(text)})
		if err != nil {
			t.Fatal(err)
		}
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
		return component
	}
	base, changed := build(`local value = 1; return value`), build(`local value = 2; return value`)
	baseID, ok := base.ValueRelationID()
	if !ok {
		t.Fatal("base Value relation unavailable")
	}
	changedID, ok := changed.ValueRelationID()
	if !ok || changedID == baseID {
		t.Fatal("mounted Program value delta did not change Value relation")
	}
}
