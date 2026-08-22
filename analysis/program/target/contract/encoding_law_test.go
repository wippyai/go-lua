package contract

import (
	"bytes"
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/exactkey"
	"github.com/wippyai/go-lua/analysis/program/target/operation"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"github.com/wippyai/go-lua/internal/framing"
)

// encodingContract builds the smallest complete subordinate graph directly
// inside Contract's owner package. It keeps codec/preimage laws independent
// of compiler.Seal, avoiding a compiler -> contract test cycle.
func encodingContract(t *testing.T) *Contract {
	t.Helper()
	return encodingContractWithEffectTail(t, vocabulary.RowUnknownOpen)
}

// encodingContractWithEffectTail builds that same graph over one chosen
// operation effect tail, so a law can state what two contracts that differ in
// observable declared content owe each other.
func encodingContractWithEffectTail(t *testing.T, tail vocabulary.RowTail) *Contract {
	t.Helper()
	keys, err := exactkey.Compile(nil)
	if err != nil {
		t.Fatal(err)
	}
	geometry, err := operation.CompileGeometry(operation.Input{})
	if err != nil {
		t.Fatal(err)
	}
	core, err := operation.CompileAnchors(geometry, keys)
	if err != nil {
		t.Fatal(err)
	}
	builder, err := operation.BeginQuery(core)
	if err != nil {
		t.Fatal(err)
	}
	opaque, ok := core.Opaque()
	if !ok {
		t.Fatal("opaque operation missing")
	}
	unknown, err := builder.AppendQueryValues(operation.QueryValuesDeclaration{Owner: opaque, Tail: vocabulary.ValuesUnknown}, nil)
	if err != nil {
		t.Fatal(err)
	}
	callback, ok := core.CallbackAt(opaque, 0)
	if !ok {
		t.Fatal("opaque callback missing")
	}
	if err := builder.AppendCallbackEffects(callback, vocabulary.RowUnknownOpen, 0, nil); err != nil {
		t.Fatal(err)
	}
	var callbackOutcomes [5]vocabulary.Values
	for index := range callbackOutcomes {
		callbackOutcomes[index] = unknown
	}
	if err := builder.AppendQueryOperation(opaque, operation.QueryOperationInput{
		Input: unknown,
		Outcomes: []operation.QueryOutcomeInput{
			{Kind: flowkind.OutcomeNormal, Values: unknown},
			{Kind: flowkind.OutcomeThrow, Values: unknown},
			{Kind: flowkind.OutcomeYield, Values: unknown},
			{Kind: flowkind.OutcomeCancel, Values: unknown},
		},
		CallbackValues: []operation.CallbackQueryInput{{
			Source: 0, Admission: schematype.CallableAdmissionOrdinary, Arguments: unknown, Outcomes: callbackOutcomes,
		}},
		Transfers: []operation.TransferInput{{
			Endpoint: vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointExternal},
			Payload:  vocabulary.InputSource{Kind: vocabulary.InputSourceAllInputs},
			Alias:    vocabulary.InputSource{Kind: vocabulary.InputSourceAllInputs},
			Identity: vocabulary.TransferIdentityUnspecified, Capabilities: vocabulary.TransferCapabilitiesUnspecified,
			Outcomes: []vocabulary.TransferPossibility{
				vocabulary.TransferMayDeliver | vocabulary.TransferMayReject,
				vocabulary.TransferMayDeliver | vocabulary.TransferMayReject,
				vocabulary.TransferMayDeliver | vocabulary.TransferMayReject,
				vocabulary.TransferMayDeliver | vocabulary.TransferMayReject,
			},
		}},
		EffectTail: tail,
	}); err != nil {
		t.Fatal(err)
	}
	sealed, err := builder.FinishQuery()
	if err != nil {
		t.Fatal(err)
	}
	contract, err := New(Input{Operations: sealed, ExactKeys: keys})
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func TestContentIDNamespaceSeparatesPriorContractIdentity(t *testing.T) {
	contract := encodingContract(t)
	var probe bytes.Buffer
	var probeWriter framing.Writer
	if err := probeWriter.Reset(&probe, "program/target-contract", contentIDCodecVersion); err != nil {
		t.Fatal(err)
	}
	if err := encodeContract(&probeWriter, contract); err != nil {
		t.Fatalf("encoding probe: %v", err)
	}
	current := contract.ContentID()
	if !current.Available() {
		opaque, opaqueOK := contract.Operations.Opaque()
		t.Fatalf("current contract has no ContentID: count=%d opaque=%d/%v", contract.Operations.OperationCount(), opaque, opaqueOK)
	}
	priorHash := sha256.New()
	var writer framing.Writer
	if err := writer.Reset(priorHash, "program/target-contract", contentIDCodecVersion-1); err != nil {
		t.Fatal(err)
	}
	if err := encodeContract(&writer, contract); err != nil {
		t.Fatal(err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	var prior identity.ContentID
	if sum := priorHash.Sum(prior[:0]); len(sum) != len(prior) {
		t.Fatal("prior target digest has wrong width")
	}
	if current == prior {
		t.Fatal("target schema reused a prior-layout ContentID")
	}
}

func TestIdentityEncodingFramesDistinctInputCoordinates(t *testing.T) {
	encode := func(input vocabulary.InputSource) []byte {
		hash := sha256.New()
		var writer framing.Writer
		if err := writer.Reset(hash, "target/identity-test", 1); err != nil {
			t.Fatal(err)
		}
		if err := encodeInput(&writer, input); err != nil {
			t.Fatal(err)
		}
		if err := writer.Finish(); err != nil {
			t.Fatal(err)
		}
		return hash.Sum(nil)
	}
	formal := encode(vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0})
	values := encode(vocabulary.InputSource{Kind: vocabulary.InputSourceValuesVar, Ordinal: 0})
	if bytes.Equal(formal, values) {
		t.Fatal("identity framing collapsed distinct input-source kinds")
	}
}
