package flow

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestRowsModelResetsAllOwnedPoolsAndBoundsRanges(t *testing.T) {
	rows := Rows{}
	if _, ok := rows.AppendValue(authored.Value{}, []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyInteger, 1)}); !ok {
		t.Fatal("AppendValue rejected a first member")
	}
	if got, ok := rows.ValueTermAt(0); !ok || got != keyspace.MakeTerm(keyspace.FamilyInteger, 1) {
		t.Fatalf("ValueTermAt(0) = %v/%v", got, ok)
	}
	rows.Reset()
	if _, ok := rows.ValueAt(0); ok {
		t.Fatal("Reset retained a Value row")
	}
	if _, ok := rangeFor(-1, 1); ok {
		t.Fatal("rangeFor accepted a negative pool length")
	}
}
