package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/flow"
)

func TestTransformerCallOccurrenceForwardsTheFlowDenominator(t *testing.T) {
	published, err := Publish(rootAssembly(t, "transformer-call-empty.lua"))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	input := published.TransformerInput()
	if got, want := input.CallCount(), published.Flow().Authored().Calls().Count(); got != want {
		t.Fatalf("CallCount = %d, want Flow denominator %d", got, want)
	}
	if _, ok := input.CallAt(-1); ok {
		t.Fatal("CallAt accepted a negative authored index")
	}
	if _, ok := input.CallAt(input.CallCount()); ok {
		t.Fatal("CallAt accepted an out-of-range authored index")
	}
	var zero TransformerInput
	if zero.CallCount() != 0 {
		t.Fatal("zero TransformerInput exposed call rows")
	}
}

func TestTransformerCallOccurrenceRejectsForeignOwnerProof(t *testing.T) {
	leftPublished, err := Publish(rootAssembly(t, "transformer-call-left.lua"))
	if err != nil {
		t.Fatalf("Publish left: %v", err)
	}
	rightPublished, err := Publish(rootAssembly(t, "transformer-call-right.lua"))
	if err != nil {
		t.Fatalf("Publish right: %v", err)
	}
	left, right := leftPublished.TransformerInput(), rightPublished.TransformerInput()
	call := CallOccurrence{input: left, body: Body{input: right}}
	if left.OwnsCallOccurrence(call) || call.Available() {
		t.Fatal("foreign nested CallOccurrence proof crossed owner fence")
	}
}

func TestTransformerCallOccurrenceRejectsHostileFormAndReceiverSplices(t *testing.T) {
	published, err := Publish(rootAssembly(t, "transformer-call-form-splice.lua"))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	input := published.TransformerInput()
	callTerm := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	calleeTerm := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	actualsTerm := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	receiver := CallOperand{input: input, call: callTerm, term: calleeTerm, kind: CallOperandReceiver}
	if input.OwnsCallOperand(receiver) || receiver.Available() {
		t.Fatal("receiver proof accepted a non-receiver authored row")
	}
	plain := CallOccurrence{input: input, call: callTerm, form: flow.CallFormPlain,
		receiver: CallOperand{input: input, call: callTerm, term: calleeTerm, kind: CallOperandReceiver},
		actuals:  CallOperand{input: input, call: callTerm, term: actualsTerm, kind: CallOperandActuals}}
	if plain.Available() {
		t.Fatal("plain call accepted a spliced receiver")
	}
	method := CallOccurrence{input: input, call: callTerm, form: flow.CallFormMethod,
		callee:  CallOperand{input: input, call: callTerm, term: calleeTerm, kind: CallOperandCallee},
		actuals: CallOperand{input: input, call: callTerm, term: actualsTerm, kind: CallOperandActuals}}
	if method.Available() {
		t.Fatal("method call accepted an omitted receiver")
	}
}

func TestTransformerCallValuesRejectHostileCallArgumentSplices(t *testing.T) {
	leftPublished, err := Publish(rootAssembly(t, "transformer-call-values-left.lua"))
	if err != nil {
		t.Fatalf("Publish left: %v", err)
	}
	rightPublished, err := Publish(rootAssembly(t, "transformer-call-values-right.lua"))
	if err != nil {
		t.Fatalf("Publish right: %v", err)
	}
	left, right := leftPublished.TransformerInput(), rightPublished.TransformerInput()
	callOne := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	callTwo := keyspace.MakeTerm(keyspace.FamilyCall, 2)
	first := CallValues{
		input: left, call: callOne, width: 2,
		occurrence: CallOccurrence{input: left, call: callOne},
	}
	foreign := first
	foreign.input = right
	if foreign.Available() || left.OwnsCallValues(foreign) {
		t.Fatal("foreign CallValues proof crossed Program ownership")
	}
	swappedCall := first
	swappedCall.call = callTwo
	if swappedCall.Available() {
		t.Fatal("CallValues accepted a swapped parent call")
	}
	argument := CallArgument{values: first, index: 1}
	if argument.Available() || left.OwnsCallArgument(argument) {
		t.Fatal("CallArgument accepted a spliced member/position proof")
	}
}

func TestTransformerCallValuesRejectArbitraryAndReplaySpanSplices(t *testing.T) {
	leftPublished, err := Publish(rootAssembly(t, "transformer-call-values-span.lua"))
	if err != nil {
		t.Fatalf("Publish left: %v", err)
	}
	rightPublished, err := Publish(rootAssembly(t, "transformer-call-values-span.lua"))
	if err != nil {
		t.Fatalf("Publish right: %v", err)
	}
	left, right := leftPublished.TransformerInput(), rightPublished.TransformerInput()
	bodyTerm := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	actualsTerm := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	local, localOK := left.Span(bodyTerm)
	foreign, foreignOK := right.Span(bodyTerm)
	if !localOK || !foreignOK || !local.Equal(foreign) {
		t.Fatal("equivalent replay did not produce equivalent root spans")
	}
	if exactCallValuesSpan(left, foreign, bodyTerm) {
		t.Fatal("foreign equivalent root span crossed CallValues ownership")
	}
	if exactCallValuesSpan(left, local, actualsTerm) {
		t.Fatal("arbitrary root span was accepted for actual Values")
	}
}
