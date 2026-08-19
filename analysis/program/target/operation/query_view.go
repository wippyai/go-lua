package operation

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

func (core Core) queryOperation(op vocabulary.Operation) (queryOperationRow, bool) {
	if op == 0 || int(op) > len(core.query.operations) {
		return queryOperationRow{}, false
	}
	return core.query.operations[int(op)-1], true
}

func (core Core) Input(op vocabulary.Operation) (vocabulary.Values, bool) {
	row, ok := core.queryOperation(op)
	return row.input, ok
}

func (core Core) TypeFormalCount(op vocabulary.Operation) int {
	row, ok := core.queryOperation(op)
	if !ok {
		return 0
	}
	return len(row.typeFormals)
}

func (core Core) TypeFormalConstraint(op vocabulary.Operation, formal vocabulary.TypeFormal) (vocabulary.Type, bool) {
	row, ok := core.queryOperation(op)
	if !ok || formal < 0 || int(formal) >= len(row.typeFormals) {
		return 0, false
	}
	typ := row.typeFormals[formal]
	return typ, typ != 0
}

func (core Core) ValueFormalCount(op vocabulary.Operation) int {
	input, ok := core.Input(op)
	if !ok {
		return 0
	}
	return core.ValuesCount(input)
}

func (core Core) ValuesVarType(op vocabulary.Operation, variable vocabulary.ValuesVar) (vocabulary.Type, bool) {
	row, ok := core.queryOperation(op)
	if !ok || variable < 0 || int(variable) >= len(row.valuesTypes) {
		return 0, false
	}
	typ := row.valuesTypes[variable]
	if typ == 0 {
		return 0, false
	}
	return typ, true
}

func (core Core) RowFormalCount(op vocabulary.Operation) int {
	row, ok := core.queryOperation(op)
	if !ok {
		return 0
	}
	return int(row.rowFormals)
}

func (core Core) ValuesCount(values vocabulary.Values) int {
	row, ok := core.queryValues(values)
	if !ok {
		return 0
	}
	return len(row.types)
}

func (core Core) ValuesAt(values vocabulary.Values, index int) (vocabulary.Type, bool) {
	row, ok := core.queryValues(values)
	if !ok || index < 0 || index >= len(row.types) {
		return 0, false
	}
	return row.types[index], true
}

func (core Core) ValuesSuffixCount(values vocabulary.Values) int {
	row, ok := core.queryValues(values)
	if !ok {
		return 0
	}
	return len(row.suffix)
}

func (core Core) ValuesSuffixAt(values vocabulary.Values, index int) (vocabulary.Type, bool) {
	row, ok := core.queryValues(values)
	if !ok || index < 0 || index >= len(row.suffix) {
		return 0, false
	}
	return row.suffix[index], true
}

func (core Core) ValuesTail(values vocabulary.Values) (vocabulary.ValuesTail, vocabulary.ValuesVar, bool) {
	row, ok := core.queryValues(values)
	if !ok {
		return 0, 0, false
	}
	return row.tail, row.varID, true
}

func (core Core) ValuesTailType(values vocabulary.Values) (vocabulary.Type, bool) {
	row, ok := core.queryValues(values)
	if !ok || row.tail != vocabulary.ValuesVariable {
		return 0, false
	}
	return core.ValuesVarType(row.owner, row.varID)
}

func (core Core) queryValues(values vocabulary.Values) (queryValuesRow, bool) {
	if values == 0 || int(values) > len(core.query.values) {
		return queryValuesRow{}, false
	}
	return core.query.values[int(values)-1], true
}

func (core Core) OutcomeAt(op vocabulary.Operation, index int) (flowkind.OutcomeKind, vocabulary.Values, bool) {
	row, ok := core.queryOperation(op)
	if !ok || index < 0 || index >= row.outcomes.len() {
		return 0, 0, false
	}
	outcome := core.query.outcomeRows[row.outcomes.start+index]
	return outcome.kind, outcome.values, true
}

// OutcomePositionAt returns the immutable flat query position for one local
// outcome. It is a join key for identity columns, not a second outcome handle.
func (core Core) OutcomePositionAt(op vocabulary.Operation, index int) (int, bool) {
	row, ok := core.queryOperation(op)
	if !ok || index < 0 || index >= row.outcomes.len() {
		return 0, false
	}
	return row.outcomes.start + index, true
}

