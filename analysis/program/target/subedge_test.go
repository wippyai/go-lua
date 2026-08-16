package target

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// protectedSubedgeOperation is the complete Target relation for a protected
// callback and its error handler. The only internal flow is explicit through
// role-keyed Subedges; callback Values equality is not used as a flow rule.
func protectedSubedgeOperation(name string, scalar, reverseCallbacks, reverseOutcomes bool) OperationSpec {
	protected, handler := CallbackRef(1), CallbackRef(2)
	handlerEdge := SubedgeRef(2)
	handlerArguments := callbackTail(2)
	if scalar {
		handlerArguments = ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesClosed}
	}
	callbacks := []CallbackSpec{
		{
			Function: InputSource{Kind: InputSourceValueFormal, Ordinal: 0}, Admission: OrdinaryCallable,
			Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4),
			Lifecycle: CallbackSyncRequiredOnce, Effects: RowSpec{Tail: RowClosed},
		},
		{
			Function: InputSource{Kind: InputSourceValueFormal, Ordinal: 1}, Admission: OrdinaryCallable,
			Arguments: handlerArguments, Outcomes: callbackOutcomes(5, 5, 6, 3, 4),
			Lifecycle: CallbackSyncOptionalOnce, Effects: RowSpec{Tail: RowClosed},
		},
	}
	if reverseCallbacks {
		callbacks[0], callbacks[1] = callbacks[1], callbacks[0]
		protected, handler = handler, protected
	}
	type namedOutcome struct {
		name string
		spec OutcomeSpec
	}
	outcomes := []namedOutcome{
		{"success", OutcomeSpec{Kind: flowkind.OutcomeNormal, Values: callbackTail(1)}},
		{"handler-return", OutcomeSpec{Kind: flowkind.OutcomeNormal, Values: callbackTail(5)}},
		{"handler-throw", OutcomeSpec{Kind: flowkind.OutcomeNormal, Values: callbackTail(6)}},
		{"yield", OutcomeSpec{Kind: flowkind.OutcomeYield, Values: callbackTail(3)}},
		{"cancel", OutcomeSpec{Kind: flowkind.OutcomeCancel, Values: callbackTail(4)}},
		{"admission-throw", OutcomeSpec{Kind: flowkind.OutcomeThrow, Values: callbackTail(2)}},
	}
	if reverseOutcomes {
		outcomes[0], outcomes[5] = outcomes[5], outcomes[0]
		outcomes[1], outcomes[4] = outcomes[4], outcomes[1]
	}
	ordinals := make(map[string]uint32, len(outcomes))
	specOutcomes := make([]OutcomeSpec, len(outcomes))
	for index, outcome := range outcomes {
		ordinals[outcome.name] = uint32(index)
		specOutcomes[index] = outcome.spec
	}
	protectedThrowResult := callbackTail(2)
	protectedThrowAdjustment := AdjustmentPreserve
	protectedThrowPlacement := PlacementTail
	if scalar {
		protectedThrowResult = handlerArguments
		protectedThrowAdjustment = AdjustmentExact
		protectedThrowPlacement = PlacementFixed
	}
	return OperationSpec{
		Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}},
		ValuesVars: 7,
		Input:      ValuesSpec{Fixed: []typ.Type{typ.Any, typ.Any}, Tail: ValuesVariable, Var: 0},
		Callbacks:  callbacks,
		Subedges: []SubedgeSpec{
			{
				Role: 100, Family: SubedgeFamilyCall,
				Callee: SubedgeCalleeSpec{Kind: SubedgeCalleeCallback, Callback: protected},
				ArgumentOrigins: []ArgumentOrigin{{
					Segment: ArgumentTail, Kind: ArgumentSourceInput, Source: InputSource{Kind: InputSourceValuesVar, Ordinal: 0},
				}},
				AdmissionFailure: AdmissionFailureSpec{
					Values: callbackTail(2),
					Route:  AdmissionRouteSpec{Route: RouteSubedge, Adjustment: protectedThrowAdjustment, Result: protectedThrowResult, Placement: protectedThrowPlacement, Subedge: handlerEdge},
				},
				Routes: []SubedgeRouteSpec{
					{Kind: flowkind.OutcomeNormal, Route: RouteOutcome, Adjustment: AdjustmentPreserve, Result: callbackTail(1), Placement: PlacementTail, Outcome: ordinals["success"]},
					{Kind: flowkind.OutcomeReturn, Route: RouteOutcome, Adjustment: AdjustmentPreserve, Result: callbackTail(1), Placement: PlacementTail, Outcome: ordinals["success"]},
					{Kind: flowkind.OutcomeThrow, Route: RouteSubedge, Adjustment: protectedThrowAdjustment, Result: protectedThrowResult, Placement: protectedThrowPlacement, Subedge: handlerEdge},
					{Kind: flowkind.OutcomeYield, Route: RoutePropagateYield, Adjustment: AdjustmentPreserve, Result: callbackTail(3)},
					{Kind: flowkind.OutcomeCancel, Route: RouteOutcome, Adjustment: AdjustmentPreserve, Result: callbackTail(4), Placement: PlacementTail, Outcome: ordinals["cancel"]},
				},
			},
			{
				Role: 200, Family: SubedgeFamilyCall,
				Callee: SubedgeCalleeSpec{Kind: SubedgeCalleeCallback, Callback: handler},
				AdmissionFailure: AdmissionFailureSpec{
					Values: callbackTail(2),
					Route:  AdmissionRouteSpec{Route: RouteOutcome, Adjustment: AdjustmentPreserve, Result: callbackTail(2), Placement: PlacementTail, Outcome: ordinals["admission-throw"]},
				},
				Routes: []SubedgeRouteSpec{
					{Kind: flowkind.OutcomeNormal, Route: RouteOutcome, Adjustment: AdjustmentPreserve, Result: callbackTail(5), Placement: PlacementTail, Outcome: ordinals["handler-return"]},
					{Kind: flowkind.OutcomeReturn, Route: RouteOutcome, Adjustment: AdjustmentPreserve, Result: callbackTail(5), Placement: PlacementTail, Outcome: ordinals["handler-return"]},
					{Kind: flowkind.OutcomeThrow, Route: RouteOutcome, Adjustment: AdjustmentPreserve, Result: callbackTail(6), Placement: PlacementTail, Outcome: ordinals["handler-throw"]},
					{Kind: flowkind.OutcomeYield, Route: RoutePropagateYield, Adjustment: AdjustmentPreserve, Result: callbackTail(3)},
					{Kind: flowkind.OutcomeCancel, Route: RouteOutcome, Adjustment: AdjustmentPreserve, Result: callbackTail(4), Placement: PlacementTail, Outcome: ordinals["cancel"]},
				},
			},
		},
		Outcomes: specOutcomes,
		Effects:  RowSpec{Tail: RowClosed},
	}
}

