package testfixture

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/domain/composite/manifesttarget"
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/dispatch"
	"github.com/wippyai/go-lua/domain/effect/ownership"
	"github.com/wippyai/go-lua/domain/type/ambient"
	"github.com/wippyai/go-lua/domain/type/channelselect"
	typetable "github.com/wippyai/go-lua/domain/type/table"
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
func StandardLibraryTarget() (*contract.Contract, error) {
	return standardLibraryTarget()
}

var standardLibraryTarget = sync.OnceValues(sealStandardLibraryTarget)

func sealStandardLibraryTarget() (*contract.Contract, error) {
	providers := stdlib.Providers()
	providers = append(providers, manifest.Provider{
		Identity:    "testfixture.wippy.host",
		Mount:       manifest.MountGlobals,
		Declaration: wippyHostManifest,
	}, manifest.Provider{
		Identity:    "testfixture.wippy.channel",
		Mount:       manifest.MountModule,
		Declaration: channelHostManifest,
	}, manifest.Provider{
		Identity:    "testfixture.wippy.process",
		Mount:       manifest.MountModule,
		Declaration: processHostManifest,
	}, manifest.Provider{
		Identity:    "testfixture.wippy.uuid",
		Mount:       manifest.MountModule,
		Declaration: uuidHostManifest,
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

func channelHostManifest() *manifestwire.Manifest {
	selectType := channelselect.SelectFunction()
	channelType := typ.Instantiate(ambient.ChannelGeneric(), typ.Any)
	newType := typ.Func().OptParam("buffer", typ.Integer).Returns(channelType).Build()
	declaration := manifestwire.New(channelselect.ModuleName)
	declaration.DefineFunctionSignature("select", signature.Function{Type: selectType})
	declaration.DefineFunctionSignature("new", signature.Function{Type: newType})
	declaration.SetExport(typetable.NewRecord().
		Field("select", selectType).
		Field("new", newType).
		Build())
	return declaration
}

// uuidHostManifest declares the identifier generator surface the corpus
// fixtures require. Only v7 is declared, because that is the only member the
// fixture corpus calls; a generated identifier is a plain string value and the
// call carries no ownership, dispatch, or transfer effect.
func uuidHostManifest() *manifestwire.Manifest {
	v7Type := typ.Func().Returns(typ.String).Build()
	declaration := manifestwire.New("uuid")
	declaration.DefineFunctionSignature("v7", signature.Function{Type: v7Type})
	declaration.SetExport(typetable.NewRecord().
		Field("v7", v7Type).
		Build())
	return declaration
}

func processHostManifest() *manifestwire.Manifest {
	channelType := typ.Instantiate(ambient.ChannelGeneric(), typ.Any)
	sendType := typ.Func().
		Param("pid", typ.String).
		Param("topic", typ.String).
		Variadic(typ.Any).
		Returns(typ.Boolean).
		Build()
	declaration := manifestwire.New("process")
	declaration.DefineFunctionSignature("send", signature.Function{
		Type:   sendType,
		Effect: effect.Empty.With(ownership.Send{FromParam: 2}),
	})
	listenType := typ.Func().Param("topic", typ.String).OptParam("options", typ.Any).Returns(channelType).Build()
	listenNestedType := typ.Func().Param("topic", typ.String).OptParam("options", typ.Any).Returns(channelType).Build()
	receiveMapType := typ.Func().Param("channel", channelType).Param("transform", typ.Any).Returns(typ.Any).Build()
	runType := typ.Func().Param("config", typ.Any).Returns(typ.Any, typ.Any).Build()
	declaration.DefineFunctionSignature("listen", signature.Function{Type: listenType})
	declaration.DefineFunctionSignature("listen_nested", signature.Function{Type: listenNestedType})
	declaration.DefineFunctionSignature("receive_map", signature.Function{Type: receiveMapType})
	declaration.DefineFunctionSignature("run", signature.Function{Type: runType})
	declaration.SetExport(typetable.NewRecord().
		Field("send", sendType).
		Field("listen", listenType).
		Field("listen_nested", listenNestedType).
		Field("receive_map", receiveMapType).
		Field("run", runType).
		Build())
	declaration.DefineFunctionOperation("send", manifestwire.Operation{
		Effects: manifestwire.RowSpec{
			Occurrences: []manifestwire.EffectSpec{{
				Target:     "process.send",
				ValueArgs:  []manifestwire.ValueFormal{0, 1},
				ValuesArgs: []manifestwire.ValuesVar{0},
				Publication: &manifestwire.PublicationEffectSpec{
					Kind:        manifestwire.PublicationEffectSendTransfer,
					Subject:     manifestwire.InputSource{Kind: manifestwire.InputSourceValues, Ordinal: 0},
					Destination: manifestwire.PublicationDestinationValueFormal,
					Context:     0,
					Escape:      manifestwire.PublicationEscapeSendTransfer,
					Mutability:  manifestwire.PublicationMutabilityCopyOnWrite,
					Lifetime:    manifestwire.PublicationLifetimePreserve,
				},
			}},
			Tail: manifestwire.RowClosed,
		},
		Transfers: []manifestwire.TransferSpec{{
			Endpoint:     manifestwire.TransferEndpoint{Kind: manifestwire.TransferEndpointExternal},
			Payload:      manifestwire.InputSource{Kind: manifestwire.InputSourceValues},
			Alias:        manifestwire.InputSource{Kind: manifestwire.InputSourceValues},
			Identity:     manifestwire.TransferIdentityUnspecified,
			Capabilities: manifestwire.TransferCapabilitiesUnspecified,
			Outcomes: []manifestwire.TransferOutcomeSpec{
				{Outcome: 0, Possibility: manifestwire.TransferMayDeliver},
				{Outcome: 1, Possibility: manifestwire.TransferMayReject},
			},
		}},
	})
	return declaration
}
