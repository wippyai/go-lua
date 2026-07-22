package state

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestCoordinateFormalRootRekeyCarriesImmutableLexicalClass(t *testing.T) {
	domain := RegisteredProductDomain(standard.Registry())
	var owner lexicalidentity.StableLexicalBodyID
	owner[0] = 91
	from, to := keyspace.New(), keyspace.New()
	source := from.FromPath(pathdom.NewPath(71, ""))
	input, middle := formal.NewRoot(owner, 1, formal.Input), formal.NewRoot(owner, 1, formal.Middle)
	class := formal.NewLexicalClassID(owner, 1)
	plan, err := domain.SealCoordinateFormalRootRekey(owner, from, to, []CoordinateFormalRootBinding{
		{Source: source, Target: input, Class: class},
		{Source: source, Target: middle, Class: class, ResolverVersions: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, root := range []formal.Root{input, middle} {
		got, ok := domain.CoordinateFormalRootClass(plan, root)
		if !ok || got != class {
			t.Fatalf("class(%#v) = %#v, %v", root, got, ok)
		}
	}
	if _, ok := domain.CoordinateFormalRootClass(plan, formal.NewRoot(owner, 2, formal.Input)); ok {
		t.Fatal("unregistered root acquired lexical class")
	}
}