func subedgeByRole(t *testing.T, c *Contract, op Operation, role uint32) SubedgeID {
	t.Helper()
	for index := 0; index < c.SubedgeCount(op); index++ {
		edge, ok := c.SubedgeAt(op, index)
		got, roleOK := c.SubedgeRole(edge)
		if ok && roleOK && got == role {
			return edge
		}
	}
	t.Fatalf("missing subedge role %d", role)
	return 0
}

func TestSubedgeSealsExplicitTransportAndCanonicalRoles(t *testing.T) {
	left := mustSeal(t, Spec{Operations: []OperationSpec{protectedSubedgeOperation("protected-subedge", false, false, false)}})
	right := mustSeal(t, Spec{Operations: []OperationSpec{protectedSubedgeOperation("protected-subedge", false, true, true)}})
	assertPublicContractEqual(t, left, right)
	if left.ContentID() != right.ContentID() {
		t.Fatal("callback/outcome/subedge authoring permutation changed ContentID")
	}
	op, ok := left.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"protected-subedge"}})
	if !ok || left.SubedgeCount(op) != 2 {
		t.Fatalf("subedge count = %d/%v", left.SubedgeCount(op), ok)
	}
	protected := subedgeByRole(t, left, op, 100)
	handler := subedgeByRole(t, left, op, 200)
	route, adjustment, result, placement, offset, outcome, sibling, destination, routeOK := left.SubedgeRouteAt(protected, flowkind.OutcomeThrow)
	if !routeOK || route != RouteSubedge || adjustment != AdjustmentPreserve || placement != PlacementTail || offset != 0 || outcome != 0 || sibling != handler || destination == 0 {
		t.Fatalf("protected Throw route = %d/%d/%d/%d/%d/%d/%d/%d/%v", route, adjustment, result, placement, offset, outcome, sibling, destination, routeOK)
	}
	arguments, argumentsOK := left.SubedgeArguments(handler)
	terminal, terminalOK := left.SubedgeTerminal(protected, flowkind.OutcomeThrow)
	if !argumentsOK || !terminalOK || result != terminal || destination != arguments {
		t.Fatalf("protected Throw endpoints = source:%d/%v result:%d destination:%d/%v", terminal, terminalOK, result, destination, argumentsOK)
	}
	if _, callbackOK := left.SubedgeCallback(protected); !callbackOK {
		t.Fatal("callback-backed subedge lost its callback source")
	}
	if left.SuspensionCount(op) != 0 {
		t.Fatal("PropagateYield fabricated an owner suspension")
	}
	for _, edge := range []SubedgeID{protected, handler} {
		route, adjustment, result, placement, offset, outcome, sibling, destination, found := left.SubedgeRouteAt(edge, flowkind.OutcomeYield)
		if !found || route != RoutePropagateYield || adjustment != AdjustmentPreserve || result == 0 || placement != PlacementInvalid || offset != 0 || outcome != 0 || sibling != 0 || destination != 0 {
			t.Fatalf("subedge %d Yield route = %d/%d/%d/%d/%d/%d/%d/%d/%v", edge, route, adjustment, result, placement, offset, outcome, sibling, destination, found)
		}
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		if _, _, _, _, _, _, _, _, ok := left.SubedgeRouteAt(handler, flowkind.OutcomeThrow); !ok {
			panic("subedge route disappeared")
		}
		if role, ok := left.SubedgeRole(handler); !ok || role != 200 {
			panic("subedge role disappeared")
		}
	}); allocs != 0 {
		t.Fatalf("subedge queries allocated %f times", allocs)
	}
}

