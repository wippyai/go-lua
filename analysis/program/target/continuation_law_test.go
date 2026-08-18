package target

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"testing"
)

func TestSuspensionCanonicalizesOutcomeOrdinalsAndKeepsCallbackValues(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{
		{
			Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"yield"}}},
			ValuesVars: 3,
			Input:      vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0},
			// Authoring order is deliberately Yield, Cancel, Normal, Throw. Seal owns the
			// canonical outcome order and remaps the suspension coordinates.
			Outcomes: []vocabulary.OutcomeSpec{
				{Kind: flowkind.OutcomeYield, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0}},
				{Kind: flowkind.OutcomeCancel, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}},
				{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 1}},
				{Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 2}},
			},
			Suspensions: []vocabulary.SuspensionSpec{
				{Yield: 0, Reentry: 2, Source: vocabulary.ReentryByCall, Multiplicity: vocabulary.ReentryOnce},
				{Yield: 0, Reentry: 3, Source: vocabulary.ReentryByProvider, Multiplicity: vocabulary.ReentryOnce},
				{Yield: 0, Reentry: 1, Source: vocabulary.ReentryByProvider, Multiplicity: vocabulary.ReentryOnce},
			},
			Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		},
		{
			Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"create"}}},
			ValuesVars: 5,
			Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed},
			Callbacks: []vocabulary.CallbackSpec{{
				Function:  vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0},
				Admission: schematype.CallableAdmissionOrdinary, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: vocabulary.CallbackRetainedOptionalOnce,
				Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
			}},
			Outcomes: []vocabulary.OutcomeSpec{{
				Kind:            flowkind.OutcomeNormal,
				Values:          vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed},
				CallbackResults: []vocabulary.CallbackResultSpec{{Result: 0, Callback: 1}},
			}},
			Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		},
		{
			Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"resume"}}},
			ValuesVars: 1,
			Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesVariable, Var: 0},
			Outcomes:   []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0}}},
			Resumes:    []vocabulary.ResumeSpec{completeResume(vocabulary.ResumeSourceValueFormal, 0, 0)},
			Effects:    vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		},
	}})

	yield, _ := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"yield"}})
	if contract.suspensionCount(yield) != 3 {
		t.Fatalf("suspension count = %d, want 3", contract.suspensionCount(yield))
	}
	for index, want := range []struct {
		reentry uint32
		kind    flowkind.OutcomeKind
		source  vocabulary.ReentrySource
	}{
		{reentry: 0, kind: flowkind.OutcomeNormal, source: vocabulary.ReentryByCall},
		{reentry: 1, kind: flowkind.OutcomeThrow, source: vocabulary.ReentryByProvider},
		{reentry: 3, kind: flowkind.OutcomeCancel, source: vocabulary.ReentryByProvider},
	} {
		yieldOutcome, reentryOutcome, source, multiplicity, ok := contract.suspensionAt(yield, index)
		if !ok || yieldOutcome != 2 || reentryOutcome != want.reentry || source != want.source || multiplicity != vocabulary.ReentryOnce {
			t.Fatalf("SuspensionAt(%d) = %d/%d/%d/%d/%v", index, yieldOutcome, reentryOutcome, source, multiplicity, ok)
		}
		if kind, _, ok := contract.OutcomeAt(yield, int(reentryOutcome)); !ok || kind != want.kind {
			t.Fatalf("reentry %d kind = %d/%v, want %d", index, kind, ok, want.kind)
		}
	}

	create, _ := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"create"}})
	callback, _, ok := contract.callbackForResult(create, 0, 0)
	if !ok || callback == 0 {
		t.Fatal("create result has no exact CallbackID")
	}
	if _, ok := contract.CallbackArguments(callback); !ok {
		t.Fatal("sealed callback Arguments correspondence missing")
	}
	for _, kind := range []flowkind.OutcomeKind{flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow, flowkind.OutcomeYield, flowkind.OutcomeCancel} {
		if _, ok := contract.CallbackOutcome(callback, kind); !ok {
			t.Fatalf("sealed callback outcome %d correspondence missing", kind)
		}
	}

	resume, _ := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"resume"}})
	resumeID, _ := contract.ResumeIDAt(resume, 0)
	owner, resumeSource, carrier, arguments, ok := contract.Resume(resumeID)
	tail, variable, tailOK := contract.ValuesTail(arguments)
	if !ok || owner != resume || resumeSource != vocabulary.ResumeSourceValueFormal || carrier != 0 || !tailOK || tail != vocabulary.ValuesVariable || variable != 0 {
		t.Fatalf("Resume = %d/%d/%d/%d/%d/%d/%v/%v, want owner/formal/0/variable/0/true", owner, resumeSource, carrier, arguments, tail, variable, ok, tailOK)
	}
}

func TestProducedResumeUsesCapturedCallbackWithoutCallableShadow(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{
		{
			Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"wrap"}}},
			ValuesVars: 5,
			Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed},
			Callbacks: []vocabulary.CallbackSpec{{
				Function:  vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0},
				Admission: schematype.CallableAdmissionOrdinary, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: vocabulary.CallbackRetainedOptionalOnce,
				Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
			}},
			Outcomes: []vocabulary.OutcomeSpec{{
				Kind:   flowkind.OutcomeNormal,
				Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed},
				Produced: []vocabulary.ProducedSpec{{
					Result: 0, Operation: 2, Captures: []vocabulary.CaptureSpec{{Kind: vocabulary.CaptureCallback, Ordinal: 1}},
				}},
			}},
			Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		},
		{
			ValuesVars: 1,
			Input:      vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0},
			Outcomes:   []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
			Resumes:    []vocabulary.ResumeSpec{completeResume(vocabulary.ResumeSourceProduced, 0, 0)},
			Effects:    vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		},
	}})
	wrap, _ := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"wrap"}})
	invoke, produced, ok := contract.producedForResult(wrap, 0, 0)
	if !ok || invoke == 0 {
		t.Fatal("wrap result did not select one ordinary Produced Operation")
	}
	kind, capture, ok := contract.producedCaptureAt(wrap, 0, produced, 0)
	if !ok || kind != vocabulary.CaptureCallback || vocabulary.CallbackID(capture) == 0 {
		t.Fatal("wrap produced operation lost its retained CallbackID")
	}
	resumeID, _ := contract.ResumeIDAt(invoke, 0)
	owner, source, carrier, arguments, ok := contract.Resume(resumeID)
	tail, variable, tailOK := contract.ValuesTail(arguments)
	if !ok || owner != invoke || source != vocabulary.ResumeSourceProduced || carrier != 0 || !tailOK || tail != vocabulary.ValuesVariable || variable != 0 {
		t.Fatalf("produced Resume = %d/%d/%d/%d/%d/%d/%v/%v", owner, source, carrier, arguments, tail, variable, ok, tailOK)
	}
}

