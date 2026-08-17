package target

import (
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func TestCallbackResultsRemapWithOutcomeAndCallbackCanonicalization(t *testing.T) {
	makeOperation := func(callbacks []CallbackSpec, results []CallbackResultSpec) OperationSpec {
		return OperationSpec{
			Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"callback-result"}}},
			ValuesVars: 5,
			Input:      ValuesSpec{Fixed: []schematype.Type{testAny, testAny}, Tail: ValuesVariable, Var: 0},
			Callbacks:  callbacks,
			Outcomes: []OutcomeSpec{{
				Kind:            flowkind.OutcomeNormal,
				Values:          ValuesSpec{Fixed: []schematype.Type{testAny, testAny}, Tail: ValuesClosed},
				CallbackResults: results,
			}},
			Effects: RowSpec{Tail: RowClosed},
		}
	}
	first := mustSeal(t, Spec{Operations: []OperationSpec{makeOperation(
		[]CallbackSpec{
			{Function: InputSource{Kind: InputSourceValueFormal, Ordinal: 1}, Admission: OrdinaryCallable, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: CallbackRetainedOptionalOnce, Effects: RowSpec{Tail: RowClosed}},
			{Function: InputSource{Kind: InputSourceValueFormal, Ordinal: 0}, Admission: OrdinaryCallable, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: CallbackRetainedOptionalOnce, Effects: RowSpec{Tail: RowClosed}},
		},
		[]CallbackResultSpec{{Result: 1, Callback: 1}, {Result: 0, Callback: 2}},
	)}})
	second := mustSeal(t, Spec{Operations: []OperationSpec{makeOperation(
		[]CallbackSpec{
			{Function: InputSource{Kind: InputSourceValueFormal, Ordinal: 0}, Admission: OrdinaryCallable, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: CallbackRetainedOptionalOnce, Effects: RowSpec{Tail: RowClosed}},
			{Function: InputSource{Kind: InputSourceValueFormal, Ordinal: 1}, Admission: OrdinaryCallable, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: CallbackRetainedOptionalOnce, Effects: RowSpec{Tail: RowClosed}},
		},
		[]CallbackResultSpec{{Result: 0, Callback: 1}, {Result: 1, Callback: 2}},
	)}})
	assertPublicContractEqual(t, first, second)

	op, ok := first.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"callback-result"}})
	if !ok || first.CallbackResultCount(op, 0) != 2 {
		t.Fatalf("callback result relation missing: %d/%v", op, ok)
	}
	callback, index, ok := first.CallbackForResult(op, 0, 0)
	if !ok || index != 0 || callback != 1 {
		t.Fatalf("result 0 callback = %d/%d/%v, want 1/0/true", callback, index, ok)
	}
	callback, index, ok = first.CallbackForResult(op, 0, 1)
	if !ok || index != 1 || callback != 2 {
		t.Fatalf("result 1 callback = %d/%d/%v, want 2/1/true", callback, index, ok)
	}
	for _, want := range []struct {
		id     CallbackID
		formal uint32
	}{{id: 1, formal: 0}, {id: 2, formal: 1}} {
		source, found := first.CallbackFunction(want.id)
		if !found || source.Kind != InputSourceValueFormal || source.Ordinal != want.formal {
			t.Fatalf("callback %d source = %#v/%v", want.id, source, found)
		}
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		if _, _, found := first.CallbackForResult(op, 0, 1); !found {
			panic("callback result disappeared")
		}
	}); allocs != 0 {
		t.Fatalf("CallbackForResult allocated %f times", allocs)
	}
}

func TestCallbackResultsRejectInvalidAndDualAuthority(t *testing.T) {
	base := OperationSpec{
		Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"invalid-callback-result"}}},
		ValuesVars: 5,
		Input:      ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesVariable, Var: 0},
		Callbacks:  []CallbackSpec{{Function: InputSource{Kind: InputSourceValueFormal}, Admission: OrdinaryCallable, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: CallbackRetainedOptionalOnce, Effects: RowSpec{Tail: RowClosed}}},
		Outcomes:   []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed}}},
		Effects:    RowSpec{Tail: RowClosed},
	}
	for _, test := range []struct {
		name    string
		results []CallbackResultSpec
	}{
		{name: "tail", results: []CallbackResultSpec{{Result: 1, Callback: 1}}},
		{name: "zero callback", results: []CallbackResultSpec{{Result: 0, Callback: 0}}},
		{name: "unknown callback", results: []CallbackResultSpec{{Result: 0, Callback: 2}}},
		{name: "duplicate result", results: []CallbackResultSpec{{Result: 0, Callback: 1}, {Result: 0, Callback: 1}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			operation := base
			operation.Outcomes = append([]OutcomeSpec(nil), base.Outcomes...)
			operation.Outcomes[0].CallbackResults = test.results
			if _, err := testSeal(&Spec{Operations: []OperationSpec{operation}}); err == nil {
				t.Fatal("invalid callback result accepted")
			}
		})
	}

	overlap := base
	overlap.Outcomes = append([]OutcomeSpec(nil), base.Outcomes...)
	overlap.Outcomes[0].CallbackResults = []CallbackResultSpec{{Result: 0, Callback: 1}}
	overlap.Outcomes[0].Produced = []ProducedSpec{{Result: 0, Operation: 2}}
	producedOnly := OperationSpec{Input: ValuesSpec{Tail: ValuesClosed}, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}}, Effects: RowSpec{Tail: RowClosed}}
	if _, err := testSeal(&Spec{Operations: []OperationSpec{overlap, producedOnly}}); err == nil {
		t.Fatal("callback result/produced overlap accepted")
	}
}

func TestCallbacksRequireValueFormalInputSource(t *testing.T) {
	operation := func(callbacks []CallbackSpec) OperationSpec {
		return OperationSpec{
			Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"callback-input-source"}}},
			ValuesVars: 5,
			Input:      ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesVariable, Var: 0},
			Callbacks:  callbacks,
			Outcomes:   []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}},
			Effects:    RowSpec{Tail: RowClosed},
		}
	}
	contract := mustSeal(t, Spec{Operations: []OperationSpec{operation([]CallbackSpec{{
		Function: InputSource{Kind: InputSourceValueFormal, Ordinal: 0}, Admission: OrdinaryCallable, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: CallbackRetainedOptionalOnce, Effects: RowSpec{Tail: RowClosed},
	}})}})
	op, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"callback-input-source"}})
	if !ok || contract.CallbackCount(op) != 1 {
		t.Fatalf("callback input-source relations missing: %d/%v", op, ok)
	}
	id, found := contract.CallbackAt(op, 0)
	got, foundSource := contract.CallbackFunction(id)
	if !found || !foundSource || got != (InputSource{Kind: InputSourceValueFormal, Ordinal: 0}) {
		t.Fatalf("callback fixed-formal source = %#v/%v/%v", got, found, foundSource)
	}
	for _, source := range []InputSource{
		{Kind: InputSourceValuesVar, Ordinal: 0},
		{Kind: InputSourceAllInputs},
	} {
		if _, err := testSeal(&Spec{Operations: []OperationSpec{operation([]CallbackSpec{{
			Function: source, Admission: OrdinaryCallable, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: CallbackRetainedOptionalOnce, Effects: RowSpec{Tail: RowClosed},
		}})}}); err == nil {
			t.Fatalf("callback accepted non-scalar source %#v", source)
		}
	}
}
