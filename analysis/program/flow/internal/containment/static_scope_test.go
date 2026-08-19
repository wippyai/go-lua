package containment

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
	staticoperators "github.com/wippyai/go-lua/analysis/program/static/operators"
)

func TestProveRejectsTypeOfScopeFromDifferentBody(t *testing.T) {
	counts := countsFor(
		c(keyspace.FamilyBody, 2),
		c(keyspace.FamilyCell, 1),
		c(keyspace.FamilyRead, 1),
		c(keyspace.FamilyValues, 1),
		c(keyspace.FamilyBind, 1),
		c(keyspace.FamilyFunction, 1),
		c(keyspace.FamilyTypeOf, 1),
	)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	child := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	fixture := newProofFixture(t, proofSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{bind}, nil},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
				Terms: []keyspace.Term{function},
			},
			Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: body, Body: child}}},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}},
				Reads: []authored.Read{{Owner: child, Source: cell}},
				Binds: []authored.Bind{{Owner: body, Values: values}},
			},
		},
		static: static.Input{
			Contracts: staticcontracts.Input{Function: []staticcontracts.FunctionContract{{}}},
			Operators: staticoperators.Input{TypeOf: []staticoperators.TypeOf{{Scope: cell, Operand: read}}},
		},
		binds:  []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		module: emptyModule(t),
	})
	if _, err := fixture.prove(); err == nil {
		t.Fatal("Prove accepted TypeOf whose operand belongs to a different Body")
	}
}
