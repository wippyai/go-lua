package stdlib

import (
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/type/typ"
	. "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/types/signature"
)

func nativeBuiltin(string) Operation              { return Operation{} }
func module(_, _ string) Operation                { return Operation{} }
func aliasModule(_ string, _ ...string) Operation { return Operation{} }
func method(_, _, _ string, in, out []typ.Type) Operation {
	return fixed(Operation{}, append([]typ.Type{typ.Any}, in...), out)
}
func values(fixed []typ.Type, open bool, variable ValuesVar) Values {
	tail := ValuesClosed
	var tailType typ.Type
	if open {
		tail = ValuesVariable
		tailType = typ.Any
	}
	return Values{Fixed: append([]typ.Type(nil), fixed...), Tail: tail, Var: variable, TailType: tailType}
}

func closed(fixed ...typ.Type) Values {
	return values(fixed, false, 0)
}

func anyValue() Values { return closed(typ.Any) }

func emptyValues() Values { return closed() }

func rejectedYield() Values {
	return closed(typ.LiteralString("attempt to yield across a C-call boundary"))
}

func terminals(normal, returned, thrown, yielded, canceled Values) []Terminal {
	return []Terminal{
		{Kind: OutcomeNormal, Values: normal},
		{Kind: OutcomeReturn, Values: returned},
		{Kind: OutcomeThrow, Values: thrown},
		{Kind: OutcomeYield, Values: yielded},
		{Kind: OutcomeCancel, Values: canceled},
	}
}

func outcomeRoute(kind OutcomeKind, result Values, adjustment Adjustment, placement Placement, outcome uint32) SubedgeRoute {
	return SubedgeRoute{Kind: kind, Route: SubedgeRouteOutcome, Adjustment: adjustment, Result: result, Placement: placement, Outcome: outcome}
}

func siblingRoute(kind OutcomeKind, result Values, adjustment Adjustment, sibling SubedgeRef) SubedgeRoute {
	return SubedgeRoute{Kind: kind, Route: SubedgeRouteSubedge, Adjustment: adjustment, Result: result, Placement: PlacementFixed, Subedge: sibling}
}

func continueRoute(kind OutcomeKind, result Values, adjustment Adjustment) SubedgeRoute {
	return SubedgeRoute{Kind: kind, Route: SubedgeRouteContinue, Adjustment: adjustment, Result: result}
}

func propagateRoute(values Values) SubedgeRoute {
	return SubedgeRoute{Kind: OutcomeYield, Route: SubedgeRoutePropagateYield, Adjustment: AdjustmentPreserve, Result: values}
}

func rejectRoute(outcome uint32) SubedgeRoute {
	return SubedgeRoute{Kind: OutcomeYield, Route: SubedgeRouteRejectYield, Adjustment: AdjustmentExact, Result: rejectedYield(), Placement: PlacementFixed, Outcome: outcome}
}

func rejectSiblingRoute(sibling SubedgeRef) SubedgeRoute {
	return SubedgeRoute{Kind: OutcomeYield, Route: SubedgeRouteRejectYield, Adjustment: AdjustmentExact, Result: rejectedYield(), Placement: PlacementFixed, Subedge: sibling}
}

func admissionToOutcome(result Values, adjustment Adjustment, placement Placement, outcome uint32) AdmissionFailure {
	return AdmissionFailure{Values: anyValue(), Route: AdmissionRoute{Route: SubedgeRouteOutcome, Adjustment: adjustment, Result: result, Placement: placement, Outcome: outcome}}
}

func admissionToSibling(result Values, sibling SubedgeRef) AdmissionFailure {
	return AdmissionFailure{Values: anyValue(), Route: AdmissionRoute{Route: SubedgeRouteSubedge, Adjustment: AdjustmentExact, Result: result, Placement: PlacementFixed, Subedge: sibling}}
}

func tailInputOrigin(variable ValuesVar) []ArgumentOrigin {
	return []ArgumentOrigin{{Segment: ArgumentTail, Kind: ArgumentSourceInput, Source: InputSource{Kind: InputSourceValues, Ordinal: uint32(variable)}}}
}

func ruleOrigins(count int) []ArgumentOrigin {
	out := make([]ArgumentOrigin, count)
	for index := range out {
		out[index] = ArgumentOrigin{Segment: ArgumentFixed, Index: uint32(index), Kind: ArgumentSourceRule}
	}
	return out
}

func ruleTailOrigin() []ArgumentOrigin {
	return []ArgumentOrigin{{Segment: ArgumentTail, Kind: ArgumentSourceRule}}
}

func fixedInputOrigin(index uint32) ArgumentOrigin {
	return ArgumentOrigin{Segment: ArgumentFixed, Index: index, Kind: ArgumentSourceInput, Source: InputSource{Kind: InputSourceValue, Ordinal: index}}
}

// ruleFamilyEdge declares a non-yieldable target-machine application whose
// operands and dependent result formula belong to the one owning operation
// Rule.  The Target row owns only the typed application boundary and complete
// terminal disposition.
func ruleFamilyEdge(role uint32, family SubedgeFamily, arguments Values, throwOutcome, cancelOutcome uint32, cancel Values) Subedge {
	return Subedge{
		Role: role, Family: family, Admission: CallableAdmissionOrdinary, Arguments: arguments,
		ArgumentOrigins:  ruleOrigins(len(arguments.Fixed)),
		Outcomes:         terminals(anyValue(), anyValue(), anyValue(), anyValue(), cancel),
		AdmissionFailure: admissionToOutcome(anyValue(), AdjustmentPreserve, PlacementFixed, throwOutcome),
		Routes: []SubedgeRoute{
			continueRoute(OutcomeNormal, anyValue(), AdjustmentExact),
			continueRoute(OutcomeReturn, anyValue(), AdjustmentExact),
			outcomeRoute(OutcomeThrow, anyValue(), AdjustmentPreserve, PlacementFixed, throwOutcome),
			rejectRoute(throwOutcome),
			outcomeRoute(OutcomeCancel, cancel, AdjustmentPreserve, PlacementTail, cancelOutcome),
		},
	}
}