func TestResumeSealsCompleteCanonicalOutcomeCorrespondence(t *testing.T) {
	seal := func(outcomes []vocabulary.OutcomeSpec, mapping []vocabulary.ResumeOutcomeSpec) *Contract {
		return mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{{
			Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"resume-canonical"}}},
			ValuesVars: 1,
			Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesVariable, Var: 0},
			Outcomes:   outcomes,
			Resumes: []vocabulary.ResumeSpec{{
				Source: vocabulary.ResumeSourceValueFormal, Carrier: 0, Arguments: callbackTail(0), Outcomes: mapping,
			}},
			Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}}})
	}
	left := seal(
		[]vocabulary.OutcomeSpec{
			{Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}},
			{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0}},
		},
		[]vocabulary.ResumeOutcomeSpec{
			{Kind: flowkind.OutcomeCancel, Outcome: 0},
			{Kind: flowkind.OutcomeYield, Outcome: 1},
			{Kind: flowkind.OutcomeThrow, Outcome: 0},
			{Kind: flowkind.OutcomeReturn, Outcome: 1},
			{Kind: flowkind.OutcomeNormal, Outcome: 1},
		},
	)
	right := seal(
		[]vocabulary.OutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0}},
			{Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}},
		},
		[]vocabulary.ResumeOutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Outcome: 0},
			{Kind: flowkind.OutcomeReturn, Outcome: 0},
			{Kind: flowkind.OutcomeThrow, Outcome: 1},
			{Kind: flowkind.OutcomeYield, Outcome: 0},
			{Kind: flowkind.OutcomeCancel, Outcome: 1},
		},
	)
	if got, want := publicContractSnapshot(t, left), publicContractSnapshot(t, right); got != want {
		t.Fatalf("resume author permutations changed public contract\nleft: %s\nright: %s", got, want)
	}
	if left.ContentID() != right.ContentID() {
		t.Fatal("resume author permutations changed ContentID")
	}
	op, ok := left.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"resume-canonical"}})
	if !ok {
		t.Fatal("resume operation missing")
	}
	resumeID, ok := left.ResumeIDAt(op, 0)
	if !ok {
		t.Fatal("resume identity missing")
	}
	if count := left.resumeOutcomeCount(resumeID); count != 5 {
		t.Fatalf("ResumeOutcomeCount = %d, want 5", count)
	}
	for index, want := range [...]struct {
		kind    flowkind.OutcomeKind
		outcome uint32
	}{
		{flowkind.OutcomeNormal, 0}, {flowkind.OutcomeReturn, 0}, {flowkind.OutcomeThrow, 1},
		{flowkind.OutcomeYield, 0}, {flowkind.OutcomeCancel, 1},
	} {
		kind, outcome, found := left.resumeOutcomeAt(resumeID, index)
		if !found || kind != want.kind || outcome != want.outcome {
			t.Fatalf("ResumeOutcomeAt(%d) = %d/%d/%v, want %d/%d/true", index, kind, outcome, found, want.kind, want.outcome)
		}
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		if _, _, ok := left.resumeOutcomeAt(resumeID, 4); !ok {
			panic("resume mapping disappeared")
		}
	}); allocs != 0 {
		t.Fatalf("ResumeOutcomeAt allocated %f times", allocs)
	}
}

func TestResumeIDsCanonicalizePermutationAndRoundTrip(t *testing.T) {
	operation := func(resumes []vocabulary.ResumeSpec) vocabulary.OperationSpec {
		return vocabulary.OperationSpec{
			Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"resume-id"}}},
			ValuesVars: 1,
			Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny, testAny}, Tail: vocabulary.ValuesVariable, Var: 0},
			Outcomes:   []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0}}},
			Resumes:    resumes,
			Effects:    vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}
	}
	left := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{operation([]vocabulary.ResumeSpec{
		completeResume(vocabulary.ResumeSourceValueFormal, 1, 0),
		completeResume(vocabulary.ResumeSourceValueFormal, 0, 0),
	})}})
	right := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{operation([]vocabulary.ResumeSpec{
		completeResume(vocabulary.ResumeSourceValueFormal, 0, 0),
		completeResume(vocabulary.ResumeSourceValueFormal, 1, 0),
	})}})
	if got, want := publicContractSnapshot(t, left), publicContractSnapshot(t, right); got != want {
		t.Fatalf("resume permutation changed public contract\nleft: %s\nright: %s", got, want)
	}
	if left.ContentID() != right.ContentID() {
		t.Fatal("resume permutation changed ContentID")
	}
	op, ok := left.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"resume-id"}})
	if !ok || left.ResumeCount(op) != 2 {
		t.Fatalf("resume identities missing: %d/%v", op, ok)
	}
	for index, wantCarrier := range [...]vocabulary.ValueFormal{0, 1} {
		id, found := left.ResumeIDAt(op, index)
		owner, source, carrier, arguments, detailsFound := left.Resume(id)
		idAgain, foundAgain := left.ResumeIDAt(owner, index)
		tail, variable, tailOK := left.ValuesTail(arguments)
		if !found || !detailsFound || !foundAgain || id == 0 || idAgain != id || owner != op ||
			source != vocabulary.ResumeSourceValueFormal || carrier != wantCarrier || !tailOK || tail != vocabulary.ValuesVariable || variable != 0 {
			t.Fatalf("resume %d = id:%d owner:%d source:%d carrier:%d args:%d tail:%d/%d found:%v/%v/%v/%v", index, id, owner, source, carrier, arguments, tail, variable, found, detailsFound, foundAgain, tailOK)
		}
	}
	if _, _, _, _, ok := left.Resume(0); ok {
		t.Fatal("zero ResumeID resolved")
	}
	if _, ok := left.ResumeIDAt(op, 2); ok {
		t.Fatal("out-of-range resume ordinal resolved")
	}
}

