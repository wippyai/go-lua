package semanticpath

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestDefinitionQualifierNamesTheAuthoredOrderOfEveryLexicalRole(t *testing.T) {
	for _, test := range []struct {
		role kind.CellRole
		want uint32
	}{
		{kind.CellLocal, uint32(source.CellRoleBind)},
		{kind.CellFormal, uint32(source.CellRoleFormal)},
		{kind.CellCapture, 0},
		{kind.CellFunctionVararg, 0},
		{kind.CellChunkVararg, 0},
	} {
		qualifier, ok := definitionQualifier(test.role, authored.Loops{}, 0)
		if !ok || qualifier != test.want {
			t.Fatalf("definitionQualifier(%v) = %d/%v, want %d/true", test.role, qualifier, ok, test.want)
		}
	}
}

func TestDefinitionQualifierRejectsARoleWithoutAnAuthoredHost(t *testing.T) {
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	if qualifier, ok := definitionQualifier(kind.CellLoop, authored.Loops{}, loop); ok || qualifier != 0 {
		t.Fatalf("definitionQualifier of a Loop Cell with no authored Loop row = %d/%v, want 0/false", qualifier, ok)
	}
	if qualifier, ok := definitionQualifier(kind.CellGlobal, authored.Loops{}, 0); ok || qualifier != 0 {
		t.Fatalf("definitionQualifier(CellGlobal) = %d/%v, want 0/false", qualifier, ok)
	}
}
