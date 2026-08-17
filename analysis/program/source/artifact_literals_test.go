package source

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestArtifactLiteralsRetainOwnerAndPayloadByFamily(t *testing.T) {
	input, index := sourceFixture(1)
	component := finalizeSource(t, input, index)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	if _, owner, value, ok := component.View().Literals().Bools().At(0); !ok || owner != body || !value {
		t.Fatalf("bool literal = %v/%v/%v, want body/true/true", owner, value, ok)
	}
	if _, owner, value, ok := component.View().Literals().Integers().At(0); !ok || owner != keyspace.MakeTerm(keyspace.FamilyBody, 2) || value != 42 {
		t.Fatalf("integer literal = %v/%d/%v, want Body2/42/true", owner, value, ok)
	}
	if _, _, _, ok := component.View().Literals().Strings().At(-1); ok {
		t.Fatal("string literal accepted a negative index")
	}
}