func TestResumeRejectsIncompleteInvalidAndDuplicateOutcomeAuthority(t *testing.T) {
	base := vocabulary.OperationSpec{
		Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"bad-resume"}}},
		ValuesVars: 1,
		Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesVariable, Var: 0},
		Outcomes: []vocabulary.OutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}},
			{Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}},
		},
		Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
	valid := completeResume(vocabulary.ResumeSourceValueFormal, 0, 0)
	for _, test := range []struct {
		name   string
		input  vocabulary.ValuesSpec
		vars   uint32
		out    []vocabulary.OutcomeSpec
		resume vocabulary.ResumeSpec
	}{
		{"arguments outside scope", base.Input, base.ValuesVars, base.Outcomes, completeResume(vocabulary.ResumeSourceValueFormal, 0, 1)},
		{"arguments must be input tail", vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed}, 1, base.Outcomes, completeResume(vocabulary.ResumeSourceValueFormal, 0, 0)},
		{"different input tail variable", vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesVariable, Var: 1}, 2, base.Outcomes, completeResume(vocabulary.ResumeSourceValueFormal, 0, 0)},
		{"unknown input tail", vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesUnknown}, 1, base.Outcomes, completeResume(vocabulary.ResumeSourceValueFormal, 0, 0)},
		{"missing kind", base.Input, base.ValuesVars, base.Outcomes, vocabulary.ResumeSpec{Source: vocabulary.ResumeSourceValueFormal, Carrier: 0, Arguments: callbackTail(0), Outcomes: valid.Outcomes[:4]}},
		{"duplicate kind", base.Input, base.ValuesVars, base.Outcomes, vocabulary.ResumeSpec{Source: vocabulary.ResumeSourceValueFormal, Carrier: 0, Arguments: callbackTail(0), Outcomes: append(append([]vocabulary.ResumeOutcomeSpec(nil), valid.Outcomes[:4]...), vocabulary.ResumeOutcomeSpec{Kind: flowkind.OutcomeNormal, Outcome: 0})}},
		{"break cannot cross activation", base.Input, base.ValuesVars, base.Outcomes, vocabulary.ResumeSpec{Source: vocabulary.ResumeSourceValueFormal, Carrier: 0, Arguments: callbackTail(0), Outcomes: append(append([]vocabulary.ResumeOutcomeSpec(nil), valid.Outcomes[:4]...), vocabulary.ResumeOutcomeSpec{Kind: flowkind.OutcomeBreak, Outcome: 0})}},
		{"outcome outside scope", base.Input, base.ValuesVars, base.Outcomes, vocabulary.ResumeSpec{Source: vocabulary.ResumeSourceValueFormal, Carrier: 0, Arguments: callbackTail(0), Outcomes: []vocabulary.ResumeOutcomeSpec{{Kind: flowkind.OutcomeNormal, Outcome: 2}, {Kind: flowkind.OutcomeReturn, Outcome: 0}, {Kind: flowkind.OutcomeThrow, Outcome: 1}, {Kind: flowkind.OutcomeYield, Outcome: 0}, {Kind: flowkind.OutcomeCancel, Outcome: 1}}}},
		{"unknown mapped outcome", base.Input, base.ValuesVars, []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesUnknown}}, {Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, completeResume(vocabulary.ResumeSourceValueFormal, 0, 0)},
	} {
		t.Run(test.name, func(t *testing.T) {
			op := base
			op.Input = test.input
			op.ValuesVars = test.vars
			op.Outcomes = test.out
			op.Resumes = []vocabulary.ResumeSpec{test.resume}
			if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{op}}); err == nil {
				t.Fatal("Seal accepted invalid resume outcome authority")
			}
		})
	}
}

func TestResumePayloadTransportUsesOnlyExistingValuesRelations(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{
		{
			Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"transport-yield"}}},
			ValuesVars: 1,
			Input:      vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0},
			Outcomes: []vocabulary.OutcomeSpec{
				{Kind: flowkind.OutcomeYield, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0}},
				{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0}},
			},
			Suspensions: []vocabulary.SuspensionSpec{{Yield: 0, Reentry: 1, Source: vocabulary.ReentryByCall, Multiplicity: vocabulary.ReentryOnce}},
			Effects:     vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		},
		{
			Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"transport-resume"}}},
			ValuesVars: 2,
			Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesVariable, Var: 0},
			Outcomes: []vocabulary.OutcomeSpec{
				{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 1}},
				{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testLiteralBool(false)}, Tail: vocabulary.ValuesClosed}},
			},
			Resumes: []vocabulary.ResumeSpec{{
				Source: vocabulary.ResumeSourceValueFormal, Carrier: 0, Arguments: callbackTail(0),
				Outcomes: []vocabulary.ResumeOutcomeSpec{
					{Kind: flowkind.OutcomeNormal, Outcome: 0}, {Kind: flowkind.OutcomeReturn, Outcome: 0},
					{Kind: flowkind.OutcomeThrow, Outcome: 1}, {Kind: flowkind.OutcomeYield, Outcome: 0},
					{Kind: flowkind.OutcomeCancel, Outcome: 1},
				},
			}},
			Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		},
	}})
	yield, _ := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"transport-yield"}})
	resume, _ := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"transport-resume"}})
	_, reentry, _, _, ok := contract.suspensionAt(yield, 0)
	if !ok {
		t.Fatal("suspension missing")
	}
	_, reentryValues, ok := contract.OutcomeAt(yield, int(reentry))
	if !ok {
		t.Fatal("reentry outcome missing")
	}
	if tail, variable, ok := contract.ValuesTail(reentryValues); !ok || tail != vocabulary.ValuesVariable || variable != 0 {
		t.Fatalf("reentry Values = %d/%d/%v, want variable/0/true", tail, variable, ok)
	}
	resumeID, _ := contract.ResumeIDAt(resume, 0)
	owner, _, _, arguments, ok := contract.Resume(resumeID)
	tail, variable, tailOK := contract.ValuesTail(arguments)
	if !ok || owner != resume || !tailOK || tail != vocabulary.ValuesVariable || variable != 0 {
		t.Fatalf("resume arguments = %d/%d/%d/%v/%v, want variable/0/true", arguments, tail, variable, ok, tailOK)
	}
	for _, item := range []struct {
		index int
		tail  vocabulary.ValuesTail
		varID vocabulary.ValuesVar
	}{
		{0, vocabulary.ValuesVariable, 1}, // Normal transports into the existing result tail.
		{2, vocabulary.ValuesClosed, 0},   // Throw selects the existing closed failure and discards payload.
	} {
		_, outcome, ok := contract.resumeOutcomeAt(resumeID, item.index)
		if !ok {
			t.Fatalf("resume outcome %d missing", item.index)
		}
		_, values, ok := contract.OutcomeAt(resume, int(outcome))
		if !ok {
			t.Fatalf("mapped operation outcome %d missing", outcome)
		}
		if tail, variable, ok := contract.ValuesTail(values); !ok || tail != item.tail || variable != item.varID {
			t.Fatalf("mapped outcome %d Values = %d/%d/%v, want %d/%d/true", item.index, tail, variable, ok, item.tail, item.varID)
		}
	}
}

func TestSuspensionAuthorPermutationHasOnePublicContract(t *testing.T) {
	operation := func(outcomes []vocabulary.OutcomeSpec, suspensions []vocabulary.SuspensionSpec) *Contract {
		return mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{{
			Bindings:    []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"permutation"}}},
			ValuesVars:  2,
			Input:       vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0},
			Outcomes:    outcomes,
			Suspensions: suspensions,
			Effects:     vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}}})
	}
	left := operation(
		[]vocabulary.OutcomeSpec{
			{Kind: flowkind.OutcomeYield, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0}},
			{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 1}},
			{Kind: flowkind.OutcomeCancel, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}},
		},
		[]vocabulary.SuspensionSpec{
			{Yield: 0, Reentry: 1, Source: vocabulary.ReentryByCall, Multiplicity: vocabulary.ReentryOnce},
			{Yield: 0, Reentry: 2, Source: vocabulary.ReentryByProvider, Multiplicity: vocabulary.ReentryOnce},
		},
	)
	right := operation(
		[]vocabulary.OutcomeSpec{
			{Kind: flowkind.OutcomeCancel, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}},
			{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 1}},
			{Kind: flowkind.OutcomeYield, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0}},
		},
		[]vocabulary.SuspensionSpec{
			{Yield: 2, Reentry: 1, Source: vocabulary.ReentryByCall, Multiplicity: vocabulary.ReentryOnce},
			{Yield: 2, Reentry: 0, Source: vocabulary.ReentryByProvider, Multiplicity: vocabulary.ReentryOnce},
		},
	)
	if leftPublic, rightPublic := publicContractSnapshot(t, left), publicContractSnapshot(t, right); leftPublic != rightPublic {
		t.Fatalf("outcome/suspension author permutation changed public contract\nleft: %s\nright: %s", leftPublic, rightPublic)
	}
}

