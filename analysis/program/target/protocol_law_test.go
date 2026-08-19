package target

import (
	"fmt"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"testing"
)

func protocolOperation(name string, input []schematype.Type) vocabulary.OperationSpec {
	return vocabulary.OperationSpec{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{name}}},
		Input:    vocabulary.ValuesSpec{Fixed: input, Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{
			Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed},
		}},
		Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
}

func TestProtocolMultipleAcquisitionsAndEntryRows(t *testing.T) {
	accept := protocolOperation("accept", nil)
	close := protocolOperation("close", []schematype.Type{testAny})
	connect := protocolOperation("connect", nil)
	contract := mustSeal(t, Spec{
		Operations: []vocabulary.OperationSpec{accept, close, connect},
		Protocols: []vocabulary.ProtocolSpec{{
			Acquisitions: []vocabulary.AcquisitionSpec{
				{Operation: 3, Outcome: 0, Result: 0, State: 1},
				{Operation: 1, Outcome: 0, Result: 0, State: 1},
			},
			States: []vocabulary.StateSpec{{Name: "open"}, {Name: "closed", Final: true}},
			Transitions: []vocabulary.TransitionSpec{{
				Operation: 2, Input: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, From: 1,
				Outcomes: []vocabulary.TransitionOutcomeSpec{{Outcome: 0, To: 2}},
			}},
			Escapes: []vocabulary.EscapeSpec{{Operation: 2, Input: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}}},
		}},
	})
	if contract.protocols.ProtocolCount() != 1 {
		t.Fatalf("ProtocolCount = %d", contract.protocols.ProtocolCount())
	}
	p, _ := contract.protocols.ProtocolAt(0)
	if contract.protocols.ProtocolAcquisitionCount(p) != 2 {
		t.Fatalf("acquisition count = %d", contract.protocols.ProtocolAcquisitionCount(p))
	}
	for index, wantName := range []string{"open", "closed"} {
		state, ok := contract.protocols.StateAt(p, index)
		name, nameOK := contract.protocols.StateName(p, state)
		if !ok || !nameOK || name != wantName {
			t.Fatalf("state %d = %d/%q", index, state, name)
		}
		final, finalOK := contract.protocols.StateFinal(p, state)
		if !finalOK || final != (index == 1) {
			t.Fatalf("final %d = %v/%v", index, final, finalOK)
		}
	}
	if op, source, ordinal, from, ok := contract.protocols.TransitionAt(p, 0); !ok || source != vocabulary.InputSourceValueFormal || ordinal != 0 || from != 1 || op == 0 {
		t.Fatalf("transition = %d/%d/%d/%d/%v", op, source, ordinal, from, ok)
	}
	if outcome, to, ok := contract.protocols.TransitionOutcomeAt(p, 0, 0); !ok || outcome != 0 || to != 2 {
		t.Fatalf("transition outcome = %d/%d/%v", outcome, to, ok)
	}
	if contract.protocols.EscapeCount(p) != 2 {
		t.Fatalf("escape count = %d", contract.protocols.EscapeCount(p))
	}
	opaque, _ := contract.Opaque()
	op, source, ordinal, ok := contract.protocols.EscapeAt(p, 1)
	if !ok || op != opaque || source != vocabulary.InputSourceAllInputs || ordinal != 0 {
		t.Fatalf("derived opaque escape = %d/%d/%d/%v", op, source, ordinal, ok)
	}
	if contract.protocols.TransitionCount(p) != 1 {
		t.Fatal("opaque fabricated a protocol transition")
	}
	if allocs := testing.AllocsPerRun(1000, func() { _, _, _, _ = contract.protocols.EscapeAt(p, 1) }); allocs != 0 {
		t.Fatalf("derived escape allocated %f times", allocs)
	}
}

