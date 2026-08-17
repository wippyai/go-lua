package manifest_test

import (
	"testing"

	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/manifest"
	moduleio "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/types/signature"
)

func TestCatalogueRejectsParallelPathAuthorities(t *testing.T) {
	provider := func(identity string) manifest.Provider {
		return manifest.Provider{Identity: identity, Mount: manifest.MountDetached, Declaration: func() *moduleio.Manifest {
			return moduleio.New("same")
		}}
	}
	if _, err := manifest.Seal(provider("left"), provider("right")); err == nil {
		t.Fatal("duplicate manifest path accepted")
	}
}

func TestCatalogueOwnsExactSignatureAndTypeLookup(t *testing.T) {
	catalogue, err := manifest.Seal(manifest.Provider{
		Identity: "provider", Mount: manifest.MountModule,
		Declaration: func() *moduleio.Manifest {
			declaration := moduleio.New("module")
			declaration.DefineType("Value", typ.String)
			declaration.DefineFunctionSignature("read", signature.Function{
				Type: typ.Func().Returns(typ.String).Build(), Effect: effect.Empty,
			})
			return declaration
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	function, ok := catalogue.Function("module.read")
	if !ok {
		t.Fatal("canonical signature missing")
	}
	bindings := function.Bindings()
	if len(bindings) != 1 || bindings[0].Mount() != manifest.MountModule || bindings[0].ModulePath() != "module" || len(bindings[0].Member()) != 1 || bindings[0].Member()[0] != "read" {
		t.Fatalf("mounted function coordinates = %#v", bindings)
	}
	if got, ok := catalogue.Type("module", "Value"); !ok || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("type lookup = %v, %v", got, ok)
	}
	if globals := catalogue.InitialGlobals(); len(globals) != 1 || globals[0] != "module" {
		t.Fatalf("initial globals = %v", globals)
	}
}
