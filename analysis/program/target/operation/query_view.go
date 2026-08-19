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
