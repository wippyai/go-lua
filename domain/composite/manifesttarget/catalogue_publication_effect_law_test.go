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

func TestManifestPublicationEffectLowersToTargetWithIdentity(t *testing.T) {
	publication := manifestwire.PublicationEffectSpec{
		Kind:        manifestwire.PublicationEffectSendTransfer,
		Subject:     manifestwire.InputSource{Kind: manifestwire.InputSourceValue, Ordinal: 0},
		Destination: manifestwire.PublicationDestinationValueFormal,
		Context:     1,
		Escape:      manifestwire.PublicationEscapeSendTransfer,
		Mutability:  manifestwire.PublicationMutabilityCopyOnWrite,
		Lifetime:    manifestwire.PublicationLifetimePreserve,
	}
	contract := sealPublicationCatalogue(t, &publication)
	sink, ok := contract.Operations.Lookup(publicationBinding("sink"))
	if !ok {
		t.Fatal("sink operation missing")
	}
	target, ok := contract.Operations.Lookup(publicationBinding("effect-target"))
	if !ok {
		t.Fatal("effect target operation missing")
	}
	if got, ok := contract.Operations.EffectTarget(sink, 0); !ok || got != target {
		t.Fatalf("effect target = %d/%t, want %d", got, ok, target)
	}
	descriptor, ok := contract.Operations.EffectPublication(sink, 0)
	if !ok || !descriptor.Valid() {
		t.Fatalf("publication descriptor = %#v/%t", descriptor, ok)
	}
	if descriptor.Kind() != vocabulary.PublicationEffectSendTransfer || descriptor.Subject() != (vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0}) ||
		descriptor.DestinationRole() != vocabulary.PublicationDestinationValueFormal || descriptor.Context() != 1 ||
		descriptor.Escape() != vocabulary.PublicationEscapeSendTransfer ||
		descriptor.Mutability() != vocabulary.PublicationMutabilityCopyOnWrite ||
		descriptor.Lifetime() != vocabulary.PublicationLifetimePreserve {
		t.Fatalf("descriptor fields = kind:%d subject:%d destination:%d context:%d escape:%d mutability:%d lifetime:%d",
			descriptor.Kind(), descriptor.Subject(), descriptor.DestinationRole(), descriptor.Context(), descriptor.Escape(), descriptor.Mutability(), descriptor.Lifetime())
	}
	first, firstOK := contract.Operations.PublicationEffectDescriptorID(sink, 0)
	occurrence, occurrenceOK := contract.Operations.PublicationEffectOccurrenceID(sink, 0)
	if !firstOK || !occurrenceOK || !first.Available() || !occurrence.Available() {
		t.Fatal("publication identity was not issued")
	}
	without, ok := sealPublicationCatalogueNoPublication(t)
	if !ok {
		t.Fatal("descriptor-free catalogue did not seal")
	}
	plainSink, ok := without.Operations.Lookup(publicationBinding("sink"))
	if !ok {
		t.Fatal("descriptor-free sink operation missing")
	}
	if _, ok := without.Operations.EffectPublication(plainSink, 0); ok {
		t.Fatal("Target publication was inferred without a manifest descriptor")
	}
}

func TestManifestPublicationEffectRejectsInvalidTypedCombination(t *testing.T) {
	invalid := manifestwire.PublicationEffectSpec{
		Kind:        manifestwire.PublicationEffectSendTransfer,
		Subject:     manifestwire.InputSource{Kind: manifestwire.InputSourceValue, Ordinal: 0},
		Destination: manifestwire.PublicationDestinationNone,
		Escape:      manifestwire.PublicationEscapeNone,
		Mutability:  manifestwire.PublicationMutabilityPreserve,
		Lifetime:    manifestwire.PublicationLifetimePreserve,
	}
	_, err := sealPublicationCatalogueErr(&invalid)
	if err == nil || !strings.Contains(err.Error(), "kind and typed consequences disagree") {
		t.Fatalf("SealCatalogue error = %v, want invalid publication consequence rejection", err)
	}
}

