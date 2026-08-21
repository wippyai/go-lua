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
	core.query = queryState{
		operations: make([]queryOperationRow, core.OperationCount()),
		callbacks:  make([]queryCallbackRow, core.geometry.callbacks.Count()),
	}
	for index := 0; index < core.geometry.callbacks.Count(); index++ {
		callback, ok := core.geometry.callbacks.At(index)
		if !ok || callback.id == 0 {
			return invalidQuery("callback geometry is not dense")
		}
		if _, ownerOK := core.operation(callback.owner); !ownerOK {
			return invalidQuery("callback geometry has no owner")
		}
		core.query.callbacks[index].owner = callback.owner
		core.query.callbacks[index].source = uint32(callback.source)
	}
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

// appendEffect freezes one effect occurrence in Core and returns its dense
// owner-issued position. The target operation is already canonical here;
// every ABI and publication fence is checked before the row is published.
func (core *Core) appendEffect(input EffectInput) (int, error) {
	if core == nil || core.query.operations == nil {
		return 0, invalidQuery("query has not started")
	}
	if input.Target == 0 || int(input.Target) > core.OperationCount() {
		return 0, invalidQuery("effect target is outside operation table")
	}
	if len(input.Values) != core.ValueFormalCount(input.Target) ||
		len(input.Types) != core.TypeFormalCount(input.Target) ||
		len(input.ValuesVar) != core.ValuesVarCount(input.Target) ||
		len(input.Rows) != core.RowFormalCount(input.Target) {
		return 0, invalidQuery("effect arguments do not match target ABI")
	}
	for _, value := range input.Values {
		if value < 0 || int(value) >= core.ValueFormalCount(input.Target) {
			return 0, invalidQuery("effect value argument is outside target ABI")
		}
	}
	for _, value := range input.Types {
		if value < 0 || int(value) >= core.TypeFormalCount(input.Target) {
			return 0, invalidQuery("effect type argument is outside target ABI")
		}
	}
	for _, value := range input.ValuesVar {
		if value < 0 || int(value) >= core.ValuesVarCount(input.Target) {
			return 0, invalidQuery("effect Values argument is outside target ABI")
		}
	}
	for _, value := range input.Rows {
		if value < 0 || int(value) >= core.RowFormalCount(input.Target) {
			return 0, invalidQuery("effect row argument is outside target ABI")
		}
	}
	publication, publicationOK, publicationErr := freezePublicationEffect(input.Publication, input.HasPublication, core.ValueFormalCount(input.Target), uint32(core.ValuesVarCount(input.Target)))
	if publicationErr != nil {
		return 0, publicationErr
	}
	row := queryEffectRow{
		target:         input.Target,
		values:         append([]vocabulary.ValueFormal(nil), input.Values...),
		types:          append([]vocabulary.TypeFormal(nil), input.Types...),
		valuesVar:      append([]vocabulary.ValuesVar(nil), input.ValuesVar...),
		rows:           append([]vocabulary.RowVar(nil), input.Rows...),
		publication:    publication,
		hasPublication: publicationOK,
	}
	position := len(core.query.effects)
	core.query.effects = append(core.query.effects, row)
	return position, nil
}

// AppendEffect publishes one operation-owned effect occurrence and returns
// its dense Core position. Callback-owned rows use AppendCallbackEffects so
// their ownership range is sealed alongside the callback row.
func (builder *QueryBuilder) AppendEffect(input EffectInput) (int, error) {
	return builder.core.appendEffect(input)
}

// AppendCallbackEffects publishes the complete expected row for one callback.
// The callback range and row schema are owned by Core; no Target row or
// forwarding index is retained.
func (builder *QueryBuilder) AppendCallbackEffects(callback vocabulary.CallbackID, tail vocabulary.RowTail, variable vocabulary.RowVar, input []EffectInput) error {
	core := &builder.core
	if core == nil || core.query.operations == nil {
		return invalidQuery("query has not started")
	}
	if callback == 0 || int(callback) > len(core.query.callbacks) {
		return invalidQuery("callback is outside callback table")
	}
	row := &core.query.callbacks[int(callback)-1]
	if row.published {
		return invalidQuery("callback effect row already published")
	}
	if tail != vocabulary.RowClosed && tail != vocabulary.RowVariable && tail != vocabulary.RowUnknownOpen {
		return invalidQuery("callback effect row has invalid tail")
	}
	if tail != vocabulary.RowVariable && variable != 0 {
		return invalidQuery("closed callback effect row carries variable")
	}
	if tail == vocabulary.RowVariable && uint64(variable) >= uint64(core.RowFormalCount(row.owner)) {
		return invalidQuery("callback effect row variable is outside owner ABI")
	}
	start := len(core.query.effects)
	for _, effect := range input {
		if _, err := core.appendEffect(effect); err != nil {
			return err
		}
	}
	row.effects = queryRange{start: start, end: len(core.query.effects)}
	row.effectTail, row.effectVar, row.published = tail, variable, true
	return nil
}

