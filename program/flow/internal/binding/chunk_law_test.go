package binding

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

func TestSealAcceptsRepeatedChunkOccurrenceFromNestedNonFunctionBodies(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	body3 := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	branch := keyspace.MakeTerm(keyspace.FamilyBranch, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	nilTerm := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 3, keyspace.FamilyCell: 1, keyspace.FamilyBranch: 1,
		keyspace.FamilyNil: 1, keyspace.FamilyVararg: 2,
	}}
	input.Storage.Cells = []authored.Cell{{Kind: authored.CellLocal, Body: body1}}
	input.Storage.Varargs = []authored.Vararg{{Owner: body2, Cell: cell}, {Owner: body3, Cell: cell}}
	input.Control.Branches = []authored.Branch{{Owner: body1, Condition: nilTerm, WhenTrue: body2, WhenFalse: body3}}
	result, finish := sealLawFixture(t, input, [][]keyspace.Term{{branch}, {}, {}}, nil, nil, nil)
	defer finish()
	if role, ok := result.Role(cell); !ok || role != kind.CellChunkVararg {
		t.Fatalf("Role(chunk) = %v/%v", role, ok)
	}
	if chunk, ok := result.ChunkVararg(); !ok || chunk != cell {
		t.Fatalf("ChunkVararg = %v/%v, want %v/true", chunk, ok, cell)
	}
}

func TestSealRejectsConflictingChunkOccurrenceCells(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	body3 := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	branch := keyspace.MakeTerm(keyspace.FamilyBranch, 1)
	cell1 := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	cell2 := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 3, keyspace.FamilyCell: 2, keyspace.FamilyBranch: 1,
		keyspace.FamilyNil: 1, keyspace.FamilyVararg: 2,
	}}
	input.Storage.Cells = []authored.Cell{{Kind: authored.CellLocal, Body: body1}, {Kind: authored.CellLocal, Body: body1}}
	input.Storage.Varargs = []authored.Vararg{{Owner: body2, Cell: cell1}, {Owner: body3, Cell: cell2}}
	input.Control.Branches = []authored.Branch{{Owner: body1, Condition: keyspace.MakeTerm(keyspace.FamilyNil, 1), WhenTrue: body2, WhenFalse: body3}}
	_, err, finish := trySealLawFixture(t, input, [][]keyspace.Term{{branch}, {}, {}}, nil, nil, nil)
	finish()
	if err == nil {
		t.Fatal("conflicting chunk occurrence Cells were accepted")
	}
}

func TestSealRejectsNonzeroActivationVarargProviderMismatch(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	cell1 := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	cell2 := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 2, keyspace.FamilyCell: 2, keyspace.FamilyFunction: 1, keyspace.FamilyVararg: 1,
	}}
	input.Storage.Cells = []authored.Cell{{Kind: authored.CellLocal, Body: body2}, {Kind: authored.CellLocal, Body: body2}}
	input.Storage.Varargs = []authored.Vararg{{Owner: body2, Cell: cell2}}
	input.Functions.Rows = []authored.Function{{Owner: body1, Body: body2, Vararg: cell1}}
	_, err, finish := trySealLawFixture(t, input, [][]keyspace.Term{{}, {}}, nil,
		[]source.FunctionFormals{{Function: function}}, nil)
	finish()
	if err == nil {
		t.Fatal("Vararg occurrence mismatching Function.Vararg was accepted")
	}
}
