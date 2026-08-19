package directfunction

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestDirectFunctionAcceptsAllLoopControlKinds(t *testing.T) {
	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	whileBody := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	repeatBody := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	numericBody := keyspace.MakeTerm(keyspace.FamilyBody, 4)
	genericBody := keyspace.MakeTerm(keyspace.FamilyBody, 5)
	nilOne := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	nilTwo := keyspace.MakeTerm(keyspace.FamilyNil, 2)
	boolOne := keyspace.MakeTerm(keyspace.FamilyBool, 1)
	boolTwo := keyspace.MakeTerm(keyspace.FamilyBool, 2)
	boolThree := keyspace.MakeTerm(keyspace.FamilyBool, 3)
	numericValues := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	genericValues := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	numericCell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	genericCell := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	whileLoop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	repeatLoop := keyspace.MakeTerm(keyspace.FamilyLoop, 2)
	numericLoop := keyspace.MakeTerm(keyspace.FamilyLoop, 3)
	genericLoop := keyspace.MakeTerm(keyspace.FamilyLoop, 4)

	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyNil: 2, keyspace.FamilyBool: 3, keyspace.FamilyBody: 5, keyspace.FamilyCell: 2,
		keyspace.FamilyValues: 2, keyspace.FamilyLoop: 4,
	}
	fixture := openDirectFixture(t, directSpec{
		counts:    counts,
		rows:      [][]keyspace.Term{{whileLoop, repeatLoop, numericLoop, genericLoop}, {}, {}, {}, {}},
		nilOwners: []keyspace.Term{entry, repeatBody},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows: []authored.Value{
					{Owner: entry, Fixed: authored.Range{End: 2}},
					{Owner: entry, Fixed: authored.Range{Start: 2, End: 3}},
				},
				Terms: []keyspace.Term{boolOne, boolTwo, boolThree},
			},
			Storage: authored.StorageInput{Cells: []authored.Cell{
				{Kind: authored.CellLocal, Body: numericBody},
				{Kind: authored.CellLocal, Body: genericBody},
			}},
			Control: authored.ControlInput{
				Loops: []authored.Loop{
					{Owner: entry, Body: whileBody, Kind: kind.LoopWhile, Control: nilOne},
					{Owner: entry, Body: repeatBody, Kind: kind.LoopRepeat, Control: nilTwo},
					{Owner: entry, Body: numericBody, Kind: kind.LoopNumericFor, Control: numericValues, Cells: authored.Range{End: 1}},
					{Owner: entry, Body: genericBody, Kind: kind.LoopGenericFor, Control: genericValues, Cells: authored.Range{Start: 1, End: 2}},
				},
				Cells: []keyspace.Term{numericCell, genericCell},
			},
		},
	})
	values := fixture.flow.Values()
	for _, test := range []struct {
		name  string
		owner keyspace.Term
		term  keyspace.Term
		kind  kind.LoopKind
		want  bool
	}{
		{name: "while scalar", owner: entry, term: nilOne, kind: kind.LoopWhile, want: true},
		{name: "repeat scalar", owner: entry, term: nilTwo, kind: kind.LoopRepeat, want: true},
		{name: "numeric values", owner: entry, term: numericValues, kind: kind.LoopNumericFor, want: true},
		{name: "generic values", owner: entry, term: genericValues, kind: kind.LoopGenericFor, want: true},
		{name: "invalid kind", owner: entry, term: nilOne, kind: kind.LoopKind(99), want: false},
		{name: "numeric scalar control", owner: entry, term: nilOne, kind: kind.LoopNumericFor, want: false},
		{name: "generic scalar control", owner: entry, term: nilOne, kind: kind.LoopGenericFor, want: false},
		{name: "numeric foreign Values owner", owner: whileBody, term: numericValues, kind: kind.LoopNumericFor, want: false},
		{name: "generic foreign Values owner", owner: repeatBody, term: genericValues, kind: kind.LoopGenericFor, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := validLoopControl(values, test.owner, test.term, test.kind, counts); got != test.want {
				t.Fatalf("validLoopControl = %v, want %v", got, test.want)
			}
		})
	}
}
