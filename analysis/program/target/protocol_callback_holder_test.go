package target

import (
	"testing"

	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func TestProtocolCallbackHoldersSealExactRetainedRelation(t *testing.T) {
	contract := mustSeal(t, protocolCallbackHolderSpec(InputSource{Kind: InputSourceValueFormal}))
	protocol, ok := contract.ProtocolAt(0)
	if !ok || contract.ProtocolCallbackHolderCount(protocol) != 1 {
		t.Fatalf("protocol callback-holder range = %d/%v", contract.ProtocolCallbackHolderCount(protocol), ok)
	}
	op, input, callback, ok := contract.ProtocolCallbackHolderAt(protocol, 0)
	if !ok || input != (InputSource{Kind: InputSourceValueFormal}) {
		t.Fatalf("callback-holder input = %d/%#v/%d/%v", op, input, callback, ok)
	}
	owner, ownerOK := contract.CallbackOwner(callback)
	if !ownerOK || owner != op {
		t.Fatalf("callback-holder owner = %d/%v, want %d", owner, ownerOK, op)
	}
	lifecycle, lifecycleOK := contract.CallbackLifecycle(callback)
	if !lifecycleOK || !retainedCallbackLifecycle(lifecycle) {
		t.Fatalf("callback-holder lifecycle = %d/%v", lifecycle, lifecycleOK)
	}
	if _, _, _, found := contract.ProtocolCallbackHolderAt(protocol, 1); found {
		t.Fatal("out-of-range callback holder resolved")
	}
	if contract.ProtocolCallbackHolderCount(Protocol(2)) != 0 {
		t.Fatal("opaque fabricated a callback-holder relation")
	}
	if allocs := testing.AllocsPerRun(1000, func() { _, _, _, _ = contract.ProtocolCallbackHolderAt(protocol, 0) }); allocs != 0 {
		t.Fatalf("callback-holder query allocated %f times", allocs)
	}
}

func TestProtocolCallbackHoldersAcceptValuesVarAndIgnoreAuthorOrder(t *testing.T) {
	left := protocolCallbackHolderSpec(InputSource{Kind: InputSourceValuesVar})
	right := protocolCallbackHolderSpec(InputSource{Kind: InputSourceValuesVar})
	left.Protocols[0].CallbackHolders = append(left.Protocols[0].CallbackHolders,
		ProtocolCallbackHolderSpec{Operation: 1, Input: InputSource{Kind: InputSourceValueFormal}, Callback: 1},
	)
	right.Protocols[0].CallbackHolders = []ProtocolCallbackHolderSpec{
		{Operation: 1, Input: InputSource{Kind: InputSourceValueFormal}, Callback: 1},
		{Operation: 1, Input: InputSource{Kind: InputSourceValuesVar}, Callback: 1},
	}
	leftContract := mustSeal(t, left)
	rightContract := mustSeal(t, right)
	if leftContract.ContentID() != rightContract.ContentID() {
		t.Fatal("callback-holder author order changed ContentID")
	}
	protocol, _ := leftContract.ProtocolAt(0)
	if leftContract.ProtocolCallbackHolderCount(protocol) != 2 {
		t.Fatalf("callback-holder count = %d", leftContract.ProtocolCallbackHolderCount(protocol))
	}
}

func TestProtocolCallbackHoldersRejectUnsealedAuthority(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Spec)
	}{
		{"all-inputs", func(spec *Spec) { spec.Protocols[0].CallbackHolders[0].Input = InputSource{Kind: InputSourceAllInputs} }},
		{"formal outside scope", func(spec *Spec) {
			spec.Protocols[0].CallbackHolders[0].Input = InputSource{Kind: InputSourceValueFormal, Ordinal: 2}
		}},
		{"Values variable outside scope", func(spec *Spec) {
			spec.Protocols[0].CallbackHolders[0].Input = InputSource{Kind: InputSourceValuesVar, Ordinal: 1}
		}},
		{"callback outside operation", func(spec *Spec) { spec.Protocols[0].CallbackHolders[0].Callback = 2 }},
		{"synchronous callback", func(spec *Spec) { spec.Operations[0].Callbacks[0].Lifecycle = CallbackSyncOptionalOnce }},
		{"duplicate triple", func(spec *Spec) {
			spec.Protocols[0].CallbackHolders = append(spec.Protocols[0].CallbackHolders, spec.Protocols[0].CallbackHolders[0])
		}},
		{"unbound operation", func(spec *Spec) {
			spec.Operations = append(spec.Operations, OperationSpec{Input: ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed}, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed}}}, Effects: RowSpec{Tail: RowClosed}})
			spec.Protocols[0].CallbackHolders[0].Operation = 2
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			spec := protocolCallbackHolderSpec(InputSource{Kind: InputSourceValueFormal})
			test.edit(&spec)
			if contract, err := testSeal(&spec); err == nil || contract != nil {
				t.Fatal("invalid callback-holder authority was published")
			}
		})
	}
}

func protocolCallbackHolderSpec(input InputSource) Spec {
	closed := ValuesSpec{Tail: ValuesClosed}
	return Spec{Operations: []OperationSpec{{
		Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"callback-holder"}}},
		ValuesVars: 1,
		Input:      ValuesSpec{Fixed: []schematype.Type{testAny, testAny}, Tail: ValuesVariable, Var: 0},
		Outcomes:   []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed}}},
		Callbacks: []CallbackSpec{{
			Function:  InputSource{Kind: InputSourceValueFormal, Ordinal: 1},
			Admission: schematype.CallableAdmissionOrdinary,
			Arguments: closed,
			Outcomes: []TerminalSpec{
				{Kind: flowkind.OutcomeNormal, Values: closed}, {Kind: flowkind.OutcomeReturn, Values: closed},
				{Kind: flowkind.OutcomeThrow, Values: closed}, {Kind: flowkind.OutcomeYield, Values: closed}, {Kind: flowkind.OutcomeCancel, Values: closed},
			},
			Lifecycle: CallbackRetainedOptionalOnce,
			Effects:   RowSpec{Tail: RowClosed},
		}},
		Effects: RowSpec{Tail: RowClosed},
	}}, Protocols: []ProtocolSpec{{
		Acquisitions:    []AcquisitionSpec{{Operation: 1, Outcome: 0, Result: 0, State: 1}},
		States:          []StateSpec{{Name: "open"}},
		CallbackHolders: []ProtocolCallbackHolderSpec{{Operation: 1, Input: input, Callback: 1}},
	}}}
}