// AppendCallbackReleases consumes the complete release relation after all
// authored operation query rows are present. Core canonicalizes the reverse
// operation ranges and stores the callback-forward handle in the callback row;
// no Target-side release table or backpatch is needed.
func (builder *QueryBuilder) AppendCallbackReleases(input []CallbackReleaseInput) error {
	if builder == nil || builder.core.query.operations == nil {
		return invalidQuery("query has not started")
	}
	if _, err := vocabulary.CheckedStoredTotal("callback release table", len(builder.core.query.callbackReleases), len(input)); err != nil {
		return err
	}
	ordered := append([]CallbackReleaseInput(nil), input...)
	sort.Slice(ordered, func(left, right int) bool {
		if ordered[left].Operation != ordered[right].Operation {
			return ordered[left].Operation < ordered[right].Operation
		}
		return compareCallbackReleaseInput(ordered[left], ordered[right]) < 0
	})

	core := &builder.core
	for index, item := range ordered {
		if item.Callback == 0 || int(item.Callback) > len(core.query.callbacks) {
			return invalidQuery("callback release callback is outside callback table")
		}
		if item.Operation == 0 || int(item.Operation) > len(core.query.operations) {
			return invalidQuery("callback release operation is outside operation table")
		}
		callback := &core.query.callbacks[int(item.Callback)-1]
		owner, ownerOK := core.CallbackOwner(item.Callback)
		if !ownerOK || !callback.valuesSet || callback.release != 0 {
			return invalidQuery("callback release callback is malformed or duplicated")
		}
		if item.Input >= vocabulary.ValueFormal(core.ValueFormalCount(item.Operation)) {
			return invalidQuery("callback release input is outside operation ABI")
		}
		if item.Outcome >= uint32(core.OutcomeCount(item.Operation)) {
			return invalidQuery("callback release outcome is outside operation")
		}
		if item.Mode != vocabulary.CallbackReleaseOne && item.Mode != vocabulary.CallbackReleaseAll {
			return invalidQuery("callback release has invalid mode")
		}
		if !vocabulary.ValidCallbackReleaseZeroBehavior(item.ZeroBehavior) {
			return invalidQuery("callback release has invalid zero-holder behavior")
		}
		switch item.ZeroBehavior {
		case vocabulary.CallbackReleaseZeroSuppress:
			if item.ZeroOutcome != 0 {
				return invalidQuery("suppressed callback release carries an outcome")
			}
		case vocabulary.CallbackReleaseZeroThrow, vocabulary.CallbackReleaseZeroIdempotent:
			if item.ZeroOutcome >= uint32(core.OutcomeCount(item.Operation)) {
				return invalidQuery("callback release zero-holder outcome is outside operation")
			}
		}
		if index > 0 && ordered[index-1].Operation == item.Operation && ordered[index-1].Callback == item.Callback {
			return invalidQuery("callback has duplicate release")
		}
		if owner == 0 {
			return invalidQuery("callback release callback has no owner")
		}
		core.query.callbackReleases = append(core.query.callbackReleases, queryCallbackReleaseRow{
			callback: item.Callback, operation: item.Operation, input: item.Input, outcome: item.Outcome,
			mode: item.Mode, zeroBehavior: item.ZeroBehavior, zeroOutcome: item.ZeroOutcome,
		})
		callback.release = uint32(len(core.query.callbackReleases))
		operation := &core.query.operations[int(item.Operation)-1]
		if operation.callbackReleases.end == 0 {
			operation.callbackReleases.start = len(core.query.callbackReleases) - 1
		}
		operation.callbackReleases.end = len(core.query.callbackReleases)
	}
	return nil
}

func compareCallbackReleaseInput(left, right CallbackReleaseInput) int {
	if left.Callback < right.Callback {
		return -1
	}
	if left.Callback > right.Callback {
		return 1
	}
	if left.Input < right.Input {
		return -1
	}
	if left.Input > right.Input {
		return 1
	}
	if left.Outcome < right.Outcome {
		return -1
	}
	if left.Outcome > right.Outcome {
		return 1
	}
	if left.Mode < right.Mode {
		return -1
	}
	if left.Mode > right.Mode {
		return 1
	}
	if left.ZeroBehavior < right.ZeroBehavior {
		return -1
	}
	if left.ZeroBehavior > right.ZeroBehavior {
		return 1
	}
	if left.ZeroOutcome < right.ZeroOutcome {
		return -1
	}
	if left.ZeroOutcome > right.ZeroOutcome {
		return 1
	}
	return 0
}

