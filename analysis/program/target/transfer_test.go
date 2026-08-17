package target

import (
	"testing"

	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func TestTransferCanonicalizesEndpointPayloadAndOutcomes(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{transferOperation(
		[]OutcomeSpec{
			{Kind: flowkind.OutcomeYield, Values: ValuesSpec{Tail: ValuesVariable, Var: 0}},
			{Kind: flowkind.OutcomeCancel, Values: ValuesSpec{Tail: ValuesClosed}},
			{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testBoolean}, Tail: ValuesClosed}},
			{Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Fixed: []schematype.Type{testString}, Tail: ValuesClosed}},
		},
		[]TransferSpec{
			transfer(TransferEndpoint{Kind: TransferEndpointExternal}, InputSource{Kind: InputSourceValuesVar}, TransferIdentityUnspecified, TransferCapabilitiesLoseAll,
				[]TransferOutcomeSpec{{Outcome: 1, Possibility: TransferMayReject}, {Outcome: 3, Possibility: TransferMayDeliver | TransferMayReject}, {Outcome: 0, Possibility: TransferMayDeliver}, {Outcome: 2, Possibility: TransferMayDeliver}}),
			transfer(TransferEndpoint{Kind: TransferEndpointInput, Input: 1}, InputSource{Kind: InputSourceValueFormal, Ordinal: 0}, TransferIdentitySame, TransferCapabilitiesPreserveAll,
				[]TransferOutcomeSpec{{Outcome: 3, Possibility: TransferMayReject}, {Outcome: 0, Possibility: TransferMayDeliver}, {Outcome: 2, Possibility: TransferMayDeliver | TransferMayReject}, {Outcome: 1, Possibility: TransferMayReject}}),
		},
	)}})
	op, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"transfer"}})
	if !ok || contract.TransferCount(op) != 2 {
		t.Fatalf("transfer operation/count = %d/%v/%d", op, ok, contract.TransferCount(op))
	}
	want := []struct {
		endpoint     TransferEndpoint
		payload      InputSource
		alias        InputSource
		identity     TransferIdentity
		capabilities TransferCapabilities
		masks        []TransferPossibility
	}{
		{TransferEndpoint{Kind: TransferEndpointInput, Input: 1}, InputSource{Kind: InputSourceValueFormal}, InputSource{Kind: InputSourceValueFormal}, TransferIdentitySame, TransferCapabilitiesPreserveAll, []TransferPossibility{TransferMayDeliver | TransferMayReject, TransferMayReject, TransferMayDeliver, TransferMayReject}},
		{TransferEndpoint{Kind: TransferEndpointExternal}, InputSource{Kind: InputSourceValuesVar}, InputSource{Kind: InputSourceValuesVar}, TransferIdentityUnspecified, TransferCapabilitiesLoseAll, []TransferPossibility{TransferMayDeliver, TransferMayDeliver | TransferMayReject, TransferMayDeliver, TransferMayReject}},
	}
	for index, expected := range want {
		id, idOK := contract.TransferIDAt(op, index)
		owner, ownerOK := contract.TransferOwner(id)
		declaredEndpoint, declaredPayload, declaredAlias, declaredIdentity, declaredCapabilities, declarationOK := contract.TransferDeclaration(id)
		if !idOK || id == 0 || !ownerOK || owner != op || !declarationOK || declaredEndpoint != expected.endpoint || declaredPayload != expected.payload || declaredAlias != expected.alias || declaredIdentity != expected.identity || declaredCapabilities != expected.capabilities {
			t.Fatalf("sealed transfer identity %d did not preserve its exact declaration", index)
		}
		endpoint, endpointOK := contract.TransferEndpointAt(op, index)
		payload, payloadOK := contract.TransferPayloadAt(op, index)
		alias, aliasOK := contract.TransferAliasAt(op, index)
		identity, identityOK := contract.TransferIdentityAt(op, index)
		capabilities, capabilitiesOK := contract.TransferCapabilitiesAt(op, index)
		if !endpointOK || endpoint != expected.endpoint || !payloadOK || payload != expected.payload || !aliasOK || alias != expected.alias ||
			!identityOK || identity != expected.identity || !capabilitiesOK || capabilities != expected.capabilities {
			t.Fatalf("transfer %d = endpoint:%#v/%v payload:%#v/%v identity:%d/%v capabilities:%d/%v", index, endpoint, endpointOK, payload, payloadOK, identity, identityOK, capabilities, capabilitiesOK)
		}
		for outcome, mask := range expected.masks {
			declaredOrdinal, declaredMask, declaredFound := contract.TransferDeclarationOutcomeAt(id, outcome)
			if !declaredFound || declaredOrdinal != uint32(outcome) || declaredMask != mask {
				t.Fatalf("sealed transfer identity outcome %d/%d lost declaration", index, outcome)
			}
			ordinal, got, found := contract.TransferOutcomeAt(op, index, outcome)
			if !found || ordinal != uint32(outcome) || got != mask {
				t.Fatalf("transfer outcome %d/%d = %d/%d/%v", index, outcome, ordinal, got, found)
			}
		}
	}
}