func TestSuspensionRejectsInvalidAuthority(t *testing.T) {
	base := vocabulary.OperationSpec{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"bad"}}},
		Input:    vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}},
			{Kind: flowkind.OutcomeYield, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}},
		},
		Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
	for _, test := range []struct {
		name       string
		suspension []vocabulary.SuspensionSpec
		resumes    []vocabulary.ResumeSpec
	}{
		{name: "normal is not yield", suspension: []vocabulary.SuspensionSpec{{Yield: 0, Reentry: 0, Source: vocabulary.ReentryByCall, Multiplicity: vocabulary.ReentryOnce}}},
		{name: "yield is not reentry", suspension: []vocabulary.SuspensionSpec{{Yield: 1, Reentry: 1, Source: vocabulary.ReentryByCall, Multiplicity: vocabulary.ReentryOnce}}},
		{name: "invalid source", suspension: []vocabulary.SuspensionSpec{{Yield: 1, Reentry: 0, Source: vocabulary.ReentrySourceInvalid, Multiplicity: vocabulary.ReentryOnce}}},
		{name: "invalid multiplicity", suspension: []vocabulary.SuspensionSpec{{Yield: 1, Reentry: 0, Source: vocabulary.ReentryByCall, Multiplicity: vocabulary.ReentryMultiplicityInvalid}}},
		{name: "same authority different multiplicity", suspension: []vocabulary.SuspensionSpec{
			{Yield: 1, Reentry: 0, Source: vocabulary.ReentryByCall, Multiplicity: vocabulary.ReentryOnce},
			{Yield: 1, Reentry: 0, Source: vocabulary.ReentryByCall, Multiplicity: vocabulary.ReentryMany},
		}},
		{name: "duplicate exact suspension", suspension: []vocabulary.SuspensionSpec{
			{Yield: 1, Reentry: 0, Source: vocabulary.ReentryByCall, Multiplicity: vocabulary.ReentryOnce},
			{Yield: 1, Reentry: 0, Source: vocabulary.ReentryByCall, Multiplicity: vocabulary.ReentryOnce},
		}},
		{name: "formal outside scope", resumes: []vocabulary.ResumeSpec{completeResume(vocabulary.ResumeSourceValueFormal, 1, 0)}},
		{name: "invalid resume source", resumes: []vocabulary.ResumeSpec{completeResume(vocabulary.ResumeSourceInvalid, 0, 0)}},
		{name: "duplicate exact resume", resumes: []vocabulary.ResumeSpec{
			completeResume(vocabulary.ResumeSourceValueFormal, 0, 0),
			completeResume(vocabulary.ResumeSourceValueFormal, 0, 0),
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			operation := base
			operation.Suspensions = test.suspension
			operation.Resumes = test.resumes
			if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{operation}}); err == nil {
				t.Fatal("invalid suspension authority accepted")
			}
		})
	}
}

func TestOpaqueSuspensionIsDerivedAndQueriesAllocateNothing(t *testing.T) {
	contract := mustSeal(t, Spec{})
	opaque, _ := contract.Opaque()
	if contract.suspensionCount(opaque) != 3 || contract.ResumeCount(opaque) != 0 {
		t.Fatal("opaque suspension was not derived exactly")
	}
	for index, wantReentry := range []uint32{0, 1, 3} {
		yield, reentry, source, multiplicity, ok := contract.suspensionAt(opaque, index)
		if !ok || yield != 2 || reentry != wantReentry || source != vocabulary.ReentryByProvider || multiplicity != vocabulary.ReentryMany {
			t.Fatalf("opaque suspension %d = %d/%d/%d/%d/%v", index, yield, reentry, source, multiplicity, ok)
		}
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		if _, _, _, _, ok := contract.suspensionAt(opaque, 2); !ok {
			panic("opaque suspension disappeared")
		}
	}); allocs != 0 {
		t.Fatalf("opaque SuspensionAt allocated %f times", allocs)
	}
}

func TestWideProducedResumesSealWithoutQuadraticValidation(t *testing.T) {
	const width = 4096
	values := make([]schematype.Type, width)
	for index := range values {
		values[index] = testAny
	}
	operations := make([]vocabulary.OperationSpec, width+1)
	operations[0] = vocabulary.OperationSpec{
		Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"wide"}}},
		ValuesVars: 5,
		Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed},
		Callbacks: []vocabulary.CallbackSpec{{
			Function:  vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0},
			Admission: schematype.CallableAdmissionOrdinary, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: vocabulary.CallbackRetainedOptionalOnce,
			Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}},
		Outcomes: []vocabulary.OutcomeSpec{{
			Kind:     flowkind.OutcomeNormal,
			Values:   vocabulary.ValuesSpec{Fixed: values, Tail: vocabulary.ValuesClosed},
			Produced: make([]vocabulary.ProducedSpec, width),
		}},
		Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
	for index := 0; index < width; index++ {
		operations[0].Outcomes[0].Produced[index] = vocabulary.ProducedSpec{
			Result: uint32(index), Operation: vocabulary.SpecRef(index + 2), Captures: []vocabulary.CaptureSpec{{Kind: vocabulary.CaptureCallback, Ordinal: 1}},
		}
		operations[index+1] = vocabulary.OperationSpec{
			ValuesVars: 1,
			Input:      vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0},
			Outcomes:   []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
			Resumes:    []vocabulary.ResumeSpec{completeResume(vocabulary.ResumeSourceProduced, 0, 0)},
			Effects:    vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}
	}
	contract := mustSeal(t, Spec{Operations: operations})
	if contract.OperationCount() != width+2 {
		t.Fatalf("operation count = %d, want %d", contract.OperationCount(), width+2)
	}
	root, _ := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"wide"}})
	last, _, ok := contract.producedForResult(root, 0, width-1)
	if !ok || contract.ResumeCount(last) != 1 {
		t.Fatal("wide produced resume was not retained")
	}
}

