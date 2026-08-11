package target

import (
	"sort"

	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

// InitialRootCount returns every sealed Target boot aggregate.
func (c *Contract) InitialRootCount() int {
	if c == nil {
		return 0
	}
	return len(c.initialRoots)
}

// GlobalEnvRoot is the sole initially mutable table root shared by every
// initial global binding. It is unavailable when the contract has no global
// bindings.
func (c *Contract) GlobalEnvRoot() (InitialRoot, bool) {
	if c == nil || c.globalEnvRoot == 0 {
		return 0, false
	}
	return c.globalEnvRoot, true
}

// InitialAbsent returns the unique canonical structural absent value. It is
// the total initial value for arbitrary Program globals and is unavailable
// only when the sealed ledger did not author an absent value.
func (c *Contract) InitialAbsent() (InitialValue, bool) {
	if c == nil || c.initialAbsent == 0 {
		return 0, false
	}
	return c.initialAbsent, true
}

func (c *Contract) InitialRootAt(index int) (InitialRoot, bool) {
	if c == nil || index < 0 || index >= len(c.initialRoots) {
		return 0, false
	}
	return InitialRoot(index + 1), true
}

func (c *Contract) initialRoot(root InitialRoot) (initialRootRow, bool) {
	if c == nil || root == 0 || uint64(root) > uint64(len(c.initialRoots)) {
		return initialRootRow{}, false
	}
	return c.initialRoots[uint32(root)-1], true
}

func (c *Contract) InitialRootIdentity(root InitialRoot) (string, bool) {
	row, ok := c.initialRoot(root)
	return row.identity, ok
}

func (c *Contract) InitialRootBootShape(root InitialRoot) (BootShape, bool) {
	row, ok := c.initialRoot(root)
	return row.shape, ok
}

func (c *Contract) bootShape(shape BootShape) (bootShapeRow, bool) {
	if c == nil || shape == 0 || uint64(shape) > uint64(len(c.bootShapes)) {
		return bootShapeRow{}, false
	}
	return c.bootShapes[uint32(shape)-1], true
}

func (c *Contract) BootShapeRoot(shape BootShape) (InitialRoot, bool) {
	row, ok := c.bootShape(shape)
	return row.root, ok
}

func (c *Contract) BootShapeAggregate(shape BootShape) (BootAggregate, bool) {
	row, ok := c.bootShape(shape)
	return row.aggregate, ok
}

// BootShapeImmutable reports the exact initial whole-object header policy of
// one boot aggregate.  It is not derived from, and has no bearing on the
// independent InitialMutability policies of its individual entries.
func (c *Contract) BootShapeImmutable(shape BootShape) (bool, bool) {
	row, ok := c.bootShape(shape)
	return row.immutable, ok
}

func (c *Contract) BootShapeValue(shape BootShape) (InitialValue, bool) {
	row, ok := c.bootShape(shape)
	return row.value, ok
}

func (c *Contract) initialValue(value InitialValue) (initialValueRow, bool) {
	if c == nil || value == 0 || uint64(value) > uint64(len(c.initialValues)) {
		return initialValueRow{}, false
	}
	return c.initialValues[uint32(value)-1], true
}

func (c *Contract) InitialValueKind(value InitialValue) (InitialValueKind, bool) {
	row, ok := c.initialValue(value)
	return row.kind, ok
}

func (c *Contract) InitialValueBoolean(value InitialValue) (bool, bool) {
	row, ok := c.initialValue(value)
	return row.boolean, ok && row.kind == InitialValueBoolean
}

func (c *Contract) InitialValueInteger(value InitialValue) (int64, bool) {
	row, ok := c.initialValue(value)
	return row.integer, ok && row.kind == InitialValueInteger
}

func (c *Contract) InitialValueFloatBits(value InitialValue) (uint64, bool) {
	row, ok := c.initialValue(value)
	return row.floatBits, ok && row.kind == InitialValueFloat
}

func (c *Contract) InitialValueString(value InitialValue) (string, bool) {
	row, ok := c.initialValue(value)
	return row.string, ok && row.kind == InitialValueString
}

func (c *Contract) InitialValueRoot(value InitialValue) (InitialRoot, bool) {
	row, ok := c.initialValue(value)
	return row.root, ok && row.kind == InitialValueRoot
}

func (c *Contract) InitialValueOperation(value InitialValue) (Operation, bool) {
	row, ok := c.initialValue(value)
	return row.operation, ok && row.kind == InitialValueOperation
}

// InitialOperation resolves one exact boot root/key occurrence directly to
// its admitted Target operation.  InitialEntry owns the canonical root/key
// binary-search index; this reduction deliberately does not walk the exact
// key or initial-entry tables and does not admit roots, literals, denied
// bindings, or other initial-value kinds as operations.
func (c *Contract) InitialOperation(root InitialRoot, key ExactKey) (Operation, bool) {
	value, _, ok := c.InitialEntry(root, key)
	if !ok {
		return 0, false
	}
	return c.InitialValueOperation(value)
}

func (c *Contract) initialValueBinding(value InitialValue) (bindingRange, bool) {
	row, ok := c.initialValue(value)
	if !ok || row.kind != InitialValueDeniedOperation || row.binding == 0 || uint64(row.binding) > uint64(len(c.initialValueBinds)) {
		return bindingRange{}, false
	}
	return c.initialValueBinds[row.binding-1], true
}

func (c *Contract) InitialValueDeniedNamespace(value InitialValue) (BindingNamespace, bool) {
	row, ok := c.initialValueBinding(value)
	return row.namespace, ok
}

func (c *Contract) InitialValueDeniedOwnerCount(value InitialValue) int {
	row, ok := c.initialValueBinding(value)
	if !ok {
		return 0
	}
	return row.owner.len()
}

func (c *Contract) InitialValueDeniedOwnerAt(value InitialValue, index int) (string, bool) {
	row, ok := c.initialValueBinding(value)
	if !ok || index < 0 || index >= row.owner.len() {
		return "", false
	}
	return c.segments[row.owner.start+uint32(index)], true
}

// InitialValueDeniedOwnerKeyAt returns the exact-key atom for a denied
// binding owner segment. The string projection is artifact spelling only.
func (c *Contract) InitialValueDeniedOwnerKeyAt(value InitialValue, index int) (ExactKey, bool) {
	row, ok := c.initialValueBinding(value)
	if !ok || index < 0 || index >= row.ownerKeys.len() {
		return 0, false
	}
	return c.bindingKeys[row.ownerKeys.start+uint32(index)], true
}

func (c *Contract) InitialValueDeniedMemberCount(value InitialValue) int {
	row, ok := c.initialValueBinding(value)
	if !ok {
		return 0
	}
	return row.member.len()
}

func (c *Contract) InitialValueDeniedMemberAt(value InitialValue, index int) (string, bool) {
	row, ok := c.initialValueBinding(value)
	if !ok || index < 0 || index >= row.member.len() {
		return "", false
	}
	return c.segments[row.member.start+uint32(index)], true
}

// InitialValueDeniedMemberKeyAt returns the exact-key atom for a denied
// binding member segment. The string projection is artifact spelling only.
func (c *Contract) InitialValueDeniedMemberKeyAt(value InitialValue, index int) (ExactKey, bool) {
	row, ok := c.initialValueBinding(value)
	if !ok || index < 0 || index >= row.memberKeys.len() {
		return 0, false
	}
	return c.bindingKeys[row.memberKeys.start+uint32(index)], true
}

// ExactKeyCount returns every Target-local canonical exact-key atom. Handles
// are contract-local and never interchangeable with Program or Link keys.
func (c *Contract) ExactKeyCount() int {
	if c == nil {
		return 0
	}
	return len(c.exactKeys)
}

func (c *Contract) ExactKeyAt(index int) (ExactKey, bool) {
	if c == nil || index < 0 || index >= len(c.exactKeys) {
		return 0, false
	}
	return ExactKey(index + 1), true
}

// ExactKeyValue returns the normalized typed Lua key payload for one sealed
// Target handle. It is the only spelling/payload projection for hot key rows.
func (c *Contract) ExactKeyValue(key ExactKey) (keyspace.LiteralValue, bool) {
	if c == nil || key == 0 || uint64(key) > uint64(len(c.exactKeys)) {
		return keyspace.LiteralValue{}, false
	}
	return c.exactKeys[key-1], true
}

func (c *Contract) InitialEntryCount() int {
	if c == nil {
		return 0
	}
	return len(c.initialEntries)
}

func (c *Contract) InitialEntryAt(index int) (InitialRoot, ExactKey, InitialValue, InitialMutability, bool) {
	if c == nil || index < 0 || index >= len(c.initialEntries) {
		return 0, 0, 0, 0, false
	}
	row := c.initialEntries[index]
	return row.root, row.key, row.value, row.mutability, true
}

// InitialEntry performs an allocation-free binary search over canonical
// root/key rows.
func (c *Contract) InitialEntry(root InitialRoot, key ExactKey) (InitialValue, InitialMutability, bool) {
	if c == nil || root == 0 || key == 0 || uint64(key) > uint64(len(c.exactKeys)) {
		return 0, 0, false
	}
	index := sort.Search(len(c.initialEntries), func(index int) bool {
		entry := c.initialEntries[index]
		return entry.root > root || (entry.root == root && entry.key >= key)
	})
	if index == len(c.initialEntries) || c.initialEntries[index].root != root || c.initialEntries[index].key != key {
		return 0, 0, false
	}
	row := c.initialEntries[index]
	return row.value, row.mutability, true
}

// InitialMetatableAttachmentCount returns the closed bootstrap attachment
// ledger. It contains no mutable table attachment state.
func (c *Contract) InitialMetatableAttachmentCount() int {
	if c == nil {
		return 0
	}
	return len(c.initialMetatables)
}

// InitialMetatableAttachmentAt returns one canonical primitive-base to
// metatable-root bootstrap relation. Base is an existing InitialValueKind and
// metatable is an existing InitialRoot with BootAggregateMetatable shape.
func (c *Contract) InitialMetatableAttachmentAt(index int) (base InitialValueKind, metatable InitialRoot, ok bool) {
	if c == nil || index < 0 || index >= len(c.initialMetatables) {
		return InitialValueInvalid, 0, false
	}
	row := c.initialMetatables[index]
	if row.base != InitialValueString || row.metatable == 0 || uint64(row.metatable) > uint64(len(c.initialRoots)) {
		return InitialValueInvalid, 0, false
	}
	shape, valid := c.InitialRootBootShape(row.metatable)
	aggregate, aggregateOK := c.BootShapeAggregate(shape)
	if !valid || !aggregateOK || aggregate != BootAggregateMetatable {
		return InitialValueInvalid, 0, false
	}
	return row.base, row.metatable, true
}

func (c *Contract) InitialBindingCount() int {
	if c == nil {
		return 0
	}
	return len(c.initialBindings)
}

func (c *Contract) InitialBindingAt(index int) (string, InitialBindingClass, InitialValue, InitialRoot, ExactKey, bool) {
	if c == nil || index < 0 || index >= len(c.initialBindings) {
		return "", 0, 0, 0, 0, false
	}
	row := c.initialBindings[index]
	value, _, ok := c.InitialEntry(row.root, row.key)
	if !ok {
		return "", 0, 0, 0, 0, false
	}
	kind, ok := c.InitialValueKind(value)
	if !ok {
		return "", 0, 0, 0, 0, false
	}
	return row.name, initialBindingClassForValue(kind), value, row.root, row.key, true
}

// InitialBinding returns the frozen three-way disposition together with the
// exact initial value that determines it; Ordinary is therefore never vague.
func (c *Contract) InitialBinding(name string) (InitialBindingClass, InitialValue, InitialRoot, ExactKey, bool) {
	if c == nil {
		return 0, 0, 0, 0, false
	}
	index := sort.Search(len(c.initialBindings), func(index int) bool { return c.initialBindings[index].name >= name })
	if index == len(c.initialBindings) || c.initialBindings[index].name != name {
		return 0, 0, 0, 0, false
	}
	row := c.initialBindings[index]
	value, _, ok := c.InitialEntry(row.root, row.key)
	if !ok {
		return 0, 0, 0, 0, false
	}
	kind, ok := c.InitialValueKind(value)
	if !ok {
		return 0, 0, 0, 0, false
	}
	return initialBindingClassForValue(kind), value, row.root, row.key, true
}

// OperationCount returns every sealed operation, including produced-only and
// explicit opaque rows. Use BoundOperationCount for source/provider bindings.
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
// Values port. Unconstrained ports use the ABI default typ.Any class.
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
// An omitted authored class is sealed as typ.Any. Closed and unknown tails do
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

func (c *Contract) CallbackCount(op Operation) int {
	row, ok := c.operation(op)
	if !ok {
		return 0
	}
	return row.callbacks.len()
}

func (c *Contract) CallbackAt(op Operation, index int) (CallbackID, bool) {
	row, ok := c.operation(op)
	if !ok || index < 0 || index >= row.callbacks.len() {
		return 0, false
	}
	return CallbackID(row.callbacks.start + uint32(index) + 1), true
}

func (c *Contract) callback(id CallbackID) (callbackRow, bool) {
	if c == nil || id == 0 || uint64(id) > uint64(len(c.callbacks)) {
		return callbackRow{}, false
	}
	return c.callbacks[uint32(id)-1], true
}

// CallbackOwner returns the exact sealed operation that owns a callback
// correspondence. The range validation keeps a malformed callback row from
// being accepted merely because its stored owner is an otherwise valid
// operation.
func (c *Contract) CallbackOwner(id CallbackID) (Operation, bool) {
	row, ok := c.callback(id)
	if !ok || row.owner == 0 {
		return 0, false
	}
	owner, ok := c.operation(row.owner)
	if !ok {
		return 0, false
	}
	index := uint32(id) - 1
	if index < owner.callbacks.start || index >= owner.callbacks.end {
		return 0, false
	}
	return row.owner, true
}

// CallbackFunction returns the exact input authority that supplies a callback
// function. Authored callbacks use ValueFormal; the opaque callback uses the
// sole maximal AllInputs authority.
func (c *Contract) CallbackFunction(id CallbackID) (InputSource, bool) {
	row, ok := c.callback(id)
	if !ok {
		return InputSource{}, false
	}
	return row.function, true
}

// CallbackArguments returns the full Values schema at the callback argument
// role. Equal Values handles across roles are structural deduplication only,
// never a flow claim.
func (c *Contract) CallbackArguments(id CallbackID) (Values, bool) {
	row, ok := c.callback(id)
	return row.arguments, ok
}

// CallbackOutcome returns the exact Values relation carried by one callback
// activation outcome. It prescribes no provider response to that outcome.
func (c *Contract) CallbackOutcome(id CallbackID, kind flowkind.OutcomeKind) (Values, bool) {
	index, valid := crossActivationOutcomeIndex(kind)
	if !valid {
		return 0, false
	}
	row, ok := c.callback(id)
	if !ok {
		return 0, false
	}
	return row.outcomes[index], true
}

// CallbackAdmission returns the callback's sole callable convention. A
// callback-backed Subedge projects this value; it never carries a duplicate.
func (c *Contract) CallbackAdmission(id CallbackID) (Admission, bool) {
	row, ok := c.callback(id)
	return row.admission, ok
}

// CallbackOpaque exposes the one explicit maximally conservative callback
// owned by the synthesized opaque operation. Missing authored Subedge rows do
// not imply a callback is closed, successful, or non-reentrant.
func (c *Contract) CallbackOpaque(id CallbackID) bool {
	owner, ok := c.CallbackOwner(id)
	return ok && owner == c.opaque
}

// CallbackSubedge returns the sole immediate typed application of callback.
// It is a derived sealed reverse index over SubedgeCallback, not a second
// execution relation or semantic identity coordinate. Retained and opaque
// callbacks intentionally have no immediate Subedge.
func (c *Contract) CallbackSubedge(id CallbackID) (SubedgeID, bool) {
	callback, ok := c.callback(id)
	if !ok || callback.subedge == 0 {
		return 0, false
	}
	edge, ok := c.subedge(callback.subedge)
	if !ok || edge.callee != SubedgeCalleeCallback || edge.callback != id {
		return 0, false
	}
	return callback.subedge, true
}

// SubedgeCount returns every sealed typed internal application owned by op.
func (c *Contract) SubedgeCount(op Operation) int {
	row, ok := c.operation(op)
	if !ok {
		return 0
	}
	return row.subedges.len()
}

func (c *Contract) SubedgeAt(op Operation, index int) (SubedgeID, bool) {
	row, ok := c.operation(op)
	if !ok || index < 0 || index >= row.subedges.len() {
		return 0, false
	}
	return SubedgeID(row.subedges.start + uint32(index) + 1), true
}

func (c *Contract) subedge(id SubedgeID) (subedgeRow, bool) {
	if c == nil || id == 0 || uint64(id) > uint64(len(c.subedges)) {
		return subedgeRow{}, false
	}
	return c.subedges[id-1], true
}

// SubedgeOwner is the only Target ownership projection needed by Link. It
// deliberately does not create a Candidate, Application, or Program Term.
func (c *Contract) SubedgeOwner(id SubedgeID) (Operation, bool) {
	row, ok := c.subedge(id)
	if !ok || row.owner == 0 {
		return 0, false
	}
	owner, ok := c.operation(row.owner)
	if !ok {
		return 0, false
	}
	index := uint32(id) - 1
	if index < owner.subedges.start || index >= owner.subedges.end {
		return 0, false
	}
	return row.owner, true
}

func (c *Contract) SubedgeRole(id SubedgeID) (uint32, bool) {
	row, ok := c.subedge(id)
	return row.role, ok
}

func (c *Contract) SubedgeFamily(id SubedgeID) (SubedgeFamily, bool) {
	row, ok := c.subedge(id)
	return row.family, ok
}

func (c *Contract) SubedgeCallee(id SubedgeID) (SubedgeCalleeKind, bool) {
	row, ok := c.subedge(id)
	return row.callee, ok
}

func (c *Contract) SubedgeAdmission(id SubedgeID) (Admission, bool) {
	row, ok := c.subedge(id)
	return row.admission, ok
}

func (c *Contract) SubedgeCallback(id SubedgeID) (CallbackID, bool) {
	row, ok := c.subedge(id)
	if !ok || row.callee != SubedgeCalleeCallback || row.callback == 0 {
		return 0, false
	}
	return row.callback, true
}

// SubedgeCapturedInitialRead reports the capture-once boot source owned by
// this edge. The SubedgeID itself is its operation-local capture identity.
func (c *Contract) SubedgeCapturedInitialRead(id SubedgeID) (InitialRoot, ExactKey, bool) {
	row, ok := c.subedge(id)
	if !ok || row.callee != SubedgeCalleeCapturedInitialRead || row.readRoot == 0 || row.readKey == 0 {
		return 0, 0, false
	}
	return row.readRoot, row.readKey, true
}

func (c *Contract) SubedgeMetaKey(id SubedgeID) (ExactKey, bool) {
	row, ok := c.subedge(id)
	if !ok || row.callee != SubedgeCalleeMetaKey || row.metaKey == 0 {
		return 0, false
	}
	return row.metaKey, true
}

// SubedgeArguments returns the contextual callee-argument Values endpoint.
// Equal Values handles in another role are never implicit dataflow.
func (c *Contract) SubedgeArguments(id SubedgeID) (Values, bool) {
	row, ok := c.subedge(id)
	return row.arguments, ok
}

// SubedgeRuleEntry reports whether an argument-free Subedge has explicit
// owner-Rule entry authority. Nonempty direct arguments use ArgumentOrigins.
func (c *Contract) SubedgeRuleEntry(id SubedgeID) (bool, bool) {
	row, ok := c.subedge(id)
	return row.ruleEntry, ok
}

// ArgumentOriginCount reports the authored complete source set for this
// contextual argument endpoint. A zero count is either the explicit nullary
// Rule entry reported by SubedgeRuleEntry or a complete sibling/admission
// route; it never implies an entry by itself.
func (c *Contract) ArgumentOriginCount(id SubedgeID) int {
	row, ok := c.subedge(id)
	if !ok {
		return 0
	}
	return row.argumentOrigins.len()
}

// ArgumentOriginAt returns one direct owner-input or owner-Rule source
// for a single argument Values segment. The source is zero for Rule entries.
func (c *Contract) ArgumentOriginAt(id SubedgeID, index int) (segment ArgumentSegment, ordinal uint32, source ArgumentSource, input InputSource, ok bool) {
	row, found := c.subedge(id)
	if !found || index < 0 || index >= row.argumentOrigins.len() {
		return ArgumentSegmentInvalid, 0, ArgumentSourceInvalid, InputSource{}, false
	}
	item := c.subedgeOrigins[row.argumentOrigins.start+uint32(index)]
	return item.segment, item.index, item.kind, item.source, true
}

func (c *Contract) SubedgeTerminal(id SubedgeID, kind flowkind.OutcomeKind) (Values, bool) {
	index, valid := crossActivationOutcomeIndex(kind)
	if !valid {
		return 0, false
	}
	row, ok := c.subedge(id)
	if !ok {
		return 0, false
	}
	return row.outcomes[index], true
}

// AdmissionFailure returns the distinct exact Values source produced
// when this edge's callable admission fails. It is neither candidate absence
// nor the callee Throw terminal.
func (c *Contract) AdmissionFailure(id SubedgeID) (Values, bool) {
	row, ok := c.subedge(id)
	if !ok || row.admissionFailure == 0 {
		return 0, false
	}
	return row.admissionFailure, true
}

// AdmissionRoute returns the one explicit transport of a callable
// admission failure. Only Outcome and Subedge routes are representable.
func (c *Contract) AdmissionRoute(id SubedgeID) (route SubedgeRoute, adjustment Adjustment, result Values, placement Placement, offset uint32, outcome uint32, sibling SubedgeID, destination Values, ok bool) {
	row, found := c.subedge(id)
	if !found || row.admissionRoute.route == RouteInvalid {
		return RouteInvalid, AdjustmentInvalid, 0, PlacementInvalid, 0, 0, 0, 0, false
	}
	item := row.admissionRoute
	return item.route, item.adjustment, item.result, item.placement, item.offset, item.outcome, item.subedge, item.destination, true
}

// SubedgeRouteAt returns the contextual projected Result and its one route.
// RejectYield's Result is the canonical C-boundary error Values, not the
// discarded child Yield payload; it may target an owner Throw or sibling edge.
func (c *Contract) SubedgeRouteAt(id SubedgeID, kind flowkind.OutcomeKind) (route SubedgeRoute, adjustment Adjustment, result Values, placement Placement, offset uint32, outcome uint32, sibling SubedgeID, destination Values, ok bool) {
	index, valid := crossActivationOutcomeIndex(kind)
	if !valid {
		return RouteInvalid, AdjustmentInvalid, 0, PlacementInvalid, 0, 0, 0, 0, false
	}
	row, found := c.subedge(id)
	if !found {
		return RouteInvalid, AdjustmentInvalid, 0, PlacementInvalid, 0, 0, 0, 0, false
	}
	item := row.routes[index]
	if item.route == RouteInvalid {
		return RouteInvalid, AdjustmentInvalid, 0, PlacementInvalid, 0, 0, 0, 0, false
	}
	return item.route, item.adjustment, item.result, item.placement, item.offset, item.outcome, item.subedge, item.destination, true
}

// CallbackLifecycle returns the complete sealed callback lifecycle relation.
func (c *Contract) CallbackLifecycle(id CallbackID) (CallbackLifecycle, bool) {
	row, ok := c.callback(id)
	return row.lifecycle, ok
}

// CallbackEffectCount returns the finite explicit occurrences in the
// callback's expected Koka row.
func (c *Contract) CallbackEffectCount(id CallbackID) int {
	row, ok := c.callback(id)
	if !ok {
		return 0
	}
	return row.effects.len()
}

func (c *Contract) CallbackEffectTarget(id CallbackID, index int) (Operation, bool) {
	effect, ok := c.callbackEffect(id, index)
	if !ok {
		return 0, false
	}
	return effect.target, true
}

func (c *Contract) CallbackEffectValueArgumentCount(id CallbackID, index int) int {
	effect, ok := c.callbackEffect(id, index)
	if !ok {
		return 0
	}
	return effect.values.len()
}

func (c *Contract) CallbackEffectValueArgumentAt(id CallbackID, index, argument int) (ValueFormal, bool) {
	effect, ok := c.callbackEffect(id, index)
	if !ok || argument < 0 || argument >= effect.values.len() {
		return 0, false
	}
	return c.effectVals[effect.values.start+uint32(argument)], true
}

func (c *Contract) CallbackEffectTypeArgumentCount(id CallbackID, index int) int {
	effect, ok := c.callbackEffect(id, index)
	if !ok {
		return 0
	}
	return effect.types.len()
}

func (c *Contract) CallbackEffectTypeArgumentAt(id CallbackID, index, argument int) (TypeFormal, bool) {
	effect, ok := c.callbackEffect(id, index)
	if !ok || argument < 0 || argument >= effect.types.len() {
		return 0, false
	}
	return c.effectType[effect.types.start+uint32(argument)], true
}

func (c *Contract) CallbackEffectValuesArgumentCount(id CallbackID, index int) int {
	effect, ok := c.callbackEffect(id, index)
	if !ok {
		return 0
	}
	return effect.valuesVar.len()
}

func (c *Contract) CallbackEffectValuesArgumentAt(id CallbackID, index, argument int) (ValuesVar, bool) {
	effect, ok := c.callbackEffect(id, index)
	if !ok || argument < 0 || argument >= effect.valuesVar.len() {
		return 0, false
	}
	return c.effectVars[effect.valuesVar.start+uint32(argument)], true
}

func (c *Contract) CallbackEffectRowArgumentCount(id CallbackID, index int) int {
	effect, ok := c.callbackEffect(id, index)
	if !ok {
		return 0
	}
	return effect.rows.len()
}

func (c *Contract) CallbackEffectRowArgumentAt(id CallbackID, index, argument int) (RowVar, bool) {
	effect, ok := c.callbackEffect(id, index)
	if !ok || argument < 0 || argument >= effect.rows.len() {
		return 0, false
	}
	return c.effectRows[effect.rows.start+uint32(argument)], true
}

// CallbackEffectTail returns the callback's expected row tail.
func (c *Contract) CallbackEffectTail(id CallbackID) (RowTail, RowVar, bool) {
	row, ok := c.callback(id)
	if !ok {
		return 0, 0, false
	}
	return row.effectTail, row.effectVar, true
}

func (c *Contract) callbackEffect(id CallbackID, index int) (effectRow, bool) {
	row, ok := c.callback(id)
	if !ok || index < 0 || index >= row.effects.len() {
		return effectRow{}, false
	}
	return c.effects[row.effects.start+uint32(index)], true
}

// CallbackRelease reports the optional explicit causal release of a retained
// callback. The release operation owns the reverse range exposed below.
func (c *Contract) CallbackRelease(id CallbackID) (Operation, ValueFormal, uint32, CallbackReleaseMode, bool) {
	row, ok := c.callback(id)
	if !ok || row.release == 0 || uint64(row.release) > uint64(len(c.callbackReleases)) {
		return 0, 0, 0, CallbackReleaseInvalid, false
	}
	release := c.callbackReleases[row.release-1]
	if release.callback != id {
		return 0, 0, 0, CallbackReleaseInvalid, false
	}
	return release.operation, release.input, release.outcome, release.mode, true
}

// CallbackReleaseZero reports the required zero-holder arm of an explicit
// retained callback release. The outcome is meaningful only for Throw and
// Idempotent; Suppress returns zero and creates no terminal successor.
func (c *Contract) CallbackReleaseZero(id CallbackID) (CallbackReleaseZeroBehavior, uint32, bool) {
	row, ok := c.callback(id)
	if !ok || row.release == 0 || uint64(row.release) > uint64(len(c.callbackReleases)) {
		return CallbackReleaseZeroInvalid, 0, false
	}
	release := c.callbackReleases[row.release-1]
	if release.callback != id || !validCallbackReleaseZeroBehavior(release.zeroBehavior) {
		return CallbackReleaseZeroInvalid, 0, false
	}
	return release.zeroBehavior, release.zeroOutcome, true
}

// CallbackReleaseCount returns releases caused by one source-visible operation.
func (c *Contract) CallbackReleaseCount(op Operation) int {
	row, ok := c.operation(op)
	if !ok {
		return 0
	}
	return row.releases.len()
}

// CallbackReleaseAt returns one release in the operation's dense direct range.
func (c *Contract) CallbackReleaseAt(op Operation, index int) (CallbackID, ValueFormal, uint32, CallbackReleaseMode, bool) {
	row, ok := c.operation(op)
	if !ok || index < 0 || index >= row.releases.len() {
		return 0, 0, 0, CallbackReleaseInvalid, false
	}
	release := c.callbackReleases[row.releases.start+uint32(index)]
	if release.operation != op {
		return 0, 0, 0, CallbackReleaseInvalid, false
	}
	return release.callback, release.input, release.outcome, release.mode, true
}

// SuspensionCount reports exact authored suspension relations. The one opaque
// Operation derives its three maximal provider reentries from Contract.Opaque;
// no duplicate opaque flag or authored fallback row exists.
func (c *Contract) SuspensionCount(op Operation) int {
	row, ok := c.operation(op)
	if !ok {
		return 0
	}
	if op == c.opaque {
		return 3
	}
	return row.suspensions.len()
}

// SuspensionAt returns an operation-owned relation. For opaque, index 0..2
// derives Yield → Normal/Throw/Cancel in that canonical outcome order; opaque
// Values remain the Contract's existing unknown Values relation.
func (c *Contract) SuspensionAt(op Operation, index int) (yield, reentry uint32, source ReentrySource, multiplicity ReentryMultiplicity, ok bool) {
	row, ok := c.operation(op)
	if !ok || index < 0 {
		return 0, 0, 0, 0, false
	}
	if op == c.opaque {
		if index >= 3 {
			return 0, 0, 0, 0, false
		}
		reentry := uint32(index)
		if index == 2 {
			reentry = 3
		}
		return 2, reentry, ReentryByProvider, ReentryMany, true
	}
	if index >= row.suspensions.len() {
		return 0, 0, 0, 0, false
	}
	value := c.suspensions[row.suspensions.start+uint32(index)]
	return value.yield, value.reentry, value.source, value.multiplicity, true
}

// SpawnCount reports the finite detached-spawn authorities owned by op. A
// sealed contract admits at most one such authority globally.
func (c *Contract) SpawnCount(op Operation) int {
	row, ok := c.operation(op)
	if !ok || op == c.opaque {
		return 0
	}
	return row.spawns.len()
}

// SpawnIDAt returns the sealed identity of an operation-owned spawn relation.
func (c *Contract) SpawnIDAt(op Operation, index int) (SpawnID, bool) {
	row, ok := c.operation(op)
	if !ok || op == c.opaque || index < 0 || index >= row.spawns.len() {
		return 0, false
	}
	return SpawnID(row.spawns.start + uint32(index) + 1), true
}

func (c *Contract) spawn(id SpawnID) (spawnRow, bool) {
	if c == nil || id == 0 || uint64(id) > uint64(len(c.spawns)) {
		return spawnRow{}, false
	}
	return c.spawns[uint32(id)-1], true
}

// Spawn exposes the one typed detached application correspondence. Function
// is the exact parent input authority and Child is its existing callback
// activation relation. ParentYield/ParentResume are canonical owner outcome
// ordinals. ChildEntry and ResumeValues are existing closed empty Packs.
func (c *Contract) Spawn(id SpawnID) (owner Operation, function InputSource, child CallbackID, parentYield, parentResume uint32, childEntry, resumeValues Values, ok bool) {
	row, ok := c.spawn(id)
	if !ok {
		return 0, InputSource{}, 0, 0, 0, 0, 0, false
	}
	return row.owner, row.function, row.child, row.yield, row.parentResume, row.childEntry, row.resumeValues, true
}

// SpawnSiblingCount is always two for a valid spawn relation: both concrete
// enabled-event orders are explicit and neither is inferred by a scheduler.
func (c *Contract) SpawnSiblingCount(id SpawnID) int {
	if _, ok := c.spawn(id); !ok {
		return 0
	}
	return 2
}

// SpawnSiblingAt returns one concrete legal sibling ordering.
func (c *Contract) SpawnSiblingAt(id SpawnID, index int) (SpawnSiblingAlternative, bool) {
	if index < 0 || index >= 2 {
		return SpawnSiblingInvalid, false
	}
	row, ok := c.spawn(id)
	if !ok {
		return SpawnSiblingInvalid, false
	}
	return row.alternatives[index], true
}

func (c *Contract) ResumeCount(op Operation) int {
	row, ok := c.operation(op)
	if !ok || op == c.opaque {
		return 0
	}
	return row.resumes.len()
}

// ResumeIDAt returns the sealed identity for one exact operation-local resume
// correspondence. The authored ordinal is consumed at this boundary and must
// not be retained as a Link or runtime identity.
func (c *Contract) ResumeIDAt(op Operation, index int) (ResumeID, bool) {
	row, ok := c.operation(op)
	if !ok || op == c.opaque || index < 0 || index >= row.resumes.len() {
		return 0, false
	}
	return ResumeID(row.resumes.start + uint32(index) + 1), true
}

func (c *Contract) resume(id ResumeID) (resumeRow, bool) {
	if c == nil || id == 0 || uint64(id) > uint64(len(c.resumes)) {
		return resumeRow{}, false
	}
	return c.resumes[uint32(id)-1], true
}

// Resume returns the owning operation and exact activation operand declaration
// for a sealed resumption correspondence. A Produced source has no
// ValueFormal carrier; its carrier is the existing Produced result origin and
// any retained CallbackID is queried through ProducedCaptureAt. Arguments is
// the existing operation Values coordinate supplied to the restored activation.
func (c *Contract) Resume(id ResumeID) (owner Operation, source ResumeSource, carrier ValueFormal, arguments Values, ok bool) {
	row, ok := c.resume(id)
	if !ok {
		return 0, 0, 0, 0, false
	}
	return row.owner, row.source, row.carrier, row.arguments, true
}

// ResumeOutcomeCount is always five for a valid resume: Normal, Return,
// Throw, Yield, and Cancel in that canonical Kind order.
func (c *Contract) ResumeOutcomeCount(resume ResumeID) int {
	if _, ok := c.resume(resume); !ok {
		return 0
	}
	return 5
}

// ResumeOutcomeAt returns one canonical cross-activation outcome mapping.
// Outcome is the canonical ordinal of the owning operation outcome.
func (c *Contract) ResumeOutcomeAt(resume ResumeID, index int) (kind flowkind.OutcomeKind, outcome uint32, ok bool) {
	if index < 0 || index >= 5 {
		return 0, 0, false
	}
	value, found := c.resume(resume)
	if !found {
		return 0, 0, false
	}
	return [...]flowkind.OutcomeKind{flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow, flowkind.OutcomeYield, flowkind.OutcomeCancel}[index], value.outcomes[index], true
}

func (c *Contract) CallbackResultCount(op Operation, outcome int) int {
	row, ok := c.operation(op)
	if !ok || outcome < 0 || outcome >= row.outcomes.len() {
		return 0
	}
	return c.outcomes[row.outcomes.start+uint32(outcome)].callbackResults.len()
}

func (c *Contract) callbackResultAt(op Operation, outcome, index int) (callbackResultRow, bool) {
	row, ok := c.operation(op)
	if !ok || outcome < 0 || outcome >= row.outcomes.len() {
		return callbackResultRow{}, false
	}
	results := c.outcomes[row.outcomes.start+uint32(outcome)].callbackResults
	if index < 0 || index >= results.len() {
		return callbackResultRow{}, false
	}
	return c.callbackResults[results.start+uint32(index)], true
}

func (c *Contract) CallbackResultAt(op Operation, outcome, index int) (uint32, CallbackID, bool) {
	row, ok := c.callbackResultAt(op, outcome, index)
	if !ok {
		return 0, 0, false
	}
	return row.result, row.callback, true
}

func (c *Contract) CallbackForResult(op Operation, outcome int, result uint32) (CallbackID, int, bool) {
	count := c.CallbackResultCount(op, outcome)
	left, right := 0, count
	for left < right {
		mid := left + (right-left)/2
		current, _, ok := c.CallbackResultAt(op, outcome, mid)
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
	current, callback, ok := c.CallbackResultAt(op, outcome, left)
	if !ok || current != result {
		return 0, 0, false
	}
	return callback, left, true
}

// ResultAliasCount returns the static result-to-input correspondences owned
// by one outcome.
func (c *Contract) ResultAliasCount(op Operation, outcome int) int {
	row, ok := c.operation(op)
	if !ok || outcome < 0 || outcome >= row.outcomes.len() {
		return 0
	}
	return c.outcomes[row.outcomes.start+uint32(outcome)].resultAliases.len()
}

func (c *Contract) resultAliasAt(op Operation, outcome, index int) (resultAliasRow, bool) {
	row, ok := c.operation(op)
	if !ok || outcome < 0 || outcome >= row.outcomes.len() {
		return resultAliasRow{}, false
	}
	aliases := c.outcomes[row.outcomes.start+uint32(outcome)].resultAliases
	if index < 0 || index >= aliases.len() {
		return resultAliasRow{}, false
	}
	return c.resultAliases[aliases.start+uint32(index)], true
}

// ResultAliasAt returns one canonical result-to-ValueFormal correspondence.
func (c *Contract) ResultAliasAt(op Operation, outcome, index int) (uint32, InputSourceKind, uint32, bool) {
	row, ok := c.resultAliasAt(op, outcome, index)
	if !ok {
		return 0, 0, 0, false
	}
	return row.result, row.source.Kind, row.source.Ordinal, true
}

// ResultAliasForResult finds the correspondence for one fixed result slot.
func (c *Contract) ResultAliasForResult(op Operation, outcome int, result uint32) (InputSourceKind, uint32, int, bool) {
	count := c.ResultAliasCount(op, outcome)
	left, right := 0, count
	for left < right {
		mid := left + (right-left)/2
		current, _, _, ok := c.ResultAliasAt(op, outcome, mid)
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
	current, kind, source, ok := c.ResultAliasAt(op, outcome, left)
	if !ok || current != result {
		return 0, 0, 0, false
	}
	return kind, source, left, true
}

func (c *Contract) ProducedCount(op Operation, outcome int) int {
	row, ok := c.operation(op)
	if !ok || outcome < 0 || outcome >= row.outcomes.len() {
		return 0
	}
	return c.outcomes[row.outcomes.start+uint32(outcome)].produced.len()
}

func (c *Contract) producedAt(op Operation, outcome, index int) (producedRow, bool) {
	row, ok := c.operation(op)
	if !ok || outcome < 0 || outcome >= row.outcomes.len() {
		return producedRow{}, false
	}
	produced := c.outcomes[row.outcomes.start+uint32(outcome)].produced
	if index < 0 || index >= produced.len() {
		return producedRow{}, false
	}
	return c.produced[produced.start+uint32(index)], true
}

func (c *Contract) ProducedAt(op Operation, outcome, index int) (uint32, Operation, bool) {
	row, ok := c.producedAt(op, outcome, index)
	if !ok {
		return 0, 0, false
	}
	return row.result, row.target, true
}

func (c *Contract) ProducedForResult(op Operation, outcome int, result uint32) (Operation, int, bool) {
	count := c.ProducedCount(op, outcome)
	left, right := 0, count
	for left < right {
		mid := left + (right-left)/2
		current, _, ok := c.ProducedAt(op, outcome, mid)
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
	current, target, ok := c.ProducedAt(op, outcome, left)
	if !ok || current != result {
		return 0, 0, false
	}
	return target, left, true
}

// FreshResultCount is the number of nominal result roots owned by one
// canonical outcome.
func (c *Contract) FreshResultCount(op Operation, outcome int) int {
	row, ok := c.operation(op)
	if !ok || outcome < 0 || outcome >= row.outcomes.len() {
		return 0
	}
	return c.outcomes[row.outcomes.start+uint32(outcome)].fresh.len()
}

func (c *Contract) freshResultAt(op Operation, outcome, index int) (freshResultRow, bool) {
	row, ok := c.operation(op)
	if !ok || outcome < 0 || outcome >= row.outcomes.len() {
		return freshResultRow{}, false
	}
	fresh := c.outcomes[row.outcomes.start+uint32(outcome)].fresh
	if index < 0 || index >= fresh.len() {
		return freshResultRow{}, false
	}
	return c.fresh[fresh.start+uint32(index)], true
}

// FreshResultAt returns one canonical fixed-result freshness relation. The
// ordinal is dense within this outcome and independent of authoring order.
func (c *Contract) FreshResultAt(op Operation, outcome, index int) (result, ordinal uint32, kind FreshKind, ok bool) {
	row, found := c.freshResultAt(op, outcome, index)
	if !found {
		return 0, 0, FreshInvalid, false
	}
	return row.result, row.ordinal, row.kind, true
}

// FreshResultForResult finds the nominal freshness relation for one fixed
// outcome result. The returned index is the canonical FreshResultAt index.
func (c *Contract) FreshResultForResult(op Operation, outcome int, result uint32) (ordinal uint32, kind FreshKind, index int, ok bool) {
	count := c.FreshResultCount(op, outcome)
	left, right := 0, count
	for left < right {
		mid := left + (right-left)/2
		current, _, _, found := c.FreshResultAt(op, outcome, mid)
		if !found {
			return 0, FreshInvalid, 0, false
		}
		if current < result {
			left = mid + 1
		} else {
			right = mid
		}
	}
	if left >= count {
		return 0, FreshInvalid, 0, false
	}
	current, ordinal, kind, found := c.FreshResultAt(op, outcome, left)
	if !found || current != result {
		return 0, FreshInvalid, 0, false
	}
	return ordinal, kind, left, true
}

func (c *Contract) ProducedCaptureCount(op Operation, outcome, produced int) int {
	row, ok := c.producedAt(op, outcome, produced)
	if !ok {
		return 0
	}
	return row.captures.len()
}

func (c *Contract) ProducedCaptureAt(op Operation, outcome, produced, capture int) (CaptureKind, uint32, bool) {
	row, ok := c.producedAt(op, outcome, produced)
	if !ok || capture < 0 || capture >= row.captures.len() {
		return 0, 0, false
	}
	value := c.captures[row.captures.start+uint32(capture)]
	return value.kind, value.ordinal, true
}

// ProducedTypeValueCapture returns the sole fixed input formal whose runtime
// TypeValue is retained by one Produced row. The relation is indexed when the
// contract seals; it neither scans captures nor infers TypeValue identity
// from the value/runtime-type spelling.
func (c *Contract) ProducedTypeValueCapture(op Operation, outcome, produced int) (ValueFormal, bool) {
	row, ok := c.producedAt(op, outcome, produced)
	if !ok || row.typeValueCapture == noTypeValueCapture || row.typeValueCapture >= uint32(row.captures.len()) {
		return 0, false
	}
	capture := c.captures[row.captures.start+row.typeValueCapture]
	if capture.kind != CaptureTypeValueFormal {
		return 0, false
	}
	return ValueFormal(capture.ordinal), true
}

// TypeBytes is deliberately cold: it returns an ownership-isolated copy of
// the frozen scoped canonical type bytes.
func (c *Contract) TypeBytes(typ Type) ([]byte, bool) {
	if c == nil || typ == 0 || uint64(typ) > uint64(len(c.types)) {
		return nil, false
	}
	return append([]byte(nil), c.types[uint32(typ)-1].bytes...), true
}

func (r indexRange) len() int { return int(r.end - r.start) }