func TestManifestOwnershipLabelDoesNotInferPublicationEffect(t *testing.T) {
	declaration := manifestwire.New("ownership-only")
	fn := typ.Func().Param("value", typ.Any).Returns(typ.Any).Build()
	declaration.DefineFunctionSignature("sink", signature.Function{
		Type: fn, Effect: signatureEffectSend(0),
	})
	providers := stdlib.Providers()
	providers = append(providers, manifest.Provider{
		Identity: "ownership-only",
		Mount:    manifest.MountModule,
		Declaration: func() *manifestwire.Manifest {
			return declaration
		},
	})
	catalogue, err := manifest.Seal(providers...)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := manifesttarget.SealCatalogue(catalogue)
	if err != nil {
		t.Fatal(err)
	}
	sink, ok := contract.Operations.Lookup(ownershipBinding("sink"))
	if !ok {
		t.Fatal("sink operation missing")
	}
	if contract.Operations.EffectCount(sink) != 0 {
		t.Fatalf("ownership-only effect count = %d, want no invocation row", contract.Operations.EffectCount(sink))
	}
}

func TestManifestPublicationEffectPreservesCallbackOccurrence(t *testing.T) {
	contract, err := sealPublicationCallbackCatalogue()
	if err != nil {
		t.Fatal(err)
	}
	sink, ok := contract.Operations.Lookup(publicationBinding("sink"))
	if !ok {
		t.Fatal("sink operation missing")
	}
	callback, ok := contract.Operations.CallbackAt(sink, 0)
	if !ok {
		t.Fatal("callback missing")
	}
	descriptor, ok := contract.Operations.CallbackEffectPublication(callback, 0)
	if !ok || !descriptor.Valid() || descriptor.Kind() != vocabulary.PublicationEffectSendTransfer {
		t.Fatalf("callback publication descriptor = %#v/%t", descriptor, ok)
	}
	target, targetOK := contract.Operations.CallbackEffectTarget(callback, 0)
	effectTarget, effectTargetOK := contract.Operations.Lookup(publicationBinding("effect-target"))
	if !targetOK || !effectTargetOK || target != effectTarget {
		t.Fatalf("callback effect target = %d/%t, want %d/%t", target, targetOK, effectTarget, effectTargetOK)
	}
}