func (core Core) BehaviorResultCount(op vocabulary.Operation) int {
	row, ok := core.queryOperation(op)
	if !ok {
		return 0
	}
	return row.behavior.len()
}

func (core Core) BehaviorResultAt(op vocabulary.Operation, index int) (outcome, result uint32, source vocabulary.InputSource, relation schema.EntryID, ok bool) {
	row, ok := core.queryOperation(op)
	if !ok || index < 0 || index >= row.behavior.len() {
		return 0, 0, vocabulary.InputSource{}, schema.EntryID{}, false
	}
	item := core.query.behaviorRows[row.behavior.start+index]
	return item.outcome, item.result, item.source, item.relation, true
}

func (core Core) BehaviorPredicateCount(op vocabulary.Operation) int {
	row, ok := core.queryOperation(op)
	if !ok {
		return 0
	}
	return row.behaviorPredicates.len()
}

func (core Core) BehaviorPredicateAt(op vocabulary.Operation, index int) (outcome, result uint32, subject vocabulary.InputSource, relation schema.EntryID, ok bool) {
	row, ok := core.queryOperation(op)
	if !ok || index < 0 || index >= row.behaviorPredicates.len() {
		return 0, 0, vocabulary.InputSource{}, schema.EntryID{}, false
	}
	item := core.query.predicateRows[row.behaviorPredicates.start+index]
	return item.outcome, item.result, item.subject, item.relation, true
}

func (core Core) TransferCount(op vocabulary.Operation) int {
	row, ok := core.queryOperation(op)
	if !ok {
		return 0
	}
	return row.transfers.len()
}

func (core Core) TransferIDAt(op vocabulary.Operation, index int) (vocabulary.TransferID, bool) {
	row, ok := core.queryOperation(op)
	if !ok || index < 0 || index >= row.transfers.len() {
		return 0, false
	}
	return vocabulary.TransferID(row.transfers.start + index + 1), true
}

func (core Core) transfer(id vocabulary.TransferID) (queryTransferRow, bool) {
	if id == 0 || int(id) > len(core.query.transfers) {
		return queryTransferRow{}, false
	}
	row := core.query.transfers[int(id)-1]
	if _, ok := core.OperationAt(int(row.owner) - 1); !ok {
		return queryTransferRow{}, false
	}
	return row, true
}

func (core Core) TransferOwner(id vocabulary.TransferID) (vocabulary.Operation, bool) {
	row, ok := core.transfer(id)
	return row.owner, ok
}

func (core Core) TransferDeclaration(id vocabulary.TransferID) (endpoint vocabulary.TransferEndpoint, payload vocabulary.InputSource, alias vocabulary.InputSource, identity vocabulary.TransferIdentity, capabilities vocabulary.TransferCapabilities, ok bool) {
	row, ok := core.transfer(id)
	if !ok {
		return vocabulary.TransferEndpoint{}, vocabulary.InputSource{}, vocabulary.InputSource{}, vocabulary.TransferIdentityInvalid, vocabulary.TransferCapabilitiesInvalid, false
	}
	return row.endpoint, row.payload, row.alias, row.identity, row.capabilities, true
}

func (core Core) TransferDeclarationOutcomeAt(id vocabulary.TransferID, index int) (uint32, vocabulary.TransferPossibility, bool) {
	row, ok := core.transfer(id)
	if !ok || index < 0 || index >= row.outcomes.len() {
		return 0, 0, false
	}
	return uint32(index), core.query.transferEnds[row.outcomes.start+index], true
}

// TransferOutcomePositionAt returns the sealed flat position of one outcome
// in the transfer owner. It is an identity-plane join key, not a new public
// handle or a reconstructed scan.
func (core Core) TransferOutcomePositionAt(id vocabulary.TransferID, index int) (int, bool) {
	row, ok := core.transfer(id)
	if !ok || index < 0 || index >= row.outcomes.len() {
		return 0, false
	}
	return row.outcomes.start + index, true
}

func (core Core) transferAt(op vocabulary.Operation, index int) (queryTransferRow, bool) {
	row, ok := core.queryOperation(op)
	if !ok || index < 0 || index >= row.transfers.len() {
		return queryTransferRow{}, false
	}
	return core.query.transfers[row.transfers.start+index], true
}

func (core Core) TransferEndpointAt(op vocabulary.Operation, index int) (vocabulary.TransferEndpoint, bool) {
	row, ok := core.transferAt(op, index)
	return row.endpoint, ok
}

