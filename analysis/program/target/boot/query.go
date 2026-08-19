package boot

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// InitialRootCount returns every sealed boot aggregate.
func (t *Table) InitialRootCount() int {
	if t == nil {
		return 0
	}
	return t.roots.Count()
}

func (t *Table) InitialRootAt(index int) (vocabulary.InitialRoot, bool) {
	if t == nil || index < 0 || index >= t.roots.Count() {
		return 0, false
	}
	_, ok := t.roots.At(index)
	if !ok {
		return 0, false
	}
	return vocabulary.InitialRoot(index + 1), true
}

func (t *Table) InitialRootIdentity(root vocabulary.InitialRoot) (string, bool) {
	row, ok := t.root(root)
	if !ok {
		return "", false
	}
	return row.identity, true
}

// InitialRootByIdentity resolves the canonical root identity without exposing
// the owner's row storage to a consumer.
func (t *Table) InitialRootByIdentity(identity string) (vocabulary.InitialRoot, bool) {
	if t == nil || identity == "" {
		return 0, false
	}
	index := sort.Search(t.roots.Count(), func(index int) bool {
		row, ok := t.roots.At(index)
		return ok && row.identity >= identity
	})
	row, ok := t.roots.At(index)
	if !ok || row.identity != identity {
		return 0, false
	}
	return vocabulary.InitialRoot(index + 1), true
}

func (t *Table) InitialRootBootShape(root vocabulary.InitialRoot) (vocabulary.BootShape, bool) {
	row, ok := t.root(root)
	if !ok {
		return 0, false
	}
	return row.shape, true
}

func (t *Table) bootShape(shape vocabulary.BootShape) (shapeRow, bool) {
	if t == nil || shape == 0 || uint64(shape) > uint64(t.shapes.Count()) {
		return shapeRow{}, false
	}
	row, ok := t.shapes.At(int(shape - 1))
	return row, ok
}

func (t *Table) root(root vocabulary.InitialRoot) (rootRow, bool) {
	if t == nil || root == 0 || uint64(root) > uint64(t.roots.Count()) {
		return rootRow{}, false
	}
	row, ok := t.roots.At(int(root - 1))
	return row, ok
}

func (t *Table) bootShapeRoot(shape vocabulary.BootShape) (vocabulary.InitialRoot, bool) {
	row, ok := t.bootShape(shape)
	if !ok {
		return 0, false
	}
	return row.root, true
}

func (t *Table) BootShapeAggregate(shape vocabulary.BootShape) (vocabulary.BootAggregate, bool) {
	row, ok := t.bootShape(shape)
	if !ok {
		return 0, false
	}
	return row.aggregate, true
}

func (t *Table) BootShapeImmutable(shape vocabulary.BootShape) (bool, bool) {
	row, ok := t.bootShape(shape)
	if !ok {
		return false, false
	}
	return row.immutable, true
}

func (t *Table) BootShapeValue(shape vocabulary.BootShape) (vocabulary.InitialValue, bool) {
	row, ok := t.bootShape(shape)
	if !ok {
		return 0, false
	}
	return row.value, true
}

func (t *Table) GlobalEnvRoot() (vocabulary.InitialRoot, bool) {
	if t == nil || t.globalRoot == 0 {
		return 0, false
	}
	return t.globalRoot, true
}

func (t *Table) InitialAbsent() (vocabulary.InitialValue, bool) {
	if t == nil || t.absent == 0 {
		return 0, false
	}
	return t.absent, true
}

func (t *Table) initialValue(value vocabulary.InitialValue) (valueRow, bool) {
	if t == nil || value == 0 || uint64(value) > uint64(t.values.Count()) {
		return valueRow{}, false
	}
	row, ok := t.values.At(int(value - 1))
	return row, ok
}

func (t *Table) InitialValueKind(value vocabulary.InitialValue) (vocabulary.InitialValueKind, bool) {
	row, ok := t.initialValue(value)
	if !ok {
		return 0, false
	}
	return row.kind, true
}

func (t *Table) InitialValueBoolean(value vocabulary.InitialValue) (bool, bool) {
	row, ok := t.initialValue(value)
	return row.boolean, ok && row.kind == vocabulary.InitialValueBoolean
}