func ruleMetaCallEdge(role uint32, key Literal, arguments Values, throwOutcome, cancelOutcome uint32, cancel Values) Subedge {
	return Subedge{
		Role: role, Family: SubedgeFamilyCall,
		Callee: SubedgeCallee{Kind: SubedgeCalleeMetaKey, MetaKey: key}, Admission: CallableAdmissionOrdinary,
		Arguments: arguments, ArgumentOrigins: ruleOrigins(len(arguments.Fixed)),
		Outcomes:         terminals(anyValue(), anyValue(), anyValue(), anyValue(), cancel),
		AdmissionFailure: admissionToOutcome(anyValue(), AdjustmentPreserve, PlacementFixed, throwOutcome),
		Routes: []SubedgeRoute{
			continueRoute(OutcomeNormal, anyValue(), AdjustmentExact),
			continueRoute(OutcomeReturn, anyValue(), AdjustmentExact),
			outcomeRoute(OutcomeThrow, anyValue(), AdjustmentPreserve, PlacementFixed, throwOutcome),
			rejectRoute(throwOutcome),
			outcomeRoute(OutcomeCancel, cancel, AdjustmentPreserve, PlacementTail, cancelOutcome),
		},
	}
}

func literalKey(text string) Literal {
	return Literal{Kind: LiteralString, String: text}
}
func normal(op Operation, in []typ.Type, openIn bool, out []typ.Type, openOut bool) Operation {
	vars := uint32(0)
	inVar, outVar := ValuesVar(0), ValuesVar(0)
	if openIn {
		vars++
		inVar = 0
	}
	if openOut {
		outVar = ValuesVar(vars)
		vars++
	}
	op.ValuesVars = vars
	op.Input = values(in, openIn, inVar)
	op.Outcomes = []Outcome{{Kind: OutcomeNormal, Values: values(out, openOut, outVar)}, {Kind: OutcomeThrow, Values: values([]typ.Type{typ.Any}, false, 0)}}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}
func fixed(op Operation, in, out []typ.Type) Operation {
	return normal(op, in, false, out, false)
}
func total(op Operation, in []typ.Type, openIn bool, out []typ.Type, openOut bool) Operation {
	vars := uint32(0)
	inVar, outVar := ValuesVar(0), ValuesVar(0)
	if openIn {
		vars++
		inVar = 0
	}
	if openOut {
		outVar = ValuesVar(vars)
		vars++
	}
	op.ValuesVars = vars
	op.Input = values(in, openIn, inVar)
	op.Outcomes = []Outcome{{Kind: OutcomeNormal, Values: values(out, openOut, outVar)}}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}
func throws(op Operation, in []typ.Type, open bool) Operation {
	vars := uint32(0)
	if open {
		vars = 1
	}
	op.ValuesVars = vars
	op.Input = values(in, open, 0)
	op.Outcomes = []Outcome{{Kind: OutcomeThrow, Values: values([]typ.Type{typ.Any}, false, 0)}}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}
func openSame(op Operation) Operation {
	op.ValuesVars = 1
	op.Input = values(nil, true, 0)
	op.Outcomes = []Outcome{{Kind: OutcomeNormal, Values: values(nil, true, 0)}, {Kind: OutcomeThrow, Values: values([]typ.Type{typ.Any}, false, 0)}}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}
func alternatives(op Operation, in []typ.Type, open bool, outputs [][]typ.Type) Operation {
	vars := uint32(0)
	if open {
		vars = 1
	}
	op.ValuesVars = vars
	op.Input = values(in, open, 0)
	for _, out := range outputs {
		op.Outcomes = append(op.Outcomes, Outcome{Kind: OutcomeNormal, Values: values(out, false, 0)})
	}
	op.Outcomes = append(op.Outcomes, Outcome{Kind: OutcomeThrow, Values: values([]typ.Type{typ.Any}, false, 0)})
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}
func alternativesTotal(op Operation, in []typ.Type, open bool, outputs [][]typ.Type) Operation {
	vars := uint32(0)
	if open {
		vars = 1
	}
	op.ValuesVars = vars
	op.Input = values(in, open, 0)
	for _, out := range outputs {
		op.Outcomes = append(op.Outcomes, Outcome{Kind: OutcomeNormal, Values: values(out, false, 0)})
	}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}
func protected(op Operation, fixed int) Operation {
	if fixed == 1 {
		return pcallProfile(op)
	}
	return xpcallProfile(op)
}

