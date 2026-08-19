package target

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema"
)

func (c *Contract) OperationCount() int {
	if c == nil {
		return 0
	}
	return c.operationCore.OperationCount()
}

// OperationAt returns one canonical operation by zero-based index. The opaque
// operation is deterministic last and participates normally in all queries.
func (c *Contract) OperationAt(index int) (vocabulary.Operation, bool) {
	if c == nil {
		return 0, false
	}
	return c.operationCore.OperationAt(index)
}

// boundOperationCount is the source/provider operation prefix. Produced-only
// operations and opaque have no source binding and are intentionally absent.
func (c *Contract) boundOperationCount() int {
	if c == nil {
		return 0
	}
	return c.operationCore.BoundCount()
}

func (c *Contract) boundOperationAt(index int) (vocabulary.Operation, bool) {
	if c == nil || index < 0 || index >= c.boundOperationCount() {
		return 0, false
	}
	return c.operationCore.OperationAt(index)
}

// Opaque returns the synthesized maximal opaque operation.
func (c *Contract) Opaque() (vocabulary.Operation, bool) {
	if c == nil {
		return 0, false
	}
	return c.operationCore.Opaque()
}

func (c *Contract) opaqueOperation(op vocabulary.Operation) bool {
	opaque, ok := c.Opaque()
	return ok && op == opaque
}

func (c *Contract) operationIndex(op vocabulary.Operation) (int, bool) {
	if c == nil || op == 0 || uint64(op) > uint64(len(c.operations)) {
		return 0, false
	}
	if _, ok := c.operationCore.OperationAt(int(op) - 1); !ok {
		return 0, false
	}
	return int(op) - 1, true
}

func (c *Contract) operation(op vocabulary.Operation) (operationRow, bool) {
	index, ok := c.operationIndex(op)
	if !ok {
		return operationRow{}, false
	}
	return c.operations[index], true
}

func (c *Contract) operationOutcomeRange(op vocabulary.Operation) (indexRange, bool) {
	row, ok := c.operation(op)
	if !ok {
		return indexRange{}, false
	}
	return row.outcomes, true
}

func (c *Contract) Input(op vocabulary.Operation) (vocabulary.Values, bool) {
	row, ok := c.operation(op)
	if !ok {
		return 0, false
	}
	return row.input, true
}

func (c *Contract) TypeFormalCount(op vocabulary.Operation) int {
	row, ok := c.operation(op)
	if !ok {
		return 0
	}
	return row.typeFormals.len()
}

// TypeFormalConstraint returns a frozen upper-bound constraint. found=false
// means either an invalid coordinate or an unconstrained valid formal.
func (c *Contract) TypeFormalConstraint(op vocabulary.Operation, formal vocabulary.TypeFormal) (vocabulary.Type, bool) {
	row, ok := c.operation(op)
	if !ok || uint64(formal) >= uint64(row.typeFormals.len()) {
		return 0, false
	}
	typeFormals := row.typeFormals
	value := c.formals[typeFormals.start+uint32(formal)]
	if value == 0 {
		return 0, false
	}
	return value, true
}

// ValueFormalCount is the number of fixed Input slots. It is the valid bound
// for EffectSpec.ValueArgs.
func (c *Contract) ValueFormalCount(op vocabulary.Operation) int {
	input, ok := c.Input(op)
	if !ok {
		return 0
	}
	return c.ValuesCount(input)
}

// ValuesVarCount is the operation-scoped open-Values variable arity. It is
// independent from the derived ValueFormal coordinate space.
func (c *Contract) ValuesVarCount(op vocabulary.Operation) int {
	if c == nil {
		return 0
	}
	return int(c.operationCore.ValuesVarCount(op))
}

// ValuesVarType returns the total sealed class for one operation-local open
// Values port. Unconstrained ports use the ABI default neutral Any class.
func (c *Contract) ValuesVarType(op vocabulary.Operation, variable vocabulary.ValuesVar) (vocabulary.Type, bool) {
	row, ok := c.operation(op)
	valuesVars := c.ValuesVarCount(op)
	if !ok || uint64(variable) >= uint64(valuesVars) || row.valuesTypes.len() != valuesVars {
		return 0, false
	}
	valuesTypes := row.valuesTypes
	return c.valuesVarTypes[valuesTypes.start+uint32(variable)], true
}

func (c *Contract) rowFormalCount(op vocabulary.Operation) int {
	row, ok := c.operation(op)
	if !ok {
		return 0
	}
	return int(row.rowFormals)
}

