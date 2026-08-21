package semanticpath

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestRootClassUsesStableFamilyDiscriminatorsForUnavailableRows(t *testing.T) {
	first := rootClass(authored.View{}, keyspace.MakeTerm(keyspace.FamilyBind, 1))
	second := rootClass(authored.View{}, keyspace.MakeTerm(keyspace.FamilyReturn, 1))
	if !first.Available() || !second.Available() || first == second {
		t.Fatal("rootClass did not preserve stable family-qualified identities")
	}
	paths, err := deriveRootPaths(structuralSourceViewForTest(), nil, [keyspace.FamilyCount][]identity.ContentID{})
	if err != nil || len(paths) != int(keyspace.FamilyCount) {
		t.Fatalf("empty root path derivation = %#v, %v; want empty planes", paths, err)
	}
}

func structuralSourceViewForTest() source.View { return source.View{} }
