package manifesttarget

import (
	"fmt"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"math"
	"strings"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/unwrap"
	"github.com/wippyai/go-lua/manifest"
	moduleio "github.com/wippyai/go-lua/manifest/wire"
)

type bootLedgerData struct {
	roots      []vocabulary.InitialRootSpec
	entries    []vocabulary.InitialEntrySpec
	bindings   []vocabulary.InitialBindingSpec
	metatables []vocabulary.InitialMetatableAttachmentSpec
}

// bootLedger authors the complete initial environment as ordinary Target
// data. Each operation value is resolved from the catalogue binding that was
// just authored; this avoids a second hand-maintained operation identity list.
func bootLedger(catalogue authoredCatalogue, declarations *manifest.Catalogue) (bootLedgerData, error) {
	var ledger bootLedgerData
	ledger.root(moduleio.GlobalEnvironmentRoot, vocabulary.BootAggregateTable, false)
	moduleRoots := make(map[string]string)
	for _, module := range declarations.Modules() {
		root := moduleio.ModuleRootIdentity(module.ProviderIdentity())
		moduleRoots[module.Path()] = root
		ledger.moduleRoot(module.Path(), root, vocabulary.BootAggregateTable, module.Immutable())
	}
	for _, environment := range declarations.InitialEnvironments() {
		for _, root := range environment.Roots() {
			aggregate, err := initialAggregate(root.Aggregate)
			if err != nil {
				return bootLedgerData{}, err
			}
			ledger.root(root.Identity, aggregate, root.Immutable)
		}
	}

	// Every unshadowed global slot records the same initial root/key row that
	// supplies its initial Cell value. The root aliases are regular Values.
	for _, module := range declarations.Modules() {
		ledger.global(module.Path(), rootValue(moduleRoots[module.Path()]))
	}

	if err := ledger.boundOperations(catalogue, moduleio.GlobalEnvironmentRoot, vocabulary.InitialMutable, vocabulary.BindingBuiltin, ""); err != nil {
		return bootLedgerData{}, err
	}
	for _, operation := range catalogue.operations {
		for _, binding := range operation.Bindings {
			if binding.Namespace == vocabulary.BindingBuiltin && len(binding.Member) == 1 {
				ledger.bindGlobal(binding.Member[0])
			}
		}
	}
	for _, module := range declarations.Modules() {
		mutability := vocabulary.InitialMutable
		if module.Immutable() {
			mutability = vocabulary.InitialFrozen
		}
		if err := ledger.boundOperations(catalogue, moduleRoots[module.Path()], mutability, vocabulary.BindingModule, module.Path()); err != nil {
			return bootLedgerData{}, err
		}
	}

	declaredEntries := make(map[string]struct{})
	for _, environment := range declarations.InitialEnvironments() {
		for _, entry := range environment.Entries() {
			root, err := initialRoot(environment, entry.Root, moduleRoots)
			if err != nil {
				return bootLedgerData{}, err
			}
			declaredEntries[root+"\x00"+entry.Key] = struct{}{}
		}
	}

	// Non-callable values come from the same provider export declarations as
	// their types. Singleton types preserve exact boot literals; aggregates and
	// runtime-populated values are represented as present-but-opaque entries.
	for _, value := range declarations.Values() {
		binding := value.Binding()
		member := binding.Member()
		if len(member) != 1 {
			return bootLedgerData{}, fmt.Errorf("target catalogue: initial value has non-direct binding %q", member)
		}
		root := moduleio.GlobalEnvironmentRoot
		mutability := vocabulary.InitialMutable
		switch binding.Mount() {
		case manifest.MountGlobals:
		case manifest.MountModule:
			var ok bool
			root, ok = moduleRoots[binding.ModulePath()]
			if !ok {
				return bootLedgerData{}, fmt.Errorf("target catalogue: initial value names unknown module %q", binding.ModulePath())
			}
			if value.Immutable() {
				mutability = vocabulary.InitialFrozen
			}
		case manifest.MountDetached:
			continue
		default:
			return bootLedgerData{}, fmt.Errorf("target catalogue: initial value has invalid mount %d", binding.Mount())
		}
		initial := initialValueFromType(value.Type())
		if binding.Mount() == manifest.MountGlobals && member[0] == "_G" {
			initial = rootValue(moduleio.GlobalEnvironmentRoot)
		}
		if _, explicitlyDeclared := declaredEntries[root+"\x00"+member[0]]; !explicitlyDeclared {
			ledger.entry(root, member[0], initial, mutability)
		}
		if binding.Mount() == manifest.MountGlobals {
			ledger.bindGlobal(member[0])
		}
	}

	for _, environment := range declarations.InitialEnvironments() {
		for _, entry := range environment.Entries() {
			root, err := initialRoot(environment, entry.Root, moduleRoots)
			if err != nil {
				return bootLedgerData{}, err
			}
			value, err := initialEntryValue(catalogue, environment, entry.Value, moduleRoots)
			if err != nil {
				return bootLedgerData{}, err
			}
			mutability, err := initialMutability(entry.Mutability)
			if err != nil {
				return bootLedgerData{}, err
			}
			ledger.entry(root, entry.Key, value, mutability)
		}
		for _, attachment := range environment.Metatables() {
			metatable, err := initialRoot(environment, attachment.Metatable, moduleRoots)
			if err != nil {
				return bootLedgerData{}, err
			}
			switch attachment.Primitive {
			case moduleio.InitialPrimitiveString:
				ledger.metatables = append(ledger.metatables, vocabulary.InitialMetatableAttachmentSpec{Base: vocabulary.InitialValueString, Metatable: metatable})
			default:
				return bootLedgerData{}, fmt.Errorf("target catalogue: unsupported initial primitive %d", attachment.Primitive)
			}
		}
	}

	return ledger, nil
}

