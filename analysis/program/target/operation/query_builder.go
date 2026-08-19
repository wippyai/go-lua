package operation

import (
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

// QueryValuesDeclaration is the frozen Values declaration consumed by the
// operation owner during Target seal. Type names are the same canonical keys
// issued by the operation's type declaration table; no Target row is retained.
type QueryValuesDeclaration struct {
	Owner  vocabulary.Operation
	Types  []string
	Tail   vocabulary.ValuesTail
	VarID  vocabulary.ValuesVar
	Suffix []string
}

// QueryBuilder is the one-shot construction capability for an operation query
// plane. Core itself has no mutator; FinishQuery consumes this builder and
// returns the immutable Core value.
type QueryBuilder struct{ core Core }

// BeginQuery opens the one mutable construction phase of the operation query
// plane. The builder owns all query rows until FinishQuery consumes it.
func BeginQuery(core Core) (*QueryBuilder, error) {
	builder := &QueryBuilder{core: core}
	if err := builder.core.beginQuery(); err != nil {
		return nil, err
	}
	return builder, nil
}

func (core *Core) beginQuery() error {
	if core == nil || core.OperationCount() == 0 {
		return invalidQuery("missing operation geometry")
	}
	if len(core.query.operations) != 0 {
		return invalidQuery("query already started")
	}
	core.query = queryState{operations: make([]queryOperationRow, core.OperationCount())}
	return nil
}

// AppendQueryTypes issues the dense type handles and stores their immutable
// declarations directly in Core. The byte map is accepted only as the
// canonical key inventory; declarations are the published semantic values.
func (core *Core) appendQueryTypes(input map[string][]byte, declarations map[string]schematype.Type) (map[string]vocabulary.Type, error) {
	if core == nil || core.query.operations == nil {
		return nil, invalidQuery("query has not started")
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	handles := make(map[string]vocabulary.Type, len(keys))
	for _, key := range keys {
		declaration, ok := declarations[key]
		if !ok || !declaration.Available() {
			return nil, invalidQuery("missing neutral type declaration")
		}
		if handle, err := checkedQueryHandle("type table", len(core.query.types)); err != nil {
			return nil, err
		} else {
			core.query.types = append(core.query.types, queryTypeRow{declaration: declaration})
			handles[key] = handle
		}
	}
	return handles, nil
}

// AppendQueryValues issues one operation-owned Values row and its type pools.
// The caller may discard the declaration immediately after this method
// returns; all published data is held by Core.
func (core *Core) appendQueryValues(input QueryValuesDeclaration, handles map[string]vocabulary.Type) (vocabulary.Values, error) {
	if core == nil || core.query.operations == nil {
		return 0, invalidQuery("query has not started")
	}
	if input.Owner == 0 || int(input.Owner) > core.OperationCount() {
		return 0, invalidQuery("Values owner is outside operation table")
	}
	row := queryValuesRow{owner: input.Owner, tail: input.Tail, varID: input.VarID}
	for _, key := range input.Types {
		typ, ok := handles[key]
		if !ok || !validQueryType(typ, len(core.query.types)) {
			return 0, invalidQuery("Values type is outside type table")
		}
		row.types = append(row.types, typ)
	}
	for _, key := range input.Suffix {
		typ, ok := handles[key]
		if !ok || !validQueryType(typ, len(core.query.types)) {
			return 0, invalidQuery("Values suffix type is outside type table")
		}
		row.suffix = append(row.suffix, typ)
	}
	core.query.values = append(core.query.values, row)
	return vocabulary.Values(len(core.query.values)), nil
}

// AppendQueryEffect records the effect query row at the same dense position
// as Target's retained effect relation. Effect identity remains Target-owned;
// this row is only the operation query view.
func (core *Core) appendQueryEffect(input EffectInput) error {
	if core == nil || core.query.operations == nil {
		return invalidQuery("query has not started")
	}
	if input.Target == 0 || int(input.Target) > core.OperationCount() {
		return invalidQuery("effect target is outside operation table")
	}
	core.query.effects = append(core.query.effects, queryEffectRow{
		target:         input.Target,
		values:         append([]vocabulary.ValueFormal(nil), input.Values...),
		types:          append([]vocabulary.TypeFormal(nil), input.Types...),
		valuesVar:      append([]vocabulary.ValuesVar(nil), input.ValuesVar...),
		rows:           append([]vocabulary.RowVar(nil), input.Rows...),
		publication:    input.Publication,
		hasPublication: input.HasPublication,
	})
	return nil
}

// AppendQueryOperation consumes one complete frozen query declaration and
// publishes its rows directly into Core. Effects refer to the dense effect
// query positions issued by AppendQueryEffect.
func (core *Core) appendQueryOperation(op vocabulary.Operation, input QueryOperationInput) error {
	if core == nil || core.query.operations == nil {
		return invalidQuery("query has not started")
	}
	if op == 0 || int(op) > len(core.query.operations) || core.query.operations[int(op)-1].outcomes.end != 0 {
		return invalidQuery("operation query row is not dense")
	}
	if len(input.Outcomes) != core.OutcomeCount(op) {
		return invalidQuery("operation outcome table does not match operation geometry")
	}
	if input.Input != 0 && !core.query.validValues(input.Input) {
		return invalidQuery("operation input is outside Values table")
	}
	for _, typ := range input.ValuesTypes {
		if !validQueryType(typ, len(core.query.types)) {
			return invalidQuery("operation Values variable type is outside type table")
		}
	}
	if len(input.ValuesTypes) != core.ValuesVarCount(op) {
		return invalidQuery("operation Values variable table does not match operation geometry")
	}
	for _, typ := range input.TypeFormals {
		if typ != 0 && !validQueryType(typ, len(core.query.types)) {
			return invalidQuery("operation type formal is outside type table")
		}
	}
	if !validQueryRange(input.EffectIndices, len(core.query.effects)) {
		return invalidQuery("operation effect index is outside effect table")
	}

	row := queryOperationRow{
		input:       input.Input,
		valuesTypes: append([]vocabulary.Type(nil), input.ValuesTypes...),
		effects:     append([]int(nil), input.EffectIndices...),
		typeFormals: append([]vocabulary.Type(nil), input.TypeFormals...),
		rowFormals:  input.RowFormals,
		effectTail:  input.EffectTail,
		effectVar:   input.EffectVar,
	}
	row.outcomes = queryRange{start: len(core.query.outcomeRows)}
	for _, outcome := range input.Outcomes {
		if outcome.Values != 0 && !core.query.validValues(outcome.Values) {
			return invalidQuery("outcome Values is outside Values table")
		}
		core.query.outcomeRows = append(core.query.outcomeRows, queryOutcomeRow{kind: outcome.Kind, values: outcome.Values})
	}
	row.outcomes.end = len(core.query.outcomeRows)
	row.behavior = queryRange{start: len(core.query.behaviorRows)}
	for _, item := range input.Behavior {
		core.query.behaviorRows = append(core.query.behaviorRows, queryBehaviorResultRow{outcome: item.Outcome, result: item.Result, source: item.Source, relation: item.Relation})
	}
	row.behavior.end = len(core.query.behaviorRows)
	row.behaviorPredicates = queryRange{start: len(core.query.predicateRows)}
	for _, item := range input.BehaviorPredicates {
		core.query.predicateRows = append(core.query.predicateRows, queryBehaviorPredicateRow{outcome: item.Outcome, result: item.Result, subject: item.Subject, relation: item.Relation})
	}
	row.behaviorPredicates.end = len(core.query.predicateRows)
	row.transfers = queryRange{start: len(core.query.transfers)}
	for _, item := range input.Transfers {
		end := len(core.query.transferEnds)
		core.query.transferEnds = append(core.query.transferEnds, item.Outcomes...)
		core.query.transfers = append(core.query.transfers, queryTransferRow{
			owner: op, endpoint: item.Endpoint, payload: item.Payload, alias: item.Alias,
			identity: item.Identity, capabilities: item.Capabilities,
			outcomes: queryRange{start: end, end: len(core.query.transferEnds)},
		})
	}
	row.transfers.end = len(core.query.transfers)
	core.query.operations[int(op)-1] = row
	return nil
}

// FinishQuery verifies that every geometry operation received exactly one
// query row. It does not create another representation.
func (core *Core) finishQuery() error {
	if core == nil || core.query.operations == nil {
		return invalidQuery("query has not started")
	}
	for index, row := range core.query.operations {
		if row.outcomes.end < row.outcomes.start || row.outcomes.end-row.outcomes.start != core.OutcomeCount(vocabulary.Operation(index+1)) {
			return invalidQuery("operation query table is incomplete")
		}
	}
	return nil
}

func (builder *QueryBuilder) AppendQueryTypes(input map[string][]byte, declarations map[string]schematype.Type) (map[string]vocabulary.Type, error) {
	return builder.core.appendQueryTypes(input, declarations)
}

func (builder *QueryBuilder) AppendQueryValues(input QueryValuesDeclaration, handles map[string]vocabulary.Type) (vocabulary.Values, error) {
	return builder.core.appendQueryValues(input, handles)
}

func (builder *QueryBuilder) AppendQueryEffect(input EffectInput) error {
	return builder.core.appendQueryEffect(input)
}

func (builder *QueryBuilder) AppendQueryOperation(op vocabulary.Operation, input QueryOperationInput) error {
	return builder.core.appendQueryOperation(op, input)
}

// FinishQuery consumes the construction capability. A failed validation also
// consumes it; callers must restart from the original immutable geometry.
func (builder *QueryBuilder) FinishQuery() (Core, error) {
	if builder == nil {
		return Core{}, invalidQuery("nil query builder")
	}
	core := builder.core
	builder.core = Core{}
	if err := core.finishQuery(); err != nil {
		return Core{}, err
	}
	return core, nil
}

func checkedQueryHandle(label string, length int) (vocabulary.Type, error) {
	if length < 0 || length >= int(^uint32(0)) {
		return 0, errors.New("target/operation: " + label + " overflow")
	}
	return vocabulary.Type(length + 1), nil
}
