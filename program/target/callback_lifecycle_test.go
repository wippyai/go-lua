package target

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
)

func callbackLifecycleOperation(name string, lifecycles ...CallbackLifecycle) OperationSpec {
	callbacks := make([]CallbackSpec, len(lifecycles))
	subedges := make([]SubedgeSpec, 0, len(lifecycles))
	input := make([]typ.Type, len(lifecycles))
	for index, lifecycle := range lifecycles {
		input[index] = typ.Any
		callbacks[index] = CallbackSpec{
			Function:  InputSource{Kind: InputSourceValueFormal, Ordinal: uint32(index)},
			Admission: OrdinaryCallable,
			Arguments: ValuesSpec{Tail: ValuesVariable, Var: 0},
			Outcomes:  callbackOutcomes(1, 1, 2, 3, 4),
			Lifecycle: lifecycle,
			Effects:   RowSpec{Tail: RowClosed},
		}
		if !retainedCallbackLifecycle(lifecycle) {
			subedges = append(subedges, callbackDirectSubedge(CallbackRef(index+1), uint32(index+1)))
		}
	}
	return OperationSpec{
		Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}},
		ValuesVars: 5,
		Input:      ValuesSpec{Fixed: input, Tail: ValuesVariable, Var: 0},
		Callbacks:  callbacks,
		Outcomes: []OutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Values: callbackTail(1)},
			{Kind: flowkind.OutcomeThrow, Values: callbackTail(2)},
			{Kind: flowkind.OutcomeCancel, Values: callbackTail(4)},
		},
		Subedges: subedges,
		Effects:  RowSpec{Tail: RowClosed},
	}
}

func callbackDirectSubedge(callback CallbackRef, role uint32) SubedgeSpec {
	return SubedgeSpec{
		Role:   role,
		Family: SubedgeFamilyCall,
		Callee: SubedgeCalleeSpec{Kind: SubedgeCalleeCallback, Callback: callback},
		ArgumentOrigins: []ArgumentOrigin{{
			Segment: ArgumentTail, Kind: ArgumentSourceInput, Source: InputSource{Kind: InputSourceValuesVar, Ordinal: 0},
		}},
		AdmissionFailure: AdmissionFailureSpec{
			Values: callbackTail(2),
			Route:  AdmissionRouteSpec{Route: RouteOutcome, Adjustment: AdjustmentPreserve, Result: callbackTail(2), Placement: PlacementTail, Outcome: 1},
		},
		Routes: []SubedgeRouteSpec{
			{Kind: flowkind.OutcomeNormal, Route: RouteOutcome, Adjustment: AdjustmentPreserve, Result: callbackTail(1), Placement: PlacementTail, Outcome: 0},
			{Kind: flowkind.OutcomeReturn, Route: RouteOutcome, Adjustment: AdjustmentPreserve, Result: callbackTail(1), Placement: PlacementTail, Outcome: 0},
			{Kind: flowkind.OutcomeThrow, Route: RouteOutcome, Adjustment: AdjustmentPreserve, Result: callbackTail(2), Placement: PlacementTail, Outcome: 1},
			{Kind: flowkind.OutcomeYield, Route: RoutePropagateYield, Adjustment: AdjustmentPreserve, Result: callbackTail(3)},
			{Kind: flowkind.OutcomeCancel, Route: RouteOutcome, Adjustment: AdjustmentPreserve, Result: callbackTail(4), Placement: PlacementTail, Outcome: 2},
		},
	}
}

func callbackTail(variable ValuesVar) ValuesSpec {
	return ValuesSpec{Tail: ValuesVariable, Var: variable}
}

func callbackOutcomes(normal, returned, thrown, yielded, canceled ValuesVar) []TerminalSpec {
	return []TerminalSpec{
		{Kind: flowkind.OutcomeNormal, Values: callbackTail(normal)},
		{Kind: flowkind.OutcomeReturn, Values: callbackTail(returned)},
		{Kind: flowkind.OutcomeThrow, Values: callbackTail(thrown)},
		{Kind: flowkind.OutcomeYield, Values: callbackTail(yielded)},
		{Kind: flowkind.OutcomeCancel, Values: callbackTail(canceled)},
	}
}

func TestCallbackLifecycleClosedVocabularyRoundTrip(t *testing.T) {
	want := []CallbackLifecycle{
		CallbackSyncOptionalOnce,
		CallbackSyncRequiredOnce,
		CallbackSyncOptionalMany,
		CallbackSyncRequiredMany,
		CallbackRetainedOptionalOnce,
		CallbackRetainedRequiredOnce,
		CallbackRetainedOptionalMany,
		CallbackRetainedRequiredMany,
	}
	contract := mustSeal(t, Spec{Operations: []OperationSpec{callbackLifecycleOperation("callback-lifecycle", want...)}})
	op, found := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"callback-lifecycle"}})
	if !found || contract.CallbackCount(op) != len(want) {
		t.Fatalf("callback lifecycle inventory = %d/%v", contract.CallbackCount(op), found)
	}
	for index, lifecycle := range want {
		id, idFound := contract.CallbackAt(op, index)
		got, lifecycleFound := contract.CallbackLifecycle(id)
		if !idFound || !lifecycleFound || got != lifecycle {
			t.Fatalf("lifecycle %d = %d/%v/%v, want %d", index, got, idFound, lifecycleFound, lifecycle)
		}
	}
	if _, found := contract.CallbackLifecycle(0); found {
		t.Fatal("zero CallbackID resolved a lifecycle")
	}
	id, _ := contract.CallbackAt(op, 0)
	if allocs := testing.AllocsPerRun(1000, func() {
		if lifecycle, ok := contract.CallbackLifecycle(id); !ok || lifecycle != CallbackSyncOptionalOnce {
			panic("callback lifecycle disappeared")
		}
	}); allocs != 0 {
		t.Fatalf("CallbackLifecycle allocated %f times", allocs)
	}
}