func (c *Contract) ValuesCount(values vocabulary.Values) int {
	row, ok := c.valuesRow(values)
	if !ok {
		return 0
	}
	return row.types.len()
}

func (c *Contract) ValuesAt(values vocabulary.Values, index int) (vocabulary.Type, bool) {
	row, ok := c.valuesRow(values)
	if !ok || index < 0 || index >= row.types.len() {
		return 0, false
	}
	return c.valueTypes[row.types.start+uint32(index)], true
}

// ValuesSuffixCount is the number of fixed end-relative Values elements.
func (c *Contract) ValuesSuffixCount(values vocabulary.Values) int {
	row, ok := c.valuesRow(values)
	if !ok {
		return 0
	}
	return row.suffix.len()
}

// ValuesSuffixAt returns one fixed end-relative Values element.
func (c *Contract) ValuesSuffixAt(values vocabulary.Values, index int) (vocabulary.Type, bool) {
	row, ok := c.valuesRow(values)
	if !ok || index < 0 || index >= row.suffix.len() {
		return 0, false
	}
	return c.valueTypes[row.suffix.start+uint32(index)], true
}

func (c *Contract) ValuesTail(values vocabulary.Values) (vocabulary.ValuesTail, vocabulary.ValuesVar, bool) {
	row, ok := c.valuesRow(values)
	if !ok {
		return 0, 0, false
	}
	return row.tail, row.varID, true
}

// ValuesTailType returns the sealed class of one open ValuesVariable tail.
// An omitted authored class is sealed as neutral Any. Closed and unknown tails do
// not carry an authored class and return found=false.
func (c *Contract) ValuesTailType(values vocabulary.Values) (vocabulary.Type, bool) {
	row, ok := c.valuesRow(values)
	if !ok || row.tail != vocabulary.ValuesVariable {
		return 0, false
	}
	return c.ValuesVarType(row.owner, row.varID)
}

func (c *Contract) valuesRow(values vocabulary.Values) (valuesRow, bool) {
	if c == nil || values == 0 || uint64(values) > uint64(len(c.values)) {
		return valuesRow{}, false
	}
	return c.values[uint32(values)-1], true
}

func (c *Contract) OutcomeCount(op vocabulary.Operation) int {
	row, ok := c.operation(op)
	if !ok {
		return 0
	}
	return row.outcomes.len()
}

// OutcomeAt returns one canonical correlated outcome case. Multiple cases may
// have the same kind; Values and FreshResult relations are the local case
// discriminators, while Produced/callback/alias rows are conjunctive.
func (c *Contract) OutcomeAt(op vocabulary.Operation, index int) (flowkind.OutcomeKind, vocabulary.Values, bool) {
	row, ok := c.operation(op)
	if !ok || index < 0 || index >= row.outcomes.len() {
		return 0, 0, false
	}
	outcomes := row.outcomes
	outcome := c.outcomes[outcomes.start+uint32(index)]
	return outcome.kind, outcome.values, true
}

// BehaviorResultCount returns the number of provider-declared result
// correspondences owned by op. It is zero when the operation has no behavior
// descriptor.
func (c *Contract) BehaviorResultCount(op vocabulary.Operation) int {
	row, ok := c.operation(op)
	if !ok {
		return 0
	}
	return row.behavior.len()
}

// BehaviorResultAt returns one sealed, opaque behavior result relation. The
// returned schema EntryID is an identity only; Target does not interpret the
// provider's vocabulary.
func (c *Contract) BehaviorResultAt(op vocabulary.Operation, index int) (outcome, result uint32, source vocabulary.InputSource, relation schema.EntryID, ok bool) {
	row, ok := c.operation(op)
	if !ok || index < 0 || index >= row.behavior.len() {
		return 0, 0, vocabulary.InputSource{}, schema.EntryID{}, false
	}
	behavior := row.behavior
	item := c.behaviorResults[behavior.start+uint32(index)]
	return item.outcome, item.result, item.source, item.relation, true
}

// BehaviorPredicateCount returns the number of provider-declared predicate
// correspondences owned by op.
func (c *Contract) BehaviorPredicateCount(op vocabulary.Operation) int {
	row, ok := c.operation(op)
	if !ok {
		return 0
	}
	return row.behaviorPredicates.len()
}

