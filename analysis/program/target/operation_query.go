package target

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func (c *Contract) OperationCount() int {
	if c == nil {
		return 0
	}
	return len(c.operations)
}

// OperationAt returns one canonical operation by zero-based index. The opaque
// operation is deterministic last and participates normally in all queries.
func (c *Contract) OperationAt(index int) (Operation, bool) {
	if c == nil || index < 0 || index >= c.OperationCount() {
		return 0, false
	}
	return Operation(index + 1), true
}

// BoundOperationCount is the source/provider operation prefix. Produced-only
// operations and opaque have no source binding and are intentionally absent.
func (c *Contract) BoundOperationCount() int {
	if c == nil {
		return 0
	}
	return int(c.boundCount)
}

func (c *Contract) BoundOperationAt(index int) (Operation, bool) {
	if c == nil || index < 0 || index >= c.BoundOperationCount() {
		return 0, false
	}
	return Operation(index + 1), true
}

// Opaque returns the synthesized maximal opaque operation.
func (c *Contract) Opaque() (Operation, bool) {
	if c == nil || c.opaque == 0 {
		return 0, false
	}
	return c.opaque, true
}

func (c *Contract) operation(op Operation) (operationRow, bool) {
	if c == nil || op == 0 || uint64(op) > uint64(len(c.operations)) {
		return operationRow{}, false
	}
	return c.operations[uint32(op)-1], true
}

func (c *Contract) Input(op Operation) (Values, bool) {
	row, ok := c.operation(op)
	if !ok {
		return 0, false
	}
	return row.input, true
}

func (c *Contract) TypeFormalCount(op Operation) int {
	row, ok := c.operation(op)
	if !ok {
		return 0
	}
	return row.typeFormals.len()
}

// TypeFormalConstraint returns a frozen upper-bound constraint. found=false
// means either an invalid coordinate or an unconstrained valid formal.
func (c *Contract) TypeFormalConstraint(op Operation, formal TypeFormal) (Type, bool) {
	row, ok := c.operation(op)
	if !ok || uint64(formal) >= uint64(row.typeFormals.len()) {
		return 0, false
	}
	value := c.formals[row.typeFormals.start+uint32(formal)]
	if value == 0 {
		return 0, false
	}
	return value, true
}

// ValueFormalCount is the number of fixed Input slots. It is the valid bound
// for EffectSpec.ValueArgs.
func (c *Contract) ValueFormalCount(op Operation) int {
	row, ok := c.operation(op)
	if !ok {
		return 0
	}
	return c.ValuesCount(row.input)
}

// ValuesVarCount is the operation-scoped open-Values variable arity. It is
// independent from the derived ValueFormal coordinate space.
func (c *Contract) ValuesVarCount(op Operation) int {
	row, ok := c.operation(op)
	if !ok {
		return 0
	}
	return int(row.valuesVars)
}

// ValuesVarType returns the total sealed class for one operation-local open
// Values port. Unconstrained ports use the ABI default neutral Any class.
func (c *Contract) ValuesVarType(op Operation, variable ValuesVar) (Type, bool) {
	row, ok := c.operation(op)
	if !ok || uint64(variable) >= uint64(row.valuesVars) || row.valuesTypes.len() != int(row.valuesVars) {
		return 0, false
	}
	return c.valuesVarTypes[row.valuesTypes.start+uint32(variable)], true
}

func (c *Contract) RowFormalCount(op Operation) int {
	row, ok := c.operation(op)
	if !ok {
		return 0
	}
	return int(row.rowFormals)
}

func (c *Contract) ValuesCount(values Values) int {
	row, ok := c.valuesRow(values)
	if !ok {
		return 0
	}
	return row.types.len()
}

func (c *Contract) ValuesAt(values Values, index int) (Type, bool) {
	row, ok := c.valuesRow(values)
	if !ok || index < 0 || index >= row.types.len() {
		return 0, false
	}
	return c.valueTypes[row.types.start+uint32(index)], true
}

// ValuesSuffixCount is the number of fixed end-relative Values elements.
func (c *Contract) ValuesSuffixCount(values Values) int {
	row, ok := c.valuesRow(values)
	if !ok {
		return 0
	}
	return row.suffix.len()
}

// ValuesSuffixAt returns one fixed end-relative Values element.
func (c *Contract) ValuesSuffixAt(values Values, index int) (Type, bool) {
	row, ok := c.valuesRow(values)
	if !ok || index < 0 || index >= row.suffix.len() {
		return 0, false
	}
	return c.valueTypes[row.suffix.start+uint32(index)], true
}

