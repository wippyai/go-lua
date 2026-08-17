package target

import (
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

// This is deliberately a digest law, not a query law. Each row changes one
// sealed semantic family and proves the canonical persistence/hash path sees
// it. More detailed validation and permutation laws live beside each family.
func TestContentIDSemanticFamilyDeltas(t *testing.T) {
	cases := []struct {
		name string
		pair func() (*Contract, *Contract)
	}{
		{"binding", func() (*Contract, *Contract) {
			return deltaSeal(t, Spec{Operations: []OperationSpec{deltaPlain("left")}}), deltaSeal(t, Spec{Operations: []OperationSpec{deltaPlain("right")}})
		}},
		{"input type", func() (*Contract, *Contract) {
			left, right := deltaPlain("input"), deltaPlain("input")
			right.Input.Fixed[0] = testNumber
			return deltaSeal(t, Spec{Operations: []OperationSpec{left}}), deltaSeal(t, Spec{Operations: []OperationSpec{right}})
		}},
		{"formal constraint", func() (*Contract, *Contract) {
			left := testNewTypeParam("T", testString)
			right := testNewTypeParam("T", testNumber)
			return deltaSeal(t, Spec{Operations: []OperationSpec{genericBuiltin("formal", left)}}), deltaSeal(t, Spec{Operations: []OperationSpec{genericBuiltin("formal", right)}})
		}},
		{"Values tail", func() (*Contract, *Contract) {
			left, right := deltaOpenValues("tail", ValuesVariable, testInteger), deltaOpenValues("tail", ValuesClosed, testInteger)
			return deltaSeal(t, Spec{Operations: []OperationSpec{left}}), deltaSeal(t, Spec{Operations: []OperationSpec{right}})
		}},
		{"Values tail class", func() (*Contract, *Contract) {
			left, right := deltaOpenValues("tail-class", ValuesVariable, testInteger), deltaOpenValues("tail-class", ValuesVariable, testInteger)
			left.Outcomes[0].Values.TailType, right.Outcomes[0].Values.TailType = testString, testNumber
			return deltaSeal(t, Spec{Operations: []OperationSpec{left}}), deltaSeal(t, Spec{Operations: []OperationSpec{right}})
		}},
		{"Values suffix", func() (*Contract, *Contract) {
			left, right := deltaOpenValues("suffix", ValuesVariable, testInteger), deltaOpenValues("suffix", ValuesVariable, testBoolean)
			return deltaSeal(t, Spec{Operations: []OperationSpec{left}}), deltaSeal(t, Spec{Operations: []OperationSpec{right}})
		}},
		{"callback", func() (*Contract, *Contract) {
			left, right := deltaCallbackOperation("callback", 0, 0, false), deltaCallbackOperation("callback", 1, 0, false)
			return deltaSeal(t, Spec{Operations: []OperationSpec{left}}), deltaSeal(t, Spec{Operations: []OperationSpec{right}})
		}},
		{"callback lifecycle", func() (*Contract, *Contract) {
			left, right := deltaCallbackOperation("callback-lifecycle", 0, 0, false), deltaCallbackOperation("callback-lifecycle", 0, 0, false)
			right.Callbacks[0].Lifecycle = CallbackRetainedOptionalMany
			return deltaSeal(t, Spec{Operations: []OperationSpec{left}}), deltaSeal(t, Spec{Operations: []OperationSpec{right}})
		}},
		{"callback outcome", func() (*Contract, *Contract) {
			left, right := deltaCallbackOperation("callback-outcome", 0, 0, false), deltaCallbackOperation("callback-outcome", 0, 0, false)
			right.Callbacks[0].Outcomes[3].Values = callbackTail(0)
			return deltaSeal(t, Spec{Operations: []OperationSpec{left}}), deltaSeal(t, Spec{Operations: []OperationSpec{right}})
		}},
		{"callback result", func() (*Contract, *Contract) {
			left, right := deltaCallbackOperation("callback-result", 0, 1, true), deltaCallbackOperation("callback-result", 0, 2, true)
			return deltaSeal(t, Spec{Operations: []OperationSpec{left}}), deltaSeal(t, Spec{Operations: []OperationSpec{right}})
		}},
		{"result alias", func() (*Contract, *Contract) {
			left, right := deltaAliasOperation("alias", 0), deltaAliasOperation("alias", 1)
			return deltaSeal(t, Spec{Operations: []OperationSpec{left}}), deltaSeal(t, Spec{Operations: []OperationSpec{right}})
		}},
		{"fresh result", func() (*Contract, *Contract) {
			left, right := deltaPlain("fresh"), deltaPlain("fresh")
			left.Outcomes[0].Values.Fixed[0] = testBuiltinTableTop()
			right.Outcomes[0].Values.Fixed[0] = testFunction()
			left.Outcomes[0].FreshResults = []FreshResultSpec{{Result: 0, Kind: FreshTable}}
			right.Outcomes[0].FreshResults = []FreshResultSpec{{Result: 0, Kind: FreshFunction}}
			return deltaSeal(t, Spec{Operations: []OperationSpec{left}}), deltaSeal(t, Spec{Operations: []OperationSpec{right}})
		}},
		{"Produced capture", func() (*Contract, *Contract) {
			left, right := deltaProduced(0), deltaProduced(1)
			return deltaSeal(t, left), deltaSeal(t, right)
		}},
		{"suspension", func() (*Contract, *Contract) {
			left, right := deltaSuspension(ReentryOnce), deltaSuspension(ReentryMany)
			return deltaSeal(t, Spec{Operations: []OperationSpec{left}}), deltaSeal(t, Spec{Operations: []OperationSpec{right}})
		}},
		{"resume", func() (*Contract, *Contract) {
			left, right := deltaResume(0), deltaResume(1)
			return deltaSeal(t, Spec{Operations: []OperationSpec{left}}), deltaSeal(t, Spec{Operations: []OperationSpec{right}})
		}},
		{"transfer", func() (*Contract, *Contract) {
			left, right := deltaTransfer(TransferMayDeliver), deltaTransfer(TransferMayReject)
			return deltaSeal(t, Spec{Operations: []OperationSpec{left}}), deltaSeal(t, Spec{Operations: []OperationSpec{right}})
		}},
		{"effect", func() (*Contract, *Contract) { return deltaSeal(t, deltaEffects(2)), deltaSeal(t, deltaEffects(3)) }},
		{"protocol finality", func() (*Contract, *Contract) {
			return deltaSeal(t, deltaProtocolFinal(false)), deltaSeal(t, deltaProtocolFinal(true))
		}},
		{"protocol transition", func() (*Contract, *Contract) {
			return deltaSeal(t, deltaProtocolTransition(false)), deltaSeal(t, deltaProtocolTransition(true))
		}},
		{"protocol escape", func() (*Contract, *Contract) {
			return deltaSeal(t, deltaProtocolEscape(0)), deltaSeal(t, deltaProtocolEscape(1))
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			left, right := test.pair()
			if left.ContentID() == right.ContentID() {
				t.Fatalf("%s semantic change did not change ContentID", test.name)
			}
		})
	}
}

func deltaSeal(t *testing.T, spec Spec) *Contract { return mustSeal(t, spec) }

func deltaPlain(name string) OperationSpec {
	return OperationSpec{Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}}, Input: ValuesSpec{Fixed: []schematype.Type{testString}, Tail: ValuesClosed}, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testString}, Tail: ValuesClosed}}}, Effects: RowSpec{Tail: RowClosed}}
}