// BehaviorPredicateAt returns one sealed, opaque behavior predicate relation.
// Branch polarity is intentionally outside this declaration: the same
// correspondence supports either a positive or a negative branch operation.
func (c *Contract) BehaviorPredicateAt(op vocabulary.Operation, index int) (outcome, result uint32, subject vocabulary.InputSource, relation schema.EntryID, ok bool) {
	row, ok := c.operation(op)
	if !ok || index < 0 || index >= row.behaviorPredicates.len() {
		return 0, 0, vocabulary.InputSource{}, schema.EntryID{}, false
	}
	predicates := row.behaviorPredicates
	item := c.behaviorPredicates[predicates.start+uint32(index)]
	return item.outcome, item.result, item.subject, item.relation, true
}

// transferCount reports the finite endpoint/payload/alias relations owned by op.
// The opaque Operation derives one maximal external/AllInputs relation.
func (c *Contract) transferCount(op vocabulary.Operation) int {
	row, ok := c.operation(op)
	if !ok {
		return 0
	}
	return row.transfers.len()
}

// transferIDAt returns the opaque sealed identity of one exact operation-owned
// transfer declaration. The authored ordinal is consumed here and is never a
// Link or domain identity.
func (c *Contract) transferIDAt(op vocabulary.Operation, index int) (vocabulary.TransferID, bool) {
	row, ok := c.operation(op)
	if !ok || index < 0 || index >= row.transfers.len() {
		return 0, false
	}
	transfers := row.transfers
	return vocabulary.TransferID(transfers.start + uint32(index) + 1), true
}

func (c *Contract) transferID(id vocabulary.TransferID) (transferRow, bool) {
	if c == nil || id == 0 || uint64(id) > uint64(len(c.transfers)) {
		return transferRow{}, false
	}
	row := c.transfers[uint32(id)-1]
	if _, ok := c.operationIndex(row.owner); !ok {
		return transferRow{}, false
	}
	return row, true
}

// transferOwner returns the exact operation that owns one sealed transfer
// declaration. It is a structural Target projection, not a domain judgment.
func (c *Contract) transferOwner(id vocabulary.TransferID) (vocabulary.Operation, bool) {
	row, ok := c.transferID(id)
	return row.owner, ok
}

// transferDeclaration returns the complete authored data relation for one
// sealed transfer identity. It makes no claim about reachable contents,
// proven aliases, isolation, ownership, or a runtime transport strategy.
func (c *Contract) transferDeclaration(id vocabulary.TransferID) (endpoint vocabulary.TransferEndpoint, payload vocabulary.InputSource, alias vocabulary.InputSource, identity vocabulary.TransferIdentity, capabilities vocabulary.TransferCapabilities, ok bool) {
	row, ok := c.transferID(id)
	if !ok {
		return vocabulary.TransferEndpoint{}, vocabulary.InputSource{}, vocabulary.InputSource{}, vocabulary.TransferIdentityInvalid, vocabulary.TransferCapabilitiesInvalid, false
	}
	return row.endpoint, row.payload, row.alias, row.identity, row.capabilities, true
}

// transferDeclarationOutcomeAt returns one canonical operation outcome
// ordinal and its exact declared delivery/rejection possibilities.
func (c *Contract) transferDeclarationOutcomeAt(id vocabulary.TransferID, index int) (uint32, vocabulary.TransferPossibility, bool) {
	row, ok := c.transferID(id)
	if !ok || index < 0 || index >= row.outcomes.len() {
		return 0, 0, false
	}
	return uint32(index), c.transferOutcomes[row.outcomes.start+uint32(index)], true
}

// transferEndpointAt returns one exact destination relation.
func (c *Contract) transferEndpointAt(op vocabulary.Operation, index int) (vocabulary.TransferEndpoint, bool) {
	row, ok := c.transfer(op, index)
	if !ok {
		return vocabulary.TransferEndpoint{}, false
	}
	return row.endpoint, true
}

// transferPayloadAt returns the exact operation-scoped payload coordinate.
func (c *Contract) transferPayloadAt(op vocabulary.Operation, index int) (vocabulary.InputSource, bool) {
	row, ok := c.transfer(op, index)
	if !ok {
		return vocabulary.InputSource{}, false
	}
	return row.payload, true
}

// transferAliasAt returns the exact operation-scoped alias anchor coordinate.
// It is a structural source selector; it asserts neither runtime aliasing nor
// a sender-side isolation class.
func (c *Contract) transferAliasAt(op vocabulary.Operation, index int) (vocabulary.InputSource, bool) {
	row, ok := c.transfer(op, index)
	if !ok {
		return vocabulary.InputSource{}, false
	}
	return row.alias, true
}

// transferIdentityAt returns the exact payload identity relation.
func (c *Contract) transferIdentityAt(op vocabulary.Operation, index int) (vocabulary.TransferIdentity, bool) {
	row, ok := c.transfer(op, index)
	if !ok {
		return vocabulary.TransferIdentityInvalid, false
	}
	return row.identity, true
}

