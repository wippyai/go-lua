package containment

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestProveDirectSourceStatementBelongsToBody(t *testing.T) {
	counts := countsFor(
		c(keyspace.FamilyBody, 1),
		c(keyspace.FamilyNil, 1),
		c(keyspace.FamilyValues, 1),
		c(keyspace.FamilyReturn, 1),
	)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	returned := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	fixture := newProofFixture(t, proofSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{returned}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
				Terms: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyNil, 1)},
			},
			Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values}}},
		},
		module: emptyModule(t),
	})
	result, err := fixture.prove()
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	if parent, ok := result.Parent(returned); !ok || parent != body {
		t.Fatalf("Return parent = %v/%v, want Body %v", parent, ok, body)
	}
	if parent, ok := result.Parent(values); !ok || parent != returned {
		t.Fatalf("Values parent = %v/%v, want Return %v", parent, ok, returned)
	}
}
