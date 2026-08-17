package target

import (
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func TestSuspensionCanonicalizesOutcomeOrdinalsAndKeepsCallbackValues(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{
		{
			Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"yield"}}},
			ValuesVars: 3,
			Input:      ValuesSpec{Tail: ValuesVariable, Var: 0},
			// Authoring order is deliberately Yield, Cancel, Normal, Throw. Seal owns the
			// canonical outcome order and remaps the suspension coordinates.
			Outcomes: []OutcomeSpec{
				{Kind: flowkind.OutcomeYield, Values: ValuesSpec{Tail: ValuesVariable, Var: 0}},
				{Kind: flowkind.OutcomeCancel, Values: ValuesSpec{Tail: ValuesClosed}},
				{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesVariable, Var: 1}},
				{Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Tail: ValuesVariable, Var: 2}},
			},
			Suspensions: []SuspensionSpec{
				{Yield: 0, Reentry: 2, Source: ReentryByCall, Multiplicity: ReentryOnce},
				{Yield: 0, Reentry: 3, Source: ReentryByProvider, Multiplicity: ReentryOnce},
				{Yield: 0, Reentry: 1, Source: ReentryByProvider, Multiplicity: ReentryOnce},
			},
			Effects: RowSpec{Tail: RowClosed},
		},
		{
			Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"create"}}},
			ValuesVars: 5,
			Input:      ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed},
			Callbacks: []CallbackSpec{{
				Function:  InputSource{Kind: InputSourceValueFormal, Ordinal: 0},
				Admission: OrdinaryCallable, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: CallbackRetainedOptionalOnce,
				Effects: RowSpec{Tail: RowClosed},
			}},
			Outcomes: []OutcomeSpec{{
				Kind:            flowkind.OutcomeNormal,
				Values:          ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed},
				CallbackResults: []CallbackResultSpec{{Result: 0, Callback: 1}},
			}},
			Effects: RowSpec{Tail: RowClosed},
		},
		{
			Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"resume"}}},
			ValuesVars: 1,
			Input:      ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesVariable, Var: 0},
			Outcomes:   []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesVariable, Var: 0}}},
			Resumes:    []ResumeSpec{completeResume(ResumeSourceValueFormal, 0, 0)},
			Effects:    RowSpec{Tail: RowClosed},
		},
	}})

	yield, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"yield"}})
	if contract.SuspensionCount(yield) != 3 {
		t.Fatalf("suspension count = %d, want 3", contract.SuspensionCount(yield))
	}
	for index, want := range []struct {
		reentry uint32
		kind    flowkind.OutcomeKind
		source  ReentrySource
	}{
		{reentry: 0, kind: flowkind.OutcomeNormal, source: ReentryByCall},
		{reentry: 1, kind: flowkind.OutcomeThrow, source: ReentryByProvider},
		{reentry: 3, kind: flowkind.OutcomeCancel, source: ReentryByProvider},
	} {
		yieldOutcome, reentryOutcome, source, multiplicity, ok := contract.SuspensionAt(yield, index)
		if !ok || yieldOutcome != 2 || reentryOutcome != want.reentry || source != want.source || multiplicity != ReentryOnce {
			t.Fatalf("SuspensionAt(%d) = %d/%d/%d/%d/%v", index, yieldOutcome, reentryOutcome, source, multiplicity, ok)
		}
		if kind, _, ok := contract.OutcomeAt(yield, int(reentryOutcome)); !ok || kind != want.kind {
			t.Fatalf("reentry %d kind = %d/%v, want %d", index, kind, ok, want.kind)
		}
	}

	create, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"create"}})
	callback, _, ok := contract.CallbackForResult(create, 0, 0)
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

	resume, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"resume"}})
	resumeID, ok := contract.ResumeIDAt(resume, 0)
	owner, resumeSource, carrier, arguments, ok := contract.Resume(resumeID)
	tail, variable, tailOK := contract.ValuesTail(arguments)
	if !ok || owner != resume || resumeSource != ResumeSourceValueFormal || carrier != 0 || !tailOK || tail != ValuesVariable || variable != 0 {
		t.Fatalf("Resume = %d/%d/%d/%d/%d/%d/%v/%v, want owner/formal/0/variable/0/true", owner, resumeSource, carrier, arguments, tail, variable, ok, tailOK)
	}
}