func sealPublicationCatalogue(t testing.TB, publication *manifestwire.PublicationEffectSpec) *targetcontract.Contract {
	t.Helper()
	contract, err := sealPublicationCatalogueErr(publication)
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func sealPublicationCatalogueErr(publication *manifestwire.PublicationEffectSpec) (*targetcontract.Contract, error) {
	declaration := manifestwire.New("publication-host")
	fn := typ.Func().Param("value", typ.Any).Param("context", typ.Any).Returns(typ.Any).Build()
	declaration.DefineFunctionSignature("sink", signature.Function{Type: fn})
	declaration.DefineFunctionSignature("effect-target", signature.Function{Type: fn})
	declaration.DefineGlobalType("sink", fn)
	declaration.DefineGlobalType("effect-target", fn)
	declaration.DefineFunctionOperation("sink", manifestwire.Operation{Effects: manifestwire.RowSpec{
		Occurrences: []manifestwire.EffectSpec{{Target: "publication-host.effect-target", ValueArgs: []manifestwire.ValueFormal{0, 1}, Publication: publication}},
		Tail:        manifestwire.RowClosed,
	}})
	providers := stdlib.Providers()
	providers = append(providers, manifest.Provider{
		Identity: "publication-host",
		Mount:    manifest.MountModule,
		Declaration: func() *manifestwire.Manifest {
			return declaration
		},
	})
	catalogue, err := manifest.Seal(providers...)
	if err != nil {
		return nil, err
	}
	return manifesttarget.SealCatalogue(catalogue)
}

func sealPublicationCatalogueNoPublication(t testing.TB) (*targetcontract.Contract, bool) {
	t.Helper()
	contract, err := sealPublicationCatalogueErr(nil)
	return contract, err == nil
}

func sealPublicationCallbackCatalogue() (*targetcontract.Contract, error) {
	declaration := manifestwire.New("publication-host")
	callbackType := typ.Func().Returns(typ.Any).Build()
	ownerType := typ.Func().Param("callback", callbackType).Param("value", typ.Any).Param("context", typ.Any).Returns(typ.Any).Build()
	targetType := typ.Func().Param("value", typ.Any).Param("context", typ.Any).Returns(typ.Any).Build()
	declaration.DefineFunctionSignature("sink", signature.Function{Type: ownerType})
	declaration.DefineFunctionSignature("effect-target", signature.Function{Type: targetType})
	publication := &manifestwire.PublicationEffectSpec{
		Kind:        manifestwire.PublicationEffectSendTransfer,
		Subject:     manifestwire.InputSource{Kind: manifestwire.InputSourceValue, Ordinal: 0},
		Destination: manifestwire.PublicationDestinationValueFormal,
		Context:     1,
		Escape:      manifestwire.PublicationEscapeSendTransfer,
		Mutability:  manifestwire.PublicationMutabilityCopyOnWrite,
		Lifetime:    manifestwire.PublicationLifetimePreserve,
	}
	emptyValues := manifestwire.Values{Tail: manifestwire.ValuesClosed}
	terminals := []manifestwire.Terminal{
		{Kind: manifestwire.OutcomeNormal, Values: emptyValues},
		{Kind: manifestwire.OutcomeReturn, Values: emptyValues},
		{Kind: manifestwire.OutcomeThrow, Values: emptyValues},
		{Kind: manifestwire.OutcomeYield, Values: emptyValues},
		{Kind: manifestwire.OutcomeCancel, Values: emptyValues},
	}
	declaration.DefineFunctionOperation("sink", manifestwire.Operation{
		Replace: true,
		Input:   manifestwire.Values{Fixed: []typ.Type{callbackType, typ.Any, typ.Any}, Tail: manifestwire.ValuesClosed},
		Outcomes: []manifestwire.Outcome{
			{Kind: manifestwire.OutcomeNormal, Values: emptyValues},
			{Kind: manifestwire.OutcomeThrow, Values: emptyValues},
		},
		Callbacks: []manifestwire.Callback{{
			Function:  manifestwire.InputSource{Kind: manifestwire.InputSourceValue, Ordinal: 0},
			Admission: manifestwire.CallableAdmissionOrdinary,
			Arguments: emptyValues,
			Outcomes:  terminals,
			Lifecycle: manifestwire.CallbackRetainedOptionalOnce,
			Effects: manifestwire.RowSpec{
				Occurrences: []manifestwire.EffectSpec{{Target: "publication-host.effect-target", ValueArgs: []manifestwire.ValueFormal{1, 0}, Publication: publication}},
				Tail:        manifestwire.RowClosed,
			},
		}},
		Effects: manifestwire.RowSpec{Tail: manifestwire.RowClosed},
	})
	providers := stdlib.Providers()
	providers = append(providers, manifest.Provider{
		Identity: "publication-host",
		Mount:    manifest.MountModule,
		Declaration: func() *manifestwire.Manifest {
			return declaration
		},
	})
	catalogue, err := manifest.Seal(providers...)
	if err != nil {
		return nil, err
	}
	return manifesttarget.SealCatalogue(catalogue)
}

func publicationBinding(member string) vocabulary.BindingSpec {
	return vocabulary.BindingSpec{Namespace: vocabulary.BindingModule, Owner: []string{"publication-host"}, Member: []string{member}}
}

func ownershipBinding(member string) vocabulary.BindingSpec {
	return vocabulary.BindingSpec{Namespace: vocabulary.BindingModule, Owner: []string{"ownership-only"}, Member: []string{member}}
}

func signatureEffectSend(index int) effect.Row {
	return effect.Empty.With(ownership.SendParam{Param: effect.ParamRef{Index: index}})
}
