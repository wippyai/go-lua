package operation

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

type queryState struct {
	operations    []queryOperationRow
	types         []queryTypeRow
	values        []queryValuesRow
	effects       []queryEffectRow
	transfers     []queryTransferRow
	transferEnds  []vocabulary.TransferPossibility
	outcomeRows   []queryOutcomeRow
	behaviorRows  []queryBehaviorResultRow
	predicateRows []queryBehaviorPredicateRow
}

type queryOperationRow struct {
	input              vocabulary.Values
	outcomes           queryRange
	behavior           queryRange
	behaviorPredicates queryRange
	valuesTypes        []vocabulary.Type
	transfers          queryRange
	effects            []int
	typeFormals        []vocabulary.Type
	rowFormals         uint32
	effectTail         vocabulary.RowTail
	effectVar          vocabulary.RowVar
}

type queryRange struct{ start, end int }

func (r queryRange) len() int {
	if r.end < r.start {
		return 0
	}
	return r.end - r.start
}

type queryTypeRow struct {
	declaration schematype.Type
}

type queryValuesRow struct {
	owner  vocabulary.Operation
	types  []vocabulary.Type
	tail   vocabulary.ValuesTail
	varID  vocabulary.ValuesVar
	suffix []vocabulary.Type
}

type queryOutcomeRow struct {
	kind   flowkind.OutcomeKind
	values vocabulary.Values
}

type queryBehaviorResultRow struct {
	outcome  uint32
	result   uint32
	source   vocabulary.InputSource
	relation schema.EntryID
}

type queryBehaviorPredicateRow struct {
	outcome  uint32
	result   uint32
	subject  vocabulary.InputSource
	relation schema.EntryID
}

type queryTransferRow struct {
	owner        vocabulary.Operation
	endpoint     vocabulary.TransferEndpoint
	payload      vocabulary.InputSource
	alias        vocabulary.InputSource
	identity     vocabulary.TransferIdentity
	capabilities vocabulary.TransferCapabilities
	outcomes     queryRange
}

type queryEffectRow struct {
	target         vocabulary.Operation
	values         []vocabulary.ValueFormal
	types          []vocabulary.TypeFormal
	valuesVar      []vocabulary.ValuesVar
	rows           []vocabulary.RowVar
	publication    vocabulary.PublicationEffectSpec
	hasPublication bool
}

