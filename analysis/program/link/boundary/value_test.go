package boundary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
)

func TestBoundaryValuesRejectForeignHandles(t *testing.T) {
	contract := boundaryEndpointTarget(t)
	p := boundaryProgram(t)
	build := func() *Component {
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
	left, right := build(), build()
	leftValue, ok := left.Values().At(0)
	if !ok {
		t.Fatal("left Value unavailable")
	}
	rightValue, ok := right.Values().At(0)
	if !ok {
		t.Fatal("right Value unavailable")
	}
	if _, ok := left.Values().Compare(leftValue, rightValue); ok {
		t.Fatal("foreign equivalent Value crossed Boundary owner fence")
	}
	if _, ok := left.Values().FindID(identity.ContentID{}); ok {
		t.Fatal("unavailable Value identity resolved")
	}
}