func (t *Table) InitialValueInteger(value vocabulary.InitialValue) (int64, bool) {
	row, ok := t.initialValue(value)
	return row.integer, ok && row.kind == vocabulary.InitialValueInteger
}

func (t *Table) InitialValueFloatBits(value vocabulary.InitialValue) (uint64, bool) {
	row, ok := t.initialValue(value)
	return row.floatBits, ok && row.kind == vocabulary.InitialValueFloat
}

func (t *Table) InitialValueString(value vocabulary.InitialValue) (string, bool) {
	row, ok := t.initialValue(value)
	return row.string, ok && row.kind == vocabulary.InitialValueString
}

func (t *Table) InitialValueRoot(value vocabulary.InitialValue) (vocabulary.InitialRoot, bool) {
	row, ok := t.initialValue(value)
	return row.root, ok && row.kind == vocabulary.InitialValueRoot
}

func (t *Table) InitialValueOperation(value vocabulary.InitialValue) (vocabulary.Operation, bool) {
	row, ok := t.initialValue(value)
	return row.operation, ok && row.kind == vocabulary.InitialValueOperation
}

func (t *Table) initialValueBinding(value vocabulary.InitialValue) (bindingRange, bool) {
	row, ok := t.initialValue(value)
	if !ok || row.kind != vocabulary.InitialValueDeniedOperation || row.binding == 0 || uint64(row.binding) > uint64(t.valueBinds.Count()) {
		return bindingRange{}, false
	}
	binding, ok := t.valueBinds.At(int(row.binding - 1))
	return binding, ok
}

func (t *Table) InitialValueDeniedNamespace(value vocabulary.InitialValue) (vocabulary.BindingNamespace, bool) {
	row, ok := t.initialValueBinding(value)
	return row.namespace, ok
}

func (t *Table) InitialValueDeniedOwnerCount(value vocabulary.InitialValue) int {
	row, ok := t.initialValueBinding(value)
	if !ok {
		return 0
	}
	return row.owner.Len()
}

func (t *Table) InitialValueDeniedOwnerAt(value vocabulary.InitialValue, index int) (string, bool) {
	row, rowOK := t.initialValueBinding(value)
	if !rowOK {
		return "", false
	}
	key, ok := t.initialValueDeniedOwnerKeyAt(value, index)
	if !ok || key == 0 {
		return "", false
	}
	segment, segmentOK := t.segments.At(row.owner, index)
	if !segmentOK {
		return "", false
	}
	return segment, true
}

func (t *Table) initialValueDeniedOwnerKeyAt(value vocabulary.InitialValue, index int) (vocabulary.ExactKey, bool) {
	row, ok := t.initialValueBinding(value)
	if !ok || index < 0 || index >= row.ownerKeys.Len() {
		return 0, false
	}
	return t.bindingKeys.At(row.ownerKeys, index)
}

func (t *Table) InitialValueDeniedMemberCount(value vocabulary.InitialValue) int {
	row, ok := t.initialValueBinding(value)
	if !ok {
		return 0
	}
	return row.member.Len()
}

func (t *Table) InitialValueDeniedMemberAt(value vocabulary.InitialValue, index int) (string, bool) {
	row, rowOK := t.initialValueBinding(value)
	if !rowOK {
		return "", false
	}
	key, ok := t.initialValueDeniedMemberKeyAt(value, index)
	if !ok || key == 0 {
		return "", false
	}
	segment, segmentOK := t.segments.At(row.member, index)
	if !segmentOK {
		return "", false
	}
	return segment, true
}

func (t *Table) initialValueDeniedMemberKeyAt(value vocabulary.InitialValue, index int) (vocabulary.ExactKey, bool) {
	row, ok := t.initialValueBinding(value)
	if !ok || index < 0 || index >= row.memberKeys.Len() {
		return 0, false
	}
	return t.bindingKeys.At(row.memberKeys, index)
}

func (t *Table) InitialEntryCount() int {
	if t == nil {
		return 0
	}
	return t.entries.Count()
}