func (core Core) TransferPayloadAt(op vocabulary.Operation, index int) (vocabulary.InputSource, bool) {
	row, ok := core.transferAt(op, index)
	return row.payload, ok
}

func (core Core) TransferAliasAt(op vocabulary.Operation, index int) (vocabulary.InputSource, bool) {
	row, ok := core.transferAt(op, index)
	return row.alias, ok
}

func (core Core) TransferIdentityAt(op vocabulary.Operation, index int) (vocabulary.TransferIdentity, bool) {
	row, ok := core.transferAt(op, index)
	return row.identity, ok
}

func (core Core) TransferCapabilitiesAt(op vocabulary.Operation, index int) (vocabulary.TransferCapabilities, bool) {
	row, ok := core.transferAt(op, index)
	return row.capabilities, ok
}

func (core Core) TransferOutcomeCount(op vocabulary.Operation, index int) int {
	row, ok := core.transferAt(op, index)
	if !ok {
		return 0
	}
	return row.outcomes.len()
}

func (core Core) TransferOutcomeAt(op vocabulary.Operation, transfer, index int) (uint32, vocabulary.TransferPossibility, bool) {
	row, ok := core.transferAt(op, transfer)
	if !ok || index < 0 || index >= row.outcomes.len() {
		return 0, 0, false
	}
	return uint32(index), core.query.transferEnds[row.outcomes.start+index], true
}

func (core Core) EffectCount(op vocabulary.Operation) int {
	row, ok := core.queryOperation(op)
	if !ok {
		return 0
	}
	return len(row.effects)
}

func (core Core) EffectTarget(op vocabulary.Operation, index int) (vocabulary.Operation, bool) {
	effect, ok := core.effect(op, index)
	return effect.target, ok
}

func (core Core) EffectValueArgumentCount(op vocabulary.Operation, index int) int {
	effect, ok := core.effect(op, index)
	if !ok {
		return 0
	}
	return len(effect.values)
}

func (core Core) EffectValueArgumentAt(op vocabulary.Operation, index, argument int) (vocabulary.ValueFormal, bool) {
	effect, ok := core.effect(op, index)
	if !ok || argument < 0 || argument >= len(effect.values) {
		return 0, false
	}
	return effect.values[argument], true
}

func (core Core) EffectTypeArgumentCount(op vocabulary.Operation, index int) int {
	effect, ok := core.effect(op, index)
	if !ok {
		return 0
	}
	return len(effect.types)
}

func (core Core) EffectTypeArgumentAt(op vocabulary.Operation, index, argument int) (vocabulary.TypeFormal, bool) {
	effect, ok := core.effect(op, index)
	if !ok || argument < 0 || argument >= len(effect.types) {
		return 0, false
	}
	return effect.types[argument], true
}

func (core Core) EffectValuesArgumentCount(op vocabulary.Operation, index int) int {
	effect, ok := core.effect(op, index)
	if !ok {
		return 0
	}
	return len(effect.valuesVar)
}

func (core Core) EffectValuesArgumentAt(op vocabulary.Operation, index, argument int) (vocabulary.ValuesVar, bool) {
	effect, ok := core.effect(op, index)
	if !ok || argument < 0 || argument >= len(effect.valuesVar) {
		return 0, false
	}
	return effect.valuesVar[argument], true
}

func (core Core) EffectRowArgumentCount(op vocabulary.Operation, index int) int {
	effect, ok := core.effect(op, index)
	if !ok {
		return 0
	}
	return len(effect.rows)
}

func (core Core) EffectRowArgumentAt(op vocabulary.Operation, index, argument int) (vocabulary.RowVar, bool) {
	effect, ok := core.effect(op, index)
	if !ok || argument < 0 || argument >= len(effect.rows) {
		return 0, false
	}
	return effect.rows[argument], true
}

func (core Core) EffectTail(op vocabulary.Operation) (vocabulary.RowTail, vocabulary.RowVar, bool) {
	row, ok := core.queryOperation(op)
	if !ok {
		return 0, 0, false
	}
	return row.effectTail, row.effectVar, true
}

