// Catalogue compilation turns provider-owned manifests into the immutable
// operation table consumed by Program and Engine. It is deliberately part of
// target: there is no parallel profile registry or adapter package.
package library

import (
	"context"
	"fmt"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"github.com/wippyai/go-lua/domain/type/typ"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	"github.com/wippyai/go-lua/manifest"
)

// SealCatalogue is the sole manifest-to-analysis entry point. Providers own
// declarations; target only validates and freezes their analysis projection.
func SealCatalogue(declarations *manifest.Catalogue) (*Contract, error) {
	spec, err := CompileCatalogue(declarations)
	if err != nil {
		return nil, err
	}
	return Seal(&spec)
}

// CompileCatalogue returns the one-shot authored form used by Seal. It is
// exposed for contract-law tests and tools that need to inspect the projection
// before it becomes immutable.
func CompileCatalogue(declarations *manifest.Catalogue) (Spec, error) {
	catalogue, err := operations(declarations)
	if err != nil {
		return Spec{}, err
	}
	// Callable-valued results refer to the sole ordinary operation that they
	// produce.  The dynamically selected iterator/resume behavior is Rule work.
	produce := func(producer, child string, captures ...CaptureSpec) error {
		return catalogue.produce(producer, child, captures...)
	}
	for _, relation := range []struct {
		producer, child string
		captures        []CaptureSpec
	}{
		{"coroutine.wrap", "coroutine.wrap.invoke", []CaptureSpec{{Kind: CaptureCallback, Ordinal: 1}}},
		{"string.gmatch", "string.gmatch.next", []CaptureSpec{{Kind: CaptureValueFormal, Ordinal: 0}, {Kind: CaptureValueFormal, Ordinal: 1}}},
		{"ipairs", "ipairs.aux", nil},
		{"utf8.codes", "utf8.codes.aux", nil},
	} {
		if err := produce(relation.producer, relation.child, relation.captures...); err != nil {
			return Spec{}, err
		}
	}
	// pairs has two distinct successful laws.  Only the no-__pairs fallback
	// produces next; a user __pairs hook supplies all three arbitrary results.
	if err := catalogue.produceAt("pairs", 1, "next"); err != nil {
		return Spec{}, err
	}
	for _, alias := range []struct {
		operation      string
		result, source uint32
	}{
		{"setmetatable", 0, 0}, {"rawset", 0, 0}, {"table.freeze", 0, 0},
		{"ipairs", 1, 0}, {"utf8.codes", 1, 0},
	} {
		if err := catalogue.resultAlias(alias.operation, alias.result, alias.source); err != nil {
			return Spec{}, err
		}
	}
	if err := catalogue.resultAliasAt("pairs", 1, 1, 0); err != nil {
		return Spec{}, err
	}
	// These result roots are nominal allocation facts. Their ordinary values,
	// callback, and produced-operation relations remain separate Target rows.
	for _, fresh := range []struct {
		operation string
		kind      schematype.FreshClass
	}{
		{"table.create", schematype.FreshClassTable},
		{"coroutine.create", schematype.FreshClassThread},
		{"coroutine.wrap", schematype.FreshClassFunction},
		{"string.gmatch", schematype.FreshClassFunction},
		{"errors.new", schematype.FreshClassError},
		{"errors.wrap", schematype.FreshClassError},
		{"errors.Error.details", schematype.FreshClassTable},
	} {
		if err := catalogue.freshResult(fresh.operation, fresh.kind); err != nil {
			return Spec{}, err
		}
	}
	if err := catalogue.selfEffects(declarations); err != nil {
		return Spec{}, err
	}
	boot, err := bootLedger(catalogue, declarations)
	if err != nil {
		return Spec{}, err
	}
	return Spec{
		Semantics:         domaincontract.NewSemantics(),
		Operations:        catalogue.operations,
		InitialRoots:      boot.roots,
		InitialEntries:    boot.entries,
		InitialBindings:   boot.bindings,
		InitialMetatables: boot.metatables,
	}, nil
}

// CatalogueBindings returns the bindings projected from one sealed manifest
// catalogue. The returned values own their slices.
func CatalogueBindings(declarations *manifest.Catalogue) ([]BindingSpec, error) {
	catalogue, err := operations(declarations)
	if err != nil {
		return nil, err
	}
	out := make([]BindingSpec, 0, len(catalogue.operations))
	for _, op := range catalogue.operations {
		out = append(out, op.Bindings...)
	}
	return out, nil
}

type operationRef uint32

// authoredCatalogue owns the one closed name-to-operation identity table.
// A zero operationRef is invalid, so an absent name can never accidentally
// become SpecRef(1), the first valid authored operation.
type authoredCatalogue struct {
	operations []OperationSpec
	names      map[string]operationRef
}

func (catalogue *authoredCatalogue) add(name string, operation OperationSpec) {
	if catalogue.names == nil {
		catalogue.names = make(map[string]operationRef)
	}
	ref := operationRef(len(catalogue.operations) + 1)
	catalogue.names[name] = ref
	catalogue.operations = append(catalogue.operations, operation)
}

func (catalogue *authoredCatalogue) lookup(name string) (operationRef, bool) {
	ref, ok := catalogue.names[name]
	if !ok || ref == 0 || int(ref) > len(catalogue.operations) {
		return 0, false
	}
	return ref, true
}

func (catalogue *authoredCatalogue) require(name string) (operationRef, error) {
	ref, ok := catalogue.lookup(name)
	if !ok {
		return 0, fmt.Errorf("target catalogue: unknown authored operation %q", name)
	}
	return ref, nil
}

func (catalogue *authoredCatalogue) at(ref operationRef) *OperationSpec {
	return &catalogue.operations[uint32(ref)-1]
}

func (catalogue *authoredCatalogue) replace(name string, operation OperationSpec) error {
	ref, err := catalogue.require(name)
	if err != nil {
		return err
	}
	// Mount coordinates always come from the provider manifest. Specialized
	// operational laws may change control-flow structure, never bindings.
	operation.Bindings = catalogue.at(ref).Bindings
	*catalogue.at(ref) = operation
	return nil
}

func (catalogue *authoredCatalogue) produce(producer, child string, captures ...CaptureSpec) error {
	return catalogue.produceAt(producer, 0, child, captures...)
}