func pcallProfile(op Operation) Operation {
	op.ValuesVars = 4
	op.Input = values([]typ.Type{typ.Any}, true, 0)
	op.Callbacks = []Callback{{
		Function: InputSource{Kind: InputSourceValue}, Admission: CallableAdmissionOrdinary,
		Arguments: nativeCallbackTail(0), Outcomes: terminals(nativeCallbackTail(1), nativeCallbackTail(1), anyValue(), nativeCallbackTail(2), nativeCallbackTail(3)),
		Lifecycle: CallbackSyncRequiredOnce, Effects: RowSpec{Tail: RowClosed},
	}}
	op.Outcomes = []Outcome{
		{Kind: OutcomeNormal, Values: values([]typ.Type{typ.LiteralBool(true)}, true, 1)},
		{Kind: OutcomeNormal, Values: closed(typ.LiteralBool(false), typ.Any)},
		{Kind: OutcomeYield, Values: nativeCallbackTail(2)},
		{Kind: OutcomeCancel, Values: nativeCallbackTail(3)},
	}
	op.Subedges = []Subedge{{
		Role: 1, Family: SubedgeFamilyCall, Callee: SubedgeCallee{Kind: SubedgeCalleeCallback, Callback: 1},
		ArgumentOrigins:  tailInputOrigin(0),
		AdmissionFailure: admissionToOutcome(anyValue(), AdjustmentPreserve, PlacementFixed, 1),
		Routes: []SubedgeRoute{
			outcomeRoute(OutcomeNormal, nativeCallbackTail(1), AdjustmentPreserve, PlacementTail, 0),
			outcomeRoute(OutcomeReturn, nativeCallbackTail(1), AdjustmentPreserve, PlacementTail, 0),
			outcomeRoute(OutcomeThrow, anyValue(), AdjustmentPreserve, PlacementFixed, 1),
			propagateRoute(nativeCallbackTail(2)),
			outcomeRoute(OutcomeCancel, nativeCallbackTail(3), AdjustmentPreserve, PlacementTail, 3),
		},
	}}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}

func xpcallProfile(op Operation) Operation {
	op.ValuesVars = 5
	op.Input = values([]typ.Type{typ.Any, typ.Any}, true, 0)
	op.Callbacks = []Callback{
		{Function: InputSource{Kind: InputSourceValue, Ordinal: 0}, Admission: CallableAdmissionOrdinary, Arguments: nativeCallbackTail(0), Outcomes: terminals(nativeCallbackTail(1), nativeCallbackTail(1), anyValue(), nativeCallbackTail(2), nativeCallbackTail(3)), Lifecycle: CallbackSyncRequiredOnce, Effects: RowSpec{Tail: RowClosed}},
		{Function: InputSource{Kind: InputSourceValue, Ordinal: 1}, Admission: CallableAdmissionDirectFunction, Arguments: anyValue(), Outcomes: terminals(nativeCallbackTail(4), nativeCallbackTail(4), anyValue(), anyValue(), nativeCallbackTail(3)), Lifecycle: CallbackSyncOptionalMany, Effects: RowSpec{Tail: RowClosed}},
	}
	op.Outcomes = []Outcome{
		{Kind: OutcomeNormal, Values: values([]typ.Type{typ.LiteralBool(true)}, true, 1)},
		{Kind: OutcomeNormal, Values: closed(typ.LiteralBool(false), typ.Any)},
		{Kind: OutcomeYield, Values: nativeCallbackTail(2)},
		{Kind: OutcomeCancel, Values: nativeCallbackTail(3)},
		{Kind: OutcomeThrow, Values: anyValue()},
	}
	op.Subedges = []Subedge{
		{
			Role: 1, Family: SubedgeFamilyCall, Callee: SubedgeCallee{Kind: SubedgeCalleeCallback, Callback: 1}, ArgumentOrigins: tailInputOrigin(0),
			AdmissionFailure: admissionToSibling(anyValue(), 2),
			Routes: []SubedgeRoute{
				outcomeRoute(OutcomeNormal, nativeCallbackTail(1), AdjustmentPreserve, PlacementTail, 0),
				outcomeRoute(OutcomeReturn, nativeCallbackTail(1), AdjustmentPreserve, PlacementTail, 0),
				siblingRoute(OutcomeThrow, anyValue(), AdjustmentExact, 2),
				propagateRoute(nativeCallbackTail(2)),
				outcomeRoute(OutcomeCancel, nativeCallbackTail(3), AdjustmentPreserve, PlacementTail, 3),
			},
		},
		{
			Role: 2, Family: SubedgeFamilyCall, Callee: SubedgeCallee{Kind: SubedgeCalleeCallback, Callback: 2},
			AdmissionFailure: admissionToOutcome(anyValue(), AdjustmentPreserve, PlacementFixed, 4),
			Routes: []SubedgeRoute{
				outcomeRoute(OutcomeNormal, anyValue(), AdjustmentExact, PlacementFixed, 1),
				outcomeRoute(OutcomeReturn, anyValue(), AdjustmentExact, PlacementFixed, 1),
				siblingRoute(OutcomeThrow, anyValue(), AdjustmentExact, 2),
				rejectSiblingRoute(2),
				outcomeRoute(OutcomeCancel, nativeCallbackTail(3), AdjustmentPreserve, PlacementTail, 3),
			},
		},
	}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}

func printProfile() Operation {
	op := nativeBuiltin("print")
	op.ValuesVars = 2
	op.Input = nativeCallbackTail(0)
	op.Outcomes = []Outcome{{Kind: OutcomeNormal, Values: emptyValues()}, {Kind: OutcomeThrow, Values: anyValue()}, {Kind: OutcomeCancel, Values: nativeCallbackTail(1)}}
	op.Subedges = []Subedge{{
		Role: 1, Family: SubedgeFamilyCall,
		Callee:    SubedgeCallee{Kind: SubedgeCalleeCapturedInitialRead, Read: CapturedInitialReadSpec{Root: GlobalEnvironmentRoot, Key: literalKey("tostring")}},
		Admission: CallableAdmissionOrdinary, Arguments: anyValue(), ArgumentOrigins: ruleOrigins(1),
		Outcomes:         terminals(anyValue(), anyValue(), anyValue(), anyValue(), nativeCallbackTail(1)),
		AdmissionFailure: admissionToOutcome(anyValue(), AdjustmentPreserve, PlacementFixed, 1),
		Routes: []SubedgeRoute{
			continueRoute(OutcomeNormal, anyValue(), AdjustmentExact),
			continueRoute(OutcomeReturn, anyValue(), AdjustmentExact),
			outcomeRoute(OutcomeThrow, anyValue(), AdjustmentPreserve, PlacementFixed, 1),
			rejectRoute(1),
			outcomeRoute(OutcomeCancel, nativeCallbackTail(1), AdjustmentPreserve, PlacementTail, 2),
		},
	}}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}

