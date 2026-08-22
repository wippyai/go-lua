package manifesttarget_test

import (
	"testing"

	targetcontract "github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"github.com/wippyai/go-lua/domain/composite/manifesttarget"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/manifest"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/stdlib"
	"github.com/wippyai/go-lua/types/signature"
)

// relationHostModule is the module name every declaration in this file is
// authored under. The tests seal a real provider manifest through the one
// composition entry point and read the declaration back out of the sealed
// Target, so the wire spelling and the sealed answer are checked together.
const relationHostModule = "relation-host"

func relationBinding(member string) vocabulary.BindingSpec {
	return vocabulary.BindingSpec{
		Namespace: vocabulary.BindingModule,
		Owner:     []string{relationHostModule},
		Member:    []string{member},
	}
}

// sealRelationCatalogue seals one authored declaration through the same
// provider composition the fixture Target uses.
func sealRelationCatalogue(declaration *manifestwire.Manifest) (*targetcontract.Contract, error) {
	providers := stdlib.Providers()
	providers = append(providers, manifest.Provider{
		Identity: relationHostModule,
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

// transferDeclaration is one authored transfer relation and the sealed answer
// it must produce.
type transferDeclaration struct {
	member       string
	endpoint     manifestwire.TransferEndpoint
	payload      manifestwire.InputSource
	alias        manifestwire.InputSource
	identity     manifestwire.TransferIdentity
	capabilities manifestwire.TransferCapabilities

	wantEndpoint     vocabulary.TransferEndpoint
	wantIdentity     vocabulary.TransferIdentity
	wantCapabilities vocabulary.TransferCapabilities
}

// declaredTransferRelations names every value in the wire transfer endpoint,
// identity and capability vocabularies. A transfer whose endpoint is one of
// the callable's own inputs addresses that input by ordinal; an external
// endpoint names no input and must leave the ordinal at zero.
func declaredTransferRelations() []transferDeclaration {
	formal := func(ordinal uint32) manifestwire.InputSource {
		return manifestwire.InputSource{Kind: manifestwire.InputSourceValue, Ordinal: ordinal}
	}
	return []transferDeclaration{{
		member:       "send_to_input",
		endpoint:     manifestwire.TransferEndpoint{Kind: manifestwire.TransferEndpointInput, Input: 0},
		payload:      formal(1),
		alias:        formal(1),
		identity:     manifestwire.TransferIdentitySame,
		capabilities: manifestwire.TransferCapabilitiesPreserveAll,

		wantEndpoint:     vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointInput, Input: 0},
		wantIdentity:     vocabulary.TransferIdentitySame,
		wantCapabilities: vocabulary.TransferCapabilitiesPreserveAll,
	}, {
		member:       "send_to_external",
		endpoint:     manifestwire.TransferEndpoint{Kind: manifestwire.TransferEndpointExternal},
		payload:      formal(1),
		alias:        formal(1),
		identity:     manifestwire.TransferIdentityDistinct,
		capabilities: manifestwire.TransferCapabilitiesLoseAll,

		wantEndpoint:     vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointExternal},
		wantIdentity:     vocabulary.TransferIdentityDistinct,
		wantCapabilities: vocabulary.TransferCapabilitiesLoseAll,
	}, {
		member:       "send_unspecified",
		endpoint:     manifestwire.TransferEndpoint{Kind: manifestwire.TransferEndpointExternal},
		payload:      formal(1),
		alias:        formal(1),
		identity:     manifestwire.TransferIdentityUnspecified,
		capabilities: manifestwire.TransferCapabilitiesUnspecified,

		wantEndpoint:     vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointExternal},
		wantIdentity:     vocabulary.TransferIdentityUnspecified,
		wantCapabilities: vocabulary.TransferCapabilitiesUnspecified,
	}}
}

func transferHostManifest() *manifestwire.Manifest {
	declaration := manifestwire.New(relationHostModule)
	memberType := typ.Func().Param("endpoint", typ.Any).Param("payload", typ.Any).Returns(typ.Boolean).Build()
	for _, relation := range declaredTransferRelations() {
		declaration.DefineFunctionSignature(relation.member, signature.Function{Type: memberType})
		declaration.DefineFunctionOperation(relation.member, manifestwire.Operation{
			Transfers: []manifestwire.TransferSpec{{
				Endpoint:     relation.endpoint,
				Payload:      relation.payload,
				Alias:        relation.alias,
				Identity:     relation.identity,
				Capabilities: relation.capabilities,
				Outcomes: []manifestwire.TransferOutcomeSpec{
					{Outcome: 0, Possibility: manifestwire.TransferMayDeliver},
					{Outcome: 1, Possibility: manifestwire.TransferMayReject},
				},
			}},
		})
	}
	return declaration
}