func (catalogue *authoredCatalogue) produceAt(producer string, outcome int, child string, captures ...CaptureSpec) error {
	producerRef, err := catalogue.require(producer)
	if err != nil {
		return err
	}
	childRef, err := catalogue.require(child)
	if err != nil {
		return err
	}
	op := catalogue.at(producerRef)
	if outcome < 0 || outcome >= len(op.Outcomes) {
		return fmt.Errorf("target catalogue: outcome %d outside %q", outcome, producer)
	}
	op.Outcomes[outcome].Produced = []ProducedSpec{{
		Result: 0, Operation: SpecRef(childRef), Captures: captures,
	}}
	return nil
}

func (catalogue *authoredCatalogue) resultAlias(operation string, result, source uint32) error {
	return catalogue.resultAliasAt(operation, 0, result, source)
}

func (catalogue *authoredCatalogue) resultAliasAt(operation string, outcome int, result, source uint32) error {
	ref, err := catalogue.require(operation)
	if err != nil {
		return err
	}
	op := catalogue.at(ref)
	if outcome < 0 || outcome >= len(op.Outcomes) {
		return fmt.Errorf("target catalogue: outcome %d outside %q", outcome, operation)
	}
	op.Outcomes[outcome].ResultAliases = []ResultAliasSpec{{
		Result: result, Source: InputSource{Kind: InputSourceValueFormal, Ordinal: source},
	}}
	return nil
}

func (catalogue *authoredCatalogue) freshResult(operation string, kind schematype.FreshClass) error {
	ref, err := catalogue.require(operation)
	if err != nil {
		return err
	}
	op := catalogue.at(ref)
	if len(op.Outcomes) == 0 || len(op.Outcomes[0].Values.Fixed) == 0 {
		return fmt.Errorf("target catalogue: %q has no first fixed normal result for FreshResult", operation)
	}
	op.Outcomes[0].FreshResults = []FreshResultSpec{{Result: 0, Kind: kind}}
	return nil
}

func (catalogue *authoredCatalogue) inputTailType(operation string, class typ.Type) error {
	ref, err := catalogue.require(operation)
	if err != nil {
		return err
	}
	values := &catalogue.at(ref).Input
	if values.Tail != ValuesVariable {
		return fmt.Errorf("target catalogue: %q input has no open Values tail", operation)
	}
	values.TailType = portable(class)
	return nil
}

func (catalogue *authoredCatalogue) outcomeTailType(operation string, outcome int, class typ.Type) error {
	ref, err := catalogue.require(operation)
	if err != nil {
		return err
	}
	op := catalogue.at(ref)
	if outcome < 0 || outcome >= len(op.Outcomes) || op.Outcomes[outcome].Values.Tail != ValuesVariable {
		return fmt.Errorf("target catalogue: %q outcome %d has no open Values tail", operation, outcome)
	}
	op.Outcomes[outcome].Values.TailType = portable(class)
	return nil
}

func nativeBuiltin(name string) OperationSpec {
	return OperationSpec{Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}}}
}
func module(owner, name string) OperationSpec {
	return OperationSpec{Bindings: []BindingSpec{{Namespace: BindingModule, Owner: []string{owner}, Member: []string{name}}}}
}
func aliasModule(owner string, members ...string) OperationSpec {
	b := make([]BindingSpec, len(members))
	for i, member := range members {
		b[i] = BindingSpec{Namespace: BindingModule, Owner: []string{owner}, Member: []string{member}}
	}
	return OperationSpec{Bindings: b}
}
func method(owner, family, name string, in, out []typ.Type) OperationSpec {
	return fixed(OperationSpec{Bindings: []BindingSpec{{Namespace: BindingModule, Owner: []string{owner}, Member: []string{family, name}}}}, append([]typ.Type{typ.Any}, in...), out)
}
func values(fixed []typ.Type, open bool, variable ValuesVar) ValuesSpec {
	tail := ValuesClosed
	var tailType schematype.Type
	if open {
		tail = ValuesVariable
		tailType = portable(typ.Any)
	}
	return ValuesSpec{Fixed: portableList(fixed), Tail: tail, Var: variable, TailType: tailType}
}

// portable is the only place this Lua catalogue crosses into Program's
// neutral authored ABI. All interpretation and validation stays in the Lua
// type-domain adapter; target receives only schema/typecontract declarations.
func portable(value typ.Type) schematype.Type {
	encoded, err := domaincontract.EncodeStorage(context.Background(), value, nil)
	if err != nil {
		panic(fmt.Sprintf("target catalogue: portable type: %v", err))
	}
	return encoded
}

func portableList(values []typ.Type) []schematype.Type {
	if len(values) == 0 {
		return nil
	}
	out := make([]schematype.Type, len(values))
	for index, value := range values {
		out[index] = portable(value)
	}
	return out
}

func closed(fixed ...typ.Type) ValuesSpec {
	return values(fixed, false, 0)
}

func anyValue() ValuesSpec { return closed(typ.Any) }

func emptyValues() ValuesSpec { return closed() }

func rejectedYield() ValuesSpec {
	return closed(typ.LiteralString("attempt to yield across a C-call boundary"))
}

func terminals(normal, returned, thrown, yielded, canceled ValuesSpec) []TerminalSpec {
	return []TerminalSpec{
		{Kind: flowkind.OutcomeNormal, Values: normal},
		{Kind: flowkind.OutcomeReturn, Values: returned},
		{Kind: flowkind.OutcomeThrow, Values: thrown},
		{Kind: flowkind.OutcomeYield, Values: yielded},
		{Kind: flowkind.OutcomeCancel, Values: canceled},
	}
}

func outcomeRoute(kind flowkind.OutcomeKind, result ValuesSpec, adjustment Adjustment, placement Placement, outcome uint32) SubedgeRouteSpec {
	return SubedgeRouteSpec{Kind: kind, Route: RouteOutcome, Adjustment: adjustment, Result: result, Placement: placement, Outcome: outcome}
}

func siblingRoute(kind flowkind.OutcomeKind, result ValuesSpec, adjustment Adjustment, sibling SubedgeRef) SubedgeRouteSpec {
	return SubedgeRouteSpec{Kind: kind, Route: RouteSubedge, Adjustment: adjustment, Result: result, Placement: PlacementFixed, Subedge: sibling}
}