func tostringProfile() Operation {
	op := nativeBuiltin("tostring")
	op.ValuesVars = 1
	op.Input = anyValue()
	op.Outcomes = []Outcome{{Kind: OutcomeNormal, Values: closed(typ.String)}, {Kind: OutcomeThrow, Values: anyValue()}, {Kind: OutcomeCancel, Values: nativeCallbackTail(0)}}
	edge := ruleMetaCallEdge(1, literalKey("__tostring"), anyValue(), 1, 2, nativeCallbackTail(0))
	edge.ArgumentOrigins = []ArgumentOrigin{fixedInputOrigin(0)}
	op.Subedges = []Subedge{edge}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}

func formatProfile() Operation {
	op := module("string", "format")
	op.ValuesVars = 2
	op.Input = values([]typ.Type{typ.String}, true, 0)
	op.Outcomes = []Outcome{{Kind: OutcomeNormal, Values: closed(typ.String)}, {Kind: OutcomeThrow, Values: anyValue()}, {Kind: OutcomeCancel, Values: nativeCallbackTail(1)}}
	op.Subedges = []Subedge{ruleMetaCallEdge(1, literalKey("__tostring"), anyValue(), 1, 2, nativeCallbackTail(1))}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}

func pairsProfile() Operation {
	op := nativeBuiltin("pairs")
	op.ValuesVars = 1
	op.Input = anyValue()
	// The meta-hook and raw fallback are separate ordinary outcomes.  A hook
	// controls every returned slot; only the fallback proves next/input/nil.
	metaThree := closed(typ.Any, typ.Any, typ.Any)
	fallbackThree := closed(typ.Any, typ.Any, typ.Nil)
	op.Outcomes = []Outcome{{Kind: OutcomeNormal, Values: metaThree}, {Kind: OutcomeNormal, Values: fallbackThree}, {Kind: OutcomeThrow, Values: anyValue()}, {Kind: OutcomeCancel, Values: nativeCallbackTail(0)}}
	op.Subedges = []Subedge{{
		Role: 1, Family: SubedgeFamilyCall, Callee: SubedgeCallee{Kind: SubedgeCalleeMetaKey, MetaKey: literalKey("__pairs")},
		Admission: CallableAdmissionOrdinary, Arguments: anyValue(), ArgumentOrigins: []ArgumentOrigin{{Segment: ArgumentFixed, Kind: ArgumentSourceInput, Source: InputSource{Kind: InputSourceValue}}},
		Outcomes:         terminals(anyValue(), anyValue(), anyValue(), anyValue(), nativeCallbackTail(0)),
		AdmissionFailure: admissionToOutcome(anyValue(), AdjustmentPreserve, PlacementFixed, 2),
		Routes: []SubedgeRoute{
			outcomeRoute(OutcomeNormal, metaThree, AdjustmentExact, PlacementFixed, 0),
			outcomeRoute(OutcomeReturn, metaThree, AdjustmentExact, PlacementFixed, 0),
			outcomeRoute(OutcomeThrow, anyValue(), AdjustmentPreserve, PlacementFixed, 2),
			rejectRoute(2),
			outcomeRoute(OutcomeCancel, nativeCallbackTail(0), AdjustmentPreserve, PlacementTail, 3),
		},
	}}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}

func ipairsProfile() Operation {
	op := fixed(nativeBuiltin("ipairs"), []typ.Type{typ.Any}, []typ.Type{typ.Any, typ.Any, typ.Integer})
	return op
}

func ipairsAuxProfile() Operation {
	op := Operation{ValuesVars: 1, Input: closed(typ.Any, typ.Integer), Outcomes: []Outcome{
		{Kind: OutcomeNormal, Values: closed(typ.Nil)},
		{Kind: OutcomeNormal, Values: closed(typ.Integer, typ.Any)},
		{Kind: OutcomeThrow, Values: anyValue()},
		{Kind: OutcomeCancel, Values: nativeCallbackTail(0)},
	}, Effects: RowSpec{Tail: RowClosed}}
	edge := ruleFamilyEdge(1, SubedgeFamilyIndexGet, closed(typ.Any, typ.Integer), 2, 3, nativeCallbackTail(0))
	edge.ArgumentOrigins = []ArgumentOrigin{fixedInputOrigin(0), {Segment: ArgumentFixed, Index: 1, Kind: ArgumentSourceRule}}
	op.Subedges = []Subedge{edge}
	return op
}

func tableOperation(name string, input []typ.Type, open bool, output Values) Operation {
	op := module("table", name)
	op.ValuesVars = 2
	op.Input = values(input, open, 0)
	op.Outcomes = []Outcome{{Kind: OutcomeNormal, Values: output}, {Kind: OutcomeThrow, Values: anyValue()}, {Kind: OutcomeCancel, Values: nativeCallbackTail(1)}}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}

func tableConcatProfile() Operation {
	op := tableOperation("concat", []typ.Type{typ.Any}, true, closed(typ.String))
	op.Subedges = []Subedge{
		ruleFamilyEdge(1, SubedgeFamilyLength, anyValue(), 1, 2, nativeCallbackTail(1)),
		ruleFamilyEdge(2, SubedgeFamilyIndexGet, closed(typ.Any, typ.Integer), 1, 2, nativeCallbackTail(1)),
	}
	return op
}

