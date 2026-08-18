package target

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"testing"
)

const rejectedYieldMessage = "attempt to yield across a C-call boundary"

// protectedSubedgeOperation is the complete Target relation for a protected
// callback and its error handler. The only internal flow is explicit through
// role-keyed Subedges; callback Values equality is not used as a flow rule.
func protectedSubedgeOperation(name string, scalar, reverseCallbacks, reverseOutcomes bool) vocabulary.OperationSpec {
	protected, handler := vocabulary.CallbackRef(1), vocabulary.CallbackRef(2)
	handlerEdge := vocabulary.SubedgeRef(2)
	handlerArguments := callbackTail(2)
	if scalar {
		handlerArguments = vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed}
	}
	callbacks := []vocabulary.CallbackSpec{
		{
			Function: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0}, Admission: schematype.CallableAdmissionOrdinary,
			Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4),
			Lifecycle: vocabulary.CallbackSyncRequiredOnce, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		},
		{
			Function: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 1}, Admission: schematype.CallableAdmissionOrdinary,
			Arguments: handlerArguments, Outcomes: callbackOutcomes(5, 5, 6, 3, 4),
			Lifecycle: vocabulary.CallbackSyncOptionalOnce, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		},
	}
	if reverseCallbacks {
		callbacks[0], callbacks[1] = callbacks[1], callbacks[0]
		protected, handler = handler, protected
	}
	type namedOutcome struct {
		name string
		spec vocabulary.OutcomeSpec
	}
	outcomes := []namedOutcome{
		{"success", vocabulary.OutcomeSpec{Kind: flowkind.OutcomeNormal, Values: callbackTail(1)}},
		{"handler-return", vocabulary.OutcomeSpec{Kind: flowkind.OutcomeNormal, Values: callbackTail(5)}},
		{"handler-throw", vocabulary.OutcomeSpec{Kind: flowkind.OutcomeNormal, Values: callbackTail(6)}},
		{"yield", vocabulary.OutcomeSpec{Kind: flowkind.OutcomeYield, Values: callbackTail(3)}},
		{"cancel", vocabulary.OutcomeSpec{Kind: flowkind.OutcomeCancel, Values: callbackTail(4)}},
		{"admission-throw", vocabulary.OutcomeSpec{Kind: flowkind.OutcomeThrow, Values: callbackTail(2)}},
	}
	if reverseOutcomes {
		outcomes[0], outcomes[5] = outcomes[5], outcomes[0]
		outcomes[1], outcomes[4] = outcomes[4], outcomes[1]
	}
	ordinals := make(map[string]uint32, len(outcomes))
	specOutcomes := make([]vocabulary.OutcomeSpec, len(outcomes))
	for index, outcome := range outcomes {
		ordinals[outcome.name] = uint32(index)
		specOutcomes[index] = outcome.spec
	}
	protectedThrowResult := callbackTail(2)
	protectedThrowAdjustment := vocabulary.AdjustmentPreserve
	protectedThrowPlacement := vocabulary.PlacementTail
	if scalar {
		protectedThrowResult = handlerArguments
		protectedThrowAdjustment = vocabulary.AdjustmentExact
		protectedThrowPlacement = vocabulary.PlacementFixed
	}
	return vocabulary.OperationSpec{
		Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{name}}},
		ValuesVars: 7,
		Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny, testAny}, Tail: vocabulary.ValuesVariable, Var: 0},
		Callbacks:  callbacks,
		Subedges: []vocabulary.SubedgeSpec{
			{
				Role: 100, Family: vocabulary.SubedgeFamilyCall,
				Callee: vocabulary.SubedgeCalleeSpec{Kind: vocabulary.SubedgeCalleeCallback, Callback: protected},
				ArgumentOrigins: []vocabulary.ArgumentOrigin{{
					Segment: vocabulary.ArgumentTail, Kind: vocabulary.ArgumentSourceInput, Source: vocabulary.InputSource{Kind: vocabulary.InputSourceValuesVar, Ordinal: 0},
				}},
				AdmissionFailure: vocabulary.AdmissionFailureSpec{
					Values: callbackTail(2),
					Route:  vocabulary.AdmissionRouteSpec{Route: vocabulary.RouteSubedge, Adjustment: protectedThrowAdjustment, Result: protectedThrowResult, Placement: protectedThrowPlacement, Subedge: handlerEdge},
				},
				Routes: []vocabulary.SubedgeRouteSpec{
					{Kind: flowkind.OutcomeNormal, Route: vocabulary.RouteOutcome, Adjustment: vocabulary.AdjustmentPreserve, Result: callbackTail(1), Placement: vocabulary.PlacementTail, Outcome: ordinals["success"]},
					{Kind: flowkind.OutcomeReturn, Route: vocabulary.RouteOutcome, Adjustment: vocabulary.AdjustmentPreserve, Result: callbackTail(1), Placement: vocabulary.PlacementTail, Outcome: ordinals["success"]},
					{Kind: flowkind.OutcomeThrow, Route: vocabulary.RouteSubedge, Adjustment: protectedThrowAdjustment, Result: protectedThrowResult, Placement: protectedThrowPlacement, Subedge: handlerEdge},
					{Kind: flowkind.OutcomeYield, Route: vocabulary.RoutePropagateYield, Adjustment: vocabulary.AdjustmentPreserve, Result: callbackTail(3)},
					{Kind: flowkind.OutcomeCancel, Route: vocabulary.RouteOutcome, Adjustment: vocabulary.AdjustmentPreserve, Result: callbackTail(4), Placement: vocabulary.PlacementTail, Outcome: ordinals["cancel"]},
				},
			},
			{
				Role: 200, Family: vocabulary.SubedgeFamilyCall,
				Callee: vocabulary.SubedgeCalleeSpec{Kind: vocabulary.SubedgeCalleeCallback, Callback: handler},
				AdmissionFailure: vocabulary.AdmissionFailureSpec{
					Values: callbackTail(2),
					Route:  vocabulary.AdmissionRouteSpec{Route: vocabulary.RouteOutcome, Adjustment: vocabulary.AdjustmentPreserve, Result: callbackTail(2), Placement: vocabulary.PlacementTail, Outcome: ordinals["admission-throw"]},
				},
				Routes: []vocabulary.SubedgeRouteSpec{
					{Kind: flowkind.OutcomeNormal, Route: vocabulary.RouteOutcome, Adjustment: vocabulary.AdjustmentPreserve, Result: callbackTail(5), Placement: vocabulary.PlacementTail, Outcome: ordinals["handler-return"]},
					{Kind: flowkind.OutcomeReturn, Route: vocabulary.RouteOutcome, Adjustment: vocabulary.AdjustmentPreserve, Result: callbackTail(5), Placement: vocabulary.PlacementTail, Outcome: ordinals["handler-return"]},
					{Kind: flowkind.OutcomeThrow, Route: vocabulary.RouteOutcome, Adjustment: vocabulary.AdjustmentPreserve, Result: callbackTail(6), Placement: vocabulary.PlacementTail, Outcome: ordinals["handler-throw"]},
					{Kind: flowkind.OutcomeYield, Route: vocabulary.RoutePropagateYield, Adjustment: vocabulary.AdjustmentPreserve, Result: callbackTail(3)},
					{Kind: flowkind.OutcomeCancel, Route: vocabulary.RouteOutcome, Adjustment: vocabulary.AdjustmentPreserve, Result: callbackTail(4), Placement: vocabulary.PlacementTail, Outcome: ordinals["cancel"]},
				},
			},
		},
		Outcomes: specOutcomes,
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
}

