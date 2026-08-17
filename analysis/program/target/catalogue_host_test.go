package target_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/dispatch"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/manifest"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/stdlib"
	"github.com/wippyai/go-lua/types/signature"
)

func TestHostGlobalManifestFlowsDirectlyIntoTarget(t *testing.T) {
	providers := stdlib.Providers()
	providers = append(providers, manifest.Provider{
		Identity: "wippy.restricted-package",
		Mount:    manifest.MountGlobals,
		Declaration: func() *manifestwire.Manifest {
			declaration := manifestwire.New("wippy.host")
			functionType := typ.Func().Param("module", typ.String).Returns(typ.Any).Build()
			declaration.DefineFunctionSignature("require", signature.Function{
				Type: functionType, Effect: effect.Empty.With(dispatch.ModuleLoad{}),
			})
			declaration.DefineGlobalType("require", functionType)
			return declaration
		},
	})
	catalogue, err := manifest.Seal(providers...)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.SealCatalogue(catalogue)
	if err != nil {
		t.Fatal(err)
	}
	binding := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"require"}}
	if _, ok := contract.Lookup(binding); !ok {
		t.Fatal("host require manifest did not become a target operation")
	}
	if _, _, _, _, ok := contract.InitialBinding("require"); !ok {
		t.Fatal("host require manifest did not become an initial global")
	}
}