// CompileQuery seals the operation query plane into the owner-issued Core.
// The returned Core owns private copies of every supplied row and pool. The
// input may be dropped immediately; no callback or mutable Target builder is
// retained by the published operation value.
func CompileQuery(core Core, input QueryInput) (Core, error) {
	if len(input.Operations) != core.OperationCount() {
		return Core{}, invalidQuery("operation table does not match operation core")
	}
	state := queryState{
		operations: make([]queryOperationRow, len(input.Operations)),
		types:      make([]queryTypeRow, len(input.Types)),
		values:     make([]queryValuesRow, len(input.Values)),
		effects:    make([]queryEffectRow, len(input.Effects)),
	}
	for index, item := range input.Types {
		if item.Handle == 0 || int(item.Handle) != index+1 || !item.Declaration.Available() {
			return Core{}, invalidQuery("type handles are not dense")
		}
		state.types[index] = queryTypeRow{declaration: item.Declaration}
	}
	for index, item := range input.Values {
		if item.Handle == 0 || int(item.Handle) != index+1 {
			return Core{}, invalidQuery("Values handles are not dense")
		}
		for _, typ := range item.Types {
			if !validQueryType(typ, len(state.types)) {
				return Core{}, invalidQuery("Values type is outside type table")
			}
		}
		for _, typ := range item.Suffix {
			if !validQueryType(typ, len(state.types)) {
				return Core{}, invalidQuery("Values suffix type is outside type table")
			}
		}
		state.values[index] = queryValuesRow{
			owner: item.Owner,
			types: append([]vocabulary.Type(nil), item.Types...),
			tail:  item.Tail, varID: item.VarID,
			suffix: append([]vocabulary.Type(nil), item.Suffix...),
		}
	}
	for index, item := range input.Effects {
		if item.Target == 0 || int(item.Target) > core.OperationCount() {
			return Core{}, invalidQuery("effect target is outside operation table")
		}
		state.effects[index] = queryEffectRow{
			target:         item.Target,
			values:         append([]vocabulary.ValueFormal(nil), item.Values...),
			types:          append([]vocabulary.TypeFormal(nil), item.Types...),
			valuesVar:      append([]vocabulary.ValuesVar(nil), item.ValuesVar...),
			rows:           append([]vocabulary.RowVar(nil), item.Rows...),
			publication:    item.Publication,
			hasPublication: item.HasPublication,
		}
	}
	var outcomes []queryOutcomeRow
	var behavior []queryBehaviorResultRow
	var predicates []queryBehaviorPredicateRow
	for index, item := range input.Operations {
		op := vocabulary.Operation(index + 1)
		if len(item.Outcomes) != core.OutcomeCount(op) {
			return Core{}, invalidQuery("operation outcome table does not match operation geometry")
		}
		if item.Input != 0 && !state.validValues(item.Input) {
			return Core{}, invalidQuery("operation input is outside Values table")
		}
		outcomeStart := len(outcomes)
		for _, outcome := range item.Outcomes {
			if outcome.Values != 0 && !state.validValues(outcome.Values) {
				return Core{}, invalidQuery("outcome Values is outside Values table")
			}
			outcomes = append(outcomes, queryOutcomeRow{kind: outcome.Kind, values: outcome.Values})
		}
		if len(item.ValuesTypes) != core.ValuesVarCount(op) {
			return Core{}, invalidQuery("operation Values variable table does not match operation geometry")
		}
		for _, typ := range item.ValuesTypes {
			if !validQueryType(typ, len(state.types)) {
				return Core{}, invalidQuery("operation Values variable type is outside type table")
			}
		}
		behaviorStart := len(behavior)
		for _, row := range item.Behavior {
			behavior = append(behavior, queryBehaviorResultRow{
				outcome: row.Outcome, result: row.Result, source: row.Source, relation: row.Relation,
			})
		}
		predicateStart := len(predicates)
		for _, row := range item.BehaviorPredicates {
			predicates = append(predicates, queryBehaviorPredicateRow{
				outcome: row.Outcome, result: row.Result, subject: row.Subject, relation: row.Relation,
			})
		}
		transferStart := len(state.transfers)
		for _, transfer := range item.Transfers {
			endStart := len(state.transferEnds)
			state.transferEnds = append(state.transferEnds, transfer.Outcomes...)
			state.transfers = append(state.transfers, queryTransferRow{
				owner: op, endpoint: transfer.Endpoint, payload: transfer.Payload,
				alias: transfer.Alias, identity: transfer.Identity, capabilities: transfer.Capabilities,
				outcomes: queryRange{start: endStart, end: len(state.transferEnds)},
			})
		}
		if !validQueryRange(item.EffectIndices, len(state.effects)) {
			return Core{}, invalidQuery("operation effect index is outside effect table")
		}
		state.operations[index] = queryOperationRow{
			input:              item.Input,
			outcomes:           queryRange{start: outcomeStart, end: len(outcomes)},
			behavior:           queryRange{start: behaviorStart, end: len(behavior)},
			behaviorPredicates: queryRange{start: predicateStart, end: len(predicates)},
			valuesTypes:        append([]vocabulary.Type(nil), item.ValuesTypes...),
			transfers:          queryRange{start: transferStart, end: len(state.transfers)},
			effects:            append([]int(nil), item.EffectIndices...),
			typeFormals:        append([]vocabulary.Type(nil), item.TypeFormals...),
			rowFormals:         item.RowFormals, effectTail: item.EffectTail, effectVar: item.EffectVar,
		}
	}
	state.outcomeRows = outcomes
	state.behaviorRows = behavior
	state.predicateRows = predicates
	core.query = state
	return core, nil
}

// These fields are kept out of queryState's public handoff. They are filled
// once, during CompileQuery, and are immutable thereafter.
func invalidQuery(message string) error { return queryError(message) }

type queryError string

func (err queryError) Error() string { return "target/operation: " + string(err) }

func validQueryRange(values []int, bound int) bool {
	for _, value := range values {
		if value < 0 || value >= bound {
			return false
		}
	}
	return true
}

func (state queryState) validValues(handle vocabulary.Values) bool {
	return handle != 0 && int(handle) <= len(state.values)
}

func validQueryType(handle vocabulary.Type, count int) bool {
	return handle != 0 && int(handle) <= count
}