func subedgeByRole(t *testing.T, c *Contract, op vocabulary.Operation, role uint32) vocabulary.SubedgeID {
	t.Helper()
	for index := 0; index < c.SubedgeCount(op); index++ {
		edge, ok := c.SubedgeAt(op, index)
		got, roleOK := c.subedgeRole(edge)
		if ok && roleOK && got == role {
			return edge
		}
	}
	t.Fatalf("missing subedge role %d", role)
	return 0
}

func TestSubedgeSealsExplicitTransportAndCanonicalRoles(t *testing.T) {
	left := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{protectedSubedgeOperation("protected-subedge", false, false, false)}})
	right := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{protectedSubedgeOperation("protected-subedge", false, true, true)}})
	assertPublicContractEqual(t, left, right)
	if left.ContentID() != right.ContentID() {
		t.Fatal("callback/outcome/subedge authoring permutation changed ContentID")
	}
	op, ok := left.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"protected-subedge"}})
	if !ok || left.SubedgeCount(op) != 2 {
		t.Fatalf("subedge count = %d/%v", left.SubedgeCount(op), ok)
	}
	protected := subedgeByRole(t, left, op, 100)
	handler := subedgeByRole(t, left, op, 200)
	route, adjustment, result, placement, offset, outcome, sibling, destination, routeOK := left.subedgeRouteAt(protected, flowkind.OutcomeThrow)
	if !routeOK || route != vocabulary.RouteSubedge || adjustment != vocabulary.AdjustmentPreserve || placement != vocabulary.PlacementTail || offset != 0 || outcome != 0 || sibling != handler || destination == 0 {
		t.Fatalf("protected Throw route = %d/%d/%d/%d/%d/%d/%d/%d/%v", route, adjustment, result, placement, offset, outcome, sibling, destination, routeOK)
	}
	arguments, argumentsOK := left.SubedgeArguments(handler)
	terminal, terminalOK := left.SubedgeTerminal(protected, flowkind.OutcomeThrow)
	if !argumentsOK || !terminalOK || result != terminal || destination != arguments {
		t.Fatalf("protected Throw endpoints = source:%d/%v result:%d destination:%d/%v", terminal, terminalOK, result, destination, argumentsOK)
	}
	if _, callbackOK := left.subedgeCallback(protected); !callbackOK {
		t.Fatal("callback-backed subedge lost its callback source")
	}
	if left.suspensionCount(op) != 0 {
		t.Fatal("PropagateYield fabricated an owner suspension")
	}
	for _, edge := range []vocabulary.SubedgeID{protected, handler} {
		route, adjustment, result, placement, offset, outcome, sibling, destination, found := left.subedgeRouteAt(edge, flowkind.OutcomeYield)
		if !found || route != vocabulary.RoutePropagateYield || adjustment != vocabulary.AdjustmentPreserve || result == 0 || placement != vocabulary.PlacementInvalid || offset != 0 || outcome != 0 || sibling != 0 || destination != 0 {
			t.Fatalf("subedge %d Yield route = %d/%d/%d/%d/%d/%d/%d/%d/%v", edge, route, adjustment, result, placement, offset, outcome, sibling, destination, found)
		}
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		if _, _, _, _, _, _, _, _, ok := left.subedgeRouteAt(handler, flowkind.OutcomeThrow); !ok {
			panic("subedge route disappeared")
		}
		if role, ok := left.subedgeRole(handler); !ok || role != 200 {
			panic("subedge role disappeared")
		}
	}); allocs != 0 {
		t.Fatalf("subedge queries allocated %f times", allocs)
	}
}

