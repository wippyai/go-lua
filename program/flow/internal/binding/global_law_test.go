package binding

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

func TestSealRejectsInvalidAndEmptyGlobalExactStringAtoms(t *testing.T) {
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1, keyspace.FamilyCell: 1}}
	input.Storage.Cells = []authored.Cell{{Kind: authored.CellGlobal, Key: 1}}
	cases := []struct {
		name  string
		atoms []keyspace.LiteralValue
	}{
		{name: "missing"},
		{name: "integer", atoms: []keyspace.LiteralValue{{Kind: keyspace.LiteralInteger, Integer: 1}}},
		{name: "empty-string", atoms: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: ""}}},
	}
	for _, test := range cases {
		_, err, finish := trySealLawFixture(t, input, [][]keyspace.Term{{}}, nil, nil, test.atoms)
		finish()
		if err == nil {
			t.Fatalf("%s global atom was accepted", test.name)
		}
	}
}

func TestAuthoredRejectsDuplicateGlobalDenseKeys(t *testing.T) {
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1, keyspace.FamilyCell: 2}}
	input.Storage.Cells = []authored.Cell{
		{Kind: authored.CellGlobal, Key: 1},
		{Kind: authored.CellGlobal, Key: 1},
	}
	if _, err := authored.Build(input); err == nil {
		t.Fatal("duplicate global dense keys were admitted to authored storage")
	}
}

func TestSealGlobalRoleHasZeroHost(t *testing.T) {
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1, keyspace.FamilyCell: 1}}
	input.Storage.Cells = []authored.Cell{{Kind: authored.CellGlobal, Key: 1}}
	result, finish := sealLawFixture(t, input, [][]keyspace.Term{{}}, nil, nil,
		[]keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "g"}})
	defer finish()
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	if role, ok := result.Role(cell); !ok || role != kind.CellGlobal {
		t.Fatalf("Role(global) = %v/%v", role, ok)
	}
	if host, ok := result.Host(cell); !ok || host != 0 {
		t.Fatalf("Host(global) = %v/%v, want 0/true", host, ok)
	}
}