func (c *Contract) ValuesTail(values Values) (ValuesTail, ValuesVar, bool) {
	row, ok := c.valuesRow(values)
	if !ok {
		return 0, 0, false
	}
	return row.tail, row.varID, true
}

// ValuesTailType returns the sealed class of one open ValuesVariable tail.
// An omitted authored class is sealed as neutral Any. Closed and unknown tails do
// not carry an authored class and return found=false.
func (c *Contract) ValuesTailType(values Values) (Type, bool) {
	row, ok := c.valuesRow(values)
	if !ok || row.tail != ValuesVariable {
		return 0, false
	}
	return c.ValuesVarType(row.owner, row.varID)
}

func (c *Contract) valuesRow(values Values) (valuesRow, bool) {
	if c == nil || values == 0 || uint64(values) > uint64(len(c.values)) {
		return valuesRow{}, false
	}
	return c.values[uint32(values)-1], true
}

func (c *Contract) OutcomeCount(op Operation) int {
	row, ok := c.operation(op)
	if !ok {
		return 0
	}
	return row.outcomes.len()
}

// OutcomeAt returns one canonical correlated outcome case. Multiple cases may
// have the same kind; Values and FreshResult relations are the local case
// discriminators, while Produced/callback/alias rows are conjunctive.
func (c *Contract) OutcomeAt(op Operation, index int) (flowkind.OutcomeKind, Values, bool) {
	row, ok := c.operation(op)
	if !ok || index < 0 || index >= row.outcomes.len() {
		return 0, 0, false
	}
	outcome := c.outcomes[row.outcomes.start+uint32(index)]
	return outcome.kind, outcome.values, true
}

// TransferCount reports the finite endpoint/payload/alias relations owned by op.
// The opaque Operation derives one maximal external/AllInputs relation.
func (c *Contract) TransferCount(op Operation) int {
	row, ok := c.operation(op)
	if !ok {
		return 0
	}
	return row.transfers.len()
}

// TransferIDAt returns the opaque sealed identity of one exact operation-owned
// transfer declaration. The authored ordinal is consumed here and is never a
// Link or domain identity.
func (c *Contract) TransferIDAt(op Operation, index int) (TransferID, bool) {
	operation, ok := c.operation(op)
	if !ok || index < 0 || index >= operation.transfers.len() {
		return 0, false
	}
	return TransferID(operation.transfers.start + uint32(index) + 1), true
}

func (c *Contract) transferID(id TransferID) (transferRow, bool) {
	if c == nil || id == 0 || uint64(id) > uint64(len(c.transfers)) {
		return transferRow{}, false
	}
	row := c.transfers[uint32(id)-1]
	if _, ok := c.operation(row.owner); !ok {
		return transferRow{}, false
	}
	return row, true
}

// TransferOwner returns the exact operation that owns one sealed transfer
// declaration. It is a structural Target projection, not a domain judgment.
func (c *Contract) TransferOwner(id TransferID) (Operation, bool) {
	row, ok := c.transferID(id)
	return row.owner, ok
}

// TransferDeclaration returns the complete authored data relation for one
// sealed transfer identity. It makes no claim about reachable contents,
// proven aliases, isolation, ownership, or a runtime transport strategy.
func (c *Contract) TransferDeclaration(id TransferID) (endpoint TransferEndpoint, payload InputSource, alias InputSource, identity TransferIdentity, capabilities TransferCapabilities, ok bool) {
	row, ok := c.transferID(id)
	if !ok {
		return TransferEndpoint{}, InputSource{}, InputSource{}, TransferIdentityInvalid, TransferCapabilitiesInvalid, false
	}
	return row.endpoint, row.payload, row.alias, row.identity, row.capabilities, true
}

// TransferDeclarationOutcomeCount reports the complete correlated outcome
// classification supplied by one sealed transfer declaration.
func (c *Contract) TransferDeclarationOutcomeCount(id TransferID) int {
	row, ok := c.transferID(id)
	if !ok {
		return 0
	}
	return row.outcomes.len()
}

// TransferDeclarationOutcomeAt returns one canonical operation outcome
// ordinal and its exact declared delivery/rejection possibilities.
func (c *Contract) TransferDeclarationOutcomeAt(id TransferID, index int) (uint32, TransferPossibility, bool) {
	row, ok := c.transferID(id)
	if !ok || index < 0 || index >= row.outcomes.len() {
		return 0, 0, false
	}
	return uint32(index), c.transferOutcomes[row.outcomes.start+uint32(index)], true
}