func TestTransferAuthorPermutationHasOnePublicContract(t *testing.T) {
	leftOutcomes := []OutcomeSpec{{Kind: flowkind.OutcomeYield, Values: ValuesSpec{Tail: ValuesVariable, Var: 0}}, {Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}, {Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Fixed: []schematype.Type{testString}, Tail: ValuesClosed}}}
	rightOutcomes := []OutcomeSpec{leftOutcomes[2], leftOutcomes[0], leftOutcomes[1]}
	left := mustSeal(t, Spec{Operations: []OperationSpec{transferOperation(leftOutcomes, []TransferSpec{
		transfer(TransferEndpoint{Kind: TransferEndpointExternal}, InputSource{Kind: InputSourceValuesVar}, TransferIdentityDistinct, TransferCapabilitiesLoseAll, []TransferOutcomeSpec{{Outcome: 2, Possibility: TransferMayReject}, {Outcome: 0, Possibility: TransferMayDeliver}, {Outcome: 1, Possibility: TransferMayDeliver | TransferMayReject}}),
		transfer(TransferEndpoint{Kind: TransferEndpointInput}, InputSource{Kind: InputSourceValueFormal}, TransferIdentitySame, TransferCapabilitiesPreserveAll, []TransferOutcomeSpec{{Outcome: 0, Possibility: TransferMayDeliver}, {Outcome: 1, Possibility: TransferMayDeliver}, {Outcome: 2, Possibility: TransferMayReject}}),
	})}})
	right := mustSeal(t, Spec{Operations: []OperationSpec{transferOperation(rightOutcomes, []TransferSpec{
		transfer(TransferEndpoint{Kind: TransferEndpointInput}, InputSource{Kind: InputSourceValueFormal}, TransferIdentitySame, TransferCapabilitiesPreserveAll, []TransferOutcomeSpec{{Outcome: 0, Possibility: TransferMayReject}, {Outcome: 2, Possibility: TransferMayDeliver}, {Outcome: 1, Possibility: TransferMayDeliver}}),
		transfer(TransferEndpoint{Kind: TransferEndpointExternal}, InputSource{Kind: InputSourceValuesVar}, TransferIdentityDistinct, TransferCapabilitiesLoseAll, []TransferOutcomeSpec{{Outcome: 1, Possibility: TransferMayDeliver}, {Outcome: 0, Possibility: TransferMayReject}, {Outcome: 2, Possibility: TransferMayDeliver | TransferMayReject}}),
	})}})
	assertPublicContractEqual(t, left, right)
	if left.ContentID() != right.ContentID() {
		t.Fatal("outcome/transfer author permutation changed ContentID")
	}
}