func TestSubedgeScalarAdjustmentHasItsOwnClosedResult(t *testing.T) {
	pack := mustSeal(t, Spec{Operations: []OperationSpec{protectedSubedgeOperation("protected-pack", false, false, false)}})
	scalar := mustSeal(t, Spec{Operations: []OperationSpec{protectedSubedgeOperation("protected-scalar", true, false, false)}})
	if pack.ContentID() == scalar.ContentID() {
		t.Fatal("pack and scalar adjustment share a content identity")
	}
	op, _ := scalar.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"protected-scalar"}})
	protected := subedgeByRole(t, scalar, op, 100)
	route, adjustment, result, placement, offset, _, _, destination, ok := scalar.SubedgeRouteAt(protected, flowkind.OutcomeThrow)
	if !ok || route != RouteSubedge || adjustment != AdjustmentExact || placement != PlacementFixed || offset != 0 || result == 0 || result != destination {
		t.Fatalf("scalar route = %d/%d/%d/%d/%d/%d/%v", route, adjustment, result, placement, offset, destination, ok)
	}
	if count := scalar.ValuesCount(result); count != 1 {
		t.Fatalf("scalar result width = %d, want 1", count)
	}
	if tail, _, ok := scalar.ValuesTail(result); !ok || tail != ValuesClosed {
		t.Fatalf("scalar result tail = %d/%v", tail, ok)
	}
}

func TestSubedgeAdmissionFailureAndArgumentAuthorityAreExplicit(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{protectedSubedgeOperation("pcall-xpcall-admission", true, false, false)}})
	op, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"pcall-xpcall-admission"}})
	protected := subedgeByRole(t, contract, op, 100)
	handler := subedgeByRole(t, contract, op, 200)

	failure, failureOK := contract.AdmissionFailure(protected)
	route, adjustment, result, placement, offset, outcome, sibling, destination, routeOK := contract.AdmissionRoute(protected)
	arguments, argumentsOK := contract.SubedgeArguments(handler)
	if !failureOK || !routeOK || failure == 0 || route != RouteSubedge || adjustment != AdjustmentExact ||
		result == 0 || placement != PlacementFixed || offset != 0 || outcome != 0 || sibling != handler ||
		destination != arguments || !argumentsOK {
		t.Fatalf("protected admission failure = %d/%v route:%d/%d/%d/%d/%d/%d/%d/%d/%v args:%d/%v",
			failure, failureOK, route, adjustment, result, placement, offset, outcome, sibling, destination, routeOK, arguments, argumentsOK)
	}
	if count := contract.ArgumentOriginCount(handler); count != 0 {
		t.Fatalf("route-fed handler has %d direct argument origins", count)
	}

	// Handler arguments are contextual even though the equivalent tail Values is
	// interned. Removing both real inbound routes must not manufacture an entry.
	orphaned := protectedSubedgeOperation("pcall-xpcall-orphan", true, false, false)
	for _, edge := range []*SubedgeSpec{&orphaned.Subedges[0], &orphaned.Subedges[1]} {
		if edge.Role != 100 {
			continue
		}
		edge.AdmissionFailure.Route = AdmissionRouteSpec{
			Route: RouteOutcome, Adjustment: AdjustmentPreserve, Result: callbackTail(2), Placement: PlacementTail, Outcome: 5,
		}
		edge.Routes[2] = SubedgeRouteSpec{
			Kind: flowkind.OutcomeThrow, Route: RouteOutcome, Adjustment: AdjustmentPreserve, Result: callbackTail(2), Placement: PlacementTail, Outcome: 5,
		}
	}
	if _, err := Seal(&Spec{Operations: []OperationSpec{orphaned}}); err == nil {
		t.Fatal("equal Values handles created an implicit handler entry")
	}

	mixed := protectedSubedgeOperation("pcall-xpcall-mixed-entry", true, false, false)
	mixed.Subedges[1].ArgumentOrigins = []ArgumentOrigin{{Segment: ArgumentFixed, Kind: ArgumentSourceRule}}
	if _, err := Seal(&Spec{Operations: []OperationSpec{mixed}}); err == nil {
		t.Fatal("route-fed handler accepted a parallel direct argument origin")
	}

	partial := protectedSubedgeOperation("pcall-xpcall-partial-entry", true, false, false)
	partial.Subedges[0].Routes[2].Result = ValuesSpec{Tail: ValuesClosed}
	if _, err := Seal(&Spec{Operations: []OperationSpec{partial}}); err == nil {
		t.Fatal("partial route accepted as complete handler argument authority")
	}
}