// TransferEndpointAt returns one exact destination relation.
func (c *Contract) TransferEndpointAt(op Operation, index int) (TransferEndpoint, bool) {
	row, ok := c.transfer(op, index)
	if !ok {
		return TransferEndpoint{}, false
	}
	return row.endpoint, true
}

// TransferPayloadAt returns the exact operation-scoped payload coordinate.
func (c *Contract) TransferPayloadAt(op Operation, index int) (InputSource, bool) {
	row, ok := c.transfer(op, index)
	if !ok {
		return InputSource{}, false
	}
	return row.payload, true
}

// TransferAliasAt returns the exact operation-scoped alias anchor coordinate.
// It is a structural source selector; it asserts neither runtime aliasing nor
// a sender-side isolation class.
func (c *Contract) TransferAliasAt(op Operation, index int) (InputSource, bool) {
	row, ok := c.transfer(op, index)
	if !ok {
		return InputSource{}, false
	}
	return row.alias, true
}

// TransferIdentityAt returns the exact payload identity relation.
func (c *Contract) TransferIdentityAt(op Operation, index int) (TransferIdentity, bool) {
	row, ok := c.transfer(op, index)
	if !ok {
		return TransferIdentityInvalid, false
	}
	return row.identity, true
}

// TransferCapabilitiesAt returns the complete capability preservation relation.
func (c *Contract) TransferCapabilitiesAt(op Operation, index int) (TransferCapabilities, bool) {
	row, ok := c.transfer(op, index)
	if !ok {
		return TransferCapabilitiesInvalid, false
	}
	return row.capabilities, true
}

// TransferOutcomeCount is the owning Operation's exact correlated Outcome
// count. Every sealed Transfer classifies the complete set.
func (c *Contract) TransferOutcomeCount(op Operation, transfer int) int {
	row, ok := c.transfer(op, transfer)
	if !ok {
		return 0
	}
	return row.outcomes.len()
}

// TransferOutcomeAt returns a canonical Outcome ordinal and its exact
// delivery/rejection possibilities.
func (c *Contract) TransferOutcomeAt(op Operation, transfer, index int) (uint32, TransferPossibility, bool) {
	row, ok := c.transfer(op, transfer)
	if !ok || index < 0 || index >= row.outcomes.len() {
		return 0, 0, false
	}
	return uint32(index), c.transferOutcomes[row.outcomes.start+uint32(index)], true
}

func (c *Contract) transfer(op Operation, index int) (transferRow, bool) {
	operation, ok := c.operation(op)
	if !ok || index < 0 || index >= operation.transfers.len() {
		return transferRow{}, false
	}
	return c.transfers[operation.transfers.start+uint32(index)], true
}

func (c *Contract) EffectCount(op Operation) int {
	row, ok := c.operation(op)
	if !ok {
		return 0
	}
	return row.effects.len()
}

func (c *Contract) EffectTarget(op Operation, index int) (Operation, bool) {
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
	target, ok := c.operation(effect.target)
	if !ok || uint64(effect.publication.subject) >= uint64(c.ValuesCount(target.input)) {
		return false
	}
	return effect.publication.destination != PublicationDestinationValueFormal ||
		uint64(effect.publication.context) < uint64(c.ValuesCount(target.input))
}

func (c *Contract) EffectValueArgumentCount(op Operation, index int) int {
	effect, ok := c.effect(op, index)
	if !ok {
		return 0
	}
	return effect.values.len()
}

func (c *Contract) EffectValueArgumentAt(op Operation, index, argument int) (ValueFormal, bool) {
	effect, ok := c.effect(op, index)
	if !ok || argument < 0 || argument >= effect.values.len() {
		return 0, false
	}
	return c.effectVals[effect.values.start+uint32(argument)], true
}

func (c *Contract) EffectTypeArgumentCount(op Operation, index int) int {
	effect, ok := c.effect(op, index)
	if !ok {
		return 0
	}
	return effect.types.len()
}

func (c *Contract) EffectTypeArgumentAt(op Operation, index, argument int) (TypeFormal, bool) {
	effect, ok := c.effect(op, index)
	if !ok || argument < 0 || argument >= effect.types.len() {
		return 0, false
	}
	return c.effectType[effect.types.start+uint32(argument)], true
}

func (c *Contract) EffectValuesArgumentCount(op Operation, index int) int {
	effect, ok := c.effect(op, index)
	if !ok {
		return 0
	}
	return effect.valuesVar.len()
}

func (c *Contract) EffectValuesArgumentAt(op Operation, index, argument int) (ValuesVar, bool) {
	effect, ok := c.effect(op, index)
	if !ok || argument < 0 || argument >= effect.valuesVar.len() {
		return 0, false
	}
	return c.effectVars[effect.valuesVar.start+uint32(argument)], true
}