func initialValueFromType(value typ.Type) vocabulary.InitialValueSpec {
	literal, ok := unwrap.Annotated(value).(*typ.Literal)
	if !ok {
		return absentValue()
	}
	switch exact := literal.Value().(type) {
	case bool:
		return vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueBoolean, Boolean: exact}
	case int64:
		return integerValue(exact)
	case float64:
		return floatValue(math.Float64bits(exact))
	case string:
		return stringValue(exact)
	default:
		return absentValue()
	}
}

func initialAggregate(aggregate moduleio.InitialAggregate) (vocabulary.BootAggregate, error) {
	switch aggregate {
	case moduleio.InitialAggregateTable:
		return vocabulary.BootAggregateTable, nil
	case moduleio.InitialAggregateMetatable:
		return vocabulary.BootAggregateMetatable, nil
	default:
		return vocabulary.BootAggregateInvalid, fmt.Errorf("target catalogue: unsupported initial aggregate %d", aggregate)
	}
}

func initialRoot(environment manifest.InitialEnvironment, reference moduleio.InitialRootReference, moduleRoots map[string]string) (string, error) {
	if reference.Module {
		root, ok := moduleRoots[environment.ModulePath()]
		if !ok {
			return "", fmt.Errorf("target catalogue: provider %q has no module root", environment.ProviderIdentity())
		}
		return root, nil
	}
	if reference.Identity == "" {
		return "", fmt.Errorf("target catalogue: provider %q names an empty initial root", environment.ProviderIdentity())
	}
	return reference.Identity, nil
}

func initialEntryValue(catalogue authoredCatalogue, environment manifest.InitialEnvironment, value moduleio.InitialValue, moduleRoots map[string]string) (vocabulary.InitialValueSpec, error) {
	switch value.Kind {
	case moduleio.InitialValueRoot:
		root, err := initialRoot(environment, value.Root, moduleRoots)
		if err != nil {
			return vocabulary.InitialValueSpec{}, err
		}
		return rootValue(root), nil
	case moduleio.InitialValueFunction:
		member := strings.Split(value.Function, ".")
		var binding vocabulary.BindingSpec
		switch environment.Mount() {
		case manifest.MountGlobals:
			binding = vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: member}
		case manifest.MountModule:
			binding = moduleBindingSpec(environment.ModulePath(), member...)
		default:
			return vocabulary.InitialValueSpec{}, fmt.Errorf("target catalogue: provider %q cannot mount an initial function", environment.ProviderIdentity())
		}
		return operationValue(catalogue, binding)
	default:
		return vocabulary.InitialValueSpec{}, fmt.Errorf("target catalogue: unsupported initial value %d", value.Kind)
	}
}

func initialMutability(mutability moduleio.InitialMutability) (vocabulary.InitialMutability, error) {
	switch mutability {
	case moduleio.InitialMutable:
		return vocabulary.InitialMutable, nil
	case moduleio.InitialFrozen:
		return vocabulary.InitialFrozen, nil
	default:
		return vocabulary.InitialMutabilityInvalid, fmt.Errorf("target catalogue: unsupported initial mutability %d", mutability)
	}
}

func (ledger *bootLedgerData) root(identity string, aggregate vocabulary.BootAggregate, immutable bool) {
	ledger.roots = append(ledger.roots, vocabulary.InitialRootSpec{
		Identity: identity,
		Shape:    vocabulary.BootShapeSpec{Aggregate: aggregate, Immutable: immutable, Value: rootValue(identity)},
	})
}

