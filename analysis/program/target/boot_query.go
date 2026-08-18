package target

import (
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"sort"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
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
func (c *Contract) GlobalEnvRoot() (vocabulary.InitialRoot, bool) {
	if c == nil || c.globalEnvRoot == 0 {
		return 0, false
	}
	return c.globalEnvRoot, true
}

// InitialAbsent returns the unique canonical structural absent value. It is
// the total initial value for arbitrary Program globals and is unavailable
// only when the sealed ledger did not author an absent value.
func (c *Contract) InitialAbsent() (vocabulary.InitialValue, bool) {
	if c == nil || c.initialAbsent == 0 {
		return 0, false
	}
	return c.initialAbsent, true
}

func (c *Contract) InitialRootAt(index int) (vocabulary.InitialRoot, bool) {
	if c == nil || index < 0 || index >= len(c.initialRoots) {
		return 0, false
	}
	return vocabulary.InitialRoot(index + 1), true
}

func (c *Contract) initialRoot(root vocabulary.InitialRoot) (initialRootRow, bool) {
	if c == nil || root == 0 || uint64(root) > uint64(len(c.initialRoots)) {
		return initialRootRow{}, false
	}
	return c.initialRoots[uint32(root)-1], true
}

func (c *Contract) InitialRootIdentity(root vocabulary.InitialRoot) (string, bool) {
	row, ok := c.initialRoot(root)
	return row.identity, ok
}

func (c *Contract) InitialRootBootShape(root vocabulary.InitialRoot) (vocabulary.BootShape, bool) {
	row, ok := c.initialRoot(root)
	return row.shape, ok
}

func (c *Contract) bootShape(shape vocabulary.BootShape) (bootShapeRow, bool) {
	if c == nil || shape == 0 || uint64(shape) > uint64(len(c.bootShapes)) {
		return bootShapeRow{}, false
	}
	return c.bootShapes[uint32(shape)-1], true
}

func (c *Contract) bootShapeRoot(shape vocabulary.BootShape) (vocabulary.InitialRoot, bool) {
	row, ok := c.bootShape(shape)
	return row.root, ok
}

func (c *Contract) BootShapeAggregate(shape vocabulary.BootShape) (vocabulary.BootAggregate, bool) {
	row, ok := c.bootShape(shape)
	return row.aggregate, ok
}

// BootShapeImmutable reports the exact initial whole-object header policy of
// one boot aggregate.  It is not derived from, and has no bearing on the
// independent InitialMutability policies of its individual entries.
func (c *Contract) BootShapeImmutable(shape vocabulary.BootShape) (bool, bool) {
	row, ok := c.bootShape(shape)
	return row.immutable, ok
}

func (c *Contract) BootShapeValue(shape vocabulary.BootShape) (vocabulary.InitialValue, bool) {
	row, ok := c.bootShape(shape)
	return row.value, ok
}

func (c *Contract) initialValue(value vocabulary.InitialValue) (initialValueRow, bool) {
	if c == nil || value == 0 || uint64(value) > uint64(len(c.initialValues)) {
		return initialValueRow{}, false
	}
	return c.initialValues[uint32(value)-1], true
}

func (c *Contract) InitialValueKind(value vocabulary.InitialValue) (vocabulary.InitialValueKind, bool) {
	row, ok := c.initialValue(value)
	return row.kind, ok
}

func (c *Contract) InitialValueBoolean(value vocabulary.InitialValue) (bool, bool) {
	row, ok := c.initialValue(value)
	return row.boolean, ok && row.kind == vocabulary.InitialValueBoolean
}

func (c *Contract) InitialValueInteger(value vocabulary.InitialValue) (int64, bool) {
	row, ok := c.initialValue(value)
	return row.integer, ok && row.kind == vocabulary.InitialValueInteger
}

func (c *Contract) InitialValueFloatBits(value vocabulary.InitialValue) (uint64, bool) {
	row, ok := c.initialValue(value)
	return row.floatBits, ok && row.kind == vocabulary.InitialValueFloat
}

func (c *Contract) InitialValueString(value vocabulary.InitialValue) (string, bool) {
	row, ok := c.initialValue(value)
	return row.string, ok && row.kind == vocabulary.InitialValueString
}

func (c *Contract) InitialValueRoot(value vocabulary.InitialValue) (vocabulary.InitialRoot, bool) {
	row, ok := c.initialValue(value)
	return row.root, ok && row.kind == vocabulary.InitialValueRoot
}

func (c *Contract) InitialValueOperation(value vocabulary.InitialValue) (vocabulary.Operation, bool) {
	row, ok := c.initialValue(value)
	return row.operation, ok && row.kind == vocabulary.InitialValueOperation
}

// InitialOperation resolves one exact boot root/key occurrence directly to
// its admitted Target operation.  InitialEntry owns the canonical root/key
// binary-search index; this reduction deliberately does not walk the exact
// key or initial-entry tables and does not admit roots, literals, denied
// bindings, or other initial-value kinds as operations.
func (c *Contract) InitialOperation(root vocabulary.InitialRoot, key vocabulary.ExactKey) (vocabulary.Operation, bool) {
	value, _, ok := c.InitialEntry(root, key)
	if !ok {
		return 0, false
	}
	return c.InitialValueOperation(value)
}

func (c *Contract) initialValueBinding(value vocabulary.InitialValue) (bindingRange, bool) {
	row, ok := c.initialValue(value)
	if !ok || row.kind != vocabulary.InitialValueDeniedOperation || row.binding == 0 || uint64(row.binding) > uint64(len(c.initialValueBinds)) {
		return bindingRange{}, false
	}
	return c.initialValueBinds[row.binding-1], true
}

func (c *Contract) InitialValueDeniedNamespace(value vocabulary.InitialValue) (vocabulary.BindingNamespace, bool) {
	row, ok := c.initialValueBinding(value)
	return row.namespace, ok
}

func (c *Contract) InitialValueDeniedOwnerCount(value vocabulary.InitialValue) int {
	row, ok := c.initialValueBinding(value)
	if !ok {
		return 0
	}
	return row.owner.len()
}

func (c *Contract) InitialValueDeniedOwnerAt(value vocabulary.InitialValue, index int) (string, bool) {
	row, ok := c.initialValueBinding(value)
	if !ok || index < 0 || index >= row.owner.len() {
		return "", false
	}
	return c.segments[row.owner.start+uint32(index)], true
}

// initialValueDeniedOwnerKeyAt returns the exact-key atom for a denied
// binding owner segment. The string projection is artifact spelling only.
func (c *Contract) initialValueDeniedOwnerKeyAt(value vocabulary.InitialValue, index int) (vocabulary.ExactKey, bool) {
	row, ok := c.initialValueBinding(value)
	if !ok || index < 0 || index >= row.ownerKeys.len() {
		return 0, false
	}
	return c.bindingKeys[row.ownerKeys.start+uint32(index)], true
}

func (c *Contract) InitialValueDeniedMemberCount(value vocabulary.InitialValue) int {
	row, ok := c.initialValueBinding(value)
	if !ok {
		return 0
	}
	return row.member.len()
}

func (c *Contract) InitialValueDeniedMemberAt(value vocabulary.InitialValue, index int) (string, bool) {
	row, ok := c.initialValueBinding(value)
	if !ok || index < 0 || index >= row.member.len() {
		return "", false
	}
	return c.segments[row.member.start+uint32(index)], true
}

// initialValueDeniedMemberKeyAt returns the exact-key atom for a denied
// binding member segment. The string projection is artifact spelling only.
func (c *Contract) initialValueDeniedMemberKeyAt(value vocabulary.InitialValue, index int) (vocabulary.ExactKey, bool) {
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

func (c *Contract) ExactKeyAt(index int) (vocabulary.ExactKey, bool) {
	if c == nil || index < 0 || index >= len(c.exactKeys) {
		return 0, false
	}
	return vocabulary.ExactKey(index + 1), true
}

// ExactKeyValue returns the normalized typed Lua key payload for one sealed
// Target handle. It is the only spelling/payload projection for hot key rows.
func (c *Contract) ExactKeyValue(key vocabulary.ExactKey) (keyspace.LiteralValue, bool) {
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

func (c *Contract) InitialEntryAt(index int) (vocabulary.InitialRoot, vocabulary.ExactKey, vocabulary.InitialValue, vocabulary.InitialMutability, bool) {
	if c == nil || index < 0 || index >= len(c.initialEntries) {
		return 0, 0, 0, 0, false
	}
	row := c.initialEntries[index]
	return row.root, row.key, row.value, row.mutability, true
}

// InitialEntry performs an allocation-free binary search over canonical
// root/key rows.
func (c *Contract) InitialEntry(root vocabulary.InitialRoot, key vocabulary.ExactKey) (vocabulary.InitialValue, vocabulary.InitialMutability, bool) {
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
func (c *Contract) InitialMetatableAttachmentAt(index int) (base vocabulary.InitialValueKind, metatable vocabulary.InitialRoot, ok bool) {
	if c == nil || index < 0 || index >= len(c.initialMetatables) {
		return vocabulary.InitialValueInvalid, 0, false
	}
	row := c.initialMetatables[index]
	if row.base != vocabulary.InitialValueString || row.metatable == 0 || uint64(row.metatable) > uint64(len(c.initialRoots)) {
		return vocabulary.InitialValueInvalid, 0, false
	}
	shape, valid := c.InitialRootBootShape(row.metatable)
	aggregate, aggregateOK := c.BootShapeAggregate(shape)
	if !valid || !aggregateOK || aggregate != vocabulary.BootAggregateMetatable {
		return vocabulary.InitialValueInvalid, 0, false
	}
	return row.base, row.metatable, true
}

func (c *Contract) InitialBindingCount() int {
	if c == nil {
		return 0
	}
	return len(c.initialBindings)
}

func (c *Contract) InitialBindingAt(index int) (string, vocabulary.InitialBindingClass, vocabulary.InitialValue, vocabulary.InitialRoot, vocabulary.ExactKey, bool) {
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
func (c *Contract) InitialBinding(name string) (vocabulary.InitialBindingClass, vocabulary.InitialValue, vocabulary.InitialRoot, vocabulary.ExactKey, bool) {
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
