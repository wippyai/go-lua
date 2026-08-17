package containment

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestProveGlobalCellRootAndChunkCellReachesEntry(t *testing.T) {
	counts := countsFor(
		c(keyspace.FamilyBody, 1),
		c(keyspace.FamilyCell, 2),
		c(keyspace.FamilyVararg, 1),
		c(keyspace.FamilyValues, 1),
		c(keyspace.FamilyReturn, 1),
	)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	global := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	chunk := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	vararg := keyspace.MakeTerm(keyspace.FamilyVararg, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	returned := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	flow := authored.Input{
		Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body, Tail: vararg}}},
		Storage: authored.StorageInput{
			Cells: []authored.Cell{
				{Kind: authored.CellGlobal, Key: 1},
				{Kind: authored.CellLocal, Body: body},
			},
			Varargs: []authored.Vararg{{Owner: body, Cell: chunk}},
		},
		Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values}}},
	}
	fixture := newProofFixture(t, proofSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{returned}},
		flow:   flow,
		exacts: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "global"}},
		module: emptyModule(t),
	})
	result, err := fixture.prove()
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	if parent, ok := result.Parent(global); ok || parent != 0 {
		t.Fatalf("global Cell parent = %v/%v, want root", parent, ok)
	}
	if parent, ok := result.Parent(chunk); !ok || parent != body {
		t.Fatalf("chunk Cell parent = %v/%v, want Entry %v", parent, ok, body)
	}
	if !result.Contains(body, chunk) || result.Contains(global, chunk) {
		t.Fatal("Cell containment intervals are not root/lexical exact")
	}
}