func ruleDiscardFamilyEdge(role uint32, family SubedgeFamily, arguments Values, throwOutcome, cancelOutcome uint32, cancel Values) Subedge {
	edge := ruleFamilyEdge(role, family, arguments, throwOutcome, cancelOutcome, cancel)
	edge.Routes[0] = continueRoute(OutcomeNormal, emptyValues(), AdjustmentExact)
	edge.Routes[1] = continueRoute(OutcomeReturn, emptyValues(), AdjustmentExact)
	return edge
}

func tableInsertProfile() Operation {
	op := tableOperation("insert", []typ.Type{typ.Any}, true, emptyValues())
	op.Subedges = []Subedge{
		ruleFamilyEdge(1, SubedgeFamilyLength, anyValue(), 1, 2, nativeCallbackTail(1)),
		ruleFamilyEdge(2, SubedgeFamilyIndexGet, closed(typ.Any, typ.Integer), 1, 2, nativeCallbackTail(1)),
		ruleDiscardFamilyEdge(3, SubedgeFamilyIndexSet, closed(typ.Any, typ.Integer, typ.Any), 1, 2, nativeCallbackTail(1)),
		ruleDiscardFamilyEdge(4, SubedgeFamilyIndexSet, closed(typ.Any, typ.Integer, typ.Any), 1, 2, nativeCallbackTail(1)),
	}
	return op
}

func tableRemoveProfile() Operation {
	op := tableOperation("remove", []typ.Type{typ.Any}, true, anyValue())
	op.Subedges = []Subedge{
		ruleFamilyEdge(1, SubedgeFamilyLength, anyValue(), 1, 2, nativeCallbackTail(1)),
		ruleFamilyEdge(2, SubedgeFamilyIndexGet, closed(typ.Any, typ.Integer), 1, 2, nativeCallbackTail(1)),
		ruleFamilyEdge(3, SubedgeFamilyIndexGet, closed(typ.Any, typ.Integer), 1, 2, nativeCallbackTail(1)),
		ruleDiscardFamilyEdge(4, SubedgeFamilyIndexSet, closed(typ.Any, typ.Integer, typ.Any), 1, 2, nativeCallbackTail(1)),
		ruleDiscardFamilyEdge(5, SubedgeFamilyIndexSet, closed(typ.Any, typ.Integer, typ.Nil), 1, 2, nativeCallbackTail(1)),
	}
	return op
}

func tableMoveProfile() Operation {
	op := tableOperation("move", []typ.Type{typ.Any, typ.Integer, typ.Integer, typ.Integer}, true, anyValue())
	op.Subedges = []Subedge{
		ruleFamilyEdge(1, SubedgeFamilyIndexGet, closed(typ.Any, typ.Integer), 1, 2, nativeCallbackTail(1)),
		ruleDiscardFamilyEdge(2, SubedgeFamilyIndexSet, closed(typ.Any, typ.Integer, typ.Any), 1, 2, nativeCallbackTail(1)),
		ruleFamilyEdge(3, SubedgeFamilyEqual, closed(typ.Any, typ.Any), 1, 2, nativeCallbackTail(1)),
	}
	return op
}

func tableUnpackProfile() Operation {
	op := Operation{ValuesVars: 3,
		Input: values([]typ.Type{typ.Any}, true, 0), Outcomes: []Outcome{{Kind: OutcomeNormal, Values: nativeCallbackTail(1)}, {Kind: OutcomeThrow, Values: anyValue()}, {Kind: OutcomeCancel, Values: nativeCallbackTail(2)}}, Effects: RowSpec{Tail: RowClosed}}
	op.Subedges = []Subedge{
		ruleFamilyEdge(1, SubedgeFamilyLength, anyValue(), 1, 2, nativeCallbackTail(2)),
		ruleFamilyEdge(2, SubedgeFamilyIndexGet, closed(typ.Any, typ.Integer), 1, 2, nativeCallbackTail(2)),
	}
	return op
}

func tableSortProfile() Operation {
	op := module("table", "sort")
	op.ValuesVars = 1
	op.Input = closed(typ.Any, typ.Any)
	op.Callbacks = []Callback{{Function: InputSource{Kind: InputSourceValue, Ordinal: 1}, Admission: CallableAdmissionDirectFunction, Arguments: closed(typ.Any, typ.Any), Outcomes: terminals(nativeCallbackTail(0), nativeCallbackTail(0), anyValue(), anyValue(), nativeCallbackTail(0)), Lifecycle: CallbackSyncOptionalMany, Effects: RowSpec{Tail: RowClosed}}}
	op.Outcomes = []Outcome{{Kind: OutcomeNormal, Values: emptyValues()}, {Kind: OutcomeThrow, Values: anyValue()}, {Kind: OutcomeCancel, Values: nativeCallbackTail(0)}}
	comparator := Subedge{Role: 3, Family: SubedgeFamilyCall, Callee: SubedgeCallee{Kind: SubedgeCalleeCallback, Callback: 1}, ArgumentOrigins: ruleOrigins(2), AdmissionFailure: admissionToOutcome(anyValue(), AdjustmentPreserve, PlacementFixed, 1), Routes: []SubedgeRoute{
		continueRoute(OutcomeNormal, anyValue(), AdjustmentExact), continueRoute(OutcomeReturn, anyValue(), AdjustmentExact), outcomeRoute(OutcomeThrow, anyValue(), AdjustmentPreserve, PlacementFixed, 1), rejectRoute(1), outcomeRoute(OutcomeCancel, nativeCallbackTail(0), AdjustmentPreserve, PlacementTail, 2),
	}}
	op.Subedges = []Subedge{
		ruleFamilyEdge(1, SubedgeFamilyLength, anyValue(), 1, 2, nativeCallbackTail(0)),
		ruleFamilyEdge(2, SubedgeFamilyIndexGet, closed(typ.Any, typ.Integer), 1, 2, nativeCallbackTail(0)),
		comparator,
		ruleFamilyEdge(4, SubedgeFamilyLess, closed(typ.Any, typ.Any), 1, 2, nativeCallbackTail(0)),
		ruleDiscardFamilyEdge(5, SubedgeFamilyIndexSet, closed(typ.Any, typ.Integer, typ.Any), 1, 2, nativeCallbackTail(0)),
		ruleDiscardFamilyEdge(6, SubedgeFamilyIndexSet, closed(typ.Any, typ.Integer, typ.Any), 1, 2, nativeCallbackTail(0)),
	}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}