func deltaOpenValues(name string, tail ValuesTail, suffix schematype.Type) OperationSpec {
	return OperationSpec{Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}}, ValuesVars: 1, Input: ValuesSpec{Fixed: []schematype.Type{testString}, Tail: ValuesClosed}, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testString}, Tail: tail, Var: 0, Suffix: []schematype.Type{suffix}}}}, Effects: RowSpec{Tail: RowClosed}}
}

func deltaCallbackOperation(name string, function, callback uint32, result bool) OperationSpec {
	callbacks := []CallbackSpec{{Function: InputSource{Kind: InputSourceValueFormal, Ordinal: function}, Admission: OrdinaryCallable, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: CallbackRetainedOptionalOnce, Effects: RowSpec{Tail: RowClosed}}}
	if result {
		callbacks = []CallbackSpec{{Function: InputSource{Kind: InputSourceValueFormal}, Admission: OrdinaryCallable, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: CallbackRetainedOptionalOnce, Effects: RowSpec{Tail: RowClosed}}, {Function: InputSource{Kind: InputSourceValueFormal, Ordinal: 1}, Admission: OrdinaryCallable, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: CallbackRetainedOptionalOnce, Effects: RowSpec{Tail: RowClosed}}}
	}
	outcome := OutcomeSpec{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testAny, testAny}, Tail: ValuesClosed}}
	if result {
		outcome.CallbackResults = []CallbackResultSpec{{Result: 0, Callback: CallbackRef(callback)}}
	}
	return OperationSpec{Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}}, ValuesVars: 5, Input: ValuesSpec{Fixed: []schematype.Type{testAny, testAny}, Tail: ValuesVariable, Var: 0}, Callbacks: callbacks, Outcomes: []OutcomeSpec{outcome}, Effects: RowSpec{Tail: RowClosed}}
}

func deltaAliasOperation(name string, ordinal uint32) OperationSpec {
	return OperationSpec{Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}}, Input: ValuesSpec{Fixed: []schematype.Type{testString, testString}, Tail: ValuesClosed}, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testString, testString}, Tail: ValuesClosed}, ResultAliases: []ResultAliasSpec{{Result: 0, Source: InputSource{Kind: InputSourceValueFormal, Ordinal: ordinal}}}}}, Effects: RowSpec{Tail: RowClosed}}
}