func TestProtocolRejectsInvalidNominalAuthority(t *testing.T) {
	base := protocolOperation("acquire", []schematype.Type{testAny})
	valid := vocabulary.ProtocolSpec{Acquisitions: []vocabulary.AcquisitionSpec{{Operation: 1, Outcome: 0, Result: 0, State: 1}}, States: []vocabulary.StateSpec{{Name: "open"}}}
	for _, test := range []struct {
		name      string
		protocols []vocabulary.ProtocolSpec
	}{
		{"empty acquisitions", []vocabulary.ProtocolSpec{{States: []vocabulary.StateSpec{{Name: "open"}}}}},
		{"empty state", []vocabulary.ProtocolSpec{{Acquisitions: valid.Acquisitions, States: []vocabulary.StateSpec{{Name: ""}}}}},
		{"invalid utf8", []vocabulary.ProtocolSpec{{Acquisitions: valid.Acquisitions, States: []vocabulary.StateSpec{{Name: "\xff"}}}}},
		{"duplicate state", []vocabulary.ProtocolSpec{{Acquisitions: valid.Acquisitions, States: []vocabulary.StateSpec{{Name: "open"}, {Name: "open"}}}}},
		{"all inputs authored", []vocabulary.ProtocolSpec{{Acquisitions: valid.Acquisitions, States: valid.States, Escapes: []vocabulary.EscapeSpec{{Operation: 1, Input: vocabulary.InputSource{Kind: vocabulary.InputSourceAllInputs}}}}}},
		{"shared acquisition", []vocabulary.ProtocolSpec{valid, valid}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{base}, Protocols: test.protocols}); err == nil {
				t.Fatal("invalid protocol accepted")
			}
		})
	}
}

func TestProtocolRejectsBadOutcomeAndTransitionCoordinates(t *testing.T) {
	base := protocolOperation("protocol-coordinates", []schematype.Type{testAny})
	valid := vocabulary.ProtocolSpec{
		Acquisitions: []vocabulary.AcquisitionSpec{{Operation: 1, Outcome: 0, Result: 0, State: 1}},
		States:       []vocabulary.StateSpec{{Name: "open"}, {Name: "closed", Final: true}},
	}
	cases := []struct {
		name  string
		value vocabulary.ProtocolSpec
	}{
		{"result outside fixed values", vocabulary.ProtocolSpec{Acquisitions: []vocabulary.AcquisitionSpec{{Operation: 1, Outcome: 0, Result: 1, State: 1}}, States: valid.States}},
		{"acquisition state outside scope", vocabulary.ProtocolSpec{Acquisitions: []vocabulary.AcquisitionSpec{{Operation: 1, Outcome: 0, Result: 0, State: 3}}, States: valid.States}},
		{"transition from outside scope", vocabulary.ProtocolSpec{Acquisitions: valid.Acquisitions, States: valid.States, Transitions: []vocabulary.TransitionSpec{{Operation: 1, Input: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, From: 3, Outcomes: []vocabulary.TransitionOutcomeSpec{{Outcome: 0, To: 1}}}}}},
		{"transition outcome outside scope", vocabulary.ProtocolSpec{Acquisitions: valid.Acquisitions, States: valid.States, Transitions: []vocabulary.TransitionSpec{{Operation: 1, Input: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, From: 1, Outcomes: []vocabulary.TransitionOutcomeSpec{{Outcome: 1, To: 2}}}}}},
		{"transition duplicate outcome", vocabulary.ProtocolSpec{Acquisitions: valid.Acquisitions, States: valid.States, Transitions: []vocabulary.TransitionSpec{{Operation: 1, Input: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, From: 1, Outcomes: []vocabulary.TransitionOutcomeSpec{{Outcome: 0, To: 2}, {Outcome: 0, To: 2}}}}}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{base}, Protocols: []vocabulary.ProtocolSpec{test.value}}); err == nil {
				t.Fatal("invalid protocol coordinate accepted")
			}
		})
	}
}

func TestProtocolPublicObservablesIgnoreStateAndAcquisitionAuthorOrder(t *testing.T) {
	operations := []vocabulary.OperationSpec{protocolOperation("acquire-a", nil), protocolOperation("acquire-b", nil)}
	left := mustSeal(t, Spec{Operations: operations, Protocols: []vocabulary.ProtocolSpec{{
		Acquisitions: []vocabulary.AcquisitionSpec{{Operation: 1, Outcome: 0, Result: 0, State: 1}, {Operation: 2, Outcome: 0, Result: 0, State: 2}},
		States:       []vocabulary.StateSpec{{Name: "open"}, {Name: "closed", Final: true}},
	}}})
	right := mustSeal(t, Spec{Operations: operations, Protocols: []vocabulary.ProtocolSpec{{
		Acquisitions: []vocabulary.AcquisitionSpec{{Operation: 2, Outcome: 0, Result: 0, State: 1}, {Operation: 1, Outcome: 0, Result: 0, State: 2}},
		States:       []vocabulary.StateSpec{{Name: "closed", Final: true}, {Name: "open"}},
	}}})
	assertPublicContractEqual(t, left, right)
	if left.ContentID() != right.ContentID() {
		t.Fatal("state/acquisition author permutation changed ContentID")
	}
}