func continueRoute(kind flowkind.OutcomeKind, result ValuesSpec, adjustment Adjustment) SubedgeRouteSpec {
	return SubedgeRouteSpec{Kind: kind, Route: RouteContinue, Adjustment: adjustment, Result: result}
}

func propagateRoute(values ValuesSpec) SubedgeRouteSpec {
	return SubedgeRouteSpec{Kind: flowkind.OutcomeYield, Route: RoutePropagateYield, Adjustment: AdjustmentPreserve, Result: values}
}

func rejectRoute(outcome uint32) SubedgeRouteSpec {
	return SubedgeRouteSpec{Kind: flowkind.OutcomeYield, Route: RouteRejectYield, Adjustment: AdjustmentExact, Result: rejectedYield(), Placement: PlacementFixed, Outcome: outcome}
}

func rejectSiblingRoute(sibling SubedgeRef) SubedgeRouteSpec {
	return SubedgeRouteSpec{Kind: flowkind.OutcomeYield, Route: RouteRejectYield, Adjustment: AdjustmentExact, Result: rejectedYield(), Placement: PlacementFixed, Subedge: sibling}
}

func admissionToOutcome(result ValuesSpec, adjustment Adjustment, placement Placement, outcome uint32) AdmissionFailureSpec {
	return AdmissionFailureSpec{Values: anyValue(), Route: AdmissionRouteSpec{Route: RouteOutcome, Adjustment: adjustment, Result: result, Placement: placement, Outcome: outcome}}
}

func admissionToSibling(result ValuesSpec, sibling SubedgeRef) AdmissionFailureSpec {
	return AdmissionFailureSpec{Values: anyValue(), Route: AdmissionRouteSpec{Route: RouteSubedge, Adjustment: AdjustmentExact, Result: result, Placement: PlacementFixed, Subedge: sibling}}
}

func tailInputOrigin(variable ValuesVar) []ArgumentOrigin {
	return []ArgumentOrigin{{Segment: ArgumentTail, Kind: ArgumentSourceInput, Source: InputSource{Kind: InputSourceValuesVar, Ordinal: uint32(variable)}}}
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
	return ArgumentOrigin{Segment: ArgumentFixed, Index: index, Kind: ArgumentSourceInput, Source: InputSource{Kind: InputSourceValueFormal, Ordinal: index}}
}

// ruleFamilyEdge declares a non-yieldable target-machine application whose
// operands and dependent result formula belong to the one owning operation
// Rule.  The Target row owns only the typed application boundary and complete
// terminal disposition.
func ruleFamilyEdge(role uint32, family SubedgeFamily, arguments ValuesSpec, throwOutcome, cancelOutcome uint32, cancel ValuesSpec) SubedgeSpec {
	return SubedgeSpec{
		Role: role, Family: family, Admission: schematype.CallableAdmissionOrdinary, Arguments: arguments,
		ArgumentOrigins:  ruleOrigins(len(arguments.Fixed)),
		Outcomes:         terminals(anyValue(), anyValue(), anyValue(), anyValue(), cancel),
		AdmissionFailure: admissionToOutcome(anyValue(), AdjustmentPreserve, PlacementFixed, throwOutcome),
		Routes: []SubedgeRouteSpec{
			continueRoute(flowkind.OutcomeNormal, anyValue(), AdjustmentExact),
			continueRoute(flowkind.OutcomeReturn, anyValue(), AdjustmentExact),
			outcomeRoute(flowkind.OutcomeThrow, anyValue(), AdjustmentPreserve, PlacementFixed, throwOutcome),
			rejectRoute(throwOutcome),
			outcomeRoute(flowkind.OutcomeCancel, cancel, AdjustmentPreserve, PlacementTail, cancelOutcome),
		},
	}
}

func ruleMetaCallEdge(role uint32, key keyspace.LiteralValue, arguments ValuesSpec, throwOutcome, cancelOutcome uint32, cancel ValuesSpec) SubedgeSpec {
	return SubedgeSpec{
		Role: role, Family: SubedgeFamilyCall,
		Callee: SubedgeCalleeSpec{Kind: SubedgeCalleeMetaKey, MetaKey: key}, Admission: schematype.CallableAdmissionOrdinary,
		Arguments: arguments, ArgumentOrigins: ruleOrigins(len(arguments.Fixed)),
		Outcomes:         terminals(anyValue(), anyValue(), anyValue(), anyValue(), cancel),
		AdmissionFailure: admissionToOutcome(anyValue(), AdjustmentPreserve, PlacementFixed, throwOutcome),
		Routes: []SubedgeRouteSpec{
			continueRoute(flowkind.OutcomeNormal, anyValue(), AdjustmentExact),
			continueRoute(flowkind.OutcomeReturn, anyValue(), AdjustmentExact),
			outcomeRoute(flowkind.OutcomeThrow, anyValue(), AdjustmentPreserve, PlacementFixed, throwOutcome),
			rejectRoute(throwOutcome),
			outcomeRoute(flowkind.OutcomeCancel, cancel, AdjustmentPreserve, PlacementTail, cancelOutcome),
		},
	}
}

func literalKey(text string) keyspace.LiteralValue {
	return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: text}
}
func normal(op OperationSpec, in []typ.Type, openIn bool, out []typ.Type, openOut bool) OperationSpec {
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
	op.Outcomes = []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: values(out, openOut, outVar)}, {Kind: flowkind.OutcomeThrow, Values: values([]typ.Type{typ.Any}, false, 0)}}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}
func fixed(op OperationSpec, in, out []typ.Type) OperationSpec {
	return normal(op, in, false, out, false)
}
func total(op OperationSpec, in []typ.Type, openIn bool, out []typ.Type, openOut bool) OperationSpec {
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
	op.Outcomes = []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: values(out, openOut, outVar)}}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}
func throws(op OperationSpec, in []typ.Type, open bool) OperationSpec {
	vars := uint32(0)
	if open {
		vars = 1
	}
	op.ValuesVars = vars
	op.Input = values(in, open, 0)
	op.Outcomes = []OutcomeSpec{{Kind: flowkind.OutcomeThrow, Values: values([]typ.Type{typ.Any}, false, 0)}}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}
