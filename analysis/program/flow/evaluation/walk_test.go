package evaluation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestSessionWalksRuntimeExactUnaryOperand(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	base := keyspace.MakeTerm(keyspace.FamilyInteger, 1)
	operand := keyspace.MakeTerm(keyspace.FamilyInteger, 2)
	unary := keyspace.MakeTerm(keyspace.FamilyUnary, 1)
	lens := keyspace.MakeTerm(keyspace.FamilyLensExact, 1)
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1, keyspace.FamilyInteger: 2,
		keyspace.FamilyUnary: 1, keyspace.FamilyLensExact: 1,
	}
	draft, err := authored.Build(authored.Input{
		Counts:    counts,
		Access:    authored.AccessInput{Exact: []authored.ExactLens{{Owner: body, Base: base, Source: unary, Kind: kind.FieldExact}}},
		Operators: authored.OperatorsInput{Unaries: []authored.Unary{{Owner: body, Op: kind.UnaryNeg, Operand: operand}}},
	})
	if err != nil {
		t.Fatalf("authored.Build: %v", err)
	}
	finalize, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("authored.Finalizer: %v", err)
	}
	t.Cleanup(func() { _ = finalize.Abort() })
	walker, err := New(finalize.View())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := walker.Start(lens); err != nil {
		t.Fatalf("Start: %v", err)
	}
	for {
		_, ok, err := walker.Next()
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		if !ok && walker.done {
			break
		}
	}
	if !walker.seen[keyspace.FamilyUnary][1] {
		t.Fatal("ordinary Session treated runtime UnaryNeg exact operand as static metadata")
	}
}
