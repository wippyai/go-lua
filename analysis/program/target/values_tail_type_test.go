package target

import (
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func TestValuesVarTailTypesAreTotalSharedAndAllocationFree(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{{
		Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"tail-class"}}},
		ValuesVars: 3,
		Input:      ValuesSpec{Tail: ValuesVariable, Var: 0, TailType: testString},
		Outcomes: []OutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesVariable, Var: 0, TailType: testString}},
			{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesVariable, Var: 1}},
			{Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Tail: ValuesClosed}},
		},
		Effects: RowSpec{Tail: RowClosed},
	}}})
	op, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"tail-class"}})
	if !ok {
		t.Fatal("tail-class operation missing")
	}
	input, _ := contract.Input(op)
	if got, found := contract.ValuesTailType(input); !found || !sameFrozenType(t, contract, got, testString) {
		t.Fatalf("input tail type = %d/%v, want string", got, found)
	}
	for _, variable := range []ValuesVar{1, 2} {
		got, found := contract.ValuesVarType(op, variable)
		if !found || !sameFrozenType(t, contract, got, testAny) {
			t.Fatalf("ValuesVarType(%d) = %d/%v, want any", variable, got, found)
		}
	}
	_, closed, _ := contract.OutcomeAt(op, 2)
	if _, found := contract.ValuesTailType(closed); found {
		t.Fatal("closed Values exposed a tail type")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		_, _ = contract.ValuesVarType(op, 0)
		_, _ = contract.ValuesTailType(input)
	}); allocations != 0 {
		t.Fatalf("Values tail type queries allocated %f times", allocations)
	}
}

func TestValuesTailTypesRejectInvalidAndConflictingClasses(t *testing.T) {
	base := OperationSpec{
		ValuesVars: 1,
		Input:      ValuesSpec{Tail: ValuesVariable, Var: 0, TailType: testString},
		Outcomes:   []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesVariable, Var: 0, TailType: testString}}},
		Effects:    RowSpec{Tail: RowClosed},
	}
	for _, test := range []struct {
		name string
		edit func(*OperationSpec)
	}{
		{"closed tail class", func(op *OperationSpec) { op.Input = ValuesSpec{Tail: ValuesClosed, TailType: testString} }},
		{"unknown tail class", func(op *OperationSpec) { op.Input = ValuesSpec{Tail: ValuesUnknown, TailType: testString} }},
		{"conflicting explicit class", func(op *OperationSpec) { op.Outcomes[0].Values.TailType = testNumber }},
		{"omitted any conflicts", func(op *OperationSpec) { op.Outcomes[0].Values.TailType = schematype.Type{} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			op := base
			op.Outcomes = append([]OutcomeSpec(nil), base.Outcomes...)
			test.edit(&op)
			if _, err := testSeal(&Spec{Operations: []OperationSpec{op}}); err == nil {
				t.Fatal("Seal accepted invalid Values tail class")
			}
		})
	}
}

func admissionTailClassOperation(name string, class schematype.Type) OperationSpec {
	empty := ValuesSpec{Tail: ValuesClosed}
	optional := testUnion(class, testNil)
	return OperationSpec{
		Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}},
		ValuesVars: 1,
		Input:      empty,
		Outcomes:   []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed}}},
		Subedges: []SubedgeSpec{{
			Role:      1,
			Family:    SubedgeFamilyLength,
			Admission: OrdinaryCallable,
			Arguments: ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed},
			ArgumentOrigins: []ArgumentOrigin{{
				Segment: ArgumentFixed, Kind: ArgumentSourceRule,
			}},
			Outcomes: []TerminalSpec{
				{Kind: flowkind.OutcomeNormal, Values: empty},
				{Kind: flowkind.OutcomeReturn, Values: empty},
				{Kind: flowkind.OutcomeThrow, Values: empty},
				{Kind: flowkind.OutcomeYield, Values: empty},
				{Kind: flowkind.OutcomeCancel, Values: empty},
			},
			AdmissionFailure: AdmissionFailureSpec{
				Values: ValuesSpec{Tail: ValuesVariable, Var: 0, TailType: class},
				Route: AdmissionRouteSpec{
					Route: RouteOutcome, Adjustment: AdjustmentExact,
					Result:    ValuesSpec{Fixed: []schematype.Type{optional}, Tail: ValuesClosed},
					Placement: PlacementFixed, Outcome: 0,
				},
			},
			Routes: []SubedgeRouteSpec{
				{Kind: flowkind.OutcomeNormal, Route: RouteContinue, Adjustment: AdjustmentExact, Result: empty},
				{Kind: flowkind.OutcomeReturn, Route: RouteContinue, Adjustment: AdjustmentExact, Result: empty},
				{Kind: flowkind.OutcomeThrow, Route: RouteContinue, Adjustment: AdjustmentExact, Result: empty},
				{Kind: flowkind.OutcomeYield, Route: RoutePropagateYield, Adjustment: AdjustmentPreserve, Result: empty},
				{Kind: flowkind.OutcomeCancel, Route: RouteContinue, Adjustment: AdjustmentExact, Result: empty},
			},
		}},
		Effects: RowSpec{Tail: RowClosed},
	}
}

func TestAdmissionFailureTailContributesToTheOneValuesVarClassTable(t *testing.T) {
	strings := mustSeal(t, Spec{Operations: []OperationSpec{admissionTailClassOperation("admission-tail-string", testString)}})
	op, found := strings.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"admission-tail-string"}})
	if !found {
		t.Fatal("admission-tail-string operation missing")
	}
	edge, found := strings.SubedgeAt(op, 0)
	if !found {
		t.Fatal("admission-tail-string Subedge missing")
	}
	failure, found := strings.AdmissionFailure(edge)
	if !found {
		t.Fatal("admission failure source missing")
	}
	if tail, variable, ok := strings.ValuesTail(failure); !ok || tail != ValuesVariable || variable != 0 {
		t.Fatalf("admission failure tail = %d/%d/%v", tail, variable, ok)
	}
	if class, ok := strings.ValuesTailType(failure); !ok || !sameFrozenType(t, strings, class, testString) {
		t.Fatalf("admission failure tail class = %d/%v, want string", class, ok)
	}

	numbers := mustSeal(t, Spec{Operations: []OperationSpec{admissionTailClassOperation("admission-tail-string", testNumber)}})
	if strings.ContentID() == numbers.ContentID() {
		t.Fatal("admission tail class was omitted from ContentID")
	}

	conflict := admissionTailClassOperation("admission-tail-conflict", testString)
	// Input does not participate in this admission transport. If the separate
	// admission source were omitted from ValuesVar closure, this would silently
	// seal as number (or fall back to Any); it must instead reject the conflict.
	conflict.Input = ValuesSpec{Tail: ValuesVariable, Var: 0, TailType: testNumber}
	if _, err := testSeal(&Spec{Operations: []OperationSpec{conflict}}); err == nil {
		t.Fatal("admission failure tail class conflicted with owner input but sealed")
	}
}

func sameFrozenType(t *testing.T, contract *Contract, frozen Type, want schematype.Type) bool {
	t.Helper()
	got, gotOK := contract.TypeDeclaration(frozen)
	return gotOK && got.Equal(want)
}