func TestSubedgeScalarAdjustmentHasItsOwnClosedResult(t *testing.T) {
	pack := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{protectedSubedgeOperation("protected-pack", false, false, false)}})
	scalar := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{protectedSubedgeOperation("protected-scalar", true, false, false)}})
	if pack.ContentID() == scalar.ContentID() {
		t.Fatal("pack and scalar adjustment share a content identity")
	}
	op, _ := scalar.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"protected-scalar"}})
	protected := subedgeByRole(t, scalar, op, 100)
	route, adjustment, result, placement, offset, _, _, destination, ok := scalar.subedgeRouteAt(protected, flowkind.OutcomeThrow)
	if !ok || route != vocabulary.RouteSubedge || adjustment != vocabulary.AdjustmentExact || placement != vocabulary.PlacementFixed || offset != 0 || result == 0 || result != destination {
		t.Fatalf("scalar route = %d/%d/%d/%d/%d/%d/%v", route, adjustment, result, placement, offset, destination, ok)
	}
	if count := scalar.ValuesCount(result); count != 1 {
		t.Fatalf("scalar result width = %d, want 1", count)
	}
	if tail, _, ok := scalar.ValuesTail(result); !ok || tail != vocabulary.ValuesClosed {
		t.Fatalf("scalar result tail = %d/%v", tail, ok)
	}
}

func TestSubedgeAdmissionFailureAndArgumentAuthorityAreExplicit(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{protectedSubedgeOperation("pcall-xpcall-admission", true, false, false)}})
	op, _ := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"pcall-xpcall-admission"}})
	protected := subedgeByRole(t, contract, op, 100)
	handler := subedgeByRole(t, contract, op, 200)

	failure, failureOK := contract.AdmissionFailure(protected)
	route, adjustment, result, placement, offset, outcome, sibling, destination, routeOK := contract.admissionRoute(protected)
	arguments, argumentsOK := contract.SubedgeArguments(handler)
	if !failureOK || !routeOK || failure == 0 || route != vocabulary.RouteSubedge || adjustment != vocabulary.AdjustmentExact ||
		result == 0 || placement != vocabulary.PlacementFixed || offset != 0 || outcome != 0 || sibling != handler ||
		destination != arguments || !argumentsOK {
		t.Fatalf("protected admission failure = %d/%v route:%d/%d/%d/%d/%d/%d/%d/%d/%v args:%d/%v",
			failure, failureOK, route, adjustment, result, placement, offset, outcome, sibling, destination, routeOK, arguments, argumentsOK)
	}
	if count := contract.argumentOriginCount(handler); count != 0 {
		t.Fatalf("route-fed handler has %d direct argument origins", count)
	}

	// Handler arguments are contextual even though the equivalent tail Values is
	// interned. Removing both real inbound routes must not manufacture an entry.
	orphaned := protectedSubedgeOperation("pcall-xpcall-orphan", true, false, false)
	for _, edge := range []*vocabulary.SubedgeSpec{&orphaned.Subedges[0], &orphaned.Subedges[1]} {
		if edge.Role != 100 {
			continue
		}
		edge.AdmissionFailure.Route = vocabulary.AdmissionRouteSpec{
			Route: vocabulary.RouteOutcome, Adjustment: vocabulary.AdjustmentPreserve, Result: callbackTail(2), Placement: vocabulary.PlacementTail, Outcome: 5,
		}
		edge.Routes[2] = vocabulary.SubedgeRouteSpec{
			Kind: flowkind.OutcomeThrow, Route: vocabulary.RouteOutcome, Adjustment: vocabulary.AdjustmentPreserve, Result: callbackTail(2), Placement: vocabulary.PlacementTail, Outcome: 5,
		}
	}
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{orphaned}}); err == nil {
		t.Fatal("equal Values handles created an implicit handler entry")
	}

	mixed := protectedSubedgeOperation("pcall-xpcall-mixed-entry", true, false, false)
	mixed.Subedges[1].ArgumentOrigins = []vocabulary.ArgumentOrigin{{Segment: vocabulary.ArgumentFixed, Kind: vocabulary.ArgumentSourceRule}}
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{mixed}}); err == nil {
		t.Fatal("route-fed handler accepted a parallel direct argument origin")
	}

	partial := protectedSubedgeOperation("pcall-xpcall-partial-entry", true, false, false)
	partial.Subedges[0].Routes[2].Result = vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{partial}}); err == nil {
		t.Fatal("partial route accepted as complete handler argument authority")
	}
}