func openSame(op OperationSpec) OperationSpec {
	op.ValuesVars = 1
	op.Input = values(nil, true, 0)
	op.Outcomes = []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: values(nil, true, 0)}, {Kind: flowkind.OutcomeThrow, Values: values([]typ.Type{typ.Any}, false, 0)}}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}
func alternatives(op OperationSpec, in []typ.Type, open bool, outputs [][]typ.Type) OperationSpec {
	vars := uint32(0)
	if open {
		vars = 1
	}
	op.ValuesVars = vars
	op.Input = values(in, open, 0)
	for _, out := range outputs {
		op.Outcomes = append(op.Outcomes, OutcomeSpec{Kind: flowkind.OutcomeNormal, Values: values(out, false, 0)})
	}
	op.Outcomes = append(op.Outcomes, OutcomeSpec{Kind: flowkind.OutcomeThrow, Values: values([]typ.Type{typ.Any}, false, 0)})
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}
func alternativesTotal(op OperationSpec, in []typ.Type, open bool, outputs [][]typ.Type) OperationSpec {
	vars := uint32(0)
	if open {
		vars = 1
	}
	op.ValuesVars = vars
	op.Input = values(in, open, 0)
	for _, out := range outputs {
		op.Outcomes = append(op.Outcomes, OutcomeSpec{Kind: flowkind.OutcomeNormal, Values: values(out, false, 0)})
	}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}
func protected(op OperationSpec, fixed int) OperationSpec {
	if fixed == 1 {
		return pcallProfile(op)
	}
	return xpcallProfile(op)
}

func pcallProfile(op OperationSpec) OperationSpec {
	op.ValuesVars = 4
	op.Input = values([]typ.Type{typ.Any}, true, 0)
	op.Callbacks = []CallbackSpec{{
		Function: InputSource{Kind: InputSourceValueFormal}, Admission: schematype.CallableAdmissionOrdinary,
		Arguments: nativeCallbackTail(0), Outcomes: terminals(nativeCallbackTail(1), nativeCallbackTail(1), anyValue(), nativeCallbackTail(2), nativeCallbackTail(3)),
		Lifecycle: CallbackSyncRequiredOnce, Effects: RowSpec{Tail: RowClosed},
	}}
	op.Outcomes = []OutcomeSpec{
		{Kind: flowkind.OutcomeNormal, Values: values([]typ.Type{typ.LiteralBool(true)}, true, 1)},
		{Kind: flowkind.OutcomeNormal, Values: closed(typ.LiteralBool(false), typ.Any)},
		{Kind: flowkind.OutcomeYield, Values: nativeCallbackTail(2)},
		{Kind: flowkind.OutcomeCancel, Values: nativeCallbackTail(3)},
	}
	op.Subedges = []SubedgeSpec{{
		Role: 1, Family: SubedgeFamilyCall, Callee: SubedgeCalleeSpec{Kind: SubedgeCalleeCallback, Callback: 1},
		ArgumentOrigins:  tailInputOrigin(0),
		AdmissionFailure: admissionToOutcome(anyValue(), AdjustmentPreserve, PlacementFixed, 1),
		Routes: []SubedgeRouteSpec{
			outcomeRoute(flowkind.OutcomeNormal, nativeCallbackTail(1), AdjustmentPreserve, PlacementTail, 0),
			outcomeRoute(flowkind.OutcomeReturn, nativeCallbackTail(1), AdjustmentPreserve, PlacementTail, 0),
			outcomeRoute(flowkind.OutcomeThrow, anyValue(), AdjustmentPreserve, PlacementFixed, 1),
			propagateRoute(nativeCallbackTail(2)),
			outcomeRoute(flowkind.OutcomeCancel, nativeCallbackTail(3), AdjustmentPreserve, PlacementTail, 3),
		},
	}}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}

func xpcallProfile(op OperationSpec) OperationSpec {
	op.ValuesVars = 5
	op.Input = values([]typ.Type{typ.Any, typ.Any}, true, 0)
	op.Callbacks = []CallbackSpec{
		{Function: InputSource{Kind: InputSourceValueFormal, Ordinal: 0}, Admission: schematype.CallableAdmissionOrdinary, Arguments: nativeCallbackTail(0), Outcomes: terminals(nativeCallbackTail(1), nativeCallbackTail(1), anyValue(), nativeCallbackTail(2), nativeCallbackTail(3)), Lifecycle: CallbackSyncRequiredOnce, Effects: RowSpec{Tail: RowClosed}},
		{Function: InputSource{Kind: InputSourceValueFormal, Ordinal: 1}, Admission: schematype.CallableAdmissionDirectFunction, Arguments: anyValue(), Outcomes: terminals(nativeCallbackTail(4), nativeCallbackTail(4), anyValue(), anyValue(), nativeCallbackTail(3)), Lifecycle: CallbackSyncOptionalMany, Effects: RowSpec{Tail: RowClosed}},
	}
	op.Outcomes = []OutcomeSpec{
		{Kind: flowkind.OutcomeNormal, Values: values([]typ.Type{typ.LiteralBool(true)}, true, 1)},
		{Kind: flowkind.OutcomeNormal, Values: closed(typ.LiteralBool(false), typ.Any)},
		{Kind: flowkind.OutcomeYield, Values: nativeCallbackTail(2)},
		{Kind: flowkind.OutcomeCancel, Values: nativeCallbackTail(3)},
		{Kind: flowkind.OutcomeThrow, Values: anyValue()},
	}
	op.Subedges = []SubedgeSpec{
		{
			Role: 1, Family: SubedgeFamilyCall, Callee: SubedgeCalleeSpec{Kind: SubedgeCalleeCallback, Callback: 1}, ArgumentOrigins: tailInputOrigin(0),
			AdmissionFailure: admissionToSibling(anyValue(), 2),
			Routes: []SubedgeRouteSpec{
				outcomeRoute(flowkind.OutcomeNormal, nativeCallbackTail(1), AdjustmentPreserve, PlacementTail, 0),
				outcomeRoute(flowkind.OutcomeReturn, nativeCallbackTail(1), AdjustmentPreserve, PlacementTail, 0),
				siblingRoute(flowkind.OutcomeThrow, anyValue(), AdjustmentExact, 2),
				propagateRoute(nativeCallbackTail(2)),
				outcomeRoute(flowkind.OutcomeCancel, nativeCallbackTail(3), AdjustmentPreserve, PlacementTail, 3),
			},
		},
		{
			Role: 2, Family: SubedgeFamilyCall, Callee: SubedgeCalleeSpec{Kind: SubedgeCalleeCallback, Callback: 2},
			AdmissionFailure: admissionToOutcome(anyValue(), AdjustmentPreserve, PlacementFixed, 4),
			Routes: []SubedgeRouteSpec{
				outcomeRoute(flowkind.OutcomeNormal, anyValue(), AdjustmentExact, PlacementFixed, 1),
				outcomeRoute(flowkind.OutcomeReturn, anyValue(), AdjustmentExact, PlacementFixed, 1),
				siblingRoute(flowkind.OutcomeThrow, anyValue(), AdjustmentExact, 2),
				rejectSiblingRoute(2),
				outcomeRoute(flowkind.OutcomeCancel, nativeCallbackTail(3), AdjustmentPreserve, PlacementTail, 3),
			},
		},
	}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}