func TestSubedgeNonCallFamiliesHaveClosedABIAndCanonicalOrigins(t *testing.T) {
	empty := ValuesSpec{Tail: ValuesClosed}
	left := exactProjectionOperation("noncall-origin-order", empty, empty)
	left.Subedges[0].Family = SubedgeFamilyIndexGet
	left.Subedges[0].Arguments = ValuesSpec{Fixed: []typ.Type{typ.Any, typ.Any}, Tail: ValuesClosed}
	left.Subedges[0].ArgumentOrigins = []ArgumentOrigin{
		{Segment: ArgumentFixed, Index: 1, Kind: ArgumentSourceRule},
		{Segment: ArgumentFixed, Index: 0, Kind: ArgumentSourceRule},
	}
	right := left
	right.Subedges = append([]SubedgeSpec(nil), left.Subedges...)
	right.Subedges[0].ArgumentOrigins = append([]ArgumentOrigin(nil), left.Subedges[0].ArgumentOrigins...)
	right.Subedges[0].ArgumentOrigins[0], right.Subedges[0].ArgumentOrigins[1] = right.Subedges[0].ArgumentOrigins[1], right.Subedges[0].ArgumentOrigins[0]

	leftContract := mustSeal(t, Spec{Operations: []OperationSpec{left}})
	rightContract := mustSeal(t, Spec{Operations: []OperationSpec{right}})
	if leftContract.ContentID() != rightContract.ContentID() {
		t.Fatal("argument-origin authoring order changed ContentID")
	}
	op, _ := leftContract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"noncall-origin-order"}})
	edge := subedgeByRole(t, leftContract, op, 1)
	if count := leftContract.ArgumentOriginCount(edge); count != 2 {
		t.Fatalf("non-Call origin count = %d", count)
	}
	for index, want := range []uint32{0, 1} {
		segment, ordinal, source, input, ok := leftContract.ArgumentOriginAt(edge, index)
		if !ok || segment != ArgumentFixed || ordinal != want || source != ArgumentSourceRule || input != (InputSource{}) {
			t.Fatalf("origin %d = %d/%d/%d/%#v/%v", index, segment, ordinal, source, input, ok)
		}
	}

	invalid := left
	invalid.Subedges = append([]SubedgeSpec(nil), left.Subedges...)
	invalid.Subedges[0].Arguments = ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesClosed}
	invalid.Subedges[0].ArgumentOrigins = []ArgumentOrigin{{Segment: ArgumentFixed, Kind: ArgumentSourceRule}}
	if _, err := Seal(&Spec{Operations: []OperationSpec{invalid}}); err == nil {
		t.Fatal("IndexGet accepted a non-closed two-argument ABI")
	}
}

func TestSubedgeAdmissionFailureChangesContentIdentity(t *testing.T) {
	left := protectedSubedgeOperation("admission-content", false, false, false)
	right := protectedSubedgeOperation("admission-content", false, false, false)
	right.Subedges[0].AdmissionFailure.Route = AdmissionRouteSpec{
		Route: RouteOutcome, Adjustment: AdjustmentPreserve, Result: callbackTail(2), Placement: PlacementTail, Outcome: 5,
	}
	leftContract := mustSeal(t, Spec{Operations: []OperationSpec{left}})
	rightContract := mustSeal(t, Spec{Operations: []OperationSpec{right}})
	if leftContract.ContentID() == rightContract.ContentID() {
		t.Fatal("admission failure destination was omitted from ContentID")
	}
}

func TestSubedgeRejectsInvalidAuthority(t *testing.T) {
	for _, check := range []struct {
		name string
		edit func(*OperationSpec)
	}{
		{"zero role", func(op *OperationSpec) { op.Subedges[0].Role = 0 }},
		{"duplicate role", func(op *OperationSpec) { op.Subedges[1].Role = op.Subedges[0].Role }},
		{"duplicate callback subedge", func(op *OperationSpec) {
			extra := op.Subedges[0]
			extra.Role = 300
			op.Subedges = append(op.Subedges, extra)
		}},
		{"callback duplicate admission", func(op *OperationSpec) { op.Subedges[0].Admission = OrdinaryCallable }},
		{"tail placement changes variable", func(op *OperationSpec) { op.Callbacks[1].Arguments = callbackTail(5) }},
		{"propagated yield changes payload", func(op *OperationSpec) { op.Subedges[0].Routes[3].Result = callbackTail(4) }},
	} {
		t.Run(check.name, func(t *testing.T) {
			op := protectedSubedgeOperation("invalid-subedge", false, false, false)
			check.edit(&op)
			if _, err := Seal(&Spec{Operations: []OperationSpec{op}}); err == nil {
				t.Fatal("invalid Subedge relation sealed")
			}
		})
	}
}

