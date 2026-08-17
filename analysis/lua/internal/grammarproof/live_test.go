package grammarproof

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/parsersource"
)

func TestLiveGrammarFiltersRecoveryAlternativesAndSortsKeys(t *testing.T) {
	grammar := []parsersource.Production{
		{Nonterminal: "stmt", Ordinal: 2, RHS: []string{"value"}},
		{Nonterminal: "stmt", Ordinal: 1, RHS: []string{"error"}},
		{Nonterminal: "expr", Ordinal: 1, RHS: []string{"value"}},
	}
	rows := liveFromGrammar(grammar)
	if len(rows) != 2 || rows[0].key != "expr#1" || rows[1].key != "stmt#2" {
		t.Fatalf("live rows = %#v, want sorted non-recovery rows", rows)
	}
}
