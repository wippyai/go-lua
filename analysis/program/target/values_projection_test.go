package target

import (
	"testing"

	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func exactProjectionOperation(name string, source, result ValuesSpec) OperationSpec {
	empty := ValuesSpec{Tail: ValuesClosed}
	valuesVars := uint32(0)
	if source.Tail == ValuesVariable {
		valuesVars = uint32(source.Var) + 1
	}
	return OperationSpec{
		Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}},
		ValuesVars: valuesVars,
		Input:      empty,
		Outcomes:   []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: empty}},
		Subedges: []SubedgeSpec{{
			Role:      1,
			Family:    SubedgeFamilyLength,
			Admission: schematype.CallableAdmissionOrdinary,
			Arguments: ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed},
			ArgumentOrigins: []ArgumentOrigin{{
				Segment: ArgumentFixed, Kind: ArgumentSourceRule,
			}},
			Outcomes: []TerminalSpec{
				{Kind: flowkind.OutcomeNormal, Values: source},
				{Kind: flowkind.OutcomeReturn, Values: empty},
				{Kind: flowkind.OutcomeThrow, Values: empty},
				{Kind: flowkind.OutcomeYield, Values: empty},
				{Kind: flowkind.OutcomeCancel, Values: empty},
			},
			AdmissionFailure: AdmissionFailureSpec{
				Values: empty,
				Route:  AdmissionRouteSpec{Route: RouteOutcome, Adjustment: AdjustmentPreserve, Result: empty, Placement: PlacementFixed},
			},
			Routes: []SubedgeRouteSpec{
				{Kind: flowkind.OutcomeNormal, Route: RouteContinue, Adjustment: AdjustmentExact, Result: result},
				{Kind: flowkind.OutcomeReturn, Route: RouteContinue, Adjustment: AdjustmentExact, Result: empty},
				{Kind: flowkind.OutcomeThrow, Route: RouteContinue, Adjustment: AdjustmentExact, Result: empty},
				{Kind: flowkind.OutcomeYield, Route: RoutePropagateYield, Adjustment: AdjustmentPreserve, Result: empty},
				{Kind: flowkind.OutcomeCancel, Route: RouteContinue, Adjustment: AdjustmentExact, Result: empty},
			},
		}},
		Effects: RowSpec{Tail: RowClosed},
	}
}

func sealsExactProjection(t *testing.T, name string, source, result ValuesSpec) bool {
	t.Helper()
	_, err := testSeal(&Spec{Operations: []OperationSpec{exactProjectionOperation(name, source, result)}})
	return err == nil
}

func TestExactProjectionAccountsForNilAndEveryOpenSuffixPosition(t *testing.T) {
	empty := ValuesSpec{Tail: ValuesClosed}
	if sealsExactProjection(t, "exact-closed-missing", ValuesSpec{Fixed: []schematype.Type{testString}, Tail: ValuesClosed}, ValuesSpec{Fixed: []schematype.Type{testString, testString}, Tail: ValuesClosed}) {
		t.Fatal("closed Values Exact projection accepted missing string instead of nil")
	}
	if !sealsExactProjection(t, "exact-nil", empty, ValuesSpec{Fixed: []schematype.Type{testNil}, Tail: ValuesClosed}) {
		t.Fatal("closed empty Values did not project nil")
	}
	if sealsExactProjection(t, "exact-nil-reject", empty, ValuesSpec{Fixed: []schematype.Type{testString}, Tail: ValuesClosed}) {
		t.Fatal("closed empty Values projected nil into string")
	}

	optionalString := testUnion(testString, testNil)
	if sealsExactProjection(t, "exact-open-scalar-reject", ValuesSpec{Tail: ValuesVariable, Var: 0, TailType: testString}, ValuesSpec{Fixed: []schematype.Type{testString}, Tail: ValuesClosed}) {
		t.Fatal("open string tail Exact projection ignored its empty/nil case")
	}
	if !sealsExactProjection(t, "exact-open-scalar", ValuesSpec{Tail: ValuesVariable, Var: 0, TailType: testString}, ValuesSpec{Fixed: []schematype.Type{optionalString}, Tail: ValuesClosed}) {
		t.Fatal("open string tail Exact projection rejected string|nil")
	}

	stringInteger := testUnion(testString, testInteger)
	stringIntegerNil := testUnion(testString, testInteger, testNil)
	source := ValuesSpec{
		Fixed: []schematype.Type{testString}, Tail: ValuesVariable, Var: 0, TailType: testString,
		Suffix: []schematype.Type{testInteger},
	}
	if !sealsExactProjection(t, "exact-open-suffix", source, ValuesSpec{Fixed: []schematype.Type{testString, stringInteger, stringIntegerNil}, Tail: ValuesClosed}) {
		t.Fatal("P·alpha·S Exact projection rejected its complete positional coverage")
	}
	if sealsExactProjection(t, "exact-open-suffix-integer", source, ValuesSpec{Fixed: []schematype.Type{testString, testString, stringIntegerNil}, Tail: ValuesClosed}) {
		t.Fatal("P·alpha·S Exact projection ignored an early suffix position")
	}
	if sealsExactProjection(t, "exact-open-suffix-nil", source, ValuesSpec{Fixed: []schematype.Type{testString, stringInteger, stringInteger}, Tail: ValuesClosed}) {
		t.Fatal("P·alpha·S Exact projection ignored the tail-short nil case")
	}
}

func TestExactProjectionRejectsConcreteTerminalContradiction(t *testing.T) {
	if sealsExactProjection(t, "exact-string-number", ValuesSpec{Fixed: []schematype.Type{testString}, Tail: ValuesClosed}, ValuesSpec{Fixed: []schematype.Type{testNumber}, Tail: ValuesClosed}) {
		t.Fatal("Exact projection accepted String terminal as Number")
	}
}
