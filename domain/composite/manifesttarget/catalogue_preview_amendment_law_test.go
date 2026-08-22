package manifesttarget_test

import (
	"strings"
	"testing"

	targetcontract "github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/composite/manifesttarget"
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/ownership"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/manifest"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/stdlib"
	"github.com/wippyai/go-lua/types/signature"
)

// A composition may mount a reference manifest it does not author and still owe
// its corpus a consequence the reference does not state. It says so with a
// preview amendment against the reference operation's canonical path. The
// reference declaration is untouched; the sealed Target answers the union.

// referenceHostManifest is a reference surface: it declares a forwarding
// callable and states no ownership, transfer or publication for it at all.
func referenceHostManifest() *manifestwire.Manifest {
	declaration := manifestwire.New(relationHostModule)
	forward := typ.Func().Param("pid", typ.String).Variadic(typ.Any).Returns(typ.Boolean).Build()
	declaration.DefineFunctionSignature("forward", signature.Function{Type: forward})
	return declaration
}

func sealWithPreview(amendments ...manifesttarget.PreviewAmendment) (*targetcontract.Contract, error) {
	providers := stdlib.Providers()
	providers = append(providers, manifest.Provider{
		Identity:    relationHostModule,
		Mount:       manifest.MountModule,
		Declaration: referenceHostManifest,
	})
	catalogue, err := manifest.Seal(providers...)
	if err != nil {
		return nil, err
	}
	return manifesttarget.SealCatalogue(catalogue, amendments...)
}

// forwardPreview states the cross-boundary consequences of the reference
// forwarding callable: the suffix leaves the caller's ownership and the
// payload is published across an external boundary.
func forwardPreview() manifesttarget.PreviewAmendment {
	return manifesttarget.PreviewAmendment{
		Operation: relationHostModule + ".forward",
		Effect:    []effect.Label{ownership.Send{FromParam: 1}},
		HasLaw:    true,
		Law: manifestwire.Operation{
			Effects: manifestwire.RowSpec{
				Occurrences: []manifestwire.EffectSpec{{
					Target:     relationHostModule + ".forward",
					ValueArgs:  []manifestwire.ValueFormal{0},
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
		},
	}
}

// TestPreviewAmendmentAddsConsequencesToAReferenceOperation is the positive
// law: the amendment's ownership label becomes the operation's formal effect
// and its law becomes the operation's transfer and publication rows.
func TestPreviewAmendmentAddsConsequencesToAReferenceOperation(t *testing.T) {
	contract, err := sealWithPreview(forwardPreview())
	if err != nil {
		t.Fatal(err)
	}
	operation, ok := contract.Operations.Lookup(relationBinding("forward"))
	if !ok {
		t.Fatal("sealed Target holds no forward operation")
	}
	if got := contract.Operations.FormalEffectCount(operation); got != 1 {
		t.Fatalf("formal effect count = %d, want the amended ownership row", got)
	}
	formal, formalOK := contract.Operations.FormalEffectAt(operation, 0)
	if !formalOK || formal.Kind != vocabulary.FormalEffectSendSuffix || formal.FromParam != 1 {
		t.Fatalf("formal effect = %#v/%t, want send suffix from 1", formal, formalOK)
	}
	if got := contract.Operations.TransferCount(operation); got != 1 {
		t.Fatalf("transfer count = %d, want the amended transfer", got)
	}
	endpoint, endpointOK := contract.Operations.TransferEndpointAt(operation, 0)
	if !endpointOK || endpoint.Kind != vocabulary.TransferEndpointExternal {
		t.Fatalf("transfer endpoint = %+v/%t, want external", endpoint, endpointOK)
	}
	publication, publicationOK := contract.Operations.EffectPublication(operation, 0)
	if !publicationOK || !publication.Valid() || publication.Kind() != vocabulary.PublicationEffectSendTransfer {
		t.Fatalf("publication = %#v/%t, want the amended send transfer", publication, publicationOK)
	}
}

// Without the amendment the same reference declares none of it. The reference
// manifest is the only thing that changed between the two seals, and it did
// not change: the consequences belong to the amendment alone.
func TestReferenceOperationCarriesNoUnamendedConsequences(t *testing.T) {
	contract, err := sealWithPreview()
	if err != nil {
		t.Fatal(err)
	}
	operation, ok := contract.Operations.Lookup(relationBinding("forward"))
	if !ok {
		t.Fatal("sealed Target holds no forward operation")
	}
	if got := contract.Operations.FormalEffectCount(operation); got != 0 {
		t.Fatalf("unamended formal effect count = %d, want none", got)
	}
	if got := contract.Operations.TransferCount(operation); got != 0 {
		t.Fatalf("unamended transfer count = %d, want none", got)
	}
	if got := contract.Operations.EffectCount(operation); got != 0 {
		t.Fatalf("unamended publication effect count = %d, want none", got)
	}
}

// An amendment against a path the catalogue does not hold is a composition
// error. Dropping it silently would let a composition believe it had stated a
// consequence the sealed Target never carries.
func TestPreviewAmendmentRefusesAnUnknownOperation(t *testing.T) {
	amendment := forwardPreview()
	amendment.Operation = relationHostModule + ".absent"
	_, err := sealWithPreview(amendment)
	if err == nil {
		t.Fatal("an amendment naming an unknown operation sealed, want a named refusal")
	}
	if !strings.Contains(err.Error(), "names unknown operation") {
		t.Fatalf("refusal = %v, want the named unknown-operation refusal", err)
	}
}

// A provider that already declares its own operational law is the authority for
// that boundary. An amendment may not overwrite it, because two authorities
// would then answer one question.
func TestPreviewAmendmentRefusesToOverwriteAnAuthoredLaw(t *testing.T) {
	providers := stdlib.Providers()
	providers = append(providers, manifest.Provider{
		Identity: relationHostModule,
		Mount:    manifest.MountModule,
		Declaration: func() *manifestwire.Manifest {
			declaration := referenceHostManifest()
			declaration.DefineFunctionOperation("forward", manifestwire.Operation{
				Effects: manifestwire.RowSpec{Tail: manifestwire.RowClosed},
			})
			return declaration
		},
	})
	catalogue, err := manifest.Seal(providers...)
	if err != nil {
		t.Fatal(err)
	}
	_, err = manifesttarget.SealCatalogue(catalogue, forwardPreview())
	if err == nil {
		t.Fatal("an amendment overwrote a provider-authored law, want a named refusal")
	}
	if !strings.Contains(err.Error(), "already declares an operational law") {
		t.Fatalf("refusal = %v, want the named authored-law refusal", err)
	}
}
