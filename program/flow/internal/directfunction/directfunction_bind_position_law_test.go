package directfunction

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// TestDirectFunctionAcceptsBindValueAdjustments keeps Source's Cell order
// independent from the evaluated Values width. A Bind may consume fixed
// positions, nil-fill after a short fixed range, or an open tail; fixed
// positions beyond the Cell order are simply not consumed.
func TestDirectFunctionAcceptsBindValueAdjustments(t *testing.T) {
	for _, test := range []struct {
		name string
		spec directSpec
	}{
		{name: "short nil-fill", spec: bindPositionShortSpec()},
		{name: "extra fixed values", spec: bindPositionExtraSpec()},
		{name: "open tail", spec: bindPositionTailSpec()},
		{name: "exact fixed", spec: bindPositionExactSpec()},
	} {
		t.Run(test.name, func(t *testing.T) {
			openDirectFixture(t, test.spec)
		})
	}
}

func bindPositionShortSpec() directSpec {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	return directSpec{
		counts: [keyspace.FamilyCount]uint32{
			keyspace.FamilyBody: 1, keyspace.FamilyCell: 1,
			keyspace.FamilyValues: 1, keyspace.FamilyBind: 1,
		},
		rows:  [][]keyspace.Term{{bind}},
		binds: []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body}}},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}},
				Binds: []authored.Bind{{Owner: body, Values: values}},
			},
		},
	}
}

func bindPositionExtraSpec() directSpec {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	return directSpec{
		counts: [keyspace.FamilyCount]uint32{
			keyspace.FamilyNil: 2, keyspace.FamilyBody: 1, keyspace.FamilyCell: 1,
			keyspace.FamilyValues: 1, keyspace.FamilyBind: 1,
		},
		rows:  [][]keyspace.Term{{bind}},
		binds: []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 2}}},
				Terms: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyNil, 1), keyspace.MakeTerm(keyspace.FamilyNil, 2)},
			},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}},
				Binds: []authored.Bind{{Owner: body, Values: values}},
			},
		},
	}
}

func bindPositionTailSpec() directSpec {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	varargCell := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	vararg := keyspace.MakeTerm(keyspace.FamilyVararg, 1)
	return directSpec{
		counts: [keyspace.FamilyCount]uint32{
			keyspace.FamilyBody: 1, keyspace.FamilyCell: 2, keyspace.FamilyVararg: 1,
			keyspace.FamilyValues: 1, keyspace.FamilyBind: 1,
		},
		rows:  [][]keyspace.Term{{bind}},
		binds: []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body, Tail: vararg}}},
			Storage: authored.StorageInput{
				Cells:   []authored.Cell{{Kind: authored.CellLocal, Body: body}, {Kind: authored.CellLocal, Body: body}},
				Varargs: []authored.Vararg{{Owner: body, Cell: varargCell}},
				Binds:   []authored.Bind{{Owner: body, Values: values}},
			},
		},
	}
}

func bindPositionExactSpec() directSpec {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	child := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	return directSpec{
		counts: [keyspace.FamilyCount]uint32{
			keyspace.FamilyBody: 2, keyspace.FamilyCell: 1,
			keyspace.FamilyValues: 1, keyspace.FamilyBind: 1, keyspace.FamilyFunction: 1,
		},
		rows:  [][]keyspace.Term{{bind}, {}},
		binds: []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		forms: []source.FunctionFormals{{Function: function}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
				Terms: []keyspace.Term{function},
			},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}},
				Binds: []authored.Bind{{Owner: body, Values: values}},
			},
			Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: body, Body: child}}},
		},
	}
}
