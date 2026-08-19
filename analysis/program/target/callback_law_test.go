package target

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"testing"
)

func callbackLifecycleOperation(name string, lifecycles ...vocabulary.CallbackLifecycle) vocabulary.OperationSpec {
	callbacks := make([]vocabulary.CallbackSpec, len(lifecycles))
	subedges := make([]vocabulary.SubedgeSpec, 0, len(lifecycles))
	input := make([]schematype.Type, len(lifecycles))
	for index, lifecycle := range lifecycles {
		input[index] = testAny
		callbacks[index] = vocabulary.CallbackSpec{
			Function:  vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: uint32(index)},
			Admission: schematype.CallableAdmissionOrdinary,
			Arguments: vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0},
			Outcomes:  callbackOutcomes(1, 1, 2, 3, 4),
			Lifecycle: lifecycle,
			Effects:   vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}
		if !retainedCallbackLifecycle(lifecycle) {
			subedges = append(subedges, callbackDirectSubedge(vocabulary.CallbackRef(index+1), uint32(index+1)))
		}
	}
	return vocabulary.OperationSpec{
		Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{name}}},
		ValuesVars: 5,
		Input:      vocabulary.ValuesSpec{Fixed: input, Tail: vocabulary.ValuesVariable, Var: 0},
		Callbacks:  callbacks,
		Outcomes: []vocabulary.OutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Values: callbackTail(1)},
			{Kind: flowkind.OutcomeThrow, Values: callbackTail(2)},
			{Kind: flowkind.OutcomeCancel, Values: callbackTail(4)},
		},
		Subedges: subedges,
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
}

func callbackDirectSubedge(callback vocabulary.CallbackRef, role uint32) vocabulary.SubedgeSpec {
	return vocabulary.SubedgeSpec{
		Role:   role,
		Family: vocabulary.SubedgeFamilyCall,
		Callee: vocabulary.SubedgeCalleeSpec{Kind: vocabulary.SubedgeCalleeCallback, Callback: callback},
		ArgumentOrigins: []vocabulary.ArgumentOrigin{{
			Segment: vocabulary.ArgumentTail, Kind: vocabulary.ArgumentSourceInput, Source: vocabulary.InputSource{Kind: vocabulary.InputSourceValuesVar, Ordinal: 0},
		}},
		AdmissionFailure: vocabulary.AdmissionFailureSpec{
			Values: callbackTail(2),
			Route:  vocabulary.AdmissionRouteSpec{Route: vocabulary.RouteOutcome, Adjustment: vocabulary.AdjustmentPreserve, Result: callbackTail(2), Placement: vocabulary.PlacementTail, Outcome: 1},
		},
		Routes: []vocabulary.SubedgeRouteSpec{
			{Kind: flowkind.OutcomeNormal, Route: vocabulary.RouteOutcome, Adjustment: vocabulary.AdjustmentPreserve, Result: callbackTail(1), Placement: vocabulary.PlacementTail, Outcome: 0},
			{Kind: flowkind.OutcomeReturn, Route: vocabulary.RouteOutcome, Adjustment: vocabulary.AdjustmentPreserve, Result: callbackTail(1), Placement: vocabulary.PlacementTail, Outcome: 0},
			{Kind: flowkind.OutcomeThrow, Route: vocabulary.RouteOutcome, Adjustment: vocabulary.AdjustmentPreserve, Result: callbackTail(2), Placement: vocabulary.PlacementTail, Outcome: 1},
			{Kind: flowkind.OutcomeYield, Route: vocabulary.RoutePropagateYield, Adjustment: vocabulary.AdjustmentPreserve, Result: callbackTail(3)},
			{Kind: flowkind.OutcomeCancel, Route: vocabulary.RouteOutcome, Adjustment: vocabulary.AdjustmentPreserve, Result: callbackTail(4), Placement: vocabulary.PlacementTail, Outcome: 2},
		},
	}
}

func callbackTail(variable vocabulary.ValuesVar) vocabulary.ValuesSpec {
	return vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: variable}
}

func callbackOutcomes(normal, returned, thrown, yielded, canceled vocabulary.ValuesVar) []vocabulary.TerminalSpec {
	return []vocabulary.TerminalSpec{
		{Kind: flowkind.OutcomeNormal, Values: callbackTail(normal)},
		{Kind: flowkind.OutcomeReturn, Values: callbackTail(returned)},
		{Kind: flowkind.OutcomeThrow, Values: callbackTail(thrown)},
		{Kind: flowkind.OutcomeYield, Values: callbackTail(yielded)},
		{Kind: flowkind.OutcomeCancel, Values: callbackTail(canceled)},
	}
}