func printProfile() OperationSpec {
	op := nativeBuiltin("print")
	op.ValuesVars = 2
	op.Input = nativeCallbackTail(0)
	op.Outcomes = []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: emptyValues()}, {Kind: flowkind.OutcomeThrow, Values: anyValue()}, {Kind: flowkind.OutcomeCancel, Values: nativeCallbackTail(1)}}
	op.Subedges = []SubedgeSpec{{
		Role: 1, Family: SubedgeFamilyCall,
		Callee:    SubedgeCalleeSpec{Kind: SubedgeCalleeCapturedInitialRead, Read: CapturedInitialReadSpec{Root: globalEnvRoot, Key: literalKey("tostring")}},
		Admission: schematype.CallableAdmissionOrdinary, Arguments: anyValue(), ArgumentOrigins: ruleOrigins(1),
		Outcomes:         terminals(anyValue(), anyValue(), anyValue(), anyValue(), nativeCallbackTail(1)),
		AdmissionFailure: admissionToOutcome(anyValue(), AdjustmentPreserve, PlacementFixed, 1),
		Routes: []SubedgeRouteSpec{
			continueRoute(flowkind.OutcomeNormal, anyValue(), AdjustmentExact),
			continueRoute(flowkind.OutcomeReturn, anyValue(), AdjustmentExact),
			outcomeRoute(flowkind.OutcomeThrow, anyValue(), AdjustmentPreserve, PlacementFixed, 1),
			rejectRoute(1),
			outcomeRoute(flowkind.OutcomeCancel, nativeCallbackTail(1), AdjustmentPreserve, PlacementTail, 2),
		},
	}}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}

func tostringProfile() OperationSpec {
	op := nativeBuiltin("tostring")
	op.ValuesVars = 1
	op.Input = anyValue()
	op.Outcomes = []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: closed(typ.String)}, {Kind: flowkind.OutcomeThrow, Values: anyValue()}, {Kind: flowkind.OutcomeCancel, Values: nativeCallbackTail(0)}}
	edge := ruleMetaCallEdge(1, literalKey("__tostring"), anyValue(), 1, 2, nativeCallbackTail(0))
	edge.ArgumentOrigins = []ArgumentOrigin{fixedInputOrigin(0)}
	op.Subedges = []SubedgeSpec{edge}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}

func formatProfile() OperationSpec {
	op := module("string", "format")
	op.ValuesVars = 2
	op.Input = values([]typ.Type{typ.String}, true, 0)
	op.Outcomes = []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: closed(typ.String)}, {Kind: flowkind.OutcomeThrow, Values: anyValue()}, {Kind: flowkind.OutcomeCancel, Values: nativeCallbackTail(1)}}
	op.Subedges = []SubedgeSpec{ruleMetaCallEdge(1, literalKey("__tostring"), anyValue(), 1, 2, nativeCallbackTail(1))}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}

func pairsProfile() OperationSpec {
	op := nativeBuiltin("pairs")
	op.ValuesVars = 1
	op.Input = anyValue()
	// The meta-hook and raw fallback are separate ordinary outcomes.  A hook
	// controls every returned slot; only the fallback proves next/input/nil.
	metaThree := closed(typ.Any, typ.Any, typ.Any)
	fallbackThree := closed(typ.Any, typ.Any, typ.Nil)
	op.Outcomes = []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: metaThree}, {Kind: flowkind.OutcomeNormal, Values: fallbackThree}, {Kind: flowkind.OutcomeThrow, Values: anyValue()}, {Kind: flowkind.OutcomeCancel, Values: nativeCallbackTail(0)}}
	op.Subedges = []SubedgeSpec{{
		Role: 1, Family: SubedgeFamilyCall, Callee: SubedgeCalleeSpec{Kind: SubedgeCalleeMetaKey, MetaKey: literalKey("__pairs")},
		Admission: schematype.CallableAdmissionOrdinary, Arguments: anyValue(), ArgumentOrigins: []ArgumentOrigin{{Segment: ArgumentFixed, Kind: ArgumentSourceInput, Source: InputSource{Kind: InputSourceValueFormal}}},
		Outcomes:         terminals(anyValue(), anyValue(), anyValue(), anyValue(), nativeCallbackTail(0)),
		AdmissionFailure: admissionToOutcome(anyValue(), AdjustmentPreserve, PlacementFixed, 2),
		Routes: []SubedgeRouteSpec{
			outcomeRoute(flowkind.OutcomeNormal, metaThree, AdjustmentExact, PlacementFixed, 0),
			outcomeRoute(flowkind.OutcomeReturn, metaThree, AdjustmentExact, PlacementFixed, 0),
			outcomeRoute(flowkind.OutcomeThrow, anyValue(), AdjustmentPreserve, PlacementFixed, 2),
			rejectRoute(2),
			outcomeRoute(flowkind.OutcomeCancel, nativeCallbackTail(0), AdjustmentPreserve, PlacementTail, 3),
		},
	}}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}

func ipairsProfile() OperationSpec {
	op := fixed(nativeBuiltin("ipairs"), []typ.Type{typ.Any}, []typ.Type{typ.Any, typ.Any, typ.Integer})
	return op
}

