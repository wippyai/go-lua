package target

import (
	"bytes"
	"crypto/sha256"
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"github.com/wippyai/go-lua/internal/framing"
	"testing"
)

func TestContentIDIsCanonicalAndOwned(t *testing.T) {
	left := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{
		contentIDOperation("alpha", []vocabulary.OutcomeSpec{
			{Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed}},
			{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testBoolean}, Tail: vocabulary.ValuesClosed}},
		}),
		contentIDOperation("beta", []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}),
	}})
	right := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{
		contentIDOperation("beta", []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}),
		contentIDOperation("alpha", []vocabulary.OutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testBoolean}, Tail: vocabulary.ValuesClosed}},
			{Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed}},
		}),
	}})
	leftID, rightID := left.ContentID(), right.ContentID()
	if !leftID.Available() || leftID != rightID || leftID != left.ContentID() {
		t.Fatalf("canonical ContentID = %v/%v", leftID, rightID)
	}

	changed := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{
		contentIDOperation("alpha", []vocabulary.OutcomeSpec{
			{Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed}},
			{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed}},
		}),
		contentIDOperation("beta", []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}),
	}})
	if leftID == changed.ContentID() {
		t.Fatal("outcome semantic change did not change ContentID")
	}
}

func TestContentIDTypeOccurrenceAllocationsAreConstant(t *testing.T) {
	seal := func(width int) *Contract {
		values := make([]schematype.Type, width)
		for index := range values {
			values[index] = testString
		}
		return mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{{
			Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"allocation"}}},
			Input:    vocabulary.ValuesSpec{Fixed: values, Tail: vocabulary.ValuesClosed},
			Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: values, Tail: vocabulary.ValuesClosed}}},
			Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}}})
	}
	small, wide := seal(1), seal(4096)
	smallAllocs := testing.AllocsPerRun(100, func() { _ = small.ContentID() })
	wideAllocs := testing.AllocsPerRun(100, func() { _ = wide.ContentID() })
	if wideAllocs != smallAllocs {
		t.Fatalf("ContentID allocations scale with repeated frozen type occurrences: small=%f wide=%f", smallAllocs, wideAllocs)
	}
}

func TestContentIDIncludesDerivedOpaqueSemanticsAndFailsClosed(t *testing.T) {
	empty := mustSeal(t, Spec{})
	if !empty.ContentID().Available() {
		t.Fatal("opaque-only contract has no ContentID")
	}
	if (&Contract{}).ContentID().Available() || (*Contract)(nil).ContentID().Available() {
		t.Fatal("unavailable contract produced a ContentID")
	}

	withProtocol := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{contentIDOperation("acquire", []vocabulary.OutcomeSpec{{
		Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed},
	}})}, Protocols: []vocabulary.ProtocolSpec{{
		Acquisitions: []vocabulary.AcquisitionSpec{{Operation: 1, Outcome: 0, Result: 0, State: 1}},
		States:       []vocabulary.StateSpec{{Name: "open"}},
	}}})
	if empty.ContentID() == withProtocol.ContentID() {
		t.Fatal("protocol and its derived opaque escape were omitted from ContentID")
	}
}

func TestContentIDTypeFormalAlphaInvariant(t *testing.T) {
	leftFormal := testNewTypeParam("T", testString)
	rightFormal := testNewTypeParam("Renamed", testString)
	left := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{genericBuiltin("identity", leftFormal)}})
	right := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{genericBuiltin("identity", rightFormal)}})
	if left.ContentID() != right.ContentID() {
		t.Fatal("alpha-equivalent type formals changed ContentID")
	}
}

// The current Target layout cannot share an identity with its immediately
// preceding namespace, even if every surviving observable row encodes alike.
func TestContentIDNamespaceSeparatesPriorContractIdentity(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{spawnTestOperation("spawn")}})
	current := contract.ContentID()
	if !current.Available() {
		t.Fatal("current contract has no ContentID")
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

func contentIDOperation(name string, outcomes []vocabulary.OutcomeSpec) vocabulary.OperationSpec {
	return vocabulary.OperationSpec{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{name}}},
		Input:    vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed},
		Outcomes: outcomes,
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
}

func TestContentIDBootEncodingTracksWholeObjectShape(t *testing.T) {
	left := completeBootSpec("Lua 5.3", vocabulary.InitialMutable)
	right := completeBootSpec("Lua 5.3", vocabulary.InitialMutable)
	right.InitialRoots[0].Shape.Immutable = !right.InitialRoots[0].Shape.Immutable
	leftID := mustSeal(t, left).ContentID()
	rightID := mustSeal(t, right).ContentID()
	if !leftID.Available() || !rightID.Available() {
		t.Fatal("boot contracts did not receive ContentIDs")
	}
	if leftID == rightID {
		t.Fatal("boot shape mutation was omitted from ContentID")
	}
}