func TestSubedgeNonCallFamiliesHaveClosedABIAndCanonicalOrigins(t *testing.T) {
	empty := vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}
	left := exactProjectionOperation("noncall-origin-order", empty, empty)
	left.Subedges[0].Family = vocabulary.SubedgeFamilyIndexGet
	left.Subedges[0].Arguments = vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny, testAny}, Tail: vocabulary.ValuesClosed}
	left.Subedges[0].ArgumentOrigins = []vocabulary.ArgumentOrigin{
		{Segment: vocabulary.ArgumentFixed, Index: 1, Kind: vocabulary.ArgumentSourceRule},
		{Segment: vocabulary.ArgumentFixed, Index: 0, Kind: vocabulary.ArgumentSourceRule},
	}
	right := left
	right.Subedges = append([]vocabulary.SubedgeSpec(nil), left.Subedges...)
	right.Subedges[0].ArgumentOrigins = append([]vocabulary.ArgumentOrigin(nil), left.Subedges[0].ArgumentOrigins...)
	right.Subedges[0].ArgumentOrigins[0], right.Subedges[0].ArgumentOrigins[1] = right.Subedges[0].ArgumentOrigins[1], right.Subedges[0].ArgumentOrigins[0]

	leftContract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{left}})
	rightContract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{right}})
	if leftContract.ContentID() != rightContract.ContentID() {
		t.Fatal("argument-origin authoring order changed ContentID")
	}
	op, _ := leftContract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"noncall-origin-order"}})
	edge := subedgeByRole(t, leftContract, op, 1)
	if count := leftContract.argumentOriginCount(edge); count != 2 {
		t.Fatalf("non-Call origin count = %d", count)
	}
	for index, want := range []uint32{0, 1} {
		segment, ordinal, source, input, ok := leftContract.ArgumentOriginAt(edge, index)
		if !ok || segment != vocabulary.ArgumentFixed || ordinal != want || source != vocabulary.ArgumentSourceRule || input != (vocabulary.InputSource{}) {
			t.Fatalf("origin %d = %d/%d/%d/%#v/%v", index, segment, ordinal, source, input, ok)
		}
	}

	invalid := left
	invalid.Subedges = append([]vocabulary.SubedgeSpec(nil), left.Subedges...)
	invalid.Subedges[0].Arguments = vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed}
	invalid.Subedges[0].ArgumentOrigins = []vocabulary.ArgumentOrigin{{Segment: vocabulary.ArgumentFixed, Kind: vocabulary.ArgumentSourceRule}}
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{invalid}}); err == nil {
		t.Fatal("IndexGet accepted a non-closed two-argument ABI")
	}
}

func TestSubedgeAdmissionFailureChangesContentIdentity(t *testing.T) {
	left := protectedSubedgeOperation("admission-content", false, false, false)
	right := protectedSubedgeOperation("admission-content", false, false, false)
	right.Subedges[0].AdmissionFailure.Route = vocabulary.AdmissionRouteSpec{
		Route: vocabulary.RouteOutcome, Adjustment: vocabulary.AdjustmentPreserve, Result: callbackTail(2), Placement: vocabulary.PlacementTail, Outcome: 5,
	}
	leftContract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{left}})
	rightContract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{right}})
	if leftContract.ContentID() == rightContract.ContentID() {
		t.Fatal("admission failure destination was omitted from ContentID")
	}
}

func TestSubedgeRejectsInvalidAuthority(t *testing.T) {
	for _, check := range []struct {
		name string
		edit func(*vocabulary.OperationSpec)
	}{
		{"zero role", func(op *vocabulary.OperationSpec) { op.Subedges[0].Role = 0 }},
		{"duplicate role", func(op *vocabulary.OperationSpec) { op.Subedges[1].Role = op.Subedges[0].Role }},
		{"duplicate callback subedge", func(op *vocabulary.OperationSpec) {
			extra := op.Subedges[0]
			extra.Role = 300
			op.Subedges = append(op.Subedges, extra)
		}},
		{"callback duplicate admission", func(op *vocabulary.OperationSpec) { op.Subedges[0].Admission = schematype.CallableAdmissionOrdinary }},
		{"tail placement changes variable", func(op *vocabulary.OperationSpec) { op.Callbacks[1].Arguments = callbackTail(5) }},
		{"propagated yield changes payload", func(op *vocabulary.OperationSpec) { op.Subedges[0].Routes[3].Result = callbackTail(4) }},
	} {
		t.Run(check.name, func(t *testing.T) {
			op := protectedSubedgeOperation("invalid-subedge", false, false, false)
			check.edit(&op)
			if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{op}}); err == nil {
				t.Fatal("invalid Subedge relation sealed")
			}
		})
	}
}

func TestSubedgeRejectYieldUsesTheExistingCanonicalThrow(t *testing.T) {
	op := protectedSubedgeOperation("reject-yield", false, false, false)
	op.Outcomes = append(op.Outcomes, vocabulary.OutcomeSpec{
		Kind:   flowkind.OutcomeThrow,
		Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testLiteralString(rejectedYieldMessage)}, Tail: vocabulary.ValuesClosed},
	})
	op.Subedges[0].Routes[3] = vocabulary.SubedgeRouteSpec{
		Kind: flowkind.OutcomeYield, Route: vocabulary.RouteRejectYield, Adjustment: vocabulary.AdjustmentExact,
		Result:    vocabulary.ValuesSpec{Fixed: []schematype.Type{testLiteralString(rejectedYieldMessage)}, Tail: vocabulary.ValuesClosed},
		Placement: vocabulary.PlacementFixed, Outcome: uint32(len(op.Outcomes) - 1),
	}
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{op}})
	sealed, _ := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"reject-yield"}})
	edge := subedgeByRole(t, contract, sealed, 100)
	route, adjustment, result, placement, offset, outcome, sibling, destination, ok := contract.subedgeRouteAt(edge, flowkind.OutcomeYield)
	if !ok || route != vocabulary.RouteRejectYield || adjustment != vocabulary.AdjustmentExact || result == 0 || placement != vocabulary.PlacementFixed || offset != 0 || sibling != 0 || destination == 0 {
		t.Fatalf("RejectYield = %d/%d/%d/%d/%d/%d/%d/%d/%v", route, adjustment, result, placement, offset, outcome, sibling, destination, ok)
	}
	kind, values, outcomeOK := contract.OutcomeAt(sealed, int(outcome))
	if !outcomeOK || kind != flowkind.OutcomeThrow || values != destination || contract.ValuesCount(destination) != 1 {
		t.Fatalf("RejectYield destination = %d/%d/%d/%v", kind, values, destination, outcomeOK)
	}
	invalid := protectedSubedgeOperation("reject-yield-invalid", false, false, false)
	invalid.Outcomes = append(invalid.Outcomes, vocabulary.OutcomeSpec{Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testLiteralString("wrong")}, Tail: vocabulary.ValuesClosed}})
	invalid.Subedges[0].Routes[3] = vocabulary.SubedgeRouteSpec{
		Kind: flowkind.OutcomeYield, Route: vocabulary.RouteRejectYield, Adjustment: vocabulary.AdjustmentExact,
		Result:    vocabulary.ValuesSpec{Fixed: []schematype.Type{testLiteralString(rejectedYieldMessage)}, Tail: vocabulary.ValuesClosed},
		Placement: vocabulary.PlacementFixed, Outcome: uint32(len(invalid.Outcomes) - 1),
	}
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{invalid}}); err == nil {
		t.Fatal("RejectYield accepted a noncanonical error payload")
	}
}