func callbackGsubProfile() Operation {
	op := module("string", "gsub")
	op.ValuesVars = 4
	op.Input = values([]typ.Type{typ.String, typ.String, typ.Any}, true, 0)
	op.Callbacks = []Callback{{Function: InputSource{Kind: InputSourceValue, Ordinal: 2}, Admission: CallableAdmissionDirectFunction, Arguments: nativeCallbackTail(1), Outcomes: terminals(nativeCallbackTail(2), nativeCallbackTail(2), anyValue(), anyValue(), nativeCallbackTail(3)), Lifecycle: CallbackSyncOptionalMany, Effects: RowSpec{Tail: RowClosed}}}
	op.Outcomes = []Outcome{{Kind: OutcomeNormal, Values: closed(typ.String, typ.Integer)}, {Kind: OutcomeThrow, Values: anyValue()}, {Kind: OutcomeCancel, Values: nativeCallbackTail(3)}}
	function := Subedge{Role: 1, Family: SubedgeFamilyCall, Callee: SubedgeCallee{Kind: SubedgeCalleeCallback, Callback: 1}, ArgumentOrigins: ruleTailOrigin(), AdmissionFailure: admissionToOutcome(anyValue(), AdjustmentPreserve, PlacementFixed, 1), Routes: []SubedgeRoute{
		continueRoute(OutcomeNormal, anyValue(), AdjustmentExact), continueRoute(OutcomeReturn, anyValue(), AdjustmentExact), outcomeRoute(OutcomeThrow, anyValue(), AdjustmentPreserve, PlacementFixed, 1), rejectRoute(1), outcomeRoute(OutcomeCancel, nativeCallbackTail(3), AdjustmentPreserve, PlacementTail, 2),
	}}
	table := ruleFamilyEdge(2, SubedgeFamilyIndexGet, closed(typ.Any, typ.Any), 1, 2, nativeCallbackTail(3))
	// The table branch is distinct from the function callback: gsub indexes
	// the replacement table with the first capture or whole match, supplied by
	// its own closed Rule coordinate rather than an invented callback input.
	table.ArgumentOrigins = []ArgumentOrigin{{Segment: ArgumentFixed, Index: 0, Kind: ArgumentSourceInput, Source: InputSource{Kind: InputSourceValue, Ordinal: 2}}, {Segment: ArgumentFixed, Index: 1, Kind: ArgumentSourceRule}}
	op.Subedges = []Subedge{function, table}
	op.GsubTableReplacement = &GsubTableReplacement{Replacement: 2, Access: 2, ResultOutcome: 0, Result: 0, EffectAliases: []uint32{0}}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}

func minMaxProfile(op Operation) Operation {
	op.ValuesVars = 2
	op.Input = values([]typ.Type{typ.Any}, true, 0)
	op.Outcomes = []Outcome{{Kind: OutcomeNormal, Values: anyValue()}, {Kind: OutcomeThrow, Values: anyValue()}, {Kind: OutcomeCancel, Values: nativeCallbackTail(1)}}
	op.Subedges = []Subedge{ruleFamilyEdge(1, SubedgeFamilyLess, closed(typ.Any, typ.Any), 1, 2, nativeCallbackTail(1))}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}

func resumeEnvelope() Operation {
	op := module("coroutine", "resume")
	op.ValuesVars = 3
	op.Input = values([]typ.Type{typ.Any}, true, 0)
	op.Outcomes = []Outcome{
		{Kind: OutcomeNormal, Values: values([]typ.Type{typ.LiteralBool(true)}, true, 1)},
		{Kind: OutcomeNormal, Values: values([]typ.Type{typ.LiteralBool(false)}, true, 2)},
	}
	op.Resumes = []Resume{resumeRelation(ResumeSourceValue, 0, 0, 0, 1)}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}

// resumeRelation is the complete activation-boundary correspondence shared by
// coroutine.resume and a produced coroutine.wrap invocation. Successful
// return and yield are ordinary successful results; restored throw/cancel
// select the operation's failure outcome. The carrier's resumption arguments
// are exactly the operation's incoming open Values variable.
func resumeRelation(source ResumeSource, carrier ValueFormal, arguments ValuesVar, success, failure uint32) Resume {
	return Resume{
		Source: source, Carrier: carrier, Arguments: nativeCallbackTail(arguments),
		Outcomes: []ResumeOutcome{
			{Kind: OutcomeNormal, Outcome: success},
			{Kind: OutcomeReturn, Outcome: success},
			{Kind: OutcomeThrow, Outcome: failure},
			{Kind: OutcomeYield, Outcome: success},
			{Kind: OutcomeCancel, Outcome: failure},
		},
	}
}