func (core Core) effect(op vocabulary.Operation, index int) (queryEffectRow, bool) {
	row, ok := core.queryOperation(op)
	if !ok || index < 0 || index >= len(row.effects) {
		return queryEffectRow{}, false
	}
	handle := row.effects[index]
	if handle < 0 || handle >= len(core.query.effects) {
		return queryEffectRow{}, false
	}
	return core.query.effects[handle], true
}

// EffectPublication returns the exact authored publication value retained by
// an operation effect. It is intentionally a vocabulary value so the owner
// does not depend on Target's public facade type.
func (core Core) EffectPublication(op vocabulary.Operation, index int) (vocabulary.PublicationEffectSpec, bool) {
	effect, ok := core.effect(op, index)
	if !ok || !effect.hasPublication {
		return vocabulary.PublicationEffectSpec{}, false
	}
	return effect.publication, true
}

func (core Core) TypeDeclaration(typ vocabulary.Type) (schematype.Type, bool) {
	if typ == 0 || int(typ) > len(core.query.types) {
		return schematype.Type{}, false
	}
	return core.query.types[int(typ)-1].declaration, true
}

const (
	opaqueSuspensionCount   = 3
	queryNoTypeValueCapture = ^uint32(0)
)

func (core Core) SuspensionCount(op vocabulary.Operation) int {
	row, ok := core.queryOperation(op)
	if !ok {
		return 0
	}
	if core.isOpaque(op) {
		return opaqueSuspensionCount
	}
	return row.suspensions.len()
}

func (core Core) SuspensionAt(op vocabulary.Operation, index int) (yield, reentry uint32, source vocabulary.ReentrySource, multiplicity vocabulary.ReentryMultiplicity, ok bool) {
	row, ok := core.queryOperation(op)
	if !ok || index < 0 {
		return 0, 0, 0, 0, false
	}
	if core.isOpaque(op) {
		if index >= opaqueSuspensionCount {
			return 0, 0, 0, 0, false
		}
		reentry := uint32(index)
		if index == 2 {
			reentry = 3
		}
		return 2, reentry, vocabulary.ReentryByProvider, vocabulary.ReentryMany, true
	}
	if index >= row.suspensions.len() {
		return 0, 0, 0, 0, false
	}
	item := core.query.suspensions[row.suspensions.start+index]
	return item.yield, item.reentry, item.source, item.multiplicity, true
}

func (core Core) SpawnCount(op vocabulary.Operation) int {
	row, ok := core.queryOperation(op)
	if !ok || core.isOpaque(op) {
		return 0
	}
	return row.spawns.len()
}

func (core Core) SpawnIDAt(op vocabulary.Operation, index int) (vocabulary.SpawnID, bool) {
	row, ok := core.queryOperation(op)
	if !ok || core.isOpaque(op) || index < 0 || index >= row.spawns.len() {
		return 0, false
	}
	return vocabulary.SpawnID(row.spawns.start + index + 1), true
}

func (core Core) spawn(id vocabulary.SpawnID) (querySpawnRow, bool) {
	if id == 0 || int(id) > len(core.query.spawns) {
		return querySpawnRow{}, false
	}
	return core.query.spawns[id-1], true
}

func (core Core) SpawnRelation(id vocabulary.SpawnID) (owner vocabulary.Operation, function vocabulary.InputSource, child vocabulary.CallbackID, parentYield, parentResume uint32, childEntry, resumeValues vocabulary.Values, ok bool) {
	row, ok := core.spawn(id)
	if !ok {
		return 0, vocabulary.InputSource{}, 0, 0, 0, 0, 0, false
	}
	return row.owner, row.function, row.child, row.yield, row.parentResume, row.childEntry, row.resumeValues, true
}

func (core Core) SpawnSiblingCount(id vocabulary.SpawnID) int {
	if _, ok := core.spawn(id); !ok {
		return 0
	}
	return 2
}

func (core Core) SpawnSiblingAt(id vocabulary.SpawnID, index int) (vocabulary.SpawnSiblingAlternative, bool) {
	row, ok := core.spawn(id)
	if !ok || index < 0 || index >= 2 {
		return vocabulary.SpawnSiblingInvalid, false
	}
	return row.alternatives[index], true
}

func (core Core) ResumeCount(op vocabulary.Operation) int {
	row, ok := core.queryOperation(op)
	if !ok || core.isOpaque(op) {
		return 0
	}
	return row.resumes.len()
}