func xpcallHandlerSelfRecurrence(name string, lifecycle vocabulary.CallbackLifecycle, reverseCallbacks, reverseOutcomes bool) vocabulary.OperationSpec {
	op := protectedSubedgeOperation(name, true, reverseCallbacks, reverseOutcomes)
	// The handler is invoked with one scalar error. A handler Throw reinvokes the
	// same handler; a handler Yield crosses the C boundary, discards that Yield
	// payload, then enters the same handler with the canonical error string.
	for index := range op.Callbacks {
		if op.Callbacks[index].Function.Ordinal != 1 {
			continue
		}
		op.Callbacks[index].Admission = schematype.CallableAdmissionDirectFunction
		op.Callbacks[index].Lifecycle = lifecycle
		op.Callbacks[index].Outcomes[2].Values = vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed}
	}
	canonicalError := vocabulary.ValuesSpec{Fixed: []schematype.Type{testLiteralString(rejectedYieldMessage)}, Tail: vocabulary.ValuesClosed}
	op.Subedges[1].Routes[2] = vocabulary.SubedgeRouteSpec{
		Kind: flowkind.OutcomeThrow, Route: vocabulary.RouteSubedge, Adjustment: vocabulary.AdjustmentExact,
		Result: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed}, Placement: vocabulary.PlacementFixed, Subedge: 2,
	}
	op.Subedges[1].Routes[3] = vocabulary.SubedgeRouteSpec{
		Kind: flowkind.OutcomeYield, Route: vocabulary.RouteRejectYield, Adjustment: vocabulary.AdjustmentExact,
		Result: canonicalError, Placement: vocabulary.PlacementFixed, Subedge: 2,
	}
	return op
}

func xpcallHandlerMultiEdgeRecurrence(name string, lifecycle vocabulary.CallbackLifecycle) vocabulary.OperationSpec {
	op := protectedSubedgeOperation(name, true, false, false)
	for index := range op.Callbacks {
		if op.Callbacks[index].Function.Ordinal != 1 {
			continue
		}
		op.Callbacks[index].Admission = schematype.CallableAdmissionDirectFunction
		op.Callbacks[index].Lifecycle = lifecycle
		op.Callbacks[index].Outcomes[2].Values = vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed}
	}
	empty := vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}
	any := vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed}
	op.Subedges[1].Routes[2] = vocabulary.SubedgeRouteSpec{
		Kind: flowkind.OutcomeThrow, Route: vocabulary.RouteSubedge, Adjustment: vocabulary.AdjustmentExact,
		Result: any, Placement: vocabulary.PlacementFixed, Subedge: 3,
	}
	op.Subedges = append(op.Subedges, vocabulary.SubedgeSpec{
		Role:      300,
		Family:    vocabulary.SubedgeFamilyLength,
		Admission: schematype.CallableAdmissionOrdinary,
		Arguments: any,
		Outcomes: []vocabulary.TerminalSpec{
			{Kind: flowkind.OutcomeNormal, Values: empty},
			{Kind: flowkind.OutcomeReturn, Values: empty},
			{Kind: flowkind.OutcomeThrow, Values: any},
			{Kind: flowkind.OutcomeYield, Values: empty},
			{Kind: flowkind.OutcomeCancel, Values: empty},
		},
		AdmissionFailure: vocabulary.AdmissionFailureSpec{
			Values: callbackTail(2),
			Route:  vocabulary.AdmissionRouteSpec{Route: vocabulary.RouteOutcome, Adjustment: vocabulary.AdjustmentPreserve, Result: callbackTail(2), Placement: vocabulary.PlacementTail, Outcome: 5},
		},
		Routes: []vocabulary.SubedgeRouteSpec{
			{Kind: flowkind.OutcomeNormal, Route: vocabulary.RouteContinue, Adjustment: vocabulary.AdjustmentExact, Result: empty},
			{Kind: flowkind.OutcomeReturn, Route: vocabulary.RouteContinue, Adjustment: vocabulary.AdjustmentExact, Result: empty},
			{Kind: flowkind.OutcomeThrow, Route: vocabulary.RouteSubedge, Adjustment: vocabulary.AdjustmentPreserve, Result: any, Placement: vocabulary.PlacementFixed, Subedge: 2},
			{Kind: flowkind.OutcomeYield, Route: vocabulary.RoutePropagateYield, Adjustment: vocabulary.AdjustmentPreserve, Result: empty},
			{Kind: flowkind.OutcomeCancel, Route: vocabulary.RouteContinue, Adjustment: vocabulary.AdjustmentExact, Result: empty},
		},
	})
	return op
}