func TestCallbackLifecyclePermutationPreservesCorrespondence(t *testing.T) {
	left := callbackLifecycleOperation("callback-lifecycle-permutation", CallbackRetainedRequiredOnce, CallbackSyncOptionalMany)
	right := callbackLifecycleOperation("callback-lifecycle-permutation", CallbackRetainedRequiredOnce, CallbackSyncOptionalMany)
	right.Callbacks[0], right.Callbacks[1] = right.Callbacks[1], right.Callbacks[0]
	right.Subedges[0].Callee.Callback = 1
	first := mustSeal(t, Spec{Operations: []OperationSpec{left}})
	second := mustSeal(t, Spec{Operations: []OperationSpec{right}})
	assertPublicContractEqual(t, first, second)
	if first.ContentID() != second.ContentID() {
		t.Fatal("derived callback Subedge index changed ContentID under authoring permutation")
	}
}

func TestCallbackSubedgeProjectsOnlyImmediateDirectExecution(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{callbackLifecycleOperation(
		"callback-subedge", CallbackSyncOptionalMany, CallbackRetainedOptionalMany,
	)}})
	op, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"callback-subedge"}})
	if !ok {
		t.Fatal("callback Subedge operation missing")
	}
	direct, directOK := contract.CallbackAt(op, 0)
	retained, retainedOK := contract.CallbackAt(op, 1)
	edge := subedgeByRole(t, contract, op, 1)
	got, found := contract.CallbackSubedge(direct)
	if !directOK || !found || got != edge {
		t.Fatalf("direct callback Subedge = %d/%v, want %d", got, found, edge)
	}
	if inverse, inverseOK := contract.SubedgeCallback(got); !inverseOK || inverse != direct {
		t.Fatalf("Subedge inverse = %d/%v, want %d", inverse, inverseOK, direct)
	}
	if !retainedOK {
		t.Fatal("retained callback missing")
	}
	if edge, found := contract.CallbackSubedge(retained); found || edge != 0 {
		t.Fatalf("retained callback unexpectedly has immediate Subedge %d/%v", edge, found)
	}
	opaque, opaqueOK := contract.Opaque()
	opaqueCallback, callbackOK := contract.CallbackAt(opaque, 0)
	if !opaqueOK || !callbackOK {
		t.Fatal("opaque callback missing")
	}
	if edge, found := contract.CallbackSubedge(opaqueCallback); found || edge != 0 {
		t.Fatalf("opaque callback unexpectedly has immediate Subedge %d/%v", edge, found)
	}
	if edge, found := contract.CallbackSubedge(0); found || edge != 0 {
		t.Fatalf("zero callback resolved immediate Subedge %d/%v", edge, found)
	}
}

func TestCallbackLifecycleRejectsInvalidAndConflictingDefinitions(t *testing.T) {
	for _, lifecycle := range []CallbackLifecycle{CallbackLifecycleInvalid, CallbackLifecycle(255)} {
		if _, err := Seal(&Spec{Operations: []OperationSpec{callbackLifecycleOperation("invalid-lifecycle", lifecycle)}}); err == nil {
			t.Fatalf("invalid callback lifecycle %d accepted", lifecycle)
		}
	}
	operation := callbackLifecycleOperation("conflicting-lifecycle", CallbackSyncOptionalOnce)
	conflict := operation.Callbacks[0]
	conflict.Lifecycle = CallbackRetainedOptionalOnce
	operation.Callbacks = append(operation.Callbacks, conflict)
	if _, err := Seal(&Spec{Operations: []OperationSpec{operation}}); err == nil {
		t.Fatal("conflicting lifecycle definitions for one callback identity accepted")
	}
}

func TestSyncCallbackRequiresExactlyOneDirectSubedge(t *testing.T) {
	missing := callbackLifecycleOperation("sync-missing-direct", CallbackSyncRequiredOnce)
	missing.Subedges = nil
	if _, err := Seal(&Spec{Operations: []OperationSpec{missing}}); err == nil {
		t.Fatal("Sync callback sealed without its direct Subedge")
	}
}

func TestOpaqueCallbackLifecycleIsMaximalAndExplicit(t *testing.T) {
	contract := mustSeal(t, Spec{})
	op, found := contract.Opaque()
	if !found || contract.CallbackCount(op) != 1 || contract.ValuesVarCount(op) != 0 {
		t.Fatalf("opaque callback inventory = %d callbacks/%d Values vars/%v", contract.CallbackCount(op), contract.ValuesVarCount(op), found)
	}
	id, idFound := contract.CallbackAt(op, 0)
	owner, ownerFound := contract.CallbackOwner(id)
	source, sourceFound := contract.CallbackFunction(id)
	lifecycle, lifecycleFound := contract.CallbackLifecycle(id)
	arguments, argumentsFound := contract.CallbackArguments(id)
	if !idFound || !ownerFound || owner != op || !sourceFound || source != (InputSource{Kind: InputSourceAllInputs}) ||
		!lifecycleFound || lifecycle != CallbackRetainedOptionalMany ||
		!argumentsFound {
		t.Fatalf("opaque callback = id:%d/%v owner:%d/%v source:%#v/%v lifecycle:%d/%v arguments:%d/%v",
			id, idFound, owner, ownerFound, source, sourceFound, lifecycle, lifecycleFound,
			arguments, argumentsFound)
	}
	input, inputOK := contract.Input(op)
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