func TestContentIDOperationEncodingTracksCanonicalOutcomeRows(t *testing.T) {
	base := []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed}}}
	left := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{contentIDOperation("operation", base)}}).ContentID()
	changed := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{contentIDOperation("operation", []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testBoolean}, Tail: vocabulary.ValuesClosed}}})}}).ContentID()
	if left == changed {
		t.Fatal("operation outcome type mutation was omitted from ContentID")
	}
}

func TestContentIDProtocolEncodingTracksStateFinality(t *testing.T) {
	left := mustSeal(t, deltaProtocolFinal(false)).ContentID()
	right := mustSeal(t, deltaProtocolFinal(true)).ContentID()
	if left == right {
		t.Fatal("protocol state finality mutation was omitted from ContentID")
	}
}

func TestContentIDSubedgeEncodingTracksRouteAuthority(t *testing.T) {
	leftSpec := Spec{Operations: []vocabulary.OperationSpec{protectedSubedgeOperation("subedge-id", false, false, false)}}
	rightSpec := Spec{Operations: []vocabulary.OperationSpec{protectedSubedgeOperation("subedge-id", true, false, false)}}
	left := mustSeal(t, leftSpec).ContentID()
	right := mustSeal(t, rightSpec).ContentID()
	if left == right {
		t.Fatal("subedge route mutation was omitted from ContentID")
	}
}

func TestContentIDValueEncodingTracksFrozenTypeDeclarations(t *testing.T) {
	left := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{contentIDOperation("value-id", []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed}}})}}).ContentID()
	right := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{contentIDOperation("value-id", []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testNumber}, Tail: vocabulary.ValuesClosed}}})}}).ContentID()
	if left == right {
		t.Fatal("frozen value type mutation was omitted from ContentID")
	}
}

// This is deliberately a digest law, not a query law. Each row changes one
// sealed semantic family and proves the canonical persistence/hash path sees
// it. More detailed validation and permutation laws live beside each family.
func TestContentIDSemanticFamilyDeltas(t *testing.T) {
	cases := []struct {
		name string
		pair func() (*Contract, *Contract)
	}{
		{"binding", func() (*Contract, *Contract) {
			return deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{deltaPlain("left")}}), deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{deltaPlain("right")}})
		}},
		{"input type", func() (*Contract, *Contract) {
			left, right := deltaPlain("input"), deltaPlain("input")
			right.Input.Fixed[0] = testNumber
			return deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{left}}), deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{right}})
		}},
		{"formal constraint", func() (*Contract, *Contract) {
			left := testNewTypeParam("T", testString)
			right := testNewTypeParam("T", testNumber)
			return deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{genericBuiltin("formal", left)}}), deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{genericBuiltin("formal", right)}})
		}},
		{"Values tail", func() (*Contract, *Contract) {
			left, right := deltaOpenValues("tail", vocabulary.ValuesVariable, testInteger), deltaOpenValues("tail", vocabulary.ValuesClosed, testInteger)
			return deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{left}}), deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{right}})
		}},
		{"Values tail class", func() (*Contract, *Contract) {
			left, right := deltaOpenValues("tail-class", vocabulary.ValuesVariable, testInteger), deltaOpenValues("tail-class", vocabulary.ValuesVariable, testInteger)
			left.Outcomes[0].Values.TailType, right.Outcomes[0].Values.TailType = testString, testNumber
			return deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{left}}), deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{right}})
		}},
		{"Values suffix", func() (*Contract, *Contract) {
			left, right := deltaOpenValues("suffix", vocabulary.ValuesVariable, testInteger), deltaOpenValues("suffix", vocabulary.ValuesVariable, testBoolean)
			return deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{left}}), deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{right}})
		}},
		{"callback", func() (*Contract, *Contract) {
			left, right := deltaCallbackOperation("callback", 0, 0, false), deltaCallbackOperation("callback", 1, 0, false)
			return deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{left}}), deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{right}})
		}},
		{"callback lifecycle", func() (*Contract, *Contract) {
			left, right := deltaCallbackOperation("callback-lifecycle", 0, 0, false), deltaCallbackOperation("callback-lifecycle", 0, 0, false)
			right.Callbacks[0].Lifecycle = vocabulary.CallbackRetainedOptionalMany
			return deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{left}}), deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{right}})
		}},
		{"callback outcome", func() (*Contract, *Contract) {
			left, right := deltaCallbackOperation("callback-outcome", 0, 0, false), deltaCallbackOperation("callback-outcome", 0, 0, false)
			right.Callbacks[0].Outcomes[3].Values = callbackTail(0)
			return deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{left}}), deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{right}})
		}},
		{"callback result", func() (*Contract, *Contract) {
			left, right := deltaCallbackOperation("callback-result", 0, 1, true), deltaCallbackOperation("callback-result", 0, 2, true)
			return deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{left}}), deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{right}})
		}},
		{"result alias", func() (*Contract, *Contract) {
			left, right := deltaAliasOperation("alias", 0), deltaAliasOperation("alias", 1)
			return deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{left}}), deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{right}})
		}},
		{"fresh result", func() (*Contract, *Contract) {
			left, right := deltaPlain("fresh"), deltaPlain("fresh")
			left.Outcomes[0].Values.Fixed[0] = testBuiltinTableTop()
			right.Outcomes[0].Values.Fixed[0] = testFunction()
			left.Outcomes[0].FreshResults = []vocabulary.FreshResultSpec{{Result: 0, Kind: schematype.FreshClassTable}}
			right.Outcomes[0].FreshResults = []vocabulary.FreshResultSpec{{Result: 0, Kind: schematype.FreshClassFunction}}
			return deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{left}}), deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{right}})
		}},
		{"Produced capture", func() (*Contract, *Contract) {
			left, right := deltaProduced(0), deltaProduced(1)
			return deltaSeal(t, left), deltaSeal(t, right)
		}},
		{"suspension", func() (*Contract, *Contract) {
			left, right := deltaSuspension(vocabulary.ReentryOnce), deltaSuspension(vocabulary.ReentryMany)
			return deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{left}}), deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{right}})
		}},
		{"resume", func() (*Contract, *Contract) {
			left, right := deltaResume(0), deltaResume(1)
			return deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{left}}), deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{right}})
		}},
		{"transfer", func() (*Contract, *Contract) {
			left, right := deltaTransfer(vocabulary.TransferMayDeliver), deltaTransfer(vocabulary.TransferMayReject)
			return deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{left}}), deltaSeal(t, Spec{Operations: []vocabulary.OperationSpec{right}})
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