// moduleRoot emits the manifest-authored path-to-root relation as Target data.
// Value consumers query this relation by exact path during sealing; no hot
// rule derives a path from the conventional ModuleRoot identity spelling.
func (ledger *bootLedgerData) moduleRoot(path, identity string, aggregate vocabulary.BootAggregate, immutable bool) {
	ledger.roots = append(ledger.roots, vocabulary.InitialRootSpec{
		Identity:   identity,
		ModulePath: path,
		Shape:      vocabulary.BootShapeSpec{Aggregate: aggregate, Immutable: immutable, Value: rootValue(identity)},
	})
}

func (ledger *bootLedgerData) entry(root, key string, value vocabulary.InitialValueSpec, mutability vocabulary.InitialMutability) {
	ledger.entries = append(ledger.entries, vocabulary.InitialEntrySpec{Root: root, Key: exactKey(key), Value: value, Mutability: mutability})
}

func (ledger *bootLedgerData) global(key string, value vocabulary.InitialValueSpec) {
	ledger.entry(moduleio.GlobalEnvironmentRoot, key, value, vocabulary.InitialMutable)
	ledger.bindGlobal(key)
}

func (ledger *bootLedgerData) bindGlobal(key string) {
	ledger.bindings = append(ledger.bindings, vocabulary.InitialBindingSpec{Name: key, Root: moduleio.GlobalEnvironmentRoot, Key: exactKey(key)})
}

func exactKey(text string) keyspace.LiteralValue {
	return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: text}
}

// boundOperations projects the already-sealed Target bindings into boot
// entries. The provider manifest selected the ABI and operations selected the
// admission policy; boot does not carry another function-name inventory.
func (ledger *bootLedgerData) boundOperations(catalogue authoredCatalogue, root string, mutability vocabulary.InitialMutability, namespace vocabulary.BindingNamespace, owner string) error {
	for _, operation := range catalogue.operations {
		for _, binding := range operation.Bindings {
			if binding.Namespace != namespace || len(binding.Member) != 1 {
				continue
			}
			if namespace == vocabulary.BindingModule && (len(binding.Owner) != 1 || binding.Owner[0] != owner) {
				continue
			}
			if namespace == vocabulary.BindingBuiltin && len(binding.Owner) != 0 {
				continue
			}
			if err := ledger.operation(catalogue, root, binding.Member[0], mutability, binding); err != nil {
				return err
			}
		}
	}
	return nil
}

func (ledger *bootLedgerData) operation(catalogue authoredCatalogue, root, key string, mutability vocabulary.InitialMutability, binding vocabulary.BindingSpec) error {
	value, err := operationValue(catalogue, binding)
	if err != nil {
		return err
	}
	ledger.entry(root, key, value, mutability)
	return nil
}

func operationValue(catalogue authoredCatalogue, binding vocabulary.BindingSpec) (vocabulary.InitialValueSpec, error) {
	for _, operation := range catalogue.operations {
		for _, candidate := range operation.Bindings {
			if sameBinding(candidate, binding) {
				return vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueOperation, Operation: cloneCatalogueBinding(binding)}, nil
			}
		}
	}
	return vocabulary.InitialValueSpec{}, fmt.Errorf("target catalogue: initial ledger names unknown operation %#v", binding)
}

func rootValue(identity string) vocabulary.InitialValueSpec {
	return vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: identity}
}

func stringValue(value string) vocabulary.InitialValueSpec {
	return vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueString, String: value}
}

func integerValue(value int64) vocabulary.InitialValueSpec {
	return vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueInteger, Integer: value}
}

func floatValue(bits uint64) vocabulary.InitialValueSpec {
	return vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueFloat, FloatBits: bits}
}

func absentValue() vocabulary.InitialValueSpec {
	return vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueAbsent}
}

func moduleBindingSpec(owner string, member ...string) vocabulary.BindingSpec {
	return vocabulary.BindingSpec{Namespace: vocabulary.BindingModule, Owner: []string{owner}, Member: append([]string(nil), member...)}
}

func cloneCatalogueBinding(binding vocabulary.BindingSpec) vocabulary.BindingSpec {
	return vocabulary.BindingSpec{Namespace: binding.Namespace, Owner: append([]string(nil), binding.Owner...), Member: append([]string(nil), binding.Member...)}
}

func sameBinding(left, right vocabulary.BindingSpec) bool {
	if left.Namespace != right.Namespace || len(left.Owner) != len(right.Owner) || len(left.Member) != len(right.Member) {
		return false
	}
	for index := range left.Owner {
		if left.Owner[index] != right.Owner[index] {
			return false
		}
	}
	for index := range left.Member {
		if left.Member[index] != right.Member[index] {
			return false
		}
	}
	return true
}
