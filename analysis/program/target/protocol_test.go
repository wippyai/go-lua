package target

import (
	"fmt"
	"testing"

	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func protocolOperation(name string, input []schematype.Type) OperationSpec {
	return OperationSpec{
		Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}},
		Input:    ValuesSpec{Fixed: input, Tail: ValuesClosed},
		Outcomes: []OutcomeSpec{{
			Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed},
		}},
		Effects: RowSpec{Tail: RowClosed},
	}
}

func TestProtocolMultipleAcquisitionsAndEntryRows(t *testing.T) {
	accept := protocolOperation("accept", nil)
	close := protocolOperation("close", []schematype.Type{testAny})
	connect := protocolOperation("connect", nil)
	contract := mustSeal(t, Spec{
		Operations: []OperationSpec{accept, close, connect},
		Protocols: []ProtocolSpec{{
			Acquisitions: []AcquisitionSpec{
				{Operation: 3, Outcome: 0, Result: 0, State: 1},
				{Operation: 1, Outcome: 0, Result: 0, State: 1},
			},
			States: []StateSpec{{Name: "open"}, {Name: "closed", Final: true}},
			Transitions: []TransitionSpec{{
				Operation: 2, Input: InputSource{Kind: InputSourceValueFormal}, From: 1,
				Outcomes: []TransitionOutcomeSpec{{Outcome: 0, To: 2}},
			}},
			Escapes: []EscapeSpec{{Operation: 2, Input: InputSource{Kind: InputSourceValueFormal}}},
		}},
	})
	if contract.ProtocolCount() != 1 {
		t.Fatalf("ProtocolCount = %d", contract.ProtocolCount())
	}
	p, _ := contract.ProtocolAt(0)
	if contract.ProtocolAcquisitionCount(p) != 2 {
		t.Fatalf("acquisition count = %d", contract.ProtocolAcquisitionCount(p))
	}
	for index, wantName := range []string{"open", "closed"} {
		state, ok := contract.StateAt(p, index)
		name, nameOK := contract.StateName(p, state)
		if !ok || !nameOK || name != wantName {
			t.Fatalf("state %d = %d/%q", index, state, name)
		}
		final, finalOK := contract.StateFinal(p, state)
		if !finalOK || final != (index == 1) {
			t.Fatalf("final %d = %v/%v", index, final, finalOK)
		}
	}
	if op, source, ordinal, from, ok := contract.TransitionAt(p, 0); !ok || source != InputSourceValueFormal || ordinal != 0 || from != 1 || op == 0 {
		t.Fatalf("transition = %d/%d/%d/%d/%v", op, source, ordinal, from, ok)
	}
	if outcome, to, ok := contract.TransitionOutcomeAt(p, 0, 0); !ok || outcome != 0 || to != 2 {
		t.Fatalf("transition outcome = %d/%d/%v", outcome, to, ok)
	}
	if contract.EscapeCount(p) != 2 {
		t.Fatalf("escape count = %d", contract.EscapeCount(p))
	}
	opaque, _ := contract.Opaque()
	op, source, ordinal, ok := contract.EscapeAt(p, 1)
	if !ok || op != opaque || source != InputSourceAllInputs || ordinal != 0 {
		t.Fatalf("derived opaque escape = %d/%d/%d/%v", op, source, ordinal, ok)
	}
	if contract.TransitionCount(p) != 1 {
		t.Fatal("opaque fabricated a protocol transition")
	}
	if allocs := testing.AllocsPerRun(1000, func() { _, _, _, _ = contract.EscapeAt(p, 1) }); allocs != 0 {
		t.Fatalf("derived escape allocated %f times", allocs)
	}
}

func TestProtocolRejectsInvalidNominalAuthority(t *testing.T) {
	base := protocolOperation("acquire", []schematype.Type{testAny})
	valid := ProtocolSpec{Acquisitions: []AcquisitionSpec{{Operation: 1, Outcome: 0, Result: 0, State: 1}}, States: []StateSpec{{Name: "open"}}}
	for _, test := range []struct {
		name      string
		protocols []ProtocolSpec
	}{
		{"empty acquisitions", []ProtocolSpec{{States: []StateSpec{{Name: "open"}}}}},
		{"empty state", []ProtocolSpec{{Acquisitions: valid.Acquisitions, States: []StateSpec{{Name: ""}}}}},
		{"invalid utf8", []ProtocolSpec{{Acquisitions: valid.Acquisitions, States: []StateSpec{{Name: "\xff"}}}}},
		{"duplicate state", []ProtocolSpec{{Acquisitions: valid.Acquisitions, States: []StateSpec{{Name: "open"}, {Name: "open"}}}}},
		{"all inputs authored", []ProtocolSpec{{Acquisitions: valid.Acquisitions, States: valid.States, Escapes: []EscapeSpec{{Operation: 1, Input: InputSource{Kind: InputSourceAllInputs}}}}}},
		{"shared acquisition", []ProtocolSpec{valid, valid}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := testSeal(&Spec{Operations: []OperationSpec{base}, Protocols: test.protocols}); err == nil {
				t.Fatal("invalid protocol accepted")
			}
		})
	}
}