func TestCallbackLifecycleClosedVocabularyRoundTrip(t *testing.T) {
	want := []vocabulary.CallbackLifecycle{
		vocabulary.CallbackSyncOptionalOnce,
		vocabulary.CallbackSyncRequiredOnce,
		vocabulary.CallbackSyncOptionalMany,
		vocabulary.CallbackSyncRequiredMany,
		vocabulary.CallbackRetainedOptionalOnce,
		vocabulary.CallbackRetainedRequiredOnce,
		vocabulary.CallbackRetainedOptionalMany,
		vocabulary.CallbackRetainedRequiredMany,
	}
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{callbackLifecycleOperation("callback-lifecycle", want...)}})
	op, found := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"callback-lifecycle"}})
	if !found || contract.Operations.CallbackCount(op) != len(want) {
		t.Fatalf("callback lifecycle inventory = %d/%v", contract.Operations.CallbackCount(op), found)
	}
	for index, lifecycle := range want {
		id, idFound := contract.Operations.CallbackAt(op, index)
		got, lifecycleFound := contract.Operations.CallbackLifecycle(id)
		if !idFound || !lifecycleFound || got != lifecycle {
			t.Fatalf("lifecycle %d = %d/%v/%v, want %d", index, got, idFound, lifecycleFound, lifecycle)
		}
	}
	if _, found := contract.Operations.CallbackLifecycle(0); found {
		t.Fatal("zero CallbackID resolved a lifecycle")
	}
	id, _ := contract.Operations.CallbackAt(op, 0)
	if allocs := testing.AllocsPerRun(1000, func() {
		if lifecycle, ok := contract.Operations.CallbackLifecycle(id); !ok || lifecycle != vocabulary.CallbackSyncOptionalOnce {
			panic("callback lifecycle disappeared")
		}
	}); allocs != 0 {
		t.Fatalf("CallbackLifecycle allocated %f times", allocs)
	}
}

func TestCallbackLifecyclePermutationPreservesCorrespondence(t *testing.T) {
	left := callbackLifecycleOperation("callback-lifecycle-permutation", vocabulary.CallbackRetainedRequiredOnce, vocabulary.CallbackSyncOptionalMany)
	right := callbackLifecycleOperation("callback-lifecycle-permutation", vocabulary.CallbackRetainedRequiredOnce, vocabulary.CallbackSyncOptionalMany)
	right.Callbacks[0], right.Callbacks[1] = right.Callbacks[1], right.Callbacks[0]
	right.Subedges[0].Callee.Callback = 1
	first := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{left}})
	second := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{right}})
	assertPublicContractEqual(t, first, second)
	if first.ContentID() != second.ContentID() {
		t.Fatal("derived callback Subedge index changed ContentID under authoring permutation")
	}
}

func TestCallbackSubedgeProjectsOnlyImmediateDirectExecution(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{callbackLifecycleOperation(
		"callback-subedge", vocabulary.CallbackSyncOptionalMany, vocabulary.CallbackRetainedOptionalMany,
	)}})
	op, ok := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"callback-subedge"}})
	if !ok {
		t.Fatal("callback Subedge operation missing")
	}
	direct, directOK := contract.Operations.CallbackAt(op, 0)
	retained, retainedOK := contract.Operations.CallbackAt(op, 1)
	edge := subedgeByRole(t, contract, op, 1)
	got, found := contract.callbackSubedge(direct)
	if !directOK || !found || got != edge {
		t.Fatalf("direct callback Subedge = %d/%v, want %d", got, found, edge)
	}
	if inverse, inverseOK := contract.subedgeCallback(got); !inverseOK || inverse != direct {
		t.Fatalf("Subedge inverse = %d/%v, want %d", inverse, inverseOK, direct)
	}
	if !retainedOK {
		t.Fatal("retained callback missing")
	}
	if edge, found := contract.callbackSubedge(retained); found || edge != 0 {
		t.Fatalf("retained callback unexpectedly has immediate Subedge %d/%v", edge, found)
	}
	opaque, opaqueOK := contract.Operations.Opaque()
	opaqueCallback, callbackOK := contract.Operations.CallbackAt(opaque, 0)
	if !opaqueOK || !callbackOK {
		t.Fatal("opaque callback missing")
	}
	if edge, found := contract.callbackSubedge(opaqueCallback); found || edge != 0 {
		t.Fatalf("opaque callback unexpectedly has immediate Subedge %d/%v", edge, found)
	}
	if edge, found := contract.callbackSubedge(0); found || edge != 0 {
		t.Fatalf("zero callback resolved immediate Subedge %d/%v", edge, found)
	}
}