func ipairsAuxProfile() OperationSpec {
	op := OperationSpec{ValuesVars: 1, Input: closed(typ.Any, typ.Integer), Outcomes: []OutcomeSpec{
		{Kind: flowkind.OutcomeNormal, Values: closed(typ.Nil)},
		{Kind: flowkind.OutcomeNormal, Values: closed(typ.Integer, typ.Any)},
		{Kind: flowkind.OutcomeThrow, Values: anyValue()},
		{Kind: flowkind.OutcomeCancel, Values: nativeCallbackTail(0)},
	}, Effects: RowSpec{Tail: RowClosed}}
	edge := ruleFamilyEdge(1, SubedgeFamilyIndexGet, closed(typ.Any, typ.Integer), 2, 3, nativeCallbackTail(0))
	edge.ArgumentOrigins = []ArgumentOrigin{fixedInputOrigin(0), {Segment: ArgumentFixed, Index: 1, Kind: ArgumentSourceRule}}
	op.Subedges = []SubedgeSpec{edge}
	return op
}

func tableOperation(name string, input []typ.Type, open bool, output ValuesSpec) OperationSpec {
	op := module("table", name)
	op.ValuesVars = 2
	op.Input = values(input, open, 0)
	op.Outcomes = []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: output}, {Kind: flowkind.OutcomeThrow, Values: anyValue()}, {Kind: flowkind.OutcomeCancel, Values: nativeCallbackTail(1)}}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}

func tableConcatProfile() OperationSpec {
	op := tableOperation("concat", []typ.Type{typ.Any}, true, closed(typ.String))
	op.Subedges = []SubedgeSpec{
		ruleFamilyEdge(1, SubedgeFamilyLength, anyValue(), 1, 2, nativeCallbackTail(1)),
		ruleFamilyEdge(2, SubedgeFamilyIndexGet, closed(typ.Any, typ.Integer), 1, 2, nativeCallbackTail(1)),
	}
	return op
}

func ruleDiscardFamilyEdge(role uint32, family SubedgeFamily, arguments ValuesSpec, throwOutcome, cancelOutcome uint32, cancel ValuesSpec) SubedgeSpec {
	edge := ruleFamilyEdge(role, family, arguments, throwOutcome, cancelOutcome, cancel)
	edge.Routes[0] = continueRoute(flowkind.OutcomeNormal, emptyValues(), AdjustmentExact)
	edge.Routes[1] = continueRoute(flowkind.OutcomeReturn, emptyValues(), AdjustmentExact)
	return edge
}

func tableInsertProfile() OperationSpec {
	op := tableOperation("insert", []typ.Type{typ.Any}, true, emptyValues())
	op.Subedges = []SubedgeSpec{
		ruleFamilyEdge(1, SubedgeFamilyLength, anyValue(), 1, 2, nativeCallbackTail(1)),
		ruleFamilyEdge(2, SubedgeFamilyIndexGet, closed(typ.Any, typ.Integer), 1, 2, nativeCallbackTail(1)),
		ruleDiscardFamilyEdge(3, SubedgeFamilyIndexSet, closed(typ.Any, typ.Integer, typ.Any), 1, 2, nativeCallbackTail(1)),
		ruleDiscardFamilyEdge(4, SubedgeFamilyIndexSet, closed(typ.Any, typ.Integer, typ.Any), 1, 2, nativeCallbackTail(1)),
	}
	return op
}

func tableRemoveProfile() OperationSpec {
	op := tableOperation("remove", []typ.Type{typ.Any}, true, anyValue())
	op.Subedges = []SubedgeSpec{
		ruleFamilyEdge(1, SubedgeFamilyLength, anyValue(), 1, 2, nativeCallbackTail(1)),
		ruleFamilyEdge(2, SubedgeFamilyIndexGet, closed(typ.Any, typ.Integer), 1, 2, nativeCallbackTail(1)),
		ruleFamilyEdge(3, SubedgeFamilyIndexGet, closed(typ.Any, typ.Integer), 1, 2, nativeCallbackTail(1)),
		ruleDiscardFamilyEdge(4, SubedgeFamilyIndexSet, closed(typ.Any, typ.Integer, typ.Any), 1, 2, nativeCallbackTail(1)),
		ruleDiscardFamilyEdge(5, SubedgeFamilyIndexSet, closed(typ.Any, typ.Integer, typ.Nil), 1, 2, nativeCallbackTail(1)),
	}
	return op
}

func tableMoveProfile() OperationSpec {
	op := tableOperation("move", []typ.Type{typ.Any, typ.Integer, typ.Integer, typ.Integer}, true, anyValue())
	op.Subedges = []SubedgeSpec{
		ruleFamilyEdge(1, SubedgeFamilyIndexGet, closed(typ.Any, typ.Integer), 1, 2, nativeCallbackTail(1)),
		ruleDiscardFamilyEdge(2, SubedgeFamilyIndexSet, closed(typ.Any, typ.Integer, typ.Any), 1, 2, nativeCallbackTail(1)),
		ruleFamilyEdge(3, SubedgeFamilyEqual, closed(typ.Any, typ.Any), 1, 2, nativeCallbackTail(1)),
	}
	return op
}

func tableUnpackProfile() OperationSpec {
	op := OperationSpec{Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"unpack"}}, {Namespace: BindingModule, Owner: []string{"table"}, Member: []string{"unpack"}}}, ValuesVars: 3,
		Input: values([]typ.Type{typ.Any}, true, 0), Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: nativeCallbackTail(1)}, {Kind: flowkind.OutcomeThrow, Values: anyValue()}, {Kind: flowkind.OutcomeCancel, Values: nativeCallbackTail(2)}}, Effects: RowSpec{Tail: RowClosed}}
	op.Subedges = []SubedgeSpec{
		ruleFamilyEdge(1, SubedgeFamilyLength, anyValue(), 1, 2, nativeCallbackTail(2)),
		ruleFamilyEdge(2, SubedgeFamilyIndexGet, closed(typ.Any, typ.Integer), 1, 2, nativeCallbackTail(2)),
	}
	return op
}

