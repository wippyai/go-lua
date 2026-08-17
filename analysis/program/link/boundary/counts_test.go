package boundary

import (
	"testing"

	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

func TestBoundaryCountRowsPublishNativeCardinality(t *testing.T) {
	contract := boundaryTarget(t, false)
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
	rows, ok := component.CountRows()
	if !ok || !rows.Available() {
		t.Fatalf("Boundary CountRows = %v/%t", rows, ok)
	}
	value, ok := rows.Value(denominator.GeneratedLinkBoundaryIDs().LinkBoundary)
	cardinality, cardinalityOK := component.Cardinality()
	if !cardinalityOK || !ok || value != uint64(cardinality) {
		t.Fatalf("Boundary denominator cardinality = %d/%t", value, ok)
	}
}
