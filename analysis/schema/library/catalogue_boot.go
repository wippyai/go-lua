package library

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/manifest"
)

// The root identities below are the closed initial-environment ABI. They are
// names, not runtime handles: Heap owns every later table instance and write.
const (
	globalEnvRoot      = "GlobalEnvRoot"
	stringMetaRoot     = "StringMetatableRoot"
	errorMetaRoot      = "ErrorMetatableRoot"
	errorMethodRoot    = "ErrorMethodRoot"
	wippyVersionString = "GopherLua 0.2 Wippy Edition"
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
	ledger.global("_G", rootValue(globalEnvRoot))
	for _, module := range declarations.Modules() {
		ledger.global(module.Path(), rootValue(moduleRoots[module.Path()]))
	}
	ledger.global("_VERSION", stringValue("Lua 5.3 - Wippy Modification"))
	ledger.global("_GOPHER_LUA_VERSION", stringValue(wippyVersionString))

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

	if mathRoot, ok := moduleRoots["math"]; ok {
		ledger.entry(mathRoot, "pi", floatValue(0x400921fb54442d18), InitialMutable)
		ledger.entry(mathRoot, "huge", floatValue(0x7ff0000000000000), InitialMutable)
		ledger.entry(mathRoot, "maxinteger", integerValue(9223372036854775807), InitialMutable)
		ledger.entry(mathRoot, "mininteger", integerValue(-9223372036854775808), InitialMutable)
	}

	if utf8Root, ok := moduleRoots["utf8"]; ok {
		ledger.entry(utf8Root, "charpattern", stringValue("[\x00-\x7F\xC2-\xF4][\x80-\xBF]*"), InitialMutable)
	}

	if errorsRoot, ok := moduleRoots["errors"]; ok {
		ledger.entry(errorsRoot, "Error", rootValue(errorMetaRoot), InitialFrozen)
		for _, item := range []struct{ key, value string }{
			{"NOT_FOUND", "NotFound"}, {"ALREADY_EXISTS", "AlreadyExists"},
			{"INVALID", "Invalid"}, {"PERMISSION_DENIED", "PermissionDenied"},
			{"UNAVAILABLE", "Unavailable"}, {"INTERNAL", "Internal"},
			{"CANCELED", "Canceled"}, {"CONFLICT", "Conflict"},
			{"TIMEOUT", "Timeout"}, {"RATE_LIMITED", "RateLimited"}, {"UNKNOWN", ""},
		} {
			ledger.entry(errorsRoot, item.key, stringValue(item.value), InitialFrozen)
		}
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

	if packageRoot, ok := moduleRoots["package"]; ok {
		for _, name := range []string{"preload", "loaders", "searchers", "loaded", "path", "cpath", "config"} {
			ledger.entry(packageRoot, name, absentValue(), InitialFrozen)
		}
	}
	return ledger, nil
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