func tableSortProfile() OperationSpec {
	op := module("table", "sort")
	op.ValuesVars = 1
	op.Input = closed(typ.Any, typ.Any)
	op.Callbacks = []CallbackSpec{{Function: InputSource{Kind: InputSourceValueFormal, Ordinal: 1}, Admission: schematype.CallableAdmissionDirectFunction, Arguments: closed(typ.Any, typ.Any), Outcomes: terminals(nativeCallbackTail(0), nativeCallbackTail(0), anyValue(), anyValue(), nativeCallbackTail(0)), Lifecycle: CallbackSyncOptionalMany, Effects: RowSpec{Tail: RowClosed}}}
	op.Outcomes = []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: emptyValues()}, {Kind: flowkind.OutcomeThrow, Values: anyValue()}, {Kind: flowkind.OutcomeCancel, Values: nativeCallbackTail(0)}}
	comparator := SubedgeSpec{Role: 3, Family: SubedgeFamilyCall, Callee: SubedgeCalleeSpec{Kind: SubedgeCalleeCallback, Callback: 1}, ArgumentOrigins: ruleOrigins(2), AdmissionFailure: admissionToOutcome(anyValue(), AdjustmentPreserve, PlacementFixed, 1), Routes: []SubedgeRouteSpec{
		continueRoute(flowkind.OutcomeNormal, anyValue(), AdjustmentExact), continueRoute(flowkind.OutcomeReturn, anyValue(), AdjustmentExact), outcomeRoute(flowkind.OutcomeThrow, anyValue(), AdjustmentPreserve, PlacementFixed, 1), rejectRoute(1), outcomeRoute(flowkind.OutcomeCancel, nativeCallbackTail(0), AdjustmentPreserve, PlacementTail, 2),
	}}
	op.Subedges = []SubedgeSpec{
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

func callbackGsubProfile() OperationSpec {
	op := module("string", "gsub")
	op.ValuesVars = 4
	op.Input = values([]typ.Type{typ.String, typ.String, typ.Any}, true, 0)
	op.Callbacks = []CallbackSpec{{Function: InputSource{Kind: InputSourceValueFormal, Ordinal: 2}, Admission: schematype.CallableAdmissionDirectFunction, Arguments: nativeCallbackTail(1), Outcomes: terminals(nativeCallbackTail(2), nativeCallbackTail(2), anyValue(), anyValue(), nativeCallbackTail(3)), Lifecycle: CallbackSyncOptionalMany, Effects: RowSpec{Tail: RowClosed}}}
	op.Outcomes = []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: closed(typ.String, typ.Integer)}, {Kind: flowkind.OutcomeThrow, Values: anyValue()}, {Kind: flowkind.OutcomeCancel, Values: nativeCallbackTail(3)}}
	function := SubedgeSpec{Role: 1, Family: SubedgeFamilyCall, Callee: SubedgeCalleeSpec{Kind: SubedgeCalleeCallback, Callback: 1}, ArgumentOrigins: ruleTailOrigin(), AdmissionFailure: admissionToOutcome(anyValue(), AdjustmentPreserve, PlacementFixed, 1), Routes: []SubedgeRouteSpec{
		continueRoute(flowkind.OutcomeNormal, anyValue(), AdjustmentExact), continueRoute(flowkind.OutcomeReturn, anyValue(), AdjustmentExact), outcomeRoute(flowkind.OutcomeThrow, anyValue(), AdjustmentPreserve, PlacementFixed, 1), rejectRoute(1), outcomeRoute(flowkind.OutcomeCancel, nativeCallbackTail(3), AdjustmentPreserve, PlacementTail, 2),
	}}
	table := ruleFamilyEdge(2, SubedgeFamilyIndexGet, closed(typ.Any, typ.Any), 1, 2, nativeCallbackTail(3))
	// The table branch is distinct from the function callback: gsub indexes
	// the replacement table with the first capture or whole match, supplied by
	// its own closed Rule coordinate rather than an invented callback input.
	table.ArgumentOrigins = []ArgumentOrigin{{Segment: ArgumentFixed, Index: 0, Kind: ArgumentSourceInput, Source: InputSource{Kind: InputSourceValueFormal, Ordinal: 2}}, {Segment: ArgumentFixed, Index: 1, Kind: ArgumentSourceRule}}
	op.Subedges = []SubedgeSpec{function, table}
	op.GsubTableReplacement = &GsubTableReplacementSpec{Replacement: 2, Access: 2, ResultOutcome: 0, Result: 0, EffectAliases: []uint32{0}}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}

func minMaxProfile(op OperationSpec) OperationSpec {
	op.ValuesVars = 2
	op.Input = values([]typ.Type{typ.Any}, true, 0)
	op.Outcomes = []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: anyValue()}, {Kind: flowkind.OutcomeThrow, Values: anyValue()}, {Kind: flowkind.OutcomeCancel, Values: nativeCallbackTail(1)}}
	op.Subedges = []SubedgeSpec{ruleFamilyEdge(1, SubedgeFamilyLess, closed(typ.Any, typ.Any), 1, 2, nativeCallbackTail(1))}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}

func resumeEnvelope() OperationSpec {
	op := module("coroutine", "resume")
	op.ValuesVars = 3
	op.Input = values([]typ.Type{typ.Any}, true, 0)
	op.Outcomes = []OutcomeSpec{
		{Kind: flowkind.OutcomeNormal, Values: values([]typ.Type{typ.LiteralBool(true)}, true, 1)},
		{Kind: flowkind.OutcomeNormal, Values: values([]typ.Type{typ.LiteralBool(false)}, true, 2)},
	}
	op.Resumes = []ResumeSpec{resumeRelation(ResumeSourceValueFormal, 0, 0, 0, 1)}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}

// resumeRelation is the complete activation-boundary correspondence shared by
// coroutine.resume and a produced coroutine.wrap invocation. Successful
// return and yield are ordinary successful results; restored throw/cancel
// select the operation's failure outcome. The carrier's resumption arguments
// are exactly the operation's incoming open Values variable.
func resumeRelation(source ResumeSource, carrier ValueFormal, arguments ValuesVar, success, failure uint32) ResumeSpec {
	return ResumeSpec{
		Source: source, Carrier: carrier, Arguments: nativeCallbackTail(arguments),
		Outcomes: []ResumeOutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Outcome: success},
			{Kind: flowkind.OutcomeReturn, Outcome: success},
			{Kind: flowkind.OutcomeThrow, Outcome: failure},
			{Kind: flowkind.OutcomeYield, Outcome: success},
			{Kind: flowkind.OutcomeCancel, Outcome: failure},
		},
	}
}