func TestSubedgeRejectYieldUsesTheExistingCanonicalThrow(t *testing.T) {
	op := protectedSubedgeOperation("reject-yield", false, false, false)
	op.Outcomes = append(op.Outcomes, OutcomeSpec{
		Kind:   flowkind.OutcomeThrow,
		Values: ValuesSpec{Fixed: []typ.Type{typ.LiteralString(rejectedYieldMessage)}, Tail: ValuesClosed},
	})
	op.Subedges[0].Routes[3] = SubedgeRouteSpec{
		Kind: flowkind.OutcomeYield, Route: RouteRejectYield, Adjustment: AdjustmentExact,
		Result:    ValuesSpec{Fixed: []typ.Type{typ.LiteralString(rejectedYieldMessage)}, Tail: ValuesClosed},
		Placement: PlacementFixed, Outcome: uint32(len(op.Outcomes) - 1),
	}
	contract := mustSeal(t, Spec{Operations: []OperationSpec{op}})
	sealed, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"reject-yield"}})
	edge := subedgeByRole(t, contract, sealed, 100)
	route, adjustment, result, placement, offset, outcome, sibling, destination, ok := contract.SubedgeRouteAt(edge, flowkind.OutcomeYield)
	if !ok || route != RouteRejectYield || adjustment != AdjustmentExact || result == 0 || placement != PlacementFixed || offset != 0 || sibling != 0 || destination == 0 {
		t.Fatalf("RejectYield = %d/%d/%d/%d/%d/%d/%d/%d/%v", route, adjustment, result, placement, offset, outcome, sibling, destination, ok)
	}
	kind, values, outcomeOK := contract.OutcomeAt(sealed, int(outcome))
	if !outcomeOK || kind != flowkind.OutcomeThrow || values != destination || contract.ValuesCount(destination) != 1 {
		t.Fatalf("RejectYield destination = %d/%d/%d/%v", kind, values, destination, outcomeOK)
	}
	invalid := protectedSubedgeOperation("reject-yield-invalid", false, false, false)
	invalid.Outcomes = append(invalid.Outcomes, OutcomeSpec{Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Fixed: []typ.Type{typ.LiteralString("wrong")}, Tail: ValuesClosed}})
	invalid.Subedges[0].Routes[3] = SubedgeRouteSpec{
		Kind: flowkind.OutcomeYield, Route: RouteRejectYield, Adjustment: AdjustmentExact,
		Result:    ValuesSpec{Fixed: []typ.Type{typ.LiteralString(rejectedYieldMessage)}, Tail: ValuesClosed},
		Placement: PlacementFixed, Outcome: uint32(len(invalid.Outcomes) - 1),
	}
	if _, err := Seal(&Spec{Operations: []OperationSpec{invalid}}); err == nil {
		t.Fatal("RejectYield accepted a noncanonical error payload")
	}
}

func xpcallHandlerSelfRecurrence(name string, lifecycle CallbackLifecycle, reverseCallbacks, reverseOutcomes bool) OperationSpec {
	op := protectedSubedgeOperation(name, true, reverseCallbacks, reverseOutcomes)
	// The handler is invoked with one scalar error. A handler Throw reinvokes the
	// same handler; a handler Yield crosses the C boundary, discards that Yield
	// payload, then enters the same handler with the canonical error string.
	for index := range op.Callbacks {
		if op.Callbacks[index].Function.Ordinal != 1 {
			continue
		}
		op.Callbacks[index].Admission = DirectFunction
		op.Callbacks[index].Lifecycle = lifecycle
		op.Callbacks[index].Outcomes[2].Values = ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesClosed}
	}
	canonicalError := ValuesSpec{Fixed: []typ.Type{typ.LiteralString(rejectedYieldMessage)}, Tail: ValuesClosed}
	op.Subedges[1].Routes[2] = SubedgeRouteSpec{
		Kind: flowkind.OutcomeThrow, Route: RouteSubedge, Adjustment: AdjustmentExact,
		Result: ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesClosed}, Placement: PlacementFixed, Subedge: 2,
	}
	op.Subedges[1].Routes[3] = SubedgeRouteSpec{
		Kind: flowkind.OutcomeYield, Route: RouteRejectYield, Adjustment: AdjustmentExact,
		Result: canonicalError, Placement: PlacementFixed, Subedge: 2,
	}
	return op
}