// transferCapabilitiesAt returns the complete capability preservation relation.
func (c *Contract) transferCapabilitiesAt(op vocabulary.Operation, index int) (vocabulary.TransferCapabilities, bool) {
	row, ok := c.transfer(op, index)
	if !ok {
		return vocabulary.TransferCapabilitiesInvalid, false
	}
	return row.capabilities, true
}

// transferOutcomeCount is the owning Operation's exact correlated Outcome
// count. Every sealed Transfer classifies the complete set.
func (c *Contract) transferOutcomeCount(op vocabulary.Operation, transfer int) int {
	row, ok := c.transfer(op, transfer)
	if !ok {
		return 0
	}
	return row.outcomes.len()
}

// transferOutcomeAt returns a canonical Outcome ordinal and its exact
// delivery/rejection possibilities.
func (c *Contract) transferOutcomeAt(op vocabulary.Operation, transfer, index int) (uint32, vocabulary.TransferPossibility, bool) {
	row, ok := c.transfer(op, transfer)
	if !ok || index < 0 || index >= row.outcomes.len() {
		return 0, 0, false
	}
	return uint32(index), c.transferOutcomes[row.outcomes.start+uint32(index)], true
}

func (c *Contract) transfer(op vocabulary.Operation, index int) (transferRow, bool) {
	row, ok := c.operation(op)
	if !ok || index < 0 || index >= row.transfers.len() {
		return transferRow{}, false
	}
	transfers := row.transfers
	return c.transfers[transfers.start+uint32(index)], true
}

func (c *Contract) EffectCount(op vocabulary.Operation) int {
	row, ok := c.operation(op)
	if !ok {
		return 0
	}
	return row.effects.len()
}

func (c *Contract) EffectTarget(op vocabulary.Operation, index int) (vocabulary.Operation, bool) {
	effect, ok := c.effect(op, index)
	if !ok {
		return 0, false
	}
	return effect.target, true
}

// validPublicationEffectRow recomputes the exact target-ABI selector fence at
// every owner query and canonical encoding boundary. The descriptor itself is
// not a capability: only a sealed effect row under this Contract can authorize
// it.
func (c *Contract) validPublicationEffectRow(effect effectRow) bool {
	if c == nil || !effect.hasPublication || !effect.publication.validConsequences() {
		return false
	}
	target, ok := c.Input(effect.target)
	if !ok || uint64(effect.publication.subject) >= uint64(c.ValuesCount(target)) {
		return false
	}
	return effect.publication.destination != vocabulary.PublicationDestinationValueFormal ||
		uint64(effect.publication.context) < uint64(c.ValuesCount(target))
}

func (c *Contract) EffectValueArgumentCount(op vocabulary.Operation, index int) int {
	effect, ok := c.effect(op, index)
	if !ok {
		return 0
	}
	return effect.values.len()
}

func (c *Contract) EffectValueArgumentAt(op vocabulary.Operation, index, argument int) (vocabulary.ValueFormal, bool) {
	effect, ok := c.effect(op, index)
	if !ok || argument < 0 || argument >= effect.values.len() {
		return 0, false
	}
	return c.effectVals[effect.values.start+uint32(argument)], true
}

func (c *Contract) EffectTypeArgumentCount(op vocabulary.Operation, index int) int {
	effect, ok := c.effect(op, index)
	if !ok {
		return 0
	}
	return effect.types.len()
}

func (c *Contract) EffectTypeArgumentAt(op vocabulary.Operation, index, argument int) (vocabulary.TypeFormal, bool) {
	effect, ok := c.effect(op, index)
	if !ok || argument < 0 || argument >= effect.types.len() {
		return 0, false
	}
	return c.effectType[effect.types.start+uint32(argument)], true
}

func (c *Contract) EffectValuesArgumentCount(op vocabulary.Operation, index int) int {
	effect, ok := c.effect(op, index)
	if !ok {
		return 0
	}
	return effect.valuesVar.len()
}

func (c *Contract) EffectValuesArgumentAt(op vocabulary.Operation, index, argument int) (vocabulary.ValuesVar, bool) {
	effect, ok := c.effect(op, index)
	if !ok || argument < 0 || argument >= effect.valuesVar.len() {
		return 0, false
	}
	return c.effectVars[effect.valuesVar.start+uint32(argument)], true
}