func callbackCreate(op OperationSpec) OperationSpec {
	op.ValuesVars = 5
	op.Input = values([]typ.Type{typ.Any}, false, 0)
	op.Callbacks = []CallbackSpec{{Function: InputSource{Kind: InputSourceValueFormal, Ordinal: 0}, Admission: schematype.CallableAdmissionDirectFunction, Arguments: nativeCallbackTail(0), Outcomes: nativeCallbackOutcomes(1, 1, 2, 3, 4), Lifecycle: CallbackRetainedOptionalOnce, Effects: RowSpec{Tail: RowClosed}}}
	op.Outcomes = []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: values([]typ.Type{typ.Any}, false, 0), CallbackResults: []CallbackResultSpec{{Result: 0, Callback: 1}}}, {Kind: flowkind.OutcomeThrow, Values: values([]typ.Type{typ.Any}, false, 0)}}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}

func callbackWrap(op OperationSpec) OperationSpec {
	op.ValuesVars = 5
	op.Input = values([]typ.Type{typ.Any}, false, 0)
	op.Callbacks = []CallbackSpec{{Function: InputSource{Kind: InputSourceValueFormal, Ordinal: 0}, Admission: schematype.CallableAdmissionDirectFunction, Arguments: nativeCallbackTail(0), Outcomes: nativeCallbackOutcomes(1, 1, 2, 3, 4), Lifecycle: CallbackRetainedOptionalOnce, Effects: RowSpec{Tail: RowClosed}}}
	op.Outcomes = []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: values([]typ.Type{typ.Any}, false, 0)}, {Kind: flowkind.OutcomeThrow, Values: values([]typ.Type{typ.Any}, false, 0)}}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}

// callbackSpawn is a detached, one-shot callback activation.  Its parent
// system-yield and later empty resume share the typed SpawnSpec; the callback
// keeps the exact function/closure authority and its complete outcome rows.
func callbackSpawn() OperationSpec {
	op := module("coroutine", "spawn")
	op.ValuesVars = 7 // input tail, empty child entry coordinate, five child outcomes
	op.Input = values([]typ.Type{typ.Any}, true, 0)
	op.Callbacks = []CallbackSpec{{
		Function:  InputSource{Kind: InputSourceValueFormal, Ordinal: 0},
		Admission: schematype.CallableAdmissionDirectFunction, Arguments: nativeCallbackTail(1), Outcomes: nativeCallbackOutcomes(2, 3, 4, 5, 6),
		Lifecycle: CallbackRetainedRequiredOnce, Effects: RowSpec{Tail: RowClosed},
	}}
	op.Outcomes = []OutcomeSpec{
		{Kind: flowkind.OutcomeYield, Values: values(nil, false, 0)},
		{Kind: flowkind.OutcomeNormal, Values: values(nil, false, 0)},
		{Kind: flowkind.OutcomeThrow, Values: values([]typ.Type{typ.Any}, false, 0)},
	}
	op.Suspensions = []SuspensionSpec{{Yield: 0, Reentry: 1, Source: ReentryByProvider, Multiplicity: ReentryOnce}}
	op.Spawns = []SpawnSpec{{
		Function: InputSource{Kind: InputSourceValueFormal, Ordinal: 0}, Child: 1,
		Yield: 0, ParentResume: 1, ChildEntry: 1,
		Alternatives: []SpawnSiblingAlternative{SpawnChildEntryThenParentResume, SpawnParentResumeThenChildEntry},
	}}
	op.Effects = RowSpec{Tail: RowClosed}
	return op
}

func nativeCallbackTail(variable ValuesVar) ValuesSpec {
	return values(nil, true, variable)
}

func nativeCallbackOutcomes(normal, returned, thrown, yielded, canceled ValuesVar) []TerminalSpec {
	return []TerminalSpec{
		{Kind: flowkind.OutcomeNormal, Values: nativeCallbackTail(normal)},
		{Kind: flowkind.OutcomeReturn, Values: nativeCallbackTail(returned)},
		{Kind: flowkind.OutcomeThrow, Values: nativeCallbackTail(thrown)},
		{Kind: flowkind.OutcomeYield, Values: nativeCallbackTail(yielded)},
		{Kind: flowkind.OutcomeCancel, Values: nativeCallbackTail(canceled)},
	}
}

// selfEffects is the closed, self-labelled Koka inventory justified by the
// target surface.  The occurrence carries the complete existing input
// coordinate correspondence; it does not introduce an effect-kind vocabulary.
func (catalogue *authoredCatalogue) selfEffects(declarations *manifest.Catalogue) error {
	declaredEffects := make(map[string]bool)
	for _, function := range declarations.Functions() {
		for _, binding := range bindingsFromDeclaration(function) {
			declaredEffects[bindingKey(binding)] = !function.Signature().Effect.Pure()
		}
	}
	for index := range catalogue.operations {
		ref := operationRef(index + 1)
		op := catalogue.at(ref)
		declared := false
		for _, binding := range op.Bindings {
			if declaredEffects[bindingKey(binding)] {
				declared = true
				break
			}
		}
		if !declared {
			// Produced callables have no provider path. Their effect identity is
			// an operational consequence of the producing law.
			name := ""
			for candidate, candidateRef := range catalogue.names {
				if candidateRef == ref {
					name = candidate
					break
				}
			}
			if name != "string.gmatch.next" && name != "ipairs.aux" {
				continue
			}
		}
		values := make([]ValueFormal, len(op.Input.Fixed))
		for i := range values {
			values[i] = ValueFormal(i)
		}
		vars := make([]ValuesVar, op.ValuesVars)
		for i := range vars {
			vars[i] = ValuesVar(i)
		}
		op.Effects = RowSpec{Occurrences: []EffectSpec{{Target: SpecRef(ref), ValueArgs: values, ValuesArgs: vars}}, Tail: RowClosed}
	}
	return nil
}

func bindingKey(binding BindingSpec) string {
	return fmt.Sprintf("%d/%q/%q", binding.Namespace, binding.Owner, binding.Member)
}
