package parserproducts

import "testing"

func TestGeneratedParserProductActionTermsHaveContiguousArenaEdges(t *testing.T) {
	terms := generatedActionTerms()
	if len(terms.Symbols) == 0 || len(terms.Scopes) == 0 || len(terms.Terms) == 0 || len(terms.Edges) == 0 {
		t.Fatalf("generated action arena = %#v, want symbols scopes terms and edges", terms)
	}
	for id := ActionTermID(1); id <= ActionTermID(len(terms.Terms)); id++ {
		term, ok := terms.Term(id)
		if !ok || term.Scope == 0 || term.Kind == ActionTermInvalid {
			t.Fatalf("generated action term %d = %#v/%v", id, term, ok)
		}
	}
}