// AppendQueryOperation consumes one complete frozen query declaration and
// publishes its rows directly into Core. Effects refer to the dense effect
// query positions issued by AppendEffect.
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
	if len(input.TypeFormals) != core.TypeFormalCount(op) {
		return invalidQuery("operation type formal table does not match operation geometry")
	}
	if !validQueryRange(input.EffectIndices, len(core.query.effects)) {
		return invalidQuery("operation effect index is outside effect table")
	}
	formalEffects, formalEffectTail, formalErr := canonicalFormalEffects(input.FormalEffects, input.FormalEffectTail)
	if formalErr != nil {
		return invalidQuery(formalErr.Error())
	}

	row := queryOperationRow{
		input:            input.Input,
		valuesTypes:      append([]vocabulary.Type(nil), input.ValuesTypes...),
		effects:          append([]int(nil), input.EffectIndices...),
		typeFormals:      append([]vocabulary.Type(nil), input.TypeFormals...),
		rowFormals:       input.RowFormals,
		effectTail:       input.EffectTail,
		effectVar:        input.EffectVar,
		formalEffects:    formalEffects,
		formalEffectTail: formalEffectTail,
	}
	row.outcomes = queryRange{start: len(core.query.outcomeRows)}
	row.outcomeSources = make([]uint32, len(input.Outcomes))
	sourceSeen := make([]bool, len(input.Outcomes))
	hasSources := len(input.Outcomes) != 0 && input.Outcomes[0].HasSource
	for index, outcome := range input.Outcomes {
		if outcome.HasSource != hasSources {
			return invalidQuery("operation outcome source coordinates are mixed")
		}
		source := uint32(index)
		if hasSources {
			source = outcome.Source
			if source >= uint32(len(input.Outcomes)) || sourceSeen[source] {
				return invalidQuery("operation outcome source coordinate is malformed")
			}
			sourceSeen[source] = true
		}
		row.outcomeSources[source] = uint32(index)
		if outcome.Values != 0 && !core.query.validValues(outcome.Values) {
			return invalidQuery("outcome Values is outside Values table")
		}
		core.query.outcomeRows = append(core.query.outcomeRows, queryOutcomeRow{source: source, kind: outcome.Kind, values: outcome.Values})
	}
	row.outcomes.end = len(core.query.outcomeRows)
	// Install the incomplete row only inside the one-shot builder so the
	// operation-owned subedge validator can resolve this operation's input and
	// type formals. FinishQuery remains the sole publication boundary, and the
	// completed row overwrites this construction value below.
	core.query.operations[int(op)-1] = row
	if err := core.appendQueryCallbackValues(op, input.CallbackValues); err != nil {
		return err
	}
	if err := core.appendQuerySubedges(op, &row, input); err != nil {
		return err
	}
	if err := core.appendQueryContinuation(op, &row, input); err != nil {
		return err
	}
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

const noQueryTypeValueCapture = ^uint32(0)

