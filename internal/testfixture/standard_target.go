package testfixture

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/domain/composite/manifesttarget"
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/dispatch"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/manifest"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/stdlib"
	"github.com/wippyai/go-lua/types/signature"
)

// StandardLibraryTarget seals the native provider manifests through the one
// domain-composition entry point. It is test scaffolding, not a runtime
// registry.
//
// The seal is one value per process. Its whole input is the compiled-in
// provider set, so every caller asks the same question, and a sealed Contract
// is immutable: sealing it again re-derives byte-identical rows, identities and
// canonical bytes for each caller that would otherwise have shared the first.
func StandardLibraryTarget() (*target.Contract, error) {
	return standardLibraryTarget()
}

var standardLibraryTarget = sync.OnceValues(sealStandardLibraryTarget)

func sealStandardLibraryTarget() (*target.Contract, error) {
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
	return manifesttarget.SealCatalogue(catalogue)
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