func TestTransferAliasIsCanonicalDeclarationAndContentAuthority(t *testing.T) {
	outcomes := []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}, {Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Tail: ValuesClosed}}}
	base := transfer(TransferEndpoint{Kind: TransferEndpointExternal}, InputSource{Kind: InputSourceValueFormal}, TransferIdentitySame, TransferCapabilitiesPreserveAll,
		[]TransferOutcomeSpec{{Outcome: 0, Possibility: TransferMayDeliver}, {Outcome: 1, Possibility: TransferMayReject}})
	other := base
	other.Alias = InputSource{Kind: InputSourceValueFormal, Ordinal: 1}
	left := mustSeal(t, Spec{Operations: []OperationSpec{transferOperation(outcomes, []TransferSpec{base})}})
	right := mustSeal(t, Spec{Operations: []OperationSpec{transferOperation(outcomes, []TransferSpec{other})}})
	if left.ContentID() == right.ContentID() {
		t.Fatal("transfer alias did not affect target ContentID")
	}
	op, ok := right.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"transfer"}})
	if !ok {
		t.Fatal("transfer operation")
	}
	alias, ok := right.TransferAliasAt(op, 0)
	if !ok || alias != other.Alias {
		t.Fatal("transfer alias lost canonical declaration")
	}
}

func TestTransferRejectsIncompleteOrInvalidAuthority(t *testing.T) {
	base := []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}, {Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Tail: ValuesClosed}}}
	valid := []TransferOutcomeSpec{{Outcome: 0, Possibility: TransferMayDeliver}, {Outcome: 1, Possibility: TransferMayReject}}
	baseTransfer := func() TransferSpec {
		return transfer(TransferEndpoint{Kind: TransferEndpointExternal}, InputSource{Kind: InputSourceValueFormal}, TransferIdentityUnspecified, TransferCapabilitiesUnspecified, valid)
	}
	tests := []struct {
		name string
		edit func(*TransferSpec)
	}{
		{"invalid endpoint", func(spec *TransferSpec) { spec.Endpoint.Kind = TransferEndpointInvalid }},
		{"external endpoint carries input", func(spec *TransferSpec) { spec.Endpoint.Input = 1 }},
		{"input endpoint outside scope", func(spec *TransferSpec) { spec.Endpoint = TransferEndpoint{Kind: TransferEndpointInput, Input: 2} }},
		{"invalid payload", func(spec *TransferSpec) { spec.Payload = InputSource{Kind: InputSourceAllInputs} }},
		{"invalid alias", func(spec *TransferSpec) { spec.Alias = InputSource{Kind: InputSourceAllInputs} }},
		{"non-input Values alias", func(spec *TransferSpec) { spec.Alias = InputSource{Kind: InputSourceValuesVar, Ordinal: 1} }},
		{"invalid identity", func(spec *TransferSpec) { spec.Identity = TransferIdentityInvalid }},
		{"invalid capabilities", func(spec *TransferSpec) { spec.Capabilities = TransferCapabilitiesInvalid }},
		{"incomplete outcomes", func(spec *TransferSpec) { spec.Outcomes = valid[:1] }},
		{"duplicate outcomes", func(spec *TransferSpec) {
			spec.Outcomes = []TransferOutcomeSpec{{Outcome: 0, Possibility: TransferMayDeliver}, {Outcome: 0, Possibility: TransferMayReject}}
		}},
		{"unknown possibility", func(spec *TransferSpec) { spec.Outcomes[0].Possibility = 4 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := baseTransfer()
			test.edit(&spec)
			if contract, err := testSeal(&Spec{Operations: []OperationSpec{transferOperation(base, []TransferSpec{spec})}}); err == nil || contract != nil {
				t.Fatal("invalid transfer was published")
			}
		})
	}
	first, second := baseTransfer(), baseTransfer()
	second.Outcomes = []TransferOutcomeSpec{{Outcome: 0, Possibility: TransferMayReject}, {Outcome: 1, Possibility: TransferMayDeliver}}
	if contract, err := testSeal(&Spec{Operations: []OperationSpec{transferOperation(base, []TransferSpec{first, second})}}); err == nil || contract != nil {
		t.Fatal("duplicate endpoint/payload/alias was published")
	}
}

