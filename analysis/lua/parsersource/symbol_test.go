package parsersource

import (
	"path/filepath"
	"testing"
)

func TestGrammarVocabularySeparatesTypedTerminalsAndNonterminals(t *testing.T) {
	root := moduleRoot(t)
	vocabulary, err := DiscoverVocabulary(filepath.Join(root, "compiler", "parse", "parser.go.y"))
	if err != nil {
		t.Fatal(err)
	}
	terminal, ok := vocabulary.Symbol("TIdent")
	if !ok || terminal.Kind != SymbolTerminal || terminal.Tag != "token" || terminal.Type == "" {
		t.Fatalf("TIdent = %#v/%v, want typed terminal", terminal, ok)
	}
	nonterminal, ok := vocabulary.Symbol("expr")
	if !ok || nonterminal.Kind != SymbolNonterminal || nonterminal.Tag != "expr" || nonterminal.Type == "" {
		t.Fatalf("expr = %#v/%v, want typed nonterminal", nonterminal, ok)
	}
	if _, ok := vocabulary.Symbol("not-a-symbol"); ok {
		t.Fatal("unknown grammar symbol was resolved")
	}
}
