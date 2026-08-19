package protocol

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/target/exactkey"
	"github.com/wippyai/go-lua/analysis/program/target/operation"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/internal/framing"
)

func protocolInput() Input {
	keys, err := exactkey.Compile([]keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "protocol"}})
	if err != nil {
		panic(err)
	}
	geometry, err := operation.CompileGeometry(operation.Input{Operations: []operation.OperationInput{{
		Source:            0,
		Bindings:          []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"protocol"}}},
		InputFormalCount:  1,
		OutcomeValueSlots: []operation.OutcomeInput{{ValueSlots: 1}},
		Callbacks:         []operation.CallbackInput{{Source: 0, Lifecycle: vocabulary.CallbackRetainedOptionalOnce}},
	}}})
	if err != nil {
		panic(err)
	}
	operations, err := operation.CompileAnchors(geometry, keys)
	if err != nil {
		panic(err)
	}
	return Input{
		Protocols: []vocabulary.ProtocolSpec{{
			Acquisitions: []vocabulary.AcquisitionSpec{{Operation: 1, Outcome: 0, Result: 0, State: 1}},
			States:       []vocabulary.StateSpec{{Name: "open", Final: true}},
			Transitions: []vocabulary.TransitionSpec{{
				Operation: 1,
				Input:     vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal},
				From:      1,
				Outcomes:  []vocabulary.TransitionOutcomeSpec{{Outcome: 0, To: 1}},
			}},
		}},
		Operations: operations,
	}
}

func TestCompilePublishesImmutableProtocolValue(t *testing.T) {
	input := protocolInput()
	table, err := Compile(input)
	if err != nil {
		t.Fatal(err)
	}
	if table.ProtocolCount() != 1 {
		t.Fatalf("protocol count = %d", table.ProtocolCount())
	}
	protocol, ok := table.ProtocolAt(0)
	if !ok || table.StateCount(protocol) != 1 || table.TransitionCount(protocol) != 1 {
		t.Fatalf("sealed protocol geometry = %d/%v states=%d transitions=%d", protocol, ok, table.StateCount(protocol), table.TransitionCount(protocol))
	}
	if got, ok := table.StateAt(protocol, 0); !ok || got == 0 {
		t.Fatalf("state handle = %d/%v", got, ok)
	}
	counts := table.Counts()
	if counts.Protocols != 1 || counts.States != 1 || counts.Transitions != 1 || counts.TransitionOutcomes != 1 || counts.Escapes != 1 {
		t.Fatalf("owner counts = %#v", counts)
	}
}

func TestProtocolRejectsInputOutsideOwnerGeometry(t *testing.T) {
	input := protocolInput()
	input.Protocols[0].Transitions[0].Input = vocabulary.InputSource{Kind: vocabulary.InputSourceValuesVar, Ordinal: 1}
	if _, err := Compile(input); err == nil {
		t.Fatal("protocol accepted a ValuesVar outside the owner geometry")
	}
}

func TestProtocolBindsOnlyOwnerIssuedRetainedCallbacks(t *testing.T) {
	input := protocolInput()
	input.Protocols[0].CallbackHolders = []vocabulary.ProtocolCallbackHolderSpec{{
		Operation: 1,
		Input:     vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal},
		Callback:  1,
	}}
	table, err := Compile(input)
	if err != nil {
		t.Fatal(err)
	}
	protocol, _ := table.ProtocolAt(0)
	if table.ProtocolCallbackHolderCount(protocol) != 1 {
		t.Fatal("callback holder was not published")
	}
	_, _, callback, ok := table.ProtocolCallbackHolderAt(protocol, 0)
	if !ok || callback != 1 {
		t.Fatalf("callback holder = %d/%v", callback, ok)
	}
}

func TestProtocolIdentityContributionUsesTheSealedRows(t *testing.T) {
	input := protocolInput()
	table, err := Compile(input)
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&buffer, "program/target-contract", 23); err != nil {
		t.Fatal(err)
	}
	if err := table.Encode(&writer); err != nil {
		t.Fatal(err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	if buffer.Len() == 0 {
		t.Fatal("protocol identity contribution is empty")
	}
}
