package target

import (
	"errors"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"sort"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// bootDraft is the complete Target-owned initial-root ledger before its
// immutable dense coordinates are appended to Contract.
type bootDraft struct {
	roots      []bootRootDraft
	values     []initialValueDraft
	entries    []initialEntryDraft
	bindings   []initialBindingDraft
	metatables []initialMetatableAttachmentDraft
	globalRoot vocabulary.InitialRoot
	absent     initialValueDraft
	hasAbsent  bool
}

type bootRootDraft struct {
	identity  string
	aggregate vocabulary.BootAggregate
	immutable bool
	value     initialValueDraft
}

type initialValueDraft struct {
	kind      vocabulary.InitialValueKind
	boolean   bool
	integer   int64
	floatBits uint64
	string    string
	root      vocabulary.InitialRoot
	operation vocabulary.Operation
	binding   vocabulary.BindingSpec
}

type initialEntryDraft struct {
	root       vocabulary.InitialRoot
	key        keyspace.LiteralValue
	value      initialValueDraft
	mutability vocabulary.InitialMutability
}

type initialBindingDraft struct {
	name string
	root vocabulary.InitialRoot
	key  keyspace.LiteralValue
}

type initialMetatableAttachmentDraft struct {
	base      vocabulary.InitialValueKind
	metatable vocabulary.InitialRoot
}

func freezeBoot(inputRoots []vocabulary.InitialRootSpec, inputEntries []vocabulary.InitialEntrySpec, inputBindings []vocabulary.InitialBindingSpec, inputMetatables []vocabulary.InitialMetatableAttachmentSpec, operations []operationDraft, sourceOperation []vocabulary.Operation) (bootDraft, error) {
	if len(inputRoots) == 0 {
		if len(inputEntries) != 0 || len(inputBindings) != 0 || len(inputMetatables) != 0 {
			return bootDraft{}, errors.New("target: initial ledger has rows without roots")
		}
		return bootDraft{}, nil
	}
	roots := make([]bootRootDraft, len(inputRoots))
	rootIndex := make(map[string]vocabulary.InitialRoot, len(inputRoots))
	for index, input := range inputRoots {
		if input.Identity == "" || input.Shape.Aggregate == vocabulary.BootAggregateInvalid || !validBootAggregate(input.Shape.Aggregate) {
			return bootDraft{}, errors.New("target: invalid initial root")
		}
		roots[index] = bootRootDraft{identity: input.Identity, aggregate: input.Shape.Aggregate, immutable: input.Shape.Immutable}
	}
	sort.Slice(roots, func(left, right int) bool { return roots[left].identity < roots[right].identity })
	for index := range roots {
		if index != 0 && roots[index-1].identity == roots[index].identity {
			return bootDraft{}, errors.New("target: duplicate initial root")
		}
		handle, err := checkedStoredHandle("initial root table", index)
		if err != nil {
			return bootDraft{}, err
		}
		rootIndex[roots[index].identity] = vocabulary.InitialRoot(handle)
	}
	for _, input := range inputRoots {
		root := rootIndex[input.Identity]
		position := int(root) - 1
		value, err := freezeInitialValue(input.Shape.Value, rootIndex, operations, sourceOperation)
		if err != nil {
			return bootDraft{}, err
		}
		if value.kind != vocabulary.InitialValueRoot || value.root != root {
			return bootDraft{}, errors.New("target: boot shape value must self-alias its initial root")
		}
		roots[position].value = value
	}

	entries := make([]initialEntryDraft, len(inputEntries))
	for index, input := range inputEntries {
		root, ok := rootIndex[input.Root]
		key, keyErr := normalizeRequiredExactKey(input.Key)
		if !ok || keyErr != nil || !validInitialMutability(input.Mutability) {
			return bootDraft{}, errors.New("target: invalid initial entry")
		}
		value, err := freezeInitialValue(input.Value, rootIndex, operations, sourceOperation)
		if err != nil {
			return bootDraft{}, err
		}
		entries[index] = initialEntryDraft{root: root, key: key, value: value, mutability: input.Mutability}
	}
	sort.Slice(entries, func(left, right int) bool { return compareInitialEntry(entries[left], entries[right]) < 0 })
	for index := 1; index < len(entries); index++ {
		if entries[index-1].root == entries[index].root && compareNormalizedKey(entries[index-1].key, entries[index].key) == 0 {
			return bootDraft{}, errors.New("target: duplicate initial entry")
		}
	}

	bindings := make([]initialBindingDraft, len(inputBindings))
	for index, input := range inputBindings {
		root, ok := rootIndex[input.Root]
		key, keyErr := normalizeRequiredExactKey(input.Key)
		if !ok || input.Name == "" || keyErr != nil {
			return bootDraft{}, errors.New("target: invalid initial binding")
		}
		entry, found := lookupInitialEntry(entries, root, key)
		if !found || initialBindingClassForValue(entry.value.kind) == vocabulary.InitialBindingInvalid {
			return bootDraft{}, errors.New("target: initial binding lacks matching ledger entry")
		}
		bindings[index] = initialBindingDraft{name: input.Name, root: root, key: key}
	}
	sort.Slice(bindings, func(left, right int) bool { return bindings[left].name < bindings[right].name })
	for index := 1; index < len(bindings); index++ {
		if bindings[index-1].name == bindings[index].name {
			return bootDraft{}, errors.New("target: duplicate initial binding")
		}
	}

	metatables := make([]initialMetatableAttachmentDraft, len(inputMetatables))
	for index, input := range inputMetatables {
		metatable, ok := rootIndex[input.Metatable]
		if !ok || input.Base != vocabulary.InitialValueString || roots[metatable-1].aggregate != vocabulary.BootAggregateMetatable {
			return bootDraft{}, errors.New("target: invalid initial metatable attachment")
		}
		metatables[index] = initialMetatableAttachmentDraft{base: input.Base, metatable: metatable}
	}
	sort.Slice(metatables, func(left, right int) bool { return metatables[left].base < metatables[right].base })
	for index := 1; index < len(metatables); index++ {
		if metatables[index-1].base == metatables[index].base {
			return bootDraft{}, errors.New("target: duplicate initial metatable attachment")
		}
	}

	values := make([]initialValueDraft, 0, len(roots)+len(entries))
	values = append(values, make([]initialValueDraft, len(roots))...)
	for index := range roots {
		values[index] = roots[index].value
	}
	for _, entry := range entries {
		values = append(values, entry.value)
	}
	sort.Slice(values, func(left, right int) bool { return compareInitialValue(values[left], values[right]) < 0 })
	unique := values[:0]
	for _, value := range values {
		if len(unique) == 0 || compareInitialValue(unique[len(unique)-1], value) != 0 {
			unique = append(unique, value)
		}
	}
	var absent initialValueDraft
	for _, value := range unique {
		if value.kind == vocabulary.InitialValueAbsent {
			absent = value
			break
		}
	}
	hasAbsent := absent.kind == vocabulary.InitialValueAbsent
	var globalRoot vocabulary.InitialRoot
	if len(bindings) != 0 {
		globalRoot = bindings[0].root
		if roots[uint32(globalRoot)-1].aggregate != vocabulary.BootAggregateTable {
			return bootDraft{}, errors.New("target: global environment root is not a table")
		}
		if !hasAbsent {
			return bootDraft{}, errors.New("target: global bindings require an absent initial value")
		}
		for _, binding := range bindings {
			key, keyOK := exactString(binding.key)
			if !keyOK || binding.name != key || binding.root != globalRoot {
				return bootDraft{}, errors.New("target: invalid global binding root or slot")
			}
			entry, ok := lookupInitialEntry(entries, binding.root, binding.key)
			if !ok || entry.mutability != vocabulary.InitialMutable {
				return bootDraft{}, errors.New("target: global binding must be initially mutable")
			}
		}
		var globalAlias initialBindingDraft
		hasGlobalAlias := false
		for _, binding := range bindings {
			if binding.name == "_G" {
				globalAlias = binding
				hasGlobalAlias = true
				break
			}
		}
		globalKey, globalKeyOK := exactString(globalAlias.key)
		if !hasGlobalAlias || globalAlias.root != globalRoot || !globalKeyOK || globalKey != "_G" {
			return bootDraft{}, errors.New("target: global bindings require a _G self-alias binding")
		}
		entry, ok := lookupInitialEntry(entries, globalRoot, keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"})
		if !ok || entry.mutability != vocabulary.InitialMutable || entry.value.kind != vocabulary.InitialValueRoot || entry.value.root != globalRoot {
			return bootDraft{}, errors.New("target: global bindings require a mutable _G self-alias")
		}
	}
	return bootDraft{roots: roots, values: unique, entries: entries, bindings: bindings, metatables: metatables, globalRoot: globalRoot, absent: absent, hasAbsent: hasAbsent}, nil
}

func freezeInitialValue(input vocabulary.InitialValueSpec, roots map[string]vocabulary.InitialRoot, operations []operationDraft, sourceOperation []vocabulary.Operation) (initialValueDraft, error) {
	switch input.Kind {
	case vocabulary.InitialValueNil:
		if input.Boolean || input.Integer != 0 || input.FloatBits != 0 || input.String != "" || input.Root != "" || !emptyBinding(input.Operation) {
			return initialValueDraft{}, errors.New("target: invalid nil initial value")
		}
		return initialValueDraft{kind: input.Kind}, nil
	case vocabulary.InitialValueBoolean:
		if input.Integer != 0 || input.FloatBits != 0 || input.String != "" || input.Root != "" || !emptyBinding(input.Operation) {
			return initialValueDraft{}, errors.New("target: invalid boolean initial value")
		}
		return initialValueDraft{kind: input.Kind, boolean: input.Boolean}, nil
	case vocabulary.InitialValueInteger:
		if input.Boolean || input.FloatBits != 0 || input.String != "" || input.Root != "" || !emptyBinding(input.Operation) {
			return initialValueDraft{}, errors.New("target: invalid integer initial value")
		}
		return initialValueDraft{kind: input.Kind, integer: input.Integer}, nil
	case vocabulary.InitialValueFloat:
		if input.Boolean || input.Integer != 0 || input.String != "" || input.Root != "" || !emptyBinding(input.Operation) {
			return initialValueDraft{}, errors.New("target: invalid float initial value")
		}
		return initialValueDraft{kind: input.Kind, floatBits: input.FloatBits}, nil
	case vocabulary.InitialValueString:
		if input.Boolean || input.Integer != 0 || input.FloatBits != 0 || input.Root != "" || !emptyBinding(input.Operation) {
			return initialValueDraft{}, errors.New("target: invalid string initial value")
		}
		return initialValueDraft{kind: input.Kind, string: input.String}, nil
	case vocabulary.InitialValueRoot:
		if input.Root == "" || input.Boolean || input.Integer != 0 || input.FloatBits != 0 || input.String != "" || !emptyBinding(input.Operation) {
			return initialValueDraft{}, errors.New("target: invalid root initial value")
		}
		root, ok := roots[input.Root]
		if !ok {
			return initialValueDraft{}, errors.New("target: foreign initial value root")
		}
		return initialValueDraft{kind: input.Kind, root: root}, nil
	case vocabulary.InitialValueOperation:
		if input.Boolean || input.Integer != 0 || input.FloatBits != 0 || input.String != "" || input.Root != "" || !vocabulary.ValidBinding(input.Operation) {
			return initialValueDraft{}, errors.New("target: invalid operation initial value")
		}
		operation, ok := operationForBinding(operations, sourceOperation, input.Operation)
		if !ok {
			return initialValueDraft{}, errors.New("target: foreign initial operation")
		}
		return initialValueDraft{kind: input.Kind, operation: operation}, nil
	case vocabulary.InitialValueDeniedOperation:
		if input.Boolean || input.Integer != 0 || input.FloatBits != 0 || input.String != "" || input.Root != "" || !vocabulary.ValidBinding(input.Operation) {
			return initialValueDraft{}, errors.New("target: invalid denied initial operation")
		}
		if _, admitted := operationForBinding(operations, sourceOperation, input.Operation); admitted {
			return initialValueDraft{}, errors.New("target: denied initial operation is admitted")
		}
		return initialValueDraft{kind: input.Kind, binding: cloneBinding(input.Operation)}, nil
	case vocabulary.InitialValueAbsent:
		if input.Boolean || input.Integer != 0 || input.FloatBits != 0 || input.String != "" || input.Root != "" || !emptyBinding(input.Operation) {
			return initialValueDraft{}, errors.New("target: invalid absent initial value")
		}
		return initialValueDraft{kind: input.Kind}, nil
	default:
		return initialValueDraft{}, errors.New("target: invalid initial value kind")
	}
}

func operationForBinding(operations []operationDraft, sourceOperation []vocabulary.Operation, binding vocabulary.BindingSpec) (vocabulary.Operation, bool) {
	for index := range operations {
		for _, candidate := range operations[index].bindings {
			if compareBinding(candidate, binding) == 0 {
				operation := sourceOperation[operations[index].source]
				return operation, operation != 0
			}
		}
	}
	return 0, false
}

func (c *Contract) appendBoot(input bootDraft, keys map[keyspace.LiteralValue]vocabulary.ExactKey) error {
	if len(input.roots) == 0 {
		return nil
	}
	if _, err := checkedStoredRange("initial root table", len(c.initialRoots), len(input.roots)); err != nil {
		return err
	}
	if _, err := checkedStoredRange("boot shape table", len(c.bootShapes), len(input.roots)); err != nil {
		return err
	}
	if _, err := checkedStoredRange("initial value table", len(c.initialValues), len(input.values)); err != nil {
		return err
	}
	for _, value := range input.values {
		if _, err := checkedStoredHandle("initial value table", len(c.initialValues)); err != nil {
			return err
		}
		row := initialValueRow{kind: value.kind, boolean: value.boolean, integer: value.integer, floatBits: value.floatBits, string: value.string, root: value.root, operation: value.operation}
		if value.kind == vocabulary.InitialValueDeniedOperation {
			binding, bindErr := c.appendInitialValueBinding(value.binding, keys)
			if bindErr != nil {
				return bindErr
			}
			row.binding = binding
		}
		c.initialValues = append(c.initialValues, row)
	}
	for index, root := range input.roots {
		handle, err := checkedStoredHandle("initial root table", len(c.initialRoots))
		if err != nil {
			return err
		}
		shape, shapeErr := checkedStoredHandle("boot shape table", len(c.bootShapes))
		if shapeErr != nil {
			return shapeErr
		}
		value, ok := initialValueHandle(input.values, root.value)
		if !ok {
			return errors.New("target: unresolved boot shape value")
		}
		if int(handle) != index+1 || int(shape) != index+1 {
			return errors.New("target: noncanonical boot identity")
		}
		c.initialRoots = append(c.initialRoots, initialRootRow{identity: root.identity, shape: vocabulary.BootShape(shape)})
		c.bootShapes = append(c.bootShapes, bootShapeRow{root: vocabulary.InitialRoot(handle), aggregate: root.aggregate, immutable: root.immutable, value: value})
	}
	if _, err := checkedStoredRange("initial entry table", len(c.initialEntries), len(input.entries)); err != nil {
		return err
	}
	for _, entry := range input.entries {
		value, ok := initialValueHandle(input.values, entry.value)
		if !ok {
			return errors.New("target: unresolved initial entry value")
		}
		key, keyErr := exactKeyHandle(keys, entry.key)
		if keyErr != nil {
			return keyErr
		}
		c.initialEntries = append(c.initialEntries, initialEntryRow{root: entry.root, key: key, value: value, mutability: entry.mutability})
	}
	if _, err := checkedStoredRange("initial binding table", len(c.initialBindings), len(input.bindings)); err != nil {
		return err
	}
	if _, err := checkedStoredRange("initial metatable attachment table", len(c.initialMetatables), len(input.metatables)); err != nil {
		return err
	}
	for _, attachment := range input.metatables {
		if attachment.base != vocabulary.InitialValueString || attachment.metatable == 0 || uint64(attachment.metatable) > uint64(len(c.initialRoots)) {
			return errors.New("target: malformed initial metatable attachment")
		}
		c.initialMetatables = append(c.initialMetatables, initialMetatableAttachmentRow(attachment))
	}
	for _, binding := range input.bindings {
		key, keyErr := exactKeyHandle(keys, binding.key)
		if keyErr != nil {
			return keyErr
		}
		c.initialBindings = append(c.initialBindings, initialBindingRow{name: binding.name, root: binding.root, key: key})
	}
	c.globalEnvRoot = input.globalRoot
	if input.hasAbsent {
		absent, ok := initialValueHandle(input.values, input.absent)
		if !ok {
			return errors.New("target: unresolved initial absent value")
		}
		c.initialAbsent = absent
	}
	return nil
}

func validBootAggregate(value vocabulary.BootAggregate) bool {
	return value == vocabulary.BootAggregateTable || value == vocabulary.BootAggregateMetatable
}

func validInitialMutability(value vocabulary.InitialMutability) bool {
	return value == vocabulary.InitialMutable || value == vocabulary.InitialFrozen
}

func emptyBinding(value vocabulary.BindingSpec) bool {
	return value.Namespace == 0 && len(value.Owner) == 0 && len(value.Member) == 0
}

func compareInitialEntry(left, right initialEntryDraft) int {
	if left.root < right.root {
		return -1
	}
	if left.root > right.root {
		return 1
	}
	return compareNormalizedKey(left.key, right.key)
}

func lookupInitialEntry(entries []initialEntryDraft, root vocabulary.InitialRoot, key keyspace.LiteralValue) (initialEntryDraft, bool) {
	index := sort.Search(len(entries), func(index int) bool {
		entry := entries[index]
		return entry.root > root || (entry.root == root && compareNormalizedKey(entry.key, key) >= 0)
	})
	if index == len(entries) || entries[index].root != root || compareNormalizedKey(entries[index].key, key) != 0 {
		return initialEntryDraft{}, false
	}
	return entries[index], true
}

func exactString(value keyspace.LiteralValue) (string, bool) {
	if value.Kind != keyspace.LiteralString {
		return "", false
	}
	return value.String, true
}

func compareInitialValue(left, right initialValueDraft) int {
	if left.kind < right.kind {
		return -1
	}
	if left.kind > right.kind {
		return 1
	}
	switch left.kind {
	case vocabulary.InitialValueBoolean:
		if !left.boolean && right.boolean {
			return -1
		}
		if left.boolean && !right.boolean {
			return 1
		}
	case vocabulary.InitialValueInteger:
		if left.integer < right.integer {
			return -1
		}
		if left.integer > right.integer {
			return 1
		}
	case vocabulary.InitialValueFloat:
		if left.floatBits < right.floatBits {
			return -1
		}
		if left.floatBits > right.floatBits {
			return 1
		}
	case vocabulary.InitialValueString:
		if left.string < right.string {
			return -1
		}
		if left.string > right.string {
			return 1
		}
	case vocabulary.InitialValueRoot:
		if left.root < right.root {
			return -1
		}
		if left.root > right.root {
			return 1
		}
	case vocabulary.InitialValueOperation:
		if left.operation < right.operation {
			return -1
		}
		if left.operation > right.operation {
			return 1
		}
	case vocabulary.InitialValueDeniedOperation:
		return compareBinding(left.binding, right.binding)
	}
	return 0
}

func initialValueHandle(values []initialValueDraft, want initialValueDraft) (vocabulary.InitialValue, bool) {
	index := sort.Search(len(values), func(index int) bool { return compareInitialValue(values[index], want) >= 0 })
	if index == len(values) || compareInitialValue(values[index], want) != 0 {
		return 0, false
	}
	return vocabulary.InitialValue(index + 1), true
}