func TestDeepProducedResumesSealIteratively(t *testing.T) {
	const depth = 1024
	operations := make([]vocabulary.OperationSpec, depth)
	for index := range operations {
		operation := vocabulary.OperationSpec{
			ValuesVars: 5,
			Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesVariable, Var: 0},
			Callbacks: []vocabulary.CallbackSpec{{
				Function:  vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0},
				Admission: schematype.CallableAdmissionOrdinary, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: vocabulary.CallbackRetainedOptionalOnce,
				Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
			}},
			Outcomes: []vocabulary.OutcomeSpec{{
				Kind:   flowkind.OutcomeNormal,
				Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed},
			}},
			Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}
		if index == 0 {
			operation.Bindings = []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"deep-resume"}}}
		} else {
			operation.Resumes = []vocabulary.ResumeSpec{completeResume(vocabulary.ResumeSourceProduced, 0, 0)}
		}
		if index+1 < depth {
			operation.Outcomes[0].Produced = []vocabulary.ProducedSpec{{
				Result: 0, Operation: vocabulary.SpecRef(index + 2), Captures: []vocabulary.CaptureSpec{{Kind: vocabulary.CaptureCallback, Ordinal: 1}},
			}}
		}
		operations[index] = operation
	}
	contract := mustSeal(t, Spec{Operations: operations})
	current, _ := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"deep-resume"}})
	for index := 1; index < depth; index++ {
		next, _, ok := contract.producedForResult(current, 0, 0)
		if !ok {
			t.Fatalf("deep produced resume ended at %d", index)
		}
		current = next
	}
	resumeID, _ := contract.ResumeIDAt(current, 0)
	_, source, _, _, ok := contract.Resume(resumeID)
	if !ok || source != vocabulary.ResumeSourceProduced {
		t.Fatal("deep terminal produced resume missing")
	}
}

func completeResume(source vocabulary.ResumeSource, carrier vocabulary.ValueFormal, arguments vocabulary.ValuesVar) vocabulary.ResumeSpec {
	return vocabulary.ResumeSpec{
		Source: source, Carrier: carrier, Arguments: callbackTail(arguments),
		Outcomes: []vocabulary.ResumeOutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Outcome: 0},
			{Kind: flowkind.OutcomeReturn, Outcome: 0},
			{Kind: flowkind.OutcomeThrow, Outcome: 0},
			{Kind: flowkind.OutcomeYield, Outcome: 0},
			{Kind: flowkind.OutcomeCancel, Outcome: 0},
		},
	}
}

func spawnTestOperation(name string) vocabulary.OperationSpec {
	return vocabulary.OperationSpec{
		Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingModule, Owner: []string{"coroutine"}, Member: []string{name}}},
		ValuesVars: 7,
		Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesVariable, Var: 0},
		Callbacks: []vocabulary.CallbackSpec{{
			Function: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, Admission: schematype.CallableAdmissionOrdinary, Arguments: callbackTail(1),
			Outcomes: callbackOutcomes(2, 3, 4, 5, 6), Lifecycle: vocabulary.CallbackRetainedRequiredOnce, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}},
		Outcomes: []vocabulary.OutcomeSpec{
			{Kind: flowkind.OutcomeYield, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}},
			{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}},
			{Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed}},
		},
		Suspensions: []vocabulary.SuspensionSpec{{Yield: 0, Reentry: 1, Source: vocabulary.ReentryByProvider, Multiplicity: vocabulary.ReentryOnce}},
		Spawns: []vocabulary.SpawnSpec{{
			Function: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, Child: 1, Yield: 0, ParentResume: 1, ChildEntry: 1,
			Alternatives: []vocabulary.SpawnSiblingAlternative{vocabulary.SpawnChildEntryThenParentResume, vocabulary.SpawnParentResumeThenChildEntry},
		}},
		Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
}

func TestSpawnSealsOneTypedDetachedAuthority(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{spawnTestOperation("spawn")}})
	op, ok := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingModule, Owner: []string{"coroutine"}, Member: []string{"spawn"}})
	if !ok || contract.spawnCount(op) != 1 {
		t.Fatalf("spawn authority = %v/%d", ok, contract.spawnCount(op))
	}
	id, _ := contract.spawnIDAt(op, 0)
	owner, function, child, yield, resume, entry, resumed, ok := contract.spawnRelation(id)
	if !ok || owner != op || function != (vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}) || yield == resume || entry != resumed {
		t.Fatalf("spawn relation = %#v/%#v/%d/%d/%d/%d/%d/%v", owner, function, child, yield, resume, entry, resumed, ok)
	}
	if childOwner, found := contract.CallbackOwner(child); !found || childOwner != op {
		t.Fatalf("child owner = %d/%v", childOwner, found)
	}
	for _, kind := range []flowkind.OutcomeKind{flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow, flowkind.OutcomeYield, flowkind.OutcomeCancel} {
		if _, found := contract.CallbackOutcome(child, kind); !found {
			t.Fatalf("child lacks total %v outcome", kind)
		}
	}
	if count := contract.spawnSiblingCount(id); count != 2 {
		t.Fatalf("sibling alternatives = %d", count)
	}
	first, firstOK := contract.spawnSiblingAt(id, 0)
	second, secondOK := contract.spawnSiblingAt(id, 1)
	if !firstOK || !secondOK || first == second {
		t.Fatalf("sibling alternatives = %d/%d/%v/%v", first, second, firstOK, secondOK)
	}
}

func TestSpawnRejectsIncompleteAndDuplicateAuthority(t *testing.T) {
	bad := spawnTestOperation("bad")
	bad.Spawns[0].Alternatives = bad.Spawns[0].Alternatives[:1]
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{bad}}); err == nil {
		t.Fatal("incomplete sibling alternatives sealed")
	}
	left, right := spawnTestOperation("left"), spawnTestOperation("right")
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{left, right}}); err == nil {
		t.Fatal("duplicate spawn authority sealed")
	}
}

func TestSpawnSiblingOrderCanonicalizesContentIdentity(t *testing.T) {
	left := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{spawnTestOperation("spawn")}})
	rightSpec := Spec{Operations: []vocabulary.OperationSpec{spawnTestOperation("spawn")}}
	rightSpec.Operations[0].Spawns[0].Alternatives[0], rightSpec.Operations[0].Spawns[0].Alternatives[1] = rightSpec.Operations[0].Spawns[0].Alternatives[1], rightSpec.Operations[0].Spawns[0].Alternatives[0]
	right := mustSeal(t, rightSpec)
	if left.ContentID() != right.ContentID() {
		t.Fatal("sibling alternative authoring order changed content identity")
	}
}

