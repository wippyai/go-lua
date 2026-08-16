package target

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/type/typ"
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
			Admission: OrdinaryCallable,
			Arguments: ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesClosed},
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
	_, err := Seal(&Spec{Operations: []OperationSpec{exactProjectionOperation(name, source, result)}})
	return err == nil
}

func TestExactProjectionAccountsForNilAndEveryOpenSuffixPosition(t *testing.T) {
	empty := ValuesSpec{Tail: ValuesClosed}
	if sealsExactProjection(t, "exact-closed-missing", ValuesSpec{Fixed: []typ.Type{typ.String}, Tail: ValuesClosed}, ValuesSpec{Fixed: []typ.Type{typ.String, typ.String}, Tail: ValuesClosed}) {
		t.Fatal("closed Values Exact projection accepted missing string instead of nil")
	}
	if !sealsExactProjection(t, "exact-nil", empty, ValuesSpec{Fixed: []typ.Type{typ.Nil}, Tail: ValuesClosed}) {
		t.Fatal("closed empty Values did not project nil")
	}
	if sealsExactProjection(t, "exact-nil-reject", empty, ValuesSpec{Fixed: []typ.Type{typ.String}, Tail: ValuesClosed}) {
		t.Fatal("closed empty Values projected nil into string")
	}

	optionalString := typ.MaterializeUnion([]typ.Type{typ.String, typ.Nil})
	if sealsExactProjection(t, "exact-open-scalar-reject", ValuesSpec{Tail: ValuesVariable, Var: 0, TailType: typ.String}, ValuesSpec{Fixed: []typ.Type{typ.String}, Tail: ValuesClosed}) {
		t.Fatal("open string tail Exact projection ignored its empty/nil case")
	}
	if !sealsExactProjection(t, "exact-open-scalar", ValuesSpec{Tail: ValuesVariable, Var: 0, TailType: typ.String}, ValuesSpec{Fixed: []typ.Type{optionalString}, Tail: ValuesClosed}) {
		t.Fatal("open string tail Exact projection rejected string|nil")
	}

	stringInteger := typ.MaterializeUnion([]typ.Type{typ.String, typ.Integer})
	stringIntegerNil := typ.MaterializeUnion([]typ.Type{typ.String, typ.Integer, typ.Nil})
	source := ValuesSpec{
		Fixed: []typ.Type{typ.String}, Tail: ValuesVariable, Var: 0, TailType: typ.String,
		Suffix: []typ.Type{typ.Integer},
	}
	if !sealsExactProjection(t, "exact-open-suffix", source, ValuesSpec{Fixed: []typ.Type{typ.String, stringInteger, stringIntegerNil}, Tail: ValuesClosed}) {
		t.Fatal("P·alpha·S Exact projection rejected its complete positional coverage")
	}
	if sealsExactProjection(t, "exact-open-suffix-integer", source, ValuesSpec{Fixed: []typ.Type{typ.String, typ.String, stringIntegerNil}, Tail: ValuesClosed}) {
		t.Fatal("P·alpha·S Exact projection ignored an early suffix position")
	}
	if sealsExactProjection(t, "exact-open-suffix-nil", source, ValuesSpec{Fixed: []typ.Type{typ.String, stringInteger, stringInteger}, Tail: ValuesClosed}) {
		t.Fatal("P·alpha·S Exact projection ignored the tail-short nil case")
	}
}

func TestExactProjectionRejectsConcreteTerminalContradiction(t *testing.T) {
	if sealsExactProjection(t, "exact-string-number", ValuesSpec{Fixed: []typ.Type{typ.String}, Tail: ValuesClosed}, ValuesSpec{Fixed: []typ.Type{typ.Number}, Tail: ValuesClosed}) {
		t.Fatal("Exact projection accepted String terminal as Number")
	}
}
