package boot

import (
	"errors"
	"sort"

	sealedrows "github.com/wippyai/go-lua/internal/rows"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/target/exactkey"
	"github.com/wippyai/go-lua/analysis/program/target/operation"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

type valueDraft struct {
	kind      vocabulary.InitialValueKind
	boolean   bool
	integer   int64
	floatBits uint64
	string    string
	root      vocabulary.InitialRoot
	operation vocabulary.Operation
	binding   vocabulary.BindingSpec
}

type rootDraft struct {
	identity  string
	aggregate vocabulary.BootAggregate
	immutable bool
	value     valueDraft
}

type entryDraft struct {
	root       vocabulary.InitialRoot
	key        keyspace.LiteralValue
	value      valueDraft
	mutability vocabulary.InitialMutability
}

type bindingDraft struct {
	name string
	root vocabulary.InitialRoot
	key  keyspace.LiteralValue
}

type metatableDraft struct {
	base      vocabulary.InitialValueKind
	metatable vocabulary.InitialRoot
}

type frozenLedger struct {
	roots       []rootRow
	shapes      []shapeRow
	values      []valueRow
	valueBinds  []bindingRange
	bindingKeys sealedrows.Pool[vocabulary.ExactKey]
	entries     []entryRow
	bindings    []bindingRow
	metatables  []metatableRow
	globalRoot  vocabulary.InitialRoot
	absent      vocabulary.InitialValue
}

func freezeLedger(input Input, keys exactkey.Table, operations operation.Core) (frozenLedger, error) {
	if len(input.InitialRoots) == 0 {
		if len(input.InitialEntries) != 0 || len(input.InitialBindings) != 0 || len(input.InitialMetatables) != 0 {
			return frozenLedger{}, errors.New("target/boot: initial ledger has rows without roots")
		}
		return frozenLedger{}, nil
	}
	roots := make([]rootDraft, len(input.InitialRoots))
	rootIndex := make(map[string]vocabulary.InitialRoot, len(input.InitialRoots))
	for index, item := range input.InitialRoots {
		if item.Identity == "" || !validAggregate(item.Shape.Aggregate) {
			return frozenLedger{}, errors.New("target/boot: invalid initial root")
		}
		roots[index] = rootDraft{identity: item.Identity, aggregate: item.Shape.Aggregate, immutable: item.Shape.Immutable}
	}
	sort.Slice(roots, func(left, right int) bool { return roots[left].identity < roots[right].identity })
	for index := range roots {
		if index != 0 && roots[index-1].identity == roots[index].identity {
			return frozenLedger{}, errors.New("target/boot: duplicate initial root")
		}
		handle, err := checkedHandle("initial root table", index)
		if err != nil {
			return frozenLedger{}, err
		}
		rootIndex[roots[index].identity] = vocabulary.InitialRoot(handle)
	}
	for _, item := range input.InitialRoots {
		root := rootIndex[item.Identity]
		value, err := freezeValue(item.Shape.Value, rootIndex, operations)
		if err != nil {
			return frozenLedger{}, err
		}
		if value.kind != vocabulary.InitialValueRoot || value.root != root {
			return frozenLedger{}, errors.New("target/boot: boot shape value must self-alias its initial root")
		}
		roots[uint32(root)-1].value = value
	}

	entries := make([]entryDraft, len(input.InitialEntries))
	for index, item := range input.InitialEntries {
		root, rootOK := rootIndex[item.Root]
		key, keyErr := normalizeKey(item.Key)
		if !rootOK || keyErr != nil || !validMutability(item.Mutability) {
			return frozenLedger{}, errors.New("target/boot: invalid initial entry")
		}
		value, err := freezeValue(item.Value, rootIndex, operations)
		if err != nil {
			return frozenLedger{}, err
		}
		entries[index] = entryDraft{root: root, key: key, value: value, mutability: item.Mutability}
	}
	sort.Slice(entries, func(left, right int) bool { return compareEntry(entries[left], entries[right]) < 0 })
	for index := 1; index < len(entries); index++ {
		if entries[index-1].root == entries[index].root && compareLiteral(entries[index-1].key, entries[index].key) == 0 {
			return frozenLedger{}, errors.New("target/boot: duplicate initial entry")
		}
	}

	bindings := make([]bindingDraft, len(input.InitialBindings))
	for index, item := range input.InitialBindings {
		root, rootOK := rootIndex[item.Root]
		key, keyErr := normalizeKey(item.Key)
		if !rootOK || item.Name == "" || keyErr != nil {
			return frozenLedger{}, errors.New("target/boot: invalid initial binding")
		}
		entry, found := lookupEntry(entries, root, key)
		if !found || initialBindingClass(entry.value.kind) == vocabulary.InitialBindingInvalid {
			return frozenLedger{}, errors.New("target/boot: initial binding lacks matching ledger entry")
		}
		bindings[index] = bindingDraft{name: item.Name, root: root, key: key}
	}
	sort.Slice(bindings, func(left, right int) bool { return bindings[left].name < bindings[right].name })
	for index := 1; index < len(bindings); index++ {
		if bindings[index-1].name == bindings[index].name {
			return frozenLedger{}, errors.New("target/boot: duplicate initial binding")
		}
	}

	metatables := make([]metatableDraft, len(input.InitialMetatables))
	for index, item := range input.InitialMetatables {
		metatable, rootOK := rootIndex[item.Metatable]
		if !rootOK || item.Base != vocabulary.InitialValueString || roots[uint32(metatable)-1].aggregate != vocabulary.BootAggregateMetatable {
			return frozenLedger{}, errors.New("target/boot: invalid initial metatable attachment")
		}
		metatables[index] = metatableDraft{base: item.Base, metatable: metatable}
	}
	sort.Slice(metatables, func(left, right int) bool { return metatables[left].base < metatables[right].base })
	for index := 1; index < len(metatables); index++ {
		if metatables[index-1].base == metatables[index].base {
			return frozenLedger{}, errors.New("target/boot: duplicate initial metatable attachment")
		}
	}

	allValues := make([]valueDraft, 0, len(roots)+len(entries))
	for _, root := range roots {
		allValues = append(allValues, root.value)
	}
	for _, entry := range entries {
		allValues = append(allValues, entry.value)
	}
	sort.Slice(allValues, func(left, right int) bool { return compareValue(allValues[left], allValues[right]) < 0 })
	unique := allValues[:0]
	for _, value := range allValues {
		if len(unique) == 0 || compareValue(unique[len(unique)-1], value) != 0 {
			unique = append(unique, value)
		}
	}
	var absent valueDraft
	for _, value := range unique {
		if value.kind == vocabulary.InitialValueAbsent {
			absent = value
			break
		}
	}
	var globalRoot vocabulary.InitialRoot
	if len(bindings) != 0 {
		globalRoot = bindings[0].root
		if roots[uint32(globalRoot)-1].aggregate != vocabulary.BootAggregateTable {
			return frozenLedger{}, errors.New("target/boot: global environment root is not a table")
		}
		if absent.kind != vocabulary.InitialValueAbsent {
			return frozenLedger{}, errors.New("target/boot: global bindings require an absent initial value")
		}
		for _, binding := range bindings {
			name, ok := exactString(binding.key)
			if !ok || binding.name != name || binding.root != globalRoot {
				return frozenLedger{}, errors.New("target/boot: invalid global binding root or slot")
			}
			entry, ok := lookupEntry(entries, binding.root, binding.key)
			if !ok || entry.mutability != vocabulary.InitialMutable {
				return frozenLedger{}, errors.New("target/boot: global binding must be initially mutable")
			}
		}
		var alias bindingDraft
		foundAlias := false
		for _, binding := range bindings {
			if binding.name == "_G" {
				alias, foundAlias = binding, true
				break
			}
		}
		aliasKey, aliasOK := exactString(alias.key)
		if !foundAlias || alias.root != globalRoot || !aliasOK || aliasKey != "_G" {
			return frozenLedger{}, errors.New("target/boot: global bindings require a _G self-alias binding")
		}
		entry, ok := lookupEntry(entries, globalRoot, keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"})
		if !ok || entry.mutability != vocabulary.InitialMutable || entry.value.kind != vocabulary.InitialValueRoot || entry.value.root != globalRoot {
			return frozenLedger{}, errors.New("target/boot: global bindings require a mutable _G self-alias")
		}
	}

	rootRows := make([]rootRow, len(roots))
	shapeRows := make([]shapeRow, len(roots))
	for index, root := range roots {
		value, ok := valueHandle(unique, root.value)
		if !ok {
			return frozenLedger{}, errors.New("target/boot: unresolved boot shape value")
		}
		rootRows[index] = rootRow{identity: root.identity, shape: vocabulary.BootShape(index + 1)}
		shapeRows[index] = shapeRow{root: vocabulary.InitialRoot(index + 1), aggregate: root.aggregate, immutable: root.immutable, value: value}
	}
	valueRows := make([]valueRow, len(unique))
	bindRanges := make([]bindingRange, 0)
	var bindingKeyBuilder sealedrows.PoolBuilder[vocabulary.ExactKey]
	for index, value := range unique {
		row := valueRow{kind: value.kind, boolean: value.boolean, integer: value.integer, floatBits: value.floatBits, string: value.string, root: value.root, operation: value.operation}
		if value.kind == vocabulary.InitialValueOperation {
			if value.operation == 0 {
				return frozenLedger{}, errors.New("target/boot: unresolved initial operation anchor")
			}
			if _, ok := operations.Anchor(value.operation); !ok {
				return frozenLedger{}, errors.New("target/boot: unresolved initial operation anchor")
			}
		}
		if value.kind == vocabulary.InitialValueDeniedOperation {
			_, err := appendBindingRange(&bindRanges, &bindingKeyBuilder, value.binding, keys)
			if err != nil {
				return frozenLedger{}, err
			}
			row.binding = uint32(len(bindRanges))
		}
		valueRows[index] = row
	}
	entryRows := make([]entryRow, len(entries))
	for index, entry := range entries {
		value, ok := valueHandle(unique, entry.value)
		if !ok {
			return frozenLedger{}, errors.New("target/boot: unresolved initial entry value")
		}
		key, ok := keys.Handle(entry.key)
		if !ok {
			return frozenLedger{}, errors.New("target/boot: unresolved exact key")
		}
		entryRows[index] = entryRow{root: entry.root, key: key, value: value, mutability: entry.mutability}
	}
	bindingRows := make([]bindingRow, len(bindings))
	for index, binding := range bindings {
		key, ok := keys.Handle(binding.key)
		if !ok {
			return frozenLedger{}, errors.New("target/boot: unresolved exact key")
		}
		bindingRows[index] = bindingRow{name: binding.name, root: binding.root, key: key}
	}
	metatableRows := make([]metatableRow, len(metatables))
	for index, metatable := range metatables {
		metatableRows[index] = metatableRow{base: metatable.base, metatable: metatable.metatable}
	}
	absentHandle, _ := valueHandle(unique, absent)
	return frozenLedger{
		roots: rootRows, shapes: shapeRows, values: valueRows,
		valueBinds: bindRanges, bindingKeys: bindingKeyBuilder.Seal(),
		entries: entryRows, bindings: bindingRows, metatables: metatableRows,
		globalRoot: globalRoot, absent: absentHandle,
	}, nil
}

func freezeValue(input vocabulary.InitialValueSpec, roots map[string]vocabulary.InitialRoot, operations operation.Core) (valueDraft, error) {
	invalidScalars := func() bool {
		return input.Boolean || input.Integer != 0 || input.FloatBits != 0 || input.String != "" || input.Root != "" || !emptyBinding(input.Operation)
	}
	switch input.Kind {
	case vocabulary.InitialValueNil:
		if invalidScalars() {
			return valueDraft{}, errors.New("target/boot: invalid nil initial value")
		}
		return valueDraft{kind: input.Kind}, nil
	case vocabulary.InitialValueBoolean:
		if input.Integer != 0 || input.FloatBits != 0 || input.String != "" || input.Root != "" || !emptyBinding(input.Operation) {
			return valueDraft{}, errors.New("target/boot: invalid boolean initial value")
		}
		return valueDraft{kind: input.Kind, boolean: input.Boolean}, nil
	case vocabulary.InitialValueInteger:
		if input.Boolean || input.FloatBits != 0 || input.String != "" || input.Root != "" || !emptyBinding(input.Operation) {
			return valueDraft{}, errors.New("target/boot: invalid integer initial value")
		}
		return valueDraft{kind: input.Kind, integer: input.Integer}, nil
	case vocabulary.InitialValueFloat:
		if input.Boolean || input.Integer != 0 || input.String != "" || input.Root != "" || !emptyBinding(input.Operation) {
			return valueDraft{}, errors.New("target/boot: invalid float initial value")
		}
		return valueDraft{kind: input.Kind, floatBits: input.FloatBits}, nil
	case vocabulary.InitialValueString:
		if input.Boolean || input.Integer != 0 || input.FloatBits != 0 || input.Root != "" || !emptyBinding(input.Operation) {
			return valueDraft{}, errors.New("target/boot: invalid string initial value")
		}
		return valueDraft{kind: input.Kind, string: input.String}, nil
	case vocabulary.InitialValueRoot:
		if input.Root == "" || input.Boolean || input.Integer != 0 || input.FloatBits != 0 || input.String != "" || !emptyBinding(input.Operation) {
			return valueDraft{}, errors.New("target/boot: invalid root initial value")
		}
		root, ok := roots[input.Root]
		if !ok {
			return valueDraft{}, errors.New("target/boot: foreign initial value root")
		}
		return valueDraft{kind: input.Kind, root: root}, nil
	case vocabulary.InitialValueOperation:
		if input.Boolean || input.Integer != 0 || input.FloatBits != 0 || input.String != "" || input.Root != "" || !vocabulary.ValidBinding(input.Operation) {
			return valueDraft{}, errors.New("target/boot: invalid operation initial value")
		}
		op, ok := operations.Lookup(input.Operation)
		if !ok {
			return valueDraft{}, errors.New("target/boot: foreign initial operation")
		}
		return valueDraft{kind: input.Kind, operation: op}, nil
	case vocabulary.InitialValueDeniedOperation:
		if input.Boolean || input.Integer != 0 || input.FloatBits != 0 || input.String != "" || input.Root != "" || !vocabulary.ValidBinding(input.Operation) {
			return valueDraft{}, errors.New("target/boot: invalid denied initial operation")
		}
		if _, admitted := operations.Lookup(input.Operation); admitted {
			return valueDraft{}, errors.New("target/boot: denied initial operation is admitted")
		}
		return valueDraft{kind: input.Kind, binding: cloneBinding(input.Operation)}, nil
	case vocabulary.InitialValueAbsent:
		if invalidScalars() {
			return valueDraft{}, errors.New("target/boot: invalid absent initial value")
		}
		return valueDraft{kind: input.Kind}, nil
	default:
		return valueDraft{}, errors.New("target/boot: invalid initial value kind")
	}
}

func appendBindingRange(ranges *[]bindingRange, keyBuilder *sealedrows.PoolBuilder[vocabulary.ExactKey], input vocabulary.BindingSpec, keys exactkey.Table) (uint32, error) {
	if !vocabulary.ValidBinding(input) {
		return 0, errors.New("target/boot: invalid denied binding")
	}
	row := bindingRange{namespace: input.Namespace}
	ownerKeys := make([]vocabulary.ExactKey, 0, len(input.Owner))
	for _, segment := range input.Owner {
		key, ok := keys.Handle(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: segment})
		if !ok {
			return 0, errors.New("target/boot: unresolved denied owner key")
		}
		ownerKeys = append(ownerKeys, key)
	}
	memberKeys := make([]vocabulary.ExactKey, 0, len(input.Member))
	for _, segment := range input.Member {
		key, ok := keys.Handle(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: segment})
		if !ok {
			return 0, errors.New("target/boot: unresolved denied member key")
		}
		memberKeys = append(memberKeys, key)
	}
	var ok bool
	row.ownerKeys, ok = keyBuilder.Append(ownerKeys)
	if !ok {
		return 0, errors.New("target/boot: denied owner key pool overflow")
	}
	row.memberKeys, ok = keyBuilder.Append(memberKeys)
	if !ok {
		return 0, errors.New("target/boot: denied member key pool overflow")
	}
	*ranges = append(*ranges, row)
	return uint32(len(*ranges)), nil
}