func TestTransferCanonicalizesEndpointPayloadAndOutcomes(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{transferOperation(
		[]vocabulary.OutcomeSpec{
			{Kind: flowkind.OutcomeYield, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0}},
			{Kind: flowkind.OutcomeCancel, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}},
			{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testBoolean}, Tail: vocabulary.ValuesClosed}},
			{Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed}},
		},
		[]vocabulary.TransferSpec{
			transfer(vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointExternal}, vocabulary.InputSource{Kind: vocabulary.InputSourceValuesVar}, vocabulary.TransferIdentityUnspecified, vocabulary.TransferCapabilitiesLoseAll,
				[]vocabulary.TransferOutcomeSpec{{Outcome: 1, Possibility: vocabulary.TransferMayReject}, {Outcome: 3, Possibility: vocabulary.TransferMayDeliver | vocabulary.TransferMayReject}, {Outcome: 0, Possibility: vocabulary.TransferMayDeliver}, {Outcome: 2, Possibility: vocabulary.TransferMayDeliver}}),
			transfer(vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointInput, Input: 1}, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0}, vocabulary.TransferIdentitySame, vocabulary.TransferCapabilitiesPreserveAll,
				[]vocabulary.TransferOutcomeSpec{{Outcome: 3, Possibility: vocabulary.TransferMayReject}, {Outcome: 0, Possibility: vocabulary.TransferMayDeliver}, {Outcome: 2, Possibility: vocabulary.TransferMayDeliver | vocabulary.TransferMayReject}, {Outcome: 1, Possibility: vocabulary.TransferMayReject}}),
		},
	)}})
	op, ok := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"transfer"}})
	if !ok || contract.transferCount(op) != 2 {
		t.Fatalf("transfer operation/count = %d/%v/%d", op, ok, contract.transferCount(op))
	}
	want := []struct {
		endpoint     vocabulary.TransferEndpoint
		payload      vocabulary.InputSource
		alias        vocabulary.InputSource
		identity     vocabulary.TransferIdentity
		capabilities vocabulary.TransferCapabilities
		masks        []vocabulary.TransferPossibility
	}{
		{vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointInput, Input: 1}, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, vocabulary.TransferIdentitySame, vocabulary.TransferCapabilitiesPreserveAll, []vocabulary.TransferPossibility{vocabulary.TransferMayDeliver | vocabulary.TransferMayReject, vocabulary.TransferMayReject, vocabulary.TransferMayDeliver, vocabulary.TransferMayReject}},
		{vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointExternal}, vocabulary.InputSource{Kind: vocabulary.InputSourceValuesVar}, vocabulary.InputSource{Kind: vocabulary.InputSourceValuesVar}, vocabulary.TransferIdentityUnspecified, vocabulary.TransferCapabilitiesLoseAll, []vocabulary.TransferPossibility{vocabulary.TransferMayDeliver, vocabulary.TransferMayDeliver | vocabulary.TransferMayReject, vocabulary.TransferMayDeliver, vocabulary.TransferMayReject}},
	}
	for index, expected := range want {
		id, idOK := contract.transferIDAt(op, index)
		owner, ownerOK := contract.transferOwner(id)
		declaredEndpoint, declaredPayload, declaredAlias, declaredIdentity, declaredCapabilities, declarationOK := contract.transferDeclaration(id)
		if !idOK || id == 0 || !ownerOK || owner != op || !declarationOK || declaredEndpoint != expected.endpoint || declaredPayload != expected.payload || declaredAlias != expected.alias || declaredIdentity != expected.identity || declaredCapabilities != expected.capabilities {
			t.Fatalf("sealed transfer identity %d did not preserve its exact declaration", index)
		}
		endpoint, endpointOK := contract.transferEndpointAt(op, index)
		payload, payloadOK := contract.transferPayloadAt(op, index)
		alias, aliasOK := contract.transferAliasAt(op, index)
		identity, identityOK := contract.transferIdentityAt(op, index)
		capabilities, capabilitiesOK := contract.transferCapabilitiesAt(op, index)
		if !endpointOK || endpoint != expected.endpoint || !payloadOK || payload != expected.payload || !aliasOK || alias != expected.alias ||
			!identityOK || identity != expected.identity || !capabilitiesOK || capabilities != expected.capabilities {
			t.Fatalf("transfer %d = endpoint:%#v/%v payload:%#v/%v identity:%d/%v capabilities:%d/%v", index, endpoint, endpointOK, payload, payloadOK, identity, identityOK, capabilities, capabilitiesOK)
		}
		for outcome, mask := range expected.masks {
			declaredOrdinal, declaredMask, declaredFound := contract.transferDeclarationOutcomeAt(id, outcome)
			if !declaredFound || declaredOrdinal != uint32(outcome) || declaredMask != mask {
				t.Fatalf("sealed transfer identity outcome %d/%d lost declaration", index, outcome)
			}
			ordinal, got, found := contract.transferOutcomeAt(op, index, outcome)
			if !found || ordinal != uint32(outcome) || got != mask {
				t.Fatalf("transfer outcome %d/%d = %d/%d/%v", index, outcome, ordinal, got, found)
			}
		}
	}
}

func TestTransferAuthorPermutationHasOnePublicContract(t *testing.T) {
	leftOutcomes := []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeYield, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0}}, {Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}, {Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed}}}
	rightOutcomes := []vocabulary.OutcomeSpec{leftOutcomes[2], leftOutcomes[0], leftOutcomes[1]}
	left := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{transferOperation(leftOutcomes, []vocabulary.TransferSpec{
		transfer(vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointExternal}, vocabulary.InputSource{Kind: vocabulary.InputSourceValuesVar}, vocabulary.TransferIdentityDistinct, vocabulary.TransferCapabilitiesLoseAll, []vocabulary.TransferOutcomeSpec{{Outcome: 2, Possibility: vocabulary.TransferMayReject}, {Outcome: 0, Possibility: vocabulary.TransferMayDeliver}, {Outcome: 1, Possibility: vocabulary.TransferMayDeliver | vocabulary.TransferMayReject}}),
		transfer(vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointInput}, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, vocabulary.TransferIdentitySame, vocabulary.TransferCapabilitiesPreserveAll, []vocabulary.TransferOutcomeSpec{{Outcome: 0, Possibility: vocabulary.TransferMayDeliver}, {Outcome: 1, Possibility: vocabulary.TransferMayDeliver}, {Outcome: 2, Possibility: vocabulary.TransferMayReject}}),
	})}})
	right := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{transferOperation(rightOutcomes, []vocabulary.TransferSpec{
		transfer(vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointInput}, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, vocabulary.TransferIdentitySame, vocabulary.TransferCapabilitiesPreserveAll, []vocabulary.TransferOutcomeSpec{{Outcome: 0, Possibility: vocabulary.TransferMayReject}, {Outcome: 2, Possibility: vocabulary.TransferMayDeliver}, {Outcome: 1, Possibility: vocabulary.TransferMayDeliver}}),
		transfer(vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointExternal}, vocabulary.InputSource{Kind: vocabulary.InputSourceValuesVar}, vocabulary.TransferIdentityDistinct, vocabulary.TransferCapabilitiesLoseAll, []vocabulary.TransferOutcomeSpec{{Outcome: 1, Possibility: vocabulary.TransferMayDeliver}, {Outcome: 0, Possibility: vocabulary.TransferMayReject}, {Outcome: 2, Possibility: vocabulary.TransferMayDeliver | vocabulary.TransferMayReject}}),
	})}})
	assertPublicContractEqual(t, left, right)
	if left.ContentID() != right.ContentID() {
		t.Fatal("outcome/transfer author permutation changed ContentID")
	}
}