func (c *Contract) EffectRowArgumentCount(op Operation, index int) int {
	effect, ok := c.effect(op, index)
	if !ok {
		return 0
	}
	return effect.rows.len()
}

func (c *Contract) EffectRowArgumentAt(op Operation, index, argument int) (RowVar, bool) {
	effect, ok := c.effect(op, index)
	if !ok || argument < 0 || argument >= effect.rows.len() {
		return 0, false
	}
	return c.effectRows[effect.rows.start+uint32(argument)], true
}

func (c *Contract) EffectTail(op Operation) (RowTail, RowVar, bool) {
	row, ok := c.operation(op)
	if !ok {
		return 0, 0, false
	}
	return row.effectTail, row.effectVar, true
}

func (c *Contract) effect(op Operation, index int) (effectRow, bool) {
	row, ok := c.operation(op)
	if !ok || index < 0 || index >= row.effects.len() {
		return effectRow{}, false
	}
	return c.effects[row.effects.start+uint32(index)], true
}

func (c *Contract) BindingCount(op Operation) int {
	row, ok := c.operation(op)
	if !ok {
		return 0
	}
	return row.bindings.len()
}

func (c *Contract) bindingAt(op Operation, index int) (bindingRange, bool) {
	row, ok := c.operation(op)
	if !ok || index < 0 || index >= row.bindings.len() {
		return bindingRange{}, false
	}
	return c.bindings[row.bindings.start+uint32(index)], true
}

func (c *Contract) BindingNamespaceAt(op Operation, binding int) (BindingNamespace, bool) {
	row, ok := c.bindingAt(op, binding)
	if !ok {
		return 0, false
	}
	return row.namespace, true
}

func (c *Contract) BindingOwnerCountAt(op Operation, binding int) int {
	row, ok := c.bindingAt(op, binding)
	if !ok {
		return 0
	}
	return row.owner.len()
}

func (c *Contract) BindingOwnerAt(op Operation, binding, index int) (string, bool) {
	row, ok := c.bindingAt(op, binding)
	if !ok || index < 0 || index >= row.owner.len() {
		return "", false
	}
	return c.segments[row.owner.start+uint32(index)], true
}

// BindingOwnerKeyAt returns the exact-key handle for the same binding owner
// segment. The string projection is cold spelling only; Link consumes this
// handle and must not normalize the segment again.
func (c *Contract) BindingOwnerKeyAt(op Operation, binding, index int) (ExactKey, bool) {
	row, ok := c.bindingAt(op, binding)
	if !ok || index < 0 || index >= row.ownerKeys.len() {
		return 0, false
	}
	return c.bindingKeys[row.ownerKeys.start+uint32(index)], true
}

func (c *Contract) BindingMemberCountAt(op Operation, binding int) int {
	row, ok := c.bindingAt(op, binding)
	if !ok {
		return 0
	}
	return row.member.len()
}

func (c *Contract) BindingMemberAt(op Operation, binding, index int) (string, bool) {
	row, ok := c.bindingAt(op, binding)
	if !ok || index < 0 || index >= row.member.len() {
		return "", false
	}
	return c.segments[row.member.start+uint32(index)], true
}

// BindingMemberKeyAt is BindingOwnerKeyAt's member-segment counterpart.
func (c *Contract) BindingMemberKeyAt(op Operation, binding, index int) (ExactKey, bool) {
	row, ok := c.bindingAt(op, binding)
	if !ok || index < 0 || index >= row.memberKeys.len() {
		return 0, false
	}
	return c.bindingKeys[row.memberKeys.start+uint32(index)], true
}

// Lookup finds an exact binding without joining, hashing, parser fallback, or
// allocation. It binary-searches the sealed canonical segment index; returned
// Operation is the same dense handle exposed by OperationAt.
func (c *Contract) Lookup(binding BindingSpec) (Operation, bool) {
	if c == nil || !validBinding(binding) {
		return 0, false
	}
	left, right := 0, len(c.lookup)
	for left < right {
		middle := left + (right-left)/2
		row := c.lookup[middle]
		order := compareBindingRangeSpec(c.bindings[row.binding], binding, c.segments)
		if order < 0 {
			left = middle + 1
		} else {
			right = middle
		}
	}
	if left >= len(c.lookup) {
		return 0, false
	}
	row := c.lookup[left]
	if !c.bindingEqual(c.bindings[row.binding], binding) {
		return 0, false
	}
	return row.operation, true
}