func TestProducedResumeUsesCapturedCallbackWithoutCallableShadow(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{
		{
			Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"wrap"}}},
			ValuesVars: 5,
			Input:      ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed},
			Callbacks: []CallbackSpec{{
				Function:  InputSource{Kind: InputSourceValueFormal, Ordinal: 0},
				Admission: OrdinaryCallable, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: CallbackRetainedOptionalOnce,
				Effects: RowSpec{Tail: RowClosed},
			}},
			Outcomes: []OutcomeSpec{{
				Kind:   flowkind.OutcomeNormal,
				Values: ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed},
				Produced: []ProducedSpec{{
					Result: 0, Operation: 2, Captures: []CaptureSpec{{Kind: CaptureCallback, Ordinal: 1}},
				}},
			}},
			Effects: RowSpec{Tail: RowClosed},
		},
		{
			ValuesVars: 1,
			Input:      ValuesSpec{Tail: ValuesVariable, Var: 0},
			Outcomes:   []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}},
			Resumes:    []ResumeSpec{completeResume(ResumeSourceProduced, 0, 0)},
			Effects:    RowSpec{Tail: RowClosed},
		},
	}})
	wrap, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"wrap"}})
	invoke, produced, ok := contract.ProducedForResult(wrap, 0, 0)
	if !ok || invoke == 0 {
		t.Fatal("wrap result did not select one ordinary Produced Operation")
	}
	kind, capture, ok := contract.ProducedCaptureAt(wrap, 0, produced, 0)
	if !ok || kind != CaptureCallback || CallbackID(capture) == 0 {
		t.Fatal("wrap produced operation lost its retained CallbackID")
	}
	resumeID, ok := contract.ResumeIDAt(invoke, 0)
	owner, source, carrier, arguments, ok := contract.Resume(resumeID)
	tail, variable, tailOK := contract.ValuesTail(arguments)
	if !ok || owner != invoke || source != ResumeSourceProduced || carrier != 0 || !tailOK || tail != ValuesVariable || variable != 0 {
		t.Fatalf("produced Resume = %d/%d/%d/%d/%d/%d/%v/%v", owner, source, carrier, arguments, tail, variable, ok, tailOK)
	}
}