func (core Core) ResumeIDAt(op vocabulary.Operation, index int) (vocabulary.ResumeID, bool) {
	row, ok := core.queryOperation(op)
	if !ok || core.isOpaque(op) || index < 0 || index >= row.resumes.len() {
		return 0, false
	}
	return vocabulary.ResumeID(row.resumes.start + index + 1), true
}

func (core Core) resume(id vocabulary.ResumeID) (queryResumeRow, bool) {
	if id == 0 || int(id) > len(core.query.resumes) {
		return queryResumeRow{}, false
	}
	return core.query.resumes[id-1], true
}

func (core Core) Resume(id vocabulary.ResumeID) (owner vocabulary.Operation, source vocabulary.ResumeSource, carrier vocabulary.ValueFormal, arguments vocabulary.Values, ok bool) {
	row, ok := core.resume(id)
	if !ok {
		return 0, 0, 0, 0, false
	}
	return row.owner, row.source, row.carrier, row.arguments, true
}

func (core Core) ResumeOutcomeCount(id vocabulary.ResumeID) int {
	if _, ok := core.resume(id); !ok {
		return 0
	}
	return 5
}

func (core Core) ResumeOutcomeAt(id vocabulary.ResumeID, index int) (flowkind.OutcomeKind, uint32, bool) {
	row, ok := core.resume(id)
	if !ok || index < 0 || index >= 5 {
		return 0, 0, false
	}
	return [...]flowkind.OutcomeKind{
		flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow,
		flowkind.OutcomeYield, flowkind.OutcomeCancel,
	}[index], row.outcomes[index], true
}

func (core Core) CallbackResultCount(op vocabulary.Operation, outcome int) int {
	row, ok := core.outcomeQueryRow(op, outcome)
	if !ok {
		return 0
	}
	return row.callbackResults.len()
}

func (core Core) CallbackResultAt(op vocabulary.Operation, outcome, index int) (uint32, vocabulary.CallbackID, bool) {
	row, ok := core.outcomeQueryRow(op, outcome)
	if !ok || index < 0 || index >= row.callbackResults.len() {
		return 0, 0, false
	}
	item := core.query.callbackResults[row.callbackResults.start+index]
	return item.result, item.callback, true
}

func (core Core) CallbackForResult(op vocabulary.Operation, outcome int, result uint32) (vocabulary.CallbackID, int, bool) {
	count := core.CallbackResultCount(op, outcome)
	left, right := 0, count
	for left < right {
		mid := left + (right-left)/2
		current, _, ok := core.CallbackResultAt(op, outcome, mid)
		if !ok {
			return 0, 0, false
		}
		if current < result {
			left = mid + 1
		} else {
			right = mid
		}
	}
	if left >= count {
		return 0, 0, false
	}
	current, callback, ok := core.CallbackResultAt(op, outcome, left)
	return callback, left, ok && current == result
}

func (core Core) ResultAliasCount(op vocabulary.Operation, outcome int) int {
	row, ok := core.outcomeQueryRow(op, outcome)
	if !ok {
		return 0
	}
	return row.resultAliases.len()
}

func (core Core) ResultAliasAt(op vocabulary.Operation, outcome, index int) (uint32, vocabulary.InputSourceKind, uint32, bool) {
	row, ok := core.outcomeQueryRow(op, outcome)
	if !ok || index < 0 || index >= row.resultAliases.len() {
		return 0, 0, 0, false
	}
	item := core.query.resultAliases[row.resultAliases.start+index]
	return item.result, item.source.Kind, item.source.Ordinal, true
}

func (core Core) ResultAliasForResult(op vocabulary.Operation, outcome int, result uint32) (vocabulary.InputSourceKind, uint32, int, bool) {
	count := core.ResultAliasCount(op, outcome)
	left, right := 0, count
	for left < right {
		mid := left + (right-left)/2
		current, _, _, ok := core.ResultAliasAt(op, outcome, mid)
		if !ok {
			return 0, 0, 0, false
		}
		if current < result {
			left = mid + 1
		} else {
			right = mid
		}
	}
	if left >= count {
		return 0, 0, 0, false
	}
	current, kind, source, ok := core.ResultAliasAt(op, outcome, left)
	return kind, source, left, ok && current == result
}

func (core Core) ProducedCount(op vocabulary.Operation, outcome int) int {
	row, ok := core.outcomeQueryRow(op, outcome)
	if !ok {
		return 0
	}
	return row.produced.len()
}