func (core *Core) appendQueryContinuation(op vocabulary.Operation, row *queryOperationRow, input QueryOperationInput) error {
	if row == nil {
		return invalidQuery("nil operation query row")
	}
	if op == 0 || int(op) > core.OperationCount() {
		return invalidQuery("continuation owner is outside operation table")
	}
	if core.isOpaque(op) && (len(input.Suspensions) != 0 || len(input.Spawns) != 0 || len(input.Resumes) != 0 || len(input.Produced) != 0 || len(input.FreshResults) != 0 || len(input.CallbackResults) != 0 || len(input.ResultAliases) != 0) {
		return invalidQuery("opaque operation cannot carry authored continuation rows")
	}
	if err := core.appendQueryOutputRows(op, row, input); err != nil {
		return err
	}

	row.suspensions = queryRange{start: len(core.query.suspensions)}
	if core.isOpaque(op) {
		// Opaque continuation behavior is sealed into the same owner row plane
		// as authored suspensions. Readers and denominator publication therefore
		// consume retained rows rather than manufacturing a count or relation.
		for _, reentry := range [...]uint32{0, 1, 3} {
			core.query.suspensions = append(core.query.suspensions, querySuspensionRow{
				yield: 2, reentry: reentry, source: vocabulary.ReentryByProvider, multiplicity: vocabulary.ReentryMany,
			})
		}
	}
	for _, item := range input.Suspensions {
		if !core.validOutcome(op, item.Yield) || !core.validOutcome(op, item.Reentry) {
			return invalidQuery("suspension outcome is outside operation")
		}
		if item.Source != vocabulary.ReentryByCall && item.Source != vocabulary.ReentryByProvider {
			return invalidQuery("suspension has invalid reentry source")
		}
		if item.Multiplicity != vocabulary.ReentryOnce && item.Multiplicity != vocabulary.ReentryMany {
			return invalidQuery("suspension has invalid multiplicity")
		}
		for prior := row.suspensions.start; prior < len(core.query.suspensions); prior++ {
			old := core.query.suspensions[prior]
			if old.yield == item.Yield && old.reentry == item.Reentry && old.source == item.Source {
				return invalidQuery("duplicate suspension")
			}
		}
		core.query.suspensions = append(core.query.suspensions, querySuspensionRow{
			yield: item.Yield, reentry: item.Reentry, source: item.Source, multiplicity: item.Multiplicity,
		})
	}
	row.suspensions.end = len(core.query.suspensions)

	row.spawns = queryRange{start: len(core.query.spawns)}
	for _, item := range input.Spawns {
		if item.Child == 0 {
			return invalidQuery("spawn child callback is unavailable")
		}
		childOwner, childOK := core.CallbackOwner(item.Child)
		if !childOK || childOwner != op {
			return invalidQuery("spawn child callback is outside operation")
		}
		if !core.validOutcome(op, item.Yield) || !core.validOutcome(op, item.ParentResume) {
			return invalidQuery("spawn outcome is outside operation")
		}
		if item.ChildEntry != 0 && !core.query.validValues(item.ChildEntry) || item.ResumeValues != 0 && !core.query.validValues(item.ResumeValues) {
			return invalidQuery("spawn Values relation is outside Values table")
		}
		if !validQuerySiblingAlternatives(item.Alternatives) {
			return invalidQuery("spawn sibling alternatives are incomplete")
		}
		core.query.spawns = append(core.query.spawns, querySpawnRow{
			owner: op, function: item.Function, child: item.Child, yield: item.Yield,
			parentResume: item.ParentResume, childEntry: item.ChildEntry, resumeValues: item.ResumeValues,
			alternatives: item.Alternatives,
		})
	}
	row.spawns.end = len(core.query.spawns)

	row.resumes = queryRange{start: len(core.query.resumes)}
	for _, item := range input.Resumes {
		if item.Arguments != 0 && !core.query.validValues(item.Arguments) {
			return invalidQuery("resume arguments are outside Values table")
		}
		for _, outcome := range item.Outcomes {
			if !core.validOutcome(op, outcome) {
				return invalidQuery("resume outcome is outside operation")
			}
		}
		core.query.resumes = append(core.query.resumes, queryResumeRow{
			owner: op, source: item.Source, carrier: item.Carrier, arguments: item.Arguments, outcomes: item.Outcomes,
		})
	}
	row.resumes.end = len(core.query.resumes)
	return nil
}

func (core Core) isOpaque(op vocabulary.Operation) bool {
	opaque, ok := core.Opaque()
	return ok && opaque == op
}

func (core Core) validOutcome(op vocabulary.Operation, outcome uint32) bool {
	return outcome < uint32(core.OutcomeCount(op))
}

func validQuerySiblingAlternatives(values [2]vocabulary.SpawnSiblingAlternative) bool {
	if values[0] == values[1] {
		return false
	}
	for _, value := range values {
		if value != vocabulary.SpawnChildEntryThenParentResume && value != vocabulary.SpawnParentResumeThenChildEntry {
			return false
		}
	}
	return true
}