func TestResumeSealsCompleteCanonicalOutcomeCorrespondence(t *testing.T) {
	seal := func(outcomes []OutcomeSpec, mapping []ResumeOutcomeSpec) *Contract {
		return mustSeal(t, Spec{Operations: []OperationSpec{{
			Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"resume-canonical"}}},
			ValuesVars: 1,
			Input:      ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesVariable, Var: 0},
			Outcomes:   outcomes,
			Resumes: []ResumeSpec{{
				Source: ResumeSourceValueFormal, Carrier: 0, Arguments: callbackTail(0), Outcomes: mapping,
			}},
			Effects: RowSpec{Tail: RowClosed},
		}}})
	}
	left := seal(
		[]OutcomeSpec{
			{Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Tail: ValuesClosed}},
			{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesVariable, Var: 0}},
		},
		[]ResumeOutcomeSpec{
			{Kind: flowkind.OutcomeCancel, Outcome: 0},
			{Kind: flowkind.OutcomeYield, Outcome: 1},
			{Kind: flowkind.OutcomeThrow, Outcome: 0},
			{Kind: flowkind.OutcomeReturn, Outcome: 1},
			{Kind: flowkind.OutcomeNormal, Outcome: 1},
		},
	)
	right := seal(
		[]OutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesVariable, Var: 0}},
			{Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Tail: ValuesClosed}},
		},
		[]ResumeOutcomeSpec{
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
	op, ok := left.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"resume-canonical"}})
	if !ok {
		t.Fatal("resume operation missing")
	}
	resumeID, ok := left.ResumeIDAt(op, 0)
	if !ok {
		t.Fatal("resume identity missing")
	}
	if count := left.ResumeOutcomeCount(resumeID); count != 5 {
		t.Fatalf("ResumeOutcomeCount = %d, want 5", count)
	}
	for index, want := range [...]struct {
		kind    flowkind.OutcomeKind
		outcome uint32
	}{
		{flowkind.OutcomeNormal, 0}, {flowkind.OutcomeReturn, 0}, {flowkind.OutcomeThrow, 1},
		{flowkind.OutcomeYield, 0}, {flowkind.OutcomeCancel, 1},
	} {
		kind, outcome, found := left.ResumeOutcomeAt(resumeID, index)
		if !found || kind != want.kind || outcome != want.outcome {
			t.Fatalf("ResumeOutcomeAt(%d) = %d/%d/%v, want %d/%d/true", index, kind, outcome, found, want.kind, want.outcome)
		}
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		if _, _, ok := left.ResumeOutcomeAt(resumeID, 4); !ok {
			panic("resume mapping disappeared")
		}
	}); allocs != 0 {
		t.Fatalf("ResumeOutcomeAt allocated %f times", allocs)
	}
}

