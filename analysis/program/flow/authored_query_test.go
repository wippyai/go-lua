package flow_test

import (
	"testing"

	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestAuthoredQueriesRoundTripOwnerRows(t *testing.T) {
	program, err := lualower.Lower(lualower.Source{
		Name: "authored-query.lua",
		Text: []byte("local value = 1\nreturn value + 2"),
	})
	if err != nil {
		t.Fatal(err)
	}
	authored := program.Flow().Authored()
	if authored.Values().Count() == 0 || authored.Operators().Binaries().Count() == 0 {
		t.Fatalf("authored rows = values %d, binaries %d; want both source relations", authored.Values().Count(), authored.Operators().Binaries().Count())
	}
	for index := 0; index < authored.Values().Count(); index++ {
		term, ok := authored.Values().At(index)
		if !ok || keyspace.TermFamily(term) != keyspace.FamilyValues {
			t.Fatalf("Values.At(%d) = %v/%v, want a Values term", index, term, ok)
		}
		if _, _, ok := authored.Values().Get(term); !ok {
			t.Fatalf("Values.Get(%v) did not resolve its authored row", term)
		}
	}
	for index := 0; index < authored.Operators().Binaries().Count(); index++ {
		term, ok := authored.Operators().Binaries().At(index)
		if !ok || keyspace.TermFamily(term) != keyspace.FamilyBinary {
			t.Fatalf("Binaries.At(%d) = %v/%v, want a Binary term", index, term, ok)
		}
		if _, _, _, _, ok := authored.Operators().Binaries().Get(term); !ok {
			t.Fatalf("Binaries.Get(%v) did not resolve its authored row", term)
		}
	}
}