func callbackCreate(op Operation) Operation {
	op.ValuesVars = 5
	op.Input = values([]typ.Type{typ.Any}, false, 0)
	op.Callbacks = []Callback{{Function: InputSource{Kind: InputSourceValue, Ordinal: 0}, Admission: CallableAdmissionDirectFunction, Arguments: nativeCallbackTail(0), Outcomes: nativeCallbackOutcomes(1, 1, 2, 3, 4), Lifecycle: CallbackRetainedOptionalOnce, Effects: RowSpec{Tail: RowClosed}}}
	op.Outcomes = []Outcome{{Kind: OutcomeNormal, Values: values([]typ.Type{typ.Any}, false, 0), CallbackResults: []CallbackResult{{Result: 0, Callback: 1}}}, {Kind: OutcomeThrow, Values: values([]typ.Type{typ.Any}, false, 0)}}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}

func callbackWrap(op Operation) Operation {
	op.ValuesVars = 5
	op.Input = values([]typ.Type{typ.Any}, false, 0)
	op.Callbacks = []Callback{{Function: InputSource{Kind: InputSourceValue, Ordinal: 0}, Admission: CallableAdmissionDirectFunction, Arguments: nativeCallbackTail(0), Outcomes: nativeCallbackOutcomes(1, 1, 2, 3, 4), Lifecycle: CallbackRetainedOptionalOnce, Effects: RowSpec{Tail: RowClosed}}}
	op.Outcomes = []Outcome{{Kind: OutcomeNormal, Values: values([]typ.Type{typ.Any}, false, 0)}, {Kind: OutcomeThrow, Values: values([]typ.Type{typ.Any}, false, 0)}}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}

// callbackSpawn is a detached, one-shot callback activation.  Its parent
// system-yield and later empty resume share the typed Spawn; the callback
// keeps the exact function/closure authority and its complete outcome rows.
func callbackSpawn() Operation {
	op := module("coroutine", "spawn")
	op.ValuesVars = 7 // input tail, empty child entry coordinate, five child outcomes
	op.Input = values([]typ.Type{typ.Any}, true, 0)
	op.Callbacks = []Callback{{
		Function:  InputSource{Kind: InputSourceValue, Ordinal: 0},
		Admission: CallableAdmissionDirectFunction, Arguments: nativeCallbackTail(1), Outcomes: nativeCallbackOutcomes(2, 3, 4, 5, 6),
		Lifecycle: CallbackRetainedRequiredOnce, Effects: RowSpec{Tail: RowClosed},
	}}
	op.Outcomes = []Outcome{
		{Kind: OutcomeYield, Values: values(nil, false, 0)},
		{Kind: OutcomeNormal, Values: values(nil, false, 0)},
		{Kind: OutcomeThrow, Values: values([]typ.Type{typ.Any}, false, 0)},
	}
	op.Suspensions = []Suspension{{Yield: 0, Reentry: 1, Source: ReentryByProvider, Multiplicity: ReentryOnce}}
	op.Spawns = []Spawn{{
		Function: InputSource{Kind: InputSourceValue, Ordinal: 0}, Child: 1,
		Yield: 0, ParentResume: 1, ChildEntry: 1,
		Alternatives: []SpawnSiblingAlternative{SpawnChildEntryThenParentResume, SpawnParentResumeThenChildEntry},
	}}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}

func nativeCallbackTail(variable ValuesVar) Values {
	return values(nil, true, variable)
}

func nativeCallbackOutcomes(normal, returned, thrown, yielded, canceled ValuesVar) []Terminal {
	return []Terminal{
		{Kind: OutcomeNormal, Values: nativeCallbackTail(normal)},
		{Kind: OutcomeReturn, Values: nativeCallbackTail(returned)},
		{Kind: OutcomeThrow, Values: nativeCallbackTail(thrown)},
		{Kind: OutcomeYield, Values: nativeCallbackTail(yielded)},
		{Kind: OutcomeCancel, Values: nativeCallbackTail(canceled)},
	}
}

func replacement(operation Operation) Operation {
	operation.Replace = true
	return operation
}

func amendment(amendments ...OutcomeAmendment) Operation {
	return Operation{OutcomeAmendments: amendments}
}

func aliasAmendment(outcome, result, source uint32) OutcomeAmendment {
	return OutcomeAmendment{Outcome: outcome, ResultAliases: []ResultAlias{{
		Result: result, Source: InputSource{Kind: InputSourceValue, Ordinal: source},
	}}}
}

func freshAmendment(outcome, result uint32, class FreshClass) OutcomeAmendment {
	return OutcomeAmendment{Outcome: outcome, FreshResults: []FreshResult{{Result: result, Class: class}}}
}

func producedAmendment(outcome, result uint32, operation string, captures ...Capture) OutcomeAmendment {
	return OutcomeAmendment{Outcome: outcome, Produced: []Produced{{Result: result, Operation: operation, Captures: captures}}}
}

func joinAmendments(operation Operation, additions ...OutcomeAmendment) Operation {
	operation.OutcomeAmendments = append(operation.OutcomeAmendments, additions...)
	return operation
}

func ipairsOperationLaw() Operation {
	ipairs := replacement(ipairsProfile())
	return joinAmendments(ipairs,
		producedAmendment(0, 0, "ipairs.aux"),
		aliasAmendment(0, 1, 0),
	)
}

func pairsOperationLaw() Operation {
	pairs := replacement(pairsProfile())
	return joinAmendments(pairs,
		producedAmendment(1, 0, "next"),
		aliasAmendment(1, 1, 0),
	)
}