func TestCallbackLifecycleRejectsInvalidAndConflictingDefinitions(t *testing.T) {
	for _, lifecycle := range []vocabulary.CallbackLifecycle{vocabulary.CallbackLifecycleInvalid, vocabulary.CallbackLifecycle(255)} {
		if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{callbackLifecycleOperation("invalid-lifecycle", lifecycle)}}); err == nil {
			t.Fatalf("invalid callback lifecycle %d accepted", lifecycle)
		}
	}
	operation := callbackLifecycleOperation("conflicting-lifecycle", vocabulary.CallbackSyncOptionalOnce)
	conflict := operation.Callbacks[0]
	conflict.Lifecycle = vocabulary.CallbackRetainedOptionalOnce
	operation.Callbacks = append(operation.Callbacks, conflict)
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{operation}}); err == nil {
		t.Fatal("conflicting lifecycle definitions for one callback identity accepted")
	}
}

func TestSyncCallbackRequiresExactlyOneDirectSubedge(t *testing.T) {
	missing := callbackLifecycleOperation("sync-missing-direct", vocabulary.CallbackSyncRequiredOnce)
	missing.Subedges = nil
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{missing}}); err == nil {
		t.Fatal("Sync callback sealed without its direct Subedge")
	}
}

func TestOpaqueCallbackLifecycleIsMaximalAndExplicit(t *testing.T) {
	contract := mustSeal(t, Spec{})
	op, found := contract.Operations.Opaque()
	if !found || contract.Operations.CallbackCount(op) != 1 || contract.Operations.ValuesVarCount(op) != 0 {
		t.Fatalf("opaque callback inventory = %d callbacks/%d Values vars/%v", contract.Operations.CallbackCount(op), contract.Operations.ValuesVarCount(op), found)
	}
	id, idFound := contract.Operations.CallbackAt(op, 0)
	owner, ownerFound := contract.Operations.CallbackOwner(id)
	source, sourceFound := contract.callbackFunction(id)
	lifecycle, lifecycleFound := contract.Operations.CallbackLifecycle(id)
	arguments, argumentsFound := contract.CallbackArguments(id)
	if !idFound || !ownerFound || owner != op || !sourceFound || source != (vocabulary.InputSource{Kind: vocabulary.InputSourceAllInputs}) ||
		!lifecycleFound || lifecycle != vocabulary.CallbackRetainedOptionalMany ||
		!argumentsFound {
		t.Fatalf("opaque callback = id:%d/%v owner:%d/%v source:%#v/%v lifecycle:%d/%v arguments:%d/%v",
			id, idFound, owner, ownerFound, source, sourceFound, lifecycle, lifecycleFound,
			arguments, argumentsFound)
	}
	input, inputOK := contract.Operations.Input(op)
	if !inputOK || arguments != input {
		t.Fatalf("opaque callback arguments = %d/%v, want opaque unknown input %d", arguments, inputOK, input)
	}
	for _, kind := range []flowkind.OutcomeKind{
		flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow,
		flowkind.OutcomeYield, flowkind.OutcomeCancel,
	} {
		values, ok := contract.CallbackOutcome(id, kind)
		if !ok || values != input {
			t.Fatalf("opaque callback outcome %d = %d/%v, want opaque unknown %d/true", kind, values, ok, input)
		}
	}
}

func callbackOutcomeOperation(name string, outcomes []vocabulary.TerminalSpec) vocabulary.OperationSpec {
	return vocabulary.OperationSpec{
		Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{name}}},
		ValuesVars: 6,
		Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesVariable, Var: 0},
		Callbacks: []vocabulary.CallbackSpec{{
			Function: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, Admission: schematype.CallableAdmissionOrdinary,
			Arguments: vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0},
			Outcomes:  outcomes, Lifecycle: vocabulary.CallbackRetainedOptionalOnce,
			Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
}