// TestManifestDeclaresEveryTransferRelation is the positive law for the
// transfer endpoint, identity and capability vocabularies: each value is
// declared by a real manifest, survives SealCatalogue, and is answered by the
// sealed Target exactly as authored.
func TestManifestDeclaresEveryTransferRelation(t *testing.T) {
	contract, err := sealRelationCatalogue(transferHostManifest())
	if err != nil {
		t.Fatal(err)
	}
	for _, relation := range declaredTransferRelations() {
		t.Run(relation.member, func(t *testing.T) {
			operation, ok := contract.Operations.Lookup(relationBinding(relation.member))
			if !ok {
				t.Fatalf("sealed Target holds no operation for %s", relation.member)
			}
			if count := contract.Operations.TransferCount(operation); count != 1 {
				t.Fatalf("transfer count = %d, want the one declared relation", count)
			}
			endpoint, endpointOK := contract.Operations.TransferEndpointAt(operation, 0)
			if !endpointOK || endpoint != relation.wantEndpoint {
				t.Fatalf("endpoint = %+v/%t, want %+v", endpoint, endpointOK, relation.wantEndpoint)
			}
			identity, identityOK := contract.Operations.TransferIdentityAt(operation, 0)
			if !identityOK || identity != relation.wantIdentity {
				t.Fatalf("identity = %d/%t, want %d", identity, identityOK, relation.wantIdentity)
			}
			capabilities, capabilitiesOK := contract.Operations.TransferCapabilitiesAt(operation, 0)
			if !capabilitiesOK || capabilities != relation.wantCapabilities {
				t.Fatalf("capabilities = %d/%t, want %d", capabilities, capabilitiesOK, relation.wantCapabilities)
			}
		})
	}
}

// TestManifestTransferEndpointInputAddressesADeclaredInput is the negative
// law for the input endpoint: the ordinal it names is a coordinate in the
// callable's own ABI, so an ordinal past the declared inputs is refused at
// seal rather than clamped or dropped.
func TestManifestTransferEndpointInputAddressesADeclaredInput(t *testing.T) {
	declaration := manifestwire.New(relationHostModule)
	memberType := typ.Func().Param("endpoint", typ.Any).Param("payload", typ.Any).Returns(typ.Boolean).Build()
	declaration.DefineFunctionSignature("send_to_input", signature.Function{Type: memberType})
	declaration.DefineFunctionOperation("send_to_input", manifestwire.Operation{
		Transfers: []manifestwire.TransferSpec{{
			Endpoint: manifestwire.TransferEndpoint{Kind: manifestwire.TransferEndpointInput, Input: 7},
			Payload:  manifestwire.InputSource{Kind: manifestwire.InputSourceValue, Ordinal: 1},
			Alias:    manifestwire.InputSource{Kind: manifestwire.InputSourceValue, Ordinal: 1},
			Identity: manifestwire.TransferIdentitySame,
			Outcomes: []manifestwire.TransferOutcomeSpec{{Outcome: 0, Possibility: manifestwire.TransferMayDeliver}},
		}},
	})
	if _, err := sealRelationCatalogue(declaration); err == nil {
		t.Fatal("an input endpoint outside the declared ABI sealed, want a refusal")
	}
}

// freshDeclaration is one authored fresh-result class and the sealed class it
// must answer.
type freshDeclaration struct {
	member string
	class  manifestwire.FreshClass
	want   schematype.FreshClass
}

// declaredFreshClasses names every value in the wire fresh-result vocabulary.
// A fresh result is the provider's claim that the named result slot is a
// newly created runtime value of that class rather than an alias of an input.
func declaredFreshClasses() []freshDeclaration {
	return []freshDeclaration{
		{member: "new_table", class: manifestwire.FreshTable, want: schematype.FreshClassTable},
		{member: "new_function", class: manifestwire.FreshFunction, want: schematype.FreshClassFunction},
		{member: "new_thread", class: manifestwire.FreshThread, want: schematype.FreshClassThread},
		{member: "new_userdata", class: manifestwire.FreshUserdata, want: schematype.FreshClassUserdata},
		{member: "new_error", class: manifestwire.FreshError, want: schematype.FreshClassError},
		{member: "new_reflection", class: manifestwire.FreshReflection, want: schematype.FreshClassReflection},
	}
}

func freshHostManifest() *manifestwire.Manifest {
	declaration := manifestwire.New(relationHostModule)
	for _, fresh := range declaredFreshClasses() {
		result := freshResultType(fresh.class)
		memberType := typ.Func().Returns(result).Build()
		declaration.DefineFunctionSignature(fresh.member, signature.Function{Type: memberType})
		declaration.DefineFunctionOperation(fresh.member, manifestwire.Operation{
			OutcomeAmendments: []manifestwire.OutcomeAmendment{{
				Outcome:      0,
				FreshResults: []manifestwire.FreshResult{{Result: 0, Class: fresh.class}},
			}},
		})
	}
	return declaration
}

// freshResultType answers the declared result type each fresh class admits.
// The class is a runtime-shape claim, so the declared type must be one the
// domain's fresh-compatibility relation accepts for that class.
func freshResultType(class manifestwire.FreshClass) typ.Type {
	switch class {
	case manifestwire.FreshTable:
		return typ.BuiltinTableTopMarker()
	default:
		return typ.Any
	}
}

// TestManifestDeclaresEveryFreshClass is the positive law for the fresh-result
// vocabulary: every class the wire offers is declared by a real manifest and
// answered by the sealed Target on the exact result slot it names.
func TestManifestDeclaresEveryFreshClass(t *testing.T) {
	contract, err := sealRelationCatalogue(freshHostManifest())
	if err != nil {
		t.Fatal(err)
	}
	for _, fresh := range declaredFreshClasses() {
		t.Run(fresh.member, func(t *testing.T) {
			operation, ok := contract.Operations.Lookup(relationBinding(fresh.member))
			if !ok {
				t.Fatalf("sealed Target holds no operation for %s", fresh.member)
			}
			if count := contract.Operations.FreshResultCount(operation, 0); count != 1 {
				t.Fatalf("fresh result count = %d, want the one declared class", count)
			}
			result, _, class, classOK := contract.Operations.FreshResultAt(operation, 0, 0)
			if !classOK || result != 0 || class != fresh.want {
				t.Fatalf("fresh result = result %d class %d (ok %t), want result 0 class %d",
					result, class, classOK, fresh.want)
			}
		})
	}
}