func nullarySubedge(role uint32, successor vocabulary.SubedgeRef, ruleEntry bool) vocabulary.SubedgeSpec {
	empty := vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}
	return vocabulary.SubedgeSpec{
		Role: role, Family: vocabulary.SubedgeFamilyCall,
		Callee:    vocabulary.SubedgeCalleeSpec{Kind: vocabulary.SubedgeCalleeMetaKey, MetaKey: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__call"}},
		Admission: schematype.CallableAdmissionOrdinary,
		RuleEntry: ruleEntry,
		Arguments: empty,
		Outcomes: []vocabulary.TerminalSpec{
			{Kind: flowkind.OutcomeNormal, Values: empty},
			{Kind: flowkind.OutcomeReturn, Values: empty},
			{Kind: flowkind.OutcomeThrow, Values: empty},
			{Kind: flowkind.OutcomeYield, Values: empty},
			{Kind: flowkind.OutcomeCancel, Values: empty},
		},
		AdmissionFailure: vocabulary.AdmissionFailureSpec{
			Values: empty,
			Route:  vocabulary.AdmissionRouteSpec{Route: vocabulary.RouteOutcome, Adjustment: vocabulary.AdjustmentPreserve, Result: empty, Placement: vocabulary.PlacementFixed, Outcome: 0},
		},
		Routes: []vocabulary.SubedgeRouteSpec{
			{Kind: flowkind.OutcomeNormal, Route: vocabulary.RouteSubedge, Adjustment: vocabulary.AdjustmentExact, Result: empty, Placement: vocabulary.PlacementFixed, Subedge: successor},
			{Kind: flowkind.OutcomeReturn, Route: vocabulary.RouteContinue, Adjustment: vocabulary.AdjustmentExact, Result: empty},
			{Kind: flowkind.OutcomeThrow, Route: vocabulary.RouteContinue, Adjustment: vocabulary.AdjustmentExact, Result: empty},
			{Kind: flowkind.OutcomeYield, Route: vocabulary.RoutePropagateYield, Adjustment: vocabulary.AdjustmentPreserve, Result: empty},
			{Kind: flowkind.OutcomeCancel, Route: vocabulary.RouteContinue, Adjustment: vocabulary.AdjustmentExact, Result: empty},
		},
	}
}

func nullarySubedgeOperation(name string, subedges ...vocabulary.SubedgeSpec) vocabulary.OperationSpec {
	return vocabulary.OperationSpec{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{name}}},
		Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
		Subedges: subedges,
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
}

func nullaryCallbackMuOperation(name string, lifecycle vocabulary.CallbackLifecycle) vocabulary.OperationSpec {
	empty := vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}
	return vocabulary.OperationSpec{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{name}}},
		Input:    vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed},
		Callbacks: []vocabulary.CallbackSpec{{
			Function:  vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal},
			Admission: schematype.CallableAdmissionOrdinary,
			Arguments: empty,
			Outcomes: []vocabulary.TerminalSpec{
				{Kind: flowkind.OutcomeNormal, Values: empty},
				{Kind: flowkind.OutcomeReturn, Values: empty},
				{Kind: flowkind.OutcomeThrow, Values: empty},
				{Kind: flowkind.OutcomeYield, Values: empty},
				{Kind: flowkind.OutcomeCancel, Values: empty},
			},
			Lifecycle: lifecycle,
			Effects:   vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: empty}},
		Subedges: []vocabulary.SubedgeSpec{{
			Role:      10,
			Family:    vocabulary.SubedgeFamilyCall,
			Callee:    vocabulary.SubedgeCalleeSpec{Kind: vocabulary.SubedgeCalleeCallback, Callback: 1},
			RuleEntry: true,
			AdmissionFailure: vocabulary.AdmissionFailureSpec{
				Values: empty,
				Route:  vocabulary.AdmissionRouteSpec{Route: vocabulary.RouteOutcome, Adjustment: vocabulary.AdjustmentPreserve, Result: empty, Placement: vocabulary.PlacementFixed, Outcome: 0},
			},
			Routes: []vocabulary.SubedgeRouteSpec{
				{Kind: flowkind.OutcomeNormal, Route: vocabulary.RouteSubedge, Adjustment: vocabulary.AdjustmentExact, Result: empty, Placement: vocabulary.PlacementFixed, Subedge: 1},
				{Kind: flowkind.OutcomeReturn, Route: vocabulary.RouteContinue, Adjustment: vocabulary.AdjustmentExact, Result: empty},
				{Kind: flowkind.OutcomeThrow, Route: vocabulary.RouteContinue, Adjustment: vocabulary.AdjustmentExact, Result: empty},
				{Kind: flowkind.OutcomeYield, Route: vocabulary.RoutePropagateYield, Adjustment: vocabulary.AdjustmentPreserve, Result: empty},
				{Kind: flowkind.OutcomeCancel, Route: vocabulary.RouteContinue, Adjustment: vocabulary.AdjustmentExact, Result: empty},
			},
		}},
		Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
}

func TestSubedgeOnceCallbackRejectsReachableRecurrence(t *testing.T) {
	for _, test := range []struct {
		name      string
		lifecycle vocabulary.CallbackLifecycle
	}{
		{"sync", vocabulary.CallbackSyncOptionalOnce},
		{"retained", vocabulary.CallbackRetainedOptionalOnce},
	} {
		t.Run(test.name, func(t *testing.T) {
			op := xpcallHandlerSelfRecurrence("xpcall-once", test.lifecycle, false, false)
			if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{op}}); err == nil {
				t.Fatal("Once callback sealed despite reachable self-reentry")
			}
		})
	}
}

