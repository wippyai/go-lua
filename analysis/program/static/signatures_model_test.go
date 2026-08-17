package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestSignaturesModelParameterRowsRetainCoordinatesAndTypes(t *testing.T) {
	input := signatureFixture(t)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	input.Signatures.TypeFunction[0].Parameters[0].Name = 0
	input.Signatures.TypeFunction[0].Parameters[0].Type = 0
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	row, ok := component.View().Signatures().TypeFunctions().ParameterAt(keyspace.MakeTerm(keyspace.FamilyTypeFunction, 1), 0)
	if !ok || row.Name != 9 || row.Type != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1) ||
		row.NameCoordinate == (source.Coordinate{}) {
		t.Fatalf("Parameter model row = %+v/%v", row, ok)
	}
}