func deltaPlain(name string) vocabulary.OperationSpec {
	return vocabulary.OperationSpec{Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{name}}}, Input: vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}}
}

func deltaOpenValues(name string, tail vocabulary.ValuesTail, suffix schematype.Type) vocabulary.OperationSpec {
	return vocabulary.OperationSpec{Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{name}}}, ValuesVars: 1, Input: vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: tail, Var: 0, Suffix: []schematype.Type{suffix}}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}}
}

func deltaCallbackOperation(name string, function, callback uint32, result bool) vocabulary.OperationSpec {
	callbacks := []vocabulary.CallbackSpec{{Function: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: function}, Admission: schematype.CallableAdmissionOrdinary, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: vocabulary.CallbackRetainedOptionalOnce, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}}}
	if result {
		callbacks = []vocabulary.CallbackSpec{{Function: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, Admission: schematype.CallableAdmissionOrdinary, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: vocabulary.CallbackRetainedOptionalOnce, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}}, {Function: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 1}, Admission: schematype.CallableAdmissionOrdinary, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: vocabulary.CallbackRetainedOptionalOnce, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}}}
	}
	outcome := vocabulary.OutcomeSpec{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny, testAny}, Tail: vocabulary.ValuesClosed}}
	if result {
		outcome.CallbackResults = []vocabulary.CallbackResultSpec{{Result: 0, Callback: vocabulary.CallbackRef(callback)}}
	}
	return vocabulary.OperationSpec{Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{name}}}, ValuesVars: 5, Input: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny, testAny}, Tail: vocabulary.ValuesVariable, Var: 0}, Callbacks: callbacks, Outcomes: []vocabulary.OutcomeSpec{outcome}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}}
}

func deltaAliasOperation(name string, ordinal uint32) vocabulary.OperationSpec {
	return vocabulary.OperationSpec{Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{name}}}, Input: vocabulary.ValuesSpec{Fixed: []schematype.Type{testString, testString}, Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testString, testString}, Tail: vocabulary.ValuesClosed}, ResultAliases: []vocabulary.ResultAliasSpec{{Result: 0, Source: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: ordinal}}}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}}
}

