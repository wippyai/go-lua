package semanticpath

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestStructuralResolverRejectsZeroAndForeignBodyCoordinates(t *testing.T) {
	var resolver *structuralResolver
	if path, ok := resolver.resolve(keyspace.MakeTerm(keyspace.FamilyValues, 1), 0); ok || path.Available() {
		t.Fatal("nil structural resolver resolved a term")
	}
	concrete := &structuralResolver{body: []identity.ContentID{{1}}}
	if path, ok := concrete.resolve(keyspace.MakeTerm(keyspace.FamilyValues, 1), keyspace.MakeTerm(keyspace.FamilyBody, 2)); ok || path.Available() {
		t.Fatal("resolver accepted an out-of-range expected Body")
	}
}