func TestCallbackOutcomeIsTotalCanonicalAndAllocationFree(t *testing.T) {
	outcomes := callbackOutcomes(1, 2, 3, 4, 5)
	reversed := append([]vocabulary.TerminalSpec(nil), outcomes...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	first := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{callbackOutcomeOperation("callback-outcome", outcomes)}})
	second := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{callbackOutcomeOperation("callback-outcome", reversed)}})
	assertPublicContractEqual(t, first, second)
	op, found := first.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"callback-outcome"}})
	id, idFound := first.Operations.CallbackAt(op, 0)
	if !found || !idFound {
		t.Fatal("callback outcome correspondence missing")
	}
	kinds := [...]flowkind.OutcomeKind{
		flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow,
		flowkind.OutcomeYield, flowkind.OutcomeCancel,
	}
	for index, kind := range kinds {
		values, ok := first.CallbackOutcome(id, kind)
		tail, variable, tailOK := first.Operations.ValuesTail(values)
		if !ok || !tailOK || tail != vocabulary.ValuesVariable || variable != vocabulary.ValuesVar(index+1) {
			t.Fatalf("callback outcome %d = %d/%d/%d/%v/%v, want tail %d", kind, values, tail, variable, ok, tailOK, index+1)
		}
	}
	for _, kind := range []flowkind.OutcomeKind{flowkind.OutcomeBreak, flowkind.OutcomeGoto} {
		if _, ok := first.CallbackOutcome(id, kind); ok {
			t.Fatalf("non-activation outcome %d resolved", kind)
		}
	}
	if _, ok := first.CallbackOutcome(0, flowkind.OutcomeNormal); ok {
		t.Fatal("zero CallbackID resolved an outcome")
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		for _, kind := range kinds {
			if _, ok := first.CallbackOutcome(id, kind); !ok {
				panic("callback outcome disappeared")
			}
		}
	}); allocs != 0 {
		t.Fatalf("CallbackOutcome allocated %f times", allocs)
	}
}

func TestCallbackOutcomeRejectsIncompleteDuplicateInvalidAndOutOfScope(t *testing.T) {
	valid := callbackOutcomes(1, 2, 3, 4, 5)
	duplicate := append([]vocabulary.TerminalSpec(nil), valid...)
	duplicate[4].Kind = flowkind.OutcomeNormal
	invalid := append([]vocabulary.TerminalSpec(nil), valid...)
	invalid[4].Kind = flowkind.OutcomeBreak
	outOfScope := append([]vocabulary.TerminalSpec(nil), valid...)
	outOfScope[4].Values.Var = 6
	for _, test := range []struct {
		name     string
		outcomes []vocabulary.TerminalSpec
	}{
		{name: "missing", outcomes: valid[:4]},
		{name: "duplicate", outcomes: duplicate},
		{name: "invalid kind", outcomes: invalid},
		{name: "Values outside scope", outcomes: outOfScope},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{callbackOutcomeOperation("invalid-callback-outcome", test.outcomes)}}); err == nil {
				t.Fatal("invalid callback outcome relation accepted")
			}
		})
	}
}

func callbackOwnerOperation(name string) vocabulary.OperationSpec {
	return vocabulary.OperationSpec{
		Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{name}}},
		ValuesVars: 5,
		Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesVariable, Var: 0},
		Callbacks: []vocabulary.CallbackSpec{{
			Function: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0}, Admission: schematype.CallableAdmissionOrdinary, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: vocabulary.CallbackRetainedOptionalOnce, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
}