func xpcallHandlerMultiEdgeRecurrence(name string, lifecycle CallbackLifecycle) OperationSpec {
	op := protectedSubedgeOperation(name, true, false, false)
	for index := range op.Callbacks {
		if op.Callbacks[index].Function.Ordinal != 1 {
			continue
		}
		op.Callbacks[index].Admission = DirectFunction
		op.Callbacks[index].Lifecycle = lifecycle
		op.Callbacks[index].Outcomes[2].Values = ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesClosed}
	}
	empty := ValuesSpec{Tail: ValuesClosed}
	any := ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesClosed}
	op.Subedges[1].Routes[2] = SubedgeRouteSpec{
		Kind: flowkind.OutcomeThrow, Route: RouteSubedge, Adjustment: AdjustmentExact,
		Result: any, Placement: PlacementFixed, Subedge: 3,
	}
	op.Subedges = append(op.Subedges, SubedgeSpec{
		Role:      300,
		Family:    SubedgeFamilyLength,
		Admission: OrdinaryCallable,
		Arguments: any,
		Outcomes: []TerminalSpec{
			{Kind: flowkind.OutcomeNormal, Values: empty},
			{Kind: flowkind.OutcomeReturn, Values: empty},
			{Kind: flowkind.OutcomeThrow, Values: any},
			{Kind: flowkind.OutcomeYield, Values: empty},
			{Kind: flowkind.OutcomeCancel, Values: empty},
		},
		AdmissionFailure: AdmissionFailureSpec{
			Values: callbackTail(2),
			Route:  AdmissionRouteSpec{Route: RouteOutcome, Adjustment: AdjustmentPreserve, Result: callbackTail(2), Placement: PlacementTail, Outcome: 5},
		},
		Routes: []SubedgeRouteSpec{
			{Kind: flowkind.OutcomeNormal, Route: RouteContinue, Adjustment: AdjustmentExact, Result: empty},
			{Kind: flowkind.OutcomeReturn, Route: RouteContinue, Adjustment: AdjustmentExact, Result: empty},
			{Kind: flowkind.OutcomeThrow, Route: RouteSubedge, Adjustment: AdjustmentPreserve, Result: any, Placement: PlacementFixed, Subedge: 2},
			{Kind: flowkind.OutcomeYield, Route: RoutePropagateYield, Adjustment: AdjustmentPreserve, Result: empty},
			{Kind: flowkind.OutcomeCancel, Route: RouteContinue, Adjustment: AdjustmentExact, Result: empty},
		},
	})
	return op
}

func nullarySubedge(role uint32, successor SubedgeRef, ruleEntry bool) SubedgeSpec {
	empty := ValuesSpec{Tail: ValuesClosed}
	return SubedgeSpec{
		Role: role, Family: SubedgeFamilyCall,
		Callee:    SubedgeCalleeSpec{Kind: SubedgeCalleeMetaKey, MetaKey: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__call"}},
		Admission: OrdinaryCallable,
		RuleEntry: ruleEntry,
		Arguments: empty,
		Outcomes: []TerminalSpec{
			{Kind: flowkind.OutcomeNormal, Values: empty},
			{Kind: flowkind.OutcomeReturn, Values: empty},
			{Kind: flowkind.OutcomeThrow, Values: empty},
			{Kind: flowkind.OutcomeYield, Values: empty},
			{Kind: flowkind.OutcomeCancel, Values: empty},
		},
		AdmissionFailure: AdmissionFailureSpec{
			Values: empty,
			Route:  AdmissionRouteSpec{Route: RouteOutcome, Adjustment: AdjustmentPreserve, Result: empty, Placement: PlacementFixed, Outcome: 0},
		},
		Routes: []SubedgeRouteSpec{
			{Kind: flowkind.OutcomeNormal, Route: RouteSubedge, Adjustment: AdjustmentExact, Result: empty, Placement: PlacementFixed, Subedge: successor},
			{Kind: flowkind.OutcomeReturn, Route: RouteContinue, Adjustment: AdjustmentExact, Result: empty},
			{Kind: flowkind.OutcomeThrow, Route: RouteContinue, Adjustment: AdjustmentExact, Result: empty},
			{Kind: flowkind.OutcomeYield, Route: RoutePropagateYield, Adjustment: AdjustmentPreserve, Result: empty},
			{Kind: flowkind.OutcomeCancel, Route: RouteContinue, Adjustment: AdjustmentExact, Result: empty},
		},
	}
}

func nullarySubedgeOperation(name string, subedges ...SubedgeSpec) OperationSpec {
	return OperationSpec{
		Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}},
		Input:    ValuesSpec{Tail: ValuesClosed},
		Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}},
		Subedges: subedges,
		Effects:  RowSpec{Tail: RowClosed},
	}
}

