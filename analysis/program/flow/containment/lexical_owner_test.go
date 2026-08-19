package containment

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/static"
	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
)

func TestProveUsesLexicalBodyParentNotConstructHost(t *testing.T) {
	counts := countsFor(
		c(keyspace.FamilyBody, 2),
		c(keyspace.FamilyValues, 1),
		c(keyspace.FamilyFunction, 1),
		c(keyspace.FamilyReturn, 1),
	)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	child := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	returned := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	flow := authored.Input{
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
			Terms: []keyspace.Term{function},
		},
		Functions: authored.FunctionsInput{
			Rows: []authored.Function{{Owner: body, Body: child}},
		},
		Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values}}},
	}
	staticInput := static.Input{
		Contracts: staticcontracts.Input{Function: []staticcontracts.FunctionContract{{}}},
	}
	fixture := newProofFixture(t, proofSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{returned}, nil},
		flow:   flow,
		static: staticInput,
		module: emptyModule(t),
	})
	result, err := fixture.prove()
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	if parent, ok := result.Parent(child); !ok || parent != body {
		t.Fatalf("child Body parent = %v/%v, want lexical Body %v", parent, ok, body)
	}
	if parent, ok := result.Parent(function); !ok || parent != keyspace.MakeTerm(keyspace.FamilyValues, 1) {
		t.Fatalf("Function parent = %v/%v, want Values", parent, ok)
	}
	if result.Contains(function, child) {
		t.Fatal("Function construct became a parent of its executable Body")
	}
}
