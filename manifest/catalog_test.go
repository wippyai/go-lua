package manifest_test

import (
	"testing"

	"github.com/wippyai/go-lua/domain/effect"
	typetable "github.com/wippyai/go-lua/domain/type/table"
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

func TestCatalogueRejectsInitialRootIdentityCollisions(t *testing.T) {
	provider := func(identity, path, root string) manifest.Provider {
		return manifest.Provider{Identity: identity, Mount: manifest.MountModule, Declaration: func() *moduleio.Manifest {
			declaration := moduleio.New(path)
			declaration.DefineInitialRoot(moduleio.InitialRoot{Identity: root, Aggregate: moduleio.InitialAggregateTable})
			return declaration
		}}
	}
	if _, err := manifest.Seal(
		provider("left", "left", "SharedRoot"),
		provider("right", "right", "SharedRoot"),
	); err == nil {
		t.Fatal("duplicate provider-owned initial root accepted")
	}
	if _, err := manifest.Seal(provider("reserved", "reserved", moduleio.GlobalEnvironmentRoot)); err == nil {
		t.Fatal("provider claimed the global environment root")
	}
}

func TestCatalogueProjectsMountedNonCallableExportsWithoutSecondRegistry(t *testing.T) {
	catalogue, err := manifest.Seal(manifest.Provider{
		Identity: "constants", Mount: manifest.MountModule, Immutable: true,
		Declaration: func() *moduleio.Manifest {
			declaration := moduleio.New("constants")
			declaration.DefineFunctionSignature("call", signature.Function{Type: typ.Func().Build(), Effect: effect.Empty})
			declaration.SetExport(typetable.NewRecord().
				ReadonlyField("answer", typ.LiteralInt(42)).
				ReadonlyField("call", typ.Func().Build()).Build())
			return declaration
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	values := catalogue.Values()
	if len(values) != 1 {
		t.Fatalf("non-callable exports = %d, want 1", len(values))
	}
	binding := values[0].Binding()
	member := binding.Member()
	if binding.Mount() != manifest.MountModule || binding.ModulePath() != "constants" || len(member) != 1 || member[0] != "answer" {
		t.Fatalf("value binding = %d/%q/%v", binding.Mount(), binding.ModulePath(), member)
	}
	if !values[0].Immutable() || !typ.TypeEquals(values[0].Type(), typ.LiteralInt(42)) {
		t.Fatalf("value declaration = immutable:%t type:%v", values[0].Immutable(), values[0].Type())
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
