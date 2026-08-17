package testfixture

import (
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/dispatch"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/manifest"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/stdlib"
	"github.com/wippyai/go-lua/types/signature"
)

// StandardLibraryTarget seals the native provider manifests through Target's
// public catalogue entry point. It is test scaffolding, not a runtime registry.
func StandardLibraryTarget() (*target.Contract, error) {
	providers := stdlib.Providers()
	providers = append(providers, manifest.Provider{
		Identity:    "testfixture.wippy.host",
		Mount:       manifest.MountGlobals,
		Declaration: wippyHostManifest,
	})
	catalogue, err := manifest.Seal(providers...)
	if err != nil {
		return nil, err
	}
	return target.SealCatalogue(catalogue)
}

func wippyHostManifest() *manifestwire.Manifest {
	declaration := manifestwire.New("wippy.host")
	functionType := typ.Func().Param("module", typ.String).Returns(typ.Any).Build()
	declaration.DefineFunctionSignature("require", signature.Function{
		Type:   functionType,
		Effect: effect.Empty.With(dispatch.ModuleLoad{}),
	})
	declaration.DefineGlobalType("require", functionType)
	return declaration
}