func TestTransferAliasIsCanonicalDeclarationAndContentAuthority(t *testing.T) {
	outcomes := []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}, {Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}
	base := transfer(vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointExternal}, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, vocabulary.TransferIdentitySame, vocabulary.TransferCapabilitiesPreserveAll,
		[]vocabulary.TransferOutcomeSpec{{Outcome: 0, Possibility: vocabulary.TransferMayDeliver}, {Outcome: 1, Possibility: vocabulary.TransferMayReject}})
	other := base
	other.Alias = vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 1}
	left := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{transferOperation(outcomes, []vocabulary.TransferSpec{base})}})
	right := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{transferOperation(outcomes, []vocabulary.TransferSpec{other})}})
	if left.ContentID() == right.ContentID() {
		t.Fatal("transfer alias did not affect target ContentID")
	}
	op, ok := right.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"transfer"}})
	if !ok {
		t.Fatal("transfer operation")
	}
	alias, ok := right.transferAliasAt(op, 0)
	if !ok || alias != other.Alias {
		t.Fatal("transfer alias lost canonical declaration")
	}
}

func TestTransferRejectsIncompleteOrInvalidAuthority(t *testing.T) {
	base := []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}, {Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}
	valid := []vocabulary.TransferOutcomeSpec{{Outcome: 0, Possibility: vocabulary.TransferMayDeliver}, {Outcome: 1, Possibility: vocabulary.TransferMayReject}}
	baseTransfer := func() vocabulary.TransferSpec {
		return transfer(vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointExternal}, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, vocabulary.TransferIdentityUnspecified, vocabulary.TransferCapabilitiesUnspecified, valid)
	}
	tests := []struct {
		name string
		edit func(*vocabulary.TransferSpec)
	}{
		{"invalid endpoint", func(spec *vocabulary.TransferSpec) { spec.Endpoint.Kind = vocabulary.TransferEndpointInvalid }},
		{"external endpoint carries input", func(spec *vocabulary.TransferSpec) { spec.Endpoint.Input = 1 }},
		{"input endpoint outside scope", func(spec *vocabulary.TransferSpec) {
			spec.Endpoint = vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointInput, Input: 2}
		}},
		{"invalid payload", func(spec *vocabulary.TransferSpec) {
			spec.Payload = vocabulary.InputSource{Kind: vocabulary.InputSourceAllInputs}
		}},
		{"invalid alias", func(spec *vocabulary.TransferSpec) {
			spec.Alias = vocabulary.InputSource{Kind: vocabulary.InputSourceAllInputs}
		}},
		{"non-input Values alias", func(spec *vocabulary.TransferSpec) {
			spec.Alias = vocabulary.InputSource{Kind: vocabulary.InputSourceValuesVar, Ordinal: 1}
		}},
		{"invalid identity", func(spec *vocabulary.TransferSpec) { spec.Identity = vocabulary.TransferIdentityInvalid }},
		{"invalid capabilities", func(spec *vocabulary.TransferSpec) { spec.Capabilities = vocabulary.TransferCapabilitiesInvalid }},
		{"incomplete outcomes", func(spec *vocabulary.TransferSpec) { spec.Outcomes = valid[:1] }},
		{"duplicate outcomes", func(spec *vocabulary.TransferSpec) {
			spec.Outcomes = []vocabulary.TransferOutcomeSpec{{Outcome: 0, Possibility: vocabulary.TransferMayDeliver}, {Outcome: 0, Possibility: vocabulary.TransferMayReject}}
		}},
		{"unknown possibility", func(spec *vocabulary.TransferSpec) { spec.Outcomes[0].Possibility = 4 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := baseTransfer()
			test.edit(&spec)
			if contract, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{transferOperation(base, []vocabulary.TransferSpec{spec})}}); err == nil || contract != nil {
				t.Fatal("invalid transfer was published")
			}
		})
	}
	first, second := baseTransfer(), baseTransfer()
	second.Outcomes = []vocabulary.TransferOutcomeSpec{{Outcome: 0, Possibility: vocabulary.TransferMayReject}, {Outcome: 1, Possibility: vocabulary.TransferMayDeliver}}
	if contract, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{transferOperation(base, []vocabulary.TransferSpec{first, second})}}); err == nil || contract != nil {
		t.Fatal("duplicate endpoint/payload/alias was published")
	}
}

func TestOpaqueTransferIsMaximalAndAllocationFree(t *testing.T) {
	contract := mustSeal(t, Spec{})
	opaque, ok := contract.Opaque()
	if !ok || contract.transferCount(opaque) != 1 {
		t.Fatalf("opaque/count = %d/%v/%d", opaque, ok, contract.transferCount(opaque))
	}
	endpoint, endpointOK := contract.transferEndpointAt(opaque, 0)
	payload, payloadOK := contract.transferPayloadAt(opaque, 0)
	alias, aliasOK := contract.transferAliasAt(opaque, 0)
	identity, identityOK := contract.transferIdentityAt(opaque, 0)
	capabilities, capabilitiesOK := contract.transferCapabilitiesAt(opaque, 0)
	if !endpointOK || endpoint != (vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointExternal}) || !payloadOK || payload != (vocabulary.InputSource{Kind: vocabulary.InputSourceAllInputs}) || !aliasOK || alias != (vocabulary.InputSource{Kind: vocabulary.InputSourceAllInputs}) || !identityOK || identity != vocabulary.TransferIdentityUnspecified || !capabilitiesOK || capabilities != vocabulary.TransferCapabilitiesUnspecified {
		t.Fatalf("opaque transfer = %#v/%v %#v/%v %#v/%v %d/%v %d/%v", endpoint, endpointOK, payload, payloadOK, alias, aliasOK, identity, identityOK, capabilities, capabilitiesOK)
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		if _, ok := contract.transferEndpointAt(opaque, 0); !ok {
			panic("opaque transfer endpoint disappeared")
		}
		if _, ok := contract.transferPayloadAt(opaque, 0); !ok {
			panic("opaque transfer payload disappeared")
		}
	}); allocs != 0 {
		t.Fatalf("opaque transfer queries allocated %f times", allocs)
	}
}

func TestTransferWideAndDeepValidationHasNoSemanticCap(t *testing.T) {
	const width = 4096
	fixed := make([]schematype.Type, width)
	transfers := make([]vocabulary.TransferSpec, width)
	for index := 0; index < width; index++ {
		fixed[index] = testAny
		transfers[width-index-1] = transfer(vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointInput, Input: vocabulary.ValueFormal(index)}, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: uint32(index)}, vocabulary.TransferIdentitySame, vocabulary.TransferCapabilitiesPreserveAll,
			[]vocabulary.TransferOutcomeSpec{{Outcome: 3, Possibility: vocabulary.TransferMayReject}, {Outcome: 1, Possibility: vocabulary.TransferMayReject}, {Outcome: 2, Possibility: vocabulary.TransferMayDeliver}, {Outcome: 0, Possibility: vocabulary.TransferMayDeliver | vocabulary.TransferMayReject}})
	}
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{{Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"wide-transfer"}}}, Input: vocabulary.ValuesSpec{Fixed: fixed, Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeCancel, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}, {Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}, {Kind: flowkind.OutcomeYield, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}, {Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Transfers: transfers, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}}}})
	op, _ := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"wide-transfer"}})
	for index := 0; index < width; index++ {
		endpoint, ok := contract.transferEndpointAt(op, index)
		if !ok || endpoint != (vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointInput, Input: vocabulary.ValueFormal(index)}) {
			t.Fatalf("wide transfer %d = %#v/%v", index, endpoint, ok)
		}
	}
}