func TestProtocolStateCoordinatesAreAlphaInvariantAcrossCyclesAndRoots(t *testing.T) {
	operations := []vocabulary.OperationSpec{
		protocolOperation("root-a", []schematype.Type{testAny}),
		protocolOperation("root-b", nil),
	}
	left := mustSeal(t, Spec{Operations: operations, Protocols: []vocabulary.ProtocolSpec{{
		States: []vocabulary.StateSpec{{Name: "entry"}, {Name: "other", Final: true}, {Name: "cycle"}},
		Acquisitions: []vocabulary.AcquisitionSpec{
			{Operation: 2, Outcome: 0, Result: 0, State: 2},
			{Operation: 1, Outcome: 0, Result: 0, State: 1},
		},
		Transitions: []vocabulary.TransitionSpec{
			{Operation: 1, Input: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, From: 3, Outcomes: []vocabulary.TransitionOutcomeSpec{{Outcome: 0, To: 1}}},
			{Operation: 1, Input: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, From: 1, Outcomes: []vocabulary.TransitionOutcomeSpec{{Outcome: 0, To: 3}}},
		},
	}}})
	right := mustSeal(t, Spec{Operations: operations, Protocols: []vocabulary.ProtocolSpec{{
		// Different diagnostic names and author order. Role entry is ref 3,
		// other is ref 2, and the cycle state is ref 1.
		States: []vocabulary.StateSpec{{Name: "z"}, {Name: "q", Final: true}, {Name: "a"}},
		Acquisitions: []vocabulary.AcquisitionSpec{
			{Operation: 1, Outcome: 0, Result: 0, State: 3},
			{Operation: 2, Outcome: 0, Result: 0, State: 2},
		},
		Transitions: []vocabulary.TransitionSpec{
			{Operation: 1, Input: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, From: 3, Outcomes: []vocabulary.TransitionOutcomeSpec{{Outcome: 0, To: 1}}},
			{Operation: 1, Input: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, From: 1, Outcomes: []vocabulary.TransitionOutcomeSpec{{Outcome: 0, To: 3}}},
		},
	}}})
	if left.ContentID() != right.ContentID() {
		t.Fatal("state alpha rename/author permutation changed ContentID")
	}
	for _, contract := range []*Contract{left, right} {
		protocol, _ := contract.protocols.ProtocolAt(0)
		if states := contract.protocols.StateCount(protocol); states != 3 {
			t.Fatalf("state count = %d, want 3", states)
		}
		state, _ := contract.protocols.StateAt(protocol, 1)
		if final, ok := contract.protocols.StateFinal(protocol, state); !ok || !final {
			t.Fatalf("second canonical root final = %v/%v", final, ok)
		}
	}
}

func TestProtocolRejectsUnreachableState(t *testing.T) {
	_, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{protocolOperation("reachable", nil)}, Protocols: []vocabulary.ProtocolSpec{{
		Acquisitions: []vocabulary.AcquisitionSpec{{Operation: 1, Outcome: 0, Result: 0, State: 1}},
		States:       []vocabulary.StateSpec{{Name: "root"}, {Name: "orphan"}},
	}}})
	if err == nil {
		t.Fatal("unreachable nominal state accepted")
	}
}