func (t *Table) InitialEntryAt(index int) (vocabulary.InitialRoot, vocabulary.ExactKey, vocabulary.InitialValue, vocabulary.InitialMutability, bool) {
	if t == nil || index < 0 || index >= t.entries.Count() {
		return 0, 0, 0, 0, false
	}
	row, ok := t.entries.At(index)
	if !ok {
		return 0, 0, 0, 0, false
	}
	return row.root, row.key, row.value, row.mutability, true
}

// InitialEntry performs an allocation-free binary search over the canonical
// root/key directory.
func (t *Table) InitialEntry(root vocabulary.InitialRoot, key vocabulary.ExactKey) (vocabulary.InitialValue, vocabulary.InitialMutability, bool) {
	if t == nil || root == 0 || key == 0 {
		return 0, 0, false
	}
	index := sort.Search(t.entries.Count(), func(index int) bool {
		row, ok := t.entries.At(index)
		if !ok {
			return false
		}
		return row.root > root || (row.root == root && row.key >= key)
	})
	row, ok := t.entries.At(index)
	if !ok || row.root != root || row.key != key {
		return 0, 0, false
	}
	return row.value, row.mutability, true
}

func (t *Table) InitialOperation(root vocabulary.InitialRoot, key vocabulary.ExactKey) (vocabulary.Operation, bool) {
	value, _, ok := t.InitialEntry(root, key)
	if !ok {
		return 0, false
	}
	return t.InitialValueOperation(value)
}

func (t *Table) InitialBindingCount() int {
	if t == nil {
		return 0
	}
	return t.bindings.Count()
}

func (t *Table) InitialBindingAt(index int) (string, vocabulary.InitialBindingClass, vocabulary.InitialValue, vocabulary.InitialRoot, vocabulary.ExactKey, bool) {
	if t == nil || index < 0 || index >= t.bindings.Count() {
		return "", 0, 0, 0, 0, false
	}
	row, ok := t.bindings.At(index)
	if !ok {
		return "", 0, 0, 0, 0, false
	}
	value, _, ok := t.InitialEntry(row.root, row.key)
	if !ok {
		return "", 0, 0, 0, 0, false
	}
	kind, ok := t.InitialValueKind(value)
	if !ok {
		return "", 0, 0, 0, 0, false
	}
	return row.name, initialBindingClass(kind), value, row.root, row.key, true
}

func (t *Table) InitialBinding(name string) (vocabulary.InitialBindingClass, vocabulary.InitialValue, vocabulary.InitialRoot, vocabulary.ExactKey, bool) {
	if t == nil || name == "" {
		return 0, 0, 0, 0, false
	}
	index := sort.Search(t.bindings.Count(), func(index int) bool {
		row, ok := t.bindings.At(index)
		return ok && row.name >= name
	})
	row, ok := t.bindings.At(index)
	if !ok || row.name != name {
		return 0, 0, 0, 0, false
	}
	value, _, ok := t.InitialEntry(row.root, row.key)
	if !ok {
		return 0, 0, 0, 0, false
	}
	kind, ok := t.InitialValueKind(value)
	if !ok {
		return 0, 0, 0, 0, false
	}
	return initialBindingClass(kind), value, row.root, row.key, true
}

func (t *Table) InitialMetatableAttachmentCount() int {
	if t == nil {
		return 0
	}
	return t.metatables.Count()
}

func (t *Table) InitialMetatableAttachmentAt(index int) (base vocabulary.InitialValueKind, metatable vocabulary.InitialRoot, ok bool) {
	if t == nil || index < 0 || index >= t.metatables.Count() {
		return 0, 0, false
	}
	row, ok := t.metatables.At(index)
	if !ok {
		return 0, 0, false
	}
	shape, shapeOK := t.InitialRootBootShape(row.metatable)
	aggregate, aggregateOK := t.BootShapeAggregate(shape)
	if row.base != vocabulary.InitialValueString || row.metatable == 0 || !shapeOK || !aggregateOK || aggregate != vocabulary.BootAggregateMetatable {
		return 0, 0, false
	}
	return row.base, row.metatable, true
}