func deltaProduced(capture uint32) Spec {
	parent := vocabulary.OperationSpec{Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"produced"}}}, Input: vocabulary.ValuesSpec{Fixed: []schematype.Type{testString, testString}, Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed}, Produced: []vocabulary.ProducedSpec{{Result: 0, Operation: 2, Captures: []vocabulary.CaptureSpec{{Kind: vocabulary.CaptureValueFormal, Ordinal: capture}}}}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}}
	child := vocabulary.OperationSpec{Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}}
	return Spec{Operations: []vocabulary.OperationSpec{parent, child}}
}

func deltaSuspension(multiplicity vocabulary.ReentryMultiplicity) vocabulary.OperationSpec {
	return vocabulary.OperationSpec{Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"suspend"}}}, Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}, {Kind: flowkind.OutcomeYield, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Suspensions: []vocabulary.SuspensionSpec{{Yield: 1, Reentry: 0, Source: vocabulary.ReentryByCall, Multiplicity: multiplicity}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}}
}

func deltaResume(carrier vocabulary.ValueFormal) vocabulary.OperationSpec {
	return vocabulary.OperationSpec{Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"resume"}}}, ValuesVars: 1, Input: vocabulary.ValuesSpec{Fixed: []schematype.Type{testString, testString}, Tail: vocabulary.ValuesVariable, Var: 0}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Resumes: []vocabulary.ResumeSpec{completeResume(vocabulary.ResumeSourceValueFormal, carrier, 0)}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}}
}

func deltaTransfer(normal vocabulary.TransferPossibility) vocabulary.OperationSpec {
	return vocabulary.OperationSpec{Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"transfer"}}}, Input: vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}, {Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Transfers: []vocabulary.TransferSpec{{Endpoint: vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointExternal}, Payload: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, Alias: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, Identity: vocabulary.TransferIdentityUnspecified, Capabilities: vocabulary.TransferCapabilitiesUnspecified, Outcomes: []vocabulary.TransferOutcomeSpec{{Outcome: 0, Possibility: normal}, {Outcome: 1, Possibility: vocabulary.TransferMayReject}}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}}
}

func deltaEffects(target vocabulary.SpecRef) Spec {
	operations := []vocabulary.OperationSpec{deltaPlain("effect-a"), deltaPlain("effect-b"), deltaPlain("effect-c")}
	operations[0].Effects = vocabulary.RowSpec{Occurrences: []vocabulary.EffectSpec{{Target: target, ValueArgs: []vocabulary.ValueFormal{0}}}, Tail: vocabulary.RowClosed}
	return Spec{Operations: operations}
}

func deltaProtocolFinal(final bool) Spec {
	return Spec{
		Operations: []vocabulary.OperationSpec{protocolOperation("protocol-final", nil)},
		Protocols: []vocabulary.ProtocolSpec{{
			Acquisitions: []vocabulary.AcquisitionSpec{{Operation: 1, Outcome: 0, Result: 0, State: 1}},
			States:       []vocabulary.StateSpec{{Name: "state", Final: final}},
		}},
	}
}

func deltaProtocolTransition(swapped bool) Spec {
	toNormal, toThrow := vocabulary.StateRef(2), vocabulary.StateRef(3)
	if swapped {
		toNormal, toThrow = toThrow, toNormal
	}
	op := protocolOperation("protocol-transition", []schematype.Type{testString})
	op.Outcomes = []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed}}, {Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed}}}
	return Spec{
		Operations: []vocabulary.OperationSpec{op},
		Protocols: []vocabulary.ProtocolSpec{{
			Acquisitions: []vocabulary.AcquisitionSpec{{Operation: 1, Outcome: 0, Result: 0, State: 1}},
			States:       []vocabulary.StateSpec{{Name: "root"}, {Name: "normal", Final: true}, {Name: "throw"}},
			Transitions: []vocabulary.TransitionSpec{{
				Operation: 1, Input: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, From: 1,
				Outcomes: []vocabulary.TransitionOutcomeSpec{{Outcome: 0, To: toNormal}, {Outcome: 1, To: toThrow}},
			}},
		}},
	}
}

func deltaProtocolEscape(ordinal uint32) Spec {
	return Spec{
		Operations: []vocabulary.OperationSpec{protocolOperation("protocol-escape", []schematype.Type{testString, testString})},
		Protocols: []vocabulary.ProtocolSpec{{
			Acquisitions: []vocabulary.AcquisitionSpec{{Operation: 1, Outcome: 0, Result: 0, State: 1}},
			States:       []vocabulary.StateSpec{{Name: "state"}},
			Escapes:      []vocabulary.EscapeSpec{{Operation: 1, Input: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: ordinal}}},
		}},
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