func TestProtocolWideStatesSealIteratively(t *testing.T) {
	const width = 4096
	states := make([]vocabulary.StateSpec, width)
	for index := range states {
		states[index] = vocabulary.StateSpec{Name: fmt.Sprintf("state-%05d", width-index)}
	}
	transitions := make([]vocabulary.TransitionSpec, width-1)
	for index := range transitions {
		transitions[index] = vocabulary.TransitionSpec{
			Operation: 1, Input: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, From: vocabulary.StateRef(index + 1),
			Outcomes: []vocabulary.TransitionOutcomeSpec{{Outcome: 0, To: vocabulary.StateRef(index + 2)}},
		}
	}
	contract := mustSeal(t, Spec{
		Operations: []vocabulary.OperationSpec{protocolOperation("wide-protocol", []schematype.Type{testAny})},
		Protocols: []vocabulary.ProtocolSpec{{
			Acquisitions: []vocabulary.AcquisitionSpec{{Operation: 1, Outcome: 0, Result: 0, State: 1}}, States: states, Transitions: transitions,
		}},
	})
	p, _ := contract.protocols.ProtocolAt(0)
	if contract.protocols.StateCount(p) != width {
		t.Fatalf("state count = %d", contract.protocols.StateCount(p))
	}
	state, _ := contract.protocols.StateAt(p, 0)
	if name, ok := contract.protocols.StateName(p, state); !ok || name != "state-04096" {
		t.Fatalf("canonical first state = %q/%v", name, ok)
	}
	if !contract.ContentID().Available() {
		t.Fatal("wide protocol has no ContentID")
	}
}

func TestProtocolWideOutcomeRemapAvoidsQuadraticSearch(t *testing.T) {
	const width = 4096
	outcomes := make([]vocabulary.OutcomeSpec, width)
	acquisitions := make([]vocabulary.AcquisitionSpec, width)
	for index := range outcomes {
		record := testRawRecord(testRawRecordParts{Fields: []testRawField{{Name: fmt.Sprintf("field-%05d", index), Type: testRawAny}}})
		outcomes[width-index-1] = vocabulary.OutcomeSpec{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testEncode(record)}, Tail: vocabulary.ValuesClosed}}
		acquisitions[index] = vocabulary.AcquisitionSpec{Operation: 1, Outcome: uint32(index), Result: 0, State: 1}
	}
	op := protocolOperation("wide-outcome-protocol", nil)
	op.Outcomes = outcomes
	contract := mustSeal(t, Spec{
		Operations: []vocabulary.OperationSpec{op},
		Protocols: []vocabulary.ProtocolSpec{{
			Acquisitions: acquisitions,
			States:       []vocabulary.StateSpec{{Name: "open"}},
		}},
	})
	p, _ := contract.protocols.ProtocolAt(0)
	if contract.protocols.ProtocolAcquisitionCount(p) != width {
		t.Fatalf("acquisition count = %d", contract.protocols.ProtocolAcquisitionCount(p))
	}
}

func TestProtocolCallbackHoldersSealExactRetainedRelation(t *testing.T) {
	contract := mustSeal(t, specWithProtocolCallbackHolder(vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}))
	handle, ok := contract.protocols.ProtocolAt(0)
	if !ok || contract.protocols.ProtocolCallbackHolderCount(handle) != 1 {
		t.Fatalf("protocol callback-holder range = %d/%v", contract.protocols.ProtocolCallbackHolderCount(handle), ok)
	}
	op, input, callback, ok := contract.protocols.ProtocolCallbackHolderAt(handle, 0)
	if !ok || input != (vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}) {
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
	if _, _, _, found := contract.protocols.ProtocolCallbackHolderAt(handle, 1); found {
		t.Fatal("out-of-range callback holder resolved")
	}
	if contract.protocols.ProtocolCallbackHolderCount(vocabulary.Protocol(2)) != 0 {
		t.Fatal("opaque fabricated a callback-holder relation")
	}
	if allocs := testing.AllocsPerRun(1000, func() { _, _, _, _ = contract.protocols.ProtocolCallbackHolderAt(handle, 0) }); allocs != 0 {
		t.Fatalf("callback-holder query allocated %f times", allocs)
	}
}