func nullaryCallbackMuOperation(name string, lifecycle CallbackLifecycle) OperationSpec {
	empty := ValuesSpec{Tail: ValuesClosed}
	return OperationSpec{
		Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}},
		Input:    ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesClosed},
		Callbacks: []CallbackSpec{{
			Function:  InputSource{Kind: InputSourceValueFormal},
			Admission: OrdinaryCallable,
			Arguments: empty,
			Outcomes: []TerminalSpec{
				{Kind: flowkind.OutcomeNormal, Values: empty},
				{Kind: flowkind.OutcomeReturn, Values: empty},
				{Kind: flowkind.OutcomeThrow, Values: empty},
				{Kind: flowkind.OutcomeYield, Values: empty},
				{Kind: flowkind.OutcomeCancel, Values: empty},
			},
			Lifecycle: lifecycle,
			Effects:   RowSpec{Tail: RowClosed},
		}},
		Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: empty}},
		Subedges: []SubedgeSpec{{
			Role:      10,
			Family:    SubedgeFamilyCall,
			Callee:    SubedgeCalleeSpec{Kind: SubedgeCalleeCallback, Callback: 1},
			RuleEntry: true,
			AdmissionFailure: AdmissionFailureSpec{
				Values: empty,
				Route:  AdmissionRouteSpec{Route: RouteOutcome, Adjustment: AdjustmentPreserve, Result: empty, Placement: PlacementFixed, Outcome: 0},
			},
			Routes: []SubedgeRouteSpec{
				{Kind: flowkind.OutcomeNormal, Route: RouteSubedge, Adjustment: AdjustmentExact, Result: empty, Placement: PlacementFixed, Subedge: 1},
				{Kind: flowkind.OutcomeReturn, Route: RouteContinue, Adjustment: AdjustmentExact, Result: empty},
				{Kind: flowkind.OutcomeThrow, Route: RouteContinue, Adjustment: AdjustmentExact, Result: empty},
				{Kind: flowkind.OutcomeYield, Route: RoutePropagateYield, Adjustment: AdjustmentPreserve, Result: empty},
				{Kind: flowkind.OutcomeCancel, Route: RouteContinue, Adjustment: AdjustmentExact, Result: empty},
			},
		}},
		Effects: RowSpec{Tail: RowClosed},
	}
}

func TestSubedgeOnceCallbackRejectsReachableRecurrence(t *testing.T) {
	for _, test := range []struct {
		name      string
		lifecycle CallbackLifecycle
	}{
		{"sync", CallbackSyncOptionalOnce},
		{"retained", CallbackRetainedOptionalOnce},
	} {
		t.Run(test.name, func(t *testing.T) {
			op := xpcallHandlerSelfRecurrence("xpcall-once", test.lifecycle, false, false)
			if _, err := Seal(&Spec{Operations: []OperationSpec{op}}); err == nil {
				t.Fatal("Once callback sealed despite reachable self-reentry")
			}
		})
	}
}

func TestSubedgeNullaryCallbackMuRespectsLifecycleMultiplicity(t *testing.T) {
	once := nullaryCallbackMuOperation("nullary-callback-once", CallbackSyncOptionalOnce)
	if _, err := Seal(&Spec{Operations: []OperationSpec{once}}); err == nil {
		t.Fatal("nullary Once callback Mu head sealed")
	}
	many := nullaryCallbackMuOperation("nullary-callback-many", CallbackSyncOptionalMany)
	contract := mustSeal(t, Spec{Operations: []OperationSpec{many}})
	op, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"nullary-callback-many"}})
	edge := subedgeByRole(t, contract, op, 10)
	callback, callbackOK := contract.SubedgeCallback(edge)
	if lifecycle, lifecycleOK := contract.CallbackLifecycle(callback); !callbackOK || !lifecycleOK || lifecycle != CallbackSyncOptionalMany {
		t.Fatalf("nullary callback lifecycle = %d/%v/%v", lifecycle, callbackOK, lifecycleOK)
	}
}

func TestSubedgeOnceCallbackRejectsReachableMultiEdgeRecurrence(t *testing.T) {
	op := xpcallHandlerMultiEdgeRecurrence("xpcall-multi-once", CallbackSyncOptionalOnce)
	if _, err := Seal(&Spec{Operations: []OperationSpec{op}}); err == nil {
		t.Fatal("Once callback sealed despite reachable multi-edge reentry")
	}
}