func TestResumeIDsCanonicalizePermutationAndRoundTrip(t *testing.T) {
	operation := func(resumes []ResumeSpec) OperationSpec {
		return OperationSpec{
			Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"resume-id"}}},
			ValuesVars: 1,
			Input:      ValuesSpec{Fixed: []schematype.Type{testAny, testAny}, Tail: ValuesVariable, Var: 0},
			Outcomes:   []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesVariable, Var: 0}}},
			Resumes:    resumes,
			Effects:    RowSpec{Tail: RowClosed},
		}
	}
	left := mustSeal(t, Spec{Operations: []OperationSpec{operation([]ResumeSpec{
		completeResume(ResumeSourceValueFormal, 1, 0),
		completeResume(ResumeSourceValueFormal, 0, 0),
	})}})
	right := mustSeal(t, Spec{Operations: []OperationSpec{operation([]ResumeSpec{
		completeResume(ResumeSourceValueFormal, 0, 0),
		completeResume(ResumeSourceValueFormal, 1, 0),
	})}})
	if got, want := publicContractSnapshot(t, left), publicContractSnapshot(t, right); got != want {
		t.Fatalf("resume permutation changed public contract\nleft: %s\nright: %s", got, want)
	}
	if left.ContentID() != right.ContentID() {
		t.Fatal("resume permutation changed ContentID")
	}
	op, ok := left.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"resume-id"}})
	if !ok || left.ResumeCount(op) != 2 {
		t.Fatalf("resume identities missing: %d/%v", op, ok)
	}
	for index, wantCarrier := range [...]ValueFormal{0, 1} {
		id, found := left.ResumeIDAt(op, index)
		owner, source, carrier, arguments, detailsFound := left.Resume(id)
		idAgain, foundAgain := left.ResumeIDAt(owner, index)
		tail, variable, tailOK := left.ValuesTail(arguments)
		if !found || !detailsFound || !foundAgain || id == 0 || idAgain != id || owner != op ||
			source != ResumeSourceValueFormal || carrier != wantCarrier || !tailOK || tail != ValuesVariable || variable != 0 {
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
	base := OperationSpec{
		Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"bad-resume"}}},
		ValuesVars: 1,
		Input:      ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesVariable, Var: 0},
		Outcomes: []OutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}},
			{Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Tail: ValuesClosed}},
		},
		Effects: RowSpec{Tail: RowClosed},
	}
	valid := completeResume(ResumeSourceValueFormal, 0, 0)
	for _, test := range []struct {
		name   string
		input  ValuesSpec
		vars   uint32
		out    []OutcomeSpec
		resume ResumeSpec
	}{
		{"arguments outside scope", base.Input, base.ValuesVars, base.Outcomes, completeResume(ResumeSourceValueFormal, 0, 1)},
		{"arguments must be input tail", ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed}, 1, base.Outcomes, completeResume(ResumeSourceValueFormal, 0, 0)},
		{"different input tail variable", ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesVariable, Var: 1}, 2, base.Outcomes, completeResume(ResumeSourceValueFormal, 0, 0)},
		{"unknown input tail", ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesUnknown}, 1, base.Outcomes, completeResume(ResumeSourceValueFormal, 0, 0)},
		{"missing kind", base.Input, base.ValuesVars, base.Outcomes, ResumeSpec{Source: ResumeSourceValueFormal, Carrier: 0, Arguments: callbackTail(0), Outcomes: valid.Outcomes[:4]}},
		{"duplicate kind", base.Input, base.ValuesVars, base.Outcomes, ResumeSpec{Source: ResumeSourceValueFormal, Carrier: 0, Arguments: callbackTail(0), Outcomes: append(append([]ResumeOutcomeSpec(nil), valid.Outcomes[:4]...), ResumeOutcomeSpec{Kind: flowkind.OutcomeNormal, Outcome: 0})}},
		{"break cannot cross activation", base.Input, base.ValuesVars, base.Outcomes, ResumeSpec{Source: ResumeSourceValueFormal, Carrier: 0, Arguments: callbackTail(0), Outcomes: append(append([]ResumeOutcomeSpec(nil), valid.Outcomes[:4]...), ResumeOutcomeSpec{Kind: flowkind.OutcomeBreak, Outcome: 0})}},
		{"outcome outside scope", base.Input, base.ValuesVars, base.Outcomes, ResumeSpec{Source: ResumeSourceValueFormal, Carrier: 0, Arguments: callbackTail(0), Outcomes: []ResumeOutcomeSpec{{Kind: flowkind.OutcomeNormal, Outcome: 2}, {Kind: flowkind.OutcomeReturn, Outcome: 0}, {Kind: flowkind.OutcomeThrow, Outcome: 1}, {Kind: flowkind.OutcomeYield, Outcome: 0}, {Kind: flowkind.OutcomeCancel, Outcome: 1}}}},
		{"unknown mapped outcome", base.Input, base.ValuesVars, []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesUnknown}}, {Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Tail: ValuesClosed}}}, completeResume(ResumeSourceValueFormal, 0, 0)},
	} {
		t.Run(test.name, func(t *testing.T) {
			op := base
			op.Input = test.input
			op.ValuesVars = test.vars
			op.Outcomes = test.out
			op.Resumes = []ResumeSpec{test.resume}
			if _, err := testSeal(&Spec{Operations: []OperationSpec{op}}); err == nil {
				t.Fatal("Seal accepted invalid resume outcome authority")
			}
		})
	}
}