func TestCallbackOwnerCanonicalRoundTripAndForeignHandles(t *testing.T) {
	left := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{
		callbackOwnerOperation("callback-owner-b"),
		callbackOwnerOperation("callback-owner-a"),
	}})
	right := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{
		callbackOwnerOperation("callback-owner-a"),
		callbackOwnerOperation("callback-owner-b"),
	}})
	if got, want := publicContractSnapshot(t, left), publicContractSnapshot(t, right); got != want {
		t.Fatalf("callback owner permutation changed public contract\nleft: %s\nright: %s", got, want)
	}
	if left.ContentID() != right.ContentID() {
		t.Fatal("callback owner permutation changed ContentID")
	}
	for _, binding := range []vocabulary.BindingSpec{
		{Namespace: vocabulary.BindingBuiltin, Member: []string{"callback-owner-a"}},
		{Namespace: vocabulary.BindingBuiltin, Member: []string{"callback-owner-b"}},
	} {
		op, found := left.Operations.Lookup(binding)
		if !found {
			t.Fatalf("callback owner operation missing: %#v", binding)
		}
		id, found := left.Operations.CallbackAt(op, 0)
		owner, ownerFound := left.Operations.CallbackOwner(id)
		if !found || !ownerFound || id == 0 || owner != op {
			t.Fatalf("callback owner round trip = %d/%d/%v/%v, want %d", id, owner, found, ownerFound, op)
		}
	}
	if _, ok := left.Operations.CallbackOwner(0); ok {
		t.Fatal("zero CallbackID resolved")
	}
	foreign := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{
		callbackOwnerOperation("foreign-a"),
		callbackOwnerOperation("foreign-b"),
		callbackOwnerOperation("foreign-c"),
	}})
	foreignOpaque, found := foreign.Operations.Opaque()
	if !found {
		t.Fatal("foreign opaque operation missing")
	}
	foreignID, found := foreign.Operations.CallbackAt(foreignOpaque, 0)
	if !found {
		t.Fatal("foreign out-of-range CallbackID missing")
	}
	if _, ok := left.Operations.CallbackOwner(foreignID); ok {
		t.Fatal("out-of-range foreign CallbackID resolved in this Contract")
	}
	first, found := left.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"callback-owner-a"}})
	if !found {
		t.Fatal("callback owner allocation operation missing")
	}
	id, found := left.Operations.CallbackAt(first, 0)
	if !found {
		t.Fatal("callback owner allocation handle missing")
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		if owner, ok := left.Operations.CallbackOwner(id); !ok || owner != first {
			panic("callback owner disappeared")
		}
	}); allocs != 0 {
		t.Fatalf("CallbackOwner allocated %f times", allocs)
	}
}

func TestCallbackReleaseZeroPolicyIsRequiredAndExact(t *testing.T) {
	for _, want := range []struct {
		behavior vocabulary.CallbackReleaseZeroBehavior
		outcome  uint32
	}{
		{vocabulary.CallbackReleaseZeroSuppress, 0},
		{vocabulary.CallbackReleaseZeroThrow, 1},
		{vocabulary.CallbackReleaseZeroIdempotent, 0},
	} {
		contract := mustSeal(t, specWithCallbackReleaseZero(vocabulary.CallbackReleaseZeroSpec{Behavior: want.behavior, Outcome: want.outcome}))
		owner, ownerOK := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"release-zero-owner"}})
		callback, callbackOK := contract.Operations.CallbackAt(owner, 0)
		got, outcome, ok := contract.callbackReleaseZero(callback)
		if !ownerOK || !callbackOK || !ok || got != want.behavior || outcome != want.outcome {
			t.Fatalf("zero policy = %d/%d/%v, want %d/%d", got, outcome, ok, want.behavior, want.outcome)
		}
	}

	for _, test := range []struct {
		name string
		zero vocabulary.CallbackReleaseZeroSpec
	}{
		{"missing", vocabulary.CallbackReleaseZeroSpec{}},
		{"suppress outcome", vocabulary.CallbackReleaseZeroSpec{Behavior: vocabulary.CallbackReleaseZeroSuppress, Outcome: 1}},
		{"throw normal", vocabulary.CallbackReleaseZeroSpec{Behavior: vocabulary.CallbackReleaseZeroThrow, Outcome: 0}},
		{"idempotent throw", vocabulary.CallbackReleaseZeroSpec{Behavior: vocabulary.CallbackReleaseZeroIdempotent, Outcome: 1}},
		{"outcome outside scope", vocabulary.CallbackReleaseZeroSpec{Behavior: vocabulary.CallbackReleaseZeroThrow, Outcome: 2}},
	} {
		t.Run(test.name, func(t *testing.T) {
			spec := specWithCallbackReleaseZero(test.zero)
			if contract, err := testSeal(&spec); err == nil || contract != nil {
				t.Fatal("invalid zero-holder policy was published")
			}
		})
	}
}