func baseDetachedFunctions() map[string]detachedFunction {
	op := replacement(ipairsAuxProfile())
	op.SelfEffect = true
	return map[string]detachedFunction{"ipairs.aux": detached(op)}
}

func tableCreateOperationLaw() Operation {
	return amendment(freshAmendment(0, 0, FreshTable))
}

func stringGmatchOperationLaw() Operation {
	return amendment(
		producedAmendment(0, 0, "string.gmatch.next",
			Capture{Kind: CaptureValue, Ordinal: 0},
			Capture{Kind: CaptureValue, Ordinal: 1}),
		freshAmendment(0, 0, FreshFunction),
	)
}

func stringFindOperationLaw() Operation  { return Operation{AppendNormal: []Values{closed(typ.Nil)}} }
func stringMatchOperationLaw() Operation { return Operation{AppendNormal: []Values{closed(typ.Nil)}} }
func stringCharOperationLaw() Operation  { return Operation{InputTailType: typ.Integer} }
func stringByteOperationLaw() Operation {
	return Operation{OutcomeTailTypes: []OutcomeTailType{{Outcome: 0, Type: typ.Integer}}}
}

func stringDetachedFunctions() map[string]detachedFunction {
	next := normal(Operation{}, nil, false, nil, true)
	next.Outcomes = append(next.Outcomes, Outcome{Kind: OutcomeNormal, Values: closed()})
	next = replacement(next)
	next.SelfEffect = true
	return map[string]detachedFunction{"gmatch.next": detached(next)}
}

func mathRandomOperationLaw() Operation {
	return Operation{AppendNormal: []Values{closed(typ.Integer)}}
}
func mathToIntegerOperationLaw() Operation {
	return Operation{ReplaceNormalSet: true, ReplaceNormal: []Values{closed(typ.Integer), closed(typ.Nil)}}
}
func mathTypeOperationLaw() Operation {
	return Operation{ReplaceNormalSet: true, ReplaceNormal: []Values{closed(typ.String), closed(typ.Nil)}}
}
func debugGetUpvalueOperationLaw() Operation {
	return replacement(alternatives(module("debug", "getupvalue"), []typ.Type{typ.Any, typ.Integer}, false, [][]typ.Type{{typ.String, typ.Any}, {typ.Nil}}))
}

func coroutineCreateOperationLaw() Operation {
	create := replacement(callbackCreate(module("coroutine", "create")))
	return joinAmendments(create, freshAmendment(0, 0, FreshThread))
}

func coroutineWrapOperationLaw() Operation {
	wrap := replacement(callbackWrap(module("coroutine", "wrap")))
	return joinAmendments(wrap,
		freshAmendment(0, 0, FreshFunction),
		producedAmendment(0, 0, "coroutine.wrap.invoke", Capture{Kind: CaptureCallback, Ordinal: 1}),
	)
}

func coroutineYieldOperationLaw() Operation {
	// The yielded payload and reentry payload share the operation's sole open
	// Values variable; the successful call return remains its fixed Any result.
	yield := replacement(Operation{ValuesVars: 1, Input: values(nil, true, 0), Effects: RowSpec{Tail: RowClosed}})
	yield.Outcomes = []Outcome{
		{Kind: OutcomeNormal, Values: closed(typ.Any)},
		{Kind: OutcomeThrow, Values: anyValue()},
		{Kind: OutcomeYield, Values: values(nil, true, 0)},
	}
	yield.Suspensions = []Suspension{{Yield: 2, Reentry: 0, Source: ReentryByCall, Multiplicity: ReentryOnce}}
	return yield
}

func coroutineDetachedFunctions() map[string]detachedFunction {
	invoke := normal(Operation{}, nil, true, nil, true)
	invoke.Resumes = []Resume{resumeRelation(ResumeSourceProduced, 0, 0, 0, 1)}
	return map[string]detachedFunction{"wrap.invoke": detached(replacement(invoke))}
}

func utf8LenOperationLaw() Operation {
	return Operation{ReplaceNormalSet: true, ReplaceNormal: []Values{closed(typ.Integer), closed(typ.Nil, typ.Integer)}}
}
func utf8OffsetOperationLaw() Operation { return Operation{AppendNormal: []Values{closed(typ.Nil)}} }
func utf8CodepointOperationLaw() Operation {
	return Operation{OutcomeTailTypes: []OutcomeTailType{{Outcome: 0, Type: typ.Integer}}}
}
func utf8CodesOperationLaw() Operation {
	return amendment(producedAmendment(0, 0, "utf8.codes.aux"), aliasAmendment(0, 1, 0))
}

func utf8DetachedFunctions() map[string]detachedFunction {
	op := alternatives(Operation{}, []typ.Type{typ.String, typ.Integer}, false, [][]typ.Type{nil, {typ.Integer, typ.Integer}})
	return map[string]detachedFunction{"codes.aux": detached(replacement(op))}
}

func errorsDetailsOperationLaw() Operation {
	details := alternativesTotal(Operation{}, []typ.Type{typ.Any}, false, [][]typ.Type{{typ.BuiltinTableTopMarker()}, {typ.Nil}})
	return joinAmendments(replacement(details), freshAmendment(0, 0, FreshTable))
}
func errorsNewOperationLaw() Operation  { return amendment(freshAmendment(0, 0, FreshError)) }
func errorsWrapOperationLaw() Operation { return amendment(freshAmendment(0, 0, FreshError)) }

func detached(operation Operation) detachedFunction {
	return detachedFunction{
		signature: signature.Function{Type: typ.Func().Build(), Effect: effect.Empty},
		operation: operation,
	}
}