func TestSubedgeXPCALLHandlerSelfRecurrenceSealsIteratively(t *testing.T) {
	op := xpcallHandlerSelfRecurrence("xpcall-self", CallbackSyncOptionalMany, false, false)
	contract := mustSeal(t, Spec{Operations: []OperationSpec{op}})
	sealed, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"xpcall-self"}})
	handler := subedgeByRole(t, contract, sealed, 200)
	if admission, ok := contract.SubedgeAdmission(handler); !ok || admission != DirectFunction {
		t.Fatalf("xpcall handler admission = %d/%v, want DirectFunction", admission, ok)
	}
	callback, callbackOK := contract.SubedgeCallback(handler)
	if lifecycle, ok := contract.CallbackLifecycle(callback); !callbackOK || !ok || lifecycle != CallbackSyncOptionalMany {
		t.Fatalf("xpcall recursive handler lifecycle = %d/%v/%v, want SyncOptionalMany", lifecycle, callbackOK, ok)
	}
	for _, kind := range []flowkind.OutcomeKind{flowkind.OutcomeThrow, flowkind.OutcomeYield} {
		route, adjustment, result, placement, offset, outcome, sibling, destination, ok := contract.SubedgeRouteAt(handler, kind)
		if !ok || adjustment != AdjustmentExact || result == 0 || placement != PlacementFixed || offset != 0 || outcome != 0 || sibling != handler || destination == 0 {
			t.Fatalf("handler %d recurrence = %d/%d/%d/%d/%d/%d/%d/%d/%v", kind, route, adjustment, result, placement, offset, outcome, sibling, destination, ok)
		}
		if kind == flowkind.OutcomeThrow && route != RouteSubedge {
			t.Fatalf("handler Throw route = %d, want Subedge", route)
		}
		if kind == flowkind.OutcomeYield && route != RouteRejectYield {
			t.Fatalf("handler Yield route = %d, want RejectYield", route)
		}
	}
}

func TestSubedgeRecurrenceManyLifecycleIsCanonicalUnderAuthoringPermutation(t *testing.T) {
	left := mustSeal(t, Spec{Operations: []OperationSpec{xpcallHandlerSelfRecurrence("xpcall-many-permutation", CallbackSyncOptionalMany, false, false)}})
	right := mustSeal(t, Spec{Operations: []OperationSpec{xpcallHandlerSelfRecurrence("xpcall-many-permutation", CallbackSyncOptionalMany, true, true)}})
	assertPublicContractEqual(t, left, right)
	if left.ContentID() != right.ContentID() {
		t.Fatal("recurrent Many callback authoring permutation changed ContentID")
	}
}

func TestSubedgeReachabilityRequiresAnExplicitRoot(t *testing.T) {
	closed := nullarySubedgeOperation("closed-route-fed", nullarySubedge(10, 2, false), nullarySubedge(20, 1, false))
	if _, err := Seal(&Spec{Operations: []OperationSpec{closed}}); err == nil {
		t.Fatal("closed route-fed Subedge SCC sealed without an executable root")
	}

	direct := nullarySubedgeOperation("nullary-rule-entry", nullarySubedge(10, 1, true))
	contract := mustSeal(t, Spec{Operations: []OperationSpec{direct}})
	op, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"nullary-rule-entry"}})
	edge := subedgeByRole(t, contract, op, 10)
	if ruleEntry, ok := contract.SubedgeRuleEntry(edge); !ok || !ruleEntry {
		t.Fatalf("nullary RuleEntry = %v/%v", ruleEntry, ok)
	}
	if !contract.ContentID().Available() {
		t.Fatal("nullary RuleEntry Mu head has no ContentID")
	}

	missing := nullarySubedgeOperation("nullary-missing-entry", nullarySubedge(10, 1, false))
	missing.Subedges[0].Routes[0] = SubedgeRouteSpec{
		Kind: flowkind.OutcomeNormal, Route: RouteOutcome, Adjustment: AdjustmentExact,
		Result: ValuesSpec{Tail: ValuesClosed}, Placement: PlacementFixed, Outcome: 0,
	}
	if _, err := Seal(&Spec{Operations: []OperationSpec{missing}}); err == nil {
		t.Fatal("nullary Subedge sealed without RuleEntry or an inbound route")
	}

	nonempty := exactProjectionOperation("nonempty-rule-entry", ValuesSpec{Tail: ValuesClosed}, ValuesSpec{Tail: ValuesClosed})
	nonempty.Subedges[0].RuleEntry = true
	if _, err := Seal(&Spec{Operations: []OperationSpec{nonempty}}); err == nil {
		t.Fatal("RuleEntry accepted a nonempty argument product")
	}
}

func TestOpaqueCallbackIsExplicitlyConservative(t *testing.T) {
	contract := mustSeal(t, Spec{})
	op, ok := contract.Opaque()
	callback, callbackOK := contract.CallbackAt(op, 0)
	if !ok || !callbackOK || !contract.CallbackOpaque(callback) || contract.SubedgeCount(op) != 0 {
		t.Fatalf("opaque callback = op:%d/%v callback:%d/%v opaque:%v subedges:%d", op, ok, callback, callbackOK, contract.CallbackOpaque(callback), contract.SubedgeCount(op))
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