func TestResumePayloadTransportUsesOnlyExistingValuesRelations(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{
		{
			Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"transport-yield"}}},
			ValuesVars: 1,
			Input:      ValuesSpec{Tail: ValuesVariable, Var: 0},
			Outcomes: []OutcomeSpec{
				{Kind: flowkind.OutcomeYield, Values: ValuesSpec{Tail: ValuesVariable, Var: 0}},
				{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesVariable, Var: 0}},
			},
			Suspensions: []SuspensionSpec{{Yield: 0, Reentry: 1, Source: ReentryByCall, Multiplicity: ReentryOnce}},
			Effects:     RowSpec{Tail: RowClosed},
		},
		{
			Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"transport-resume"}}},
			ValuesVars: 2,
			Input:      ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesVariable, Var: 0},
			Outcomes: []OutcomeSpec{
				{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesVariable, Var: 1}},
				{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testLiteralBool(false)}, Tail: ValuesClosed}},
			},
			Resumes: []ResumeSpec{{
				Source: ResumeSourceValueFormal, Carrier: 0, Arguments: callbackTail(0),
				Outcomes: []ResumeOutcomeSpec{
					{Kind: flowkind.OutcomeNormal, Outcome: 0}, {Kind: flowkind.OutcomeReturn, Outcome: 0},
					{Kind: flowkind.OutcomeThrow, Outcome: 1}, {Kind: flowkind.OutcomeYield, Outcome: 0},
					{Kind: flowkind.OutcomeCancel, Outcome: 1},
				},
			}},
			Effects: RowSpec{Tail: RowClosed},
		},
	}})
	yield, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"transport-yield"}})
	resume, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"transport-resume"}})
	_, reentry, _, _, ok := contract.SuspensionAt(yield, 0)
	if !ok {
		t.Fatal("suspension missing")
	}
	_, reentryValues, ok := contract.OutcomeAt(yield, int(reentry))
	if !ok {
		t.Fatal("reentry outcome missing")
	}
	if tail, variable, ok := contract.ValuesTail(reentryValues); !ok || tail != ValuesVariable || variable != 0 {
		t.Fatalf("reentry Values = %d/%d/%v, want variable/0/true", tail, variable, ok)
	}
	resumeID, ok := contract.ResumeIDAt(resume, 0)
	owner, _, _, arguments, ok := contract.Resume(resumeID)
	tail, variable, tailOK := contract.ValuesTail(arguments)
	if !ok || owner != resume || !tailOK || tail != ValuesVariable || variable != 0 {
		t.Fatalf("resume arguments = %d/%d/%d/%v/%v, want variable/0/true", arguments, tail, variable, ok, tailOK)
	}
	for _, item := range []struct {
		index int
		tail  ValuesTail
		varID ValuesVar
	}{
		{0, ValuesVariable, 1}, // Normal transports into the existing result tail.
		{2, ValuesClosed, 0},   // Throw selects the existing closed failure and discards payload.
	} {
		_, outcome, ok := contract.ResumeOutcomeAt(resumeID, item.index)
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
	operation := func(outcomes []OutcomeSpec, suspensions []SuspensionSpec) *Contract {
		return mustSeal(t, Spec{Operations: []OperationSpec{{
			Bindings:    []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"permutation"}}},
			ValuesVars:  2,
			Input:       ValuesSpec{Tail: ValuesVariable, Var: 0},
			Outcomes:    outcomes,
			Suspensions: suspensions,
			Effects:     RowSpec{Tail: RowClosed},
		}}})
	}
	left := operation(
		[]OutcomeSpec{
			{Kind: flowkind.OutcomeYield, Values: ValuesSpec{Tail: ValuesVariable, Var: 0}},
			{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesVariable, Var: 1}},
			{Kind: flowkind.OutcomeCancel, Values: ValuesSpec{Tail: ValuesClosed}},
		},
		[]SuspensionSpec{
			{Yield: 0, Reentry: 1, Source: ReentryByCall, Multiplicity: ReentryOnce},
			{Yield: 0, Reentry: 2, Source: ReentryByProvider, Multiplicity: ReentryOnce},
		},
	)
	right := operation(
		[]OutcomeSpec{
			{Kind: flowkind.OutcomeCancel, Values: ValuesSpec{Tail: ValuesClosed}},
			{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesVariable, Var: 1}},
			{Kind: flowkind.OutcomeYield, Values: ValuesSpec{Tail: ValuesVariable, Var: 0}},
		},
		[]SuspensionSpec{
			{Yield: 2, Reentry: 1, Source: ReentryByCall, Multiplicity: ReentryOnce},
			{Yield: 2, Reentry: 0, Source: ReentryByProvider, Multiplicity: ReentryOnce},
		},
	)
	if leftPublic, rightPublic := publicContractSnapshot(t, left), publicContractSnapshot(t, right); leftPublic != rightPublic {
		t.Fatalf("outcome/suspension author permutation changed public contract\nleft: %s\nright: %s", leftPublic, rightPublic)
	}
}