func (core *Core) appendQueryOutputRows(op vocabulary.Operation, operation *queryOperationRow, input QueryOperationInput) error {
	outcomeCount := core.OutcomeCount(op)
	produced := make([][]ProducedQueryInput, outcomeCount)
	for _, item := range input.Produced {
		if item.Outcome >= uint32(outcomeCount) || item.Target == 0 || int(item.Target) > core.OperationCount() {
			return invalidQuery("produced relation is outside operation table")
		}
		if item.Result >= core.outcomeSlots(op, item.Outcome) {
			return invalidQuery("produced result is outside outcome slots")
		}
		for captureIndex, capture := range item.Captures {
			if capture.Kind == 0 || capture.Kind > vocabulary.CaptureCallback {
				return invalidQuery("produced capture has invalid kind")
			}
			if capture.Kind == vocabulary.CaptureCallback {
				owner, ok := core.CallbackOwner(vocabulary.CallbackID(capture.Ordinal))
				if !ok || owner != op {
					return invalidQuery("produced callback capture is outside operation")
				}
			}
			if capture.Kind == vocabulary.CaptureTypeValueFormal && captureIndex > int(^uint32(0)) {
				return invalidQuery("produced capture index overflow")
			}
		}
		produced[item.Outcome] = append(produced[item.Outcome], item)
	}
	for outcome, items := range produced {
		start := len(core.query.produced)
		for index, item := range items {
			if index > 0 && items[index-1].Result >= item.Result {
				return invalidQuery("produced rows are not strictly ordered")
			}
			captureStart := len(core.query.captures)
			typeValueCapture := noQueryTypeValueCapture
			for captureIndex, capture := range item.Captures {
				if capture.Kind == vocabulary.CaptureTypeValueFormal {
					if typeValueCapture != noQueryTypeValueCapture {
						return invalidQuery("produced row has multiple TypeValue captures")
					}
					typeValueCapture = uint32(captureIndex)
				}
				core.query.captures = append(core.query.captures, queryCaptureRow{kind: capture.Kind, ordinal: capture.Ordinal})
			}
			core.query.produced = append(core.query.produced, queryProducedRow{
				result: item.Result, target: item.Target,
				captures: queryRange{start: captureStart, end: len(core.query.captures)}, typeValueCapture: typeValueCapture,
			})
		}
		operationOutcome := &core.query.outcomeRows[operation.outcomes.start+outcome]
		operationOutcome.produced = queryRange{start: start, end: len(core.query.produced)}
	}

	fresh := make([][]FreshResultInput, outcomeCount)
	for _, item := range input.FreshResults {
		if item.Outcome >= uint32(outcomeCount) || item.Result >= core.outcomeSlots(op, item.Outcome) {
			return invalidQuery("fresh result is outside outcome slots")
		}
		fresh[item.Outcome] = append(fresh[item.Outcome], item)
	}
	for outcome, items := range fresh {
		start := len(core.query.fresh)
		for index, item := range items {
			if index > 0 && items[index-1].Result >= item.Result {
				return invalidQuery("fresh rows are not strictly ordered")
			}
			core.query.fresh = append(core.query.fresh, queryFreshRow{result: item.Result, ordinal: item.Ordinal, kind: item.Kind})
		}
		core.query.outcomeRows[operation.outcomes.start+outcome].fresh = queryRange{start: start, end: len(core.query.fresh)}
	}

	callbackResults := make([][]CallbackResultInput, outcomeCount)
	for _, item := range input.CallbackResults {
		if item.Outcome >= uint32(outcomeCount) || item.Result >= core.outcomeSlots(op, item.Outcome) {
			return invalidQuery("callback result is outside outcome slots")
		}
		owner, ok := core.CallbackOwner(item.Callback)
		if !ok || owner != op {
			return invalidQuery("callback result callback is outside operation")
		}
		callbackResults[item.Outcome] = append(callbackResults[item.Outcome], item)
	}
	for outcome, items := range callbackResults {
		start := len(core.query.callbackResults)
		for index, item := range items {
			if index > 0 && items[index-1].Result >= item.Result {
				return invalidQuery("callback result rows are not strictly ordered")
			}
			core.query.callbackResults = append(core.query.callbackResults, queryCallbackResultRow{result: item.Result, callback: item.Callback})
		}
		core.query.outcomeRows[operation.outcomes.start+outcome].callbackResults = queryRange{start: start, end: len(core.query.callbackResults)}
	}

	aliases := make([][]ResultAliasInput, outcomeCount)
	for _, item := range input.ResultAliases {
		if item.Outcome >= uint32(outcomeCount) || item.Result >= core.outcomeSlots(op, item.Outcome) {
			return invalidQuery("result alias is outside outcome slots")
		}
		aliases[item.Outcome] = append(aliases[item.Outcome], item)
	}
	for outcome, items := range aliases {
		start := len(core.query.resultAliases)
		for index, item := range items {
			if index > 0 && items[index-1].Result >= item.Result {
				return invalidQuery("result alias rows are not strictly ordered")
			}
			core.query.resultAliases = append(core.query.resultAliases, queryResultAliasRow{result: item.Result, source: item.Source})
		}
		core.query.outcomeRows[operation.outcomes.start+outcome].resultAliases = queryRange{start: start, end: len(core.query.resultAliases)}
	}
	return nil
}

func (core Core) outcomeSlots(op vocabulary.Operation, outcome uint32) uint32 {
	slots, ok := core.OutcomeValueSlots(op, int(outcome))
	if !ok {
		return 0
	}
	return slots
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