func TestCallbackReleaseZeroPolicyAffectsContentID(t *testing.T) {
	suppress := mustSeal(t, specWithCallbackReleaseZero(vocabulary.CallbackReleaseZeroSpec{Behavior: vocabulary.CallbackReleaseZeroSuppress}))
	throw := mustSeal(t, specWithCallbackReleaseZero(vocabulary.CallbackReleaseZeroSpec{Behavior: vocabulary.CallbackReleaseZeroThrow, Outcome: 1}))
	idempotent := mustSeal(t, specWithCallbackReleaseZero(vocabulary.CallbackReleaseZeroSpec{Behavior: vocabulary.CallbackReleaseZeroIdempotent, Outcome: 0}))
	if suppress.ContentID() == throw.ContentID() || throw.ContentID() == idempotent.ContentID() || suppress.ContentID() == idempotent.ContentID() {
		t.Fatal("zero-holder release policy was omitted from ContentID")
	}
}

func specWithCallbackReleaseZero(zero vocabulary.CallbackReleaseZeroSpec) Spec {
	closed := vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}
	return Spec{Operations: []vocabulary.OperationSpec{
		{
			Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"release-zero-owner"}}},
			Input:    vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed},
			Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: closed}},
			Callbacks: []vocabulary.CallbackSpec{{
				Function: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, Admission: schematype.CallableAdmissionOrdinary,
				Arguments: closed,
				Outcomes: []vocabulary.TerminalSpec{
					{Kind: flowkind.OutcomeNormal, Values: closed}, {Kind: flowkind.OutcomeReturn, Values: closed},
					{Kind: flowkind.OutcomeThrow, Values: closed}, {Kind: flowkind.OutcomeYield, Values: closed}, {Kind: flowkind.OutcomeCancel, Values: closed},
				},
				Lifecycle: vocabulary.CallbackRetainedOptionalOnce,
				Effects:   vocabulary.RowSpec{Tail: vocabulary.RowClosed},
				Release:   &vocabulary.CallbackReleaseSpec{Operation: 2, Input: 0, Outcome: 0, Mode: vocabulary.CallbackReleaseOne, Zero: zero},
			}},
			Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		},
		{
			Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"release-zero-target"}}},
			Input:    vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed},
			Outcomes: []vocabulary.OutcomeSpec{
				{Kind: flowkind.OutcomeNormal, Values: closed},
				{Kind: flowkind.OutcomeThrow, Values: closed},
			},
			Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		},
	}}
}

func TestCallbackResultsRemapWithOutcomeAndCallbackCanonicalization(t *testing.T) {
	makeOperation := func(callbacks []vocabulary.CallbackSpec, results []vocabulary.CallbackResultSpec) vocabulary.OperationSpec {
		return vocabulary.OperationSpec{
			Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"callback-result"}}},
			ValuesVars: 5,
			Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny, testAny}, Tail: vocabulary.ValuesVariable, Var: 0},
			Callbacks:  callbacks,
			Outcomes: []vocabulary.OutcomeSpec{{
				Kind:            flowkind.OutcomeNormal,
				Values:          vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny, testAny}, Tail: vocabulary.ValuesClosed},
				CallbackResults: results,
			}},
			Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}
	}
	first := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{makeOperation(
		[]vocabulary.CallbackSpec{
			{Function: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 1}, Admission: schematype.CallableAdmissionOrdinary, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: vocabulary.CallbackRetainedOptionalOnce, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}},
			{Function: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0}, Admission: schematype.CallableAdmissionOrdinary, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: vocabulary.CallbackRetainedOptionalOnce, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}},
		},
		[]vocabulary.CallbackResultSpec{{Result: 1, Callback: 1}, {Result: 0, Callback: 2}},
	)}})
	second := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{makeOperation(
		[]vocabulary.CallbackSpec{
			{Function: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0}, Admission: schematype.CallableAdmissionOrdinary, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: vocabulary.CallbackRetainedOptionalOnce, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}},
			{Function: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 1}, Admission: schematype.CallableAdmissionOrdinary, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: vocabulary.CallbackRetainedOptionalOnce, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}},
		},
		[]vocabulary.CallbackResultSpec{{Result: 0, Callback: 1}, {Result: 1, Callback: 2}},
	)}})
	assertPublicContractEqual(t, first, second)

	op, ok := first.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"callback-result"}})
	if !ok || first.callbackResultCount(op, 0) != 2 {
		t.Fatalf("callback result relation missing: %d/%v", op, ok)
	}
	callback, index, ok := first.callbackForResult(op, 0, 0)
	if !ok || index != 0 || callback != 1 {
		t.Fatalf("result 0 callback = %d/%d/%v, want 1/0/true", callback, index, ok)
	}
	callback, index, ok = first.callbackForResult(op, 0, 1)
	if !ok || index != 1 || callback != 2 {
		t.Fatalf("result 1 callback = %d/%d/%v, want 2/1/true", callback, index, ok)
	}
	for _, want := range []struct {
		id     vocabulary.CallbackID
		formal uint32
	}{{id: 1, formal: 0}, {id: 2, formal: 1}} {
		source, found := first.callbackFunction(want.id)
		if !found || source.Kind != vocabulary.InputSourceValueFormal || source.Ordinal != want.formal {
			t.Fatalf("callback %d source = %#v/%v", want.id, source, found)
		}
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		if _, _, found := first.callbackForResult(op, 0, 1); !found {
			panic("callback result disappeared")
		}
	}); allocs != 0 {
		t.Fatalf("CallbackForResult allocated %f times", allocs)
	}
}