func TestSubedgeNullaryCallbackMuRespectsLifecycleMultiplicity(t *testing.T) {
	once := nullaryCallbackMuOperation("nullary-callback-once", vocabulary.CallbackSyncOptionalOnce)
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{once}}); err == nil {
		t.Fatal("nullary Once callback Mu head sealed")
	}
	many := nullaryCallbackMuOperation("nullary-callback-many", vocabulary.CallbackSyncOptionalMany)
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{many}})
	op, _ := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"nullary-callback-many"}})
	edge := subedgeByRole(t, contract, op, 10)
	callback, callbackOK := contract.subedgeCallback(edge)
	if lifecycle, lifecycleOK := contract.CallbackLifecycle(callback); !callbackOK || !lifecycleOK || lifecycle != vocabulary.CallbackSyncOptionalMany {
		t.Fatalf("nullary callback lifecycle = %d/%v/%v", lifecycle, callbackOK, lifecycleOK)
	}
}

func TestSubedgeOnceCallbackRejectsReachableMultiEdgeRecurrence(t *testing.T) {
	op := xpcallHandlerMultiEdgeRecurrence("xpcall-multi-once", vocabulary.CallbackSyncOptionalOnce)
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{op}}); err == nil {
		t.Fatal("Once callback sealed despite reachable multi-edge reentry")
	}
}

func TestSubedgeXPCALLHandlerSelfRecurrenceSealsIteratively(t *testing.T) {
	op := xpcallHandlerSelfRecurrence("xpcall-self", vocabulary.CallbackSyncOptionalMany, false, false)
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{op}})
	sealed, _ := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"xpcall-self"}})
	handler := subedgeByRole(t, contract, sealed, 200)
	if admission, ok := contract.subedgeAdmission(handler); !ok || admission != schematype.CallableAdmissionDirectFunction {
		t.Fatalf("xpcall handler admission = %d/%v, want schematype.CallableAdmissionDirectFunction", admission, ok)
	}
	callback, callbackOK := contract.subedgeCallback(handler)
	if lifecycle, ok := contract.CallbackLifecycle(callback); !callbackOK || !ok || lifecycle != vocabulary.CallbackSyncOptionalMany {
		t.Fatalf("xpcall recursive handler lifecycle = %d/%v/%v, want SyncOptionalMany", lifecycle, callbackOK, ok)
	}
	for _, kind := range []flowkind.OutcomeKind{flowkind.OutcomeThrow, flowkind.OutcomeYield} {
		route, adjustment, result, placement, offset, outcome, sibling, destination, ok := contract.subedgeRouteAt(handler, kind)
		if !ok || adjustment != vocabulary.AdjustmentExact || result == 0 || placement != vocabulary.PlacementFixed || offset != 0 || outcome != 0 || sibling != handler || destination == 0 {
			t.Fatalf("handler %d recurrence = %d/%d/%d/%d/%d/%d/%d/%d/%v", kind, route, adjustment, result, placement, offset, outcome, sibling, destination, ok)
		}
		if kind == flowkind.OutcomeThrow && route != vocabulary.RouteSubedge {
			t.Fatalf("handler Throw route = %d, want Subedge", route)
		}
		if kind == flowkind.OutcomeYield && route != vocabulary.RouteRejectYield {
			t.Fatalf("handler Yield route = %d, want RejectYield", route)
		}
	}
}

func TestSubedgeRecurrenceManyLifecycleIsCanonicalUnderAuthoringPermutation(t *testing.T) {
	left := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{xpcallHandlerSelfRecurrence("xpcall-many-permutation", vocabulary.CallbackSyncOptionalMany, false, false)}})
	right := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{xpcallHandlerSelfRecurrence("xpcall-many-permutation", vocabulary.CallbackSyncOptionalMany, true, true)}})
	assertPublicContractEqual(t, left, right)
	if left.ContentID() != right.ContentID() {
		t.Fatal("recurrent Many callback authoring permutation changed ContentID")
	}
}

func TestSubedgeReachabilityRequiresAnExplicitRoot(t *testing.T) {
	closed := nullarySubedgeOperation("closed-route-fed", nullarySubedge(10, 2, false), nullarySubedge(20, 1, false))
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{closed}}); err == nil {
		t.Fatal("closed route-fed Subedge SCC sealed without an executable root")
	}

	direct := nullarySubedgeOperation("nullary-rule-entry", nullarySubedge(10, 1, true))
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{direct}})
	op, _ := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"nullary-rule-entry"}})
	edge := subedgeByRole(t, contract, op, 10)
	if ruleEntry, ok := contract.subedgeRuleEntry(edge); !ok || !ruleEntry {
		t.Fatalf("nullary RuleEntry = %v/%v", ruleEntry, ok)
	}
	if !contract.ContentID().Available() {
		t.Fatal("nullary RuleEntry Mu head has no ContentID")
	}

	missing := nullarySubedgeOperation("nullary-missing-entry", nullarySubedge(10, 1, false))
	missing.Subedges[0].Routes[0] = vocabulary.SubedgeRouteSpec{
		Kind: flowkind.OutcomeNormal, Route: vocabulary.RouteOutcome, Adjustment: vocabulary.AdjustmentExact,
		Result: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Placement: vocabulary.PlacementFixed, Outcome: 0,
	}
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{missing}}); err == nil {
		t.Fatal("nullary Subedge sealed without RuleEntry or an inbound route")
	}

	nonempty := exactProjectionOperation("nonempty-rule-entry", vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed})
	nonempty.Subedges[0].RuleEntry = true
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{nonempty}}); err == nil {
		t.Fatal("RuleEntry accepted a nonempty argument product")
	}
}

