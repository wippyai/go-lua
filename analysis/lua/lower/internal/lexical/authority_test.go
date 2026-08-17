package lexical

import "testing"

func TestLexicalAuthorityStartsClean(t *testing.T) {
	b := New(nil, nil, nil, "lexical.lua", nil, nil, nil)
	if b == nil || !b.Clean() {
		t.Fatal("new lexical authority did not start clean")
	}
}

func TestLexicalSpanUsesCanonicalSourceName(t *testing.T) {
	b := New(nil, nil, nil, "lexical.lua", nil, nil, nil)
	if got := b.span(nil); got.File != "lexical.lua" {
		t.Fatalf("nil span file = %q, want lexical.lua", got.File)
	}
}