func TestProtocolCallbackHoldersAcceptValuesVarAndIgnoreAuthorOrder(t *testing.T) {
	left := specWithProtocolCallbackHolder(vocabulary.InputSource{Kind: vocabulary.InputSourceValuesVar})
	right := specWithProtocolCallbackHolder(vocabulary.InputSource{Kind: vocabulary.InputSourceValuesVar})
	left.Protocols[0].CallbackHolders = append(left.Protocols[0].CallbackHolders,
		vocabulary.ProtocolCallbackHolderSpec{Operation: 1, Input: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, Callback: 1},
	)
	right.Protocols[0].CallbackHolders = []vocabulary.ProtocolCallbackHolderSpec{
		{Operation: 1, Input: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, Callback: 1},
		{Operation: 1, Input: vocabulary.InputSource{Kind: vocabulary.InputSourceValuesVar}, Callback: 1},
	}
	leftContract := mustSeal(t, left)
	rightContract := mustSeal(t, right)
	if leftContract.ContentID() != rightContract.ContentID() {
		t.Fatal("callback-holder author order changed ContentID")
	}
	handle, _ := leftContract.protocols.ProtocolAt(0)
	if leftContract.protocols.ProtocolCallbackHolderCount(handle) != 2 {
		t.Fatalf("callback-holder count = %d", leftContract.protocols.ProtocolCallbackHolderCount(handle))
	}
}

func TestProtocolCallbackHoldersRejectUnsealedAuthority(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Spec)
	}{
		{"all-inputs", func(spec *Spec) {
			spec.Protocols[0].CallbackHolders[0].Input = vocabulary.InputSource{Kind: vocabulary.InputSourceAllInputs}
		}},
		{"formal outside scope", func(spec *Spec) {
			spec.Protocols[0].CallbackHolders[0].Input = vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 2}
		}},
		{"Values variable outside scope", func(spec *Spec) {
			spec.Protocols[0].CallbackHolders[0].Input = vocabulary.InputSource{Kind: vocabulary.InputSourceValuesVar, Ordinal: 1}
		}},
		{"callback outside operation", func(spec *Spec) { spec.Protocols[0].CallbackHolders[0].Callback = 2 }},
		{"synchronous callback", func(spec *Spec) { spec.Operations[0].Callbacks[0].Lifecycle = vocabulary.CallbackSyncOptionalOnce }},
		{"duplicate triple", func(spec *Spec) {
			spec.Protocols[0].CallbackHolders = append(spec.Protocols[0].CallbackHolders, spec.Protocols[0].CallbackHolders[0])
		}},
		{"unbound operation", func(spec *Spec) {
			spec.Operations = append(spec.Operations, vocabulary.OperationSpec{Input: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}})
			spec.Protocols[0].CallbackHolders[0].Operation = 2
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			spec := specWithProtocolCallbackHolder(vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal})
			test.edit(&spec)
			if contract, err := testSeal(&spec); err == nil || contract != nil {
				t.Fatal("invalid callback-holder authority was published")
			}
		})
	}
}

func specWithProtocolCallbackHolder(input vocabulary.InputSource) Spec {
	closed := vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}
	return Spec{Operations: []vocabulary.OperationSpec{{
		Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"callback-holder"}}},
		ValuesVars: 1,
		Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny, testAny}, Tail: vocabulary.ValuesVariable, Var: 0},
		Outcomes:   []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed}}},
		Callbacks: []vocabulary.CallbackSpec{{
			Function:  vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 1},
			Admission: schematype.CallableAdmissionOrdinary,
			Arguments: closed,
			Outcomes: []vocabulary.TerminalSpec{
				{Kind: flowkind.OutcomeNormal, Values: closed}, {Kind: flowkind.OutcomeReturn, Values: closed},
				{Kind: flowkind.OutcomeThrow, Values: closed}, {Kind: flowkind.OutcomeYield, Values: closed}, {Kind: flowkind.OutcomeCancel, Values: closed},
			},
			Lifecycle: vocabulary.CallbackRetainedOptionalOnce,
			Effects:   vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}},
		Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}}, Protocols: []vocabulary.ProtocolSpec{{
		Acquisitions:    []vocabulary.AcquisitionSpec{{Operation: 1, Outcome: 0, Result: 0, State: 1}},
		States:          []vocabulary.StateSpec{{Name: "open"}},
		CallbackHolders: []vocabulary.ProtocolCallbackHolderSpec{{Operation: 1, Input: input, Callback: 1}},
	}}}
}