func TestSuspensionRejectsInvalidAuthority(t *testing.T) {
	base := OperationSpec{
		Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"bad"}}},
		Input:    ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed},
		Outcomes: []OutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}},
			{Kind: flowkind.OutcomeYield, Values: ValuesSpec{Tail: ValuesClosed}},
		},
		Effects: RowSpec{Tail: RowClosed},
	}
	for _, test := range []struct {
		name       string
		suspension []SuspensionSpec
		resumes    []ResumeSpec
	}{
		{name: "normal is not yield", suspension: []SuspensionSpec{{Yield: 0, Reentry: 0, Source: ReentryByCall, Multiplicity: ReentryOnce}}},
		{name: "yield is not reentry", suspension: []SuspensionSpec{{Yield: 1, Reentry: 1, Source: ReentryByCall, Multiplicity: ReentryOnce}}},
		{name: "invalid source", suspension: []SuspensionSpec{{Yield: 1, Reentry: 0, Source: ReentrySourceInvalid, Multiplicity: ReentryOnce}}},
		{name: "invalid multiplicity", suspension: []SuspensionSpec{{Yield: 1, Reentry: 0, Source: ReentryByCall, Multiplicity: ReentryMultiplicityInvalid}}},
		{name: "same authority different multiplicity", suspension: []SuspensionSpec{
			{Yield: 1, Reentry: 0, Source: ReentryByCall, Multiplicity: ReentryOnce},
			{Yield: 1, Reentry: 0, Source: ReentryByCall, Multiplicity: ReentryMany},
		}},
		{name: "duplicate exact suspension", suspension: []SuspensionSpec{
			{Yield: 1, Reentry: 0, Source: ReentryByCall, Multiplicity: ReentryOnce},
			{Yield: 1, Reentry: 0, Source: ReentryByCall, Multiplicity: ReentryOnce},
		}},
		{name: "formal outside scope", resumes: []ResumeSpec{completeResume(ResumeSourceValueFormal, 1, 0)}},
		{name: "invalid resume source", resumes: []ResumeSpec{completeResume(ResumeSourceInvalid, 0, 0)}},
		{name: "duplicate exact resume", resumes: []ResumeSpec{
			completeResume(ResumeSourceValueFormal, 0, 0),
			completeResume(ResumeSourceValueFormal, 0, 0),
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			operation := base
			operation.Suspensions = test.suspension
			operation.Resumes = test.resumes
			if _, err := testSeal(&Spec{Operations: []OperationSpec{operation}}); err == nil {
				t.Fatal("invalid suspension authority accepted")
			}
		})
	}
}