func TestOpaqueCallbackIsExplicitlyConservative(t *testing.T) {
	contract := mustSeal(t, Spec{})
	op, ok := contract.Opaque()
	callback, callbackOK := contract.CallbackAt(op, 0)
	if !ok || !callbackOK || !contract.callbackOpaque(callback) || contract.SubedgeCount(op) != 0 {
		t.Fatalf("opaque callback = op:%d/%v callback:%d/%v opaque:%v subedges:%d", op, ok, callback, callbackOK, contract.callbackOpaque(callback), contract.SubedgeCount(op))
	}
	arguments, argumentsOK := contract.CallbackArguments(callback)
	input, inputOK := contract.Input(op)
	if !argumentsOK || !inputOK || arguments != input {
		t.Fatalf("opaque callback arguments = %d/%v input=%d/%v", arguments, argumentsOK, input, inputOK)
	}
	for _, kind := range []flowkind.OutcomeKind{flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow, flowkind.OutcomeYield, flowkind.OutcomeCancel} {
		values, found := contract.CallbackOutcome(callback, kind)
		if !found || values != input {
			t.Fatalf("opaque callback terminal %d = %d/%v", kind, values, found)
		}
	}
}

func TestSubedgeFreezeResolvesAuthoredEdgesToDenseIDs(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{protectedSubedgeOperation("freeze-edge", false, false, false)}})
	op, ok := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"freeze-edge"}})
	if !ok || contract.SubedgeCount(op) == 0 {
		t.Fatalf("subedge owner = %d/%v count=%d", op, ok, contract.SubedgeCount(op))
	}
	edge, ok := contract.SubedgeAt(op, 0)
	if !ok || edge == 0 {
		t.Fatalf("SubedgeAt = %d/%v", edge, ok)
	}
	if owner, ok := contract.subedgeOwner(edge); !ok || owner != op {
		t.Fatalf("SubedgeOwner = %d/%v, want %d/true", owner, ok, op)
	}
}

func TestSubedgeRowsPreserveCanonicalRoleAndFamily(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{protectedSubedgeOperation("row-edge", false, false, false)}})
	op, _ := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"row-edge"}})
	edge, ok := contract.SubedgeAt(op, 0)
	if !ok {
		t.Fatal("subedge row missing")
	}
	if role, ok := contract.subedgeRole(edge); !ok || role == 0 {
		t.Fatalf("SubedgeRole = %d/%v", role, ok)
	}
	if family, ok := contract.SubedgeFamily(edge); !ok || family != vocabulary.SubedgeFamilyCall {
		t.Fatalf("SubedgeFamily = %d/%v", family, ok)
	}
}

func TestSubedgeValidationRejectsInvalidFamilyAuthority(t *testing.T) {
	spec := Spec{Operations: []vocabulary.OperationSpec{protectedSubedgeOperation("invalid-edge", false, false, false)}}
	spec.Operations[0].Subedges[0].Family = vocabulary.SubedgeFamilyInvalid
	if _, err := testSeal(&spec); err == nil {
		t.Fatal("invalid subedge family was accepted")
	}
}

func TestOperationSubedgeRelationProjectsOnlySealedCoordinates(t *testing.T) {
	operation := protectedSubedgeOperation("neutral-subedge-relation", false, false, false)
	for index := range operation.Outcomes {
		operation.Outcomes[index].Values.Fixed = []schematype.Type{testAny}
	}
	operation.SubedgeRelation = &vocabulary.SubedgeRelationSpec{
		Operand: 1, Selector: 37, Subedge: 1, ResultOutcome: 0, Result: 0,
	}
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{operation}})
	op, ok := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"neutral-subedge-relation"}})
	if !ok {
		t.Fatal("operation missing")
	}
	operand, selector, subedge, outcome, result, ok := contract.OperationSubedgeRelation(op)
	if !ok || operand != 1 || selector != 37 || subedge == 0 || result != 0 {
		t.Fatalf("relation = %d/%d/%d/%d/%d/%v", operand, selector, subedge, outcome, result, ok)
	}
	if got, ok := contract.OperationSubedgeRelationOutcome(op, flowkind.OutcomeNormal); !ok || got != outcome {
		t.Fatalf("normal outcome = %d/%v", got, ok)
	}
	if count := contract.OperationSubedgeRelationEffectAliasCount(op); count != 0 {
		t.Fatalf("effect aliases = %d, want 0", count)
	}
}

func TestSealSubedgeRelationRejectsForeignCoordinates(t *testing.T) {
	valid := func() vocabulary.OperationSpec {
		operation := protectedSubedgeOperation("neutral-subedge-relation-seal", false, false, false)
		for index := range operation.Outcomes {
			operation.Outcomes[index].Values.Fixed = []schematype.Type{testAny}
		}
		operation.SubedgeRelation = &vocabulary.SubedgeRelationSpec{
			Operand: 1, Selector: 1, Subedge: 1, ResultOutcome: 0, Result: 0,
		}
		return operation
	}
	tests := []struct {
		name   string
		mutate func(*vocabulary.SubedgeRelationSpec)
	}{
		{"operand", func(row *vocabulary.SubedgeRelationSpec) { row.Operand = 2 }},
		{"subedge", func(row *vocabulary.SubedgeRelationSpec) { row.Subedge = 3 }},
		{"outcome", func(row *vocabulary.SubedgeRelationSpec) { row.ResultOutcome = 6 }},
		{"result", func(row *vocabulary.SubedgeRelationSpec) { row.Result = 1 }},
		{"effect", func(row *vocabulary.SubedgeRelationSpec) { row.EffectAliases = []uint32{0} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			operation := valid()
			test.mutate(operation.SubedgeRelation)
			if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{operation}}); err == nil {
				t.Fatal("foreign coordinate admitted")
			}
		})
	}
}