func TestProtocolRejectsBadOutcomeAndTransitionCoordinates(t *testing.T) {
	base := protocolOperation("protocol-coordinates", []schematype.Type{testAny})
	valid := ProtocolSpec{
		Acquisitions: []AcquisitionSpec{{Operation: 1, Outcome: 0, Result: 0, State: 1}},
		States:       []StateSpec{{Name: "open"}, {Name: "closed", Final: true}},
	}
	cases := []struct {
		name  string
		value ProtocolSpec
	}{
		{"result outside fixed values", ProtocolSpec{Acquisitions: []AcquisitionSpec{{Operation: 1, Outcome: 0, Result: 1, State: 1}}, States: valid.States}},
		{"acquisition state outside scope", ProtocolSpec{Acquisitions: []AcquisitionSpec{{Operation: 1, Outcome: 0, Result: 0, State: 3}}, States: valid.States}},
		{"transition from outside scope", ProtocolSpec{Acquisitions: valid.Acquisitions, States: valid.States, Transitions: []TransitionSpec{{Operation: 1, Input: InputSource{Kind: InputSourceValueFormal}, From: 3, Outcomes: []TransitionOutcomeSpec{{Outcome: 0, To: 1}}}}}},
		{"transition outcome outside scope", ProtocolSpec{Acquisitions: valid.Acquisitions, States: valid.States, Transitions: []TransitionSpec{{Operation: 1, Input: InputSource{Kind: InputSourceValueFormal}, From: 1, Outcomes: []TransitionOutcomeSpec{{Outcome: 1, To: 2}}}}}},
		{"transition duplicate outcome", ProtocolSpec{Acquisitions: valid.Acquisitions, States: valid.States, Transitions: []TransitionSpec{{Operation: 1, Input: InputSource{Kind: InputSourceValueFormal}, From: 1, Outcomes: []TransitionOutcomeSpec{{Outcome: 0, To: 2}, {Outcome: 0, To: 2}}}}}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := testSeal(&Spec{Operations: []OperationSpec{base}, Protocols: []ProtocolSpec{test.value}}); err == nil {
				t.Fatal("invalid protocol coordinate accepted")
			}
		})
	}
}

func TestProtocolPublicObservablesIgnoreStateAndAcquisitionAuthorOrder(t *testing.T) {
	operations := []OperationSpec{protocolOperation("acquire-a", nil), protocolOperation("acquire-b", nil)}
	left := mustSeal(t, Spec{Operations: operations, Protocols: []ProtocolSpec{{
		Acquisitions: []AcquisitionSpec{{Operation: 1, Outcome: 0, Result: 0, State: 1}, {Operation: 2, Outcome: 0, Result: 0, State: 2}},
		States:       []StateSpec{{Name: "open"}, {Name: "closed", Final: true}},
	}}})
	right := mustSeal(t, Spec{Operations: operations, Protocols: []ProtocolSpec{{
		Acquisitions: []AcquisitionSpec{{Operation: 2, Outcome: 0, Result: 0, State: 1}, {Operation: 1, Outcome: 0, Result: 0, State: 2}},
		States:       []StateSpec{{Name: "closed", Final: true}, {Name: "open"}},
	}}})
	assertPublicContractEqual(t, left, right)
	if left.ContentID() != right.ContentID() {
		t.Fatal("state/acquisition author permutation changed ContentID")
	}
}