func deltaProduced(capture uint32) Spec {
	parent := OperationSpec{Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"produced"}}}, Input: ValuesSpec{Fixed: []schematype.Type{testString, testString}, Tail: ValuesClosed}, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed}, Produced: []ProducedSpec{{Result: 0, Operation: 2, Captures: []CaptureSpec{{Kind: CaptureValueFormal, Ordinal: capture}}}}}}, Effects: RowSpec{Tail: RowClosed}}
	child := OperationSpec{Input: ValuesSpec{Tail: ValuesClosed}, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}}, Effects: RowSpec{Tail: RowClosed}}
	return Spec{Operations: []OperationSpec{parent, child}}
}

func deltaSuspension(multiplicity ReentryMultiplicity) OperationSpec {
	return OperationSpec{Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"suspend"}}}, Input: ValuesSpec{Tail: ValuesClosed}, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}, {Kind: flowkind.OutcomeYield, Values: ValuesSpec{Tail: ValuesClosed}}}, Suspensions: []SuspensionSpec{{Yield: 1, Reentry: 0, Source: ReentryByCall, Multiplicity: multiplicity}}, Effects: RowSpec{Tail: RowClosed}}
}

func deltaResume(carrier ValueFormal) OperationSpec {
	return OperationSpec{Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"resume"}}}, ValuesVars: 1, Input: ValuesSpec{Fixed: []schematype.Type{testString, testString}, Tail: ValuesVariable, Var: 0}, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}}, Resumes: []ResumeSpec{completeResume(ResumeSourceValueFormal, carrier, 0)}, Effects: RowSpec{Tail: RowClosed}}
}

func deltaTransfer(normal TransferPossibility) OperationSpec {
	return OperationSpec{Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"transfer"}}}, Input: ValuesSpec{Fixed: []schematype.Type{testString}, Tail: ValuesClosed}, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}, {Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Tail: ValuesClosed}}}, Transfers: []TransferSpec{{Endpoint: TransferEndpoint{Kind: TransferEndpointExternal}, Payload: InputSource{Kind: InputSourceValueFormal}, Alias: InputSource{Kind: InputSourceValueFormal}, Identity: TransferIdentityUnspecified, Capabilities: TransferCapabilitiesUnspecified, Outcomes: []TransferOutcomeSpec{{Outcome: 0, Possibility: normal}, {Outcome: 1, Possibility: TransferMayReject}}}}, Effects: RowSpec{Tail: RowClosed}}
}

func deltaEffects(target SpecRef) Spec {
	operations := []OperationSpec{deltaPlain("effect-a"), deltaPlain("effect-b"), deltaPlain("effect-c")}
	operations[0].Effects = RowSpec{Occurrences: []EffectSpec{{Target: target, ValueArgs: []ValueFormal{0}}}, Tail: RowClosed}
	return Spec{Operations: operations}
}

func deltaProtocolFinal(final bool) Spec {
	return Spec{
		Operations: []OperationSpec{protocolOperation("protocol-final", nil)},
		Protocols: []ProtocolSpec{{
			Acquisitions: []AcquisitionSpec{{Operation: 1, Outcome: 0, Result: 0, State: 1}},
			States:       []StateSpec{{Name: "state", Final: final}},
		}},
	}
}

func deltaProtocolTransition(swapped bool) Spec {
	toNormal, toThrow := StateRef(2), StateRef(3)
	if swapped {
		toNormal, toThrow = toThrow, toNormal
	}
	op := protocolOperation("protocol-transition", []schematype.Type{testString})
	op.Outcomes = []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed}}, {Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed}}}
	return Spec{
		Operations: []OperationSpec{op},
		Protocols: []ProtocolSpec{{
			Acquisitions: []AcquisitionSpec{{Operation: 1, Outcome: 0, Result: 0, State: 1}},
			States:       []StateSpec{{Name: "root"}, {Name: "normal", Final: true}, {Name: "throw"}},
			Transitions: []TransitionSpec{{
				Operation: 1, Input: InputSource{Kind: InputSourceValueFormal}, From: 1,
				Outcomes: []TransitionOutcomeSpec{{Outcome: 0, To: toNormal}, {Outcome: 1, To: toThrow}},
			}},
		}},
	}
}

func deltaProtocolEscape(ordinal uint32) Spec {
	return Spec{
		Operations: []OperationSpec{protocolOperation("protocol-escape", []schematype.Type{testString, testString})},
		Protocols: []ProtocolSpec{{
			Acquisitions: []AcquisitionSpec{{Operation: 1, Outcome: 0, Result: 0, State: 1}},
			States:       []StateSpec{{Name: "state"}},
			Escapes:      []EscapeSpec{{Operation: 1, Input: InputSource{Kind: InputSourceValueFormal, Ordinal: ordinal}}},
		}},
	}
}