func TestOpaqueTransferIsMaximalAndAllocationFree(t *testing.T) {
	contract := mustSeal(t, Spec{})
	opaque, ok := contract.Opaque()
	if !ok || contract.TransferCount(opaque) != 1 {
		t.Fatalf("opaque/count = %d/%v/%d", opaque, ok, contract.TransferCount(opaque))
	}
	endpoint, endpointOK := contract.TransferEndpointAt(opaque, 0)
	payload, payloadOK := contract.TransferPayloadAt(opaque, 0)
	alias, aliasOK := contract.TransferAliasAt(opaque, 0)
	identity, identityOK := contract.TransferIdentityAt(opaque, 0)
	capabilities, capabilitiesOK := contract.TransferCapabilitiesAt(opaque, 0)
	if !endpointOK || endpoint != (TransferEndpoint{Kind: TransferEndpointExternal}) || !payloadOK || payload != (InputSource{Kind: InputSourceAllInputs}) || !aliasOK || alias != (InputSource{Kind: InputSourceAllInputs}) || !identityOK || identity != TransferIdentityUnspecified || !capabilitiesOK || capabilities != TransferCapabilitiesUnspecified {
		t.Fatalf("opaque transfer = %#v/%v %#v/%v %#v/%v %d/%v %d/%v", endpoint, endpointOK, payload, payloadOK, alias, aliasOK, identity, identityOK, capabilities, capabilitiesOK)
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		if _, ok := contract.TransferEndpointAt(opaque, 0); !ok {
			panic("opaque transfer endpoint disappeared")
		}
		if _, ok := contract.TransferPayloadAt(opaque, 0); !ok {
			panic("opaque transfer payload disappeared")
		}
	}); allocs != 0 {
		t.Fatalf("opaque transfer queries allocated %f times", allocs)
	}
}

func TestTransferWideAndDeepValidationHasNoSemanticCap(t *testing.T) {
	const width = 4096
	fixed := make([]schematype.Type, width)
	transfers := make([]TransferSpec, width)
	for index := 0; index < width; index++ {
		fixed[index] = testAny
		transfers[width-index-1] = transfer(TransferEndpoint{Kind: TransferEndpointInput, Input: ValueFormal(index)}, InputSource{Kind: InputSourceValueFormal, Ordinal: uint32(index)}, TransferIdentitySame, TransferCapabilitiesPreserveAll,
			[]TransferOutcomeSpec{{Outcome: 3, Possibility: TransferMayReject}, {Outcome: 1, Possibility: TransferMayReject}, {Outcome: 2, Possibility: TransferMayDeliver}, {Outcome: 0, Possibility: TransferMayDeliver | TransferMayReject}})
	}
	contract := mustSeal(t, Spec{Operations: []OperationSpec{{Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"wide-transfer"}}}, Input: ValuesSpec{Fixed: fixed, Tail: ValuesClosed}, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeCancel, Values: ValuesSpec{Tail: ValuesClosed}}, {Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Tail: ValuesClosed}}, {Kind: flowkind.OutcomeYield, Values: ValuesSpec{Tail: ValuesClosed}}, {Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}}, Transfers: transfers, Effects: RowSpec{Tail: RowClosed}}}})
	op, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"wide-transfer"}})
	for index := 0; index < width; index++ {
		endpoint, ok := contract.TransferEndpointAt(op, index)
		if !ok || endpoint != (TransferEndpoint{Kind: TransferEndpointInput, Input: ValueFormal(index)}) {
			t.Fatalf("wide transfer %d = %#v/%v", index, endpoint, ok)
		}
	}
}

func transfer(endpoint TransferEndpoint, payload InputSource, identity TransferIdentity, capabilities TransferCapabilities, outcomes []TransferOutcomeSpec) TransferSpec {
	return TransferSpec{Endpoint: endpoint, Payload: payload, Alias: payload, Identity: identity, Capabilities: capabilities, Outcomes: outcomes}
}

func transferOperation(outcomes []OutcomeSpec, transfers []TransferSpec) OperationSpec {
	return OperationSpec{Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"transfer"}}}, ValuesVars: 1, Input: ValuesSpec{Fixed: []schematype.Type{testAny, testString}, Tail: ValuesVariable, Var: 0}, Outcomes: outcomes, Transfers: transfers, Effects: RowSpec{Tail: RowClosed}}
}