func TestProtocolStateCoordinatesAreAlphaInvariantAcrossCyclesAndRoots(t *testing.T) {
	operations := []OperationSpec{
		protocolOperation("root-a", []schematype.Type{testAny}),
		protocolOperation("root-b", nil),
	}
	left := mustSeal(t, Spec{Operations: operations, Protocols: []ProtocolSpec{{
		States: []StateSpec{{Name: "entry"}, {Name: "other", Final: true}, {Name: "cycle"}},
		Acquisitions: []AcquisitionSpec{
			{Operation: 2, Outcome: 0, Result: 0, State: 2},
			{Operation: 1, Outcome: 0, Result: 0, State: 1},
		},
		Transitions: []TransitionSpec{
			{Operation: 1, Input: InputSource{Kind: InputSourceValueFormal}, From: 3, Outcomes: []TransitionOutcomeSpec{{Outcome: 0, To: 1}}},
			{Operation: 1, Input: InputSource{Kind: InputSourceValueFormal}, From: 1, Outcomes: []TransitionOutcomeSpec{{Outcome: 0, To: 3}}},
		},
	}}})
	right := mustSeal(t, Spec{Operations: operations, Protocols: []ProtocolSpec{{
		// Different diagnostic names and author order. Role entry is ref 3,
		// other is ref 2, and the cycle state is ref 1.
		States: []StateSpec{{Name: "z"}, {Name: "q", Final: true}, {Name: "a"}},
		Acquisitions: []AcquisitionSpec{
			{Operation: 1, Outcome: 0, Result: 0, State: 3},
			{Operation: 2, Outcome: 0, Result: 0, State: 2},
		},
		Transitions: []TransitionSpec{
			{Operation: 1, Input: InputSource{Kind: InputSourceValueFormal}, From: 3, Outcomes: []TransitionOutcomeSpec{{Outcome: 0, To: 1}}},
			{Operation: 1, Input: InputSource{Kind: InputSourceValueFormal}, From: 1, Outcomes: []TransitionOutcomeSpec{{Outcome: 0, To: 3}}},
		},
	}}})
	if left.ContentID() != right.ContentID() {
		t.Fatal("state alpha rename/author permutation changed ContentID")
	}
	for _, contract := range []*Contract{left, right} {
		protocol, _ := contract.ProtocolAt(0)
		if states := contract.StateCount(protocol); states != 3 {
			t.Fatalf("state count = %d, want 3", states)
		}
		state, _ := contract.StateAt(protocol, 1)
		if final, ok := contract.StateFinal(protocol, state); !ok || !final {
			t.Fatalf("second canonical root final = %v/%v", final, ok)
		}
	}
}

func TestProtocolRejectsUnreachableState(t *testing.T) {
	_, err := testSeal(&Spec{Operations: []OperationSpec{protocolOperation("reachable", nil)}, Protocols: []ProtocolSpec{{
		Acquisitions: []AcquisitionSpec{{Operation: 1, Outcome: 0, Result: 0, State: 1}},
		States:       []StateSpec{{Name: "root"}, {Name: "orphan"}},
	}}})
	if err == nil {
		t.Fatal("unreachable nominal state accepted")
	}
}

func TestProtocolWideStatesSealIteratively(t *testing.T) {
	const width = 4096
	states := make([]StateSpec, width)
	for index := range states {
		states[index] = StateSpec{Name: fmt.Sprintf("state-%05d", width-index)}
	}
	transitions := make([]TransitionSpec, width-1)
	for index := range transitions {
		transitions[index] = TransitionSpec{
			Operation: 1, Input: InputSource{Kind: InputSourceValueFormal}, From: StateRef(index + 1),
			Outcomes: []TransitionOutcomeSpec{{Outcome: 0, To: StateRef(index + 2)}},
		}
	}
	contract := mustSeal(t, Spec{
		Operations: []OperationSpec{protocolOperation("wide-protocol", []schematype.Type{testAny})},
		Protocols: []ProtocolSpec{{
			Acquisitions: []AcquisitionSpec{{Operation: 1, Outcome: 0, Result: 0, State: 1}}, States: states, Transitions: transitions,
		}},
	})
	p, _ := contract.ProtocolAt(0)
	if contract.StateCount(p) != width {
		t.Fatalf("state count = %d", contract.StateCount(p))
	}
	state, _ := contract.StateAt(p, 0)
	if name, ok := contract.StateName(p, state); !ok || name != "state-04096" {
		t.Fatalf("canonical first state = %q/%v", name, ok)
	}
	if !contract.ContentID().Available() {
		t.Fatal("wide protocol has no ContentID")
	}
}

func TestProtocolWideOutcomeRemapAvoidsQuadraticSearch(t *testing.T) {
	const width = 4096
	outcomes := make([]OutcomeSpec, width)
	acquisitions := make([]AcquisitionSpec, width)
	for index := range outcomes {
		record := testRawRecord(testRawRecordParts{Fields: []testRawField{{Name: fmt.Sprintf("field-%05d", index), Type: testRawAny}}})
		outcomes[width-index-1] = OutcomeSpec{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testEncode(record)}, Tail: ValuesClosed}}
		acquisitions[index] = AcquisitionSpec{Operation: 1, Outcome: uint32(index), Result: 0, State: 1}
	}
	op := protocolOperation("wide-outcome-protocol", nil)
	op.Outcomes = outcomes
	contract := mustSeal(t, Spec{
		Operations: []OperationSpec{op},
		Protocols: []ProtocolSpec{{
			Acquisitions: acquisitions,
			States:       []StateSpec{{Name: "open"}},
		}},
	})
	p, _ := contract.ProtocolAt(0)
	if contract.ProtocolAcquisitionCount(p) != width {
		t.Fatalf("acquisition count = %d", contract.ProtocolAcquisitionCount(p))
	}
}