func (c *Contract) EffectRowArgumentCount(op vocabulary.Operation, index int) int {
	effect, ok := c.effect(op, index)
	if !ok {
		return 0
	}
	return effect.rows.len()
}

func (c *Contract) effectRowArgumentAt(op vocabulary.Operation, index, argument int) (vocabulary.RowVar, bool) {
	effect, ok := c.effect(op, index)
	if !ok || argument < 0 || argument >= effect.rows.len() {
		return 0, false
	}
	return c.effectRows[effect.rows.start+uint32(argument)], true
}

func (c *Contract) EffectTail(op vocabulary.Operation) (vocabulary.RowTail, vocabulary.RowVar, bool) {
	row, ok := c.operation(op)
	if !ok {
		return 0, 0, false
	}
	return row.effectTail, row.effectVar, true
}

func (c *Contract) effect(op vocabulary.Operation, index int) (effectRow, bool) {
	row, ok := c.operation(op)
	if !ok || index < 0 || index >= row.effects.len() {
		return effectRow{}, false
	}
	effects := row.effects
	return c.effects[effects.start+uint32(index)], true
}

func (c *Contract) BindingCount(op vocabulary.Operation) int {
	if c == nil {
		return 0
	}
	return c.operationCore.BindingCount(op)
}

func (c *Contract) bindingAt(op vocabulary.Operation, index int) (bindingRange, bool) {
	row, ok := c.operation(op)
	if !ok || index < 0 || index >= row.bindings.len() {
		return bindingRange{}, false
	}
	return c.bindings[row.bindings.start+uint32(index)], true
}

func (c *Contract) BindingNamespaceAt(op vocabulary.Operation, binding int) (vocabulary.BindingNamespace, bool) {
	if c == nil {
		return 0, false
	}
	return c.operationCore.BindingNamespaceAt(op, binding)
}

func (c *Contract) BindingOwnerCountAt(op vocabulary.Operation, binding int) int {
	if c == nil {
		return 0
	}
	return c.operationCore.BindingOwnerCountAt(op, binding)
}

func (c *Contract) bindingOwnerAt(op vocabulary.Operation, binding, index int) (string, bool) {
	if c == nil {
		return "", false
	}
	return c.operationCore.BindingOwnerAt(op, binding, index)
}

// bindingOwnerKeyAt returns the exact-key handle for the same binding owner
// segment. The string projection is cold spelling only; Link consumes this
// handle and must not normalize the segment again.
func (c *Contract) bindingOwnerKeyAt(op vocabulary.Operation, binding, index int) (vocabulary.ExactKey, bool) {
	row, ok := c.bindingAt(op, binding)
	if !ok || index < 0 || index >= row.ownerKeys.len() {
		return 0, false
	}
	return c.bindingKeys[row.ownerKeys.start+uint32(index)], true
}

func (c *Contract) BindingMemberCountAt(op vocabulary.Operation, binding int) int {
	if c == nil {
		return 0
	}
	return c.operationCore.BindingMemberCountAt(op, binding)
}

func (c *Contract) BindingMemberAt(op vocabulary.Operation, binding, index int) (string, bool) {
	if c == nil {
		return "", false
	}
	return c.operationCore.BindingMemberAt(op, binding, index)
}

// bindingMemberKeyAt is BindingOwnerKeyAt's member-segment counterpart.
func (c *Contract) bindingMemberKeyAt(op vocabulary.Operation, binding, index int) (vocabulary.ExactKey, bool) {
	row, ok := c.bindingAt(op, binding)
	if !ok || index < 0 || index >= row.memberKeys.len() {
		return 0, false
	}
	return c.bindingKeys[row.memberKeys.start+uint32(index)], true
}

// Lookup finds an exact binding without joining, hashing, parser fallback, or
// allocation. The operation owner binary-searches its sealed canonical segment
// index; returned Operation is the same dense handle exposed by OperationAt.
func (c *Contract) Lookup(binding vocabulary.BindingSpec) (vocabulary.Operation, bool) {
	if c == nil {
		return 0, false
	}
	return c.operationCore.Lookup(binding)
}

// PublicationEffectDescriptor returns the immutable Target-owned publication
// semantics for one exact ordinary effect occurrence. Generic effects remain
// absent unless their author explicitly supplied PublicationEffectSpec.
func (c *Contract) PublicationEffectDescriptor(op vocabulary.Operation, index int) (PublicationEffectDescriptor, bool) {
	row, ok := c.effect(op, index)
	if !ok || !c.sealed || !c.validPublicationEffectRow(row) {
		return PublicationEffectDescriptor{}, false
	}
	return row.publication, true
}
