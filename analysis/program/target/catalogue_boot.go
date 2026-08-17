package target

import (
	"fmt"
	"math"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/unwrap"
	"github.com/wippyai/go-lua/manifest"
)

// The root identities below are the closed initial-environment ABI. They are
// names, not runtime handles: Heap owns every later table instance and write.
const (
	globalEnvRoot   = "GlobalEnvRoot"
	stringMetaRoot  = "StringMetatableRoot"
	errorMetaRoot   = "ErrorMetatableRoot"
	errorMethodRoot = "ErrorMethodRoot"
)

type bootLedgerData struct {
	roots      []InitialRootSpec
	entries    []InitialEntrySpec
	bindings   []InitialBindingSpec
	metatables []InitialMetatableAttachmentSpec
}

// bootLedger authors the complete initial environment as ordinary Target
// data. Each operation value is resolved from the catalogue binding that was
// just authored; this avoids a second hand-maintained operation identity list.
func bootLedger(catalogue authoredCatalogue, declarations *manifest.Catalogue) (bootLedgerData, error) {
	var ledger bootLedgerData
	ledger.root(globalEnvRoot, BootAggregateTable, false)
	moduleRoots := make(map[string]string)
	for _, module := range declarations.Modules() {
		root := moduleRoot(module.ProviderIdentity())
		moduleRoots[module.Path()] = root
		ledger.root(root, BootAggregateTable, module.Immutable())
	}
	if _, ok := moduleRoots["string"]; ok {
		ledger.root(stringMetaRoot, BootAggregateMetatable, false)
	}
	if _, ok := moduleRoots["errors"]; ok {
		ledger.root(errorMetaRoot, BootAggregateMetatable, true)
		ledger.root(errorMethodRoot, BootAggregateTable, true)
	}

	// Every unshadowed global slot records the same initial root/key row that
	// supplies its initial Cell value. The root aliases are regular Values.
	for _, module := range declarations.Modules() {
		ledger.global(module.Path(), rootValue(moduleRoots[module.Path()]))
	}

	if err := ledger.boundOperations(catalogue, globalEnvRoot, InitialMutable, BindingBuiltin, ""); err != nil {
		return bootLedgerData{}, err
	}
	for _, operation := range catalogue.operations {
		for _, binding := range operation.Bindings {
			if binding.Namespace == BindingBuiltin && len(binding.Member) == 1 {
				ledger.bindGlobal(binding.Member[0])
			}
		}
	}
	for _, module := range declarations.Modules() {
		mutability := InitialMutable
		if module.Immutable() {
			mutability = InitialFrozen
		}
		if err := ledger.boundOperations(catalogue, moduleRoots[module.Path()], mutability, BindingModule, module.Path()); err != nil {
			return bootLedgerData{}, err
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
		root := globalEnvRoot
		mutability := InitialMutable
		switch binding.Mount() {
		case manifest.MountGlobals:
		case manifest.MountModule:
			var ok bool
			root, ok = moduleRoots[binding.ModulePath()]
			if !ok {
				return bootLedgerData{}, fmt.Errorf("target catalogue: initial value names unknown module %q", binding.ModulePath())
			}
			if value.Immutable() {
				mutability = InitialFrozen
			}
		case manifest.MountDetached:
			continue
		default:
			return bootLedgerData{}, fmt.Errorf("target catalogue: initial value has invalid mount %d", binding.Mount())
		}
		initial := initialValueFromType(value.Type())
		if binding.Mount() == manifest.MountGlobals && member[0] == "_G" {
			initial = rootValue(globalEnvRoot)
		}
		ledger.entry(root, member[0], initial, mutability)
		if binding.Mount() == manifest.MountGlobals {
			ledger.bindGlobal(member[0])
		}
	}

	if _, ok := moduleRoots["errors"]; ok {
		if err := ledger.operation(catalogue, errorMetaRoot, "__tostring", InitialFrozen, moduleBindingSpec("errors", "Error", "__tostring")); err != nil {
			return bootLedgerData{}, err
		}
		if err := ledger.operation(catalogue, errorMetaRoot, "__concat", InitialFrozen, moduleBindingSpec("errors", "Error", "__concat")); err != nil {
			return bootLedgerData{}, err
		}
		ledger.entry(errorMetaRoot, "__index", rootValue(errorMethodRoot), InitialFrozen)
		if err := ledger.operations(catalogue, errorMethodRoot, InitialFrozen, "errors", []string{"kind", "retryable", "details", "message", "stack"}, "Error"); err != nil {
			return bootLedgerData{}, err
		}
	}

	// String's primitive metatable is distinct from the library table; it
	// shares only the exact __index alias to that library root.
	if stringRoot, ok := moduleRoots["string"]; ok {
		ledger.entry(stringMetaRoot, "__index", rootValue(stringRoot), InitialMutable)
		ledger.metatables = append(ledger.metatables, InitialMetatableAttachmentSpec{Base: InitialValueString, Metatable: stringMetaRoot})
	}

	return ledger, nil
}

func initialValueFromType(value typ.Type) InitialValueSpec {
	literal, ok := unwrap.Annotated(value).(*typ.Literal)
	if !ok {
		return absentValue()
	}
	switch exact := literal.Value().(type) {
	case bool:
		return InitialValueSpec{Kind: InitialValueBoolean, Boolean: exact}
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

func moduleRoot(provider string) string { return "ModuleRoot:" + provider }

func (ledger *bootLedgerData) root(identity string, aggregate BootAggregate, immutable bool) {
	ledger.roots = append(ledger.roots, InitialRootSpec{
		Identity: identity,
		Shape:    BootShapeSpec{Aggregate: aggregate, Immutable: immutable, Value: rootValue(identity)},
	})
}

func (ledger *bootLedgerData) entry(root, key string, value InitialValueSpec, mutability InitialMutability) {
	ledger.entries = append(ledger.entries, InitialEntrySpec{Root: root, Key: exactKey(key), Value: value, Mutability: mutability})
}

func (ledger *bootLedgerData) global(key string, value InitialValueSpec) {
	ledger.entry(globalEnvRoot, key, value, InitialMutable)
	ledger.bindGlobal(key)
}

func (ledger *bootLedgerData) bindGlobal(key string) {
	ledger.bindings = append(ledger.bindings, InitialBindingSpec{Name: key, Root: globalEnvRoot, Key: exactKey(key)})
}

func exactKey(text string) keyspace.LiteralValue {
	return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: text}
}

func (ledger *bootLedgerData) operations(catalogue authoredCatalogue, root string, mutability InitialMutability, owner string, names []string, memberPrefix ...string) error {
	for _, name := range names {
		member := append(append([]string(nil), memberPrefix...), name)
		if err := ledger.operation(catalogue, root, name, mutability, moduleBindingSpec(owner, member...)); err != nil {
			return err
		}
	}
	return nil
}

// boundOperations projects the already-sealed Target bindings into boot
// entries. The provider manifest selected the ABI and operations selected the
// admission policy; boot does not carry another function-name inventory.
func (ledger *bootLedgerData) boundOperations(catalogue authoredCatalogue, root string, mutability InitialMutability, namespace BindingNamespace, owner string) error {
	for _, operation := range catalogue.operations {
		for _, binding := range operation.Bindings {
			if binding.Namespace != namespace || len(binding.Member) != 1 {
				continue
			}
			if namespace == BindingModule && (len(binding.Owner) != 1 || binding.Owner[0] != owner) {
				continue
			}
			if namespace == BindingBuiltin && len(binding.Owner) != 0 {
				continue
			}
			if err := ledger.operation(catalogue, root, binding.Member[0], mutability, binding); err != nil {
				return err
			}
		}
	}
	return nil
}

func (ledger *bootLedgerData) operation(catalogue authoredCatalogue, root, key string, mutability InitialMutability, binding BindingSpec) error {
	value, err := operationValue(catalogue, binding)
	if err != nil {
		return err
	}
	ledger.entry(root, key, value, mutability)
	return nil
}

func operationValue(catalogue authoredCatalogue, binding BindingSpec) (InitialValueSpec, error) {
	for _, operation := range catalogue.operations {
		for _, candidate := range operation.Bindings {
			if sameBinding(candidate, binding) {
				return InitialValueSpec{Kind: InitialValueOperation, Operation: cloneCatalogueBinding(binding)}, nil
			}
		}
	}
	return InitialValueSpec{}, fmt.Errorf("target catalogue: initial ledger names unknown operation %#v", binding)
}

func rootValue(identity string) InitialValueSpec {
	return InitialValueSpec{Kind: InitialValueRoot, Root: identity}
}

func stringValue(value string) InitialValueSpec {
	return InitialValueSpec{Kind: InitialValueString, String: value}
}

func integerValue(value int64) InitialValueSpec {
	return InitialValueSpec{Kind: InitialValueInteger, Integer: value}
}

func floatValue(bits uint64) InitialValueSpec {
	return InitialValueSpec{Kind: InitialValueFloat, FloatBits: bits}
}

func absentValue() InitialValueSpec {
	return InitialValueSpec{Kind: InitialValueAbsent}
}

func moduleBindingSpec(owner string, member ...string) BindingSpec {
	return BindingSpec{Namespace: BindingModule, Owner: []string{owner}, Member: append([]string(nil), member...)}
}

func cloneCatalogueBinding(binding BindingSpec) BindingSpec {
	return BindingSpec{Namespace: binding.Namespace, Owner: append([]string(nil), binding.Owner...), Member: append([]string(nil), binding.Member...)}
}

func sameBinding(left, right BindingSpec) bool {
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