func transfer(endpoint vocabulary.TransferEndpoint, payload vocabulary.InputSource, identity vocabulary.TransferIdentity, capabilities vocabulary.TransferCapabilities, outcomes []vocabulary.TransferOutcomeSpec) vocabulary.TransferSpec {
	return vocabulary.TransferSpec{Endpoint: endpoint, Payload: payload, Alias: payload, Identity: identity, Capabilities: capabilities, Outcomes: outcomes}
}

func transferOperation(outcomes []vocabulary.OutcomeSpec, transfers []vocabulary.TransferSpec) vocabulary.OperationSpec {
	return vocabulary.OperationSpec{Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"transfer"}}}, ValuesVars: 1, Input: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny, testString}, Tail: vocabulary.ValuesVariable, Var: 0}, Outcomes: outcomes, Transfers: transfers, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}}
}

func TestFreshOutcomeOrderingRemapsSuspensionAndResumeCoordinates(t *testing.T) {
	resumeMappings := []vocabulary.ResumeOutcomeSpec{
		{Kind: flowkind.OutcomeNormal, Outcome: 0}, {Kind: flowkind.OutcomeReturn, Outcome: 0},
		{Kind: flowkind.OutcomeThrow, Outcome: 0}, {Kind: flowkind.OutcomeYield, Outcome: 0},
		{Kind: flowkind.OutcomeCancel, Outcome: 0},
	}
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{
		{
			Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"fresh-suspend"}}},
			Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
			// The Fresh Table case is source ordinal zero but sorts after the
			// otherwise-identical no-fresh case.
			Outcomes: []vocabulary.OutcomeSpec{
				{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed}, FreshResults: []vocabulary.FreshResultSpec{{Result: 0, Kind: schematype.FreshClassTable}}},
				{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed}},
				{Kind: flowkind.OutcomeYield, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}},
			},
			Suspensions: []vocabulary.SuspensionSpec{{Yield: 2, Reentry: 0, Source: vocabulary.ReentryByCall, Multiplicity: vocabulary.ReentryOnce}},
			Effects:     vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		},
		{
			Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"fresh-resume"}}},
			ValuesVars: 1,
			Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesVariable, Var: 0},
			Outcomes: []vocabulary.OutcomeSpec{
				{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed}, FreshResults: []vocabulary.FreshResultSpec{{Result: 0, Kind: schematype.FreshClassThread}}},
				{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed}},
			},
			Resumes: []vocabulary.ResumeSpec{{Source: vocabulary.ResumeSourceValueFormal, Carrier: 0, Arguments: callbackTail(0), Outcomes: resumeMappings}},
			Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		},
		{
			Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"fresh-spawn"}}},
			ValuesVars: 7,
			Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesVariable, Var: 0},
			Callbacks: []vocabulary.CallbackSpec{{
				Function: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, Admission: schematype.CallableAdmissionOrdinary, Arguments: callbackTail(1),
				Outcomes: callbackOutcomes(2, 3, 4, 5, 6), Lifecycle: vocabulary.CallbackRetainedRequiredOnce,
				Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
			}},
			Outcomes: []vocabulary.OutcomeSpec{
				{Kind: flowkind.OutcomeYield, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}},
				{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed}, FreshResults: []vocabulary.FreshResultSpec{{Result: 0, Kind: schematype.FreshClassTable}}},
				{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}},
				{Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed}},
			},
			Suspensions: []vocabulary.SuspensionSpec{{Yield: 0, Reentry: 2, Source: vocabulary.ReentryByProvider, Multiplicity: vocabulary.ReentryOnce}},
			Spawns: []vocabulary.SpawnSpec{{
				Function: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, Child: 1, Yield: 0, ParentResume: 2, ChildEntry: 2,
				Alternatives: []vocabulary.SpawnSiblingAlternative{vocabulary.SpawnChildEntryThenParentResume, vocabulary.SpawnParentResumeThenChildEntry},
			}},
			Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		},
	}})

	suspend, _ := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"fresh-suspend"}})
	yield, reentry, _, _, ok := contract.suspensionAt(suspend, 0)
	if !ok {
		t.Fatal("fresh suspension missing")
	}
	if kind, _, found := contract.OutcomeAt(suspend, int(yield)); !found || kind != flowkind.OutcomeYield {
		t.Fatalf("suspension yield = %d/%d/%v", yield, kind, found)
	}
	if _, kind, _, found := contract.freshResultForResult(suspend, int(reentry), 0); !found || kind != schematype.FreshClassTable {
		t.Fatalf("suspension reentry = %d lacks remapped schematype.FreshClassTable case", reentry)
	}

	resume, _ := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"fresh-resume"}})
	resumeID, ok := contract.ResumeIDAt(resume, 0)
	if !ok {
		t.Fatal("fresh resume missing")
	}
	for index := 0; index < contract.resumeOutcomeCount(resumeID); index++ {
		_, outcome, found := contract.resumeOutcomeAt(resumeID, index)
		if !found {
			t.Fatalf("resume mapping %d missing", index)
		}
		if _, kind, _, fresh := contract.freshResultForResult(resume, int(outcome), 0); !fresh || kind != schematype.FreshClassThread {
			t.Fatalf("resume mapping %d = outcome %d without remapped schematype.FreshClassThread case", index, outcome)
		}
	}

	spawn, _ := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"fresh-spawn"}})
	spawnID, ok := contract.spawnIDAt(spawn, 0)
	if !ok {
		t.Fatal("fresh spawn missing")
	}
	_, _, _, parentYield, parentResume, childEntry, resumeValues, found := contract.spawnRelation(spawnID)
	if !found {
		t.Fatal("fresh spawn relation unavailable")
	}
	if kind, _, found := contract.OutcomeAt(spawn, int(parentYield)); !found || kind != flowkind.OutcomeYield {
		t.Fatalf("spawn yield = %d/%d/%v", parentYield, kind, found)
	}
	if kind, values, found := contract.OutcomeAt(spawn, int(parentResume)); !found || kind != flowkind.OutcomeNormal || values != childEntry || values != resumeValues || contract.ValuesCount(values) != 0 {
		t.Fatalf("spawn resume = %d/%d/%d/%d/%v", parentResume, kind, childEntry, resumeValues, found)
	}
}