func TestCallbackResultsRejectInvalidAndDualAuthority(t *testing.T) {
	base := vocabulary.OperationSpec{
		Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"invalid-callback-result"}}},
		ValuesVars: 5,
		Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesVariable, Var: 0},
		Callbacks:  []vocabulary.CallbackSpec{{Function: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, Admission: schematype.CallableAdmissionOrdinary, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: vocabulary.CallbackRetainedOptionalOnce, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}}},
		Outcomes:   []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed}}},
		Effects:    vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
	for _, test := range []struct {
		name    string
		results []vocabulary.CallbackResultSpec
	}{
		{name: "tail", results: []vocabulary.CallbackResultSpec{{Result: 1, Callback: 1}}},
		{name: "zero callback", results: []vocabulary.CallbackResultSpec{{Result: 0, Callback: 0}}},
		{name: "unknown callback", results: []vocabulary.CallbackResultSpec{{Result: 0, Callback: 2}}},
		{name: "duplicate result", results: []vocabulary.CallbackResultSpec{{Result: 0, Callback: 1}, {Result: 0, Callback: 1}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			operation := base
			operation.Outcomes = append([]vocabulary.OutcomeSpec(nil), base.Outcomes...)
			operation.Outcomes[0].CallbackResults = test.results
			if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{operation}}); err == nil {
				t.Fatal("invalid callback result accepted")
			}
		})
	}

	overlap := base
	overlap.Outcomes = append([]vocabulary.OutcomeSpec(nil), base.Outcomes...)
	overlap.Outcomes[0].CallbackResults = []vocabulary.CallbackResultSpec{{Result: 0, Callback: 1}}
	overlap.Outcomes[0].Produced = []vocabulary.ProducedSpec{{Result: 0, Operation: 2}}
	producedOnly := vocabulary.OperationSpec{Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}}
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{overlap, producedOnly}}); err == nil {
		t.Fatal("callback result/produced overlap accepted")
	}
}

func TestCallbacksRequireValueFormalInputSource(t *testing.T) {
	operation := func(callbacks []vocabulary.CallbackSpec) vocabulary.OperationSpec {
		return vocabulary.OperationSpec{
			Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"callback-input-source"}}},
			ValuesVars: 5,
			Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesVariable, Var: 0},
			Callbacks:  callbacks,
			Outcomes:   []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
			Effects:    vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}
	}
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{operation([]vocabulary.CallbackSpec{{
		Function: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0}, Admission: schematype.CallableAdmissionOrdinary, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: vocabulary.CallbackRetainedOptionalOnce, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}})}})
	op, ok := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"callback-input-source"}})
	if !ok || contract.Operations.CallbackCount(op) != 1 {
		t.Fatalf("callback input-source relations missing: %d/%v", op, ok)
	}
	id, found := contract.Operations.CallbackAt(op, 0)
	got, foundSource := contract.callbackFunction(id)
	if !found || !foundSource || got != (vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0}) {
		t.Fatalf("callback fixed-formal source = %#v/%v/%v", got, found, foundSource)
	}
	for _, source := range []vocabulary.InputSource{
		{Kind: vocabulary.InputSourceValuesVar, Ordinal: 0},
		{Kind: vocabulary.InputSourceAllInputs},
	} {
		if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{operation([]vocabulary.CallbackSpec{{
			Function: source, Admission: schematype.CallableAdmissionOrdinary, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: vocabulary.CallbackRetainedOptionalOnce, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}})}}); err == nil {
			t.Fatalf("callback accepted non-scalar source %#v", source)
		}
	}
}