func TestOpaqueSuspensionIsDerivedAndQueriesAllocateNothing(t *testing.T) {
	contract := mustSeal(t, Spec{})
	opaque, _ := contract.Opaque()
	if contract.SuspensionCount(opaque) != 3 || contract.ResumeCount(opaque) != 0 {
		t.Fatal("opaque suspension was not derived exactly")
	}
	for index, wantReentry := range []uint32{0, 1, 3} {
		yield, reentry, source, multiplicity, ok := contract.SuspensionAt(opaque, index)
		if !ok || yield != 2 || reentry != wantReentry || source != ReentryByProvider || multiplicity != ReentryMany {
			t.Fatalf("opaque suspension %d = %d/%d/%d/%d/%v", index, yield, reentry, source, multiplicity, ok)
		}
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		if _, _, _, _, ok := contract.SuspensionAt(opaque, 2); !ok {
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
	operations := make([]OperationSpec, width+1)
	operations[0] = OperationSpec{
		Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"wide"}}},
		ValuesVars: 5,
		Input:      ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed},
		Callbacks: []CallbackSpec{{
			Function:  InputSource{Kind: InputSourceValueFormal, Ordinal: 0},
			Admission: OrdinaryCallable, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: CallbackRetainedOptionalOnce,
			Effects: RowSpec{Tail: RowClosed},
		}},
		Outcomes: []OutcomeSpec{{
			Kind:     flowkind.OutcomeNormal,
			Values:   ValuesSpec{Fixed: values, Tail: ValuesClosed},
			Produced: make([]ProducedSpec, width),
		}},
		Effects: RowSpec{Tail: RowClosed},
	}
	for index := 0; index < width; index++ {
		operations[0].Outcomes[0].Produced[index] = ProducedSpec{
			Result: uint32(index), Operation: SpecRef(index + 2), Captures: []CaptureSpec{{Kind: CaptureCallback, Ordinal: 1}},
		}
		operations[index+1] = OperationSpec{
			ValuesVars: 1,
			Input:      ValuesSpec{Tail: ValuesVariable, Var: 0},
			Outcomes:   []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}},
			Resumes:    []ResumeSpec{completeResume(ResumeSourceProduced, 0, 0)},
			Effects:    RowSpec{Tail: RowClosed},
		}
	}
	contract := mustSeal(t, Spec{Operations: operations})
	if contract.OperationCount() != width+2 {
		t.Fatalf("operation count = %d, want %d", contract.OperationCount(), width+2)
	}
	root, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"wide"}})
	last, _, ok := contract.ProducedForResult(root, 0, width-1)
	if !ok || contract.ResumeCount(last) != 1 {
		t.Fatal("wide produced resume was not retained")
	}
}

func TestDeepProducedResumesSealIteratively(t *testing.T) {
	const depth = 1024
	operations := make([]OperationSpec, depth)
	for index := range operations {
		operation := OperationSpec{
			ValuesVars: 5,
			Input:      ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesVariable, Var: 0},
			Callbacks: []CallbackSpec{{
				Function:  InputSource{Kind: InputSourceValueFormal, Ordinal: 0},
				Admission: OrdinaryCallable, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: CallbackRetainedOptionalOnce,
				Effects: RowSpec{Tail: RowClosed},
			}},
			Outcomes: []OutcomeSpec{{
				Kind:   flowkind.OutcomeNormal,
				Values: ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed},
			}},
			Effects: RowSpec{Tail: RowClosed},
		}
		if index == 0 {
			operation.Bindings = []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"deep-resume"}}}
		} else {
			operation.Resumes = []ResumeSpec{completeResume(ResumeSourceProduced, 0, 0)}
		}
		if index+1 < depth {
			operation.Outcomes[0].Produced = []ProducedSpec{{
				Result: 0, Operation: SpecRef(index + 2), Captures: []CaptureSpec{{Kind: CaptureCallback, Ordinal: 1}},
			}}
		}
		operations[index] = operation
	}
	contract := mustSeal(t, Spec{Operations: operations})
	current, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"deep-resume"}})
	for index := 1; index < depth; index++ {
		next, _, ok := contract.ProducedForResult(current, 0, 0)
		if !ok {
			t.Fatalf("deep produced resume ended at %d", index)
		}
		current = next
	}
	resumeID, ok := contract.ResumeIDAt(current, 0)
	_, source, _, _, ok := contract.Resume(resumeID)
	if !ok || source != ResumeSourceProduced {
		t.Fatal("deep terminal produced resume missing")
	}
}

func completeResume(source ResumeSource, carrier ValueFormal, arguments ValuesVar) ResumeSpec {
	return ResumeSpec{
		Source: source, Carrier: carrier, Arguments: callbackTail(arguments),
		Outcomes: []ResumeOutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Outcome: 0},
			{Kind: flowkind.OutcomeReturn, Outcome: 0},
			{Kind: flowkind.OutcomeThrow, Outcome: 0},
			{Kind: flowkind.OutcomeYield, Outcome: 0},
			{Kind: flowkind.OutcomeCancel, Outcome: 0},
		},
	}
}
