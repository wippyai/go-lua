package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestGrammarIdentityRejectsWrongABI(t *testing.T) {
	if grammar, ok := NewGrammarIdentity(identity.ContentID{1}, GrammarABIVersion-1); ok || grammar.Available() {
		t.Fatal("wrong grammar ABI was admitted")
	}
	grammar, ok := NewGrammarIdentity(identity.ContentID{1}, GrammarABIVersion)
	if !ok || !grammar.Available() || grammar.SchemaDigest() != (identity.ContentID{1}) {
		t.Fatal("valid grammar identity unavailable")
	}
}
