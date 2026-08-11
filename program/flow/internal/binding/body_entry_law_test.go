package binding

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

func TestSealRejectsZeroInvalidAndNonParentlessEntries(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1, keyspace.FamilyCell: 1, keyspace.FamilyBind: 1, keyspace.FamilyValues: 1,
	}}
	input.Values.Rows = []authored.Value{{Owner: body}}
	input.Storage.Cells = []authored.Cell{{Kind: authored.CellLocal, Body: body}}
	input.Storage.Binds = []authored.Bind{{Owner: body, Values: values}}
	order := []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}}

	for _, entry := range []keyspace.Term{0, keyspace.MakeTerm(keyspace.FamilyCell, 1), keyspace.MakeTerm(keyspace.FamilyBody, 2)} {
		_, err, finish := trySealLawFixtureAtEntry(t, input, [][]keyspace.Term{{bind}}, order, nil, nil, entry)
		finish()
		if err == nil {
			t.Fatalf("entry %v was accepted", entry)
		}
	}
}

func TestSealRejectsSelectingAValidNonParentlessBodyAsEntry(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 2, keyspace.FamilyFunction: 1}}
	input.Functions.Rows = []authored.Function{{Owner: body1, Body: body2}}
	_, err, finish := trySealLawFixtureAtEntry(t, input, [][]keyspace.Term{{}, {}}, nil,
		[]source.FunctionFormals{{Function: function}}, nil, body2)
	finish()
	if err == nil {
		t.Fatal("valid non-parentless Body was accepted as Entry")
	}
}