func (core Core) ProducedAt(op vocabulary.Operation, outcome, index int) (uint32, vocabulary.Operation, bool) {
	row, ok := core.outcomeQueryRow(op, outcome)
	if !ok || index < 0 || index >= row.produced.len() {
		return 0, 0, false
	}
	item := core.query.produced[row.produced.start+index]
	return item.result, item.target, true
}

func (core Core) ProducedForResult(op vocabulary.Operation, outcome int, result uint32) (vocabulary.Operation, int, bool) {
	count := core.ProducedCount(op, outcome)
	left, right := 0, count
	for left < right {
		mid := left + (right-left)/2
		current, _, ok := core.ProducedAt(op, outcome, mid)
		if !ok {
			return 0, 0, false
		}
		if current < result {
			left = mid + 1
		} else {
			right = mid
		}
	}
	if left >= count {
		return 0, 0, false
	}
	current, target, ok := core.ProducedAt(op, outcome, left)
	return target, left, ok && current == result
}

func (core Core) ProducedCaptureCount(op vocabulary.Operation, outcome, produced int) int {
	item, ok := core.producedRowAt(op, outcome, produced)
	if !ok {
		return 0
	}
	return item.captures.len()
}

func (core Core) ProducedCaptureAt(op vocabulary.Operation, outcome, produced, capture int) (vocabulary.CaptureKind, uint32, bool) {
	item, ok := core.producedRowAt(op, outcome, produced)
	if !ok || capture < 0 || capture >= item.captures.len() {
		return 0, 0, false
	}
	value := core.query.captures[item.captures.start+capture]
	return value.kind, value.ordinal, true
}

func (core Core) ProducedTypeValueCapture(op vocabulary.Operation, outcome, produced int) (vocabulary.ValueFormal, bool) {
	item, ok := core.producedRowAt(op, outcome, produced)
	if !ok || item.typeValueCapture == queryNoTypeValueCapture || item.typeValueCapture >= uint32(item.captures.len()) {
		return 0, false
	}
	value := core.query.captures[item.captures.start+int(item.typeValueCapture)]
	if value.kind != vocabulary.CaptureTypeValueFormal {
		return 0, false
	}
	return vocabulary.ValueFormal(value.ordinal), true
}

func (core Core) FreshResultCount(op vocabulary.Operation, outcome int) int {
	row, ok := core.outcomeQueryRow(op, outcome)
	if !ok {
		return 0
	}
	return row.fresh.len()
}

func (core Core) FreshResultAt(op vocabulary.Operation, outcome, index int) (result, ordinal uint32, kind schematype.FreshClass, ok bool) {
	row, ok := core.outcomeQueryRow(op, outcome)
	if !ok || index < 0 || index >= row.fresh.len() {
		return 0, 0, schematype.FreshClassInvalid, false
	}
	item := core.query.fresh[row.fresh.start+index]
	return item.result, item.ordinal, item.kind, true
}

func (core Core) FreshResultForResult(op vocabulary.Operation, outcome int, result uint32) (ordinal uint32, kind schematype.FreshClass, index int, ok bool) {
	count := core.FreshResultCount(op, outcome)
	left, right := 0, count
	for left < right {
		mid := left + (right-left)/2
		current, _, _, found := core.FreshResultAt(op, outcome, mid)
		if !found {
			return 0, schematype.FreshClassInvalid, 0, false
		}
		if current < result {
			left = mid + 1
		} else {
			right = mid
		}
	}
	if left >= count {
		return 0, schematype.FreshClassInvalid, 0, false
	}
	current, ordinal, kind, found := core.FreshResultAt(op, outcome, left)
	return ordinal, kind, left, found && current == result
}

func (core Core) outcomeQueryRow(op vocabulary.Operation, outcome int) (queryOutcomeRow, bool) {
	row, ok := core.queryOperation(op)
	if !ok || outcome < 0 || outcome >= row.outcomes.len() {
		return queryOutcomeRow{}, false
	}
	return core.query.outcomeRows[row.outcomes.start+outcome], true
}

func (core Core) producedRowAt(op vocabulary.Operation, outcome, index int) (queryProducedRow, bool) {
	row, ok := core.outcomeQueryRow(op, outcome)
	if !ok || index < 0 || index >= row.produced.len() {
		return queryProducedRow{}, false
	}
	return core.query.produced[row.produced.start+index], true
}
